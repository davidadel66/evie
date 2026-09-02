// Everything stateful the reducer can't be: React state, the delta buffer, and
// the two request paths. Components read `items`/`status` and call
// `send`/`answer`; nothing else in the app touches the API.

import { useCallback, useEffect, useRef, useState } from "react";
import { answerApproval } from "../api/approve";
import { ApiError, StreamTruncated, streamChat } from "../api/stream";
import type { ServerEvent } from "./events";
import { appendUser, reduce, setApprovalState, type Item } from "./reducer";

export type Status = "idle" | "streaming" | "error";

/** Flush cadence and text pacing, the load-bearing perf decision
 *  (serve.decisions.md): render cost scales with flushes, not tokens, so
 *  events pool and render once per tick. Within a flush, text is released a
 *  slice at a time rather than all of it — network bursts render as steady
 *  typing instead of jumps (the REPL's smoothPrinter trick). The slice grows
 *  with the backlog, so a deep queue catches up instead of lagging. */
const FLUSH_MS = 40;

/** splitBatch gates text events (delta/reasoning) to `budget` characters,
 *  returning the events to apply now and the ones to hold for the next
 *  tick. Non-text events pass through in wire order — but nothing overtakes
 *  held-back text. Exported for tests; the UI only ever calls flush. */
export function splitBatch(
  batch: ServerEvent[],
  budget: number,
): [applied: ServerEvent[], rest: ServerEvent[]] {
  const applied: ServerEvent[] = [];
  const rest: ServerEvent[] = [];
  let left = budget;
  for (const ev of batch) {
    if (rest.length > 0) {
      rest.push(ev);
      continue;
    }
    if (ev.type !== "delta" && ev.type !== "reasoning") {
      applied.push(ev);
      continue;
    }
    if (ev.text.length <= left) {
      left -= ev.text.length;
      applied.push(ev);
    } else if (left > 0) {
      applied.push({ ...ev, text: ev.text.slice(0, left) });
      rest.push({ ...ev, text: ev.text.slice(left) });
    } else {
      rest.push(ev);
    }
  }
  return [applied, rest];
}

export type Session = {
  items: Item[];
  status: Status;
  /** Messages sent mid-turn, fired in order once the turn ends. */
  queue: string[];
  /** Banner text when status is "error"; null otherwise. */
  problem: string | null;
  send: (text: string) => void;
  answer: (reqId: string, approve: boolean) => void;
  dismissProblem: () => void;
  reset: () => void;
};

export function useSession(): Session {
  const [items, setItems] = useState<Item[]>([]);
  const [status, setStatus] = useState<Status>("idle");
  const [queue, setQueue] = useState<string[]>([]);
  const [problem, setProblem] = useState<string | null>(null);

  // Events pool here between flushes. A ref, not state: appending must not
  // render, and the timer reads whatever has landed by the time it fires.
  const pendingRef = useRef<ServerEvent[]>([]);
  const timerRef = useRef<number | null>(null);

  const flush = useCallback((pace: boolean) => {
    timerRef.current = null;
    const batch = pendingRef.current;
    if (batch.length === 0) return;

    let applied = batch;
    if (pace) {
      let backlog = 0;
      for (const ev of batch) {
        if (ev.type === "delta" || ev.type === "reasoning") {
          backlog += ev.text.length;
        }
      }
      const budget = Math.max(4, Math.ceil(backlog / 10));
      [applied, pendingRef.current] = splitBatch(batch, budget);
    } else {
      pendingRef.current = [];
    }

    // One setState for the whole batch — the point of the exercise.
    setItems((prev) => applied.reduce((acc, ev) => reduce(acc, ev), prev));

    // Held-back text drains on the next tick, no new event required.
    if (pendingRef.current.length > 0) {
      timerRef.current = window.setTimeout(() => flush(true), FLUSH_MS);
    }
  }, []);

  const enqueue = useCallback(
    (ev: ServerEvent) => {
      pendingRef.current.push(ev);
      // turn_done is the last event of the turn; dump everything still
      // pooled (the REPL's done() flushes its tail the same way) so the
      // composer re-enables immediately.
      if (ev.type === "turn_done" || ev.type === "response_discarded") {
        if (timerRef.current !== null) clearTimeout(timerRef.current);
        flush(false);
        return;
      }
      if (timerRef.current === null) {
        timerRef.current = window.setTimeout(() => flush(true), FLUSH_MS);
      }
    },
    [flush],
  );

  // A turn outliving the component would flush into a dead setState; abort it.
  const abortRef = useRef<AbortController | null>(null);
  useEffect(() => {
    return () => {
      abortRef.current?.abort();
      if (timerRef.current !== null) clearTimeout(timerRef.current);
    };
  }, []);

  const send = useCallback(
    (text: string) => {
      const message = text.trim();
      if (message === "") return;
      // A turn holds the session lock server-side (a second Send is a 409),
      // so mid-turn messages pool here instead. A queued message is NOT in
      // items — the transcript must never claim the server saw something it
      // hasn't.
      if (status === "streaming") {
        setQueue((q) => [...q, message]);
        return;
      }

      setItems((prev) => appendUser(prev, message));
      setStatus("streaming");
      setProblem(null);

      const ctl = new AbortController();
      abortRef.current = ctl;

      streamChat(
        message,
        (ev) => {
          // error events are banner state, not transcript state.
          if (ev.type === "error") setProblem(ev.message);
          else enqueue(ev);
        },
        ctl.signal,
      )
        .then(() => {
          // A turn that reported an error still completed; keep the banner but
          // let David type again.
          setStatus((s) => (s === "error" ? s : "idle"));
        })
        .catch((err: unknown) => {
          if (ctl.signal.aborted) return;
          flush(false);
          setProblem(describe(err));
          setStatus("error");
        });
    },
    [enqueue, flush, status],
  );

  const answer = useCallback((reqId: string, approve: boolean) => {
    // Optimistic: the click is the decision, the request only relays it.
    setItems((prev) =>
      setApprovalState(prev, reqId, approve ? "approved" : "declined"),
    );
    answerApproval(reqId, approve)
      .then((accepted) => {
        // 404 means the id was already gone — the turn moved on without this
        // answer, so the card must say expired, not approved.
        if (!accepted) {
          setItems((prev) => setApprovalState(prev, reqId, "expired"));
        }
      })
      .catch((err: unknown) => setProblem(describe(err)));
  }, []);

  const dismissProblem = useCallback(() => {
    setProblem(null);
    setStatus((s) => (s === "error" ? s : "idle"));
  }, []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = null;
    pendingRef.current = [];
    setItems([]);
    setQueue([]);
    setProblem(null);
    setStatus("idle");
  }, []);

  // Drain the queue: a finished turn fires the next waiting message. Only
  // "idle" drains — after an error the queue parks until David sends
  // something manually, rather than firing into a broken stream.
  useEffect(() => {
    if (status !== "idle" || queue.length === 0) return;
    const [next, ...rest] = queue;
    setQueue(rest);
    send(next);
  }, [status, queue, send]);

  return { items, status, queue, problem, send, answer, dismissProblem, reset };
}

/** describe turns a thrown value into banner text. The two typed failures get
 *  their own message; anything else (a network throw) falls back to its own. */
function describe(err: unknown): string {
  if (err instanceof StreamTruncated) return err.message;
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "something went wrong";
}
