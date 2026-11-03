import { afterEach, describe, expect, it, vi } from "vitest";
import { listSkills } from "./skills";
import { sendDetached } from "./stream";

afterEach(() => vi.unstubAllGlobals());

// skills.spec.md, Owner invocation: the skill travels apart from the text.
describe("chat skill API", () => {
  it("lists the open chat's skills", async () => {
    const skills = [{name:"grilling",description:"Grill a plan.",ownerOnly:false},{name:"owl",description:"Doors.",argumentHint:"[shift]",ownerOnly:true}];
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({available:true,skills})));
    vi.stubGlobal("fetch",fetch);
    expect(await listSkills("session-1")).toEqual({available:true,skills});
    expect(fetch.mock.calls[0][0]).toBe("/api/skills/list");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({sessionId:"session-1"});
  });
  it("says when the chat can't use skills", async () => {
    vi.stubGlobal("fetch",vi.fn().mockResolvedValue(new Response(JSON.stringify({available:false,skills:[]}))));
    expect(await listSkills("session-1")).toEqual({available:false,skills:[]});
  });
  it("surfaces a failed list", async () => {
    vi.stubGlobal("fetch",vi.fn().mockResolvedValue(new Response(JSON.stringify({error:"Chat changed; refresh and try again"}),{status:409})));
    await expect(listSkills("session-1")).rejects.toThrow("Chat changed; refresh and try again");
  });
  it("sends the skill apart from the message text", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({status:"started"}),{status:202}));
    vi.stubGlobal("fetch",fetch);
    await sendDetached("my plan","session-1","openai/test","grilling");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({message:"my plan",sessionId:"session-1",model:"openai/test",detach:true,skill:"grilling"});
    await sendDetached("hello","session-1","openai/test");
    expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({message:"hello",sessionId:"session-1",model:"openai/test",detach:true});
  });
  it("surfaces the server's refusal of a skill", async () => {
    vi.stubGlobal("fetch",vi.fn().mockResolvedValue(new Response(JSON.stringify({error:"This chat has no skill /wrap."}),{status:422})));
    await expect(sendDetached("x","session-1","openai/test","wrap")).rejects.toThrow("This chat has no skill /wrap.");
  });
});
