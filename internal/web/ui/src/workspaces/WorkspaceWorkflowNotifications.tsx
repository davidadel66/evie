import { useCallback, useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { workflowNotificationAPI, type WorkflowCapStatus, type WorkflowNotification, type WorkflowNotificationAPI } from "../api/workflows";

// Durable workflow notifications on the Workspace home (Stage 14 sub-stage
// 11): every run of this Workspace that finished or needs David (an answer,
// a reconciliation, a pause, the daily cap) and every schedule slot that
// was skipped or could not start leaves a notice here, whether the run was
// started from the CLI, this page, a chat or its schedule. David marks them
// read; nothing else changes them. The line above shows today's spend of
// all workflow runs against the daily cap.

function grouped(n: number): string {
  return n.toLocaleString("en-US");
}

function dollars(micro: number): string {
  return `$${(micro / 1_000_000).toFixed(2)}`;
}

export function WorkflowCapLine({caps}: {caps: WorkflowCapStatus}) {
  const paused = caps.paused_runs.length;
  const pausedText = paused === 0 ? "" : ` ${paused === 1 ? "1 run" : `${paused} runs`} paused at the cap continue by themselves after the reset.`;
  if (!caps.enforced) {
    return <p className="text-muted-text text-xs leading-5">No daily cap is enforced by this Evie. Today workflow runs have used {grouped(caps.spent.tokens)} tokens and {dollars(caps.spent.micro_usd)}.{pausedText}</p>;
  }
  return <p className={`text-xs leading-5 ${caps.reached ? "text-danger" : "text-muted-text"}`}>
    Daily cap: {grouped(caps.spent.tokens)} of {grouped(caps.cap.tokens)} tokens and {dollars(caps.spent.micro_usd)} of {dollars(caps.cap.micro_usd)} used today ({caps.day}); it resets at 00:00 UTC.
    {paused > 0 && <span> {paused === 1 ? "1 run" : `${paused} runs`} paused at the cap; each continues by itself after the reset.</span>}
  </p>;
}

export function WorkflowNotificationList({notifications, busy, onRead}: {
  notifications: WorkflowNotification[];
  busy: boolean;
  onRead: (notification: WorkflowNotification) => void;
}) {
  if (notifications.length === 0) {
    return <p className="text-muted-text text-xs">No unread workflow notifications.</p>;
  }
  return <ul className="border-hair border-t">
    {notifications.map((n) => <li key={n.id} className="border-hair border-b py-2 text-xs leading-5">
      <div className="flex flex-wrap items-baseline gap-2">
        {!n.read_at && <span className="text-teal">Unread</span>}
        <span className="text-fainter">{new Date(n.created_at).toLocaleString()}</span>
      </div>
      <p className="text-body break-words">{n.text}</p>
      {!n.read_at && <button type="button" disabled={busy} onClick={() => onRead(n)} className="text-muted-text underline underline-offset-2 disabled:opacity-40">Mark read</button>}
    </li>)}
  </ul>;
}

export function WorkspaceWorkflowNotifications({workspace, api = workflowNotificationAPI}: {workspace: Workspace; api?: WorkflowNotificationAPI}) {
  const [notifications, setNotifications] = useState<WorkflowNotification[] | null>(null);
  const [caps, setCaps] = useState<WorkflowCapStatus | null>(null);
  const [all, setAll] = useState(false);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const refresh = useCallback(async () => {
    const [page, status] = await Promise.all([api.list(workspace.id, all), api.status(workspace.id)]);
    setNotifications(page.notifications);
    setCaps(status.caps);
  }, [api, workspace.id, all]);
  useEffect(() => {
    refresh().catch(() => setProblem("Workflow notifications could not be loaded."));
  }, [refresh]);
  const act = async (action: () => Promise<unknown>) => {
    setBusy(true); setProblem("");
    try { await action(); await refresh(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "The notification could not be updated. Refresh before trying again."); }
    finally { setBusy(false); }
  };
  const unread = (notifications ?? []).filter((n) => !n.read_at).length;
  return <section aria-label="Workflow notifications" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Workflow notifications</h2>
    <p className="text-muted-text mb-3 max-w-[620px] text-xs leading-5">Runs that finished or need you, and schedule slots that were skipped or could not start. Also on the command line: <code className="font-mono">evie workflows notifications {workspace.id}</code>.</p>
    {caps && <div className="mb-3"><WorkflowCapLine caps={caps} /></div>}
    {notifications === null ? <p className="text-muted-text text-xs">Loading notifications…</p>
      : <WorkflowNotificationList notifications={all ? notifications : notifications.filter((n) => !n.read_at)} busy={busy}
        onRead={(n) => void act(() => api.markRead([n.id]))} />}
    <div className="mt-2 flex gap-3 text-xs">
      {unread > 1 && <button type="button" disabled={busy} onClick={() => void act(() => api.markAllRead(workspace.id))} className="text-body underline underline-offset-2 disabled:opacity-40">Mark all read</button>}
      <button type="button" disabled={busy} onClick={() => setAll(!all)} className="text-muted-text underline underline-offset-2 disabled:opacity-40">{all ? "Show unread only" : "Show read ones too"}</button>
    </div>
    {problem && <p role="alert" className="text-danger mt-2 text-xs">{problem}</p>}
  </section>;
}
