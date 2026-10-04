package workflowruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
)

// Durable restart and leases (Stage 14 sub-stage 4). Every test uses a real
// temporary SQLite file; a "process" is its own connection pool, store and
// runtime over that file, with its own lease holder. A crash is a process
// that stops inside a node and never writes again: its node call never
// returns and its database handle is closed, so it neither renews nor
// releases its lease.

// testClock is the store's and runtimes' clock, so lease expiry, retry
// backoff and pause rechecks happen exactly when a test advances it.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// testLease is the lease every lease test uses: long in wall-clock terms
// (heartbeats every ten seconds never fire during a test), and expired only
// when the test clock moves past it.
const testLease = 30 * time.Second

func newLeaseHarness(t *testing.T) *runHarness {
	t.Helper()
	h := &runHarness{harness: &harness{t: t, repo: procedural.New(filepath.Join(t.TempDir(), "procedural")),
		clock:     &testClock{now: time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)},
		scheduler: SchedulerConfig{Workers: 2, Lease: testLease, Poll: 2 * time.Millisecond, Recheck: time.Minute}},
		path: filepath.Join(t.TempDir(), "evie.db")}
	h.reopen()
	t.Cleanup(func() { _ = h.db.Close() })
	return h
}

// process is one Evie process over the harness's database file.
type process struct {
	db    *sql.DB
	store *eviedb.Store
	rt    *Runtime
}

func (h *runHarness) process() *process {
	h.t.Helper()
	db, err := eviedb.OpenDBAt(h.path)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = db.Close() })
	store := eviedb.NewStore(db)
	if h.clock != nil {
		store.UseClock(h.clock.Now, time.UTC)
	}
	if h.providers == nil {
		h.providers = ContractsOnly(notesCatalog())
	}
	rt, err := New(Config{Store: store, Repo: h.repo, Capabilities: h.providers, Workers: ceiling, Scheduler: h.scheduler, Models: h.models, ModelsOff: h.modelsOff,
		Agents: h.agentsOf(store)})
	if err != nil {
		h.t.Fatal(err)
	}
	if h.clock != nil {
		rt.now = h.clock.Now
	}
	return &process{db: db, store: store, rt: rt}
}

// crashInside makes p die inside step: the first call of it never returns
// and p's database is closed, so p never writes, renews or releases again.
// The returned channel is closed once p is dead.
func (h *runHarness) crashInside(p *process, step string) <-chan struct{} {
	h.t.Helper()
	dead, forever := make(chan struct{}), make(chan struct{})
	var once sync.Once
	p.rt.call = func(_ string, v *workflow.Validated, s string, input map[string]any) (any, error) {
		if s == step {
			once.Do(func() {
				_ = p.db.Close()
				close(dead)
			})
			<-forever
		}
		return v.CallFunction(s, input)
	}
	h.t.Cleanup(func() { close(forever) })
	return dead
}

// eventually polls cond until it holds or ten seconds pass.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (h *runHarness) progress(id string) eviedb.WorkflowRunProgress {
	h.t.Helper()
	p, err := h.store.WorkflowRunProgress(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return p
}

func (h *runHarness) state(id string) eviedb.WorkflowRunState { return h.progress(id).Run.State }

func (h *runHarness) detach(ws memory.WorkspaceID, inputs map[string]json.RawMessage) eviedb.WorkflowRun {
	h.t.Helper()
	res, err := h.rt.StartRun(context.Background(), RunRequest{WorkspaceID: ws, Workflow: "split-bill", Inputs: inputs, Surface: "cli", Detach: true})
	if err != nil {
		h.t.Fatal(err)
	}
	return res.Run.Run
}

func kinds(events []eviedb.WorkflowRunEvent) []eviedb.WorkflowRunEventKind {
	out := []eviedb.WorkflowRunEventKind{}
	for _, e := range events {
		out = append(out, e.Kind)
	}
	return out
}

var dinnerOutputs = map[string]any{"shares": []any{int64(334), int64(333), int64(333)}, "leftover_cents": int64(1)}

