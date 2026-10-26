package eviedb

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
)

// requireRetrievalRowsMatch fails unless the lookup holds exactly the FTS
// table's rows, each under its own id.
func requireRetrievalRowsMatch(t *testing.T, db *sql.DB, r retrievalRows) {
	t.Helper()
	read := func(query string) []string {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var rowid int64
			var id string
			if err := rows.Scan(&rowid, &id); err != nil {
				t.Fatal(err)
			}
			out = append(out, fmt.Sprintf("%d:%s", rowid, id))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	indexed := read(`SELECT rowid,` + r.key + ` FROM ` + r.fts + ` ORDER BY rowid`)
	lookup := read(`SELECT fts_rowid,` + r.key + ` FROM ` + r.rows + ` ORDER BY fts_rowid`)
	if !slices.Equal(indexed, lookup) {
		t.Fatalf("%s lookup does not match the index:\nindex  %v\nlookup %v", r.fts, indexed, lookup)
	}
}

func countIndexRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const repeatedConversationRowsSQL = `SELECT count(*) FROM (SELECT event_id FROM memory_retrieval_event_fts_v4 GROUP BY event_id HAVING count(*)>1)`

func TestConversationIndexRefreshReplacesEachMessagesOwnRow(t *testing.T) {
	store := conversationIndexCorpus(t, 40)
	requireRetrievalRowsMatch(t, store.db, conversationIndexRows)
	if _, err := store.db.Exec(`INSERT OR IGNORE INTO memory_retrieval_event_dirty
 SELECT id FROM events WHERE event_type IN ('user_message','assistant_message')`); err != nil {
		t.Fatal(err)
	}
	refreshUntilActive(t, store)
	if n := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4`); n != 40 {
		t.Fatalf("index holds %d rows for 40 messages", n)
	}
	if n := countIndexRows(t, store.db, repeatedConversationRowsSQL); n != 0 {
		t.Fatalf("%d messages kept their old row beside the new one", n)
	}
	requireRetrievalRowsMatch(t, store.db, conversationIndexRows)
}

func TestConversationIndexDropsMessagesThatStopBeingEligible(t *testing.T) {
	ctx := context.Background()
	store := openIndexedStore(t, filepath.Join(t.TempDir(), "evie.db"))
	project, err := store.RegisterProject(ctx, "Garden", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inProject, err := store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	global, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appendIndexedMessages(t, store, inProject.ID, 0, 6)
	appendIndexedMessages(t, store, global.ID, 6, 4)
	refreshUntilActive(t, store)
	if n := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4`); n != 10 {
		t.Fatalf("index holds %d rows for 10 messages", n)
	}

	if _, err := store.db.Exec(`UPDATE projects SET archived=1 WHERE id=?`, project.ID); err != nil {
		t.Fatal(err)
	}
	refreshUntilActive(t, store)
	if n := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4 f JOIN events e ON e.id=f.event_id WHERE e.session_id=?`, inProject.ID); n != 0 {
		t.Fatalf("%d messages of an archived project are still indexed", n)
	}
	if n := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4`); n != 4 {
		t.Fatalf("index holds %d rows, want the 4 Global messages", n)
	}
	requireRetrievalRowsMatch(t, store.db, conversationIndexRows)
}

func TestAcceptedIndexRefreshReplacesTheClaimsOwnRow(t *testing.T) {
	ctx, store, global, _ := openOperationHistoryStore(t)
	proposal := rememberLiteralAt(t, ctx, store, global, "Remember that my home city is Lisbon.", "home_city", text("Lisbon"), "71500000-0000-4000-8000-0000000000a1")
	refreshUntilActive(t, store)
	const claimRows = `SELECT count(*) FROM memory_retrieval_fts_v3 WHERE claim_id=?`
	if n := countIndexRows(t, store.db, claimRows, proposal.ClaimID); n != 1 {
		t.Fatalf("accepted Claim has %d index rows, want 1", n)
	}
	if _, err := store.db.Exec(`INSERT OR IGNORE INTO memory_retrieval_dirty VALUES(?)`, proposal.ClaimID); err != nil {
		t.Fatal(err)
	}
	refreshUntilActive(t, store)
	if n := countIndexRows(t, store.db, claimRows, proposal.ClaimID); n != 1 {
		t.Fatalf("refreshed Claim has %d index rows, want 1", n)
	}
	requireRetrievalRowsMatch(t, store.db, acceptedIndexRows)
}

