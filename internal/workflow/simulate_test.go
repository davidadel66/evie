package workflow

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func deepResearchFixtures(t *testing.T) Fixtures {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "deep-research.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseFixtures(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func fixtures(t *testing.T, raw string) Fixtures {
	t.Helper()
	f, err := ParseFixtures([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func simulate(t *testing.T, v *Validated, f Fixtures) *Simulation {
	t.Helper()
	s, err := v.Simulate(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func entries(s *Simulation, step string) []TraceEntry {
	var out []TraceEntry
	for _, e := range s.Trace {
		if e.Step == step {
			out = append(out, e)
		}
	}
	return out
}

func TestWorkflowSimulatesDeepResearch(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "deep-research"), testEnvironment())
	s := simulate(t, v, deepResearchFixtures(t))
	if s.Status != "succeeded" {
		t.Fatalf("status %s: %s\n%s", s.Status, s.Error, s.Text())
	}
	if s.ExecutableHash != v.Hash() || !strings.HasPrefix(s.RunKey, "deep-research:sha256:") {
		t.Fatalf("hash %s run key %s", s.ExecutableHash, s.RunKey)
	}
	// Research item 2 failed once, then succeeded on its retry; item 1's
	// partial wrap-up report counts as success.
	research := entries(s, "research")
	var item2 []TraceEntry
	for _, e := range research {
		if e.Item != nil && *e.Item == 2 {
			item2 = append(item2, e)
		}
	}
	if len(item2) != 2 || item2[0].Status != "failed" || item2[0].Attempt != 1 || item2[1].Status != "succeeded" || item2[1].Attempt != 2 {
		t.Fatalf("research item 2 = %+v", item2)
	}
	// Claims merged the duplicate fuel claim and kept only fetched sources.
	claims := entries(s, "claims")[0].Output.(map[string]any)
	if claims["count"] != int64(4) {
		t.Fatalf("claims = %v", claims)
	}
	first := claims["claims"].([]any)[0].(map[string]any)
	if got := first["unverified_sources"].([]any); len(got) != 1 || got[0] != "https://example.org/never-fetched" {
		t.Fatalf("first claim = %v", first)
	}
	// 12 ballots; ballot 10 failed and the step continued (partial).
	verify := entries(s, "verify")
	summary := verify[len(verify)-1].Output.(map[string]any)
	if summary["succeeded"] != int64(11) || summary["failed"] != int64(1) {
		t.Fatalf("verify summary = %v", summary)
	}
	if fails := summary["failures"].([]any); fails[0].(map[string]any)["index"] != int64(10) {
		t.Fatalf("verify failures = %v", fails)
	}
	// c3 had two refuting votes; c4 lost a vote but kept two supporting.
	verified := s.Outputs["verified_claims"].([]any)
	ids := []string{}
	for _, c := range verified {
		ids = append(ids, c.(map[string]any)["id"].(string))
	}
	if strings.Join(ids, ",") != "c1,c2,c4" {
		t.Fatalf("verified claims = %v", ids)
	}
	if s.Outputs["report"].(map[string]any)["title"] != "What limits the range of electric aircraft" {
		t.Fatalf("report = %v", s.Outputs["report"])
	}
	if len(s.Previews) != 0 {
		t.Fatalf("previews = %v", s.Previews)
	}
	wantTokens := int64(1800 + 120000 + 98000 + 4000 + 90000 + 9*30000 + 28000 + 26000 + 5000 + 9000)
	if s.Tokens != wantTokens {
		t.Fatalf("tokens = %d, want %d", s.Tokens, wantTokens)
	}
}

// The same definition and fixtures produce a byte-identical trace, in text
// and JSON, however many times and from however many loads.
func TestWorkflowSimulationIsDeterministic(t *testing.T) {
	var texts, jsons []string
	for range 3 {
		v := mustValidate(t, copyDefinition(t, "deep-research"), testEnvironment())
		s := simulate(t, v, deepResearchFixtures(t))
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		texts, jsons = append(texts, s.Text()), append(jsons, string(data))
	}
	for i := 1; i < 3; i++ {
		if texts[i] != texts[0] || jsons[i] != jsons[0] {
			t.Fatal("simulation output differs between runs")
		}
	}
}

func notesFixtures(notes string, extra string) string {
	return `{"inputs": {"day": "2026-10-05"}, "steps": {
		"read": {"output": {"notes": ` + notes + `}}` + extra + `}}`
}

func TestWorkflowSimulationPreviewsEffectsWithoutExecuting(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment())
	s := simulate(t, v, fixtures(t, notesFixtures(`[{"id": "n1", "text": "Call the bank."}]`, `,
		"summarize": {"output": {"digest": "- Call the bank."}, "tokens": 300},
		"write": {"output": {"entry_id": "entry-7"}}`)))
	if s.Status != "succeeded" {
		t.Fatalf("status %s: %s", s.Status, s.Error)
	}
	if len(s.Previews) != 1 {
		t.Fatalf("previews = %+v", s.Previews)
	}
	p := s.Previews[0]
	if p.Step != "write" || p.Capability != "notes.append" || p.Connection != "conn-notes" || p.Resource != "notebook:digest" {
		t.Fatalf("preview = %+v", p)
	}
	if p.Input.(map[string]any)["text"] != "- Call the bank." {
		t.Fatalf("preview input = %v", p.Input)
	}
	write := entries(s, "write")
	if len(write) != 1 || write[0].Status != "previewed" || !strings.Contains(write[0].Note, "not executed") {
		t.Fatalf("write entry = %+v", write)
	}
	branch := entries(s, "check")
	if len(branch) != 1 || branch[0].Next != "summarize" || branch[0].Status != "branched" {
		t.Fatalf("branch entry = %+v", branch)
	}
	if !strings.Contains(s.Notice, "no Effect Receipt") || !strings.Contains(s.Notice, "no authority") {
		t.Fatalf("notice = %q", s.Notice)
	}
	if !strings.Contains(s.Text(), "PREVIEW write notes.append") {
		t.Fatalf("text does not show the preview:\n%s", s.Text())
	}
}

func TestWorkflowSimulationStopsAtAnUnansweredInterruption(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment())
	s := simulate(t, v, fixtures(t, notesFixtures(`[]`, "")))
	if s.Status != "waiting_for_human" {
		t.Fatalf("status = %s", s.Status)
	}
	ask := entries(s, "ask")
	if len(ask) != 1 || ask[0].Status != "waiting" || !strings.Contains(ask[0].Note, "No notes were found") {
		t.Fatalf("ask = %+v", ask)
	}
	answered := simulate(t, v, fixtures(t, notesFixtures(`[]`, `, "ask": {"response": {"expected": true}}`)))
	if answered.Status != "succeeded" || entries(answered, "ask")[0].Status != "answered" {
		t.Fatalf("answered = %s %+v", answered.Status, entries(answered, "ask"))
	}
	if _, ok := answered.Outputs["digest"]; ok {
		t.Fatalf("digest produced on the interruption path: %v", answered.Outputs)
	}
	invalid := simulate(t, v, fixtures(t, notesFixtures(`[]`, `, "ask": {"response": {"expected": "yes"}}`)))
	if invalid.Status != "failed" || !strings.Contains(invalid.Error, "expected") {
		t.Fatalf("invalid response = %s %s", invalid.Status, invalid.Error)
	}
}

func TestWorkflowSimulationRetriesInvalidAIOutputWithinItsPolicy(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment())
	notes := `[{"id": "n1", "text": "x"}]`
	// summarize has the default single attempt: one invalid output fails it.
	s := simulate(t, v, fixtures(t, notesFixtures(notes, `, "summarize": {"attempts": [{"output": {"digest": 7}}, {"output": {"digest": "ok"}}]}`)))
	if s.Status != "failed" || !strings.Contains(s.Error, "summarize") || len(entries(s, "summarize")) != 1 {
		t.Fatalf("status %s error %q entries %+v", s.Status, s.Error, entries(s, "summarize"))
	}
	if len(entries(s, "write")) != 0 {
		t.Fatal("a step after a failed step ran")
	}
	// With two attempts allowed, the second, valid output is used.
	dir := copyDefinition(t, "notes-digest")
	edit(t, dir, "workflow.yaml", "    output_schema: schemas/digest.schema.json\n", "    output_schema: schemas/digest.schema.json\n    retry:\n      max_attempts: 2\n")
	v2 := mustValidate(t, dir, testEnvironment())
	s2 := simulate(t, v2, fixtures(t, notesFixtures(notes, `, "summarize": {"attempts": [{"output": {"digest": 7}}, {"output": {"digest": "ok"}}]}, "write": {"output": {"entry_id": "e"}}`)))
	got := entries(s2, "summarize")
	if s2.Status != "succeeded" || len(got) != 2 || got[0].Status != "failed" || !strings.Contains(got[0].Error, "digest") {
		t.Fatalf("status %s entries %+v", s2.Status, got)
	}
}

