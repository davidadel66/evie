package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
)

func TestCompilerStorageDeniedByGenericSQL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db, err := eviedb.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tables := []string{"memory_compiler_position_counter", "memory_compiler_event_positions", "memory_compiler_event_coordinates", "memory_compiler_generations", "memory_compiler_selections", "memory_compiler_jobs", "memory_compiler_capacity", "memory_compiler_stages", "memory_compiler_candidate_groups", "memory_compiler_candidates", "memory_compiler_coverage"}
	for _, table := range tables {
		for _, query := range []string{fmt.Sprintf("SELECT * FROM %s", table), fmt.Sprintf("SELECT * FROM main.%s", table), fmt.Sprintf("SELECT * FROM \"%s\"", table), fmt.Sprintf("SELECT jobs.name FROM jobs JOIN %s ON 1=1", table), fmt.Sprintf("SELECT (SELECT COUNT(*) FROM %s) FROM jobs", table)} {
			if out, err := queryDB(context.Background(), evieQueryArgs(t, query)); err == nil {
				t.Fatalf("compiler storage leaked via %s: %s", query, out)
			}
		}
	}
}
