package workflowruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/task"
)

// Explore and build agent steps (Stage 14 sub-stage 9): an explore worker
// reads only inside the run's Workspace folder; a build worker writes only
// in its own git worktree and branch in the Workspace repository, on the
// Task the step names, and the harness commits its work on its branch;
// nothing is merged. A build worker interrupted by a crash is continued in
// its kept worktree, on its own child session, under the new attempt's
// authority.

// localStep is a definition with one agent step of worker; a build step
// takes its Task from the task_id input.
func localStep(worker string) string {
	inputs, stepInput := "", "      note: $inputs.note\n"
	if worker == "build" {
		inputs = "  task_id:\n    type: string\n    required: true\n"
		stepInput += "      task_id: $inputs.task_id\n"
	}
	return fmt.Sprintf(`schema_version: 1
name: local-%[1]s
description: One %[1]s worker.
inputs:
  note:
    type: string
    required: true
%[2]srun_key:
  inputs: [note]
model_policies:
  worker:
    version: 1
    provider: openrouter
    model: deepseek/deepseek-v4.1-flash
    temperature: 0
    max_output_tokens: 4000
limits:
  max_duration: 1h
  max_attempts: 10
  max_parallel: 1
  max_tokens: 1000000
retention:
  classification: ordinary
start: work
steps:
  - id: work
    kind: agent
    worker: %[1]s
    model_policy: worker
    objective: prompts/work.md
    input:
%[3]s    output_schema: schemas/finding.schema.json
    budget:
      time: 5m
      tokens: 200000
outputs:
  answer: $steps.work.output.result.answer
`, worker, inputs, stepInput)
}

func localDefinition(t *testing.T, worker string) string {
	return writeDefinition(t, "local-"+worker, localStep(worker), map[string]string{
		"prompts/work.md": "Do the local work the note in the assignment data asks for.\n", "schemas/finding.schema.json": findingSchema})
}

func localFixtures(worker string) string {
	inputs := `"note": "n"`
	if worker == "build" {
		inputs += `, "task_id": "t"`
	}
	return `{"inputs": {` + inputs + `}, "steps": {"work": {"output": {"status": "succeeded", "summary": "s", "sources": [], "result": {"answer": "x"}}, "tokens": 1000}}}`
}

// toolResultOf is the content of the request's last tool result.
func lastToolResult(r openrouter.ChatRequest) string {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == "tool" {
			return r.Messages[i].Content
		}
	}
	return ""
}

func gitIn(t *testing.T, dir string, args ...string) string {
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

// ownerRepo is the owner's checkout: one commit on master and an
// uncommitted edit no worker may see or touch.
func ownerRepo(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "README.md"), "# Repo\n")
	gitIn(t, repo, "init", "-q", "-b", "master")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "first")
	writeFile(t, filepath.Join(repo, "README.md"), "# Repo\nowner's uncommitted edit\n")
	return repo
}

