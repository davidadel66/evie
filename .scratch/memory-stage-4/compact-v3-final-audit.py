import datetime,hashlib,json,pathlib,re,subprocess
repo=pathlib.Path.cwd(); base=repo/'cmd/evie/docs/fixtures/memory-stage-4-spike/v1'; root=base/'qwen-compact-v3'
def digest(data):return 'sha256:'+hashlib.sha256(data).hexdigest()
def sha(p):return digest(p.read_bytes())
def read(p):return json.loads(p.read_text())
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=False)
def normalize(s):return 'sha256:'+s.removeprefix('sha256:')
old=read(repo/'.scratch/memory-stage-4/pre-category-schema-artifact-baseline.json')['files']
assert len(old)==142
for name,h in old.items():assert sha(repo/name)==normalize(h),name
manifest=read(root/'experiment-manifest.json')
for name,h in manifest['file_sha256'].items():assert sha(base/name)==h,name
for field in ['source_file_sha256','support_script_sha256']:
 for name,h in manifest[field].items():assert sha(repo/name)==h,name
assert sha(pathlib.Path(manifest['inference_runner_binary']))==manifest['inference_runner_binary_sha256']
pre=read(root/'reports/predispatch.json');run=read(root/'reports/development.json')
assert run['prepared_requests']==pre['prepared_requests'];assert len(run['runs'])==10
for actual,planned in zip(run['runs'],pre['prepared_requests']):
 assert actual['compact']==planned
 assert actual['status']=='ok' and actual['server_release']=='finished_response'
 request=actual['compact']['request'];assert digest(request.encode())==actual['request_sha256']
 assert digest(actual['raw'].encode())==actual['raw_sha256']
 req=json.loads(request)
 assert digest(req['system'].encode())==actual['compact']['system_sha256']
 assert digest(canonical(req['format']).encode())==actual['compact']['schema_sha256']
 assert req['options']=={'num_ctx':8192,'num_predict':768,'seed':17,'temperature':0}
 assert actual['seed']==17 and actual['repetition']==1
pairs=read(root/'reports/paired-comparison.json');gold={c['case_id']:c for c in read(base/'development.gold.json')['cases']}
assert pairs['quality_comparison_attempts_across_all_configurations']==110
for panel in pairs['panels']:
 p=repo/panel['original_report'];assert sha(p)==panel['original_report_sha256'];source=read(p);matches=[0,0]
 for index in panel['selected_original_run_indexes']:
  item=source['runs'][index];assert item['status']=='ok'
  for axis in (0,1):
   found=set()
   for prop in item['proposals']:
    if axis and not prop['retained']:continue
    if source.get('wire_version') and not prop.get('expanded'):continue
    c=prop['candidate']
    for i,e in enumerate(gold[item['case_id']]['expected']):
     if e['label']!='required_useful':continue
     if any(c.get(k)!=v for k,v in e['meaning'].items()):continue
     if any(sorted(map(canonical,c.get(k) or []))!=sorted(map(canonical,e.get(k) or [])) for k in ['sources','context']):continue
     found.add(i)
   matches[axis]+=len(found)
 assert matches==[panel['exact_required_matches_raw_and_retained']]*2,(panel['configuration'],matches)
score=read(root/'reports/development-initial-score.json');proposed=read(root/'output-adjudications.proposed.json')
assert proposed['status']=='proposed_not_human_reviewed' and proposed['reviewer'] is None and proposed['reviewed_at'] is None
assert sha(root/proposed['packet_file'])==proposed['packet_sha256']
assert sha(root/proposed['source_score_file'])==proposed['source_score_sha256']
assert sha(base/'development.gold.json')==proposed['gold_sha256']
assert {(x['case_id'],x['candidate_sha256']) for x in proposed['decisions']}=={(x['case_id'],x['candidate_sha256']) for x in score['unadjudicated']}
assert len(proposed['decisions'])==13
assert next(x for x in proposed['decisions'] if x['id']=='C05')['candidate_sha256']=='sha256:e2332ccab62606d71bff818644dca17bfc2d6162cb370e6d1a97ab6b32055506'
out=repo/'.scratch/memory-stage-4/should-not-score-unapproved-v3.json';assert not out.exists()
command=[manifest['inference_runner_binary'],'-score',str(root/'reports/development.json'),'-adjudications',str(root/'output-adjudications.proposed.json'),'-output',str(out)]
result=subprocess.run(command,capture_output=True,text=True)
assert result.returncode!=0 and 'actual human output adjudication required' in result.stderr,result.stderr
assert not out.exists()
report=repo/'cmd/evie/docs/research/memory-stage-4-local-extractor-spike.md';readme=repo/'scripts/memory-extractor-spike/README.md'
markdown=list(root.rglob('*.md'))+[report,readme]
for path in markdown:
 text=path.read_text()
 for number,line in enumerate(text.splitlines(),1):assert line==line.rstrip(),(path,number)
 for link in re.findall(r'\]\(([^)]+)\)',text):
  if '://' in link or link.startswith('#'):continue
  target=link.split('#')[0]
  if target:assert (path.parent/target).exists(),(path,link)
check=subprocess.run(['git','diff','--check'],capture_output=True,text=True);assert check.returncode==0,check.stdout+check.stderr
shutdown=read(root/'reports/owned-shutdown.json')
assert shutdown['all_request_specific_done'] and shutdown['no_inference_outstanding'] and shutdown['all_owned_group_pids_exited'] and not shutdown['remaining_processes']
assert shutdown['development_report_sha256']==sha(root/'reports/development.json')
final=root/'reports/final-verification.json';assert not final.exists()
record={'schema_version':1,'recorded_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'preserved_prior_artifacts':len(old),'prior_artifact_hashes_match':True,'final_predispatch_requests_equal_actual':True,'raw_hashes_valid':True,'derived_schema_and_system_hashes_valid':True,'paired_exact_counts_independently_recalculated':True,'quality_comparisons':110,'pending_packet_and_proposed_record_hashes_valid':True,'pending_distinct_objects_in_packet':13,'unapproved_adjudication_rejected':{'exit_code':result.returncode,'stderr':result.stderr.strip(),'output_not_created':True},'new_file_whitespace_and_markdown_links':'passed','git_diff_check_exit_code':check.returncode,'source_and_frozen_file_pins_match':True,'owned_shutdown_sha256':sha(root/'reports/owned-shutdown.json'),'file_sha256':{str(p.relative_to(root)):sha(p) for p in sorted(root.rglob('*')) if p.is_file() and p!=final},'report_sha256':sha(report),'readme_sha256':sha(readme),'verification_record_sha256':sha(root/'reports/verification.json'),'audit_command':'python3 .scratch/memory-stage-4/compact-v3-final-audit.py','audit_script_sha256':sha(pathlib.Path(__file__))}
final.write_text(json.dumps(record,indent=2)+'\n')
print(json.dumps({'status':'passed','previous_artifacts':len(old),'frozen_v3_artifacts_excluding_receipt':len(record['file_sha256']),'final_receipt_sha256':sha(final),'raw_report_sha256':sha(root/'reports/development.json'),'no_human_labels_applied':True}))
