// Package workflowruntime is the Workflow Runtime's single entry point
// (workflow-runtime.spec.md). Stage 14 sub-stage 2 gives it activation and
// Standing Authority: propose a definition into procedural Git with its
// validation report and dry run, present the approval card (executable and
// authority diffs against the current version), approve one exact version
// for future runs, deactivate it, and list and show definitions. Runs come
// in later sub-stages through this same type.
//
// Validation runs against the bound Workspace: its current Workspace Access
// and the runtime's capability catalog (empty until providers declare
// workflow contracts in sub-stage 5) and the operator's worker ceiling. The
// store re-checks access, the Workspace's worker allowances and the
// reviewed version inside the approval transaction, so a change racing the
// approval refuses it rather than slipping past.
//
// Only owner surfaces call Propose, Approve and Deactivate: the CLI and the
// web management API. No model tool reaches them.
package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
)

// Store is the durable activation state the runtime needs.
type Store interface {
	ListWorkspaces(ctx context.Context, includeArchived bool) ([]memory.Workspace, error)
	WorkspaceAccess(ctx context.Context, id memory.WorkspaceID) (eviedb.WorkspaceAccessRevision, error)
	RecordWorkflowProposal(ctx context.Context, p eviedb.WorkflowProposal) (eviedb.WorkflowProposal, error)
	WorkflowProposal(ctx context.Context, id string) (eviedb.WorkflowProposal, error)
	ActivateWorkflow(ctx context.Context, approval eviedb.WorkflowApproval) (eviedb.WorkflowActivation, error)
	DeactivateWorkflow(ctx context.Context, ws memory.WorkspaceID, name string, version int64, reason, surface string) (eviedb.WorkflowActivation, error)
	WorkflowDefinition(ctx context.Context, ws memory.WorkspaceID, name string) (eviedb.WorkflowDefinitionState, error)
	ListWorkflowDefinitions(ctx context.Context, ws memory.WorkspaceID) ([]eviedb.WorkflowDefinitionState, error)
}

// Config wires a Runtime.
type Config struct {
	Store Store
	Repo  *procedural.Repo
	// Catalog resolves Capability Contracts; nil is the empty catalog.
	Catalog workflow.Catalog
	// Workers is the operator's subagent ceiling bounding agent budgets.
	Workers workflow.WorkerCeiling
}

// Runtime is the Workflow Runtime.
type Runtime struct {
	store   Store
	repo    *procedural.Repo
	catalog workflow.Catalog
	workers workflow.WorkerCeiling
}

// New returns a Runtime. It touches neither Git nor the database.
func New(c Config) (*Runtime, error) {
	if c.Store == nil || c.Repo == nil {
		return nil, errors.New("workflow runtime needs a store and a procedural repository")
	}
	if c.Workers.Deadline <= 0 || c.Workers.Tokens <= 0 {
		return nil, errors.New("workflow runtime needs the operator's worker ceiling")
	}
	if c.Catalog == nil {
		c.Catalog = workflow.NewCatalog()
	}
	return &Runtime{store: c.Store, repo: c.Repo, catalog: c.Catalog, workers: c.Workers}, nil
}

func ownerSurface(surface string) bool { return surface == "cli" || surface == "web" }

func (r *Runtime) environment(access workflow.WorkspaceAccess) workflow.Environment {
	return workflow.Environment{Capabilities: r.catalog, Access: access, Workers: r.workers}
}

// Proposal is the owner proposing the definition in Dir for a Workspace.
// Fixtures are the dry run approval needs; a proposal without them can be
// reviewed, and needs none when it only changes documentation.
type Proposal struct {
	WorkspaceID memory.WorkspaceID
	Dir         string
	Fixtures    *workflow.Fixtures
	Surface     string
}

