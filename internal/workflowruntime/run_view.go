package workflowruntime

import (
	"context"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// What the web UI's run list and run panel read besides the run records
// (workspace-activity.spec.md story 2): each run's step states, and the
// steps of the exact version a run is pinned to.

// RunStepStates is each run's steps with the state of their latest attempt.
func (r *Runtime) RunStepStates(ctx context.Context, runIDs []string) (map[string]map[string]string, error) {
	return r.store.WorkflowRunStepStates(ctx, runIDs)
}

// DefinitionView is one version's workflow.yaml and its step outline.
type DefinitionView struct {
	WorkspaceID memory.WorkspaceID     `json:"workspace_id"`
	Name        string                 `json:"name"`
	Version     int64                  `json:"version"`
	Commit      string                 `json:"commit"`
	Source      string                 `json:"source"`
	Steps       []workflow.StepOutline `json:"steps"`
}

// RunDefinition reads the workflow.yaml a run is pinned to, at its commit,
// and outlines its steps. It reads no other file of the definition.
func (r *Runtime) RunDefinition(ctx context.Context, runID string) (DefinitionView, error) {
	run, err := r.store.WorkflowRunRecord(ctx, runID)
	if err != nil {
		return DefinitionView{}, err
	}
	data, err := r.repo.Source(ctx, run.Commit, run.Path).ReadFile("workflow.yaml")
	if err != nil {
		return DefinitionView{}, fmt.Errorf("read %s version %d's workflow.yaml at commit %s: %w", run.Workflow, run.Version, short(run.Commit), err)
	}
	steps, err := workflow.Outline(data)
	if err != nil {
		return DefinitionView{}, fmt.Errorf("outline %s version %d: %w", run.Workflow, run.Version, err)
	}
	return DefinitionView{WorkspaceID: run.WorkspaceID, Name: run.Workflow, Version: run.Version, Commit: run.Commit, Source: string(data), Steps: steps}, nil
}
