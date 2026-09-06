package eviedb_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerWorkerLaterCandidatesVisibleAcrossEarlierGap(t *testing.T) {
	f := newCompilerFixture(t)
	ctx := context.Background()
	generation := compilerGeneration()
	generationID, _, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		t.Fatal(err)
	}
	firstSelection := f.selection(t, "I prefer tea.", true)
	secondSelection := f.selection(t, "I prefer coffee.", true)
	first, err := f.store.QueueCandidateUnit(ctx, f.session.ScopeContext(), firstSelection, generation, &scriptedCompiler{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.QueueCandidateUnit(ctx, f.session.ScopeContext(), secondSelection, generation, &scriptedCompiler{})
	if err != nil {
		t.Fatal(err)
	}
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		if r.Window.Selection.RootID == firstSelection.RootID {
			return eviedb.CompilerExtraction{Raw: []byte("null"), ReleaseEvidence: "completed"}, nil
		}
		return compilerOutput(r, []memory.ExtractorCandidate{f.candidate(r)}), nil
	}}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{generationID: extractor}}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || err == nil {
		t.Fatalf("missing output treated as success: %v %v", worked, err)
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || err != nil {
		t.Fatalf("later work stalled: %v %v", worked, err)
	}
	visible, err := f.store.InspectCompilation(ctx, f.session.ScopeContext(), second.JobID)
	if err != nil || visible.State != "completed_candidates" || len(visible.Candidates) != 1 {
		t.Fatalf("later candidates %+v %v", visible, err)
	}
	earlier, err := f.store.InspectCompilation(ctx, f.session.ScopeContext(), first.JobID)
	if err != nil || earlier.State != "retry_wait" || len(earlier.Candidates) != 0 {
		t.Fatalf("gap disappeared %+v %v", earlier, err)
	}
	var firstCount int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_coverage WHERE job_id=?`, first.JobID).Scan(&firstCount); err != nil || firstCount != 0 {
		t.Fatalf("false coverage %d %v", firstCount, err)
	}
	var raw []byte
	if err := f.db.QueryRow(`SELECT event_ids FROM memory_compiler_coverage WHERE job_id=?`, second.JobID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var covered []memory.EventID
	if err := json.Unmarshal(raw, &covered); err != nil {
		t.Fatal(err)
	}
	if len(covered) != len(second.Window.NewEventIDs) {
		t.Fatalf("wrong exact interval %v", covered)
	}
	for i, id := range covered {
		if id != second.Window.NewEventIDs[i] {
			t.Fatalf("coverage changed selected source %v", covered)
		}
	}
	accepted, err := f.store.InspectLiteralClaims(ctx, f.session.ScopeContext())
	if err != nil || len(accepted.Claims) != 1 || accepted.ScopeRevision != 1 {
		t.Fatalf("worker changed accepted state %+v %v", accepted, err)
	}
}

func TestCompilerWorkerSealsAcceptedContextOnFirstAttempt(t *testing.T) {
	f := newCompilerFixture(t)
	ctx := context.Background()
	generation := compilerGeneration()
	id, _, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		t.Fatal(err)
	}
	selected := f.selection(t, "I prefer tea.", true)
	job, err := f.store.QueueCandidateUnit(ctx, f.session.ScopeContext(), selected, generation, &scriptedCompiler{})
	if err != nil {
		t.Fatal(err)
	}
	addAccepted := func(key, value string) {
		source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "/remember " + value})
		proposal, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: key, SourceEventID: source.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.ApplyRememberLiteral(ctx, f.lease, proposal); err != nil {
			t.Fatal(err)
		}
	}
	addAccepted("idem:v1:90000000-0000-4000-8000-000000000002", "coffee")
	var firstRequest []byte
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		if len(r.ScopeRevisions) != 1 || r.ScopeRevisions[0].Revision != 2 {
			t.Errorf("snapshot was not taken on attempt one: %+v", r.ScopeRevisions)
		}
		sealed, _ := json.Marshal(r)
		if firstRequest == nil {
			firstRequest = sealed
			return eviedb.CompilerExtraction{Raw: []byte("null"), ReleaseEvidence: "completed"}, nil
		}
		if string(sealed) != string(firstRequest) {
			t.Error("retry refreshed accepted context")
		}
		return compilerOutput(r, []memory.ExtractorCandidate{}), nil
	}}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: extractor}}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || err == nil {
		t.Fatalf("first attempt %v %v", worked, err)
	}
	addAccepted("idem:v1:90000000-0000-4000-8000-000000000003", "water")
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET retry_at=unixepoch('now') WHERE job_id=?`, job.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET request='{}' WHERE job_id=?`, job.JobID); err == nil {
		t.Fatal("database allowed rewriting sealed request")
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || err != nil {
		t.Fatalf("retry %v %v", worked, err)
	}
}
