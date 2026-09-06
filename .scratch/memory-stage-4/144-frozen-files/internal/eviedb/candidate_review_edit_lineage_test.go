package eviedb_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewEditPreservesAndInvalidatesPriorTemporalChoice(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	a := temporalAuthority(t, f)
	compiled := compileTemporal(t, f, "I said tea before. I now drink coffee; I do not know the date of the change.", func(c *memory.ExtractorCandidate) {
		c.Proposition.Object.Literal.Value = "coffee"
		c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError}}
	})
	selected := chooseTemporal(t, f, a, candidateRef(compiled), memory.CorrectionError)
	old, err := f.store.PrepareOwnerCandidateReview(ctx, a, selected.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	proposal := selected.Candidate.Proposal
	proposal.Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion", Correction: &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionChanged}}}
	edited, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: selected.Ref, Proposal: proposal, Reason: "This is a change, with an unknown effective date."})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Ref.InterpretationRevision != 2 || edited.Edit.ParentRevision != 1 || edited.Edit.Before.Temporal == nil || edited.Edit.Before.Temporal.Choice.Mode != memory.CorrectionError || edited.Edit.After.Temporal != nil || edited.Temporal != nil {
		t.Fatalf("choice lineage %+v", edited)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(old, "90000000-0000-4000-8000-000000000354")); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("old choice approval %v", err)
	}
	options, err := f.store.OwnerCandidateTemporalOptions(ctx, a, edited.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ChooseOwnerCandidateTemporal(ctx, a, memory.ReviewTemporalDecision{Candidate: edited.Ref, OptionsSHA256: options.SHA256, Choice: memory.ReviewTemporalChoice{OldClaimID: options.Alternatives[0].Claim.ID, Mode: memory.CorrectionChanged}}); err == nil || !strings.Contains(err.Error(), "effective time") {
		t.Fatalf("invented unknown instant: %v", err)
	}
	// Owner chooses a supported ordinary assertion; no supersession or world-date
	// inference is implied by the edit text or the observed transaction instant.
	proposal.Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion"}
	current, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: edited.Ref, Proposal: proposal, Reason: "Record the assertion while the effective date remains unknown."})
	if err != nil {
		t.Fatal(err)
	}
	if current.Ref.InterpretationRevision != 3 || current.Edit.ParentRevision != 2 || current.Original.Proposal.Temporal.Correction.Modes[0] != memory.CorrectionError {
		t.Fatalf("original/parent lost %+v", current)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, current.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	if p.Effect.Members[0].Correction != nil || p.Effect.Claims[0].Claim.ValidTime.From != nil || p.Effect.Claims[0].Claim.ValidTime.To != nil {
		t.Fatal("unknown assertion acquired correction/date")
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000355")); err != nil {
		t.Fatal(err)
	}
	history, err := f.store.InspectOwnerCandidateEditRevision(ctx, a, current.Ref.ID, 2)
	if err != nil || history.Before.Temporal == nil || history.After.Proposal.Temporal.Correction.Modes[0] != memory.CorrectionChanged {
		t.Fatalf("abandoned edit history %+v %v", history, err)
	}
	assertTemporalReplay(t, f)
}
func TestOwnerReviewEditRequiresFreshIdentityChoice(t *testing.T) {
	ctx := context.Background()
	f, a, r := sharedBatchFixture(t)
	before, err := f.store.InspectOwnerCandidate(ctx, a, r.Groups[0].Candidates[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	edited, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: before.Ref, Proposal: before.Candidate.Proposal, Reason: "Reconsider the identity interpretation."})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Identity != nil || edited.Edit.Before.Identity == nil || edited.Edit.After.Identity != nil || edited.Ref.InterpretationRevision != before.Ref.InterpretationRevision+1 {
		t.Fatalf("identity was carried forward %+v", edited)
	}
	if _, err = f.store.PrepareOwnerCandidateReview(ctx, a, edited.Ref, "accept"); err == nil || !strings.Contains(err.Error(), "needs_choice") {
		t.Fatalf("old choice reused %v", err)
	}
	o, err := f.store.OwnerCandidateIdentityOptions(ctx, a, edited.Ref)
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: edited.Ref, OptionsSHA256: o.SHA256, Choices: before.Identity.Choices})
	if err != nil {
		t.Fatal(err)
	}
	if chosen.Ref.InterpretationRevision != edited.Ref.InterpretationRevision+1 || chosen.Edit.Revision != edited.Ref.InterpretationRevision {
		t.Fatal("identity/edit lineage collapsed")
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000356")); err != nil {
		t.Fatal(err)
	}
	assertTemporalReplay(t, f)
}
func TestOwnerReviewEditReasonBoundAndSecretExclusion(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	ref := candidateRef(compiled)
	for _, reason := range []string{strings.Repeat("x", 4097), "api_key=1234567890secret"} {
		if _, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: ref, Proposal: compiled.Candidates[0].Proposal, Reason: reason}); err == nil {
			t.Fatal("unsafe edit reason persisted")
		}
	}
	edited, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: ref, Proposal: compiled.Candidates[0].Proposal, Reason: strings.Repeat("x", 4096)})
	if err != nil || edited.Ref.InterpretationRevision != 1 {
		t.Fatalf("inclusive reason bound %+v %v", edited, err)
	}
	var n int
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_review_edit_revisions`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("invalid edit side effect %d %v", n, err)
	}
}

func TestOwnerReviewTerminalEditAndChoicesPrecedeSourceEligibility(t *testing.T) {
	for _, action := range []string{"accept", "reject"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			f, compiled, a := reviewCandidateFixture(t)
			ref := candidateRef(compiled)
			p, err := f.store.PrepareOwnerCandidateReview(ctx, a, ref, action)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000360")); err != nil {
				t.Fatal(err)
			}
			if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='detector-next'`); err != nil {
				t.Fatal(err)
			}
			item, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: ref, Proposal: compiled.Candidates[0].Proposal})
			if !errors.Is(err, eviedb.ErrReviewResolved) || item.Ref.ID != "" || len(item.Candidate.Support) != 0 {
				t.Fatalf("terminal edit disclosed/revalidated %+v %v", item, err)
			}
			if _, err = f.store.OwnerCandidateIdentityOptions(ctx, a, ref); !errors.Is(err, eviedb.ErrReviewResolved) {
				t.Fatalf("terminal identity options %v", err)
			}
			if item, err = f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref}); !errors.Is(err, eviedb.ErrReviewResolved) || item.Ref.ID != "" {
				t.Fatalf("terminal identity choice %+v %v", item, err)
			}
			if _, err = f.store.OwnerCandidateTemporalOptions(ctx, a, ref); !errors.Is(err, eviedb.ErrReviewResolved) {
				t.Fatalf("terminal temporal options %v", err)
			}
			if item, err = f.store.ChooseOwnerCandidateTemporal(ctx, a, memory.ReviewTemporalDecision{Candidate: ref}); !errors.Is(err, eviedb.ErrReviewResolved) || item.Ref.ID != "" {
				t.Fatalf("terminal temporal choice %+v %v", item, err)
			}
			var edits, identity, temporal int
			if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_review_edit_revisions),(SELECT count(*) FROM memory_review_identity_revisions),(SELECT count(*) FROM memory_review_temporal_revisions)`).Scan(&edits, &identity, &temporal); err != nil || edits+identity+temporal != 0 {
				t.Fatalf("terminal mutation %d/%d/%d %v", edits, identity, temporal, err)
			}
			if _, err = f.store.EditOwnerCandidate(ctx, eviedb.OwnerReviewContext{}, memory.ReviewEditDecision{Candidate: ref, Proposal: compiled.Candidates[0].Proposal}); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
				t.Fatalf("terminal visibility preceded authorization %v", err)
			}
		})
	}
}

func TestOwnerReviewInvalidRequestReasonsAreTypedAndNeverPersist(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	ref := candidateRef(compiled)
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.store.PrepareOwnerCandidateBatch(ctx, a, independentBatch([]memory.CandidateRef{ref}))
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"api_key=1234567890secret", strings.Repeat("r", 4097), string([]byte{0xff})} {
		d := decisionFor(p, "90000000-0000-4000-8000-000000000361")
		d.Reason = reason
		if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, d); !errors.Is(err, eviedb.ErrReviewInvalidRequest) {
			t.Fatalf("single bad reason: %v", err)
		}
		bd := batchDecision(b, "90000000-0000-4000-8000-000000000362")
		bd.Reason = reason
		if _, err = f.store.ResolveOwnerCandidateBatch(ctx, a, bd); !errors.Is(err, eviedb.ErrReviewInvalidRequest) {
			t.Fatalf("batch bad reason: %v", err)
		}
		if _, err = f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: ref, Proposal: compiled.Candidates[0].Proposal, Reason: reason}); !errors.Is(err, eviedb.ErrReviewInvalidRequest) {
			t.Fatalf("edit bad reason: %v", err)
		}
	}
	d := decisionFor(p, "90000000-0000-4000-8000-000000000361")
	d.DeliveryKey = "invalid"
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, d); !errors.Is(err, eviedb.ErrReviewInvalidRequest) {
		t.Fatalf("single bad key %v", err)
	}
	bd := batchDecision(b, "90000000-0000-4000-8000-000000000362")
	bd.DeliveryKey = "invalid"
	if _, err = f.store.ResolveOwnerCandidateBatch(ctx, a, bd); !errors.Is(err, eviedb.ErrReviewInvalidRequest) {
		t.Fatalf("batch bad key %v", err)
	}
	var count int
	if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_review_edit_revisions)+(SELECT count(*) FROM memory_review_audits)+(SELECT count(*) FROM memory_review_deliveries)+(SELECT count(*) FROM memory_review_batch_deliveries)+(SELECT count(*) FROM memory_review_resolutions)+(SELECT count(*) FROM semantic_operations WHERE schema_version=6)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid request persisted state %d %v", count, err)
	}
	current, err := f.store.InspectOwnerCandidate(ctx, a, ref.ID)
	if err != nil || current.Ref != ref || current.Candidate.ReviewState != "unresolved" {
		t.Fatalf("invalid request changed candidate %+v %v", current, err)
	}
}
