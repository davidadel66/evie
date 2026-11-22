// What evie's work says in plain words, apart from any screen: each tool
// call as a sentence, such as "Read README.md" or "Ran bun test", and a run
// of calls summed up, as T3 Code's work log sums one up: a run of one call
// says it, and a longer run counts its calls by kind, such as "Read 2 files
// and ran 1 command" (the rule adapted from T3 Code, MIT licence, notice in
// LICENSES/t3code-MIT.txt).

// A call as the page knows it: the tool's name, and its input in short, on
// one line, such as the file's path or the command.
export type CallSaid = { name: string; input: string }

// How long an input may be before it is cut short.
const LONG = 80

// The sentence for one call, and for one whose input is not known, as a
// call read only by its result.
const SAID: Record<string, [(input: string) => string, string]> = {
  read: [(input) => `Read ${input}`, "Read a file"],
  edit: [(input) => `Edited ${input}`, "Edited a file"],
  write: [(input) => `Wrote ${input}`, "Wrote a file"],
  shell: [(input) => `Ran ${input}`, "Ran a command"],
  grep: [(input) => `Searched for ${input}`, "Searched the code"],
  glob: [(input) => `Looked for ${input}`, "Looked for files"],
  web_search: [(input) => `Searched the web for ${input}`, "Searched the web"],
  web_fetch: [(input) => `Read ${input}`, "Read a web page"],
  subagent: [(input) => `Asked a subagent to ${input.charAt(0).toLowerCase()}${input.slice(1)}`, "Asked a subagent"],
  skill: [(input) => `Used the ${input} skill`, "Used a skill"],
}
// Those whose sentence does not name what they worked on.
const PLAIN: Record<string, string> = {
  ask: "Asked you a question",
  job_stop: "Stopped background work",
  background: "Moved a command to the background",
  subagent_send: "Wrote to a subagent",
}

export function stepWords(call: CallSaid): string {
  if (call.name === "patch") {
    const files = [...call.input.matchAll(/\*\*\* (?:Add|Update|Delete) File: (\S+)/g)].map((found) => found[1]!)
    return files.length === 0 ? "Edited files" : `Edited ${listed(files)}`
  }
  const plain = PLAIN[call.name]
  if (plain !== undefined) return plain
  const said = SAID[call.name]
  if (said === undefined) return `Used ${call.name}`
  const input = call.input.trim()
  if (input === "") return said[1]
  return said[0](input.length > LONG ? `${input.slice(0, LONG)}…` : input)
}

// The kinds a run's calls are counted by, each with how a count of it is
// said; a kind left out is another action.
type Kind = "read" | "change" | "command" | "code" | "web" | "subagent"
const KIND_OF: Record<string, Kind> = {
  read: "read",
  web_fetch: "read",
  edit: "change",
  write: "change",
  patch: "change",
  shell: "command",
  background: "command",
  grep: "code",
  glob: "code",
  web_search: "web",
  subagent: "subagent",
}
const COUNTED: Record<Kind, (count: number) => string> = {
  read: (count) => `Read ${count} ${count === 1 ? "file" : "files"}`,
  change: (count) => `Changed ${count} ${count === 1 ? "file" : "files"}`,
  command: (count) => `Ran ${count} ${count === 1 ? "command" : "commands"}`,
  code: (count) => `Searched code ${count} ${count === 1 ? "time" : "times"}`,
  web: (count) => `Searched the web ${count} ${count === 1 ? "time" : "times"}`,
  subagent: (count) => `Asked ${count} ${count === 1 ? "subagent" : "subagents"}`,
}

// A run of calls in one sentence: the call itself, for a run of one;
// otherwise the first two kinds it used, in the order it first used them,
// each counted, and any calls of other kinds as other actions.
export function runWords(calls: readonly CallSaid[]): string {
  if (calls.length === 1) return stepWords(calls[0]!)
  const counts = new Map<Kind, number>()
  for (const call of calls) {
    const kind = KIND_OF[call.name]
    if (kind !== undefined) counts.set(kind, (counts.get(kind) ?? 0) + 1)
  }
  const shown = [...counts].slice(0, 2)
  const others = calls.length - shown.reduce((sum, [, count]) => sum + count, 0)
  const clauses = [...shown.map(([kind, count]) => COUNTED[kind](count)), ...(others > 0 ? [`Performed ${others} other ${others === 1 ? "action" : "actions"}`] : [])]
  return listed(clauses.map((clause, at) => (at === 0 ? clause : `${clause.charAt(0).toLowerCase()}${clause.slice(1)}`)))
}

// Words in a list: "a", "a and b", "a, b and c".
const listed = (words: readonly string[]) => (words.length <= 1 ? (words[0] ?? "") : `${words.slice(0, -1).join(", ")} and ${words.at(-1)}`)
