import type { InspectorTarget } from "./Panel";
import type { Item } from "../store/reducer";
import type { Workspace } from "../api/contextSessions";
import { inspectToolFile } from "./fileInspection";
import { selectedMemoryTools } from "./toolSelection";

export function refreshInspection(target:InspectorTarget, sessionId:string|undefined, items:Item[], workspace?:Workspace):InspectorTarget {
 if(target.kind==="repository-instructions"&&!target.turnId&&workspace?.id===target.workspaceId)return {...target,folderRevision:workspace.folder?.revision??0,settingsRevision:workspace.instructions?.revision??0};
 if(target.kind==="memory-evidence")return {...target,memoryTools:selectedMemoryTools(target,sessionId,items)};
 const key=target.kind==="file"?target.file.key:target.kind==="tool"?target.tool.key:undefined;
 const tool=key?items.find(item=>item.kind==="tool"&&item.key===key):undefined;
 if(tool?.kind!=="tool")return target;
 if(target.kind==="file"){const file=inspectToolFile(tool);return file?{kind:"file",file}:target;}
 return {kind:"tool",tool};
}
export type PaneTab =
 | { id: "files"; kind: "files" }
 | { id: "changes"; kind: "changes" }
 | { id: "terminal"; kind: "terminal" }
 | { id: string; kind: "disk"; path: string }
 | { id: string; kind: "inspection"; target: InspectorTarget };
export function inspectionKey(target: InspectorTarget): string {
 switch(target.kind) {
 case "repository-instructions": return target.turnId?`instructions:${target.sessionId}:${target.turnId}`:`instructions:${target.workspaceId}`;
 case "file":return `recorded-file:${target.file.key}`;
 case "tool":return `tool:${target.tool.key}`;
 case "memory-evidence":return `memory:${target.sessionId}:${target.snapshotId}`;
 case "memory":return `record:${target.detail.scope.scope_id}:${target.detail.object_kind}:${target.detail.object_id}`;
 case "workspace":return `workspace:${target.workspace.id}`;
 case "scope":return `scope:${target.scope.workspaceId ?? target.scope.projectId ?? "global"}`;
 default:return target.kind;
 }
}
export function openPaneTab(tabs:PaneTab[], tab:PaneTab):PaneTab[] {
 return tabs.some(item=>item.id===tab.id)?tabs.map(item=>item.id===tab.id?tab:item):[...tabs,tab];
}
export function closePaneTab(tabs:PaneTab[],active:string,id:string):{tabs:PaneTab[];active:string} {
 const index=tabs.findIndex(tab=>tab.id===id);const next=tabs.filter(tab=>tab.id!==id);
 return {tabs:next,active:active===id?next[Math.max(0,index-1)]?.id??"":active};
}
