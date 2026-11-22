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
// stored, once it has one. `running` while it is being carried out.
export type Call = { id: string; name: string; input: string; result?: string; running: boolean }

// What a turn's line of steps opens to: each call, and what evie wrote
// between them.
export type Step = { kind: "call"; call: Call } | { kind: "said"; text: string }

// One thing the page draws. `key` stays the same as more is read, and
// `turn` is the number of the turn it belongs to, counted in what has been
// read, so that the page can space turns apart.
export type Item = { key: string; turn: number } & (
  | { kind: "owner"; text: string }
  | { kind: "reply"; text: string; streaming: boolean }
  // A turn's tool calls, as one line: `running` while the turn is, and
  // `seconds`, once it has ended, how long it worked.
  | { kind: "steps"; steps: Step[]; running: boolean; seconds: number }
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
  // The turn being read: its line of steps, once it has a call, and when it
  // started, which for a turn begun before the first event read is that event.
  let steps: Steps | undefined
  let started: number | undefined
  // Whether the turn being read has ended. One begun before the first event
  // read has not, until its end is read.
  let ended = false
  // How long the turn has waited for the owner's answers, which is not
  // counted as work.
  let waited = 0
  const calls = new Map<string, Call>()
  const questions = new Map<string, { item: Asked; time: number }>()
  // When each item happened, to place each job's start and end among them.
  const times = new Map<Item, number>()
  const add = (item: Item, time: number) => {
    items.push(item)
    times.set(item, time)
  }
  const stepsAt = (key: string, time: number) => {
    if (steps) return steps
    steps = { key, turn, kind: "steps", steps: [], running: false, seconds: 0 }
    add(steps, time)
    return steps
  }
  for (const event of view.events) {
    const key = `e${event.seq}`
    started ??= event.time
    if (event.type === "turn_start") {
      turn += 1
      steps = undefined
      started = event.time
      ended = false
      waited = 0
    }
    if (event.type === "user_message") add({ key, turn, kind: "owner", text: event.body.text }, event.time)
    if (event.type === "assistant_message") {
      const asked = event.body.toolCalls ?? []
      // A reply that asks for tools is part of the turn's steps; the one
      // that asks for none is what evie says.
      if (asked.length === 0 && event.body.text !== "") add({ key, turn, kind: "reply", text: event.body.text, streaming: false }, event.time)
      if (event.body.interrupted && event.body.text !== "") add({ key: `${key}-cut`, turn, kind: "note", text: CUT_SHORT }, event.time)
      if (asked.length > 0) {
        const line = stepsAt(`s${event.seq}`, event.time)
        if (event.body.text !== "") line.steps.push({ kind: "said", text: event.body.text })
        for (const call of asked) {
          const made: Call = { id: call.id, name: call.name, input: inputOf(call.arguments), running: false }
          calls.set(call.id, made)
          line.steps.push({ kind: "call", call: made })
        }
      }
    }
    if (event.type === "tool_result") {
      const call = calls.get(event.body.callId)
      if (call) call.result = event.body.text
      // A call asked for on a page not read yet is known by its result.
      else stepsAt(`s${event.seq}`, event.time).steps.push({ kind: "call", call: { id: event.body.callId, name: event.body.name, input: "", result: event.body.text, running: false } })
    }
    if (event.type === "question") {
      const item: Asked = { key, turn, kind: "question", text: event.body.text, choices: event.body.choices, waiting: true }
      questions.set(event.body.id, { item, time: event.time })
      add(item, event.time)
    }
    // Once answered, the choices go, and the answer is the owner's message.
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
      if (steps) steps.seconds = secondsBetween(started + waited, event.time)
      if (event.body.reason === "error") add({ key, turn, kind: "failed", error: event.body.error ?? "The turn failed." }, event.time)
      // A turn stopped as its reply was being written says both at once.
      const last = items.at(-1)
      if (event.body.reason === "cancelled" && last?.kind === "note" && last.text === CUT_SHORT) last.text = `Stopped. ${CUT_SHORT}`
      else if (event.body.reason === "cancelled") add({ key, turn, kind: "note", text: "Stopped." }, event.time)
    }
  }
  // The turn running now is the last one. It works on the first of its
  // calls with no result, or, with no calls yet and nothing written, on
  // its first step.
  if (view.turnRunning && !ended) {
    const working = steps ?? (view.replySoFar === "" ? stepsAt("working", Infinity) : undefined)
    if (working) working.running = true
    const next = working?.steps.find((step) => step.kind === "call" && step.call.result === undefined)
    if (next?.kind === "call") next.call.running = true
  }
  if (view.replySoFar !== "") add({ key: "streaming", turn, kind: "reply", text: view.replySoFar, streaming: true }, Infinity)

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
