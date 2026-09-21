import { useEffect, useState } from "react";
import { folderRequest, type FolderRef, type GitChanges } from "../api/localFolder";
import { ChevronDown, ChevronRight } from "../ui/Icon";

export function ChangesView({root}: {root: FolderRef}) {
  const [view,setView]=useState("working");const [base,setBase]=useState("HEAD");const [draft,setDraft]=useState("HEAD");const [refresh,setRefresh]=useState(0);
  const [changes,setChanges]=useState<GitChanges>();const [error,setError]=useState("");
  useEffect(()=>{const abort=new AbortController();setChanges(undefined);setError("");void folderRequest<GitChanges>("changes",{workspaceId:root.workspaceId,revision:root.revision,view,base},abort.signal).then(setChanges).catch(error=>{if(!abort.signal.aborted)setError(error.message);});return()=>abort.abort();},[root.workspaceId,root.revision,view,base,refresh]);
  return <div className="flex min-h-0 flex-1 flex-col">
    <div className="border-editor-hair flex flex-wrap items-center gap-3 border-b px-4 py-3">
      <select aria-label="Review comparison" value={view} onChange={event=>setView(event.target.value)} className="bg-editor text-ink rounded py-1 text-sm outline-none"><option value="working">Working tree</option><option value="staged">Staged</option><option value="branch">Branch</option></select>
      <span className="text-faint min-w-0 flex-1 truncate text-xs">{changes?.branch}</span>
      <button type="button" onClick={()=>setRefresh(value=>value+1)} aria-label="Refresh changes" className="text-muted-text p-1">↻</button>
      {view==="branch" && <form className="flex w-full items-center gap-2 text-xs" onSubmit={event=>{event.preventDefault();setBase(draft);}}><span className="text-faint">Compare with</span><input aria-label="Base revision" value={draft} onChange={event=>setDraft(event.target.value)} className="border-editor-hair min-w-0 flex-1 rounded border px-2 py-1"/><button className="text-teal px-2 py-1">Compare</button></form>}
    </div>
    <div className="min-h-0 flex-1 overflow-auto">
      {error?<p role="alert" className="text-danger p-5">{error}</p>:!changes?<p className="text-faint p-5">Reading changes…</p>:!changes.available?<p className="text-muted-text p-5">This folder is not in a Git repository. You can still browse its files.</p>:!changes.files.length?<p className="text-muted-text p-5">No changes in this comparison.</p>:<>{changes.files.map(file=><ChangeFile key={`${view}:${base}:${refresh}:${file.path}`} root={root} path={file.path} status={file.status} view={view} base={base}/>)}</>}
    </div>
  </div>;
}
function ChangeFile({root,path,status,view,base}: {root:FolderRef;path:string;status:string;view:string;base:string}) {
 const [open,setOpen]=useState(false);const [patch,setPatch]=useState<string>();const [error,setError]=useState("");const [viewed,setViewed]=useState(false);
 useEffect(()=>{if(!open||patch!==undefined)return;const abort=new AbortController();void folderRequest<{text:string}>("diff",{workspaceId:root.workspaceId,revision:root.revision,path,view,base},abort.signal).then(result=>setPatch(result.text)).catch(error=>{if(!abort.signal.aborted)setError(error.message);});return()=>abort.abort();},[open,root.workspaceId,root.revision,path,view,base,patch]);
 return <section className="border-editor-hair border-b">
  <div className="bg-card flex items-center gap-2 px-3 py-3">
   <button type="button" aria-expanded={open} onClick={()=>setOpen(value=>!value)} className="flex min-w-0 flex-1 items-center gap-2 text-left">{open?<ChevronDown/>:<ChevronRight/>}<span className="text-teal w-5 flex-none font-mono text-xs">{status}</span><span className={`truncate text-xs ${viewed?"text-faint":"text-body"}`} title={path}>{path}</span></button>
   <label className="text-faint flex cursor-pointer items-center gap-1.5 text-[10px]"><input type="checkbox" checked={viewed} onChange={event=>setViewed(event.target.checked)}/>Viewed</label>
  </div>
  {open && <div className="overflow-x-auto font-mono text-xs leading-6">{error?<p role="alert" className="text-danger p-4 font-sans">{error}</p>:patch===undefined?<p className="text-faint p-4 font-sans">Loading diff…</p>:patch?<pre className="m-0 min-w-max">{patch.split("\n").map((line,i)=><div key={i} className={`px-4 ${line.startsWith("+")&&!line.startsWith("+++")?"bg-ok-bg text-ok-ink":line.startsWith("-")&&!line.startsWith("---")?"bg-danger-bg text-danger-ink":line.startsWith("@@")?"bg-selected text-teal":"text-editor-muted"}`}>{line||" "}</div>)}</pre>:<p className="text-muted-text p-4 font-sans">No text changes to display.</p>}</div>}
 </section>;
}
