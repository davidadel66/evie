package workflow

import (
	"reflect"
	"strings"
	"testing"
)

// Stage 14 sub-stage 10 (2026-10-04): reviewed definitions bundled into the
// binary as templates. Each validates against the operator's default worker
// ceiling, its dry run with the bundled fixtures succeeds, and only its
// reviewed files would be committed.

func TestWorkflowTemplatesListTheBundledDefinitions(t *testing.T) {
	list := Templates()
	if len(list) != 1 || list[0].Name != "deep-research" || !strings.Contains(list[0].Description, "cited report") {
		t.Fatalf("templates = %+v", list)
	}
	if _, ok := TemplateSource("no-such"); ok {
		t.Fatal("an unknown template resolved")
	}
	if _, err := TemplateFixtures("no-such"); err == nil {
		t.Fatal("an unknown template has fixtures")
	}
}

func TestWorkflowDeepResearchTemplateValidatesAndSimulates(t *testing.T) {
	src, ok := TemplateSource("deep-research")
	if !ok {
		t.Fatal("deep-research template missing")
	}
	env := Environment{Workers: WorkerCeiling{Deadline: testEnvironment().Workers.Deadline, Tokens: testEnvironment().Workers.Tokens}}
	v, err := Validate(src, env)
	if err != nil {
		t.Fatal(err)
	}
	report := v.Report()
	if len(report.Warnings) != 0 {
		t.Fatalf("warnings: %v", report.Warnings)
	}
	files := []string{}
	for name := range v.Files() {
		files = append(files, name)
	}
	want := []string{"README.md", "prompts/plan.md", "prompts/research.md", "prompts/synthesize.md", "prompts/verify.md",
		"schemas/angles.schema.json", "schemas/findings.schema.json", "schemas/report.schema.json", "schemas/verdict.schema.json", "workflow.yaml"}
	if !reflect.DeepEqual(sortedStrings(files), want) {
		t.Fatalf("committed files = %v", sortedStrings(files))
	}
	// Conservative defaults: at most four angles, twelve claims, three votes,
	// one pinned model, and a token limit that covers every worker's budget.
	if l := report.Limits; l.MaxTokens < report.WorstCase.AgentTokens || l.MaxParallel != 4 {
		t.Fatalf("limits %+v worst case %+v", l, report.WorstCase)
	}
	for _, p := range report.ModelPolicies {
		if p.Model != "deepseek/deepseek-v4.1-flash" {
			t.Fatalf("policy %s uses %s", p.Name, p.Model)
		}
	}
	for _, w := range report.Workers {
		switch w.Step {
		case "research":
			if w.MaxItems != 4 {
				t.Fatalf("research fan-out %+v", w)
			}
		case "verify":
			if w.MaxItems != 36 {
				t.Fatalf("verify fan-out %+v", w)
			}
		}
	}
	functions := []string{}
	for _, f := range report.Functions {
		functions = append(functions, f.ID)
	}
	if !reflect.DeepEqual(functions, []string{"check_citations@1", "collect_claims@1", "expand_ballots@1", "tally_verdicts@1"}) {
		t.Fatalf("functions = %v", functions)
	}
	fixtures, err := TemplateFixtures("deep-research")
	if err != nil {
		t.Fatal(err)
	}
	s, err := v.Simulate(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != SimulationSucceeded {
		t.Fatalf("dry run %s: %s\n%s", s.Status, s.Error, s.Text())
	}
	report2 := s.Outputs["report"].(map[string]any)
	if report2["status"] != "verified" || len(report2["citations"].([]any)) == 0 {
		t.Fatalf("dry-run report = %v", report2)
	}
	// Every citation the dry run keeps is a verifier's fetched evidence.
	for _, c := range report2["citations"].([]any) {
		url := c.(map[string]any)["url"].(string)
		if !strings.HasPrefix(url, "https://example.org/evidence/") {
			t.Fatalf("citation %s is not verifier evidence", url)
		}
	}
}

// The template's executable hash is pinned: changing a prompt, schema or
// the graph must be deliberate, and changes what an owner approves.
func TestWorkflowDeepResearchTemplateHashIsPinned(t *testing.T) {
	src, _ := TemplateSource("deep-research")
	v, err := Validate(src, Environment{Workers: WorkerCeiling{Deadline: testEnvironment().Workers.Deadline, Tokens: testEnvironment().Workers.Tokens}})
	if err != nil {
		t.Fatal(err)
	}
	const pinned = "sha256:e40aefece53eac2eaf5ba61cef5ae4c3d0386fcb04730d6804a5749b23ef34cb"
	if v.Hash() != pinned {
		t.Fatalf("deep-research template executable hash = %s, pinned %s", v.Hash(), pinned)
	}
}
