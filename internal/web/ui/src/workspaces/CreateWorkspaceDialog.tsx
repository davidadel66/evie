import { useEffect, useId, useRef, useState } from "react";
import { chooseWorkspaceFolder, type WorkspaceCreation } from "../api/contextSessions";
import { repositoryAPI, type GitHubRepository, type GitHubStatus } from "../api/repository";
import { Folder } from "../ui/Icon";
import { listPresets, type PresetInspection } from "../api/management";
import { Dialog } from "../ui/Dialog";
import { defaultFolder, presetDescription, presetLabel, workspaceCreation, type RepositoryKind, type WorkspaceDraft } from "./workspaceCreation";

const input = "border-hair-input bg-card text-ink placeholder:text-faint focus-visible:ring-teal w-full min-w-0 rounded-lg border px-3 py-2.5 text-[13px] focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50";
const initialDraft: WorkspaceDraft = { name: "", presetId: "standard", folderPath: "", repository: "none" };

export function CreateWorkspaceDialog({ onClose, onCreate }: { onClose: () => void; onCreate: (options: WorkspaceCreation) => Promise<void> }) {
  const [draft, setDraft] = useState(initialDraft);
  const [presets, setPresets] = useState<PresetInspection[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadProblem, setLoadProblem] = useState("");
  const [problem, setProblem] = useState("");
  const [retry, setRetry] = useState(0);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const [pickingFolder, setPickingFolder] = useState(false);
  const [folderProblem, setFolderProblem] = useState("");
  const pickerRequest = useRef<AbortController | null>(null);
  const pickerButton = useRef<HTMLButtonElement>(null);
  const nameId = useId();
  const presetId = useId();
  const presetHelpId = useId();
  const folderHelpId = useId();
  const folderInputId = useId();
  const [account, setAccount] = useState<GitHubStatus>();
  const [repos, setRepos] = useState<GitHubRepository[] | null>(null);
  const [reposProblem, setReposProblem] = useState("");

  useEffect(() => {
    let current = true;
    repositoryAPI.account().then((value) => { if (current) setAccount(value); },
      () => { if (current) setAccount({state: "unreachable", message: "GitHub could not be checked."}); });
    return () => { current = false; };
  }, []);

  useEffect(() => {
    if (draft.repository !== "github_existing" || repos !== null) return;
    let current = true;
    repositoryAPI.githubRepositories().then((value) => { if (current) setRepos(value.repositories); },
      (error: unknown) => { if (current) { setRepos([]); setReposProblem(error instanceof Error ? error.message : "Your GitHub repositories could not be listed."); } });
    return () => { current = false; };
  }, [draft.repository, repos]);

  useEffect(() => {
    let current = true;
    setLoading(true);
    setLoadProblem("");
    listPresets().then(({ presets: available }) => {
      if (current) setPresets(available);
    }).catch((error: unknown) => {
      if (current) setLoadProblem(error instanceof Error ? error.message : "Agent presets could not be loaded.");
    }).finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [retry]);

  useEffect(() => () => { pickerRequest.current?.abort(); }, []);

  const chooseFolder = async () => {
    if (pickerRequest.current || savingRef.current) return;
    const request = new AbortController();
    pickerRequest.current = request;
    setPickingFolder(true);
    setFolderProblem("");
    try {
      const choice = await chooseWorkspaceFolder(request.signal);
      if (!request.signal.aborted && !choice.cancelled) {
        setDraft(current => ({ ...current, folderPath: choice.path }));
        setProblem("");
      }
    } catch (error: unknown) {
      if (!request.signal.aborted) setFolderProblem(error instanceof Error ? error.message : "The folder picker could not be opened. Try again.");
    } finally {
      if (!request.signal.aborted) {
        pickerRequest.current = null;
        setPickingFolder(false);
        requestAnimationFrame(() => pickerButton.current?.focus());
      }
    }
  };

  const selectedPreset = presets.find((preset) => preset.id === draft.presetId);
  const update = (patch: Partial<WorkspaceDraft>) => { setDraft((value) => ({ ...value, ...patch })); setProblem(""); };
  const submit = async () => {
    if (savingRef.current || pickerRequest.current || loading || loadProblem) return;
    let options: WorkspaceCreation;
    try {
      options = workspaceCreation(draft, presets);
    } catch (error: unknown) {
      setProblem(error instanceof Error ? error.message : "Check the workspace details.");
      return;
    }
    savingRef.current = true;
    setSaving(true);
    setProblem("");
    try {
      await onCreate(options);
    } catch (error: unknown) {
      setProblem(error instanceof Error ? error.message : "Workspace could not be created. Try again.");
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  return (
    <Dialog title="Create workspace" description="Give your conversations a place of their own." busy={saving || pickingFolder} onClose={onClose}>
      <form onSubmit={(event) => { event.preventDefault(); void submit(); }} className="space-y-5">
        <div>
          <label htmlFor={nameId} className="text-body mb-2 block text-xs font-medium">Workspace name</label>
          <input id={nameId} data-dialog-autofocus required autoComplete="off" value={draft.name} onChange={(event) => update({ name: event.target.value })} disabled={saving} placeholder="e.g. Interview prep" className={input} />
        </div>

        <div>
          <label htmlFor={presetId} className="text-body mb-2 block text-xs font-medium">Agent preset</label>
          <select id={presetId} aria-describedby={presetHelpId} value={draft.presetId} disabled={loading || Boolean(loadProblem) || saving} onChange={(event) => update({ presetId: event.target.value, allowResearchDelegation: false })} className={input}>
            {!presets.some((preset) => preset.id === "standard") && <option value="standard">Standard{loading ? " — loading…" : " — unavailable"}</option>}
            {presets.map((preset) => <option key={preset.id} value={preset.id} disabled={!preset.valid}>{presetLabel(preset.id)}{preset.valid ? "" : " — unavailable"}</option>)}
          </select>
          <p id={presetHelpId} className="text-muted-text mt-2 text-xs leading-5">
            {selectedPreset ? presetDescription(selectedPreset) : "Choose which plugin capabilities this workspace can use."}
          </p>
          {loadProblem && <div className="mt-2 text-xs leading-5"><p role="alert" className="text-danger-ink max-h-24 overflow-y-auto break-words">{loadProblem}</p><button type="button" onClick={() => setRetry((value) => value + 1)} className="text-body focus-visible:ring-teal mt-1 rounded underline underline-offset-2 focus-visible:ring-2 focus-visible:outline-none">Retry</button></div>}
          {!loading && !loadProblem && <PresetDiagnostics preset={selectedPreset} />}
          {draft.presetId === "standard" && <div className="mt-4">
            <label className="text-body flex items-center gap-2 text-xs"><input type="checkbox" className="accent-teal" checked={draft.allowResearchDelegation ?? false} disabled={saving || loading || !presets.some(preset => preset.id === "research" && preset.valid)} onChange={event => update({allowResearchDelegation: event.target.checked})} />Allow research delegation</label>
            <p className="text-muted-text mt-1 pl-5 text-xs leading-5">Let eligible chats send web research to separate workers. Workers receive only the assignment and selected context.</p>
          </div>}
        </div>

        <RepositoryChoiceFields draft={draft} account={account} repos={repos} reposProblem={reposProblem} disabled={saving || pickingFolder}
          onChange={(patch) => {
            const next = {...draft, ...patch};
            // A repository needs a folder: suggest ~/code/<name> until the owner types one.
            if (patch.repository && patch.repository !== "none" && !draft.folderPath.trim()) next.folderPath = defaultFolder(next.repoName || next.repoFullName?.split("/")[1] || next.name);
            if (patch.repository === "github_new" && !draft.repoName) next.repoName = defaultFolder(draft.name).replace("~/code/", "");
            if (patch.repoFullName && (!draft.folderPath.trim() || draft.folderPath === defaultFolder(draft.repoFullName?.split("/")[1] ?? draft.name))) next.folderPath = defaultFolder(patch.repoFullName.split("/")[1] ?? "");
            update(next);
          }} />

        <fieldset disabled={saving || pickingFolder}>
          <legend className="text-body mb-2 text-xs font-medium">{folderLabel(draft.repository ?? "none")}</legend>
          <div className="flex gap-2">
            <input id={folderInputId} aria-label="Folder path" aria-describedby={folderHelpId} value={draft.folderPath} onChange={(event) => { update({ folderPath: event.target.value }); setFolderProblem(""); }}
              placeholder="~/code/project" spellCheck={false} autoComplete="off" className={`${input} font-mono`} />
            <button ref={pickerButton} type="button" onClick={() => void chooseFolder()} title="Choose a folder in the macOS picker" className="border-hair-input text-body hover:bg-hover hover:text-ink focus-visible:ring-teal flex flex-none items-center gap-2 rounded-lg border px-3 py-2.5 text-xs font-medium focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50">
              <Folder size={14} /> {pickingFolder ? "Choosing…" : "Choose…"}
            </button>
          </div>
          <p id={folderHelpId} className="text-muted-text mt-2 text-xs leading-5">{folderHelp(draft.repository ?? "none")}</p>
          {pickingFolder && <p role="status" className="text-muted-text mt-2 text-xs">Choose a folder in the macOS window, or cancel to return here.</p>}
          {folderProblem && <p role="alert" className="text-danger-ink mt-2 text-xs leading-5">{folderProblem}</p>}
        </fieldset>

        {problem && <p role="alert" className="text-danger-ink max-h-24 overflow-y-auto break-words text-xs leading-5">{problem}</p>}
        <footer className="border-hair flex items-center justify-end gap-2 border-t pt-5">
          <button type="button" disabled={saving || pickingFolder} onClick={onClose} className="text-muted-text hover:bg-hover hover:text-ink focus-visible:ring-teal rounded-lg px-4 py-2.5 text-xs font-medium focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">Cancel</button>
          <button type="submit" disabled={saving || pickingFolder || loading || Boolean(loadProblem) || !draft.name.trim() || !selectedPreset?.valid} className="bg-teal text-primary-foreground hover:bg-teal-hover focus-visible:ring-teal rounded-lg px-4 py-2.5 text-xs font-semibold focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">{saving ? "Creating…" : "Create workspace"}</button>
        </footer>
      </form>
    </Dialog>
  );
}

function folderLabel(kind: RepositoryKind): string {
  return kind === "none" ? "Local folder (optional)" : kind === "local" ? "Folder to create" : kind === "github_new" ? "Folder to clone into" : "Folder";
}

function folderHelp(kind: RepositoryKind): string {
  switch (kind) {
  case "none": return "An existing folder, or leave it empty. Type a path or choose one.";
  case "local": return "Evie creates this folder with a git repository on main and one empty first commit.";
  case "github_new": return "Evie creates the repository on GitHub with a first commit, then clones it into this new folder.";
  case "github_existing": return "Evie clones the repository into this new folder, or uses a clone of it already there.";
  }
}

const choices: { kind: RepositoryKind; label: string; github: boolean }[] = [
  {kind: "none", label: "No repository", github: false},
  {kind: "local", label: "New repository on this Mac", github: false},
  {kind: "github_new", label: "New repository on GitHub", github: true},
  {kind: "github_existing", label: "Connect a GitHub repository", github: true},
];

/** The Repository a new Workspace's folder starts with (changes.spec.md story 2b). GitHub choices need a working token. */
export function RepositoryChoiceFields({draft, account, repos, reposProblem, disabled, onChange}: {
  draft: WorkspaceDraft;
  account?: GitHubStatus;
  repos: GitHubRepository[] | null;
  reposProblem?: string;
  disabled: boolean;
  onChange: (patch: Partial<WorkspaceDraft>) => void;
}) {
  const kind = draft.repository ?? "none";
  const connected = account?.state === "connected";
  return <fieldset disabled={disabled}>
    <legend className="text-body mb-2 text-xs font-medium">Repository</legend>
    <div className="space-y-1.5">
      {choices.map((choice) => <label key={choice.kind} className={`${choice.github && !connected ? "text-faint" : "text-body"} flex items-center gap-2 text-xs`}>
        <input type="radio" name="repository" className="accent-teal" checked={kind === choice.kind} disabled={choice.github && !connected} onChange={() => onChange({repository: choice.kind})} />
        {choice.label}
      </label>)}
    </div>
    <p className={`${connected ? "text-muted-text" : "text-amber"} mt-2 text-xs leading-5`}>{!account ? "Checking GitHub…" : connected ? `GitHub: connected as ${account.login}.` : account.message}</p>
    {kind === "github_new" && <div className="border-hair bg-card mt-3 space-y-3 rounded-lg border p-3 text-xs">
      <label className="text-muted-text flex flex-col gap-1">Repository name
        <input value={draft.repoName ?? ""} onChange={(event) => onChange({repoName: event.target.value})} maxLength={100} spellCheck={false} autoComplete="off" className={`${input} font-mono`} />
      </label>
      <div className="flex gap-5">
        <label className="text-body flex items-center gap-2"><input type="radio" name="repo-visibility" className="accent-teal" checked={!draft.repoPublic} onChange={() => onChange({repoPublic: false})} />Private</label>
        <label className="text-body flex items-center gap-2"><input type="radio" name="repo-visibility" className="accent-teal" checked={!!draft.repoPublic} onChange={() => onChange({repoPublic: true})} />Public</label>
      </div>
      {draft.repoPublic && <p className="text-amber leading-5">Anyone on the internet will be able to read this repository.</p>}
    </div>}
    {kind === "github_existing" && <div className="mt-3 text-xs">
      {repos === null ? <p className="text-muted-text">Loading your repositories…</p>
        : reposProblem ? <p role="alert" className="text-danger-ink">{reposProblem}</p>
          : <label className="text-muted-text flex flex-col gap-1">Repository
            <select value={draft.repoFullName ?? ""} onChange={(event) => onChange({repoFullName: event.target.value})} className={`${input} font-mono`}>
              <option value="" disabled>Choose a repository…</option>
              {repos.map((repo) => <option key={repo.full_name} value={repo.full_name}>{repo.full_name}{repo.private ? " (private)" : ""}</option>)}
            </select>
          </label>}
    </div>}
  </fieldset>;
}

export function PresetDiagnostics({ preset }: { preset?: PresetInspection }) {
  const messages = preset?.valid ? preset.warnings ?? [] : preset?.errors ?? [];
  return <>
    {!preset?.valid && <p role="alert" className="text-danger-ink mt-2 text-xs leading-5">This preset is unavailable. Choose another preset or check plugin settings.</p>}
    {messages.length > 0 && <details className={`${preset?.valid ? "text-amber-ink" : "text-danger-ink"} mt-2 text-xs leading-5`}>
      <summary className="focus-visible:ring-teal cursor-pointer rounded focus-visible:ring-2 focus-visible:outline-none">{preset?.valid ? "Some optional capabilities are unavailable" : "Show unavailable capabilities"}</summary>
      <ul aria-label="Capability details" className="mt-2 max-h-32 list-disc space-y-2 overflow-y-auto pl-5 pr-2 break-words">
        {messages.map((message, index) => <li key={index}>{message}</li>)}
      </ul>
    </details>}
  </>;
}
