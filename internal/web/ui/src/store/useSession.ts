// Everything stateful the reducer can't be: React state, the delta buffer, and
// the request paths. Components read `items`/`status` and call
// `send`/`answer`/`stop`; nothing else in the app touches the chat API.

import { useCallback, useEffect, useRef, useState } from "react";
import { readHistory } from "../api/history";
import { answerApproval } from "../api/approve";
import { ApiError, StreamTruncated, followChat, sendDetached } from "../api/stream";
import { cancelTurn, compactSession, compactionNotice, type ComposerNotice } from "../api/turnControls";
import { composerCommand } from "../chat/composerCommand";
import { approvalAlternative } from "../chat/approvalArgs";
import { isOwnerStop, type ServerEvent } from "./events";
import { appendUser, reduce, setApprovalAlwaysApplies, setApprovalState, type Item } from "./reducer";

export type Status = "idle" | "streaming" | "error";

// Each chat's last view, so switching back shows it at once while its
// history and feed catch up; queued messages wait with their chat
// (multi-chat.spec.md Stage 3).
const views = new Map<string, { items: Item[]; queue: string[] }>();

/** How long to wait before following a chat's feed again after it dropped. */
const RECONNECT_MS = 1000;

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
  /** Records a memory card's "Always applies" switch before it is answered. */
  setAlwaysApplies: (reqId: string, alwaysApplies: boolean) => void;
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
  // answer reads the latest card state (the "Always applies" switch) whether
  // it comes from the card's button or the keyboard shortcut.
  const latestItems = useRef(items);
  latestItems.current = items;
  const [status, setStatus] = useState<Status>("idle");
  const [queue, setQueue] = useState<string[]>([]);
  const latestQueue = useRef(queue);
  latestQueue.current = queue;
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

  // The followed feed outliving the component would flush into a dead
  // setState; abort it. Aborting never stops a turn: only stop does.
  const followRef = useRef<AbortController | null>(null);
  const reconnectRef = useRef<number | null>(null);
  // Turns this view follows live; history copies of them are left out.
  const liveTurns = useRef(new Set<string>());
  useEffect(() => {
    return () => {
      followRef.current?.abort();
      if (reconnectRef.current !== null) clearTimeout(reconnectRef.current);
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
      // The composer opens the model picker before sending; never send it as text.
      if (command?.kind === "model") return false;
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

      // The turn runs on the server whether or not this tab stays; its
      // events arrive through the chat's feed, which this view follows.
      const generation = epoch.current;
      sendDetached(message, sessionId, model).catch((err: unknown) => {
        if (generation !== epoch.current) return;
        setStopping(false);
        flush(false);
        setItems((prev) => reduce(prev, { type: "error", message: describe(err) }));
        setProblem(describe(err));
        setStatus("error");
      });
      return true;
    },
    [flush, compact, status, compacting, historyLoading, historyProblem, sessionId, model],
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
    const tool = latestItems.current.find((item) => item.kind === "tool" && item.approval?.reqId === reqId);
    const alternative = approve && tool?.kind === "tool" && approvalAlternative(tool.name, tool.args, tool.approval?.alwaysApplies);
    // Optimistic: the click is the decision, the request only relays it.
    setItems((prev) =>
      setApprovalState(prev, reqId, approve ? "approved" : "declined"),
    );
    answerApproval(reqId, approve, alternative)
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

  const setAlwaysApplies = useCallback((reqId: string, alwaysApplies: boolean) => {
    setItems((prev) => setApprovalAlwaysApplies(prev, reqId, alwaysApplies));
  }, []);

  const dismissProblem = useCallback(() => {
    setProblem(null);
    setStatus((s) => (s === "error" ? s : "idle"));
  }, []);

  const reset = useCallback((id?: string) => {
    epoch.current++;
    historyReady.current = false;
    followRef.current?.abort();
    followRef.current = null;
    if (reconnectRef.current !== null) clearTimeout(reconnectRef.current);
    reconnectRef.current = null;
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = null;
    pendingRef.current = [];
    liveTurns.current = new Set();
    const view = id ? views.get(id) : undefined;
    setItems(view?.items ?? []);
    setQueue(view?.queue ?? []);
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
      const settled = page.items.filter((item) => !(item.turn && liveTurns.current.has(item.turn.id)));
      setItems((current) => cursor ? [...settled, ...current] : settled);
      setBefore(page.before);
      historyReady.current = true;
    } catch (error) {
      if (!ctl.signal.aborted) setHistoryProblem(describe(error));
    } finally {
      if (!ctl.signal.aborted) setHistoryLoading(false);
    }
  }, [sessionId]);

  // Live events of the followed chat, after its history is in place.
  const onLive = useCallback((ev: ServerEvent) => {
    if (ev.type === "feed_state") return;
    if (ev.type === "user_message") liveTurns.current.add(ev.turnId);
    if (ev.type === "turn_started") liveTurns.current.add(ev.id);
    if (ev.type === "user_message" || ev.type === "turn_started") setStatus((s) => (s === "error" ? s : "streaming"));
    // Keep the banner if the server reports an error. A stop David asked
    // for is his own action, shown in the transcript.
    if (ev.type === "error" && !isOwnerStop(ev)) setProblem(ev.message);
    // A refused turn holds queued messages, as a refused send always has.
    if (ev.type === "error" && ev.code === "turn_refused") setStatus("error");
    if (ev.type === "turn_done") {
      setStopping(false);
      setStatus((s) => (s === "error" ? s : "idle"));
    }
    enqueue(ev);
  }, [enqueue]);

  // follow subscribes to the chat's feed first, then reads its latest history
  // page: the running turn is rebuilt from the feed and its history copy left
  // out, so nothing is missed or shown twice. A dropped feed is followed
  // again after a pause, catching up the same way.
  const follow = useCallback((id: string) => {
    followRef.current?.abort();
    const ctl = new AbortController();
    followRef.current = ctl;
    const early: ServerEvent[] = [];
    let live = false;
    let running = false;
    historyReady.current = false;
    setHistoryLoading(true);
    setHistoryProblem(null);
    const catchUp = async () => {
      try {
        const page = await readHistory(id, undefined, ctl.signal);
        if (ctl.signal.aborted) return;
        const merged = mergeFollowed(page.items, early, running);
        liveTurns.current = merged.liveTurns;
        if (merged.problem) setProblem(merged.problem);
        pendingRef.current = [];
        setItems(merged.items);
        setBefore(page.before);
        setStatus((s) => (merged.status === "idle" && s === "error" ? s : merged.status));
        historyReady.current = true;
        live = true;
      } catch (error) {
        if (!ctl.signal.aborted) setHistoryProblem(describe(error));
      } finally {
        if (!ctl.signal.aborted) setHistoryLoading(false);
      }
    };
    followChat(id, (ev) => {
      if (ctl.signal.aborted) return;
      if (live) return onLive(ev);
      if (ev.type === "feed_state") {
        running = ev.running;
        void catchUp();
        return;
      }
      early.push(ev);
    }, ctl.signal).then(
      () => {
        if (ctl.signal.aborted) return;
        reconnectRef.current = window.setTimeout(() => follow(id), RECONNECT_MS);
      },
      (error: unknown) => {
        if (ctl.signal.aborted) return;
        if (error instanceof ApiError && error.status === 409) {
          setHistoryProblem(error.message);
          setHistoryLoading(false);
          return;
        }
        reconnectRef.current = window.setTimeout(() => follow(id), RECONNECT_MS);
      },
    );
  }, [onLive]);

  useEffect(() => {
    historySession.current = sessionId;
    reset(sessionId);
    setBefore(undefined);
    setHistoryProblem(null);
    if (sessionId) follow(sessionId);
    else setHistoryLoading(false);
    return () => {
      historyAbort.current?.abort();
      // Keep this chat's view for switching back.
      if (sessionId) views.set(sessionId, { items: latestItems.current, queue: latestQueue.current });
    };
  }, [sessionId, reset, follow]);

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

  return { items: historySession.current === sessionId ? items : (sessionId ? views.get(sessionId)?.items : undefined) ?? [], status, queue, clearQueue: () => setQueue([]), problem, send, answer, setAlwaysApplies, stop, stopping, compacting, notice, dismissProblem, reset, historyLoading, historyProblem, hasOlder: !!before, loadOlder: () => void loadHistory(before), retryHistory: () => { if (historyCursor.current || !sessionId) void loadHistory(historyCursor.current); else follow(sessionId); } };
}

