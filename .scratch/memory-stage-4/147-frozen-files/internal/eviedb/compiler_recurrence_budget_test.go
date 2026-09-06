package eviedb

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerRecurrenceSixteenCandidatePublicationMutationBound(t *testing.T) {
	ctx := context.Background()
	f, _, refs := batchBoundaryFixture(t, 16)
	var job string
	if err := f.db.QueryRow(`SELECT job_id FROM memory_compiler_candidates WHERE candidate_id=?`, refs[0].ID).Scan(&job); err != nil {
		t.Fatal(err)
	}
	first, err := f.store.InspectCompilation(ctx, f.owner, job)
	if err != nil {
		t.Fatal(err)
	}
	generation := f.generation
	generation.Decoding.Seed = 700
	queued, err := f.store.QueueCandidateUnit(ctx, f.owner, first.Window.Selection, generation, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.claimCompilerJob(ctx, f.owner, queued.JobID, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	proposals := []memory.ExtractorCandidate{}
	for _, candidate := range first.Candidates {
		proposals = append(proposals, candidate.Proposal)
	}
	validated, err := validateCompilerOutput(claim.Request, compilerJSON(memory.CompilerResponse{RequestID: claim.Request.ID, Candidates: proposals}))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.stageCompilerResult(ctx, f.owner, claim.JobID, claim.Holder, claim.Fence, claim.Request, validated); err != nil {
		t.Fatal(err)
	}
	// There is no model call, lease heartbeat or other writer in this fixture.
	// One pooled connection makes total_changes include all trigger mutations.
	f.db.SetMaxOpenConns(1)
	f.db.SetMaxIdleConns(1)
	var before, after int64
	if err = f.db.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err = f.store.publishCompilerResult(ctx, f.owner, claim.JobID, claim.Holder, claim.Fence, claim.Request); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if delta := after - before; delta > 64 {
		t.Fatalf("publication mutations=%d exceed bound64 (including trigger writes)", delta)
	}
	t.Logf("publication mutations including triggers: %d", after-before)
	var count int
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_compiler_recurrence`).Scan(&count); err != nil || count != 32 {
		t.Fatal("incomplete atomic group", count, err)
	}
	var candidate memory.MemoryCandidate
	var raw []byte
	if err = f.db.QueryRow(`SELECT envelope FROM memory_compiler_candidates WHERE job_id=? LIMIT 1`, queued.JobID).Scan(&raw); err != nil || json.Unmarshal(raw, &candidate) != nil || candidate.EquivalentTo == "" {
		t.Fatalf("suppressed original: %s %v", raw, err)
	}
}

func TestCompilerRecurrenceMigrationFailureRollsBackPageAndCursor(t *testing.T) {
	ctx := context.Background()
	f, _, _ := batchBoundaryFixture(t, 32)
	if _, err := f.db.Exec(`DROP TABLE memory_compiler_recurrence;DROP TABLE memory_compiler_recurrence_migration;` + compilerRecurrenceSchema + `CREATE TRIGGER fail_legacy_recurrence BEFORE INSERT ON memory_compiler_recurrence WHEN NEW.publication_order>1 BEGIN SELECT RAISE(ABORT,'injected migration failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if err := ensureCandidateReviewSchema(ctx, f.db); err == nil {
		t.Fatal("migration failure hidden")
	}
	var count, cursor int
	if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_compiler_recurrence),(SELECT last_rowid FROM memory_compiler_recurrence_migration WHERE singleton=1)`).Scan(&count, &cursor); err != nil || count != 0 || cursor != 0 {
		t.Fatalf("partial migration %d %d %v", count, cursor, err)
	}
	if _, err := f.db.Exec(`DROP TRIGGER fail_legacy_recurrence`); err != nil {
		t.Fatal(err)
	}
	if err := ensureCandidateReviewSchema(ctx, f.db); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT count(*) FROM memory_compiler_recurrence`).Scan(&count); err != nil || count != 31 {
		t.Fatal("retry page", count, err)
	}
}
