import { ago } from "../ui/time";
import type { Notice } from "../api/notifications";

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
