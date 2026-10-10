package changes_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
)

// changes.spec.md story 3b: while evie serve runs, each Workspace with a
// GitHub Repository is synced every 60 seconds, one sync at a time per
// Workspace: its open pull requests with their checks, and the merges and
// closes done on GitHub.

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	shaC = "cccccccccccccccccccccccccccccccccccccccc"
)

// fakePulls is GitHub for davidadel66/evie: the open pull requests, the
// ones merged or closed since, and the checks on each commit. It counts
// every request it answers.
type fakePulls struct {
	t        *testing.T
	mu       sync.Mutex
	open     map[int]map[string]any
	finished map[int]map[string]any
	runs     map[string][]map[string]any
	statuses map[string][]map[string]any
	requests []string
	// listed hears of each list of open pull requests; hold, when set,
	// keeps the list waiting until it is closed.
	listed       chan struct{}
	hold         chan struct{}
	refuseChecks bool
}

func newFakePulls(t *testing.T) (*fakePulls, changes.PullRequestClient) {
	f := &fakePulls{t: t, open: map[int]map[string]any{}, finished: map[int]map[string]any{}, runs: map[string][]map[string]any{},
		statuses: map[string][]map[string]any{}, listed: make(chan struct{}, 16)}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, github.New(server.URL, token, server.Client())
}

func pull(number int, branch, sha string, draft bool, updated time.Time) map[string]any {
	return map[string]any{"number": number, "state": "open", "draft": draft, "title": "Pull " + strconv.Itoa(number), "body": "Notes for " + strconv.Itoa(number),
		"html_url": "https://github.com/davidadel66/evie/pull/" + strconv.Itoa(number), "updated_at": updated.Format(time.RFC3339),
		"head": map[string]any{"ref": branch, "sha": sha}, "base": map[string]any{"ref": "master"}, "user": map[string]any{"login": "davidadel66"}}
}

// finish moves an open pull request to merged or closed on GitHub.
func (f *fakePulls) finish(number int, merged bool, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.open[number]
	delete(f.open, number)
	p["state"], p["merged"], p["mergeable"], p["updated_at"] = "closed", merged, nil, at.Format(time.RFC3339)
	if merged {
		p["merged_at"] = at.Format(time.RFC3339)
	}
	f.finished[number] = p
}

func (f *fakePulls) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakePulls) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakePulls) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
}

