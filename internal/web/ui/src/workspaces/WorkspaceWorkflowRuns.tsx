import { useCallback, useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import {
  parseRunInputs, workflowAPI, workflowRunAPI,
  type WorkflowAPI, type WorkflowRun, type WorkflowRunAPI, type WorkflowRunDetail, type WorkflowRunState,
} from "../api/workflows";

// Runs of this Workspace's workflows (Stage 14 sub-stage 3). David starts a
// run of an active workflow with its inputs; Evie executes its deterministic
// and branch steps, recording each node attempt and checkpoint, before the
// response. A start whose Run Key already has a run shows that run instead.
// Definitions with other step kinds are refused until the runtime supports
// them.

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
      {run.error && <p className="text-danger mt-1 leading-5">{run.error}</p>}
    </li>)}
  </ul>;
}

export function WorkflowRunView({detail, onClose}: {detail: WorkflowRunDetail; onClose: () => void}) {
  const run = detail.run;
  return <div role="dialog" aria-label={`Run ${run.id}`} className="border-hair bg-app mt-3 rounded-[8px] border p-4 text-xs leading-5">
    <h3 className="text-body mb-2 text-sm font-medium">{run.workflow} version {run.version}: {stateLabel[run.state]}</h3>
    <p className="text-muted-text">Run <code className="font-mono">{run.id}</code>, started from the {run.surface === "web" ? "web" : "CLI"}{run.session_id ? ` in session ${run.session_id}` : ""}.</p>
    <p className="text-muted-text">Run key <code className="font-mono">{run.run_key}</code></p>
    <p className="text-muted-text">Executable hash <code className="font-mono">{run.executable_hash}</code></p>
    <p className="text-muted-text">Pinned to Workspace revision {run.workspace_revision}, access revision {run.access_revision}.</p>
    {run.error && <p role="alert" className="text-danger mt-2">{run.error}</p>}
    <section className="mt-3">
      <h4 className="text-body font-medium">Inputs</h4>
      <pre className="font-mono whitespace-pre-wrap">{json(run.inputs)}</pre>
    </section>
    <section className="mt-3">
      <h4 className="text-body font-medium">Nodes</h4>
      {detail.attempts.length === 0 ? <p className="text-muted-text">No node has started.</p> : <ol>
        {detail.attempts.map((a) => <li key={a.seq} className="mt-1">
          <p className={a.state === "failed" ? "text-danger" : "text-body"}>
            {`#${a.seq} ${a.step} attempt ${a.attempt}: ${a.state}${a.next ? ` → ${a.next}` : ""}`}
            <span className="text-fainter">{` (from checkpoint ${a.input_checkpoint})`}</span>
            {a.state === "running" && <span className="text-muted-text"> in flight</span>}
          </p>
          {a.error && <p className="text-danger">{a.error}</p>}
          {a.note && <p className="text-muted-text">{a.note}</p>}
        </li>)}
      </ol>}
      {!["succeeded", "failed", "cancelled"].includes(run.state) && run.next_step && <p className="text-muted-text mt-1">Next: {run.next_step} from checkpoint {run.checkpoint}.</p>}
    </section>
    {run.outputs && <section className="mt-3">
      <h4 className="text-body font-medium">Outputs</h4>
      <pre className="font-mono whitespace-pre-wrap">{json(run.outputs)}</pre>
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

export function WorkspaceWorkflowRuns({workspace, api = workflowRunAPI, definitionsAPI = workflowAPI}: {
  workspace: Workspace;
  api?: WorkflowRunAPI;
  definitionsAPI?: WorkflowAPI;
}) {
  const [runs, setRuns] = useState<WorkflowRun[] | null>(null);
  const [active, setActive] = useState<string[]>([]);
  const [detail, setDetail] = useState<WorkflowRunDetail | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  const refresh = useCallback(async () => {
    const [page, definitions] = await Promise.all([api.runs(workspace.id), definitionsAPI.list(workspace.id)]);
    setRuns(page.runs);
    setActive(definitions.workflows.filter((d) => d.status === "active").map((d) => d.name));
  }, [api, definitionsAPI, workspace.id]);
  useEffect(() => {
    refresh().catch(() => setProblem("Workflow runs could not be loaded."));
  }, [refresh]);
  const act = async (action: () => Promise<string>) => {
    setBusy(true); setProblem(""); setMessage("");
    try { setMessage(await action()); }
    catch (error) { setProblem(error instanceof Error ? error.message : "The workflow request failed. Refresh before trying again."); }
    finally { setBusy(false); }
  };
  const start = (name: string, text: string) => act(async () => {
    const inputs = parseRunInputs(text);
    if (!inputs) throw new Error("Inputs must be one JSON object of input names to values.");
    const result = await api.start(workspace.id, name, inputs);
    setDetail(result.run);
    await refresh();
    const run = result.run.run;
    return result.existing
      ? `A run of ${run.workflow} already holds this Run Key (${stateLabel[run.state].toLowerCase()}); nothing new was started.`
      : `Run of ${run.workflow} ${stateLabel[run.state].toLowerCase()}.`;
  });
  const inspect = (run: WorkflowRun) => act(async () => {
    setDetail(await api.inspect(run.id));
    return "";
  });
  return <section aria-label="Workflow runs" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Workflow runs</h2>
    <p className="text-muted-text mb-3 max-w-[620px] text-xs leading-5">Start an active workflow with its inputs. Each run is pinned to the approved version and records every step it takes; a run whose Run Key already ran is shown instead of starting again. Workflows with AI, agent, map, capability or interruption steps cannot run yet.</p>
    {workspace.state === "active" && <WorkflowRunStart workflows={active} busy={busy} onStart={(name, inputs) => void start(name, inputs)} />}
    {runs === null ? <p className="text-muted-text text-xs">Loading runs…</p>
      : <WorkflowRunList runs={runs} busy={busy} onInspect={(run) => void inspect(run)} />}
    {detail && <WorkflowRunView detail={detail} onClose={() => setDetail(null)} />}
    <p role="status" className="text-muted-text mt-2 text-xs">{busy ? "Working…" : message}</p>
    {problem && <p role="alert" className="text-danger mt-2 text-xs">{problem}</p>}
  </section>;
}
