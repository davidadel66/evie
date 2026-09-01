package eviedb

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
)

func TestPluginEnabledConfigurationSeedsOncePersistsAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	enabled, revision, err := store.ResolvePluginEnabled(context.Background(), "web", true)
	if err != nil || !enabled || revision != 1 {
		t.Fatalf("fresh default enabled=%v revision=%d err=%v", enabled, revision, err)
	}
	if revision, err = store.SetPluginEnabled(context.Background(), "web", false); err != nil || revision != 2 {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	enabled, revision, err = NewStore(db).ResolvePluginEnabled(context.Background(), "web", true)
	if err != nil || enabled || revision != 2 {
		t.Fatalf("reopened desired enabled=%v revision=%d err=%v, want false@2", enabled, revision, err)
	}
}

func TestPluginEnabledConfigurationConcurrentSeedDoesNotOverwriteDesiredState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	if _, err := store.SetPluginEnabled(context.Background(), "web", false); err != nil {
		t.Fatal(err)
	}

	const readers = 16
	var wg sync.WaitGroup
	results := make(chan bool, readers)
	errs := make(chan error, readers)
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enabled, _, err := store.ResolvePluginEnabled(context.Background(), "web", true)
			results <- enabled
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for enabled := range results {
		if enabled {
			t.Fatal("concurrent default seed overwrote durable disabled state")
		}
	}
}

func TestPluginEnabledConfigurationConcurrentWritersIncrementRevisionWithoutLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	stores := []*Store{NewStore(dbA), NewStore(dbB)}
	if _, _, err := stores[0].ResolvePluginEnabled(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}

	const writers = 32
	type result struct {
		revision uint64
		enabled  bool
		err      error
	}
	results := make(chan result, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enabled := i%2 == 0
			revision, err := stores[i%len(stores)].SetPluginEnabled(context.Background(), "web", enabled)
			results <- result{revision: revision, enabled: enabled, err: err}
		}()
	}
	wg.Wait()
	close(results)
	statesByRevision := make(map[uint64]bool, writers)
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if _, duplicate := statesByRevision[result.revision]; duplicate {
			t.Fatalf("duplicate durable revision %d", result.revision)
		}
		statesByRevision[result.revision] = result.enabled
	}
	for revision := uint64(2); revision <= writers+1; revision++ {
		if _, found := statesByRevision[revision]; !found {
			t.Fatalf("missing durable revision %d: %v", revision, statesByRevision)
		}
	}
	enabled, revision, found, err := stores[1].PluginEnabled(context.Background(), "web")
	if err != nil || !found || revision != writers+1 || enabled != statesByRevision[revision] {
		t.Fatalf("final enabled=%v revision=%d found=%v err=%v", enabled, revision, found, err)
	}
}

func TestPluginEnabledConfigurationMigratesExistingRowsToInitialRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`
		CREATE TABLE plugin_enabled_configuration (
			plugin_id TEXT PRIMARY KEY NOT NULL,
			enabled INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO plugin_enabled_configuration (plugin_id, enabled, updated_at)
		VALUES ('web', 0, '2026-01-01T00:00:00Z');
	`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	enabled, revision, found, err := NewStore(db).PluginEnabled(context.Background(), "web")
	if err != nil || !found || enabled || revision != 1 {
		t.Fatalf("migrated enabled=%v revision=%d found=%v err=%v, want false@1", enabled, revision, found, err)
	}
}
