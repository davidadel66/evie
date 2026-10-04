package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
)

func TestWorkflowExamplesValidate(t *testing.T) {
	for _, name := range []string{"deep-research", "notes-digest"} {
		t.Run(name, func(t *testing.T) {
			v := mustValidate(t, filepath.Join("testdata", name), testEnvironment())
			if v.Report().Workflow != name {
				t.Fatalf("report names %q", v.Report().Workflow)
			}
		})
	}
}

// The worker names are the subagent presets' own identities.
func TestWorkflowWorkerNamesMatchSubagentPresets(t *testing.T) {
	if WorkerResearch != delegation.WorkerResearch || WorkerExplore != delegation.WorkerExplore || WorkerBuild != delegation.WorkerBuild {
		t.Fatal("workflow worker names differ from the subagent presets")
	}
}

type validationCase struct {
	name      string
	def       string
	mutate    func(t *testing.T, dir string)
	env       func(env *Environment)
	fragments []string
}

func runValidationCases(t *testing.T, cases []validationCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := tc.def
			if def == "" {
				def = "deep-research"
			}
			dir := copyDefinition(t, def)
			if tc.mutate != nil {
				tc.mutate(t, dir)
			}
			env := testEnvironment()
			if tc.env != nil {
				tc.env(&env)
			}
			_, err := validateDir(dir, env)
			wantProblem(t, err, tc.fragments...)
		})
	}
}

func replace(old, replacement string) func(t *testing.T, dir string) {
	return func(t *testing.T, dir string) { edit(t, dir, "workflow.yaml", old, replacement) }
}

func remove(rel string) func(t *testing.T, dir string) {
	return func(t *testing.T, dir string) {
		if err := os.Remove(filepath.Join(dir, rel)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkflowValidationRejectsDefinitionAndFileProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "unsupported schema version", mutate: replace("schema_version: 1\n", "schema_version: 2\n"), fragments: []string{"schema_version", "unsupported", "2"}},
		{name: "missing schema version", mutate: replace("schema_version: 1\n", ""), fragments: []string{"schema_version", "required"}},
		{name: "invalid name", mutate: replace("name: deep-research\n", "name: Deep Research\n"), fragments: []string{"name"}},
		{name: "missing README", mutate: remove("README.md"), fragments: []string{"README.md"}},
		{name: "missing prompt", mutate: remove("prompts/plan.md"), fragments: []string{"steps.plan.prompt", "prompts/plan.md"}},
		{name: "missing schema", mutate: remove("schemas/verdict.schema.json"), fragments: []string{"steps.verify.item.output_schema", "schemas/verdict.schema.json"}},
		{name: "prompt outside the definition", mutate: replace("prompt: prompts/plan.md", "prompt: ../plan.md"), fragments: []string{"steps.plan.prompt", "inside the definition"}},
		{name: "absolute prompt", mutate: replace("prompt: prompts/plan.md", "prompt: /etc/passwd.md"), fragments: []string{"steps.plan.prompt", "relative"}},
		{name: "prompt is not Markdown", mutate: replace("prompt: prompts/plan.md", "prompt: schemas/angles.schema.json"), fragments: []string{"steps.plan.prompt", ".md"}},
		{name: "schema is not JSON", mutate: func(t *testing.T, dir string) {
			writeFile(t, dir, "schemas/angles.schema.json", "{not json")
		}, fragments: []string{"schemas/angles.schema.json", "JSON"}},
		{name: "unsupported schema keyword", mutate: func(t *testing.T, dir string) {
			edit(t, dir, "schemas/verdict.schema.json", `"rationale": {"type": "string", "minLength": 1, "maxLength": 1000}`, `"rationale": {"type": "string", "pattern": "^x"}`)
		}, fragments: []string{"schemas/verdict.schema.json", "pattern"}},
		{name: "open object schema", mutate: func(t *testing.T, dir string) {
			edit(t, dir, "schemas/verdict.schema.json", `"additionalProperties": false,`, ``)
		}, fragments: []string{"schemas/verdict.schema.json", "additionalProperties"}},
		{name: "non-object output schema", mutate: func(t *testing.T, dir string) {
			writeFile(t, dir, "schemas/verdict.schema.json", `{"type": "string"}`)
		}, fragments: []string{"steps.verify.item.output_schema", "object"}},
		{name: "missing retention", mutate: replace("retention:\n  classification: ordinary\n", ""), fragments: []string{"retention", "required"}},
		{name: "unknown retention class", mutate: replace("classification: ordinary", "classification: public"), fragments: []string{"retention.classification", "public"}},
		{name: "unknown output step", mutate: replace("  report: $steps.synthesize.output\n", "  report: $steps.summarise.output\n"), fragments: []string{"outputs.report", "summarise"}},
	})
}

func TestWorkflowValidationRejectsGraphProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "unknown start", mutate: replace("start: plan\n", "start: planning\n"), fragments: []string{"start", "planning"}},
		{name: "missing start", mutate: replace("start: plan\n", ""), fragments: []string{"start", "required"}},
		{name: "unknown next", mutate: replace("    next: claims\n", "    next: claim\n"), fragments: []string{"steps.research.next", "claim"}},
		{name: "duplicate step id", mutate: replace("  - id: ballots\n", "  - id: claims\n"), fragments: []string{"duplicate step id", "claims"}},
		{name: "invalid step id", mutate: replace("  - id: ballots\n    kind", "  - id: Ballots!\n    kind"), fragments: []string{"Ballots!"}},
		{name: "unreachable step", mutate: replace("    next: ballots\n", "    next: verify\n"), fragments: []string{"steps.ballots", "unreachable"}},
		{name: "cycle", mutate: replace("    retry:\n      max_attempts: 2\n\noutputs:", "    retry:\n      max_attempts: 2\n    next: plan\n\noutputs:"), fragments: []string{"cycle", "plan"}},
		{name: "self loop", mutate: replace("    next: ballots\n", "    next: claims\n"), fragments: []string{"cycle", "claims"}},
		{name: "unknown kind", mutate: replace("    kind: deterministic\n    description: One ballot", "    kind: shell\n    description: One ballot"), fragments: []string{"steps.ballots.kind", "shell"}},
		{name: "reference to a step that may not have run", def: "notes-digest", mutate: replace("      text: $steps.summarize.output.digest\n", "      text: $steps.ask.output.note\n"), fragments: []string{"steps.write.input.text", "ask", "every path"}},
		{name: "reference to a later step", mutate: replace("      max_angles: $inputs.max_angles\n", "      max_angles: $steps.synthesize.output.title\n"), fragments: []string{"steps.plan.input.max_angles", "synthesize"}},
		{name: "unknown field in output", mutate: replace("      angle: $item.question\n", "      angle: $item.topic\n"), fragments: []string{"steps.research.item.input.angle", "topic"}},
		{name: "unknown input", mutate: replace("      question: $inputs.question\n      max_angles", "      question: $inputs.query\n      max_angles"), fragments: []string{"steps.plan.input.question", "query"}},
		{name: "item outside a map", mutate: replace("      question: $inputs.question\n      max_angles", "      question: $item.question\n      max_angles"), fragments: []string{"steps.plan.input.question", "$item"}},
		{name: "malformed reference", mutate: replace("      question: $inputs.question\n      max_angles", "      question: $input.question\n      max_angles"), fragments: []string{"steps.plan.input.question", "$input.question"}},
	})
}

func TestWorkflowValidationRejectsBranchProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "branch without default", def: "notes-digest", mutate: replace("    default: summarize\n", ""), fragments: []string{"steps.check.default", "required"}},
		{name: "branch without cases", def: "notes-digest", mutate: replace("    cases:\n      - when:\n          ref: $steps.read.output.notes\n          op: empty\n        next: ask\n", "    cases: []\n"), fragments: []string{"steps.check.cases"}},
		{name: "unknown case target", def: "notes-digest", mutate: replace("        next: ask\n", "        next: asking\n"), fragments: []string{"steps.check.cases[0].next", "asking"}},
		{name: "unknown operator", def: "notes-digest", mutate: replace("          op: empty\n", "          op: matches\n"), fragments: []string{"steps.check.cases[0].when.op", "matches"}},
		{name: "value type does not fit", def: "notes-digest", mutate: replace("          op: empty\n", "          op: gt\n          value: \"x\"\n"), fragments: []string{"steps.check.cases[0].when", "gt"}},
		{name: "empty with a value", def: "notes-digest", mutate: replace("          op: empty\n", "          op: empty\n          value: 1\n"), fragments: []string{"steps.check.cases[0].when.value"}},
		{name: "branch with retry", def: "notes-digest", mutate: replace("    default: summarize\n", "    default: summarize\n    retry:\n      max_attempts: 2\n"), fragments: []string{"steps.check", "retry"}},
	})
}

func TestWorkflowValidationRejectsLimitsAndRunKeyProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "missing limits", mutate: replace("limits:\n  max_duration: 2h\n  max_attempts: 200\n  max_parallel: 4\n  max_tokens: 12000000\n", ""), fragments: []string{"limits", "required"}},
		{name: "missing token limit", mutate: replace("  max_tokens: 12000000\n", ""), fragments: []string{"limits.max_tokens", "required"}},
		{name: "missing duration", mutate: replace("  max_duration: 2h\n", ""), fragments: []string{"limits.max_duration", "required"}},
		{name: "invalid duration", mutate: replace("  max_duration: 2h\n", "  max_duration: two hours\n"), fragments: []string{"limits.max_duration"}},
		{name: "duration too long", mutate: replace("  max_duration: 2h\n", "  max_duration: 400h\n"), fragments: []string{"limits.max_duration", "168h"}},
		{name: "parallelism too high", mutate: replace("  max_attempts: 200\n  max_parallel: 4\n", "  max_attempts: 200\n  max_parallel: 64\n"), fragments: []string{"limits.max_parallel"}},
		{name: "empty run key", mutate: replace("  inputs: [question]\n", "  inputs: []\n"), fragments: []string{"run_key.inputs"}},
		{name: "missing run key", mutate: replace("run_key:\n  inputs: [question]\n", ""), fragments: []string{"run_key", "required"}},
		{name: "unknown run key input", mutate: replace("  inputs: [question]\n", "  inputs: [query]\n"), fragments: []string{"run_key.inputs", "query"}},
		{name: "repeated run key input", mutate: replace("  inputs: [question]\n", "  inputs: [question, question]\n"), fragments: []string{"run_key.inputs", "question", "more than once"}},
		{name: "optional run key input without default", mutate: func(t *testing.T, dir string) {
			edit(t, dir, "workflow.yaml", "inputs:\n  question:\n", "inputs:\n  note:\n    type: string\n  question:\n")
			edit(t, dir, "workflow.yaml", "  inputs: [question]\n", "  inputs: [question, note]\n")
		}, fragments: []string{"run_key.inputs", "note", "default"}},
		{name: "number run key input", mutate: func(t *testing.T, dir string) {
			edit(t, dir, "workflow.yaml", "inputs:\n  question:\n", "inputs:\n  weight:\n    type: number\n    default: 0.5\n  question:\n")
			edit(t, dir, "workflow.yaml", "  inputs: [question]\n", "  inputs: [question, weight]\n")
		}, fragments: []string{"run_key.inputs", "weight", "number"}},
		{name: "schedule does not feed the run key", def: "notes-digest", mutate: replace("    day: $schedule.date\n", "    day: \"2026-10-04\"\n"), fragments: []string{"run_key", "$schedule"}},
	})
}

