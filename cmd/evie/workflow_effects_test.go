package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// reconcileCommands records the reconciliations the CLI asks for.
type reconcileCommands struct {
	workflowCommands
	got    []workflowruntime.Reconciliation
	answer eviedb.WorkflowRunDetail
}

func (r *reconcileCommands) Reconcile(_ context.Context, req workflowruntime.Reconciliation) (eviedb.WorkflowRunDetail, error) {
	r.got = append(r.got, req)
	return r.answer, nil
}

// The owner reconciles an effect whose outcome is unknown with `evie
// workflows reconcile <run-id> <step> --outcome happened|did-not-happen
// [--evidence <text>] [--output-json <file>] [--detach]` (Stage 14
// sub-stage 6); the surface is always the CLI.
func TestWorkflowCLIReconcile(t *testing.T) {
	ctx := context.Background()
	output := filepath.Join(t.TempDir(), "output.json")
	if err := os.WriteFile(output, []byte(`{"entry_id": "entry-1", "balance_cents": 250}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rt := &reconcileCommands{answer: eviedb.WorkflowRunDetail{Run: eviedb.WorkflowRun{ID: "run-1", Workflow: "post-share", Version: 1, State: eviedb.WorkflowRunSucceeded}}}
	var out bytes.Buffer
	handled, err := runWorkflowActivation(ctx, []string{"workflows", "reconcile", "run-1", "post", "--outcome", "happened", "--evidence", "the statement shows entry-1",
		"--output-json", output}, &out, rt)
	if !handled || err != nil {
		t.Fatalf("reconcile = %v, %v\n%s", handled, err, out.String())
	}
	want := workflowruntime.Reconciliation{RunID: "run-1", Step: "post", Outcome: "happened", Evidence: "the statement shows entry-1",
		Output: json.RawMessage(`{"entry_id": "entry-1", "balance_cents": 250}`), Surface: "cli"}
	if len(rt.got) != 1 || rt.got[0].RunID != want.RunID || rt.got[0].Step != want.Step || rt.got[0].Outcome != want.Outcome ||
		rt.got[0].Evidence != want.Evidence || string(rt.got[0].Output) != string(want.Output) || rt.got[0].Surface != "cli" || rt.got[0].Detach {
		t.Fatalf("reconciliation = %+v", rt.got)
	}
	if !strings.Contains(out.String(), "Reconciled steps.post of run run-1 as happened") || !strings.Contains(out.String(), "Run run-1: post-share version 1") {
		t.Fatalf("output:\n%s", out.String())
	}
	out.Reset()
	rt.answer.Run.State, rt.answer.Run.Error = eviedb.WorkflowRunFailed, "steps.post: Effect Intent #1 did not happen"
	if _, err := runWorkflowActivation(ctx, []string{"workflows", "reconcile", "--json", "--detach", "run-1", "post", "--outcome", "did-not-happen"}, &out, rt); err == nil ||
		!strings.Contains(err.Error(), "failed") {
		t.Fatalf("a reconciliation that fails the run: %v", err)
	}
	var shown eviedb.WorkflowRunDetail
	if json.Unmarshal(out.Bytes(), &shown) != nil || shown.Run.ID != "run-1" || len(rt.got) != 2 || !rt.got[1].Detach || rt.got[1].Outcome != "did-not-happen" {
		t.Fatalf("--json output %s, request %+v", out.String(), rt.got)
	}
	for _, args := range [][]string{
		{"workflows", "reconcile"},
		{"workflows", "reconcile", "run-1"},
		{"workflows", "reconcile", "run-1", "post"},
		{"workflows", "reconcile", "run-1", "post", "--outcome", "maybe"},
		{"workflows", "reconcile", "run-1", "post", "extra", "--outcome", "happened"},
		{"workflows", "reconcile", "run-1", "post", "--outcome", "happened", "--outcome", "happened"},
	} {
		if _, err := runWorkflowActivation(ctx, args, &out, rt); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`[1, 2]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runWorkflowActivation(ctx, []string{"workflows", "reconcile", "run-1", "post", "--outcome", "happened", "--output-json", bad}, &out, rt); err == nil ||
		!strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("an output that is not an object: %v", err)
	}
	if len(rt.got) != 2 {
		t.Fatalf("refused commands reached the runtime: %+v", rt.got)
	}
	var review bytes.Buffer
	if handled, _ := runWorkflowManagement(ctx, []string{"workflows", "reconcile"}, &review, func(string) string { return "" }); handled {
		t.Fatal("reconcile was answered before the database opened")
	}
}

// A foreground run that ends needing reconciliation exits with an error
// naming the reason, like a paused run.
func TestWorkflowCLIRunNeedingReconciliationIsAnError(t *testing.T) {
	err := runResultError(workflowruntime.RunResult{Run: eviedb.WorkflowRunDetail{Run: eviedb.WorkflowRun{ID: "run-1", State: eviedb.WorkflowRunNeedsReconciliation,
		Error: "steps.post: the outcome of Effect Intent #1 is unknown"}}})
	if err == nil || !strings.Contains(err.Error(), "needs reconciliation") || !strings.Contains(err.Error(), "Effect Intent #1") {
		t.Fatalf("error = %v", err)
	}
}
