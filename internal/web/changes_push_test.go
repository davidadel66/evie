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

// stubChangePusher records each push and answers it as told.
type stubChangePusher struct {
	pushed []string
	err    error
}

func (s *stubChangePusher) Push(_ context.Context, ws memory.WorkspaceID, folder, branch string) (changes.PushResult, error) {
	s.pushed = append(s.pushed, string(ws)+" "+folder+" "+branch)
	if s.err != nil {
		return changes.PushResult{}, s.err
	}
	return changes.PushResult{Change: changes.Change{Branch: branch, State: changes.Open}, PullRequest: 4, URL: "https://github.com/davidadel66/notes/pull/4",
		Verify: &changes.VerifyOutcome{Command: "go test ./...", Passed: true}}, nil
}

// changes.spec.md story 4a: the owner pushes one of Evie's draft Changes
// from the Workspace's own folder, and each refusal gets its own status.
func TestChangesPushSendsTheWorkspaceBranchAndAnswersItsPullRequest(t *testing.T) {
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
	stub := &stubChangePusher{}
	handler := WithChangePusher(NewContextServer(nil, nil, nil, &folderTestController{store: store}), stub).Handler()
	call := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest("/api/changes/push", body))
		return rec
	}
	push := `{"workspace_id":"` + string(workspace.ID) + `","branch":"evie/1a2b3c4d-fix-footer"}`

	res := call(push)
	var result changes.PushResult
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &result) != nil || result.PullRequest != 4 || result.URL != "https://github.com/davidadel66/notes/pull/4" ||
		result.Verify == nil || !result.Verify.Passed {
		t.Fatalf("push = %d %s", res.Code, res.Body)
	}
	if len(stub.pushed) != 1 || stub.pushed[0] != string(workspace.ID)+" "+folder.Path+" evie/1a2b3c4d-fix-footer" {
		t.Fatalf("pushed %v, want the Workspace's folder %s", stub.pushed, folder.Path)
	}
	for err, want := range map[error]int{
		fmt.Errorf("%w: evie/1a2b3c4d-fix-footer is already merged", changes.ErrNotPushable): http.StatusConflict,
		changes.ErrNoToken:     http.StatusConflict,
		github.ErrUnauthorized: http.StatusConflict,
		changes.ErrBusy:        http.StatusConflict,
		fmt.Errorf("pushed evie/1a2b3c4d-fix-footer, but opening its pull request failed: %w: A pull request already exists", github.ErrUnprocessable): http.StatusUnprocessableEntity,
		fmt.Errorf("%w: pushing evie/1a2b3c4d-fix-footer failed: rejected", changes.ErrPushFailed):                                                     http.StatusBadGateway,
		errors.New("run Verify on evie/1a2b3c4d-fix-footer: the sandbox is unavailable"):                                                               http.StatusBadGateway,
	} {
		stub.err = err
		if res := call(push); res.Code != want || !strings.Contains(res.Body.String(), err.Error()) {
			t.Fatalf("%v = %d %s, want %d", err, res.Code, res.Body, want)
		}
	}
	stub.pushed = nil
	if res := call(`{"workspace_id":"missing","branch":"evie/1a2b3c4d-fix-footer"}`); res.Code != 404 {
		t.Fatalf("unknown Workspace = %d %s", res.Code, res.Body)
	}
	// The folder is the Workspace's, never one from the browser.
	for _, body := range []string{`{}`, `{"workspace_id":"` + string(workspace.ID) + `"}`, `{"branch":"evie/x"}`,
		`{"workspace_id":"x","branch":"evie/x","folder":"/etc"}`, `{"workspace_id":"x","branch":"` + strings.Repeat("b", 300) + `"}`, `not json`} {
		if res := call(body); res.Code != 400 {
			t.Fatalf("%s = %d %s", body, res.Code, res.Body)
		}
	}
	if len(stub.pushed) != 0 {
		t.Fatalf("refused requests pushed %v", stub.pushed)
	}
	get := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687/api/changes/push", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, get)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", rec.Code)
	}
}
