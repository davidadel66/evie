package workflowruntime

import (
	"context"
	"fmt"

	"github.com/davidadel66/evie/internal/eviedb"
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

// DefinitionView is one version's workflow.yaml and its outline. A
// proposal not yet approved has version 0 and names the proposal.
type DefinitionView struct {
	WorkspaceID memory.WorkspaceID     `json:"workspace_id"`
	Name        string                 `json:"name"`
	Version     int64                  `json:"version"`
	Proposal    string                 `json:"proposal,omitempty"`
	Commit      string                 `json:"commit"`
	Source      string                 `json:"source"`
	Description string                 `json:"description,omitempty"`
	Steps       []workflow.StepOutline `json:"steps"`
}

// RunDefinition reads the workflow.yaml a run is pinned to, at its commit,
// and outlines its steps. It reads no other file of the definition.
func (r *Runtime) RunDefinition(ctx context.Context, runID string) (DefinitionView, error) {
	run, err := r.store.WorkflowRunRecord(ctx, runID)
	if err != nil {
		return DefinitionView{}, err
	}
	return r.definitionView(ctx, DefinitionView{WorkspaceID: run.WorkspaceID, Name: run.Workflow, Version: run.Version, Commit: run.Commit}, run.Path)
}

// WorkflowDefinitionView reads a Workspace workflow's workflow.yaml for the
// Workflows tab: its current version at its recorded commit, or, when no
// version was ever approved, its newest proposal waiting for approval.
func (r *Runtime) WorkflowDefinitionView(ctx context.Context, ws memory.WorkspaceID, name string) (DefinitionView, error) {
	state, err := r.store.WorkflowDefinition(ctx, ws, name)
	if err != nil {
		return DefinitionView{}, err
	}
	if a := state.Current; a != nil {
		return r.definitionView(ctx, DefinitionView{WorkspaceID: ws, Name: name, Version: a.Version, Commit: a.Commit}, a.Path)
	}
	for _, summary := range state.Proposals {
		if !summary.Pending {
			continue
		}
		read := r.store.WorkflowProposal
		if state.Source == eviedb.WorkflowSourceGlobal {
			read = r.store.GlobalWorkflowProposal
		}
		p, err := read(ctx, summary.ID)
		if err != nil {
			return DefinitionView{}, err
		}
		return r.definitionView(ctx, DefinitionView{WorkspaceID: ws, Name: name, Proposal: p.ID, Commit: p.Commit}, p.Path)
	}
	return DefinitionView{}, fmt.Errorf("%w: %s has no approved version and no proposal waiting", eviedb.ErrWorkflowNotFound, name)
}

// definitionView reads workflow.yaml at the view's commit and outlines it;
// it reads no other file of the definition.
func (r *Runtime) definitionView(ctx context.Context, v DefinitionView, path string) (DefinitionView, error) {
	data, err := r.repo.Source(ctx, v.Commit, path).ReadFile("workflow.yaml")
	if err != nil {
		return v, fmt.Errorf("read %s's workflow.yaml at commit %s: %w", v.Name, short(v.Commit), err)
	}
	outline, err := workflow.Outline(data)
	if err != nil {
		return v, fmt.Errorf("outline %s: %w", v.Name, err)
	}
	v.Source, v.Description, v.Steps = string(data), outline.Description, outline.Steps
	return v, nil
}