// Two runtimes over one database never both advance one run: with both
// schedulers polling the same runs, every node runs once, by the holder
// of the run's lease, and no run is ever inside two nodes at once.
func TestWorkflowRunsAreAdvancedByOneRuntimeAtATime(t *testing.T) {
	h := newLeaseHarness(t)
	h.scheduler.Workers = 3
	w, _ := h.approveSplitBill(splitBillDir)
	a, b := h.process(), h.process()
	var mu sync.Mutex
	inside := map[string]bool{}
	calls := map[string][]string{}
	var overlap []string
	observe := func(holder string) func(string, *workflow.Validated, string, map[string]any) (any, error) {
		return func(run string, v *workflow.Validated, step string, input map[string]any) (any, error) {
			mu.Lock()
			if inside[run] {
				overlap = append(overlap, run+" "+step)
			}
			inside[run] = true
			calls[run] = append(calls[run], step+"@"+holder)
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			inside[run] = false
			mu.Unlock()
			return v.CallFunction(step, input)
		}
	}
	a.rt.call, b.rt.call = observe(a.rt.Holder()), observe(b.rt.Holder())
	ctx := context.Background()
	stopA, stopB := a.rt.Start(ctx), b.rt.Start(ctx)
	var ids []string
	for i := range 8 {
		ids = append(ids, h.detach(w.ID, billInputs(fmt.Sprintf("dinner-%d", i), int64(1000+i), 3)).ID)
	}
	for _, id := range ids {
		eventually(t, "run "+id+" to finish", func() bool { return h.state(id).Terminal() })
	}
	if err := errors.Join(stopA(), stopB()); err != nil {
		t.Fatal(err)
	}
	if len(overlap) > 0 {
		t.Fatalf("a run was inside two nodes at once: %v", overlap)
	}
	for _, id := range ids {
		d := h.inspect(id)
		if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Attempts) != 3 || len(d.Events) != 0 {
			t.Fatalf("run %s = %+v attempts %+v events %+v", id, d.Run, summarize(d), d.Events)
		}
		for _, at := range d.Attempts {
			if at.State != eviedb.WorkflowAttemptSucceeded || (at.LeaseHolder != a.rt.Holder() && at.LeaseHolder != b.rt.Holder()) {
				t.Fatalf("run %s attempt %+v", id, at)
			}
		}
		if len(calls[id]) != 2 {
			t.Fatalf("run %s called its nodes %v", id, calls[id])
		}
	}
}

// A holder whose lease expired while it was inside a node writes nothing
// after another runtime takes over: its late output is refused, the new
// holder records its attempt interrupted and retries it from the same
// checkpoint, and the run's output is the uninterrupted one.
func TestWorkflowRunStaleHolderCannotWriteAfterTakeover(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	a, b := h.process(), h.process()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	a.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "split" {
			once.Do(func() { close(entered) })
			<-release
		}
		return v.CallFunction(step, input)
	}
	type result struct {
		res RunResult
		err error
	}
	done := make(chan result, 1)
	go func() {
		res, err := a.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"})
		done <- result{res, err}
	}()
	<-entered
	id := h.runs(w.ID)[0].ID
	h.clock.Advance(testLease + time.Second)
	stop := b.rt.Start(ctx)
	eventually(t, "the run to finish under the new holder", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	close(release)
	got := <-done
	if got.err != nil || got.res.Run.Run.ID != id || got.res.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("the stale holder's start = %+v, %v", got.res.Run.Run, got.err)
	}
	d := h.inspect(id)
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptInterrupted, "", 0},
		{"split", 2, eviedb.WorkflowAttemptSucceeded, "check", 0},
		{"check", 1, eviedb.WorkflowAttemptSucceeded, "leftover", 1},
		{"leftover", 1, eviedb.WorkflowAttemptSucceeded, "", 2},
	}
	if got := summarize(d); !reflect.DeepEqual(got, want) {
		t.Fatalf("attempts = %+v", got)
	}
	if d.Attempts[0].LeaseHolder != a.rt.Holder() || d.Attempts[1].LeaseHolder != b.rt.Holder() || d.Attempts[1].LeaseToken <= d.Attempts[0].LeaseToken {
		t.Fatalf("holders = %+v", d.Attempts)
	}
	if !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventLeaseTakenOver, eviedb.WorkflowEventAttemptInterrupted}) ||
		!strings.Contains(d.Events[0].Detail, a.rt.Holder()) {
		t.Fatalf("events = %+v", d.Events)
	}
	if !reflect.DeepEqual(decoded(t, d.Run.Outputs), dinnerOutputs) {
		t.Fatalf("outputs = %s", d.Run.Outputs)
	}
	checkOrdering(t, d)
	stale := eviedb.WorkflowRunLease{RunID: id, Holder: a.rt.Holder(), Token: d.Attempts[0].LeaseToken}
	if _, err := a.store.BeginWorkflowAttempt(ctx, stale, eviedb.WorkflowAttemptStart{RunID: id, Checkpoint: 3, Step: "split", Kind: workflow.KindDeterministic, Attempt: 1, Input: json.RawMessage(`{}`)}); !errors.Is(err, eviedb.ErrWorkflowRunLeaseLost) {
		t.Fatalf("a stale holder's write: %v", err)
	}
	text := RunText(d)
	for _, want := range []string{"split attempt 1: interrupted", "Lease: released by " + b.rt.Holder(), "Recovery", "lease_taken_over", "attempt_interrupted"} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspection lacks %q:\n%s", want, text)
		}
	}
}

