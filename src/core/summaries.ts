import type { Database } from "bun:sqlite"
import { NOT_ACTIVITY } from "./log.ts"
import type { SessionState, SessionSummary, SummaryEvent } from "./types.ts"

// Summaries: one line for each session, for a client to list sessions by
// without opening them, and sent again whenever that line changes.
//
// A summary is worked out afresh each time, from the session's log and
// from what is running, and the last one sent of each session is kept, to
// tell whether it has changed. A subagent's session has none: it is part of
// the session that started it. Nor has a session the owner has said nothing
// in and that has asked nothing, such as one a terminal opened and left: it
// is no conversation to list. It has one from its first message or question,
// and keeps it, so that a client is always told how it stands.

export type Summaries = {
  // Every session that has a summary, the latest active first.
  list(): SessionSummary[]
  // Where a session stands now, or undefined when there is no such session
  // or it is a subagent's.
  state(session: string): SessionState | undefined
  // Something has happened in a session: its summary is sent if it has changed.
  changed(session: string): void
  // Sends the summary of each session that has changed with nothing
  // happening in it, which only time does: a week of quiet settles it.
  look(): void
  // Forgets a session that has been deleted.
  forget(session: string): void
}

// What a summary is worked out from: the session's own row, and a few
// events read from the ends of its log. Each is read through the log's key,
// a session and an event's number, from the end that holds it, so only a
// few of each session's events are read however long its log is.
type Row = {
  id: string
  project: string | null
  created: number
  // When its latest event was stored, settling, naming and moving aside,
  // which are not activity, or null when it has none.
  active: number | null
  // The body of its latest event, naming and moving aside, when that is the
  // owner settling it, or bringing it back, after anything else happened in
  // it, and when that was.
  hand: string | null
  handTime: number | null
  // The body of its first message, or null when it has none.
  first: string | null
  // The body of the latest name it was given, or null when it has none.
  named: string | null
  // Which came last of its turns' starts and ends, or null when it has neither.
  turn: "turn_start" | "turn_end" | null
}

const COLUMNS = `session.id, session.project, session.created,
  (SELECT time FROM event WHERE event.session = session.id AND NOT ${NOT_ACTIVITY} ORDER BY seq DESC LIMIT 1) AS active,
  (SELECT CASE type WHEN 'settled' THEN body END FROM event WHERE event.session = session.id AND (type = 'settled' OR NOT ${NOT_ACTIVITY}) ORDER BY seq DESC LIMIT 1) AS hand,
  (SELECT CASE type WHEN 'settled' THEN time END FROM event WHERE event.session = session.id AND (type = 'settled' OR NOT ${NOT_ACTIVITY}) ORDER BY seq DESC LIMIT 1) AS handTime,
  (SELECT body FROM event WHERE event.session = session.id AND type = 'user_message' ORDER BY seq LIMIT 1) AS first,
  (SELECT body FROM event WHERE event.session = session.id AND type = 'named' ORDER BY seq DESC LIMIT 1) AS named,
  (SELECT type FROM event WHERE event.session = session.id AND type IN ('turn_start', 'turn_end') ORDER BY seq DESC LIMIT 1) AS turn`

// How long a session goes without activity before it settles by itself.
const SETTLES_AFTER = 7 * 24 * 60 * 60_000

