// One compact block above the review inbox: is the background memory pass
// on (and if not, why), which model it uses, today's spend against its caps,
// and how the latest run went. It never shows message or memory text.

import { useEffect, useState } from "react";
import { memoryBackgroundAPI, type MemoryBackgroundAPI, type MemoryBackgroundRun, type MemoryBackgroundStatus } from "../api/memoryBackground";

const stateLabels: Record<MemoryBackgroundStatus["state"], string> = { on: "On", disabled: "Off", paused: "Paused" };

function tokens(value: number): string {
  return value.toLocaleString("en-US");
}

function dollars(micro: number): string {
  const value = micro / 1_000_000;
  return `$${value > 0 && value < 0.01 ? value.toFixed(4) : value.toFixed(2)}`;
}

function when(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
}

function runSummary(run: MemoryBackgroundRun): string {
  const proposals = `${run.queued} ${run.queued === 1 ? "proposal" : "proposals"}`;
  switch (run.status) {
    case "succeeded": return `${proposals} from ${run.messages} ${run.messages === 1 ? "message" : "messages"}`;
    case "deferred": return "waited for a running conversation; it will try again";
    case "skipped": return "skipped a batch the model kept answering badly";
    case "superseded": return "another Evie process covered the same messages first";
    default: return "failed; it will retry";
  }
}

export function BackgroundPassStatus({ api = memoryBackgroundAPI, refreshKey }: { api?: MemoryBackgroundAPI; refreshKey?: unknown }) {
  const [status, setStatus] = useState<MemoryBackgroundStatus>();
  const [problem, setProblem] = useState("");
  useEffect(() => {
    let live = true;
    api.status().then((value) => { if (live) { setStatus(value); setProblem(""); } },
      (error: unknown) => { if (live) setProblem(error instanceof Error ? error.message : "The background pass status is unavailable."); });
    return () => { live = false; };
  }, [api, refreshKey]);
  if (!status && !problem) return null;
  return <BackgroundPassStatusView status={status} problem={problem} />;
}

export function BackgroundPassStatusView({ status, problem }: { status?: MemoryBackgroundStatus; problem: string }) {
  if (!status) {
    return problem ? <section aria-label="Background memory pass" className="text-muted-text border-hair flex-none border-b px-5 py-3 text-xs">Background memory pass: {problem}</section> : null;
  }
  const today = status.progress.today;
  const spentTokens = today.input_tokens + today.output_tokens;
  const lastRun = status.progress.last_run;
  const lastError = status.progress.last_error;
  return <section aria-label="Background memory pass" className="text-body border-hair flex-none border-b px-5 py-3 text-xs">
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
      <h2 className="text-ink text-sm font-medium">Background memory pass</h2>
      <span className={status.state === "on" ? "text-ok-ink" : "text-amber-ink"}>{stateLabels[status.state]}</span>
      {status.model && <span className="text-muted-text font-mono">{status.model}</span>}
    </div>
    {status.reason && <p className="text-amber-ink mt-1">{status.reason}</p>}
    {status.enabled && <p className="text-muted-text mt-1">
      Reads only your own messages, after {status.idle_minutes} minutes idle or every {status.turn_trigger} messages, and queues what it finds here. Nothing is saved until you approve it.
    </p>}
    <p className="text-muted-text mt-1">
      Today: {tokens(spentTokens)} of {tokens(status.daily_token_cap)} tokens · {dollars(today.cost_micro_usd)} of {dollars(status.daily_micro_usd_cap)}
      {status.progress.pending_proposals > 0 && ` · ${status.progress.pending_proposals} waiting for review`}
    </p>
    {lastRun && <p className="text-muted-text mt-1">Last run {when(lastRun.finished_at)}: {runSummary(lastRun)}</p>}
    {lastError && lastError.run_id !== lastRun?.run_id && <p className="text-muted-text mt-1">Last problem {when(lastError.finished_at)}: {lastError.error || runSummary(lastError)}</p>}
    {lastError && lastError.run_id === lastRun?.run_id && lastError.error && <p className="text-amber-ink mt-1">Last problem: {lastError.error}</p>}
    {status.progress.sessions.length > 0 && <details className="mt-1">
      <summary className="text-muted-text hover:text-body cursor-pointer">Conversations</summary>
      <ul className="text-muted-text mt-1 space-y-[2px]">
        {status.progress.sessions.map((session) => <li key={session.session_id}>
          {session.title || "Untitled conversation"} · read through event {session.cursor}
          {session.pending_owner_messages > 0 && ` · ${session.pending_owner_messages} new ${session.pending_owner_messages === 1 ? "message" : "messages"}`}
          {session.last_status && ` · last run ${session.last_status}`}
          {session.failures > 0 && ` · ${session.failures} ${session.failures === 1 ? "failure" : "failures"}, next try ${session.next_attempt_at ? when(session.next_attempt_at) : "soon"}`}
        </li>)}
      </ul>
    </details>}
  </section>;
}
