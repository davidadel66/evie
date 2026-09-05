package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func reviewPromotionFixture(t *testing.T) (*compilerFixture, memory.PromotionProposal) {
	t.Helper()
	ctx := context.Background()
	f, original, _ := reviewCandidateFixture(t)
	selected := original.Window.Selection
	selected.Destination = "session:" + string(f.session.ID)
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return compilerOutput(r, []memory.ExtractorCandidate{original.Candidates[0].Proposal}), nil
	}}
	compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selected, compilerGeneration(), extractor)
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("session compile %+v %v", compiled, err)
	}
	authority, err := f.store.LocalOwnerReviewContext(ctx, selected.Destination)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "90000000-0000-4000-8000-000000000170"))
	if err != nil {
		t.Fatal(err)
	}
	event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote the reviewed café preference to global memory."})
	proposal, err := f.store.PreparePromotion(ctx, f.session.ScopeContext(), memory.PromotionRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000171", SourceEventID: event.ID, SourceClaimID: accepted.Operation.ClaimIDs[0], DestinationScopeKey: "global"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Sources) != 1 || proposal.Sources[0].Evidence != "café" {
		t.Fatalf("promotion disclosure %+v", proposal.Sources)
	}
	payload, err := json.Marshal(memory.ApprovalPayload{Decision: memory.ApprovalApproved, ProposalSHA256: proposal.ProposalSHA256, PreparedSHA256: proposal.PreparedSHA256})
	if err != nil {
		t.Fatal(err)
	}
	f.append(t, memory.EventInput{ParentID: proposal.Evidence.EventID, Type: memory.EventApproval, ExecutionID: memory.ExecutionID(proposal.OperationID), Payload: payload})
	return f, proposal
}

func TestOwnerReviewPromotionRevalidatesPolicyBeforeApply(t *testing.T) {
	ctx := context.Background()
	f, proposal := reviewPromotionFixture(t)
	if _, err := f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-detector-v2'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ApplyPromotion(ctx, f.lease, proposal); !errors.Is(err, eviedb.ErrReviewInvalidSource) {
		t.Fatalf("changed policy accepted promotion: %v", err)
	}
	var operations int
	if err := f.db.QueryRow(`SELECT count(*) FROM semantic_operations WHERE operation_id=?`, proposal.OperationID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatal("failed promotion wrote accepted operation")
	}
	hidden, err := f.store.PreparePromotion(ctx, f.session.ScopeContext(), proposal.Request)
	if !errors.Is(err, eviedb.ErrReviewInvalidSource) || len(hidden.Sources) != 0 {
		t.Fatalf("redacted evidence became executable %+v %v", hidden, err)
	}
}

func TestOwnerReviewPromotionRetainsPolicyAndContextAfterCommit(t *testing.T) {
	ctx := context.Background()
	f, proposal := reviewPromotionFixture(t)
	result, err := f.store.ApplyPromotion(ctx, f.lease, proposal)
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectSourceLink, proposal.Sources[0].ID)
	if err != nil || source.Source.Evidence != "café" {
		t.Fatalf("promoted exact source %+v %v", source, err)
	}
	if len(source.Operations) != 1 || source.Operations[0].SchemaVersion != 4 || source.Operations[0].PreparedJSON == "" {
		t.Fatalf("promotion operation %+v", source.Operations)
	}
	var original string
	if err = f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, result.OperationID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-detector-v2',revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	source, err = f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectSourceLink, proposal.Sources[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if source.Source.Evidence != "" || len(source.Operations) != 1 || source.Operations[0].PreparedJSON != "" || source.Operations[0].ProposalJSON != "" || source.Operations[0].ResultJSON != "" {
		t.Fatalf("promotion lost origin protection %+v", source)
	}
	claim, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectClaim, result.DestinationClaimID)
	if err != nil || len(claim.Sources) != 1 || claim.Sources[0].Source.Evidence != "" {
		t.Fatalf("promoted Claim disclosure %+v %v", claim, err)
	}
	read, err := f.store.InspectLiteralClaims(ctx, f.session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	for _, claim := range read.Claims {
		if claim.ID == result.DestinationClaimID && claim.Source.Evidence != "" {
			t.Fatalf("exact read leaked promoted evidence %+v", claim)
		}
	}
	// Reusing an accepted delivery is not a fresh approval, but inspecting its
	// stored proposal is still subject to current source disclosure policy.
	repeated, err := f.store.ApplyPromotion(ctx, f.lease, proposal)
	if err != nil || repeated.OperationID != result.OperationID {
		t.Fatalf("accepted delivery changed %+v %v", repeated, err)
	}
	if hidden, err := f.store.PreparePromotion(ctx, f.session.ScopeContext(), proposal.Request); !errors.Is(err, eviedb.ErrReviewInvalidSource) || len(hidden.Sources) != 0 {
		t.Fatalf("accepted proposal bypass %+v %v", hidden, err)
	}
	verified, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !verified.Valid {
		t.Fatalf("current policy changed historical promotion replay %+v %v", verified, err)
	}
	var after string
	f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, result.OperationID).Scan(&after)
	if after != original {
		t.Fatal("historical promotion was rewritten")
	}
}
