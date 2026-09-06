package eviedb_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerRecurrenceBoundedLegacyMigrationPreservesBytes(t *testing.T) {
	f, first, _ := reviewCandidateFixture(t)
	for seed := 100; seed < 163; seed++ {
		repeatRecurrence(t, f, first, seed, first.Candidates[0].Proposal)
	}
	snapshot := func(db *sql.DB) string {
		t.Helper()
		rows, err := db.Query(`SELECT candidate_id,hex(envelope),review_state,review_revision,COALESCE(equivalent_to,'') FROM memory_compiler_candidates ORDER BY candidate_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out strings.Builder
		for rows.Next() {
			var id, envelope, state, primary string
			var revision int64
			if err = rows.Scan(&id, &envelope, &state, &revision, &primary); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "%s|%s|%s|%d|%s\n", id, envelope, state, revision, primary)
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	original := snapshot(f.db)
	// Remove only #147's side projection to reproduce a populated pre-upgrade DB.
	if _, err := f.db.Exec(`DROP TABLE memory_compiler_recurrence;DROP TABLE memory_compiler_recurrence_migration`); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{31, 62, 64, 64} {
		db, err := eviedb.OpenDBAt(f.path)
		if err != nil {
			t.Fatal(err)
		}
		var got int
		if err = db.QueryRow(`SELECT count(*) FROM memory_compiler_recurrence`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("migration read/mutation page=%d want=%d", got, want)
		}
		if snapshot(db) != original {
			t.Fatal("migration rewrote retained output or terminal metadata")
		}
		if _, err = db.Exec(`UPDATE memory_compiler_recurrence SET checked_review=999`); err == nil {
			t.Fatal("mutable publication lineage")
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	repeated := repeatRecurrence(t, f, first, 200, first.Candidates[0].Proposal)
	if repeated.Candidates[0].EquivalentTo != first.Candidates[0].ID {
		t.Fatal("legacy primary not retained")
	}
}

func TestCompilerRecurrenceLookupPlansStayIndexed(t *testing.T) {
	f, first, _ := reviewCandidateFixture(t)
	// These are inert retained metadata rows for the query planner, not staged
	// outputs or accepted operations. Publication still validates real envelopes.
	_, err := f.db.Exec(`WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000) INSERT INTO memory_compiler_candidates(candidate_id,job_id,ordinal,envelope,equivalence_hash) SELECT printf('metadata-%05d',x),?,100+x,'{}',printf('hash-%d',x) FROM n`, first.JobID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.db.Exec(`INSERT INTO memory_compiler_recurrence(candidate_id,encoding_version,exact_hash,related_hash,exact_encoding,related_encoding,relationship,suppressed,checked_interpretation,checked_review,checked_state) SELECT candidate_id,'compiler-recurrence-v2',equivalence_hash,equivalence_hash,'{}','{}','primary',0,0,0,'unresolved' FROM memory_compiler_candidates WHERE candidate_id LIKE 'metadata-%'`)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ query, index string }{
		{`SELECT candidate_id,exact_encoding FROM memory_compiler_recurrence WHERE exact_hash='x' AND suppressed=0 ORDER BY presentation_epoch DESC,publication_order LIMIT 1`, "memory_compiler_recurrence_exact"},
		{`SELECT candidate_id,related_encoding FROM memory_compiler_recurrence WHERE related_hash='x' AND suppressed=0 ORDER BY publication_order DESC LIMIT 1`, "memory_compiler_recurrence_related"},
		{`SELECT candidate_id FROM memory_compiler_candidates WHERE equivalence_hash='x' AND equivalent_to IS NULL ORDER BY candidate_id LIMIT 1`, "memory_compiler_legacy_recurrence"},
		{`SELECT c.rowid,c.candidate_id,c.envelope,j.request,g.manifest,COALESCE(c.equivalent_to,'') FROM memory_compiler_candidates c JOIN memory_compiler_jobs j ON j.job_id=c.job_id JOIN memory_compiler_generations g ON g.generation_id=j.generation_id WHERE c.rowid>1000 ORDER BY c.rowid LIMIT 31`, "INTEGER PRIMARY KEY"},
	}
	for _, test := range cases {
		rows, err := f.db.Query("EXPLAIN QUERY PLAN " + test.query)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(detail + "\n")
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(plan.String(), test.index) || strings.Contains(plan.String(), "SCAN ") || strings.Contains(plan.String(), "TEMP B-TREE") {
			t.Fatalf("unbounded recurrence lookup:\n%s", plan.String())
		}
	}
}

func TestCompilerRecurrencePublicationRollbackAndStageAdoption(t *testing.T) {
	ctx := context.Background()
	f, first, a := reviewCandidateFixture(t)
	g := compilerGeneration()
	g.Decoding.Seed = 400
	if _, err := f.db.Exec(`CREATE TRIGGER fail_recurrence BEFORE INSERT ON memory_compiler_recurrence BEGIN SELECT RAISE(ABORT,'injected recurrence insert failure'); END`); err != nil {
		t.Fatal(err)
	}
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return compilerOutput(r, []memory.ExtractorCandidate{first.Candidates[0].Proposal}), nil
	}}
	if _, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), first.Window.Selection, g, extractor); err == nil {
		t.Fatal("publication failure hidden")
	}
	var candidates, coverage, stages int
	if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_compiler_candidates),(SELECT count(*) FROM memory_compiler_coverage),(SELECT count(*) FROM memory_compiler_stages WHERE consumed=0)`).Scan(&candidates, &coverage, &stages); err != nil {
		t.Fatal(err)
	}
	if candidates != 1 || coverage != 1 || stages != 1 {
		t.Fatalf("partial candidate/lineage/coverage publication: %d/%d/%d", candidates, coverage, stages)
	}
	if _, err := f.db.Exec(`DROP TRIGGER fail_recurrence;UPDATE memory_compiler_jobs SET lease_until=0 WHERE state='staged'`); err != nil {
		t.Fatal(err)
	}
	id, _, _ := memory.CompilerGenerationIdentity(g)
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: extractor}}
	if worked, err := f.store.RunCompilerStep(ctx, config); err != nil || !worked || extractor.calls.Load() != 1 {
		t.Fatalf("staged adoption %v %v calls=%d", worked, err, extractor.calls.Load())
	}
	page, err := f.store.ListOwnerCandidates(ctx, a, memory.OwnerCandidateQuery{})
	if err != nil || len(page.Candidates) != 1 {
		t.Fatalf("atomic primary inbox %+v %v", page, err)
	}
}
