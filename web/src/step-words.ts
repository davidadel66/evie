// What evie's work says in plain words, apart from any screen, for the
// session view's work log and the workspace's side chat alike: each tool
// call as a sentence, such as "Read README.md" or "Ran bun test", or while
// it runs what it is doing, "Running bun test"; the kind of thing it did,
// which an icon shows; whether it failed; and a run of calls summed up, as
// T3 Code's work log sums one up: a run of one call says it, and a longer
// run counts its calls by kind, such as "Read 2 files and ran 1 command"
// (the rules adapted from T3 Code, MIT licence, notice in
// LICENSES/t3code-MIT.txt).

// A call as the page knows it: the tool's name; its input in short, on one
// line, such as the file's path or the command; and, when the page has
// it, its input whole, as read from what the model wrote (`fields`).
export type CallSaid = { name: string; input: string; fields?: Readonly<Record<string, unknown>> }

// What a call did, which its icon shows.
export type StepKind = "read" | "edit" | "command" | "search" | "web" | "agent" | "question" | "other"

// How long what a call worked on may be before it is cut short.
const LONG = 80

// Each tool's sentence once done and while it runs, what it names from its
// input, and what it says when that is not known, as for a call read only
// by its result. `in` adds where a search looked.
type Rule = { done: string; doing: string; names?: string; none: string; path?: true; in?: true; lower?: true }
const RULES: Record<string, Rule> = {
  read: { done: "Read", doing: "Reading", names: "path", none: "a file", path: true },
  edit: { done: "Edited", doing: "Editing", names: "path", none: "a file", path: true },
  write: { done: "Wrote", doing: "Writing", names: "path", none: "a file", path: true },
  shell: { done: "Ran", doing: "Running", names: "command", none: "a command" },
  grep: { done: "Searched for", doing: "Searching for", names: "pattern", none: "", in: true },
  glob: { done: "Looked for", doing: "Looking for", names: "pattern", none: "files", in: true },
  web_search: { done: "Searched the web for", doing: "Searching the web for", names: "query", none: "" },
  web_fetch: { done: "Read", doing: "Reading", names: "url", none: "a web page" },
  subagent: { done: "Asked a subagent to", doing: "Asking a subagent to", names: "description", none: "", lower: true },
  skill: { done: "Loaded the", doing: "Loading the", names: "name", none: "" },
}
// What is said when a search names nothing.
const NONE_SAID: Record<string, [string, string]> = {
  grep: ["Searched the code", "Searching the code"],
  web_search: ["Searched the web", "Searching the web"],
  subagent: ["Asked a subagent", "Asking a subagent"],
  skill: ["Loaded a skill", "Loading a skill"],
}
// Those whose sentence does not name what they worked on.
const PLAIN: Record<string, [string, string]> = {
  ask: ["Asked you a question", "Asking you a question"],
  subagent_send: ["Sent a subagent a message", "Sending a subagent a message"],
  job_stop: ["Stopped background work", "Stopping background work"],
  background: ["Moved a command to the background", "Moving a command to the background"],
}

// A call's sentence, once done, or with `running`, while it runs. A path
// inside the project's `folder` is given from there, as the tools take it.
export function stepWords(call: CallSaid, how: { running?: boolean; folder?: string } = {}): string {
  const tense = how.running ? 1 : 0
  if (call.name === "patch") {
    const files = patchFiles(call).map((file) => fromFolder(file, how.folder))
    const named = files.length === 0 ? "files" : files.length <= 2 ? listed(files) : `${files[0]} +${files.length - 1} more`
    return `${how.running ? "Editing" : "Edited"} ${named}`
  }
  const plain = PLAIN[call.name]
  if (plain !== undefined) return plain[tense]
  const rule = RULES[call.name]
  if (rule === undefined) return how.running ? `Using ${call.name}` : `Used ${call.name}`
  let what = targetOf(call, rule, how.folder)
  if (what === undefined) return NONE_SAID[call.name]?.[tense] ?? `${how.running ? rule.doing : rule.done} ${rule.none}`
  if (rule.lower) what = `${what.charAt(0).toLowerCase()}${what.slice(1)}`
  if (call.name === "skill") what = `${what} skill`
  const where = rule.in && typeof call.fields?.path === "string" && call.fields.path.trim() !== "" ? ` in ${fromFolder(oneLine(call.fields.path), how.folder)}` : ""
  return `${how.running ? rule.doing : rule.done} ${what}${where}`
}

