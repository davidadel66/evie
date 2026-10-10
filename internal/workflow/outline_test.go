package workflow_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/workflow"
)

// workspace-activity.spec.md: the web UI shows a definition's steps in YAML
// order, where each leads, and the lines that define it.
func TestOutlineListsStepsInYAMLOrderWithTheirLines(t *testing.T) {
	src := `schema_version: 1
name: review
description: Review proposals.

start: gather

steps:
  - id: gather
    kind: capability
    description: Read the candidates.
    capability: finance.review_candidates
    retry:
      max_attempts: 2
      backoff: 1m
    next: any

  - id: any
    kind: branch
    cases:
      - when:
          ref: $steps.gather.output.count
          op: eq
          value: 0
        next: nothing
    default: review

  - id: review
    kind: interrupt
    description: The owner answers.
    message: |
      Review the lines.
      Answer {} to accept.
    next: nothing

  - id: nothing
    kind: deterministic
    function: require_zero@1
`
	got, err := workflow.Outline([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []workflow.StepOutline{
		{ID: "gather", Kind: "capability", Description: "Read the candidates.", Next: []string{"any"}, MaxAttempts: 2, FirstLine: 8, LastLine: 15},
		{ID: "any", Kind: "branch", Next: []string{"nothing", "review"}, FirstLine: 17, LastLine: 25},
		{ID: "review", Kind: "interrupt", Description: "The owner answers.", Next: []string{"nothing"}, FirstLine: 27, LastLine: 33},
		{ID: "nothing", Kind: "deterministic", FirstLine: 35, LastLine: 37},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outline =\n%+v\nwant\n%+v", got, want)
	}
	lines := strings.Split(src, "\n")
	for _, s := range got {
		if !strings.Contains(lines[s.FirstLine-1], "id: "+s.ID) {
			t.Fatalf("step %s starts on line %d: %q", s.ID, s.FirstLine, lines[s.FirstLine-1])
		}
	}
}

func TestOutlineRefusesWhatTheParserRefuses(t *testing.T) {
	for name, src := range map[string]string{
		"empty":   "",
		"anchors": "schema_version: 1\nname: &n x\nsteps: []\n",
		"unknown": "schema_version: 1\nname: x\nsurprise: true\n",
	} {
		if _, err := workflow.Outline([]byte(src)); err == nil {
			t.Fatalf("%s: outlined", name)
		}
	}
}
