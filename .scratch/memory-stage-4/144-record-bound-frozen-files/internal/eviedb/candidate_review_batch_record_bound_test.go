package eviedb

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func recordBoundCandidates(t *testing.T) (*workerFixture, OwnerReviewContext, []memory.CandidateRef) {
	t.Helper()
	ctx := context.Background()
	f := newWorkerFixture(t)
	appendEvent := func(input memory.EventInput) memory.Event {
		event, err := f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	seedSource := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink water."})
	seed, err := f.store.PrepareRememberLiteral(ctx, f.owner, memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000016144", SourceEventID: seedSource.ID,
		Predicate: "drinks", PredicateLabel: "drinks", PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "water"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ApplyRememberLiteral(ctx, f.lease, seed); err != nil {
		t.Fatal(err)
	}
	f.generation.EntityPolicy = memory.CompilerIdentityPolicyV2
	f.generation.PredicatePolicy = memory.CompilerIdentityPolicyV2
	f.generation.ValidationPolicy = memory.CompilerIdentityPolicyV2
	f.generation.EquivalencePolicy = memory.CompilerIdentityPolicyV2
	f.generation.EffectPolicy = memory.CompilerIdentityPolicyV2
	authority, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	var refs []memory.CandidateRef
	for index := 0; index < 21; index++ {
		subjectName, objectName := fmt.Sprintf("M%02d", index), fmt.Sprintf("N%02d", index)
		if index >= 19 {
			subjectName = "I"
		}
		root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: subjectName + " knows " + objectName + "."})
		if index == 20 {
			appendEvent(memory.EventInput{ParentID: root.ID, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I also work with " + objectName + "."})
		}
		end := appendEvent(memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Noted."})
		compiled, err := f.store.CompileCandidateUnit(ctx, f.owner, memory.CompilationSelection{
			SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: end.Sequence, Destination: "global",
		}, f.generation, &workerScript{run: func(_ context.Context, request memory.CompilerRequest) (CompilerExtraction, error) {
			support := []memory.EvidenceLocator{}
			for _, source := range request.Window.Sources {
				if source.Usage == "new_support" {
					support = append(support, source.Locator)
				}
			}
			wantSources := 1
			if index == 20 {
				wantSources = 2
			}
			if len(support) != wantSources {
				t.Fatalf("candidate %d support = %d; want %d", index, len(support), wantSources)
			}
			candidate := memory.ExtractorCandidate{
				Proposition: memory.ClaimProposition{SubjectEntityID: seed.Subject.ID, Polarity: memory.PolarityAffirmed},
				Support:     support, Context: []memory.EvidenceLocator{},
				Identity: &memory.CandidateIdentityProposal{
					Object: &memory.EntityMention{Name: objectName, EntityType: "person", Support: support[0]},
					Predicate: &memory.PredicateDefinition{
						Token: fmt.Sprintf("relationship_%c", 'a'+index), Label: "knows",
						ObjectConstraint: memory.ConstraintEntity, Cardinality: memory.CardinalityMany,
					},
				},
			}
			if index < 19 {
				candidate.Proposition.SubjectEntityID = ""
				candidate.Identity.Subject = &memory.EntityMention{Name: subjectName, EntityType: "person", Support: support[0]}
			}
			return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: request.ID, Candidates: []memory.ExtractorCandidate{candidate}}), ReleaseEvidence: "completed"}, nil
		}})
		if err != nil || len(compiled.Candidates) != 1 {
			t.Fatalf("compile candidate %d: %+v, %v", index, compiled, err)
		}
		candidate := compiled.Candidates[0]
		ref := memory.CandidateRef{ID: candidate.ID, ReviewRevision: candidate.ReviewRevision}
		options, err := f.store.OwnerCandidateIdentityOptions(ctx, authority, ref)
		if err != nil {
			t.Fatal(err)
		}
		choices := memory.ReviewIdentityChoices{
			Object: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true},
		}
		if index < 19 {
			choices.Subject = &memory.ReviewEntityChoice{Create: true}
		}
		chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, authority, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: options.SHA256, Choices: choices})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, chosen.Ref)
	}
	return f, authority, refs
}

