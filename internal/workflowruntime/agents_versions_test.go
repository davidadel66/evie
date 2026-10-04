package workflowruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
)

// Version compatibility (Stage 14 sub-stage 9). Runs pin the current
// runtime (evie.workflow.runtime/5 then, /6 since sub-stage 10); an Evie
// that cannot run it pauses such a run visibly instead of advancing it, and
// only an Evie that runs agent and map steps can finish a run, including
// the owner's cancellation, while a worker it started is unsettled: the
// database refuses the others.

// An Evie whose runtime is older than the run's pauses a run that pins it,
// starting nothing; a current Evie resumes it.
func TestWorkflowOlderEviePausesAnAgentRunVisibly(t *testing.T) {
	ctx := context.Background()
	h, _ := newAgentHarness(t, nil, &workerModel{answer: answerResearch})
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 1, 100000, 500000), fixtures(t, researchFixtures)))
	res, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "one-research", TextInputs: map[string]string{"question": "Q"}, Surface: "cli", Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	older := h.process()
	older.rt.compatible = []string{"evie.workflow.runtime/1", "evie.workflow.runtime/2", "evie.workflow.runtime/3", "evie.workflow.runtime/4", "evie.workflow.runtime/5"}
	if _, err := older.rt.execute(ctx, id); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunWaitingForDependency || !strings.Contains(d.Run.Error, RuntimeVersion+" is not one this Evie declares compatible") ||
		len(d.Attempts) != 0 || len(d.Workers) != 0 {
		t.Fatalf("under an older Evie = %+v", d.Run)
	}
	h.clock.Advance(time.Minute)
	if _, err := h.process().rt.execute(ctx, id); err != nil {
		t.Fatal(err)
	}
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("resumed = %+v\n%s", d.Run, RunText(d))
	}
}

// An agent step paused in an Evie without workers continues by itself in
// one that has them.
func TestWorkflowAgentStepPausesWithoutWorkersAndContinuesWithThem(t *testing.T) {
	ctx := context.Background()
	h, _ := newAgentHarness(t, nil, &workerModel{answer: answerResearch})
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 1, 100000, 500000), fixtures(t, researchFixtures)))
	res, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "one-research", TextInputs: map[string]string{"question": "Q"}, Surface: "cli", Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	factory := h.agentsFor
	h.agentsFor = nil
	without := h.process()
	h.agentsFor = factory
	if _, err := without.rt.execute(ctx, id); err != nil {
		t.Fatal(err)
	}
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunWaitingForDependency || !strings.Contains(d.Run.Error, "agent steps are off in this Evie") || len(d.Attempts) != 0 {
		t.Fatalf("without workers = %+v", d.Run)
	}
	h.clock.Advance(time.Minute)
	if _, err := h.process().rt.execute(ctx, id); err != nil {
		t.Fatal(err)
	}
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("with workers = %+v\n%s", d.Run, RunText(d))
	}
}

