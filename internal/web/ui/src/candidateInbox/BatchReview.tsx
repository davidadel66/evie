import { useState } from "react";
import type { BatchRequest, OwnerCandidate, ReviewDependency } from "../api/candidateReview";
import type { CandidateInboxController, InboxState } from "./controller";
import { DependencyDisclosure, ProposalMeaning, ReviewEffects, CanonicalDisclosure } from "./ReviewDisclosure";
import { supportedBatch } from "./previewSupport";

const button = "border-hair-input text-body hover:bg-hover rounded border px-3 py-2 text-xs disabled:opacity-40";
const input = "border-hair-input bg-app rounded border px-2 py-1 text-xs";
type Row = { item: OwnerCandidate; group: string; action: "accept" | "reject" };

export function BatchReview({ state, controller }: { state: InboxState; controller: CandidateInboxController }) {
 const [rows, setRows] = useState<Row[]>([]); const [dependencies, setDependencies] = useState<ReviewDependency[]>([]);
 const [target, setTarget] = useState(""); const [provider, setProvider] = useState(""); const [field, setField] = useState("subject"); const [fromField, setFromField] = useState("subject"); const [problem, setProblem] = useState("");
 const changed = () => { controller.invalidateBatchDraft(); setProblem(""); };
 const add = () => {
  const item = state.detail; if (!item) return; changed();
  const index = rows.findIndex((row) => row.item.ref.candidate_id === item.ref.candidate_id);
  if (index >= 0) setRows(rows.map((row, i) => i === index ? { ...row, item } : row));
  else setRows([...rows, { item, group: `group-${rows.length + 1}`, action: "accept" }]);
 };
 const prepare = () => {
  const request: BatchRequest = { groups: [] };
  for (const row of rows) {
   let group = request.groups.find((entry) => entry.group_id === row.group);
   if (!group) { group = { group_id: row.group, action: row.action, candidates: [], dependencies: [] }; request.groups.push(group); }
   if (group.action !== row.action) { setProblem("Every candidate in one atomic group must have the same action. Choose matching actions before preparing."); return; }
   group.candidates.push(row.item.ref);
  }
  for (const dependency of dependencies) {
   const group = request.groups.find((entry) => entry.candidates.some((ref) => ref.candidate_id === dependency.candidate_id));
   if (!group) { setProblem("A dependency refers to an omitted candidate. Restore its complete group before preparing."); return; }
   group.dependencies.push(dependency);
  }
  void controller.prepareBatch(request);
 };
 if (!state.scope) return null;
 return <section aria-label="Batch review" className="border-hair my-4 space-y-3 rounded border p-4">
  <h3 className="text-ink font-medium">Review dependent candidates together</h3>
  <p className="text-muted-text text-xs">Add inspected candidates, assign the same group name to changes that must succeed together, and explicitly bind any shared new identity. Separate groups may succeed independently. Nothing is accepted until the complete batch preview is confirmed.</p>
  <button className={button} type="button" disabled={state.busy || !state.detail || state.detail.candidate.review_state !== "unresolved" || rows.length >= 64 && !rows.some((row) => row.item.ref.candidate_id === state.detail?.ref.candidate_id)} onClick={add}>{rows.some((row) => row.item.ref.candidate_id === state.detail?.ref.candidate_id) ? "Replace selection with inspected revision" : "Add inspected candidate to batch"}</button>
  {rows.map((row, index) => <section className="border-hair space-y-2 border-t pt-3" key={row.item.ref.candidate_id}><p className="break-all text-xs">Selected {index + 1}: {row.item.ref.candidate_id} · interpretation {row.item.ref.interpretation_revision} · review {row.item.ref.review_revision}</p>{state.detail?.ref.candidate_id === row.item.ref.candidate_id && !state.detail.redacted && state.detail.ref.interpretation_revision === row.item.ref.interpretation_revision && <ProposalMeaning proposal={state.detail.candidate.proposal}/>}<label className="mr-3 inline-flex flex-col text-xs">Atomic group<input aria-label={`Atomic group for candidate ${index + 1}`} className={input} value={row.group} disabled={state.busy} onChange={(event) => { changed(); setRows(rows.map((entry,i) => i === index ? { ...entry, group: event.target.value } : entry)); }}/></label><label className="mr-3 inline-flex flex-col text-xs">Group action<select aria-label={`Action for candidate ${index + 1}`} className={input} value={row.action} disabled={state.busy} onChange={(event) => { changed(); setRows(rows.map((entry,i) => i === index ? { ...entry, action: event.target.value as "accept" | "reject" } : entry)); }}><option value="accept">Accept</option><option value="reject">Reject</option></select></label><button className={button} disabled={state.busy} onClick={() => { changed(); setRows(rows.filter((_,i) => i !== index)); }}>Remove from selection</button></section>)}
  {!!rows.length && <>
   <fieldset className="border-hair space-y-2 border-t pt-3" disabled={state.busy}><legend className="pt-3">Explicit shared identity dependency</legend><p className="text-muted-text text-xs">The provider must precede its dependent in the same atomic group. Both candidates must explicitly choose the compatible new definition. A dependency does not merge Entities or grant permission.</p>
    <label className="mr-2 inline-flex flex-col text-xs">Dependent candidate<select aria-label="Dependent candidate" className={input} value={target} onChange={(event) => setTarget(event.target.value)}><option value="">Choose…</option>{rows.map((row,i) => <option key={row.item.ref.candidate_id} value={row.item.ref.candidate_id}>{i + 1}: {row.item.ref.candidate_id}</option>)}</select></label>
    <label className="mr-2 inline-flex flex-col text-xs">Dependent field<select aria-label="Dependent field" className={input} value={field} onChange={(event) => setField(event.target.value)}>{["subject","object","predicate"].map((value) => <option key={value}>{value}</option>)}</select></label>
    <label className="mr-2 inline-flex flex-col text-xs">Provider candidate<select aria-label="Provider candidate" className={input} value={provider} onChange={(event) => setProvider(event.target.value)}><option value="">Choose…</option>{rows.map((row,i) => <option key={row.item.ref.candidate_id} value={row.item.ref.candidate_id}>{i + 1}: {row.item.ref.candidate_id}</option>)}</select></label>
    <label className="mr-2 inline-flex flex-col text-xs">Provider field<select aria-label="Provider field" className={input} value={fromField} onChange={(event) => setFromField(event.target.value)}>{["subject","object","predicate"].map((value) => <option key={value}>{value}</option>)}</select></label>
    <button className={button} disabled={!target || !provider} type="button" onClick={() => { changed(); setDependencies([...dependencies,{candidate_id:target, field, from_candidate_id:provider, from_field:fromField}]); }}>Add explicit dependency</button>
   </fieldset>
   <DependencyDisclosure dependencies={dependencies}/>{dependencies.map((_,i) => <button className={`${button} mr-2`} disabled={state.busy} key={i} onClick={() => { changed(); setDependencies(dependencies.filter((_,index) => index !== i)); }}>Remove dependency {i + 1}</button>)}
   <p className="text-muted-text text-xs">The complete request is checked against the selected revisions and all dependency closure rules. Limits: 20 groups, 64 candidates, 256 semantic records and 256 KiB of complete disclosure. Reduce complete groups if a bound is exceeded.</p>
   <button type="button" className={button} disabled={state.busy} onClick={prepare}>Prepare complete batch preview</button>
  </>}
  {problem && <p role="alert">{problem}</p>}
  {state.batchPreview && <article aria-label="Exact batch preview" className="border-teal-hair space-y-4 rounded border p-3"><h4 className="font-medium">Review every atomic group and its exact changes</h4><p>Each group succeeds or fails together. Independent successful groups may commit while another group fails. Failed groups in this delivery are recorded and never retried automatically.</p>{state.batchPreview.groups.map((group) => <section className="border-hair border-t pt-3" key={group.group_id}><h5>{group.group_id}: {group.preview.action}</h5>{group.preview.action === "reject" && <p>Reject every displayed candidate in this group. Accepted memory remains unchanged.</p>}<ReviewEffects preview={group.preview}/></section>)}<CanonicalDisclosure value={state.batchPreview} title="Complete batch binding and revision vector"/><label className="block text-xs">Optional batch reason<input aria-label="Optional batch reason" className={`${input} ml-2`} value={state.reason} disabled={state.busy || !!state.batchPending} onChange={(event) => controller.setReason(event.target.value)}/></label><button className={button} disabled={state.busy || !supportedBatch(state.batchPreview)} onClick={() => void controller.resolveBatch()}>{state.batchPending ? "Retry the exact batch delivery" : "Confirm all displayed group actions"}</button></article>}
  {state.batchResult && <article aria-label="Recorded batch decision"><h4>Durable batch results</h4><p className="break-all text-xs">Preview {state.batchResult.preview_id} · delivery {state.batchResult.delivery_key}</p><ol>{state.batchResult.groups.map((group) => <li className="mt-3" key={group.group_id}><p>{group.group_id}: {group.outcome}{group.failure_code && ` · ${group.failure_code}`}</p>{group.result && <p className="break-all text-xs">Audit {group.result.audit_id}{group.result.operation && ` · operation ${group.result.operation.operation_id} · ${group.result.operation.transaction_time}`}</p>}{group.prior_resolutions?.map((prior) => <section className="border-hair mt-2 border-l pl-3" key={prior.delivery_key}><h5>Earlier recorded decision: {prior.action}</h5><p className="break-all text-xs">Delivery {prior.delivery_key} · audit {prior.audit_id} · candidate revisions {prior.candidates.map((ref) => `${ref.candidate_id}: ${ref.interpretation_revision}/${ref.review_revision}`).join(", ")}</p>{prior.operation && <p className="break-all text-xs">Earlier operation {prior.operation.operation_id} · {prior.operation.transaction_time}</p>}<p className="text-xs">This earlier decision is not a successful change by the current failed group.</p></section>)}{!group.result && <p className="text-xs">No accepted operation recorded for this group. Inspect current candidates, prepare a new complete group and explicitly approve again.</p>}</li>)}</ol>{state.batchOperations?.map((operation) => <section key={operation.operation_id}><h5 className="mt-3 break-all">Recorded operation {operation.operation_id}</h5><ReviewEffects preview={operation.preview}/></section>)}{state.batchPriorOperations?.map(({group_id,operation}) => <section key={`${group_id}:${operation.operation_id}`}><h5 className="mt-3 break-all">Earlier accepted operation for {group_id}: {operation.operation_id}</h5><p className="text-xs">Recorded before this failed group; displayed under current source permissions.</p><ReviewEffects preview={operation.preview}/></section>)}</article>}
 </section>;
}
