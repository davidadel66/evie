package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// The GitHub Plugin (changes.spec.md story 4b) contributes one workflow
// effect, github.open_change: push one of Evie's draft Changes in the
// run's Workspace Repository and open its pull request, as the owner's
// `evie changes push` does, but only for a branch that passes the
// Workspace's Verify command and with the pull request text the workflow
// gives. A step names its build by the execution_id an agent step outputs
// and the Repository by the resource repo:<owner>/<name> on connection
// github; the Workspace's Access must list that resource and the
// workflow's Standing Authority must allow the call.
//
// It is idempotent: a second request for the same branch finds the open
// pull request instead of opening another. Reconcile looks the branch's
// pull request up in any state, and none proves the effect did not
// happen; pushing the branch again is harmless. The default Resource
// Conflict Keys serialize every open on one Repository.

const (
	GitHubPluginID               PluginID     = "github"
	GitHubOpenChangeCapabilityID CapabilityID = "github.open_change"
	// GitHubConnectionID is the connection github.open_change is served on;
	// its resources are repo:<owner>/<name>.
	GitHubConnectionID = "github"
	// GitHubResourcePrefix starts every resource github.open_change
	// serves: repo:<owner>/<name>.
	GitHubResourcePrefix = "repo:"

	githubContractVersion = "1.0.0"
)

// ChangeOpener pushes and finds Evie's Changes for workflows. OpenChange
// errors that wrap ErrWorkflowEffectNotPerformed prove no pull request was
// opened; any other leaves the outcome unknown.
type ChangeOpener interface {
	OpenChange(ctx context.Context, ws memory.WorkspaceID, resource, execution string, text delegation.ChangeText) (OpenedChange, error)
	// FindChange is the pull request, open or finished, on the branch of
	// the Change built by execution, or nil when it has none.
	FindChange(ctx context.Context, ws memory.WorkspaceID, resource, execution string) (*OpenedChange, error)
}

// OpenedChange is a Change's branch and pull request.
type OpenedChange struct {
	Branch      string
	PullRequest int
	URL         string
	// Existing is a pull request the branch already had.
	Existing bool
}

// GitHub is the GitHub Plugin.
type GitHub struct{ opener ChangeOpener }

// NewGitHub is the GitHub Plugin; a nil opener (no token, or a process
// that only reviews definitions) refuses every request.
func NewGitHub(opener ChangeOpener) GitHub { return GitHub{opener: opener} }

func (GitHub) Start(context.Context) error { return nil }
func (GitHub) Stop(context.Context) error  { return nil }

func (GitHub) Manifest() Manifest {
	return Manifest{ID: GitHubPluginID, ImplementationVersion: "1.0.0", KernelCompatibility: VersionRange{Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0"},
		Capabilities: []CapabilityContract{{ID: GitHubOpenChangeCapabilityID, Version: githubContractVersion}}}
}

// WorkflowEffects is github.open_change.
func (g GitHub) WorkflowEffects() []WorkflowEffectCapability {
	return []WorkflowEffectCapability{{
		ID: GitHubOpenChangeCapabilityID, ContractVersion: githubContractVersion,
		Input: workflow.ObjectType(
			workflow.Field{Name: "execution_id", Type: boundedString(1, delegation.MaxKeyBytes)},
			workflow.Field{Name: "title", Type: boundedString(1, delegation.MaxChangeTitleRunes)},
			workflow.Field{Name: "description", Type: boundedString(1, delegation.MaxChangeDescriptionBytes)},
		),
		Output: workflow.ObjectType(
			workflow.Field{Name: "branch", Type: workflow.StringType()},
			workflow.Field{Name: "pull_request", Type: workflow.IntegerType()},
			workflow.Field{Name: "url", Type: workflow.StringType()},
			workflow.Field{Name: "existing", Type: workflow.BooleanType()},
		),
		// Repositories differ per Workspace: Workspace Access names the
		// exact repo:<owner>/<name>, and the opener checks it is the
		// Workspace's own.
		Connections:    []string{GitHubConnectionID},
		ResourcePrefix: GitHubResourcePrefix,
		Idempotent:     true,
		Perform:        g.openChange,
		Reconcile:      g.findChange,
	}}
}

// openChangeInput is github.open_change's input.
type openChangeInput struct {
	ExecutionID string `json:"execution_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// openChangeRequest refuses a request outside connection github, without
// a run's Workspace, or with input the contract does not allow; each is
// proof nothing happened.
func (g GitHub) openChangeRequest(req WorkflowEffectRequest) (openChangeInput, error) {
	var in openChangeInput
	switch {
	case g.opener == nil:
		return in, EffectNotPerformed(errors.New("GitHub is not connected in this process: add GITHUB_TOKEN to ~/.evie/.env and restart evie serve"))
	case req.Connection != GitHubConnectionID:
		return in, EffectNotPerformed(fmt.Errorf("github.open_change is served on connection %s, not %s", GitHubConnectionID, req.Connection))
	case req.WorkspaceID == "":
		return in, EffectNotPerformed(errors.New("github.open_change opens Changes of a Workspace's Repository, and this run has no Workspace"))
	}
	if err := decodeFinanceInput(req.Input, &in); err != nil {
		return in, EffectNotPerformed(err)
	}
	if strings.TrimSpace(in.ExecutionID) == "" {
		return in, EffectNotPerformed(errors.New("execution_id is blank: give the build step's execution_id"))
	}
	return in, nil
}

func (g GitHub) openChange(ctx context.Context, req WorkflowEffectRequest) (any, error) {
	in, err := g.openChangeRequest(req)
	if err != nil {
		return nil, err
	}
	opened, err := g.opener.OpenChange(ctx, req.WorkspaceID, req.Resource, strings.TrimSpace(in.ExecutionID), delegation.ChangeText{Title: in.Title, Description: in.Description})
	if err != nil {
		return nil, err
	}
	return openedValue(opened), nil
}

func (g GitHub) findChange(ctx context.Context, req WorkflowEffectRequest) (WorkflowEffectLookup, error) {
	in, err := g.openChangeRequest(req)
	if err != nil {
		return WorkflowEffectLookup{}, err
	}
	found, err := g.opener.FindChange(ctx, req.WorkspaceID, req.Resource, strings.TrimSpace(in.ExecutionID))
	if err != nil {
		return WorkflowEffectLookup{}, err
	}
	if found == nil {
		return WorkflowEffectLookup{Outcome: WorkflowEffectDidNotHappen, Evidence: "no pull request on GitHub has the build's branch as its head"}, nil
	}
	return WorkflowEffectLookup{Outcome: WorkflowEffectHappened, Output: openedValue(*found),
		Evidence: fmt.Sprintf("pull request #%d has the build's branch %s as its head", found.PullRequest, found.Branch)}, nil
}

func openedValue(o OpenedChange) map[string]any {
	return map[string]any{"branch": o.Branch, "pull_request": int64(o.PullRequest), "url": o.URL, "existing": o.Existing}
}
