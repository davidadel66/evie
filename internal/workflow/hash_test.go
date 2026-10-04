package workflow

import (
	"path/filepath"
	"regexp"
	"testing"
)

func hashOf(t *testing.T, dir string) string {
	t.Helper()
	return mustValidate(t, dir, testEnvironment()).Hash()
}

func TestWorkflowHashIsStableAndWellFormed(t *testing.T) {
	dir := filepath.Join("testdata", "deep-research")
	first, second := hashOf(t, dir), hashOf(t, copyDefinition(t, "deep-research"))
	if first != second {
		t.Fatalf("same content hashed differently: %s and %s", first, second)
	}
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(first) {
		t.Fatalf("hash %q is not sha256:<hex>", first)
	}
}

// The deep-research example's executable hash is pinned: a change to
// canonicalization would silently invalidate every approval, so it must be a
// deliberate, reviewed edit of this value.
func TestWorkflowDeepResearchHashIsPinned(t *testing.T) {
	const pinned = "sha256:1a31702c311c677e9a4e6f0fd3060c221d640c8508ca211d406e06b214fa1ad1"
	if got := hashOf(t, filepath.Join("testdata", "deep-research")); got != pinned {
		t.Fatalf("deep-research executable hash = %s, pinned %s", got, pinned)
	}
}

// Documentation-only edits (README, comments, descriptions, formatting, key
// and step order, spelling out a default, equivalent durations, schema
// formatting) keep the executable hash, so they need no new approval.
func TestWorkflowDocumentationOnlyEditsKeepTheHash(t *testing.T) {
	base := hashOf(t, filepath.Join("testdata", "deep-research"))
	cases := map[string]func(t *testing.T, dir string){
		"README": func(t *testing.T, dir string) {
			writeFile(t, dir, "README.md", "# Entirely rewritten\n\nNew explanation.\n")
		},
		"comments":               replace("# Deep research: plan angles", "# A different comment\n# Deep research: plan angles"),
		"workflow description":   replace("  Research one question from several angles, verify each claim with three\n", "  Research a question thoroughly, verify each claim with three\n"),
		"step description":       replace("    description: One ballot per claim and voter.\n", "    description: Three ballots for every claim.\n"),
		"removed description":    replace("    description: One ballot per claim and voter.\n", ""),
		"input description":      replace("    description: The question to research.\n", "    description: What to find out.\n"),
		"quoting and flow style": replace("run_key:\n  inputs: [question]\n", "run_key:\n  inputs:\n    - \"question\"\n"),
		"key order":              replace("  max_duration: 2h\n  max_attempts: 200\n", "  max_attempts: 200\n  max_duration: 2h\n"),
		"equivalent duration":    replace("  max_duration: 2h\n", "  max_duration: 120m\n"),
		"explicit default retry": replace("    function: expand_ballots@1\n", "    function: expand_ballots@1\n    retry:\n      max_attempts: 1\n"),
		"step order": func(t *testing.T, dir string) {
			ballots := "  - id: ballots\n    kind: deterministic\n    description: One ballot per claim and voter.\n    function: expand_ballots@1\n    input:\n      claims: $steps.claims.output.claims\n      votes: 3\n    next: verify\n\n"
			edit(t, dir, "workflow.yaml", ballots, "")
			edit(t, dir, "workflow.yaml", "\noutputs:\n", "\n"+ballots+"outputs:\n")
		},
		"schema formatting": func(t *testing.T, dir string) {
			writeFile(t, dir, "schemas/verdict.schema.json", `{"required":["verdict","rationale"],"properties":{"rationale":{"maxLength":1000,"minLength":1,"type":"string"},"verdict":{"enum":["supported","refuted","unclear"],"type":"string"}},"additionalProperties":false,"type":"object"}`)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dir := copyDefinition(t, "deep-research")
			mutate(t, dir)
			if got := hashOf(t, dir); got != base {
				t.Fatalf("documentation-only edit changed the hash: %s -> %s", base, got)
			}
		})
	}
}

