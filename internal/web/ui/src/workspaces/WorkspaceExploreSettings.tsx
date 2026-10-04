import { useEffect, useState } from "react";
import { setWorkspaceExplore, type Workspace } from "../api/contextSessions";
import { exploreSettingFor } from "./exploreSetting";

function stateText(workspace: Workspace): string {
  const folder = Boolean(workspace.folder?.path);
  switch (workspace.explore?.setting ?? "default") {
    case "off": return "Turned off, with or without a folder.";
    case "on": return folder ? "Turned on: workers read only inside this Workspace's folder." : "Turned on: each assignment names the absolute path of a folder to read.";
    default: return folder ? "On by default: workers read only inside this Workspace's folder." : "Off by default until this Workspace has a folder. Turn it on to let each assignment name the absolute path of a folder to read.";
  }
}

export function WorkspaceExploreSettings({workspace, onChanged}: {workspace: Workspace; onChanged?: () => Promise<void>}) {
  const [confirmed, setConfirmed] = useState(workspace);
  const [saving, setSaving] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => { setConfirmed(workspace); }, [workspace]);
  const save = async (checked: boolean) => {
    setSaving(true); setProblem(""); setMessage("");
    try {
      const saved = await setWorkspaceExplore(confirmed.id, confirmed.currentRevisionId, exploreSettingFor(confirmed, checked));
      setConfirmed(saved);
      setMessage(saved.explore?.allowed ? "Explore allowed. New chats can use it." : checked ? "Saved, but explore is not allowed: attach a folder or refresh." : "Explore workers turned off. Active workers are stopping.");
      try { await onChanged?.(); }
      catch { setProblem("Permission saved, but the Workspace list could not refresh. Refresh before starting a new chat."); }
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Explore permission could not be confirmed. Refresh before trying again.");
    } finally { setSaving(false); }
  };
  return <section aria-label="Explore workers" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Explore workers</h2>
    {(confirmed.defaultPresetId ?? "standard") === "standard" ? <>
      <label className="text-body flex items-center gap-2 text-xs"><input type="checkbox" className="accent-teal" checked={confirmed.explore?.allowed ?? false} disabled={saving || confirmed.state !== "active"} onChange={event => void save(event.target.checked)} />Allow explore workers</label>
      <p className="text-muted-text mt-2 max-w-[620px] text-xs leading-5">{stateText(confirmed)} Workers read files, run read-only commands, and query databases, then return a summary. Turning this on applies to new chats; turning it off stops active explore workers, and chats it stopped stay restricted if you turn it on again.</p>
    </> : <p className="text-muted-text text-xs leading-5">The Research preset cannot delegate. Use a Standard Workspace for explore workers.</p>}
    <p role="status" className="text-muted-text mt-2 text-xs">{saving ? "Saving permission…" : message}</p>
    {problem && <div className="mt-3 text-xs"><p role="alert" className="text-danger">{problem}</p>{onChanged && <button type="button" disabled={saving} onClick={() => void onChanged().then(()=>setProblem(""),()=>setProblem("Workspace permissions could not be refreshed."))} className="text-body mt-2 underline underline-offset-2">Refresh permissions</button>}</div>}
  </section>;
}
