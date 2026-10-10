package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
)

// stubChangeSync records each refresh and answers it as told.
type stubChangeSync struct {
	refreshed []string
	err       error
}

func (s *stubChangeSync) Refresh(_ context.Context, ws memory.WorkspaceID, folder string) (changes.SyncResult, error) {
	s.refreshed = append(s.refreshed, string(ws)+" "+folder)
	if s.err != nil {
		return changes.SyncResult{}, s.err
	}
	return changes.SyncResult{Synced: true, PullRequests: 2, Finished: 1, Requests: 6}, nil
}

// changes.spec.md story 3b: opening the Changes tab asks for a sync now,
// of the Workspace's own folder, and each refusal gets its own status.
func TestChangesRefreshSyncsTheWorkspaceFolderNow(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "evie")
	if err != nil {
		t.Fatal(err)
	}
	folder, err := store.SetWorkspaceFolder(ctx, workspace.ID, 0, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubChangeSync{}
	handler := WithChangeSync(NewContextServer(nil, nil, nil, &folderTestController{store: store}), stub).Handler()
	call := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest("/api/changes/refresh", body))
		return rec
	}
	refresh := `{"workspace_id":"` + string(workspace.ID) + `"}`

	res := call(refresh)
	var result changes.SyncResult
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &result) != nil || result != (changes.SyncResult{Synced: true, PullRequests: 2, Finished: 1, Requests: 6}) {
		t.Fatalf("refresh = %d %s", res.Code, res.Body)
	}
	if len(stub.refreshed) != 1 || stub.refreshed[0] != string(workspace.ID)+" "+folder.Path {
		t.Fatalf("refreshed %v, want the Workspace's folder %s", stub.refreshed, folder.Path)
	}
	for err, want := range map[error]int{
		changes.ErrNoToken:     http.StatusConflict,
		github.ErrUnauthorized: http.StatusConflict,
		fmt.Errorf("list the open pull requests of davidadel66/evie: %w", github.ErrNotFound):                                           http.StatusConflict,
		fmt.Errorf("read the check runs of pull request #7: %w: Resource not accessible by personal access token", github.ErrForbidden): http.StatusUnprocessableEntity,
		errors.New("github: cannot reach api.github.com"):                                                                               http.StatusBadGateway,
	} {
		stub.err = err
		if res := call(refresh); res.Code != want || !strings.Contains(res.Body.String(), err.Error()) {
			t.Fatalf("%v = %d %s, want %d", err, res.Code, res.Body, want)
		}
	}
	stub.refreshed = nil
	if res := call(`{"workspace_id":"missing"}`); res.Code != 404 {
		t.Fatalf("unknown Workspace = %d %s", res.Code, res.Body)
	}
	// The folder is the Workspace's, never one from the browser.
	for _, body := range []string{`{}`, `{"workspace_id":"x","folder":"/etc"}`, `not json`} {
		if res := call(body); res.Code != 400 {
			t.Fatalf("%s = %d %s", body, res.Code, res.Body)
		}
	}
	if len(stub.refreshed) != 0 {
		t.Fatalf("refused requests synced %v", stub.refreshed)
	}
	get := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687/api/changes/refresh", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, get)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", rec.Code)
	}
}

// A real Syncer behind the route: a local Repository has nothing to sync.
func TestChangesRefreshOfALocalRepositorySyncsNothing(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	syncer := changes.NewSyncer(changes.SyncConfig{Store: store})
	handler := WithChangeSync(NewContextServer(nil, nil, nil, &folderTestController{store: store}), syncer).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, managementRequest("/api/changes/refresh", `{"workspace_id":"`+string(workspace.ID)+`"}`))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"synced":false`) {
		t.Fatalf("refresh = %d %s", rec.Code, rec.Body)
	}
}
