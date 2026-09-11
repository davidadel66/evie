"""Serialize fresh manual dev09..16 judgments. No semantic decision is inferred by this program."""
import json,pathlib,hashlib
B=pathlib.Path('/tmp/evie-memory-stage5')
P=B/'integrated-development-v4-evidence/assessment-packets'
O=B/'integrated-development-v4-assessments-experiment'
def packet(case,condition):return json.loads((P/f'{case}-{condition}.json').read_text())
def support(span,*records,basis='retrieved_evidence',context=()):
 return dict(status='supported',answer_spans=[span] if isinstance(span,str) else list(span),basis=basis,source_record_ids=list(records),source_event_ids=[],context_spans=list(context))
def missing():return dict(status='missing',answer_spans=[],basis='none',source_record_ids=[],source_event_ids=[],context_spans=[])
def prop(span,*records,basis='retrieved_evidence',context=()):
 d=support(span,*records,basis=basis,context=context);d['status']='grounded';return d
def quote(record,text):return dict(status='correct',source_record_id=record,source_event_id='',answer_span=text,quote=text)
def save(case,condition,components,propositions,cited,quotes,semantic,notes,extras=(),temporal='not_applicable',abstention='not_applicable',attribution='pass'):
 p=packet(case,condition);f=O/f'{case}-{condition}.json';d=json.loads(f.read_text());answer=p['final_answer'];bindings=p['bindings']
 assert len(components)==len(p['gold']['expected_answer_components'])
 for i,c in enumerate(components):c['index']=i
 for c in components+propositions:
  assert all(s in answer for s in c['answer_spans']),(case,condition,c)
  c['source_event_ids']=[bindings[r]['source']['event_id'] for r in c['source_record_ids']]
 for q in quotes:
  q['source_event_id']=bindings[q['source_record_id']]['source']['event_id']
  assert q['quote'] in answer and q['quote'] in bindings[q['source_record_id']]['source']['evidence'],(case,condition,q)
 citations=[]
 for r in p['gold']['support_sets'][0]:
  event=bindings[r]['source']['event_id'] if r in cited else ''
  assert not event or event in answer
  citations.append(dict(record_id=r,status='correct' if r in cited else 'missing',cited_event_id=event,answer_span=event))
 additional=[]
 for r in extras:
  event=bindings[r]['source']['event_id'];assert event in answer
  additional.append(dict(status='correct',cited_event_id=event,answer_span=event,source_record_ids=[r],source_event_ids=[event]))
 d.update(review_complete=True,components=components,personal_propositions=propositions,citations=citations,additional_citations=additional,source_quotes=quotes,behavior=dict(clarification='not_applicable',abstention=abstention,attribution=attribution,temporal=temporal,conflict='not_applicable',general='not_applicable',unnecessary_clarification=False),hard_violations=dict(fabricated_source_citations=0,retired_as_current_assertions=0,authority_or_speaker_violations=0,silent_conflict_resolutions=0),semantic_pass=semantic,notes=notes)
 f.write_text(json.dumps(d,indent=2,ensure_ascii=False)+'\n')

# Each listed baseline was individually read with its actual empty evidence/context.
baselines={
 'dev09_exact_alias':'Cannot identify the stored KIN-84 preference or original event from its available context; asks for the record without guessing Ines or a gift. No personal fact asserted.',
 'dev10_exact_entity_id':'Repeats the requested exact entity ID only to identify the lookup it cannot perform. Does not assert an identity or travel preference without original evidence.',
 'dev11_graph_bridge':'Explicitly leaves both partner identity and snack preference unsupported. The absence is limited to information available here; no invented relationship, snack or citation.',
 'dev12_dense_sleep':'Cannot identify the improvised support from absent original travel evidence and does not infer a purchased pillow or medical claim.',
 'dev13_neighbor_clamp':'Cannot verify original selection/advice events and asks for the missing excerpt. The forward-looking distinction between user decision and assistant advice is a description of the requested task, not invented advice.',
 'dev14_neighbor_tentative':'Does not invent the option or precaution and cannot establish finality without the missing conversation; asks for original records.',
 'dev15_historical_validity':'Explains that the historical venue record is unavailable. Distinguishes required valid dates from saved time in task terms, without inventing venue, dates or events.',
 'dev16_retired_suppression':'With no current memory evidence, explicitly leaves shelf and tape unknown and does not repeat retired text or invent an event.'}
for case,note in baselines.items():
 for condition in ['no_recall','recent_context']:
  save(case,condition,[missing() for _ in packet(case,condition)['gold']['expected_answer_components']],[],[],[],True,'Fresh v4 manual review. Honest-absence comparison baseline: '+note+' Missing personal components and required citations remain in their denominators.',abstention='pass',attribution='not_applicable',temporal='pass' if case.startswith(('dev15','dev16')) else 'not_applicable')
