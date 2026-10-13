import { describe, expect, it } from "vitest";
import { mergeFollowed, splitBatch } from "./useSession";
import type { ServerEvent } from "./events";
import { reduce, type Item } from "./reducer";

describe("splitBatch", () => {
  it("applies everything when text fits the budget", () => {
    const batch: ServerEvent[] = [
      { type: "delta", text: "ab" },
      { type: "reasoning", text: "cd" },
      { type: "reasoning_done" },
    ];
    const [applied, rest] = splitBatch(batch, 10);
    expect(applied).toEqual(batch);
    expect(rest).toEqual([]);
  });

  it("splits a text event that straddles the budget", () => {
    const batch: ServerEvent[] = [
      { type: "delta", text: "abc" },
      { type: "delta", text: "def" },
    ];
    const [applied, rest] = splitBatch(batch, 4);
    expect(applied).toEqual([
      { type: "delta", text: "abc" },
      { type: "delta", text: "d" },
    ]);
    expect(rest).toEqual([{ type: "delta", text: "ef" }]);
  });

  it("lets nothing overtake held-back text, even non-text events", () => {
    const batch: ServerEvent[] = [
      { type: "delta", text: "abcdef" },
      { type: "assistant_done", content: "abcdef" },
      { type: "turn_done" },
    ];
    const [applied, rest] = splitBatch(batch, 2);
    expect(applied).toEqual([{ type: "delta", text: "ab" }]);
    expect(rest).toEqual([
      { type: "delta", text: "cdef" },
      { type: "assistant_done", content: "abcdef" },
      { type: "turn_done" },
    ]);
  });

  it("passes non-text events through while budget remains", () => {
    const batch: ServerEvent[] = [
      { type: "reasoning", text: "hmm" },
      { type: "reasoning_done" },
      { type: "delta", text: "ok" },
    ];
    const [applied, rest] = splitBatch(batch, 3);
    expect(applied).toEqual([
      { type: "reasoning", text: "hmm" },
      { type: "reasoning_done" },
    ]);
    expect(rest).toEqual([{ type: "delta", text: "ok" }]);
  });

  it("never lets a discarded marker overtake buffered text", () => {
    const batch: ServerEvent[] = [
      { type: "delta", text: "partial" },
      {
        type: "response_discarded",
        reason: "provider_error",
        message: "Response interrupted; streamed text was not saved.",
      },
    ];
    const [applied, rest] = splitBatch(batch, 3);
    expect(applied).toEqual([{ type: "delta", text: "par" }]);
    expect(rest).toEqual([
      { type: "delta", text: "tial" },
      {
        type: "response_discarded",
        reason: "provider_error",
        message: "Response interrupted; streamed text was not saved.",
      },
    ]);
  });
});

// multi-chat.spec.md Stage 3: rejoining a chat rebuilds its running turn from
// the feed and leaves out the history copy of that turn.
describe("mergeFollowed", () => {
  const started: ServerEvent = { type: "turn_started", id: "t2", startedAt: 5, status: "working" };
  const history: Item[] = [
    { kind: "user", key: "t1", text: "earlier", turn: { id: "t1", status: "complete" } },
    { kind: "assistant", key: "a1", text: "earlier answer", streaming: false, phase: "final_answer", turn: { id: "t1", status: "complete" } },
    { kind: "user", key: "t2", text: "edit it", turn: { id: "t2", status: "working" } },
  ];
  const replay: ServerEvent[] = [
    { type: "user_message", turnId: "t2", text: "edit it" },
    started,
    { type: "tool_call", id: "c1", name: "edit_file", args: "{}" },
    { type: "approval_request", id: "p1", name: "edit_file", args: "{}" },
    { type: "approval_decided", id: "p1", state: "approved" },
  ];

  it("shows the running turn once, with its recorded approval", () => {
    const merged = mergeFollowed(history, replay, true);
    expect(merged.status).toBe("streaming");
    expect(merged.items.filter((item) => item.kind === "user").map((item) => item.kind === "user" && item.text)).toEqual(["earlier", "edit it"]);
    const tool = merged.items.find((item) => item.kind === "tool");
    expect(tool?.kind === "tool" && tool.approval?.state).toBe("approved");
    // The tool's result arriving later keeps the recorded answer.
    const after = reduce(merged.items, { type: "tool_result", id: "c1", content: "ok", isError: false });
    const settled = after.find((item) => item.kind === "tool");
    expect(settled?.kind === "tool" && settled.approval?.state).toBe("approved");
  });

  it("ends idle when the turn finished before history arrived, and holds the queue after a refusal", () => {
    expect(mergeFollowed(history, [...replay, { type: "turn_done" }], true).status).toBe("idle");
    expect(mergeFollowed([], [], false).status).toBe("idle");
    const refused = mergeFollowed([], [{ type: "error", message: "a turn is already in progress", code: "turn_refused" }, { type: "turn_done" }], true);
    expect(refused).toMatchObject({ status: "error", problem: "a turn is already in progress" });
  });
});
