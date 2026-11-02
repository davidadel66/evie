import type { ContextSessionSnapshot, StoredSession } from "../api/contextSessions";

// Chats with no Workspace (ui-chat.spec.md, plain chats amendment,
// 2026-11-02): the server's unscoped sessions, with no filesystem project.

export function isPlainChat(session: Pick<StoredSession, "workspaceId" | "projectId">): boolean {
  return !session.workspaceId && !session.projectId;
}

/** Where a chat lives, as the app names it: its Workspace or project, or
 * No workspace. */
export function sessionScopeLabel(session: StoredSession, snapshot: ContextSessionSnapshot): string {
  if (session.workspaceId) return snapshot.workspaces.find((workspace) => workspace.id === session.workspaceId)?.displayName ?? "Workspace";
  if (session.projectId) return snapshot.projects.find((project) => project.id === session.projectId)?.displayName ?? "Project";
  return "No workspace";
}

/** New chat reopens the open chat instead of making another while that
 * chat has no Workspace and nothing in it yet: no title, no loaded
 * messages, and no history still loading. */
export function reusableChat(active: StoredSession | undefined, messages: number, loading: boolean): boolean {
  return !!active && isPlainChat(active) && !active.title.trim() && messages === 0 && !loading;
}
