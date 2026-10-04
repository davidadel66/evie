package workflowruntime

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
)

// A worker whose report has no valid "## Result" block fails its attempt,
// which retries as a fresh worker under a fresh budget; a worker that wraps
// up at its budget returns a partial report, which succeeds with the
// status marking it.
func TestWorkflowAgentStepTypedResultRetriesAndPartialSucceeds(t *testing.T) {
	var mu sync.Mutex
	workers := 0
	model := &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if toolResults(r) == 0 && r.ToolChoice != "none" {
			mu.Lock()
			workers++
			mu.Unlock()
			// The first call uses 95% of the 10,000-token budget, so the
			// worker's next call is its tool-free wrap-up.
			resp := fetchCalls("https://example.org/a")
			resp.Usage = usage(9000, 500)
			return resp, nil
		}
		mu.Lock()
		n := workers
		mu.Unlock()
		if n == 1 {
			return finalReport("No typed result.", nil, "https://example.org/a"), nil
		}
		return finalReport("Wrapped up.", map[string]any{"answer": "partial answer"}, "https://example.org/a"), nil
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 2, 10000, 500000), fixtures(t, researchFixtures)))
	res, err := h.startRun(h.rt, w.ID, "one-research", map[string]string{"question": "Q"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	outputs, _ := decodeObject(d.Run.Outputs)
	if outputs["answer"] != "partial answer" || outputs["status"] != "partial" {
		t.Fatalf("outputs = %s", d.Run.Outputs)
	}
	if len(d.Attempts) != 2 || d.Attempts[0].State != eviedb.WorkflowAttemptFailed || !strings.Contains(d.Attempts[0].Error, "typed result: the report has no ## Result section") ||
		d.Attempts[1].State != eviedb.WorkflowAttemptSucceeded || !strings.Contains(d.Attempts[1].Note, "partial") {
		t.Fatalf("attempts = %+v", d.Attempts)
	}
	if len(d.Workers) != 2 || d.Workers[0].ExecutionID == d.Workers[1].ExecutionID || d.Workers[1].Status != "partial" || d.Workers[1].Reason != "token_budget" {
		t.Fatalf("workers = %+v", d.Workers)
	}
	wrapUp := false
	for _, r := range model.recorded() {
		wrapUp = wrapUp || r.ToolChoice == "none"
	}
	if !wrapUp {
		t.Fatal("the second worker never wrapped up")
	}
}

// Losing the run's lease ends a worker's authority: its watchdog stops it,
// the stale holder records nothing, and the runtime that takes the run over
// records the attempt interrupted with its worker and runs a fresh worker.
func TestWorkflowAgentWorkerLosesAuthorityWithTheRunsLease(t *testing.T) {
	ctx := context.Background()
	entered := make(chan string, 4)
	var mu sync.Mutex
	held := true
	model := &workerModel{answer: func(ctx context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		mu.Lock()
		hold := held
		held = false
		mu.Unlock()
		if hold && toolResults(r) == 0 {
			entered <- "held"
			<-ctx.Done()
			return openrouter.ChatResponse{}, ctx.Err()
		}
		return answerResearch(ctx, r)
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 1, 100000, 500000), fixtures(t, researchFixtures)))
	stale := h.process()
	done := make(chan error, 1)
	go func() {
		_, err := h.startRun(stale.rt, w.ID, "one-research", map[string]string{"question": "Q"})
		done <- err
	}()
	<-entered
	id := h.runs(w.ID)[0].ID
	first := h.inspect(id).Workers[0]
	if err := stale.store.AuthorizeSubagent(ctx, first.ExecutionID); err != nil {
		t.Fatalf("worker authority under the live lease: %v", err)
	}
	h.clock.Advance(testLease + time.Second)
	// The worker's authority ended with the lease, before anyone took over.
	eventually(t, "the worker to stop", func() bool {
		exec, _, err := h.store.RunWorkerExecution(ctx, id, first.ExecutionID)
		return err == nil && exec.Terminal()
	})
	exec, _, _ := h.store.RunWorkerExecution(ctx, id, first.ExecutionID)
	if exec.State != "interrupted" || exec.Result.Reason != "authority_ended" {
		t.Fatalf("the stale worker = %s %+v", exec.State, exec.Result)
	}
	if err := h.store.AuthorizeSubagent(ctx, first.ExecutionID); err == nil {
		t.Fatal("a worker kept authority after its run's lease expired")
	}
	// The foreground start keeps waiting and claims the run again under a
	// new lease; nothing was recorded under the lost one.
	<-done
	if !h.state(id).Terminal() {
		next := h.process()
		if _, err := next.rt.execute(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Attempts) != 2 || d.Attempts[0].State != eviedb.WorkflowAttemptInterrupted ||
		len(d.Workers) != 2 || d.Workers[0].Status != "interrupted" || d.Workers[1].ExecutionID == first.ExecutionID || d.Attempts[1].LeaseToken < 2 {
		t.Fatalf("after takeover = %+v\n%s", d.Run, RunText(d))
	}
	for _, e := range d.Events {
		if e.Kind == eviedb.WorkflowEventAttemptInterrupted && e.Token < 2 {
			t.Fatalf("the lost lease recorded an interruption: %+v", e)
		}
	}
}

// The owner's cancellation reaches a worker in flight: it is stopped, its
// attempt recorded interrupted with its worker's outcome, and the run ends
// cancelled; the worker's authority ends with the run's running state.
func TestWorkflowAgentCancellationStopsTheWorker(t *testing.T) {
	ctx := context.Background()
	entered := make(chan struct{}, 1)
	model := &workerModel{answer: func(ctx context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return openrouter.ChatResponse{}, ctx.Err()
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 1, 100000, 500000), fixtures(t, researchFixtures)))
	done := make(chan RunResult, 1)
	go func() {
		res, _ := h.startRun(h.rt, w.ID, "one-research", map[string]string{"question": "Q"})
		done <- res
	}()
	<-entered
	id := h.runs(w.ID)[0].ID
	execution := h.inspect(id).Workers[0].ExecutionID
	owner := h.process()
	if _, err := owner.rt.Cancel(ctx, CancelRequest{RunID: id, Reason: "not needed", Surface: "cli", Detach: true}); err != nil {
		t.Fatal(err)
	}
	if err := owner.store.AuthorizeSubagent(ctx, execution); err == nil {
		t.Fatal("a worker kept authority after its run was cancelled")
	}
	res := waitRun(t, done)
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunCancelled || d.Attempts[0].State != eviedb.WorkflowAttemptInterrupted ||
		!strings.Contains(d.Attempts[0].Error, "the owner cancelled the run while the worker ran") || d.Workers[0].FinishedAt == nil {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	exec, _, err := h.store.RunWorkerExecution(ctx, id, execution)
	if err != nil || !exec.Terminal() {
		t.Fatalf("worker = %+v %v", exec, err)
	}
}
