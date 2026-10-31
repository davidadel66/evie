package main

import (
	"database/sql"
	"path/filepath"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/youtube"
)

// evieDatabases are Evie's databases, each created by its package's real
// open-and-migrate entry point inside a throwaway directory. Finance's
// entry point takes its path from the home directory, which the tool points
// at that directory while it runs.
var evieDatabases = []databaseSource{
	{ID: "evie", Package: "internal/eviedb", Open: func(dir string) (*sql.DB, error) {
		return eviedb.OpenDBAt(filepath.Join(dir, "evie.db"))
	}},
	{ID: "finance", Package: "internal/finance", Open: func(string) (*sql.DB, error) {
		return finance.OpenDB()
	}},
	{ID: "youtube", Package: "internal/youtube", Open: func(dir string) (*sql.DB, error) {
		return youtube.OpenDBAt(filepath.Join(dir, "transcripts.db"))
	}},
}
