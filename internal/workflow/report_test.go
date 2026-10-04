package workflow

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func wantLines(t *testing.T, text string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(text, line) {
			t.Fatalf("report lacks %q:\n%s", line, text)
		}
	}
}

func TestWorkflowReportShowsTheDeepResearchReview(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "deep-research"), testEnvironment())
	r := v.Report()
	text := r.Text()
	wantLines(t, text,
		"Workflow deep-research (schema 1)",
		"Executable hash: "+v.Hash(),
		"Graph (start: plan)",
		"1. plan [ai] model planner, prompt prompts/plan.md, output schemas/angles.schema.json -> research",
		"2. research [map] over $steps.plan.output.angles: at most 6 items, 4 at once, partial (at least 1 must succeed)",
		"item [agent] worker research, model researcher, objective prompts/research.md, budget 10m / 400,000 tokens",
		"3. claims [deterministic] collect_claims@1 -> ballots",
		"6. filter [deterministic] majority_filter@1 -> synthesize",
		"7. synthesize [ai] model writer, prompt prompts/synthesize.md, output schemas/report.schema.json -> end",
		"question: string, required, max length 2000",
		"max_angles: integer, default 4, minimum 1, maximum 6",
		"report: $steps.synthesize.output (always)",
		"Run key: deep-research + question",
		"Capability contracts: none",
		"research: research worker, up to 6 items x 2 attempts, 10m / 400,000 tokens each",
		"verify: research worker, up to 36 items x 1 attempt, 5m / 150,000 tokens each",
		"collect_claims@1",
		"Resources: none",
		"plan: 2 attempts",
		"planner v1: openrouter deepseek/deepseek-v4.1-flash, temperature 0.2, max output 4,000 tokens",
		"Requested Standing Authority: none",
		"Schedule: none (manual starts only)",
		"Retention: ordinary",
		"max duration 2h, 200 attempts, 4 at once, 12,000,000 tokens",
		"Worst case: 10,200,000 agent tokens, 55 attempts",
		"README.md (documentation, not hashed)",
		"prompts/plan.md",
	)
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back["executable_hash"] != v.Hash() || back["workflow"] != "deep-research" {
		t.Fatalf("JSON report = %s", data)
	}
}

func TestWorkflowReportShowsAuthorityResourcesAndSchedule(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment())
	text := v.Report().Text()
	wantLines(t, text,
		"1. read [capability] notes.list 1.0.0 (read) on conn-notes notebook:inbox -> check",
		"2. check [branch] $steps.read.output.notes empty -> ask; otherwise -> summarize",
		"[interrupt] response schemas/ask.schema.json -> end",
		"[capability] notes.append 1.0.0 (effect, idempotent) on conn-notes notebook:digest -> end",
		"notes.list 1.0.0: read",
		"notes.append 1.0.0: effect, idempotent, reconciled by the owner, conflict keys per resource",
		"conn-notes notebook:digest: write",
		"conn-notes notebook:inbox: read",
		"read: 3 attempts, backoff 2s",
		"notes.append on conn-notes: notebook:digest, at most 1 call per run",
		"Schedule: 0 7 * * 1-5 America/New_York, catch up each missed run (at most 3), inputs day=$schedule.date",
		"Retention: sensitive; sensitive inputs: day",
		"digest: $steps.summarize.output.digest (only when summarize runs)",
	)
}