func (f *fakePulls) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.Path)
	hold := f.hold
	f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	const repo = "/repos/davidadel66/evie"
	path := r.URL.Path
	if path == repo+"/pulls" {
		f.listed <- struct{}{}
		if hold != nil {
			<-hold
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case path == repo+"/pulls" && r.URL.RawQuery == "state=open&per_page=100":
		numbers := []int{}
		for n := range f.open {
			numbers = append(numbers, n)
		}
		sort.Ints(numbers)
		list := []map[string]any{}
		for _, n := range numbers {
			list = append(list, f.open[n])
		}
		json.NewEncoder(w).Encode(list)
	case strings.HasPrefix(path, repo+"/pulls/"):
		n, _ := strconv.Atoi(strings.TrimPrefix(path, repo+"/pulls/"))
		if p, ok := f.finished[n]; ok {
			json.NewEncoder(w).Encode(p)
		} else if p, ok := f.open[n]; ok {
			json.NewEncoder(w).Encode(p)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	case path == repo+"/actions/runs":
		if f.refuseChecks {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"Resource not accessible by personal access token"}`))
			return
		}
		sha := r.URL.Query().Get("head_sha")
		json.NewEncoder(w).Encode(map[string]any{"total_count": len(f.runs[sha]), "workflow_runs": f.runs[sha]})
	case strings.HasPrefix(path, repo+"/commits/") && strings.HasSuffix(path, "/status"):
		sha := strings.TrimSuffix(strings.TrimPrefix(path, repo+"/commits/"), "/status")
		json.NewEncoder(w).Encode(map[string]any{"state": "pending", "statuses": f.statuses[sha]})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// runIDs numbers workflow runs in the order a test makes them, as GitHub
// does: a later run of a workflow has a larger ID.
var runIDs int

func run(name, status, conclusion string) map[string]any {
	runIDs++
	return map[string]any{"id": runIDs, "name": name, "status": status, "conclusion": conclusion,
		"html_url": fmt.Sprintf("https://github.com/davidadel66/evie/actions/runs/%d", runIDs)}
}

func status(name, state string) map[string]any {
	return map[string]any{"context": name, "state": state, "target_url": "https://ci.example.com/" + name}
}

func syncStore(t *testing.T) (*eviedb.Store, memory.Workspace) {
	t.Helper()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	ws, err := store.RegisterWorkspace(context.Background(), "evie")
	if err != nil {
		t.Fatal(err)
	}
	return store, ws
}

var evieRepo = changes.Repository{Kind: changes.GitHub, Owner: "davidadel66", Name: "evie", BaseBranch: "master"}

// pullChanges is what the owner sees: the Workspace's pull requests as
// Changes, by ID.
func pullChanges(t *testing.T, store *eviedb.Store, ws memory.WorkspaceID) map[string]changes.Change {
	t.Helper()
	list, err := changes.PullRequestSource{Store: store}.Changes(context.Background(), ws, evieRepo)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]changes.Change{}
	for _, c := range list {
		byID[c.ID] = c
	}
	return byID
}

func TestSyncRecordsOpenPullRequestsWithTheirChecks(t *testing.T) {
	ctx := context.Background()
	fake, client := newFakePulls(t)
	at := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	fake.open[7] = pull(7, "feature/sync", shaA, true, at)
	fake.open[8] = pull(8, "evie/1234abcd-x", shaB, false, at.Add(-time.Hour))
	fake.runs[shaA] = []map[string]any{
		run("queued", "queued", ""), run("running", "in_progress", ""), run("ok", "completed", "success"), run("broke", "completed", "failure"),
		run("slow", "completed", "timed_out"), run("stopped", "completed", "cancelled"), run("asks", "completed", "action_required"),
		run("meh", "completed", "neutral"), run("skipped", "completed", "skipped"), run("waits", "waiting", ""), run("boot", "completed", "startup_failure"),
		// A workflow run again: its newest run is its check.
		run("again", "completed", "failure"), run("again", "completed", "success"),
	}
	fake.statuses[shaA] = []map[string]any{status("ci/wait", "pending"), status("ci/ok", "success"), status("ci/no", "failure"), status("ci/err", "error")}
	store, ws := syncStore(t)
	syncer := changes.NewSyncer(changes.SyncConfig{GitHub: client, Store: store})

	result, err := syncer.Sync(ctx, ws.ID, evieRepo)
	if err != nil {
		t.Fatal(err)
	}
	// One list, then check runs and statuses for each open pull request.
	if want := (changes.SyncResult{Synced: true, PullRequests: 2, Requests: 5}); result != want || fake.count() != 5 {
		t.Fatalf("result = %+v, want %+v; GitHub answered %d requests", result, want, fake.count())
	}
	got := pullChanges(t, store, ws.ID)
	seven := got["pr:7"]
	if seven.State != changes.Open || seven.Author != changes.ByOwner || seven.Branch != "feature/sync" || seven.Base != "master" || seven.Head != shaA ||
		seven.Title != "Pull 7" || seven.Notes != "Notes for 7" || seven.PullRequest == nil || !seven.PullRequest.Draft || seven.PullRequest.Number != 7 ||
		seven.PullRequest.URL != "https://github.com/davidadel66/evie/pull/7" || !seven.UpdatedAt.Equal(at) {
		t.Fatalf("pr:7 = %+v %+v", seven, seven.PullRequest)
	}
	states := map[string]changes.CheckState{}
	for _, check := range seven.Checks {
		states[check.Name] = check.State
	}
	want := map[string]changes.CheckState{
		"queued": changes.CheckPending, "running": changes.CheckRunning, "ok": changes.CheckPassed, "broke": changes.CheckFailed,
		"slow": changes.CheckFailed, "stopped": changes.CheckFailed, "asks": changes.CheckFailed, "meh": changes.CheckNeutral, "skipped": changes.CheckNeutral,
		"waits": changes.CheckPending, "boot": changes.CheckFailed, "again": changes.CheckPassed,
		"ci/wait": changes.CheckPending, "ci/ok": changes.CheckPassed, "ci/no": changes.CheckFailed, "ci/err": changes.CheckFailed,
	}
	if len(seven.Checks) != len(want) || !equalStates(states, want) {
		t.Fatalf("checks = %+v", seven.Checks)
	}
	links := map[string]string{}
	for _, check := range seven.Checks {
		links[check.Name] = check.URL
	}
	if !strings.HasPrefix(links["queued"], "https://github.com/davidadel66/evie/actions/runs/") || links["ci/wait"] != "https://ci.example.com/ci/wait" ||
		links["again"] != fmt.Sprintf("https://github.com/davidadel66/evie/actions/runs/%d", runIDs) {
		t.Fatalf("check links = %+v", seven.Checks)
	}
	if eight := got["pr:8"]; eight.State != changes.Open || eight.Checks != nil || eight.PullRequest.Draft || eight.Branch != "evie/1234abcd-x" {
		t.Fatalf("pr:8 = %+v", eight)
	}
	tokenAbsent(t, store, ws.ID)
}

// tokenAbsent fails if the token reached what sync stored.
func tokenAbsent(t *testing.T, store *eviedb.Store, ws memory.WorkspaceID) {
	t.Helper()
	records, err := store.ChangePullRequests(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(records)
	if len(records) == 0 || strings.Contains(string(encoded), token) {
		t.Fatalf("stored %s", encoded)
	}
}

func equalStates(a, b map[string]changes.CheckState) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestSyncRecordsMergesAndClosesDoneOnGitHub(t *testing.T) {
	ctx := context.Background()
	fake, client := newFakePulls(t)
	at := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	fake.open[1] = pull(1, "one", shaA, false, at)
	fake.open[2] = pull(2, "two", shaB, false, at)
	fake.open[3] = pull(3, "three", shaC, false, at)
	store, ws := syncStore(t)
	syncer := changes.NewSyncer(changes.SyncConfig{GitHub: client, Store: store})
	if result, err := syncer.Sync(ctx, ws.ID, evieRepo); err != nil || result.PullRequests != 3 || result.Requests != 7 {
		t.Fatalf("first sync = %+v, %v", result, err)
	}

	fake.finish(1, true, at.Add(time.Minute))
	fake.finish(2, false, at.Add(2*time.Minute))
	fake.reset()
	result, err := syncer.Sync(ctx, ws.ID, evieRepo)
	// One list, two for the open #3, one read each of #1 and #2.
	if err != nil || result != (changes.SyncResult{Synced: true, PullRequests: 1, Finished: 2, Requests: 5}) || fake.count() != 5 {
		t.Fatalf("second sync = %+v, %v; GitHub answered %d", result, err, fake.count())
	}
	got := pullChanges(t, store, ws.ID)
	if got["pr:1"].State != changes.Merged || got["pr:1"].PullRequest.State != "merged" || !got["pr:1"].UpdatedAt.Equal(at.Add(time.Minute)) {
		t.Fatalf("pr:1 = %+v", got["pr:1"])
	}
	if got["pr:2"].State != changes.Closed || got["pr:2"].PullRequest.State != "closed" {
		t.Fatalf("pr:2 = %+v", got["pr:2"])
	}
	if got["pr:3"].State != changes.Open {
		t.Fatalf("pr:3 = %+v", got["pr:3"])
	}

	// A finished pull request is not read again.
	fake.reset()
	if result, err := syncer.Sync(ctx, ws.ID, evieRepo); err != nil || result.Requests != 3 || result.Finished != 0 || fake.count() != 3 {
		t.Fatalf("third sync = %+v, %v; GitHub answered %d", result, err, fake.count())
	}
}

func TestOnlyAGitHubRepositorySyncs(t *testing.T) {
	ctx := context.Background()
	fake, client := newFakePulls(t)
	fake.open[7] = pull(7, "feature", shaA, false, time.Now())
	store, ws := syncStore(t)
	syncer := changes.NewSyncer(changes.SyncConfig{GitHub: client, Store: store})
	for _, repo := range []changes.Repository{{Kind: changes.Local, BaseBranch: "main"}, {Kind: changes.None}} {
		if result, err := syncer.Sync(ctx, ws.ID, repo); err != nil || result != (changes.SyncResult{}) {
			t.Fatalf("%s = %+v, %v", repo.Kind, result, err)
		}
	}
	local := repository(t, "main")
	if result, err := syncer.Refresh(ctx, ws.ID, local); err != nil || result.Synced {
		t.Fatalf("refresh of a local folder = %+v, %v", result, err)
	}
	if result, err := syncer.Refresh(ctx, ws.ID, ""); err != nil || result.Synced {
		t.Fatalf("refresh without a folder = %+v, %v", result, err)
	}
	if fake.count() != 0 {
		t.Fatalf("a Repository not on GitHub reached GitHub: %v", fake.paths())
	}
	// A GitHub folder is read from git alone; only the sync reaches GitHub.
	clone := repository(t, "master")
	gitIn(t, clone, "remote", "add", "origin", "git@github.com:davidadel66/evie.git")
	if result, err := syncer.Refresh(ctx, ws.ID, clone); err != nil || !result.Synced || result.PullRequests != 1 || fake.count() != 3 {
		t.Fatalf("refresh of a GitHub folder = %+v, %v; requests %v", result, err, fake.paths())
	}
	// Without a token, a GitHub Repository cannot sync; a local one still
	// has nothing to sync.
	tokenless := changes.NewSyncer(changes.SyncConfig{Store: store})
	if _, err := tokenless.Sync(ctx, ws.ID, evieRepo); !errors.Is(err, changes.ErrNoToken) {
		t.Fatalf("without a token = %v", err)
	}
	if result, err := tokenless.Refresh(ctx, ws.ID, local); err != nil || result.Synced {
		t.Fatalf("local without a token = %+v, %v", result, err)
	}
}

func TestASyncThatGitHubRefusesRecordsNothing(t *testing.T) {
	ctx := context.Background()
	fake, client := newFakePulls(t)
	fake.open[7] = pull(7, "feature", shaA, false, time.Now())
	fake.refuseChecks = true
	store, ws := syncStore(t)
	_, err := changes.NewSyncer(changes.SyncConfig{GitHub: client, Store: store}).Sync(ctx, ws.ID, evieRepo)
	// It names the permission the token lacks.
	if !errors.Is(err, github.ErrForbidden) || !strings.Contains(err.Error(), "#7") || !strings.Contains(err.Error(), "not accessible") ||
		!strings.Contains(err.Error(), "Actions permission") {
		t.Fatalf("refused checks = %v", err)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("the token reached the error: %v", err)
	}
	if got := pullChanges(t, store, ws.ID); len(got) != 0 {
		t.Fatalf("a refused sync recorded %+v", got)
	}
}

func TestOneSyncAtATimePerWorkspace(t *testing.T) {
	ctx := context.Background()
	fake, client := newFakePulls(t)
	fake.open[7] = pull(7, "feature", shaA, false, time.Now())
	fake.hold = make(chan struct{})
	store, ws := syncStore(t)
	syncer := changes.NewSyncer(changes.SyncConfig{GitHub: client, Store: store})
	first := make(chan changes.SyncResult, 1)
	go func() {
		result, err := syncer.Sync(ctx, ws.ID, evieRepo)
		if err != nil {
			t.Errorf("first sync: %v", err)
		}
		first <- result
	}()
	waitFor(t, fake.listed, "the first sync to list pull requests")

	second := make(chan changes.SyncResult, 1)
	go func() {
		result, err := syncer.Sync(ctx, ws.ID, evieRepo)
		if err != nil {
			t.Errorf("second sync: %v", err)
		}
		second <- result
	}()
	select {
	case result := <-second:
		if result != (changes.SyncResult{AlreadyRunning: true}) {
			t.Fatalf("second sync = %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a second sync waited for the first")
	}
	close(fake.hold)
	if result := <-first; !result.Synced || result.PullRequests != 1 {
		t.Fatalf("first sync = %+v", result)
	}
	if fake.count() != 3 {
		t.Fatalf("GitHub answered %v", fake.paths())
	}
	// Once it has finished, the Workspace syncs again.
	if result, err := syncer.Sync(ctx, ws.ID, evieRepo); err != nil || !result.Synced {
		t.Fatalf("next sync = %+v, %v", result, err)
	}
}

func waitFor(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestRunSyncsEachGitHubWorkspaceOnEveryTick(t *testing.T) {
	fake, client := newFakePulls(t)
	fake.open[7] = pull(7, "feature", shaA, false, time.Now())
	store, ws := syncStore(t)
	ctx := context.Background()
	clone := repository(t, "master")
	gitIn(t, clone, "remote", "add", "origin", "https://github.com/davidadel66/evie.git")
	if _, err := store.SetWorkspaceFolder(ctx, ws.ID, 0, clone); err != nil {
		t.Fatal(err)
	}
	// A local Repository, a Workspace without a folder, and a GitHub
	// Repository the token cannot see, which does not stop the others.
	local, _ := store.RegisterWorkspace(ctx, "local")
	if _, err := store.SetWorkspaceFolder(ctx, local.ID, 0, repository(t, "main")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RegisterWorkspace(ctx, "finance"); err != nil {
		t.Fatal(err)
	}
	gone, _ := store.RegisterWorkspace(ctx, "gone")
	goneClone := repository(t, "main")
	gitIn(t, goneClone, "remote", "add", "origin", "https://github.com/davidadel66/gone.git")
	if _, err := store.SetWorkspaceFolder(ctx, gone.ID, 0, goneClone); err != nil {
		t.Fatal(err)
	}

	ticks := make(chan time.Time)
	var every time.Duration
	stopped := false
	syncer := changes.NewSyncer(changes.SyncConfig{GitHub: client, Store: store, Workspaces: store, Logf: t.Logf,
		Ticker: func(d time.Duration) (<-chan time.Time, func()) {
			every = d
			return ticks, func() { stopped = true }
		}})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		syncer.Run(runCtx)
		close(done)
	}()
	// It syncs when it starts, then on every tick; the tick is taken only
	// once the first round has finished.
	waitFor(t, fake.listed, "the sync at start")
	ticks <- time.Now()
	waitFor(t, fake.listed, "the sync on the tick")
	cancel()
	waitFor(t, done, "Run to stop")

	if every != 60*time.Second || !stopped {
		t.Fatalf("ticker every %s, stopped %v", every, stopped)
	}
	lists := 0
	for _, path := range fake.paths() {
		if path == "/repos/davidadel66/evie/pulls" {
			lists++
		}
		if !strings.HasPrefix(path, "/repos/davidadel66/evie/") && !strings.HasPrefix(path, "/repos/davidadel66/gone/") {
			t.Fatalf("requested %s", path)
		}
	}
	if lists != 2 {
		t.Fatalf("listed %d times: %v", lists, fake.paths())
	}
	if got := pullChanges(t, store, ws.ID); got["pr:7"].State != changes.Open {
		t.Fatalf("synced = %+v", got)
	}
	if got := pullChanges(t, store, local.ID); len(got) != 0 {
		t.Fatalf("local Workspace = %+v", got)
	}
}
