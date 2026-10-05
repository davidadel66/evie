package workflow

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The built-in deep-research v2 definition (2026-10-04): scope, search,
// deduplicate, extract, rank, three adversarial verifiers per claim,
// tally, and a checked report or an honest inconclusive one. It validates
// against the operator's default worker ceiling with no warning, requests
// no Standing Authority, pins one model, and its dry run succeeds.

const builtinDeepResearch = "builtin/deep-research"

func builtinEnvironment() Environment {
	return Environment{Workers: WorkerCeiling{Deadline: testEnvironment().Workers.Deadline, Tokens: testEnvironment().Workers.Tokens}}
}

func validateBuiltinDeepResearch(t *testing.T) *Validated {
	t.Helper()
	v, err := Validate(DirSource{Dir: builtinDeepResearch}, builtinEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func builtinFixtures(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(builtinDeepResearch + "/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]any
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestWorkflowDeepResearchV2ValidatesWithinItsBounds(t *testing.T) {
	v := validateBuiltinDeepResearch(t)
	report := v.Report()
	if report.Workflow != "deep-research" || len(report.Warnings) != 0 {
		t.Fatalf("workflow %s, warnings %v", report.Workflow, report.Warnings)
	}
	for _, p := range report.ModelPolicies {
		if p.Model != "deepseek/deepseek-v4.1-flash" || p.Provider != "openrouter" {
			t.Fatalf("policy %s uses %s/%s", p.Name, p.Provider, p.Model)
		}
	}
	if len(report.ModelPolicies) != 5 {
		t.Fatalf("model policies = %+v", report.ModelPolicies)
	}
	var functions []string
	for _, f := range report.Functions {
		functions = append(functions, f.ID)
	}
	if want := []string{"check_citations@1", "dedup_sources@1", "expand_ballots@2", "rank_claims@1", "tally_adversarial@1"}; !reflect.DeepEqual(functions, want) {
		t.Fatalf("functions = %v", functions)
	}
	bounds := map[string]int{}
	for _, w := range report.Workers {
		if w.Worker != WorkerResearch {
			t.Fatalf("worker %+v is not a research worker", w)
		}
		bounds[w.Step] = w.MaxItems
	}
	if want := map[string]int{"search": 5, "extract": 15, "verify": 75}; !reflect.DeepEqual(bounds, want) {
		t.Fatalf("map bounds = %v", bounds)
	}
	l := report.Limits
	if report.WorstCase.AgentTokens > l.MaxTokens || l.MaxTokens != 6_000_000 || l.MaxParallel != 4 || report.WorstCase.AgentTokens != 5_800_000 {
		t.Fatalf("limits %+v, worst case %+v", l, report.WorstCase)
	}
	if report.WorstCase.Attempts > l.MaxAttempts {
		t.Fatalf("worst-case attempts %d above max_attempts %d", report.WorstCase.Attempts, l.MaxAttempts)
	}
	// Effect-free: no capability step, no Standing Authority, no schedule.
	if len(report.Authority) != 0 || report.Schedule != nil || len(report.Capabilities) != 0 || len(report.Resources) != 0 {
		t.Fatalf("authority %v schedule %v capabilities %v resources %v", report.Authority, report.Schedule, report.Capabilities, report.Resources)
	}
	for _, kind := range v.StepKinds() {
		if kind == KindCapability || kind == KindInterrupt {
			t.Fatalf("step kind %s in a built-in", kind)
		}
	}
	var files []string
	for name := range v.Files() {
		files = append(files, name)
	}
	want := []string{"README.md", "prompts/extract.md", "prompts/scope.md", "prompts/search.md", "prompts/synthesize.md", "prompts/verify.md",
		"schemas/angles.schema.json", "schemas/extraction.schema.json", "schemas/report.schema.json", "schemas/search.schema.json", "schemas/verdict.schema.json", "workflow.yaml"}
	if !reflect.DeepEqual(sortedStrings(files), want) {
		t.Fatalf("committed files = %v", sortedStrings(files))
	}
	if text := report.Text(); !strings.Contains(text, "partial (items may all fail)") || !strings.Contains(text, "partial (at least 1 must succeed)") {
		t.Fatalf("report text does not show the failure policies:\n%s", text)
	}
}

// The executable hash is pinned: a change to a prompt, schema, function or
// the graph is deliberate and makes a new built-in version.
func TestWorkflowDeepResearchV2HashIsPinned(t *testing.T) {
	const pinned = "sha256:a06c763a357abf77977f0ad505b73485e5d2d869ed2bb889dce8d86c684e21b2"
	if got := validateBuiltinDeepResearch(t).Hash(); got != pinned {
		t.Fatalf("built-in deep-research executable hash = %s, pinned %s", got, pinned)
	}
}

// Each worker's objective plus its largest possible assignment data fits
// the operator's default assignment limit (EVIE_SUBAGENTS_ASSIGNMENT_BYTES,
// 8 KiB), so no worker is refused for its size.
func TestWorkflowDeepResearchV2AssignmentsFitTheDefaultLimit(t *testing.T) {
	const assignmentBytes = 8192
	text := func(n int) string { return strings.Repeat("x", n) }
	question := text(2000)
	for _, tc := range []struct {
		objective string
		data      map[string]any
	}{
		{"prompts/search.md", map[string]any{"question": question, "label": text(60), "query": text(300), "rationale": text(300)}},
		{"prompts/extract.md", map[string]any{"question": question, "url": text(500), "title": text(300), "angle": text(60)}},
		{"prompts/verify.md", map[string]any{"question": question, "claim": text(400), "quote": text(600), "source_url": text(500),
			"source_quality": "unreliable", "publish_date": text(40), "voter": int64(3)}},
	} {
		objective, err := os.ReadFile(builtinDeepResearch + "/" + tc.objective)
		if err != nil {
			t.Fatal(err)
		}
		data, err := EncodeValue(tc.data)
		if err != nil {
			t.Fatal(err)
		}
		if size := len(objective) + len(data); size > assignmentBytes {
			t.Errorf("%s: objective %d + data %d = %d bytes; limit %d", tc.objective, len(objective), len(data), size, assignmentBytes)
		}
	}
}

func simulateBuiltin(t *testing.T, fixtures map[string]any) *Simulation {
	t.Helper()
	raw, err := json.Marshal(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseFixtures(raw)
	if err != nil {
		t.Fatal(err)
	}
	s, err := validateBuiltinDeepResearch(t).Simulate(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWorkflowDeepResearchV2DryRunWritesACheckedReport(t *testing.T) {
	s := simulateBuiltin(t, builtinFixtures(t))
	if s.Status != SimulationSucceeded {
		t.Fatalf("dry run %s: %s\n%s", s.Status, s.Error, s.Text())
	}
	if s.Outputs["outcome"] != "confirmed" {
		t.Fatalf("outcome = %v", s.Outputs["outcome"])
	}
	if _, ok := s.Outputs["inconclusive"]; ok {
		t.Fatal("a confirmed run produced the inconclusive report")
	}
	report := s.Outputs["report"].(map[string]any)
	if report["status"] != "verified" || len(report["citations"].([]any)) != 2 {
		t.Fatalf("report = %v", report)
	}
	for _, section := range []string{"## Findings", "## Caveats", "## Open questions", "Confidence: high"} {
		if !strings.Contains(report["body_markdown"].(string), section) {
			t.Fatalf("report lacks %q", section)
		}
	}
	stats := s.Outputs["stats"].(map[string]any)
	if stats["sources"] != int64(4) || stats["duplicates"] != int64(1) || stats["claims_verified"] != int64(4) || stats["refuted"] != int64(1) || stats["confirmed"] != int64(3) {
		t.Fatalf("stats = %v", stats)
	}
	confidence := map[string]string{}
	for _, c := range s.Outputs["confirmed_claims"].([]any) {
		m := c.(map[string]any)
		confidence[m["id"].(string)] = m["confidence"].(string)
	}
	// c1 primary 3-0 high; c2 secondary capped at medium; c4 primary 3-0.
	if want := map[string]string{"c1": "high", "c2": "medium", "c4": "high"}; !reflect.DeepEqual(confidence, want) {
		t.Fatalf("confidence = %v", confidence)
	}
	refuted := s.Outputs["refuted_claims"].([]any)
	if len(refuted) != 1 || refuted[0].(map[string]any)["counter_source"] != "https://example.org/evidence/range" {
		t.Fatalf("refuted = %v", refuted)
	}
}

// When every claim is refuted the run still succeeds, with no report and an
// honest inconclusive summary instead: no writer runs.
func TestWorkflowDeepResearchV2DryRunAllRefutedIsInconclusive(t *testing.T) {
	f := builtinFixtures(t)
	verify := f["steps"].(map[string]any)["verify"].(map[string]any)
	delete(verify, "items")
	verify["item_default"] = map[string]any{
		"output": map[string]any{
			"status": "succeeded", "summary": "Contradicted.",
			"sources": []any{map[string]any{"url": "https://example.org/evidence/against", "cited": true}},
			"result":  map[string]any{"refuted": true, "evidence": "A newer measurement contradicts it.", "evidence_urls": []any{"https://example.org/evidence/against"}, "confidence": "high"},
		},
		"tokens": 30000,
	}
	delete(f["steps"].(map[string]any), "synthesize")
	s := simulateBuiltin(t, f)
	if s.Status != SimulationSucceeded {
		t.Fatalf("dry run %s: %s\n%s", s.Status, s.Error, s.Text())
	}
	if _, ok := s.Outputs["report"]; ok {
		t.Fatal("an all-refuted run wrote a report")
	}
	if s.Outputs["outcome"] != "all_refuted" {
		t.Fatalf("outcome = %v", s.Outputs["outcome"])
	}
	inconclusive := s.Outputs["inconclusive"].(map[string]any)
	if inconclusive["title"] != "Research inconclusive" || !strings.Contains(inconclusive["summary"].(string), "All 4 claims were refuted") ||
		!strings.Contains(inconclusive["body_markdown"].(string), "## Refuted claims") || len(inconclusive["citations"].([]any)) != 0 {
		t.Fatalf("inconclusive = %v", inconclusive)
	}
	for _, e := range s.Trace {
		if e.Step == "synthesize" {
			t.Fatalf("the writer ran: %+v", e)
		}
	}
}
