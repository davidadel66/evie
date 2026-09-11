package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func TestReferenceRecallAllowsTargetedHypothesisWithoutChangingIdentity(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	original := referencePreference(f, source, "Maya", "mother", "MOM-27", "jasmine tea")
	f.refresh()
	before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	client := &expansionBoundaryClient{}
	client.reply = func(request openrouter.ChatRequest, call int) step {
		if call == 0 {
			if strings.Contains(retrievalData(t, request), string(original.Claim.ID)) {
				t.Fatal("fixture requires missing initial identity support")
			}
			return assistantStep("", nil, toolCall("targeted-reference", "memory_search", `{"query":"MOM-27"}`))
		}
		found := false
		for _, evidence := range expansionBoundaryEvidence(t, request) {
			found = found || evidence.ClaimID == original.Claim.ID
		}
		if !found {
			t.Fatal("targeted existing read could not resolve the eligible alias hypothesis")
		}
		return assistantStep("If you mean Maya, the original source says she likes jasmine tea. Is Maya the person you mean?", nil)
	}
	if err := f.automaticSession(reader, client).Send(context.Background(), "What present would she like?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 2 {
		t.Fatal("targeted read must precede the scripted clarification response")
	}
	after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil || !reflect.DeepEqual(before.Claims, after.Claims) || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
		t.Fatal("targeted hypothesis changed accepted identity")
	}
}

func TestReferenceRecallMeasuresBoundedInputsWithManyOpaqueHypotheses(t *testing.T) {
	f := newRetrievalFixture(t)
	reader := f.global()
	for i := 0; i < 20; i++ {
		f.converse(reader, strings.Repeat("database checksum discussion ", 30))
	}
	f.refresh()
	client := &fakeClient{steps: []step{assistantStep("Bounded request checked.", nil)}}
	question := "ERR-401 ERR-402 ERR-403 " + strings.Repeat("present uncertainty ", 50)
	if err := f.automaticSession(reader, client).Send(context.Background(), question, &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var input *memory.RetrievalInterpretation
	for _, event := range events {
		if event.Type == memory.EventContextSnapshot {
			var snapshot memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Memory != nil {
				input = snapshot.Memory.Interpretation
			}
		}
	}
	if input == nil || input.CurrentBytes != 512 || input.ExaminedEarlierMessages != 16 || input.ExaminedEarlierBytes != 6144 || input.EarlierMessages > 2 || input.EarlierBytes > 768 || input.ExactSelectors != 2 || input.ExactQueryBytes != 14 || input.QueryBytes > 1024 {
		t.Fatalf("actual complete-turn interpretation bounds = %+v", input)
	}
	wire, err := openrouter.RequestBytes(client.reqs[0])
	if err != nil {
		t.Fatal(err)
	}
	report, _ := json.Marshal(map[string]any{"version": "reference-boundaries-v1", "interpretation": input, "complete_request_bytes": len(wire), "evidence_count": len(expansionBoundaryEvidence(t, client.reqs[0])), "model_quality": "not measured; scripted provider"})
	t.Logf("REFERENCE_BOUND_REPORT=%s", report)
}
