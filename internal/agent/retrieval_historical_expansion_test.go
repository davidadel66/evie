package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestHistoricalConversationExpansionInheritsAnchorIntentAndKnowledgePin(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	basis := f.remember(source, memory.MemoryEverywhere, "historical expansion seed")
	neighbor := f.converse(source, "The ambertrip to Kyoto was only tentative.")
	anchor := f.converse(source, "The opalbooking discussion refers to that earlier possibility.")
	claims := f.acceptRangeClaims(source, neighbor, basis, [][2]string{{"ambertrip", "ambertrip to Kyoto was only tentative"}})
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[0])
	f.refresh()
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"opalbooking","intent":"historical","as_known_at":"`+anchor.RecordedAt.Format(time.RFC3339Nano)+`"}`))
		case 1:
			found := expansionBoundaryFind(t, expansionBoundaryEvidence(t, request), anchor.ID)
			return expansionBoundaryCall(t, "expand", found.ID, 2, 0)
		default:
			return assistantStep("The historical statement describes a tentative possibility.", nil)
		}
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Explain the earlier tentative discussion.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, evidence := range expansionBoundaryEvidence(t, client.reqs[2]) {
		if evidence.Sources[0].EventID != neighbor.ID || !strings.Contains(evidence.Text, "ambertrip") {
			continue
		}
		found = true
		if evidence.Intent != memory.RetrievalHistorical || !evidence.AsKnownAt.Equal(anchor.RecordedAt) || evidence.Status != memory.SemanticStatusActive || evidence.CurrentStatus != memory.SemanticStatusRetired || evidence.ClaimID != "" {
			t.Fatalf("expanded passage changed historical intent, knowledge pin or attribution: %+v", evidence)
		}
	}
	if !found {
		t.Fatalf("historical expansion omitted corresponding retired original context: %s", retrievalData(t, client.reqs[2]))
	}
}

func TestMemoryCorrectionBeforeDispatchPreservesOriginalReadPinAndFreshLifecycle(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	accepted := f.remember(source, memory.MemoryEverywhere, "celestite marker")
	f.refresh()
	effective := time.Now().UTC().Add(24 * time.Hour)
	args, err := json.Marshal(map[string]any{
		"idempotency_key": "idem:v1:" + uuid.NewString(), "claim_id": accepted.ClaimID,
		"subject_entity_id": accepted.Subject.ID, "predicate_id": accepted.Predicate.ID,
		"literal_kind": "text", "literal_value": "jade marker", "polarity": "affirmed", "mode": "changed", "effective_time": effective.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("lookup", "memory_search", `{"query":"celestite"}`), toolCall("correction", "memory_correct_claim", string(args))),
		assistantStep("The marker remains celestite until the approved future change.", nil),
	}}
	if err := f.session(reader, client).Send(context.Background(), "Look up my saved marker, then record that it will change to jade tomorrow.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		t.Fatal(err)
	}
	current, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectClaim, accepted.ClaimID)
	if err != nil || current.Status != memory.SemanticStatusSuperseded {
		t.Fatalf("fixture correction was not accepted: %+v %v", current, err)
	}
	for _, evidence := range expansionBoundaryEvidence(t, client.reqs[1]) {
		if evidence.ClaimID != accepted.ClaimID {
			continue
		}
		if evidence.Status != memory.SemanticStatusActive || evidence.CurrentStatus != memory.SemanticStatusSuperseded || evidence.CorrectionMode != "" || evidence.CurrentCorrectionMode != memory.CorrectionChanged || evidence.EffectiveValidTime == nil || evidence.EffectiveValidTime.To != nil {
			t.Fatalf("new correction state was attached to the earlier read pin: %+v", evidence)
		}
		if !evidence.AsKnownAt.Before(current.Lifecycle[len(current.Lifecycle)-1].TransactionTime) {
			t.Fatalf("fixture did not correct after the original read pin: %+v", evidence)
		}
		return
	}
	t.Fatalf("still-applicable original evidence vanished: %s", retrievalData(t, client.reqs[1]))
}

func TestOriginalMemoryReceiptRemainsInspectableAfterRetirementButNotSourceRevocation(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	accepted := f.remember(source, memory.MemoryEverywhere, "citrine archive")
	f.refresh()
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"citrine"}`), toolCall("original", "memory_search_conversations", `{"query":"citrine"}`)),
		assistantStep("Both forms of attributed evidence received.", nil),
	}}
	if err := f.session(reader, client).Send(context.Background(), "Find the saved marker and its original statement.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var refs []memory.RetrievalReference
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err = json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil {
			refs = snapshot.Memory.Evidence
		}
	}
	if len(refs) != 2 {
		t.Fatalf("fixture requires accepted and original excerpt receipts: %+v", refs)
	}
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, accepted.ClaimID)
	inspected, err := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), refs)
	if err != nil || len(inspected) != len(refs) {
		t.Fatalf("inspect original receipt: %+v %v", inspected, err)
	}
	for _, item := range inspected {
		if !item.Available || item.Evidence == nil || item.CurrentStatus != memory.SemanticStatusRetired || item.Evidence.Status != memory.SemanticStatusActive || item.Evidence.Intent != memory.RetrievalCurrent || item.Reference.CurrentStatus != memory.SemanticStatusActive {
			t.Fatalf("retirement erased or relabeled the original supplied receipt: %+v", item)
		}
	}
	f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, accepted.ClaimID)
	f.lifecycle(source, "memory_retract_source", memory.SemanticObjectSourceLink, accepted.SourceLinkID)
	inspected, err = f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), refs)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inspected {
		if item.Available || item.Evidence != nil {
			t.Fatalf("original receipt restored revoked source access: %+v", item)
		}
	}
}
