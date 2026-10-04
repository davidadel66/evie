package subagents_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/worktree"
	"github.com/google/uuid"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// buildRepo is the owner's checkout: a repository with one commit on master
// and an uncommitted change the build worker must never see or touch.
func buildRepo(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	repo := filepath.Join(base, "repo")
	for path, content := range map[string]string{"main.go": "package main\n\nfunc main() {}\n", "README.md": "# Repo\n"} {
		if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "init", "-q", "-b", "master")
	git(t, repo, "config", "user.name", "Owner")
	git(t, repo, "config", "user.email", "owner@example.com")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "first")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Repo\nowner's uncommitted edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// ownerTasks is the orchestrator's own Task authority in parent's scope.
func ownerTasks(p delegation.Parent) context.Context {
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{ActorID: string(memory.LocalOwnerID), SessionID: string(p.Scope.SessionID),
		WorkspaceID: string(p.Scope.WorkspaceID), ProjectID: string(p.Scope.ProjectID), RunID: "orchestrator",
		LeaseHolderID: string(p.Lease.HolderID), LeaseToken: uint64(p.Lease.FencingToken), LeaseGeneration: uint64(p.Lease.Generation)})
}

// taskTree is a root with two sibling subtasks; the build worker gets the
// first, and another root stands for an unrelated tree.
type taskTree struct{ root, assigned, sibling, other task.Task }

func (f *fixture) taskTree(t *testing.T, p delegation.Parent) taskTree {
	t.Helper()
	ctx := ownerTasks(p)
	root, err := f.store.CreateGlobalTask(ctx, task.CreateInput{Title: "Ship the parser", IdempotencyKey: "root"})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := f.store.DecomposeGlobalTask(ctx, root.ID, task.DecomposeInput{ExpectedRevision: root.Revision, Children: []task.ChildInput{{Title: "Write the parser"}, {Title: "Write the docs"}}, IdempotencyKey: "children"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.store.CreateGlobalTask(ctx, task.CreateInput{Title: "Unrelated", IdempotencyKey: "other"})
	if err != nil {
		t.Fatal(err)
	}
	return taskTree{root: tree.Parent, assigned: tree.Children[0], sibling: tree.Children[1], other: other}
}

// build commits the parent's delegate_build intent for requests and runs it
// through the supervisor.
func (f *fixture) build(t *testing.T, ctx context.Context, p delegation.Parent, requests []delegation.Assignment) ([]delegation.Result, error) {
	t.Helper()
	b, _ := json.Marshal(struct {
		Assignments []delegation.Assignment `json:"assignments"`
	}{requests})
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-" + uuid.NewString(), Name: delegation.BuildToolName, Arguments: string(b)}})
	intent, err := f.store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: p.SourceEventID, ExecutionID: memory.ExecutionID(uuid.NewString()), Payload: payload})
	if err != nil {
		return nil, err
	}
	p.IntentEventID = intent.ID
	return f.supervisor.Build(ctx, p, requests)
}

// resultOf is the content of tool call id's result in r.
func resultOf(r openrouter.ChatRequest, id string) string {
	for _, m := range r.Messages {
		if m.Role == "tool" && m.ToolCallID == id {
			return m.Content
		}
	}
	return ""
}

// revisionIn is the revision of the Task in a todo tool result.
func revisionIn(t *testing.T, content string) uint64 {
	var value struct {
		Revision uint64 `json:"revision"`
		Parent   struct {
			Revision uint64 `json:"revision"`
		} `json:"parent"`
	}
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		t.Errorf("todo result %q: %v", content, err)
	}
	if value.Revision == 0 {
		return value.Parent.Revision
	}
	return value.Revision
}

func call(id, name string, args any) openrouter.ChatResponse {
	b, _ := json.Marshal(args)
	return multiCall(toolCall(id, name, string(b)))
}

// buildScript plays a build child: step i of steps answers its i-th model
// call, given the request (which holds earlier tool results).
type buildScript struct {
	mu       sync.Mutex
	steps    []func(r openrouter.ChatRequest) openrouter.ChatResponse
	calls    int
	requests []openrouter.ChatRequest
}

func (s *buildScript) ChatStream(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r)
	if s.calls >= len(s.steps) {
		return openrouter.ChatResponse{}, errors.New("build script exhausted")
	}
	step := s.steps[s.calls]
	s.calls++
	return step(r), nil
}

func (s *buildScript) recorded() []openrouter.ChatRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]openrouter.ChatRequest(nil), s.requests...)
}

func buildReport(summary string) string {
	return "## Summary\n" + summary + "\n\n## Details\nparser.go added; main.go calls it.\n\n## Limitations\n- Only the happy path is tested.\n"
}

func toolNamesOf(r openrouter.ChatRequest) []string { return toolNames(r) }

