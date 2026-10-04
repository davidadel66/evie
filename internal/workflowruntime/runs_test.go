package workflowruntime

import (
	"bytes"
	"compress/zlib"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
)

// runHarness is a real temporary SQLite file, reopened between steps as a
// restart would, and a real procedural Git repository.
type runHarness struct {
	*harness
	path string
	db   *sql.DB
}

func newRunHarness(t *testing.T) *runHarness {
	t.Helper()
	h := &runHarness{harness: &harness{t: t, repo: procedural.New(filepath.Join(t.TempDir(), "procedural"))}, path: filepath.Join(t.TempDir(), "evie.db")}
	h.reopen()
	t.Cleanup(func() { _ = h.db.Close() })
	return h
}

// reopen closes the database and opens the same file again with a new store
// and runtime.
func (h *runHarness) reopen() {
	h.t.Helper()
	if h.db != nil {
		if err := h.db.Close(); err != nil {
			h.t.Fatal(err)
		}
	}
	db, err := eviedb.OpenDBAt(h.path)
	if err != nil {
		h.t.Fatal(err)
	}
	h.db, h.store = db, eviedb.NewStore(db)
	h.rt = h.runtime()
}

var splitBillDir = filepath.Join(testdata, "split-bill")

func splitBillFixtures(t *testing.T) *workflow.Fixtures {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdata, "split-bill.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	return fixtures(t, string(data))
}

// approveSplitBill proposes and approves the split-bill example in a new
// Workspace.
func (h *runHarness) approveSplitBill(dir string) (memory.Workspace, eviedb.WorkflowActivation) {
	h.t.Helper()
	w := h.workspace()
	return w, h.approve(h.propose(w.ID, dir, splitBillFixtures(h.t)))
}

func billInputs(bill string, total, people int64) map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"bill_id":     json.RawMessage(fmt.Sprintf("%q", bill)),
		"total_cents": json.RawMessage(fmt.Sprint(total)),
		"people":      json.RawMessage(fmt.Sprint(people)),
	}
}

