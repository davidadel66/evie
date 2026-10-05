package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/workflow"
)

// Stage 14 sub-stage 10 (2026-10-04): bundled templates. `evie workflows
// templates` lists them without a database, and `evie workflows propose
// <workspace-id> --template <name>` proposes one into the Workspace's
// procedural repository with its bundled dry run; the owner's normal
// approval follows.

func TestWorkflowCLITemplatesListsBundledTemplates(t *testing.T) {
	var out bytes.Buffer
	handled, err := runWorkflowManagement(context.Background(), []string{"workflows", "templates"}, &out, func(string) string { return "" })
	if !handled || err != nil {
		t.Fatalf("templates: %v %v", handled, err)
	}
	for _, want := range []string{"deep-research (built in)", "cited report", "question (string, required)", "evie workflows customize deep-research --workspace <workspace-id> | --global", "without an approval"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("templates lacks %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	if _, err := runWorkflowManagement(context.Background(), []string{"workflows", "templates", "--json"}, &out, func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Templates []workflow.TemplateInfo `json:"templates"`
	}
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil || len(listed.Templates) != 1 || listed.Templates[0].Name != "deep-research" {
		t.Fatalf("templates --json = %s (%v)", out.String(), err)
	}
	if _, err := runWorkflowManagement(context.Background(), []string{"workflows", "templates", "extra"}, &out, func(string) string { return "" }); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("templates with an argument: %v", err)
	}
}

func TestWorkflowCLIProposesTheDeepResearchTemplate(t *testing.T) {
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
		{"workflows", "propose", string(w.ID)},
		{"workflows", "propose", string(w.ID), "some/dir", "--template", "deep-research"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if _, err := c.run("workflows", "propose", string(w.ID), "--template", "nope"); err == nil || !strings.Contains(err.Error(), "no bundled workflow template") {
		t.Fatalf("unknown template: %v", err)
	}
	card := c.mustRun("workflows", "propose", string(w.ID), "--template", "deep-research")
	for _, want := range []string{"Workflow deep-research in Workspace " + string(w.ID), "Template: deep-research (bundled with Evie)", "Dry run: succeeded",
		"Requested Standing Authority: none", "Workers: research", "To approve:"} {
		if !strings.Contains(card, want) {
			t.Fatalf("card lacks %q:\n%s", want, card)
		}
	}
	m := approveLine.FindStringSubmatch(card)
	if m == nil {
		t.Fatalf("card has no approve command:\n%s", card)
	}
	approved := c.mustRun("workflows", "approve", m[1], "--hash", m[2], "--authority", m[3], "--expect-version", m[4])
	if !strings.Contains(approved, "Activated deep-research version 1") || !strings.Contains(approved, "No run was started") {
		t.Fatalf("approve output:\n%s", approved)
	}
	// Proposing the same template again is the active version: no approval.
	again := c.mustRun("workflows", "propose", string(w.ID), "--template", "deep-research")
	if !strings.Contains(again, "No approval needed") {
		t.Fatalf("second proposal:\n%s", again)
	}
}

// `evie workflows report <run-id> <execution-id>` reads a run's worker
// report from the report store; it refuses an execution that is not the
// run's.
func TestWorkflowCLIReportNeedsTheRunsExecution(t *testing.T) {
	c := newWorkflowCLI(t)
	if _, err := c.run("workflows", "report", "run-1"); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("report without an execution: %v", err)
	}
	if _, err := c.run("workflows", "report", "run-1", "exec-1"); err == nil || !strings.Contains(err.Error(), "no delegated execution") {
		t.Fatalf("report of an unknown execution: %v", err)
	}
}

// The chat's workflow tools are on by default: they start only workflows
// the owner approved or built into Evie, inside the gates every start has.
// Since the built-in decision (2026-10-04) the Subagents plugin is on by
// default too, so the built-in deep-research's research workers can run
// once the Workspace allows research.
func TestWorkflowsPluginIsEnabledByDefault(t *testing.T) {
	if !defaultPluginEnabled[plugins.WorkflowsPluginID] {
		t.Fatal("the Workflows plugin is off by default")
	}
	if !defaultPluginEnabled[plugins.SubagentsPluginID] || !defaultPluginEnabled[plugins.WebPluginID] {
		t.Fatal("the built-in deep-research's workers are off by default")
	}
}
