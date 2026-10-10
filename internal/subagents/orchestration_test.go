package subagents_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/worktree"
	"github.com/google/uuid"
)

// Stage 13 (2026-10-04): Task Tree orchestration. The orchestrator is the
// parent model inside its turn; todo_ready gives it conflict-free batches
// and the harness refuses premature or conflicting build admissions.

// assignmentOf is a worker request's assignment message.
func assignmentOf(r openrouter.ChatRequest) string {
	for _, m := range r.Messages {
		if m.Role == "user" && strings.HasPrefix(m.Content, "Assignment from the orchestrator:") {
			return m.Content
		}
	}
	return ""
}

// withoutTodoReady is receipt as composed before todo_ready existed.
func withoutTodoReady(receipt composition.Receipt) composition.Receipt {
	receipt.Capabilities = slices.DeleteFunc(receipt.Capabilities, func(c composition.Capability) bool { return c.ID == string(task.CapabilityReady) })
	receipt.ToolSchemas = slices.DeleteFunc(receipt.ToolSchemas, func(s composition.ToolSchema) bool { return s.Name == "todo_ready" })
	return receipt
}

// plannedTree is the Stage 13 acceptance tree: A and B are independent, C
// depends on both, D's paths overlap A's, and E declares no paths.
type plannedTree struct {
	root          task.Task
	a, b, c, d, e task.Task
}

