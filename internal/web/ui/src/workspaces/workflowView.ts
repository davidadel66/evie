import type { WorkflowDefinition, WorkflowRun, WorkflowSchedule, WorkflowScheduleStatus, WorkflowStepOutline } from "../api/workflows";
import { runTone, type RunTone } from "./runView";

// The Workflows tab and pop-up (workspace-activity.spec.md story 3): when a
// workflow runs, its status, its recent runs at a glance, and its YAML.

const weekdays = ["Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays"];

/** When a workflow runs, in words: "On request", "Daily 09:00 Detroit", "Mondays 09:30 Detroit", "Weekdays 08:00 UTC". Other schedules show their cron. */
export function scheduleText(schedule: WorkflowSchedule | undefined): string {
  if (!schedule) return "On request";
  const zone = schedule.timezone.split("/").pop()!.replaceAll("_", " ");
  const [minute, hour, day, month, weekday, ...rest] = schedule.cron.trim().split(/\s+/);
  const plain = /^\d{1,2}$/;
  if (rest.length > 0 || !plain.test(minute ?? "") || !plain.test(hour ?? "") || day !== "*" || month !== "*") return `${schedule.cron} (${zone})`;
  const time = `${hour.padStart(2, "0")}:${minute.padStart(2, "0")} ${zone}`;
  if (weekday === "*") return `Daily ${time}`;
  if (weekday === "1-5") return `Weekdays ${time}`;
  if (/^[0-6]$/.test(weekday)) return `${weekdays[Number(weekday)]} ${time}`;
  return `${schedule.cron} (${zone})`;
}

export type WorkflowStatusView = { label: string; tone: "amber" | "muted" | "danger" | "faint" };

/** A workflow's status for the owner: whether it runs, and whether a version waits for approval. */
export function workflowStatus(definition: WorkflowDefinition, schedule?: WorkflowScheduleStatus): WorkflowStatusView {
  switch (definition.status) {
  case "proposed": return {label: "Needs approval", tone: "amber"};
  case "blocked": return {label: "Blocked", tone: "danger"};
  case "inactive": return {label: "Inactive", tone: "faint"};
  }
  if (definition.pending > 0) return {label: "Update needs approval", tone: "amber"};
  if (schedule?.paused) return {label: "Paused", tone: "muted"};
  return {label: "Active", tone: "muted"};
}

/** Whether the pop-up offers the approval card: a waiting proposal, or a definition that is not running and can be approved again. */
export function needsReview(definition: WorkflowDefinition): boolean {
  return definition.pending > 0 || definition.status === "blocked" || definition.status === "inactive";
}

/** A workflow's last runs, oldest first, as tones for its dots. */
export function recentRuns(runs: WorkflowRun[], workflow: string, count = 10): WorkflowRun[] {
  return runs.filter((run) => run.workflow === workflow).slice(0, count);
}

export function runDots(runs: WorkflowRun[]): RunTone[] {
  return [...runs].reverse().map(runTone);
}

/** Where a step leads and how it retries, as short notes. */
export function stepNotes(step: WorkflowStepOutline): string[] {
  const notes: string[] = [];
  if (step.next?.length) notes.push(step.kind === "branch" ? `then ${step.next.join(" or ")}` : `then ${step.next.join(", ")}`);
  if (step.max_attempts && step.max_attempts > 1) notes.push(`up to ${step.max_attempts} attempts`);
  return notes;
}

/** A step's line range: "L54–59", or "L54" for one line. */
export function lineRange(step: WorkflowStepOutline): string {
  return step.first_line === step.last_line ? `L${step.first_line}` : `L${step.first_line}–${step.last_line}`;
}

/** One workflow.yaml line split for highlighting: indent, key, the rest, and a trailing comment. */
export type YamlLine = { indent: string; dash: string; key: string; rest: string; comment: string; depth: number };

export function yamlLine(raw: string): YamlLine {
  if (/^\s*#/.test(raw)) return {indent: "", dash: "", key: "", rest: "", comment: raw, depth: 0};
  const match = /^(\s*)(- )?([\w.-]+)(:(?:\s|$).*|:)$/.exec(raw);
  let rest = match ? match[4] : raw, comment = "";
  const hash = rest.search(/\s#/);
  if (hash >= 0) { comment = rest.slice(hash); rest = rest.slice(0, hash); }
  if (!match) return {indent: "", dash: "", key: "", rest, comment, depth: 0};
  return {indent: match[1], dash: match[2] ?? "", key: match[3], rest, comment, depth: match[1].length + (match[2] ? 2 : 0)};
}
