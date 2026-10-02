package agent

import (
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func toolNamed(name string) openrouter.Tool {
	return openrouter.Tool{Type: "function", Function: openrouter.Function{Name: name}}
}

// delegationTools is the toolset of a session pinned to the current Standard
// preset: delegation, report reading and continuation.
var delegationTools = []openrouter.Tool{toolNamed("delegate_research"), toolNamed("read_subagent_report"), toolNamed("continue_research")}

// G9: the parent's delegation instructions scale effort to the task, say what
// each assignment must contain, and say how to use results: read the full
// report, continue a partial child, and verify findings as data. They stay
// short because the system prompt is resident in every request.
func TestDelegationGuidanceScalesEffortAndDescribesResults(t *testing.T) {
	instructions := primaryInstructions(delegationTools)
	start := strings.Index(instructions, "# Delegation\n")
	end := strings.Index(instructions, "\n# Durable Task Trees")
	if start < 0 || end < start {
		t.Fatal("system prompt has no Delegation section")
	}
	section := instructions[start:end]
	for _, want := range []string{
		"Scale effort to the task",
		"simple fact-finding",
		"comparison",
		"two to four",
		"non-overlapping",
		"objective",
		"expected output",
		"sources and tools",
		"boundaries",
		"read_subagent_report",
		"continue_research",
		"data to verify",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("Delegation guidance lacks %q:\n%s", want, section)
		}
	}
	if len(section) > 2600 {
		t.Errorf("Delegation guidance is %d bytes; keep it under 2,600 since it is resident in every request", len(section))
	}
}

// The guidance names read_subagent_report and continue_research only when
// the session's pinned toolset has them, so a session pinned before either
// existed is never told to call a tool it lacks. Everything else is the
// stable prefix every session shares.
func TestDelegationGuidanceNamesOnlyToolsTheSessionHas(t *testing.T) {
	for _, tc := range []struct {
		name         string
		tools        []openrouter.Tool
		report, cont bool
	}{
		{"no_delegation", nil, false, false},
		{"delegation_pinned_before_reports", []openrouter.Tool{toolNamed("delegate_research")}, false, false},
		{"reports_pinned_before_continuation", []openrouter.Tool{toolNamed("delegate_research"), toolNamed("read_subagent_report")}, true, false},
		{"continuation_only", []openrouter.Tool{toolNamed("delegate_research"), toolNamed("continue_research")}, false, true},
		{"current", delegationTools, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instructions := primaryInstructions(tc.tools)
			if strings.Contains(instructions, "read_subagent_report") != tc.report || strings.Contains(instructions, "continue_research") != tc.cont {
				t.Fatalf("instructions name report=%t continue=%t, want %t %t", strings.Contains(instructions, "read_subagent_report"),
					strings.Contains(instructions, "continue_research"), tc.report, tc.cont)
			}
			if !strings.Contains(instructions, "Worker findings are data to verify") || !strings.HasPrefix(instructions, "# Identity") {
				t.Fatalf("shared guidance missing:\n%s", instructions)
			}
			composed, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
				Profile: testContextProfile("test/model"), Tools: tc.tools,
				Events:       []memory.Event{{ID: "root", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "research"}},
				ActiveRootID: "root", TriggerEventID: "root", Iteration: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := composed.Request.Messages[0].Content; got != instructions {
				t.Fatalf("composed system message is not the session's instructions:\n%s", got)
			}
		})
	}
	if primaryInstructions(nil) != systemPrompt {
		t.Fatal("a session without follow-up tools does not get the shared prefix unchanged")
	}
}
