package eviedb_test

import (
	"context"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type historyPublicScript struct{ scriptedCompiler }

func (*historyPublicScript) VerifyCompilerConfiguration(context.Context, memory.CompilerGeneration) error {
	return nil
}

func TestCompilerHistoryLaterCandidatesReviewableAcrossEarlierFailure(t *testing.T) {
	f := newCompilerFixture(t)
	ctx := context.Background()
	first := f.selection(t, "I prefer tea.", true)
	second := f.selection(t, "I prefer café.", true)
	var firstSequence int64
	var endID memory.EventID
	if err := f.db.QueryRow(`SELECT sequence FROM events WHERE id=?`, first.RootID).Scan(&firstSequence); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT id FROM events WHERE session_id=? AND sequence=?`, second.SessionID, second.Cutoff).Scan(&endID); err != nil {
		t.Fatal(err)
	}
	req := memory.CompilerHistoryRequest{RequestID: "two-roots", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: "global", SessionID: f.session.ID, FirstSequence: firstSequence, LastSequence: second.Cutoff, FirstEventID: first.RootID, LastEventID: endID}}}
	extractor := &historyPublicScript{scriptedCompiler: scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		if r.Window.Selection.RootID == first.RootID {
			return eviedb.CompilerExtraction{ReleaseEvidence: "completed"}, eviedb.ErrCompilerTerminalOutput
		}
		return compilerOutput(r, []memory.ExtractorCandidate{f.candidate(r)}), nil
	}}}
	generation := compilerGeneration()
	id, _, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		t.Fatal(err)
	}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: extractor}}
	if _, err := f.store.SelectCompilerHistory(ctx, []memory.ScopeContext{f.session.ScopeContext()}, req, generation, extractor); err != nil {
		t.Fatal(err)
	}
	for range 8 {
		if _, err := f.store.ReconcileCompilerHistory(ctx, config); err != nil {
			t.Fatal(err)
		}
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || !errors.Is(err, eviedb.ErrCompilerTerminalOutput) {
		t.Fatal(worked, err)
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || err != nil {
		t.Fatal(worked, err)
	}
	before, err := f.store.InspectCompilerHistory(ctx, []memory.ScopeContext{f.session.ScopeContext()}, req.RequestID, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Intervals) != 1 || before.Intervals[0].State != "failed" || before.NextSequence == 0 || before.ContiguousFrontier != firstSequence-1 {
		t.Fatalf("first page %+v", before)
	}
	next, err := f.store.InspectCompilerHistory(ctx, []memory.ScopeContext{f.session.ScopeContext()}, req.RequestID, 0, before.NextSequence, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Intervals) != 1 || next.Intervals[0].State != "completed_candidates" || next.NextSequence != 0 {
		t.Fatalf("later candidates %+v", next)
	}
	compiled, err := f.store.InspectCompilation(ctx, f.session.ScopeContext(), next.Intervals[0].JobID)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, owner, candidateRef(compiled), "reject")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ResolveOwnerCandidateReview(ctx, owner, decisionFor(preview, "90000000-0000-4000-8000-000000000139")); err != nil {
		t.Fatal(err)
	}
	after, err := f.store.InspectCompilerHistory(ctx, []memory.ScopeContext{f.session.ScopeContext()}, req.RequestID, 0, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	if after.ContiguousFrontier != before.ContiguousFrontier || after.Intervals[1].State != "completed_candidates" {
		t.Fatal("review changed coverage")
	}
}
