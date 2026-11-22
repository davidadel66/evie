import type { ChangedFile, Job, StoredEvent, TurnChanges } from "../../src/core/types.ts"
import { shownAttachment, type ShownAttachment } from "./attachments.ts"
import { stepKind, stepSentence, type StepKind } from "./step-words.ts"
import { durationWords, runSummary } from "./work-log.ts"

// How a conversation reads in the web app, apart from its screen: a
// session's stored events, with the reply being written and its jobs,
// turned into what the page draws, in order.

// What the page knows of a session: its stored events, in order, from the
// earliest read so far; whether there are earlier ones; the reply being
// written now, as far as it has come; whether a turn is running; and its
// background jobs. `pending` are the owner's messages sent from this page
// that the session has not taken in yet, in the order sent, each with the
// files on their way with it. `notes` are
// said in this page and kept nowhere, such as what a command said, each
// at the time of the session's last event when it was said. `folder` is
// the session's project's, when the page knows it, which the work log
// names the files in it from.
export type View = {
  events: readonly StoredEvent[]
  earlier: boolean
  replySoFar: string
  turnRunning: boolean
  jobs: readonly Job[]
  pending?: readonly Pending[]
  notes?: readonly PageNote[]
  folder?: string
}
export type PageNote = { key: string; time: number; text: string }
export type Pending = { text: string; files?: readonly File[] }

// One tool call: its name; what it did, as one sentence of the work log,
// in the present while it is being carried out (`running`), and the kind of
// thing it did; a short form of its input; and its result as stored, once
// it has one, with that result in short.
export type Call = { id: string; name: string; kind: StepKind; sentence: string; input: string; result?: string; short?: string; running: boolean }

// One thing the page draws. `key` stays the same as more is read, and
// `turn` is the number of the turn it belongs to, counted in what has been
// read, so that the page can space turns apart.
export type Item = { key: string; turn: number } & (
  // `attachments` are the files attached to the owner's message, when it has any.
  // `sending` are the files of a message on its way.
  | { kind: "owner"; text: string; attachments?: ShownAttachment[]; sending?: readonly File[] }
  | { kind: "reply"; text: string; streaming: boolean }
  // A run of tool calls with nothing said between them, as the work log
  // shows it: a run of one call as that call's sentence, a run of several
  // counted by kind, and while one of them is being carried out, what it
  // is doing. The first run of a turn that ended without being folded also
  // says how long the turn worked (`worked`).
  | { kind: "steps"; calls: Call[]; label: string; running: boolean; worked?: string }
  // The head of the turn running now, as T3 Code's work log has it: when it
  // started, when its start has been read; how long it has waited for the
  // owner's answers; and while a question waits, since when. The page
  // counts how long it has worked from these.
  | { kind: "working"; started?: number; waited: number; waitingSince?: number }
  // Something said in quiet words, with `more` to open, in Markdown.
  | { kind: "note"; text: string; more?: string }
  // `waiting` while the log has no answer to it; `id` ties it to its answer.
  | { kind: "question"; id: string; text: string; choices: string[]; waiting: boolean }
  | { kind: "failed"; error: string }
  // A turn that ended as completed, as T3 Code draws one: what came before
  // its final reply, evie's words and its runs of calls, folded into one
  // line saying how long the turn worked, or, when its start was not read,
  // which tools it used. Questions, answers and notes stay outside it.
  | { kind: "fold"; label: string; items: Item[] }
  // The files a completed turn changed, after its final reply, with the
  // turn's number in the session, for the diff panel to open on.
  | { kind: "changes"; number: number; files: ChangedFile[] }
)

type Steps = Extract<Item, { kind: "steps" }>
type Asked = Extract<Item, { kind: "question" }>

// What the page says of a reply cut short, and of a compaction, as the
// terminal says them.
const CUT_SHORT = "The reply was cut short."
const COMPACTED = "The conversation was getting long, so its earlier part is now a summary."

