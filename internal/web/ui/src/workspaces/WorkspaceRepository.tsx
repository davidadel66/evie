import { useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { repositoryAPI, type Repository, type RepositoryAPI } from "../api/repository";

// Settings → Repository (changes.spec.md story 1): what the Workspace
// folder is as a Repository, none, local or on GitHub, and whether the
// GitHub token can reach it.

export function RepositoryView({repository}: {repository: Repository}) {
  const github = repository.github;
  const link = github?.url ?? (repository.owner ? `https://github.com/${repository.owner}/${repository.name}` : undefined);
  return <section aria-label="Repository" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Repository</h2>
    {repository.kind === "none" ? <>
      <p className="text-body text-xs">No repository</p>
      {repository.problem && <p className="text-muted-text mt-1 text-xs leading-5">{repository.problem}</p>}
    </> : <dl className="grid max-w-[620px] grid-cols-[96px_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-xs">
      <dt className="text-faint">Kept</dt>
      <dd className="text-body">{repository.kind === "github" ? <>
        On GitHub, <a href={link} target="_blank" rel="noreferrer" className="text-teal font-mono underline-offset-2 hover:underline">{repository.owner}/{repository.name}</a>
        {github?.private !== undefined && <span className="text-muted-text">{github.private ? " · private" : " · public"}</span>}
      </> : "On this Mac only"}</dd>
      {repository.base_branch && <><dt className="text-faint">Base branch</dt><dd className="text-body font-mono">{repository.base_branch}</dd></>}
      {repository.folder && <><dt className="text-faint">Folder</dt><dd className="text-muted-text font-mono [overflow-wrap:anywhere]">{repository.folder}</dd></>}
      {repository.kind === "local" && repository.remote_url && <><dt className="text-faint">origin</dt><dd className="text-muted-text font-mono [overflow-wrap:anywhere]">{repository.remote_url}</dd></>}
      {github && <><dt className="text-faint">GitHub</dt>
        <dd className={github.state === "connected" ? "text-ok" : "text-amber"}>{github.state === "connected" ? `Connected as ${github.login}` : github.message}</dd></>}
    </dl>}
  </section>;
}

/** Reads the Workspace's Repository when Settings opens, and again when its folder changes. */
export function WorkspaceRepository({workspace, api = repositoryAPI}: {workspace: Workspace; api?: RepositoryAPI}) {
  const [repository, setRepository] = useState<Repository>();
  const [problem, setProblem] = useState("");
  useEffect(() => {
    let live = true;
    api.read(workspace.id).then((value) => { if (live) setRepository(value); }, (error) => { if (live) setProblem(error instanceof Error ? error.message : "The repository could not be read."); });
    return () => { live = false; };
  }, [api, workspace.id, workspace.folder?.revision]);
  if (repository) return <RepositoryView repository={repository} />;
  return <section aria-label="Repository" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Repository</h2>
    {problem ? <p role="alert" className="text-danger text-xs">{problem}</p> : <p className="text-muted-text text-xs">Reading the repository…</p>}
  </section>;
}
