package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemoryInvestigationDoesNotRefreshThroughRetiredAlias(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	original := referencePreference(f, source, "Maya", "mother", "MOM-27", "jasmine tea")
	f.refresh()
	var alias memory.SemanticID
	for _, item := range original.Aliases {
		if item.Value == "MOM-27" {
			alias = item.ID
		}
	}
	if alias == "" {
		t.Fatal("fixture lacks alias")
	}
	retire, _ := json.Marshal(map[string]any{"idempotency_key": "idem:v1:" + uuid.NewString(), "object_kind": "alias", "object_id": alias})
	correct, _ := json.Marshal(map[string]any{"idempotency_key": "idem:v1:" + uuid.NewString(), "claim_id": original.Claim.ID, "subject_entity_id": original.Claim.SubjectEntityID, "predicate_id": original.Claim.PredicateID, "object_entity_id": original.Claim.ObjectEntityID, "polarity": "denied", "mode": "error"})
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("search", "memory_search", `{"query":"MOM-27"}`))
		case 1:
			if !strings.Contains(retrievalData(t, request), `"alias_value":"MOM-27"`) {
				t.Fatal("original exact mapping not supplied")
			}
			return assistantStep("", nil, toolCall("retire", "memory_retire", string(retire)), toolCall("correct", "memory_correct_claim", string(correct)))
		default:
			return assistantStep("The former alias cannot establish the current recipient. Who should I use?", nil)
		}
	}}
	if err := f.session(reader, client).Send(context.Background(), "Read MOM-27, then retire that alias and correct the preference.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		t.Fatal(err)
	}
	aliasState, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectAlias, alias)
	if err != nil || aliasState.Status != memory.SemanticStatusRetired {
		t.Fatalf("alias mutation not accepted: %+v %v", aliasState, err)
	}
	claimState, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectClaim, original.Claim.ID)
	if err != nil || claimState.Status != memory.SemanticStatusSuperseded {
		t.Fatalf("correction not accepted: %+v %v", claimState, err)
	}
	final := retrievalData(t, client.reqs[2])
	if strings.Contains(final, "chrysanthemum tea") || strings.Contains(final, "jasmine tea") || !strings.Contains(final, `"status":"unavailable"`) {
		t.Fatalf("refresh used a retired identity to supply person-specific evidence: %s", final)
	}
	receipts := investigationReceipts(t, f, reader)
	if len(receipts) != 2 || len(receipts[0].Evidence) != 1 || len(receipts[0].Evidence[0].IdentityMatches) != 1 || len(receipts[1].Evidence) != 0 {
		t.Fatalf("refresh lost immutable original identity or fabricated current mapping: %+v", receipts)
	}
}
