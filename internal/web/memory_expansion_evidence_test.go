package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestConversationExpansionEvidenceHTTPInspectsAdditionalOriginalPositionsAfterRestart(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	store := eviedb.NewStore(db)
	source, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	sourceClient := &fakeClient{steps: []fakeStep{{content: "Recorded."}, {content: "Recorded."}}}
	sourceRuntime := agent.NewWithToolset(sourceClient, webTestContextProfile("test"), store.BindHistory(source.ID, "expansion-source"), source.ScopeContext(), store.BindTurnOwner(source.ID, "expansion-source"), tools.NewToolset(definitions))
	neighborText := "My mother is Maya; she mentioned a café exhibit."
	anchorText := "She might visit the orchidgallery next month."
	for _, text := range []string{neighborText, anchorText} {
		stream, err := newSSEEvents(httptest.NewRecorder())
		if err != nil {
			t.Fatal(err)
		}
		if err = sourceRuntime.Send(ctx, text, stream, nil); err != nil {
			t.Fatal(err)
		}
	}
	sourceEvents, err := sourceRuntime.HistoryEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var anchorID, neighborID memory.EventID
	originals := make(map[memory.EventID]string)
	for _, event := range sourceEvents {
		originals[event.ID] = event.Content
		if event.Content == anchorText {
			anchorID = event.ID
		}
		if event.Content == neighborText {
			neighborID = event.ID
		}
	}
	coverage, err := store.RefreshMemoryIndex(ctx, 256)
	if err != nil || coverage.State != "active" {
		t.Fatalf("index=%+v: %v", coverage, err)
	}
	reader, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expectedAnchor := fmt.Sprintf("excerpt:%s:0:%d", anchorID, len(anchorText))
	expandArgs, err := json.Marshal(map[string]any{"evidence_id": expectedAnchor, "before": 2, "after": 0})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []fakeStep{
		{toolCalls: []openrouter.ToolCall{{ID: "lookup", Type: "function", Function: openrouter.FunctionCall{Name: "memory_search_conversations", Arguments: `{"query":"orchidgallery"}`}}}},
		{toolCalls: []openrouter.ToolCall{{ID: "expand", Type: "function", Function: openrouter.FunctionCall{Name: "memory_expand_conversation", Arguments: string(expandArgs)}}}},
		{content: "Original surrounding statements received."},
	}}
	runtime := agent.NewWithToolset(client, webTestContextProfile("test"), store.BindHistory(reader.ID, "expansion-reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "expansion-reader"), tools.NewToolset(definitions))
	stream := httptest.NewRecorder()
	streamEvents, err := newSSEEvents(stream)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Send(ctx, "Inspect the earlier tentative discussion.", streamEvents, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stream.Body.String(), `"excerptCount":1`) || !strings.Contains(stream.Body.String(), `"excerptCount":3`) {
		t.Fatalf("search and expanded excerpt activity missing: %s", stream.Body.String())
	}
	history, err := runtime.HistoryEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	type originalRequest struct {
		id      memory.EventID
		receipt memory.RetrievalReceipt
	}
	var requests []originalRequest
	for _, event := range history {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err = json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil && len(snapshot.Memory.Evidence) > 0 {
			requests = append(requests, originalRequest{event.ID, *snapshot.Memory})
		}
	}
	if len(requests) != 2 || len(requests[0].receipt.Evidence) != 1 || requests[0].receipt.Evidence[0].ID != expectedAnchor || len(requests[1].receipt.Evidence) != 3 {
		t.Fatalf("search and expansion did not retain distinct exact requests: %+v", requests)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	runtime = agent.New(nil, webTestContextProfile("test"), store.BindHistory(reader.ID, "reopened-expansion-reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "reopened-expansion-reader"))
	server := NewContextMemoryServer(runtime, nil, nil, nil, store)
	server.activeSession = reader
	for index, original := range requests {
		body, err := json.Marshal(map[string]string{"sessionId": string(reader.ID), "snapshotId": string(original.id)})
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, managementRequest("/api/memory/evidence", string(body)))
		if rr.Code != http.StatusOK || rr.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("inspection status=%d body=%s", rr.Code, rr.Body.String())
		}
		var inspected struct {
			SnapshotID memory.EventID               `json:"snapshotId"`
			Evidence   []memory.RetrievalInspection `json:"evidence"`
		}
		if err = json.Unmarshal(rr.Body.Bytes(), &inspected); err != nil {
			t.Fatal(err)
		}
		if inspected.SnapshotID != original.id || len(inspected.Evidence) != len(original.receipt.Evidence) {
			t.Fatalf("inspection changed the selected original request: %s", rr.Body.String())
		}
		foundNeighbor := false
		for i, item := range inspected.Evidence {
			if !item.Available || item.Evidence == nil || !reflect.DeepEqual(item.Reference, original.receipt.Evidence[i]) || len(item.Evidence.Sources) != 1 {
				t.Fatalf("original reference unavailable or replaced after restart: %+v", item)
			}
			evidence := item.Evidence
			sourceRef := evidence.Sources[0]
			text := originals[sourceRef.EventID]
			if evidence.Kind != memory.RetrievalConversationExcerpt || evidence.ClaimID != "" || evidence.Text != text || sourceRef.SessionID != source.ID || sourceRef.LocatorKind != memory.LocatorUTF8ByteRange || sourceRef.LocatorValue != fmt.Sprintf("0:%d", len(text)) || sourceRef.EvidenceSHA256 != memory.CompilerHash([]byte(text)) {
				t.Fatalf("HTTP inspection lost exact original source positions: %+v", evidence)
			}
			if sourceRef.EventID == neighborID {
				foundNeighbor = true
				if sourceRef.Actor != memory.SemanticActorOwner || sourceRef.Authority != memory.AuthorityOwnerStatement {
					t.Fatalf("expanded neighbor lost attribution: %+v", sourceRef)
				}
			}
		}
		if foundNeighbor != (index == 1) {
			t.Fatalf("later expansion replaced the earlier receipt or lost its additional source: %s", rr.Body.String())
		}
	}
}
