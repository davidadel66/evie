"""Actual #165 local query-to-Kernel-evidence measurement, never a model judge."""
import argparse
import asyncio
import hashlib
import importlib.metadata
import json
import os
from pathlib import Path
import platform
import sqlite3
import subprocess
import sys
import threading
import time

import hnswlib
import numpy as np
import psutil

from endpoint import LocalEmbedding

SCOPES = ["global", "general", "workspace", "project_a", "project_b"]
KINDS = ["accepted_memory", "conversation_excerpt"]
METHODS = ["lexical", "sqlite_dense", "hnsw_dense", "sqlite_hybrid", "hnsw_hybrid"]

def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

def write(path, value):
    Path(path).write_text(json.dumps(value, indent=2, allow_nan=False) + "\n")

def samples(values):
    values = sorted(int(x) for x in values)
    return {"count": len(values), "p50": values[(len(values)-1)//2],
            "p95": values[max(0, int(np.ceil(len(values)*.95))-1)], "max": values[-1], "samples": values}

class MemoryMonitor:
    def __init__(self, pids):
        self.pids, self.rows, self.done = pids, [], threading.Event()
        self.thread = threading.Thread(target=self.run, daemon=True)
        self.thread.start()

    def run(self):
        start = time.perf_counter_ns()
        while not self.done.is_set():
            processes = {}
            for pid in self.pids:
                try:
                    process = psutil.Process(pid)
                    for p in [process] + process.children(recursive=True):
                        processes[p.pid] = p.memory_info().rss
                except (psutil.NoSuchProcess, psutil.AccessDenied):
                    pass
            self.rows.append({"elapsed_ns": time.perf_counter_ns()-start, "rss_by_pid": processes, "total_rss_bytes": sum(processes.values())})
            self.done.wait(.02)

    def finish(self):
        self.done.set(); self.thread.join()
        return {"sampling_interval_ms": 20, "peak_combined_rss_bytes": max(r["total_rss_bytes"] for r in self.rows), "samples": self.rows}

class Kernel:
    def __init__(self, binary, corpus, state):
        self.process = subprocess.Popen([str(binary), str(corpus), str(state)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)

    def call(self, **request):
        self.process.stdin.write(json.dumps(request)+"\n"); self.process.stdin.flush()
        line = self.process.stdout.readline()
        if not line:
            raise RuntimeError("Kernel experiment process terminated")
        response = json.loads(line)
        if response.get("error"):
            raise RuntimeError(response["error"])
        return response

    def close(self):
        self.process.stdin.close()
        self.process.wait(timeout=10)
        if self.process.returncode != 0:
            raise RuntimeError("Kernel failed during shutdown")

def new_hnsw(vectors, config):
    index = hnswlib.Index(space="cosine", dim=config["dimensions"])
    index.init_index(max_elements=len(vectors), M=config["hnsw_M"], ef_construction=config["hnsw_ef_construction"], random_seed=config["seed"])
    index.set_num_threads(1)
    index.add_items(vectors, np.arange(len(vectors)), num_threads=1)
    index.set_ef(config["hnsw_ef_search"])
    return index

def run(args):
    out, state = Path(args.output), Path(args.state)
    out.mkdir(parents=True, exist_ok=False); state.mkdir(parents=True, exist_ok=False)
    freeze = json.loads(Path(args.freeze).read_text()); config = freeze["configuration"]
    fixture_dir = Path(args.fixtures)
    # Reject artifact changes before opening this partition's query file.
    for name, expected in freeze["fixture_sha256"].items():
        if digest(fixture_dir / name) != expected:
            raise RuntimeError("fixture changed after freeze")
    if digest(args.broker) != freeze["broker_sha256"]:
        raise RuntimeError("Kernel binary changed after freeze")
    for name, expected in freeze["script_sha256"].items():
        if digest(Path(__file__).parent / name) != expected:
            raise RuntimeError("experiment code changed after freeze")
    cases = json.loads((fixture_dir / (args.partition+".json")).read_text())
    embedder = LocalEmbedding(args.endpoint, config["model"], config["dimensions"])
    started = time.perf_counter_ns()
    kernel = Kernel(args.broker, fixture_dir / "corpus.json", state / "kernel")
    monitor = MemoryMonitor([os.getpid(), kernel.process.pid, args.server_pid])
    try:
        groups, unique = {}, {}
        for scope in SCOPES:
            for kind in KINDS:
                rows = kernel.call(Action="corpus", Scope=scope, Kind=kind)["items"]
                rows.sort(key=lambda r:r["id"])
                for row in rows:
                    if row["id"].startswith("secret-"):
                        raise RuntimeError("secret fixture reached embedding input")
                    unique[row["evidence"]["text"]] = None
                groups[scope+"/"+kind] = rows
        setup_ns = time.perf_counter_ns()-started
        texts = sorted(unique)
        started=time.perf_counter_ns(); embedding_batches=[]
        for pos in range(0,len(texts),config["batch_size"]):
            batch=texts[pos:pos+config["batch_size"]]; tick=time.perf_counter_ns()
            vectors=embedder.embed(batch)
            embedding_batches.append({"count":len(batch),"elapsed_ns":time.perf_counter_ns()-tick})
            for text, vector in zip(batch,vectors,strict=True): unique[text]=np.asarray(vector,dtype=np.float32)
        embedding_ns=time.perf_counter_ns()-started
        dbpath=state/"vectors.sqlite"; db=sqlite3.connect(dbpath)
        started=time.perf_counter_ns()
        db.execute("CREATE TABLE vectors (actor_kind TEXT, id TEXT, vector BLOB, PRIMARY KEY(actor_kind,id))")
        matrices={}
        for key,rows in groups.items():
            matrix=np.stack([unique[r["evidence"]["text"]] for r in rows]); matrices[key]=matrix
            db.executemany("INSERT INTO vectors VALUES (?,?,?)",[(key,r["id"],v.tobytes()) for r,v in zip(rows,matrix,strict=True)])
        db.commit(); sqlite_build_ns=time.perf_counter_ns()-started
        started=time.perf_counter_ns(); indexes={}
        for key,matrix in matrices.items():
            indexes[key]=new_hnsw(matrix,config)
            indexes[key].save_index(str(state/(key.replace("/","-")+".hnsw")))
        hnsw_build_ns=time.perf_counter_ns()-started
        # Actual persistence round-trip; ef is deliberately restored explicitly.
        db.close(); started=time.perf_counter_ns(); db=sqlite3.connect(dbpath)
        sqlite_restart_count=db.execute("SELECT count(*) FROM vectors").fetchone()[0]
        sqlite_restart_ns=time.perf_counter_ns()-started
        indexes.clear(); started=time.perf_counter_ns()
        for key in matrices:
            index=hnswlib.Index(space="cosine",dim=config["dimensions"])
            index.load_index(str(state/(key.replace("/","-")+".hnsw")))
            index.set_ef(config["hnsw_ef_search"]); index.set_num_threads(1); indexes[key]=index
        hnsw_restart_ns=time.perf_counter_ns()-started
        # Rebuild uses persisted SQLite vectors, not freshly generated random data.
        started=time.perf_counter_ns(); rebuilt={}
        for key in matrices:
            rows=db.execute("SELECT id,vector FROM vectors WHERE actor_kind=? ORDER BY id",(key,)).fetchall()
            rebuilt[key]=new_hnsw(np.stack([np.frombuffer(r[1],dtype=np.float32) for r in rows]),config)
        hnsw_rebuild_ns=time.perf_counter_ns()-started
        traces=[]; restart_comparisons=[]
        def dense(case, method):
            tick=time.perf_counter_ns(); vector=np.asarray(embedder.embed([case["query"]])[0],dtype=np.float32); embed_ns=time.perf_counter_ns()-tick
            key=case["scope"]+"/"+case["kind"]; tick=time.perf_counter_ns()
            if method=="sqlite":
                stored=db.execute("SELECT id,vector FROM vectors WHERE actor_kind=? ORDER BY id",(key,)).fetchall()
                matrix=np.stack([np.frombuffer(r[1],dtype=np.float32) for r in stored])
                scores=matrix@vector; ordered=sorted(range(len(stored)),key=lambda i:(-float(scores[i]),stored[i][0]))[:config["candidates"]]
                ids=[stored[i][0] for i in ordered if scores[i]>=config["min_cosine"]]
            else:
                labels,distances=indexes[key].knn_query(vector,k=min(config["candidates"],len(groups[key])),num_threads=1)
                ids=[groups[key][int(i)]["id"] for i,d in zip(labels[0],distances[0],strict=True) if 1-float(d)>=config["min_cosine"]]
                labels2,_=rebuilt[key].knn_query(vector,k=min(config["candidates"],len(groups[key])),num_threads=1)
                restart_comparisons.append(bool(np.array_equal(labels,labels2)))
            rank_ns=time.perf_counter_ns()-tick
            validated=kernel.call(Action="validate",Scope=case["scope"],IDs=ids,Limit=8,MaxBytes=24576)
            return validated["items"],embed_ns,rank_ns
        for repetition in range(config["repetitions"]):
            for case_index,case in enumerate(cases):
                # Rotate condition order without using outcomes, reducing warmup/order bias.
                rotation=(case_index+repetition)%len(METHODS)
                for method in METHODS[rotation:]+METHODS[:rotation]:
                    tick=time.perf_counter_ns(); embed_ns=rank_ns=0
                    if method=="lexical":
                        result=kernel.call(Action="search",Scope=case["scope"],Kind=case["kind"],Query=case["query"],Limit=config["result_limit"],MaxBytes=config["context_bytes"])
                    else:
                        rows,embed_ns,rank_ns=dense(case,method.split("_")[0])
                        if method.endswith("hybrid"):
                            lexical=kernel.call(Action="search",Scope=case["scope"],Kind=case["kind"],Query=case["query"],Limit=8,MaxBytes=24576)["items"]
                            scores={}
                            for ranking in [rows,lexical]:
                                for rank,row in enumerate(ranking,1): scores[row["id"]]=scores.get(row["id"],0)+1/(config["rrf_k"]+rank)
                            ids=sorted(scores,key=lambda i:(-scores[i],i))[:8]
                        else: ids=[row["id"] for row in rows]
                        result=kernel.call(Action="validate",Scope=case["scope"],IDs=ids,Limit=config["result_limit"],MaxBytes=config["context_bytes"])
                    elapsed=time.perf_counter_ns()-tick; ids=[row["id"] for row in result["items"]]
                    forbidden=set(ids)&set(case["forbidden"])
                    if forbidden or result["bytes"]>config["context_bytes"] or len(ids)>config["result_limit"]:
                        raise RuntimeError("scope/context gate failed")
                    traces.append({"case":case["id"],"category":case["category"],"method":method,"repetition":repetition,"ids":ids,
                                   "expected":case["expected"],"matched":len(set(ids)&set(case["expected"])),"forbidden_count":len(forbidden),
                                   "elapsed_ns":elapsed,"embedding_ns":embed_ns,"vector_ranking_ns":rank_ns,"context_bytes":result["bytes"]})
        summaries={}
        for method in METHODS:
            rows=[r for r in traces if r["method"]==method]
            summaries[method]={"evidence_recall":sum(r["matched"] for r in rows)/sum(len(r["expected"]) for r in rows),
                "paraphrase_recall":float(np.mean([r["matched"] for r in rows if r["category"]=="paraphrase"])),
                "lexical_control_recall":float(np.mean([r["matched"] for r in rows if r["category"]=="lexical_control"])),
                "latency_ns":samples([r["elapsed_ns"] for r in rows]),"embedding_ns":samples([r["embedding_ns"] for r in rows]),
                "vector_ranking_ns":samples([r["vector_ranking_ns"] for r in rows]),"context_bytes":samples([r["context_bytes"] for r in rows])}
        agreement=[]
        for case in cases:
            exact=next(r["ids"] for r in traces if r["case"]==case["id"] and r["method"]=="sqlite_dense")
            approximate=next(r["ids"] for r in traces if r["case"]==case["id"] and r["method"]=="hnsw_dense")
            agreement.append(len(set(exact)&set(approximate))/max(1,len(exact)))
        report={"partition":args.partition,"freeze_sha256":digest(args.freeze),"started_from_parent":freeze["git_revision"],
            "cases":len(cases),"families":len(set(c["family"] for c in cases)),"configuration":config,"unique_embedded_texts":len(texts),
            "eligible_rows_by_actor_kind":{k:len(v) for k,v in groups.items()},"kernel_fixture_and_export_ns":setup_ns,
            "embedding_build_ns":embedding_ns,"embedding_batches":embedding_batches,"sqlite_build_ns":sqlite_build_ns,"hnsw_build_ns":hnsw_build_ns,
            "sqlite_restart_ns":sqlite_restart_ns,"sqlite_restart_rows":sqlite_restart_count,"hnsw_restart_ns":hnsw_restart_ns,
            "hnsw_rebuild_from_sqlite_ns":hnsw_rebuild_ns,"hnsw_restart_rebuild_query_agreement":sum(restart_comparisons)/len(restart_comparisons),
            "hnsw_vs_sqlite_delivered_set_recall":sum(agreement)/len(agreement),"sqlite_bytes":dbpath.stat().st_size,
            "hnsw_bytes":sum(p.stat().st_size for p in state.glob("*.hnsw")),"methods":summaries}
        db.close(); write(out/"traces.json",traces); write(out/"report.json",report)
        print(json.dumps({"partition":args.partition,"methods":{m:{"recall":s["evidence_recall"],"paraphrase":s["paraphrase_recall"],"p95_ms":s["latency_ns"]["p95"]/1e6} for m,s in summaries.items()}}),flush=True)
    finally:
        write(out/"resources.json",monitor.finish()); kernel.close()

def freeze(args):
    output=Path(args.output)
    if output.exists(): raise RuntimeError("never overwrite a frozen configuration")
    directory=Path(args.fixtures)
    model_manifest=Path(args.models)/"manifests/registry.ollama.ai/library/all-minilm/22m"
    manifest=json.loads(model_manifest.read_text())
    layers=[]
    for layer in manifest["layers"]:
        path=Path(args.models)/"blobs"/layer["digest"].replace(":","-")
        if digest(path)!=layer["digest"].split(":")[1]: raise RuntimeError("model blob digest mismatch")
        layers.append({"media_type":layer["mediaType"],"sha256":digest(path),"bytes":path.stat().st_size})
    configuration={"model":"all-minilm:22m","dimensions":384,"dtype":"float32","distance":"cosine","min_cosine":.25,
        "batch_size":16,"candidates":8,"result_limit":4,"context_bytes":8192,"rrf_k":60,"repetitions":3,
        "hnsw_M":16,"hnsw_ef_construction":100,"hnsw_ef_search":64,"seed":165,"hnsw_threads":1,
        "embedding_num_thread":4,"embedding_truncate":False,"embedding_keep_alive":"30s","embedding_deadline_seconds":10,
        "selection_gates":{"zero_scope_or_secret_leaks":True,"dense_paraphrase_recall_min":.85,"dense_paraphrase_improvement_over_lexical_min":.10,
                           "full_query_p95_ms_max":250,"hnsw_delivered_set_recall_min":.98,"restart_rebuild_agreement_min":1,
                           "hnsw_selection_requires_full_query_p95_improvement_fraction":.20}}
    write(output,{"version":"memory-retrieval-spike-v1","frozen_at_utc":time.strftime("%Y-%m-%dT%H:%M:%SZ",time.gmtime()),
        "partition_policy":"whole synthetic source/topic families; corpus indexed before evaluation; heldout questions excluded from development; separate from #167/#168",
        "configuration":configuration,"git_revision":subprocess.check_output(["git","rev-parse","HEAD"],text=True).strip(),
        "broker_sha256":digest(args.broker),"script_sha256":{p.name:digest(p) for p in Path(__file__).parent.glob("*.py")},
        "fixture_sha256":{n:digest(directory/n) for n in ["corpus.json","development.json","heldout.json"]},
        "hardware":{"platform":platform.platform(),"machine":platform.machine(),"physical_memory_bytes":psutil.virtual_memory().total,
                    "physical_cpu_count":psutil.cpu_count(logical=False),"logical_cpu_count":psutil.cpu_count(),
                    "model":subprocess.check_output(["sysctl","-n","hw.model"],text=True).strip()},
        "dependencies":{"python":platform.python_version(),"numpy":importlib.metadata.version("numpy"),"hnswlib":importlib.metadata.version("hnswlib"),
                        "psutil":importlib.metadata.version("psutil"),"sqlite":sqlite3.sqlite_version,"ollama":"0.6.3"},
        "model_manifest_sha256":digest(model_manifest),"model_manifest":manifest,"model_layers":layers})

if __name__=="__main__":
    parser=argparse.ArgumentParser(); sub=parser.add_subparsers(dest="command",required=True)
    freezing=sub.add_parser("freeze"); freezing.add_argument("--models",required=True)
    running=sub.add_parser("run"); running.add_argument("--partition",choices=["development","heldout"],required=True)
    running.add_argument("--freeze",required=True); running.add_argument("--state",required=True); running.add_argument("--server-pid",type=int,required=True)
    running.add_argument("--endpoint",default="http://127.0.0.1:11565")
    for child in [freezing,running]:
        child.add_argument("--broker",required=True); child.add_argument("--fixtures",required=True); child.add_argument("--output",required=True)
    arguments=parser.parse_args(); freeze(arguments) if arguments.command=="freeze" else run(arguments)
