package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Built-in workflows (owner decision 2026-10-04) from the owner's CLI: the
// built-in deep-research is listed and runnable without a proposal, the
// owner disables and enables it, customizes it for one Workspace or
// Globally through the normal approval, and reviews Global definitions.

func TestWorkflowCLIBuiltinListDisableAndEnable(t *testing.T) {
	ctx := context.Background()
	c := newWorkflowCLI(t)
	w, err := c.store.RegisterWorkspace(ctx, "Research")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"workflows", "builtin"},
		{"workflows", "builtin", "list", "extra"},
		{"workflows", "builtin", "disable"},
		{"workflows", "builtin", "toggle", "deep-research"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	list := c.mustRun("workflows", "builtin", "list", "--workspace", string(w.ID))
	for _, want := range []string{"deep-research  enabled", "not run yet", "workers research", "at most 6000000 tokens",
		"in Workspace " + string(w.ID) + ": runs the built-in", "turns on research delegation in the Workspace's settings", "without an approval"} {
		if !strings.Contains(list, want) {
			t.Fatalf("list lacks %q:\n%s", want, list)
		}
	}
	off := c.mustRun("workflows", "builtin", "disable", "deep-research", "--reason", "trying my own")
	if !strings.Contains(off, "Disabled the built-in deep-research for every Workspace") {
		t.Fatalf("disable:\n%s", off)
	}
	if _, err := c.run("workflows", "builtin", "disable", "deep-research"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("disabling twice: %v", err)
	}
	if list := c.mustRun("workflows", "builtin", "list"); !strings.Contains(list, "deep-research  disabled (trying my own)") {
		t.Fatalf("list while disabled:\n%s", list)
	}
	// A run of a disabled built-in is refused with how to enable it.
	if _, err := c.run("workflows", "run", string(w.ID), "deep-research", "--input", "question=Q"); err == nil || !strings.Contains(err.Error(), "evie workflows builtin enable deep-research") {
		t.Fatalf("run while disabled: %v", err)
	}
	if on := c.mustRun("workflows", "builtin", "enable", "deep-research"); !strings.Contains(on, "Enabled the built-in deep-research") {
		t.Fatalf("enable:\n%s", on)
	}
	var listed struct {
		Builtins []workflowruntime.BuiltinView `json:"builtins"`
	}
	if err := json.Unmarshal([]byte(c.mustRun("workflows", "builtin", "list", "--json")), &listed); err != nil || len(listed.Builtins) != 1 ||
		!listed.Builtins[0].Enabled || listed.Builtins[0].Control == nil || !listed.Builtins[0].Control.Enabled {
		t.Fatalf("list --json = %+v, %v", listed, err)
	}
	if _, err := c.run("workflows", "builtin", "disable", "no-such"); err == nil || !strings.Contains(err.Error(), "no built-in workflow") {
		t.Fatalf("disabling an unknown built-in: %v", err)
	}
}

func TestWorkflowCLICustomizesABuiltinForAWorkspaceAndGlobally(t *testing.T) {
	ctx := context.Background()
	c := newWorkflowCLI(t)
	w, err := c.store.RegisterWorkspace(ctx, "Research")
	if err != nil {
		t.Fatal(err)
	}
	if w, err = c.store.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, true); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"workflows", "customize", "deep-research"},
		{"workflows", "customize", "deep-research", "--workspace", string(w.ID), "--global"},
		{"workflows", "customize", "--global"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	card := c.mustRun("workflows", "customize", "deep-research", "--workspace", string(w.ID))
	for _, want := range []string{"Workflow deep-research in Workspace " + string(w.ID), "Template: deep-research", "Dry run: succeeded", "To approve:"} {
		if !strings.Contains(card, want) {
			t.Fatalf("Workspace card lacks %q:\n%s", want, card)
		}
	}
	global := c.mustRun("workflows", "customize", "deep-research", "--global")
	for _, want := range []string{"Global workflow deep-research", "Requested Standing Authority: none", "Dry run: succeeded", "To approve:"} {
		if !strings.Contains(global, want) {
			t.Fatalf("Global card lacks %q:\n%s", want, global)
		}
	}
	m := approveLine.FindStringSubmatch(global)
	if m == nil {
		t.Fatalf("Global card has no approve command:\n%s", global)
	}
	approved := c.mustRun("workflows", "approve", m[1], "--hash", m[2], "--authority", m[3], "--expect-version", m[4])
	if !strings.Contains(approved, "Activated Global deep-research version 1") || !strings.Contains(approved, "No run was started") {
		t.Fatalf("approve:\n%s", approved)
	}
	if list := c.mustRun("workflows", "global", "list"); !strings.Contains(list, "global  deep-research  active  v1") {
		t.Fatalf("global list:\n%s", list)
	}
	if show := c.mustRun("workflows", "global", "show", "deep-research"); !strings.Contains(show, "Global workflow deep-research") || !strings.Contains(show, "Version 1") {
		t.Fatalf("global show:\n%s", show)
	}
	if list := c.mustRun("workflows", "builtin", "list", "--workspace", string(w.ID)); !strings.Contains(list, "overridden everywhere by the Global copy, version 1") ||
		!strings.Contains(list, "runs the Global copy version 1") {
		t.Fatalf("builtin list with a Global copy:\n%s", list)
	}
	if out := c.mustRun("workflows", "global", "deactivate", "deep-research", "--version", "1"); !strings.Contains(out, "Deactivated Global deep-research version 1") {
		t.Fatalf("global deactivate:\n%s", out)
	}
	if list := c.mustRun("workflows", "global", "list"); !strings.Contains(list, "inactive") {
		t.Fatalf("global list after deactivation:\n%s", list)
	}
}
