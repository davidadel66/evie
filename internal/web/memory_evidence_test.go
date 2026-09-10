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

func TestMemoryEvidenceHTTPRejectsAnotherSelectedConversation(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	stored, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	holder := memory.LeaseHolderID("evidence-http")
	runtime := agent.New(&fakeClient{}, webTestContextProfile("test"), store.BindHistory(stored.ID, holder), stored.ScopeContext(), store.BindTurnOwner(stored.ID, holder))
	server := NewContextMemoryServer(runtime, nil, nil, nil, store)
	server.activeSession = stored
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/memory/evidence", `{"sessionId":"other-conversation","snapshotId":"request-1"}`))
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "memory_session_changed") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMemoryEvidenceHTTPInspectsOriginalRequestAfterRestart(t *testing.T) {
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
	claimID := seedWebMemoryLiteral(t, store, source, "083", "Detroit")
	coverage, err := store.RefreshMemoryIndex(ctx, 256)
	if err != nil || coverage.State != "active" {
		t.Fatalf("index=%+v: %v", coverage, err)
	}
	reader, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	client := &fakeClient{steps: []fakeStep{
		{toolCalls: []openrouter.ToolCall{{ID: "lookup", Type: "function", Function: openrouter.FunctionCall{Name: "memory_search", Arguments: `{"query":"Detroit"}`}}}},
		{content: "Original evidence received."},
	}}
	runtime := agent.NewWithToolset(client, webTestContextProfile("test"), store.BindHistory(reader.ID, "evidence-reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "evidence-reader"), tools.NewToolset(definitions))
	stream := httptest.NewRecorder()
	events, err := newSSEEvents(stream)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Send(ctx, "Look up my timezone.", events, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stream.Body.String(), "event: memory_activity") || !strings.Contains(stream.Body.String(), `"acceptedCount":1`) {
		t.Fatalf("memory activity missing: %s", stream.Body.String())
	}
	var snapshotID memory.EventID
	history, err := runtime.HistoryEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range history {
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
		t.Fatal("request has no evidence receipt")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	runtime = agent.New(nil, webTestContextProfile("test"), store.BindHistory(reader.ID, "reopened-reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "reopened-reader"))
	server := NewContextMemoryServer(runtime, nil, nil, &fakeContextSessionController{}, store)
	server.activeSession = reader
	body, err := json.Marshal(map[string]string{"sessionId": string(reader.ID), "snapshotId": string(snapshotID)})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/memory/evidence", string(body)))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Detroit") || !strings.Contains(rr.Body.String(), string(claimID)) || !strings.Contains(rr.Body.String(), `"available":true`) {
		t.Fatalf("inspection status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("original evidence response permits stale caching")
	}
	replayed := httptest.NewRecorder()
	server.Handler().ServeHTTP(replayed, managementRequest("/api/context-sessions/history", `{"sessionId":"`+string(reader.ID)+`"}`))
	if replayed.Code != http.StatusOK || !strings.Contains(replayed.Body.String(), `"kind":"memory"`) || !strings.Contains(replayed.Body.String(), string(snapshotID)) {
		t.Fatalf("history status=%d body=%s", replayed.Code, replayed.Body.String())
	}
}