// Propose validates the definition against the Workspace's current access,
// simulates it with the fixtures, commits exactly its reviewed files to
// procedural Git, re-validates the commit, records the proposal with its
// report and dry run, and returns its approval card. Nothing is activated.
func (r *Runtime) Propose(ctx context.Context, req Proposal) (Card, error) {
	if !ownerSurface(req.Surface) {
		return Card{}, eviedb.ErrWorkflowApprover
	}
	if strings.TrimSpace(req.Dir) == "" {
		return Card{}, errors.New("propose needs a definition directory")
	}
	workspace, err := r.workspace(ctx, req.WorkspaceID)
	if err != nil {
		return Card{}, err
	}
	if workspace.State != memory.WorkspaceActive {
		return Card{}, eviedb.ErrWorkspaceInactive
	}
	access, err := r.store.WorkspaceAccess(ctx, req.WorkspaceID)
	if err != nil {
		return Card{}, err
	}
	env := r.environment(access.Access)
	local, err := workflow.Validate(workflow.DirSource{Dir: req.Dir}, env)
	if err != nil {
		return Card{}, err
	}
	if req.Fixtures != nil {
		// Fixture problems are reported before anything is committed.
		if _, err := local.Simulate(*req.Fixtures); err != nil {
			return Card{}, err
		}
	}
	path := eviedb.WorkflowDefinitionPath(req.WorkspaceID, local.Name())
	authority := workflow.RequirementsOf(local.Report()).AuthorityHash
	message := fmt.Sprintf("Propose workflow %s for Workspace %s\n\nExecutable-Hash: %s\nAuthority-Hash: %s\nProposed-By: owner (%s)\n",
		local.Name(), req.WorkspaceID, local.Hash(), authority, req.Surface)
	commit, _, err := r.repo.Commit(ctx, path, local.Files(), message)
	if err != nil {
		return Card{}, err
	}
	committed, err := workflow.Validate(r.repo.Source(ctx, commit, path), env)
	if err != nil {
		return Card{}, fmt.Errorf("the committed definition did not validate: %w", err)
	}
	if committed.Hash() != local.Hash() {
		return Card{}, fmt.Errorf("the committed definition hashes to %s, not %s", committed.Hash(), local.Hash())
	}
	p := eviedb.WorkflowProposal{
		WorkspaceID: req.WorkspaceID, Name: committed.Name(), Commit: commit, Path: path,
		ExecutableHash: committed.Hash(), AuthorityHash: authority, Report: committed.Report(),
		ProposedBy: "owner", Surface: req.Surface,
	}
	if req.Fixtures != nil {
		if p.Simulation, err = committed.Simulate(*req.Fixtures); err != nil {
			return Card{}, err
		}
	}
	recorded, err := r.store.RecordWorkflowProposal(ctx, p)
	if err != nil {
		return Card{}, err
	}
	return r.card(ctx, recorded)
}

// Card is the approval card: what approving a proposal would bind and
// change, and why it would be refused now.
type Card struct {
	Proposal        eviedb.WorkflowProposal    `json:"proposal"`
	Current         *eviedb.WorkflowActivation `json:"current,omitempty"`
	CurrentStatus   eviedb.WorkflowStatus      `json:"current_status,omitempty"`
	CurrentBlocked  []string                   `json:"current_blocked,omitempty"`
	ExpectedVersion int64                      `json:"expected_version"`
	NeedsApproval   bool                       `json:"needs_approval"`
	Diff            workflow.ReviewDiff        `json:"diff"`
	Requirements    workflow.Requirements      `json:"requirements"`
	Refusals        []string                   `json:"refusals,omitempty"`
	Patch           string                     `json:"patch"`
	ApproveCommand  string                     `json:"approve_command,omitempty"`
}

// Card builds the approval card for a recorded proposal.
func (r *Runtime) Card(ctx context.Context, proposalID string) (Card, error) {
	p, err := r.store.WorkflowProposal(ctx, proposalID)
	if err != nil {
		return Card{}, err
	}
	return r.card(ctx, p)
}

func (r *Runtime) card(ctx context.Context, p eviedb.WorkflowProposal) (Card, error) {
	c := Card{Proposal: p, Requirements: workflow.RequirementsOf(p.Report)}
	state, err := r.store.WorkflowDefinition(ctx, p.WorkspaceID, p.Name)
	if err != nil {
		return Card{}, err
	}
	var base *workflow.Report
	baseCommit := ""
	if cur := state.Current; cur != nil {
		c.Current, c.CurrentStatus, c.CurrentBlocked, c.ExpectedVersion = cur, state.Status, state.Blocked, cur.Version
		known, err := r.store.WorkflowProposal(ctx, cur.ProposalID)
		if err != nil {
			return Card{}, err
		}
		base, baseCommit = &known.Report, cur.Commit
	}
	c.Diff = workflow.DiffReports(base, p.Report)
	c.NeedsApproval = !(state.Status == eviedb.WorkflowActive && c.Current.ExecutableHash == p.ExecutableHash && c.Current.AuthorityHash == p.AuthorityHash)
	if c.Patch, err = r.repo.Diff(ctx, baseCommit, p.Commit, p.Path); err != nil {
		return Card{}, err
	}
	if c.NeedsApproval {
		if c.Refusals, err = r.precheck(ctx, p, c.Requirements); err != nil {
			return Card{}, err
		}
		c.ApproveCommand = fmt.Sprintf("evie workflows approve %s --hash %s --authority %s --expect-version %d", p.ID, p.ExecutableHash, p.AuthorityHash, c.ExpectedVersion)
	}
	return c, nil
}