// A cancellation of a run whose holder died with a worker in flight can be
// finished only by an Evie that runs workers: an older Evie's claim would
// record the attempt interrupted and try to finish the run cancelled, and
// the database refuses that while the worker call is open; nor can it
// rewrite the run-owned execution. The current Evie's claim settles the
// worker and finishes the cancellation.
func TestWorkflowOnlyAnEvieThatRunsWorkersFinishesACancellationWithAWorkerInFlight(t *testing.T) {
	ctx := context.Background()
	entered := make(chan struct{})
	var dead *process
	model := &workerModel{answer: func(ctx context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		_ = dead.db.Close()
		close(entered)
		<-ctx.Done()
		return openrouter.ChatResponse{}, ctx.Err()
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 1, 100000, 500000), fixtures(t, researchFixtures)))
	dead = h.process()
	go func() { _, _ = h.startRun(dead.rt, w.ID, "one-research", map[string]string{"question": "Q"}) }()
	<-entered
	id := h.runs(w.ID)[0].ID
	execution := h.inspect(id).Workers[0].ExecutionID
	if _, err := h.rt.Cancel(ctx, CancelRequest{RunID: id, Surface: "cli", Detach: true}); err != nil {
		t.Fatal(err)
	}
	// What an older Evie's claim and cancellation would write.
	now := formatTestTime(h.clock.Now())
	if _, err := h.db.Exec(`UPDATE workflow_node_attempts SET state='interrupted',error='interrupted: an older Evie',finished_at=? WHERE run_id=? AND state='running'`, now, id); err != nil {
		t.Fatal(err)
	}
	_, err := h.db.Exec(`UPDATE workflow_runs SET state='cancelled',error='cancelled by the owner via cli',finished_at=?,updated_at=? WHERE id=?`, now, now, id)
	if err == nil || !strings.Contains(err.Error(), "finishes only after every worker it started ended") {
		t.Fatalf("an older Evie finished the cancellation: %v", err)
	}
	if _, err := h.db.Exec(`UPDATE subagent_executions SET state='interrupted',record_json=json_remove(record_json,'$.run') WHERE id=?`, execution); err == nil ||
		!strings.Contains(err.Error(), "only by an Evie that runs workflow agent steps") {
		t.Fatalf("an older Evie rewrote a run-owned execution: %v", err)
	}
	h.clock.Advance(testLease + time.Second)
	if _, err := h.process().rt.execute(ctx, id); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunCancelled || d.Workers[0].FinishedAt == nil || d.Workers[0].Status != "interrupted" {
		t.Fatalf("after the current Evie = %+v workers %+v", d.Run, d.Workers)
	}
	exec, _, err := h.store.RunWorkerExecution(ctx, id, execution)
	if err != nil || exec.State != "interrupted" || exec.Result.Reason != delegation.ReasonRunOwnershipEnded {
		t.Fatalf("execution = %+v %v", exec, err)
	}
}

func formatTestTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Run-owned admission requires the run's own lease and a node attempt in
// flight under it: a worker for a run whose lease is not the caller's, or
// for an attempt that is not in flight, is refused and nothing is
// recorded.
func TestWorkflowRunOwnedAdmissionNeedsTheRunsLeaseAndAnAttemptInFlight(t *testing.T) {
	ctx := context.Background()
	h, _ := newAgentHarness(t, nil, &workerModel{answer: answerResearch})
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 1, 100000, 500000), fixtures(t, researchFixtures)))
	res, err := h.startRun(h.rt, w.ID, "one-research", map[string]string{"question": "Q"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	agents := h.agentsOf(h.store)
	owner := delegation.RunOwner{RunID: d.Run.ID, WorkspaceID: w.ID, Workflow: "one-research", Step: "research", AttemptSeq: d.Attempts[0].Seq,
		Holder: d.Attempts[0].LeaseHolder, Token: d.Attempts[0].LeaseToken, Policy: "researcher", PolicyVersion: 1, Model: "deepseek/deepseek-v4.1-flash", MaxOutputTokens: 4000}
	req := delegation.RunAssignment{Owner: owner, Worker: "research", Objective: researchObjective, Data: `{"question":"Q"}`, Deadline: time.Minute, TokenBudget: 1000}
	for name, mutate := range map[string]func(*delegation.RunAssignment){
		"a finished run under its released lease": func(*delegation.RunAssignment) {},
		"another holder":            func(r *delegation.RunAssignment) { r.Owner.Holder = "someone-else" },
		"an attempt never recorded": func(r *delegation.RunAssignment) { r.Owner.AttemptSeq = 99 },
	} {
		r := req
		mutate(&r)
		if _, err := agents.RunWorker(ctx, r); !errors.Is(err, delegation.ErrAuthority) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	var executions int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM subagent_executions`).Scan(&executions); err != nil || executions != 1 {
		t.Fatalf("executions = %d %v", executions, err)
	}
	// A budget above the operator's policy is refused before admission.
	over := req
	over.TokenBudget = delegation.DefaultPolicy().TokenBudget + 1
	if _, err := agents.RunWorker(ctx, over); !errors.Is(err, delegation.ErrPolicy) {
		t.Fatalf("over budget: %v", err)
	}
}
