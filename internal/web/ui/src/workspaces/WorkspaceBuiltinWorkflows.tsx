import { useCallback, useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { workflowBuiltinAPI, type WorkflowBuiltin, type WorkflowBuiltinAPI } from "../api/workflowBuiltins";
import { workflowColumns } from "./workflowView";

// Built-in workflows (owner decision 2026-10-04). They are available in
// every Workspace without a proposal or approval: they hold no Standing
// Authority and run only read-only workers the Workspace must allow. David
// turns one off or on here for every Workspace; an approved copy of the
// same name overrides it (this Workspace's, then the Global one).

function where(b: WorkflowBuiltin): string {
  const w = b.workspace;
  if (!w) return "";
  const version = w.version ? ` version ${w.version}` : "";
  switch (w.source) {
    case "workspace": return `This Workspace runs its own approved copy${version}.`;
    case "global": return `This Workspace runs the Global approved copy${version}.`;
    default: return "This Workspace runs the built-in.";
  }
}

export function BuiltinWorkflowList({builtins, workspaceId, busy, onToggle}: {
  builtins: WorkflowBuiltin[];
  workspaceId: string;
  busy: boolean;
  onToggle: (builtin: WorkflowBuiltin, enabled: boolean) => void;
}) {
  if (builtins.length === 0) {
    return <p className="text-muted-text px-2.5 py-3 text-xs">No workflows are built into this Evie.</p>;
  }
  // Rows of the Workflows table (Workflows amendment, 2026-11-02): name with
  // a Built in tag and a one-line description, "From chat" for when it
  // runs, and the On switch for its status; bounds, where this Workspace
  // runs it, problems and the customize command sit under Details.
  return <ul>
    {builtins.map((b) => {
      const blocked = b.enabled ? (b.workspace?.blocked ?? []) : [];
      return <li key={b.name} className={`${workflowColumns} border-hair items-start border-b px-2.5 py-3`}>
        <span className="min-w-0">
          <span className="flex items-baseline gap-2">
            <span className="text-ink font-mono text-[12.5px]">{b.name}</span>
            <span className="border-hair-strong text-muted-text rounded-full border px-1.5 text-[10px] leading-4">Built in</span>
          </span>
          <span className="text-muted-text mt-[3px] block truncate text-xs">{b.description}</span>
          <details className="mt-1 text-xs">
            <summary className="text-faint hover:text-body cursor-pointer">Details</summary>
            <div className="text-muted-text mt-1 max-w-[560px] space-y-1 leading-5">
              <p>{b.description}</p>
              <p className="text-fainter">
                {b.enabled ? "On for every Workspace" : "Off for every Workspace"}
                {!b.enabled && b.control?.reason ? ` (${b.control.reason})` : ""}
                {" · "}runs {b.workers.length > 0 ? `${b.workers.join(", ")} workers` : "no workers"}, at most {b.max_tokens.toLocaleString("en-US")} tokens a run
                {b.version ? ` · version ${b.version}` : ""}
              </p>
              {b.workspace && <p>{where(b)}</p>}
              {blocked.map((reason) => <p key={reason} className="text-danger">Cannot start here yet: {reason}</p>)}
              {b.problem && <p className="text-danger">{b.problem}</p>}
              <p className="text-fainter">Customize it with <code className="font-mono">evie workflows customize {b.name} --workspace {workspaceId}</code> (or <code className="font-mono">--global</code>), then approve the copy.</p>
            </div>
          </details>
        </span>
        <span className="text-muted-text text-xs">From chat</span>
        <span className="text-fainter hidden text-[11.5px] sm:block">See Runs</span>
        <label className="text-body flex items-center justify-end gap-2 text-xs">
          <input type="checkbox" role="switch" className="accent-teal" aria-label={`${b.name} on for every Workspace`} checked={b.enabled} disabled={busy}
            onChange={(event) => onToggle(b, event.target.checked)} />
          {b.enabled ? "On" : "Off"}
        </label>
      </li>;
    })}
  </ul>;
}

export function WorkspaceBuiltinWorkflows({workspace, api = workflowBuiltinAPI}: {workspace: Workspace; api?: WorkflowBuiltinAPI}) {
  const [builtins, setBuiltins] = useState<WorkflowBuiltin[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  const refresh = useCallback(async () => {
    const page = await api.list(workspace.id);
    setBuiltins(page.builtins);
  }, [api, workspace.id]);
  useEffect(() => {
    refresh().catch(() => setProblem("Built-in workflows could not be loaded."));
  }, [refresh]);
  const toggle = async (builtin: WorkflowBuiltin, enabled: boolean) => {
    setBusy(true); setProblem(""); setMessage("");
    try {
      await api.setEnabled(builtin.name, enabled);
      await refresh();
      setMessage(enabled
        ? `Turned ${builtin.name} on for every Workspace: chats can start it again, and its paused runs continue.`
        : `Turned ${builtin.name} off for every Workspace: no new run starts, and its runs in progress pause at their next step.`);
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "The built-in workflow could not be changed. Refresh before trying again.");
    } finally { setBusy(false); }
  };
  return <section aria-label="Built-in workflows">
    {builtins === null ? <p className="text-muted-text px-2.5 py-3 text-xs">Loading built-in workflows…</p>
      : <BuiltinWorkflowList builtins={builtins} workspaceId={workspace.id} busy={busy || workspace.state !== "active"} onToggle={(b, enabled) => void toggle(b, enabled)} />}
    <p role="status" className="text-muted-text mt-2 px-2.5 text-xs">{busy ? "Working…" : message}</p>
    {problem && <p role="alert" className="text-danger mt-2 px-2.5 text-xs">{problem}</p>}
  </section>;
}
