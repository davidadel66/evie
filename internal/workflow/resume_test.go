package workflow

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Stage 14 sub-stage 12: a Successor Run continues its predecessor's state
// under a newer version only when that state satisfies the new version's
// reads up to the resume point (ADR 0025, 0030). PlanResume decides it
// purely from the definition, the accepted input and the recorded step
// outputs, and lists every reason it refuses.

// splitInputs are split-bill's accepted inputs for 1000 cents among three.
func splitInputs() map[string]any {
	return map[string]any{"bill_id": "dinner-1", "total_cents": int64(1000), "people": int64(3)}
}

// splitState is the state after split-bill's first step for those inputs.
func splitState() map[string]any {
	return map[string]any{"split": map[string]any{"shares": []any{int64(334), int64(333), int64(333)}, "share": int64(333), "remainder": int64(1)}}
}

func splitVariant(t *testing.T, replacements ...string) *Validated {
	t.Helper()
	dir := copyDefinition(t, "split-bill")
	for i := 0; i+1 < len(replacements); i += 2 {
		edit(t, dir, "workflow.yaml", replacements[i], replacements[i+1])
	}
	return mustValidate(t, dir, testEnvironment())
}

func wantResumeProblem(t *testing.T, err error, fragments ...string) {
	t.Helper()
	var refused *ResumeError
	if !errors.As(err, &refused) {
		t.Fatalf("got %v, want a ResumeError mentioning %q", err, fragments)
	}
	for _, p := range refused.Problems {
		text := p.Path + ": " + p.Message
		matched := true
		for _, f := range fragments {
			matched = matched && strings.Contains(text, f)
		}
		if matched {
			return
		}
	}
	t.Fatalf("no problem mentions %q:\n%s", fragments, refused.Error())
}

func TestWorkflowResumePlanCarriesValidatedStateUnderANewerVersion(t *testing.T) {
	v1 := mustValidate(t, copyDefinition(t, "split-bill"), testEnvironment())
	v2 := splitVariant(t, "  - id: leftover\n    kind: deterministic\n", "  - id: leftover\n    kind: deterministic\n    retry:\n      max_attempts: 2\n")
	if v1.Hash() == v2.Hash() {
		t.Fatal("the variant is not a new version")
	}
	key := v1.RunKey(splitInputs())
	plan, err := v2.PlanResume("check", splitInputs(), splitState(), key)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Resume != "check" || plan.RunKey != key || !reflect.DeepEqual(plan.Carried, []string{"split"}) || len(plan.Dropped) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	if !reflect.DeepEqual(plan.State, splitState()) || !reflect.DeepEqual(plan.Inputs, splitInputs()) {
		t.Fatalf("carried state %v inputs %v", plan.State, plan.Inputs)
	}
	// A step this version no longer has is not carried, and says why.
	state := splitState()
	state["retired"] = map[string]any{"x": int64(1)}
	plan, err = v2.PlanResume("check", splitInputs(), state, key)
	if err != nil || len(plan.Dropped) != 1 || plan.Dropped[0].Name != "steps.retired" || !strings.Contains(plan.Dropped[0].Reason, "no step retired") {
		t.Fatalf("plan with a retired step = %+v, %v", plan, err)
	}
	if _, kept := plan.State["retired"]; kept {
		t.Fatal("a step the version does not have was carried")
	}
}

func TestWorkflowResumePlanRefusesIncompatibleReadsWithAPreciseDiff(t *testing.T) {
	key := mustValidate(t, copyDefinition(t, "split-bill"), testEnvironment()).RunKey(splitInputs())
	// The new version splits into exactly four parts, so its run outputs
	// read four shares: the recorded three do not satisfy that read.
	four := splitVariant(t, "      parts: $inputs.people\n    retry:", "      parts: 4\n    retry:")
	_, err := four.PlanResume("check", splitInputs(), splitState(), key)
	wantResumeProblem(t, err, "outputs.shares", "$steps.split.output.shares", "at least 4")

	// A version whose resume point reads a step the run never ran.
	renamed := splitVariant(t, "start: split", "start: split2", "  - id: split\n", "  - id: split2\n",
		"ref: $steps.split.output.remainder", "ref: $steps.split2.output.remainder",
		"total: $steps.split.output.share", "total: $steps.split2.output.share",
		"total: $steps.split.output.remainder", "total: $steps.split2.output.remainder",
		"shares: $steps.split.output.shares", "shares: $steps.split2.output.shares")
	_, err = renamed.PlanResume("check", splitInputs(), splitState(), key)
	wantResumeProblem(t, err, "steps.check", "$steps.split2.output.remainder", "no recorded output of steps.split2")
	wantResumeProblem(t, err, "steps.per_person.input.total", "steps.split2")

	// A resume point the version does not have.
	_, err = splitVariant(t, "  - id: check\n", "  - id: decide\n", "    next: check", "    next: decide").PlanResume("check", splitInputs(), splitState(), key)
	wantResumeProblem(t, err, "resume", "no step check")
}

