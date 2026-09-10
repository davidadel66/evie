import type { ActivityTurn } from "../store/events";
import type { Item } from "../store/reducer";

export type ToolItem = Extract<Item, { kind: "tool" }>;
type Assistant = Extract<Item, { kind: "assistant" }>;
export type ActivityGroup = {
  key: string;
  user?: Extract<Item, { kind: "user" }>;
  turn?: ActivityTurn;
  activity: Item[];
  answer: Assistant[];
  active: boolean;
};

// Projection only: event order, approvals and queue ownership stay in the
// reducer. Durable IDs let a page beginning inside a turn join its older page.
export function activityTurns(items: Item[], streaming: boolean): ActivityGroup[] {
  const groups: ActivityGroup[] = [];
  const byID = new Map<string, ActivityGroup>();
  let current: ActivityGroup | undefined;
  for (const item of items) {
    const id = item.turn?.id ?? (item.kind === "user" ? item.key : current?.key ?? `partial-${item.key}`);
    let group = byID.get(id);
    if (!group) {
      group = {key: item.kind === "user" ? item.key : id, activity: [], answer: [], active: false, turn: item.turn};
      groups.push(group);
      byID.set(id, group);
    }
    current = group;
    if (item.turn) group.turn = item.turn;
    if (item.kind === "user") group.user = item;
    else group.activity.push(item);
  }
  for (const group of groups) {
    group.active = streaming && group === groups[groups.length - 1] && group.turn?.status !== "complete" && group.turn?.status !== "incomplete";
    const last = group.activity[group.activity.length - 1];
    group.activity = group.activity.filter((item) => {
      if (item.kind === "reasoning" && !item.text.trim()) return false;
      if (item.kind !== "assistant" || item.discarded) return true;
      const legacyFinal = !group.turn && !group.active && item === last && !item.streaming && !item.phase;
      if (item.phase === "final_answer" || legacyFinal) {
        group.answer.push(item);
        return false;
      }
      return true;
    });
  }
  return groups;
}

export function needsAttention(item: Item): boolean {
  return item.kind === "memory" || item.kind === "notice" || (item.kind === "assistant" && !!item.discarded) ||
    (item.kind === "tool" && (!!item.isErr || item.approval?.state === "pending" || item.approval?.state === "declined" || item.approval?.state === "expired"));
}

type ActionKind = "read" | "edit" | "command" | "search" | "data" | "tool";
const actions: Record<string, { kind: ActionKind; action: string; done: string; subject: string[] }> = {
  read_file: {kind: "read", action: "Read file", done: "Read file", subject: ["path"]},
  edit_file: {kind: "edit", action: "Edit file", done: "Edited file", subject: ["path"]},
  bash: {kind: "command", action: "Command", done: "Ran command", subject: ["command"]},
  web_search: {kind: "search", action: "Search web", done: "Searched web", subject: ["query"]},
  web_fetch: {kind: "read", action: "Fetch page", done: "Fetched page", subject: ["url"]},
  query_db: {kind: "data", action: "Query data", done: "Queried data", subject: ["query", "sql"]},
  edit_db: {kind: "edit", action: "Edit data", done: "Edited data", subject: ["query", "sql"]},
};

export function toolLabel(tool: ToolItem) {
  const spec = actions[tool.name] ?? {kind: "tool" as const, action: tool.name.replace(/_/g, " "), done: tool.name.replace(/_/g, " "), subject: []};
  let subject = "";
  try {
    const args: unknown = JSON.parse(tool.args);
    if (args && typeof args === "object") {
      for (const key of spec.subject) {
        const value = Reflect.get(args, key);
        if (typeof value === "string") { subject = value.replace(/\s+/g, " ").trim(); break; }
      }
    }
  } catch { /* Exact malformed arguments remain available in details. */ }
  if (spec.subject[0] === "path") subject = subject.split("/").filter(Boolean).pop() ?? subject;
  let label: string;
  if (tool.approval?.state === "declined") label = `${spec.action} declined`;
  else if (tool.approval?.state === "expired") label = `${spec.action} approval expired`;
  else if (tool.approval?.state === "pending" && tool.result === undefined) label = `${spec.action} awaiting approval`;
  else if (tool.isErr) label = `${spec.action} failed`;
  else if (tool.result === undefined) label = `${spec.action} requested`;
  else label = spec.kind === "tool" ? `${spec.done} completed` : spec.done;
  return {label, subject, kind: spec.kind};
}

export function toolBatchLabel(tools: ToolItem[]): string {
  if (tools.some((tool) => tool.result === undefined)) return "Tool activity";
  const labels = new Set(tools.map((tool) => {
    switch (toolLabel(tool).kind) {
      case "read": return tool.name === "web_fetch" ? "Fetched pages" : "Read files";
      case "edit": return tool.name === "edit_db" ? "Edited data" : "Edited files";
      case "command": return "Ran commands";
      case "search": return "Searched web";
      case "data": return "Queried data";
      default: return "Used tools";
    }
  }));
  return [...labels].map((label, i) => i === 0 ? label : label.toLowerCase()).join(", ");
}

export function elapsedLabel(start?: number, end?: number): string | null {
  if (!start || !end || end < start) return null;
  const seconds = Math.round((end - start) / 1000);
  if (seconds < 1) return "<1s";
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}
