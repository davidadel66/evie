package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestConversationEvidenceHTTPKeepsWorkspaceAttributionWithoutGlobalHistory(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(store), plugins.NewMemory(store))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID, plugins.MemoryPluginID} {
		if err = manager.Enable(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	composition, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspace(ctx, "General")
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	global, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, conversation := range []struct {
		session memory.Session
		text    string
	}{
		{source, "I might grow saffron beside the café window."},
		{global, "My global-only saffron plan stays in Global history."},
	} {
		client := &fakeClient{steps: []fakeStep{{content: "Recorded."}}}
		runtime := agent.NewWithToolset(client, webTestContextProfile("test"), store.BindHistory(conversation.session.ID, "source-writer"), conversation.session.ScopeContext(), store.BindTurnOwner(conversation.session.ID, "source-writer"), composition.Toolset)
		stream, err := newSSEEvents(httptest.NewRecorder())
		if err != nil {
			t.Fatal(err)
		}
		if err = runtime.Send(ctx, conversation.text, stream, nil); err != nil {
			t.Fatal(err)
		}
	}
	coverage, err := store.RefreshMemoryIndex(ctx, 256)
	if err != nil || coverage.State != "active" {
		t.Fatalf("index=%+v: %v", coverage, err)
	}
	reader, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []fakeStep{
		{toolCalls: []openrouter.ToolCall{{ID: "conversation-lookup", Type: "function", Function: openrouter.FunctionCall{Name: "memory_search_conversations", Arguments: `{"query":"saffron"}`}}}},
		{content: "Original conversation evidence received."},
	}}
	runtime := agent.NewWithToolset(client, webTestContextProfile("test"), store.BindHistory(reader.ID, "conversation-reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "conversation-reader"), composition.Toolset)
	stream := httptest.NewRecorder()
	streamEvents, err := newSSEEvents(stream)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Send(ctx, "Find my earlier gardening discussion.", streamEvents, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stream.Body.String(), `"excerptCount":1`) || !strings.Contains(stream.Body.String(), `"acceptedCount":0`) {
		t.Fatalf("missing excerpt activity: %s", stream.Body.String())
	}
	events, err := runtime.HistoryEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var snapshotID memory.EventID
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err = json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil && len(snapshot.Memory.Evidence) == 1 {
			snapshotID = event.ID
		}
	}
	if snapshotID == "" {
		t.Fatal("conversation search did not retain an original evidence receipt")
	}
	server := NewContextMemoryServer(runtime, nil, nil, nil, store)
	server.activeSession = reader
	body, err := json.Marshal(map[string]string{"sessionId": string(reader.ID), "snapshotId": string(snapshotID)})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/memory/evidence", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var receipt struct {
		Evidence []memory.RetrievalInspection `json:"evidence"`
	}
	if err = json.Unmarshal(rr.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Evidence) != 1 || !receipt.Evidence[0].Available || receipt.Evidence[0].Evidence == nil {
		t.Fatalf("unavailable excerpt: %s", rr.Body.String())
	}
	evidence := receipt.Evidence[0].Evidence
	if evidence.Kind != memory.RetrievalConversationExcerpt || evidence.ClaimID != "" || evidence.Text != "I might grow saffron beside the café window." || len(evidence.Sources) != 1 || evidence.Sources[0].SessionID != source.ID || evidence.Sources[0].Actor != memory.SemanticActorOwner {
		t.Fatalf("excerpt lost identity or attribution: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "global-only") || strings.Contains(rr.Body.String(), string(global.ID)) || strings.Contains(rr.Body.String(), `"claim_id"`) {
		t.Fatalf("excerpt gained unrelated history or a fabricated Claim: %s", rr.Body.String())
	}
}
