// A Workspace's Repository (changes.spec.md story 1): read from its current
// folder on each request, never stored.

export type RepositoryKind = "none" | "local" | "github";
export type TokenState = "connected" | "no_token" | "invalid_token" | "no_access" | "unreachable";

export type GitHubStatus = { state: TokenState; login?: string; private?: boolean; url?: string; message?: string };

export type Repository = {
  kind: RepositoryKind;
  folder?: string;
  /** Why there is no Repository. */
  problem?: string;
  base_branch?: string;
  /** origin as configured, without credentials. */
  remote_url?: string;
  owner?: string;
  name?: string;
  github?: GitHubStatus;
};

export const repositoryAPI = {
  read: async (workspaceId: string): Promise<Repository> => {
    const response = await fetch("/api/workspaces/repository", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ workspace_id: workspaceId }), cache: "no-store" });
    const value = await response.json();
    if (!response.ok) throw new Error(value.error ?? `The repository could not be read (${response.status})`);
    return value as Repository;
  },
};
export type RepositoryAPI = typeof repositoryAPI;
