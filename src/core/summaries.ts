import type { Database } from "bun:sqlite"
import type { EventBodies, SessionState, SessionSummary, SummaryEvent } from "./types.ts"

// Summaries: one line for each session, for a client to list sessions by
// without opening them, and sent again whenever that line changes.
//
// A summary is worked out afresh each time, from the session's log and
// from what is running, and the last one sent of each session is kept, to
// tell whether it has changed. A subagent's session has none: it is part of
// the session that started it.

export type Summaries = {
  // Every session that is not a subagent's, the latest active first.
  list(): SessionSummary[]
  // Something has happened in a session, or it was made: its summary is
  // sent if it has changed.
  changed(session: string): void
  // Sends the summary of each session that has changed with nothing
  // happening in it, which only time does: a week of quiet settles it.
  look(): void
}

// What a summary is worked out from: the session's own row, and a few
// events read from the ends of its log. Each is read through the log's key,
// a session and an event's number, from the end that holds it, so only a
// few of each session's events are read however long its log is.
type Row = {
  id: string
  project: string | null
  created: number
  // When its latest event was stored, settling aside, or null when it has none.
  active: number | null
  // The body of its latest event when that is the owner settling it, or
  // bringing it back, after anything else happened in it.
  hand: string | null
  // The body of its first message, or null when it has none.
  first: string | null
  // Which came last of its turns' starts and ends, or null when it has neither.
  turn: "turn_start" | "turn_end" | null
}

const COLUMNS = `session.id, session.project, session.created,
  (SELECT time FROM event WHERE event.session = session.id AND type != 'settled' ORDER BY seq DESC LIMIT 1) AS active,
  (SELECT CASE type WHEN 'settled' THEN body END FROM event WHERE event.session = session.id ORDER BY seq DESC LIMIT 1) AS hand,
  (SELECT body FROM event WHERE event.session = session.id AND type = 'user_message' ORDER BY seq LIMIT 1) AS first,
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

  const lastActive = (row: Row) => row.active ?? row.created
  const stateOf = (row: Row): SessionState => {
    if (options.asking(row.id)) return "needs_you"
    // A turn is running when the last one to start has not ended. One cut
    // off when Evie stopped has not ended either, until its session is
    // opened again and it is closed off, but nothing runs it.
    if ((row.turn === "turn_start" && options.open(row.id)) || options.jobRunning(row.id)) return "working"
    const byHand = row.hand === null ? undefined : (JSON.parse(row.hand) as EventBodies["settled"]).settled
    // The owner's choice, when made after the last activity, wins over the week of quiet.
    if (byHand !== undefined) return byHand ? "settled" : "active"
    return Date.now() - lastActive(row) >= SETTLES_AFTER ? "settled" : "active"
  }

  const summaryOf = (row: Row): SessionSummary => ({
    session: row.id,
    project: row.project,
    name: row.first === null ? null : nameFrom((JSON.parse(row.first) as { text: string }).text),
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
  for (const summary of all.all().map(summaryOf)) sent.set(summary.session, summary)

  return {
    list: () => all.all().map(summaryOf),
    changed(session) {
      const row = one.get(session)
      if (row) send(summaryOf(row))
    },
    look() {
      for (const row of all.all()) send(summaryOf(row))
    },
  }
}

const same = (one: SessionSummary, other: SessionSummary) =>
  one.project === other.project && one.name === other.name && one.state === other.state && one.lastActive === other.lastActive

// How long a session's name may be, in characters, before it is cut short.
const NAME_LENGTH = 60

// A session's name until it is given one: the opening words of its first
// message, from its first line, cut at a word's end with an ellipsis when
// that line is long.
function nameFrom(text: string): string | null {
  const first = text.trim().split("\n")[0]!.trim()
  if (first === "") return null
  if (first.length <= NAME_LENGTH) return first
  // One character past the limit, to tell whether the last word fits whole.
  const cut = first.slice(0, NAME_LENGTH + 1)
  const end = cut.lastIndexOf(" ")
  return `${(end > 0 ? cut.slice(0, end) : cut.slice(0, NAME_LENGTH)).trimEnd()}…`
}