func TestWorkflowValidationRejectsInputAndScheduleProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "required with default", mutate: replace("    type: integer\n    default: 4\n", "    type: integer\n    required: true\n    default: 4\n"), fragments: []string{"inputs.max_angles", "required", "default"}},
		{name: "default of the wrong type", mutate: replace("    default: 4\n", "    default: \"four\"\n"), fragments: []string{"inputs.max_angles.default"}},
		{name: "default out of range", mutate: replace("    default: 4\n", "    default: 9\n"), fragments: []string{"inputs.max_angles.default", "maximum"}},
		{name: "unknown input type", mutate: replace("    type: integer\n", "    type: date\n"), fragments: []string{"inputs.max_angles.type", "date"}},
		{name: "length bound on an integer", mutate: replace("    minimum: 1\n", "    minimum: 1\n    max_length: 3\n"), fragments: []string{"inputs.max_angles.max_length"}},
		{name: "bad cron", def: "notes-digest", mutate: replace(`  cron: "0 7 * * 1-5"`, `  cron: "0 25 * * *"`), fragments: []string{"schedule.cron", "25"}},
		{name: "named cron field", def: "notes-digest", mutate: replace(`  cron: "0 7 * * 1-5"`, `  cron: "0 7 * * MON"`), fragments: []string{"schedule.cron", "MON"}},
		{name: "unknown timezone", def: "notes-digest", mutate: replace("  timezone: America/New_York\n", "  timezone: Mars/Olympus\n"), fragments: []string{"schedule.timezone", "Mars/Olympus"}},
		{name: "local timezone", def: "notes-digest", mutate: replace("  timezone: America/New_York\n", "  timezone: Local\n"), fragments: []string{"schedule.timezone", "IANA"}},
		{name: "unbounded catch-up", def: "notes-digest", mutate: replace("    max_runs: 3\n", ""), fragments: []string{"schedule.catch_up.max_runs"}},
		{name: "catch-up too large", def: "notes-digest", mutate: replace("    max_runs: 3\n", "    max_runs: 90\n"), fragments: []string{"schedule.catch_up.max_runs", "31"}},
		{name: "schedule misses a required input", def: "notes-digest", mutate: replace("  inputs:\n    day: $schedule.date\n", "  inputs: {}\n"), fragments: []string{"schedule.inputs", "day"}},
	})
}

func TestWorkflowValidationRejectsUnauthorizedCapabilities(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "unknown capability", def: "notes-digest", env: func(env *Environment) {
			env.Capabilities = NewCatalog()
		}, fragments: []string{"steps.read.capability", "notes.list", "not in the capability catalog"}},
		{name: "contract version mismatch", def: "notes-digest", mutate: func(t *testing.T, dir string) {
			edit(t, dir, "workflow.yaml", "    capability: notes.list\n    contract: 1.0.0\n", "    capability: notes.list\n    contract: 2.0.0\n")
		}, fragments: []string{"steps.read.contract", "2.0.0"}},
		{name: "connection outside Workspace Access", def: "notes-digest", env: func(env *Environment) {
			env.Access = WorkspaceAccess{}
		}, fragments: []string{"conn-notes", "Workspace Access"}},
		{name: "resource outside Workspace Access", def: "notes-digest", env: func(env *Environment) {
			env.Access = WorkspaceAccess{Connections: []ConnectionAccess{{ID: "conn-notes", Resources: []string{"notebook:inbox"}}}}
		}, fragments: []string{"notebook:digest", "Workspace Access"}},
		{name: "step not covered by requested authority", def: "notes-digest", mutate: replace("    resource: \"notebook:digest\"\n", "    resource: \"notebook:inbox\"\n"), fragments: []string{"steps.write", "notebook:inbox", "Standing Authority"}},
		{name: "unused requested resource", def: "notes-digest", mutate: replace(`      resources: ["notebook:digest"]`, `      resources: ["notebook:digest", "notebook:inbox"]`), fragments: []string{"authority.operations[1]", "notebook:inbox", "no step uses"}},
		{name: "unused requested operation", mutate: replace("start: plan\n", "authority:\n  operations:\n    - capability: notes.append\n      connection: conn-notes\n      resources: [\"notebook:digest\"]\n      max_calls_per_run: 1\nstart: plan\n"), fragments: []string{"authority.operations[0]", "no step uses"}},
		{name: "more calls than requested", def: "notes-digest", mutate: replace("    retry:\n      max_attempts: 2\n\noutputs:", "    retry:\n      max_attempts: 2\n    next: again\n  - id: again\n    kind: capability\n    capability: notes.append\n    contract: 1.0.0\n    connection: conn-notes\n    resource: \"notebook:digest\"\n    input:\n      day: $inputs.day\n      text: $steps.summarize.output.digest\n\noutputs:"), fragments: []string{"authority.operations[1]", "max_calls_per_run"}},
		{name: "retried effect without idempotency", def: "notes-digest", env: func(env *Environment) {
			c, _ := env.Capabilities.Capability("notes.append")
			c.Idempotent = false
			l, _ := env.Capabilities.Capability("notes.list")
			env.Capabilities = NewCatalog(l, c)
		}, fragments: []string{"steps.write.retry", "idempotent"}},
		{name: "merge is not a workflow capability", def: "notes-digest", mutate: replace("    capability: notes.append\n    contract: 1.0.0\n", "    capability: subagents.merge\n    contract: 1.0.0\n"), fragments: []string{"steps.write.capability", "merge"}},
		{name: "missing capability input", def: "notes-digest", mutate: replace("      text: $steps.summarize.output.digest\n", ""), fragments: []string{"steps.write.input", "text"}},
		{name: "unknown capability input", def: "notes-digest", mutate: replace("      text: $steps.summarize.output.digest\n", "      text: $steps.summarize.output.digest\n      cc: \"x\"\n"), fragments: []string{"steps.write.input.cc"}},
		{name: "capability input of the wrong type", def: "notes-digest", mutate: replace("      text: $steps.summarize.output.digest\n", "      text: $steps.read.output.notes\n"), fragments: []string{"steps.write.input.text", "string"}},
	})
}

