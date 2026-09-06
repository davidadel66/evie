import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react";
import { compilerDiagnosticsAPI, type CompilerDiagnostics, type DiagnosticView } from "../api/compilerDiagnostics";
import { CompilerDiagnosticsController, type DiagnosticsState } from "./controller";

const button = "border-hair-input text-body hover:bg-hover rounded border px-3 py-2 text-xs disabled:opacity-40";
const input = "border-hair-input bg-app rounded border px-3 py-2";
const views: [DiagnosticView, string][] = [["jobs", "Jobs and coverage"], ["selections", "Selected units"], ["live_roots", "Live selection obligations"], ["candidates", "Review backlog"], ["activations", "Activations"], ["history", "Historical selection"], ["selection", "Event selection"], ["foreground", "Foreground timings"]];
const countLabels: Record<string, string> = {
 jobs_queued: "Queued jobs", jobs_running: "Running jobs", jobs_retry_wait: "Jobs waiting to retry", jobs_staged: "Staged jobs",
 jobs_completed_candidates: "Jobs completed with candidates", jobs_completed_empty: "Jobs completed empty", jobs_excluded: "Excluded jobs", jobs_failed: "Failed jobs", jobs_cancelled: "Cancelled jobs",
 candidates_unresolved: "Unresolved inbox candidates", candidates_accepted: "Accepted candidates", candidates_rejected: "Rejected candidates", candidates_suppressed: "Suppressed repeat candidates", attempts: "Inference attempts", cancellations: "Cancellations", retries: "Retries",
};

export function CompilerHealth() {
 const [controller] = useState(() => new CompilerDiagnosticsController(compilerDiagnosticsAPI));
 const state = useSyncExternalStore(controller.subscribe, controller.snapshot, controller.snapshot);
 useEffect(() => { void controller.loadScopes(); return () => controller.invalidate(); }, [controller]);
 return <CompilerHealthView state={state} controller={controller}/>;
}

