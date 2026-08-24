package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestFreshSchemaHasNullableSessionTitle(t *testing.T) {
	db := newTestDB(t)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'title'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("title columns = %d, want 1", count)
	}
	session := mustCreateGlobalSession(t, NewStore(db))
	if session.Title != "" {
		t.Fatalf("new session title = %q, want null/empty", session.Title)
	}
	var title sql.NullString
	if err := db.QueryRow(`SELECT title FROM sessions WHERE id = ?`, session.ID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title.Valid {
		t.Fatalf("fresh title = %+v, want NULL", title)
	}
}

func TestLegacyTitleUpgradeBackfillsIdempotentlyAndPreservesUpdatedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyTitleDatabase(t, path)

	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	var title, updatedAt string
	if err := db.QueryRow(`SELECT title, updated_at FROM sessions WHERE id = 'legacy'`).Scan(&title, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if title != "first title" || updatedAt != "2026-08-24T10:00:00Z" {
		t.Fatalf("backfill title=%q updated_at=%q", title, updatedAt)
	}
	if _, err := db.Exec(`UPDATE sessions SET title = 'Kept title' WHERE id = 'legacy'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatalf("idempotent reopen: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.QueryRow(`SELECT title, updated_at FROM sessions WHERE id = 'legacy'`).Scan(&title, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if title != "Kept title" || updatedAt != "2026-08-24T10:00:00Z" {
		t.Fatalf("reopen overwrote title/timestamp: title=%q updated_at=%q", title, updatedAt)
	}
}

func TestTwoConcurrentLegacyOpensSerializeTitleUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyTitleDatabase(t, path)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	firstOwned := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondAttempting := make(chan struct{})
	firstSchema := make(chan struct{})
	secondSchema := make(chan struct{})
	startFirstMigration := make(chan struct{})
	startSecondMigration := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	t.Cleanup(release)
	var startFirstOnce, startSecondOnce sync.Once
	startFirst := func() { startFirstOnce.Do(func() { close(startFirstMigration) }) }
	startSecond := func() { startSecondOnce.Do(func() { close(startSecondMigration) }) }
	t.Cleanup(startSecond)
	t.Cleanup(startFirst)
	type openResult struct {
		db  *sql.DB
		err error
	}
	startOpen := func(hooks openDBAtHooks) <-chan openResult {
		done := make(chan openResult)
		go func() {
			db, err := openDBAtContextWithHooks(ctx, path, hooks)
			select {
			case done <- openResult{db: db, err: err}:
			case <-ctx.Done():
				if db != nil {
					_ = db.Close()
				}
			}
		}()
		return done
	}
	waitSignal := func(name string, signal <-chan struct{}) {
		t.Helper()
		select {
		case <-signal:
		case <-ctx.Done():
			t.Fatalf("timeout waiting for %s: %v", name, ctx.Err())
		}
	}
	waitOpen := func(name string, done <-chan openResult) openResult {
		t.Helper()
		select {
		case result := <-done:
			return result
		case <-ctx.Done():
			t.Fatalf("timeout waiting for %s opener: %v", name, ctx.Err())
			return openResult{}
		}
	}

	var backfills atomic.Int32
	firstDone := startOpen(openDBAtHooks{
		afterSchema: func() {
			close(firstSchema)
			select {
			case <-startFirstMigration:
			case <-ctx.Done():
			}
		},
		sessionTitleUpgrade: sessionTitleUpgradeHooks{
			afterTransactionOwned: func() {
				close(firstOwned)
				select {
				case <-releaseFirst:
				case <-ctx.Done():
				}
			},
			beforeBackfill: func() { backfills.Add(1) },
		},
	})
	waitSignal("first schema setup", firstSchema)
	secondDone := startOpen(openDBAtHooks{
		afterSchema: func() {
			close(secondSchema)
			select {
			case <-startSecondMigration:
			case <-ctx.Done():
			}
		},
		sessionTitleUpgrade: sessionTitleUpgradeHooks{
			afterFastMissingCheck: func() { close(secondAttempting) },
			beforeBackfill:        func() { backfills.Add(1) },
		},
	})
	waitSignal("second schema setup", secondSchema)
	startFirst()
	waitSignal("first migration ownership", firstOwned)
	startSecond()
	waitSignal("second migration attempt", secondAttempting)
	release()
	first := waitOpen("first", firstDone)
	second := waitOpen("second", secondDone)
	if first.err != nil || second.err != nil {
		if first.db != nil {
			_ = first.db.Close()
		}
		if second.db != nil {
			_ = second.db.Close()
		}
		t.Fatalf("production opener errors: first=%v second=%v", first.err, second.err)
	}
	dbA, dbB := first.db, second.db
	t.Cleanup(func() {
		if dbA != nil {
			_ = dbA.Close()
		}
		if dbB != nil {
			_ = dbB.Close()
		}
	})
	var columns, schemaTables int
	if err := dbA.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'title'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if err := dbA.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'jobs'`).Scan(&schemaTables); err != nil {
		t.Fatal(err)
	}
	var title, updatedAt string
	if err := dbA.QueryRow(`SELECT title, updated_at FROM sessions WHERE id = 'legacy'`).Scan(&title, &updatedAt); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if columns != 1 || schemaTables != 1 || title != "first title" || updatedAt != "2026-08-24T10:00:00Z" ||
		backfills.Load() != 1 || info.Mode().Perm() != 0o600 {
		t.Fatalf(
			"concurrent opener columns=%d schema-tables=%d title=%q updated_at=%q backfills=%d mode=%04o",
			columns, schemaTables, title, updatedAt, backfills.Load(), info.Mode().Perm(),
		)
	}
	if err := dbA.Close(); err != nil {
		t.Fatalf("close first production opener: %v", err)
	}
	dbA = nil
	if err := dbB.Close(); err != nil {
		t.Fatalf("close second production opener: %v", err)
	}
	dbB = nil
}

