import { describe, expect, it } from "vitest";
import { appendUser, reduce, setApprovalState, type Item } from "./reducer";
import { parseEvent, type ServerEvent } from "./events";

/** fold replays a whole event sequence, the way a real turn arrives. */
function fold(events: ServerEvent[], start: Item[] = [], now = () => 1000) {
  return events.reduce((items, ev) => reduce(items, ev, now), start);
}

describe("reduce", () => {
  it("streams deltas into one assistant item", () => {
    const items = fold([
      { type: "delta", text: "Hel" },
      { type: "delta", text: "lo" },
    ]);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "assistant",
      text: "Hello",
      streaming: true,
    });
  });

  it("closes the assistant item on assistant_done", () => {
    const items = fold([
      { type: "delta", text: "hi" },
      { type: "assistant_done", content: "hi" },
    ]);
    expect(items[0]).toMatchObject({ streaming: false, text: "hi" });
  });

  it("uses committed async-provider content when no delta was admitted", () => {
    const items = fold([
      { type: "reasoning", text: "thinking" },
      { type: "reasoning_done" },
      { type: "assistant_done", content: "complete answer" },
      { type: "turn_done" },
    ]);
    expect(items).toHaveLength(2);
    expect(items[1]).toMatchObject({
      kind: "assistant",
      text: "complete answer",
      streaming: false,
    });
  });

  it("reconciles a partial async-provider delta to committed content", () => {
    const items = fold([
      { type: "delta", text: "complete " },
      { type: "assistant_done", content: "complete answer" },
      { type: "turn_done" },
    ]);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "assistant",
      text: "complete answer",
      streaming: false,
    });
  });

  it("replaces a divergent async-provider delta with committed content", () => {
    const items = fold([
      { type: "delta", text: "speculative answer" },
      { type: "assistant_done", content: "committed answer" },
      { type: "turn_done" },
    ]);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "assistant",
      text: "committed answer",
      streaming: false,
    });
  });

  it("removes streamed text when committed assistant content is empty", () => {
    const items = fold([
      { type: "delta", text: "speculative answer" },
      { type: "assistant_done", content: "" },
      { type: "turn_done" },
    ]);
    expect(items).toEqual([]);
  });

  it("drops the empty assistant message of a tool-only turn", () => {
    // The real sequence: no deltas, assistant_done with no content, then the
    // tool calls. An empty bubble must never reach the transcript.
    const items = fold([
      { type: "assistant_done", content: "" },
      { type: "tool_call", id: "c1", name: "get_time", args: "{}" },
      { type: "tool_result", id: "c1", content: "10:24", isError: false },
      { type: "turn_done" },
    ]);
    expect(items.map((i) => i.kind)).toEqual(["tool"]);
  });

  it("starts a fresh assistant item after a tool result", () => {
    const items = fold([
      { type: "delta", text: "checking" },
      { type: "assistant_done", content: "checking" },
      { type: "tool_call", id: "c1", name: "get_time", args: "{}" },
      { type: "tool_result", id: "c1", content: "10:24", isError: false },
      { type: "delta", text: "it is 10:24" },
      { type: "assistant_done", content: "it is 10:24" },
      { type: "turn_done" },
    ]);
    expect(items.map((i) => i.kind)).toEqual(["assistant", "tool", "assistant"]);
    expect(items[2]).toMatchObject({ text: "it is 10:24", streaming: false });
  });

  it("records the tool result and its error flag", () => {
    const items = fold([
      { type: "tool_call", id: "c1", name: "bash", args: '{"cmd":"ls"}' },
      { type: "tool_result", id: "c1", content: "boom", isError: true },
    ]);
    expect(items[0]).toMatchObject({ result: "boom", isErr: true });
  });

  it("measures elapsed time between call and result", () => {
    const clock = (() => {
      const times = [1000, 1250];
      let i = 0;
      return () => times[i++];
    })();
    const items = fold(
      [
        { type: "tool_call", id: "c1", name: "bash", args: "{}" },
        { type: "tool_result", id: "c1", content: "ok", isError: false },
      ],
      [],
      clock,
    );
    expect(items[0]).toMatchObject({ ms: 250 });
  });

  it("ignores a tool_result for an unknown id", () => {
    const items = fold([
      { type: "tool_result", id: "ghost", content: "x", isError: false },
    ]);
    expect(items).toEqual([]);
  });

  it("attaches an approval to the matching pending tool call", () => {
    const preview = {
      path: "/tmp/a",
      oldText: "before",
      newText: "after",
      isNew: false,
    };
    const items = fold([
      { type: "tool_call", id: "c1", name: "edit_file", args: '{"path":"a"}' },
      {
        type: "approval_request",
        id: "ap1",
        name: "edit_file",
        args: '{"path":"a"}',
        preview,
      },
    ]);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "tool",
      approval: { reqId: "ap1", state: "pending", preview },
    });
  });

  it("picks the newest ungated call when a tool is called twice", () => {
    const items = fold([
      { type: "tool_call", id: "c1", name: "edit_file", args: '{"path":"a"}' },
      { type: "approval_request", id: "ap1", name: "edit_file", args: '{"path":"a"}' },
      { type: "tool_result", id: "c1", content: "done", isError: false },
      { type: "tool_call", id: "c2", name: "edit_file", args: '{"path":"b"}' },
      { type: "approval_request", id: "ap2", name: "edit_file", args: '{"path":"b"}' },
    ]);
    expect(items[0]).toMatchObject({ approval: { reqId: "ap1" } });
    expect(items[1]).toMatchObject({ approval: { reqId: "ap2", state: "pending" } });
  });

  it("synthesizes a tool item for an out-of-band approval", () => {
    const items = fold([
      { type: "approval_request", id: "ap1", name: "edit_file", args: '{"path":"a"}' },
    ]);
    expect(items[0]).toMatchObject({
      kind: "tool",
      id: "",
      name: "edit_file",
      approval: { reqId: "ap1", state: "pending" },
    });
  });

  it("expires a still-pending approval at turn_done", () => {
    const items = fold([
      { type: "tool_call", id: "c1", name: "edit_file", args: "{}" },
      { type: "approval_request", id: "ap1", name: "edit_file", args: "{}" },
      { type: "turn_done" },
    ]);
    expect(items[0]).toMatchObject({ approval: { state: "expired" } });
  });

  it("leaves a resolved approval alone at turn_done", () => {
    let items = fold([
      { type: "tool_call", id: "c1", name: "edit_file", args: "{}" },
      { type: "approval_request", id: "ap1", name: "edit_file", args: "{}" },
    ]);
    items = setApprovalState(items, "ap1", "approved");
    items = reduce(items, { type: "tool_result", id: "c1", content: "ok", isError: false });
    items = reduce(items, { type: "turn_done" });
    expect(items[0]).toMatchObject({
      approval: { state: "approved" },
      result: "ok",
    });
  });

  it("closes a streaming assistant item at turn_done", () => {
    const items = fold([{ type: "delta", text: "cut off" }, { type: "turn_done" }]);
    expect(items[0]).toMatchObject({ streaming: false, text: "cut off" });
  });

  it("preserves partial text with an inline discarded warning through turn_done", () => {
    const items = fold([
      { type: "delta", text: "partial answer" },
      {
        type: "response_discarded",
        reason: "lease_lost",
        message: "Response interrupted; streamed text was not saved.",
      },
      { type: "turn_done" },
    ]);
    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({
      kind: "assistant",
      text: "partial answer",
      streaming: false,
      discarded: {
        reason: "lease_lost",
        message: "Response interrupted; streamed text was not saved.",
      },
    });
  });

  it("adds a standalone discarded warning for reasoning-only output", () => {
    const items = fold([
      { type: "reasoning", text: "unfinished thought" },
      { type: "reasoning_done" },
      {
        type: "response_discarded",
        reason: "assistant_persistence_failed",
        message: "Response interrupted; streamed text was not saved.",
      },
      { type: "turn_done" },
    ]);
    expect(items.map((item) => item.kind)).toEqual(["reasoning", "notice"]);
    expect(items[1]).toMatchObject({
      kind: "notice",
      reason: "assistant_persistence_failed",
      text: "Response interrupted; streamed text was not saved.",
    });
  });

  it("streams reasoning fragments into one item, in wire order", () => {
    const items = fold([
      { type: "reasoning", text: "Compute " },
      { type: "reasoning", text: "17*23" },
      { type: "reasoning_done" },
      { type: "delta", text: "391" },
      { type: "assistant_done", content: "391" },
      { type: "turn_done" },
    ]);
    expect(items.map((i) => i.kind)).toEqual(["reasoning", "assistant"]);
    expect(items[0]).toMatchObject({
      kind: "reasoning",
      text: "Compute 17*23",
      streaming: false,
    });
    expect(items[1]).toMatchObject({
      kind: "assistant",
      text: "391",
      streaming: false,
    });
  });

  it("measures the thought duration client-side", () => {
    let t = 1000;
    const clock = () => t;
    let items = fold([{ type: "reasoning", text: "hmm" }], [], clock);
    t = 7000; // 6s of thinking
    items = fold([{ type: "reasoning_done" }], items, clock);
    expect(items[0]).toMatchObject({ streaming: false, ms: 6000 });
  });

  it("closes a reasoning block left open at turn_done", () => {
    const items = fold([
      { type: "reasoning", text: "cut off" },
      { type: "turn_done" },
    ]);
    expect(items[0]).toMatchObject({ kind: "reasoning", streaming: false });
  });

  it("starts a fresh reasoning item per assistant message", () => {
    const items = fold([
      { type: "reasoning", text: "first thought" },
      { type: "reasoning_done" },
      { type: "assistant_done", content: "" },
      { type: "tool_call", id: "c1", name: "echo", args: "{}" },
      { type: "tool_result", id: "c1", content: "ok", isError: false },
      { type: "reasoning", text: "second thought" },
      { type: "reasoning_done" },
      { type: "delta", text: "answer" },
      { type: "assistant_done", content: "answer" },
      { type: "turn_done" },
    ]);
    expect(items.map((i) => i.kind)).toEqual([
      "reasoning",
      "tool",
      "reasoning",
      "assistant",
    ]);
  });

  it("leaves the transcript untouched on error", () => {
    const before = fold([{ type: "delta", text: "hi" }]);
    const after = reduce(before, { type: "error", message: "provider down" });
    expect(after).toEqual(before);
  });

  it("does not mutate its input", () => {
    const before = fold([{ type: "delta", text: "hi" }]);
    const snapshot = structuredClone(before);
    reduce(before, { type: "delta", text: " there" });
    expect(before).toEqual(snapshot);
  });
});

