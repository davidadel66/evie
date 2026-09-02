package agent

import (
	"context"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

type recordingSemanticMemory struct {
	applyLiteralCalls    int
	literalProposal      memory.RememberLiteralProposal
	applyCorrectionCalls int
	correctionProposal   memory.CorrectClaimProposal
}

func (r *recordingSemanticMemory) PrepareRememberLiteral(context.Context, memory.ScopeContext, memory.RememberLiteralRequest) (memory.RememberLiteralProposal, error) {
	return memory.RememberLiteralProposal{}, nil
}

func (r *recordingSemanticMemory) ApplyRememberLiteral(_ context.Context, _ memory.TurnLease, proposal memory.RememberLiteralProposal) (memory.RememberLiteralResult, error) {
	r.applyLiteralCalls++
	r.literalProposal = proposal
	return memory.RememberLiteralResult{OperationID: proposal.OperationID, ClaimID: proposal.ClaimID}, nil
}

func (r *recordingSemanticMemory) InspectLiteralClaims(context.Context, memory.ScopeContext) (memory.LiteralClaimsInspection, error) {
	return memory.LiteralClaimsInspection{}, nil
}

func (r *recordingSemanticMemory) PrepareRememberEntity(context.Context, memory.ScopeContext, memory.RememberEntityRequest) (memory.RememberEntityProposal, error) {
	return memory.RememberEntityProposal{}, nil
}

func (r *recordingSemanticMemory) ApplyRememberEntity(context.Context, memory.TurnLease, memory.RememberEntityProposal) (memory.RememberEntityResult, error) {
	return memory.RememberEntityResult{}, nil
}

func (r *recordingSemanticMemory) InspectEntityClaims(context.Context, memory.ScopeContext) (memory.EntityClaimsInspection, error) {
	return memory.EntityClaimsInspection{}, nil
}

func (r *recordingSemanticMemory) InspectEntityClaimsAtScope(context.Context, memory.ScopeContext, bool) (memory.EntityClaimsInspection, error) {
	return memory.EntityClaimsInspection{}, nil
}

func (r *recordingSemanticMemory) LookupEntitiesByAlias(context.Context, memory.ScopeContext, string) ([]memory.AliasEntityMatch, error) {
	return nil, nil
}

func (r *recordingSemanticMemory) LookupEntitiesByAliasAtScope(context.Context, memory.ScopeContext, string, bool) ([]memory.AliasEntityMatch, error) {
	return nil, nil
}

func (r *recordingSemanticMemory) InspectSemanticEntity(context.Context, memory.ScopeContext, memory.SemanticID) (memory.SemanticEntity, error) {
	return memory.SemanticEntity{}, nil
}

func (r *recordingSemanticMemory) InspectSemanticEntityAtScope(context.Context, memory.ScopeContext, memory.SemanticID, bool) (memory.SemanticEntity, error) {
	return memory.SemanticEntity{}, nil
}

func (r *recordingSemanticMemory) PrepareCorrectClaim(context.Context, memory.ScopeContext, memory.CorrectClaimRequest) (memory.CorrectClaimProposal, error) {
	return memory.CorrectClaimProposal{}, nil
}

func (r *recordingSemanticMemory) ApplyCorrectClaim(_ context.Context, _ memory.TurnLease, proposal memory.CorrectClaimProposal) (memory.CorrectClaimResult, error) {
	r.applyCorrectionCalls++
	r.correctionProposal = proposal
	return memory.CorrectClaimResult{OperationID: proposal.OperationID, ReplacementClaimID: proposal.ReplacementClaim.ID}, nil
}

func (r *recordingSemanticMemory) InspectClaims(context.Context, memory.ScopeContext, memory.ClaimQuery) (memory.ClaimsInspection, error) {
	return memory.ClaimsInspection{}, nil
}

func TestRememberLiteralAppliesNewPredicateDefinitionAndClaimOnlyAfterApproval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		decision  tools.Decision
		wantApply int
	}{
		{name: "approved", decision: tools.Approved, wantApply: 1},
		{name: "declined", decision: tools.Declined, wantApply: 0},
		{name: "expired", decision: tools.Expired, wantApply: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := newTestSession(nil, "test-model")
			semantic := &recordingSemanticMemory{}
			proposal := memory.RememberLiteralProposal{
				OperationID: "60000000-0000-4000-8000-000000000001",
				SessionID:   "test-session",
				Predicate: memory.SemanticPredicate{
					ID: "60000000-0000-4000-8000-000000000002", Token: "home_city", Version: 2,
					Label: "cities called home", ObjectConstraint: memory.PredicateObjectConstraint(memory.LiteralText),
					Cardinality: memory.CardinalityMany, Create: true,
				},
				ClaimID: "60000000-0000-4000-8000-000000000003", ClaimCreate: true,
				Literal:  memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
				Polarity: memory.PolarityAffirmed,
				Source:   memory.SemanticSource{EventID: "source-event"},
			}
			result, err := session.ResolveRememberLiteral(context.Background(), semantic, proposal, test.decision)
			if err != nil {
				t.Fatal(err)
			}
			if semantic.applyLiteralCalls != test.wantApply {
				t.Fatalf("ApplyRememberLiteral calls = %d, want %d", semantic.applyLiteralCalls, test.wantApply)
			}
			if test.wantApply == 1 && (result.OperationID != proposal.OperationID ||
				semantic.literalProposal.Predicate != proposal.Predicate || semantic.literalProposal.ClaimID != proposal.ClaimID) {
				t.Fatalf("approved exact proposal changed: result=%+v applied=%+v", result, semantic.literalProposal)
			}
		})
	}
}

func TestCorrectClaimAppliesExactProposalOnlyAfterApproval(t *testing.T) {
	for _, test := range []struct {
		name      string
		decision  tools.Decision
		wantApply int
	}{
		{name: "approved", decision: tools.Approved, wantApply: 1},
		{name: "declined", decision: tools.Declined, wantApply: 0},
		{name: "expired", decision: tools.Expired, wantApply: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := newTestSession(nil, "test-model")
			semantic := &recordingSemanticMemory{}
			proposal := memory.CorrectClaimProposal{
				OperationID: "60000000-0000-4000-8000-000000000071", SessionID: "test-session",
				OldClaim:         memory.SemanticClaim{ID: "60000000-0000-4000-8000-000000000072"},
				ReplacementClaim: memory.SemanticClaim{ID: "60000000-0000-4000-8000-000000000073"},
				Source:           memory.SemanticSource{EventID: "source-event"}, Mode: memory.CorrectionError,
			}
			result, err := session.ResolveCorrectClaim(context.Background(), semantic, proposal, test.decision)
			if err != nil {
				t.Fatal(err)
			}
			if semantic.applyCorrectionCalls != test.wantApply {
				t.Fatalf("ApplyCorrectClaim calls = %d, want %d", semantic.applyCorrectionCalls, test.wantApply)
			}
			if test.wantApply == 1 && (result.ReplacementClaimID != proposal.ReplacementClaim.ID ||
				semantic.correctionProposal.Mode != proposal.Mode || semantic.correctionProposal.OldClaim.ID != proposal.OldClaim.ID) {
				t.Fatalf("approved exact correction changed: result=%+v applied=%+v", result, semantic.correctionProposal)
			}
		})
	}
}
