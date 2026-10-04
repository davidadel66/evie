import { useEffect, useState } from "react";
import { proposeWorkspaceVerify, setWorkspaceVerify, type VerifyProposal, type Workspace } from "../api/contextSessions";

// The merge gate's commands (Stage 12): the setup command runs with network
// in each new build worktree before the worker starts; the verify command
// runs offline on a worker's branch before and after its rebase, and only a
// branch that passes both runs is offered for merging. Only David sets them:
// Evie's suggestion from the repository's instructions fills the fields and
// saves nothing.
export function WorkspaceVerifySettings({workspace, onChanged}: {workspace: Workspace; onChanged?: () => Promise<void>}) {
  const current = workspace.verify ?? {setup: "", verify: "", revision: 0};
  const [revision, setRevision] = useState(current.revision);
  const [setup, setSetup] = useState(current.setup);
  const [verify, setVerify] = useState(current.verify);
  const [suggestion, setSuggestion] = useState<VerifyProposal | null>(null);
  const [saving, setSaving] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => {
    setRevision(current.revision);
    setSetup(current.setup);
    setVerify(current.verify);
  }, [current.revision, current.setup, current.verify]);
  const changed = setup !== current.setup || verify !== current.verify;
  const save = async () => {
    setSaving(true); setProblem(""); setMessage("");
    try {
      const saved = await setWorkspaceVerify(workspace.id, revision, setup, verify);
      const next = saved.verify ?? {setup, verify, revision: revision + 1};
      setRevision(next.revision);
      setMessage(next.verify ? "Saved. Merge proposals run this verify command." : "Saved. Without a verify command, Evie cannot propose merges.");
      try { await onChanged?.(); }
      catch { setProblem("Saved, but the Workspace could not refresh. Refresh before changing it again."); }
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "The commands could not be saved. Refresh before trying again.");
    } finally { setSaving(false); }
  };
  const suggest = async () => {
    setProblem(""); setMessage("");
    try {
      const proposal = await proposeWorkspaceVerify(workspace.id);
      setSuggestion(proposal);
      if (!proposal.setup && !proposal.verify) setMessage("The repository's instructions suggest no commands.");
    } catch {
      setProblem("The repository's instructions could not be read.");
    }
  };
  const disabled = saving || workspace.state !== "active";
  return <section aria-label="Merge checks" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Merge checks</h2>
    <p className="text-muted-text mb-3 max-w-[620px] text-xs leading-5">Before Evie offers to merge a build worker&apos;s branch, it runs the verify command on the branch in its worktree with no network, rebases the branch onto its target and runs it again. Merges need your approval every time. The setup command runs once in each new worktree, with network, before the worker starts, so dependencies can be installed.</p>
    <label className="text-body mb-1 block text-xs" htmlFor={`verify-${workspace.id}`}>Verify command</label>
    <input id={`verify-${workspace.id}`} value={verify} disabled={disabled} onChange={event => setVerify(event.target.value)} placeholder="./scripts/verify-change.sh" className="border-hair bg-app text-body mb-3 w-full max-w-[620px] rounded-[6px] border px-2 py-1 font-mono text-xs" />
    <label className="text-body mb-1 block text-xs" htmlFor={`setup-${workspace.id}`}>Setup command (optional, runs with network)</label>
    <input id={`setup-${workspace.id}`} value={setup} disabled={disabled} onChange={event => setSetup(event.target.value)} placeholder="npm ci" className="border-hair bg-app text-body mb-3 w-full max-w-[620px] rounded-[6px] border px-2 py-1 font-mono text-xs" />
    <div className="flex items-center gap-3 text-xs">
      <button type="button" disabled={disabled || !changed} onClick={() => void save()} className="bg-teal text-primary-foreground rounded-[6px] px-3 py-1 font-semibold disabled:opacity-40">Save commands</button>
      <button type="button" disabled={disabled || !workspace.folder?.path} onClick={() => void suggest()} className="text-body underline underline-offset-2 disabled:opacity-40">Suggest from repository instructions</button>
    </div>
    {suggestion && (suggestion.setup || suggestion.verify) && <div role="note" className="text-muted-text mt-3 max-w-[620px] text-xs leading-5">
      <p>Evie found these in the repository&apos;s instructions. Nothing is saved until you choose to.</p>
      {suggestion.verify && <p>Verify: <code className="font-mono">{suggestion.verify}</code> ({suggestion.verifySource})</p>}
      {suggestion.setup && <p>Setup: <code className="font-mono">{suggestion.setup}</code> ({suggestion.setupSource})</p>}
      <button type="button" disabled={disabled} onClick={() => { if (suggestion.verify) setVerify(suggestion.verify); if (suggestion.setup) setSetup(suggestion.setup); }} className="text-body mt-1 underline underline-offset-2">Use these</button>
    </div>}
    {!current.verify && <p className="text-muted-text mt-3 text-xs">No verify command is set, so Evie cannot propose merges for this Workspace.</p>}
    <p role="status" className="text-muted-text mt-2 text-xs">{saving ? "Saving commands…" : message}</p>
    {problem && <p role="alert" className="text-danger mt-2 text-xs">{problem}</p>}
  </section>;
}
