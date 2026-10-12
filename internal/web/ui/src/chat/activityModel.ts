import type { ActivityTurn } from "../store/events";
import type { Item } from "../store/reducer";
import { readMemoryToolResult } from "../artifacts/memoryToolResult";

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
  if (item.kind === "tool") {
    const result = readMemoryToolResult(item);
    if (result?.kind === "search" && (result.truncated || (result.status !== "success" && result.status !== "empty"))) return true;
  }
  if (item.kind === "memory") {
    const memory = item.memory;
    return (memory.status !== "success" && memory.status !== "empty") || memory.requestStatus === "interrupted" ||
      !!(memory.historicalCount || memory.retiredCount || memory.conflictCount);
  }
  return item.kind === "notice" || (item.kind === "assistant" && !!item.discarded) ||
    (item.kind === "tool" && (!!item.isErr || item.approval?.state === "pending" || item.approval?.state === "declined" || item.approval?.state === "expired"));
}

export function awaitingApproval(item: Item): boolean {
  return item.kind === "tool" && item.approval?.state === "pending" && item.result === undefined;
}

// Collapsed activity reports settled tool problems as one line; the rows stay
// in the expanded trace and pending approvals keep their own actionable rows.
export function problemSummary(items: Item[]): string | null {
  const tools = items.filter((item): item is ToolItem => item.kind === "tool" && needsAttention(item) && !awaitingApproval(item));
  if (tools.length === 0) return null;
  const failed = tools.every((tool) => tool.isErr && tool.approval?.state !== "declined" && tool.approval?.state !== "expired");
  const one = tools.length === 1;
  return `${tools.length} ${one ? "action" : "actions"} ${failed ? "failed" : one ? "needs attention" : "need attention"}`;
}

type ActionKind ="read" | "edit" | "command" | "search" | "data" | "memory" | "tool";
const actions: Record<string, { kind: ActionKind; action: string; done: string; subject: string[] }> = {
  read_file: {kind: "read", action: "Read file", done: "Read file", subject: ["path"]},
  edit_file: {kind: "edit", action: "Edit file", done: "Edited file", subject: ["path"]},
  bash: {kind: "command", action: "Command", done: "Ran command", subject: ["command"]},
  web_search: {kind: "search", action: "Search web", done: "Searched web", subject: ["query"]},
  web_fetch: {kind: "read", action: "Fetch page", done: "Fetched page", subject: ["url"]},
  query_db: {kind: "data", action: "Query data", done: "Queried data", subject: ["query", "sql"]},
  edit_db: {kind: "edit", action: "Edit data", done: "Edited data", subject: ["query", "sql"]},
  memory_search: {kind: "memory", action: "Search memory", done: "Searched memory", subject: ["query"]},
  memory_list_objects: {kind: "memory", action: "List memory records", done: "Listed memory records", subject: []},
  memory_inspect_object: {kind: "memory", action: "Read memory record", done: "Read memory record", subject: []},
  memory_query_claims: {kind: "memory", action: "Query memory claims", done: "Queried memory claims", subject: []},
  memory_traverse: {kind: "memory", action: "Follow memory links", done: "Followed memory links", subject: []},
  memory_search_conversations: {kind: "memory", action: "Search conversations", done: "Searched conversations", subject: ["query"]},
  memory_expand_conversation: {kind: "memory", action: "Read conversation context", done: "Read conversation context", subject: []},
  delegate_research: {kind: "tool", action: "Research with subagents", done: "Research with subagents", subject: []},
  delegate_explore: {kind: "tool", action: "Explore with subagents", done: "Explore with subagents", subject: []},
  delegate_build: {kind: "tool", action: "Build with a subagent", done: "Build with a subagent", subject: []},
  continue_research: {kind: "tool", action: "Continue a subagent", done: "Continue a subagent", subject: ["execution_id"]},
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
  const memoryResult = readMemoryToolResult(tool);
  const searchProblem = memoryResult?.kind === "search" && memoryResult.status !== "success" && memoryResult.status !== "empty"
    ? ({partial: "partially completed", failed: "failed", unavailable: "unavailable", cancelled: "cancelled", exhausted: "budget exhausted"} as Record<string, string>)[memoryResult.status] ?? "outcome unknown" : undefined;
  let label: string;
  if (tool.approval?.state === "declined") label = `${spec.action} declined`;
  else if (tool.approval?.state === "expired") label = `${spec.action} approval expired`;
  else if (tool.approval?.state === "pending" && tool.result === undefined) label = `${spec.action} awaiting approval`;
  else if (tool.isErr) label = `${spec.action} failed`;
  else if (searchProblem) label = `${spec.action} ${searchProblem}`;
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
      case "memory": return "Checked memory";
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

// The live line's view of reasoning: the latest bold heading when the model
// writes them, else its latest complete sentence. Null until one completes,
// so a half-written sentence never shows.
export function currentThought(text: string, streaming: boolean): string | null {
  const headings = [...text.matchAll(/^\s*\*\*(.+?)\*\*\s*$/gm)];
  if (headings.length > 0) return plainThought(headings[headings.length - 1][1]);
  const sentences: string[] = [];
  for (const line of text.split(/\n+/)) {
    // "Dr. Smith" and "the U.S. Census" stay one sentence.
    let current = "";
    for (const part of line.split(/(?<=[.!?…]["'”)]?)\s+(?=["'“(]?[A-Z0-9])/)) {
      current = current ? `${current} ${part}` : part;
      if (!abbreviated.test(current)) {
        sentences.push(current);
        current = "";
      }
    }
    if (current) sentences.push(current);
  }
  const plain = sentences.map(plainThought).filter(Boolean);
  if (streaming && plain.length > 0 && !endsSentence(plain[plain.length - 1])) plain.pop();
  return plain[plain.length - 1] ?? null;
}

// A period after these is not a sentence end: common abbreviations and
// initials such as "U.S.".
const abbreviated = /(?:\b(?:e\.g|i\.e|etc|vs|cf|approx|Dr|Mr|Mrs|Ms|Prof|St|No)|\b[A-Z](?:\.[A-Z])*)\.$/;

// Whether streamed text has finished its last sentence. A trailing number
// and period may still become "3.5", so it waits for more text.
function endsSentence(text: string): boolean {
  return /[.!?…:]["'”)]?$/.test(text) && !abbreviated.test(text) && !/\d\.$/.test(text);
}

function plainThought(text: string): string {
  return text.replace(/^\s*(?:[-*>#]+|\d+[.)])\s+/, "").replace(/\*\*|__|`/g, "").replace(/\s+/g, " ").trim();
}
