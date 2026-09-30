import { useEffect, useState } from "react";
import { setWorkspaceResearch, type Workspace } from "../api/contextSessions";

export function WorkspaceResearchSettings({workspace, onChanged}: {workspace: Workspace; onChanged?: () => Promise<void>}) {
  const [confirmed, setConfirmed] = useState(workspace);
  const [saving, setSaving] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => { setConfirmed(workspace); }, [workspace]);
  const save = async (enabled: boolean) => {
    setSaving(true); setProblem(""); setMessage("");
    try {
      const saved = await setWorkspaceResearch(confirmed.id, confirmed.currentRevisionId, enabled);
      setConfirmed(saved);
      setMessage(enabled ? "Research allowed. Start a new chat to use it." : "Research delegation disabled. Active workers are stopping.");
      try { await onChanged?.(); }
      catch { setProblem("Permission saved, but the Workspace list could not refresh. Refresh before starting a new chat."); }
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Research permission could not be confirmed. Refresh before trying again.");
    } finally { setSaving(false); }
  };
  return <section aria-label="Research delegation" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Research delegation</h2>
    {(confirmed.defaultPresetId ?? "standard") === "standard" ? <>
      <label className="text-body flex items-center gap-2 text-xs"><input type="checkbox" className="accent-teal" checked={confirmed.allowedPresetIds?.includes("research") ?? false} disabled={saving || confirmed.state !== "active"} onChange={event => void save(event.target.checked)} />Allow research delegation</label>
      <p className="text-muted-text mt-2 max-w-[620px] text-xs leading-5">Eligible new chats can launch workers with web search and page reading. Workers receive only their assignment and selected context. Enabling applies to new chats; turning this off stops active research. Previously revoked chats stay restricted if you enable it again.</p>
    </> : <p className="text-muted-text text-xs leading-5">The Research preset can read the web directly but cannot delegate. Use a Standard Workspace for research workers.</p>}
    <p role="status" className="text-muted-text mt-2 text-xs">{saving ? "Saving permission…" : message}</p>
    {problem && <div className="mt-3 text-xs"><p role="alert" className="text-danger">{problem}</p>{onChanged && <button type="button" disabled={saving} onClick={() => void onChanged().then(()=>setProblem(""),()=>setProblem("Workspace permissions could not be refreshed."))} className="text-body mt-2 underline underline-offset-2">Refresh permissions</button>}</div>}
  </section>;
}
