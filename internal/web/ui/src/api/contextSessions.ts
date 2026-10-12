export type WorkspaceExploreSetting = "default" | "on" | "off";
export type WorkspaceBuildSetting = "default" | "on" | "off";

export type Workspace = {
  defaultPresetId?: string;
  allowedPresetIds?: string[];
  explore?: { setting: WorkspaceExploreSetting; allowed: boolean };
  build?: { setting: WorkspaceBuildSetting; allowed: boolean };
  verify?: WorkspaceVerify;
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
	allowResearchDelegation?: boolean;
  displayName: string;
  presetId?: string;
  folderPath?: string;
  createFolder?: boolean;
  /** The Repository the folder starts with (changes.spec.md story 2b). */
  repository?: RepositoryChoice;
};

export type RepositoryChoice = {
  kind: "local" | "github_new" | "github_existing";
  owner?: string;
  name?: string;
  private?: boolean;
};

export function setWorkspaceResearch(workspaceId: string, revision: string, enabled: boolean): Promise<Workspace> {
  return postJSON("/api/workspaces/research", {workspaceId, revision, enabled});
}

export function setWorkspaceExplore(workspaceId: string, revision: string, setting: WorkspaceExploreSetting): Promise<Workspace> {
  return postJSON("/api/workspaces/explore", {workspaceId, revision, setting});
}

export function setWorkspaceBuild(workspaceId: string, revision: string, setting: WorkspaceBuildSetting): Promise<Workspace> {
  return postJSON("/api/workspaces/build", {workspaceId, revision, setting});
}

/** The merge gate's commands (Stage 12): setup runs with network in each
 *  fresh build worktree; verify gates every merge proposal. */
export type WorkspaceVerify = { setup: string; verify: string; revision: number };

/** Commands Evie found in the folder's repository instructions. */
export type VerifyProposal = { setup?: string; setupSource?: string; verify?: string; verifySource?: string };

export function setWorkspaceVerify(workspaceId: string, revision: number, setup: string, verify: string): Promise<Workspace> {
  return postJSON("/api/workspaces/verify", {workspaceId, revision, setup, verify});
}

export function proposeWorkspaceVerify(workspaceId: string): Promise<VerifyProposal> {
  return postJSON("/api/workspaces/verify/propose", {workspaceId});
}

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

/** archiveWorkspace is the owner's "delete": the Workspace leaves the
 *  sidebar and takes no new sessions or runs; its history is kept. */
export function archiveWorkspace(workspaceId: string): Promise<{workspace: Workspace}> {
  return postJSON("/api/workspaces/archive", { workspaceId });
}

export function restoreWorkspace(workspaceId: string): Promise<{workspace: Workspace}> {
  return postJSON("/api/workspaces/restore", { workspaceId });
}

export function selectContextSession(selection: ContextSessionSelection): Promise<OpenedContextSession> {
  return postJSON("/api/context-sessions/select", selection);
}