// A small definition whose list type does not bound the map, so the
// runtime bound is what stops it.
func listDefinition(t *testing.T, failure string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "list")
	writeFile(t, dir, "README.md", "List.\n")
	writeFile(t, dir, "prompts/list.md", "List things.\n")
	writeFile(t, dir, "prompts/judge.md", "Judge one thing.\n")
	writeFile(t, dir, "schemas/list.schema.json", `{"type":"object","additionalProperties":false,"required":["things"],"properties":{"things":{"type":"array","items":{"type":"string"}}}}`)
	writeFile(t, dir, "schemas/judge.schema.json", `{"type":"object","additionalProperties":false,"required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)
	writeFile(t, dir, "workflow.yaml", `schema_version: 1
name: list
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
  max_duration: 10m
  max_attempts: 50
  max_parallel: 2
  max_tokens: 100000
retention:
  classification: ordinary
start: list
steps:
  - id: list
    kind: ai
    model_policy: m
    prompt: prompts/list.md
    input:
      topic: $inputs.topic
    output_schema: schemas/list.schema.json
    next: judge
  - id: judge
    kind: map
    over: $steps.list.output.things
    max_items: 3
    max_parallel: 2
`+failure+`
    item:
      kind: ai
      model_policy: m
      prompt: prompts/judge.md
      input:
        thing: $item
      output_schema: schemas/judge.schema.json
outputs:
  judged: $steps.judge.output.results
`)
	return dir
}

func listFixtures(things []string, items string) string {
	data, _ := json.Marshal(things)
	return `{"inputs": {"topic": "t"}, "steps": {"list": {"output": {"things": ` + string(data) + `}}, "judge": {` + items + `}}}`
}

func TestWorkflowSimulationEnforcesMapBounds(t *testing.T) {
	v := mustValidate(t, listDefinition(t, "    on_item_failure: fail"), testEnvironment())
	if !strings.Contains(strings.Join(v.Report().Warnings, "\n"), "steps.judge.over") {
		t.Fatalf("no warning about the unbounded list: %q", v.Report().Warnings)
	}
	s := simulate(t, v, fixtures(t, listFixtures([]string{"a", "b", "c", "d"}, `"item_default": {"output": {"ok": true}}`)))
	if s.Status != "failed" || !strings.Contains(s.Error, "4 items") || !strings.Contains(s.Error, "max_items 3") {
		t.Fatalf("status %s error %q", s.Status, s.Error)
	}
	for _, e := range entries(s, "judge") {
		if e.Item != nil {
			t.Fatalf("an item started on an oversized list: %+v", e)
		}
	}
}

func TestWorkflowSimulationMapFailurePolicies(t *testing.T) {
	things := []string{"a", "b", "c"}
	items := `"item_default": {"output": {"ok": true}}, "items": {"1": {"error": "model timeout"}}`

	failFast := simulate(t, mustValidate(t, listDefinition(t, "    on_item_failure: fail"), testEnvironment()), fixtures(t, listFixtures(things, items)))
	if failFast.Status != "failed" {
		t.Fatalf("fail policy: status %s", failFast.Status)
	}
	for _, e := range entries(failFast, "judge") {
		if e.Item != nil && *e.Item == 2 {
			t.Fatal("fail policy started an item after a failure")
		}
	}

	partial := simulate(t, mustValidate(t, listDefinition(t, "    on_item_failure: partial\n    min_succeeded: 2"), testEnvironment()), fixtures(t, listFixtures(things, items)))
	if partial.Status != "succeeded" {
		t.Fatalf("partial policy: %s %s", partial.Status, partial.Error)
	}
	results := partial.Outputs["judged"].([]any)
	if len(results) != 2 || results[0].(map[string]any)["index"] != int64(0) || results[1].(map[string]any)["index"] != int64(2) || results[1].(map[string]any)["item"] != "c" {
		t.Fatalf("results = %v", results)
	}

	short := simulate(t, mustValidate(t, listDefinition(t, "    on_item_failure: partial\n    min_succeeded: 3"), testEnvironment()), fixtures(t, listFixtures(things, items)))
	if short.Status != "failed" || !strings.Contains(short.Error, "min_succeeded 3") {
		t.Fatalf("partial below minimum: %s %q", short.Status, short.Error)
	}
}

func TestWorkflowSimulationEnforcesRunLimits(t *testing.T) {
	dir := listDefinition(t, "    on_item_failure: fail")
	edit(t, dir, "workflow.yaml", "  max_attempts: 50\n", "  max_attempts: 2\n")
	s := simulate(t, mustValidate(t, dir, testEnvironment()), fixtures(t, listFixtures([]string{"a", "b"}, `"item_default": {"output": {"ok": true}}`)))
	if s.Status != "failed" || !strings.Contains(s.Error, "limits.max_attempts") {
		t.Fatalf("attempt limit: %s %q", s.Status, s.Error)
	}
	dir = listDefinition(t, "    on_item_failure: fail")
	s = simulate(t, mustValidate(t, dir, testEnvironment()), fixtures(t, listFixtures([]string{"a", "b"}, `"item_default": {"output": {"ok": true}, "tokens": 60000}`)))
	if s.Status != "failed" || !strings.Contains(s.Error, "limits.max_tokens") {
		t.Fatalf("token limit: %s %q", s.Status, s.Error)
	}
}

func TestWorkflowSimulationRefusesBadFixtures(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "deep-research"), testEnvironment())
	cases := map[string]string{
		"deterministic step":      `{"inputs": {"question": "q"}, "steps": {"claims": {"output": {}}}}`,
		"unknown step":            `{"inputs": {"question": "q"}, "steps": {"nope": {"output": {}}}}`,
		"missing AI output":       `{"inputs": {"question": "q"}, "steps": {}}`,
		"missing required input":  `{"inputs": {}, "steps": {"plan": {"output": {"angles": []}}}}`,
		"input of the wrong type": `{"inputs": {"question": "q", "max_angles": "three"}, "steps": {}}`,
		"input out of range":      `{"inputs": {"question": "q", "max_angles": 9}, "steps": {}}`,
		"unknown input":           `{"inputs": {"question": "q", "colour": "red"}, "steps": {}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Simulate(fixtures(t, raw)); err == nil {
				t.Fatal("simulation accepted bad fixtures")
			}
		})
	}
	notes := mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment())
	if _, err := notes.Simulate(fixtures(t, notesFixtures(`[{"id": "n1", "text": "x"}]`, `,
		"summarize": {"output": {"digest": "d"}}, "write": {"error": "timeout"}`))); err == nil || !strings.Contains(err.Error(), "effect") {
		t.Fatalf("an effect fixture simulated a failure: %v", err)
	}
	if _, err := ParseFixtures([]byte(`{"inputs": {}, "stepz": {}}`)); err == nil {
		t.Fatal("fixtures with an unknown field parsed")
	}
	if _, err := ParseFixtures([]byte(`{"inputs": {}} {"again": 1}`)); err == nil {
		t.Fatal("two fixture documents parsed")
	}
}

