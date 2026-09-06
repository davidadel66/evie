package eviedb_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func repeatRecurrence(t *testing.T, f *compilerFixture, original memory.Compilation, seed int, proposal memory.ExtractorCandidate) memory.Compilation {
	t.Helper()
	g := compilerGeneration()
	g.Decoding.Seed = seed
	out, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), original.Window.Selection, g, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return compilerOutput(r, []memory.ExtractorCandidate{proposal}), nil
	}})
	if err != nil || len(out.Candidates) != 1 {
		t.Fatalf("repeat: %+v %v", out, err)
	}
	return out
}
func TestCompilerRecurrencePreservesEveryOwnerStateAndOriginal(t *testing.T) {
	for _, state := range []string{"unresolved", "accepted", "rejected", "edited", "edited_accepted", "edited_rejected"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			f, first, a := reviewCandidateFixture(t)
			ref := candidateRef(first)
			var originalBytes []byte
			if err := f.db.QueryRow(`SELECT envelope FROM memory_compiler_candidates WHERE candidate_id=?`, ref.ID).Scan(&originalBytes); err != nil {
				t.Fatal(err)
			}
			edited := state == "edited" || state == "edited_accepted" || state == "edited_rejected"
			if edited {
				proposal := first.Candidates[0].Proposal
				proposal.Proposition.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}
				next, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: ref, Proposal: proposal, Reason: "Keep the actual owner interpretation."})
				if err != nil {
					t.Fatal(err)
				}
				ref = next.Ref
			}
			terminal := state == "accepted" || state == "edited_accepted" || state == "rejected" || state == "edited_rejected"
			if terminal {
				action := "accept"
				if state == "rejected" || state == "edited_rejected" {
					action = "reject"
				}
				p, err := f.store.PrepareOwnerCandidateReview(ctx, a, ref, action)
				if err != nil {
					t.Fatal(err)
				}
				r, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000147"))
				if err != nil {
					t.Fatal(err)
				}
				ref = r.Candidates[0]
			}
			second := repeatRecurrence(t, f, first, 27, first.Candidates[0].Proposal)
			c := second.Candidates[0]
			if c.EquivalentTo != first.Candidates[0].ID || c.ReviewState != "unresolved" || c.ReviewRevision != 0 {
				t.Fatalf("copied or lost authority: %+v", c)
			}
			if _, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(second), "accept"); err == nil {
				t.Fatal("suppressed independent preview")
			}
			lineage, err := f.store.InspectOwnerCandidateLineage(ctx, a, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if lineage.Origin == nil || lineage.Origin.Ref != ref || lineage.Checked == nil || *lineage.Checked != ref || lineage.Relationship != "exact_original" || lineage.Generation.Decoding.Seed != 27 {
				t.Fatalf("lineage: %+v", lineage)
			}
			if terminal != (lineage.Resolution != nil) {
				t.Fatalf("resolution: %+v", lineage)
			}
			if edited && (lineage.Origin.Edit == nil || lineage.Origin.Original == nil || lineage.Origin.Candidate.Proposal.Proposition.Object.Literal.Value != "tea" || lineage.Candidate.Candidate.Proposal.Proposition.Object.Literal.Value != "café") {
				t.Fatalf("owner edit replaced: %+v", lineage)
			}
			var after []byte
			if err = f.db.QueryRow(`SELECT envelope FROM memory_compiler_candidates WHERE candidate_id=?`, first.Candidates[0].ID).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if string(originalBytes) != string(after) {
				t.Fatal("original bytes changed")
			}
			reopened, err := eviedb.OpenDBAt(f.path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			store := eviedb.NewStore(reopened)
			authority, err := store.LocalOwnerReviewContext(ctx, "global")
			if err != nil {
				t.Fatal(err)
			}
			again, err := store.InspectOwnerCandidateLineage(ctx, authority, c.ID)
			if err != nil || again.Origin == nil || again.Origin.Ref != ref {
				t.Fatalf("reopen: %+v %v", again, err)
			}
			assertTemporalReplay(t, f)
		})
	}
}
func TestCompilerRecurrenceLaterDecisionAndNewSupport(t *testing.T) {
	ctx := context.Background()
	f, first, a := reviewCandidateFixture(t)
	second := repeatRecurrence(t, f, first, 27, first.Candidates[0].Proposal)
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(first), "accept")
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000247"))
	if err != nil {
		t.Fatal(err)
	}
	lineage, err := f.store.InspectOwnerCandidateLineage(ctx, a, second.Candidates[0].ID)
	if err != nil || lineage.Checked.ReviewRevision != 0 || lineage.Origin.Ref.ReviewRevision != 1 || lineage.Resolution == nil || lineage.Resolution.AuditID != resolution.AuditID {
		t.Fatalf("later decision: %+v %v", lineage, err)
	}
	selection := f.selection(t, "I prefer café again.", true)
	g := compilerGeneration()
	g.Decoding.Seed = 28
	third, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selection, g, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		proposal := f.candidate(r)
		proposal.Proposition.Object.Literal.Value = "café"
		return compilerOutput(r, []memory.ExtractorCandidate{proposal}), nil
	}})
	if err != nil || len(third.Candidates) != 1 || third.Candidates[0].EquivalentTo != "" {
		t.Fatalf("new support: %+v %v", third, err)
	}
	lineage, err = f.store.InspectOwnerCandidateLineage(ctx, a, third.Candidates[0].ID)
	if err != nil || lineage.Relationship != "different_support" || lineage.Origin == nil || lineage.Origin.Ref.ID != first.Candidates[0].ID {
		t.Fatalf("related support: %+v %v", lineage, err)
	}
	next, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(third), "accept")
	if err != nil {
		t.Fatal(err)
	}
	if next.Effect.Claims[0].Create || !next.Effect.Claims[0].Sources[0].Create {
		t.Fatalf("not fresh attachment: %+v", next.Effect)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='revoked'`); err != nil {
		t.Fatal(err)
	}
	authority, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	redacted, err := f.store.InspectOwnerCandidateLineage(ctx, authority, second.Candidates[0].ID)
	if err != nil || !redacted.Candidate.Redacted || redacted.Origin != nil || redacted.Resolution != nil {
		t.Fatalf("source policy disclosure: %+v %v", redacted, err)
	}
}
func TestCompilerRecurrenceLatePublicationChecksEditRevision(t *testing.T) {
	ctx := context.Background()
	f, first, a := reviewCandidateFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan memory.Compilation, 1)
	fail := make(chan error, 1)
	g := compilerGeneration()
	g.Decoding.Seed = 34
	go func() {
		out, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), first.Window.Selection, g, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
			close(entered)
			<-release
			return compilerOutput(r, []memory.ExtractorCandidate{first.Candidates[0].Proposal}), nil
		}})
		done <- out
		fail <- err
	}()
	<-entered
	proposal := first.Candidates[0].Proposal
	proposal.Proposition.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}
	edited, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: candidateRef(first), Proposal: proposal})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, a, edited.Ref, "reject")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(preview, "90000000-0000-4000-8000-000000000347"))
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	out := <-done
	if err = <-fail; err != nil {
		t.Fatal(err)
	}
	lineage, err := f.store.InspectOwnerCandidateLineage(ctx, a, out.Candidates[0].ID)
	if err != nil || lineage.Checked == nil || *lineage.Checked != result.Candidates[0] || lineage.Origin.Edit == nil {
		t.Fatalf("stale classification: %+v %v", lineage, err)
	}
}
func TestCompilerRecurrencePolicyAndMeaningChangesStayFresh(t *testing.T) {
	for _, name := range []string{"polarity", "time", "policy"} {
		t.Run(name, func(t *testing.T) {
			f, first, _ := reviewCandidateFixture(t)
			proposal := first.Candidates[0].Proposal
			g := compilerGeneration()
			g.Decoding.Seed = 47
			switch name {
			case "polarity":
				proposal.Proposition.Polarity = memory.PolarityDenied
			case "time":
				proposal.TemporalQualification = "unknown historic date"
			case "policy":
				g.EquivalencePolicy = memory.CompilerEquivalencePolicyV2
			}
			out, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), first.Window.Selection, g, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				return compilerOutput(r, []memory.ExtractorCandidate{proposal}), nil
			}})
			if err != nil || len(out.Candidates) != 1 || out.Candidates[0].EquivalentTo != "" {
				t.Fatalf("changed %s: %+v %v", name, out, err)
			}
		})
	}
}

func TestCompilerRecurrenceChangedAcceptedGraphRequiresFreshReview(t *testing.T) {
	for _, kind := range []memory.SemanticObjectKind{memory.SemanticObjectClaim, memory.SemanticObjectSourceLink} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := context.Background()
			f, first, a := reviewCandidateFixture(t)
			p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(first), "accept")
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000447"))
			if err != nil {
				t.Fatal(err)
			}
			event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Retire this old memory."})
			action := memory.LifecycleRetire
			id := result.Operation.ClaimIDs[0]
			if kind == memory.SemanticObjectSourceLink {
				action = memory.LifecycleRetractSource
				id = result.Operation.SourceLinkIDs[0]
			}
			lifecycle, err := f.store.PrepareMemoryLifecycle(ctx, f.session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000448", SourceEventID: event.ID, Action: action, ObjectKind: kind, ObjectID: id})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.ApplyMemoryLifecycle(ctx, f.lease, lifecycle); err != nil {
				t.Fatal(err)
			}
			second := repeatRecurrence(t, f, first, 48, first.Candidates[0].Proposal)
			if second.Candidates[0].EquivalentTo != "" {
				t.Fatal("changed current graph hidden")
			}
			lineage, err := f.store.InspectOwnerCandidateLineage(ctx, a, second.Candidates[0].ID)
			if err != nil || lineage.Relationship != "current_effect_changed" || lineage.Origin == nil || lineage.Resolution == nil {
				t.Fatalf("changed graph origin %+v %v", lineage, err)
			}
			third := repeatRecurrence(t, f, first, 49, first.Candidates[0].Proposal)
			if third.Candidates[0].EquivalentTo != second.Candidates[0].ID {
				t.Fatal("later recurrence did not retain the new unresolved primary")
			}
			assertTemporalReplay(t, f)
		})
	}
}

func TestCompilerRecurrenceUpgradeHistoryCoverageAndCancellation(t *testing.T) {
	ctx := context.Background()
	f, first, a := reviewCandidateFixture(t)
	reject, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(first), "reject")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(reject, "90000000-0000-4000-8000-000000000547")); err != nil {
		t.Fatal(err)
	}
	extractor := &historyPublicScript{scriptedCompiler: scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return compilerOutput(r, []memory.ExtractorCandidate{first.Candidates[0].Proposal}), nil
	}}}
	g := compilerGeneration()
	selector := memory.CompilerLiveSelector{SourceScope: "global", SessionID: f.session.ID, Destination: "global"}
	old, err := f.store.ActivateCompiler(ctx, f.session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "147-original", Selector: selector}, g, extractor)
	if err != nil {
		t.Fatal(err)
	}
	g.Prompt += " Use the reviewed schema carefully."
	id, _, err := memory.CompilerGenerationIdentity(g)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := f.store.ActivateCompiler(ctx, f.session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "147-upgrade", ExpectedRevision: old.Revision, Selector: selector}, g, extractor)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.GenerationID == first.GenerationID || upgraded.AfterPosition != old.AfterPosition {
		t.Fatalf("activation frontier %+v", upgraded)
	}
	var jobs int
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_compiler_jobs WHERE generation_id=?`, id).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatal("activation swept history", jobs, err)
	}
	var firstSequence int64
	var last memory.EventID
	if err = f.db.QueryRow(`SELECT sequence FROM events WHERE id=?`, first.Window.Selection.RootID).Scan(&firstSequence); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT id FROM events WHERE session_id=? AND sequence=?`, f.session.ID, first.Window.Selection.Cutoff).Scan(&last); err != nil {
		t.Fatal(err)
	}
	selection := memory.CompilerHistoryRequest{RequestID: "147-selected-range", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: "global", SessionID: f.session.ID, FirstSequence: firstSequence, LastSequence: first.Window.Selection.Cutoff, FirstEventID: first.Window.Selection.RootID, LastEventID: last}}}
	owners := []memory.ScopeContext{f.session.ScopeContext()}
	receipt, err := f.store.SelectCompilerHistory(ctx, owners, selection, g, extractor)
	if err != nil {
		t.Fatal(err)
	}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: extractor}}
	for range 5 {
		if _, err = f.store.ReconcileCompilerHistory(ctx, config); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := f.store.InspectCompilerHistory(ctx, owners, selection.RequestID, 0, 0, 64)
	if err != nil || pending.ContiguousFrontier != firstSequence-1 {
		t.Fatalf("borrowed old coverage %+v %v", pending, err)
	}
	cancelled, err := f.store.CancelCompilerHistory(ctx, owners, memory.CompilerHistoryChange{RequestID: selection.RequestID, OperationID: "147-cancel", ExpectedRevision: pending.RequestState.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); err != nil || worked {
		t.Fatal("cancelled historical job dispatched", worked, err)
	}
	if _, err = f.store.ResumeCompilerHistory(ctx, owners, memory.CompilerHistoryChange{RequestID: selection.RequestID, OperationID: "147-resume", ExpectedRevision: cancelled.Revision}, g, extractor); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err = f.store.ReconcileCompilerHistory(ctx, config); err != nil {
			t.Fatal(err)
		}
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); err != nil || !worked {
		t.Fatal("resumed selected generation", worked, err)
	}
	complete, err := f.store.InspectCompilerHistory(ctx, owners, selection.RequestID, 0, 0, 64)
	if err != nil || complete.Receipt.GenerationID != receipt.GenerationID || complete.ContiguousFrontier != selection.Ranges[0].LastSequence {
		t.Fatalf("coverage %+v %v", complete, err)
	}
	compiled, err := f.store.InspectCompilation(ctx, f.session.ScopeContext(), complete.Intervals[0].JobID)
	if err != nil || len(compiled.Candidates) != 1 || compiled.Candidates[0].EquivalentTo != first.Candidates[0].ID {
		t.Fatalf("upgrade review %+v %v", compiled, err)
	}
	calls := extractor.calls.Load()
	if _, err = f.store.SelectCompilerHistory(ctx, owners, selection, g, extractor); err != nil {
		t.Fatal(err)
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); err != nil || worked || extractor.calls.Load() != calls {
		t.Fatal("same range recompiled", worked, err)
	}
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_compiler_coverage c JOIN memory_compiler_jobs j ON j.job_id=c.job_id WHERE j.generation_id IN (?,?)`, first.GenerationID, id).Scan(&jobs); err != nil || jobs != 2 {
		t.Fatal("generation coverage lost", jobs, err)
	}
}

