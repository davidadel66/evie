import { afterEach, describe, expect, it, vi } from "vitest";
import { readTerminalStream, terminalControls, type TerminalEvent } from "./terminal";

const root = { workspaceId: "workspace", revision: 3 };
function stream(chunks: string[]) {
  return new Response(new ReadableStream({ start(controller) { for (const chunk of chunks) controller.enqueue(new TextEncoder().encode(chunk)); controller.close(); } }));
}
describe("terminal transport", () => {
  afterEach(() => vi.unstubAllGlobals());
  it("handles split frames and waits for each output to render", async () => {
    const events: TerminalEvent[] = [];
    let release = () => {};
    const blocked = new Promise<void>(resolve => { release = resolve; });
    const reading = readTerminalStream(stream(['{"type":"rea', 'dy","id":"one"}\n{"type":"output","data":"w6k="}\n{"type":"exit","code":7}\n']), new AbortController().signal, async event => { events.push(event); if (event.type === "output") await blocked; });
    await vi.waitFor(() => expect(events).toHaveLength(2));
    release(); await reading;
    expect(events.at(-1)).toEqual({ type: "exit", code: 7 });
  });
  it("distinguishes a lost connection from shell exit", async () => {
    await expect(readTerminalStream(stream(['{"type":"ready","id":"one"}\n']), new AbortController().signal, async () => {})).rejects.toThrow("disconnected");
    await expect(readTerminalStream(stream(['{"type":"ready","id":"one"}\n{"type":"exit"}\n']), new AbortController().signal, async () => {})).resolves.toBeUndefined();
  });
  it("stops delivering buffered output after cancellation", async () => {
    const controller = new AbortController(), seen: string[] = [];
    await readTerminalStream(stream(['{"type":"ready","id":"one"}\n{"type":"output","data":"QQ=="}\n']), controller.signal, async event => { seen.push(event.type); controller.abort(); });
    expect(seen).toEqual(["ready"]);
  });
  it("serializes input and resize, bounds escaped input, and preserves Unicode", async () => {
    const payloads: Record<string, unknown>[] = [];
    let release = () => {};
    const first = new Promise<void>(resolve => { release = resolve; });
    const fetchMock = vi.fn(async (_url: string, options: RequestInit) => { const text = String(options.body); expect(new TextEncoder().encode(text).length).toBeLessThan(4096); payloads.push(JSON.parse(text)); if (payloads.length === 1) await first; return new Response('{"ok":true}'); });
    vi.stubGlobal("fetch", fetchMock);
    const controls = terminalControls(root, "one", new AbortController().signal, () => { throw new Error("unexpected failure"); });
    const text = "\u001b😀".repeat(600);
    controls.input(text); controls.resize(120, 40);
    await vi.waitFor(() => expect(payloads).toHaveLength(1));
    release(); await vi.waitFor(() => expect(payloads.at(-1)).toMatchObject({ columns: 120, rows: 40 }));
    expect(payloads.filter(body => body.data).map(body => body.data).join("")).toBe(text);
  });
  it("does not replay failed input or send queued commands after failure", async () => {
    const failed = vi.fn(), fetchMock = vi.fn(async () => { throw new Error("lost"); });
    vi.stubGlobal("fetch", fetchMock);
    const controls = terminalControls(root, "one", new AbortController().signal, failed);
    controls.input("one\n"); controls.input("two\n");
    await vi.waitFor(() => expect(failed).toHaveBeenCalledOnce());
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
