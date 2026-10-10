import { useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { repositoryAPI, type GitHubStatus, type Repository, type RepositoryAPI } from "../api/repository";
import { repositoryName } from "./repositoryName";

// Settings → Repository (changes.spec.md stories 1 and 2a): what the
// Workspace folder is as a Repository, none, local or on GitHub, whether
// the GitHub token can reach it, and Publish to GitHub for a local one.

export type PublishState = {
  open: boolean;
  account?: GitHubStatus;
  busy: boolean;
  problem?: string;
};

export function PublishForm({defaultName, state, onPublish, onCancel}: {
  defaultName: string;
  state: PublishState;
  onPublish: (name: string, isPrivate: boolean) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState(defaultName);
  const [isPrivate, setPrivate] = useState(true);
  const [understood, setUnderstood] = useState(false);
  const connected = state.account?.state === "connected";
  const ready = connected && name.trim() !== "" && (isPrivate || understood) && !state.busy;
  return <form aria-label="Publish to GitHub" className="border-hair bg-card mt-4 max-w-[620px] rounded-[8px] border p-4 text-xs"
    onSubmit={(event) => { event.preventDefault(); if (ready) onPublish(name.trim(), isPrivate); }}>
    <p className="text-body leading-5">Creates a repository on {connected ? <span className="font-mono">{state.account?.login}</span> : "your"} GitHub account, sets it as origin and pushes the base branch. Other branches stay on this Mac.</p>
    {!state.account ? <p className="text-muted-text mt-3">Checking GitHub…</p>
      : !connected && <p role="alert" className="text-amber mt-3">{state.account.message}</p>}
    <label className="text-muted-text mt-3 flex flex-col gap-1">Repository name
      <input value={name} onChange={(event) => setName(event.target.value)} maxLength={100} spellCheck={false}
        className="border-hair-input bg-code text-ink focus:border-teal max-w-[320px] rounded-md border px-2.5 py-1.5 font-mono outline-none" />
    </label>
    <fieldset className="mt-3 flex gap-5">
      <legend className="sr-only">Visibility</legend>
      <label className="text-body flex items-center gap-2"><input type="radio" name="visibility" className="accent-teal" checked={isPrivate} onChange={() => setPrivate(true)} />Private</label>
      <label className="text-body flex items-center gap-2"><input type="radio" name="visibility" className="accent-teal" checked={!isPrivate} onChange={() => { setPrivate(false); setUnderstood(false); }} />Public</label>
    </fieldset>
    {!isPrivate && <label className="text-amber mt-3 flex items-center gap-2"><input type="checkbox" className="accent-teal" checked={understood} onChange={(event) => setUnderstood(event.target.checked)} />Anyone on the internet can read this code and its history</label>}
    {state.problem && <p role="alert" className="text-danger mt-3 leading-5">{state.problem}</p>}
    <div className="mt-4 flex items-center gap-3">
      <button type="submit" disabled={!ready} className="bg-teal text-primary-foreground rounded-[6px] px-3 py-1.5 font-semibold disabled:opacity-40">{state.busy ? "Publishing…" : state.problem ? "Try again" : "Publish to GitHub"}</button>
      <button type="button" disabled={state.busy} onClick={onCancel} className="text-muted-text disabled:opacity-40">Cancel</button>
    </div>
  </form>;
}

export function RepositoryView({repository, publish, onOpenPublish, onPublish, onCancelPublish}: {
  repository: Repository;
  publish?: PublishState;
  onOpenPublish?: () => void;
  onPublish?: (name: string, isPrivate: boolean) => void;
  onCancelPublish?: () => void;
}) {
  const github = repository.github;
  const link = github?.url ?? (repository.owner ? `https://github.com/${repository.owner}/${repository.name}` : undefined);
  const publishable = repository.kind === "local" && !repository.remote_url;
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
    {publishable && onOpenPublish && !publish?.open && <button type="button" onClick={onOpenPublish} className="border-teal-hair text-teal-hover hover:bg-teal-deep mt-4 rounded-[7px] border px-3 py-[6px] text-xs">Publish to GitHub</button>}
    {publishable && publish?.open && onPublish && onCancelPublish &&
      <PublishForm defaultName={repositoryName(repository.folder)} state={publish} onPublish={onPublish} onCancel={onCancelPublish} />}
  </section>;
}

/** Reads the Workspace's Repository when Settings opens, again when its folder changes, and publishes a local one. */
export function WorkspaceRepository({workspace, api = repositoryAPI}: {workspace: Workspace; api?: RepositoryAPI}) {
  const [repository, setRepository] = useState<Repository>();
  const [problem, setProblem] = useState("");
  const [publish, setPublish] = useState<PublishState>({open: false, busy: false});
  useEffect(() => {
    let live = true;
    api.read(workspace.id).then((value) => { if (live) setRepository(value); }, (error) => { if (live) setProblem(error instanceof Error ? error.message : "The repository could not be read."); });
    return () => { live = false; };
  }, [api, workspace.id, workspace.folder?.revision]);
  const open = () => {
    setPublish({open: true, busy: false});
    api.account().then((account) => setPublish((current) => ({...current, account})),
      () => setPublish((current) => ({...current, account: {state: "unreachable", message: "GitHub could not be checked."}})));
  };
  const submit = (name: string, isPrivate: boolean) => {
    setPublish((current) => ({...current, busy: true, problem: undefined}));
    api.publish(workspace.id, name, isPrivate).then((value) => { setRepository(value); setPublish({open: false, busy: false}); },
      (error) => setPublish((current) => ({...current, busy: false, problem: error instanceof Error ? error.message : "The repository could not be published."})));
  };
  if (repository) {
    return <RepositoryView repository={repository} publish={publish} onOpenPublish={workspace.state === "active" ? open : undefined}
      onPublish={submit} onCancelPublish={() => setPublish({open: false, busy: false})} />;
  }
  return <section aria-label="Repository" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Repository</h2>
    {problem ? <p role="alert" className="text-danger text-xs">{problem}</p> : <p className="text-muted-text text-xs">Reading the repository…</p>}
  </section>;
}
