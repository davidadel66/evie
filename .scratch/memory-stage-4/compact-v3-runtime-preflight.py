#!/usr/bin/env python3
"""Current artifact+metadata preflight only. No generation endpoint is called."""
import datetime,hashlib,json,os,pathlib,signal,socket,subprocess,time,urllib.request
base=pathlib.Path('cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v3')
reports=base/'reports'; root=pathlib.Path.cwd(); cached=pathlib.Path.home()/'.ollama/models'
def now(): return datetime.datetime.now(datetime.timezone.utc).isoformat()
def digest(b): return 'sha256:'+hashlib.sha256(b).hexdigest()
def file_info(p):
    h=hashlib.sha256()
    with p.open('rb') as f:
        for b in iter(lambda:f.read(1024*1024),b''): h.update(b)
    return {'sha256':'sha256:'+h.hexdigest(),'bytes':p.stat().st_size}
def save(name,v):
    with (reports/name).open('x') as f: json.dump(v,f,indent=2);f.write('\n')
manifest=json.loads((base/'runtime-manifest.json').read_text())
record={'observed_at':now(),'inference_requests_dispatched':0,'hashing':'streamed1MiB chunks','files':{}}
for name,expected in manifest['runtime_files'].items():
    actual=file_info(pathlib.Path(name));record['files'][name]=actual
    assert actual==expected,(name,actual,expected)
p=cached/'manifests/registry.ollama.ai/library/qwen2.5/7b-instruct-q4_K_M'
actual=file_info(p);assert actual['sha256']==manifest['manifest_sha256'];record['files'][str(p)]=actual
for item in manifest['artifacts']:
    p=cached/'blobs'/item['digest'].replace(':','-');actual=file_info(p);record['files'][str(p)]=actual
    assert actual=={'sha256':item['actual_sha256'],'bytes':item['actual_bytes']},str(p)
record['all_pinned_runtime_model_files_match']=True
for name,cmd in {'disk':['/bin/df','-k',str(root)],'host_swap':['/usr/sbin/sysctl','vm.swapusage'],'pressure_level':['/usr/sbin/sysctl','kern.memorystatus_vm_pressure_level']}.items(): record[name]=subprocess.check_output(cmd,text=True)
save('artifact-preflight.json',record)
with socket.socket() as sock:
    assert sock.connect_ex(('127.0.0.1',11434))!=0,'loopback port already occupied; refusing to reuse server'
env={k:v for k,v in os.environ.items() if not k.startswith('OLLAMA_') and k.lower() not in ['http_proxy','https_proxy','all_proxy','no_proxy']}
overrides={'OLLAMA_HOST':'127.0.0.1:11434','OLLAMA_NEW_ENGINE':'false','OLLAMA_NUM_PARALLEL':'1','OLLAMA_MAX_LOADED_MODELS':'1','OLLAMA_MAX_QUEUE':'1','OLLAMA_KEEP_ALIVE':'1m','OLLAMA_NOPRUNE':'true'};env.update(overrides)
command=['/Applications/Ollama.app/Contents/Resources/ollama','serve']
log=(reports/'ollama-server.log').open('xb');proc=subprocess.Popen(command,env=env,stdout=log,stderr=subprocess.STDOUT,start_new_session=True);log.close()
try:
    identity=subprocess.check_output(['/bin/ps','-p',str(proc.pid),'-o','pid=,ppid=,lstart=,command='],text=True)
    save('owned-server-start.json',{'started_at':now(),'pid':proc.pid,'process_group':os.getpgid(proc.pid),'command':command,'environment_overrides':overrides,'proxy_variables':'removed','start_identity':identity})
    client=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    for _ in range(100):
        try:
            raw=client.open('http://127.0.0.1:11434/api/version',timeout=2).read();version=json.loads(raw);break
        except Exception:
            if proc.poll() is not None:raise RuntimeError('owned server exited before metadata')
            time.sleep(.1)
    else:raise RuntimeError('metadata start timeout')
    assert version=={'version':'0.6.3'},version
    body=json.dumps({'model':manifest['model']}).encode()
    actual_raw=client.open(urllib.request.Request('http://127.0.0.1:11434/api/show',data=body,headers={'Content-Type':'application/json'}),timeout=30).read()
    with (reports/'runtime-api-actual.json').open('xb') as f:f.write(actual_raw)
    actual=json.loads(actual_raw);expected=json.loads((base/'runtime-api-metadata.json').read_text())
    actual['version']=version
    for key,value in expected.items():
        if key=='model_info':
            for field,want in value.items():
                got=actual[key][field]
                if isinstance(want,dict) and 'count' in want:
                    got={'count':len(got),'json_sha256':digest(json.dumps(got,separators=(',',':')).encode())}
                assert got==want,(key,field,got,want)
            assert set(actual[key])==set(value)
        elif key=='tensors':
            got={'count':len(actual[key]),'canonical_json_sha256':digest(json.dumps(actual[key],separators=(',',':')).encode())}
            assert got==value,(key,got,value)
        else:assert actual[key]==value,(key,actual[key],value)
    save('runtime-observations.json',{'at':now(),'owned_server_pid':proc.pid,'version':version,'all_frozen_api_metadata_matched':True,'array_hash_encoding':"json.dumps(value,separators=(',',':')) retaining API dictionary insertion order; no sorted keys",'actual_api_sha256':digest(actual_raw),'inference_requests_dispatched':0})
    print(json.dumps({'status':'metadata_ready_no_inference','owned_server_pid':proc.pid}))
except BaseException as e:
    os.killpg(proc.pid,signal.SIGTERM)
    try:proc.wait(timeout=10)
    except subprocess.TimeoutExpired:os.killpg(proc.pid,signal.SIGKILL);proc.wait(timeout=10)
    save('preflight-failure.json',{'at':now(),'error':str(e),'server_pid':proc.pid,'server_exit_code':proc.returncode,'inference_requests_dispatched':0})
    raise
