package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type folderTestController struct {
	fakeContextSessionController
	store *eviedb.Store
}

func (c *folderTestController) WorkspaceFolder(ctx context.Context, id memory.WorkspaceID) (memory.WorkspaceFolder, error) {
	return c.store.WorkspaceFolder(ctx, id)
}
func (c *folderTestController) SetWorkspaceFolder(ctx context.Context, id memory.WorkspaceID, revision int64, path string) (memory.WorkspaceFolder, error) {
	return c.store.SetWorkspaceFolder(ctx, id, revision, path)
}
func TestWorkspaceFolderHTTPReadsCurrentFolderAndRejectsStaleOrBusyChanges(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Learning")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# Notes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	controller := &folderTestController{store: store}
	server := NewContextServer(nil, nil, nil, controller)
	handler := server.Handler()
	call := func(route string, body any) *httptest.ResponseRecorder {
		t.Helper()
		encoded, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest(route, string(encoded)))
		return rec
	}
	req := folderRequest{WorkspaceID: workspace.ID, Path: root}
	for _, route := range []string{"/api/workspaces/folder", "/api/folder/read"} {
		if res := call(route, folderRequest{}); res.Code != 400 {
			t.Fatalf("missing Workspace at %s=%d", route, res.Code)
		}
	}
	if res := call("/api/workspaces/folder", req); res.Code != 200 {
		t.Fatalf("attach=%d %s", res.Code, res.Body)
	}
	req.Revision = 1
	req.Path = "notes.md"
	if res := call("/api/folder/read", req); res.Code != 200 || !strings.Contains(res.Body.String(), "# Notes") {
		t.Fatalf("read=%d %s", res.Code, res.Body)
	}
	req.Path = "../evie.db"
	if res := call("/api/folder/read", req); res.Code != 422 {
		t.Fatalf("traversal=%d", res.Code)
	}
	req.Path = "notes.md"
	req.Revision = 0
	if res := call("/api/folder/read", req); res.Code != 409 {
		t.Fatalf("stale read=%d", res.Code)
	}
	server.activeTurns = 1
	req.Revision = 1
	req.Path = ""
	if res := call("/api/workspaces/folder", req); res.Code != 409 {
		t.Fatalf("busy edit=%d", res.Code)
	}
	server.activeTurns = 0
	if res := call("/api/workspaces/folder", req); res.Code != 200 {
		t.Fatalf("detach=%d %s", res.Code, res.Body)
	}
	req.Path = "notes.md"
	if res := call("/api/folder/read", req); res.Code != 409 {
		t.Fatalf("old folder request=%d", res.Code)
	}
	request := managementRequest("/api/workspaces/folder", `{}`)
	request.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request)
	if rec.Code != 403 {
		t.Fatalf("origin=%d", rec.Code)
	}
}
