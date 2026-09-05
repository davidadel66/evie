import { useEffect, useState, useSyncExternalStore } from "react";
import { candidateReviewAPI, type OwnerCandidate, type ReviewPreview } from "../api/candidateReview";
import { CandidateInboxController, type InboxState } from "./controller";
import { CandidateSources, ProposalMeaning, ReviewEffects, EditLineage } from "./ReviewDisclosure";
import { supportedPreview } from "./previewSupport";
import { AdvancedReviewForms } from "./AdvancedReviewForms";
import { BatchReview } from "./BatchReview";
import { browserPendingDecisionJournal } from "./pendingDecision";

const button = "border-hair-input text-body hover:bg-hover rounded border px-3 py-2 text-xs disabled:opacity-40";

export function CandidateInbox() {
  const [controller] = useState(() => new CandidateInboxController(candidateReviewAPI, undefined, browserPendingDecisionJournal()));
  const state = useSyncExternalStore(controller.subscribe, controller.snapshot, controller.snapshot);
  useEffect(() => { void controller.loadScopes(); return () => controller.invalidate(); }, [controller]);
  return <CandidateInboxView state={state} onScope={(scope) => void controller.selectScope(scope)} onScopes={(cursor) => void controller.loadScopes(cursor)} onLoad={(cursor) => void controller.load(cursor)} onInspect={(id) => void controller.inspect(id)} onPrepare={(action) => void controller.prepare(action)} onResolve={() => void controller.resolve()} onRecover={() => void controller.recover()} onReason={(reason) => controller.setReason(reason)} controller={controller} />;
}

