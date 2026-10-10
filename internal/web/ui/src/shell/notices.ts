import type { Notice } from "../api/notifications";

/** What a notice is about, with its Workspace's name when it has one: "Finance · finance-review". */
export function noticeContext(notice: Notice, workspaceNames: Record<string, string>): string {
  const workspace = notice.target.workspace_id ? workspaceNames[notice.target.workspace_id] : undefined;
  return workspace ? `${workspace} · ${notice.label}` : notice.label;
}

/** When a notice arrived, relative to now for the last day: "Just now", "12m ago", "3h ago", then "Oct 5". */
export function noticeTime(createdAt: string, now: Date): string {
  const created = new Date(createdAt);
  const minutes = Math.floor((now.getTime() - created.getTime()) / 60_000);
  if (minutes < 1) return "Just now";
  if (minutes < 60) return `${minutes}m ago`;
  if (minutes < 24 * 60) return `${Math.floor(minutes / 60)}h ago`;
  return created.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

/** An unread count for a badge. */
export function unreadBadge(unread: number): string {
  return unread > 99 ? "99+" : String(unread);
}
