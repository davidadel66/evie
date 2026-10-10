import type { WorkflowCapStatus, WorkflowNotification } from "../api/workflows";

// Durable workflow notifications and the daily cap line (Stage 14 sub-stage
// 11). The Workspace page no longer lists notices: the app-wide
// notifications icon does (workspace-activity.spec.md). These two views
// stay for the design system.

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