export function CompilerHealthView({ state, controller }: { state: DiagnosticsState; controller: CompilerDiagnosticsController }) {
 const scope = state.scopes?.scopes.find((item) => item.scope_key === state.scope);
 return <section aria-label="Compiler health" className="text-body flex min-h-0 flex-1 flex-col overflow-auto p-5 text-sm">
  <header className="mb-5"><h2 className="text-ink text-lg font-medium">Compiler health</h2><p className="text-muted-text mt-1 text-xs">Inspect background progress and review workload for one exact scope and session, including closed conversations.</p></header>
  <div className="border-hair flex flex-wrap items-end gap-3 border-b pb-4">
   <label className="flex min-w-48 flex-col gap-1 text-xs">Memory scope<select className={input} value={state.scope} onChange={(event) => void controller.selectScope(event.target.value)}>
    <option value="">Choose a scope…</option>{state.scope && !scope && <option value={state.scope}>{state.scope}</option>}
    {state.scopes?.scopes.map((item) => <option key={item.scope_key} value={item.scope_key}>{item.label} · {item.kind}</option>)}
   </select></label>
   <button type="button" className={button} disabled={state.scopesBusy} onClick={() => void controller.loadScopes()}>Refresh scopes</button>
   {state.scopes?.next_cursor && <button type="button" className={button} disabled={state.scopesBusy} onClick={() => void controller.loadScopes(state.scopes?.next_cursor)}>Next scope page</button>}
   {state.scopes?.indexing && <button type="button" className={button} disabled={state.scopesBusy} onClick={() => void controller.loadScopes()}>Continue indexing scopes</button>}
   <label className="flex min-w-48 flex-col gap-1 text-xs">Session<select className={input} value={state.session} disabled={!state.scope || state.busy} onChange={(event) => void controller.selectSession(event.target.value)}>
    <option value="">Choose a session…</option>{state.sessions?.session_ids.map((id) => <option key={id} value={id}>{id}</option>)}
   </select></label>
   <button type="button" className={button} disabled={!state.scope || state.busy} onClick={() => void controller.loadSessions()}>Refresh sessions</button>
   {state.sessions?.next_cursor && <button type="button" className={button} disabled={state.busy} onClick={() => void controller.loadSessions(state.sessions?.next_cursor)}>Next session page</button>}
   <label className="flex flex-col gap-1 text-xs">Diagnostic view<select className={input} value={state.view} onChange={(event) => void controller.selectView(event.target.value as DiagnosticView)}>{views.map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label>
  </div>
  {state.view === "selection" && <div className="my-4 flex flex-wrap items-end gap-3">
   <label className="flex flex-1 flex-col gap-1 text-xs">Exact generation ID<input className={`${input} min-w-64`} list="compiler-diagnostic-generations" maxLength={64} value={state.generation} onChange={(event) => controller.setGeneration(event.target.value)} placeholder="Choose a known generation or enter its exact ID"/></label>
   <datalist id="compiler-diagnostic-generations">{state.generations.map((id) => <option key={id} value={id}/>)}</datalist>
   <p className="text-muted-text w-full text-xs">Choose a generation explicitly from job, candidate, activation, or history metadata. Its identity does not establish that a configured runtime is currently available.</p>
  </div>}
  <div className="my-4 flex flex-wrap items-center gap-3"><button type="button" className={button} disabled={!state.session || state.busy || (state.view === "selection" && !state.generation)} onClick={() => void controller.load()}>Refresh diagnostics</button>{state.page?.next_cursor && <button type="button" className={button} disabled={state.busy} onClick={() => void controller.load(state.page?.next_cursor)}>Next diagnostic page</button>}<p className="text-muted-text text-xs">Each page is a fresh snapshot. Next advances the current listing; refresh starts at the first page.</p></div>
  {(state.busy || state.scopesBusy) && <p role="status" className="text-muted-text py-3">Loading bounded diagnostics…</p>}
  {state.problem && <p role="alert" className="border-danger-hair text-danger-ink my-3 rounded border p-3">{state.problem}</p>}
  {!state.scope && <p className="text-muted-text py-6">Choose a memory scope to begin.</p>}
  {state.scope && <p className="text-muted-text break-all text-xs">Exact scope: {state.scope}{state.session && ` · session: ${state.session}`}</p>}
  {state.sessions?.session_ids.length === 0 && <p className="text-muted-text py-4">No available sessions in this page.</p>}
  {state.page && <DiagnosticPage page={state.page}/>}
 </section>;
}

function DiagnosticPage({ page }: { page: CompilerDiagnostics }) {
 const countEntries = Object.entries(countLabels).filter(([key]) => Object.hasOwn(page.counts, key));
 return <>
  <p className="text-muted-text mt-3 text-xs">Snapshot: {timestamp(page.as_of_unix_ms)} · revision {page.revision}</p>
  <p className="my-3">Shared compiler capacity: {page.capacity_state === "available" ? "Available" : page.capacity_state === "busy" ? "Busy" : page.capacity_state === "capacity_blocked" ? "Blocked until server release is verified" : "Unavailable"}</p>
  <article className="border-hair my-3 rounded border p-4" aria-label="Scoped operational counts"><h3 className="text-ink font-medium">Counts for this scope and session</h3>
   {page.indexing && <p role="status" className="text-amber-ink mt-2">Indexing older records: these counts are partial. Refresh to continue indexing.</p>}
   <p className="text-muted-text mt-1 text-xs">Counts cover the indexed records in this scope and session, across generations. Page rows are a bounded portion. Unresolved counts exclude suppressed repeats.</p>
   {countEntries.length === 0 ? <p className="mt-3">No counts recorded.</p> : <dl className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">{countEntries.map(([key, label]) => <div key={key}><dt className="text-muted-text text-xs">{label}</dt><dd className="text-ink">{page.counts[key]}</dd></div>)}</dl>}
  </article>
  {page.view === "jobs" && <>
   <p className="text-muted-text my-3 text-xs">Selected and completed counts refer to new evidence. Sequence bounds are bounding coordinates; intervening events are not necessarily evidence. A failed or cancelled gap remains unresolved when a later job succeeds. Excluded work is distinct from successful empty extraction.</p>
   <Rows empty={page.jobs.length === 0}>{page.jobs.map((job) => <article key={job.job_id} className="border-hair rounded border p-4">
    <h3 className="text-ink break-all font-medium">{job.job_id}</h3><p className="mt-2">State: {job.state} · lane: {job.lane} · attempts: {job.attempts}</p>
    <p className="mt-1">Selected new events: {job.selected_new_events} · Completed new events: {job.completed_new_events}</p><p>Publication commit: {duration(job.publication_nanos)}</p><p>Candidate freshness from measured terminal commit: {duration(job.candidate_freshness_nanos)}</p><p className="text-muted-text text-xs">Sequence bounds: {job.first_sequence}–{job.last_sequence}</p>
    {job.reason && <p className="mt-2">Reason: {job.reason}</p>}{job.pause_reason && <p>Pause: {job.pause_reason}</p>}{job.recovery && <p>Recovery: {job.recovery}</p>}{job.retry_at ? <p>Retry due: {timestamp(job.retry_at * 1000)}</p> : null}
    <Metadata><p>Generation: {job.generation_id}</p><p>Queued: {timestamp(job.queued_at_unix_ms)}</p><p>Published: {timestamp(job.published_at_unix_ms)}</p></Metadata>
    {job.measurements.length === 0 ? <p className="text-muted-text mt-3 text-xs">No attempt timing observation is recorded.</p> : <div className="mt-3 overflow-x-auto"><table className="w-full text-left text-xs"><caption className="mb-2 text-left">Observed attempt timings; missing values remain incomplete</caption><thead><tr>{["Attempt / fence", "Claimed", "Outcome", "Queue wait", "Inference", "Validation / resolution", "Database completion"].map((label) => <th className="p-2" key={label}>{label}</th>)}</tr></thead><tbody>{job.measurements.map((attempt) => <tr key={`${attempt.attempt}:${attempt.fence}`}><td className="p-2">{attempt.attempt} / {attempt.fence}</td><td className="p-2">{timestamp(attempt.claimed_at_unix_ms)}</td><td className="p-2">{attempt.observed_outcome}</td>{[attempt.queue_wait_nanos, attempt.inference_nanos, attempt.validation_nanos, attempt.database_completion_nanos].map((value, i) => <td className="p-2" key={i}>{duration(value)}</td>)}</tr>)}</tbody></table></div>}
   </article>)}</Rows>
  </>}
  {page.view === "selections" && <>
   <p className="text-muted-text my-3 text-xs">Selected units include work awaiting a job slot. Their state remains distinct from job completion; sequence bounds describe exact selected members, not every intervening event.</p>
   <Rows empty={page.selections.length === 0}>{page.selections.map((item) => <article key={item.selection_id} className="border-hair rounded border p-4"><h3 className="text-ink break-all font-medium">{item.selection_id}</h3><p className="mt-2">State: {item.state} · Selected new events: {item.selected_new_events}</p><p>Sequence bounds: {item.first_sequence}–{item.last_sequence}</p>{item.reason && <p>Reason: {item.reason}</p>}<Metadata><p>Job: {item.job_id || "Not materialized"}</p><p>Generation: {item.generation_id}</p></Metadata></article>)}</Rows>
  </>}
  {page.view === "live_roots" && <>
   <p className="text-muted-text my-3 text-xs">Live selection obligations include deferred and unmaterialized work. A pending extension is a separate obligation even when an earlier prefix completed.</p>
   <Rows empty={page.live_roots.length === 0}>{page.live_roots.map((item) => <article key={`${item.activation_id}:${item.root_id}`} className="border-hair rounded border p-4"><h3 className="text-ink break-all font-medium">Root: {item.root_id}</h3><p className="mt-2">State: {item.state}</p><p>Sequence bounds: {item.first_sequence}–{item.last_sequence}</p>{item.reason && <p>Reason: {item.reason}</p>}<Metadata><p>Activation: {item.activation_id}</p><p>Selection: {item.selection_id || "Not materialized"}</p></Metadata></article>)}</Rows>
  </>}
  {page.view === "candidates" && <>
   <p className="text-muted-text my-3 text-xs">Elapsed inbox age is time since publication for a current unresolved item. Active measured review time must be collected during the pilot; it is unavailable here. Approval rate is not accuracy.</p>
   <Rows empty={page.candidates.length === 0}>{page.candidates.map((item) => <article key={item.ref.candidate_id} className="border-hair rounded border p-4">
    <h3 className="text-ink break-all font-medium">{item.ref.candidate_id}</h3><p className="mt-2">Review state: {item.review_state} · Edited: {item.edited ? "Yes" : "No"}</p>
    {item.review_state === "unresolved" && !item.equivalent_to && <p>Elapsed inbox age at snapshot: {item.published_at_unix_ms === null ? "Unavailable" : duration(Math.max(0, page.as_of_unix_ms - item.published_at_unix_ms) * 1e6)}</p>}
    {item.equivalent_to && <p className="mt-2 break-all">Suppressed repeat; review origin: {item.equivalent_to}</p>}
    <Metadata><p>Interpretation revision: {item.ref.interpretation_revision} · review revision: {item.ref.review_revision}</p><p>Published: {timestamp(item.published_at_unix_ms)}</p><p>Decided: {timestamp(item.decided_at_unix_ms)}</p><p>Job: {item.job_id}</p><p>Generation: {item.generation_id}</p></Metadata>
   </article>)}</Rows>
  </>}
  {page.view === "activations" && <>
   <p className="text-muted-text my-3 text-xs">Activation frontiers bound selection; they do not establish completed coverage. A retained generation identity does not establish current runtime availability.</p>
   <Rows empty={page.activations.length === 0}>{page.activations.map((item) => <article key={item.activation_id} className="border-hair rounded border p-4"><h3 className="text-ink break-all font-medium">{item.activation_id}</h3><p className="mt-2">Work: {item.work_paused ? "Paused" : "Not paused"} · revision: {item.revision}</p><p>Selected after commit position {item.after_position}{item.through_position === undefined ? "; no recorded closing frontier" : ` through ${item.through_position}`}</p><Metadata><p>Source scope: {item.selector.source_scope}</p><p>Destination: {item.selector.destination}</p><p>Session selector: {item.selector.session_id || "Eligible sessions in this source scope"}</p><p>Generation: {item.generation_id}</p></Metadata></article>)}</Rows>
  </>}
  {page.view === "history" && <>
   <p className="text-muted-text my-3 text-xs">The scanned cursor records discovery, not completed coverage. Inspect jobs for completion and unresolved gaps.</p>
   <Rows empty={page.history.length === 0}>{page.history.map((item) => <article key={`${item.request_id}:${item.range_index}`} className="border-hair rounded border p-4"><h3 className="text-ink break-all font-medium">{item.request_id} · range {item.range_index}</h3><p className="mt-2">Selected sequence bounds: {item.first_sequence}–{item.last_sequence}</p><p>Scanned through sequence: {item.scanned_sequence} · {item.cancelled ? "Cancelled" : "Not cancelled"}</p><Metadata><p>Revision: {item.revision}</p><p>Generation: {item.generation_id}</p></Metadata></article>)}</Rows>
  </>}
  {page.view === "selection" && <>
   <p className="text-muted-text my-3 text-xs">Selection membership is specific to the chosen generation. Outside selection is unprocessed by that selection; selected membership does not establish successful compilation.</p>
   <Rows empty={page.selection.length === 0}><div className="overflow-x-auto"><table className="w-full text-left text-xs"><thead><tr><th className="p-2">Event</th><th className="p-2">Sequence</th><th className="p-2">Membership</th></tr></thead><tbody>{page.selection.map((item) => <tr key={item.event_id}><td className="break-all p-2">{item.event_id}</td><td className="p-2">{item.sequence}</td><td className="p-2">{item.membership}</td></tr>)}</tbody></table></div></Rows>
  </>}
  {page.view === "foreground" && <>
   <p className="text-muted-text my-3 text-xs">Terminal commit and response finalization are separate observed boundaries. These are not request duration or first-token time. Missing observations, including interrupted processes, stay unavailable.</p>
   <Rows empty={page.foreground.length === 0}>{page.foreground.map((item) => <article key={item.root_id} className="border-hair rounded border p-4"><h3 className="text-ink break-all font-medium">Root: {item.root_id}</h3><p className="mt-2">Outcome: {item.outcome}</p><p>Started: {timestamp(item.started_at_unix_ms)}</p><dl className="mt-3 grid gap-3 sm:grid-cols-2"><div><dt>Terminal event commit</dt><dd>{duration(item.terminal_commit_nanos)}</dd><dd className="text-muted-text text-xs">Observed at: {timestamp(item.terminal_committed_at_unix_ms)}</dd></div><div><dt>Response finalization</dt><dd>{duration(item.response_finalization_nanos)}</dd><dd className="text-muted-text text-xs">Observed at: {timestamp(item.response_finalized_at_unix_ms)}</dd></div></dl></article>)}</Rows>
  </>}
 </>;
}

function Rows({ empty, children }: { empty: boolean; children: ReactNode }) { return empty ? <p className="text-muted-text my-4">No records in this page. A next page can still contain records.</p> : <div className="my-4 space-y-3">{children}</div>; }
function Metadata({ children }: { children: ReactNode }) { return <div className="text-muted-text mt-3 space-y-1 break-all text-xs">{children}</div>; }
function timestamp(value: number | null) { if (value === null) return "Unavailable / incomplete"; return new Date(value).toISOString(); }
function duration(value: number | null) {
 if (value === null) return "Unavailable / incomplete";
 const ms = value / 1e6;
 if (ms < 1000) return `${ms.toFixed(3)} ms`;
 if (ms < 60000) return `${(ms / 1000).toFixed(3)} s`;
 return `${(ms / 60000).toFixed(2)} min`;
}
