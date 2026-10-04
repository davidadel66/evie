package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

var splitBillDir = filepath.Join("..", "..", "internal", "workflow", "testdata", "split-bill")

// approveSplitBillCLI proposes and approves the split-bill example through
// the CLI in a new Workspace.
func (c *workflowCLI) approveSplitBill() memory.Workspace {
	c.t.Helper()
	w, err := c.store.RegisterWorkspace(context.Background(), "Bills")
	if err != nil {
		c.t.Fatal(err)
	}
	card := c.mustRun("workflows", "propose", string(w.ID), splitBillDir, "--fixtures", filepath.Join("..", "..", "internal", "workflow", "testdata", "split-bill.fixtures.json"))
	m := approveLine.FindStringSubmatch(card)
	if m == nil {
		c.t.Fatalf("no approve command:\n%s", card)
	}
	c.mustRun("workflows", "approve", m[1], "--hash", m[2], "--authority", m[3], "--expect-version", m[4])
	return w
}

var runIDLine = regexp.MustCompile(`Run (\S+): split-bill version`)

// The owner runs an approved workflow from the CLI, sees its nodes and
// outputs, lists and inspects runs; a duplicate start returns the run that
// holds the Run Key.
func TestWorkflowCLIRunListAndInspect(t *testing.T) {
	c := newWorkflowCLI(t)
	w := c.approveSplitBill()
	out := c.mustRun("workflows", "run", string(w.ID), "split-bill", "--input", "bill_id=dinner-1", "--input", "total_cents=1000", "--input", "people=3")
	for _, want := range []string{"split-bill version 1", "succeeded", "Run key: split-bill:sha256:", "check attempt 1: succeeded -> leftover", "leftover_cents: 1", "shares: [334,333,333]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("run output lacks %q:\n%s", want, out)
		}
	}
	m := runIDLine.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no run ID:\n%s", out)
	}
	id := m[1]
	again := c.mustRun("workflows", "run", string(w.ID), "split-bill", "--input", "bill_id=dinner-1", "--input", "total_cents=5", "--input", "people=1")
	if !strings.Contains(again, "already holds this Run Key") || !strings.Contains(again, "Run "+id) {
		t.Fatalf("duplicate run output:\n%s", again)
	}
	inputs := filepath.Join(t.TempDir(), "inputs.json")
	if err := os.WriteFile(inputs, []byte(`{"bill_id": "dinner-2", "total_cents": 900}`), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := c.mustRun("workflows", "run", "--json", string(w.ID), "split-bill", "--input-json", inputs, "--input", "people=3")
	var started workflowruntime.RunResult
	var outputs map[string]any
	if err := json.Unmarshal([]byte(raw), &started); err != nil || started.Existing || started.Run.Run.State != eviedb.WorkflowRunSucceeded ||
		json.Unmarshal(started.Run.Run.Outputs, &outputs) != nil || outputs["per_person"] != float64(300) {
		t.Fatalf("run --json = %s (%v)", raw, err)
	}

	listed := c.mustRun("workflows", "runs", string(w.ID))
	if strings.Count(listed, "split-bill v1") != 2 || !strings.Contains(listed, id) {
		t.Fatalf("runs:\n%s", listed)
	}
	raw = c.mustRun("workflows", "runs", "--json", "--workflow", "split-bill")
	var runs struct {
		Runs []eviedb.WorkflowRun `json:"runs"`
	}
	if err := json.Unmarshal([]byte(raw), &runs); err != nil || len(runs.Runs) != 2 || runs.Runs[0].ID != started.Run.Run.ID {
		t.Fatalf("runs --json = %s (%v)", raw, err)
	}
	if none := c.mustRun("workflows", "runs", string(w.ID), "--workflow", "other"); !strings.Contains(none, "No workflow runs") {
		t.Fatalf("no runs:\n%s", none)
	}
	inspected := c.mustRun("workflows", "inspect", id)
	for _, want := range []string{"Run " + id, "split attempt 1: succeeded -> check (from checkpoint 0)", `input {"parts":3,"total":1000}`, "Inputs: bill_id=\"dinner-1\", people=3, total_cents=1000"} {
		if !strings.Contains(inspected, want) {
			t.Fatalf("inspect lacks %q:\n%s", want, inspected)
		}
	}
	raw = c.mustRun("workflows", "inspect", "--json", id)
	var detail eviedb.WorkflowRunDetail
	if err := json.Unmarshal([]byte(raw), &detail); err != nil || len(detail.Attempts) != 3 || len(detail.Checkpoints) != 4 {
		t.Fatalf("inspect --json = %s (%v)", raw, err)
	}
	if _, err := c.run("workflows", "inspect", "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing run: %v", err)
	}
}