// Stage 11 through the primary acceptance seam: the parent model delegates
// a code change on its Task through the real Plugin, dispatcher and agent
// loop. The build child claims and updates its Task, writes and edits files
// in its worktree without approval, runs a command in the sandbox when the
// host has one, and adds a subtask; its escapes are refused. The harness
// commits on the worker's branch, and the parent receives the branch,
// commit and files changed with the commands framed as child output. The
// owner's checkout, master, the sibling and parent Tasks are untouched; the
// grant and claim end with the attempt.
func TestComposedParentDelegatesBuildAndReceivesBranchCommitsAndFiles(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	ctx := context.Background()
	project, err := f.store.RegisterProject(ctx, "Built", repo)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.CreateProjectSessionWithComposition(ctx, project.ID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	setupTurn := f.liveTurn(t, stored)
	tree := f.taskTree(t, setupTurn)
	if err := f.store.ReleaseTurnLease(ctx, stored.ID, setupTurn.Lease.HolderID, setupTurn.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	masterBefore := git(t, repo, "rev-parse", "master")
	sandbox := tools.WorkerSandboxAvailable()
	owned := string(tree.assigned.ID)

	var built []delegation.ParentResult
	var parentCalls, childCalls atomic.Int32
	var childRequests []openrouter.ChatRequest
	var mu sync.Mutex
	child := []func(r openrouter.ChatRequest) openrouter.ChatResponse{
		func(openrouter.ChatRequest) openrouter.ChatResponse {
			return call("get", "todo_get", map[string]any{"task_id": owned})
		},
		func(openrouter.ChatRequest) openrouter.ChatResponse {
			return call("claim", "todo_claim", map[string]any{"task_id": owned, "idempotency_key": "claim"})
		},
		func(r openrouter.ChatRequest) openrouter.ChatResponse {
			return call("progress", "todo_update", map[string]any{"task_id": owned, "expected_revision": revisionIn(t, resultOf(r, "get")), "status": "in_progress", "result_summary": "parser written", "idempotency_key": "progress"})
		},
		func(openrouter.ChatRequest) openrouter.ChatResponse {
			return multiCall(toolCall("write", "write_file", `{"path":"parser.go","content":"package main\n\nfunc parse() {}\n"}`),
				toolCall("edit", "edit_file", `{"path":"main.go","old_string":"func main() {}","new_string":"func main() { parse() }"}`),
				toolCall("escape-file", "write_file", fmt.Sprintf(`{"path":%q,"content":"pwned"}`, filepath.Join(repo, "README.md"))),
				toolCall("escape-up", "write_file", `{"path":"../../../README.md","content":"pwned"}`))
		},
		func(r openrouter.ChatRequest) openrouter.ChatResponse {
			if !sandbox {
				return call("noop", "todo_get", map[string]any{"task_id": owned})
			}
			return multiCall(toolCall("test", "bash", `{"command":"test -f parser.go && grep -c parse main.go"}`),
				toolCall("escape-bash", "bash", fmt.Sprintf(`{"command":"echo pwned > %s"}`, filepath.Join(repo, "README.md"))))
		},
		func(r openrouter.ChatRequest) openrouter.ChatResponse {
			return multiCall(toolCall("sibling", "todo_update", fmt.Sprintf(`{"task_id":%q,"expected_revision":%d,"title":"hijacked","idempotency_key":"sibling"}`, tree.sibling.ID, tree.sibling.Revision)),
				toolCall("parent", "todo_get", fmt.Sprintf(`{"task_id":%q}`, tree.root.ID)),
				toolCall("other", "todo_claim", fmt.Sprintf(`{"task_id":%q,"idempotency_key":"other"}`, tree.other.ID)))
		},
		func(r openrouter.ChatRequest) openrouter.ChatResponse {
			return call("decompose", "todo_decompose", map[string]any{"task_id": owned, "expected_revision": revisionIn(t, resultOf(r, "progress")), "children": []map[string]string{{"title": "Parse errors"}}, "idempotency_key": "decompose"})
		},
		func(openrouter.ChatRequest) openrouter.ChatResponse {
			return response(buildReport("BUILD-SUMMARY-SENTINEL: added parser.go and wired it into main.go."))
		},
	}
	client := clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if slices.Contains(toolNames(r), delegation.BuildToolName) {
			switch parentCalls.Add(1) {
			case 1:
				return call("build", delegation.BuildToolName, map[string]any{"assignments": []map[string]string{{"idempotency_key": "parser", "objective": "Add a parser and call it from main", "task_id": owned}}}), nil
			}
			if err := json.Unmarshal([]byte(resultOf(r, "build")), &built); err != nil {
				return openrouter.ChatResponse{}, fmt.Errorf("build result %q: %v", resultOf(r, "build"), err)
			}
			return response("The parser is on its branch."), nil
		}
		n := int(childCalls.Add(1))
		mu.Lock()
		childRequests = append(childRequests, r)
		mu.Unlock()
		if n > len(child) {
			return openrouter.ChatResponse{}, errors.New("child script exhausted")
		}
		return child[n-1](r), nil
	})
	configure(t, f, client)
	session := agent.NewWithToolset(client, f.profile, f.store.BindHistory(stored.ID, "owner"), stored.ScopeContext(), f.store.BindTurnOwner(stored.ID, "owner"), resolved.Toolset)
	if err := session.Send(ctx, "Build the parser", &eventSink{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(built) != 1 {
		t.Fatalf("parent received %d build results", len(built))
	}
	r := built[0]
	if r.Status != "succeeded" || r.Worker != delegation.WorkerBuild || r.Build == nil {
		t.Fatalf("build result = %+v", r)
	}
	payload := sealedBuildFrame(t, r.Summary)
	if !strings.Contains(payload, "BUILD-SUMMARY-SENTINEL") || !strings.Contains(payload, "Commands run (outcome recorded by Evie's harness):") {
		t.Fatalf("framed build output = %q", payload)
	}
	if sandbox && (!strings.Contains(payload, "- exit 0: test -f parser.go") || !strings.Contains(payload, "echo pwned")) {
		t.Fatalf("commands were not listed with their outcomes: %q", payload)
	}
	b := r.Build
	if !strings.HasPrefix(b.Branch, "evie/") || b.Base != "master" || b.BaseCommit != masterBefore || len(b.Commits) != 1 || b.Commits[0].Subject != "Add a parser and call it from main" ||
		b.FilesChanged != 2 || len(b.Files) != 2 || b.Uncommitted || b.Worktree != filepath.Join(repo, ".evie", "worktrees", r.ExecutionID) {
		t.Fatalf("build view = %+v", b)
	}
	if head := git(t, repo, "rev-parse", "refs/heads/"+b.Branch); head != b.Head || head != b.Commits[0].SHA {
		t.Fatalf("branch head %s, report %+v", head, b)
	}
	if got := git(t, repo, "show", b.Head+":parser.go"); !strings.Contains(got, "func parse()") {
		t.Fatalf("committed parser.go = %q", got)
	}
	if got := git(t, repo, "log", "-1", "--format=%an|%B", b.Head); !strings.HasPrefix(got, "Owner|Add a parser and call it from main\n\nEvie build execution "+r.ExecutionID) {
		t.Fatalf("harness commit = %q", got)
	}
	// The owner's checkout, its branch and its uncommitted change are intact.
	if git(t, repo, "rev-parse", "master") != masterBefore || git(t, repo, "rev-parse", "--abbrev-ref", "HEAD") != "master" {
		t.Fatal("master or the owner's checkout moved")
	}
	if got, _ := os.ReadFile(filepath.Join(repo, "README.md")); string(got) != "# Repo\nowner's uncommitted edit\n" {
		t.Fatalf("owner's checkout changed: %q", got)
	}
	if status := git(t, repo, "status", "--porcelain"); status != "M README.md" {
		t.Fatalf("owner's status = %q", status)
	}

	// The child's tools and the escapes' refusals.
	mu.Lock()
	requests := append([]openrouter.ChatRequest(nil), childRequests...)
	mu.Unlock()
	wantTools := []string{"read_file", "edit_file", "write_file"}
	if sandbox {
		wantTools = append(wantTools, "bash")
	}
	wantTools = append(wantTools, "todo_get", "todo_update", "todo_decompose", "todo_claim", "todo_release")
	if got := toolNames(requests[0]); !slices.Equal(got, wantTools) {
		t.Fatalf("child tools = %v, want %v", got, wantTools)
	}
	if first := requests[0].Messages[len(requests[0].Messages)-1].Content; !strings.Contains(first, "Worktree pinned by Evie's harness") || !strings.Contains(first, "Your Task: "+owned) {
		t.Fatalf("assignment lacks the worktree and Task note: %q", first)
	}
	last := requests[len(requests)-1]
	for id, want := range map[string]string{"escape-file": "outside", "escape-up": "outside", "sibling": "not found", "parent": "not found", "other": "not found"} {
		if got := resultOf(last, id); !strings.Contains(strings.ToLower(got), want) && !strings.Contains(got, "denied") {
			t.Fatalf("escape %s = %q; want refusal naming %q", id, got, want)
		}
	}
	if sandbox && !strings.Contains(resultOf(last, "escape-bash"), "Operation not permitted") {
		t.Fatalf("bash escape = %q", resultOf(last, "escape-bash"))
	}

	// The Task: the worker's own subtree changed; nothing else did.
	turn := f.liveTurn(t, stored)
	owner := ownerTasks(turn)
	assigned, err := f.store.GetGlobalTask(owner, tree.assigned.ID)
	if err != nil || assigned.Status != task.StatusInProgress || assigned.ResultSummary != "parser written" {
		t.Fatalf("assigned Task = %+v %v", assigned, err)
	}
	subtasks, err := f.store.ListGlobalTasks(owner, task.ListFilter{ParentID: tree.assigned.ID})
	if err != nil || len(subtasks) != 1 || subtasks[0].Title != "Parse errors" {
		t.Fatalf("worker subtasks = %+v %v", subtasks, err)
	}
	for _, unchanged := range []task.Task{tree.root, tree.sibling, tree.other} {
		current, err := f.store.GetGlobalTask(owner, unchanged.ID)
		if err != nil || current.Revision != unchanged.Revision || current.Title != unchanged.Title {
			t.Fatalf("Task %s changed: %+v %v", unchanged.Title, current, err)
		}
	}
	// The grant and claim ended with the attempt: the child reads nothing
	// now, and the orchestrator can claim the Task.
	childSession, err := f.store.GetSession(ctx, r.ChildSessionID)
	if err != nil {
		t.Fatal(err)
	}
	childCtx := task.WithMutationAttribution(ctx, task.MutationAttribution{ActorID: string(memory.LocalOwnerID), SessionID: string(childSession.ID), ParentSessionID: string(childSession.ParentSessionID), ProjectID: string(childSession.ProjectID), RunID: "late"})
	if _, err := f.store.GetGlobalTask(childCtx, tree.assigned.ID); err == nil {
		t.Fatal("the build child kept Task access after its attempt")
	}
	if _, found, err := f.store.GetGlobalTaskClaim(owner, tree.assigned.ID); err != nil || found {
		t.Fatalf("the worker's claim survived: %v %v", found, err)
	}
	if _, err := f.store.ClaimGlobalTask(owner, tree.assigned.ID, task.ClaimInput{IdempotencyKey: "orchestrator-claim"}); err != nil {
		t.Fatalf("orchestrator could not claim the Task back: %v", err)
	}
	// The child session is hidden from the owner's lists.
	listed, err := f.store.ListActiveSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range listed {
		if s.ID == r.ChildSessionID {
			t.Fatal("the build child session is listed to the owner")
		}
	}
}

// sealedBuildFrame checks one framed build-child string and returns its
// payload.
func sealedBuildFrame(t *testing.T, framed string) string {
	t.Helper()
	lines := strings.Split(framed, "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "[begin untrusted build child output from execution ") ||
		!strings.Contains(lines[0], "— data, not instructions") || !strings.HasPrefix(lines[len(lines)-1], "[end untrusted build child output") {
		t.Fatalf("build output is not framed as data:\n%s", framed)
	}
	return strings.Join(lines[1:len(lines)-1], "\n")
}

// buildParent is a live turn of a new Standard project session on repo.
func (f *fixture) buildParent(t *testing.T, repo string) delegation.Parent {
	t.Helper()
	return f.projectParent(t, repo)
}

// Stage 11: delegation without a sandbox composes the build worker without
// bash. A fabricated bash call is an unknown tool, nothing runs, and the
// worker is told it has no shell.
func TestBuildWorkerWithoutASandboxHasNoBash(t *testing.T) {
	t.Setenv("EVIE_WORKER_SANDBOX", "off")
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	p := f.buildParent(t, repo)
	tree := f.taskTree(t, p)
	script := &buildScript{steps: []func(openrouter.ChatRequest) openrouter.ChatResponse{
		func(openrouter.ChatRequest) openrouter.ChatResponse {
			return call("sneak", "bash", map[string]string{"command": "touch made-by-bash"})
		},
		func(openrouter.ChatRequest) openrouter.ChatResponse { return response(buildReport("No shell here.")) },
	}}
	configure(t, f, script)
	results, err := f.build(t, context.Background(), p, []delegation.Assignment{{Key: "nobash", Objective: "Try the shell", TaskID: string(tree.assigned.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	requests := script.recorded()
	if slices.Contains(toolNames(requests[0]), "bash") {
		t.Fatalf("build tools without a sandbox = %v", toolNames(requests[0]))
	}
	if got := resultOf(requests[1], "sneak"); !strings.Contains(got, "Unknown Tool Call") {
		t.Fatalf("fabricated bash = %q", got)
	}
	if !strings.Contains(lastMessage(requests[0]), "You have no shell") {
		t.Fatalf("the worker was not told it has no shell: %q", lastMessage(requests[0]))
	}
	if _, err := os.Stat(filepath.Join(results[0].Build.Worktree, "made-by-bash")); !os.IsNotExist(err) {
		t.Fatal("a fabricated bash call ran")
	}
	if results[0].Status != "succeeded" || len(results[0].Build.Commits) != 0 || results[0].Build.FilesChanged != 0 {
		t.Fatalf("result = %+v %+v", results[0], results[0].Build)
	}
}

// Stage 11: when a build child's turn ends, its turn lease is released and,
// with it, its claim on its project-scoped Task; the release is the Kernel's
// ordinary lease cleanup, not a failure masked by the accepted report.
func TestBuildChildTurnEndReleasesItsLeaseAndClaim(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	p := f.buildParent(t, repo)
	tree := f.taskTree(t, p)
	owned := string(tree.assigned.ID)
	configure(t, f, &buildScript{steps: []func(openrouter.ChatRequest) openrouter.ChatResponse{
		func(openrouter.ChatRequest) openrouter.ChatResponse {
			return call("claim", "todo_claim", map[string]any{"task_id": owned, "idempotency_key": "claim"})
		},
		func(openrouter.ChatRequest) openrouter.ChatResponse {
			return response(buildReport("Claimed and done."))
		},
	}})
	results, err := f.build(t, context.Background(), p, []delegation.Assignment{{Key: "lease", Objective: "Claim", TaskID: owned}})
	if err != nil || results[0].Status != "succeeded" {
		t.Fatalf("build = %+v %v", results, err)
	}
	var holder any
	if err := f.db.QueryRow(`SELECT holder_id FROM session_turn_leases WHERE session_id=?`, results[0].ChildSessionID).Scan(&holder); err != nil || holder != nil {
		t.Fatalf("the child's turn lease is still held: %v %v", holder, err)
	}
	var runs []string
	rows, err := f.db.Query(`SELECT run_id FROM task_claim_events WHERE task_id=? AND operation='release'`, owned)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var run string
		if err := rows.Scan(&run); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run)
	}
	if !slices.Equal(runs, []string{"turn-lease-cleanup"}) {
		t.Fatalf("claim releases = %v, want the lease cleanup's", runs)
	}
}

// Stage 11: a build child that wraps up at its budget still has its work
// committed by the harness on its branch, and settles partial with it.
func TestBuildHarnessCommitsAWrappedUpChildAsPartial(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	p := f.buildParent(t, repo)
	tree := f.taskTree(t, p)
	var n atomic.Int32
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if r.ToolChoice == "none" {
			return response(buildReport("Wrapped up after writing files.")), nil
		}
		i := n.Add(1)
		return call(fmt.Sprintf("w%d", i), "write_file", map[string]string{"path": fmt.Sprintf("gen/file%02d.txt", i), "content": "x\n"}), nil
	}))
	results, err := f.build(t, context.Background(), p, []delegation.Assignment{{Key: "loop", Objective: "Generate files", TaskID: string(tree.assigned.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Status != delegation.StatePartial || r.Build == nil || len(r.Build.Commits) != 1 || r.Build.FilesChanged == 0 || r.Build.Uncommitted {
		t.Fatalf("partial build = %+v %+v", r, r.Build)
	}
	if got := git(t, repo, "ls-tree", "-r", "--name-only", r.Build.Head); !strings.Contains(got, "gen/file01.txt") {
		t.Fatalf("the wrap-up commit lacks the worker's files: %s", got)
	}
}

// Stage 11: a crash mid-attempt leaves the worktree. The attempt is admitted
// and started exactly as the supervisor does, its worktree is created, and
// the worker writes a file and claims its Task; then the process dies with
// no settlement and the parent's turn ownership ends. Recovery marks the
// attempt interrupted, keeps the branch and the uncommitted work for
// inspection, ends the Task grant and claim, and says how to resume or clean
// up; the owner's cleanup removes the worktree, after which the child cannot
// be continued there.
func TestBuildRecoveryAfterACrashKeepsTheWorktreeForInspection(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	p := f.buildParent(t, repo)
	tree := f.taskTree(t, p)
	ctx := context.Background()
	request := []delegation.Assignment{{Key: "crash", Objective: "Work in progress", TaskID: string(tree.assigned.ID)}}
	invoked, err := f.invokeTool(ctx, p, delegation.BuildToolName, request)
	if err != nil {
		t.Fatal(err)
	}
	template, err := f.manager.ResolvePreset(plugins.BuildPresetID)
	if err != nil {
		t.Fatal(err)
	}
	parentReceipt, err := f.store.GetCompositionReceipt(ctx, p.Scope.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	childReceipt, err := subagents.BuildReceiptForTest(template.Receipt, parentReceipt)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := worktree.Plan(ctx, repo, "", uuid.NewString(), "Work in progress")
	if err != nil {
		t.Fatal(err)
	}
	attempts, err := f.store.AdmitBuildSubagents(ctx, invoked, request, []delegation.Worktree{plan}, []error{nil}, childReceipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a := attempts[0]
	if _, started, err := f.store.StartSubagent(ctx, a.ID); err != nil || !started {
		t.Fatalf("start = %v %v", started, err)
	}
	if err := worktree.Ensure(ctx, *a.Worktree, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Worktree.Path, "wip.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	childLease, err := f.store.AcquireTurnLease(ctx, a.Child.ID, memory.LeaseHolderID(a.Child.ID), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	childCtx := task.WithMutationAttribution(ctx, task.MutationAttribution{ActorID: string(memory.LocalOwnerID), SessionID: string(a.Child.ID), ParentSessionID: string(p.Scope.SessionID),
		ProjectID: string(p.Scope.ProjectID), RunID: "child", LeaseHolderID: string(childLease.HolderID), LeaseToken: uint64(childLease.FencingToken), LeaseGeneration: uint64(childLease.Generation)})
	if _, err := f.store.ClaimGlobalTask(childCtx, tree.assigned.ID, task.ClaimInput{IdempotencyKey: "claim"}); err != nil {
		t.Fatalf("the worker could not claim its Task: %v", err)
	}
	if _, err := f.store.GetGlobalTask(childCtx, tree.sibling.ID); err == nil {
		t.Fatal("the worker's grant reaches a sibling Task")
	}
	if _, err := f.store.GetGlobalTask(childCtx, tree.root.ID); err == nil {
		t.Fatal("the worker's grant reaches its parent Task")
	}

	// The crash: nothing settles the attempt, and the parent's turn
	// ownership ends. Recovery takes over.
	if err := f.store.ReleaseTurnLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if n, err := f.store.RecoverSubagents(ctx); err != nil || n != 1 {
		t.Fatalf("recovered %d: %v", n, err)
	}
	turn := f.liveTurn(t, mustSession(t, f, p.Scope.SessionID))
	recovered, err := f.store.InspectSubagent(ctx, turn, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != "interrupted" || recovered.Result.Worker != delegation.WorkerBuild || recovered.Result.Build == nil ||
		recovered.Result.Build.Branch != a.Worktree.Branch || recovered.Result.Build.Worktree != a.Worktree.Path {
		t.Fatalf("recovered = %s %+v", recovered.State, recovered.Result)
	}
	notes := strings.Join(recovered.Result.Notes, " ")
	if !strings.Contains(notes, "kept for inspection") || !strings.Contains(notes, "evie worktrees clean "+a.ID) || !strings.Contains(notes, "base \""+a.Worktree.Branch+"\"") {
		t.Fatalf("recovery notes = %q", notes)
	}
	if present, dirty, err := worktree.Status(ctx, *a.Worktree); err != nil || !present || !dirty {
		t.Fatalf("worktree after recovery = %v %v %v", present, dirty, err)
	}
	if _, found, err := f.store.GetGlobalTaskClaim(ownerTasks(turn), tree.assigned.ID); err != nil || found {
		t.Fatalf("the crashed worker's claim survived recovery: %v %v", found, err)
	}
	if _, err := f.store.GetGlobalTask(childCtx, tree.assigned.ID); err == nil {
		t.Fatal("the crashed worker kept its Task grant")
	}

	// Cleanup by the owner's command path.
	listed, err := f.store.ListBuildWorktrees(ctx)
	if err != nil || len(listed) != 1 || listed[0].ExecutionID != a.ID || listed[0].State != "interrupted" {
		t.Fatalf("listed worktrees = %+v %v", listed, err)
	}
	w, err := f.store.BuildWorktreeFor(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.Remove(ctx, w, false); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MarkBuildWorktreeRemoved(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Worktree.Path); !os.IsNotExist(err) {
		t.Fatal("cleanup left the worktree")
	}
	if got := git(t, repo, "branch", "--list", a.Worktree.Branch); got == "" {
		t.Fatal("cleanup deleted the branch without being asked")
	}
}

// invokeTool commits the parent's intent to call tool with requests.
func (f *fixture) invokeTool(ctx context.Context, p delegation.Parent, tool string, requests []delegation.Assignment) (delegation.Parent, error) {
	b, _ := json.Marshal(struct {
		Assignments []delegation.Assignment `json:"assignments"`
	}{requests})
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-" + uuid.NewString(), Name: tool, Arguments: string(b)}})
	intent, err := f.store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: p.SourceEventID, ExecutionID: memory.ExecutionID(uuid.NewString()), Payload: payload})
	if err != nil {
		return p, err
	}
	p.IntentEventID = intent.ID
	return p, nil
}

// Stage 11: continuing a build child resumes it in the same worktree on the
// same branch, under a fresh grant on its Task; the harness commits the
// follow-up's work as a second commit. Once the owner removes the worktree,
// the child cannot be continued.
func TestBuildContinuationResumesInTheSameWorktree(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	p := f.buildParent(t, repo)
	tree := f.taskTree(t, p)
	ctx := context.Background()
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		last := lastMessage(r)
		switch {
		case strings.Contains(last, "Follow-up assignment") && resultOf(r, "second") == "":
			return call("second", "write_file", map[string]string{"path": "second.txt", "content": "two\n"}), nil
		case strings.Contains(last, "Follow-up assignment") || resultOf(r, "second") != "":
			return response(buildReport("Second change.")), nil
		case resultOf(r, "first") == "":
			return call("first", "write_file", map[string]string{"path": "first.txt", "content": "one\n"}), nil
		}
		return response(buildReport("First change.")), nil
	}))
	first, err := f.build(t, ctx, p, []delegation.Assignment{{Key: "first", Objective: "First change", TaskID: string(tree.assigned.ID)}})
	if err != nil || first[0].Status != "succeeded" || len(first[0].Build.Commits) != 1 {
		t.Fatalf("first = %+v %v", first, err)
	}
	second, err := f.continueResearch(t, p, first[0].ExecutionID, "Add the second file")
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "succeeded" || second.Build == nil || second.Build.Worktree != first[0].Build.Worktree || second.Build.Branch != first[0].Build.Branch ||
		len(second.Build.Commits) != 2 || second.Build.Commits[0].Subject != "Continue: Add the second file" || second.Build.FilesChanged != 2 {
		t.Fatalf("continuation = %+v %+v", second, second.Build)
	}
	if err := worktree.Remove(ctx, delegation.Worktree{}, false); err == nil {
		t.Fatal("removing an unplanned worktree succeeded")
	}
	// A worktree deleted behind the harness's back is never recreated for a
	// continuation: the attempt fails and says why.
	if err := os.RemoveAll(second.Build.Worktree); err != nil {
		t.Fatal(err)
	}
	missing, err := f.continueResearch(t, p, second.ExecutionID, "Third change")
	if err != nil {
		t.Fatal(err)
	}
	if missing.Status != "failed" || missing.Reason != "worktree_unavailable" || missing.Build == nil || !strings.Contains(missing.Build.CommitError, "missing") ||
		!strings.Contains(strings.Join(missing.Notes, " "), "Worktree problem") {
		t.Fatalf("continuation without its worktree = %+v %+v", missing, missing.Build)
	}
	if _, err := os.Stat(second.Build.Worktree); !os.IsNotExist(err) {
		t.Fatal("the harness recreated a deleted worktree for a continuation")
	}
	w, err := f.store.BuildWorktreeFor(ctx, second.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.Remove(ctx, w, false); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MarkBuildWorktreeRemoved(ctx, second.ExecutionID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.continueResearch(t, p, second.ExecutionID, "One more"); err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("continuation after cleanup: %v", err)
	}
}

// unfinishedBuilds lists admitted or running build attempts.
func (f *fixture) unfinishedBuilds(t *testing.T) []delegation.Attempt {
	t.Helper()
	rows, err := f.db.Query(`SELECT record_json FROM subagent_executions WHERE state IN ('admitted','running')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []delegation.Attempt
	for rows.Next() {
		var record string
		if err := rows.Scan(&record); err != nil {
			t.Fatal(err)
		}
		var a delegation.Attempt
		if err := json.Unmarshal([]byte(record), &a); err != nil {
			t.Fatal(err)
		}
		if a.Worktree != nil {
			out = append(out, a)
		}
	}
	return out
}

func mustSession(t *testing.T, f *fixture, id memory.SessionID) memory.Session {
	t.Helper()
	s, err := f.store.GetSession(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Stage 11: build admission refuses, before any model call or attempt, a
// folder that is not a repository, a repository mid-merge, a base it cannot
// resolve, a Task that is completed or claimed by a live execution, and a
// parent pinned before build delegation; a retained key still replays when
// the repository changed since.
func TestBuildAdmissionRefusesUnsafeRepositoriesTasksAndParents(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	script := &buildScript{steps: []func(openrouter.ChatRequest) openrouter.ChatResponse{
		func(openrouter.ChatRequest) openrouter.ChatResponse { return response(buildReport("done")) },
	}}
	configure(t, f, script)

	plain, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := f.buildParent(t, plain)
	tree := f.taskTree(t, p)
	if _, err := f.build(t, ctx, p, []delegation.Assignment{{Key: "plain", Objective: "x", TaskID: string(tree.assigned.ID)}}); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("plain folder: %v", err)
	}

	repo := buildRepo(t)
	p = f.buildParent(t, repo)
	tree = f.taskTree(t, p)
	owned := string(tree.assigned.ID)
	refusals := []struct {
		name  string
		setup func()
		a     delegation.Assignment
		want  string
	}{
		{"unknown base", nil, delegation.Assignment{Key: "base", Objective: "x", TaskID: owned, Base: "no-such-branch"}, "cannot be resolved"},
		{"merge in progress", func() {
			if err := os.WriteFile(filepath.Join(repo, ".git", "MERGE_HEAD"), []byte(git(t, repo, "rev-parse", "HEAD")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, delegation.Assignment{Key: "merge", Objective: "x", TaskID: owned}, "merge"},
		{"claimed Task", func() {
			if err := os.Remove(filepath.Join(repo, ".git", "MERGE_HEAD")); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.ClaimGlobalTask(ownerTasks(p), tree.assigned.ID, task.ClaimInput{IdempotencyKey: "held"}); err != nil {
				t.Fatal(err)
			}
		}, delegation.Assignment{Key: "claimed", Objective: "x", TaskID: owned}, "claimed by an active execution"},
		{"unknown Task", nil, delegation.Assignment{Key: "ghost", Objective: "x", TaskID: "no-such-task"}, "not found"},
	}
	for _, c := range refusals {
		if c.setup != nil {
			c.setup()
		}
		if _, err := f.build(t, ctx, p, []delegation.Assignment{c.a}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: %v; want refusal naming %q", c.name, err, c.want)
		}
	}
	if len(script.recorded()) != 0 || f.attemptCount(t) != 0 {
		t.Fatal("a refused build reached a model or admitted an attempt")
	}
	if out := git(t, repo, "branch", "--list", "evie/*"); out != "" {
		t.Fatalf("a refused build created a branch: %s", out)
	}

	// A fresh Task is admitted; its key then replays even mid-merge.
	if _, err := f.store.ReleaseGlobalTaskClaim(ownerTasks(p), tree.assigned.ID, task.ReleaseInput{IdempotencyKey: "release"}); err != nil {
		t.Fatal(err)
	}
	request := []delegation.Assignment{{Key: "ok", Objective: "Do it", TaskID: owned}}
	first, err := f.build(t, ctx, p, request)
	if err != nil || first[0].Status != "succeeded" {
		t.Fatalf("admitted build = %+v %v", first, err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "MERGE_HEAD"), []byte(git(t, repo, "rev-parse", "HEAD")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := f.build(t, ctx, p, request)
	if err != nil || again[0].ExecutionID != first[0].ExecutionID || !again[0].Replayed {
		t.Fatalf("replay = %+v %v", again, err)
	}
	if err := os.Remove(filepath.Join(repo, ".git", "MERGE_HEAD")); err != nil {
		t.Fatal(err)
	}

	// A completed Task is refused.
	ownerCtx := ownerTasks(p)
	completed := task.StatusCompleted
	current, err := f.store.GetGlobalTask(ownerCtx, tree.sibling.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ClaimGlobalTask(ownerCtx, tree.sibling.ID, task.ClaimInput{IdempotencyKey: "finish-claim"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.UpdateGlobalTask(ownerCtx, tree.sibling.ID, task.UpdateInput{ExpectedRevision: current.Revision, Status: &completed, IdempotencyKey: "finish"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.build(t, ctx, p, []delegation.Assignment{{Key: "done-task", Objective: "x", TaskID: string(tree.sibling.ID)}}); err == nil || !strings.Contains(err.Error(), "completed") {
		t.Fatalf("completed Task: %v", err)
	}

	// A parent pinned to the Standard preset before build delegation
	// (bfd4eb42…) cannot delegate build work, and keeps its own tools.
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	old := withoutTodoReady(composition.Clone(resolved.Receipt))
	old.Preset.Version = "sha256:bfd4eb42705bce9f2ab68b1b0832d224f11b1e5bbb1f0221768188cd2d322d4e"
	// The merge gate (Stage 12) came after build, so it goes too.
	old.Capabilities = slices.DeleteFunc(old.Capabilities, func(c composition.Capability) bool {
		return c.ID == delegation.BuildCapabilityID || c.ID == delegation.MergeCapabilityID
	})
	old.ToolSchemas = slices.DeleteFunc(old.ToolSchemas, func(s composition.ToolSchema) bool {
		return s.Name == delegation.BuildToolName || s.Name == delegation.MergeToolName
	})
	reopened, err := f.manager.ResumeComposition(old)
	if err != nil {
		t.Fatalf("pre-build receipt does not reopen: %v", err)
	}
	if slices.Contains(toolNamesOfSet(reopened.Toolset), delegation.BuildToolName) || !slices.Contains(toolNamesOfSet(reopened.Toolset), delegation.ExploreToolName) {
		t.Fatal("a pre-build session's tools changed")
	}
	session, err := f.store.CreateProjectSessionWithComposition(ctx, p.Scope.ProjectID, old)
	if err != nil {
		t.Fatal(err)
	}
	oldParent := f.liveTurn(t, session)
	oldTree := f.taskTree(t, oldParent)
	if _, err := f.build(t, ctx, oldParent, []delegation.Assignment{{Key: "old", Objective: "x", TaskID: string(oldTree.assigned.ID)}}); err == nil || !strings.Contains(err.Error(), "does not permit build delegation") {
		t.Fatalf("build from a parent without the capability: %v", err)
	}
}

func toolNamesOfSet(set tools.Toolset) []string {
	var out []string
	for _, schema := range set.Schemas() {
		out = append(out, schema.Function.Name)
	}
	return out
}

// Stage 11: a Workspace needs the owner's explicit build setting, which is
// off by default even with a folder; turning it on allows new chats, and
// turning it off revokes them durably.
func TestWorkspaceBuildNeedsTheOwnersExplicitPermission(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	repo := buildRepo(t)
	configure(t, f, &buildScript{steps: []func(openrouter.ChatRequest) openrouter.ChatResponse{
		func(openrouter.ChatRequest) openrouter.ChatResponse { return response(buildReport("done")) },
	}})
	ws, err := f.store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Code", FolderPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if ws.Build.Setting != memory.WorkspaceBuildDefault || ws.Build.Allowed {
		t.Fatalf("new Workspace build = %+v", ws.Build)
	}
	workspaceParent := func(revision memory.WorkspaceRevisionID) delegation.Parent {
		resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
		if err != nil {
			t.Fatal(err)
		}
		session, err := f.store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, revision, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		return f.liveTurn(t, session)
	}
	p := workspaceParent(ws.CurrentRevisionID)
	tree := f.taskTree(t, p)
	if _, err := f.build(t, ctx, p, []delegation.Assignment{{Key: "off", Objective: "x", TaskID: string(tree.assigned.ID)}}); err == nil || !strings.Contains(err.Error(), "explicit permission") {
		t.Fatalf("default Workspace build: %v", err)
	}
	on, err := f.store.SetWorkspaceBuild(ctx, ws.ID, ws.CurrentRevisionID, memory.WorkspaceBuildOn)
	if err != nil || !on.Build.Allowed || on.Build.Setting != memory.WorkspaceBuildOn || on.Explore != ws.Explore {
		t.Fatalf("turned on = %+v %v", on, err)
	}
	p = workspaceParent(on.CurrentRevisionID)
	tree = f.taskTree(t, p)
	if r, err := f.build(t, ctx, p, []delegation.Assignment{{Key: "on", Objective: "x", TaskID: string(tree.assigned.ID)}}); err != nil || r[0].Status != "succeeded" {
		t.Fatalf("Workspace build when on: %+v %v", r, err)
	}
	// A research change carries the build setting forward.
	research, err := f.store.SetWorkspaceResearch(ctx, ws.ID, on.CurrentRevisionID, true)
	if err != nil || !research.Build.Allowed {
		t.Fatalf("research change dropped build: %+v %v", research.Build, err)
	}
	off, err := f.store.SetWorkspaceBuild(ctx, ws.ID, research.CurrentRevisionID, memory.WorkspaceBuildOff)
	if err != nil || off.Build.Allowed {
		t.Fatalf("turned off = %+v %v", off.Build, err)
	}
	again, err := f.store.SetWorkspaceBuild(ctx, ws.ID, off.CurrentRevisionID, memory.WorkspaceBuildOn)
	if err != nil {
		t.Fatal(err)
	}
	// The chat pinned to the earlier "on" revision stays revoked.
	other := f.taskTree(t, p)
	if _, err := f.build(t, ctx, p, []delegation.Assignment{{Key: "revoked", Objective: "x", TaskID: string(other.sibling.ID)}}); err == nil || !strings.Contains(err.Error(), "explicit permission") {
		t.Fatalf("revoked chat built: %v", err)
	}
	_ = again
}
