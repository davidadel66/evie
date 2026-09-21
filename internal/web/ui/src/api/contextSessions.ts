export type Workspace = {
  defaultPresetId?: string;
  allowedPresetIds?: string[];
  instructions?: {enabled:boolean;revision:number};
	 folder?: { path: string; revision: number };
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
  ownerDisplayName?: string;
  workspaces: Workspace[];
  projects: Project[];
  sessions: StoredSession[];
  archivedSessions?: StoredSession[];
  activeSession?: StoredSession;
  activeScope?: ContextScope;
};

export type ContextSessionSelection =
  | { sessionId: string }
  | { workspaceId: string; workspaceRevision: string }
  | { projectId: string }
  | { unscoped: true };

export type OpenedContextSession = { session: StoredSession; scope: ContextScope };

async function postJSON<T>(path: string, body: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    ...(signal ? { signal } : {}),
  });
  const value = (await response.json()) as T & { error?: string };
  if (!response.ok) throw new Error(value.error ?? `Request failed (${response.status})`);
  return value;
}

export function listContextSessions(): Promise<ContextSessionSnapshot> {
  return postJSON("/api/context-sessions/list", {});
}

export type WorkspaceCreation = {
  displayName: string;
  presetId?: string;
  folderPath?: string;
  createFolder?: boolean;
};

export function registerWorkspace(options: WorkspaceCreation): Promise<Workspace> {
  return postJSON("/api/workspaces/register", options);
}

export type ChosenWorkspaceFolder = { path: string; cancelled: boolean };

export function chooseWorkspaceFolder(signal?: AbortSignal): Promise<ChosenWorkspaceFolder> {
  return postJSON("/api/workspaces/choose-folder", {}, signal);
}

export function archiveSession(sessionId: string): Promise<{session: StoredSession}> {
  return postJSON("/api/context-sessions/archive", { sessionId });
}

export function restoreSession(sessionId: string): Promise<{session: StoredSession}> {
  return postJSON("/api/context-sessions/restore", { sessionId });
}

export function selectContextSession(selection: ContextSessionSelection): Promise<OpenedContextSession> {
  return postJSON("/api/context-sessions/select", selection);
}
