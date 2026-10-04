package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// cancelCommands records the cancellations and starts the CLI asks for.
type cancelCommands struct {
	workflowCommands
	cancels []workflowruntime.CancelRequest
	starts  []workflowruntime.RunRequest
	answer  eviedb.WorkflowRunDetail
}

func (c *cancelCommands) Cancel(_ context.Context, req workflowruntime.CancelRequest) (eviedb.WorkflowRunDetail, error) {
	c.cancels = append(c.cancels, req)
	return c.answer, nil
}

func (c *cancelCommands) StartRun(_ context.Context, req workflowruntime.RunRequest) (workflowruntime.RunResult, error) {
	c.starts = append(c.starts, req)
	return workflowruntime.RunResult{Run: c.answer}, nil
}

// The owner cancels a run with `evie workflows cancel <run-id> [--reason
// <text>] [--detach]` (Stage 14 sub-stage 8): end to end, a queued run is
// cancelled and its Run Key is free again; a finished run is refused.
func TestWorkflowCLICancel(t *testing.T) {
	c := newWorkflowCLI(t)
	w := c.approveSplitBill()
	out := c.mustRun("workflows", "run", "--detach", string(w.ID), "split-bill", "--input", "bill_id=dinner-1", "--input", "total_cents=1000", "--input", "people=3")
	m := runIDLine.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no run ID:\n%s", out)
	}
	id := m[1]
	out = c.mustRun("workflows", "cancel", id, "--reason", "entered twice")
	for _, want := range []string{"Cancelled run " + id, "cancelled", "Cancelled by the owner via cli", "entered twice"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cancel output lacks %q:\n%s", want, out)
		}
	}
	again := c.mustRun("workflows", "run", string(w.ID), "split-bill", "--input", "bill_id=dinner-1", "--input", "total_cents=1000", "--input", "people=3")
	if strings.Contains(again, "already holds this Run Key") || !strings.Contains(again, "succeeded") {
		t.Fatalf("a start of the freed key:\n%s", again)
	}
	done := runIDLine.FindStringSubmatch(again)[1]
	if _, err := c.run("workflows", "cancel", done); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Fatalf("cancelling a succeeded run: %v", err)
	}

	// The request the CLI sends, and its usage.
	ctx := context.Background()
	rt := &cancelCommands{answer: eviedb.WorkflowRunDetail{Run: eviedb.WorkflowRun{ID: "run-1", Workflow: "post-share", Version: 1, State: eviedb.WorkflowRunCancelling,
		Error: "cancelling once its effect is reconciled: reconcile it with: evie workflows reconcile run-1 post"}}}
	var buf bytes.Buffer
	if _, err := runWorkflowActivation(ctx, []string{"workflows", "cancel", "run-1"}, &buf, rt); err == nil || !strings.Contains(err.Error(), "still cancelling") {
		t.Fatalf("a cancellation that waits for reconciliation: %v", err)
	}
	buf.Reset()
	if _, err := runWorkflowActivation(ctx, []string{"workflows", "cancel", "--json", "--detach", "run-1", "--reason", "stop"}, &buf, rt); err != nil {
		t.Fatalf("a detached cancellation: %v", err)
	}
	var shown eviedb.WorkflowRunDetail
	if json.Unmarshal(buf.Bytes(), &shown) != nil || shown.Run.ID != "run-1" {
		t.Fatalf("--json output %s", buf.String())
	}
	want := []workflowruntime.CancelRequest{{RunID: "run-1", Surface: "cli"}, {RunID: "run-1", Reason: "stop", Detach: true, Surface: "cli"}}
	if len(rt.cancels) != 2 || rt.cancels[0] != want[0] || rt.cancels[1] != want[1] {
		t.Fatalf("cancellations = %+v", rt.cancels)
	}
	for _, args := range [][]string{
		{"workflows", "cancel"},
		{"workflows", "cancel", "run-1", "run-2"},
		{"workflows", "cancel", "run-1", "--reason", "a", "--reason", "b"},
		{"workflows", "cancel", "run-1", "--why", "a"},
		{"workflows", "cancel", "--detach", "--detach", "run-1"},
	} {
		if _, err := runWorkflowActivation(ctx, args, &buf, rt); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if len(rt.cancels) != 2 {
		t.Fatalf("refused commands reached the runtime: %+v", rt.cancels)
	}
	var review bytes.Buffer
	if handled, _ := runWorkflowManagement(ctx, []string{"workflows", "cancel"}, &review, func(string) string { return "" }); handled {
		t.Fatal("cancel was answered before the database opened")
	}
}

// The owner reruns a succeeded logical execution with `evie workflows run
// ... --rerun-of <run-id> --reason <text>` (ADR 0021's audited override);
// a reason without the run it overrides is a usage error.
func TestWorkflowCLIRerunOverride(t *testing.T) {
	ctx := context.Background()
	rt := &cancelCommands{answer: eviedb.WorkflowRunDetail{Run: eviedb.WorkflowRun{ID: "run-2", Workflow: "post-share", Version: 1, State: eviedb.WorkflowRunSucceeded}}}
	var out bytes.Buffer
	if _, err := runWorkflowActivation(ctx, []string{"workflows", "run", "ws-1", "post-share", "--rerun-of", "run-1", "--reason", "the ledger was restored"}, &out, rt); err != nil {
		t.Fatalf("rerun: %v\n%s", err, out.String())
	}
	if len(rt.starts) != 1 || rt.starts[0].RerunOf != "run-1" || rt.starts[0].Reason != "the ledger was restored" || rt.starts[0].Surface != "cli" ||
		len(rt.starts[0].TextInputs) != 0 {
		t.Fatalf("start = %+v", rt.starts)
	}
	for _, args := range [][]string{
		{"workflows", "run", "ws-1", "post-share", "--reason", "x"},
		{"workflows", "run", "ws-1", "post-share", "--rerun-of", "run-1"},
		{"workflows", "run", "ws-1", "post-share", "--rerun-of", "run-1", "--rerun-of", "run-2", "--reason", "x"},
		{"workflows", "run", "ws-1", "post-share", "--rerun-of"},
	} {
		if _, err := runWorkflowActivation(ctx, args, &out, rt); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if len(rt.starts) != 1 {
		t.Fatalf("refused commands reached the runtime: %+v", rt.starts)
	}
}
