package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
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

	dbA, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbA.Close() })
	dbB, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbB.Close() })

	firstOwned := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondAttempting := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	var backfills atomic.Int32
	go func() {
		firstDone <- ensureSessionTitlesWithHooks(context.Background(), dbA, sessionTitleUpgradeHooks{
			afterTransactionOwned: func() {
				close(firstOwned)
				<-releaseFirst
			},
			beforeBackfill: func() { backfills.Add(1) },
		})
	}()
	<-firstOwned
	go func() {
		secondDone <- ensureSessionTitlesWithHooks(context.Background(), dbB, sessionTitleUpgradeHooks{
			afterFastMissingCheck: func() { close(secondAttempting) },
			beforeBackfill:        func() { backfills.Add(1) },
		})
	}()
	<-secondAttempting
	select {
	case err := <-secondDone:
		t.Fatalf("competing migration returned before owner released: %v", err)
	default:
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second migration: %v", err)
	}

	var columns int
	if err := dbA.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'title'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	var title, updatedAt string
	if err := dbA.QueryRow(`SELECT title, updated_at FROM sessions WHERE id = 'legacy'`).Scan(&title, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if columns != 1 || title != "first title" || updatedAt != "2026-08-24T10:00:00Z" || backfills.Load() != 1 {
		t.Fatalf("concurrent migration columns=%d title=%q updated_at=%q backfills=%d", columns, title, updatedAt, backfills.Load())
	}
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

	long := strings.Repeat("界", 81) + "\nignored"
	if _, err := history.Append(context.Background(), lease, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: long,
	}); err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("界", 79) + "…"
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
			('title', 'legacy', 5, 'user_message', 'user', ' first' || char(10) || ' title ', '2026-08-24T09:02:00Z'),
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