func TestCompilerRecurrenceAcceptedNewIdentityDoesNotReopenOriginal(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	first := compileIdentity(t, f, "I work with Maya.", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) { identityProposal(c) })
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	ref := candidateRef(first)
	options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, ref)
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: options.SHA256, Choices: memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(preview, "90000000-0000-4000-8000-000000000647"))
	if err != nil {
		t.Fatal(err)
	}
	g := identityGeneration()
	g.Decoding.Seed = 600
	second, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), first.Window.Selection, g, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		c := first.Candidates[0].Proposal
		confidence := 0.99
		c.Identity.Confidence = &confidence
		c.Identity.Uncertainty = "Changed model wording"
		return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
	}})
	if err != nil || len(second.Candidates) != 1 || second.Candidates[0].EquivalentTo != first.Candidates[0].ID {
		t.Fatalf("own accepted identity reopened the original: %+v %v", second, err)
	}
	lineage, err := f.store.InspectOwnerCandidateLineage(ctx, a, second.Candidates[0].ID)
	if err != nil || lineage.Origin == nil || lineage.Origin.Identity == nil || lineage.Resolution == nil || lineage.Resolution.AuditID != result.AuditID {
		t.Fatalf("accepted identity lineage %+v %v", lineage, err)
	}
}

func TestCompilerRecurrenceNewSessionSupportShowsPriorDecision(t *testing.T) {
	ctx := context.Background()
	f, first, a := reviewCandidateFixture(t)
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(first), "accept")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(preview, "90000000-0000-4000-8000-000000000747"))
	if err != nil {
		t.Fatal(err)
	}
	other := *f
	other.session, err = f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other.lease, err = f.store.AcquireTurnLease(ctx, other.session.ID, "recurrence-other", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	selected := other.selection(t, "I prefer café.", true)
	later, err := f.store.CompileCandidateUnit(ctx, other.session.ScopeContext(), selected, compilerGeneration(), &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		proposal := other.candidate(r)
		proposal.Proposition.Object.Literal.Value = "café"
		return compilerOutput(r, []memory.ExtractorCandidate{proposal}), nil
	}})
	if err != nil || len(later.Candidates) != 1 || later.Candidates[0].EquivalentTo != "" {
		t.Fatalf("new session candidate %+v %v", later, err)
	}
	lineage, err := f.store.InspectOwnerCandidateLineage(ctx, a, later.Candidates[0].ID)
	if err != nil || lineage.Relationship != "different_support" || lineage.Origin == nil || lineage.Resolution == nil || lineage.Resolution.AuditID != resolved.AuditID {
		t.Fatalf("cross-session decision %+v %v", lineage, err)
	}
	if lineage.Selection.SessionID != other.session.ID || lineage.Origin.Candidate.Support[0].SessionID != f.session.ID {
		t.Fatal("source sessions were conflated")
	}
	fresh, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(later), "accept")
	if err != nil || fresh.Effect.Claims[0].Create || !fresh.Effect.Claims[0].Sources[0].Create || fresh.Effect.Claims[0].Sources[0].SessionID != other.session.ID {
		t.Fatalf("fresh attachment %+v %v", fresh, err)
	}
}