func (h *runHarness) start(ws memory.WorkspaceID, inputs map[string]json.RawMessage) RunResult {
	h.t.Helper()
	res, err := h.rt.StartRun(context.Background(), RunRequest{WorkspaceID: ws, Workflow: "split-bill", Inputs: inputs, Surface: "cli"})
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

func (h *runHarness) inspect(id string) eviedb.WorkflowRunDetail {
	h.t.Helper()
	d, err := h.rt.InspectRun(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return d
}

func (h *runHarness) runs(ws memory.WorkspaceID) []eviedb.WorkflowRun {
	h.t.Helper()
	runs, err := h.rt.ListRuns(context.Background(), RunFilter{WorkspaceID: ws})
	if err != nil {
		h.t.Fatal(err)
	}
	return runs
}

func decoded(t *testing.T, data json.RawMessage) any {
	t.Helper()
	v, err := workflow.DecodeValue(data)
	if err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return v
}

type attemptSummary struct {
	Step    string
	Attempt int
	State   eviedb.WorkflowAttemptState
	Next    string
	From    int64
}

func summarize(d eviedb.WorkflowRunDetail) []attemptSummary {
	out := []attemptSummary{}
	for _, a := range d.Attempts {
		out = append(out, attemptSummary{a.Step, a.Attempt, a.State, a.Next, a.InputCheckpoint})
	}
	return out
}

// checkOrdering asserts each attempt started from the checkpoint before it
// and each succeeded attempt produced the next checkpoint, in time order.
func checkOrdering(t *testing.T, d eviedb.WorkflowRunDetail) {
	t.Helper()
	for i, a := range d.Attempts {
		if a.State == eviedb.WorkflowAttemptRunning {
			continue
		}
		var next *eviedb.WorkflowCheckpoint
		for j := range d.Checkpoints {
			if d.Checkpoints[j].AfterAttempt == a.Seq {
				next = &d.Checkpoints[j]
			}
		}
		if a.State == eviedb.WorkflowAttemptFailed || a.State == eviedb.WorkflowAttemptInterrupted {
			if next != nil {
				t.Fatalf("failed attempt %d produced checkpoint %d", a.Seq, next.Seq)
			}
			continue
		}
		if next == nil || next.Seq != a.InputCheckpoint+1 || next.CreatedAt.Before(*a.FinishedAt) {
			t.Fatalf("attempt %d (%s) has no checkpoint right after it: %+v", a.Seq, a.Step, d.Checkpoints)
		}
		if i+1 < len(d.Attempts) && d.Attempts[i+1].StartedAt.Before(next.CreatedAt) {
			t.Fatalf("attempt %d started before checkpoint %d was recorded", d.Attempts[i+1].Seq, next.Seq)
		}
	}
}

// A run starts only through the activation: it reads the approved commit,
// pins the version, both hashes, the Workspace and access revisions and its
// Execution Composition, runs every deterministic and branch node with a
// checkpoint after each, and records its outputs.
func TestWorkflowRunExecutesDeterministicAndBranchStepsWithPins(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	w := h.workspace()
	if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowNotFound) {
		t.Fatalf("start before any proposal: %v", err)
	}
	card := h.propose(w.ID, splitBillDir, splitBillFixtures(t))
	if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowNotActive) {
		t.Fatalf("start before approval: %v", err)
	}
	a := h.approve(card)
	h.setAccess(w.ID, 0, "notebook:inbox")

	res := h.start(w.ID, billInputs("dinner-1", 1000, 3))
	if res.Existing || res.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("result = %+v", res)
	}
	h.reopen()
	d := h.inspect(res.Run.Run.ID)
	run := d.Run
	v, err := workflow.Validate(workflow.DirSource{Dir: splitBillDir}, workflow.Environment{Workers: ceiling})
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := v.AcceptInputs(billInputs("dinner-1", 1000, 3), nil)
	if run.WorkspaceID != w.ID || run.Workflow != "split-bill" || run.Version != a.Version || run.Commit != a.Commit || run.Path != a.Path ||
		run.ExecutableHash != a.ExecutableHash || run.AuthorityHash != a.AuthorityHash || run.WorkspaceRevision != w.CurrentRevisionID ||
		run.AccessRevision != 1 || run.RunKey != v.RunKey(inputs) || run.Surface != "cli" || run.StartedAt == nil || run.FinishedAt == nil {
		t.Fatalf("run = %+v", run)
	}
	if !reflect.DeepEqual(decoded(t, run.Inputs), inputs) {
		t.Fatalf("inputs = %s", run.Inputs)
	}
	var composition struct {
		Runtime   string   `json:"runtime"`
		Kinds     []string `json:"step_kinds"`
		Functions []string `json:"functions"`
	}
	if err := json.Unmarshal(run.Composition, &composition); err != nil || composition.Runtime != RuntimeVersion ||
		!reflect.DeepEqual(composition.Functions, []string{"split_evenly@1"}) || !reflect.DeepEqual(composition.Kinds, []string{"branch", "deterministic"}) {
		t.Fatalf("composition = %s, %v", run.Composition, err)
	}
	wantOutputs := map[string]any{"shares": []any{int64(334), int64(333), int64(333)}, "leftover_cents": int64(1)}
	if !reflect.DeepEqual(decoded(t, run.Outputs), wantOutputs) {
		t.Fatalf("outputs = %s", run.Outputs)
	}
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptSucceeded, "check", 0},
		{"check", 1, eviedb.WorkflowAttemptSucceeded, "leftover", 1},
		{"leftover", 1, eviedb.WorkflowAttemptSucceeded, "", 2},
	}
	if got := summarize(d); !reflect.DeepEqual(got, want) {
		t.Fatalf("attempts = %+v", got)
	}
	if len(d.Checkpoints) != 4 || d.Checkpoints[3].Next != "" || d.Checkpoints[2].Next != "leftover" {
		t.Fatalf("checkpoints = %+v", d.Checkpoints)
	}
	if !reflect.DeepEqual(decoded(t, d.Attempts[0].Input), map[string]any{"total": int64(1000), "parts": int64(3)}) {
		t.Fatalf("split input = %s", d.Attempts[0].Input)
	}
	if !strings.Contains(d.Attempts[1].Note, "default") || !strings.Contains(string(d.Attempts[1].Input), "$steps.split.output.remainder") {
		t.Fatalf("branch attempt = %+v", d.Attempts[1])
	}
	checkOrdering(t, d)

	// The exact split takes the other branch.
	even := h.start(w.ID, billInputs("dinner-2", 900, 3))
	d = h.inspect(even.Run.Run.ID)
	if got := summarize(d); got[1].Next != "per_person" || got[2].Step != "per_person" {
		t.Fatalf("even attempts = %+v", got)
	}
	if !reflect.DeepEqual(decoded(t, d.Run.Outputs), map[string]any{"shares": []any{int64(300), int64(300), int64(300)}, "per_person": int64(300)}) {
		t.Fatalf("even outputs = %s", d.Run.Outputs)
	}
}

