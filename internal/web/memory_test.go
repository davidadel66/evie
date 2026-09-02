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
	"github.com/davidadel66/evie/internal/tools"
)

func TestSemanticMemoryHTTPUsesExactReadScopePaginationDetailAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claimID := seedWebMemoryLiteral(t, store, storedSession, "080", "Detroit")
	server := NewContextMemoryServer(nil, nil, nil, nil, store)
	server.activeSession = storedSession
	handler := server.Handler()

	scopes := httptest.NewRecorder()
	handler.ServeHTTP(scopes, managementRequest("/api/memory/scopes", `{}`))
	if scopes.Code != http.StatusOK || !strings.Contains(scopes.Body.String(), `"scope_key":"global"`) {
		t.Fatalf("scope status=%d body=%s", scopes.Code, scopes.Body.String())
	}

	before, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, managementRequest("/api/memory/objects", `{"scopeKey":"global","pageSize":1}`))
	var firstPage memory.SemanticObjectPage
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatal(err)
	}
	if first.Code != http.StatusOK || firstPage.Metadata.SelectedScope != "global" || len(firstPage.Metadata.AllowedScopes) != 1 || len(firstPage.Objects) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first page status=%d result=%+v body=%s", first.Code, firstPage, first.Body.String())
	}

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, managementRequest("/api/memory/inspect", `{"scopeKey":"global","kind":"claim","id":"`+string(claimID)+`"}`))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"status":"active"`) ||
		!strings.Contains(detail.Body.String(), `"sources"`) || !strings.Contains(detail.Body.String(), `"operations"`) ||
		!strings.Contains(detail.Body.String(), `"proposal_sha256"`) {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	after, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil || len(after) != len(before) {
		t.Fatalf("HTTP reads changed Episodic events: before=%d after=%d error=%v", len(before), len(after), err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	restarted := NewContextMemoryServer(nil, nil, nil, nil, store)
	restarted.activeSession = storedSession
	cursorBody, err := json.Marshal(map[string]any{"scopeKey": "global", "pageSize": 1, "cursor": firstPage.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	next := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(next, managementRequest("/api/memory/objects", string(cursorBody)))
	if next.Code != http.StatusOK || !strings.Contains(next.Body.String(), `"selected_scope":"global"`) {
		t.Fatalf("restarted page status=%d body=%s", next.Code, next.Body.String())
	}

	_ = seedWebMemoryLiteral(t, store, storedSession, "081", "Chicago")
	stale := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(stale, managementRequest("/api/memory/objects", string(cursorBody)))
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"code":"memory_stale_cursor"`) {
		t.Fatalf("stale page status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestSemanticMemoryHTTPRequiresActiveAndExplicitScopeAndIsReadOnly(t *testing.T) {
	server := NewContextMemoryServer(nil, nil, nil, nil, &memoryHTTPFake{})
	handler := server.Handler()
	missingSession := httptest.NewRecorder()
	handler.ServeHTTP(missingSession, managementRequest("/api/memory/scopes", `{}`))
	if missingSession.Code != http.StatusConflict || !strings.Contains(missingSession.Body.String(), "memory_scope_required") {
		t.Fatalf("missing active scope status=%d body=%s", missingSession.Code, missingSession.Body.String())
	}
	server.activeSession = memory.Session{ID: "session-1", Status: memory.SessionActive}
	missingScope := httptest.NewRecorder()
	handler.ServeHTTP(missingScope, managementRequest("/api/memory/objects", `{}`))
	if missingScope.Code != http.StatusBadRequest || !strings.Contains(missingScope.Body.String(), "invalid_memory_query") {
		t.Fatalf("missing explicit scope status=%d body=%s", missingScope.Code, missingScope.Body.String())
	}
	method := httptest.NewRecorder()
	handler.ServeHTTP(method, httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:6687/api/memory/objects", nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("mutating method status=%d body=%s", method.Code, method.Body.String())
	}
}

func TestSemanticMemoryHTTPSurfacesQuarantineAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = seedWebMemoryLiteral(t, store, storedSession, "082", "Detroit")
	if _, err := db.ExecContext(ctx, `UPDATE semantic_scopes SET revision = 9 WHERE scope_key = 'global'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	assertQuarantined := func() {
		t.Helper()
		reopened, err := eviedb.OpenDBAt(path)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		restarted := NewContextMemoryServer(nil, nil, nil, nil, eviedb.NewStore(reopened))
		restarted.activeSession = storedSession
		handler := restarted.Handler()
		scopes := httptest.NewRecorder()
		handler.ServeHTTP(scopes, managementRequest("/api/memory/scopes", `{}`))
		if scopes.Code != http.StatusOK || !strings.Contains(scopes.Body.String(), `"scope_key":"global"`) ||
			!strings.Contains(scopes.Body.String(), `"quarantined":true`) || !strings.Contains(scopes.Body.String(), `"quarantine_reason"`) {
			t.Fatalf("quarantine scope status=%d body=%s", scopes.Code, scopes.Body.String())
		}
		objects := httptest.NewRecorder()
		handler.ServeHTTP(objects, managementRequest("/api/memory/objects", `{"scopeKey":"global"}`))
		if objects.Code != http.StatusLocked || !strings.Contains(objects.Body.String(), `"code":"memory_scope_quarantined"`) {
			t.Fatalf("quarantine object status=%d body=%s", objects.Code, objects.Body.String())
		}
	}

	assertQuarantined()
	assertQuarantined()
}

func seedWebMemoryLiteral(t *testing.T, store *eviedb.Store, stored memory.Session, suffix, value string) memory.SemanticID {
	t.Helper()
	holder := memory.LeaseHolderID("web-memory-" + suffix)
	session := agent.New(&fakeClient{}, webTestContextProfile("test"), store.BindHistory(stored.ID, holder),
		stored.ScopeContext(), store.BindTurnOwner(stored.ID, holder))
	proposal, err := session.PrepareRememberLiteral(context.Background(), store, "/remember timezone_name "+value, memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:70000000-0000-4000-8000-000000000" + suffix,
		Predicate:      "timezone_name", PredicateLabel: "timezone name", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ResolveRememberLiteral(context.Background(), store, proposal, tools.Approved); err != nil {
		t.Fatal(err)
	}
	return proposal.ClaimID
}

type memoryHTTPFake struct{}

func (*memoryHTTPFake) PrepareCreateGraphLink(context.Context, memory.ScopeContext, memory.CreateGraphLinkRequest) (memory.CreateGraphLinkProposal, error) {
	return memory.CreateGraphLinkProposal{}, nil
}
func (*memoryHTTPFake) ApplyCreateGraphLink(context.Context, memory.TurnLease, memory.CreateGraphLinkProposal) (memory.CreateGraphLinkResult, error) {
	return memory.CreateGraphLinkResult{}, nil
}
func (*memoryHTTPFake) PrepareMemoryLifecycle(context.Context, memory.ScopeContext, memory.MemoryLifecycleRequest) (memory.MemoryLifecycleProposal, error) {
	return memory.MemoryLifecycleProposal{}, nil
}
func (*memoryHTTPFake) ApplyMemoryLifecycle(context.Context, memory.TurnLease, memory.MemoryLifecycleProposal) (memory.MemoryLifecycleResult, error) {
	return memory.MemoryLifecycleResult{}, nil
}
func (*memoryHTTPFake) ListSemanticScopes(context.Context, memory.ScopeContext, memory.SemanticScopeListQuery) (memory.SemanticScopePage, error) {
	return memory.SemanticScopePage{}, nil
}
func (*memoryHTTPFake) ListSemanticObjects(context.Context, memory.ScopeContext, memory.SemanticObjectListQuery) (memory.SemanticObjectPage, error) {
	return memory.SemanticObjectPage{}, nil
}
func (*memoryHTTPFake) InspectSemanticObject(context.Context, memory.ScopeContext, memory.SemanticObjectKind, memory.SemanticID) (memory.SemanticObjectInspection, error) {
	return memory.SemanticObjectInspection{}, nil
}
func (*memoryHTTPFake) InspectSemanticObjectAt(context.Context, memory.ScopeContext, memory.SemanticObjectKind, memory.SemanticID, memory.ClaimQuery) (memory.SemanticObjectInspection, error) {
	return memory.SemanticObjectInspection{}, nil
}
func (*memoryHTTPFake) TraverseSemanticNeighborhood(context.Context, memory.ScopeContext, memory.SemanticTraversalQuery) (memory.SemanticNeighborhood, error) {
	return memory.SemanticNeighborhood{}, nil
}
