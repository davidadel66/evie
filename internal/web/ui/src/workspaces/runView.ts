import { modelCallSpend, workerCallSpend, workflowMapViews, type WorkflowInterruption, type WorkflowRun, type WorkflowRunDetail, type WorkflowRunState, type WorkflowStepOutline } from "../api/workflows";
import { duration } from "../ui/time";
import { compactCount } from "./workspaceActivity";

// The Runs tab and run panel (workspace-activity.spec.md story 2): what a
// run is doing, in the order its definition lists its steps, derived from
// the run record and each step's latest attempt.

/** A step as the owner reads it in a run. */
export type StepState = "done" | "running" | "waiting" | "failed" | "cancelled" | "pending" | "skipped";
export type RunStep = WorkflowStepOutline & { state: StepState };

const finished = new Set<WorkflowRunState>(["succeeded", "failed", "cancelled"]);
const needsOwner = new Set<WorkflowRunState>(["waiting_for_human", "needs_reconciliation"]);

export function isFinished(run: WorkflowRun): boolean {
  return finished.has(run.state);
}

/**
 * The definition's steps in YAML order with their state in this run: done,
 * the current one (running, or waiting on the owner), failed or cancelled
 * where the run stopped, not reached yet, or skipped by a branch once the
 * run succeeded without them.
 */
export function runSteps(run: WorkflowRun, outline: WorkflowStepOutline[], attempts: Record<string, string> = run.step_states ?? {}): RunStep[] {
  const done = isFinished(run);
  return outline.map((step) => {
    const latest = attempts[step.id];
    let state: StepState;
    if (!done && run.next_step === step.id) state = needsOwner.has(run.state) ? "waiting" : "running";
    else if (latest === "succeeded") state = "done";
    else if (latest === "failed") state = "failed";
    else if (latest === "running" || latest === "interrupted") state = done ? "cancelled" : "running";
    else state = run.state === "succeeded" ? "skipped" : "pending";
    return {...step, state};
  });
}

/** Where a run is: "step 3 of 5" while unfinished, "5 steps" after. */
export function stepProgress(run: WorkflowRun, steps: RunStep[]): string {
  const current = steps.findIndex((step) => step.state === "running" || step.state === "waiting");
  if (!isFinished(run) && current >= 0) return `step ${current + 1} of ${steps.length}`;
  return steps.length === 1 ? "1 step" : `${steps.length} steps`;
}

export type RunGroup = { label: string; tone: "amber" | "muted" | "faint"; runs: WorkflowRun[] };

/** Waiting for you, Running, Earlier: runs that need the owner on top; empty groups left out. */
export function groupRuns(runs: WorkflowRun[]): RunGroup[] {
  const groups: RunGroup[] = [
    {label: "Waiting for you", tone: "amber", runs: runs.filter((run) => needsOwner.has(run.state))},
    {label: "Running", tone: "muted", runs: runs.filter((run) => !needsOwner.has(run.state) && !finished.has(run.state))},
    {label: "Earlier", tone: "faint", runs: runs.filter((run) => finished.has(run.state))},
  ];
  return groups.filter((group) => group.runs.length > 0);
}

export type RunTone = "amber" | "teal" | "ok" | "danger" | "ghost" | "faint";

export const runStateLabel: Record<WorkflowRunState, string> = {
  queued: "Queued", running: "Running", waiting_for_human: "Waiting for you", waiting_for_dependency: "Paused",
  needs_reconciliation: "Needs you to settle an effect", cancelling: "Cancelling", cancelled: "Cancelled", failed: "Failed", succeeded: "Succeeded",
};

export function runTone(run: WorkflowRun): RunTone {
  switch (run.state) {
  case "waiting_for_human": case "needs_reconciliation": return "amber";
  case "running": case "cancelling": return "teal";
  case "succeeded": return "ok";
  case "failed": return "danger";
  case "cancelled": return "ghost";
  default: return "faint";
  }
}

/** A run's row in one line: what it asks, what it is doing, why it failed, or what it produced. */
export function runNote(run: WorkflowRun, steps: RunStep[], question?: WorkflowInterruption): string {
  const current = steps.find((step) => step.state === "running" || step.state === "waiting");
  const about = (step: RunStep | undefined) => step ? `${step.id}${step.description ? `: ${step.description}` : ""}` : run.next_step ?? "";
  switch (run.state) {
  case "waiting_for_human": return `Waiting for you at ${about(current) || question?.step || "a question"}`;
  case "needs_reconciliation": return `Needs you to settle steps.${question?.step ?? run.next_step ?? "?"}'s effect`;
  case "running": return `Running ${about(current)}`.trim();
  case "queued": return "Queued";
  case "waiting_for_dependency": return run.not_before ? `Paused until ${new Date(run.not_before).toLocaleTimeString("en-US", {hour: "numeric", minute: "2-digit"})}` : "Paused";
  case "cancelling": return "Cancelling";
  case "cancelled": return "Cancelled";
  case "failed": return run.error ?? "Failed";
  case "succeeded": return summarizeOutputs(run.outputs) || "Succeeded";
  }
}