// Before a node runs, its attempt is durable with its input checkpoint and
// attempt number; its output and the next checkpoint are durable before the
// next node starts. A second connection observes this from inside each
// function call.
func TestWorkflowRunRecordsEachAttemptBeforeItRunsAndCheckpointsBeforeTheNext(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	observer, err := eviedb.OpenDBAt(h.path)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	view := eviedb.NewStore(observer)
	var seen []string
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		runs, err := view.ListWorkflowRuns(ctx, eviedb.WorkflowRunFilter{WorkspaceID: w.ID})
		if err != nil || len(runs) != 1 {
			t.Fatalf("observer runs = %+v, %v", runs, err)
		}
		p, err := view.WorkflowRunProgress(ctx, runs[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Run.State != eviedb.WorkflowRunRunning || p.InFlight == nil || p.InFlight.Step != step || p.InFlight.InputCheckpoint != p.Checkpoint.Seq || p.Checkpoint.Next != step {
			t.Fatalf("calling %s before its attempt was durable: %+v", step, p)
		}
		var recorded map[string]any
		if err := json.Unmarshal(p.InFlight.Input, &recorded); err != nil || len(recorded) != len(input) {
			t.Fatalf("recorded input %s for %v", p.InFlight.Input, input)
		}
		seen = append(seen, fmt.Sprintf("%s@%d/attempt %d", step, p.Checkpoint.Seq, p.InFlight.Attempt))
		return v.CallFunction(step, input)
	}
	h.start(w.ID, billInputs("dinner-1", 1000, 3))
	// leftover starts from checkpoint 2: after split's output (1) and the
	// branch decision (2) were both durable.
	if want := []string{"split@0/attempt 1", "leftover@2/attempt 1"}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("observed %v, want %v", seen, want)
	}
}

// One Run Key, one logical execution: a duplicate start returns the
// existing run (active or succeeded) and starts nothing; a failed run leaves
// its key free; other keys run independently.
func TestWorkflowRunKeyDeduplicatesStarts(t *testing.T) {
	h := newRunHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	first := h.start(w.ID, billInputs("dinner-1", 1000, 3))
	h.reopen()
	again := h.start(w.ID, billInputs("dinner-1", 5000, 4))
	if !again.Existing || again.Run.Run.ID != first.Run.Run.ID || again.Run.Run.State != eviedb.WorkflowRunSucceeded || len(again.Run.Attempts) != 3 {
		t.Fatalf("duplicate start = %+v", again)
	}
	failed := h.start(w.ID, billInputs("dinner-2", 1000, 5000))
	if failed.Run.Run.State != eviedb.WorkflowRunFailed {
		t.Fatalf("failed run = %+v", failed.Run.Run)
	}
	retried := h.start(w.ID, billInputs("dinner-2", 1000, 2))
	if retried.Existing || retried.Run.Run.ID == failed.Run.Run.ID || retried.Run.Run.State != eviedb.WorkflowRunSucceeded || retried.Run.Run.RunKey != failed.Run.Run.RunKey {
		t.Fatalf("start after a failed run = %+v", retried.Run.Run)
	}
	if runs := h.runs(w.ID); len(runs) != 3 {
		t.Fatalf("runs = %+v", runs)
	}
}

// Inputs are validated against the declared inputs before anything is
// recorded; command-line text converts only to the declared type.
func TestWorkflowRunInputsAreValidatedBeforeAnythingIsRecorded(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	for name, req := range map[string]RunRequest{
		"missing":  {Inputs: map[string]json.RawMessage{"bill_id": json.RawMessage(`"b"`)}},
		"type":     {Inputs: map[string]json.RawMessage{"bill_id": json.RawMessage(`"b"`), "total_cents": json.RawMessage(`"10"`), "people": json.RawMessage(`2`)}},
		"unknown":  {Inputs: billInputs("b", 10, 2), TextInputs: map[string]string{"tip": "1"}},
		"twice":    {Inputs: billInputs("b", 10, 2), TextInputs: map[string]string{"people": "2"}},
		"text":     {TextInputs: map[string]string{"bill_id": "b", "total_cents": "1e3", "people": "2"}},
		"negative": {Inputs: billInputs("b", -5, 2)},
	} {
		req.WorkspaceID, req.Workflow, req.Surface = w.ID, "split-bill", "cli"
		if _, err := h.rt.StartRun(ctx, req); !errors.Is(err, ErrRunInput) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("invalid starts recorded runs: %+v", runs)
	}
	res, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", TextInputs: map[string]string{"bill_id": "b", "total_cents": "10", "people": "2"}, Surface: "web"})
	if err != nil || res.Run.Run.State != eviedb.WorkflowRunSucceeded || res.Run.Run.Surface != "web" {
		t.Fatalf("text inputs = %+v, %v", res, err)
	}
	for _, surface := range []string{"chat", "model", ""} {
		if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("c", 1, 1), Surface: surface}); !errors.Is(err, eviedb.ErrWorkflowRunSurface) {
			t.Fatalf("surface %q: %v", surface, err)
		}
	}
}

