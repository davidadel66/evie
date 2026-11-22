// The work log, as T3 Code writes it, apart from its screen, beside each
// call's own sentence (step-words.ts): a run of calls in one sentence that
// counts them by kind, "Read 2 files and ran 1 command"; and how long a
// turn has worked while it runs, "Working for 12s". Adapted from T3 Code's
// work log (MIT licence, notice in LICENSES/t3code-MIT.txt), for evie's
// own tools.
import { fieldsOf, stepKind } from "./step-words.ts"

// A call as the model asked for it: the tool's name, and its input as the
// model wrote it, which may not be JSON.
export type Asked = { name: string; arguments: string }

// What a run of calls did, counted by what each kind of call did, as T3
// Code's group says it: the two kinds that matter most, in the order they
// first came, and every other call as an action. Running commands and
// changing files matter most, and a tool evie does not know least.
type Counted = "read" | "edit" | "command" | "search" | "web" | "fetch" | "agent" | "question" | "other"
const COUNTED: Record<Counted, { words: (count: number) => string; rank: number }> = {
  command: { words: (count) => `Ran ${count} ${count === 1 ? "command" : "commands"}`, rank: 0 },
  edit: { words: (count) => `Changed ${count} ${count === 1 ? "file" : "files"}`, rank: 0 },
  agent: { words: (count) => `Asked ${count} ${count === 1 ? "subagent" : "subagents"}`, rank: 0 },
  read: { words: (count) => `Read ${count} ${count === 1 ? "file" : "files"}`, rank: 1 },
  search: { words: (count) => `Searched the code ${times(count)}`, rank: 1 },
  web: { words: (count) => `Searched the web ${times(count)}`, rank: 1 },
  fetch: { words: (count) => `Fetched ${count} ${count === 1 ? "page" : "pages"}`, rank: 1 },
  question: { words: (count) => `Asked ${count} ${count === 1 ? "question" : "questions"}`, rank: 1 },
  other: { words: (count) => `Used ${count} ${count === 1 ? "tool" : "tools"}`, rank: 2 },
}
const times = (count: number) => (count === 1 ? "1 time" : `${count} times`)

export function runSummary(calls: readonly Asked[]): string {
  // Each kind, in the order it first came, with how many it did. Changes
  // count the files changed, each once.
  const kinds = new Map<Counted, { count: number; files: Set<string> }>()
  for (const each of calls) {
    const counted = countedAs(each)
    const kind = kinds.get(counted) ?? { count: 0, files: new Set<string>() }
    const file = counted === "edit" ? fieldsOf(each.arguments).path : undefined
    if (typeof file === "string") {
      if (!kind.files.has(file)) kind.count += 1
      kind.files.add(file)
    } else kind.count += 1
    kinds.set(counted, kind)
  }
  const all = [...kinds].map(([counted, { count }], at) => ({ counted, count, at, calls: calls.filter((each) => countedAs(each) === counted).length }))
  const chosen = [...all].sort((a, b) => COUNTED[a.counted].rank - COUNTED[b.counted].rank || a.at - b.at).slice(0, 2).sort((a, b) => a.at - b.at)
  const rest = calls.length - chosen.reduce((sum, each) => sum + each.calls, 0)
  const parts = [...chosen.map((each) => COUNTED[each.counted].words(each.count)), ...(rest > 0 ? [`Performed ${rest} other ${rest === 1 ? "action" : "actions"}`] : [])]
  return listed(parts.map((part, at) => (at === 0 ? part : part[0]!.toLowerCase() + part.slice(1))))
}

// A call's kind as a run counts it: a page fetched apart from a search of the web.
const countedAs = (call: Asked): Counted => (call.name === "web_fetch" ? "fetch" : stepKind(call.name))

// How long a running turn has worked, in whole seconds, from its start:
// the time it has waited for the owner's answers is not work, and while a
// question waits, the time stands where it was asked.
export function workedSeconds(turn: { started: number; waited: number; waitingSince?: number }, now: number): number {
  const until = turn.waitingSince ?? now
  return Math.max(0, Math.floor((until - turn.started - turn.waited) / 1000))
}

// What a running turn's head says: how long it has worked, counting up.
export const workingFor = (seconds: number) => `Working for ${durationWords(seconds)}`

// A length of time in its two largest units: "12s", "1m 4s", "1h 2m".
export function durationWords(seconds: number): string {
  const [hours, minutes, rest] = [Math.floor(seconds / 3600), Math.floor((seconds % 3600) / 60), seconds % 60]
  const words = hours > 0 ? [`${hours}h`, minutes > 0 ? `${minutes}m` : ""] : minutes > 0 ? [`${minutes}m`, rest > 0 ? `${rest}s` : ""] : [`${rest}s`]
  return words.filter((word) => word !== "").join(" ")
}

// Words in a list: "a", "a and b", "a, b and c".
export const listed = (words: readonly string[]) => (words.length <= 1 ? (words[0] ?? "") : `${words.slice(0, -1).join(", ")} and ${words.at(-1)}`)
