package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

func notesTestCatalog() workflow.Catalog {
	note := workflow.ObjectType(workflow.Field{Name: "id", Type: workflow.StringType()}, workflow.Field{Name: "text", Type: workflow.StringType()})
	return workflow.NewCatalog(
		workflow.CapabilityContract{ID: "notes.list", Contract: "1.0.0",
			Input:  workflow.ObjectType(workflow.Field{Name: "day", Type: workflow.StringType()}),
			Output: workflow.ObjectType(workflow.Field{Name: "notes", Type: workflow.ArrayType(note)})},
		workflow.CapabilityContract{ID: "notes.append", Contract: "1.0.0", Effect: true, Idempotent: true,
			Input:  workflow.ObjectType(workflow.Field{Name: "day", Type: workflow.StringType()}, workflow.Field{Name: "text", Type: workflow.StringType()}),
			Output: workflow.ObjectType(workflow.Field{Name: "entry_id", Type: workflow.StringType()})},
	)
}

type workflowCLI struct {
	t     *testing.T
	store *eviedb.Store
	rt    *workflowruntime.Runtime
}

func newWorkflowCLI(t *testing.T) *workflowCLI {
	t.Helper()
	store := backgroundTestStore(t)
	rt, err := workflowruntime.New(workflowruntime.Config{
		Store: store, Repo: procedural.New(filepath.Join(t.TempDir(), "procedural")), Catalog: notesTestCatalog(),
		Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &workflowCLI{t: t, store: store, rt: rt}
}

func (c *workflowCLI) run(args ...string) (string, error) {
	c.t.Helper()
	var out bytes.Buffer
	handled, err := runWorkflowActivation(context.Background(), args, &out, c.rt)
	if !handled {
		c.t.Fatalf("%q was not handled", args)
	}
	return out.String(), err
}

func (c *workflowCLI) mustRun(args ...string) string {
	c.t.Helper()
	out, err := c.run(args...)
	if err != nil {
		c.t.Fatalf("%q: %v\n%s", args, err, out)
	}
	return out
}

func writeNotesFixtures(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixtures.json")
	if err := os.WriteFile(path, []byte(`{"inputs": {"day": "2026-10-05"}, "steps": {
		"read": {"output": {"notes": [{"id": "n1", "text": "Call the bank."}]}},
		"summarize": {"output": {"digest": "- Call the bank."}},
		"write": {"output": {"entry_id": "e1"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

var approveLine = regexp.MustCompile(`evie workflows approve (\S+) --hash (\S+) --authority (\S+) --expect-version (\d+)`)

// The owner's CLI flow: set Workspace Access, propose with a dry run, review
// the card, approve exactly what it showed, list, show and deactivate.
func TestWorkflowCLIProposeApproveDeactivate(t *testing.T) {
	ctx := context.Background()
	c := newWorkflowCLI(t)
	w, err := c.store.RegisterWorkspace(ctx, "Notes")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if handled, err := runWorkspaceManagement(ctx, []string{"workspaces", "access", string(w.ID), "--expect", "0", "--set", "conn-notes=notebook:inbox,notebook:digest"}, &out, c.store); !handled || err != nil {
		t.Fatalf("set access: %v %v %s", handled, err, out.String())
	}
	if !strings.Contains(out.String(), `"revision": 1`) || !strings.Contains(out.String(), "notebook:digest") {
		t.Fatalf("access = %s", out.String())
	}

	card := c.mustRun("workflows", "propose", string(w.ID), notesDigestDir, "--fixtures", writeNotesFixtures(t))
	for _, want := range []string{"Workflow notes-digest in Workspace " + string(w.ID), "Requested Standing Authority", "Dry run: succeeded", "To approve:"} {
		if !strings.Contains(card, want) {
			t.Fatalf("card lacks %q:\n%s", want, card)
		}
	}
	m := approveLine.FindStringSubmatch(card)
	if m == nil {
		t.Fatalf("card has no approve command:\n%s", card)
	}
	id, hash, authority, expect := m[1], m[2], m[3], m[4]
	if diff := c.mustRun("workflows", "diff", id); !strings.Contains(diff, hash) || !strings.Contains(diff, "b/workflow.yaml") {
		t.Fatalf("diff:\n%s", diff)
	}
	if listed := c.mustRun("workflows", "list"); !strings.Contains(listed, "notes-digest") || !strings.Contains(listed, "proposed") {
		t.Fatalf("list before approval:\n%s", listed)
	}

	for _, args := range [][]string{
		{"workflows", "approve", id},
		{"workflows", "approve", id, "--hash", hash, "--authority", authority},
		{"workflows", "approve", id, "--hash", hash, "--expect-version", expect},
		{"workflows", "approve", "--hash", hash, "--authority", authority, "--expect-version", expect},
		{"workflows", "approve", id, "extra", "--hash", hash, "--authority", authority, "--expect-version", expect},
		{"workflows", "approve", id, "--hash", hash, "--authority", authority, "--expect-version", "zero"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if _, err := c.run("workflows", "approve", id, "--hash", "sha256:"+strings.Repeat("0", 64), "--authority", authority, "--expect-version", expect); err == nil {
		t.Fatal("a hash the owner did not review was approved")
	}
	approved := c.mustRun("workflows", "approve", id, "--hash", hash, "--authority", authority, "--expect-version", expect)
	if !strings.Contains(approved, "Activated notes-digest version 1") || !strings.Contains(approved, "No run was started") || !strings.Contains(approved, "schedule") {
		t.Fatalf("approve output:\n%s", approved)
	}

	raw := c.mustRun("workflows", "list", string(w.ID), "--json")
	var listed struct {
		Workflows []eviedb.WorkflowDefinitionState `json:"workflows"`
	}
	if err := json.Unmarshal([]byte(raw), &listed); err != nil || len(listed.Workflows) != 1 || listed.Workflows[0].Status != eviedb.WorkflowActive {
		t.Fatalf("list --json = %s (%v)", raw, err)
	}
	shown := c.mustRun("workflows", "show", string(w.ID), "notes-digest")
	for _, want := range []string{"Status: active", "Version 1", hash, "notes.append on conn-notes", "approved by owner via cli"} {
		if !strings.Contains(shown, want) {
			t.Fatalf("show lacks %q:\n%s", want, shown)
		}
	}
	if _, err := c.run("workflows", "deactivate", string(w.ID), "notes-digest"); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("deactivate without a version: %v", err)
	}
	if _, err := c.run("workflows", "deactivate", string(w.ID), "notes-digest", "--version", "2"); err == nil {
		t.Fatal("a stale version was deactivated")
	}
	deactivated := c.mustRun("workflows", "deactivate", string(w.ID), "notes-digest", "--version", "1", "--reason", "retired")
	if !strings.Contains(deactivated, "Deactivated notes-digest version 1") {
		t.Fatalf("deactivate output:\n%s", deactivated)
	}
	if shown := c.mustRun("workflows", "show", string(w.ID), "notes-digest"); !strings.Contains(shown, "Status: inactive") || !strings.Contains(shown, "retired") {
		t.Fatalf("show after deactivation:\n%s", shown)
	}
}

// Narrowing Workspace Access from the CLI blocks the approved definition,
// and the CLI shows why.
func TestWorkflowCLIShowsBlockedDefinitions(t *testing.T) {
	ctx := context.Background()
	c := newWorkflowCLI(t)
	w, err := c.store.RegisterWorkspace(ctx, "Notes")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := runWorkspaceManagement(ctx, []string{"workspaces", "access", string(w.ID), "--expect", "0", "--set", "conn-notes=notebook:inbox,notebook:digest"}, &out, c.store); err != nil {
		t.Fatal(err)
	}
	card := c.mustRun("workflows", "propose", string(w.ID), notesDigestDir, "--fixtures", writeNotesFixtures(t))
	m := approveLine.FindStringSubmatch(card)
	c.mustRun("workflows", "approve", m[1], "--hash", m[2], "--authority", m[3], "--expect-version", m[4])
	out.Reset()
	if _, err := runWorkspaceManagement(ctx, []string{"workspaces", "access", string(w.ID), "--expect", "1", "--set", "conn-notes=notebook:inbox"}, &out, c.store); err != nil {
		t.Fatal(err)
	}
	if listed := c.mustRun("workflows", "list"); !strings.Contains(listed, "blocked") || !strings.Contains(listed, "notebook:digest") {
		t.Fatalf("list after narrowing:\n%s", listed)
	}
	if _, err := runWorkspaceManagement(ctx, []string{"workspaces", "access", string(w.ID), "--expect", "1", "--clear"}, &out, c.store); err == nil {
		t.Fatal("a stale access revision was written")
	}
	for _, args := range [][]string{
		{"workspaces", "access", string(w.ID), "--set", "conn-notes=x"},
		{"workspaces", "access", string(w.ID), "--expect", "2", "--set", "conn-notes"},
		{"workspaces", "access", string(w.ID), "--expect", "2"},
		{"workspaces", "access", string(w.ID), "--expect", "2", "--clear", "--set", "c=r"},
	} {
		if _, err := runWorkspaceManagement(ctx, args, &out, c.store); err == nil {
			t.Fatalf("%q succeeded", args)
		}
	}
	out.Reset()
	if _, err := runWorkspaceManagement(ctx, []string{"workspaces", "access", string(w.ID)}, &out, c.store); err != nil || !strings.Contains(out.String(), `"revision": 2`) {
		t.Fatalf("show access: %v %s", err, out.String())
	}
}

func TestWorkflowCLIActivationUsage(t *testing.T) {
	c := newWorkflowCLI(t)
	var out bytes.Buffer
	if handled, err := runWorkflowActivation(context.Background(), []string{"workspaces", "list"}, &out, c.rt); handled || err != nil {
		t.Fatalf("other command: handled %v err %v", handled, err)
	}
	for _, args := range [][]string{
		{"workflows", "propose"},
		{"workflows", "propose", "ws"},
		{"workflows", "show", "ws"},
		{"workflows", "diff"},
		{"workflows", "list", "a", "b"},
		{"workflows", "deactivate", "ws", "name", "--version", "x"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	// Review commands that need no database stay with sub-stage 1's handler;
	// activation commands are not answered there.
	for _, sub := range []string{"propose", "list", "show", "diff", "approve", "deactivate"} {
		if handled, _ := runWorkflowManagement(context.Background(), []string{"workflows", sub}, &out, func(string) string { return "" }); handled {
			t.Fatalf("%s was answered before the database opened", sub)
		}
	}
}

// Only the owner approves workflows: no model-facing tool of the kernel or
// any compiled plugin proposes, approves or deactivates one.
func TestWorkflowApprovalHasNoModelTool(t *testing.T) {
	store := backgroundTestStore(t)
	policy, err := configuredSubagentPolicy(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := subagents.New(store, policy)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, schema := range tools.KernelToolset().Schemas() {
		names = append(names, schema.Function.Name)
	}
	for _, provider := range []interface {
		ToolCapabilities() []plugins.ToolCapability
	}{
		plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(store), plugins.NewMemory(store), plugins.NewSubagents(supervisor),
	} {
		for _, capability := range provider.ToolCapabilities() {
			names = append(names, capability.Tool.Schema.Function.Name, string(capability.ID))
		}
	}
	if len(names) < 10 {
		t.Fatalf("only %d tools found", len(names))
	}
	for _, name := range names {
		if strings.Contains(strings.ToLower(name), "workflow") {
			t.Fatalf("model-facing tool %s reaches workflows", name)
		}
	}
}