// Deterministic steps retry within their Retry Policy from the same input
// checkpoint; the run fails after the last attempt, and the run's
// limits.max_attempts bounds every attempt.
func TestWorkflowRunRetriesDeterministicStepsWithinBounds(t *testing.T) {
	h := newRunHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	failed := h.start(w.ID, billInputs("too-many", 1000, 5000))
	d := h.inspect(failed.Run.Run.ID)
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptFailed, "", 0},
		{"split", 2, eviedb.WorkflowAttemptFailed, "", 0},
		{"split", 3, eviedb.WorkflowAttemptFailed, "", 0},
	}
	if got := summarize(d); !reflect.DeepEqual(got, want) {
		t.Fatalf("attempts = %+v", got)
	}
	if d.Run.State != eviedb.WorkflowRunFailed || d.Run.Error != "steps.split failed after 3 attempts: parts must be between 1 and 1000" || len(d.Checkpoints) != 1 || d.Run.Outputs != nil {
		t.Fatalf("run = %+v checkpoints %+v", d.Run, d.Checkpoints)
	}

	// A transient failure is retried with the same recorded input.
	calls := 0
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("transient")
		}
		return v.CallFunction(step, input)
	}
	ok := h.start(w.ID, billInputs("flaky", 1000, 3))
	d = h.inspect(ok.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded || d.Attempts[0].Error != "transient" || d.Attempts[1].Step != "split" || d.Attempts[1].Attempt != 2 ||
		d.Attempts[1].InputCheckpoint != 0 || string(d.Attempts[0].Input) != string(d.Attempts[1].Input) {
		t.Fatalf("retried run = %+v", d)
	}
	checkOrdering(t, d)

	// The run's attempt limit stops retries the step would still allow.
	limited := copyDefinition(t, "split-bill")
	writeFile(t, filepath.Join(limited, "workflow.yaml"), strings.Replace(readFile(t, filepath.Join(limited, "workflow.yaml")), "max_attempts: 10", "max_attempts: 2", 1))
	h.approve(h.propose(w.ID, limited, splitBillFixtures(t)))
	h.rt.call = nil
	capped := h.start(w.ID, billInputs("capped", 1000, 5000))
	d = h.inspect(capped.Run.Run.ID)
	if len(d.Attempts) != 2 || d.Run.State != eviedb.WorkflowRunFailed || !strings.Contains(d.Run.Error, "limits.max_attempts 2") || d.Run.Version != 2 {
		t.Fatalf("capped run = %+v attempts %+v", d.Run, summarize(d))
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A definition with steps this runtime cannot execute yet is refused at
// start, before anything is recorded; it is never partly executed.
func TestWorkflowRunRefusesStepKindsThisRuntimeCannotRunYet(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	notes := h.workspace("notebook:inbox", "notebook:digest")
	h.approve(h.propose(notes.ID, filepath.Join(testdata, "notes-digest"), fixtures(t, notesFixtures)))
	_, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: notes.ID, Workflow: "notes-digest", TextInputs: map[string]string{"day": "2026-10-05"}, Surface: "cli"})
	// Capability reads run since sub-stage 5; its effect (notes.append)
	// waits for the effect protocol.
	var unsupported *UnsupportedStepsError
	if !errors.Is(err, ErrUnsupportedSteps) || !errors.As(err, &unsupported) || !reflect.DeepEqual(unsupported.Kinds, []string{"ai", "capability effect", "interrupt"}) ||
		!strings.Contains(err.Error(), "not yet supported in this runtime version") {
		t.Fatalf("notes-digest: %v", err)
	}
	research := h.workspace()
	if _, err := h.store.SetWorkspaceResearch(ctx, research.ID, research.CurrentRevisionID, true); err != nil {
		t.Fatal(err)
	}
	h.approve(h.propose(research.ID, filepath.Join(testdata, "deep-research"), deepResearchFixtures(t)))
	_, err = h.rt.StartRun(ctx, RunRequest{WorkspaceID: research.ID, Workflow: "deep-research", TextInputs: map[string]string{"question": "Is it?"}, Surface: "cli"})
	if !errors.As(err, &unsupported) || !reflect.DeepEqual(unsupported.Kinds, []string{"agent", "ai", "map"}) {
		t.Fatalf("deep-research: %v", err)
	}
	if runs, err := h.rt.ListRuns(ctx, RunFilter{}); err != nil || len(runs) != 0 {
		t.Fatalf("refused starts recorded runs: %+v, %v", runs, err)
	}
}