export function createSummaries(options: {
  db: Database
  // What only the running core knows of a session: whether a question in it
  // is waiting for the owner's answer, whether it is open in this process,
  // which is the only place a turn of it can run, and whether a background
  // job of its own is running.
  asking(session: string): boolean
  open(session: string): boolean
  jobRunning(session: string): boolean
  emit(event: SummaryEvent): void
}): Summaries {
  const { db } = options
  const all = db.query<Row, []>(
    `SELECT ${COLUMNS} FROM session WHERE parent IS NULL ORDER BY COALESCE(active, created) DESC, created DESC`,
  )
  const one = db.query<Row, [string]>(`SELECT ${COLUMNS} FROM session WHERE id = ? AND parent IS NULL`)
  // Read only for a session with no message, whose log is short.
  const asked = db.query<{ asked: number }, [string]>("SELECT EXISTS (SELECT 1 FROM event WHERE session = ? AND type = 'question') AS asked")

  const listed = (row: Row) => row.first !== null || asked.get(row.id)?.asked === 1
  const lastActive = (row: Row) => row.active ?? row.created
  const stateOf = (row: Row): SessionState => {
    if (options.asking(row.id)) return "needs_you"
    // A turn is running when the last one to start has not ended. One cut
    // off when Evie stopped has not ended either, until its session is
    // opened again and it is closed off, but nothing runs it.
    if ((row.turn === "turn_start" && options.open(row.id)) || options.jobRunning(row.id)) return "working"
    const byHand = bodyOf(row.hand)?.settled
    // Settled by the owner after the last activity, it stays settled.
    if (byHand === true) return "settled"
    // Brought back by the owner, the week of quiet starts again then, as it would with activity.
    const quietSince = byHand === false ? Math.max(lastActive(row), row.handTime!) : lastActive(row)
    return Date.now() - quietSince >= SETTLES_AFTER ? "settled" : "active"
  }

  const summaryOf = (row: Row): SessionSummary => ({
    session: row.id,
    project: row.project,
    name: givenName(bodyOf(row.named)?.name) ?? nameOf(bodyOf(row.first)?.text),
    state: stateOf(row),
    lastActive: lastActive(row),
  })

  // The last summary sent of each session, or worked out when Evie started.
  const sent = new Map<string, SessionSummary>()
  const send = (summary: SessionSummary) => {
    const was = sent.get(summary.session)
    if (was && same(was, summary)) return
    sent.set(summary.session, summary)
    options.emit({ type: "summary", ...summary })
  }
  for (const summary of all.all().filter(listed).map(summaryOf)) sent.set(summary.session, summary)

  return {
    list: () => all.all().filter(listed).map(summaryOf),
    state(session) {
      const row = one.get(session)
      return row ? stateOf(row) : undefined
    },
    changed(session) {
      const row = one.get(session)
      if (row && listed(row)) send(summaryOf(row))
    },
    look() {
      for (const row of all.all().filter(listed)) send(summaryOf(row))
    },
    forget(session) {
      sent.delete(session)
    },
  }
}

const same = (one: SessionSummary, other: SessionSummary) =>
  one.project === other.project && one.name === other.name && one.state === other.state && one.lastActive === other.lastActive

// A stored body read back, or undefined when it is not a JSON object. Evie
// writes only bodies of the right shape, but a damaged database can hold
// others, which give no name and no choice by hand rather than stopping Evie.
function bodyOf(body: string | null): Record<string, unknown> | undefined {
  if (body === null) return undefined
  try {
    const parsed: unknown = JSON.parse(body)
    return typeof parsed === "object" && parsed !== null ? (parsed as Record<string, unknown>) : undefined
  } catch {
    return undefined
  }
}

// The name a session was given, or null when the body holds none.
const givenName = (name: unknown) => (typeof name === "string" && name !== "" ? name : null)

// How long a session's name may be, in characters, before it is cut short.
const NAME_LENGTH = 60

// A session's name until it is given one: the opening words of its first
// message, from its first line, cut at a word's end with an ellipsis when
// that line is long. None when the message's text is not text. A
// subagent is named the same way, from its task.
export function nameOf(text: unknown): string | null {
  if (typeof text !== "string") return null
  const first = text.trim().split("\n")[0]!.trim()
  if (first === "") return null
  if (first.length <= NAME_LENGTH) return first
  // One character past the limit, to tell whether the last word fits whole.
  const cut = first.slice(0, NAME_LENGTH + 1)
  const end = cut.lastIndexOf(" ")
  return `${(end > 0 ? cut.slice(0, end) : cut.slice(0, NAME_LENGTH)).trimEnd()}…`
}