// A run that fails exits with an error after showing it; invalid input and
// definitions this runtime cannot execute start nothing.
func TestWorkflowCLIRunFailuresAndRefusals(t *testing.T) {
	ctx := context.Background()
	c := newWorkflowCLI(t)
	w := c.approveSplitBill()
	out, err := c.run("workflows", "run", string(w.ID), "split-bill", "--input", "bill_id=b", "--input", "total_cents=10", "--input", "people=5000")
	if err == nil || !strings.Contains(err.Error(), "failed") || !strings.Contains(out, "split attempt 3: failed") {
		t.Fatalf("failed run: %v\n%s", err, out)
	}
	if _, err := c.run("workflows", "run", string(w.ID), "split-bill", "--input", "bill_id=b", "--input", "total_cents=ten", "--input", "people=2"); err == nil || !strings.Contains(err.Error(), "inputs.total_cents") {
		t.Fatalf("invalid input: %v", err)
	}
	notes, err := c.store.RegisterWorkspace(ctx, "Notes")
	if err != nil {
		t.Fatal(err)
	}
	var access bytes.Buffer
	if _, err := runWorkspaceManagement(ctx, []string{"workspaces", "access", string(notes.ID), "--expect", "0", "--set", "conn-notes=notebook:inbox,notebook:digest"}, &access, c.store); err != nil {
		t.Fatal(err)
	}
	card := c.mustRun("workflows", "propose", string(notes.ID), notesDigestDir, "--fixtures", writeNotesFixtures(t))
	m := approveLine.FindStringSubmatch(card)
	c.mustRun("workflows", "approve", m[1], "--hash", m[2], "--authority", m[3], "--expect-version", m[4])
	// Its AI step needs remote workflow models, which this Evie has off
	// (Stage 14 sub-stage 7).
	if _, err := c.run("workflows", "run", string(notes.ID), "notes-digest", "--input", "day=2026-10-05"); err == nil || !strings.Contains(err.Error(), "runs AI steps, which are unavailable") {
		t.Fatalf("AI steps without remote models: %v", err)
	}
	if listed := c.mustRun("workflows", "runs", string(notes.ID)); !strings.Contains(listed, "No workflow runs") {
		t.Fatalf("refused start recorded a run:\n%s", listed)
	}
}

