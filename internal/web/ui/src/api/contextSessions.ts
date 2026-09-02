export type Workspace = {
  id: string;
  displayName: string;
  state: "active" | "archived";
  currentRevisionId: string;
  createdAt: string;
  updatedAt: string;
};

export type Project = {
  id: string;
  displayName: string;
  canonicalRoot: string;
  archived: boolean;
  createdAt: string;
  updatedAt: string;
};

export type StoredSession = {
  id: string;
  workspaceId?: string;
  workspaceRevisionSnapshot?: string;
  projectId?: string;
  projectRootSnapshot?: string;
  title: string;
  status: "active" | "closed";
  createdAt: string;
  updatedAt: string;
  activityAt?: string;
};

export type ContextScope = {
  kind: "workspace" | "project" | "unscoped";
  displayName: string;
  workspaceId?: string;
  workspaceRevision?: string;
  projectId?: string;
  projectRoot?: string;
};

export type ContextSessionSnapshot = {
  workspaces: Workspace[];
  projects: Project[];
  sessions: StoredSession[];
  activeSession?: StoredSession;
  activeScope?: ContextScope;
};

export type ContextSessionSelection =
  | { sessionId: string }
  | { workspaceId: string; workspaceRevision: string }
  | { projectId: string }
  | { unscoped: true };

export type OpenedContextSession = { session: StoredSession; scope: ContextScope };

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const value = (await response.json()) as T & { error?: string };
  if (!response.ok) throw new Error(value.error ?? `Request failed (${response.status})`);
  return value;
}

export function listContextSessions(): Promise<ContextSessionSnapshot> {
  return postJSON("/api/context-sessions/list", {});
}

export function registerWorkspace(displayName: string): Promise<Workspace> {
  return postJSON("/api/workspaces/register", { displayName });
}

export function selectContextSession(selection: ContextSessionSelection): Promise<OpenedContextSession> {
  return postJSON("/api/context-sessions/select", selection);
}
