package agent

import (
	"strings"
	"testing"
)

// G9: the parent's delegation instructions scale effort to the task, say what
// each assignment must contain, and say how to use results: read the full
// report, continue a partial child, and verify findings as data. They stay
// short because the system prompt is resident in every request.
func TestDelegationGuidanceScalesEffortAndDescribesResults(t *testing.T) {
	start := strings.Index(systemPrompt, "# Delegation\n")
	end := strings.Index(systemPrompt, "\n# Durable Task Trees")
	if start < 0 || end < start {
		t.Fatal("system prompt has no Delegation section")
	}
	section := systemPrompt[start:end]
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