// folderWorkspace is a Workspace with a local folder; build is turned on
// when asked.
func (h *runHarness) folderWorkspace(folder string, build bool) memory.Workspace {
	h.t.Helper()
	ctx := context.Background()
	w := h.workspace()
	if _, err := h.store.SetWorkspaceFolder(ctx, w.ID, 0, folder); err != nil {
		h.t.Fatal(err)
	}
	if build {
		current, err := h.workspaceNow(w.ID)
		if err != nil {
			h.t.Fatal(err)
		}
		if _, err := h.store.SetWorkspaceBuild(ctx, w.ID, current.CurrentRevisionID, memory.WorkspaceBuildOn); err != nil {
			h.t.Fatal(err)
		}
	}
	current, err := h.workspaceNow(w.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return current
}

func (h *runHarness) workspaceNow(id memory.WorkspaceID) (memory.Workspace, error) {
	all, err := h.store.ListWorkspaces(context.Background(), true)
	if err != nil {
		return memory.Workspace{}, err
	}
	for _, w := range all {
		if w.ID == id {
			return w, nil
		}
	}
	return memory.Workspace{}, fmt.Errorf("no Workspace %s", id)
}

// An explore worker reads inside the run's Workspace folder and nowhere
// else: a read outside it is refused to the worker, and its result is the
// step's output.
func TestWorkflowAgentExploreWorkerReadsOnlyTheWorkspaceFolder(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(base, "notes")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(folder, "notes.txt"), "the answer is 42\n")
	writeFile(t, filepath.Join(base, "outside.txt"), "outside secret\n")
	model := &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		switch toolResults(r) {
		case 0:
			return toolCall("read_file", map[string]string{"path": "../outside.txt"}), nil
		case 1:
			return toolCall("read_file", map[string]string{"path": "notes.txt"}), nil
		}
		answer := "unknown"
		if strings.Contains(lastToolResult(r), "the answer is 42") {
			answer = "42"
		}
		return finalReport("Read notes.txt.", map[string]any{"answer": answer}), nil
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.folderWorkspace(folder, false)
	if !w.Explore.Allowed {
		t.Fatalf("explore = %+v", w.Explore)
	}
	h.approve(h.propose(w.ID, localDefinition(t, "explore"), fixtures(t, localFixtures("explore"))))
	res, err := h.startRun(h.rt, w.ID, "local-explore", map[string]string{"note": "find the answer"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	outputs, _ := decodeObject(d.Run.Outputs)
	if d.Run.State != eviedb.WorkflowRunSucceeded || outputs["answer"] != "42" {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	requests := model.recorded()
	refused := lastToolResult(requests[1])
	if strings.Contains(refused, "outside secret") || refused == "" {
		t.Fatalf("a read outside the folder returned %q", refused)
	}
	var names []string
	for _, tool := range requests[0].Tools {
		names = append(names, tool.Function.Name)
	}
	if strings.Join(names, ",") != "read_file,query_db,bash" {
		t.Fatalf("explore tools = %v", names)
	}
	if exec, _, err := h.store.RunWorkerExecution(context.Background(), d.Run.ID, d.Workers[0].ExecutionID); err != nil || exec.Root != folder {
		t.Fatalf("explore root = %q %v", exec.Root, err)
	}
}

// buildTask is a Task in the global scope for a run's build worker.
func (h *runHarness) buildTask(title string) task.Task {
	h.t.Helper()
	ctx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{ActorID: string(memory.LocalOwnerID), SessionID: "owner-cli", RunID: "test"})
	created, err := h.store.CreateGlobalTask(ctx, task.CreateInput{Title: title, IdempotencyKey: task.IdempotencyKey(strings.ReplaceAll(title, " ", "-"))})
	if err != nil {
		h.t.Fatal(err)
	}
	return created
}

// A build worker writes only in its own worktree and branch on its Task;
// the harness commits its work there and nothing is merged: the owner's
// checkout and master are untouched and the branch waits for the Stage 12
// gate. Its Task Access Grant ends with it.
func TestWorkflowAgentBuildWorkerCommitsOnItsBranchAndNothingMerges(t *testing.T) {
	ctx := context.Background()
	repo := ownerRepo(t)
	model := &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if toolResults(r) == 0 {
			return toolCall("write_file", map[string]string{"path": "hello.txt", "content": "hello\n"}), nil
		}
		return finalReport("Added hello.txt.", map[string]any{"answer": "added"}), nil
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.folderWorkspace(repo, true)
	h.approve(h.propose(w.ID, localDefinition(t, "build"), fixtures(t, localFixtures("build"))))
	master := gitIn(t, repo, "rev-parse", "master")
	work := h.buildTask("Add hello")
	res, err := h.startRun(h.rt, w.ID, "local-build", map[string]string{"note": "add hello.txt", "task_id": string(work.ID)})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	exec, _, err := h.store.RunWorkerExecution(ctx, d.Run.ID, d.Workers[0].ExecutionID)
	if err != nil || exec.Worktree == nil || exec.Result == nil || exec.Result.Build == nil || len(exec.Result.Build.Commits) != 1 {
		t.Fatalf("build execution = %+v %v", exec, err)
	}
	branch := exec.Worktree.Branch
	if files := gitIn(t, repo, "show", "--name-only", "--format=", branch); files != "hello.txt" {
		t.Fatalf("branch %s changed %q", branch, files)
	}
	if now := gitIn(t, repo, "rev-parse", "master"); now != master {
		t.Fatal("master moved: a workflow merged")
	}
	if _, err := os.Stat(filepath.Join(repo, "hello.txt")); !os.IsNotExist(err) {
		t.Fatal("the owner's checkout changed")
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "README.md")); !strings.Contains(string(data), "owner's uncommitted edit") {
		t.Fatal("the owner's uncommitted edit changed")
	}
	if exec.Gate != nil && exec.Gate.Merged != nil {
		t.Fatal("a workflow's build was merged")
	}
	var open int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM task_access_grants WHERE grantee_session_id=? AND ended_at IS NULL`, exec.Child.ID).Scan(&open); err != nil || open != 0 {
		t.Fatalf("open grants = %d %v", open, err)
	}
}

// A build worker cut short by a crash keeps its worktree; the attempt that
// runs again continues that worker in its worktree, on its own child
// session and branch, rather than starting fresh, and the harness commits
// both attempts' work on the one branch.
func TestWorkflowAgentInterruptedBuildContinuesInItsKeptWorktree(t *testing.T) {
	ctx := context.Background()
	repo := ownerRepo(t)
	var dead *process
	var once sync.Once
	died := make(chan struct{})
	model := &workerModel{answer: func(ctx context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		resumed := strings.Contains(r.Messages[len(r.Messages)-1].Content, "resumes this assignment") || strings.Contains(assignmentOfAny(r), "resumes this assignment")
		switch {
		case !resumed && toolResults(r) == 0:
			return toolCall("write_file", map[string]string{"path": "first.txt", "content": "first\n"}), nil
		case !resumed:
			first := false
			once.Do(func() { first = true })
			if first && dead != nil {
				_ = dead.db.Close()
				close(died)
				<-ctx.Done()
				return openrouter.ChatResponse{}, ctx.Err()
			}
		case toolResults(r) <= 1:
			return toolCall("write_file", map[string]string{"path": "second.txt", "content": "second\n"}), nil
		}
		return finalReport("Wrote both files.", map[string]any{"answer": "both"}), nil
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.folderWorkspace(repo, true)
	h.approve(h.propose(w.ID, localDefinition(t, "build"), fixtures(t, localFixtures("build"))))
	work := h.buildTask("Write two files")
	dead = h.process()
	h.advanceOnSleep(dead.rt)
	go func() {
		_, _ = h.startRun(dead.rt, w.ID, "local-build", map[string]string{"note": "write two files", "task_id": string(work.ID)})
	}()
	<-died
	id := h.runs(w.ID)[0].ID
	h.clock.Advance(testLease + 1e9)
	next := h.process()
	if _, err := next.rt.execute(ctx, id); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Workers) != 2 {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	first, _, err := h.store.RunWorkerExecution(ctx, id, d.Workers[0].ExecutionID)
	if err != nil || first.State != "interrupted" || first.Worktree == nil {
		t.Fatalf("interrupted build = %+v %v", first, err)
	}
	second, _, err := h.store.RunWorkerExecution(ctx, id, d.Workers[1].ExecutionID)
	if err != nil || second.Continues == nil || second.Continues.ExecutionID != first.ID || second.Child.ID != first.Child.ID ||
		second.Worktree == nil || second.Worktree.Path != first.Worktree.Path || d.Workers[1].Continues != first.ID {
		t.Fatalf("continuation = %+v %v", second, err)
	}
	files := gitIn(t, repo, "diff", "--name-only", first.Worktree.BaseCommit, first.Worktree.Branch)
	if files != "first.txt\nsecond.txt" {
		t.Fatalf("branch files = %q", files)
	}
}

// assignmentOfAny is the text of every user message.
func assignmentOfAny(r openrouter.ChatRequest) string {
	var b strings.Builder
	for _, m := range r.Messages {
		if m.Role == "user" {
			b.WriteString(m.Content)
		}
	}
	return b.String()
}
