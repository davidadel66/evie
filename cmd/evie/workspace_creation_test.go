package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

func TestWorkspaceCreationResearchPresetRestrictsRootToolsAndResumesReceipt(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	manager := sessionCompositionManager(t)
	profile, err := openrouter.NewExplicitContextProfile("test/model", 300000, 200000, 12000)
	if err != nil {
		t.Fatal(err)
	}
	var selected []plugins.ResolvedComposition
	client := &resumeCaptureClient{}
	factory := func(session memory.Session, composition plugins.ResolvedComposition) (*agent.Session, error) {
		selected = append(selected, composition)
		holder := memory.LeaseHolderID("workspace-test-" + session.ID)
		return agent.NewWithToolset(client, profile, store.BindHistory(session.ID, holder), session.ScopeContext(), store.BindTurnOwner(session.ID, holder), composition.Toolset), nil
	}
	controller := newWebContextSessionController(store, manager, factory)
	workspace, err := controller.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Research", PresetID: "research"})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := controller.SelectSession(ctx, web.ContextSessionSelection{WorkspaceID: workspace.ID, WorkspaceRevision: workspace.CurrentRevisionID})
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Agent.Send(ctx, "Find a useful source", &replEvents{out: io.Discard}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || strings.Contains(client.requests[0].Messages[0].Content, "delegated web researcher") || !strings.Contains(client.requests[0].Messages[0].Content, "primary agent") {
		t.Fatalf("root instructions changed to worker role: %+v", client.requests)
	}
	receipt, err := store.GetCompositionReceipt(ctx, opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	controller = newWebContextSessionController(store, resumeOnlyCompositionManager{Manager: manager}, factory)
	resumed, err := controller.SelectSession(ctx, web.ContextSessionSelection{SessionID: opened.Session.ID})
	if err != nil || resumed.Session.ID != opened.Session.ID {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	if len(selected) != 2 || !reflect.DeepEqual(selected[1].Receipt, receipt) || receipt.Preset.ID != "research" {
		t.Fatalf("composition changed: %+v", selected)
	}
	for _, composition := range selected {
		if names := schemaNames(composition.Toolset); !reflect.DeepEqual(names, []string{"web_search", "web_fetch"}) {
			t.Fatalf("research tools = %v", names)
		}
		for _, name := range []string{"bash", "read_file", "query_db", "todo_list", "memory_search", "delegate_research"} {
			call := openrouter.ToolCall{ID: "forged", Type: "function"}
			call.Function.Name, call.Function.Arguments = name, "{}"
			_, rejected, err := composition.Toolset.ExecuteWithApprovalAuthorizedCompletion(ctx, call, nil, nil, nil, nil)
			if err != nil || !rejected {
				t.Fatalf("forged %s allowed: %v", name, err)
			}
		}
	}
}

func TestWorkspaceCreationValidatesPresetBeforeCreatingFolder(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager := sessionCompositionManager(t)
	controller := newWebContextSessionController(store, manager, nil)
	ctx := context.Background()
	for _, preset := range []string{"unknown", "research"} {
		if preset == "research" {
			if err := manager.SetEnabled(plugins.WebPluginID, false); err != nil {
				t.Fatal(err)
			}
		}
		path := filepath.Join(t.TempDir(), "must-not-exist")
		if _, err := controller.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Invalid", PresetID: preset, FolderPath: path, CreateFolder: true}); err == nil {
			t.Fatalf("invalid %s accepted", preset)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid preset created folder: %v", err)
		}
	}
	listed, err := store.ListWorkspaces(ctx, false)
	if err != nil || len(listed) != 0 {
		t.Fatalf("invalid preset created Workspace: %+v, %v", listed, err)
	}
}

func TestWorkspaceCreationREPLUsesWorkspaceDefaultPreset(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	ctx := context.Background()
	manager := sessionCompositionManager(t)
	standard, err := manager.ResolvePresetContext(ctx, plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Research", PresetID: "research"})
	if err != nil {
		t.Fatal(err)
	}
	chooser := &receiptBoundREPLStore{Store: store, composition: standard, resolvePreset: manager.ResolvePresetContext}
	session, err := chooser.CreateWorkspaceSessionForChooser(ctx, workspace.ID, workspace.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	selection := chooser.selection(session)
	receipt, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil || selection.createdComposition == nil || selection.createdComposition.Receipt.Preset.ID != "research" || !reflect.DeepEqual(receipt, selection.createdComposition.Receipt) {
		t.Fatalf("CLI composition = %+v, %v", selection, err)
	}
}

// workspaces.decisions.md, 2026-11-02: a Workspace created in the app is
// Standard with research delegation on, through the real controller and store.
func TestWorkspaceCreationHTTPStoresResearchByDefault(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	controller := newWebContextSessionController(store, sessionCompositionManager(t), nil)
	handler := web.NewContextServer(nil, nil, nil, controller).Handler()
	register := func(body string) memory.Workspace {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687/api/workspaces/register", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("%s = %d %s", body, response.Code, response.Body)
		}
		var created memory.Workspace
		if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		return created
	}
	for body, want := range map[string][]string{
		`{"displayName":"Plain"}`:                               {"standard", "research"},
		`{"displayName":"Off","allowResearchDelegation":false}`: {"standard"},
	} {
		created := register(body)
		stored, err := storedWorkspace(store, created.ID)
		if err != nil || stored.DefaultPresetID != "standard" || !reflect.DeepEqual(stored.AllowedPresetIDs, want) {
			t.Fatalf("%s stored %+v, %v; want allowed %v", body, stored, err, want)
		}
	}
}

// The REPL creates Workspaces as the app does (workspaces.decisions.md,
// 2026-11-02).
func TestWorkspaceCreationREPLAllowsResearch(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager := sessionCompositionManager(t)
	repl := &receiptBoundREPLStore{Store: store, resolvePreset: manager.ResolvePresetContext}
	created, err := repl.RegisterWorkspace(context.Background(), "From the REPL")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := storedWorkspace(store, created.ID)
	if err != nil || stored.DefaultPresetID != "standard" || !reflect.DeepEqual(stored.AllowedPresetIDs, []string{"standard", "research"}) {
		t.Fatalf("REPL Workspace %+v, %v", stored, err)
	}
}

func storedWorkspace(store *eviedb.Store, id memory.WorkspaceID) (memory.Workspace, error) {
	listed, err := store.ListWorkspaces(context.Background(), false)
	for _, workspace := range listed {
		if workspace.ID == id {
			return workspace, err
		}
	}
	return memory.Workspace{}, fmt.Errorf("Workspace %s not listed: %v", id, err)
}
