import { useState } from "react";
import {
  modelCallSpend, workerCallSpend, workflowMapViews,
  type WorkflowInterruption, type WorkflowRun, type WorkflowRunDetail, type WorkflowRunState, type WorkflowWorkerCall,
} from "../api/workflows";
import { WorkflowInterruptionAnswer } from "./WorkflowInterruptionAnswer";

// Runs of this Workspace's workflows (Stage 14 sub-stage 3). David starts a
// run of an active workflow with its inputs; Evie executes its deterministic
// and branch steps, recording each node attempt and checkpoint, and
// responds when the run finishes or pauses. A start whose Run Key already
// has a run shows that run instead. Definitions with other step kinds are
// refused until the runtime supports them. Since sub-stage 4 a run survives
// restarts: one Evie process at a time advances it under a fenced lease, a
// paused run shows why, and inspection lists its lease and recovery events.
// Since sub-stage 7 a run may wait for David's answer to a question, or to
// an effect whose outcome is unknown; he answers it here, from any later
// session, and AI steps show their model calls and spend. Since sub-stage 8
// he cancels an unfinished run here: no further step starts, what is in
// flight finishes (an effect completes or is reconciled first), and the run
// ends cancelled with its completed steps recorded and its Run Key free. A
// rerun of a succeeded run shows the override it was started under. Since
// sub-stage 10 the view shows each map step's items (index, state,
// attempts, worker execution, spend) and every worker call, and names a run
// Evie started from a chat in this Workspace (workflow_start).
//
// The Runs tab (WorkspaceRuns.tsx, workspace-activity.spec.md) replaced the
// container that lived here; these views stay for the design system, and
// the start form is still used there until the workflow pop-up has it.

const stateLabel: Record<WorkflowRunState, string> = {
  queued: "Queued",
  running: "Running",
  waiting_for_human: "Waiting for you",
  waiting_for_dependency: "Waiting for a dependency",
  needs_reconciliation: "Needs reconciliation",
  cancelling: "Cancelling",
  cancelled: "Cancelled",
  failed: "Failed",
  succeeded: "Succeeded",
};

