import { ago } from "../ui/time";
import type { Notice } from "../api/notifications";
import type { WorkspaceTab } from "../workspaces/workspaceActivity";

/** What a notice is about, with its Workspace's name when it has one: "Finance · finance-review". */
export function noticeContext(notice: Notice, workspaceNames: Record<string, string>): string {
  const workspace = notice.target.workspace_id ? workspaceNames[notice.target.workspace_id] : undefined;
  return workspace ? `${workspace} · ${notice.label}` : notice.label;
}

/** When a notice arrived, relative to now for the last day. */
export function noticeTime(createdAt: string, now: Date): string {
  return ago(createdAt, now);
}

/** An unread count for a badge. */
export function unreadBadge(unread: number): string {
  return unread > 99 ? "99+" : String(unread);
}

/**
 * Where opening a notice leads in its Workspace: a run opens in the Runs
 * tab, a Change opens its Workspace (the Changes tab comes with
 * changes.spec.md story 6), and any other workflow notice opens the Runs tab.
 */
export function noticeDestination(notice: Notice): { tab?: WorkspaceTab; run?: string } {
  switch (notice.target.kind) {
    case "workflow_run":
      return {tab: "runs", run: notice.target.run_id};
    case "change":
      return {};
    default:
      return {tab: "runs"};
  }
}
