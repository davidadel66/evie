package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
)

func evieQueryArgs(t *testing.T, query string) string {
	t.Helper()

	data, err := json.Marshal(map[string]string{
		"db":    "evie",
		"query": query,
	})
	if err != nil {
		t.Fatalf("marshal query arguments: %v", err)
	}
	return string(data)
}

func TestQueryDBEvieAllowsOnlyPublicTables(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db, err := eviedb.OpenDB()
	if err != nil {
		t.Fatalf("open isolated Evie database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	if _, err := db.Exec(`
		INSERT INTO sessions (id, created_at, updated_at)
		VALUES ('private-session', '2026-08-17T12:00:00Z', '2026-08-17T12:00:00Z');
		INSERT INTO events (
			id, session_id, sequence, event_type, role, content, recorded_at
		) VALUES (
			'private-event', 'private-session', 1, 'user_message', 'user',
			'private conversation', '2026-08-17T12:00:00Z'
		);
	`); err != nil {
		t.Fatalf("seed private session event: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO jobs (name, schedule, command, created_at)
		VALUES ('public-job', '* * * * *', 'true', '2026-08-17T12:00:00Z')
	`); err != nil {
		t.Fatalf("seed public job: %v", err)
	}

	out, err := queryDB(context.Background(), evieQueryArgs(t, `SELECT name FROM jobs`))
	if err != nil {
		t.Fatalf("query public jobs table: %v", err)
	}
	if !strings.Contains(out, "public-job") {
		t.Fatalf("public jobs query output = %q, want seeded job", out)
	}

	out, err = queryDB(context.Background(), evieQueryArgs(t, `
		SELECT jobs.name
		FROM jobs
		LEFT JOIN job_runs ON job_runs.job_id = jobs.id
	`))
	if err != nil {
		t.Fatalf("join public Evie tables: %v", err)
	}
	if !strings.Contains(out, "public-job") {
		t.Fatalf("public join output = %q, want seeded job", out)
	}

	privateQueries := []struct {
		name  string
		query string
	}{
		{name: "direct memory table", query: `SELECT content FROM events`},
		{name: "joined memory table", query: `SELECT jobs.name FROM jobs JOIN events ON 1 = 1`},
		{name: "implicit memory join", query: `SELECT jobs.name FROM jobs, events`},
		{name: "nested memory query", query: `SELECT (SELECT content FROM events) FROM jobs`},
		{name: "SQLite schema", query: `SELECT name FROM sqlite_schema`},
		{name: "qualified memory table", query: `SELECT content FROM main.events`},
		{name: "quoted memory table", query: `SELECT content FROM "events"`},
		{name: "semantic scopes", query: `SELECT scope_key FROM semantic_scopes`},
		{name: "semantic operations", query: `SELECT operation_id FROM semantic_operations`},
		{name: "semantic operation scopes", query: `SELECT operation_id FROM semantic_operation_scopes`},
		{name: "semantic predicates", query: `SELECT token FROM semantic_predicates`},
		{name: "semantic entities", query: `SELECT canonical_name FROM semantic_entities`},
		{name: "semantic claims", query: `SELECT literal_value FROM semantic_claims`},
		{name: "semantic source links", query: `SELECT evidence_sha256 FROM semantic_source_links`},
		{name: "semantic state events", query: `SELECT state FROM semantic_state_events`},
	}

	for _, tt := range privateQueries {
		t.Run(tt.name, func(t *testing.T) {
			out, err := queryDB(context.Background(), evieQueryArgs(t, tt.query))
			if err == nil {
				t.Fatalf("private query succeeded with output %q", out)
			}
		})
	}
}