// A process that crashes inside a node is recovered by the next process's
// startup scan once its lease expires: the interrupted attempt is recorded,
// the step runs again from the same checkpoint with the same input, and the
// output is identical to an uninterrupted run. An interruption is not one
// of the step's attempts (sub-stage 5), so a step whose policy allows one
// attempt runs again too.
func TestWorkflowRunRecoversFromACrashInsideANode(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	reference := h.start(w.ID, billInputs("reference", 1000, 3))

	a := h.process()
	dead := h.crashInside(a, "split")
	go func() {
		_, _ = a.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"})
	}()
	<-dead
	h.reopen()
	runs := h.runs(w.ID)
	id := runs[0].ID
	if p := h.progress(id); p.Run.State != eviedb.WorkflowRunRunning || p.InFlight == nil || p.InFlight.Step != "split" {
		t.Fatalf("after the crash = %+v", p)
	}
	// The dead holder's lease is still live: nobody takes the run early.
	b := h.process()
	stop := b.rt.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	if p := h.progress(id); p.InFlight == nil || len(h.inspect(id).Events) != 0 {
		t.Fatalf("taken over before the lease expired: %+v", p)
	}
	h.clock.Advance(testLease)
	eventually(t, "recovery to finish the run", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptInterrupted, "", 0},
		{"split", 2, eviedb.WorkflowAttemptSucceeded, "check", 0},
		{"check", 1, eviedb.WorkflowAttemptSucceeded, "leftover", 1},
		{"leftover", 1, eviedb.WorkflowAttemptSucceeded, "", 2},
	}
	if got := summarize(d); !reflect.DeepEqual(got, want) || string(d.Attempts[0].Input) != string(d.Attempts[1].Input) {
		t.Fatalf("attempts = %+v", d.Attempts)
	}
	if !reflect.DeepEqual(decoded(t, d.Run.Outputs), decoded(t, reference.Run.Run.Outputs)) || !reflect.DeepEqual(decoded(t, d.Run.Outputs), dinnerOutputs) {
		t.Fatalf("recovered outputs %s, uninterrupted %s", d.Run.Outputs, reference.Run.Run.Outputs)
	}
	if !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventLeaseTakenOver, eviedb.WorkflowEventAttemptInterrupted}) {
		t.Fatalf("events = %+v", d.Events)
	}
	checkOrdering(t, d)

	// leftover allows one attempt: its interruption does not spend it.
	c := h.process()
	dead = h.crashInside(c, "leftover")
	go func() {
		_, _ = c.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-2", 1000, 3), Surface: "cli"})
	}()
	<-dead
	h.clock.Advance(testLease)
	e := h.process()
	stop = e.rt.Start(ctx)
	other := h.runs(w.ID)[0].ID
	eventually(t, "recovery to settle the second run", func() bool { return h.state(other).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d = h.inspect(other)
	want = []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptSucceeded, "check", 0},
		{"check", 1, eviedb.WorkflowAttemptSucceeded, "leftover", 1},
		{"leftover", 1, eviedb.WorkflowAttemptInterrupted, "", 2},
		{"leftover", 2, eviedb.WorkflowAttemptSucceeded, "", 2},
	}
	if got := summarize(d); d.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(decoded(t, d.Run.Outputs), dinnerOutputs) {
		t.Fatalf("run whose one-attempt step was interrupted = %+v attempts %+v", d.Run, got)
	}
}

