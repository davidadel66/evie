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
// summary is the completed header's phrase, "one|many" with # for the count.
const actions: Record<string, { kind: ActionKind; action: string; done: string; subject: string[]; summary: string }> = {
  read_file: {kind: "read", action: "Read file", done: "Read file", subject: ["path"], summary: "read a file|read # files"},
  edit_file: {kind: "edit", action: "Edit file", done: "Edited file", subject: ["path"], summary: "edited a file|edited # files"},
  bash: {kind: "command", action: "Command", done: "Ran command", subject: ["command"], summary: "ran a command|ran # commands"},
  web_search: {kind: "search", action: "Search web", done: "Searched web", subject: ["query"], summary: "searched the web"},
  web_fetch: {kind: "read", action: "Fetch page", done: "Fetched page", subject: ["url"], summary: "read a page|read # pages"},
  query_db: {kind: "data", action: "Query data", done: "Queried data", subject: ["query", "sql"], summary: "queried data"},
  edit_db: {kind: "edit", action: "Edit data", done: "Edited data", subject: ["query", "sql"], summary: "edited data"},
  memory_search: {kind: "memory", action: "Search memory", done: "Searched memory", subject: ["query"], summary: "checked memory"},
  memory_list_objects: {kind: "memory", action: "List memory records", done: "Listed memory records", subject: [], summary: "checked memory"},
  memory_inspect_object: {kind: "memory", action: "Read memory record", done: "Read memory record", subject: [], summary: "checked memory"},
  memory_query_claims: {kind: "memory", action: "Query memory claims", done: "Queried memory claims", subject: [], summary: "checked memory"},
  memory_traverse: {kind: "memory", action: "Follow memory links", done: "Followed memory links", subject: [], summary: "checked memory"},
  memory_search_conversations: {kind: "memory", action: "Search conversations", done: "Searched conversations", subject: ["query"], summary: "checked memory"},
  memory_expand_conversation: {kind: "memory", action: "Read conversation context", done: "Read conversation context", subject: [], summary: "checked memory"},
  delegate_research: {kind: "tool", action: "Research with subagents", done: "Research with subagents", subject: [], summary: "used subagents"},
  delegate_explore: {kind: "tool", action: "Explore with subagents", done: "Explore with subagents", subject: [], summary: "used subagents"},
  delegate_build: {kind: "tool", action: "Build with a subagent", done: "Build with a subagent", subject: [], summary: "used subagents"},
  continue_research: {kind: "tool", action: "Continue a subagent", done: "Continue a subagent", subject: ["execution_id"], summary: "used subagents"},
};

function actionOf(name: string) {
  return actions[name] ?? {kind: "tool" as const, action: name.replace(/_/g, " "), done: name.replace(/_/g, " "), subject: [], summary: "used a tool|used # tools"};
}