/** How the runtime masks a sensitive run's values on the owner's surfaces. */
const masked = "[sensitive]";

/** A run's outputs in one line: its scalar values and list sizes, at most four; masked values are left out. */
export function summarizeOutputs(outputs: Record<string, unknown> | undefined): string {
  const parts: string[] = [];
  for (const [key, value] of Object.entries(outputs ?? {})) {
    if (value === masked) continue;
    if (typeof value === "number" || typeof value === "boolean" || (typeof value === "string" && value.length <= 40)) parts.push(`${key} ${value}`);
    else if (Array.isArray(value)) parts.push(`${key} ${value.length === 1 ? "1 item" : `${value.length} items`}`);
    if (parts.length === 4) break;
  }
  return parts.join(" · ");
}

/** How a run started, for its row. */
export function runTrigger(run: WorkflowRun): string {
  return {schedule: "Schedule", chat: "Chat", cli: "CLI", web: "Web"}[run.surface] ?? run.surface;
}

/** How a run started, for its panel; a chat start names the chat when it is known. */
export function runOrigin(run: WorkflowRun): string {
  switch (run.surface) {
  case "schedule": return "Started by its schedule";
  case "chat": return "Started by Evie in";
  case "cli": return "Started from the CLI";
  case "web": return "Started from the web";
  default: return `Started from ${run.surface}`;
  }
}

/** A run's duration and spend so far: model calls and worker calls, recorded or estimated. */
export function runSummary(detail: WorkflowRunDetail, steps: RunStep[], now: Date): string {
  const run = detail.run;
  const start = run.started_at ?? run.created_at;
  const end = run.finished_at ? new Date(run.finished_at) : now;
  let tokens = 0, micro = 0;
  for (const call of detail.model_calls ?? []) { const s = modelCallSpend(call); tokens += s.tokens; micro += s.microUSD; }
  for (const call of detail.workers ?? []) { const s = workerCallSpend(call); tokens += s.tokens; micro += s.microUSD; }
  const spend = tokens > 0 ? `${compactCount(tokens)} tokens · $${(micro / 1_000_000).toFixed(2)}` : "no model spend";
  return [duration(end.getTime() - new Date(start).getTime()), stepProgress(run, steps), spend].join(" · ");
}

/** The answer choices of a question whose schema allows one property with a fixed set of values. */
export function enumChoices(schema: Record<string, unknown>): { property: string; values: string[] } | undefined {
  if (schema.type !== "object" || typeof schema.properties !== "object" || schema.properties === null) return undefined;
  const properties = schema.properties as Record<string, { enum?: unknown }>;
  const keys = Object.keys(properties);
  if (keys.length !== 1) return undefined;
  const values = properties[keys[0]]?.enum;
  if (!Array.isArray(values) || values.length === 0 || values.length > 6 || !values.every((value) => typeof value === "string")) return undefined;
  return {property: keys[0], values};
}

/** A JSON value in one line for the outputs list. */
export function outputValue(value: unknown): string {
  const text = typeof value === "string" ? value : JSON.stringify(value);
  return text.length > 120 ? `${text.slice(0, 117)}…` : text;
}

/** A run detail's latest attempt state per step, map items aside, like the run list's step states. */
export function latestStates(detail: WorkflowRunDetail): Record<string, string> {
  const states: Record<string, string> = {};
  for (const attempt of [...detail.attempts].sort((a, b) => a.seq - b.seq)) {
    if (attempt.item === undefined) states[attempt.step] = attempt.state;
  }
  return states;
}

/** Each map step's progress in a run, from its latest list of items: "4 of 6 items done, 1 failed". */
export function mapProgress(detail: WorkflowRunDetail): Record<string, string> {
  const latest: Record<string, ReturnType<typeof workflowMapViews>[number]> = {};
  for (const view of workflowMapViews(detail)) {
    if (!latest[view.step] || view.checkpoint > latest[view.step].checkpoint) latest[view.step] = view;
  }
  const out: Record<string, string> = {};
  for (const [step, view] of Object.entries(latest)) {
    const count = (state: string) => view.items.filter((item) => item.state === state).length;
    const parts = [`${count("succeeded")} of ${view.items.length} ${view.items.length === 1 ? "item" : "items"} done`];
    if (count("failed")) parts.push(`${count("failed")} failed`);
    if (count("running")) parts.push(`${count("running")} running`);
    out[step] = parts.join(", ");
  }
  return out;
}

/** One workflow version a run is pinned to, for caching its outline. */
export function versionKey(run: WorkflowRun): string {
  return `${run.workflow}@${run.version}@${run.commit}`;
}

// The Runs list as one line per run (runs list amendment, 2026-11-02): a
// waiting run reads as a task, and finished runs sit under their workflow
// with a dot per run, so thirty days of a daily workflow is one row.