// What a run says while one of its calls is being carried out, shortly, as
// T3 Code's live row does: a command by its program, "Running bun"; any
// other call by its sentence.
export function liveWords(call: CallSaid, folder?: string): string {
  if (call.name !== "shell") return stepWords(call, { running: true, ...(folder !== undefined && { folder }) })
  const command = targetOf(call, RULES.shell!, folder)
  return command === undefined ? "Running a command" : `Running ${programOf(command)}`
}

// What a call worked on, alone, as a run whose own row says what is being
// done lists the call that does it, as T3 Code lists a running command by
// the command alone; or its sentence, when it names nothing.
export function stepTarget(call: CallSaid, folder?: string): string {
  const rule = RULES[call.name]
  if (call.name === "patch") return patchFiles(call).map((file) => fromFolder(file, folder)).join(", ") || stepWords(call, { running: true })
  const what = rule === undefined ? undefined : targetOf(call, rule, folder)
  return what ?? stepWords(call, { running: true, ...(folder !== undefined && { folder }) })
}

// The kind of thing a call of a tool does, by the tool's name.
export function stepKind(name: string): StepKind {
  switch (name) {
    case "read":
    case "skill":
      return "read"
    case "edit":
    case "write":
    case "patch":
      return "edit"
    case "shell":
    case "background":
      return "command"
    case "grep":
    case "glob":
      return "search"
    case "web_search":
    case "web_fetch":
      return "web"
    case "subagent":
    case "subagent_send":
      return "agent"
    case "ask":
      return "question"
    default:
      return "other"
  }
}

