"""Package existing synthetic evidence; never create model or human observations."""
import collections, datetime, gzip, hashlib, io, json, pathlib, statistics, subprocess, tarfile
ROOT = pathlib.Path(__file__).resolve().parents[2]
S = ROOT / '.scratch/memory-stage-4'
OUT = ROOT / 'cmd/evie/docs/fixtures/memory-stage4-pilot/v1'

def read(path): return json.loads(path.read_text())
def digest(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def write(path, data):
    with path.open('x') as file: file.write(json.dumps(data, indent=2, sort_keys=True)+'\n')
def archive(path, members):
    with path.open('xb') as raw:
        with gzip.GzipFile(filename='', mode='wb', fileobj=raw, mtime=0) as zipped:
            with tarfile.open(fileobj=zipped, mode='w') as tar:
                for name, source in sorted(members):
                    data=source.read_bytes(); info=tarfile.TarInfo(name);info.size=len(data);info.mode=0o644;info.mtime=0
                    tar.addfile(info, io.BytesIO(data))
    return {'path':path.name,'sha256':digest(path),'bytes':path.stat().st_size,'files':len(members)}

def matrix(directory, strict):
    report=read(directory/'report.json'); counts=collections.Counter(); states=collections.Counter(); outcomes=collections.Counter(); jobs=[]; foreground=[]; resolutions=[]; raw=[]
    assert report['source']['sha256']==report['source_after_sha256']
    for trial in report['runs']:
        file=directory/(trial['id']+'.json');resource=directory/(trial['id']+'.resources.json')
        assert digest(file)==trial['report_sha256'];assert digest(resource)==trial['resources_sha256']
        data=read(file); assert data['disposable_database_removed'] is True; assert data['release_eligible'] is False
        if strict:
            assert trial['status']=='passed' and trial['exit_code']==0 and not data.get('error')
            assert not data['outcome_failures'];assert all(data['counts'].get(k,0)==v for k,v in data['expected_counts'].items())
            seen=set()
            for job in data['jobs']:
                assert job['job_id'] not in seen;seen.add(job['job_id'])
                assert job['attempts']==1 and job['selected_new_events']>0
                if job['state']=='failed':assert job.get('reason')=='invalid_source_or_effect' and job['completed_new_events']==0
                else:assert job['state'] in ['completed_candidates','completed_empty'] and job['selected_new_events']==job['completed_new_events']
        counts.update(data['counts']);states.update(j['state'] for j in data['jobs']);jobs.extend(data['jobs']);foreground.extend(data['foreground']);resolutions.extend(data['scripted_resolution_nanos']);raw.append(data)
        outcomes.update(m['observed_outcome'] for j in data['jobs'] for m in j['measurements'])
    if strict:assert report['infrastructure_status']=='passed' and not report['failures'] and len(report['runs'])==99
    variants=[]
    for variant in report['variant_ids']:
        runs=[r for r in report['runs'] if r['variant']==variant]; ds=[d for d,r in zip(raw,report['runs']) if r['variant']==variant]
        row={'variant':variant,'paired_repetitions':3,'foreground_turns_per_mode':sum(len(d['foreground']) for d in ds if d['workload']['mode']=='disabled'),
             'sampled_peak_go_process_tree_rss_mib':max(r['sampled_peak_process_tree_rss_kib'] for r in runs)/1024,
             'max_freshness_seconds':max((r['metrics']['candidate_freshness_nanos']['max'] for r in runs if r['metrics']['candidate_freshness_nanos']['max'] is not None),default=None),
             'max_db_after_mib':max(d['storage_after']['db'] for d in ds)/1024**2,
             'max_wal_after_mib':max(d['storage_after']['db-wal'] for d in ds)/1024**2}
        if row['max_freshness_seconds'] is not None:row['max_freshness_seconds']/=1e9
        for mode in ['new','history']:
            for metric in ['terminal_commit_nanos','response_finalization_nanos']:
                deltas=[p['p95_delta'] for p in report['paired_deltas'] if p['variant']==variant and p['mode']==mode and p['metric']==metric]
                assert len(deltas)==3, (variant,mode,metric,len(deltas))
                row[mode+'_'+metric.replace('_nanos','')+'_median_p95_delta_ms']=statistics.median(deltas)/1e6
        variants.append(row)
    return report,raw,{'trial_count':len(report['runs']),'foreground_turns':len(foreground),'counts':dict(counts),'job_states':dict(states),'attempt_outcomes':dict(outcomes),'scripted_review_operations':len(resolutions),
        'max_terminal_commit_ms':max(x['terminal_commit_nanos'] for x in foreground)/1e6,'max_response_finalization_ms':max(x['response_finalization_nanos'] for x in foreground)/1e6,
        'scripted_resolution_median_ms':statistics.median(resolutions)/1e6,'scripted_resolution_max_ms':max(resolutions)/1e6,
        'variants':variants}

old,_,old_summary=matrix(S/'150-matrix-v1',False)
new,raw,summary=matrix(S/'150-matrix-v3',True)
conformance=read(S/'150-auto-gap-conformance/report.json')
assert conformance['status']=='passed' and not conformance['failures'] and not conformance['skipped_checks']
assert conformance['source']['sha256']==conformance['source_after_sha256']
assert new['conformance']['sha256']==digest(S/'150-auto-gap-conformance/report.json')
assert new['conformance']['source']==conformance['source']
assert digest(S/'150-matrix-v3/memory-stage4-pilot')==new['binary_sha256']
checkpoint=read(S/'150-auto-gap-checkpoint.json')
conformance_files={x['path']:x.get('sha256') for x in conformance['source']['files']}
assert all(conformance_files.get(name)==sha for name,sha in new['source']['files'].items())
assert hashlib.sha256(json.dumps(new['source']['files'],sort_keys=True,separators=(',',':')).encode()).hexdigest()==new['source']['sha256']
assert len(checkpoint['files'])==13
for name,sha in checkpoint['files'].items():
    assert digest(ROOT/name)==sha==conformance_files.get(name),name
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',checkpoint['tree']],cwd=ROOT))) as tree:
    tree_files={m.name:hashlib.sha256(tree.extractfile(m).read()).hexdigest() for m in tree.getmembers() if m.isfile() and (m.name in new['source']['files'] or m.name in checkpoint['files'])}
assert all(tree_files.get(name)==sha for name,sha in new['source']['files'].items())
assert all(tree_files.get(name)==sha for name,sha in checkpoint['files'].items())
archives=[]
for label,dirname,before,after in [('original-matrix-v1','150-matrix-v1','150-environment-before.json','150-environment-after.json'),('corrected-matrix-v3','150-matrix-v3','150-environment-v3-before.json','150-environment-v3-after.json')]:
    directory=S/dirname
    members=[(path.name,path) for path in directory.iterdir() if path.suffix in ['.json','.stderr']]
    members += [('host-before.json',S/before),('host-after.json',S/after)]
    if label.startswith('original'):members.append(('post-analysis-finding.json',S/'150-original-matrix-finding.json'))
    archives.append(archive(OUT/(label+'.tar.gz'),members))
partial=S/'150-matrix-v2'
partial_members=[(p.name,p) for p in partial.iterdir() if p.suffix in ['.json','.stderr']]
partial_members += [('interruption.json',S/'150-matrix-v2-interruption.json'),('confirmed-finding.json',S/'150-historical-gap-finding.json'),('host-before.json',S/'150-environment-v2-before.json'),('host-after.json',S/'150-environment-v2-after.json')]
archives.append(archive(OUT/'interrupted-matrix-v2.tar.gz',partial_members))
checks=[]
for name in ['150-corrected-conformance','150-corrected-conformance-v2','150-auto-gap-conformance']:
    checks += [(name+'/'+p.name,p) for p in (S/name).iterdir() if p.suffix in ['.json','.log']]
checks += [('reviews/'+name,S/name) for name in ['150-boundary-review.json','150-outcome-independent-review.json','150-spec-review-auto-gap-final.json','150-spec-review-auto-gap-final.md','150-standards-review.md','150-review-historical-gap-red.log','150-review-history-first-red.log','150-review-auto-gap-green.log','150-auto-gap-focused-verification.json']]
archives.append(archive(OUT/'corrected-conformance-evidence.tar.gz',checks))
result={'version':'memory-stage4-pilot-published-observations-v1','published_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),
 'pilot_status':'incomplete','infrastructure_status':'passed_after_correcting_confirmed_contract_failure','release_eligible':False,
 'original_finding':read(S/'150-original-matrix-finding.json'),'historical_gap_finding':read(S/'150-historical-gap-finding.json'),'partial_matrix':read(S/'150-matrix-v2-interruption.json'),'original_summary':old_summary,'corrected_summary':summary,
 'measured_tree':read(S/'150-auto-gap-checkpoint.json')['tree'],'measurement_vs_delivery':'The frozen code tree was measured before these report artifacts were published. Delivery adds report files and has a different whole-tree fingerprint. All thirteen measured ticket tooling/production files are retained unchanged; this report does not claim the final delivery commit was measured.','corrected_source_sha256':new['source']['sha256'],'corrected_binary_sha256':new['binary_sha256'],
 'corrected_matrix_report_sha256':digest(S/'150-matrix-v3/report.json'),'corrected_conformance_report_sha256':digest(S/'150-auto-gap-conformance/report.json'),
 'generation':raw[0]['generation'],'generation_id':raw[0]['generation_id'],'spike_contract':new['spike_contract'],
 'environment_before':read(S/'150-environment-v3-before.json'),'environment_after':read(S/'150-environment-v3-after.json'),
 'artifacts':archives,'chosen_model_configuration':None,'quality':new['quality'],'human_review':new['human_review'],
 'numerical_release_gates':None,'model_server_resources':None,'final_holdout':{'created':False,'exposed':False,'run':False},
 'limitations':new['limitations']+['Three repetitions do not establish a stable p95 or sustained capacity; foreground p95 uses sixteen samples per trial.',
  'Power source and charge state are recorded per experiment and differ between runs. Cache, thermal conditions and unrelated OS work were not controlled.',
  'Original and corrected matrices use different catch-up validation; their total observed intervals are not a paired before/after performance experiment.',
  'The corrected matrix includes exactly the injected invalid-output failure in each history trial. Passed infrastructure does not mean no failed historical work.',
  'Cancellation and stale/retry recovery are separate deterministic conformance evidence; this workload does not provoke them.']}
write(OUT/'infrastructure-observations.json',result)
print(json.dumps({'artifacts':archives,'summary':summary},indent=2))

def ms(value): return 'unavailable' if value is None else f'{value:.3f}'
def power(observation): return next(x['stdout'].strip() for x in observation['commands'] if x['command']==['pmset','-g','batt'])
lines=[
'# Stage 4 infrastructure pilot observations',
'',
'The final 99-trial infrastructure matrix passes exact deterministic outcome checks after two reconciliation defects were corrected. **The actual model/owner pilot and final release evaluation remain incomplete.** No adequate model, model-output adjudication, David review observations or numerical release gates were inferred from these measurements.',
'',
'All work used the real Kernel, SQLite, Agent.Send, compiler host processes and public candidate preview/resolve APIs with explicitly scripted providers and artificial data. There was no learned-model inference or human semantic judgment. Final-holdout narratives were not created, exposed or run.',
'',
f'The measured code tree is `{result["measured_tree"]}`. The final matrix binary SHA256 is `{result["corrected_binary_sha256"]}`. [The machine-readable report](infrastructure-observations.json) pins its source inventory, generation, workload, corpus/scoring-contract hashes and conformance. Publishing these report artifacts changes the delivery tree: the eventual delivery commit is not claimed to have been measured. The thirteen frozen ticket production/tooling files are preserved unchanged.',
'',
'Each of eleven workload variants has three paired repetitions of compilation disabled, explicit new-evidence processing, and new evidence competing with explicit historical catch-up. Mode order rotates. The baseline uses 10,000 archived events, 256-byte inputs, one accepted Claim, one session destination, 25ms scripted extraction delay, one worker, sixteen foreground turns and sixteen available historical roots. Variants independently change retained events to 100,000/1,000,000; source bytes to 4,096/12,000; Claims to 100/1,000; destinations to sixteen; service delay to 0/250ms; or workers to two. These factors were not combined into a capacity promise.',
'',
f'The final run contains **{summary["foreground_turns"]:,} foreground turns**, **{summary["counts"]["foreground_persisted_events"]:,} actual persisted foreground events**, **{summary["counts"]["attempts"]:,} extraction attempts** and **{summary["scripted_review_operations"]:,} scripted preview/resolve operations**. Job outcomes are {summary["job_states"].get("completed_candidates",0):,} completed-candidate, {summary["job_states"].get("completed_empty",0):,} completed-empty and {summary["job_states"].get("failed",0):,} failed. The failed jobs are exactly one deliberately injected invalid-output historical root per history trial; **unexpected job outcomes are zero**. Each real dispatch is preserved; cooperating extraction intervals did not overlap. Every disposable database was removed.',
'',
'The event denominator includes context snapshots committed by Agent.Send, even though those snapshots cannot support a memory. Completed-event counts are checked against the actual selected membership, and history counts come from the explicit history receipt. A failed historical root remains a coverage gap while later work progresses. This finite workload did not induce retries, cancellation or stale-attempt waste; those remain separately tested conformance behaviors.',
'',
'Paired foreground overhead is the median of three within-repetition p95 differences in milliseconds. Each p95 has sixteen observations and therefore equals that trial\'s maximum under nearest-rank calculation. These are small-sample observations, not stable tail-latency estimates. Negative deltas are noise, not evidence that compilation accelerates foreground work.',
'',
'| Variant | Terminal, new | Terminal, history | Finalization, new | Finalization, history |',
'|---|---:|---:|---:|---:|']
for v in summary['variants']:
    lines.append('| '+v['variant']+' | '+' | '.join(ms(v[mode+'_'+metric+'_median_p95_delta_ms']) for metric,mode in [('terminal_commit','new'),('terminal_commit','history'),('response_finalization','new'),('response_finalization','history')])+' |')
lines += ['',
f'Across all final foreground samples, the largest terminal-event commit was {ms(summary["max_terminal_commit_ms"])}ms and the largest host response finalization was {ms(summary["max_response_finalization_ms"])}ms. The host finalizer follows an actual write to os.DevNull; it excludes browser, SSE, network and conversational-provider latency. Scripted public preview/resolve median was {ms(summary["scripted_resolution_median_ms"])}ms, maximum {ms(summary["scripted_resolution_max_ms"])}ms. This is database resolution cost, not active human review time.',
'',
'| Variant | Maximum candidate freshness, seconds | Sampled peak Go process-tree RSS, MiB | Maximum DB after, MiB | Maximum WAL after, MiB |',
'|---|---:|---:|---:|---:|']
for v in summary['variants']:
    lines.append('| '+v['variant']+' | '+' | '.join(ms(v[k]) for k in ['max_freshness_seconds','sampled_peak_go_process_tree_rss_mib','max_db_after_mib','max_wal_after_mib'])+' |')
lines += ['',
'Retained archived data is inserted in bounded transactions before the measured foreground interval. The million-event variant grows the real indexed database; foreground conversations remain separate small sessions. One busy destination and sixteen destinations test different workload distributions, while Workspace/project authorization is separately covered by deterministic conformance. RSS/CPU are process-tree samples at 100ms, including setup, and can miss peaks. There is no model server, so model-server resource observations are null.',
'',
'Source-arrival, observed compilation and candidate-arrival rates are retained per trial in the raw matrix report. Finite catch-up completion is not a sustained capacity measurement. Owner review capacity, active seconds, candidates per useful accepted change, supported useful precision and required-memory recall remain unavailable. Scripted approval rate or an empty inbox is not a quality score.',
'',
'The host is an Apple M3 Pro with 18 GiB RAM and eleven CPUs, macOS 15.7.8. Exact power observations for the final matrix:',
'', '```text',power(result['environment_before']),power(result['environment_after']),'```','',
'OS cache, thermal state and unrelated operating-system activity were uncontrolled. The original matrix ran on battery; the final matrix ran under the power conditions above. Original and final total intervals also use different catch-up validation. These experiments are not a paired before/after benchmark of the correction.',
'',
'The first complete matrix produced 111 unexpected zero-attempt empty-selection failures across 54 trials, although its original command-level checks returned success. That matrix is disqualified. Source capture could seal a root before discovery reached its last member; later-root coordinates then manufactured an empty suffix during reconsideration. The correction preserves exact root boundaries and reuses already captured members, including sparse coordinates, pre-activation roots and interleaved late members.',
'',
'The second matrix was interrupted after twenty complete trial receipts when independent review reproduced another ownership gap: A1..2 and an explicitly owned historical A5..6 surround B3..4. Both automatic reconciliation orders now record a proven zero-member gap as excluded/no_root_members bookkeeping with no job or coverage. Tests verify both orders, three database reopen cycles, unchanged historical ownership/lane, no false B coverage, genuine later A progress and unchanged direct explicit-selection errors. The partial run retains nineteen complete resource receipts; sampling for the twentieth trial was interrupted. It is not used for paired conclusions.',
'',
'The runner now waits for discovery/materialization to settle and validates exact jobs, attempts, dispatches, candidates, states, selected events and completed coverage. It retains expected counts and failures alongside raw evidence. Fresh browser/full conformance and both independent review axes passed on the measured tree before the final matrix. One prior conformance run failed only because the browser receipt path was mistyped; that failed report is preserved beside the subsequent valid runs.',
'',
'Artifacts preserve the original evidence and denominators without database files or executables:',
'']
for artifact in archives: lines.append(f'- [{artifact["path"]}]({artifact["path"]}): {artifact["bytes"]:,} bytes; SHA256 `{artifact["sha256"]}`.')
lines += ['',
'To inspect raw receipts, extract an archive with `tar -xzf ARCHIVE -C NEW_DIRECTORY`. Its report.json and plan.json identify every workload command and receipt hash. The interrupted archive instead contains interruption.json and the retained partial receipts. [The runner instructions](../../../../../../scripts/memory-stage4-pilot/README.md) reproduce the experiment from a frozen code checkout after complete matching conformance; no threshold is supplied by this report.',
'',
'[Owner-session preparation](preparation.json) and the explicit active-time recorder are ready for actual sessions once a development configuration and output adjudication are available. Numerical quality, recall, foreground, freshness, resource and review gates must be frozen from that actual pilot before final-holdout exposure or ongoing enablement. Tickets #150 and #151 experimental acceptance stays pending.',
'']
with (OUT/'infrastructure-results.md').open('x') as file:file.write('\n'.join(lines))
