from pathlib import Path
import json,subprocess,hashlib,importlib.util
from datetime import datetime,timezone
base=Path('/tmp/evie-memory-stage5');root=Path('/Users/davidboktor/code/evie-memory-stage-5')
prepared=json.loads((base/'168-seal-heldout-v1-command-prepared.json').read_text());cmd=prepared['command'];runner=Path(cmd[2])
s=importlib.util.spec_from_file_location('frozen_v4_runner',runner);r=importlib.util.module_from_spec(s);s.loader.exec_module(r)
workload=Path(cmd[cmd.index('--workload')+1]);assert r.sha256(workload)==prepared['corpus_sha256']
checks=base/'integrated-heldout-v1-final-checks'
attestation=json.loads((checks/'deterministic-pending-freeze.json').read_text())
assert all(x['exit_code']==0 for x in attestation['commands'])
r.write_json(base/'integrated-heldout-v1-plan.json',dict(declared_at_utc=datetime.now(timezone.utc).isoformat(),seal_command=cmd,development_freeze_sha256=r.sha256(runner.parent/'freeze.json'),corpus_sha256=prepared['corpus_sha256'],measurement_order=['index','local','operating','reader'],failure_policy='No retry, warmup, replacement or retuning. Preserve every first attempt. Continue independent local and operating diagnostics after an execution failure; if prerequisites fail, retain the withheld-reader reason and all planned denominator failures. No failing gate may be waived.'))
def execute(command,stem):
 try:r.run_command(command,root,base/(stem+'-execution'))
 except RuntimeError as error:
  print(str(error),flush=True)
 result=json.loads((base/(stem+'-execution.json')).read_text())
 print(json.dumps(dict(stage=stem,exit_code=result['exit_code'])),flush=True)
 return result['exit_code']
if execute(cmd,'integrated-heldout-v1-seal'):raise SystemExit(1)
freeze=(base/'integrated-heldout-v1/freeze.json').resolve();f=json.loads(freeze.read_text());dev=json.loads((runner.parent/'freeze.json').read_text())
assert f['binary_sha256']==dev['binary_sha256'] and f['same_development_executable'] is True and f['partition']=='heldout'
manifest=json.loads((freeze.parent/'compiled-source-manifest.json').read_text());assert manifest==json.loads((checks/'verified-source-manifest.json').read_text())
assert manifest=={n:r.sha256(root/n) for n in manifest}
assert f['workload_sha256']==prepared['corpus_sha256'] and attestation['matrix_sha256']==f['matrix_sha256']
attestation.update(freeze_sha256=r.sha256(freeze),compiled_source_manifest_sha256=f['compiled_source_manifest_sha256'])
r.write_json(base/'integrated-heldout-v1-deterministic.json',attestation)
failures=[]
for mode in ['index','local','operating']:
 command=['python3','-B',str(freeze.parent/'run.py'),'run','--freeze',str(freeze),'--mode',mode,'--output',str(base/('integrated-heldout-v1-'+mode))]
 r.write_json(base/('integrated-heldout-v1-'+mode+'-command.json'),command)
 if execute(command,'integrated-heldout-v1-'+mode):failures.append(mode)
if failures:
 r.write_json(base/'integrated-heldout-v1-reader-not-run.json',dict(reason='Prerequisite execution failed; no reader generations started.',failed_modes=failures,recorded_at_utc=datetime.now(timezone.utc).isoformat()))
 raise SystemExit(1)
command=['python3','-B',str(freeze.parent/'run.py'),'run','--freeze',str(freeze),'--mode','reader','--output',str(base/'integrated-heldout-v1-reader')]
r.write_json(base/'integrated-heldout-v1-reader-command.json',command)
raise SystemExit(execute(command,'integrated-heldout-v1-reader'))
