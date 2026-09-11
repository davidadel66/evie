from pathlib import Path
import json,importlib.util
root=Path('/Users/davidboktor/code/evie-memory-stage-5');b=Path('/tmp/evie-memory-stage5');f=b/'integrated-heldout-v2/freeze.json'
s=importlib.util.spec_from_file_location('frozen_runner',f.parent/'run.py');r=importlib.util.module_from_spec(s);s.loader.exec_module(r)
assert json.loads((b/'integrated-heldout-v2-reader/execution.json').read_text())['finished_at_utc']
out=b/'168-final-handoff-checks';out.mkdir(exist_ok=False)
manifest=json.loads((f.parent/'compiled-source-manifest.json').read_text());assert manifest=={p:r.sha256(root/p) for p in manifest}
r.write_json(out/'verified-source-manifest.json',manifest)
names=['TestAutomaticMemoryRecallSuppliesPreferenceOnFirstRequest','TestConversationSearchFindsUncompiledOriginalStatement','TestReferenceRecallKeepsEarlierRecipientAcrossUnrelatedTopicRoots','TestConversationExpansionResolvesTentativePronounFromOriginalNeighbors','TestMemorySearchTurnSuppliesConflictingClaimsAndNewerOwnerStatementWithoutOverwriting','TestConversationSearchRetirementSubtractsSharedAndOverlappingUTF8Ranges','TestMemoryReceiptHTTPKeepsOriginalVersionsThroughCorrectionRetirementRestrictionAndRestart','TestAutomaticMemoryRecallFindsUncompiledFactAndKeepsNoMatchDistinctFromUnavailable']
matrix=json.loads((f.parent/'matrix.json').read_text());ui={(t['path'],n) for row in matrix['rows'] for t in row['tests'] if not t['path'].endswith('.go') for n in t['names']}
commands=[('verify_change',['./scripts/verify-change.sh'],root),('demonstrations',['go','test','-json','./internal/agent','./internal/web','-run','^('+'|'.join(names)+')$','-count=1'],root),('ui_demonstrations',['./node_modules/.bin/vitest','run',*sorted({p.removeprefix('internal/web/ui/') for p,n in ui}),'--reporter=json','--outputFile='+str(out/'ui-results.json')],root/'internal/web/ui')]
records=[]
for label,command,cwd in commands:
 prefix=out/label
 try:r.run_command(command,cwd,prefix,r.worker_environment())
 except RuntimeError as error:print(str(error),flush=True)
 rec=json.loads(Path(str(prefix)+'.json').read_text());rec['log_sha256']=r.sha256(str(prefix)+'.log')
 if label=='demonstrations':
  events=[json.loads(line) for line in Path(str(prefix)+'.log').read_text().splitlines()]
  passed={e.get('Test') for e in events if e.get('Action')=='pass'}
  rec['required_tests']=names;rec['missing_passes']=sorted(set(names)-passed)
  rec['failed_or_skipped']=[e for e in events if e.get('Action') in ['fail','skip'] and e.get('Test') in names]
 if label=='ui_demonstrations' and (out/'ui-results.json').exists():
  result=json.loads((out/'ui-results.json').read_text());passed={(str(Path(t['name']).relative_to(root)),a['title']) for t in result['testResults'] for a in t['assertionResults'] if a['status']=='passed'}
  rec.update(required_assertions=len(ui),missing_passes=sorted(ui-passed),passed_tests=result['numPassedTests'],failed_tests=result['numFailedTests'],pending_tests=result['numPendingTests'],structured_results_sha256=r.sha256(out/'ui-results.json'))
 records.append(rec);print(json.dumps(dict(stage=label,exit_code=rec['exit_code'],missing=rec.get('missing_passes',[]))),flush=True)
assert manifest=={p:r.sha256(root/p) for p in manifest}
r.write_json(out/'handoff-verification.json',dict(freeze_sha256=r.sha256(f),compiled_source_manifest_sha256=r.sha256(out/'verified-source-manifest.json'),compiled_inputs_unchanged=True,commands=records,all_pass=all(x['exit_code']==0 and not x.get('missing_passes') and not x.get('failed_or_skipped') and not x.get('failed_tests') and not x.get('pending_tests') for x in records),note='Final repository verification and eight deterministic public-turn/HTTP demonstrations. UI component checks are identified as automated; no manual browser demonstration is claimed. Earlier frozen matrix checks remain independently retained.'))
