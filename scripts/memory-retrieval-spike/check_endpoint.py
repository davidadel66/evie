"""Observable loopback/Unix HTTP checks at the experiment's model I/O seam."""
import asyncio
import json
from pathlib import Path
import tempfile
import time

from endpoint import LocalEmbedding

async def checks():
    report=[]
    for endpoint in ["http://192.0.2.1:11434", "https://127.0.0.1:11434", "http://localhost:11434",
                     "http://127.0.0.1.example.com", "http://user:pass@127.0.0.1", "http://127.0.0.1/path", "unix:relative.sock"]:
        try: LocalEmbedding(endpoint)
        except ValueError: report.append({"check":"reject_endpoint","endpoint":endpoint,"passed":True})
        else: raise AssertionError("endpoint was accepted")
    hit_count=0
    async def sink(reader,writer):
        nonlocal hit_count
        hit_count+=1; writer.close(); await writer.wait_closed()
    sink_server=await asyncio.start_server(sink,"127.0.0.1",0)
    sink_port=sink_server.sockets[0].getsockname()[1]
    mode="valid"; requests=0
    async def handler(reader,writer):
        nonlocal requests
        try:
            header=await reader.readuntil(b"\r\n\r\n")
            headers=dict(line.lower().split(b":",1) for line in header.split(b"\r\n") if b":" in line)
            await reader.readexactly(int(headers[b"content-length"]))
            requests+=1
            if mode=="delay": await asyncio.sleep(.5)
            if mode=="redirect":
                writer.write(f"HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1:{sink_port}/leak\r\nContent-Length: 0\r\n\r\n".encode())
            else:
                payload={"model":"all-minilm:22m","embeddings":[[1.0]+[0.0]*383]}
                if mode=="wrong_dimension": payload["embeddings"]=[[1.0]]
                if mode=="wrong_count": payload["embeddings"]*=2
                if mode=="zero": payload["embeddings"]=[[0.0]*384]
                if mode=="nonfinite": payload["embeddings"][0][0]=float("nan")
                if mode=="wrong_model": payload["model"]="remote-fallback"
                body=json.dumps(payload).encode()
                if mode=="malformed_json": body=b"not json"
                writer.write(f"HTTP/1.1 200 OK\r\nContent-Length: {len(body)}\r\n\r\n".encode()+body)
            await writer.drain()
        except (ConnectionError, asyncio.IncompleteReadError): pass
        finally:
            writer.close()
            try: await writer.wait_closed()
            except ConnectionError: pass
    server=await asyncio.start_server(handler,"127.0.0.1",0)
    client=LocalEmbedding(f"http://127.0.0.1:{server.sockets[0].getsockname()[1]}")
    try:
        assert len((await client.embed_async(["synthetic eligible input"]))[0])==384
        report.append({"check":"literal_loopback_success","passed":True})
        for mode in ["redirect","wrong_dimension","wrong_count","zero","nonfinite","wrong_model","malformed_json"]:
            before=requests
            try: await client.embed_async(["synthetic eligible input"])
            except ValueError: pass
            else: raise AssertionError(mode+" accepted")
            assert requests==before+1 and hit_count==0
            report.append({"check":mode,"requests":requests-before,"redirect_destination_requests":hit_count,"passed":True})
        mode="delay"; start=time.perf_counter_ns()
        try: await client.embed_async(["synthetic eligible input"],timeout=.025)
        except TimeoutError: pass
        else: raise AssertionError("deadline ignored")
        elapsed=time.perf_counter_ns()-start; assert elapsed<250_000_000
        report.append({"check":"deadline","elapsed_ns":elapsed,"passed":True})
        task=asyncio.create_task(client.embed_async(["synthetic eligible input"]))
        await asyncio.sleep(.025); start=time.perf_counter_ns(); task.cancel()
        try: await task
        except asyncio.CancelledError: pass
        else: raise AssertionError("cancellation ignored")
        elapsed=time.perf_counter_ns()-start; assert elapsed<250_000_000
        report.append({"check":"caller_cancellation","elapsed_ns":elapsed,"passed":True})
        mode="valid"
        with tempfile.TemporaryDirectory(prefix="evie-embed-unix-") as temp:
            path=str(Path(temp)/"model.sock"); unix=await asyncio.start_unix_server(handler,path)
            try: assert len((await LocalEmbedding("unix:"+path).embed_async(["synthetic eligible input"]))[0])==384
            finally: unix.close(); await unix.wait_closed()
        report.append({"check":"unix_socket_success","passed":True})
    finally:
        server.close(); sink_server.close(); await server.wait_closed(); await sink_server.wait_closed()
    # A stopped primary endpoint cannot trigger any secondary/remote request.
    before=hit_count
    try: await client.embed_async(["synthetic eligible input"])
    except ConnectionError: pass
    else: raise AssertionError("closed local endpoint succeeded")
    assert hit_count==before
    report.append({"check":"closed_endpoint_no_fallback","fallback_requests":0,"passed":True})
    return {"version":"memory-retrieval-endpoint-probes-v1","checks":report,"passed":all(r["passed"] for r in report)}

if __name__=="__main__":
    import sys
    result=asyncio.run(checks()); Path(sys.argv[1]).write_text(json.dumps(result,indent=2)+"\n")
    print(json.dumps({"passed":result["passed"],"checks":len(result["checks"])}))