func recordBoundBatch(refs []memory.CandidateRef) memory.ReviewBatchRequest {
	request := memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{}}
	for index, ref := range refs {
		request.Groups = append(request.Groups, memory.ReviewBatchGroupRequest{
			ID: fmt.Sprintf("records%d", index), Action: "accept", Candidates: []memory.CandidateRef{ref}, Dependencies: []memory.ReviewDependency{},
		})
	}
	return request
}

func TestOwnerReviewBatchRealSQLiteInclusiveSemanticRecordBound(t *testing.T) {
	ctx := context.Background()
	f, authority, refs := recordBoundCandidates(t)
	// Each full relationship creates two Entities, two Aliases, a Predicate,
	// Claim and Source Link (including lifecycle records): 13 records. Reusing
	// the seeded subject gives 10; the second owner source gives 12.
	for _, test := range []struct{ index, records int }{{0, 13}, {19, 10}, {20, 12}} {
		preview, err := f.store.PrepareOwnerCandidateBatch(ctx, authority, recordBoundBatch([]memory.CandidateRef{refs[test.index]}))
		if err != nil {
			t.Fatal(err)
		}
		if got := len(preview.Groups[0].Preview.Effect.Records); got != test.records {
			t.Fatalf("candidate %d enumerates %d records; want %d", test.index, got, test.records)
		}
	}
	exactRefs := append(append([]memory.CandidateRef{}, refs[:18]...), refs[19], refs[20])
	preview, err := f.store.PrepareOwnerCandidateBatch(ctx, authority, recordBoundBatch(exactRefs))
	if err != nil {
		t.Fatalf("exact 256-record batch rejected: %v", err)
	}
	records := 0
	for _, group := range preview.Groups {
		records += len(group.Preview.Effect.Records)
	}
	if records != 256 || len(preview.Groups) != 20 || len(completeReviewBatchBytes(preview)) > 256*1024 {
		t.Fatalf("exact batch was truncated or crossed another bound: groups=%d records=%d bytes=%d", len(preview.Groups), records, len(completeReviewBatchBytes(preview)))
	}
	var before int
	if err := f.db.QueryRow(`SELECT count(*) FROM memory_review_batch_previews`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	tooManyRefs := append(append([]memory.CandidateRef{}, refs[:19]...), refs[19])
	rejected, err := f.store.PrepareOwnerCandidateBatch(ctx, authority, recordBoundBatch(tooManyRefs))
	if !errors.Is(err, ErrReviewTooLarge) || rejected.ID != "" {
		t.Fatalf("257-record batch returned a preview: %+v, %v", rejected, err)
	}
	var after, accepted int
	if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_review_batch_previews),(SELECT count(*) FROM semantic_operations WHERE schema_version=6)`).Scan(&after, &accepted); err != nil {
		t.Fatal(err)
	}
	if after != before || accepted != 0 {
		t.Fatalf("oversized preparation persisted a batch/effect: previews=%d->%d accepted=%d", before, after, accepted)
	}
	decision := memory.ReviewBatchDecision{
		DeliveryKey: "idem:v1:90000000-0000-4000-8000-000000017144", PreviewID: preview.ID, PreviewSHA256: preview.SHA256,
		Actions: []memory.ReviewBatchAction{},
	}
	for _, group := range preview.Groups {
		decision.Actions = append(decision.Actions, memory.ReviewBatchAction{GroupID: group.ID, Action: "accept"})
	}
	result, err := f.store.ResolveOwnerCandidateBatch(ctx, authority, decision)
	if err != nil || len(result.Groups) != 20 {
		t.Fatalf("resolve exact 256 records: %+v, %v", result, err)
	}
	for _, group := range result.Groups {
		if group.Outcome != "accepted" {
			t.Fatalf("inclusive batch member did not commit: %+v", group)
		}
	}
	replay, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !replay.Valid {
		t.Fatalf("exact-bound accepted replay: %+v, %v", replay, err)
	}
}
