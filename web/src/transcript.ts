import type { Job, StoredEvent } from "../../src/core/types.ts"

// How a conversation reads in the web app, apart from its screen: a
// session's stored events, with the reply being written and its jobs,
// turned into what the page draws, in order.

// What the page knows of a session: its stored events, in order, from the
// earliest read so far; whether there are earlier ones; the reply being
// written now, as far as it has come; whether a turn is running; and its
// background jobs.
export type View = {
  events: readonly StoredEvent[]
  earlier: boolean
  replySoFar: string
  turnRunning: boolean
  jobs: readonly Job[]
}

// One tool call: its name, a short form of its input, and its result as
// stored, once it has one, with that result in short. `running` while it is
// being carried out.
export type Call = { id: string; name: string; input: string; result?: string; short?: string; running: boolean }

// One thing the page draws. `key` stays the same as more is read, and
// `turn` is the number of the turn it belongs to, counted in what has been
// read, so that the page can space turns apart.
export type Item = { key: string; turn: number } & (
  | { kind: "owner"; text: string }
  | { kind: "reply"; text: string; streaming: boolean }
  // A run of tool calls with nothing said between them, as one line, and
  // what that line says: Working while one of them is, how long the turn
  // worked on the turn's first run once it has ended, or else which tools
  // the run used.
  | { kind: "steps"; calls: Call[]; label: string; running: boolean }
  // Something said in quiet words, with `more` to open, in Markdown.
  | { kind: "note"; text: string; more?: string }
  | { kind: "question"; text: string; choices: string[]; waiting: boolean }
  | { kind: "failed"; error: string }
)

type Steps = Extract<Item, { kind: "steps" }>
type Asked = Extract<Item, { kind: "question" }>

// What the page says of a reply cut short, and of a compaction, as the
// terminal says them.
const CUT_SHORT = "The reply was cut short."
const COMPACTED = "The conversation was getting long, so its earlier part is now a summary."

export function transcriptOf(view: View): Item[] {
  const items: Item[] = []
  let turn = 0
  // The turn being read: when it started, if its start has been read; its
  // first run of calls, which says how long it worked; and whether it has
  // ended. One begun before the first event read has not, until its end is read.
  let started: number | undefined
  let firstRun: Steps | undefined
  let ended = false
  // How long the turn has waited for the owner's answers, which is not
  // counted as work.
  let waited = 0
  const calls = new Map<string, Call>()
  const questions = new Map<string, { item: Asked; time: number }>()
  const worked = new Map<Steps, number>()
  // When each item happened, to place each job's start and end among them.
  const times = new Map<Item, number>()
  const add = (item: Item, time: number) => {
    items.push(item)
    times.set(item, time)
  }
  // The run a call joins: the last thing drawn, when it is a run of this
  // turn's, or a new one.
  const runAt = (time: number): Steps => {
    const last = items.at(-1)
    if (last?.kind === "steps" && last.turn === turn) return last
    const run: Steps = { key: "", turn, kind: "steps", calls: [], label: "", running: false }
    firstRun ??= run
    add(run, time)
    return run
  }
  for (const event of view.events) {
    const key = `e${event.seq}`
    if (event.type === "turn_start") {
      turn += 1
      started = event.time
      firstRun = undefined
      ended = false
      waited = 0
    }
    if (event.type === "user_message") add({ key, turn, kind: "owner", text: event.body.text }, event.time)
    if (event.type === "assistant_message") {
      if (event.body.text !== "") add({ key, turn, kind: "reply", text: event.body.text, streaming: false }, event.time)
      if (event.body.interrupted && event.body.text !== "") add({ key: `${key}-cut`, turn, kind: "note", text: CUT_SHORT }, event.time)
      for (const call of event.body.toolCalls ?? []) {
        const made: Call = { id: call.id, name: call.name, input: inputOf(call.arguments), running: false }
        calls.set(call.id, made)
        runAt(event.time).calls.push(made)
      }
    }
    if (event.type === "tool_result") {
      const call = calls.get(event.body.callId)
      if (call) Object.assign(call, { result: event.body.text, short: shortOf(event.body.text) })
      // A call asked for on a page not read yet is known by its result.
      else runAt(event.time).calls.push({ id: event.body.callId, name: event.body.name, input: "", result: event.body.text, short: shortOf(event.body.text), running: false })
    }
    if (event.type === "question") {
      const item: Asked = { key, turn, kind: "question", text: event.body.text, choices: event.body.choices, waiting: true }
      questions.set(event.body.id, { item, time: event.time })
      add(item, event.time)
    }
    // Once answered, the choices go, and the answer is the owner's message.
    // The time waited is taken off the turn's work only when the question
    // was read too.
    if (event.type === "answer") {
      const asked = questions.get(event.body.id)
      if (asked) {
        asked.item.waiting = false
        waited += event.time - asked.time
      }
      if (event.body.choice === null) add({ key, turn, kind: "note", text: "Not answered." }, event.time)
      else add({ key, turn, kind: "owner", text: event.body.choice }, event.time)
    }
    if (event.type === "note") {
      const [first = "", ...rest] = event.body.text.split("\n")
      add({ key, turn, kind: "note", text: `Note to evie: ${first}`, ...(rest.join("").trim() !== "" && { more: event.body.text }) }, event.time)
    }
    if (event.type === "compaction") add({ key, turn, kind: "note", text: COMPACTED, more: event.body.summary }, event.time)
    if (event.type === "turn_end") {
      ended = true
      // How long it worked is known only when its start was read.
      if (firstRun && started !== undefined) worked.set(firstRun, secondsBetween(started + waited, event.time))
      if (event.body.reason === "error") add({ key, turn, kind: "failed", error: event.body.error ?? "The turn failed." }, event.time)
      // A turn stopped as its reply was being written says both at once.
      const last = items.at(-1)
      if (event.body.reason === "cancelled" && last?.kind === "note" && last.text === CUT_SHORT) last.text = `Stopped. ${CUT_SHORT}`
      else if (event.body.reason === "cancelled") add({ key, turn, kind: "note", text: "Stopped." }, event.time)
    }
  }
  // The turn running now is the last one. It works on the first call of its
  // latest run that has no result; or, with none and nothing being written,
  // on its next step, said at the end.
  if (view.turnRunning && !ended) {
    const latest = items.findLast((item): item is Steps => item.kind === "steps" && item.turn === turn)
    const next = latest?.calls.find((call) => call.result === undefined)
    if (latest && next) {
      latest.running = next.running = true
    } else if (view.replySoFar === "") add({ key: "working", turn, kind: "steps", calls: [], label: "Working", running: true }, Infinity)
  }
  if (view.replySoFar !== "") add({ key: "streaming", turn, kind: "reply", text: view.replySoFar, streaming: true }, Infinity)
  // Each run is known by its last call, which reading the page before does
  // not change, and says what it is.
  for (const item of items) {
    if (item.kind !== "steps" || item.key === "working") continue
    item.key = `s-${item.calls.at(-1)!.id}`
    const seconds = worked.get(item)
    item.label = item.running ? "Working" : seconds !== undefined ? workedFor(seconds) : `Used ${listed([...new Set(item.calls.map((call) => call.name))])}`
  }

  // Each job's start, and its end once it has ended, where it happened,
  // after whatever came before it. One from before the first event read is
  // left for when the page before is read.
  const from = view.earlier ? (view.events[0]?.time ?? Infinity) : -Infinity
  const lines = view.jobs.flatMap((job) => {
    const start = { key: `job${job.id}`, time: job.started, text: `Background job ${job.id} started: ${job.title}` }
    if (job.status === "running") return [start]
    const ended = job.ended ?? job.started
    const how = job.status === "unknown" ? "ended (how is not known)" : job.status
    const took = Math.max(0, Math.round((ended - job.started) / 1000))
    return [start, { key: `job${job.id}-end`, time: ended, text: `Background job ${job.id} ${how} after ${took}s: ${job.title}` }]
  })
  for (const line of lines.filter((each) => each.time >= from).sort((a, b) => a.time - b.time)) {
    const at = items.findIndex((item) => times.get(item)! > line.time)
    const place = at === -1 ? items.length : at
    const item: Item = { key: line.key, turn: items[place - 1]?.turn ?? 0, kind: "note", text: line.text }
    items.splice(place, 0, item)
    times.set(item, line.time)
  }
  return items
}

