package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

func TestStage4HistoryGapEmptyAndGenerationConformance(t *testing.T) {
	f := newStage4ConformanceFixture(t)
	f.generation.EquivalencePolicy = memory.CompilerEquivalencePolicyV2
	var err error
	f.generationID, _, err = memory.CompilerGenerationIdentity(f.generation)
	if err != nil {
		t.Fatal(err)
	}
	outside := f.foreground("This earlier conversation remains outside selected compilation.")
	gapSelection := f.foreground("I prefer tea but this scripted compilation will fail.")
	gap, err := f.store.QueueCandidateUnit(f.ctx, f.session.ScopeContext(), gapSelection, f.generation, f.extractor("tea"))
	if err != nil {
		t.Fatal(err)
	}
	failed := f.extractor("")
	failed.run = func(_ context.Context, _ memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return eviedb.CompilerExtraction{ReleaseEvidence: "completed"}, eviedb.ErrCompilerTerminalOutput
	}
	if worked, err := f.store.RunCompilerStep(f.ctx, f.config(failed)); !worked || err == nil {
		t.Fatalf("expected terminal scripted gap %v %v", worked, err)
	}
	emptySelection := f.foreground("No durable memory is requested from this small talk.")
	empty := f.compile(emptySelection, f.generation, f.extractor(""))
	if empty.State != "completed_empty" || len(empty.Candidates) != 0 {
		t.Fatalf("empty completion %+v", empty)
	}
	decisionSelection := f.foreground("I drink coffee and espresso.")
	original := f.compile(decisionSelection, f.generation, f.extractor("coffee"))
	first := f.inspectCandidate(original, "global")
	a := f.authority("global")
	stale, err := f.store.PrepareOwnerCandidateReview(f.ctx, a, first.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	editedProposal := first.Candidate.Proposal
	editedProposal.Proposition.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralText, Value: "espresso"}
	edited, err := f.store.EditOwnerCandidate(f.ctx, a, memory.ReviewEditDecision{Candidate: first.Ref, Proposal: editedProposal, Reason: "Select the other explicitly named drink for this scripted interpretation."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(f.ctx, a, memory.ReviewDecision{DeliveryKey: stage4Key(200), PreviewID: stale.ID, PreviewSHA256: stale.SHA256, Action: "accept"}); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("old interpretation approval reused: %v", err)
	}
	server := httptest.NewServer(web.WithCandidateReview(web.NewServer(nil), f.store).Handler())
	defer server.Close()
	var reject memory.ReviewPreview
	stage4HTTP(t, server, "prepare", map[string]any{"scope_key": "global", "candidate": edited.Ref, "action": "reject"}, 200, &reject)
	var rejected memory.ReviewResult
	stage4CLI(t, f.store, &rejected, "resolve", "--scope", "global", "--preview", reject.ID, "--digest", reject.SHA256, "--delivery", stage4Key(201), "--action", "reject")
	if rejected.Operation != nil {
		t.Fatal("rejection changed accepted memory")
	}
	// Explicit history covers the failed earlier unit and later completed-empty
	// unit only. Neither out-of-order success nor a scan cursor fills that gap.
	events, err := f.store.LoadEvents(f.ctx, f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var firstSequence int64
	var lastID memory.EventID
	for _, event := range events {
		if event.ID == gapSelection.RootID {
			firstSequence = event.Sequence
		}
		if event.Sequence == emptySelection.Cutoff {
			lastID = event.ID
		}
	}
	request := memory.CompilerHistoryRequest{RequestID: "stage4-explicit-history", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: "global", SessionID: f.session.ID, FirstSequence: firstSequence, LastSequence: emptySelection.Cutoff, FirstEventID: gapSelection.RootID, LastEventID: lastID}}}
	receipt, err := f.store.SelectCompilerHistory(f.ctx, []memory.ScopeContext{f.session.ScopeContext()}, request, f.generation, f.extractor(""))
	if err != nil {
		t.Fatal(err)
	}
	for range 12 {
		if _, err = f.store.ReconcileCompilerHistory(f.ctx, f.config(f.extractor(""))); err != nil {
			t.Fatal(err)
		}
	}
	progress, err := f.store.InspectCompilerHistory(f.ctx, []memory.ScopeContext{f.session.ScopeContext()}, request.RequestID, 0, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	if progress.ContiguousFrontier != firstSequence-1 || len(progress.Intervals) != 2 || progress.Intervals[0].State != "failed" || progress.Intervals[1].State != "completed_empty" || progress.SelectedSessionEvents != int64(emptySelection.Cutoff-firstSequence+1) || progress.OutsideSelectionEvents != int64(len(events))-(emptySelection.Cutoff-firstSequence+1) {
		t.Fatalf("gap/completed/outside conflation %+v", progress)
	}
	f.closeSource()
	oldSession := f.session
	f.reopen()
	a = f.authority("global")
	next := f.generation
	next.Decoding.Seed++
	repeated := f.compile(decisionSelection, next, f.extractor("coffee"))
	if len(repeated.Candidates) != 1 || repeated.Candidates[0].EquivalentTo != first.Ref.ID || repeated.Candidates[0].ReviewState != "unresolved" {
		t.Fatalf("recurrence copied/lost owner authority %+v", repeated)
	}
	lineage, err := f.store.InspectOwnerCandidateLineage(f.ctx, a, repeated.Candidates[0].ID)
	if err != nil || !lineage.Suppressed || lineage.Origin == nil || lineage.Origin.Ref != rejected.Candidates[0] || lineage.Origin.Edit == nil || lineage.Resolution == nil || lineage.Resolution.AuditID != rejected.AuditID || lineage.Decision == nil || lineage.Decision.Action != "reject" {
		t.Fatalf("generation erased edit/rejection lineage %+v %v", lineage, err)
	}
	var cliLineage memory.CandidateLineage
	stage4CLI(t, f.store, &cliLineage, "lineage", "--scope", "global", "--id", repeated.Candidates[0].ID)
	kernelJSON, _ := json.Marshal(lineage)
	cliJSON, _ := json.Marshal(cliLineage)
	if string(kernelJSON) != string(cliJSON) {
		t.Fatal("CLI lineage differs from preserved Kernel history")
	}
	freshSession, err := f.store.CreateGlobalSession(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.session = freshSession
	newSupport := f.foreground("I drink coffee and espresso.")
	fresh := f.compile(newSupport, next, f.extractor("coffee"))
	freshItem := f.inspectCandidate(fresh, "global")
	if freshItem.Candidate.EquivalentTo != "" || freshItem.Candidate.ReviewState != "unresolved" {
		t.Fatal("new evidence inherited old rejection")
	}
	freshLineage, err := f.store.InspectOwnerCandidateLineage(f.ctx, f.authority("global"), freshItem.Ref.ID)
	if err != nil || freshLineage.Suppressed {
		t.Fatalf("fresh support suppressed %+v %v", freshLineage, err)
	}
	selector := memory.CompilerLiveSelector{SourceScope: "global", Destination: "global", SessionID: f.session.ID}
	activation, err := f.store.ActivateCompiler(f.ctx, f.session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "stage4-next-generation", Selector: selector}, next, f.extractor("coffee"))
	if err != nil {
		t.Fatal(err)
	}
	newest := next
	newest.Prompt += " Preserve the same bounded output contract."
	upgraded, err := f.store.ActivateCompiler(f.ctx, f.session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "stage4-upgrade-generation", Selector: selector, ExpectedRevision: activation.Revision}, newest, f.extractor("coffee"))
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.GenerationID == activation.GenerationID || upgraded.AfterPosition != activation.AfterPosition {
		t.Fatal("generation frontier changed without appended evidence")
	}
	var swept int
	if err = f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_jobs WHERE generation_id=?`, upgraded.GenerationID).Scan(&swept); err != nil || swept != 0 {
		t.Fatal("generation activation swept historical events", swept, err)
	}
	oldProgress, err := f.store.InspectCompilerHistory(f.ctx, []memory.ScopeContext{oldSession.ScopeContext()}, request.RequestID, 0, 0, 64)
	if err != nil || oldProgress.Receipt.GenerationID != receipt.GenerationID || oldProgress.ContiguousFrontier != progress.ContiguousFrontier {
		t.Fatalf("generation rewrote prior coverage %+v %v", oldProgress, err)
	}
	claims, err := f.store.InspectLiteralClaims(f.ctx, f.session.ScopeContext())
	if err != nil || len(claims.Claims) != 1 {
		t.Fatalf("unaccepted/rejected state escaped into accepted reads %+v %v", claims, err)
	}
	verified, err := f.store.VerifySemanticProjection(f.ctx)
	if err != nil || !verified.Valid {
		t.Fatalf("history canonical replay %v %+v", err, verified)
	}
	raw, _ := json.Marshal(lineage.Origin.Edit)
	stage4Evidence(t, "history_gap_empty_generation_lineage", map[string]any{"gap_job_id": gap.JobID, "empty_job_id": empty.JobID, "selected_events": progress.SelectedSessionEvents, "outside_events": progress.OutsideSelectionEvents, "outside_root": outside.RootID, "contiguous_frontier": progress.ContiguousFrontier, "receipt_generation": receipt.GenerationID, "repeat_generation": repeated.GenerationID, "upgrade_generation": upgraded.GenerationID, "edit_sha256": memory.CompilerHash(raw), "preserved_rejection": true, "fresh_support_requires_review": true, "automatic_history_sweep": false, "canonical_replay": true})
}
