import { afterEach, describe, expect, it, vi } from "vitest";
import { listModels, selectModel } from "./models";
import { streamChat } from "./stream";

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