// Words in a list: "a", "a and b", "a, b and c".
const listed = (words: string[]) => (words.length <= 1 ? (words[0] ?? "") : `${words.slice(0, -1).join(", ")} and ${words.at(-1)}`)

// A call's result in short: its first line with anything on it, cut short.
const SHORT = 80
function shortOf(result: string): string {
  const first = result.split("\n").map((line) => line.trim()).find((line) => line !== "") ?? ""
  return first.length > SHORT ? `${first.slice(0, SHORT)}…` : first
}

// How long, in whole seconds, from one time to another, and at least one.
const secondsBetween = (from: number, to: number) => Math.max(1, Math.round((to - from) / 1000))

// A call's input in short, on one line: what it works on, as a file's path
// or a command, when it names one, or else its first piece of text, or
// else the input as the model wrote it.
const NAMED = ["path", "command", "pattern", "url", "query", "description", "question", "name", "id"]
function inputOf(written: string): string {
  let input: unknown
  try {
    input = JSON.parse(written)
  } catch {
    return oneLine(written)
  }
  if (typeof input !== "object" || input === null) return oneLine(written)
  const fields = input as Record<string, unknown>
  const named = NAMED.map((name) => fields[name]).find((value) => typeof value === "string")
  const text = named ?? Object.values(fields).find((value) => typeof value === "string")
  return oneLine(typeof text === "string" ? text : written)
}
const oneLine = (text: string) => text.replace(/\s+/g, " ").trim()

// What a turn's line of steps says once the turn has ended: how long it
// worked, in its two largest units.
export function workedFor(seconds: number): string {
  const [hours, minutes, rest] = [Math.floor(seconds / 3600), Math.floor((seconds % 3600) / 60), seconds % 60]
  const words = hours > 0 ? [`${hours}h`, minutes > 0 ? `${minutes}m` : ""] : minutes > 0 ? [`${minutes}m`, rest > 0 ? `${rest}s` : ""] : [`${rest}s`]
  return `Worked for ${words.filter((word) => word !== "").join(" ")}`
}
