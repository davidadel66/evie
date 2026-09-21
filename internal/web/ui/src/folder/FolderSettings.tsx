import { useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { folderRef, folderRequest } from "../api/localFolder";
import { Folder } from "../ui/Icon";

export function FolderSettings({ workspace, busy, onChanged, onOpen }: {workspace: Workspace; busy: boolean; onChanged?: () => Promise<void>; onOpen?: () => void}) {
  const [path, setPath] = useState(workspace.folder?.path ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const save = async (value: string) => {
    setSaving(true); setError("");
    try { await folderRequest("/api/workspaces/folder", {...folderRef(workspace), path: value}); await onChanged?.(); setPath(value); }
    catch (error) { setError(error instanceof Error ? error.message : "Folder could not be saved."); }
    finally { setSaving(false); }
  };
  const toggleInstructions = async (enabled:boolean) => {
    setSaving(true);setError("");
    try {await folderRequest("/api/repository-instructions/settings",{workspaceId:workspace.id,revision:workspace.instructions?.revision??0,enabled});await onChanged?.();}
    catch(error){setError(error instanceof Error?error.message:"Instructions could not be saved.");}
    finally{setSaving(false);}
  };
  return <section aria-label="Local folder" className="border-hair mt-7 border-b pb-7">
    <div className="mb-3 flex items-center gap-2 text-sm font-medium"><Folder size={15} /> Local folder</div>
    <p className="text-muted-text mb-3 text-xs leading-5">Connect a folder on this computer for files, changes, and commands. Conversations and memory stay in this Workspace.</p>
    <form onSubmit={event => {event.preventDefault(); void save(path.trim());}} className="flex flex-wrap gap-2">
      <input aria-label="Local folder path" placeholder="/Users/you/code/project" value={path} onChange={event => setPath(event.target.value)} className="border-hair-input bg-card text-ink focus:border-teal min-w-0 flex-[1_1_260px] rounded-md border px-3 py-2 font-mono text-xs outline-none" />
      <button type="submit" disabled={busy || saving || !path.trim() || workspace.state !== "active"} className="border-teal-hair text-teal-hover rounded-md border px-3 py-2 text-xs disabled:opacity-40">{saving ? "Saving…" : workspace.folder?.path ? "Save folder" : "Attach folder"}</button>
      {workspace.folder?.path && <><button type="button" onClick={onOpen} className="border-hair-strong rounded-md border px-3 py-2 text-xs">Open files</button><button type="button" disabled={busy || saving} onClick={() => void save("")} className="text-muted-text px-2 text-xs disabled:opacity-40">Detach</button></>}
    </form>
    {workspace.folder?.path&&<div className="mt-4"><label className="text-body flex items-center gap-2 text-xs"><input type="checkbox" checked={workspace.instructions?.enabled??true} disabled={busy||saving||workspace.state!=="active"} onChange={event=>void toggleInstructions(event.target.checked)} className="accent-teal"/>Use repository instructions</label><p className="text-muted-text mt-1 pl-5 text-xs leading-5">Load root AGENTS.md, or CLAUDE.md when absent, before each turn. Your requests take precedence.</p></div>}
    {error && <p role="alert" className="text-danger mt-3 text-xs">{error}</p>}
  </section>;
}
