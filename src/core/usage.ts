import type { Database } from "bun:sqlite"
import type { ProjectUsage, UsagePeriod } from "./types.ts"

// What a workspace has used: its sessions' replies, summed from the event
// log, which is the truth. Each reply the model made is an `assistant_message`
// event whose body carries the provider's token counts and, from the time
// evie began recording it, the cost. Nothing is kept apart from the log, so
// there is nothing to fall out of step with it. A subagent is a session of
// the same project, so it counts. A session moved to another project counts
// where it is now, with all its replies.

const DAY = 24 * 60 * 60 * 1000

// The start of today on this computer's clock.
function midnight(now: number): number {
  const start = new Date(now)
  start.setHours(0, 0, 0, 0)
  return start.getTime()
}

type Row = { sessions: number; replies: number; input: number; output: number; cached: number; cost: number; unpriced: number }

export function readUsage(db: Database, project: string, now = Date.now()): ProjectUsage {
  const since = { today: midnight(now), week: now - 7 * DAY, month: now - 30 * DAY, all: 0 }
  const query = db.query<Row, [string, number]>(
    `SELECT
       COUNT(DISTINCT event.session) AS sessions,
       COUNT(*) AS replies,
       COALESCE(SUM(json_extract(event.body, '$.usage.inputTokens')), 0) AS input,
       COALESCE(SUM(json_extract(event.body, '$.usage.outputTokens')), 0) AS output,
       COALESCE(SUM(json_extract(event.body, '$.usage.cachedTokens')), 0) AS cached,
       COALESCE(SUM(json_extract(event.body, '$.usage.cost')), 0) AS cost,
       COALESCE(SUM(json_extract(event.body, '$.usage.cost') IS NULL), 0) AS unpriced
     FROM event JOIN session ON session.id = event.session
     WHERE session.project = ? AND event.type = 'assistant_message' AND event.time >= ?
       AND json_extract(event.body, '$.usage') IS NOT NULL`,
  )
  const period = (from: number): UsagePeriod => {
    const row = query.get(project, from)!
    return { sessions: row.sessions, replies: row.replies, inputTokens: row.input, outputTokens: row.output, cachedTokens: row.cached, cost: row.cost, unpriced: row.unpriced }
  }
  return { today: period(since.today), week: period(since.week), month: period(since.month), all: period(since.all) }
}