// A node that keeps killing its process is not rerun forever: after the
// interruption bound (three by default) at one checkpoint, the next claim
// fails the run visibly instead of running the node again.
func TestWorkflowRunFailsAfterTheInterruptionBound(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	first := h.process()
	dead := h.crashInside(first, "split")
	go func() {
		_, _ = first.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"})
	}()
	<-dead
	id := h.runs(w.ID)[0].ID
	for crash := 2; crash <= 3; crash++ {
		h.clock.Advance(testLease)
		p := h.process()
		dead := h.crashInside(p, "split")
		// The dead process's scheduler stops polling; its pass stays stuck
		// inside the node, as a killed process would.
		polling, kill := context.WithCancel(ctx)
		p.rt.Start(polling)
		<-dead
		kill()
	}
	h.clock.Advance(testLease)
	last := h.process()
	stop := last.rt.Start(ctx)
	eventually(t, "the run to fail at the bound", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptInterrupted, "", 0},
		{"split", 2, eviedb.WorkflowAttemptInterrupted, "", 0},
		{"split", 3, eviedb.WorkflowAttemptInterrupted, "", 0},
	}
	if got := summarize(d); d.Run.State != eviedb.WorkflowRunFailed || !reflect.DeepEqual(got, want) ||
		!strings.HasPrefix(d.Run.Error, "steps.split was interrupted 3 times at checkpoint 0") {
		t.Fatalf("crash-looping run = %+v attempts %+v", d.Run, got)
	}
}