// A changed correction projects OldAfter from the correction ledger. The
// original Claim row intentionally retains OldBefore and is never rewritten.
func TestCompilerRecurrenceChangedCorrectionUsesPersistedProjection(t *testing.T) {
	for _, compound := range []bool{false, true} {
		name := "native"
		if compound {
			name = "compound"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			f := newCompilerFixture(t)
			a := temporalAuthority(t, f)
			effective := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
			compile := func(content, value string) memory.Compilation {
				return compileTemporal(t, f, content, func(c *memory.ExtractorCandidate) {
					c.Proposition.Object.Literal.Value = value
					c.ValidTime.From = &effective
					c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionChanged}, EffectiveTime: &effective}
				})
			}
			original := []memory.Compilation{compile("Since May 1 2025 I drink coffee instead of tea.", "coffee")}
			if compound {
				event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I eat rice."})
				seed, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000008147", SourceEventID: event.ID, Predicate: "eats", PredicateLabel: "eats", PredicateCardinality: memory.CardinalityOne, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "rice"}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, seed); err != nil {
					t.Fatal(err)
				}
				f.predicate = seed.Predicate.ID
				original = append(original, compile("Since May 1 2025 I eat pasta instead of rice.", "pasta"))
			}
			refs := []memory.CandidateRef{}
			for _, candidate := range original {
				refs = append(refs, chooseTemporal(t, f, a, candidateRef(candidate), memory.CorrectionChanged).Ref)
			}
			var resolved memory.ReviewResult
			var effect *memory.ReviewEffect
			if compound {
				preview, err := f.store.PrepareOwnerCandidateBatch(ctx, a, memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{{ID: "changed_corrections", Action: "accept", Candidates: refs, Dependencies: []memory.ReviewDependency{}}}})
				if err != nil {
					t.Fatal(err)
				}
				result, err := f.store.ResolveOwnerCandidateBatch(ctx, a, batchDecision(preview, "90000000-0000-4000-8000-000000008148"))
				if err != nil || result.Groups[0].Outcome != "accepted" {
					t.Fatalf("compound acceptance %+v %v", result, err)
				}
				resolved = *result.Groups[0].Result
				effect = preview.Groups[0].Preview.Effect
			} else {
				preview, err := f.store.PrepareOwnerCandidateReview(ctx, a, refs[0], "accept")
				if err != nil {
					t.Fatal(err)
				}
				resolved, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(preview, "90000000-0000-4000-8000-000000008148"))
				if err != nil {
					t.Fatal(err)
				}
				effect = preview.Effect
			}
			repeat := func(candidate memory.Compilation, seed int) memory.Compilation {
				t.Helper()
				g := temporalGeneration()
				g.Decoding.Seed = seed
				out, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), candidate.Window.Selection, g, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
					return compilerOutput(r, []memory.ExtractorCandidate{candidate.Candidates[0].Proposal}), nil
				}})
				if err != nil || len(out.Candidates) != 1 {
					t.Fatalf("repeat %+v %v", out, err)
				}
				return out
			}
			members := effect.Members
			if !compound {
				members = []memory.ReviewEffect{*effect}
			}
			for index, candidate := range original {
				var unchanged, projected sql.NullString
				if err := f.db.QueryRow(`SELECT c.valid_to,x.old_effective_to FROM semantic_claims c JOIN semantic_claim_corrections x ON x.old_claim_id=c.claim_id WHERE c.claim_id=?`, members[index].Correction.OldClaim.ID).Scan(&unchanged, &projected); err != nil {
					t.Fatal(err)
				}
				if unchanged.Valid || !projected.Valid {
					t.Fatal("fixture did not preserve immutable OldBefore and projected OldAfter")
				}
				next := repeat(candidate, 8100+index)
				if next.Candidates[0].EquivalentTo != candidate.Candidates[0].ID {
					t.Fatalf("accepted changed correction reopened: %s -> %s", candidate.Candidates[0].ID, next.Candidates[0].EquivalentTo)
				}
				lineage, err := f.store.InspectOwnerCandidateLineage(ctx, a, next.Candidates[0].ID)
				if err != nil || lineage.Relationship != "exact_original" || lineage.Resolution == nil || lineage.Resolution.AuditID != resolved.AuditID {
					t.Fatalf("correction recurrence lineage %+v %v", lineage, err)
				}
			}
			assertTemporalReplay(t, f)
			// A genuine subsequent correction of the accepted replacement must still
			// invalidate equality, including when that replacement belongs to a group.
			later := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
			event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Since May 1 2026 I drink water instead of coffee."})
			replacement := original[0].Candidates[0].Proposal.Proposition
			replacement.Object = memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "water"}}
			correction, err := f.store.PrepareCorrectClaim(ctx, f.session.ScopeContext(), memory.CorrectClaimRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000008149", SourceEventID: event.ID, OldClaimID: effect.Claims[0].Claim.ID, Replacement: replacement, Mode: memory.CorrectionChanged, EffectiveTime: &later})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.ApplyCorrectClaim(ctx, f.lease, correction); err != nil {
				t.Fatal(err)
			}
			for index, candidate := range original {
				next := repeat(candidate, 8200+index)
				if next.Candidates[0].EquivalentTo != "" {
					t.Fatal("genuine intervening correction was suppressed")
				}
				lineage, err := f.store.InspectOwnerCandidateLineage(ctx, a, next.Candidates[0].ID)
				if err != nil || lineage.Relationship != "current_effect_changed" {
					t.Fatalf("intervening correction %+v %v", lineage, err)
				}
			}
			assertTemporalReplay(t, f)
		})
	}
}