func (f *fixture) plannedTree(t *testing.T, p delegation.Parent, withE bool) plannedTree {
	t.Helper()
	ctx := ownerTasks(p)
	root, err := f.store.CreateGlobalTask(ctx, task.CreateInput{Title: "Ship the feature", IdempotencyKey: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	children := []task.ChildInput{
		{Title: "A", Paths: []string{"pkg/a"}}, {Title: "B", Paths: []string{"pkg/b"}},
		{Title: "C", Paths: []string{"pkg/c"}, DependsOnChildren: []int{0, 1}}, {Title: "D", Paths: []string{"pkg/a/extra.go"}},
	}
	if withE {
		children = append(children, task.ChildInput{Title: "E"})
	}
	tree, err := f.store.DecomposeGlobalTask(ctx, root.ID, task.DecomposeInput{ExpectedRevision: root.Revision, IdempotencyKey: "plan", Children: children})
	if err != nil {
		t.Fatal(err)
	}
	out := plannedTree{root: tree.Parent, a: tree.Children[0], b: tree.Children[1], c: tree.Children[2], d: tree.Children[3]}
	if withE {
		out.e = tree.Children[4]
	}
	return out
}

func readiness(t *testing.T, f *fixture, p delegation.Parent, tree task.ID) task.Readiness {
	t.Helper()
	r, err := f.store.ReadyGlobalTasks(ownerTasks(p), task.ReadyQuery{TreeID: tree, Limits: task.DispatchLimits{MaxBatch: 8, PerParent: 2, Runtime: 4, PerTurn: 16}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func batchTaskIDs(r task.Readiness) [][]task.ID {
	out := [][]task.ID{}
	for _, batch := range r.Batches {
		var ids []task.ID
		for _, item := range batch {
			ids = append(ids, item.TaskID)
		}
		out = append(out, ids)
	}
	return out
}

func blockedReason(r task.Readiness, id task.ID) (task.BlockReason, bool) {
	for _, b := range r.Blocked {
		if b.TaskID == id && len(b.Reasons) > 0 {
			return b.Reasons[0], true
		}
	}
	return task.BlockReason{}, false
}

func mustReceipt(t *testing.T, f *fixture, session memory.SessionID) composition.Receipt {
	t.Helper()
	receipt, err := f.store.GetCompositionReceipt(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func attemptCount(t *testing.T, f *fixture) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM subagent_executions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The harness enforces the Task Tree rules at admission, whatever the model
// asks: a Task waits for its dependencies, two conflicting Tasks never run
// at once (in one call or across calls), and a Task without paths conflicts
// with every running build. Independent work runs alongside.
func TestBuildAdmissionEnforcesTaskTreeDependenciesAndConflicts(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	p := f.buildParent(t, repo)
	tree := f.plannedTree(t, p, true)
	ctx := context.Background()

	_, err := f.build(t, ctx, p, []delegation.Assignment{{Key: "c", Objective: "Implement C", TaskID: string(tree.c.ID)}})
	if err == nil || !strings.Contains(err.Error(), "depends on unfinished work") || !strings.Contains(err.Error(), string(tree.a.ID)) || !strings.Contains(err.Error(), string(tree.b.ID)) {
		t.Fatalf("premature C = %v", err)
	}
	_, err = f.build(t, ctx, p, []delegation.Assignment{{Key: "a", Objective: "Implement A", TaskID: string(tree.a.ID)}, {Key: "d", Objective: "Implement D", TaskID: string(tree.d.ID)}})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("Task %s conflicts with Task %s in this same delegate_build call (pkg/a/extra.go overlaps pkg/a)", tree.d.ID, tree.a.ID)) {
		t.Fatalf("A and D in one call = %v", err)
	}
	if n := attemptCount(t, f); n != 0 {
		t.Fatalf("refused calls admitted %d attempts", n)
	}

	wrote, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if strings.Contains(assignmentOf(r), "Implement A") {
			if resultOf(r, "wa") == "" {
				return call("wa", "write_file", map[string]string{"path": "pkg/a/a.go", "content": "package a\n"}), nil
			}
			once.Do(func() { close(wrote) })
			<-release
		}
		return response(buildReport("Done.")), nil
	}))
	var first []delegation.Result
	var firstErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		first, firstErr = f.build(t, ctx, p, []delegation.Assignment{{Key: "a", Objective: "Implement A", TaskID: string(tree.a.ID)}})
	}()
	select {
	case <-wrote:
	case <-done:
		t.Fatalf("A ended before its worker wrote: %+v %v", first, firstErr)
	case <-time.After(30 * time.Second):
		t.Fatal("A's worker never ran")
	}
	running := f.unfinishedBuilds(t)
	if len(running) != 1 {
		t.Fatalf("running builds = %d", len(running))
	}
	_, err = f.build(t, ctx, p, []delegation.Assignment{{Key: "d", Objective: "Implement D", TaskID: string(tree.d.ID)}})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("conflicts with build %s, which is running in this repository on Task %s (pkg/a/extra.go overlaps pkg/a)", running[0].ID, tree.a.ID)) {
		t.Fatalf("D while A runs = %v", err)
	}
	_, err = f.build(t, ctx, p, []delegation.Assignment{{Key: "e", Objective: "Implement E", TaskID: string(tree.e.ID)}})
	if err == nil || !strings.Contains(err.Error(), "it declares no paths, so it may touch any file") {
		t.Fatalf("E while A runs = %v", err)
	}
	// While A runs, todo_ready says so and offers B only.
	r := readiness(t, f, p, tree.root.ID)
	if got := batchTaskIDs(r); !slices.Equal(got[0], []task.ID{tree.b.ID}) || len(r.InFlight) != 1 || r.InFlight[0].Build.ExecutionID != running[0].ID {
		t.Fatalf("readiness while A runs: batches %v in flight %+v", got, r.InFlight)
	}
	if reason, _ := blockedReason(r, tree.d.ID); reason.Code != task.ReasonConflictsWithBuild || !slices.Contains(reason.ExecutionIDs, running[0].ID) {
		t.Fatalf("D's reason = %+v", reason)
	}
	independent, err := f.build(t, ctx, p, []delegation.Assignment{{Key: "b", Objective: "Implement B", TaskID: string(tree.b.ID)}})
	if err != nil || independent[0].Status != "succeeded" {
		t.Fatalf("B alongside A = %+v %v", independent, err)
	}
	close(release)
	<-done
	if firstErr != nil || first[0].Status != "succeeded" {
		t.Fatalf("A = %+v %v", first, firstErr)
	}
	// A finished but is not merged: the harness admits D now (no build is
	// running), while todo_ready still holds D until A's branch merges.
	r = readiness(t, f, p, tree.root.ID)
	if reason, _ := blockedReason(r, tree.d.ID); reason.Code != task.ReasonConflictsWithBuild || !strings.Contains(reason.Detail, "awaiting merge") {
		t.Fatalf("D after A finished = %+v", reason)
	}
}

// A conversation pinned before todo_ready cannot declare paths, so its
// builds of Tasks without paths keep running in parallel as before.
func TestParentPinnedBeforeTodoReadyKeepsParallelBuilds(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	ctx := context.Background()
	current, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	old := withoutTodoReady(composition.Clone(current.Receipt))
	old.Preset.Version = "sha256:6566fc2a89af6fb8661b00d2222d49125b9181153ef5a2e97888552221864a54"
	reopened, err := f.manager.ResumeComposition(old)
	if err != nil {
		t.Fatalf("pre-ready receipt does not reopen: %v", err)
	}
	if names := toolNamesOfSet(reopened.Toolset); slices.Contains(names, "todo_ready") || !slices.Contains(names, delegation.BuildToolName) {
		t.Fatalf("a pre-ready session's tools changed: %v", names)
	}
	project, err := f.store.RegisterProject(ctx, "Older chat", repo)
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.store.CreateProjectSessionWithComposition(ctx, project.ID, old)
	if err != nil {
		t.Fatal(err)
	}
	p := f.liveTurn(t, session)
	tree := f.taskTree(t, p)
	configure(t, f, clientFunc(func(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		return response(buildReport("Done.")), nil
	}))
	results, err := f.build(t, ctx, p, []delegation.Assignment{
		{Key: "one", Objective: "First", TaskID: string(tree.assigned.ID)}, {Key: "two", Objective: "Second", TaskID: string(tree.sibling.ID)},
	})
	if err != nil || len(results) != 2 || results[0].Status != "succeeded" || results[1].Status != "succeeded" {
		t.Fatalf("pre-ready parallel builds = %+v %v", results, err)
	}
}

// orchestration records what the scripted orchestrator saw.
type orchestration struct {
	mu      sync.Mutex
	ids     map[string]task.ID // title → Task
	results map[string]string  // tool call ID → result
}

func (o *orchestration) task(title string) task.ID {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.ids[title]
}

// workerFiles is the file each worker writes.
var workerFiles = map[string]string{"A": "pkg/a/a.go", "B": "pkg/b/b.go", "C": "pkg/c/c.go", "D": "pkg/a/extra.go"}

// worker plays one build worker, identified by its assignment: it claims
// its Task, writes its file, completes the Task and reports.
func (o *orchestration) worker(t *testing.T, r openrouter.ChatRequest) openrouter.ChatResponse {
	assignment := assignmentOf(r)
	title := ""
	for name := range workerFiles {
		if strings.Contains(assignment, "Implement "+name+"\n") {
			title = name
		}
	}
	id := o.task(title)
	switch {
	case title == "":
		return response("unknown assignment")
	case resultOf(r, "claim-"+title) == "":
		return call("claim-"+title, "todo_claim", map[string]string{"task_id": string(id), "idempotency_key": "claim-" + title})
	case resultOf(r, "write-"+title) == "":
		return call("write-"+title, "write_file", map[string]string{"path": workerFiles[title], "content": "package " + strings.ToLower(title) + "\n"})
	case resultOf(r, "get-"+title) == "":
		return call("get-"+title, "todo_get", map[string]string{"task_id": string(id)})
	case resultOf(r, "done-"+title) == "":
		return call("done-"+title, "todo_update", map[string]any{"task_id": string(id), "expected_revision": revisionIn(t, resultOf(r, "get-"+title)),
			"status": "completed", "result_summary": "Implemented " + title, "idempotency_key": "done-" + title})
	}
	return response(buildReport("Implemented " + title + "."))
}

func parsedReadiness(t *testing.T, content string) task.Readiness {
	t.Helper()
	var r task.Readiness
	if err := json.Unmarshal([]byte(content), &r); err != nil {
		t.Errorf("todo_ready result %q: %v", content, err)
	}
	return r
}

func parsedBuilds(t *testing.T, content string) []delegation.ParentResult {
	t.Helper()
	var results []delegation.ParentResult
	if err := json.Unmarshal([]byte(content), &results); err != nil {
		t.Errorf("delegate_build result %q: %v", content, err)
	}
	return results
}

// assignmentsFor is one batch as delegate_build assignments.
func assignmentsFor(batch []task.ReadyTask) []map[string]string {
	var out []map[string]string
	for _, item := range batch {
		out = append(out, map[string]string{"idempotency_key": "build-" + item.Title, "objective": "Implement " + item.Title + "\n", "task_id": string(item.TaskID)})
	}
	return out
}

// Stage 13 through the composed parent loop with a scripted orchestrator:
// it plans a four-Task tree (A and B independent, C depending on both, D
// overlapping A), reads todo_ready, is refused when it dispatches C early or
// A with D, dispatches the first batch (A and B) in parallel to build
// workers in their own worktrees, merges each through the Stage 12 gate
// with the owner's approval (B rebased onto the moved master), reads
// todo_ready again, dispatches C and D together, merges them, and finds the
// root completable. Conflicting Tasks never ran at once.
func TestComposedOrchestratorDispatchesATaskTreeInConflictFreeBatches(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	ownerCommit(t, repo, "ok.sh", "#!/bin/sh\necho CHECK-OK\n")
	ctx := context.Background()
	project, err := f.store.RegisterProject(ctx, "Orchestrated", repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SetProjectVerify(ctx, project.ID, 0, "", "sh ./ok.sh"); err != nil {
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
	root, err := f.store.CreateGlobalTask(ownerTasks(setupTurn), task.CreateInput{Title: "Ship the feature", IdempotencyKey: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseTurnLease(ctx, stored.ID, setupTurn.Lease.HolderID, setupTurn.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}

	o := &orchestration{ids: map[string]task.ID{}, results: map[string]string{}}
	steps := []string{"plan", "ready1", "early", "clash", "build1", "mergeA", "mergeB", "ready2", "build2", "mergeC", "mergeD", "ready3"}
	client := clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if !slices.Contains(toolNames(r), "todo_ready") {
			return o.worker(t, r), nil
		}
		next := ""
		for _, step := range steps {
			if resultOf(r, step) == "" {
				next = step
				break
			}
		}
		switch next {
		case "plan":
			return call("plan", "todo_decompose", map[string]any{"task_id": string(root.ID), "expected_revision": root.Revision, "idempotency_key": "plan",
				"children": []map[string]any{
					{"title": "A", "paths": []string{"pkg/a"}}, {"title": "B", "paths": []string{"pkg/b"}},
					{"title": "C", "paths": []string{"pkg/c"}, "depends_on_children": []int{0, 1}}, {"title": "D", "paths": []string{"pkg/a/extra.go"}},
				}}), nil
		case "ready1":
			var planned task.Decomposition
			if err := json.Unmarshal([]byte(resultOf(r, "plan")), &planned); err != nil || len(planned.Children) != 4 {
				return openrouter.ChatResponse{}, fmt.Errorf("plan result %q: %v", resultOf(r, "plan"), err)
			}
			o.mu.Lock()
			for _, child := range planned.Children {
				o.ids[child.Title] = child.ID
			}
			o.mu.Unlock()
			return call("ready1", "todo_ready", map[string]string{"tree_id": string(root.ID)}), nil
		case "early":
			return call("early", delegation.BuildToolName, map[string]any{"assignments": []map[string]string{{"idempotency_key": "early-C", "objective": "Implement C\n", "task_id": string(o.task("C"))}}}), nil
		case "clash":
			return call("clash", delegation.BuildToolName, map[string]any{"assignments": []map[string]string{
				{"idempotency_key": "clash-A", "objective": "Implement A\n", "task_id": string(o.task("A"))},
				{"idempotency_key": "clash-D", "objective": "Implement D\n", "task_id": string(o.task("D"))}}}), nil
		case "build1":
			ready := parsedReadiness(t, resultOf(r, "ready1"))
			if len(ready.Batches) == 0 {
				return openrouter.ChatResponse{}, errors.New("nothing ready")
			}
			return call("build1", delegation.BuildToolName, map[string]any{"assignments": assignmentsFor(ready.Batches[0])}), nil
		case "mergeA", "mergeB":
			built := parsedBuilds(t, resultOf(r, "build1"))
			i := map[string]int{"mergeA": 0, "mergeB": 1}[next]
			if len(built) != 2 {
				return openrouter.ChatResponse{}, fmt.Errorf("build1 = %q", resultOf(r, "build1"))
			}
			return call(next, delegation.MergeToolName, withChangeText(map[string]string{"execution_id": built[i].ExecutionID})), nil
		case "ready2", "ready3":
			return call(next, "todo_ready", map[string]string{"tree_id": string(root.ID)}), nil
		case "build2":
			ready := parsedReadiness(t, resultOf(r, "ready2"))
			if len(ready.Batches) == 0 {
				return openrouter.ChatResponse{}, fmt.Errorf("nothing ready after the first batch: %s", resultOf(r, "ready2"))
			}
			return call("build2", delegation.BuildToolName, map[string]any{"assignments": assignmentsFor(ready.Batches[0])}), nil
		case "mergeC", "mergeD":
			built := parsedBuilds(t, resultOf(r, "build2"))
			i := map[string]int{"mergeC": 0, "mergeD": 1}[next]
			if len(built) != 2 {
				return openrouter.ChatResponse{}, fmt.Errorf("build2 = %q", resultOf(r, "build2"))
			}
			return call(next, delegation.MergeToolName, withChangeText(map[string]string{"execution_id": built[i].ExecutionID})), nil
		}
		o.mu.Lock()
		for _, step := range steps {
			o.results[step] = resultOf(r, step)
		}
		o.mu.Unlock()
		return response("The feature is merged."), nil
	})
	configure(t, f, client)
	var cards []string
	approver := func(_ context.Context, name, _ string, preview *tools.FileChangePreview) tools.Decision {
		if name != delegation.MergeToolName {
			t.Errorf("approval asked for %s", name)
			return tools.Declined
		}
		cards = append(cards, preview.Path)
		return tools.Approved
	}
	session := agent.NewWithToolset(client, f.profile, f.store.BindHistory(stored.ID, "owner"), stored.ScopeContext(), f.store.BindTurnOwner(stored.ID, "owner"), resolved.Toolset)
	if err := session.Send(ctx, "Ship the feature with workers", &eventSink{}, approver); err != nil {
		t.Fatal(err)
	}
	results := o.results
	a, b, c, d := o.task("A"), o.task("B"), o.task("C"), o.task("D")

	ready1 := parsedReadiness(t, results["ready1"])
	if got := batchTaskIDs(ready1); len(got) != 2 || !slices.Equal(got[0], []task.ID{a, b}) || !slices.Equal(got[1], []task.ID{d}) {
		t.Fatalf("first read: batches %v", got)
	}
	if reason, _ := blockedReason(ready1, c); reason.Code != task.ReasonWaitingOnDependencies {
		t.Fatalf("first read: C = %+v", reason)
	}
	if !strings.Contains(results["early"], "depends on unfinished work") || !strings.Contains(results["clash"], "in this same delegate_build call") {
		t.Fatalf("the harness admitted premature or conflicting work: %q / %q", results["early"], results["clash"])
	}
	for _, step := range []string{"mergeA", "mergeB", "mergeC", "mergeD"} {
		if r := proposal(t, results[step]); r.Status != delegation.MergeMerged {
			t.Fatalf("%s = %+v", step, r)
		}
	}
	if !proposal(t, results["mergeB"]).Rebased {
		t.Fatal("B was not rebased onto the master A's merge moved")
	}
	if len(cards) != 4 {
		t.Fatalf("the owner approved %d merges", len(cards))
	}
	ready2 := parsedReadiness(t, results["ready2"])
	if got := batchTaskIDs(ready2); len(got) != 1 || !slices.Equal(got[0], []task.ID{c, d}) {
		t.Fatalf("second read: batches %v (blocked %+v)", got, ready2.Blocked)
	}
	ready3 := parsedReadiness(t, results["ready3"])
	if len(ready3.Batches) != 0 || len(ready3.Completable) != 1 || ready3.Completable[0].TaskID != root.ID || ready3.Counts[task.StatusCompleted] != 4 {
		t.Fatalf("final read = %s", results["ready3"])
	}
	for title, path := range workerFiles {
		if git(t, repo, "cat-file", "-t", "master:"+path) != "blob" {
			t.Fatalf("%s's file %s is not on master", title, path)
		}
	}
	// A and D conflict: D started only after A ended.
	turn := f.liveTurn(t, mustSession(t, f, stored.ID))
	execution := func(content string, i int) delegation.Attempt {
		built := parsedBuilds(t, content)
		got, err := f.store.InspectSubagent(ctx, turn, built[i].ExecutionID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	aRun, dRun := execution(results["build1"], 0), execution(results["build2"], 1)
	if aRun.Assignment.TaskID != string(a) || dRun.Assignment.TaskID != string(d) || !dRun.StartedAt.After(*aRun.EndedAt) {
		t.Fatalf("A ran %v–%v, D started %v", aRun.StartedAt, aRun.EndedAt, dRun.StartedAt)
	}
	for _, id := range []task.ID{a, b, c, d} {
		got, err := f.store.GetGlobalTask(ownerTasks(turn), id)
		if err != nil || got.Status != task.StatusCompleted {
			t.Fatalf("Task %s = %+v %v", id, got, err)
		}
	}
}

// Restart safety: after a crash, todo_ready reflects what recovery left. A
// claim of a turn that died counts as released, an interrupted build whose
// worktree was kept is offered for resumption on its branch, and its
// dependents still wait; a new turn resumes the tree from there.
func TestOrchestrationResumesATreeAfterACrash(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	p := f.buildParent(t, repo)
	tree := f.plannedTree(t, p, true)
	ctx := context.Background()
	// Another conversation of the project claimed E in a turn that died
	// (its lease expired, nobody released it), and A's build was running.
	other, err := f.store.CreateProjectSessionWithComposition(ctx, p.Scope.ProjectID, mustReceipt(t, f, p.Scope.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	dying, err := f.store.AcquireTurnLease(ctx, other.ID, "dying-turn", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	dyingTurn := delegation.Parent{Scope: other.ScopeContext(), Lease: dying}
	if _, err := f.store.ClaimGlobalTask(ownerTasks(dyingTurn), tree.e.ID, task.ClaimInput{IdempotencyKey: "claim-e"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	request := []delegation.Assignment{{Key: "a", Objective: "Implement A", TaskID: string(tree.a.ID)}}
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
	plan, err := worktree.Plan(ctx, repo, "", uuid.NewString(), "Implement A")
	if err != nil {
		t.Fatal(err)
	}
	attempts, err := f.store.AdmitBuildSubagents(ctx, invoked, request, []delegation.Worktree{plan}, []error{nil}, childReceipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	crashed := attempts[0]
	if _, started, err := f.store.StartSubagent(ctx, crashed.ID); err != nil || !started {
		t.Fatalf("start = %v %v", started, err)
	}
	if err := worktree.Ensure(ctx, *crashed.Worktree, true); err != nil {
		t.Fatal(err)
	}
	// Before recovery, A's build still reads as running.
	if r := readiness(t, f, p, tree.root.ID); len(r.InFlight) != 1 || r.InFlight[0].Build.Phase != task.BuildRunning {
		t.Fatalf("before the crash: %+v", r.InFlight)
	}
	// The process dies: the parent's turn is gone; recovery interrupts A.
	if err := f.store.ReleaseTurnLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if n, err := f.store.RecoverSubagents(ctx); err != nil || n != 1 {
		t.Fatalf("recovered %d: %v", n, err)
	}
	turn := f.liveTurn(t, mustSession(t, f, p.Scope.SessionID))
	r := readiness(t, f, turn, tree.root.ID)
	var resumeA, staleE *task.ReadyTask
	for _, batch := range r.Batches {
		for i := range batch {
			switch batch[i].TaskID {
			case tree.a.ID:
				resumeA = &batch[i]
			case tree.e.ID:
				staleE = &batch[i]
			}
		}
	}
	if resumeA == nil || resumeA.LastBuild == nil || resumeA.LastBuild.State != "interrupted" || resumeA.LastBuild.Phase != task.BuildResumable ||
		!strings.Contains(resumeA.Note, fmt.Sprintf("delegate_build with base %q", crashed.Worktree.Branch)) {
		t.Fatalf("A after the crash = %+v", resumeA)
	}
	if staleE == nil || !staleE.StaleClaim {
		t.Fatalf("E after the crash = %+v", staleE)
	}
	if reason, _ := blockedReason(r, tree.c.ID); reason.Code != task.ReasonWaitingOnDependencies {
		t.Fatalf("C after the crash = %+v", reason)
	}
	// The new turn resumes A on its kept branch; E's stale claim is
	// released when E is dispatched (alone: it declares no paths).
	configure(t, f, clientFunc(func(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		return response(buildReport("Resumed.")), nil
	}))
	resumed, err := f.build(t, ctx, turn, []delegation.Assignment{{Key: "a-resume", Objective: "Resume A", TaskID: string(tree.a.ID), Base: crashed.Worktree.Branch}})
	if err != nil || resumed[0].Status != "succeeded" || resumed[0].Build.Base != crashed.Worktree.Branch {
		t.Fatalf("resumed A = %+v %v", resumed, err)
	}
	e, err := f.build(t, ctx, turn, []delegation.Assignment{{Key: "e", Objective: "Implement E", TaskID: string(tree.e.ID)}})
	if err != nil || e[0].Status != "succeeded" {
		t.Fatalf("E after its stale claim = %+v %v", e, err)
	}
	if _, claimed, err := f.store.GetGlobalTaskClaim(ownerTasks(turn), tree.e.ID); err != nil || claimed {
		t.Fatalf("E's stale claim survived: %v %v", claimed, err)
	}
}