// An attempt's output and the next checkpoint are one transaction: a fault
// while recording the checkpoint leaves neither, never an output without
// its checkpoint. Recovery then treats the attempt as interrupted and the
// run ends exactly as an uninterrupted one.
func TestWorkflowRunAttemptOutputAndCheckpointCommitTogether(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	if _, err := h.db.ExecContext(ctx, `CREATE TRIGGER disk_full BEFORE INSERT ON workflow_run_checkpoints WHEN NEW.seq = 1
 BEGIN SELECT RAISE(ABORT,'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("start with a failing checkpoint write: %v", err)
	}
	id := h.runs(w.ID)[0].ID
	d := h.inspect(id)
	if len(d.Attempts) != 1 || d.Attempts[0].State != eviedb.WorkflowAttemptRunning || d.Attempts[0].Output != nil || len(d.Checkpoints) != 1 ||
		d.Lease == nil || d.Lease.ReleasedAt == nil {
		t.Fatalf("after the fault = %+v attempts %+v checkpoints %+v lease %+v", d.Run, d.Attempts, d.Checkpoints, d.Lease)
	}
	if _, err := h.db.ExecContext(ctx, `DROP TRIGGER disk_full`); err != nil {
		t.Fatal(err)
	}
	stop := h.rt.Start(ctx)
	eventually(t, "the run to finish", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d = h.inspect(id)
	if got := summarize(d); len(got) != 4 || got[0].State != eviedb.WorkflowAttemptInterrupted || got[1].Step != "split" || got[1].Attempt != 2 ||
		!reflect.DeepEqual(decoded(t, d.Run.Outputs), dinnerOutputs) {
		t.Fatalf("recovered run = %+v attempts %+v", d.Run, got)
	}
	checkOrdering(t, d)
}

// A retry's backoff is a durable not-before time: the run is released while
// it waits, a restarted process does not retry it early, and it retries
// once the time comes.
func TestWorkflowRunRetryBackoffIsDurableAcrossRestart(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	dir := copyDefinition(t, "split-bill")
	yaml := filepath.Join(dir, "workflow.yaml")
	writeFile(t, yaml, strings.Replace(readFile(t, yaml), "      max_attempts: 3\n", "      max_attempts: 3\n      backoff: 1m\n", 1))
	w := h.workspace()
	h.approve(h.propose(w.ID, dir, splitBillFixtures(t)))
	a := h.process()
	failed := false
	a.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "split" && !failed {
			failed = true
			return nil, errors.New("transient")
		}
		return v.CallFunction(step, input)
	}
	failedAt := h.clock.Now()
	id := h.detach(w.ID, billInputs("dinner-1", 1000, 3)).ID
	stop := a.rt.Start(ctx)
	eventually(t, "the first attempt to fail", func() bool { return h.progress(id).Attempts == 1 && h.progress(id).Run.NotBefore != nil })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	p := h.progress(id)
	if !p.Run.NotBefore.Equal(failedAt.Add(time.Minute)) || p.Run.State != eviedb.WorkflowRunRunning || p.InFlight != nil {
		t.Fatalf("after the failed attempt = %+v", p.Run)
	}

	h.reopen()
	b := h.process()
	stop = b.rt.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	if p := h.progress(id); p.Attempts != 1 || p.Run.State.Terminal() {
		t.Fatalf("retried before its not-before time: %+v", p)
	}
	h.clock.Advance(time.Minute)
	eventually(t, "the retry", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if got := summarize(d); len(got) != 4 || got[0].State != eviedb.WorkflowAttemptFailed || got[1].Attempt != 2 || d.Run.State != eviedb.WorkflowRunSucceeded ||
		d.Attempts[1].StartedAt.Before(failedAt.Add(time.Minute)) || d.Attempts[1].LeaseHolder != b.rt.Holder() {
		t.Fatalf("retried run = %+v attempts %+v", d.Run, d.Attempts)
	}
	if !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventRetryScheduled}) {
		t.Fatalf("events = %+v", d.Events)
	}
}

// Before a run continues, its definition must still admit it and still be
// the approved one: a deactivated or blocked definition and a pinned commit
// that no longer hashes to its approval pause the run visibly (waiting for
// a dependency, with the reason) instead of continuing. A pause is checked
// again periodically and resumes on its own once its cause is gone; a run
// of a superseded version continues under its own pins.
func TestWorkflowRunPausesWhenItsDefinitionCannotContinue(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	deactivated, da := h.approveSplitBill(splitBillDir)
	archived, _ := h.approveSplitBill(splitBillDir)
	// Git stores equal files once, so the tampered Workspace's definition
	// differs by a comment (outside the hash) to have a blob of its own.
	own := copyDefinition(t, "split-bill")
	original := []byte(readFile(t, filepath.Join(splitBillDir, "workflow.yaml")) + "# The tampered Workspace's copy.\n")
	writeFile(t, filepath.Join(own, "workflow.yaml"), string(original))
	tampered, ta := h.approveSplitBill(own)
	superseded, _ := h.approveSplitBill(splitBillDir)
	runD := h.detach(deactivated.ID, billInputs("d", 1000, 3)).ID
	runA := h.detach(archived.ID, billInputs("a", 1000, 3)).ID
	runT := h.detach(tampered.ID, billInputs("t", 1000, 3)).ID
	runS := h.detach(superseded.ID, billInputs("s", 1000, 3)).ID

	if _, err := h.rt.Deactivate(ctx, Deactivation{WorkspaceID: deactivated.ID, Name: da.Name, Version: da.Version, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.ArchiveWorkspace(ctx, archived.ID); err != nil {
		t.Fatal(err)
	}
	tamperBlob(t, h.repo.Root(), ta.Commit, ta.Path+"/workflow.yaml", []byte(strings.Replace(string(original), "max_attempts: 3", "max_attempts: 5", 1)))
	newer := copyDefinition(t, "split-bill")
	writeFile(t, filepath.Join(newer, "workflow.yaml"), strings.Replace(readFile(t, filepath.Join(splitBillDir, "workflow.yaml")), "max_attempts: 10", "max_attempts: 9", 1))
	h.approve(h.propose(superseded.ID, newer, splitBillFixtures(t)))

	p := h.process()
	stop := p.rt.Start(ctx)
	paused := func(id string) bool { return h.state(id) == eviedb.WorkflowRunWaitingForDependency }
	eventually(t, "the runs to pause", func() bool { return paused(runD) && paused(runA) && paused(runT) })
	eventually(t, "the superseded version's run", func() bool { return h.state(runS).Terminal() })
	for id, reason := range map[string]string{runD: "version 1 was deactivated", runA: "the Workspace is archived", runT: "no longer matches its approval"} {
		d := h.inspect(id)
		if !strings.Contains(d.Run.Error, reason) || d.Run.NotBefore == nil || len(d.Attempts) != 0 ||
			!reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventPaused}) || !strings.Contains(RunText(d), "Paused: ") {
			t.Fatalf("paused run %s = %+v events %+v", id, d.Run, d.Events)
		}
	}
	if d := h.inspect(runS); d.Run.State != eviedb.WorkflowRunSucceeded || d.Run.Version != 1 {
		t.Fatalf("superseded version's run = %+v", d.Run)
	}
	// Rechecks keep a paused run paused without repeating its event; a new
	// approval does not revive a run whose own version was deactivated.
	p2, err := h.store.WorkflowProposal(ctx, da.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	h.approve(Card{Proposal: p2, ExpectedVersion: 1})
	// A paused run holds its Run Key.
	if again := h.start(deactivated.ID, billInputs("d", 1, 1)); !again.Existing || again.Run.Run.ID != runD {
		t.Fatalf("start of a paused run's key = %+v", again)
	}
	tamperBlob(t, h.repo.Root(), ta.Commit, ta.Path+"/workflow.yaml", original)
	h.clock.Advance(time.Minute)
	eventually(t, "the restored definition's run to resume and finish", func() bool { return h.state(runT).Terminal() })
	eventually(t, "the deactivated run's recheck", func() bool { return h.progress(runD).Run.NotBefore.After(h.clock.Now()) })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if d := h.inspect(runD); d.Run.State != eviedb.WorkflowRunWaitingForDependency || len(d.Events) != 1 {
		t.Fatalf("deactivated run after a recheck = %+v events %+v", d.Run, d.Events)
	}
	d := h.inspect(runT)
	if d.Run.State != eviedb.WorkflowRunSucceeded || d.Run.Error != "" ||
		!reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventPaused, eviedb.WorkflowEventResumed}) {
		t.Fatalf("resumed run = %+v events %+v", d.Run, d.Events)
	}
}

// A run continues only under a runtime that declares its pinned runtime
// compatible (ADR 0025): any other pauses it visibly and never migrates
// it; a compatible runtime resumes it.
func TestWorkflowRunPausesUnderAnIncompatibleRuntime(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	id := h.detach(w.ID, billInputs("dinner-1", 1000, 3)).ID
	upgraded := h.process()
	upgraded.rt.compatible = []string{"evie.workflow.runtime/99"}
	stop := upgraded.rt.Start(ctx)
	eventually(t, "the run to pause", func() bool { return h.state(id) == eviedb.WorkflowRunWaitingForDependency })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if !strings.Contains(d.Run.Error, RuntimeVersion) || !strings.Contains(d.Run.Error, "compatible") || len(d.Attempts) != 0 {
		t.Fatalf("paused run = %+v", d.Run)
	}
	h.clock.Advance(time.Minute)
	rolledBack := h.process()
	stop = rolledBack.rt.Start(ctx)
	eventually(t, "the compatible runtime to finish the run", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(decoded(t, d.Run.Outputs), dinnerOutputs) {
		t.Fatalf("run = %+v", d.Run)
	}
}

// A background start records the run and returns at once; the scheduler
// completes it. A foreground start in a process whose scheduler runs waits
// for it, and a caller that stops waiting does not stop the run.
func TestWorkflowBackgroundStartReturnsImmediatelyAndCompletes(t *testing.T) {
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	var mu sync.Mutex
	gate := make(chan struct{})
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		mu.Lock()
		g := gate
		mu.Unlock()
		<-g
		return v.CallFunction(step, input)
	}
	stop := h.rt.Start(context.Background())
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	run := h.detach(w.ID, billInputs("dinner-1", 1000, 3))
	if run.State.Terminal() {
		t.Fatalf("a background start waited for the run: %+v", run)
	}
	close(gate)
	eventually(t, "the background run", func() bool { return h.state(run.ID) == eviedb.WorkflowRunSucceeded })

	fg := h.start(w.ID, billInputs("dinner-2", 1000, 3))
	if fg.Run.Run.State != eviedb.WorkflowRunSucceeded || fg.Run.Attempts[0].LeaseHolder != h.rt.Holder() {
		t.Fatalf("foreground start = %+v", fg.Run.Run)
	}

	mu.Lock()
	gate = make(chan struct{})
	held := gate
	mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() {
		_, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-3", 1000, 3), Surface: "web"})
		errs <- err
	}()
	eventually(t, "the third run to start", func() bool { return len(h.runs(w.ID)) == 3 && h.progress(h.runs(w.ID)[0].ID).InFlight != nil })
	cancel()
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("a waiter that stopped waiting: %v", err)
	}
	close(held)
	third := h.runs(w.ID)[0].ID
	eventually(t, "the run its waiter left", func() bool { return h.state(third) == eviedb.WorkflowRunSucceeded })
}

// The scheduler advances at most its configured number of runs at once.
func TestWorkflowSchedulerBoundsConcurrency(t *testing.T) {
	h := newLeaseHarness(t)
	h.scheduler.Workers = 2
	w, _ := h.approveSplitBill(splitBillDir)
	p := h.process()
	var mu sync.Mutex
	inside, most := 0, 0
	gate := make(chan struct{})
	p.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		mu.Lock()
		inside++
		most = max(most, inside)
		mu.Unlock()
		<-gate
		mu.Lock()
		inside--
		mu.Unlock()
		return v.CallFunction(step, input)
	}
	var ids []string
	for i := range 5 {
		ids = append(ids, h.detach(w.ID, billInputs(fmt.Sprintf("dinner-%d", i), 1000, 3)).ID)
	}
	stop := p.rt.Start(context.Background())
	eventually(t, "two runs inside nodes", func() bool { mu.Lock(); defer mu.Unlock(); return inside == 2 })
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	if inside != 2 || most != 2 {
		t.Fatalf("inside %d nodes at once (most %d) with two workers", inside, most)
	}
	mu.Unlock()
	close(gate)
	for _, id := range ids {
		eventually(t, "every run", func() bool { return h.state(id) == eviedb.WorkflowRunSucceeded })
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if most != 2 {
		t.Fatalf("most runs at once = %d", most)
	}
}

// Shutdown lets a node in flight finish and record, then releases every
// lease: the next process continues at once, with no takeover and nothing
// interrupted.
func TestWorkflowSchedulerShutdownReleasesLeases(t *testing.T) {
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	p := h.process()
	entered, gate := make(chan struct{}), make(chan struct{})
	var once sync.Once
	p.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		once.Do(func() { close(entered) })
		<-gate
		return v.CallFunction(step, input)
	}
	id := h.detach(w.ID, billInputs("dinner-1", 1000, 3)).ID
	stop := p.rt.Start(context.Background())
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	time.Sleep(20 * time.Millisecond)
	select {
	case err := <-stopped:
		t.Fatalf("shutdown abandoned the node in flight: %v", err)
	default:
	}
	close(gate)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunRunning || d.Run.Checkpoint != 1 || len(d.Attempts) != 1 || d.Attempts[0].State != eviedb.WorkflowAttemptSucceeded ||
		d.Lease == nil || d.Lease.ReleasedAt == nil || !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventLeaseReleased}) {
		t.Fatalf("after shutdown = %+v attempts %+v lease %+v events %+v", d.Run, summarize(d), d.Lease, d.Events)
	}
	next := h.process()
	stop = next.rt.Start(context.Background())
	eventually(t, "the next process to finish the run", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d = h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Attempts) != 3 || len(d.Events) != 1 {
		t.Fatalf("continued run = %+v attempts %+v events %+v", d.Run, summarize(d), d.Events)
	}
}

// Heartbeats renew the lease while a long node runs, so another process
// polling for expired leases never takes the run over (wall clock).
func TestWorkflowRunHeartbeatKeepsALongNodesLease(t *testing.T) {
	h := newRunHarness(t)
	h.scheduler = SchedulerConfig{Workers: 1, Lease: 2 * time.Second, Poll: 5 * time.Millisecond, Recheck: time.Minute}
	w, _ := h.approveSplitBill(splitBillDir)
	a, b := h.process(), h.process()
	entered := make(chan struct{})
	var once sync.Once
	a.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "split" {
			once.Do(func() { close(entered) })
			time.Sleep(3500 * time.Millisecond)
		}
		return v.CallFunction(step, input)
	}
	done := make(chan RunResult, 1)
	go func() {
		res, _ := a.rt.StartRun(context.Background(), RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"})
		done <- res
	}()
	<-entered
	stop := b.rt.Start(context.Background())
	res := <-done
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Attempts) != 3 || len(d.Events) != 0 || d.Lease == nil || d.Lease.Holder != a.rt.Holder() ||
		!d.Lease.HeartbeatAt.After(d.Lease.AcquiredAt.Add(2*time.Second)) {
		t.Fatalf("run = %+v attempts %+v events %+v lease %+v", d.Run, summarize(d), d.Events, d.Lease)
	}
}
