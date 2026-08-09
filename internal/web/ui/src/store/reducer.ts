// The transcript model: one flat list of items, appended in wire order.
//
// Approvals are not their own item. The server emits tool_call →
// approval_request → (answer) → tool_result, and the design shows the pending
// card resolving into the compact tool row — so the approval lives on the tool
// item it gates.

import type { ServerEvent } from "./events";

export type ApprovalState = "pending" | "approved" | "declined" | "expired";

export type Approval = {
  /** The server's approval id — what /api/approve is called with. */
  reqId: string;
  state: ApprovalState;
};

export type Item =
  | { kind: "user"; key: string; text: string }
  | { kind: "assistant"; key: string; text: string; streaming: boolean }
  | {
      kind: "reasoning";
      key: string;
      text: string;
      streaming: boolean;
      /** Client-side clock at the first fragment — the "Thought for Ns". */
      startedAt: number;
      ms?: number;
    }
  | {
      kind: "tool";
      key: string;
      /** Tool call id from the provider; "" for a synthetic item. */
      id: string;
      name: string;
      args: string;
      approval?: Approval;
      result?: string;
      isErr?: boolean;
      /** Client-side clock, used for the card's duration chip. */
      startedAt: number;
      ms?: number;
    };

/** now is injectable so tests get deterministic durations. */
export type Clock = () => number;

let seq = 0;
/** Keys must be stable across re-renders and unique for React's reconciler;
 *  a counter is both, and unlike an index it survives an item being removed. */
function nextKey(prefix: string): string {
  seq += 1;
  return `${prefix}-${seq}`;
}

/** appendUser records what David sent. Not a server event — the UI knows the
 *  message before the stream exists, and shows it immediately. */
export function appendUser(items: Item[], text: string): Item[] {
  return [...items, { kind: "user", key: nextKey("u"), text }];
}

/** reduce folds one server event into the transcript. Pure: same inputs, same
 *  output, no mutation of the input array or its items. */
export function reduce(
  items: Item[],
  ev: ServerEvent,
  now: Clock = Date.now,
): Item[] {
  switch (ev.type) {
    case "delta": {
      const last = items[items.length - 1];
      if (last?.kind === "assistant" && last.streaming) {
        return replaceLast(items, { ...last, text: last.text + ev.text });
      }
      return [
        ...items,
        {
          kind: "assistant",
          key: nextKey("a"),
          text: ev.text,
          streaming: true,
        },
      ];
    }

    case "reasoning": {
      // Its own item kind, not part of the assistant message: thinking often
      // arrives before any content exists, and the transcript keeps wire order.
      const last = items[items.length - 1];
      if (last?.kind === "reasoning" && last.streaming) {
        return replaceLast(items, { ...last, text: last.text + ev.text });
      }
      return [
        ...items,
        {
          kind: "reasoning",
          key: nextKey("r"),
          text: ev.text,
          streaming: true,
          startedAt: now(),
        },
      ];
    }

    case "reasoning_done": {
      const last = items[items.length - 1];
      if (last?.kind !== "reasoning" || !last.streaming) return items;
      return replaceLast(items, {
        ...last,
        streaming: false,
        ms: now() - last.startedAt,
      });
    }

    case "assistant_done": {
      const last = items[items.length - 1];
      if (last?.kind !== "assistant" || !last.streaming) return items;
      // AssistantDone fires for every assistant message, including the
      // tool-only ones that carry no text. The design has no empty bubbles,
      // so drop it rather than render a blank.
      if (last.text === "") return items.slice(0, -1);
      return replaceLast(items, { ...last, streaming: false });
    }

    case "tool_call": {
      // A tool call ends the assistant message that requested it: the model's
      // text came before the calls, and any later delta is a new message.
      const closed = closeStreaming(items);
      return [
        ...closed,
        {
          kind: "tool",
          key: nextKey("t"),
          id: ev.id,
          name: ev.name,
          args: ev.args,
          startedAt: now(),
        },
      ];
    }

    case "approval_request": {
      const idx = findAwaitingTool(items, ev.name);
      const approval: Approval = { reqId: ev.id, state: "pending" };
      if (idx === -1) {
        // No matching tool call — the request arrived out of band (a reload
        // mid-turn, say). Synthesize the item so the card is still actionable
        // rather than dropping an approval David has to answer.
        return [
          ...items,
          {
            kind: "tool",
            key: nextKey("t"),
            id: "",
            name: ev.name,
            args: ev.args,
            approval,
            startedAt: now(),
          },
        ];
      }
      return replaceAt(items, idx, { ...items[idx], approval } as Item);
    }

    case "tool_result": {
      const idx = findToolByID(items, ev.id);
      if (idx === -1) return items;
      const tool = items[idx];
      if (tool.kind !== "tool") return items;
      // The approval state is left alone: the click already set it, and a
      // still-pending state here means the server resolved it without us
      // (expiry), which turn_done settles.
      return replaceAt(items, idx, {
        ...tool,
        result: ev.content,
        isErr: ev.isError,
        ms: now() - tool.startedAt,
      });
    }

    case "turn_done":
      return items.map((it) => {
        if (it.kind === "assistant" && it.streaming) {
          return { ...it, streaming: false };
        }
        if (it.kind === "reasoning" && it.streaming) {
          // The server guarantees reasoning_done first; if the turn ended
          // without one, close the block rather than leave "Thinking…" up.
          return { ...it, streaming: false, ms: now() - it.startedAt };
        }
        if (it.kind === "tool" && it.approval?.state === "pending") {
          // The turn is over and nobody answered: the server took Expired.
          return { ...it, approval: { ...it.approval, state: "expired" } };
        }
        return it;
      });

    case "error":
      // Errors are banner state, not transcript state (the design puts them
      // in a top banner). useSession picks this up; the list is untouched.
      return items;
  }
}

/** setApprovalState is the click path: the browser knows the outcome before
 *  any event confirms it, so the card updates immediately. */
export function setApprovalState(
  items: Item[],
  reqId: string,
  state: ApprovalState,
): Item[] {
  return items.map((it) =>
    it.kind === "tool" && it.approval?.reqId === reqId
      ? { ...it, approval: { ...it.approval, state } }
      : it,
  );
}

/** findAwaitingTool locates the newest tool call of this name that hasn't run
 *  and isn't already gated — the one this approval must belong to. Matching by
 *  name (not id) is forced: approval_request carries its own id, not the tool
 *  call's. */
function findAwaitingTool(items: Item[], name: string): number {
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i];
    if (
      it.kind === "tool" &&
      it.name === name &&
      it.result === undefined &&
      !it.approval
    ) {
      return i;
    }
  }
  return -1;
}

function findToolByID(items: Item[], id: string): number {
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i];
    if (it.kind === "tool" && it.id === id) return i;
  }
  return -1;
}

function closeStreaming(items: Item[]): Item[] {
  const last = items[items.length - 1];
  if (last?.kind !== "assistant" || !last.streaming) return items;
  if (last.text === "") return items.slice(0, -1);
  return replaceLast(items, { ...last, streaming: false });
}

function replaceLast(items: Item[], item: Item): Item[] {
  return replaceAt(items, items.length - 1, item);
}

function replaceAt(items: Item[], idx: number, item: Item): Item[] {
  const out = items.slice();
  out[idx] = item;
  return out;
}
