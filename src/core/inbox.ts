import type { Database } from "bun:sqlite"
import type { Attachment } from "./types.ts"

// The inbox: the only way input enters a session. A client adds a row, and
// the turn loop takes the waiting rows.
export type InboxMessage = { text: string; attachments?: Attachment[] }

export type Inbox = {
  add(session: string, message: InboxMessage): void
  waiting(session: string): boolean
  take(session: string): InboxMessage[]
}

export function createInbox(db: Database): Inbox {
  const insert = db.query("INSERT INTO inbox (session, body, time) VALUES (?, ?, ?)")
  const count = db.query<{ rows: number }, [string]>("SELECT COUNT(*) AS rows FROM inbox WHERE session = ?")
  const select = db.query<{ id: number; body: string }, [string]>(
    "SELECT id, body FROM inbox WHERE session = ? ORDER BY id",
  )
  const remove = db.query("DELETE FROM inbox WHERE id = ?")

  const takeRows = db.transaction((session: string): InboxMessage[] => {
    const rows = select.all(session)
    for (const row of rows) remove.run(row.id)
    return rows.map((row) => JSON.parse(row.body) as InboxMessage)
  })

  return {
    add(session, message) {
      insert.run(session, JSON.stringify(message), Date.now())
    },
    waiting(session) {
      return (count.get(session)?.rows ?? 0) > 0
    },
    take(session) {
      return takeRows(session)
    },
  }
}
