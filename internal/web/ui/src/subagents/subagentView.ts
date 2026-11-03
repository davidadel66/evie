import type { Subagent, SubagentAction, SubagentReport, SubagentSource, SubagentState } from "../api/subagents";
import { elapsedLabel, toolLabel, type ToolItem } from "../chat/activityModel";

// Presentation of delegated children (chat-activity.spec.md, subagent
// inspector amendment): pure, so the row label and the inspector agree.

const subagentTools = new Set(["delegate_research", "delegate_explore", "delegate_build", "continue_research"]);

/** Whether a chat tool row starts or continues subagents. */
export function startsSubagents(tool: ToolItem): boolean {
  return subagentTools.has(tool.name);
}

export function subagentFinished(state: SubagentState): boolean {
  return state !== "admitted" && state !== "running";
}

const stateLabels: Record<SubagentState, string> = {
  admitted: "Queued", running: "Working", succeeded: "Done", partial: "Partial report",
  failed: "Failed", cancelled: "Cancelled", interrupted: "Interrupted",
};

export function subagentStateLabel(state: SubagentState): string {
  return stateLabels[state] ?? state;
}

export function subagentStateTone(state: SubagentState): "live" | "quiet" | "done" | "attention" {
  if (state === "running") return "live";
  if (state === "admitted") return "quiet";
  return state === "succeeded" ? "done" : "attention";
}

const verbs: Record<string, string> = {research: "Researching", explore: "Exploring", build: "Building"};

/** The delegate row's label while its children work: "Researching · 2 of 4 done". */
export function subagentProgressLabel(list: Subagent[]): string | null {
  if (list.length === 0) return null;
  const done = list.filter((child) => subagentFinished(child.state)).length;
  return `${verbs[list[0].worker] ?? "Working"} · ${done} of ${list.length} done`;
}

/** A child's latest requested action as its chat row would read. */
export function subagentActionText(action: SubagentAction): string {
  const tool: ToolItem = {kind: "tool", key: "latest", id: "latest", name: action.name, args: action.args, startedAt: 0, result: action.done ? "" : undefined};
  const {label, subject} = toolLabel(tool);
  return subject ? `${label} · ${subject}` : label;
}

/** Wall time since the child started, or its whole run once it ended. */
export function subagentElapsed(child: Subagent, now: number): string | null {
  if (!child.startedAt) return null;
  return elapsedLabel(Date.parse(child.startedAt), child.endedAt ? Date.parse(child.endedAt) : now);
}

/** The worker detail's two views (artifacts.spec.md). */
export type SubagentView = "report" | "activity";

/**
 * Report once the child ended with a report; Activity while it runs or when
 * it ended without one. Before its report is read, a finished child that
 * succeeded or stopped partway is expected to have one.
 */
export function subagentDefaultView(child: Subagent, report?: SubagentReport): SubagentView {
  if (!subagentFinished(child.state)) return "activity";
  if (report) return report.text === undefined ? "activity" : "report";
  return child.state === "succeeded" || child.state === "partial" ? "report" : "activity";
}

const workerLabels: Record<string, string> = {research: "Research worker", explore: "Explore worker", build: "Build worker"};

/** What the attempt cost: "At least 550 tokens · 3 model responses". The
 *  total, else the input and output counts it has; usage that some response
 *  did not report, or missing one of those counts, is a lower bound. */
export function subagentUsageText(report: SubagentReport): string | null {
  const usage = report.usage;
  const parts = [usage?.input_tokens, usage?.output_tokens];
  const counted = parts.filter((count) => count !== undefined);
  const total = usage?.total_tokens ?? (counted.length > 0 ? counted.reduce((sum, count) => sum + count, 0) : undefined);
  const lowerBound = usage?.incomplete || (usage?.total_tokens === undefined && counted.length < parts.length);
  const tokens = total === undefined ? "Tokens not reported"
    : `${lowerBound ? "At least " : ""}${total.toLocaleString("en-US")} ${total === 1 ? "token" : "tokens"}`;
  const responses = report.modelResponses === undefined ? "" : `${report.modelResponses} ${report.modelResponses === 1 ? "model response" : "model responses"}`;
  if (!usage && !responses) return null;
  return [tokens, responses].filter(Boolean).join(" · ");
}

/** How Evie verified a source: "Fetched · cited in the report". */
export function subagentSourceText(source: SubagentSource): string {
  return `${source.fetched ? "Fetched" : "Only listed by a search"} · ${source.cited ? "cited in the report" : "not cited"}`;
}

/** Evie's line above a report: the worker kind, how it ended and why. */
export function subagentReportFacts(report: SubagentReport): string {
  return [
    workerLabels[report.worker] ?? `${report.worker} worker`,
    subagentStateLabel(report.status),
    report.reason ? `Reason: ${report.reason.replaceAll("_", " ")}` : "",
  ].filter(Boolean).join(" · ");
}
