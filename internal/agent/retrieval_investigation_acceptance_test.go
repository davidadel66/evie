package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemoryInvestigationRefreshesCurrentQueryAfterSameTurnCorrection(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	old := f.remember(source, memory.MemoryEverywhere, "celestite marker")
	f.refresh()
	args, err := json.Marshal(map[string]any{
		"idempotency_key": "idem:v1:" + uuid.NewString(), "claim_id": old.ClaimID,
		"subject_entity_id": old.Subject.ID, "predicate_id": old.Predicate.ID,
		"literal_kind": "text", "literal_value": "jade marker", "polarity": "affirmed", "mode": "error",
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("lookup", "memory_search", `{"query":"retrieval marker"}`)),
		assistantStep("", nil, toolCall("correct", "memory_correct_claim", string(args))),
		assistantStep("The approved correction is jade marker.", nil),
	}}
	if err := f.session(reader, client).Send(context.Background(), "Check my saved marker, then correct the mistaken value to jade marker.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		t.Fatal(err)
	}
	inspection, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectClaim, old.ClaimID)
	if err != nil || inspection.Status != memory.SemanticStatusSuperseded {
		t.Fatalf("fixture correction was not accepted: %+v %v", inspection, err)
	}
	first, final := retrievalData(t, client.reqs[1]), retrievalData(t, client.reqs[2])
	if !strings.Contains(first, string(old.ClaimID)) || !strings.Contains(first, "celestite marker") {
		t.Fatal("first request did not receive the original accepted marker")
	}
	if !strings.Contains(final, "jade marker") || strings.Contains(final, string(old.ClaimID)) {
		t.Fatalf("invalidated current query was not refreshed before continuation: %s", final)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var receipts []memory.RetrievalReceipt
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil {
			receipts = append(receipts, *snapshot.Memory)
		}
	}
	if len(receipts) != 2 || len(receipts[0].Evidence) == 0 || receipts[0].Evidence[0].ClaimID != old.ClaimID || len(receipts[1].Evidence) == 0 || receipts[1].Evidence[0].ClaimID == old.ClaimID {
		t.Fatalf("refresh rewrote original receipt or omitted its replacement: %+v", receipts)
	}
}

func TestMemoryInvestigationCanFollowNewInformationWithAFullEvidenceSet(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	for i := 0; i < 8; i++ {
		f.converse(source, fmt.Sprintf("The auroragrid ledger has original entry %d.", i))
	}
	target := f.converse(source, "The bonsai irrigation trial uses a ceramic reservoir.")
	f.refresh()
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("initial", "memory_search_conversations", `{"query":"auroragrid"}`)),
		assistantStep("", nil, toolCall("followup", "memory_search_conversations", `{"query":"bonsai"}`)),
		assistantStep("The follow-up source answers the irrigation question.", nil),
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Investigate the ledger, then the bonsai irrigation trial.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if got := len(expansionBoundaryEvidence(t, client.reqs[1])); got != 8 {
		t.Fatalf("fixture did not fill the held evidence set: %d", got)
	}
	final := expansionBoundaryEvidence(t, client.reqs[2])
	found := false
	for _, item := range final {
		for _, source := range item.Sources {
			found = found || source.EventID == target.ID
		}
	}
	if !found || len(final) > 8 {
		t.Fatalf("new evidence could not replace older context within the same bound: %s", retrievalData(t, client.reqs[2]))
	}
}

// Replayed memory-tool outcomes and resent evidence are charged once per turn
// (harness review C2), so ordinary continuations keep every supported finding
// without exhausting the cumulative memory budget.
func TestMemoryInvestigationChargesReplayedOutcomesOnceAndKeepsSupportedFindings(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	f.remember(source, memory.MemoryEverywhere, "moonstone keepsake")
	f.converse(source, "The indigo observatory experiment is provisional.")
	f.refresh()
	check := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "continue_check", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) { return "Checked the current request.", nil }}
	var delivery memoryDeliveryTally
	first := 0
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		charged := delivery.add(t, request, func(message openrouter.Message) bool {
			return strings.HasPrefix(message.Content, "[begin untrusted semantic memory")
		})
		if charged > 36*1024 {
			t.Errorf("actual cumulative memory delivery escaped the declared 36 KiB bound: %d", charged)
			return assistantStep("A budget violation was observed.", nil)
		}
		if call == 0 {
			return assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"moonstone"}`), toolCall("original", "memory_search_conversations", `{"query":"indigo"}`))
		}
		if call == 1 {
			first = charged
		}
		if charged != first || len(expansionBoundaryEvidence(t, request)) != 2 ||
			strings.Contains(retrievalData(t, request), `"status":"exhausted"`) {
			t.Fatalf("continuation %d was recharged or lost supported findings: charged=%d first=%d", call, charged, first)
		}
		if call == 20 {
			return assistantStep("The supplied sources establish a moonstone keepsake and a provisional observatory experiment.", nil)
		}
		return assistantStep("", nil, toolCall(fmt.Sprintf("check-%d", call), "continue_check", `{}`))
	}}
	if err := f.session(f.global(), client, check).Send(context.Background(), "Investigate the keepsake and observatory, checking the current request as needed.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 21 {
		t.Fatalf("provider requests=%d", len(client.reqs))
	}
}

func TestMemoryInvestigationKeepsFailedSearchDistinctAfterLaterEmptySearch(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	saved := f.remember(source, memory.MemoryEverywhere, "opal ledger")
	f.refresh()
	client := &fakeClient{steps: []step{
		assistantStep("", nil,
			toolCall("supported", "memory_search", `{"query":"opal"}`),
			toolCall("failed", "memory_search_conversations", `{"query":"***"}`),
			toolCall("empty", "memory_search", `{"query":"zirconium"}`)),
		assistantStep("The opal source is available; the failed conversation lookup prevents an exhaustive conclusion.", nil),
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Investigate my saved ledger and the available conversation evidence.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	data := retrievalData(t, client.reqs[1])
	if !strings.Contains(data, `"status":"partial"`) || !strings.Contains(data, string(saved.ClaimID)) {
		t.Fatalf("a later empty search erased the earlier failure or supported result: %s", data)
	}
	statuses := map[string]string{"failed": "failed", "empty": "empty", "supported": "success"}
	for _, message := range client.reqs[1].Messages {
		if want, ok := statuses[message.ToolCallID]; message.Role == "tool" && ok {
			if !strings.Contains(message.Content, `"status":"`+want+`"`) {
				t.Fatalf("individual search outcome changed: %+v", message)
			}
			delete(statuses, message.ToolCallID)
		}
	}
	if len(statuses) != 0 {
		t.Fatalf("missing exact tool outcomes: %+v", statuses)
	}
}