describe("appendUser", () => {
  it("appends a user item with a unique key", () => {
    const items = appendUser(appendUser([], "one"), "two");
    expect(items.map((i) => (i as { text: string }).text)).toEqual(["one", "two"]);
    expect(items[0].key).not.toEqual(items[1].key);
  });
});

describe("parseEvent", () => {
  it("parses a known event", () => {
    expect(parseEvent("delta", '{"text":"hi"}')).toEqual({
      type: "delta",
      text: "hi",
    });
  });

  it("ignores unknown event names so future board_* events are harmless", () => {
    expect(parseEvent("board_start", '{"id":"b1"}')).toBeNull();
  });

  it("ignores malformed JSON rather than throwing mid-stream", () => {
    expect(parseEvent("delta", "{not json")).toBeNull();
  });

  it("parses turn_done's empty payload", () => {
    expect(parseEvent("turn_done", "{}")).toEqual({ type: "turn_done" });
  });

  it("parses the reasoning pair", () => {
    expect(parseEvent("reasoning", '{"text":"hmm"}')).toEqual({
      type: "reasoning",
      text: "hmm",
    });
    expect(parseEvent("reasoning_done", "{}")).toEqual({
      type: "reasoning_done",
    });
  });

  it("parses response_discarded as a known event", () => {
    expect(
      parseEvent(
        "response_discarded",
        '{"reason":"provider_response_invalid","message":"Response interrupted; streamed text was not saved."}',
      ),
    ).toEqual({
      type: "response_discarded",
      reason: "provider_response_invalid",
      message: "Response interrupted; streamed text was not saved.",
    });
  });

  it("parses a full-file approval preview", () => {
    expect(
      parseEvent(
        "approval_request",
        '{"id":"ap1","name":"edit_file","args":"{}","preview":{"path":"/tmp/a","oldText":"before","newText":"after","isNew":false}}',
      ),
    ).toMatchObject({
      type: "approval_request",
      preview: {
        path: "/tmp/a",
        oldText: "before",
        newText: "after",
        isNew: false,
      },
    });
  });
});
