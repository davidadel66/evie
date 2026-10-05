package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Stage 14 sub-stage 12: the owner migrates a run with `evie workflows
// migrate`, and sees and runs the retention sweep with `evie workflows
// retention status|sweep [--dry-run]`.

type migrateCommands struct {
	workflowCommands
	migrations []workflowruntime.MigrationRequest
	sweeps     []workflowruntime.SweepRequest
	answer     workflowruntime.MigrationResult
}

func (c *migrateCommands) Migrate(_ context.Context, req workflowruntime.MigrationRequest) (workflowruntime.MigrationResult, error) {
	c.migrations = append(c.migrations, req)
	return c.answer, nil
}

func (c *migrateCommands) Sweep(_ context.Context, req workflowruntime.SweepRequest) (workflowruntime.SweepResult, error) {
	c.sweeps = append(c.sweeps, req)
	return workflowruntime.SweepResult{SweepID: "sweep-1", DryRun: req.DryRun, Tombstones: []eviedb.WorkflowRetentionTombstone{}}, nil
}

func TestWorkflowCLIMigrate(t *testing.T) {
	ctx := context.Background()
	rt := &migrateCommands{answer: workflowruntime.MigrationResult{
		Migration:   eviedb.WorkflowRunMigration{SuccessorID: "run-2", PredecessorID: "run-1", FromVersion: 1, ToVersion: 2, Resume: "check"},
		Predecessor: eviedb.WorkflowRun{ID: "run-1", Workflow: "split-bill", Version: 1, State: eviedb.WorkflowRunSuperseded},
		Successor:   eviedb.WorkflowRunDetail{Run: eviedb.WorkflowRun{ID: "run-2", Workflow: "split-bill", Version: 2, State: eviedb.WorkflowRunSucceeded}},
	}}
	var out bytes.Buffer
	if _, err := runWorkflowActivation(ctx, []string{"workflows", "migrate", "run-1", "--to-version", "2", "--reason", "new rules"}, &out, rt); err != nil {
		t.Fatalf("migrate: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Migrated run run-1 (version 1) to run run-2 (version 2), resuming at steps.check") {
		t.Fatalf("output:\n%s", out.String())
	}
	out.Reset()
	if _, err := runWorkflowActivation(ctx, []string{"workflows", "migrate", "--json", "--detach", "run-1", "--to-version", "2"}, &out, rt); err != nil {
		t.Fatal(err)
	}
	var shown workflowruntime.MigrationResult
	if json.Unmarshal(out.Bytes(), &shown) != nil || shown.Migration.SuccessorID != "run-2" {
		t.Fatalf("--json output %s", out.String())
	}
	want := []workflowruntime.MigrationRequest{{RunID: "run-1", ToVersion: 2, Reason: "new rules", Surface: "cli"}, {RunID: "run-1", ToVersion: 2, Detach: true, Surface: "cli"}}
	if len(rt.migrations) != 2 || rt.migrations[0] != want[0] || rt.migrations[1] != want[1] {
		t.Fatalf("migrations = %+v", rt.migrations)
	}
	for _, args := range [][]string{
		{"workflows", "migrate"},
		{"workflows", "migrate", "run-1"},
		{"workflows", "migrate", "run-1", "--to-version", "two"},
		{"workflows", "migrate", "run-1", "--to-version", "0"},
		{"workflows", "migrate", "run-1", "run-2", "--to-version", "2"},
		{"workflows", "migrate", "run-1", "--to-version", "2", "--why", "x"},
	} {
		if _, err := runWorkflowActivation(ctx, args, &out, rt); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if len(rt.migrations) != 2 {
		t.Fatalf("refused commands reached the runtime: %+v", rt.migrations)
	}

	// End to end: a finished run is never migrated.
	c := newWorkflowCLI(t)
	w := c.approveSplitBill()
	done := runIDLine.FindStringSubmatch(c.mustRun("workflows", "run", string(w.ID), "split-bill", "--input", "bill_id=dinner-1", "--input", "total_cents=1000", "--input", "people=3"))[1]
	if _, err := c.run("workflows", "migrate", done, "--to-version", "2"); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Fatalf("migrating a finished run: %v", err)
	}
	var review bytes.Buffer
	if handled, _ := runWorkflowManagement(ctx, []string{"workflows", "migrate"}, &review, func(string) string { return "" }); handled {
		t.Fatal("migrate was answered before the database opened")
	}
}

func TestWorkflowCLIRetention(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	policy := workflowruntime.DefaultRetention()
	zero := time.Duration(0)
	policy.Ordinary.Compact = &zero
	rt, err := workflowruntime.New(workflowruntime.Config{
		Store: store, Repo: procedural.New(filepath.Join(t.TempDir(), "procedural")), Capabilities: workflowruntime.ContractsOnly(notesTestCatalog()),
		Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000}, Retention: &policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &workflowCLI{t: t, store: store, rt: rt}
	w := c.approveSplitBill()
	id := runIDLine.FindStringSubmatch(c.mustRun("workflows", "run", string(w.ID), "split-bill", "--input", "bill_id=dinner-1", "--input", "total_cents=1000", "--input", "people=3"))[1]
	status := c.mustRun("workflows", "retention", "status")
	for _, want := range []string{"ordinary: mask on, redact never, delete notices never, compact at once after a run ended",
		"sensitive: mask on, redact never, delete notices never, compact 7 days after a run ended", "Due now", "compact checkpoint_states of run " + id} {
		if !strings.Contains(status, want) {
			t.Fatalf("status lacks %q:\n%s", want, status)
		}
	}
	dry := c.mustRun("workflows", "retention", "sweep", "--dry-run")
	if !strings.Contains(dry, "Dry run") || !strings.Contains(dry, "compact checkpoint_states of run "+id) {
		t.Fatalf("dry run:\n%s", dry)
	}
	swept := c.mustRun("workflows", "retention", "sweep")
	if !strings.Contains(swept, "compacted checkpoint_states of run "+id) {
		t.Fatalf("sweep:\n%s", swept)
	}
	raw := c.mustRun("workflows", "retention", "status", "--json")
	var shown workflowruntime.RetentionStatus
	if err := json.Unmarshal([]byte(raw), &shown); err != nil || len(shown.State.Totals) != 1 || shown.State.Totals[0].Action != "compact" || len(shown.Due.Tombstones) != 0 {
		t.Fatalf("status --json = %s (%v)", raw, err)
	}
	if inspected := c.mustRun("workflows", "inspect", id); !strings.Contains(inspected, "Compacted checkpoint_states") {
		t.Fatalf("inspect:\n%s", inspected)
	}
	// The request the CLI sends, and its usage.
	fake := &migrateCommands{}
	var out bytes.Buffer
	if _, err := runWorkflowActivation(ctx, []string{"workflows", "retention", "sweep", "--dry-run", "--json"}, &out, fake); err != nil {
		t.Fatal(err)
	}
	if len(fake.sweeps) != 1 || fake.sweeps[0] != (workflowruntime.SweepRequest{DryRun: true, Surface: "cli"}) {
		t.Fatalf("sweeps = %+v", fake.sweeps)
	}
	for _, args := range [][]string{
		{"workflows", "retention"},
		{"workflows", "retention", "purge"},
		{"workflows", "retention", "sweep", "--dry-run", "--dry-run"},
		{"workflows", "retention", "status", "extra"},
	} {
		if _, err := runWorkflowActivation(ctx, args, &out, fake); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
}

func TestWorkflowRetentionEnvironment(t *testing.T) {
	env := func(values map[string]string) func(string) string { return func(k string) string { return values[k] } }
	got, err := workflowRetention(env(nil))
	if err != nil || !got.Ordinary.Mask || got.Ordinary.Redact != nil || got.Sensitive.Notices != nil || got.Ordinary.Compact == nil {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	got, err = workflowRetention(env(map[string]string{"EVIE_WORKFLOW_RETENTION_SENSITIVE": "redact=30d,notices=90d"}))
	if err != nil || got.Sensitive.Redact == nil || *got.Sensitive.Redact != 30*24*time.Hour || got.Ordinary.Redact != nil || !got.Sensitive.Mask {
		t.Fatalf("sensitive policy = %+v, %v", got, err)
	}
	if _, err := workflowRetention(env(map[string]string{"EVIE_WORKFLOW_RETENTION_ORDINARY": "forget=everything"})); err == nil ||
		!strings.Contains(err.Error(), "EVIE_WORKFLOW_RETENTION_ORDINARY") {
		t.Fatalf("a malformed policy: %v", err)
	}
}