func TestWorkflowValidationRejectsModelPolicyProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "unknown model policy", mutate: replace("    model_policy: planner\n", "    model_policy: planr\n"), fragments: []string{"steps.plan.model_policy", "planr"}},
		{name: "floating model alias", mutate: replace("    model: anthropic/claude-sonnet-4.5\n", "    model: anthropic/claude-sonnet-latest\n"), fragments: []string{"model_policies.writer.model", "latest"}},
		{name: "unknown provider", mutate: replace("    provider: openrouter\n    model: anthropic", "    provider: anthropic\n    model: anthropic"), fragments: []string{"model_policies.writer.provider"}},
		{name: "missing policy version", mutate: replace("  writer:\n    version: 1\n", "  writer:\n"), fragments: []string{"model_policies.writer.version"}},
		{name: "temperature out of range", mutate: replace("    temperature: 0.4\n", "    temperature: 3.5\n"), fragments: []string{"model_policies.writer.temperature"}},
		{name: "missing output token cap", mutate: replace("    max_output_tokens: 12000\n", ""), fragments: []string{"model_policies.writer.max_output_tokens"}},
		{name: "AI step without schema", mutate: replace("    output_schema: schemas/report.schema.json\n", ""), fragments: []string{"steps.synthesize.output_schema", "required"}},
	})
}

func TestWorkflowValidationRejectsMapProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "parallelism above the run limit", mutate: replace("    max_items: 6\n    max_parallel: 4\n", "    max_items: 6\n    max_parallel: 8\n"), fragments: []string{"steps.research.max_parallel", "limits.max_parallel"}},
		{name: "missing max_items", mutate: replace("    max_items: 6\n", ""), fragments: []string{"steps.research.max_items", "required"}},
		{name: "too many items", mutate: replace("    max_items: 36\n", "    max_items: 5000\n"), fragments: []string{"steps.verify.max_items", "1000"}},
		{name: "declared list can exceed max_items", mutate: replace("    max_items: 6\n", "    max_items: 4\n"), fragments: []string{"steps.research.over", "6", "max_items 4"}},
		{name: "unknown failure policy", mutate: replace("    on_item_failure: partial\n    min_succeeded: 1\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/research.md", "    on_item_failure: ignore\n    min_succeeded: 1\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/research.md"), fragments: []string{"steps.research.on_item_failure", "ignore"}},
		{name: "partial without a minimum", mutate: replace("    on_item_failure: partial\n    min_succeeded: 1\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/research.md", "    on_item_failure: partial\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/research.md"), fragments: []string{"steps.research.min_succeeded", "required"}},
		{name: "minimum above max_items", mutate: replace("    min_succeeded: 1\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/research.md", "    min_succeeded: 7\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/research.md"), fragments: []string{"steps.research.min_succeeded", "max_items"}},
		{name: "minimum with fail", mutate: replace("    on_item_failure: partial\n    min_succeeded: 1\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/research.md", "    on_item_failure: fail\n    min_succeeded: 1\n    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/research.md"), fragments: []string{"steps.research.min_succeeded", "partial"}},
		{name: "over a run input", mutate: replace("    over: $steps.plan.output.angles\n", "    over: $inputs.question\n"), fragments: []string{"steps.research.over", "prior step"}},
		{name: "over something that is not a list", mutate: replace("    over: $steps.ballots.output.ballots\n", "    over: $steps.ballots.output.count\n"), fragments: []string{"steps.verify.over", "list"}},
		{name: "nested map", mutate: replace("    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md", "    item:\n      kind: map\n      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md"), fragments: []string{"steps.verify.item.kind", "map"}},
		{name: "capability item", def: "notes-digest", mutate: replace("  - id: summarize\n    kind: ai\n", "  - id: summarize\n    kind: map\n    over: $steps.read.output.notes\n    max_items: 5\n    max_parallel: 1\n    on_item_failure: fail\n    item:\n      kind: capability\n      capability: notes.list\n      contract: 1.0.0\n      connection: conn-notes\n      resource: \"notebook:inbox\"\n      input:\n        day: $item.id\n    next: summarize2\n  - id: summarize2\n    kind: ai\n"), fragments: []string{"steps.summarize.item.kind", "capability"}},
		{name: "item with its own id", mutate: replace("    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md", "    item:\n      id: voter\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md"), fragments: []string{"steps.verify.item", "id"}},
		{name: "map without item", mutate: replace("    item:\n      kind: agent\n      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md\n      input:\n        claim: $item.text\n        cited_sources: $item.sources\n      output_schema: schemas/verdict.schema.json\n      budget:\n        time: 5m\n        tokens: 150000\n", ""), fragments: []string{"steps.verify.item", "required"}},
	})
}

func TestWorkflowValidationRejectsAgentProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "unknown worker preset", mutate: replace("      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md", "      worker: coder\n      model_policy: researcher\n      objective: prompts/verify.md"), fragments: []string{"steps.verify.item.worker", "coder", "research, explore, build"}},
		{name: "missing budget", mutate: replace("      budget:\n        time: 5m\n        tokens: 150000\n", ""), fragments: []string{"steps.verify.item.budget", "required"}},
		{name: "token budget above the worker ceiling", mutate: replace("        tokens: 150000\n", "        tokens: 2000000\n"), fragments: []string{"steps.verify.item.budget.tokens", "1,000,000"}},
		{name: "time budget above the worker ceiling", mutate: replace("        time: 5m\n", "        time: 20m\n"), fragments: []string{"steps.verify.item.budget.time", "15m"}},
		{name: "token budget above the run limit", mutate: replace("  max_tokens: 12000000\n", "  max_tokens: 100000\n"), fragments: []string{"steps.research.item.budget.tokens", "limits.max_tokens"}},
		{name: "missing objective", mutate: replace("      objective: prompts/verify.md\n", ""), fragments: []string{"steps.verify.item.objective", "required"}},
		{name: "missing model policy", mutate: replace("      model_policy: researcher\n      objective: prompts/verify.md\n", "      objective: prompts/verify.md\n"), fragments: []string{"steps.verify.item.model_policy", "required"}},
		{name: "build without a Task", mutate: replace("      worker: research\n      model_policy: researcher\n      objective: prompts/verify.md", "      worker: build\n      model_policy: researcher\n      objective: prompts/verify.md"), fragments: []string{"steps.verify.item.input", "task_id"}},
		{name: "operator ceiling applies", env: func(env *Environment) {
			env.Workers = WorkerCeiling{Deadline: 3 * time.Minute, Tokens: 1_000_000}
		}, fragments: []string{"steps.verify.item.budget.time", "3m"}},
	})
}