func TestWorkflowCLIRunUsage(t *testing.T) {
	c := newWorkflowCLI(t)
	inputs := filepath.Join(t.TempDir(), "inputs.json")
	if err := os.WriteFile(inputs, []byte(`{"a": 1, "a": 2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"workflows", "run"},
		{"workflows", "run", "ws"},
		{"workflows", "run", "ws", "name", "extra"},
		{"workflows", "run", "ws", "name", "--input", "novalue"},
		{"workflows", "run", "ws", "name", "--input", "=x"},
		{"workflows", "run", "ws", "name", "--input", "a=1", "--input", "a=2"},
		{"workflows", "run", "ws", "name", "--input-json", inputs, "--input-json", inputs},
		{"workflows", "run", "ws", "name", "--input"},
		{"workflows", "runs", "a", "b"},
		{"workflows", "runs", "--workflow"},
		{"workflows", "inspect"},
		{"workflows", "inspect", "a", "b"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if _, err := c.run("workflows", "run", "ws", "name", "--input-json", inputs); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("repeated input names in a file: %v", err)
	}
	// Run commands need the database; the review handler leaves them.
	var out bytes.Buffer
	for _, sub := range []string{"run", "runs", "inspect"} {
		if handled, _ := runWorkflowManagement(context.Background(), []string{"workflows", sub}, &out, func(string) string { return "" }); handled {
			t.Fatalf("%s was answered before the database opened", sub)
		}
	}
}

// A detached run is recorded and shown at once, queued for a long-lived
// Evie process; inspection says nobody has claimed it yet. A process with a
// scheduler then finishes it, and inspection shows the released lease.
func TestWorkflowCLIRunDetachRecordsTheRunForTheScheduler(t *testing.T) {
	c := newWorkflowCLI(t)
	w := c.approveSplitBill()
	out := c.mustRun("workflows", "run", "--detach", string(w.ID), "split-bill", "--input", "bill_id=dinner-1", "--input", "total_cents=1000", "--input", "people=3")
	for _, want := range []string{"queued", "Detached:", "evie serve", "Lease: not claimed yet"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detached run output lacks %q:\n%s", want, out)
		}
	}
	m := runIDLine.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no run ID:\n%s", out)
	}
	stop := c.rt.Start(context.Background())
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(c.mustRun("workflows", "inspect", m[1]), "succeeded") {
		if time.Now().After(deadline) {
			t.Fatal("the scheduler did not finish the detached run")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if inspected := c.mustRun("workflows", "inspect", m[1]); !strings.Contains(inspected, "Lease: released by "+c.rt.Holder()) {
		t.Fatalf("inspect after the scheduler:\n%s", inspected)
	}
	if _, err := c.run("workflows", "run", "--detach", "--detach", string(w.ID), "split-bill"); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("repeated --detach: %v", err)
	}
}

// The scheduler's worker bound comes from EVIE_WORKFLOW_WORKERS; a bad
// value is refused rather than ignored.
func TestWorkflowSchedulerWorkersFromEnvironment(t *testing.T) {
	for value, ok := range map[string]bool{"": true, "1": true, "4": true, "32": true, "0": false, "33": false, "-1": false, "two": false} {
		_, err := workflowSchedulerConfig(func(name string) string {
			if name == "EVIE_WORKFLOW_WORKERS" {
				return value
			}
			return ""
		})
		if (err == nil) != ok {
			t.Fatalf("EVIE_WORKFLOW_WORKERS=%q: %v", value, err)
		}
	}
	config, err := workflowSchedulerConfig(func(name string) string {
		if name == "EVIE_WORKFLOW_WORKERS" {
			return "4"
		}
		return ""
	})
	if err != nil || config.Workers != 4 {
		t.Fatalf("config = %+v, %v", config, err)
	}
}

// The owner approves a compatible provider substitution for a paused run
// with `evie workflows substitute <run-id> <capability> <version>`; the
// runtime refuses one the run's pins and the provider's evidence do not
// permit.
func TestWorkflowCLISubstitute(t *testing.T) {
	c := newWorkflowCLI(t)
	w := c.approveSplitBill()
	out := c.mustRun("workflows", "run", "--detach", string(w.ID), "split-bill", "--input", "bill_id=dinner-1", "--input", "total_cents=1000", "--input", "people=3")
	m := runIDLine.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no run ID:\n%s", out)
	}
	if _, err := c.run("workflows", "substitute", m[1], "finance.month_cash_flow", "1.2.0"); err == nil || !strings.Contains(err.Error(), "pins no provider of finance.month_cash_flow") {
		t.Fatalf("a substitution for a run without that provider: %v", err)
	}
	for _, args := range [][]string{
		{"workflows", "substitute"},
		{"workflows", "substitute", m[1], "finance.month_cash_flow"},
		{"workflows", "substitute", m[1], "finance.month_cash_flow", "1.2.0", "extra"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
}

// The production Workflow Runtime reaches the workflow capabilities
// compiled into Evie through a Plugin Manager that follows the owner's
// durable plugin choices: disabling Finance makes its reads unavailable.
func TestWorkflowRuntimeUsesTheCompiledWorkflowCapabilities(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	manager, err := newWorkflowPluginManager(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if contract, ok := manager.WorkflowContract("finance.month_cash_flow"); !ok || contract.Effect {
		t.Fatalf("compiled contract = %+v, %v", contract, ok)
	}
	if _, err := manager.ResolveWorkflowCapability(ctx, "finance.month_cash_flow"); err != nil {
		t.Fatalf("finance is enabled by default: %v", err)
	}
	if _, err := store.SetPluginEnabled(ctx, "finance", false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveWorkflowCapability(ctx, "finance.month_cash_flow"); !errors.Is(err, plugins.ErrWorkflowCapabilityUnavailable) {
		t.Fatalf("after the owner disabled finance: %v", err)
	}
	rt, err := newWorkflowRuntime(store, func(string) string { return "" }, manager)
	if err != nil || rt == nil {
		t.Fatalf("runtime = %v, %v", rt, err)
	}
}
