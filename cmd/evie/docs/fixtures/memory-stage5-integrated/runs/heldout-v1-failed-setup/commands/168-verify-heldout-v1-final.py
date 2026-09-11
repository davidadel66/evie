from pathlib import Path
import os,json,importlib.util,subprocess,re
root=Path('/Users/davidboktor/code/evie-memory-stage-5');base=root/'cmd/evie/docs/fixtures/memory-stage5-integrated'
s=importlib.util.spec_from_file_location('runner',Path('/private/tmp/evie-memory-stage5/integrated-development-v4/run.py'));r=importlib.util.module_from_spec(s);s.loader.exec_module(r)
out=Path('/tmp/evie-memory-stage5/integrated-heldout-v1-final-checks');out.mkdir(exist_ok=False)
matrix=Path('/private/tmp/evie-memory-stage5/integrated-development-v4/matrix.json')
m=json.loads(matrix.read_text())
manifest={n:r.sha256(root/n) for n in r.compiled_paths(root)};r.write_json(out/'verified-source-manifest.json',manifest)
go={};ui={}
for row in m['rows']:
 for test in row['tests']:
  if test['path'].endswith('.go'):go[test['name']]='./'+str(Path(test['path']).parent)
  else:
   for name in test['names']:ui[(test['path'],name)]=True
names=sorted(go);packages=sorted(set(go.values()))
commands=[('verify_change',['./scripts/verify-change.sh'],root),('go_matrix',['go','test','-json',*packages,'-run','^('+'|'.join(names)+')$','-count=1'],root),('ui_matrix',['./node_modules/.bin/vitest','run',*sorted(set(p.removeprefix('internal/web/ui/') for p,_ in ui)),'--reporter=json','--outputFile='+str(out/'ui-results.json')],root/'internal/web/ui')]
records=[]
for kind,cmd,cwd in commands:
 prefix=out/kind;r.run_command(cmd,cwd,prefix,r.worker_environment())
 rec=dict(id=kind+'_heldout_v1',kind=kind,command=cmd,cwd=str(cwd),log_path=str(prefix)+'.log',log_sha256=r.sha256(str(prefix)+'.log'),exit_code=0)
 if kind=='go_matrix':
  events=[json.loads(line) for line in Path(str(prefix)+'.log').read_text().splitlines()]
  passed={(e['Package'],e['Test']) for e in events if e.get('Action')=='pass' and 'Test' in e}
  required={('github.com/davidadel66/evie/'+go[n].removeprefix('./'),n) for n in names};assert required<=passed
  assert not [e for e in events if e.get('Action') in ['fail','skip'] and e.get('Test') in names]
  rec['matched_tests']=[dict(package=p,name=n) for p,n in sorted(required)]
 if kind=='ui_matrix':
  result=json.loads((out/'ui-results.json').read_text());assert result['numFailedTests']==0 and result['numPendingTests']==0
  passed={(str(Path(t['name']).relative_to(root)),a['title']) for t in result['testResults'] for a in t['assertionResults'] if a['status']=='passed'};assert set(ui)<=passed
  rec.update(matched_tests=[dict(path=p,name=n) for p,n in sorted(ui)],structured_results_path=str(out/'ui-results.json'),structured_results_sha256=r.sha256(out/'ui-results.json'))
 records.append(rec);print(json.dumps(dict(kind=kind,exit_code=0,matched=len(rec.get('matched_tests',[])))),flush=True)
assert manifest=={n:r.sha256(root/n) for n in manifest}
r.write_json(out/'deterministic-pending-freeze.json',dict(version=1,matrix_sha256=r.sha256(matrix),commands=records,compiled_inputs_equal_verified_checkout=True,compiled_source_manifest_sha256=r.sha256(out/'verified-source-manifest.json'),timing_note='All mapped deterministic checks and required repository verification preceded held-out sealing after the verified #167 commit; source equality is checked again when binding the completed freeze.'))
print('ALL_DETERMINISTIC_CHECKS_PASS',flush=True)
