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
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// stubChangeMerger records each merge and close and answers as told.
type stubChangeMerger struct {
	calls []string
	err   error
}

func (s *stubChangeMerger) Merge(_ context.Context, ws memory.WorkspaceID, folder, id string) (changes.MergeResult, error) {
	s.calls = append(s.calls, "merge "+string(ws)+" "+folder+" "+id)
	if s.err != nil {
		return changes.MergeResult{Status: delegation.MergeVerifyFailed, Verification: []delegation.GateRun{{Command: "go test ./...", Output: "--- FAIL: TestFix"}}}, s.err
	}
	return changes.MergeResult{Change: changes.Change{ID: id, State: changes.Merged}, Status: delegation.MergeMerged}, nil
}

func (s *stubChangeMerger) Close(_ context.Context, ws memory.WorkspaceID, folder, id string) (changes.CloseResult, error) {
	s.calls = append(s.calls, "close "+string(ws)+" "+folder+" "+id)
	if s.err != nil {
		return changes.CloseResult{}, s.err
	}
	return changes.CloseResult{Change: changes.Change{ID: id, State: changes.Closed}}, nil
}

// changes.spec.md story 5a: the owner merges and closes a Workspace's
// Changes by ID in its own folder; close needs confirm, and a refused merge
// answers with the gate's runs.
func TestChangesMergeAndCloseRoutes(t *testing.T) {
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
	stub := &stubChangeMerger{}
	handler := WithChangeMerger(NewContextServer(nil, nil, nil, &folderTestController{store: store}), stub).Handler()
	call := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest(path, body))
		return rec
	}
	ws := `"workspace_id":"` + string(workspace.ID) + `"`
	merge := `{` + ws + `,"change_id":"build:evie/1a2b3c4d-fix"}`

	res := call("/api/changes/merge", merge)
	var result changes.MergeResult
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &result) != nil || result.Status != delegation.MergeMerged || result.Change.State != changes.Merged {
		t.Fatalf("merge = %d %s", res.Code, res.Body)
	}
	if res := call("/api/changes/close", `{`+ws+`,"change_id":"build:evie/1a2b3c4d-fix"}`); res.Code != 400 || !strings.Contains(res.Body.String(), "confirm") {
		t.Fatalf("close without confirm = %d %s", res.Code, res.Body)
	}
	if res := call("/api/changes/close", `{`+ws+`,"change_id":"build:evie/1a2b3c4d-fix","confirm":true}`); res.Code != 200 || !strings.Contains(res.Body.String(), `"state":"closed"`) {
		t.Fatalf("close = %d %s", res.Code, res.Body)
	}
	want := []string{"merge " + string(workspace.ID) + " " + folder.Path + " build:evie/1a2b3c4d-fix", "close " + string(workspace.ID) + " " + folder.Path + " build:evie/1a2b3c4d-fix"}
	if fmt.Sprint(stub.calls) != fmt.Sprint(want) {
		t.Fatalf("calls = %v", stub.calls)
	}

	stub.err = fmt.Errorf("%w: The verify command failed on the branch.", changes.ErrNotMerged)
	res = call("/api/changes/merge", merge)
	var refused struct {
		Code   string              `json:"code"`
		Error  string              `json:"error"`
		Result changes.MergeResult `json:"result"`
	}
	if res.Code != http.StatusConflict || json.Unmarshal(res.Body.Bytes(), &refused) != nil || refused.Code != "not_merged" ||
		!strings.Contains(refused.Error, "verify command failed") || len(refused.Result.Verification) != 1 || refused.Result.Verification[0].Output != "--- FAIL: TestFix" {
		t.Fatalf("refused merge = %d %s", res.Code, res.Body)
	}
	// A refusal before the gate ran has no runs to show.
	refusing := &stubRefusal{}
	refusalHandler := WithChangeMerger(NewContextServer(nil, nil, nil, &folderTestController{store: store}), refusing).Handler()
	rec := httptest.NewRecorder()
	refusalHandler.ServeHTTP(rec, managementRequest("/api/changes/merge", merge))
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), `"result"`) {
		t.Fatalf("refusal before the gate = %d %s", rec.Code, rec.Body)
	}
	for err, want := range map[error]int{changes.ErrBusy: http.StatusConflict, errors.New("git failed"): http.StatusInternalServerError} {
		stub.err = err
		if res := call("/api/changes/merge", merge); res.Code != want {
			t.Fatalf("%v = %d %s", err, res.Code, res.Body)
		}
	}
	stub.calls = nil
	if res := call("/api/changes/merge", `{"workspace_id":"missing","change_id":"pr:3"}`); res.Code != 404 {
		t.Fatalf("unknown Workspace = %d %s", res.Code, res.Body)
	}
	for _, body := range []string{`{}`, `{` + ws + `}`, `{"change_id":"pr:3"}`, `{` + ws + `,"change_id":"pr:3","folder":"/etc"}`, `not json`} {
		if res := call("/api/changes/merge", body); res.Code != 400 {
			t.Fatalf("%s = %d %s", body, res.Code, res.Body)
		}
	}
	if len(stub.calls) != 0 {
		t.Fatalf("refused requests reached the merger: %v", stub.calls)
	}
}

// stubRefusal refuses every merge before the gate runs.
type stubRefusal struct{ stubChangeMerger }

func (stubRefusal) Merge(context.Context, memory.WorkspaceID, string, string) (changes.MergeResult, error) {
	return changes.MergeResult{}, fmt.Errorf("%w: build:evie/x is already closed", changes.ErrNotMerged)
}