export function toolLabel(tool: ToolItem) {
  const spec = actionOf(tool.name);
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

// A completed turn's heading: each kind of successful action once, in order
// of first use, such as "Searched the web, read 2 pages". Actions that need
// attention are left to the problem summary. Null when none succeeded.
export function actionSummary(items: Item[]): string | null {
  const counts = new Map<string, number>();
  for (const item of items) {
    if (item.kind !== "tool" || item.result === undefined || needsAttention(item)) continue;
    const phrase = actionOf(item.name).summary;
    counts.set(phrase, (counts.get(phrase) ?? 0) + 1);
  }
  if (counts.size === 0) return null;
  return capitalize([...counts].map(([phrase, count]) => {
    const [one, many = one] = phrase.split("|");
    return count === 1 ? one : many.replace("#", String(count));
  }).join(", "));
}

function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

// Streamed text can't be told from the answer until it commits. Short text
// with no line break is often a progress message ("I'll look at the card's
// transactions."), so the live line titles it instead of showing it.
export function mayBeProgress(text: string): boolean {
  const trimmed = text.trim();
  return trimmed.length <= 240 && !trimmed.includes("\n");
}

export function elapsedLabel(start?: number, end?: number): string | null {
  if (!start || !end || end < start) return null;
  const seconds = Math.round((end - start) / 1000);
  if (seconds < 1) return "<1s";
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

// A thought's title: its latest bold heading when the model writes them,
// else its latest intent sentence written as an action ("Let me check the
// fees." becomes "Checking the fees"), else its latest complete sentence.
// Null until one completes, so a half-written sentence never shows. more
// says whether the thought has text beyond the sentence its title came from.
export function describeThought(text: string, streaming: boolean): {title: string | null; more: boolean} {
  const whole = plainThought(text);
  const titled = (title: string | null, source: string | null) => ({title, more: source === null ? whole !== "" : whole !== source});
  const headings = [...text.matchAll(/^\s*\*\*(.+?)\*\*\s*$/gm)];
  if (headings.length > 0) {
    const heading = plainThought(headings[headings.length - 1][1]);
    return titled(heading, heading);
  }
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
  for (let i = plain.length - 1; i >= 0; i--) {
    // An initial such as "Parcel B." can join two sentences; an intent
    // opener after a full stop still starts one.
    const parts = plain[i].split(intentStart);
    for (let j = parts.length - 1; j >= 0; j--) {
      const action = asAction(parts[j]);
      if (action) return titled(action, parts.length === 1 ? plain[i] : parts[j]);
    }
  }
  const latest = plain[plain.length - 1] ?? null;
  return titled(latest, latest);
}

export function thoughtTitle(text: string, streaming: boolean): string | null {
  return describeThought(text, streaming).title;
}

// Openers such as "Okay," before the intent, and hedges before its verb,
// carry nothing a title needs. A negated intent ("I should just not…") is
// not an action, nor is "Let me know", which asks David; neither is
// rewritten.
const openers = "(?:(?:now|okay|ok|so|alright|first|next|then|actually|wait|right|great)[,.!]?\\s+)*";
const leads = "(?:let me|let[’']s|let us|i need to|i[’']ll|i will|i should|i[’']m going to|i am going to|i want to|i have to|i must)";
const hedges = "(?:(?:also|just|now|first|then|still|again|maybe|perhaps|probably|quickly|carefully|really|actually|briefly|finally|definitely|simply|further|properly|manually)\\s+)*(?:(?:need|have|try|want) to\\s+)?";
const refused = new RegExp(`^${openers}(?:${leads}\\s+(?:\\S+\\s+){0,3}?(?:not|never)\\b|let me know\\b)`, "i");
const intent = new RegExp(`^${openers}(?:${leads}\\s+${hedges}([a-z]+(?:-[a-z]+)*)|(?:i[’']m|i am)\\s+(?:(?:now|also|still|just)\\s+)*([a-z]+ing))\\b(.*)$`, "i");
const intentStart = new RegExp(`(?<=[.!?…]["'”)]?)\\s+(?=${openers}(?:${leads}|i[’']m|i am)\\b)`, "i");

// A trailing reason or follow-on clause is cut: the title says what, not
// why. En dashes cut only with spaces, so "2024–2025" stays whole.
const aside = /\s*(?:;|\s[—–]\s|—|\s-{1,2}\s|,\s*(?:since|because|as|so that|so|which|but|then|and then|to see|to make sure|to confirm|in case|before|after|while|given)\b)/i;

function asAction(sentence: string): string | null {
  const match = refused.test(sentence) ? null : intent.exec(sentence);
  if (!match) return null;
  const verb = match[1] ? gerund(match[1].toLowerCase()) : match[2].toLowerCase();
  const rest = match[3].split(aside)[0].replace(/[.!?…:]+["'”)]?$/, "");
  return capitalize(`${verb}${rest}`.trim());
}

// "see" reads better as what the model does: "Let me see if…" is checking.
const gerundOverrides: Record<string, string> = {see: "checking", be: "being", lie: "lying", die: "dying", tie: "tying"};
// Stressed final syllables double their consonant; one-syllable words are
// caught by the pattern below. A prefix alone can't tell "rerun" from
// "render", so longer words are listed.
const doubledGerunds = new Set(["begin", "commit", "submit", "admit", "permit", "omit", "emit", "remit", "occur", "recur", "refer", "prefer", "defer", "deter", "infer", "regret", "forget", "upset", "reset", "control", "patrol", "compel", "propel", "expel", "repel", "rebel", "equip", "quit", "transfer", "rebut", "format", "program", "debug", "rerun", "refit", "remap", "replan", "recap", "unwrap", "unzip", "unpin", "uncap", "output", "input", "embed", "overlap"]);
const doubles = /^[^aeiou]*[aeiou][^aeiouwxy]$/;

function gerund(verb: string): string {
  const dash = verb.lastIndexOf("-");
  if (dash >= 0) return `${verb.slice(0, dash + 1)}${gerund(verb.slice(dash + 1))}`;
  if (gerundOverrides[verb]) return gerundOverrides[verb];
  if (doubledGerunds.has(verb) || doubles.test(verb)) return `${verb}${verb.slice(-1)}ing`;
  if (/(?:[^aeiouy]|u)e$/.test(verb) && verb.length > 2) return `${verb.slice(0, -1)}ing`;
  return `${verb}ing`;
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
