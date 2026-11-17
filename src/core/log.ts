import type { Database } from "bun:sqlite"
import type { EventBodies, StoredEvent, StoredEventType } from "./types.ts"

// The event log: the add-only record of everything that happened in a
// session. It is the truth; a request to the model is rebuilt from it.
export type EventLog = {
  createSession(folder: string): string
  append<Type extends StoredEventType>(session: string, type: Type, body: EventBodies[Type]): StoredEvent
  events(session: string): StoredEvent[]
}

type EventRow = { session: string; seq: number; type: string; body: string; time: number }

export function createEventLog(db: Database): EventLog {
  const nextSeq = db.query<{ seq: number }, [string]>(
    "SELECT COALESCE(MAX(seq), 0) + 1 AS seq FROM event WHERE session = ?",
  )
  const insert = db.query("INSERT INTO event (session, seq, type, body, time) VALUES (?, ?, ?, ?, ?)")
  const select = db.query<EventRow, [string]>(
    "SELECT session, seq, type, body, time FROM event WHERE session = ? ORDER BY seq",
  )

  const appendRow = db.transaction((session: string, type: string, body: string, time: number): number => {
    const seq = nextSeq.get(session)?.seq ?? 1
    insert.run(session, seq, type, body, time)
    return seq
  })

  return {
    createSession(folder) {
      const id = crypto.randomUUID()
      db.query("INSERT INTO session (id, folder, created) VALUES (?, ?, ?)").run(id, folder, Date.now())
      return id
    },

    append(session, type, body) {
      const time = Date.now()
      const seq = appendRow(session, type, JSON.stringify(body), time)
      return { session, seq, type, body, time } as StoredEvent
    },

    events(session) {
      return select.all(session).map(
        (row) =>
          ({
            session: row.session,
            seq: row.seq,
            type: row.type,
            body: JSON.parse(row.body),
            time: row.time,
          }) as StoredEvent,
      )
    },
  }
}