// `changes` are the session's turns that changed files, as the server
// lists them, each known by its end's event.
export function transcriptOf(view: View, changes: readonly TurnChanges[] = []): Item[] {
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
  // Each call's input as the model wrote it, which its sentence is made from.
  const asked = new Map<Call, string>()
  const sentenced = (call: { name: string; arguments: string }, running: boolean) => ({
    kind: stepKind(call.name),
    sentence: stepSentence(call.name, call.arguments, { running, ...(view.folder !== undefined && { folder: view.folder }) }),
  })
  // A run of calls counted by kind.
  const summed = (all: Call[]) => runSummary(all.map((call) => ({ name: call.name, arguments: asked.get(call) ?? "{}" })))
  // The question the running turn waits on, by when it was asked.
  let waitingSince: number | undefined
  const questions = new Map<string, { item: Asked; time: number }>()
  const worked = new Map<Steps, number>()
  // How each turn read ended, by its number: its end's number, whether it
  // completed, and how long it worked when its start was read; and the
  // turns with a reply cut short.
  const ends = new Map<number, { seq: number; completed: boolean; seconds?: number }>()
  const cut = new Set<number>()
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
      waitingSince = undefined
    }
    if (event.type === "user_message") {
      const attached = (event.body.attachments ?? []).map((each) => shownAttachment(event.session, each))
      add({ key, turn, kind: "owner", text: event.body.text, ...(attached.length > 0 && { attachments: attached }) }, event.time)
    }
    if (event.type === "assistant_message") {
      if (event.body.text !== "") add({ key, turn, kind: "reply", text: event.body.text, streaming: false }, event.time)
      if (event.body.interrupted && event.body.text !== "") {
        add({ key: `${key}-cut`, turn, kind: "note", text: CUT_SHORT }, event.time)
        cut.add(turn)
      }
      for (const call of event.body.toolCalls ?? []) {
        const made: Call = { id: call.id, name: call.name, ...sentenced(call, false), input: inputOf(call.arguments), running: false }
        asked.set(made, call.arguments)
        calls.set(call.id, made)
        runAt(event.time).calls.push(made)
      }
    }
    if (event.type === "tool_result") {
      const call = calls.get(event.body.callId)
      if (call) Object.assign(call, { result: event.body.text, short: shortOf(event.body.text) })
      // A call asked for on a page not read yet is known by its result.
      else {
        const known = { name: event.body.name, arguments: "{}" }
        runAt(event.time).calls.push({ id: event.body.callId, name: event.body.name, ...sentenced(known, false), input: "", result: event.body.text, short: shortOf(event.body.text), running: false })
      }
    }
    if (event.type === "question") {
      const item: Asked = { key, turn, kind: "question", id: event.body.id, text: event.body.text, choices: event.body.choices, waiting: true }
      questions.set(event.body.id, { item, time: event.time })
      waitingSince ??= event.time
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
      waitingSince = [...questions.values()].find((each) => each.item.waiting)?.time
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
      const seconds = started === undefined ? undefined : secondsBetween(started + waited, event.time)
      if (firstRun && seconds !== undefined) worked.set(firstRun, seconds)
      ends.set(turn, { seq: event.seq, completed: event.body.reason === "completed", ...(seconds !== undefined && { seconds }) })
      if (event.body.reason === "error") add({ key, turn, kind: "failed", error: event.body.error ?? "The turn failed." }, event.time)
      const changed = event.body.reason === "completed" ? changes.find((each) => each.end === event.seq) : undefined
      if (changed) add({ key: `changes-${event.seq}`, turn, kind: "changes", number: changed.turn, files: changed.files }, event.time)
      // A turn stopped as its reply was being written says both at once.
      const last = items.at(-1)
      if (event.body.reason === "cancelled" && last?.kind === "note" && last.text === CUT_SHORT) last.text = `Stopped. ${CUT_SHORT}`
      else if (event.body.reason === "cancelled") add({ key, turn, kind: "note", text: "Stopped." }, event.time)
    }
  }
  // The turn running now is the last one. It works on the first call of its
  // latest run that has no result; or, with none and nothing being written,
  // on its next step, said at the end.
  const running = view.turnRunning && !ended
  if (running) {
    const latest = items.findLast((item): item is Steps => item.kind === "steps" && item.turn === turn)
    const next = latest?.calls.find((call) => call.result === undefined)
    if (latest && next) latest.running = next.running = true
  }
  if (view.replySoFar !== "") add({ key: "streaming", turn, kind: "reply", text: view.replySoFar, streaming: true }, Infinity)
  // The owner's messages the session has not taken in yet come last, as
  // they will: a turn takes them in at its next step.
  for (const [at, { text, files }] of (view.pending ?? []).entries()) {
    add({ key: `pending${at}`, turn, kind: "owner", text, ...(files !== undefined && files.length > 0 && { sending: files }) }, Infinity)
  }
  // Each run is known by its last call, which reading the page before does
  // not change, and says what it is.
  for (const item of items) {
    if (item.kind !== "steps") continue
    item.key = `s-${item.calls.at(-1)!.id}`
    for (const call of item.calls) if (call.running) Object.assign(call, sentenced({ name: call.name, arguments: asked.get(call) ?? "{}" }, true))
    const now = item.calls.find((call) => call.running)
    item.label = now !== undefined ? now.sentence : item.calls.length === 1 ? item.calls[0]!.sentence : summed(item.calls)
    const seconds = worked.get(item)
    if (seconds !== undefined) item.worked = workedFor(seconds)
  }
  // The running turn's head goes under the owner's messages it started
  // with, before anything it has done, and before any message of the
  // owner's not taken in yet.
  if (running) {
    const first = items.findIndex((item) => item.turn === turn && !(item.kind === "owner" && item.key.startsWith("e")))
    const place = first === -1 ? items.length : first
    const head: Item = { key: "working", turn, kind: "working", ...(started !== undefined && { started }), waited, ...(waitingSince !== undefined && { waitingSince }) }
    items.splice(place, 0, head)
    times.set(head, items[place - 1] === undefined ? -Infinity : times.get(items[place - 1]!)!)
  }

  // Each job's start, and its end once it has ended, and what a command
  // said, where it happened, after whatever came before it. One from before
  // the first event read is left for when the page before is read.
  const from = view.earlier ? (view.events[0]?.time ?? Infinity) : -Infinity
  const lines = view.jobs.flatMap((job) => {
    const start = { key: `job${job.id}`, time: job.started, text: `Background job ${job.id} started: ${job.title}` }
    if (job.status === "running") return [start]
    const ended = job.ended ?? job.started
    const how = job.status === "unknown" ? "ended (how is not known)" : job.status
    const took = Math.max(0, Math.round((ended - job.started) / 1000))
    return [start, { key: `job${job.id}-end`, time: ended, text: `Background job ${job.id} ${how} after ${took}s: ${job.title}` }]
  })
  // The notes said in this page go where they were said, in the same way.
  for (const line of [...lines.filter((each) => each.time >= from), ...(view.notes ?? [])].sort((a, b) => a.time - b.time)) {
    const at = items.findIndex((item) => times.get(item)! > line.time)
    const place = at === -1 ? items.length : at
    const item: Item = { key: line.key, turn: items[place - 1]?.turn ?? 0, kind: "note", text: line.text }
    items.splice(place, 0, item)
    times.set(item, line.time)
  }
  return folded(items, ends, cut, summed)
}