func TestOpeningRepairsTheIndexLookup(t *testing.T) {
	reopen := func(t *testing.T, store *Store, path string) *Store {
		t.Helper()
		store.db.Close()
		return openIndexedStore(t, path)
	}

	t.Run("an index written before the lookup existed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "evie.db")
		store := conversationIndexCorpusAt(t, path, 30)
		if _, err := store.db.Exec(`DROP TABLE memory_retrieval_event_fts_v4_rows; DROP TABLE memory_retrieval_fts_v3_rows`); err != nil {
			t.Fatal(err)
		}
		store = reopen(t, store, path)
		if n := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4_rows`); n != 30 {
			t.Fatalf("lookup holds %d rows after upgrade, want 30", n)
		}
		requireRetrievalRowsMatch(t, store.db, conversationIndexRows)
		requireRetrievalRowsMatch(t, store.db, acceptedIndexRows)
	})

	t.Run("rows an older binary wrote without it", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "evie.db")
		store := conversationIndexCorpusAt(t, path, 30)
		var refreshed, scope, body string
		if err := store.db.QueryRow(`SELECT event_id,scope_key,body FROM memory_retrieval_event_fts_v4 ORDER BY rowid LIMIT 1`).Scan(&refreshed, &scope, &body); err != nil {
			t.Fatal(err)
		}
		// An older binary refreshes one message and appends another, touching
		// only the FTS table.
		if _, err := store.db.Exec(`DELETE FROM memory_retrieval_event_fts_v4 WHERE event_id=?`, refreshed); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`INSERT INTO memory_retrieval_event_fts_v4(generation,event_id,scope_key,body,keys) VALUES(?,?,?,?,'')`, conversationIndexGeneration, refreshed, scope, body); err != nil {
			t.Fatal(err)
		}
		global, err := store.CreateGlobalSession(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		appendIndexedMessages(t, store, global.ID, 30, 1)
		if _, err := store.db.Exec(`DELETE FROM memory_retrieval_event_fts_v4_rows WHERE fts_rowid=(SELECT max(fts_rowid) FROM memory_retrieval_event_fts_v4_rows)`); err != nil {
			t.Fatal(err)
		}

		store = reopen(t, store, path)
		requireRetrievalRowsMatch(t, store.db, conversationIndexRows)
		if _, err := store.db.Exec(`INSERT OR IGNORE INTO memory_retrieval_event_dirty VALUES(?)`, refreshed); err != nil {
			t.Fatal(err)
		}
		refreshUntilActive(t, store)
		if n := countIndexRows(t, store.db, repeatedConversationRowsSQL); n != 0 {
			t.Fatalf("%d messages kept a row the older binary wrote", n)
		}
		if n := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4`); n != 31 {
			t.Fatalf("index holds %d rows for 31 messages", n)
		}
		requireRetrievalRowsMatch(t, store.db, conversationIndexRows)
	})

	t.Run("a stale entry cannot remove another message's row", func(t *testing.T) {
		ctx := context.Background()
		store := conversationIndexCorpus(t, 4)
		var stale, other string
		var otherRow int64
		if err := store.db.QueryRow(`SELECT event_id FROM memory_retrieval_event_fts_v4 ORDER BY rowid LIMIT 1`).Scan(&stale); err != nil {
			t.Fatal(err)
		}
		if err := store.db.QueryRow(`SELECT rowid,event_id FROM memory_retrieval_event_fts_v4 ORDER BY rowid DESC LIMIT 1`).Scan(&otherRow, &other); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`INSERT OR REPLACE INTO memory_retrieval_event_fts_v4_rows(fts_rowid,event_id) VALUES(?,?)`, otherRow, stale); err != nil {
			t.Fatal(err)
		}
		conn, err := store.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := conversationIndexRows.remove(ctx, conn, stale); err != nil {
			t.Fatal(err)
		}
		if n := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4 WHERE rowid=? AND event_id=?`, otherRow, other); n != 1 {
			t.Fatal("removing one message deleted another message's row through a stale entry")
		}
		if n := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4 WHERE event_id=?`, stale); n != 0 {
			t.Fatal("the message's own row survived its removal")
		}
	})
}

// Coverage counts the lookups and the dirty queues, never the FTS tables, so
// its numbers must keep matching the indexes through appends, refreshes and
// removals.
func TestIndexCoverageMatchesTheIndexes(t *testing.T) {
	ctx := context.Background()
	store := openIndexedStore(t, filepath.Join(t.TempDir(), "evie.db"))
	project, err := store.RegisterProject(ctx, "Garden", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inProject, err := store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	global, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	requireConversationCoverage := func(t *testing.T, pending, indexed int) {
		t.Helper()
		coverage, err := conversationIndexCoverage(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if actual := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_event_fts_v4`); actual != indexed {
			t.Fatalf("index holds %d rows, want %d", actual, indexed)
		}
		if coverage.Pending != int64(pending) || coverage.Indexed != int64(indexed) {
			t.Fatalf("coverage pending=%d indexed=%d, want pending=%d indexed=%d", coverage.Pending, coverage.Indexed, pending, indexed)
		}
	}

	appendIndexedMessages(t, store, inProject.ID, 0, 6)
	appendIndexedMessages(t, store, global.ID, 6, 4)
	requireConversationCoverage(t, 0, 10)

	if _, err := store.db.Exec(`INSERT OR IGNORE INTO memory_retrieval_event_dirty
 SELECT id FROM events WHERE event_type IN ('user_message','assistant_message') ORDER BY rowid LIMIT 3`); err != nil {
		t.Fatal(err)
	}
	requireConversationCoverage(t, 3, 10)
	refreshUntilActive(t, store)
	requireConversationCoverage(t, 0, 10)

	if _, err := store.db.Exec(`UPDATE projects SET archived=1 WHERE id=?`, project.ID); err != nil {
		t.Fatal(err)
	}
	requireConversationCoverage(t, 6, 10)
	refreshUntilActive(t, store)
	requireConversationCoverage(t, 0, 4)

	proposal := rememberLiteralAt(t, ctx, store, global, "Remember that my home city is Lisbon.", "home_city", text("Lisbon"), "71500000-0000-4000-8000-0000000000a2")
	refreshUntilActive(t, store)
	if _, err := store.db.Exec(`INSERT OR IGNORE INTO memory_retrieval_dirty VALUES(?)`, proposal.ClaimID); err != nil {
		t.Fatal(err)
	}
	accepted, err := memoryIndexCoverage(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if actual := countIndexRows(t, store.db, `SELECT count(*) FROM memory_retrieval_fts_v3`); accepted.Pending != 1 || accepted.Indexed != int64(actual) || actual != 1 {
		t.Fatalf("accepted coverage pending=%d indexed=%d, want pending=1 indexed=1 (index holds %d)", accepted.Pending, accepted.Indexed, actual)
	}
}
