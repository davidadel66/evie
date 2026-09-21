import { useEffect, useState } from "react";
import { folderRequest, type DiskFile, type FolderEntry, type FolderListing, type FolderRef } from "../api/localFolder";
import { FileCode } from "../artifacts/FileViewer";
import { Markdown } from "../chat/Markdown";
import { ChevronDown, ChevronRight, FileIcon, Folder } from "../ui/Icon";

export function FileTree({ root, onOpen }: {root: FolderRef; onOpen: (path: string) => void}) {
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState<FolderListing>();
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    if (!query.trim()) {setSearch(undefined); setError(""); return;}
    const abort = new AbortController();
    const timer = setTimeout(() => { void folderRequest<FolderListing>("list", {workspaceId:root.workspaceId,revision:root.revision, query}, abort.signal).then(setSearch).catch(error => {if (!abort.signal.aborted) setError(String(error.message));}); }, 180);
    return () => {clearTimeout(timer); abort.abort();};
  }, [root.workspaceId, root.revision, query, refresh]);
  return <div className="flex min-h-0 flex-1 flex-col">
    <div className="border-editor-hair flex gap-1 border-b p-2">
      <input aria-label="Filter files" placeholder="Filter files…" value={query} onChange={e => setQuery(e.target.value)} className="border-editor-hair bg-card focus:border-teal min-w-0 flex-1 rounded-md border px-2 py-1.5 text-xs outline-none" />
      <button type="button" aria-label="Refresh files" title="Refresh files" onClick={() => setRefresh(value => value + 1)} className="text-editor-muted hover:text-ink rounded px-1.5">↻</button>
    </div>
    <nav aria-label="Files" className="min-h-0 flex-1 overflow-auto py-1 text-xs">
      {error && <p role="alert" className="text-danger p-3">{error}</p>}
      {query.trim() ? search ? <>{search.entries.map(entry => <FileRow key={entry.path} entry={entry} root={root} depth={0} onOpen={onOpen} searching />)}{!search.entries.length && <p className="text-faint p-3">No matching files.</p>}{search.truncated && <p className="text-amber-ink p-3">Search is limited. Narrow your filter to find more files.</p>}</> : <p className="text-faint p-3">Searching…</p>
        : <Directory key={refresh} root={root} path="." depth={0} onOpen={onOpen} />}
    </nav>
  </div>;
}
function Directory({root,path,depth,onOpen}: {root: FolderRef; path: string; depth: number; onOpen: (path: string) => void}) {
  const [listing, setListing] = useState<FolderListing>(); const [error,setError] = useState("");
  useEffect(() => {const abort = new AbortController(); void folderRequest<FolderListing>("list", {workspaceId:root.workspaceId,revision:root.revision,path},abort.signal).then(setListing).catch(error => {if(!abort.signal.aborted)setError(error.message);}); return () => abort.abort();},[root.workspaceId,root.revision,path]);
  if(error)return <p role="alert" className="text-danger p-3 text-xs">{error}</p>;
  if(!listing)return <p className="text-faint px-3 py-2">Loading…</p>;
  return <>{listing.entries.map(entry => <FileRow key={entry.path} root={root} entry={entry} depth={depth} onOpen={onOpen} />)}{!listing.entries.length && <p className="text-faint px-3 py-2">Empty folder</p>}{listing.truncated && <p className="text-amber-ink px-3 py-2">Large folder. Use the filter to narrow the list.</p>}</>;
}
function FileRow({root,entry,depth,onOpen,searching=false}: {root: FolderRef; entry: FolderEntry; depth: number; onOpen: (path: string)=>void; searching?: boolean}) {
  const [expanded,setExpanded]=useState(false);
  return <div><button type="button" aria-expanded={entry.directory?expanded:undefined} title={entry.path} onClick={() => entry.directory ? setExpanded(value=>!value) : onOpen(entry.path)} style={{paddingLeft:10+depth*14}} className="text-body hover:bg-selected focus-visible:ring-teal flex w-full items-center gap-1.5 py-1.5 pr-2 text-left focus-visible:ring-1 focus-visible:outline-none">
    {entry.directory ? expanded ? <ChevronDown size={12}/> : <ChevronRight size={12}/> : <span className="w-3 flex-none"/>}
    {entry.directory?<Folder size={13}/>:<FileIcon size={13}/>}<span className="truncate">{searching?entry.path:entry.name}</span>
  </button>{expanded && <Directory root={root} path={entry.path} depth={depth+1} onOpen={onOpen}/>}</div>;
}
export function DiskFileView({root,path,treeVisible,onToggleTree}: {root: FolderRef;path:string;treeVisible:boolean;onToggleTree:()=>void}) {
  const [file,setFile]=useState<DiskFile>();const [error,setError]=useState("");const [preview,setPreview]=useState(true);const [refresh,setRefresh]=useState(0);
  useEffect(()=>{const abort=new AbortController();setError("");void folderRequest<DiskFile>("read",{workspaceId:root.workspaceId,revision:root.revision,path},abort.signal).then(setFile).catch(error=>{if(!abort.signal.aborted)setError(error.message);});return()=>abort.abort();},[root.workspaceId,root.revision,path,refresh]);
  const markdown=/\.(md|mdx|markdown)$/i.test(path);
  return <div className="flex min-h-0 flex-1 flex-col">
    <div className="border-editor-hair flex min-h-11 flex-none items-center gap-3 border-b px-3 text-xs">
      <span className="text-editor-muted min-w-0 flex-1 truncate" title={path}>{path.split("/").join(" › ")}</span>
      {markdown && <button type="button" onClick={()=>setPreview(value=>!value)} className="text-body whitespace-nowrap">{preview?"View source":"Preview"}</button>}
      <button type="button" aria-label="Reload file" title="Reload file from disk" onClick={()=>setRefresh(value=>value+1)} className="text-editor-muted p-1">↻</button>
      <button type="button" aria-label="Toggle file tree" aria-pressed={treeVisible} onClick={onToggleTree} className={treeVisible?"bg-selected text-teal rounded p-1.5":"text-editor-muted p-1.5"}><Folder size={17}/></button>
    </div>
    <div tabIndex={0} role="region" aria-label="Current file contents" className="min-h-0 flex-1 overflow-auto">
      {error?<p role="alert" className="text-danger p-5">{error}</p>:!file?<p className="text-faint p-5">Opening file…</p>:markdown&&preview?<article className="mx-auto max-w-[850px] px-6 py-6"><Markdown text={file.text} streaming={false}/></article>:<FileCode text={file.text} path={path}/>}
    </div>
    <div className="border-editor-hair text-editor-muted border-t px-3 py-1 text-[10px]">Current file on disk{file?` · ${file.size.toLocaleString()} bytes`:""}</div>
  </div>;
}
