package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestConversationExpansionResolvesTentativePronounFromOriginalNeighbors(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	subject := f.converse(source, "Maya is considering a visit to Kyoto.")
	pronoun := f.converse(source, "She hasn't booked it yet.")
	f.refresh()
	before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	anchor := fmt.Sprintf("excerpt:%s:0:%d", pronoun.ID, len(pronoun.Content))
	args, _ := json.Marshal(map[string]any{"evidence_id": anchor, "before": 2, "after": 0})
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("find-pronoun", "memory_search_conversations", `{"query":"booked"}`)),
		assistantStep("", nil, toolCall("read-neighbors", "memory_expand_conversation", string(args))),
		assistantStep("The source describes Maya's possible trip; it remains unbooked.", nil),
	}}
	if err := f.session(reader, client).Send(context.Background(), "Who was considering that trip?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	initial := retrievalData(t, client.reqs[1])
	expanded := retrievalData(t, client.reqs[2])
	if strings.Contains(initial, string(subject.ID)) || !strings.Contains(initial, anchor) {
		t.Fatalf("initial hit lost narrow anchor: %s", initial)
	}
	if !strings.Contains(expanded, string(subject.ID)) || !strings.Contains(expanded, "Maya is considering") || !strings.Contains(expanded, "hasn't booked") || !strings.Contains(expanded, "conversation_expansion") {
		t.Fatalf("expanded request lacks attributed context: %s", expanded)
	}
	after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil || after.ScopeRevision != before.ScopeRevision || len(after.Claims) != len(before.Claims) {
		t.Fatalf("expansion changed accepted memory: %+v %v", after, err)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var receipts []*memory.RetrievalReceipt
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil {
			receipts = append(receipts, snapshot.Memory)
		}
	}
	if len(receipts) != 2 || len(receipts[0].Evidence) != 1 || len(receipts[1].Evidence) != 3 {
		t.Fatalf("request receipts did not capture exact additions: %+v", receipts)
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	for _, receipt := range receipts {
		inspected, err := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), receipt.Evidence)
		if err != nil || len(inspected) != len(receipt.Evidence) {
			t.Fatalf("restart inspection: %+v %v", inspected, err)
		}
		for _, item := range inspected {
			if !item.Available || item.Evidence.ID != item.Reference.ID {
				t.Fatalf("original expanded source unavailable: %+v", item)
			}
		}
	}

}
