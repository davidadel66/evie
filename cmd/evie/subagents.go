package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
)

func configuredSubagentPolicy(getenv func(string) string) (delegation.Policy, error) {
	p := delegation.DefaultPolicy()
	for _, limit := range []struct {
		name  string
		value *int
	}{
		{"PER_PARENT", &p.PerParent}, {"RUNTIME", &p.Runtime}, {"MAX_BATCH", &p.MaxBatch},
		{"MODEL_CALLS", &p.ModelCalls}, {"ASSIGNMENT_BYTES", &p.AssignmentBytes},
		{"REQUEST_BYTES", &p.RequestBytes}, {"RESULT_BYTES", &p.ResultBytes}, {"OUTPUT_TOKENS", &p.OutputTokens},
	} {
		name := "EVIE_SUBAGENTS_" + limit.name
		if raw := getenv(name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return p, fmt.Errorf("%s requires a positive integer", name)
			}
			*limit.value = n
		}
	}
	if raw := getenv("EVIE_SUBAGENTS_DEADLINE"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return p, fmt.Errorf("EVIE_SUBAGENTS_DEADLINE: %w", err)
		}
		p.Deadline = d
	}
	return p, p.Validate()
}
func configureSubagentRuntime(supervisor *subagents.Supervisor, manager *plugins.Manager, client agent.Client, profile openrouter.ContextProfile) {
	supervisor.Configure(client, profile, func(ctx context.Context, receipt *composition.Receipt) (subagents.Composition, error) {
		var resolved plugins.ResolvedComposition
		var err error
		if receipt == nil {
			resolved, err = manager.ResolvePresetContext(ctx, plugins.ResearchPresetID)
		} else {
			resolved, err = manager.ResumeCompositionContext(ctx, *receipt)
		}
		return subagents.Composition{Receipt: resolved.Receipt, Toolset: resolved.Toolset, Instructions: plugins.ResearchInstructions}, err
	})
}

func startSubagentRecovery(ctx context.Context, supervisor *subagents.Supervisor) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := supervisor.RunRecovery(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Subagent recovery stopped: %v", err)
		}
	}()
	return func() { cancel(); <-done }
}
