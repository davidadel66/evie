import { describe, expect, it } from "vitest";
import { splitBatch } from "./useSession";
import type { ServerEvent } from "./events";

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
});