func TestWorkflowValidationRejectsFunctionProblems(t *testing.T) {
	runValidationCases(t, []validationCase{
		{name: "function not in the registry", mutate: replace("    function: collect_claims@1\n", "    function: run_shell@1\n"), fragments: []string{"steps.claims.function", "run_shell@1", "reviewed registry"}},
		{name: "unknown function version", mutate: replace("    function: collect_claims@1\n", "    function: collect_claims@2\n"), fragments: []string{"steps.claims.function", "collect_claims@2"}},
		{name: "unversioned function", mutate: replace("    function: collect_claims@1\n", "    function: collect_claims\n"), fragments: []string{"steps.claims.function", "@"}},
		{name: "literal outside the function's bounds", mutate: replace("      votes: 3\n", "      votes: 9\n"), fragments: []string{"steps.ballots.input.votes", "7"}},
		{name: "missing argument", mutate: replace("      quorum: 2\n", ""), fragments: []string{"steps.filter.input", "quorum"}},
		{name: "unknown argument", mutate: replace("      quorum: 2\n", "      quorum: 2\n      strict: true\n"), fragments: []string{"steps.filter.input.strict"}},
		{name: "argument of the wrong type", mutate: replace("      max_claims: 12\n", "      max_claims: \"12\"\n"), fragments: []string{"steps.claims.input.max_claims", "integer"}},
		{name: "argument of the wrong shape", mutate: replace("      findings: $steps.research.output.results\n", "      findings: $steps.plan.output.angles\n"), fragments: []string{"steps.claims.input.findings"}},
		{name: "literal that looks like a reference", mutate: replace("      quorum: 2\n", "      quorum: 2\n      claims_note: \"$5\"\n"), fragments: []string{"steps.filter.input.claims_note"}},
	})
}

func TestWorkflowValidationWarnsWithoutFailing(t *testing.T) {
	dir := copyDefinition(t, "deep-research")
	edit(t, dir, "workflow.yaml", "model_policies:\n", "model_policies:\n  spare:\n    version: 1\n    provider: openrouter\n    model: deepseek/deepseek-v4.1-flash\n    max_output_tokens: 100\n")
	edit(t, dir, "workflow.yaml", "  max_tokens: 12000000\n", "  max_tokens: 5000000\n")
	v := mustValidate(t, dir, testEnvironment())
	warnings := v.Report().Warnings
	for _, want := range []string{"model_policies.spare", "worst-case agent tokens"} {
		found := false
		for _, w := range warnings {
			if contains(w, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no warning mentions %q: %q", want, warnings)
		}
	}
}

func TestWorkflowValidationRequiresAWorkerCeiling(t *testing.T) {
	env := testEnvironment()
	env.Workers = WorkerCeiling{}
	if _, err := validateDir(filepath.Join("testdata", "deep-research"), env); err == nil {
		t.Fatal("validation without a worker ceiling succeeded")
	}
}

// Files are read through os.Root: a link out of the definition is refused
// like a path that climbs out of it.
func TestWorkflowValidationRefusesLinksOutOfTheDefinition(t *testing.T) {
	dir := copyDefinition(t, "deep-research")
	outsideDir := t.TempDir()
	writeFile(t, outsideDir, "outside.md", "Secret instructions.\n")
	if err := os.Remove(filepath.Join(dir, "prompts", "plan.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outsideDir, "outside.md"), filepath.Join(dir, "prompts", "plan.md")); err != nil {
		t.Fatal(err)
	}
	_, err := validateDir(dir, testEnvironment())
	wantProblem(t, err, "steps.plan.prompt", "inside the definition")
}
