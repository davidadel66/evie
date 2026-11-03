import { afterEach, describe, expect, it, vi } from "vitest";
import { listModels, selectModel } from "./models";
import { listSkills } from "./skills";
import { sendDetached, streamChat } from "./stream";

afterEach(() => vi.unstubAllGlobals());
describe("chat model API", () => {
  it("binds list and selection to the chat and the displayed revision", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({model:"openai/test",revision:2,models:[]})));
    vi.stubGlobal("fetch",fetch);
    await listModels("session-1");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({sessionId:"session-1"});
    fetch.mockResolvedValue(new Response(JSON.stringify({model:"anthropic/test",revision:3})));
    await selectModel("session-1","anthropic/test",2);
    expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({sessionId:"session-1",model:"anthropic/test",revision:2});
  });
  it("surfaces a rejected model change", async () => {
    vi.stubGlobal("fetch",vi.fn().mockResolvedValue(new Response(JSON.stringify({error:"Chat changed"}),{status:409})));
    await expect(selectModel("session-1","anthropic/test",0)).rejects.toThrow("Chat changed");
  });
  it("sends the displayed model to protect against a switch in another browser", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response('event: turn_done\ndata: {}\n\n'));
    vi.stubGlobal("fetch",fetch);
    await streamChat("hello",()=>{},undefined,"session-1","openai/test");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({message:"hello",sessionId:"session-1",model:"openai/test"});
  });
});

// skills.spec.md, Owner invocation: the skill travels apart from the text.
describe("chat skill API", () => {
  it("lists the open chat's skills", async () => {
    const skills = [{name:"grilling",description:"Grill a plan.",ownerOnly:false},{name:"owl",description:"Doors.",argumentHint:"[shift]",ownerOnly:true}];
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({skills})));
    vi.stubGlobal("fetch",fetch);
    expect(await listSkills("session-1")).toEqual(skills);
    expect(fetch.mock.calls[0][0]).toBe("/api/skills/list");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({sessionId:"session-1"});
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
