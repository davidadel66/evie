from pathlib import Path
import copy,hashlib,json,subprocess
root=Path('.scratch/memory-stage-4/compact-v3-feasibility');base=Path('cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v2');binary=Path('.scratch/memory-stage-4/ollama-v0.6.3-schema-proof/proof').resolve()
schema=json.loads((base/'output.schema.json').read_text());plan=json.loads((base/'reports/predispatch.json').read_text());prefix=(base/'prompt.txt').read_text().split('Closed output schema:\n')[0]+'Closed output schema:\n'
def canonical(v):return json.dumps(v,sort_keys=True,separators=(',',':'),ensure_ascii=False)
def candidate(sources,context):return {'context':context,'effect':'assert','identity':'resolved','kind':'fact','object':'tea','object_kind':'text','polarity':'affirmed','predicate':'preference','sources':sources,'subject_entity_ref':'','subject_name':'','subject_type':'owner','temporal':''}
def makecase(name,sources,context,valid=True):return {'name':name,'json':canonical({'candidates':[candidate(sources,context)],'window_id':'w1'}),'accepted':valid}
results=[]
for rec in plan['prepared_requests']:
 req=json.loads(rec['request']);inp=json.loads(req['prompt']);support=[x['alias'] for x in rec['seal']['sources'] if x['source']['ownership']!='context'];ctx=[x['alias'] for x in rec['seal']['sources'] if x['source']['ownership']=='context'];new=copy.deepcopy(schema);props=new['properties']['candidates']['items']['properties']
 for branch in props['sources']['items']['anyOf']:branch['properties']['ref']['enum']=support
 if ctx:
  for branch in props['context']['items']['anyOf']:branch['properties']['ref']['enum']=ctx
 else:
  # const[] is intentional: the pinned converter does not enforce maxItems
  # on arrays without items. Use the verified exact constant instead.
  props['context']={'type':'array','const':[]}
 s=canonical(new);system=prefix+s;rendered='<|im_start|>system\n'+system+'<|im_end|>\n<|im_start|>user\n'+req['prompt']+'<|im_end|>\n<|im_start|>assistant\n'
 name=inp['window_id'];sp=root/(name+'.schema.json');sp.write_text(s)
 cases=[makecase('empty-context', [{'ref':support[0]}],[]),makecase('unknown-source',[{'ref':'unknown'}],[],False),makecase('unknown-context',[{'ref':support[0]}],[{'ref':'unknown'}],False),makecase('source-in-context',[{'ref':support[0]}],[{'ref':support[0]}],False)]
 for axis,aliases in [('source',support),('context',ctx)]:
  for alias in aliases:
   for label,ref in [('omitted',{'ref':alias}),('whole',{'ref':alias,'selector':'whole'}),('date',{'ref':alias,'selector':'date'}),('range',{'ref':alias,'selector':'range','start':0,'end':1})]:
    cases.append(makecase(axis+'/'+alias+'/'+label,[ref] if axis=='source' else [{'ref':support[0]}],[] if axis=='source' else [ref]))
 for alias in ctx:cases.append(makecase('context-in-source/'+alias,[{'ref':alias}],[],False))
 for alias in support:
  for label,ref in [('dangling-start',{'ref':alias,'start':0}),('whole-coords',{'ref':alias,'selector':'whole','start':0,'end':1}),('range-no-end',{'ref':alias,'selector':'range','start':0})]:cases.append(makecase(label,[ref],[],False))
 if not ctx:cases.append(makecase('nonempty-context-no-assistant',[{'ref':support[0]}],[{'ref':support[0],'selector':'whole'}],False))
 cp=root/(name+'.cases.json');cp.write_text(json.dumps(cases,indent=2)+'\n');gp=root/(name+'.gbnf');p=subprocess.run([str(binary),str(sp),str(gp),str(cp)],text=True,capture_output=True);(root/(name+'.results.txt')).write_text(p.stdout+p.stderr);assert p.returncode==0,p.stdout+p.stderr
 results.append({'case_id':name,'source_aliases':support,'context_aliases':ctx,'schema_bytes':len(s.encode()),'system_bytes':len(system.encode()),'input_bytes':len(req['prompt'].encode()),'full_rendered_bytes':len(rendered.encode()),'including_output768_and_reserve64':len(rendered.encode())+2+768+64,'grammar_bytes':gp.stat().st_size,'offline_grammar_cases':len(cases),'all_passed':True})
# Confirm that maxItems0 without items is silently ignored by this runtime,
# whereas const[] directly accepts exactly the empty array.
for label,definition,expected in [('maxItems0-no-items',{'type':'array','maxItems':0},True),('const-empty',{'type':'array','const':[]},False)]:
 sp=root/(label+'.schema.json');sp.write_text(canonical(definition));cp=root/(label+'.cases.json');cp.write_text(json.dumps([{'name':'nonempty-array','json':'[1]','accepted':expected},{'name':'empty-array','json':'[]','accepted':True}],indent=2)+'\n');p=subprocess.run([str(binary),str(sp),str(root/(label+'.gbnf')),str(cp)],text=True,capture_output=True);(root/(label+'.results.txt')).write_text(p.stdout+p.stderr);assert p.returncode==0,p.stdout+p.stderr
summary={'purpose':'Offline feasibility only; no v2 mutation, v3 production implementation, inference, download or label application','derived_from_schema_sha256':'sha256:'+hashlib.sha256((base/'output.schema.json').read_bytes()).hexdigest(),'proof_binary_sha256':'sha256:'+hashlib.sha256(binary.read_bytes()).hexdigest(),'cases':results,'maximum_context_bound':max(x['including_output768_and_reserve64'] for x in results),'minimum_context_headroom':8192-max(x['including_output768_and_reserve64'] for x in results),'offline_grammar_cases_passed':sum(x['offline_grammar_cases'] for x in results)+4,'max_grammar_bytes':max(x['grammar_bytes'] for x in results),'empty_context_constraint':'type:array,const:[]; verified actual pinned converter/parser. maxItems0 with no items is ignored by pinned runtime.'}
(root/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary,indent=2))
