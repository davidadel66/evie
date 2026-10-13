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

/** BackgroundPassDot is the pass as one coloured dot beside the review; its
 *  status opens from it. */
export function BackgroundPassDot({ api = memoryBackgroundAPI, refreshKey }: { api?: MemoryBackgroundAPI; refreshKey?: unknown }) {
  const [status, setStatus] = useState<MemoryBackgroundStatus>();
  const [problem, setProblem] = useState("");
  useEffect(() => {
    let live = true;
    api.status().then((value) => { if (live) { setStatus(value); setProblem(""); } },
      (error: unknown) => { if (live) setProblem(error instanceof Error ? error.message : "The background pass status is unavailable."); });
    return () => { live = false; };
  }, [api, refreshKey]);
  if (!status && !problem) return null;
  const tone = status?.state === "on" ? "bg-ok-ink" : "bg-amber-ink";
  return <details className="relative">
    <summary className="text-muted-text hover:text-body flex cursor-pointer list-none items-center gap-1.5 text-xs" title="Background memory pass">
      <span className={`h-2 w-2 rounded-full ${tone}`} />Pass {status ? stateLabels[status.state].toLowerCase() : "unknown"}
    </summary>
    <div className="border-hair-input bg-sidebar absolute right-0 top-full z-30 mt-2 w-80 overflow-hidden rounded-lg border shadow-xl">
      <BackgroundPassStatusView status={status} problem={problem} />
    </div>
  </details>;
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
    return problem ? <section aria-label="Background memory pass" className="text-muted-text border-hair flex-none border-b px-5 py-2 text-xs">Background memory pass: {problem}</section> : null;
  }
  const today = status.progress.today;
  const spentTokens = today.input_tokens + today.output_tokens;
  const lastRun = status.progress.last_run;
  const lastError = status.progress.last_error;
  const problemText = lastError && (lastError.run_id !== lastRun?.run_id || lastError.error) ? lastError.error || runSummary(lastError) : "";
  // One line at a glance; what explains or audits the pass is under Details.
  return <section aria-label="Background memory pass" className="text-muted-text border-hair flex-none border-b px-5 py-2 text-xs">
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
      <h2 className="text-body font-medium">Background memory pass</h2>
      <span className={status.state === "on" ? "text-ok-ink" : "text-amber-ink"}>{stateLabels[status.state]}</span>
      <span>· {dollars(today.cost_micro_usd)} of {dollars(status.daily_micro_usd_cap)} today</span>
      {lastRun && <span>· Last run {when(lastRun.finished_at)}: {runSummary(lastRun)}</span>}
      <details className="ml-auto">
        <summary className="hover:text-body cursor-pointer">Details</summary>
        <div className="mt-1 space-y-1">
          {status.model && <p className="font-mono">{status.model}</p>}
          {status.enabled && <p>Reads only your own messages, after {status.idle_minutes} minutes idle or every {status.turn_trigger} messages, and queues what it finds here. Nothing is saved until you approve it.</p>}
          <p>Today: {tokens(spentTokens)} of {tokens(status.daily_token_cap)} tokens{status.progress.pending_proposals > 0 && ` · ${status.progress.pending_proposals} waiting for review`}</p>
          {status.progress.sessions.length > 0 && <>
            <p>Conversations</p>
            <ul className="space-y-[2px]">
              {status.progress.sessions.map((session) => <li key={session.session_id}>
                {session.title || "Untitled conversation"} · read through event {session.cursor}
                {session.pending_owner_messages > 0 && ` · ${session.pending_owner_messages} new ${session.pending_owner_messages === 1 ? "message" : "messages"}`}
                {session.last_status && ` · last run ${session.last_status}`}
                {session.failures > 0 && ` · ${session.failures} ${session.failures === 1 ? "failure" : "failures"}, next try ${session.next_attempt_at ? when(session.next_attempt_at) : "soon"}`}
              </li>)}
            </ul>
          </>}
        </div>
      </details>
    </div>
    {status.reason && <p className="text-amber-ink mt-1">{status.reason}</p>}
    {problemText && <p className="text-amber-ink mt-1">Last problem {lastError && when(lastError.finished_at)}: {problemText}</p>}
  </section>;
}