func TestCurrentSchemaTitleUpgradeFastPathSkipsWriterAndBackfill(t *testing.T) {
	db := newTestDB(t)
	transactionOwned, backfill := false, false
	if err := ensureSessionTitlesWithHooks(context.Background(), db, sessionTitleUpgradeHooks{
		afterTransactionOwned: func() { transactionOwned = true },
		beforeBackfill:        func() { backfill = true },
	}); err != nil {
		t.Fatal(err)
	}
	if transactionOwned || backfill {
		t.Fatalf("current-schema reopen entered migration path: transaction=%t backfill=%t", transactionOwned, backfill)
	}
}

func TestFencedRootEventInitializesTitleWithoutOverwritingOrTouchingUpdatedAt(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	session := mustCreateGlobalSession(t, store)
	originalUpdated := session.UpdatedAt.Format(time.RFC3339Nano)
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, "holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	history := store.BindHistory(session.ID, lease.HolderID)

	if _, err := history.Append(context.Background(), lease, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: " \t\n\x1b\u200b ",
	}); err != nil {
		t.Fatal(err)
	}
	assertNullSessionTitle(t, db, session.ID)

	if _, err := history.Append(context.Background(), lease, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: " live\n\ttitle ",
	}); err != nil {
		t.Fatal(err)
	}
	want := "live title"
	var title, updatedAt string
	if err := db.QueryRow(`SELECT title, updated_at FROM sessions WHERE id = ?`, session.ID).Scan(&title, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if title != want || updatedAt != originalUpdated {
		t.Fatalf("initialized title=%q updated_at=%q, want title=%q updated_at=%q", title, updatedAt, want, originalUpdated)
	}
	if _, err := history.Append(context.Background(), lease, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "later title",
	}); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := db.QueryRow(`SELECT title FROM sessions WHERE id = ?`, session.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != want {
		t.Fatalf("later root overwrote title: %q", after)
	}
}

func TestFailedOrStaleFencedAppendCannotInitializeTitle(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	session := mustCreateGlobalSession(t, store)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	setTurnLeaseTime(store, now)
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, "holder", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEventWithLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken+1, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "stale title",
	}); !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("stale append error=%v", err)
	}
	assertNullSessionTitle(t, db, session.ID)

	samples := 0
	store.now = func() time.Time {
		samples++
		if samples == 1 {
			return now
		}
		return lease.ExpiresAt
	}
	if _, err := store.AppendEventWithLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "rolled back title",
	}); !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("rollback append error=%v", err)
	}
	assertNullSessionTitle(t, db, session.ID)
	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ?`, session.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatalf("rolled-back title append left %d events", events)
	}
}

func assertNullSessionTitle(t *testing.T, db *sql.DB, id memory.SessionID) {
	t.Helper()
	var title sql.NullString
	if err := db.QueryRow(`SELECT title FROM sessions WHERE id = ?`, id).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title.Valid {
		t.Fatalf("session title = %+v, want NULL", title)
	}
}

func mustCreateGlobalSession(t *testing.T, store *Store) memory.Session {
	t.Helper()
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func createLegacyTitleDatabase(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `
		CREATE TABLE projects (
			id TEXT PRIMARY KEY NOT NULL,
			display_name TEXT NOT NULL,
			canonical_root TEXT NOT NULL UNIQUE,
			archived INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY NOT NULL,
			project_id TEXT REFERENCES projects(id),
			project_root_snapshot TEXT,
			parent_session_id TEXT REFERENCES sessions(id),
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE events (
			id TEXT PRIMARY KEY NOT NULL,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			sequence INTEGER NOT NULL,
			project_id TEXT REFERENCES projects(id),
			parent_id TEXT,
			event_type TEXT NOT NULL,
			role TEXT,
			execution_id TEXT,
			content TEXT NOT NULL DEFAULT '',
			payload_json TEXT NOT NULL DEFAULT '{}',
			recorded_at TEXT NOT NULL,
			format_version INTEGER NOT NULL DEFAULT 1,
			UNIQUE(session_id, sequence),
			UNIQUE(id, session_id),
			FOREIGN KEY(parent_id, session_id) REFERENCES events(id, session_id)
		);
		INSERT INTO sessions (id, status, created_at, updated_at)
		VALUES ('legacy', 'active', '2026-08-24T09:00:00Z', '2026-08-24T10:00:00Z');
		INSERT INTO events (id, session_id, sequence, event_type, role, content, recorded_at)
		VALUES
			('assistant', 'legacy', 1, 'assistant_message', 'assistant', 'not user evidence', '2026-08-24T09:00:10Z'),
			('wrong-role', 'legacy', 2, 'user_message', 'assistant', 'wrong role', '2026-08-24T09:00:20Z'),
			('child', 'legacy', 3, 'user_message', 'user', 'child evidence', '2026-08-24T09:00:30Z'),
			('blank', 'legacy', 4, 'user_message', 'user', '  ' || char(10) || char(9), '2026-08-24T09:01:00Z'),
			('title', 'legacy', 5, 'user_message', 'user', 'first' || char(10) || char(9) || 'title', '2026-08-24T09:02:00Z'),
			('later', 'legacy', 6, 'user_message', 'user', 'later', '2026-08-24T09:03:00Z');
		UPDATE events SET parent_id = 'assistant' WHERE id = 'child';
	`
	if _, err := db.Exec(legacy); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}
