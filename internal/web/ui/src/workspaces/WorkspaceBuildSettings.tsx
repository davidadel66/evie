import { useEffect, useState } from "react";
import { setWorkspaceBuild, type Workspace } from "../api/contextSessions";

function stateText(workspace: Workspace): string {
  switch (workspace.build?.setting ?? "default") {
    case "on": return "Turned on.";
    case "off": return "Turned off.";
    default: return "Off by default: writing code is a bigger grant than reading it.";
  }
}

// Build workers (Stage 11) write code in their own git worktree, so they
// need the owner's explicit on; there is no folder default as for explore.
export function WorkspaceBuildSettings({workspace, onChanged}: {workspace: Workspace; onChanged?: () => Promise<void>}) {
  const [confirmed, setConfirmed] = useState(workspace);
  const [saving, setSaving] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => { setConfirmed(workspace); }, [workspace]);
  const save = async (checked: boolean) => {
    setSaving(true); setProblem(""); setMessage("");
    try {
      const saved = await setWorkspaceBuild(confirmed.id, confirmed.currentRevisionId, checked ? "on" : "off");
      setConfirmed(saved);
      setMessage(saved.build?.allowed ? "Build workers allowed. New chats can use them." : "Build workers turned off. Active build workers are stopping.");
      try { await onChanged?.(); }
      catch { setProblem("Permission saved, but the Workspace list could not refresh. Refresh before starting a new chat."); }
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Build permission could not be confirmed. Refresh before trying again.");
    } finally { setSaving(false); }
  };
  return <section aria-label="Build workers" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Build workers</h2>
    {(confirmed.defaultPresetId ?? "standard") === "standard" ? <>
      <label className="text-body flex items-center gap-2 text-xs"><input type="checkbox" className="accent-teal" checked={confirmed.build?.allowed ?? false} disabled={saving || confirmed.state !== "active"} onChange={event => void save(event.target.checked)} />Allow build workers</label>
      <p className="text-muted-text mt-2 max-w-[620px] text-xs leading-5">{stateText(confirmed)} Each worker changes code for one Task in its own git worktree on a new branch of this Workspace&apos;s folder, which must be a git repository, and runs tests in a sandbox with no network. Nothing is merged until you approve it. Turning this on applies to new chats; turning it off stops active build workers, and chats it stopped stay restricted if you turn it on again.</p>
    </> : <p className="text-muted-text text-xs leading-5">The Research preset cannot delegate. Use a Standard Workspace for build workers.</p>}
    <p role="status" className="text-muted-text mt-2 text-xs">{saving ? "Saving permission…" : message}</p>
    {problem && <div className="mt-3 text-xs"><p role="alert" className="text-danger">{problem}</p>{onChanged && <button type="button" disabled={saving} onClick={() => void onChanged().then(()=>setProblem(""),()=>setProblem("Workspace permissions could not be refreshed."))} className="text-body mt-2 underline underline-offset-2">Refresh permissions</button>}</div>}
  </section>;
}
