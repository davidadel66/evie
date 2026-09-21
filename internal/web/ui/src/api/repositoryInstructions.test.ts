import { afterEach, describe, expect, it, vi } from "vitest";
import { readInstructions } from "./repositoryInstructions";
import { instructionLabel } from "../folder/instructionLabel";
import type { InstructionSnapshot } from "./repositoryInstructions";

describe("repository instruction inspection",()=>{
 afterEach(()=>vi.unstubAllGlobals());
 it("requests recorded turn identity separately from current disk",async()=>{
  const fetchMock=vi.fn(async()=>new Response('{}'));vi.stubGlobal("fetch",fetchMock);
  const target={workspaceId:"workspace",folderRevision:2,settingsRevision:3};
  await readInstructions(target);await readInstructions({...target,sessionId:"session",turnId:"turn"});
  expect(fetchMock).toHaveBeenNthCalledWith(1,"/api/repository-instructions/preview",expect.objectContaining({body:JSON.stringify({workspaceId:"workspace",folderRevision:2})}));
  expect(fetchMock).toHaveBeenNthCalledWith(2,"/api/repository-instructions/snapshot",expect.objectContaining({body:JSON.stringify({workspaceId:"workspace",folderRevision:2,sessionId:"session",turnId:"turn"})}));
 });
 it("surfaces unavailable snapshots rather than substituting current content",async()=>{
  vi.stubGlobal("fetch",vi.fn(async()=>new Response('{"error":"No recorded snapshot"}',{status:404})));
  await expect(readInstructions({workspaceId:"w",folderRevision:1,settingsRevision:0,sessionId:"s",turnId:"t"})).rejects.toThrow("No recorded snapshot");
 });
 it("distinguishes disabled, missing and unreadable guidance",()=>{
  const base:InstructionSnapshot={workspaceId:"w",folder:{path:"/project",revision:1},settings:{enabled:true,revision:0},status:"loaded",file:"AGENTS.md",capturedAt:"today"};
  expect(instructionLabel(base)).toBe("AGENTS.md");expect(instructionLabel({...base,status:"disabled"})).toBe("Instructions off");expect(instructionLabel({...base,status:"missing"})).toBe("No instructions");expect(instructionLabel({...base,status:"error"})).toBe("Instructions unavailable");
 });
});
