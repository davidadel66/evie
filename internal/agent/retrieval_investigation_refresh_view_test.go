package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemoryInvestigationRefreshesLostClaimWithoutUndoingAnotherExplicitView(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	changed := f.remember(source, memory.MemoryEverywhere, "amethyst mineral")
	preserved := f.remember(source, memory.MemoryEverywhere, "garnet mineral")
	f.refresh()
	historical, _ := json.Marshal(map[string]any{"query": preserved.ClaimID, "intent": "historical"})
	correct, _ := json.Marshal(map[string]any{"idempotency_key": "idem:v1:" + uuid.NewString(), "claim_id": changed.ClaimID, "subject_entity_id": changed.Subject.ID, "predicate_id": changed.Predicate.ID, "literal_kind": "text", "literal_value": "jasper mineral", "polarity": "affirmed", "mode": "error"})
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("initial", "memory_search", `{"query":"mineral"}`)),
		assistantStep("", nil, toolCall("explicit-view", "memory_search", string(historical))),
		assistantStep("", nil, toolCall("correct", "memory_correct_claim", string(correct))),
		assistantStep("The current correction is jasper, with the separately selected garnet historical view retained.", nil),
	}}
	if err := f.session(reader, client).Send(context.Background(), "Compare minerals, select the garnet historical view, then correct amethyst to jasper.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		t.Fatal(err)
	}
	final := retrievalData(t, client.reqs[3])
	if !strings.Contains(final, "jasper mineral") || strings.Contains(final, string(changed.ClaimID)) {
		t.Fatalf("a newer view of a different result suppressed correction refresh: %s", final)
	}
	var before, after memory.RetrievalReference
	for _, item := range expansionBoundaryEvidence(t, client.reqs[2]) {
		if item.ClaimID == preserved.ClaimID {
			before = item.Reference()
		}
	}
	for _, item := range expansionBoundaryEvidence(t, client.reqs[3]) {
		if item.ClaimID == preserved.ClaimID {
			after = item.Reference()
		}
	}
	if before.ClaimID == "" || before.Intent != memory.RetrievalHistorical || !reflect.DeepEqual(before, after) {
		t.Fatalf("refresh undid explicit historical view: before=%+v after=%+v", before, after)
	}
}