function json(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function when(time: string): string {
  const date = new Date(time);
  return Number.isNaN(date.getTime()) ? time : date.toLocaleString();
}

const paused = (run: WorkflowRun) => run.state === "waiting_for_dependency";

const finished = (state: WorkflowRunState) => state === "succeeded" || state === "failed" || state === "cancelled";

/** A run whose error line is a status, not a failure: paused, or cancelled by David. */
const quiet = (run: WorkflowRun) => paused(run) || run.state === "cancelling" || run.state === "cancelled";

/** A run's error line: why it failed, why it is paused, or how it was cancelled. */
function reason(run: WorkflowRun): string {
  return paused(run) ? `Paused: ${run.error}` : run.error ?? "";
}

export function WorkflowRunList({runs, busy, onInspect}: {
  runs: WorkflowRun[];
  busy: boolean;
  onInspect: (run: WorkflowRun) => void;
}) {
  if (runs.length === 0) {
    return <p className="text-muted-text text-xs">No runs yet.</p>;
  }
  return <ul className="border-hair border-t">
    {runs.map((run) => <li key={run.id} className="border-hair border-b py-3 text-xs">
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="text-body font-mono">{run.workflow}</span>
        <span className="text-fainter">version {run.version}</span>
        <span className={run.state === "failed" ? "text-danger" : "text-muted-text"}>{stateLabel[run.state]}</span>
        <span className="text-fainter">{when(run.created_at)}</span>
        <button type="button" disabled={busy} onClick={() => onInspect(run)} className="text-body underline underline-offset-2 disabled:opacity-40">Inspect</button>
      </div>
      {run.error && <p className={`${quiet(run) ? "text-muted-text" : "text-danger"} mt-1 leading-5`}>{reason(run)}</p>}
    </li>)}
  </ul>;
}

/** Who started a run: the owner from the web or the CLI, or Evie from a chat. */
function origin(run: WorkflowRun): string {
  const where = run.surface === "chat" ? "started by Evie in your chat (workflow_start)" : `started from the ${run.surface === "web" ? "web" : "CLI"}`;
  return `${where}${run.session_id ? ` in session ${run.session_id}` : ""}`;
}

function dollars(microUSD: number): string {
  return `$${(microUSD / 1_000_000).toFixed(6)}`;
}

function budget(ms: number): string {
  return ms % 60000 === 0 ? `${ms / 60000}m` : ms % 1000 === 0 ? `${ms / 1000}s` : `${ms}ms`;
}

/** One worker call: which attempt ran which worker, as which execution, and how it ended. */
function workerLine(c: WorkflowWorkerCall): string {
  const label = `${c.step}${c.item !== undefined ? ` item ${c.item}` : ""} attempt #${c.attempt_seq}`;
  let text = `${label}: ${c.worker} worker, ${c.policy} v${c.policy_version} ${c.model}, budget ${budget(c.budget_ms)} / ${c.budget_tokens} tokens`;
  if (c.execution_id) text += `; execution ${c.execution_id}`;
  if (c.adopted_from) text += `; took the result of execution ${c.adopted_from}`;
  if (c.continues) text += `; continued execution ${c.continues} in its worktree`;
  const spend = workerCallSpend(c);
  text += c.finished_at
    ? `; ${c.status ?? "ended"}${c.reason ? ` (${c.reason})` : ""}, ${spend.tokens} tokens${c.usage_complete ? "" : " (counted at least at its budget)"}, ${dollars(spend.microUSD)}`
    : `; in flight, counted at its budget of ${spend.tokens} tokens`;
  return text;
}

function resolutionLine(i: WorkflowInterruption): string {
  const r = i.resolution;
  if (!r) return "";
  const who = r.kind === "answered" ? `answered via ${r.surface}${r.session_id ? ` from session ${r.session_id}` : ""}`
    : r.kind === "reconciled" ? "settled by the provider's reconciliation" : "withdrawn";
  return `Interruption #${i.seq} at ${i.step}: ${who}${r.answer ? `: ${JSON.stringify(r.answer)}` : ""}`;
}

/** David's cancellation of an unfinished run, with an optional reason. */
function WorkflowRunCancel({busy, onCancel}: {busy: boolean; onCancel: (reason: string) => void}) {
  const [why, setWhy] = useState("");
  return <form className="mt-3 flex flex-wrap items-center gap-2" onSubmit={(event) => { event.preventDefault(); onCancel(why); }}>
    <input aria-label="Cancellation reason" value={why} onChange={(event) => setWhy(event.target.value)} maxLength={1000} placeholder="Reason (optional)"
      className="border-hair bg-app text-body min-w-0 flex-1 rounded-[6px] border px-2 py-1" />
    <button type="submit" disabled={busy} className="border-hair text-body rounded-[6px] border px-3 py-1 font-semibold disabled:opacity-40">Cancel run</button>
  </form>;
}

export function WorkflowRunView({detail, onClose, busy = false, onAnswer, onCancel}: {
  detail: WorkflowRunDetail;
  onClose: () => void;
  busy?: boolean;
  onAnswer?: (step: string, answer: Record<string, unknown>) => void;
  onCancel?: (reason: string) => void;
}) {
  const run = detail.run;
  const cancellation = detail.cancellation;
  const override = detail.override;
  const interruptions = detail.interruptions ?? [];
  const open = interruptions.filter((i) => !i.resolution);
  const calls = detail.model_calls ?? [];
  const spend = calls.reduce((total, call) => {
    const s = modelCallSpend(call);
    return {tokens: total.tokens + s.tokens, microUSD: total.microUSD + s.microUSD};
  }, {tokens: 0, microUSD: 0});
  const workers = detail.workers ?? [];
  const workerSpend = workers.reduce((total, call) => {
    const s = workerCallSpend(call);
    return {tokens: total.tokens + s.tokens, microUSD: total.microUSD + s.microUSD};
  }, {tokens: 0, microUSD: 0});
  const maps = workflowMapViews(detail);
  const steps = detail.attempts.filter((a) => a.item === undefined);
  return <div role="dialog" aria-label={`Run ${run.id}`} className="border-hair bg-app mt-3 rounded-[8px] border p-4 text-xs leading-5">
    <h3 className="text-body mb-2 text-sm font-medium">{run.workflow} version {run.version}: {stateLabel[run.state]}</h3>
    <p className="text-muted-text">Run <code className="font-mono">{run.id}</code>, {origin(run)}.</p>
    <p className="text-muted-text">Run key <code className="font-mono">{run.run_key}</code></p>
    {override && <p className="text-muted-text">{`Rerun of ${override.rerun_of} under your override via ${override.surface} at ${when(override.recorded_at)}: ${override.reason}`}</p>}
    <p className="text-muted-text">Executable hash <code className="font-mono">{run.executable_hash}</code></p>
    <p className="text-muted-text">Pinned to Workspace revision {run.workspace_revision}, access revision {run.access_revision}.</p>
    {cancellation && <p className="text-muted-text mt-2">{`${run.state === "cancelled" ? "Cancelled" : "Cancellation requested"} via ${cancellation.surface} at ${when(cancellation.requested_at)}, while the run was ${stateLabel[cancellation.from_state].toLowerCase()}${cancellation.reason ? `: ${cancellation.reason}` : ""}`}</p>}
    {run.error && (quiet(run)
      ? <p role="status" className="text-muted-text mt-2">{reason(run)}{paused(run) && run.not_before ? ` Checked again after ${when(run.not_before)}.` : ""}</p>
      : <p role="alert" className="text-danger mt-2">{run.error}</p>)}
    {run.not_before && !quiet(run) && !finished(run.state) && <p className="text-muted-text">Retry not before {when(run.not_before)}.</p>}
    {onCancel && !finished(run.state) && run.state !== "cancelling" && <WorkflowRunCancel key={run.id} busy={busy} onCancel={onCancel} />}
    {open.length > 0 && <section className="mt-3" aria-label="Waiting for you">
      <h4 className="text-body font-medium">Waiting for you</h4>
      {open.map((i) => onAnswer
        ? <WorkflowInterruptionAnswer key={i.seq} interruption={i} busy={busy} onAnswer={onAnswer} />
        : <p key={i.seq} className="text-muted-text">{i.message}</p>)}
    </section>}
    {detail.lease && <p className="text-muted-text">{detail.lease.released_at
      ? `Lease released by ${detail.lease.holder} (lease ${detail.lease.token}) at ${when(detail.lease.released_at)}; last heartbeat ${when(detail.lease.heartbeat_at)}.`
      : `Lease held by ${detail.lease.holder} (lease ${detail.lease.token}) until ${when(detail.lease.expires_at)}; last heartbeat ${when(detail.lease.heartbeat_at)}.`}</p>}
    <section className="mt-3">
      <h4 className="text-body font-medium">Inputs</h4>
      <pre className="font-mono whitespace-pre-wrap">{json(run.inputs)}</pre>
    </section>
    <section className="mt-3">
      <h4 className="text-body font-medium">Nodes</h4>
      {steps.length === 0 ? <p className="text-muted-text">No node has started.</p> : <ol>
        {steps.map((a) => <li key={a.seq} className="mt-1">
          <p className={a.state === "failed" || a.state === "interrupted" ? "text-danger" : "text-body"}>
            {`#${a.seq} ${a.step} attempt ${a.attempt}: ${a.state}${a.next ? ` → ${a.next}` : ""}`}
            <span className="text-fainter">{` (from checkpoint ${a.input_checkpoint})`}</span>
            {a.state === "running" && <span className="text-muted-text"> in flight</span>}
          </p>
          {a.error && <p className="text-danger">{a.error}</p>}
          {a.note && <p className="text-muted-text">{a.note}</p>}
        </li>)}
      </ol>}
      {!finished(run.state) && run.state !== "cancelling" && run.next_step && <p className="text-muted-text mt-1">Next: {run.next_step} from checkpoint {run.checkpoint}.</p>}
    </section>
    {maps.map((m) => <section key={`${m.step}@${m.checkpoint}`} className="mt-3" aria-label={`Map items of ${m.step}`}>
      <h4 className="text-body font-medium">{`Map items of ${m.step} (from checkpoint ${m.checkpoint})`}</h4>
      <ol>{m.items.map((it) => <li key={it.index} className={`${it.state === "failed" || it.state === "interrupted" ? "text-danger" : "text-muted-text"} mt-1`}>
        {`#${it.index} ${JSON.stringify(it.item)}: ${it.state}${it.attempts > 0 ? `, ${it.attempts} attempt${it.attempts === 1 ? "" : "s"}` : ""}${it.execution ? `, worker execution ${it.execution}` : ""}${it.tokens > 0 ? `, ${it.tokens} tokens, ${dollars(it.microUSD)}` : ""}${it.error ? ` - ${it.error}` : ""}`}
      </li>)}</ol>
    </section>)}
    {workers.length > 0 && <section className="mt-3" aria-label="Workers">
      <h4 className="text-body font-medium">Workers</h4>
      <ol>{workers.map((c) => <li key={c.attempt_seq} className="text-muted-text mt-1">{workerLine(c)}</li>)}</ol>
      <p className="text-muted-text mt-1">{`Worker spend: ${workerSpend.tokens} tokens, ${dollars(workerSpend.microUSD)}`}</p>
    </section>}
    {interruptions.some((i) => i.resolution) && <section className="mt-3">
      <h4 className="text-body font-medium">Interruptions</h4>
      <ol>{interruptions.filter((i) => i.resolution).map((i) => <li key={i.seq} className="text-muted-text mt-1">{resolutionLine(i)}</li>)}</ol>
    </section>}
    {calls.length > 0 && <section className="mt-3">
      <h4 className="text-body font-medium">Model calls</h4>
      <ol>{calls.map((c) => <li key={c.attempt_seq} className="text-muted-text mt-1">
        {`${c.step} attempt #${c.attempt_seq}: ${c.policy} v${c.policy_version} ${c.model}, ${modelCallSpend(c).tokens} tokens${c.finished_at ? "" : " (estimated: it may have been billed)"}${c.error ? `; ${c.error}` : ""}`}
      </li>)}</ol>
      <p className="text-muted-text mt-1">{`AI spend: ${spend.tokens} tokens, $${(spend.microUSD / 1_000_000).toFixed(6)}`}</p>
    </section>}
    {run.outputs && <section className="mt-3">
      <h4 className="text-body font-medium">Outputs</h4>
      <pre className="font-mono whitespace-pre-wrap">{json(run.outputs)}</pre>
    </section>}
    {detail.events && detail.events.length > 0 && <section className="mt-3">
      <h4 className="text-body font-medium">Recovery</h4>
      <ol>
        {detail.events.map((e) => <li key={e.seq} className="text-muted-text mt-1">{`${when(e.at)} ${e.kind} by ${e.holder} (lease ${e.token}): ${e.detail}`}</li>)}
      </ol>
    </section>}
    <button type="button" onClick={onClose} className="text-body mt-3 underline underline-offset-2">Close</button>
  </div>;
}

export function WorkflowRunStart({workflows, busy, onStart}: {
  workflows: string[];
  busy: boolean;
  onStart: (name: string, inputs: string) => void;
}) {
  const [name, setName] = useState("");
  const [inputs, setInputs] = useState("{}");
  const selected = workflows.includes(name) ? name : workflows[0] ?? "";
  return <form className="mb-4 flex flex-col gap-2 text-xs" onSubmit={(event) => { event.preventDefault(); if (selected) onStart(selected, inputs); }}>
    {workflows.length === 0
      ? <p className="text-muted-text">No active workflow can be started. Approve one above first.</p>
      : <label className="text-muted-text flex items-center gap-2">Workflow
        <select value={selected} onChange={(event) => setName(event.target.value)} className="border-hair bg-app text-body rounded-[6px] border px-2 py-1 font-mono">
          {workflows.map((w) => <option key={w} value={w}>{w}</option>)}
        </select>
      </label>}
    <label className="text-muted-text flex flex-col gap-1">Inputs (a JSON object)
      <textarea value={inputs} onChange={(event) => setInputs(event.target.value)} rows={3} spellCheck={false} className="border-hair bg-app text-body rounded-[6px] border px-2 py-1 font-mono" />
    </label>
    <div><button type="submit" disabled={busy || !selected} className="bg-teal text-primary-foreground rounded-[6px] px-3 py-1 font-semibold disabled:opacity-40">Start run</button></div>
  </form>;
}
