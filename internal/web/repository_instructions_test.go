package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

type instructionTestController struct {
	fakeContextSessionController
	store *eviedb.Store
}

func (c *instructionTestController) PreviewRepositoryInstructions(ctx context.Context, id memory.WorkspaceID) (memory.RepositoryInstructionSnapshot, error) {
	return c.store.PreviewRepositoryInstructions(ctx, id)
}
func (c *instructionTestController) SetRepositoryInstructionSettings(ctx context.Context, id memory.WorkspaceID, rev int64, on bool) (memory.RepositoryInstructionSettings, error) {
	return c.store.SetRepositoryInstructionSettings(ctx, id, rev, on)
}
func (c *instructionTestController) RepositoryInstructionSnapshot(ctx context.Context, id memory.SessionID, turn memory.EventID) (memory.RepositoryInstructionSnapshot, error) {
	return c.store.RepositoryInstructionSnapshot(ctx, id, turn)
}
func TestRepositoryInstructionHTTPSettingsAndSessionIsolation(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "Instructions")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("instruction data"), 0600)
	if _, err = store.SetWorkspaceFolder(ctx, ws.ID, 0, root); err != nil {
		t.Fatal(err)
	}
	manager, _ := managedBuiltinServer(t)
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
		if err := manager.Enable(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	history := store.BindHistory(session.ID, "test")
	turn, err := history.Append(ctx, lease, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = history.RepositoryInstructions(ctx, lease, turn.ID); err != nil {
		t.Fatal(err)
	}
	server := NewContextServer(nil, nil, nil, &instructionTestController{store: store})
	server.activeSession = session
	handler := server.Handler()
	call := func(action string, body repositoryInstructionRequest) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest("/api/repository-instructions/"+action, string(data)))
		return rec
	}
	req := repositoryInstructionRequest{WorkspaceID: ws.ID, FolderRevision: 1, SessionID: session.ID, TurnID: turn.ID}
	if got := call("preview", req); got.Code != 200 {
		t.Fatalf("preview=%d %s", got.Code, got.Body)
	}
	req.FolderRevision = 0
	if got := call("preview", req); got.Code != 409 {
		t.Fatalf("stale folder=%d", got.Code)
	}
	req.FolderRevision = 1
	if got := call("snapshot", req); got.Code != 200 {
		t.Fatalf("snapshot=%d %s", got.Code, got.Body)
	}
	req.SessionID = "another"
	if got := call("snapshot", req); got.Code != 409 {
		t.Fatalf("foreign session=%d", got.Code)
	}
	req.SessionID = session.ID
	req.TurnID = "another"
	if got := call("snapshot", req); got.Code != 404 {
		t.Fatalf("foreign turn=%d", got.Code)
	}
	if got := call("settings", req); got.Code != 400 {
		t.Fatalf("missing setting=%d", got.Code)
	}
	off := false
	req.Enabled = &off
	server.activeTurns = 1
	if got := call("settings", req); got.Code != 409 {
		t.Fatalf("busy=%d", got.Code)
	}
	server.activeTurns = 0
	if got := call("settings", req); got.Code != 200 {
		t.Fatalf("save=%d %s", got.Code, got.Body)
	}
	if got := call("settings", req); got.Code != 409 {
		t.Fatalf("stale setting=%d", got.Code)
	}
	request := managementRequest("/api/repository-instructions/settings", `{}`)
	request.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request)
	if rec.Code != 403 {
		t.Fatalf("origin=%d", rec.Code)
	}
}
