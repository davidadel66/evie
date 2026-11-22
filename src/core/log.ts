import type { Database } from "bun:sqlite"
import type { Attachment, EventBodies, StoredEvent, StoredEventType } from "./types.ts"

// The event log: the add-only record of everything that happened in a
// session. It is the truth; a request to the model is rebuilt from it. A
// session's events are only ever taken away all at once, when the session
// is deleted for good.
export type EventLog = {
  append<Type extends StoredEventType>(session: string, type: Type, body: EventBodies[Type]): StoredEvent
  events(session: string): StoredEvent[]
  // Up to `limit` of a session's events, the latest of those numbered below
  // `before`, in order, and whether there are earlier ones.
  page(session: string, before: number, limit: number): { events: StoredEvent[]; earlier: boolean }
  // A session's events numbered after `after`, in order, as many as fit in
  // `bytes`, each measured as it is sent, as JSON, but always at least one;
  // their size; and whether more follow. Only those, and the one after
  // them, are read.
  after(session: string, after: number, bytes: number): { events: StoredEvent[]; bytes: number; more: boolean }
  // The number of a session's last event, 0 when it has none, and whether
  // its last turn has started and not ended, read without reading its events.
  latest(session: string): { seq: number; turnRunning: boolean }
  // The sessions whose last turn started and never ended, as when Evie
  // stopped in the middle of it.
  cutOff(): string[]
  // Takes away every event of a session, which is being deleted.
  remove(session: string): void
  // The files attached to a session's messages, oldest first, read from
  // its messages that have any, not from the rest of its log.
  attached(session: string): Attachment[]
}

type EventRow = { session: string; seq: number; type: string; body: string; time: number }

// The events that are not activity in a session, as an SQL condition on a
// row of the event table named `event`: the owner settling it or bringing
// it back, naming it, and moving it, with the note that tells the model of
// the move, which is the event straight after it.
export const NOT_ACTIVITY = `(event.type IN ('settled', 'named', 'moved') OR (event.type = 'note' AND EXISTS (SELECT 1 FROM event AS move WHERE move.session = event.session AND move.seq = event.seq - 1 AND move.type = 'moved')))`

export function createEventLog(db: Database): EventLog {
  const nextSeq = db.query<{ seq: number }, [string]>(
    "SELECT COALESCE(MAX(seq), 0) + 1 AS seq FROM event WHERE session = ?",
  )
  const insert = db.query("INSERT INTO event (session, seq, type, body, time) VALUES (?, ?, ?, ?, ?)")
  const select = db.query<EventRow, [string]>(
    "SELECT session, seq, type, body, time FROM event WHERE session = ? ORDER BY seq",
  )

  // Read back from `before` through the log's key, so only the page is read.
  const selectBefore = db.query<EventRow, [string, number, number]>(
    "SELECT session, seq, type, body, time FROM event WHERE session = ? AND seq < ? ORDER BY seq DESC LIMIT ?",
  )
  // Read row by row, and left once the page is full. It is made afresh
  // each time: Bun 1.4.0 refuses to read again through a statement whose
  // last reading was left part way ("bad parameter or other API misuse").
  const AFTER = "SELECT session, seq, type, body, time FROM event WHERE session = ? AND seq > ? ORDER BY seq"
  const lastSeq = db.query<{ seq: number }, [string]>("SELECT COALESCE(MAX(seq), 0) AS seq FROM event WHERE session = ?")
  const lastTurn = db.query<{ type: string }, [string]>(
    "SELECT type FROM event WHERE session = ? AND type IN ('turn_start', 'turn_end') ORDER BY seq DESC LIMIT 1",
  )
  const lastTurns = db.query<{ id: string }, []>(
    `SELECT id FROM session WHERE (SELECT type FROM event WHERE event.session = session.id AND type IN ('turn_start', 'turn_end') ORDER BY seq DESC LIMIT 1) = 'turn_start'`,
  )
  const removeAll = db.query("DELETE FROM event WHERE session = ?")
  const withAttachments = db.query<{ body: string }, [string]>(
    `SELECT body FROM event WHERE session = ? AND type = 'user_message' AND body LIKE '%"attachments":%' ORDER BY seq`,
  )
  const fromRow = (row: EventRow) =>
    ({
      session: row.session,
      seq: row.seq,
      type: row.type,
      body: JSON.parse(row.body),
      time: row.time,
    }) as StoredEvent

  const appendRow = db.transaction((session: string, type: string, body: string, time: number): number => {
    const seq = nextSeq.get(session)?.seq ?? 1
    insert.run(session, seq, type, body, time)
    return seq
  })

  return {
    append(session, type, body) {
      const time = Date.now()
      const seq = appendRow(session, type, JSON.stringify(body), time)
      return { session, seq, type, body, time } as StoredEvent
    },

    events(session) {
      return select.all(session).map(fromRow)
    },

    page(session, before, limit) {
      // One more than the page is read, to tell whether there are earlier ones.
      const rows = selectBefore.all(session, before, limit + 1)
      return { events: rows.slice(0, limit).reverse().map(fromRow), earlier: rows.length > limit }
    },

    after(session, after, bytes) {
      const events: StoredEvent[] = []
      let size = 0
      const selectAfter = db.prepare<EventRow, [string, number]>(AFTER)
      try {
        for (const row of selectAfter.iterate(session, after)) {
          const event = fromRow(row)
          const more = Buffer.byteLength(JSON.stringify(event))
          if (events.length > 0 && size + more > bytes) return { events, bytes: size, more: true }
          events.push(event)
          size += more
        }
        return { events, bytes: size, more: false }
      } finally {
        selectAfter.finalize()
      }
    },

    latest(session) {
      return { seq: lastSeq.get(session)?.seq ?? 0, turnRunning: lastTurn.get(session)?.type === "turn_start" }
    },

    attached(session) {
      return withAttachments.all(session).flatMap((row) => (JSON.parse(row.body) as EventBodies["user_message"]).attachments ?? [])
    },

    cutOff() {
      return lastTurns.all().map((row) => row.id)
    },

    remove(session) {
      removeAll.run(session)
    },
  }
}