// Deactivation and blocks are applied when each run starts.
func TestWorkflowRunRefusedWhenDeactivatedOrBlocked(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	w, a := h.approveSplitBill(splitBillDir)
	if _, err := h.rt.Deactivate(ctx, Deactivation{WorkspaceID: w.ID, Name: a.Name, Version: a.Version, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("b", 1, 1), Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowNotActive) {
		t.Fatalf("deactivated: %v", err)
	}
	archived, _ := h.approveSplitBill(splitBillDir)
	if _, err := h.store.ArchiveWorkspace(ctx, archived.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: archived.ID, Workflow: "split-bill", Inputs: billInputs("b", 1, 1), Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowBlocked) {
		t.Fatalf("archived Workspace: %v", err)
	}
	if runs := h.runs(""); len(runs) != 0 {
		t.Fatalf("runs = %+v", runs)
	}
}

// tamperBlob rewrites the loose object of path at commit in the bare
// procedural repository, as damage or an edit outside Evie would.
func tamperBlob(t *testing.T, root, commit, path string, content []byte) {
	t.Helper()
	out, err := exec.Command("git", "--git-dir="+root, "rev-parse", commit+":"+path).Output()
	if err != nil {
		t.Fatal(err)
	}
	blob := strings.TrimSpace(string(out))
	object := filepath.Join(root, "objects", blob[:2], blob[2:])
	var buf bytes.Buffer
	z := zlib.NewWriter(&buf)
	fmt.Fprintf(z, "blob %d\x00", len(content))
	z.Write(content)
	z.Close()
	if err := os.Chmod(object, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(object, buf.Bytes(), 0o444); err != nil {
		t.Fatal(err)
	}
}

// Each start re-reads the approved commit through procedural Git and
// re-hashes it: content that no longer hashes to the approval is refused.
func TestWorkflowRunRefusesADefinitionThatNoLongerMatchesItsApproval(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	w, a := h.approveSplitBill(splitBillDir)
	source := readFile(t, filepath.Join(splitBillDir, "workflow.yaml"))
	tamperBlob(t, h.repo.Root(), a.Commit, a.Path+"/workflow.yaml", []byte(strings.Replace(source, "max_attempts: 3", "max_attempts: 5", 1)))
	_, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("b", 1, 1), Surface: "cli"})
	if !errors.Is(err, ErrDefinitionMismatch) || !strings.Contains(err.Error(), a.ExecutableHash) {
		t.Fatalf("tampered definition: %v", err)
	}
	tamperBlob(t, h.repo.Root(), a.Commit, a.Path+"/workflow.yaml", []byte("schema_version: 1\nname: split-bill\n"))
	if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("b", 1, 1), Surface: "cli"}); !errors.Is(err, ErrDefinitionMismatch) {
		t.Fatalf("invalid definition at the approved commit: %v", err)
	}
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("runs = %+v", runs)
	}
}

// Runs are listed newest first, by Workspace and workflow, and inspected by
// ID with every attempt and checkpoint.
func TestWorkflowRunsAreListedAndInspected(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	other, _ := h.approveSplitBill(splitBillDir)
	first := h.start(w.ID, billInputs("a", 10, 3))
	second := h.start(w.ID, billInputs("b", 10, 2))
	h.start(other.ID, billInputs("a", 10, 2))
	h.reopen()
	runs := h.runs(w.ID)
	if len(runs) != 2 || runs[0].ID != second.Run.Run.ID || runs[1].ID != first.Run.Run.ID {
		t.Fatalf("runs = %+v", runs)
	}
	if all, err := h.rt.ListRuns(ctx, RunFilter{Workflow: "split-bill"}); err != nil || len(all) != 3 {
		t.Fatalf("all = %d, %v", len(all), err)
	}
	if none, err := h.rt.ListRuns(ctx, RunFilter{WorkspaceID: w.ID, Workflow: "notes-digest"}); err != nil || len(none) != 0 {
		t.Fatalf("other workflow = %+v, %v", none, err)
	}
	if _, err := h.rt.InspectRun(ctx, "missing"); !errors.Is(err, eviedb.ErrWorkflowRunNotFound) {
		t.Fatalf("missing run: %v", err)
	}
	text := RunText(h.inspect(first.Run.Run.ID))
	for _, want := range []string{"split-bill version 1", "succeeded", "Run key: split-bill:sha256:", "split attempt 1: succeeded", "check attempt 1: succeeded -> leftover", "Outputs", "leftover_cents: 1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text lacks %q:\n%s", want, text)
		}
	}
}
