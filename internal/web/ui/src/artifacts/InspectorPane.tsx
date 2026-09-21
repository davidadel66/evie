import { useEffect, useRef, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import type { Item } from "../store/reducer";
import { folderRef } from "../api/localFolder";
import { TerminalView } from "../folder/Terminal";
import { ChangesView } from "../folder/Changes";
import { DiskFileView, FileTree } from "../folder/Files";
import { Cross, Database, FileIcon, Folder, Layers, TerminalIcon } from "../ui/Icon";
import { InspectorContent, type InspectorTarget } from "./Panel";
import { targetTitle } from "./inspectorTitle";
import { closePaneTab, inspectionKey, openPaneTab, refreshInspection, type PaneTab } from "./paneTabs";

export type InspectorPaneProps = {target?:InspectorTarget;items?:Item[];requestVersion?:number;filesRequest?:number;workspace?:Workspace;sessionId?:string;visible:boolean;focused:boolean;onClose:()=>void;onManageFolder:()=>void};
export function InspectorPane(props:InspectorPaneProps) {
 const folderIdentity=`${props.workspace?.id ?? "none"}:${props.workspace?.folder?.revision ?? 0}`;
 return <Pane key={`${folderIdentity}:${props.sessionId ?? "none"}`} {...props}/>;
}
function Pane({target,items=[],sessionId,requestVersion=0,filesRequest=0,workspace,visible,focused,onClose,onManageFolder}:InspectorPaneProps) {
 const hasFolder=!!workspace?.folder?.path;
 const [tabs,setTabs]=useState<PaneTab[]>(hasFolder?[{id:"files",kind:"files"}]:[]);
 const [active,setActive]=useState(hasFolder?"files":"");
 const [treeVisible,setTreeVisible]=useState(()=>{try{return localStorage.getItem("evie.fileTree")!=="false";}catch{return true;}});
 const [width,setWidth]=useState(()=>{try{return Math.max(380,Math.min(1100,Number(localStorage.getItem("evie.paneWidth"))||680));}catch{return 680;}});
 const closeRef=useRef<HTMLButtonElement>(null);
 const lastRequest=useRef("");
 useEffect(()=>{if(visible)closeRef.current?.focus();},[visible]);
 const requestKey=target?inspectionKey(target):"";
 const open=(tab:PaneTab)=>{setTabs(current=>openPaneTab(current,tab));setActive(tab.id);};
 useEffect(()=>{
  if(!target||!requestKey||lastRequest.current===`${requestKey}:${requestVersion}`)return;
  lastRequest.current=`${requestKey}:${requestVersion}`;setTabs(current=>openPaneTab(current,{id:requestKey,kind:"inspection",target}));setActive(requestKey);
  if(visible)closeRef.current?.focus();
 },[requestKey,requestVersion,target,visible]);
 useEffect(()=>{if(filesRequest>0){setTabs(current=>openPaneTab(current,{id:"files",kind:"files"}));setActive("files");}},[filesRequest]);
 useEffect(()=>{try{localStorage.setItem("evie.fileTree",String(treeVisible));localStorage.setItem("evie.paneWidth",String(width));}catch{/* preferences stay in memory */}},[treeVisible,width]);
 const selected=tabs.find(tab=>tab.id===active);
 const root=workspace?folderRef(workspace):undefined;
 const toggleTree=()=>setTreeVisible(value=>!value);
 const openFile=(path:string)=>open({id:`disk:${path}`,kind:"disk",path});
 const close=(id:string)=>{const next=closePaneTab(tabs,active,id);setTabs(next.tabs);setActive(next.active);};
 const resize=(event:React.PointerEvent)=>{
  const start=event.clientX,initial=width;event.currentTarget.setPointerCapture(event.pointerId);
  const move=(next:PointerEvent)=>setWidth(Math.max(380,Math.min(1100,initial+start-next.clientX)));
  const end=()=>{window.removeEventListener("pointermove",move);window.removeEventListener("pointerup",end);window.removeEventListener("pointercancel",end);};
  window.addEventListener("pointermove",move);window.addEventListener("pointerup",end);window.addEventListener("pointercancel",end);
 };
 return <aside aria-label="Inspector" hidden={!visible} style={{"--pane-width":`${width}px`} as React.CSSProperties} className={`${!visible?"hidden":"flex"} ${focused?"min-w-0 flex-1":"flex-none w-[var(--pane-width)] max-w-[calc(100%_-_360px)] @max-[760px]/workbench:absolute @max-[760px]/workbench:inset-0 @max-[760px]/workbench:z-30 @max-[760px]/workbench:w-full @max-[760px]/workbench:max-w-full"} bg-editor border-editor-hair relative min-h-0 flex-col border-l`} onKeyDown={event=>{
  if(event.target instanceof Element && event.target.closest("[data-terminal]"))return;
  if(event.key==="Escape"){event.preventDefault();event.stopPropagation();onClose();}
  if(event.key!=="Tab"||(event.currentTarget.parentElement?.clientWidth ?? window.innerWidth)>760)return;
  const controls=[...event.currentTarget.querySelectorAll<HTMLElement>('button,input,select,textarea,summary,[tabindex="0"],a[href]')].filter(node=>{
   if(node.tabIndex<0||node.matches(":disabled")||node.getClientRects().length===0)return false;
   for(let parent=node.parentElement;parent&&parent!==event.currentTarget;parent=parent.parentElement){
    if(parent.tagName==="DETAILS"&&!parent.hasAttribute("open")){
     const summary=parent.querySelector(":scope > summary");
     if(!summary?.contains(node))return false;
    }
   }
   return true;
  });
  const destination=event.shiftKey&&document.activeElement===controls[0]?controls.at(-1):!event.shiftKey&&document.activeElement===controls.at(-1)?controls[0]:undefined;
  if(destination){event.preventDefault();destination.focus();}
 }}>
  {!focused && <div role="separator" aria-label="Resize inspector" aria-orientation="vertical" aria-valuenow={width} aria-valuemin={380} aria-valuemax={1100} tabIndex={0} onPointerDown={resize} onKeyDown={event=>{if(event.key==="ArrowLeft"||event.key==="ArrowRight"){event.preventDefault();setWidth(value=>Math.max(380,Math.min(1100,value+(event.key==="ArrowLeft"?30:-30))));}}} className="hover:bg-teal/40 focus-visible:bg-teal absolute inset-y-0 -left-1 z-10 w-1.5 cursor-col-resize touch-none @max-[760px]/workbench:hidden"/>}
  <header className="border-editor-hair bg-topbar flex h-11 flex-none items-center gap-1 border-b px-2">
   <button type="button" title="Files" aria-label="Open files" onClick={()=>open({id:"files",kind:"files"})} className={`rounded p-2 ${selected?.kind==="files"||selected?.kind==="disk"?"text-teal":"text-muted-text hover:text-body"}`}><Folder size={16}/></button>
   <button type="button" title="Changes" aria-label="Open changes" disabled={!hasFolder} onClick={()=>open({id:"changes",kind:"changes"})} className="text-muted-text hover:text-body rounded p-2 disabled:opacity-30"><Layers size={16}/></button>
   <button type="button" title="Terminal" aria-label="Open terminal" disabled={!hasFolder} onClick={()=>open({id:"terminal",kind:"terminal"})} className="text-muted-text hover:text-body rounded p-2 disabled:opacity-30"><TerminalIcon size={16}/></button>
   <span className="text-editor-muted ml-1 min-w-0 flex-1 truncate text-xs" title={workspace?.folder?.path}>{workspace?.folder?.path?.split("/").pop() ?? "Details"}</span>
   <button ref={closeRef} type="button" aria-label="Close inspector" onClick={onClose} className="text-editor-muted hover:text-body rounded p-2"><Cross size={14}/></button>
  </header>
  {tabs.length>0 && <div role="tablist" aria-label="Inspector tabs" className="border-editor-hair flex h-10 flex-none overflow-x-auto border-b">
   {tabs.map(tab=><div key={tab.id} className={`border-editor-hair flex min-w-0 flex-none items-center border-r ${tab.id===active?"bg-selected text-ink":"text-editor-muted"}`}>
    <button id={`pane-tab-${encodeURIComponent(tab.id)}`} type="button" role="tab" aria-selected={tab.id===active} aria-controls="pane-content" tabIndex={tab.id===active?0:-1} onClick={()=>setActive(tab.id)} onKeyDown={event=>{
     const index=tabs.indexOf(tab);let next:number|undefined;
     if(event.key==="ArrowRight")next=(index+1)%tabs.length;if(event.key==="ArrowLeft")next=(index+tabs.length-1)%tabs.length;if(event.key==="Home")next=0;if(event.key==="End")next=tabs.length-1;
     if(next!==undefined){event.preventDefault();setActive(tabs[next].id);document.getElementById(`pane-tab-${encodeURIComponent(tabs[next].id)}`)?.focus();}
    }} className="flex max-w-[220px] items-center gap-2 px-3 py-2 text-xs outline-offset-[-2px]">
     {tab.kind==="inspection"?<Database size={12}/>:tab.kind==="disk"?<FileIcon size={12}/>:tab.kind==="terminal"?<TerminalIcon size={12}/>:<Folder size={12}/>}<span className="truncate">{tabLabel(tab)}</span>
    </button><button type="button" aria-label={`Close ${tabLabel(tab)}`} onClick={()=>close(tab.id)} className="hover:text-ink mr-1 rounded p-1"><Cross size={11}/></button>
   </div>)}
  </div>}
  <div id="pane-content" role="tabpanel" aria-labelledby={selected?`pane-tab-${encodeURIComponent(selected.id)}`:undefined} className="flex min-h-0 min-w-0 flex-1">
   {root&&tabs.some(tab=>tab.kind==="terminal")&&<div hidden={selected?.kind!=="terminal"} className={selected?.kind==="terminal"?"flex min-h-0 min-w-0 flex-1":"hidden"}><TerminalView root={root} active={visible&&selected?.kind==="terminal"}/></div>}
   {selected?.kind==="terminal"?null:selected?.kind==="inspection"?<div className="flex min-h-0 min-w-0 flex-1 flex-col"><div className="border-editor-hair text-muted-text border-b px-4 py-2 text-xs">{targetTitle(selected.target)}{selected.target.kind==="file"?" · Recorded in conversation":""}</div><div className="flex min-h-0 flex-1 flex-col overflow-auto"><InspectorContent key={selected.id} target={refreshInspection(selected.id===requestKey&&target?target:selected.target,sessionId,items,workspace)}/></div></div>
   :!hasFolder||!root?<div className="text-muted-text m-auto max-w-80 p-6 text-center text-sm leading-6"><Folder size={24} className="mx-auto mb-4"/><p>Attach a local folder to browse files and review changes alongside your conversation.</p><button type="button" onClick={onManageFolder} className="text-teal mt-4 text-xs">{workspace?"Set Workspace folder":"Open Workspaces"}</button></div>
   :selected?.kind==="changes"?<ChangesView root={root}/>
   :selected?.kind==="disk"?<><DiskFileView key={selected.path} root={root} path={selected.path} treeVisible={treeVisible} onToggleTree={toggleTree}/>{treeVisible&&<div className="border-editor-hair w-[220px] min-w-[150px] max-w-[42%] flex-none border-l"><FileTree root={root} onOpen={openFile}/></div>}</>
   :<div className="flex min-h-0 flex-1"><div className="min-w-0 flex-1"><FileTree root={root} onOpen={openFile}/></div></div>}
  </div>
 </aside>;
}
function tabLabel(tab:PaneTab):string {
 switch(tab.kind){case "files":return "Files";case "changes":return "Changes";case "terminal":return "Terminal";case "disk":return tab.path.split("/").pop()??tab.path;case "inspection":return targetTitle(tab.target);}
}