// Whether a call failed, as its result, written for the model, says: a
// command that ended badly or could not start, a call the owner or a rule
// refused, or a tool that broke. A call stopped with its turn did not fail.
export function stepFailed(result: string | undefined): boolean {
  if (result === undefined) return false
  return FAILED.some((said) => said.test(result))
}
const FAILED = [
  /\[Exit status: [1-9]\d*\]\s*$/,
  /\[Ended by signal [^\]]+\]\s*$/,
  /\[The command could not be started: /,
  /\[Stopped: the command ran past its timeout/,
  /^Not allowed: /,
  /^The owner said no to this/,
  /^Not done: this needs the owner's say-so/,
  /This is a bug in evie or a plugin, not a problem with the input\.\s*$/,
]

// The kinds a run's calls are counted by, each with how a count of it is
// said and how much it matters: running commands, changing files and
// asking subagents most, a tool evie does not know least, as T3 Code ranks
// them.
type Counted = "read" | "change" | "command" | "code" | "web" | "page" | "subagent" | "other"
const COUNTED: Record<Counted, { words: (count: number) => string; rank: number }> = {
  command: { words: (count) => `Ran ${count} ${count === 1 ? "command" : "commands"}`, rank: 0 },
  change: { words: (count) => `Changed ${count} ${count === 1 ? "file" : "files"}`, rank: 0 },
  subagent: { words: (count) => `Asked ${count} ${count === 1 ? "subagent" : "subagents"}`, rank: 0 },
  read: { words: (count) => `Read ${count} ${count === 1 ? "file" : "files"}`, rank: 1 },
  code: { words: (count) => `Searched code ${count} ${count === 1 ? "time" : "times"}`, rank: 1 },
  web: { words: (count) => `Searched the web ${count} ${count === 1 ? "time" : "times"}`, rank: 1 },
  page: { words: (count) => `Read ${count} ${count === 1 ? "web page" : "web pages"}`, rank: 1 },
  other: { words: (count) => `Used ${count} ${count === 1 ? "tool" : "tools"}`, rank: 2 },
}
const COUNTED_AS: Record<string, Counted> = {
  read: "read",
  edit: "change",
  write: "change",
  patch: "change",
  shell: "command",
  background: "command",
  grep: "code",
  glob: "code",
  web_search: "web",
  web_fetch: "page",
  subagent: "subagent",
}

// A run of calls in one sentence: the call itself, for a run of one;
// otherwise the two kinds that matter most, in the order the run first
// used them, each counted, and the other calls as other actions. Changes
// count the files changed, each once, a patch each file it names.
export function runWords(calls: readonly CallSaid[]): string {
  if (calls.length === 1) return stepWords(calls[0]!)
  const kinds = new Map<Counted, { calls: number; files: Set<string>; unnamed: number }>()
  for (const call of calls) {
    const counted = COUNTED_AS[call.name] ?? "other"
    const kind = kinds.get(counted) ?? { calls: 0, files: new Set<string>(), unnamed: 0 }
    kind.calls += 1
    if (counted === "change") {
      const files = call.name === "patch" ? patchFiles(call) : [targetOf(call, RULES[call.name]!)].flatMap((file) => (file === undefined ? [] : [file]))
      if (files.length === 0) kind.unnamed += 1
      for (const file of files) kind.files.add(file)
    }
    kinds.set(counted, kind)
  }
  const all = [...kinds].map(([counted, kind], at) => ({ counted, at, calls: kind.calls, count: counted === "change" ? kind.files.size + kind.unnamed : kind.calls }))
  const chosen = [...all].sort((a, b) => COUNTED[a.counted].rank - COUNTED[b.counted].rank || a.at - b.at).slice(0, 2).sort((a, b) => a.at - b.at)
  const rest = calls.length - chosen.reduce((sum, each) => sum + each.calls, 0)
  const clauses = [...chosen.map((each) => COUNTED[each.counted].words(each.count)), ...(rest > 0 ? [`Performed ${rest} other ${rest === 1 ? "action" : "actions"}`] : [])]
  return listed(clauses.map((clause, at) => (at === 0 ? clause : `${clause.charAt(0).toLowerCase()}${clause.slice(1)}`)))
}

// What a call names from its input, on one line, cut short when long: its
// own field of the input whole, when the page has it, or else the input in
// short. A path is given from the project's folder.
function targetOf(call: CallSaid, rule: Rule, folder?: string): string | undefined {
  const named = rule.names === undefined ? undefined : call.fields?.[rule.names]
  const given = typeof named === "string" ? named : call.input
  if (typeof given !== "string" || given.trim() === "") return undefined
  const what = rule.path ? fromFolder(oneLine(given), folder) : oneLine(given)
  return what.length > LONG ? `${what.slice(0, LONG)}…` : what
}

// The files a patch names, as the patch tool reads them, from the patch
// whole, line by line, or from it in short, on one line.
function patchFiles(call: CallSaid): string[] {
  const whole = typeof call.fields?.patchText === "string" ? call.fields.patchText : undefined
  if (whole !== undefined) return [...whole.matchAll(/^[ \t]*\*\*\* (?:Add File|Update File|Delete File):[ \t]*(.+?)[ \t]*$/gm)].map((found) => found[1]!)
  return [...call.input.matchAll(/\*\*\* (?:Add|Update|Delete) File: (\S+)/g)].map((found) => found[1]!)
}

// A command's program, as T3 Code names a running one: its first word,
// past any settings given before it, such as `CI=1`.
function programOf(command: string): string {
  const words = command.trim().split(/\s+/)
  const program = words.find((word) => !/^[A-Za-z_][A-Za-z0-9_]*=/.test(word)) ?? words[0] ?? command
  return program.replace(/^.*\//, "")
}

// The fields of an input as the model wrote it, or none when it is not a
// JSON object.
export function fieldsOf(written: string): Record<string, unknown> {
  try {
    const input: unknown = JSON.parse(written)
    return typeof input === "object" && input !== null && !Array.isArray(input) ? (input as Record<string, unknown>) : {}
  } catch {
    return {}
  }
}

const oneLine = (text: string) => text.replace(/\s+/g, " ").trim()

// A path from the project's folder when it is inside it, as the tools
// name files back to the model; otherwise as given.
function fromFolder(path: string, folder: string | undefined): string {
  if (folder === undefined || !path.startsWith("/")) return path
  const top = folder.replace(/\/+$/, "")
  return path.startsWith(`${top}/`) ? path.slice(top.length + 1) : path
}

// Words in a list: "a", "a and b", "a, b and c".
export const listed = (words: readonly string[]) => (words.length <= 1 ? (words[0] ?? "") : `${words.slice(0, -1).join(", ")} and ${words.at(-1)}`)
