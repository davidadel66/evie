package workflow

import (
	"path/filepath"
	"strings"
	"testing"
)

// Stage 14 sub-stage 10 (2026-10-04): an agent step's output keeps the
// worker's summary, sources, limitations, tokens and typed result, and names
// the worker execution whose full report the report store keeps, so
// checkpoints (which hold every step output so far) stay bounded however
// many workers a map runs. Runs pinned to evie.workflow.runtime/5 still get
// the full report (FullReport), as they did when they started.

func lookDefinition(t *testing.T, output string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "look")
	writeFile(t, dir, "README.md", "Look.\n")
	writeFile(t, dir, "prompts/look.md", "Read the repository's README and summarize it.\n")
	writeFile(t, dir, "schemas/topic.schema.json", `{"type":"object","additionalProperties":false,"required":["topic"],"properties":{"topic":{"type":"string"}}}`)
	writeFile(t, dir, "workflow.yaml", `schema_version: 1
name: look
inputs:
  topic:
    type: string
    required: true
run_key:
  inputs: [topic]
model_policies:
  m:
    version: 1
    provider: openrouter
    model: deepseek/deepseek-v4.1-flash
    max_output_tokens: 100
limits:
  max_duration: 30m
  max_attempts: 5
  max_parallel: 1
  max_tokens: 500000
retention:
  classification: ordinary
start: look
steps:
  - id: look
    kind: agent
    worker: explore
    model_policy: m
    objective: prompts/look.md
    input:
      topic: $inputs.topic
    output_schema: schemas/topic.schema.json
    budget:
      time: 5m
      tokens: 200000
outputs:
  out: `+output+`
`)
	return dir
}

func TestWorkflowAgentOutputKeepsSummaryAndResultNotTheReport(t *testing.T) {
	v := mustValidate(t, lookDefinition(t, "$steps.look.output.summary"), testEnvironment())
	report := "## Summary\nA Go project.\n\n## Details\n" + strings.Repeat("Long evidence. ", 2000) + "\n\n## Result\n```json\n{\"topic\":\"go\"}\n```\n"
	r := WorkerResult{Status: "succeeded", Summary: "A Go project.", Report: report, ExecutionID: "exec-1",
		Sources: []WorkerSource{{URL: "https://example.org/a", Cited: true}}, Tokens: 1200}
	out, _, err := v.AgentOutput("look", r)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if _, has := m["report"]; has {
		t.Fatalf("bounded output carries the full report: %v", m)
	}
	if m["execution_id"] != "exec-1" || m["report_bytes"] != int64(len(report)) || m["summary"] != "A Go project." ||
		m["result"].(map[string]any)["topic"] != "go" || len(m["sources"].([]any)) != 1 {
		t.Fatalf("bounded output = %v", m)
	}
	encoded, _ := EncodeValue(out)
	if len(encoded) > 1024 {
		t.Fatalf("bounded output is %d bytes", len(encoded))
	}
	// A run pinned before sub-stage 10 keeps the full report.
	r.FullReport = true
	full, _, err := v.AgentOutput("look", r)
	if err != nil {
		t.Fatal(err)
	}
	if fm := full.(map[string]any); fm["report"] != report {
		t.Fatalf("full output = %v", fm)
	}
}

// No step can read an agent step's full report: its output does not carry
// it, and validation says where it is instead.
func TestWorkflowValidationRefusesReadingAnAgentReport(t *testing.T) {
	_, err := validateDir(lookDefinition(t, "$steps.look.output.report"), testEnvironment())
	wantProblem(t, err, "outputs.out", "report", "report store")
	if _, err := validateDir(lookDefinition(t, "$steps.look.output.execution_id"), testEnvironment()); err != nil {
		t.Fatalf("the execution reference is readable: %v", err)
	}
}
