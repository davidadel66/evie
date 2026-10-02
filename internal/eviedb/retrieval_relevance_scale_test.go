package eviedb

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type txEventExecutor struct{ tx *sql.Tx }

func (e txEventExecutor) queryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return e.tx.QueryRowContext(ctx, query, args...)
}

// relevanceScaleHistory builds Global history whose common words recur in
// thousands of messages, as a long-running assistant's do. The index is made
// active first so each message is projected as it is appended, as in
// production, instead of backfilled afterwards.
func relevanceScaleHistory(t *testing.T, messages int) (*Store, memory.Session) {
	t.Helper()
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	store := NewStore(newTestDB(t))
	ctx := context.Background()
	refresh := func() {
		for {
			coverage, err := store.RefreshMemoryIndex(ctx, 256)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.State == "active" && coverage.Pending == 0 {
				return
			}
		}
	}
	refresh()
	rng := rand.New(rand.NewPCG(20261001, 1))
	common := []string{"plan", "run", "today", "dinner", "car", "week", "book", "test", "trip", "garden", "budget", "call", "morning", "list", "check"}
	rare := []string{"lisbon", "alfama", "ferritin", "saffron", "crocus", "booster", "biscuit", "tyre", "revolut", "passport"}
	for made := 0; made < messages; {
		session, err := store.CreateGlobalSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 500 && made < messages; i, made = i+1, made+1 {
			text := fmt.Sprintf("Please %s the %s and the %s for %s.", common[rng.IntN(len(common))], common[rng.IntN(len(common))], common[rng.IntN(len(common))], common[rng.IntN(len(common))])
			if rng.IntN(100) == 0 {
				text += " Also the " + rare[rng.IntN(len(rare))] + "."
			}
			role, kind := memory.RoleUser, memory.EventUserMessage
			if i%2 == 1 {
				role, kind = memory.RoleAssistant, memory.EventAssistantMessage
			}
			if _, err := store.appendEvent(ctx, txEventExecutor{tx}, session.ID, memory.EventInput{Type: kind, Role: role, Content: text}); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	refresh()
	reader, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return store, reader
}

// Harness review final pass, finding 4: the floor counted every occurrence of
// every request word, one query each, so at 20k messages a follow-up spent
// most of the 500 ms read deadline counting, and past it the search maps to
// exhausted and refuses later reads in the turn. Counts are now exact only
// below relevanceCommonDocuments, which is all the floor distinguishes.
func TestRelevanceFrequenciesStayFastAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale timing skipped in -short mode")
	}
	store, reader := relevanceScaleHistory(t, 20000)
	ctx := context.Background()
	// A follow-up's plan: its own words, two earlier topics and continuity,
	// most of them words that occur in thousands of messages.
	relevance := memory.RetrievalRelevance{
		Current: []string{"plan", "run", "today", "dinner", "lisbon", "dates"},
		Context: [][]string{{"car", "week", "book", "test", "trip"}, {"garden", "budget", "call", "morning", "list"}, {"check", "ferritin", "saffron", "crocus", "booster", "biscuit"}},
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	// The fastest of three runs measures the work, not a busy machine.
	counting := time.Duration(1<<63 - 1)
	var frequency map[string]int
	for i := 0; i < 3; i++ {
		started := time.Now()
		frequency, err = relevanceFrequencies(ctx, tx, &relevance, "global", time.Now().UTC(), reader.ID, 0)
		counting = min(counting, time.Since(started))
		if err != nil {
			t.Fatal(err)
		}
	}
	var exact int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM memory_retrieval_event_fts_v3 WHERE memory_retrieval_event_fts_v3 MATCH '"ferritin"'`).Scan(&exact); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	if frequency["plan"] != relevanceCommonDocuments || frequency["dates"] != 0 || exact == 0 || exact >= relevanceCommonDocuments || frequency["ferritin"] != exact {
		t.Fatalf("frequencies must be exact below the common bound and saturate at it: plan=%d dates=%d ferritin=%d (exact %d)", frequency["plan"], frequency["dates"], frequency["ferritin"], exact)
	}
	query := memory.RetrievalQuery{Kind: memory.RetrievalConversationExcerpt, Text: "plan run today dinner lisbon dates car week book test trip garden budget call morning list check", Limit: 2, MaxBytes: 6 * 1024, ExcludeCurrentRequestCopies: true, Relevance: &relevance}
	searching := time.Duration(1<<63 - 1)
	for i := 0; i < 3; i++ {
		started := time.Now()
		result, err := store.SearchMemory(ctx, reader.ScopeContext(), query)
		elapsed := time.Since(started)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status == memory.RetrievalExhausted || result.Status == memory.RetrievalFailed {
			t.Fatalf("search at scale ended %s after %s", result.Status, elapsed)
		}
		searching = min(searching, elapsed)
	}
	t.Logf("20k messages: frequency counting %s, search %s (deadline %s)", counting, searching, retrievalDeadline)
	if counting > retrievalDeadline/10 || searching > retrievalDeadline/2 {
		t.Fatalf("at 20k messages counting took %s and search %s; the read deadline is %s", counting, searching, retrievalDeadline)
	}
}
