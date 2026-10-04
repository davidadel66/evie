package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

var (
	deepResearchDir      = filepath.Join("..", "..", "internal", "workflow", "testdata", "deep-research")
	deepResearchFixtures = filepath.Join("..", "..", "internal", "workflow", "testdata", "deep-research.fixtures.json")
	notesDigestDir       = filepath.Join("..", "..", "internal", "workflow", "testdata", "notes-digest")
)

func runWorkflowsCLI(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	handled, err := runWorkflowManagement(context.Background(), args, &out, func(k string) string { return env[k] })
	if !handled {
		t.Fatalf("%v was not handled", args)
	}
	return out.String(), err
}

func TestWorkflowCLIIgnoresOtherCommands(t *testing.T) {
	var out bytes.Buffer
	if handled, err := runWorkflowManagement(context.Background(), []string{"worktrees", "list"}, &out, func(string) string { return "" }); handled || err != nil {
		t.Fatalf("handled %v err %v", handled, err)
	}
}

func TestWorkflowCLIValidatePrintsTheReview(t *testing.T) {
	text, err := runWorkflowsCLI(t, nil, "workflows", "validate", deepResearchDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Workflow deep-research (schema 1)", "Executable hash: sha256:", "Graph (start: plan)", "Requested Standing Authority: none"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output lacks %q:\n%s", want, text)
		}
	}
	raw, err := runWorkflowsCLI(t, nil, "workflows", "validate", "--json", deepResearchDir)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Valid  bool `json:"valid"`
		Report struct {
			Workflow string `json:"workflow"`
			Hash     string `json:"executable_hash"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	if !report.Valid || report.Report.Workflow != "deep-research" || !strings.HasPrefix(report.Report.Hash, "sha256:") {
		t.Fatalf("JSON report = %s", raw)
	}
}

// The operator's subagent ceiling bounds agent budgets.
func TestWorkflowCLIUsesTheOperatorWorkerCeiling(t *testing.T) {
	text, err := runWorkflowsCLI(t, map[string]string{"EVIE_SUBAGENTS_DEADLINE": "4m"}, "workflows", "validate", deepResearchDir)
	if err == nil || !strings.Contains(text, "steps.research.item.budget.time") {
		t.Fatalf("err %v output:\n%s", err, text)
	}
}

// The CLI reviews against the compiled workflow capabilities, so a
// capability no plugin provides (the test-only notes.list) is reported.
func TestWorkflowCLIValidateReportsProblems(t *testing.T) {
	text, err := runWorkflowsCLI(t, nil, "workflows", "validate", notesDigestDir)
	if err == nil {
		t.Fatalf("notes-digest validated in the CLI:\n%s", text)
	}
	if !strings.Contains(text, "steps.read.capability") || !strings.Contains(text, "not in the capability catalog") {
		t.Fatalf("output:\n%s", text)
	}
	raw, err := runWorkflowsCLI(t, nil, "workflows", "validate", "--json", notesDigestDir)
	if err == nil {
		t.Fatal("invalid definition returned no error")
	}
	var report struct {
		Valid    bool `json:"valid"`
		Problems []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"problems"`
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil || report.Valid || len(report.Problems) == 0 {
		t.Fatalf("JSON problems = %s (%v)", raw, err)
	}
}

var (
	sharedSpendingDir      = filepath.Join("..", "..", "internal", "workflow", "testdata", "shared-spending")
	sharedSpendingFixtures = filepath.Join("..", "..", "internal", "workflow", "testdata", "shared-spending.fixtures.json")
)

// Standalone review uses the workflow capabilities compiled into Evie (Stage
// 14 sub-stage 5): a definition that reads finance.month_cash_flow
// validates and simulates. Workspace Access exists only in a Workspace, so
// it is checked when the definition is proposed there.
func TestWorkflowCLIReviewsCapabilityReadsAgainstTheCompiledCatalog(t *testing.T) {
	text, err := runWorkflowsCLI(t, nil, "workflows", "validate", sharedSpendingDir)
	if err != nil {
		t.Fatalf("%v:\n%s", err, text)
	}
	for _, want := range []string{"finance.month_cash_flow 1.0.0: read", "finance.month_cash_flow on finance-local: ledger", "Workspace Access is not checked outside a Workspace"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output lacks %q:\n%s", want, text)
		}
	}
	sim, err := runWorkflowsCLI(t, nil, "workflows", "simulate", sharedSpendingDir, "--fixtures", sharedSpendingFixtures)
	if err != nil || !strings.Contains(sim, "Status: succeeded") {
		t.Fatalf("simulation %v:\n%s", err, sim)
	}
	raw, err := runWorkflowsCLI(t, nil, "workflows", "validate", "--json", sharedSpendingDir)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(raw), "{") || !strings.Contains(raw, `"valid": true`) {
		t.Fatalf("JSON review %v:\n%s", err, raw)
	}
}

func TestWorkflowCLISimulatePrintsTheTrace(t *testing.T) {
	first, err := runWorkflowsCLI(t, nil, "workflows", "simulate", deepResearchDir, "--fixtures", deepResearchFixtures)
	if err != nil {
		t.Fatalf("%v:\n%s", err, first)
	}
	for _, want := range []string{"Simulation of deep-research", "Status: succeeded", "research #2 attempt 2", "Effects suppressed"} {
		if !strings.Contains(first, want) {
			t.Fatalf("output lacks %q:\n%s", want, first)
		}
	}
	second, _ := runWorkflowsCLI(t, nil, "workflows", "simulate", "--fixtures", deepResearchFixtures, deepResearchDir)
	if first != second {
		t.Fatal("simulation output differs between runs")
	}
	raw, err := runWorkflowsCLI(t, nil, "workflows", "simulate", "--json", deepResearchDir, "--fixtures", deepResearchFixtures)
	if err != nil {
		t.Fatal(err)
	}
	var sim struct {
		Status string `json:"status"`
		Trace  []any  `json:"trace"`
	}
	if err := json.Unmarshal([]byte(raw), &sim); err != nil || sim.Status != "succeeded" || len(sim.Trace) == 0 {
		t.Fatalf("JSON simulation = %s (%v)", raw, err)
	}
}

func TestWorkflowCLIUsage(t *testing.T) {
	for _, args := range [][]string{
		{"workflows"},
		{"workflows", "validate"},
		{"workflows", "simulate", deepResearchDir},
		{"workflows", "simulate", deepResearchDir, "--fixtures"},
		{"workflows", "execute", deepResearchDir},
		{"workflows", "validate", deepResearchDir, "extra"},
	} {
		text, err := runWorkflowsCLI(t, nil, args...)
		if err == nil || !strings.Contains(err.Error(), "usage: evie workflows") {
			t.Fatalf("%v: err %v output %s", args, err, text)
		}
	}
	if _, err := runWorkflowsCLI(t, nil, "workflows", "simulate", deepResearchDir, "--fixtures", "missing.json"); err == nil {
		t.Fatal("missing fixture file accepted")
	}
}
