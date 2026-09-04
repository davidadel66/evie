package main

import (
	"path/filepath"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
)

func testTaskStore(t *testing.T) *eviedb.Store {
	t.Helper()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return eviedb.NewStore(db)
}
