package eviedb

import (
	"context"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// holdWriteLock takes SQLite's write lock on another connection until the
// test ends, so anything that tries to write waits for it.
func holdWriteLock(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.ExecContext(context.Background(), `ROLLBACK`)
		conn.Close()
	})
}

// requireNoWrite fails if run waits on the held write lock.
func requireNoWrite(t *testing.T, what string, run func(context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	if err := run(ctx); err != nil {
		t.Fatalf("%s while another connection held the write lock: %v", what, err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("%s waited %s for the write lock", what, elapsed)
	}
}

// The maintenance loop ticks once a second. With nothing to index, a tick
// must not take the write lock that turns need.
func TestIdleRetrievalMaintenanceTakesNoWriteLock(t *testing.T) {
	t.Run("dense retrieval off", func(t *testing.T) {
		t.Setenv("EVIE_REMOTE_MEMORY", "on")
		t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
		ctx := context.Background()
		store := NewStore(newTestDB(t))
		session, err := store.CreateGlobalSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.appendEvent(ctx, txEventExecutor{tx}, session.ID, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Plan the garden."}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		for {
			coverage, err := store.RefreshMemoryIndex(ctx, 256)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.State == "active" && coverage.Pending == 0 {
				break
			}
		}
		holdWriteLock(t, store)
		requireNoWrite(t, "an idle maintenance tick", func(ctx context.Context) error {
			_, err := store.RefreshMemoryIndex(ctx, 256)
			return err
		})
	})

	t.Run("dense retrieval on and selected", func(t *testing.T) {
		t.Setenv("EVIE_REMOTE_MEMORY", "on")
		t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "http://127.0.0.1:9")
		store := NewStore(newTestDB(t))
		if _, err := store.ensureDenseGeneration(context.Background()); err != nil {
			t.Fatal(err)
		}
		holdWriteLock(t, store)
		requireNoWrite(t, "checking an already selected dense generation", func(ctx context.Context) error {
			_, err := store.ensureDenseGeneration(ctx)
			return err
		})
	})

	t.Run("turning dense retrieval off still disables it", func(t *testing.T) {
		t.Setenv("EVIE_REMOTE_MEMORY", "on")
		t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "http://127.0.0.1:9")
		store := NewStore(newTestDB(t))
		if _, err := store.ensureDenseGeneration(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
		if _, err := store.ensureDenseGeneration(context.Background()); err != nil {
			t.Fatal(err)
		}
		var enabled int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM memory_dense_selection WHERE enabled=1`).Scan(&enabled); err != nil || enabled != 0 {
			t.Fatalf("dense selection still enabled (%d, %v)", enabled, err)
		}
	})
}
