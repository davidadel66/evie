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

async function post<T>(path: string, body: unknown, failure: string): Promise<T> {
  const response = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json();
  if (!response.ok) throw new Error(value.error ?? `${failure} (${response.status})`);
  return value as T;
}

export const repositoryAPI = {
  read: (workspaceId: string): Promise<Repository> => post("/api/workspaces/repository", { workspace_id: workspaceId }, "The repository could not be read"),
  /** Creates the GitHub repository on the token owner's account, sets origin and pushes the base branch. */
  publish: (workspaceId: string, name: string, isPrivate: boolean): Promise<Repository> =>
    post("/api/workspaces/repository/publish", { workspace_id: workspaceId, name, private: isPrivate }, "The repository could not be published"),
  /** The token check alone. */
  account: (): Promise<GitHubStatus> => post("/api/github/status", {}, "GitHub could not be checked"),
  /** The token owner's own repositories, most recently updated first. */
  githubRepositories: (): Promise<{ repositories: GitHubRepository[] }> => post("/api/github/repositories", {}, "Your GitHub repositories could not be listed"),
};

export type GitHubRepository = { name: string; full_name: string; private: boolean; default_branch?: string; owner: { login: string } };
export type RepositoryAPI = typeof repositoryAPI;