function capitalize(word: string): string {
  return word.charAt(0).toUpperCase() + word.slice(1);
}

/**
 * A waiting run's row as a task: the step as a verb, then the largest list
 * its question carries ("Review 12 lines"); without a list, the question's
 * first sentence; without a question, the step alone.
 */
export function runTask(run: WorkflowRun, steps: RunStep[], question?: WorkflowInterruption): string {
  const step = question?.step ?? steps.find((s) => s.state === "waiting")?.id ?? run.next_step ?? "";
  if (run.state === "needs_reconciliation") return `Settle ${step || "a step"}'s effect`;
  const verb = step ? capitalize(step.replace(/[_-]+/g, " ")) : "Answer";
  const input = question?.request.input;
  const source = input && typeof input === "object" ? (input as Record<string, unknown>) : question?.request ?? {};
  let best: [string, number] | undefined;
  for (const [key, value] of Object.entries(source)) {
    if (Array.isArray(value) && (!best || value.length > best[1])) best = [key, value.length];
  }
  if (best) return `${verb} ${best[1]} ${best[0].replace(/[_-]+/g, " ")}`;
  const sentence = question?.message.split(/(?<=[.!?])\s/)[0].trim() ?? "";
  return sentence ? `${verb}: ${sentence.length > 80 ? `${sentence.slice(0, 77)}…` : sentence}` : verb;
}

/** How long a run took, once it has started: "41s", "2m 06s". */
export function runDuration(run: WorkflowRun): string {
  if (!run.started_at || !run.finished_at) return "";
  return duration(new Date(run.finished_at).getTime() - new Date(run.started_at).getTime());
}

export type WorkflowHistory = {
  workflow: string;
  /** Newest first. */
  runs: WorkflowRun[];
  /** The workflow's recent outcomes in a phrase: "28 succeeded, 5 failed · last failed Nov 1". */
  summary: string;
  /** Oldest left, at most thirty, with the state each dot shows. */
  dots: { run: WorkflowRun; tone: RunTone; label: string }[];
};

const shortDate = (at: string) => new Date(at).toLocaleDateString("en-US", {month: "short", day: "numeric"});

/** Finished runs under their workflow, the workflow with the newest run first. */
export function historyByWorkflow(runs: WorkflowRun[]): WorkflowHistory[] {
  const byWorkflow = new Map<string, WorkflowRun[]>();
  for (const run of runs) {
    if (!isFinished(run)) continue;
    byWorkflow.set(run.workflow, [...(byWorkflow.get(run.workflow) ?? []), run]);
  }
  const histories = [...byWorkflow.entries()].map(([workflow, list]) => {
    const sorted = [...list].sort((a, b) => b.created_at.localeCompare(a.created_at));
    const count = (state: WorkflowRunState) => sorted.filter((run) => run.state === state).length;
    const parts: string[] = [];
    if (count("succeeded")) parts.push(`${count("succeeded")} succeeded`);
    if (count("failed")) parts.push(`${count("failed")} failed`);
    if (count("cancelled")) parts.push(`${count("cancelled")} cancelled`);
    const lastFailed = sorted.find((run) => run.state === "failed");
    const summary = parts.join(", ") + (lastFailed && sorted[0].state !== "failed" ? ` · last failed ${shortDate(lastFailed.created_at)}` : "");
    const dots = sorted.slice(0, 30).reverse().map((run) => ({run, tone: runTone(run), label: `${shortDate(run.created_at)} ${runStateLabel[run.state].toLowerCase()}`}));
    return {workflow, runs: sorted, summary, dots};
  });
  return histories.sort((a, b) => b.runs[0].created_at.localeCompare(a.runs[0].created_at));
}

/**
 * Whether a question accepts `{}` as its answer: an object schema with no
 * required properties and no minimum, which is how a review step says
 * "accept everything as proposed" (run detail amendment, 2026-11-02).
 */
export function acceptsEmptyAnswer(schema: Record<string, unknown>): boolean {
  if (schema.type !== undefined && schema.type !== "object") return false;
  if (Array.isArray(schema.required) && schema.required.length > 0) return false;
  if (typeof schema.minProperties === "number" && schema.minProperties > 0) return false;
  return true;
}

/** The largest list a question carries and its name, for "Open the 12 lines". */
export function questionList(question: WorkflowInterruption): { key: string; count: number } | undefined {
  const input = question.request.input;
  const source = input && typeof input === "object" ? (input as Record<string, unknown>) : question.request;
  let best: { key: string; count: number } | undefined;
  for (const [key, value] of Object.entries(source)) {
    if (Array.isArray(value) && (!best || value.length > best.count)) best = {key: key.replace(/[_-]+/g, " "), count: value.length};
  }
  return best;
}

/** A message's first sentence, for a question's one-line summary. */
export function firstSentence(text: string, limit = 160): string {
  const sentence = text.split(/(?<=[.!?])\s/)[0].trim();
  return sentence.length > limit ? `${sentence.slice(0, limit - 1)}…` : sentence;
}
