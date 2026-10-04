package workflow

import (
	"strings"
	"testing"
)

// The accepted YAML subset (2026-10-04): every form that can smuggle
// structure or silently convert a value is refused with a problem naming the
// line, and the quoted spelling of the same text is accepted.
func TestWorkflowStrictYAMLRejectsUnsafeForms(t *testing.T) {
	cases := []struct {
		name        string
		old, new    string
		fragments   []string
		appendExtra string
	}{
		{name: "unknown top-level field", old: "start: plan\n", new: "start: plan\nextra: 1\n", fragments: []string{"field extra not found"}},
		{name: "unknown step field", old: "    retry:\n      max_attempts: 2\n    next: research", new: "    retry:\n      max_attempts: 2\n    nxt: research\n    next: research", fragments: []string{"field nxt not found"}},
		{name: "field of another kind", old: "    model_policy: planner\n", new: "    model_policy: planner\n    worker: research\n", fragments: []string{"steps.plan", "worker", "ai steps"}},
		{name: "anchor", old: "  max_attempts: 200\n  max_parallel: 4\n", new: "  max_attempts: 200\n  max_parallel: &p 4\n", fragments: []string{"anchor"}},
		{name: "alias", old: "  max_attempts: 200\n  max_parallel: 4\n", new: "  max_attempts: &n 200\n  max_parallel: *n\n", fragments: []string{"alias"}},
		{name: "merge key", old: "limits:\n", new: "limits:\n  <<: {max_parallel: 4}\n", fragments: []string{"merge key"}},
		{name: "explicit tag", old: "  max_tokens: 12000000\n", new: "  max_tokens: !!int \"12000000\"\n", fragments: []string{"explicit tag"}},
		{name: "custom tag", old: "  max_duration: 2h\n", new: "  max_duration: !duration 2h\n", fragments: []string{"explicit tag"}},
		{name: "duplicate key", old: "start: plan\n", new: "start: plan\nstart: research\n", fragments: []string{"duplicate key", "start"}},
		{name: "null value", old: "  classification: ordinary\n", new: "  classification:\n", fragments: []string{"null"}},
		{name: "tilde null", old: "  classification: ordinary\n", new: "  classification: ~\n", fragments: []string{"null"}},
		{name: "yes", old: "    required: true\n", new: "    required: yes\n", fragments: []string{"yes", "quote"}},
		{name: "On", old: "    required: true\n", new: "    required: On\n", fragments: []string{"On", "quote"}},
		{name: "capitalised boolean", old: "    required: true\n", new: "    required: True\n", fragments: []string{"True", "true or false"}},
		{name: "hexadecimal", old: "  max_attempts: 200\n", new: "  max_attempts: 0xC8\n", fragments: []string{"0xC8", "decimal"}},
		{name: "octal", old: "  max_attempts: 200\n", new: "  max_attempts: 0310\n", fragments: []string{"0310", "decimal"}},
		{name: "exponent", old: "  max_tokens: 12000000\n", new: "  max_tokens: 1.2e7\n", fragments: []string{"1.2e7"}},
		{name: "plus sign", old: "  max_attempts: 200\n", new: "  max_attempts: +200\n", fragments: []string{"+200"}},
		{name: "underscore", old: "  max_tokens: 12000000\n", new: "  max_tokens: 12_000_000\n", fragments: []string{"12_000_000"}},
		{name: "infinity", old: "    temperature: 0.2\n", new: "    temperature: .inf\n", fragments: []string{".inf"}},
		{name: "decimal into integer", old: "  max_attempts: 200\n", new: "  max_attempts: 200.0\n", fragments: []string{"limits.max_attempts", "integer"}},
		{name: "number into text", old: "    model: deepseek/deepseek-v4.1-flash\n    temperature: 0.2", new: "    model: 4\n    temperature: 0.2", fragments: []string{"model_policies.planner.model", "text", "quote"}},
		{name: "boolean into text", old: "  classification: ordinary\n", new: "  classification: true\n", fragments: []string{"retention.classification", "text"}},
		{name: "text into integer", old: "  max_attempts: 200\n", new: "  max_attempts: \"200\"\n", fragments: []string{"max_attempts"}},
		{name: "unquoted date", old: "  classification: ordinary\n", new: "  classification: 2026-10-04\n", fragments: []string{"2026-10-04", "quote"}},
		{name: "non-text key", old: "start: plan\n", new: "start: plan\n1: x\n", fragments: []string{"key"}},
		{name: "second document", old: "", appendExtra: "---\nname: other\n", fragments: []string{"one YAML document"}},
		{name: "syntax error", old: "start: plan\n", new: "start: [plan\n", fragments: []string{"workflow.yaml"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := copyDefinition(t, "deep-research")
			if tc.old != "" {
				edit(t, dir, "workflow.yaml", tc.old, tc.new)
			}
			if tc.appendExtra != "" {
				edit(t, dir, "workflow.yaml", "\n  verified_claims: $steps.filter.output.accepted\n", "\n  verified_claims: $steps.filter.output.accepted\n"+tc.appendExtra)
			}
			_, err := validateDir(dir, testEnvironment())
			wantProblem(t, err, tc.fragments...)
		})
	}
}

func TestWorkflowStrictYAMLAcceptsQuotedText(t *testing.T) {
	dir := copyDefinition(t, "deep-research")
	edit(t, dir, "workflow.yaml", "name: deep-research\n", "name: 'deep-research'\n")
	edit(t, dir, "workflow.yaml", "    description: The question to research.\n", "    description: \"yes\"\n")
	edit(t, dir, "workflow.yaml", "    description: How many research angles to plan.\n", "    description: \"2026-10-04\"\n")
	edit(t, dir, "workflow.yaml", "run_key:\n  inputs: [question]\n", "run_key:\n  inputs:\n    - \"question\"\n")
	mustValidate(t, dir, testEnvironment())
}

func TestWorkflowStrictYAMLRefusesOversizedFile(t *testing.T) {
	dir := copyDefinition(t, "deep-research")
	edit(t, dir, "workflow.yaml", "schema_version: 1\n", "schema_version: 1\n#"+strings.Repeat("x", 300<<10)+"\n")
	_, err := validateDir(dir, testEnvironment())
	wantProblem(t, err, "workflow.yaml", "KiB")
}
