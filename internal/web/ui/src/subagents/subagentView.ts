import type { Subagent, SubagentAction, SubagentState } from "../api/subagents";
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
