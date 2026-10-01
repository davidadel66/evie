// Everything stateful the reducer can't be: React state, the delta buffer, and
// the request paths. Components read `items`/`status` and call
// `send`/`answer`/`stop`; nothing else in the app touches the chat API.

import { useCallback, useEffect, useRef, useState } from "react";
import { readHistory } from "../api/history";
import { answerApproval } from "../api/approve";
import { ApiError, StreamTruncated, streamChat } from "../api/stream";
import { cancelTurn, compactSession, compactionNotice, type ComposerNotice } from "../api/turnControls";
import { composerCommand } from "../chat/composerCommand";
import { isOwnerStop, type ServerEvent } from "./events";
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
  clearQueue: () => void;
  /** Banner text when status is "error"; null otherwise. */
  problem: string | null;
  /** Sends a message or runs a composer command. False leaves the draft in
   *  place: nothing was sent, queued, or started. */
  send: (text: string) => boolean;
  answer: (reqId: string, approve: boolean) => void;
  /** Asks the server to stop the running turn; its stream reports the end. */
  stop: () => void;
  stopping: boolean;
  /** A `/compact` request is running; messages queue until it finishes. */
  compacting: boolean;
  /** The latest composer command's result. */
  notice: ComposerNotice | null;
  dismissProblem: () => void;
  reset: () => void;
  historyLoading: boolean;
  historyProblem: string | null;
  hasOlder: boolean;
  loadOlder: () => void;
  retryHistory: () => void;
};

