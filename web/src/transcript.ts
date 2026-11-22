import type { ChangedFile, Job, StoredEvent, TurnChanges } from "../../src/core/types.ts"
import { shownAttachment, type ShownAttachment } from "./attachments.ts"
import { fieldsOf, liveWords, runWords, stepFailed, stepKind, stepTarget, stepWords, type StepKind } from "./step-words.ts"
import { durationWords } from "./work-log.ts"

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
// thing it did; what it worked on, alone (`target`); its input in short, on
// one line, and whole, as read from what the model wrote (`fields`); when
// it was asked for; and its result as stored, once it has one, with that
// result in short, and whether it says the call failed.
export type Call = {
  id: string
  name: string
  kind: StepKind
  sentence: string
  target: string
  input: string
  fields: Readonly<Record<string, unknown>>
  time: number
  result?: string
  short?: string
  running: boolean
  failed: boolean
}

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
  // is doing, shortly. The first run of a turn that ended without being
  // folded also says how long the turn worked, or after how long the owner
  // stopped it (`worked`).
  | { kind: "steps"; calls: Call[]; label: string; running: boolean; worked?: string }
  // The turn running now between its steps: neither writing, nor carrying
  // out a call, nor waiting for an answer, as T3 Code says it, Thinking.
  | { kind: "thinking" }
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
  // The turn's own questions, of which one waiting means it is not thinking.
  const ownQuestions = new Set<string>()
  const calls = new Map<string, Call>()
  const folder = view.folder !== undefined ? { folder: view.folder } : {}
  const questions = new Map<string, { item: Asked; time: number }>()
  const worked = new Map<Steps, { ms: number; stopped: boolean }>()
  // How each turn read ended, by its number: its end's number, whether it
  // completed, and how long it worked when its start was read; and the
  // turns with a reply cut short.
  const ends = new Map<number, { seq: number; completed: boolean; ms?: number }>()
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
  // A call, as asked for with its input as the model wrote it, read once.
  const callOf = (id: string, name: string, written: string | undefined, time: number): Call => {
    const read = written === undefined ? { fields: {}, input: "" } : readInput(id, written)
    const said = { name, input: read.input, fields: read.fields }
    return { id, name, kind: stepKind(name), sentence: stepWords(said, folder), target: stepTarget(said, view.folder), input: read.input, fields: read.fields, time, running: false, failed: false }
  }
  for (const event of view.events) {
    const key = `e${event.seq}`
    if (event.type === "turn_start") {
      turn += 1
      started = event.time
      firstRun = undefined
      ended = false
      ownQuestions.clear()
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
        const made = callOf(call.id, call.name, call.arguments, event.time)
        calls.set(call.id, made)
        runAt(event.time).calls.push(made)
      }
    }
    if (event.type === "tool_result") {
      // A call asked for on a page not read yet is known by its result.
      let call = calls.get(event.body.callId)
      if (call === undefined) {
        call = callOf(event.body.callId, event.body.name, undefined, event.time)
        runAt(event.time).calls.push(call)
      }
      Object.assign(call, { result: event.body.text, short: shortOf(event.body.text), failed: stepFailed(event.body.text) })
    }
    if (event.type === "question") {
      const item: Asked = { key, turn, kind: "question", id: event.body.id, text: event.body.text, choices: event.body.choices, waiting: true }
      questions.set(event.body.id, { item, time: event.time })
      ownQuestions.add(event.body.id)
      add(item, event.time)
    }
    // Once answered, the choices go, and the answer is the owner's message.
    if (event.type === "answer") {
      const asked = questions.get(event.body.id)
      if (asked) asked.item.waiting = false
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
      // How long it worked, from its start to its end, as T3 Code counts
      // it, waits for the owner included, is known only when its start was read.
      const ms = started === undefined ? undefined : Math.max(0, event.time - started)
      if (firstRun && ms !== undefined) worked.set(firstRun, { ms, stopped: event.body.reason === "cancelled" })
      ends.set(turn, { seq: event.seq, completed: event.body.reason === "completed", ...(ms !== undefined && { ms }) })
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
  // latest run that has no result; or, with none, nothing being written
  // and no question of its own waiting, it thinks, as said at the end.
  if (view.turnRunning && !ended) {
    const latest = items.findLast((item): item is Steps => item.kind === "steps" && item.turn === turn)
    const next = latest?.calls.find((call) => call.result === undefined)
    const asking = [...ownQuestions].some((id) => questions.get(id)?.item.waiting)
    if (latest && next) latest.running = next.running = true
    else if (view.replySoFar === "" && !asking) add({ key: "thinking", turn, kind: "thinking" }, Infinity)
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
    const now = item.calls.find((call) => call.running)
    if (now !== undefined) now.sentence = stepWords(now, { running: true, ...folder })
    item.label = now !== undefined ? liveWords(now, view.folder) : item.calls.length === 1 ? item.calls[0]!.sentence : runWords(item.calls)
    const took = worked.get(item)
    if (took !== undefined) item.worked = took.stopped ? `You stopped after ${durationWords(took.ms)}` : workedFor(took.ms)
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
  return folded(items, ends, cut)
}

// When the turn running now started, from its start's time, or nothing
// when no turn runs or its start has not been read.
export function runningSince(view: View): number | undefined {
  if (!view.turnRunning) return undefined
  const last = view.events.findLast((event) => event.type === "turn_start" || event.type === "turn_end")
  return last?.type === "turn_start" ? last.time : undefined
}

// Each turn that ended as completed, with nothing cut short, folded as T3
// Code folds one: its runs of calls and evie's words before the final
// reply go into one line, placed where the first of them was; what else
// came in between, such as a question, its answer or a note, follows the
// line in order, then the final reply. The fold is known by the number of
// the turn's end, which reading the page before does not change.
function folded(items: Item[], ends: Map<number, { seq: number; completed: boolean; ms?: number }>, cut: Set<number>): Item[] {
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
    const label = end.ms !== undefined ? workedFor(end.ms) : runWords(runs.flatMap((run) => run.calls))
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

// A call's input, read once from what the model wrote, as each is kept by
// the call's id, so that the conversation drawn again as a reply streams in
// does not read every call again: its fields, and in short, on one line,
// what it works on, as a file's path or a command, when it names one, or
// else its first piece of text, or else the input as the model wrote it.
const NAMED = ["path", "command", "pattern", "url", "query", "description", "question", "name", "id"]
const read = new Map<string, { written: string; fields: Record<string, unknown>; input: string }>()
// How many calls' inputs are kept, the oldest let go first.
const KEPT = 2000
function readInput(id: string, written: string): { fields: Record<string, unknown>; input: string } {
  const kept = read.get(id)
  if (kept?.written === written) return kept
  const fields = fieldsOf(written)
  const named = NAMED.map((name) => fields[name]).find((value) => typeof value === "string")
  const text = named ?? Object.values(fields).find((value) => typeof value === "string")
  const made = { written, fields, input: oneLine(typeof text === "string" ? text : written) }
  read.set(id, made)
  if (read.size > KEPT) read.delete(read.keys().next().value!)
  return made
}
const oneLine = (text: string) => text.replace(/\s+/g, " ").trim()

// What a turn's line says once the turn has ended: how long it worked, in
// milliseconds, as T3 Code says a duration.
export const workedFor = (ms: number) => `Worked for ${durationWords(ms)}`