# Recent-context dev13 explicitly restates the selection supplied by the current question.
case='dev13_neighbor_clamp';condition='recent_context';span='**Your decision:** the fold-flat easel you selected.';ctx='For the fold-flat easel I selected'
save(case,condition,[support(span,basis='current_request',context=[ctx]),missing()],[prop(span,basis='current_request',context=[ctx])],[],[],True,'Fresh v4 manual review. The answer restates the owner selection explicitly supplied by the current question, but neither verifies its original event nor supplies the missing assistant advice. This earns current-request semantic support only, zero retrieved-source or citation credit. The honest-absence baseline safely asks for the source excerpt.',abstention='pass',attribution='pass')

case='dev09_exact_alias'
for condition in ['automatic','automatic_deeper','oracle','tool_only']:
 c=[support('You identified KIN-84 as Ines','dev09_ines'),support('she likes candied orange peel.','dev09_ines')]
 ps=[prop('You identified KIN-84 as Ines','dev09_ines'),prop('she likes candied orange peel.','dev09_ines')]
 save(case,condition,c,ps,['dev09_ines'],[],True,'Fresh v4 manual review. Exact KIN-84 source binds Ines; her recorded candied-orange-peel preference supports the gift. KIN-48/Hugo is not substituted. The original owner event is correctly cited; prose is a paraphrase, not a claimed quotation.')

case='dev10_exact_entity_id'
rows={
 'automatic':('Entity `f39fe4e7-9315-4641-b331-549aa742ff8e` (Soren)','preferred a **folding watercolor palette** for travel.',True),
 'automatic_deeper':('Entity `f39fe4e7-9315-4641-b331-549aa742ff8e`','preferred a **folding watercolor palette** for travel.',False),
 'oracle':('Soren (entity `f39fe4e7-9315-4641-b331-549aa742ff8e`)','preferred a **folding watercolor palette** for travel.',True),
 'tool_only':('The entity `f39fe4e7-9315-4641-b331-549aa742ff8e` (Soren)','preferred a **folding watercolor palette** for travel.',True)}
for condition,(identity,pref,quoted) in rows.items():
 save(case,condition,[support(identity,'dev10_soren'),support(pref,'dev10_soren')],[prop(identity,'dev10_soren'),prop(pref,'dev10_soren')],['dev10_soren'],[quote('dev10_soren','Soren prefers a folding watercolor palette for travel.')] if quoted else [],True,'Fresh v4 manual review. The exact requested subject ID selects Soren’s accepted record and folding watercolor palette; Vesna is not substituted. '+('The answer explicitly uses the exact requested entity ID without repeating the canonical name. That identifies the requested subject by its supplied exact ID; name-token presence is not the semantic criterion. ' if condition=='automatic_deeper' else '')+'The original owner event is correct; any displayed source quote exactly preserves source bytes.')

case='dev11_graph_bridge'
rows={
 'automatic':('You stated that Lucia’s climbing partner is Omar.','You stated that Omar prefers dried mulberries on long walks.'),
 'automatic_deeper':('You stated Lucia’s climbing partner is Omar','You stated Omar prefers dried mulberries on long walks'),
 'oracle':('You stated that Lucia’s climbing partner is Omar.','You stated that Omar prefers dried mulberries on long walks.'),
 'tool_only':("Lucia's climbing partner is Omar.",'Omar prefers dried mulberries on long walks.')}
for condition,(relationship,pref) in rows.items():
 save(case,condition,[support(relationship,'dev11_bridge'),support(pref,'dev11_preference')],[prop(relationship,'dev11_bridge'),prop(pref,'dev11_preference')],['dev11_bridge','dev11_preference'],[quote('dev11_bridge',relationship),quote('dev11_preference',pref)] if condition=='tool_only' else [],True,'Fresh v4 manual review. Both the accepted Lucia-to-Omar relationship and Omar’s dried-mulberry preference are actually delivered and correctly connected. Both original owner events are cited. The recommendation follows these two statements; Paolo’s distractor preference is not used. Automatic now has the preference source in this new run; no prior-run judgment was reused.')

case='dev12_dense_sleep'
rows={
 'automatic':'A rolled sweater under your neck helped you doze comfortably on overnight rail journeys without waking up stiff.',
 'automatic_deeper':'A **rolled sweater under your neck** helped you doze on overnight rail journeys without waking up stiff.',
 'oracle':'A rolled sweater under your neck helped you doze on overnight rail journeys without waking up stiff.',
 'tool_only':'A **rolled sweater under your neck** helped you doze on overnight rail journeys without waking up stiff.'}
