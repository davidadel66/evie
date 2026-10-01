package main

import (
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
)

func TestSubagentPolicyBudgetsByTimeAndTokens(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	p, err := configuredSubagentPolicy(env(nil))
	if err != nil || p != delegation.DefaultPolicy() || p.Deadline != 15*time.Minute || p.TokenBudget != 1_000_000 || p.PerTurn != 16 {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	p, err = configuredSubagentPolicy(env(map[string]string{
		"EVIE_SUBAGENTS_TOKEN_BUDGET": "250000", "EVIE_SUBAGENTS_PER_TURN": "4", "EVIE_SUBAGENTS_DEADLINE": "5m",
	}))
	if err != nil || p.TokenBudget != 250000 || p.PerTurn != 4 || p.Deadline != 5*time.Minute {
		t.Fatalf("overrides: %+v %v", p, err)
	}
	for _, retired := range []string{"EVIE_SUBAGENTS_MODEL_CALLS", "EVIE_SUBAGENTS_OUTPUT_TOKENS"} {
		if _, err = configuredSubagentPolicy(env(map[string]string{retired: "8"})); err == nil || !strings.Contains(err.Error(), retired) || !strings.Contains(err.Error(), "TOKEN_BUDGET") {
			t.Fatalf("retired %s: %v", retired, err)
		}
	}
	for name, value := range map[string]string{"EVIE_SUBAGENTS_TOKEN_BUDGET": "0", "EVIE_SUBAGENTS_PER_TURN": "-1", "EVIE_SUBAGENTS_TOKEN_BUDGET ": "x"} {
		if _, err = configuredSubagentPolicy(env(map[string]string{strings.TrimSpace(name): value})); err == nil {
			t.Fatalf("%s=%s accepted", name, value)
		}
	}
}