export function useSession(sessionId?: string, model?: string): Session {
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyProblem, setHistoryProblem] = useState<string | null>(null);
  const [before, setBefore] = useState<string>();
  const historyCursor = useRef<string | undefined>(undefined);
  const historyAbort = useRef<AbortController | null>(null);
  const historySession = useRef(sessionId);
  const historyReady = useRef(false);
  const epoch = useRef(0);
  const [items, setItems] = useState<Item[]>([]);
  const [status, setStatus] = useState<Status>("idle");
  const [queue, setQueue] = useState<string[]>([]);
  const [problem, setProblem] = useState<string | null>(null);
  const [stopping, setStopping] = useState(false);
  const [compacting, setCompacting] = useState(false);
  const [notice, setNotice] = useState<ComposerNotice | null>(null);

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

  // Manual compaction shares the turn's session lock server-side, so it is
  // refused while a turn streams rather than queued behind it.
  const compact = useCallback((): boolean => {
    if (status === "streaming") {
      setNotice({ tone: "warning", text: "Finish or stop the current turn before compacting." });
      return false;
    }
    if (compacting) return false;
    const generation = epoch.current;
    setCompacting(true);
    setNotice({ tone: "info", text: "Compacting context…" });
    compactSession(sessionId)
      .then((outcome) => { if (generation === epoch.current) setNotice(compactionNotice(outcome)); })
      .catch((err: unknown) => { if (generation === epoch.current) setNotice({ tone: "warning", text: describe(err) }); })
      .finally(() => { if (generation === epoch.current) setCompacting(false); });
    return true;
  }, [status, compacting, sessionId]);

  const send = useCallback(
    (text: string): boolean => {
      const message = text.trim();
      if (message === "" || !historyReady.current || historyLoading || historyProblem || historySession.current !== sessionId) return false;
      const command = composerCommand(message);
      if (command?.kind === "usage") {
        setNotice({ tone: "warning", text: command.message });
        return false;
      }
      if (command?.kind === "compact") return compact();
      // A turn holds the session lock server-side (a second Send is a 409),
      // so mid-turn messages pool here instead. A queued message is NOT in
      // items — the transcript must never claim the server saw something it
      // hasn't.
      if (status === "streaming" || compacting) {
        setQueue((q) => [...q, message]);
        return true;
      }

      setItems((prev) => appendUser(prev, message));
      setStatus("streaming");
      setProblem(null);
      setNotice(null);

      const ctl = new AbortController();
      abortRef.current = ctl;

      streamChat(
        message,
        (ev) => {
          if (ctl.signal.aborted) return;
          // Keep the banner and stop activity if the server reports an error.
          // A stop David asked for is his own action, shown in the transcript.
          if (ev.type === "error" && !isOwnerStop(ev)) setProblem(ev.message);
          enqueue(ev);
        },
        ctl.signal,
        sessionId,
        model,
      )
        .then(() => {
          if (ctl.signal.aborted) return;
          setStopping(false);
          // A turn that reported an error still completed; keep the banner but
          // let David type again.
          setStatus((s) => (s === "error" ? s : "idle"));
        })
        .catch((err: unknown) => {
          if (ctl.signal.aborted) return;
          setStopping(false);
          flush(false);
          setItems((prev) => reduce(prev, { type: "error", message: describe(err) }));
          setProblem(describe(err));
          setStatus("error");
        });
      return true;
    },
    [enqueue, flush, compact, status, compacting, historyLoading, historyProblem, sessionId, model],
  );

  // Stopping is a request: the turn keeps streaming until the server records
  // the interruption and ends the stream, which returns the UI to idle.
  const stop = useCallback(() => {
    if (status !== "streaming" || stopping) return;
    const generation = epoch.current;
    setStopping(true);
    cancelTurn(sessionId)
      .then((result) => {
        // "idle": the turn ended before the request arrived; its stream is
        // already finishing on its own.
        if (generation === epoch.current && result === "idle") setStopping(false);
      })
      .catch((err: unknown) => {
        if (generation !== epoch.current) return;
        setStopping(false);
        setProblem(describe(err));
      });
  }, [status, stopping, sessionId]);

  const answer = useCallback((reqId: string, approve: boolean) => {
    const generation = epoch.current;
    // Optimistic: the click is the decision, the request only relays it.
    setItems((prev) =>
      setApprovalState(prev, reqId, approve ? "approved" : "declined"),
    );
    answerApproval(reqId, approve)
      .then((accepted) => {
        if (generation !== epoch.current) return;
        // 404 means the id was already gone — the turn moved on without this
        // answer, so the card must say expired, not approved.
        if (!accepted) {
          setItems((prev) => setApprovalState(prev, reqId, "expired"));
        }
      })
      .catch((err: unknown) => { if (generation === epoch.current) setProblem(describe(err)); });
  }, []);

  const dismissProblem = useCallback(() => {
    setProblem(null);
    setStatus((s) => (s === "error" ? s : "idle"));
  }, []);

  const reset = useCallback(() => {
    epoch.current++;
    historyReady.current = false;
    abortRef.current?.abort();
    abortRef.current = null;
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = null;
    pendingRef.current = [];
    setItems([]);
    setQueue([]);
    setProblem(null);
    setStopping(false);
    setCompacting(false);
    setNotice(null);
    setStatus("idle");
  }, []);

  const loadHistory = useCallback(async (cursor?: string) => {
    if (!sessionId) return;
    historyAbort.current?.abort();
    historyCursor.current = cursor;
    const ctl = new AbortController();
    historyAbort.current = ctl;
    historyReady.current = false;
    setHistoryLoading(true);
    setHistoryProblem(null);
    try {
      const page = await readHistory(sessionId, cursor, ctl.signal);
      if (ctl.signal.aborted) return;
      setItems((current) => cursor ? [...page.items, ...current] : page.items);
      setBefore(page.before);
      historyReady.current = true;
    } catch (error) {
      if (!ctl.signal.aborted) setHistoryProblem(describe(error));
    } finally {
      if (!ctl.signal.aborted) setHistoryLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    historySession.current = sessionId;
    reset();
    setBefore(undefined);
    setHistoryProblem(null);
    if (sessionId) void loadHistory();
    else setHistoryLoading(false);
    return () => { historyAbort.current?.abort(); };
  }, [sessionId, reset, loadHistory]);

  // Drain the queue: a finished turn fires the next waiting message. Only
  // "idle" drains — after an error the queue parks until David sends
  // something manually, rather than firing into a broken stream. A running
  // compaction holds the queue the same way a turn does.
  useEffect(() => {
    if (status !== "idle" || compacting || queue.length === 0 || !historyReady.current || historySession.current !== sessionId) return;
    const [next, ...rest] = queue;
    setQueue(rest);
    send(next);
  }, [status, compacting, queue, send, sessionId]);

  return { items: historySession.current === sessionId ? items : [], status, queue, clearQueue: () => setQueue([]), problem, send, answer, stop, stopping, compacting, notice, dismissProblem, reset, historyLoading, historyProblem, hasOlder: !!before, loadOlder: () => void loadHistory(before), retryHistory: () => void loadHistory(historyCursor.current) };
}

/** describe turns a thrown value into banner text. The two typed failures get
 *  their own message; anything else (a network throw) falls back to its own. */
function describe(err: unknown): string {
  if (err instanceof StreamTruncated) return err.message;
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "something went wrong";
}