/** mergeFollowed joins a followed chat's latest history page with the feed
 *  events that arrived before it: the turns the feed carries are rebuilt from
 *  it and their history copies left out, so nothing shows twice or goes
 *  missing. running is the feed's state when it was followed. */
export function mergeFollowed(page: Item[], early: ServerEvent[], running: boolean): { items: Item[]; liveTurns: Set<string>; status: Status; problem?: string } {
  const liveTurns = new Set<string>();
  let problem: string | undefined;
  let status: Status = running ? "streaming" : "idle";
  for (const ev of early) {
    if (ev.type === "user_message") liveTurns.add(ev.turnId);
    if (ev.type === "turn_started") liveTurns.add(ev.id);
    if (ev.type === "user_message" || ev.type === "turn_started") status = "streaming";
    if (ev.type === "turn_done" && status === "streaming") status = "idle";
    if (ev.type === "error" && !isOwnerStop(ev)) problem = ev.message;
    if (ev.type === "error" && ev.code === "turn_refused") status = "error";
  }
  const settled = page.filter((item) => !(item.turn && liveTurns.has(item.turn.id)));
  return { items: early.reduce((acc, ev) => reduce(acc, ev), settled), liveTurns, status, problem };
}

/** describe turns a thrown value into banner text. The two typed failures get
 *  their own message; anything else (a network throw) falls back to its own. */
function describe(err: unknown): string {
  if (err instanceof StreamTruncated) return err.message;
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "something went wrong";
}