// Each turn that ended as completed, with nothing cut short, folded as T3
// Code folds one: its runs of calls and evie's words before the final
// reply go into one line, placed where the first of them was; what else
// came in between, such as a question, its answer or a note, follows the
// line in order, then the final reply. The fold is known by the number of
// the turn's end, which reading the page before does not change.
function folded(items: Item[], ends: Map<number, { seq: number; completed: boolean; seconds?: number }>, cut: Set<number>, summed: (calls: Call[]) => string): Item[] {
  const out: Item[] = []
  for (let from = 0; from < items.length; ) {
    const turn = items[from]!.turn
    let to = from
    while (to < items.length && items[to]!.turn === turn) to += 1
    const group = items.slice(from, to)
    from = to
    const end = ends.get(turn)
    const final = group.findLastIndex((item) => item.kind === "reply")
    const before = final === -1 ? group : group.slice(0, final)
    const inside = before.filter((item) => item.kind === "steps" || item.kind === "reply")
    if (!end?.completed || cut.has(turn) || inside.length === 0) {
      out.push(...group)
      continue
    }
    // Inside, each run says what it did; the fold says how long.
    const runs = inside.filter((item): item is Steps => item.kind === "steps")
    for (const run of runs) delete run.worked
    const label = end.seconds !== undefined ? workedFor(end.seconds) : summed(runs.flatMap((run) => run.calls))
    const first = group.indexOf(inside[0]!)
    out.push(
      ...group.slice(0, first),
      { key: `fold-${end.seq}`, turn, kind: "fold", label, items: inside },
      ...before.slice(first).filter((item) => !(inside as Item[]).includes(item)),
      ...(final === -1 ? [] : group.slice(final)),
    )
  }
  return out
}


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

// What a turn's line says once the turn has ended: how long it worked, in
// its two largest units.
export const workedFor = (seconds: number) => `Worked for ${durationWords(seconds)}`