for condition,span in rows.items():
 save(case,condition,[support(span,'dev12_target')],[prop(span,'dev12_target')],['dev12_target'],[],True,'Fresh v4 manual review. Correctly paraphrases the original owner travel observation: improvised rolled sweater under the neck, overnight rail dozing and avoiding stiffness. Does not invent a purchased pillow or medical diagnosis. The original owner event is correct. Tool-only earns support only after its actual conversation search, following the empty accepted search.' if condition=='tool_only' else 'Fresh v4 manual review. Correctly paraphrases the actual original owner travel observation with improvised rolled sweater, neck placement, overnight rail dozing and avoiding stiffness. Original event is correct; no medical diagnosis or purchased pillow is inferred.')

case='dev13_neighbor_clamp'
rows={
 'automatic':('You **selected the fold-flat easel** for the pop-up print stall.','the **assistant suggested tightening the blue clamp before hanging the sample frame**.','That was assembly advice—not your decision or confirmation that assembly had been done—'),
 'automatic_deeper':('You selected the fold-flat easel for the pop-up print stall.','**The assistant’s suggestion immediately afterward:** “tighten the blue clamp before hanging the sample frame.”','The assembly instruction was assistant advice—not your decision or confirmation that assembly had been performed.'),
 'oracle':('You selected the **fold-flat easel** for the pop-up print stall.','**Assistant’s suggestion immediately afterward:** “tighten the blue clamp before hanging the sample frame.”','This was assembly advice, **not confirmation that you had done it**.'),
 'tool_only':('You selected the fold-flat easel for the pop-up print stall.','**The assistant’s advice immediately afterward:** “tighten the blue clamp before hanging the sample frame.”','This was an assembly suggestion—not your decision or confirmation that the action had been performed.')}
for condition,(selection,advice,uncertain) in rows.items():
 save(case,condition,[support(selection,'dev13_anchor'),support(advice,'dev13_neighbor')],[prop(selection,'dev13_anchor'),prop(advice,'dev13_neighbor'),prop(uncertain,'dev13_neighbor')],['dev13_anchor','dev13_neighbor'],[] if condition=='automatic' else [quote('dev13_neighbor','tighten the blue clamp before hanging the sample frame.')],True,'Fresh v4 manual review. The original owner selection and immediately subsequent assistant assembly advice are both supplied with distinct events and actors. The response preserves that distinction and explicitly denies evidence of completed assembly. The quoted lower-case advice, when used, is an exact original substring.')

case='dev14_neighbor_tentative'
rows={
 'automatic':('You were considering the **square canvas stool**, explicitly as a provisional choice—not a final decision.','I suggested that, if you chose it, you should **test the rear latch on level ground before packing it**.','Neither a final selection nor a completed test is established by these sources.'),
 'automatic_deeper':('You were considering the **square canvas stool**, explicitly as “only my provisional choice”—not a final decision.','I suggested testing the **rear latch on level ground before packing it**, conditional on choosing that option.','This was a proposed precaution, **not a report of a completed test**.'),
 'oracle':('You were considering the **square canvas stool**, explicitly as a provisional choice—not a final decision.','I suggested that, if you chose it, you **test the rear latch on level ground before packing it**.','This was a proposed precaution, not a report of a completed test; no test result was established.'),
 'tool_only':('The **square canvas stool**, which you described as “still only my provisional choice.” It was **not a final decision**.','If you chose it, **test the rear latch on level ground before packing it**.','This was an assistant-proposed precaution, **not a report that a test had been completed**.')}
for condition,(selection,advice,uncertain) in rows.items():
 comps=[support(selection,'dev14_anchor'),support(advice,'dev14_neighbor'),support([selection,uncertain],'dev14_anchor','dev14_neighbor')]
 ps=[prop(selection,'dev14_anchor'),prop(advice,'dev14_neighbor'),prop(uncertain,*(['dev14_anchor','dev14_neighbor'] if condition=='automatic' else ['dev14_neighbor']))]
 qs=[];extras=[]
 if condition=='automatic_deeper':qs=[quote('dev14_anchor','only my provisional choice')]
 if condition=='tool_only':
  qs=[quote('dev14_anchor','still only my provisional choice.')];extras=['dev14_later']
  ps.append(prop('You subsequently said you would decide after checking the forecast','dev14_later'))
 save(case,condition,comps,ps,['dev14_anchor','dev14_neighbor'],qs,True,'Fresh v4 manual review. Preserves the provisional owner choice and conditional assistant precaution; no final selection or completed latch test is asserted. Original actors/events are correct and quoted source fragments retain their exact bytes. '+('The extra forecast decision statement and its owner-event citation were also actually supplied by the bounded expansion.' if extras else ''),extras=extras)

