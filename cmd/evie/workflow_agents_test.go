package main

import (
	"context"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
)

// Agent steps' workers (Stage 14 sub-stage 9) run through the Kernel
// subagent supervisor only when the owner allowed remote workflow models,
// as AI steps do: their workers send the step's input to the model
// provider. Without that, or without a supervisor, agent steps are off with
// the reason a start or a paused run shows.
func TestWorkflowWorkersFollowTheRemoteModelSetting(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	supervisor, err := subagents.New(store, delegation.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := newWorkflowPluginManager(ctx, store, supervisor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetPluginEnabled(ctx, string(plugins.SubagentsPluginID), true); err != nil {
		t.Fatal(err)
	}
	env := func(values map[string]string) func(string) string { return func(k string) string { return values[k] } }
	agents, off, err := workflowWorkers(env(nil), supervisor, manager)
	if err != nil || agents != nil || !strings.Contains(off, "EVIE_WORKFLOW_REMOTE_MODELS=on") {
		t.Fatalf("default = %v, %q, %v", agents, off, err)
	}
	agents, off, err = workflowWorkers(env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "on"}), supervisor, manager)
	if err != nil || agents != nil || !strings.Contains(off, "OPENROUTER_API_KEY") {
		t.Fatalf("on without a key = %v, %q, %v", agents, off, err)
	}
	agents, off, err = workflowWorkers(env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "on", "OPENROUTER_API_KEY": "k"}), nil, manager)
	if err != nil || agents != nil || !strings.Contains(off, "supervisor") {
		t.Fatalf("without a supervisor = %v, %q, %v", agents, off, err)
	}
	agents, off, err = workflowWorkers(env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "on", "OPENROUTER_API_KEY": "k"}), supervisor, manager)
	if err != nil || agents == nil || off != "" {
		t.Fatalf("on = %v, %q, %v", agents, off, err)
	}
	// The worker presets compose through the command's Plugin Manager, so
	// a run can pin them.
	if digest, err := agents.RunWorkerPin(ctx, "research"); err != nil || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("research pin = %q, %v", digest, err)
	}
	if _, _, err := workflowWorkers(env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "yes"}), supervisor, manager); err == nil {
		t.Fatal("an unknown setting was accepted")
	}
	rt, err := newWorkflowRuntime(store, env(map[string]string{"HOME": t.TempDir()}), manager, supervisor, manager)
	if err != nil || rt == nil {
		t.Fatalf("runtime = %v, %v", rt, err)
	}
}
