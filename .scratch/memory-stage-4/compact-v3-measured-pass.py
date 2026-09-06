import datetime,hashlib,json,os,pathlib,signal,subprocess,time
base=pathlib.Path('cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v3');reports=base/'reports'
def now():return datetime.datetime.now(datetime.timezone.utc).isoformat()
def sha(p):return 'sha256:'+hashlib.sha256(p.read_bytes()).hexdigest()
def save(name,value):
    with (reports/name).open('x') as f:json.dump(value,f,indent=2);f.write('\n')
plan=json.loads((base/'execution-plan.json').read_text());manifest=json.loads((base/'experiment-manifest.json').read_text());start=json.loads((reports/'owned-server-start.json').read_text());pid=start['pid']
assert sha(base/'experiment-manifest.json')==plan['experiment_manifest_sha256']
assert sha(pathlib.Path(manifest['inference_runner_binary']))==manifest['inference_runner_binary_sha256']
for name,h in manifest['source_file_sha256'].items():assert sha(pathlib.Path(name))==h,name
for name,h in manifest['file_sha256'].items():assert sha(base.parent/name)==h,name
identity=subprocess.check_output(['/bin/ps','-p',str(pid),'-o','pid=,ppid=,lstart=,command='],text=True)
assert identity.split()[2:]==start['start_identity'].split()[2:],'owned process identity changed'
assert os.getpgid(pid)==start['process_group']==pid
prefix=[str(pid) if x=='OWNED_PID_FROM_START_RECORD' else x for x in plan['measurement_prefix']]
command=prefix+plan['inference_command']
assert not (reports/'development.json').exists() and not (reports/'development-resources.json').exists()
save('dispatch.json',{'at':now(),'authorization':'Parent dispatched after current metadata preflight, combined full repository verification and independent Standards/Spec passes','root_verification_sha256':sha(pathlib.Path('.scratch/memory-stage-4/category-schema-root-verification.json')),'experiment_manifest_sha256':sha(base/'experiment-manifest.json'),'execution_plan_sha256':sha(base/'execution-plan.json'),'predispatch_sha256':sha(reports/'predispatch.json'),'binary_sha256':sha(pathlib.Path(command[command.index('--')+1])),'all_frozen_inputs_matched':True,'command':command,'owned_server_pid':pid})
measure=None;engine_saved=False;observed={pid};owned_lines=[];code=None
try:
    measure=subprocess.Popen(command)
    while measure.poll() is None:
        lines=subprocess.check_output(['/bin/ps','-axo','pid,ppid,pgid,rss,command'],text=True).splitlines()
        owned_lines=[line for line in lines[1:] if int(line.split(None,4)[2])==pid]
        observed.update(int(line.split()[0]) for line in owned_lines)
        if not engine_saved:
            log=(reports/'ollama-server.log').read_text()
            if 'llama runner started in' in log:
                markers=['inference compute','starting llama server','starting go runner','offload','failed to load','msg=system','llama_init_from_model: n_ctx','llama_kv_cache_init','llama runner started in','buffer size','n_seq_max']
                selected=[line for line in log.splitlines() if any(marker in line for marker in markers)]
                assert 'starting go runner' in log and '--ctx-size 8192' in log and '--parallel 1' in log
                assert 'llama_init_from_model: n_ctx         = 8192' in log and 'offloaded 29/29 layers' in log
                save('first-load-engine.json',{'at':now(),'owned_server_pid':pid,'log':'ollama-server.log','selected_engine_lines':selected,'owned_process_tree_snapshot':owned_lines,'legacy_engine_context8192_parallel1_confirmed':True,'limitations':'Runtime backend/context/allocation diagnostics are not measured unified GPU totals. Character-level offline grammar proof is separate from this actual model load.'});engine_saved=True
                print('FIRST_LOAD: pinned legacy engine/context8192/parallel1/Metal29layers confirmed',flush=True)
        time.sleep(1)
    code=measure.wait()
finally:
    if measure is not None and measure.poll() is None:
        measure.terminate()
        try:measure.wait(timeout=8)
        except subprocess.TimeoutExpired:measure.kill();measure.wait(timeout=8)
    result=json.loads((reports/'development.json').read_text()) if (reports/'development.json').exists() else None
    done=bool(result) and all(x['server_release']=='finished_response' for x in result['runs'])
    lines=subprocess.check_output(['/bin/ps','-axo','pid,ppid,pgid,rss,command'],text=True).splitlines()
    owned_lines=[line for line in lines[1:] if int(line.split(None,4)[2])==pid];observed.update(int(line.split()[0]) for line in owned_lines)
    t=time.monotonic();os.killpg(pid,signal.SIGTERM)
    for _ in range(100):
        rows=subprocess.check_output(['/bin/ps','-axo','pid,pgid'],text=True).splitlines()[1:]
        remaining=[line for line in rows if int(line.split()[1])==pid]
        if not remaining:break
        time.sleep(.1)
    else:
        os.killpg(pid,signal.SIGKILL);time.sleep(.2)
        rows=subprocess.check_output(['/bin/ps','-axo','pid,pgid'],text=True).splitlines()[1:];remaining=[line for line in rows if int(line.split()[1])==pid]
    elapsed=time.monotonic()-t
    save('owned-shutdown.json',{'at':now(),'owned_server_pid':pid,'observed_owned_group_pids':sorted(observed),'last_owned_process_snapshot':owned_lines,'completed_requests':len(result['runs']) if result else 0,'all_request_specific_done':done,'no_inference_outstanding':done,'all_owned_group_pids_exited':not remaining,'remaining_processes':remaining,'termination_seconds':elapsed,'measurement_exit_code':code,'first_load_engine_recorded':engine_saved,'development_report_sha256':sha(reports/'development.json') if result else None,'disk':subprocess.check_output(['/bin/df','-k','.'],text=True),'host_swap':subprocess.check_output(['/usr/sbin/sysctl','vm.swapusage'],text=True),'pressure_level':subprocess.check_output(['/usr/sbin/sysctl','kern.memorystatus_vm_pressure_level'],text=True),'cache_artifacts_preserved':True})
    assert not remaining,'owned process group failed to exit'
    print(json.dumps({'measurement_exit_code':code,'attempted':len(result['runs']) if result else 0,'all_done':done,'owned_group_exited':not remaining}),flush=True)
if code!=0:raise SystemExit(code or 1)