case='dev15_historical_validity'
# Automatic has current Harbor-loft data only; accurate current-data discussion cannot answer this historical question.
ps=[prop('**Harbor loft**, stated by you','dev15_current'),prop('**Recorded validity interval:** both `from` and `to` are unspecified.','dev15_current'),prop('**Statement observed:** 11 September 2026 at `05:59:08.323652Z`.','dev15_current'),prop('**Memory saved (transaction time):** 11 September 2026 at `05:59:08.325073Z`.','dev15_current')]
save(case,'automatic',[missing(),missing(),missing()],ps,[],[],False,'Fresh v4 manual review. Only the current Harbor-loft Claim and its original owner statement are supplied; the retired historical Willow-annex source is absent. The response correctly abstains from substituting the current venue, reports the supplied observation/transaction timestamps faithfully and does not treat unspecified validity as June 2022 proof. Nevertheless all three requested historical components and the required original historical citation are missing. Automatic is a retrieval comparison, not an honest-absence no-recall baseline exception; semantic result remains failure.',extras=['dev15_current'],temporal='pass',abstention='pass')
rows={
 'automatic_deeper':('The historical record identifies **Willow annex** as your rehearsal venue on **15 June 2022**.','**Recorded validity interval:** 1 March 2022 until 1 November 2022, which includes 15 June.','That memory is **retired** and is cited only as historical evidence—not as your current venue.', ['The owner statement was recorded at `2026-09-11T05:59:08.320495Z`','the memory claim was created at `2026-09-11T05:59:08.321969Z`']),
 'oracle':('Your rehearsal venue on **15 June 2022 was Willow annex**, according to the historical memory.','**Recorded validity interval:** 1 March 2022 to 1 November 2022.','This memory is **retired** and establishes a historical venue, not your current one.',['**Statement saved:** 11 September 2026 at 05:59:08 UTC—distinct from the period it describes.']),
 'tool_only':('The historical record identifies **Willow annex** as your rehearsal venue on **15 June 2022**.','**Recorded validity interval:** 1 March 2022 to 1 November 2022, which includes that date.','This memory is now **retired**; it supports the historical answer, not a claim that Willow annex is your current venue.',['**Statement saved:** 11 September 2026 at `05:59:08.321969Z`—the memory’s transaction time, not when you used the venue.'])}
for condition,(venue,interval,status,times) in rows.items():
 save(case,condition,[support(venue,'dev15_old'),support(interval,'dev15_old'),support(status,'dev15_old')],[prop(venue,'dev15_old'),prop(interval,'dev15_old'),prop(status,'dev15_old')]+[prop(s,'dev15_old') for s in times],['dev15_old'],[quote('dev15_old','Remember that my rehearsal venue was the Willow annex from 1 March 2022 until 1 November 2022.')],True,'Fresh v4 manual review. Actually delivered historical accepted Willow-annex Claim is retired now, with March-to-November 2022 validity containing the requested June date. The answer correctly separates this interval from its actual September 2026 observation/transaction time, cites the exact owner source and preserves the whole quoted statement. No historical query pin is confused with save time or a current venue assertion.',temporal='pass')

case='dev16_retired_suppression'
rows={
 'automatic':('**Parcel shelf instruction:** Use the **east entrance locker**.','**Parcel label tape:** Use **matte cream**.','Both are active in current memory; no retired instructions are included.'),
 'automatic_deeper':('**Parcel shelf instruction:** east entrance locker.','**Parcel label tape:** matte cream.','Both are active in current memory; no retired instructions are included.'),
 'oracle':('**Parcel shelf instruction:** east entrance locker.','**Parcel label tape:** matte cream.','Both are current, active memories; no retired instructions are included.'),
 'tool_only':('**Parcel shelf instruction:** east entrance locker.','**Label tape:** matte cream.','Both are marked active in current memory; no retired instructions are included.')}
for condition,(shelf,tape,status) in rows.items():
 save(case,condition,[support(shelf,'dev16_active'),support(tape,'dev16_live')],[prop(shelf,'dev16_active'),prop(tape,'dev16_live'),prop(status,'dev16_active','dev16_live')],['dev16_active','dev16_live'],[],True,'Fresh v4 manual review. Both current accepted owner facts are supplied and correctly cited with their separate original events: east entrance locker and matte cream. The retired shelf source/text is absent from actual inputs and answer. The unrelated live tape Claim remains usable. Current-status assertions match the actual active projections.',temporal='pass')

files=sorted(O.glob('*.json'));assert len(files)==48
for f in files:assert json.loads(f.read_text())['review_complete'] is True
print(json.dumps({'files':len(files),'semantic_pass':sum(json.loads(f.read_text())['semantic_pass'] for f in files),'manual_judgments_serialized':True}))
