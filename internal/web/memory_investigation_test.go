package web

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestMemoryInvestigationHTTPPreservesPartialAfterFailedAndEmptySearches(t *testing.T) {
	f := newReceiptHTTPFixture(t)
	client := &fakeClient{steps: []fakeStep{
		receiptHTTPTool("supported", "memory_search", `{"query":"azurefolio"}`),
		receiptHTTPTool("failed", "memory_search_conversations", `{"query":"***"}`),
		receiptHTTPTool("empty", "memory_search", `{"query":"zirconium"}`),
		{content: "The accepted marker is azurefolio. The failed conversation search leaves a gap; the later empty search does not resolve it."},
	}}
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	holder := memory.LeaseHolderID("investigation-http")
	runtime := agent.NewWithToolset(client, webTestContextProfile("test"), f.store.BindHistory(f.reader.ID, holder), f.reader.ScopeContext(), f.store.BindTurnOwner(f.reader.ID, holder), tools.NewToolset(definitions), agent.WithAutomaticMemoryRecall(false))
	server := NewContextMemoryServer(runtime, nil, nil, &fakeContextSessionController{}, f.store)
	server.activeSession = f.reader
	stream := httptest.NewRecorder()
	server.Handler().ServeHTTP(stream, chatRequest(fmt.Sprintf(`{"sessionId":%q,"message":"Investigate the saved marker and available conversation evidence."}`, f.reader.ID)))
	if stream.Code != http.StatusOK || strings.Contains(stream.Body.String(), "event: error\n") || !strings.HasSuffix(stream.Body.String(), "event: turn_done\ndata: {}\n\n") {
		t.Fatalf("HTTP turn did not complete: status=%d %s", stream.Code, stream.Body.String())
	}
	if len(client.reqs) != 4 {
		t.Fatalf("provider requests=%d, want separate continuations for all three outcomes", len(client.reqs))
	}

	events, err := f.store.LoadEvents(context.Background(), f.reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshots, responses []memory.Event
	for _, event := range events {
		switch event.Type {
		case memory.EventContextSnapshot:
			snapshots = append(snapshots, event)
		case memory.EventAssistantMessage:
			responses = append(responses, event)
		}
	}
	if len(snapshots) != 4 || len(responses) != 4 {
		t.Fatalf("saved requests/responses=%d/%d", len(snapshots), len(responses))
	}
	answer := responses[3].ID
	wantStatuses := []string{"success", "partial", "partial"}
	activities := investigationSSEData(t, stream.Body.String(), "memory_activity")
	if len(activities) != len(wantStatuses) {
		t.Fatalf("live memory activity=%s", activities)
	}
	for i, raw := range activities {
		var activity memoryActivity
		if err := json.Unmarshal(raw, &activity); err != nil {
			t.Fatal(err)
		}
		if activity.Status != wantStatuses[i] || activity.SnapshotID != snapshots[i+1].ID || activity.Iteration != i+2 || activity.RequestStatus != "prepared" || activity.AcceptedCount != 1 || activity.ExcerptCount != 0 {
			t.Fatalf("live activity erased the investigation state or changed its request: %s", raw)
		}
		if strings.Contains(string(raw), string(f.accepted.ClaimID)) || strings.Contains(string(raw), "azurefolio") {
			t.Fatalf("compact activity exposed source content or identity: %s", raw)
		}
	}
	outcomes := map[string]string{"supported": "success", "failed": "failed", "empty": "empty"}
	liveResults := investigationSSEData(t, stream.Body.String(), "tool_result")
	if len(liveResults) != len(outcomes) {
		t.Fatalf("live tool results=%s", liveResults)
	}
	for _, raw := range liveResults {
		var result struct{ ID, Content string }
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if want, ok := outcomes[result.ID]; !ok || !strings.Contains(result.Content, `"status":"`+want+`"`) {
			t.Fatalf("live tool outcome lost its own status: %s", raw)
		}
	}

	history := httptest.NewRecorder()
	server.Handler().ServeHTTP(history, managementRequest("/api/context-sessions/history", fmt.Sprintf(`{"sessionId":%q}`, f.reader.ID)))
	var page struct{ Items []historyItem }
	if history.Code != http.StatusOK || json.Unmarshal(history.Body.Bytes(), &page) != nil {
		t.Fatalf("history=%d %s", history.Code, history.Body.String())
	}
	memoryIndex, toolCount := 0, 0
	for _, item := range page.Items {
		if item.Memory != nil {
			if memoryIndex >= len(wantStatuses) || item.Memory.Status != wantStatuses[memoryIndex] || item.Memory.SnapshotID != snapshots[memoryIndex+1].ID || item.Memory.RequestStatus != "completed" || item.Memory.AnswerID != answer || item.Memory.AcceptedCount != 1 {
				t.Fatalf("history activity differs from the original request: %+v", item.Memory)
			}
			memoryIndex++
		}
		if item.Kind == "tool" {
			want, ok := outcomes[item.ID]
			if !ok || item.Result == nil || !strings.Contains(*item.Result, `"status":"`+want+`"`) {
				t.Fatalf("history tool outcome lost its own status: %+v", item)
			}
			toolCount++
		}
	}
	if memoryIndex != 3 || toolCount != 3 {
		t.Fatalf("history lost activities or tools: memory=%d tools=%d", memoryIndex, toolCount)
	}

	inspection := f.inspect(runtime, map[string]string{"answerId": string(answer)})
	var inspected memoryEvidenceResponse
	if inspection.Code != http.StatusOK || json.Unmarshal(inspection.Body.Bytes(), &inspected) != nil {
		t.Fatalf("inspection=%d %s", inspection.Code, inspection.Body.String())
	}
	if inspected.AnswerID != answer || inspected.Status != "partial" || len(inspected.Requests) != 4 {
		t.Fatalf("answer inspection lost the partial state or request sequence: %s", inspection.Body.String())
	}
	charged := 0
	for i, request := range client.reqs {
		wire, err := openrouter.RequestBytes(request)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(snapshots[i].Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		original := inspected.Requests[i]
		if snapshot.RequestSHA256 != fmt.Sprintf("%x", sha256.Sum256(wire)) || snapshot.SerializedBytes != int64(len(wire)) || original.RequestSHA256 != snapshot.RequestSHA256 || original.SerializedBytes != snapshot.SerializedBytes || original.SnapshotID != snapshots[i].ID || original.ResponseID != responses[i].ID || original.Iteration != i+1 || original.RequestStatus != "completed" {
			t.Fatalf("request %d receipt does not describe the actual provider bytes: %+v", i+1, original)
		}
		replayed := 0
		for _, message := range request.Messages {
			want, retrievalOutcome := outcomes[message.ToolCallID]
			if message.Role == "tool" && retrievalOutcome {
				if !strings.Contains(message.Content, `"status":"`+want+`"`) {
					t.Fatalf("request %d changed a replayed tool outcome: %+v", i+1, message)
				}
				replayed++
			}
			if message.Role == "user" && strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") || message.Role == "tool" && retrievalOutcome {
				encoded, err := json.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				charged += len(encoded)
			}
		}
		if replayed != i {
			t.Fatalf("request %d lost an individual tool outcome: replayed=%d", i+1, replayed)
		}
		if i == 0 {
			if snapshot.Memory != nil || len(original.Evidence) != 0 {
				t.Fatal("pre-search request claimed retrieved evidence")
			}
			continue
		}
		if snapshot.Memory == nil || snapshot.Memory.Status != wantStatuses[i-1] || original.Status != snapshot.Memory.Status || len(snapshot.Memory.Evidence) != 1 || len(original.Evidence) != 1 || !original.Evidence[0].Available || !reflect.DeepEqual(original.Evidence[0].Reference, snapshot.Memory.Evidence[0]) || snapshot.Memory.Evidence[0].ClaimID != f.accepted.ClaimID {
			t.Fatalf("request %d lost its immutable accepted evidence or state: %+v", i+1, original)
		}
		accounting := snapshot.Memory.Investigation
		if accounting == nil || accounting.SearchAttempts != i || accounting.RefreshAttempts != 0 || accounting.ReusedEvidence != min(i-1, 1) || accounting.CumulativeMemoryBytes != charged || charged > 36*1024 || !reflect.DeepEqual(accounting.Outcomes, []string{"success", "failed", "empty"}[:i]) {
			t.Fatalf("request %d accounting does not match actual delivered memory messages (%d bytes): %+v", i+1, charged, accounting)
		}
	}
}

func investigationSSEData(t *testing.T, stream, event string) []json.RawMessage {
	t.Helper()
	var payloads []json.RawMessage
	prefix := "event: " + event + "\ndata: "
	for _, block := range strings.Split(stream, "\n\n") {
		if raw, ok := strings.CutPrefix(block, prefix); ok {
			if !json.Valid([]byte(raw)) {
				t.Fatalf("invalid %s SSE JSON: %q", event, raw)
			}
			payloads = append(payloads, json.RawMessage(raw))
		}
	}
	return payloads
}
