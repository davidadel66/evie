// A Workspace's Changes (changes.spec.md): its Repository, the branches of
// work in it, one Change with its diff, and the owner's actions on them.

export type CheckState = "pending" | "running" | "passed" | "failed" | "neutral";
export type Check = { name: string; state: CheckState; url?: string };

export type PullRequest = {
  number: number;
  url: string;
  /** GitHub's: open, merged or closed. */
  state: string;
  /** GitHub's own draft marking, which blocks merging. */
  draft?: boolean;
  head_sha: string;
  author: string;
  mergeable?: boolean;
  updated_at: string;
};

export type Change = {
  /** "build:<branch>" for Evie's, "pr:<number>" for the owner's pull requests. */
  id: string;
  workspace_id: string;
  branch: string;
  base: string;
  state: "draft" | "open" | "merged" | "closed";
  needs_rebase?: boolean;
  author: "evie" | "owner";
  origin?: { session_id?: string; run_id?: string; workflow?: string; step?: string };
  title: string;
  notes?: string;
  head?: string;
  worktree?: string;
  files_changed?: number;
  insertions?: number;
  deletions?: number;
  execution_id?: string;
  pull_request?: PullRequest;
  checks?: Check[];
  /** When the owner set "Merge when checks pass". */
  armed_at?: string;
  updated_at: string;
};

export type Repository = {
  kind: "none" | "local" | "github";
  folder?: string;
  problem?: string;
  base_branch?: string;
  owner?: string;
  name?: string;
  behind?: number;
};

export type ComparedFile = {
  path: string;
  old_path?: string;
  status: "added" | "modified" | "deleted" | "renamed" | "copied";
  additions: number;
  deletions: number;
  binary?: boolean;
  /** The file's object at the head, so a "Viewed" mark resets when it changes. */
  blob?: string;
  /** The file's hunks, from its first "@@" line. */
  patch?: string;
  excluded?: boolean;
  truncated?: boolean;
};

export type ChangeDetail = { repository: Repository; change: Change; files: ComparedFile[]; truncated?: boolean; problem?: string };
export type ChangeList = { repository: Repository; changes: Change[] };
export type MergeResult = { change: Change; status: string; notes?: string[] };

/** A refusal from a Changes route, with what the merge gate ran when it ran. */
export class ChangeActionError extends Error {
  readonly code?: string;
  constructor(message: string, code?: string) {
    super(message);
    this.code = code;
  }
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/changes/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json().catch(() => ({}));
  if (!response.ok) throw new ChangeActionError(value.error ?? `Changes request failed (${response.status})`, value.code);
  return value as T;
}

export const changesAPI = {
  list: (workspaceId: string): Promise<ChangeList> => post("list", { workspace_id: workspaceId }),
  show: (workspaceId: string, changeId: string): Promise<ChangeDetail> => post("show", { workspace_id: workspaceId, change_id: changeId }),
  /** Asks GitHub for the Workspace's pull requests now. */
  refresh: (workspaceId: string): Promise<unknown> => post("refresh", { workspace_id: workspaceId }),
  merge: (workspaceId: string, changeId: string): Promise<MergeResult> => post("merge", { workspace_id: workspaceId, change_id: changeId }),
  arm: (workspaceId: string, changeId: string): Promise<Change> => post("merge", { workspace_id: workspaceId, change_id: changeId, when_checks_pass: true }),
  disarm: (workspaceId: string, changeId: string): Promise<Change> => post("disarm", { workspace_id: workspaceId, change_id: changeId }),
  close: (workspaceId: string, changeId: string): Promise<{ change: Change; notes?: string[] }> =>
    post("close", { workspace_id: workspaceId, change_id: changeId, confirm: true }),
  push: (workspaceId: string, branch: string): Promise<{ change: Change; pull_request: number; url: string }> =>
    post("push", { workspace_id: workspaceId, branch }),
};
export type ChangesAPI = typeof changesAPI;
