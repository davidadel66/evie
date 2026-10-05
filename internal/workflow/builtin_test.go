package workflow

import (
	"strings"
	"testing"
)

// Built-in workflows (owner decision 2026-10-04): a built-in or Global
// definition holds nothing a Workflow Approval grants. The rule refuses
// capability steps (reads and effects), requested Standing Authority, a
// schedule and build workers, and admits read-only research and explore
// workers.

func TestBuiltinQualificationAdmitsAReadOnlyResearchDefinition(t *testing.T) {
	v := mustValidate(t, copyDefinition(t, "deep-research"), testEnvironment())
	if problems := QualifyBuiltin(v.Report()); len(problems) != 0 {
		t.Fatalf("deep research does not qualify: %v", problems)
	}
}

func TestBuiltinQualificationRefusesEffectsAuthorityAndSchedules(t *testing.T) {
	v := mustValidate(t, copyDefinition(t, "notes-digest"), testEnvironment())
	problems := strings.Join(QualifyBuiltin(v.Report()), "\n")
	for _, want := range []string{
		"steps.read calls a capability",
		"steps.write calls a capability",
		"it requests Standing Authority",
		"it declares a schedule",
	} {
		if !strings.Contains(problems, want) {
			t.Errorf("problems lack %q:\n%s", want, problems)
		}
	}
}

func TestBuiltinQualificationRefusesBuildWorkers(t *testing.T) {
	dir := copyDefinition(t, "deep-research")
	// The research map's item becomes a build worker on a Task.
	edit(t, dir, "workflow.yaml", "      worker: research\n      model_policy: researcher\n      objective: prompts/research.md", "      worker: build\n      model_policy: researcher\n      objective: prompts/research.md")
	edit(t, dir, "workflow.yaml", "        angle_id: $item.id\n", "        angle_id: $item.id\n        task_id: $item.id\n")
	v := mustValidate(t, dir, testEnvironment())
	problems := QualifyBuiltin(v.Report())
	if len(problems) != 1 || !strings.Contains(problems[0], "steps.research runs build workers") || !strings.Contains(problems[0], "read-only research and explore") {
		t.Fatalf("problems = %v", problems)
	}
}
