import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { closePaneTab, inspectionKey, openPaneTab, refreshInspection, type PaneTab } from "./paneTabs";
import type { ToolItem } from "../chat/activityModel";
import { InspectorPane } from "./InspectorPane";

describe("shared inspector tabs", () => {
 it("refreshes current instruction settings while preserving historical instructions and open work",()=>{
  const current={kind:"repository-instructions",workspaceId:"workspace",folderRevision:1,settingsRevision:0} as const;
  const historical={...current,sessionId:"session",turnId:"turn"};
  const workspace={id:"workspace",displayName:"Demo",state:"active",currentRevisionId:"revision",createdAt:"",updatedAt:"",folder:{path:"/project",revision:1},instructions:{enabled:false,revision:1}} as const;
  const tabs:PaneTab[]=[{id:"files",kind:"files"},{id:"terminal",kind:"terminal"},{id:inspectionKey(current),kind:"inspection",target:current}];
  expect(refreshInspection(current,"session",[],workspace)).toMatchObject({settingsRevision:1,folderRevision:1});
  expect(refreshInspection(historical,"session",[],workspace)).toEqual(historical);
  expect(tabs.map(tab=>tab.kind)).toEqual(["files","terminal","inspection"]);
 });
 it("keeps historical instructions, current instructions and a live terminal distinct",()=>{
  const current={kind:"repository-instructions",workspaceId:"workspace",folderRevision:1,settingsRevision:0} as const;
  const historical={...current,sessionId:"session",turnId:"turn"};
  let tabs:PaneTab[]=[{id:"terminal",kind:"terminal"}];
  for(const target of [current,historical])tabs=openPaneTab(tabs,{id:inspectionKey(target),kind:"inspection",target});
  expect(tabs).toHaveLength(3);expect(tabs[0]).toEqual({id:"terminal",kind:"terminal"});expect(tabs[1].id).not.toBe(tabs[2].id);
 });
 it("refreshes an older tool tab after another tool was selected",()=>{
  const first:ToolItem={kind:"tool",key:"first",id:"a",name:"bash",args:"{}",startedAt:0};
  const second:ToolItem={...first,key:"second",id:"b"};
  const target={kind:"tool",tool:first} as const;
  const tabs=openPaneTab(openPaneTab([],{id:inspectionKey(target),kind:"inspection",target}),{id:"tool:second",kind:"inspection",target:{kind:"tool",tool:second}});
  const saved=tabs[0];
  if(saved.kind!=="inspection")throw new Error("expected inspection tab");
  expect(refreshInspection(saved.target,"one",[{...first,result:"completed output"},second])).toMatchObject({kind:"tool",tool:{key:"first",result:"completed output"}});
 });
 it("keeps an open disk file when inspecting memory and returns to it when memory closes", () => {
  const file:PaneTab={id:"disk:notes.md",kind:"disk",path:"notes.md"};
  const target={kind:"memory-evidence",sessionId:"one",snapshotId:"receipt"} as const;
  let tabs=openPaneTab([],file);
  const key=inspectionKey(target);
  tabs=openPaneTab(tabs,{id:key,kind:"inspection",target});
  expect(tabs.map(tab=>tab.kind)).toEqual(["disk","inspection"]);
  tabs=openPaneTab(tabs,{id:key,kind:"inspection",target});
  expect(tabs).toHaveLength(2);
  expect(closePaneTab(tabs,key,key)).toEqual({tabs:[file],active:file.id});
 });
 it("keeps a recorded file distinct from the current disk file",()=>{
  const target={kind:"file",file:{key:"read-1",path:"notes.md",status:"Read",content:{kind:"code",coverage:"full",text:"old"}}} as const;
  const tabs=openPaneTab([{id:"disk:notes.md",kind:"disk",path:"notes.md"}],{id:inspectionKey(target),kind:"inspection",target});
  expect(tabs).toHaveLength(2);expect(tabs[0].id).not.toBe(tabs[1].id);
 });
 it("exposes the shared pane and folder setup without a second inspector",()=>{
  const html=renderToStaticMarkup(createElement(InspectorPane,{visible:true,focused:false,onClose:()=>{},onManageFolder:()=>{}}));
  expect(html.match(/aria-label="Inspector"/g)).toHaveLength(1);
  expect(html).toContain("Open files");expect(html).toContain("Open changes");expect(html).toContain("Open Workspaces");
 });
});
