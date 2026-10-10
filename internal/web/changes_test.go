package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// changes.spec.md story 3a: the Changes tab lists a Workspace's Changes
// through one owner-only route, read from its current folder; and the
// folder cannot be changed while one of them is unfinished.
func TestChangesRouteListsBuildChangesAndTheFolderStaysWhileOneIsUnfinished(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Code")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "first"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if _, err := store.SetWorkspaceFolder(ctx, workspace.ID, 0, repo); err != nil {
		t.Fatal(err)
	}
	// A build Evie delegated from a chat in this Workspace, as the harness
	// records it.
	parent, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, now := uuid.NewString(), time.Now().UTC()
	attempt := delegation.Attempt{ID: id, State: "succeeded", CreatedAt: now, EndedAt: &now, Child: child,
		Parent:     delegation.Parent{Scope: memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: parent.ID, WorkspaceID: workspace.ID}},
		Assignment: delegation.Assignment{Key: "key", Objective: "Add the login page"},
		Worktree:   &delegation.Worktree{Repo: repo, Path: filepath.Join(repo, ".evie", "worktrees", id), Branch: "evie/" + id[:8] + "-login", Base: "main"}}
	record, _ := json.Marshal(attempt)
	if _, err := db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`,
		id, parent.ID, "key", "digest", child.ID, attempt.State, string(record)); err != nil {
		t.Fatal(err)
	}

	lister := &changes.Lister{Repositories: changes.NewService(changes.Config{}), Builds: changes.NewBuildSource(store)}
	handler := WithChanges(NewContextServer(nil, nil, nil, &folderTestController{store: store}), lister).Handler()
	call := func(route, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest(route, body))
		return rec
	}
	res := call("/api/changes/list", `{"workspace_id":"`+string(workspace.ID)+`"}`)
	var listed struct {
		Repository changes.Repository `json:"repository"`
		Changes    []changes.Change   `json:"changes"`
	}
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &listed) != nil {
		t.Fatalf("list = %d %s", res.Code, res.Body)
	}
	if listed.Repository.Kind != changes.Local || len(listed.Changes) != 1 {
		t.Fatalf("listed = %+v", listed)
	}
	if c := listed.Changes[0]; c.ExecutionID != id || c.ID != "build:"+attempt.Worktree.Branch || c.State != changes.Draft || c.Title != "Add the login page" || c.Branch != attempt.Worktree.Branch ||
		c.Origin == nil || c.Origin.SessionID != string(parent.ID) {
		t.Fatalf("Change = %+v", c)
	}
	if res := call("/api/changes/list", `{"workspace_id":"missing"}`); res.Code != 404 {
		t.Fatalf("unknown Workspace = %d %s", res.Code, res.Body)
	}
	for _, body := range []string{`{}`, `{"workspace_id":"` + string(workspace.ID) + `","folder":"/etc"}`} {
		if res := call("/api/changes/list", body); res.Code != 400 {
			t.Fatalf("%s = %d %s", body, res.Code, res.Body)
		}
	}

	// The draft's worktree lives in the folder: changing or detaching it
	// is refused with a reason, until the draft is merged or closed.
	for _, path := range []string{t.TempDir(), ""} {
		body, _ := json.Marshal(folderRequest{WorkspaceID: workspace.ID, Revision: 1, Path: path})
		res := call("/api/workspaces/folder", string(body))
		if res.Code != 409 || !strings.Contains(res.Body.String(), "unfinished Changes") {
			t.Fatalf("folder %q = %d %s", path, res.Code, res.Body)
		}
	}
	if err := store.MarkBuildWorktreeRemoved(ctx, id); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(folderRequest{WorkspaceID: workspace.ID, Revision: 1, Path: ""})
	if res := call("/api/workspaces/folder", string(body)); res.Code != 200 {
		t.Fatalf("detach after close = %d %s", res.Code, res.Body)
	}
}
