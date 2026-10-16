package eviedb

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// openIndexedStore opens a database at path with both lexical indexes active
// and dense retrieval off.
func openIndexedStore(tb testing.TB, path string) *Store {
	tb.Helper()
	tb.Setenv("EVIE_REMOTE_MEMORY", "on")
	tb.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	db, err := OpenDBAt(path)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	store := NewStore(db)
	refreshUntilActive(tb, store)
	return store
}

// appendIndexedMessages appends alternating user and assistant messages in one
// transaction, so each is projected as it is appended, as in production.
func appendIndexedMessages(tb testing.TB, store *Store, session memory.SessionID, first, count int) {
	tb.Helper()
	ctx := context.Background()
	words := []string{"plan", "run", "today", "dinner", "car", "week", "book", "test", "trip", "garden", "budget", "call"}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		tb.Fatal(err)
	}
	for n := first; n < first+count; n++ {
		role, kind := memory.RoleUser, memory.EventUserMessage
		if n%2 == 1 {
			role, kind = memory.RoleAssistant, memory.EventAssistantMessage
		}
		text := fmt.Sprintf("Message %d: %s the %s before the %s.", n, words[n%len(words)], words[(n/3)%len(words)], words[(n/7)%len(words)])
		if _, err := store.appendEvent(ctx, txEventExecutor{tx}, session, memory.EventInput{Type: kind, Role: role, Content: text}); err != nil {
			tx.Rollback()
			tb.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
}

// conversationIndexCorpusAt fills an active conversation index at path with
// Global messages, 500 to a session.
func conversationIndexCorpusAt(tb testing.TB, path string, messages int) *Store {
	tb.Helper()
	store := openIndexedStore(tb, path)
	for made := 0; made < messages; made += 500 {
		session, err := store.CreateGlobalSession(context.Background())
		if err != nil {
			tb.Fatal(err)
		}
		appendIndexedMessages(tb, store, session.ID, made, min(500, messages-made))
	}
	refreshUntilActive(tb, store)
	return store
}

func conversationIndexCorpus(tb testing.TB, messages int) *Store {
	tb.Helper()
	return conversationIndexCorpusAt(tb, filepath.Join(tb.TempDir(), "evie.db"), messages)
}

func refreshUntilActive(tb testing.TB, store *Store) {
	tb.Helper()
	for {
		coverage, err := store.RefreshMemoryIndex(context.Background(), 256)
		if err != nil {
			tb.Fatal(err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			return
		}
	}
}

// BenchmarkConversationIndexRefreshBatch times one full refresh batch on a
// 50,000-message index: each of the 256 messages already has a row, which
// the refresh must find and delete before it writes the new one.
func BenchmarkConversationIndexRefreshBatch(b *testing.B) {
	store := conversationIndexCorpus(b, 50000)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if _, err := store.db.ExecContext(ctx, `INSERT OR IGNORE INTO memory_retrieval_event_dirty
 SELECT id FROM events WHERE event_type IN ('user_message','assistant_message') ORDER BY rowid LIMIT 256 OFFSET ?`, (i*256)%49000); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err := store.refreshConversationIndex(ctx, 256); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkConversationIndexCoverage times the coverage read on the same
// index, which every refresh batch runs twice. It is reported beside the
// batch so the delete cost can be told apart from the coverage cost.
func BenchmarkConversationIndexCoverage(b *testing.B) {
	store := conversationIndexCorpus(b, 50000)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conversationIndexCoverage(ctx, store.db); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOpenIndexedDatabase times opening a database whose 50,000-message
// index is already reconciled: the lookup check must stay a constant cost.
func BenchmarkOpenIndexedDatabase(b *testing.B) {
	path := filepath.Join(b.TempDir(), "evie.db")
	conversationIndexCorpusAt(b, path, 50000).db.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db, err := OpenDBAt(path)
		if err != nil {
			b.Fatal(err)
		}
		db.Close()
	}
}