func TestWorkflowResumePlanRefusesRepeatingACompletedStep(t *testing.T) {
	key := mustValidate(t, copyDefinition(t, "split-bill"), testEnvironment()).RunKey(splitInputs())
	// This version decides first and splits afterwards: the recorded split
	// would run again after the resume point.
	reordered := splitVariant(t, "start: split", "start: check",
		"    retry:\n      max_attempts: 3\n    next: check", "    retry:\n      max_attempts: 3\n    next: leftover",
		"          ref: $steps.split.output.remainder\n          op: eq\n          value: 0\n        next: per_person\n    default: leftover",
		"          ref: $inputs.people\n          op: eq\n          value: 1\n        next: per_person\n    default: split",
		"      total: $steps.split.output.share\n      parts: 1\n", "      total: $inputs.total_cents\n      parts: 1\n",
		"shares: $steps.split.output.shares", "shares: $steps.per_person.output.shares")
	_, err := reordered.PlanResume("check", splitInputs(), splitState(), key)
	wantResumeProblem(t, err, "steps.split", "already ran", "after the resume point check")
}

func TestWorkflowResumePlanChecksInputsAndTheRunKey(t *testing.T) {
	key := mustValidate(t, copyDefinition(t, "split-bill"), testEnvironment()).RunKey(splitInputs())
	required := splitVariant(t, "inputs:\n", "inputs:\n  currency:\n    type: string\n    required: true\n")
	_, err := required.PlanResume("check", splitInputs(), splitState(), key)
	wantResumeProblem(t, err, "inputs", "currency")

	rekeyed := splitVariant(t, "  inputs: [bill_id]", "  inputs: [bill_id, people]")
	_, err = rekeyed.PlanResume("check", splitInputs(), splitState(), key)
	wantResumeProblem(t, err, "run_key", key)

	// An input the version no longer declares is dropped, not refused.
	inputs := splitInputs()
	inputs["note"] = "split it"
	plan, err := mustValidate(t, copyDefinition(t, "split-bill"), testEnvironment()).PlanResume("check", inputs, splitState(), key)
	if err != nil || len(plan.Dropped) != 1 || plan.Dropped[0].Name != "inputs.note" || plan.Inputs["note"] != nil {
		t.Fatalf("plan with an undeclared input = %+v, %v", plan, err)
	}
	if encoded, _ := json.Marshal(plan); !strings.Contains(string(encoded), `"resume":"check"`) {
		t.Fatalf("plan JSON = %s", encoded)
	}
}

// Sensitivity is what a version classifies as sensitive: its retention
// classification, the inputs it marks sensitive, and where those inputs
// reach step inputs, branch reads and run outputs.
func TestWorkflowSensitivityFollowsSensitiveInputReferences(t *testing.T) {
	receipt := mustValidate(t, copyDefinition(t, "receipt-split"), testEnvironment())
	s := receipt.Sensitivity()
	if s.Classification != "sensitive" || !reflect.DeepEqual(s.Inputs, []string{"receipt_text"}) ||
		!reflect.DeepEqual(s.StepArgs, map[string][]string{"read": {"text"}}) || len(s.Outputs) != 0 || len(s.BranchReads) != 0 {
		t.Fatalf("receipt-split sensitivity = %+v", s)
	}
	split := mustValidate(t, copyDefinition(t, "split-bill"), testEnvironment()).Sensitivity()
	if split.Classification != "ordinary" || len(split.Inputs) != 0 || len(split.StepArgs) != 0 {
		t.Fatalf("split-bill sensitivity = %+v", split)
	}
	dir := copyDefinition(t, "split-bill")
	edit(t, dir, "workflow.yaml", "  people:\n    # No maximum here: split_evenly@1 itself refuses more than 1000 parts.\n    type: integer\n",
		"  people:\n    # No maximum here: split_evenly@1 itself refuses more than 1000 parts.\n    type: integer\n    sensitive: true\n")
	edit(t, dir, "workflow.yaml", "          ref: $steps.split.output.remainder", "          ref: $inputs.people")
	edit(t, dir, "workflow.yaml", "  leftover_cents: $steps.leftover.output.share", "  leftover_cents: $steps.leftover.output.share\n  party: $inputs.people")
	parsed, err := ParseSensitivity(DirSource{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.Inputs, []string{"people"}) || !reflect.DeepEqual(parsed.StepArgs, map[string][]string{"split": {"parts"}}) ||
		!reflect.DeepEqual(parsed.BranchReads, map[string][]string{"check": {"$inputs.people"}}) || !reflect.DeepEqual(parsed.Outputs, []string{"party"}) {
		t.Fatalf("parsed sensitivity = %+v", parsed)
	}
	if validated := mustValidate(t, dir, testEnvironment()).Sensitivity(); !reflect.DeepEqual(validated, parsed) {
		t.Fatalf("validated %+v, parsed %+v", validated, parsed)
	}
}