// precheck lists why approving would be refused now. The store's approval
// transaction is what enforces it; this only tells the reviewer first.
func (r *Runtime) precheck(ctx context.Context, p eviedb.WorkflowProposal, req workflow.Requirements) ([]string, error) {
	var out []string
	if p.Simulation == nil {
		out = append(out, "no dry run: propose it again with fixtures before approving")
	} else if p.Simulation.Status != workflow.SimulationSucceeded {
		out = append(out, fmt.Sprintf("the dry run ended %s: approval needs one that succeeded", p.Simulation.Status))
	}
	w, err := r.workspace(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if w.State != memory.WorkspaceActive {
		return append(out, "the Workspace is archived"), nil
	}
	for _, worker := range req.Workers {
		allowed := false
		switch worker {
		case workflow.WorkerResearch:
			allowed = slices.Contains(w.AllowedPresetIDs, worker)
		case workflow.WorkerExplore:
			allowed = w.Explore.Allowed
		case workflow.WorkerBuild:
			allowed = w.Build.Allowed
		}
		if !allowed {
			out = append(out, fmt.Sprintf("agent steps run %s workers, which this Workspace does not allow; allow them in the Workspace's settings first", worker))
		}
	}
	access, err := r.store.WorkspaceAccess(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out = append(out, workflow.AuthorityOutside(req.Authority, access.Access)...)
	if _, err := r.revalidate(ctx, p, access.Access); err != nil {
		out = append(out, err.Error())
	}
	return out, nil
}

// workspace reads one Workspace with its current revision's settings.
func (r *Runtime) workspace(ctx context.Context, id memory.WorkspaceID) (memory.Workspace, error) {
	workspaces, err := r.store.ListWorkspaces(ctx, true)
	if err != nil {
		return memory.Workspace{}, err
	}
	i := slices.IndexFunc(workspaces, func(w memory.Workspace) bool { return w.ID == id })
	if i < 0 {
		return memory.Workspace{}, fmt.Errorf("%w: %s", eviedb.ErrWorkspaceNotFound, id)
	}
	return workspaces[i], nil
}

// revalidate reads the proposal's exact commit and validates it against the
// current catalog, access and worker ceiling; its hash must be unchanged.
func (r *Runtime) revalidate(ctx context.Context, p eviedb.WorkflowProposal, access workflow.WorkspaceAccess) (*workflow.Validated, error) {
	v, err := workflow.Validate(r.repo.Source(ctx, p.Commit, p.Path), r.environment(access))
	if err != nil {
		return nil, fmt.Errorf("the proposal is no longer valid: %w", err)
	}
	if v.Hash() != p.ExecutableHash {
		return nil, fmt.Errorf("the proposal now hashes to %s, not %s; propose it again", v.Hash(), p.ExecutableHash)
	}
	return v, nil
}

// Approval is the owner approving exactly what a card showed.
type Approval struct {
	ProposalID      string `json:"proposal_id"`
	ExecutableHash  string `json:"executable_hash"`
	AuthorityHash   string `json:"authority_hash"`
	ExpectedVersion int64  `json:"expected_version"`
	Surface         string `json:"-"`
	Detail          string `json:"-"`
}

// Approve re-validates the proposal's exact commit against the Workspace's
// current access, the catalog and the worker ceiling, then records the
// owner's approval, which the store checks again in one transaction. It
// activates the definition, and any schedule it declares, for future runs
// and starts none.
func (r *Runtime) Approve(ctx context.Context, a Approval) (eviedb.WorkflowActivation, error) {
	if !ownerSurface(a.Surface) {
		return eviedb.WorkflowActivation{}, eviedb.ErrWorkflowApprover
	}
	p, err := r.store.WorkflowProposal(ctx, a.ProposalID)
	if err != nil {
		return eviedb.WorkflowActivation{}, err
	}
	if a.ExecutableHash != p.ExecutableHash || a.AuthorityHash != p.AuthorityHash {
		return eviedb.WorkflowActivation{}, eviedb.ErrWorkflowHashMismatch
	}
	access, err := r.store.WorkspaceAccess(ctx, p.WorkspaceID)
	if err != nil {
		return eviedb.WorkflowActivation{}, err
	}
	if _, err := r.revalidate(ctx, p, access.Access); err != nil {
		return eviedb.WorkflowActivation{}, err
	}
	return r.store.ActivateWorkflow(ctx, eviedb.WorkflowApproval{
		ProposalID: a.ProposalID, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash, ExpectedVersion: a.ExpectedVersion,
		Approver: eviedb.WorkflowApprover{Actor: "owner", Surface: a.Surface, Detail: a.Detail},
	})
}

// Deactivation is the owner revoking a definition's current version.
type Deactivation struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	Name        string             `json:"name"`
	Version     int64              `json:"version"`
	Reason      string             `json:"reason,omitempty"`
	Surface     string             `json:"-"`
}

// Deactivate ends the definition's current version: no new run starts.
func (r *Runtime) Deactivate(ctx context.Context, d Deactivation) (eviedb.WorkflowActivation, error) {
	if !ownerSurface(d.Surface) {
		return eviedb.WorkflowActivation{}, eviedb.ErrWorkflowApprover
	}
	return r.store.DeactivateWorkflow(ctx, d.WorkspaceID, d.Name, d.Version, d.Reason, d.Surface)
}

// List lists definitions in a Workspace, or every Workspace when ws is "".
func (r *Runtime) List(ctx context.Context, ws memory.WorkspaceID) ([]eviedb.WorkflowDefinitionState, error) {
	return r.store.ListWorkflowDefinitions(ctx, ws)
}

// Show is one definition's status and history.
func (r *Runtime) Show(ctx context.Context, ws memory.WorkspaceID, name string) (eviedb.WorkflowDefinitionState, error) {
	return r.store.WorkflowDefinition(ctx, ws, name)
}

// Text renders the card for the terminal.
func (c Card) Text() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	p := c.Proposal
	line("Workflow %s in Workspace %s", p.Name, p.WorkspaceID)
	line("Proposal %s (commit %s), proposed by %s via %s at %s", p.ID, short(p.Commit), p.ProposedBy, p.Surface, p.ProposedAt.Format("2006-01-02 15:04:05Z07:00"))
	line("Executable hash: %s", p.ExecutableHash)
	line("Authority hash:  %s", p.AuthorityHash)
	if c.Current == nil {
		line("Current version: none approved")
	} else {
		status := string(c.CurrentStatus)
		if len(c.CurrentBlocked) > 0 {
			status += ": " + strings.Join(c.CurrentBlocked, "; ")
		}
		line("Current version: %d (%s), %s", c.Current.Version, status, c.Current.ExecutableHash)
	}
	line("")
	b.WriteString(c.Diff.Text())
	line("")
	if len(c.Requirements.Authority) == 0 {
		line("Requested Standing Authority: none")
	} else {
		line("Requested Standing Authority")
		for _, op := range c.Requirements.Authority {
			line("  %s on %s: %s, at most %d calls per run", op.Capability, op.Connection, strings.Join(op.Resources, ", "), op.MaxCallsPerRun)
		}
	}
	if len(c.Requirements.Workers) > 0 {
		line("Workers: %s (needs the Workspace's allowance for each)", strings.Join(c.Requirements.Workers, ", "))
	}
	if s := c.Requirements.Schedule; s != nil {
		line("Schedule: %s %s, catch-up %s; approving also activates this schedule (scheduled runs start once schedules ship)", s.Cron, s.Timezone, s.CatchUp)
	} else {
		line("Schedule: none (manual starts only)")
	}
	line("Validation: valid, %d warnings", len(p.Report.Warnings))
	for _, w := range p.Report.Warnings {
		line("  %s", w)
	}
	if sim := p.Simulation; sim != nil {
		summary := fmt.Sprintf("Dry run: %s, %d attempts, %d tokens, %d effect previews (none executed)", sim.Status, sim.Attempts, sim.Tokens, len(sim.Previews))
		if sim.Error != "" {
			summary += ": " + sim.Error
		}
		line("%s", summary)
	} else {
		line("Dry run: none")
	}
	line("")
	switch {
	case !c.NeedsApproval && p.DocumentsVersion > 0:
		line("No approval needed: this changes documentation of active version %d and is recorded.", p.DocumentsVersion)
	case !c.NeedsApproval:
		line("No approval needed: this is the active version.")
	case len(c.Refusals) > 0:
		line("Approval would be refused now:")
		for _, reason := range c.Refusals {
			line("  %s", reason)
		}
	default:
		line("Approving activates exactly this version and its Standing Authority for future runs; it starts no run.")
		line("To approve:")
		line("  %s", c.ApproveCommand)
	}
	if c.Patch != "" {
		line("")
		line("Files")
		b.WriteString(c.Patch)
	}
	return b.String()
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