// Every behavior-affecting edit changes the executable hash, so it needs a
// new approval.
func TestWorkflowBehaviorEditsChangeTheHash(t *testing.T) {
	base := hashOf(t, filepath.Join("testdata", "deep-research"))
	cases := map[string]func(t *testing.T, dir string){
		"prompt text": func(t *testing.T, dir string) {
			edit(t, dir, "prompts/plan.md", "at most `max_angles` of them", "at most `max_angles` of them, ordered by importance")
		},
		"prompt whitespace": func(t *testing.T, dir string) {
			edit(t, dir, "prompts/verify.md", "Check one claim independently.", "Check one claim  independently.")
		},
		"schema bound": func(t *testing.T, dir string) {
			edit(t, dir, "schemas/verdict.schema.json", `"maxLength": 1000`, `"maxLength": 999`)
		},
		"schema description": func(t *testing.T, dir string) {
			edit(t, dir, "schemas/verdict.schema.json", `"verdict": {"type": "string",`, `"verdict": {"description": "The vote.", "type": "string",`)
		},
		"model":                replace("    model: anthropic/claude-sonnet-4.5\n", "    model: anthropic/claude-opus-4.1\n"),
		"temperature":          replace("    temperature: 0.4\n", "    temperature: 0.5\n"),
		"model policy version": replace("  writer:\n    version: 1\n", "  writer:\n    version: 2\n"),
		"limit":                replace("  max_attempts: 200\n", "  max_attempts: 199\n"),
		"input default":        replace("    default: 4\n", "    default: 3\n"),
		"input bound":          replace("    max_length: 2000\n", "    max_length: 1999\n"),
		"sensitive input":      replace("    max_length: 2000\n", "    max_length: 2000\n    sensitive: true\n"),
		"run key":              replace("  inputs: [question]\n", "  inputs: [question, max_angles]\n"),
		"retention":            replace("classification: ordinary", "classification: sensitive"),
		"map bound":            replace("    max_items: 36\n", "    max_items: 40\n"),
		"map parallelism":      replace("    max_items: 6\n    max_parallel: 4\n", "    max_items: 6\n    max_parallel: 3\n"),
		"failure policy":       replace("    on_item_failure: partial\n    min_succeeded: 1\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md", "    on_item_failure: fail\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md"),
		"agent budget":         replace("        tokens: 150000\n", "        tokens: 150001\n"),
		"worker":               replace("      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md", "      worker: explore\n      model_policy: researcher\n      objective: prompts/verify.md"),
		"function argument":    replace("      quorum: 2\n", "      quorum: 3\n"),
		"retry":                replace("    function: expand_ballots@1\n", "    function: expand_ballots@1\n    retry:\n      max_attempts: 2\n"),
		"backoff":              replace("    next: synthesize\n", "    next: synthesize\n    retry:\n      max_attempts: 2\n      backoff: 1s\n"),
		"input mapping":        replace("      rejected_claims: $steps.filter.output.rejected\n", ""),
		"output":               replace("\n  verified_claims: $steps.filter.output.accepted\n", "\n"),
		"prompt file renamed": func(t *testing.T, dir string) {
			writeFile(t, dir, "prompts/plan2.md", "You plan research. Split the question in the input data into distinct,\nnon-overlapping research angles, at most `max_angles` of them. Each angle\nis one focused question a researcher can answer with web sources.\n\nGive every angle a short identifier of lowercase letters, digits and\nhyphens. Return only the JSON object the output schema describes.\n")
			edit(t, dir, "workflow.yaml", "prompt: prompts/plan.md", "prompt: prompts/plan2.md")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dir := copyDefinition(t, "deep-research")
			mutate(t, dir)
			if got := hashOf(t, dir); got == base {
				t.Fatalf("behavior edit kept the hash %s", base)
			}
		})
	}
}

// A referenced function's identity is part of the hash: a new version of
// the reviewed code changes the hash of the same files.
func TestWorkflowFunctionIdentityIsHashed(t *testing.T) {
	v := mustValidate(t, filepath.Join("testdata", "deep-research"), testEnvironment())
	registry := reviewedFunctions()
	changed := map[string]Function{}
	for k, f := range registry {
		changed[k] = f
	}
	f := changed["majority_filter@1"]
	f.Params = append([]Param(nil), f.Params...)
	f.Params[2].Max = 8
	changed["majority_filter@1"] = f
	other, err := executableHash(v.definition, v.files, changed)
	if err != nil {
		t.Fatal(err)
	}
	if other == v.Hash() {
		t.Fatal("changing a function's signature kept the hash")
	}
}
