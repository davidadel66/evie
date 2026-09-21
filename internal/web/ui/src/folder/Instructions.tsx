import { useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { readInstructions, type InstructionSnapshot, type InstructionTarget } from "../api/repositoryInstructions";
import { FileCode } from "../artifacts/FileViewer";
import { instructionLabel } from "./instructionLabel";
import { FileIcon } from "../ui/Icon";

export function InstructionBadge({workspace,refresh,onOpen}:{workspace:Workspace;refresh?:string;onOpen:(trigger:HTMLButtonElement)=>void}) {
 const [snapshot,setSnapshot]=useState<InstructionSnapshot>();
 const [failed,setFailed]=useState(false);
 const {id,folder,instructions}=workspace;
 useEffect(()=>{
  const controller=new AbortController();setSnapshot(undefined);setFailed(false);
  void readInstructions({workspaceId:id,folderRevision:folder?.revision??0,settingsRevision:instructions?.revision??0},controller.signal).then(value=>{if(!controller.signal.aborted)setSnapshot(value);}).catch(()=>{if(!controller.signal.aborted)setFailed(true);});
  return ()=>controller.abort();
 },[id,folder?.revision,instructions?.revision,refresh]);
 if(!folder?.path)return null;
 return <button type="button" aria-label="Inspect repository instructions" onClick={event=>onOpen(event.currentTarget)} title="Repository instructions for this Workspace" className="text-muted-text hover:text-teal mt-2 flex max-w-full items-center gap-1.5 rounded text-left text-[11px]"><FileIcon size={12}/><span className="truncate">{failed?"Instructions unavailable":instructionLabel(snapshot)}</span></button>;
}
export function InstructionsView({target}:{target:InstructionTarget}) {
 const [snapshot,setSnapshot]=useState<InstructionSnapshot>();
 const [error,setError]=useState("");
 const [refresh,setRefresh]=useState(0);
 const {workspaceId,folderRevision,settingsRevision,sessionId,turnId}=target;
 useEffect(()=>{
  const controller=new AbortController();setSnapshot(undefined);setError("");
  void readInstructions({workspaceId,folderRevision,settingsRevision,sessionId,turnId},controller.signal).then(value=>{if(!controller.signal.aborted)setSnapshot(value);}).catch(error=>{if(!controller.signal.aborted)setError(error instanceof Error?error.message:"Instructions are unavailable.");});
  return ()=>controller.abort();
 },[workspaceId,folderRevision,settingsRevision,sessionId,turnId,refresh]);
 return <section aria-label="Repository instruction details" className="flex min-h-0 flex-1 flex-col">
  <div className="border-editor-hair space-y-2 border-b px-4 py-3 text-xs">
   <div className="flex items-center gap-3"><span className="text-editor-text flex-1 font-medium">{turnId?"Instructions for this turn":"Current repository instructions"}</span><button type="button" className="text-teal" onClick={()=>setRefresh(value=>value+1)}>Refresh</button></div>
   <p className="text-editor-muted">{turnId?(!snapshot?"Recorded instructions are shown when a snapshot is available.":snapshot.prepared?"Recorded with the prepared model request. This does not establish that the model followed the instructions.":"Captured for this turn; inclusion in a model request has not been recorded."):"Preview from disk. New turns capture their own snapshot."}</p>
   {snapshot&&<><p className="text-editor-text break-all">{snapshot.folder.path}{snapshot.file?` / ${snapshot.file}`:""}</p><p className="text-editor-muted">{instructionLabel(snapshot)}{turnId?` · ${new Date(snapshot.capturedAt).toLocaleString()}`:""}</p>{snapshot.sha256&&<details className="text-editor-muted"><summary className="cursor-pointer">Version</summary><p className="mt-2 break-all font-mono">SHA-256 {snapshot.sha256}</p></details>}</>}
  </div>
  {error?<p role="alert" className="text-danger p-4 text-sm">{error}</p>:!snapshot?<p className="text-editor-muted p-4 text-sm">Loading instructions…</p>:snapshot.status==="loaded"?<div tabIndex={0} aria-label="Instruction source" className="min-h-0 flex-1 overflow-auto"><FileCode text={snapshot.text??""} path={snapshot.file??"AGENTS.md"}/></div>:<p className="text-editor-muted p-4 text-sm leading-6">{snapshot.detail??(snapshot.status==="missing"?"No root AGENTS.md or CLAUDE.md was found. Add one to the attached folder to supply repository guidance.":snapshot.status==="disabled"?"Repository instructions were disabled. Enable them in Workspace folder settings for future turns.":"Attach a local folder in Workspace settings to use repository instructions.")}</p>}
 </section>;
}