func TestWorkflowAgentResultIsExtractedFromTheReport(t *testing.T) {
	report := "## Summary\nDone.\n\n## Result\n```json\n{\"a\": 1}\n```\n\nThen a correction:\n\n```json\n{\"a\": 2}\n```\n"
	got, err := extractAgentResult(report)
	if err != nil || got.(map[string]any)["a"] != int64(2) {
		t.Fatalf("extract = %v, %v", got, err)
	}
	for name, bad := range map[string]string{
		"no section":   "## Summary\nNo result.\n",
		"no block":     "## Result\nsupported\n",
		"invalid JSON": "## Result\n```json\n{oops}\n```\n",
		"not object":   "## Result\n```json\n[1]\n```\n",
	} {
		if _, err := extractAgentResult(bad); err == nil {
			t.Fatalf("%s: extracted a result", name)
		}
	}
}

// A top-level agent step returns the worker result contract; a branch can
// test its status.
func TestWorkflowSimulatesATopLevelAgentStep(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "look")
	writeFile(t, dir, "README.md", "Look.\n")
	writeFile(t, dir, "prompts/look.md", "Read the repository's README and summarize it.\n")
	writeFile(t, dir, "schemas/ack.schema.json", `{"type":"object","additionalProperties":false,"required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)
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
    budget:
      time: 5m
      tokens: 200000
    next: check
  - id: check
    kind: branch
    cases:
      - when:
          ref: $steps.look.output.status
          op: eq
          value: partial
        next: confirm
    default: done
  - id: confirm
    kind: interrupt
    message: The explore worker stopped at its budget. Is its summary enough?
    response_schema: schemas/ack.schema.json
  - id: done
    kind: interrupt
    message: Review the summary.
    input:
      summary: $steps.look.output.summary
    response_schema: schemas/ack.schema.json
outputs:
  summary: $steps.look.output.summary
`)
	v := mustValidate(t, dir, testEnvironment())
	s := simulate(t, v, fixtures(t, `{"inputs": {"topic": "readme"}, "steps": {
		"look": {"output": {"status": "partial", "summary": "It is a Go project.", "sources": []}, "tokens": 150000},
		"confirm": {"response": {"ok": true}}}}`))
	if s.Status != "succeeded" || entries(s, "check")[0].Next != "confirm" || s.Outputs["summary"] != "It is a Go project." {
		t.Fatalf("status %s error %q trace:\n%s", s.Status, s.Error, s.Text())
	}
	look := entries(s, "look")[0].Output.(map[string]any)
	// Since sub-stage 10 the output names the report's size, not its text.
	if _, has := look["report"]; has || look["report_bytes"] != int64(len("## Summary\nIt is a Go project.\n")) || look["tokens"] != int64(150000) {
		t.Fatalf("worker result = %v", look)
	}
	// A value outside the status enum is refused at validation.
	edit(t, dir, "workflow.yaml", "          value: partial\n", "          value: failed\n")
	_, err := validateDir(dir, testEnvironment())
	wantProblem(t, err, "steps.check.cases[0].when.value", "failed")
	// A fixture reporting more tokens than the budget is a fixture error.
	edit(t, dir, "workflow.yaml", "          value: failed\n", "          value: partial\n")
	if _, err := mustValidate(t, dir, testEnvironment()).Simulate(fixtures(t, `{"inputs": {"topic": "readme"}, "steps": {
		"look": {"output": {"status": "succeeded", "summary": "s", "sources": []}, "tokens": 250000}}}`)); err == nil {
		t.Fatal("tokens above the agent budget were accepted")
	}
}

// Simulation contacts no provider: the package imports no model, worker,
// web, database, network or process code, so nothing it runs can reach one.
func TestWorkflowPackageCannotReachProviders(t *testing.T) {
	forbidden := []string{
		"github.com/davidadel66/evie/internal/", "net", "net/http", "os/exec", "database/sql", "syscall", "plugin", "unsafe",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			for _, bad := range forbidden {
				if path == bad || (strings.HasSuffix(bad, "/") && strings.HasPrefix(path, bad)) {
					t.Fatalf("%s imports %s", name, path)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no package files checked")
	}
}
