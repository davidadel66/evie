package eviedb

import (
	"context"
	"database/sql"
	"errors"
)

// An FTS table can be searched only by its words: the event or Claim id is an
// UNINDEXED column, so finding the row to replace by its id reads the whole
// table, inside the refresh's write lock (about 16ms a row at 50,000
// messages). Each lexical index therefore keeps a plain lookup from the id to
// the FTS rowid, written in the same transaction as the row, and a refresh
// deletes through it. The FTS tables and their generations are unchanged.
const retrievalRowsSchema = `
CREATE TABLE IF NOT EXISTS memory_retrieval_fts_v3_rows(fts_rowid INTEGER PRIMARY KEY,claim_id TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS memory_retrieval_fts_v3_rows_claim ON memory_retrieval_fts_v3_rows(claim_id);
CREATE TABLE IF NOT EXISTS memory_retrieval_event_fts_v4_rows(fts_rowid INTEGER PRIMARY KEY,event_id TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS memory_retrieval_event_fts_v4_rows_event ON memory_retrieval_event_fts_v4_rows(event_id);
`

// retrievalRows names one FTS table, its lookup table and the id column they
// share.
type retrievalRows struct{ fts, rows, key string }

var (
	acceptedIndexRows     = retrievalRows{"memory_retrieval_fts_v3", "memory_retrieval_fts_v3_rows", "claim_id"}
	conversationIndexRows = retrievalRows{"memory_retrieval_event_fts_v4", "memory_retrieval_event_fts_v4_rows", "event_id"}
)

// retrievalConn lets a maintenance transaction write index rows the way an
// event transaction does: every write returns a row.
type retrievalConn struct{ conn *sql.Conn }

func (c retrievalConn) queryRowContext(ctx context.Context, statement string, args ...any) rowScanner {
	return c.conn.QueryRowContext(ctx, statement, args...)
}

// insert runs statement, an INSERT of id's row into the FTS table, and
// records the row in the lookup. FTS5 assigns the rowid after RETURNING is
// evaluated (RETURNING rowid reads -1), so the rowid is read back with
// last_insert_rowid on the same connection. A rowid an older binary freed and
// the FTS table reused replaces the stale entry.
func (r retrievalRows) insert(ctx context.Context, q eventQueryExecutor, id string, statement string, args ...any) error {
	var inserted string
	if err := q.queryRowContext(ctx, statement+` RETURNING `+r.key, args...).Scan(&inserted); err != nil {
		return err
	}
	var rowid int64
	if err := q.queryRowContext(ctx, `SELECT last_insert_rowid()`).Scan(&rowid); err != nil {
		return err
	}
	return q.queryRowContext(ctx, `INSERT OR REPLACE INTO `+r.rows+`(fts_rowid,`+r.key+`) VALUES(?,?) RETURNING fts_rowid`, rowid, id).Scan(&rowid)
}

// remove deletes every FTS row the lookup holds for id, then its entries. A
// row is deleted only while it still carries id, so an entry an older binary
// left stale cannot remove another id's row.
func (r retrievalRows) remove(ctx context.Context, q *sql.Conn, id string) error {
	rows, err := q.QueryContext(ctx, `SELECT fts_rowid FROM `+r.rows+` WHERE `+r.key+`=?`, id)
	if err != nil {
		return err
	}
	var rowids []int64
	for rows.Next() {
		var rowid int64
		if err = rows.Scan(&rowid); err != nil {
			rows.Close()
			return err
		}
		rowids = append(rowids, rowid)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, rowid := range rowids {
		if _, err = q.ExecContext(ctx, `DELETE FROM `+r.fts+` WHERE rowid=? AND `+r.key+`=?`, rowid, id); err != nil {
			return err
		}
	}
	_, err = q.ExecContext(ctx, `DELETE FROM `+r.rows+` WHERE `+r.key+`=?`, id)
	return err
}

// current reports whether the lookup's newest entry is the FTS table's newest
// row. An FTS table adds each row above its current top, so a row an older
// binary wrote without the lookup leaves the two tops different, as does
// removing the top row. A lower row removed without the lookup leaves an
// entry that remove skips.
func (r retrievalRows) current(ctx context.Context, q semanticInspectionQueryer) (bool, error) {
	top := func(statement string) (int64, string, error) {
		var rowid int64
		var id string
		err := q.QueryRowContext(ctx, statement).Scan(&rowid, &id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", nil
		}
		return rowid, id, err
	}
	ftsRow, ftsID, err := top(`SELECT rowid,` + r.key + ` FROM ` + r.fts + ` ORDER BY rowid DESC LIMIT 1`)
	if err != nil {
		return false, err
	}
	lookupRow, lookupID, err := top(`SELECT fts_rowid,` + r.key + ` FROM ` + r.rows + ` ORDER BY fts_rowid DESC LIMIT 1`)
	if err != nil {
		return false, err
	}
	return ftsRow == lookupRow && ftsID == lookupID, nil
}

// reconcileRetrievalRows rebuilds a lookup that does not match its FTS table:
// on the first open after the lookups were added, and after an older binary,
// which writes the FTS tables without them, changed the index. The common
// no-op case reads two rows per table and takes no write lock.
func reconcileRetrievalRows(ctx context.Context, db *sql.DB) error {
	for _, r := range []retrievalRows{acceptedIndexRows, conversationIndexRows} {
		current, err := r.current(ctx, db)
		if err != nil {
			return err
		}
		if current {
			continue
		}
		if err := withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
			if current, err := r.current(ctx, conn); err != nil || current {
				return err
			}
			if _, err := conn.ExecContext(ctx, `DELETE FROM `+r.rows); err != nil {
				return err
			}
			_, err := conn.ExecContext(ctx, `INSERT INTO `+r.rows+`(fts_rowid,`+r.key+`) SELECT rowid,`+r.key+` FROM `+r.fts)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
