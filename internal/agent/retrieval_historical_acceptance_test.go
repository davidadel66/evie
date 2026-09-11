package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func (f *retrievalFixture) historicalSearch(record memory.Session, tool, query string, options map[string]any) *fakeClient {
	f.t.Helper()
	args := map[string]any{"query": query, "intent": "historical"}
	for key, value := range options {
		args[key] = value
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		f.t.Fatal(err)
	}
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("historical-read", tool, string(encoded))), assistantStep("Historical source evidence received.", nil)}}
	if err := f.session(record, client).Send(context.Background(), "Find the original historical statement and retain its status.", &recorder{}, nil); err != nil {
		f.t.Fatal(err)
	}
	return client
}

func TestHistoricalMemorySearchIncludesMarkedRetiredEvidenceWithoutRestoringAccess(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	accepted := f.remember(source, memory.MemoryEverywhere, "iridium keepsake")
	f.refresh()
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, accepted.ClaimID)
	f.refresh()
	ordinary, _ := f.search(f.global(), "iridium")
	if strings.Contains(retrievalData(t, ordinary.reqs[1]), string(accepted.ClaimID)) {
		t.Fatal("ordinary search revived retired evidence")
	}
	before, err := f.store.InspectClaims(context.Background(), source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	historical := f.historicalSearch(f.global(), "memory_search", "iridium", nil)
	data := retrievalData(t, historical.reqs[1])
	if !strings.Contains(data, string(accepted.ClaimID)) || !strings.Contains(data, `"intent":"historical"`) || !strings.Contains(data, `"current_status":"retired"`) || !strings.Contains(data, string(accepted.Source.EventID)) {
		t.Fatalf("historical evidence lost retirement/provenance: %s", data)
	}
	var projection struct {
		HistoricalOnly []string `json:"historical_only"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "EVIE_MEMORY_DATA\n")), &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection.HistoricalOnly) != 1 || projection.HistoricalOnly[0] != "claim:"+string(accepted.ClaimID) {
		t.Fatalf("provider lacks explicit historical-only evidence label: %+v", projection)
	}
	after, err := f.store.InspectClaims(context.Background(), source.ScopeContext(), memory.ClaimQuery{})
	if err != nil || after.ScopeRevision != before.ScopeRevision || len(after.Claims) != len(before.Claims) {
		t.Fatalf("historical read mutated accepted state: %+v %v", after, err)
	}
	// Exercise restoration and subsequent source retraction as separate changes.
	f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, accepted.ClaimID)
	f.lifecycle(source, "memory_retract_source", memory.SemanticObjectSourceLink, accepted.SourceLinkID)
	restricted := f.historicalSearch(f.global(), "memory_search", "iridium", nil)
	if data := retrievalData(t, restricted.reqs[1]); strings.Contains(data, string(accepted.ClaimID)) || strings.Contains(data, "iridium keepsake") {
		t.Fatalf("historical intent restored revoked source access: %s", data)
	}
}

func TestHistoricalConversationSearchPartitionsRetiredEvidenceAndKnowledgeCutoff(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	basis := f.remember(source, memory.MemoryEverywhere, "historic marker seed")
	event := f.converse(source, "The café plan was topaz; the later garden idea is marigold.")
	claims := f.acceptRangeClaims(source, event, basis, [][2]string{{"topaz", "café plan was topaz"}})
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[0])
	f.refresh()
	ordinary := f.searchConversations(f.global(), "topaz")
	if strings.Contains(retrievalData(t, ordinary.reqs[1]), "topaz") {
		t.Fatal("ordinary conversation search revived retired range")
	}
	historical := f.historicalSearch(f.global(), "memory_search_conversations", "topaz", nil)
	data := retrievalData(t, historical.reqs[1])
	if !strings.Contains(data, "café plan was topaz") || !strings.Contains(data, `"current_status":"retired"`) || !strings.Contains(data, string(event.ID)) {
		t.Fatalf("historical range lost attribution/status: %s", data)
	}
	beforeSource := event.RecordedAt.Add(-time.Nanosecond)
	earlier := f.historicalSearch(f.global(), "memory_search_conversations", "topaz", map[string]any{"as_known_at": beforeSource.Format(time.RFC3339Nano)})
	if data := retrievalData(t, earlier.reqs[1]); strings.Contains(data, "topaz") {
		t.Fatalf("knowledge cutoff disclosed future statement: %s", data)
	}
	atSource := f.historicalSearch(f.global(), "memory_search_conversations", "topaz", map[string]any{"as_known_at": event.RecordedAt.Format(time.RFC3339Nano)})
	if data := retrievalData(t, atSource.reqs[1]); !strings.Contains(data, string(event.ID)) {
		t.Fatalf("inclusive observed-time boundary omitted source: %s", data)
	}
}