export function CandidateInboxView({ state, onScope, onScopes, onLoad, onInspect, onPrepare, onResolve, onRecover, onReason, controller }: {
  controller?: CandidateInboxController; state: InboxState; onScope: (scope: string) => void; onScopes: (cursor?: string) => void; onLoad: (cursor?: string) => void;
  onInspect: (id: string) => void; onPrepare: (action: "accept" | "reject") => void; onResolve: () => void; onRecover: () => void; onReason: (reason: string) => void;
}) {
  const selected = state.scopes?.scopes.find((scope) => scope.scope_key === state.scope);
  const item = state.detail;
  return <section aria-label="Candidate inbox" className="text-body flex min-h-0 flex-1 flex-col overflow-auto p-5 text-sm">
    <header className="mb-5"><h2 className="text-ink text-lg font-medium">Review memory candidates</h2><p className="text-muted-text mt-1 text-xs">Suggestions stay here until you review their sources and approve the exact memory change.</p></header>
    <div className="border-hair flex flex-wrap items-end gap-3 border-b pb-4">
      <label className="flex min-w-56 flex-col gap-1 text-xs">Review scope
        <select aria-label="Review scope" className="border-hair-input bg-app rounded border px-3 py-2" value={state.scope} onChange={(event) => onScope(event.target.value)}>
          <option value="">Choose one memory scope…</option>
          {state.scope && !selected && <option value={state.scope}>{state.scope}</option>}
          {state.scopes?.scopes.map((scope) => <option key={scope.scope_key} value={scope.scope_key}>{scope.label} · {scope.kind}</option>)}
        </select>
      </label>
      <button type="button" className={button} disabled={state.scopesBusy} onClick={() => onScopes()}>Refresh scopes</button>
      {state.scopes?.next_cursor && <button type="button" className={button} disabled={state.scopesBusy} onClick={() => onScopes(state.scopes?.next_cursor)}>More scopes</button>}
      {state.scopes?.indexing && <button type="button" className={button} disabled={state.scopesBusy} onClick={() => onScopes()}>Continue loading older scopes</button>}
      <button type="button" className={button} disabled={!state.scope || state.busy} onClick={() => onLoad()}>Refresh inbox</button>
    </div>
    {state.scopesBusy && <p role="status" className="text-muted-text py-3">Loading memory scopes…</p>}
    {state.problem && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink my-3 rounded border p-3">{state.problem}</p>}
    {state.notice && <p role="status" className="text-ok-ink my-3">{state.notice}</p>}
    {state.recovery && state.recovery.decision.preview_id !== state.preview?.preview_id && state.recovery.decision.preview_id !== state.batchPreview?.preview_id && <div className="border-amber-hair text-amber-ink my-3 rounded border p-3"><p>An earlier {"actions" in state.recovery.decision ? "batch" : state.recovery.decision.action} decision has no confirmed response. Retry its exact delivery in {state.recovery.scope} before confirming another candidate.</p><button className={`${button} mt-2`} type="button" disabled={state.busy} onClick={onRecover}>Recover earlier decision</button></div>}
    {state.busy && <p role="status" className="text-muted-text py-3">Loading the current review…</p>}
    {!state.scope && <p className="text-muted-text py-8">Choose a scope to inspect its suggestions, including suggestions from closed conversations.</p>}
    {state.scope && <p className="text-muted-text mt-3 break-all text-xs">Exact scope: {state.scope}{state.page ? ` · inbox revision ${state.page.revision}` : ""}</p>}
    {state.page && <div className="border-hair my-4 rounded border">
      {state.page.candidates.length === 0 ? <p className="text-muted-text p-4">No unresolved candidates in this page.</p> : <ul className="divide-hair divide-y">
        {state.page.candidates.map((candidate) => <li key={candidate.ref.candidate_id}><button type="button" className="hover:bg-hover w-full p-4 text-left disabled:opacity-40" disabled={state.busy} onClick={() => onInspect(candidate.ref.candidate_id)}>
          <span className="block">{candidateMeaning(candidate)}</span><span className="text-muted-text mt-1 block text-xs">{candidate.candidate.review_state} · {candidate.redacted ? "Evidence unavailable" : `${candidate.candidate.support?.length ?? 0} supporting source(s)`}</span>
        </button></li>)}
      </ul>}
      {state.page.next_cursor && <div className="border-hair border-t p-3"><button type="button" className={button} disabled={state.busy} onClick={() => onLoad(state.page?.next_cursor)}>Next candidate page</button></div>}
    </div>}
    {item && <article className="border-hair bg-card my-2 rounded border p-4" aria-label="Candidate detail">
      <h3 className="text-ink font-medium">{candidateMeaning(item)}</h3>
      <p className="text-muted-text mt-1 text-xs">State: {item.candidate.review_state} · interpretation {item.ref.interpretation_revision} · review {item.ref.review_revision}</p>
      {item.redacted ? <p className="text-amber-ink mt-3">The source is no longer eligible to display. Acceptance is blocked; rejection remains available.</p> : <>
        <CandidateSources title="Supporting evidence" sources={item.candidate.support ?? []} />
        <CandidateSources title="Interpretation context (not supporting evidence)" sources={item.candidate.context ?? []} />
        {item.candidate.proposal.temporal_qualification && <p className="mt-3">Time qualification: {item.candidate.proposal.temporal_qualification}</p>}
      </>}
      <details className="text-muted-text mt-3 break-all text-xs"><summary>Candidate origin</summary><dl className="mt-2 space-y-1"><div><dt>Candidate</dt><dd>{item.ref.candidate_id}</dd></div><div><dt>Generation</dt><dd>{item.generation_id}</dd></div><div><dt>Compilation</dt><dd>{item.job_id}</dd></div><div><dt>Destination</dt><dd>{item.destination}</dd></div></dl></details>
      {!item.redacted && <><ProposalMeaning proposal={item.candidate.proposal}/>{item.edit && <EditLineage edit={item.edit}/>}<AdvancedReviewForms key={item.ref.candidate_id} state={state} controller={controller}/></>}
      {item.candidate.review_state === "unresolved" && <div className="mt-4 flex gap-3">
        <button type="button" className={button} disabled={state.busy || item.redacted} onClick={() => onPrepare("accept")}>Preview acceptance</button>
        <button type="button" className={button} disabled={state.busy} onClick={() => onPrepare("reject")}>Preview rejection</button>
      </div>}
    </article>}
    {controller && <BatchReview key={`${state.scope}:${state.disclosureEpoch ?? 0}`} state={state} controller={controller}/>}
    {state.preview && <article className="border-teal-hair bg-panel my-4 rounded border p-4" aria-label="Exact review preview">
      <h3 className="text-ink font-medium">{state.preview.action === "accept" ? "Review the exact memory change" : "Review rejection"}</h3>
      {state.preview.action === "accept" ? <ReviewEffects preview={state.preview} /> : <p className="mt-3">Reject this candidate at the displayed interpretation and review revisions. No accepted memory will change.</p>}
      <PreviewIdentity preview={state.preview} />
      <label className="mt-4 flex flex-col gap-1 text-xs">Optional reason<input aria-label="Optional reason" maxLength={4096} className="border-hair-input bg-app rounded border px-3 py-2" value={state.reason} disabled={state.busy || !!state.pending} onChange={(event) => onReason(event.target.value)} /></label>
      <p className="text-muted-text my-3 text-xs">Confirming applies only to this exact preview. A changed preview requires your approval again.</p>
      <button type="button" className={`${button} border-teal-hair text-teal`} disabled={state.busy || (state.preview.action === "accept" && !supportedPreview(state.preview))} onClick={onResolve}>{state.pending ? "Retry the same decision" : state.preview.action === "accept" ? "Accept this exact memory" : "Reject this candidate"}</button>
    </article>}
    {state.result && <article aria-label="Recorded review decision" className="border-hair my-4 rounded border p-4">
      <h3 className="text-ink font-medium">Recorded decision: {state.result.action === "accept" ? "accepted" : "rejected"}</h3>
      {state.operation && <><p className="text-muted-text mt-2 text-xs">Accepted provenance, loaded from the recorded operation under current source permissions.</p><ReviewEffects preview={state.operation.preview} /></>}
      <details className="text-muted-text mt-3 break-all text-xs"><summary>Decision receipt</summary><p>Audit: {state.result.audit_id}</p><p>Delivery: {state.result.delivery_key}</p>{state.result.operation && <p>Operation: {state.result.operation.operation_id} · {state.result.operation.transaction_time}</p>}</details>
    </article>}
  </section>;
}

function candidateMeaning(item: OwnerCandidate): string {
  if (item.redacted) return "Candidate with unavailable evidence";
  const prop = item.candidate.proposal.proposition;
  if (item.candidate.proposal.identity) return `${item.candidate.proposal.identity.subject?.name ?? prop.subject_entity_id} · ${item.candidate.proposal.identity.predicate?.label ?? prop.predicate_id} · ${item.candidate.proposal.identity.object?.name ?? prop.object.literal?.value ?? prop.object.entity_id}`;
  return `${prop.polarity === "denied" ? "Denied: " : ""}${prop.object.literal?.value ?? "Entity relationship"}`;
}

function PreviewIdentity({ preview }: { preview: ReviewPreview }) { return <details className="text-muted-text mt-3 break-all text-xs"><summary>Preview binding</summary><p>Scope: {preview.scope_key}</p><p>Generation: {preview.generation_id}</p><p>Preview: {preview.preview_id}</p><p>Preview digest: {preview.preview_sha256}</p><p>Effect digest: {preview.effect_sha256}</p>{preview.candidates.map((item) => <p key={item.ref.candidate_id}>Candidate: {item.ref.candidate_id} · interpretation {item.ref.interpretation_revision} · review {item.ref.review_revision}</p>)}</details>; }
