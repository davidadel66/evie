package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Built-in and Global workflows (owner decision 2026-10-04; the "Built-in
// workflows" amendment of workflow-runtime.spec.md).
//
// A built-in is a reviewed definition compiled into Evie
// (internal/workflow's embedded definitions) that qualifies as built-in
// (workflow.QualifyBuiltin). It is available in every Workspace without a
// proposal or approval, and the owner disables or enables it for every
// Workspace. Its identity is its executable hash: before the first run
// that pins a hash, the runtime commits the embedded files to procedural
// Git at builtin/workflows/<name> and records the version
// (workflow_builtin_versions), so every run reads its pinned version from
// its commit exactly as a Workspace run does, and a run pinned to an
// earlier built-in version stays resumable after Evie ships a newer one.
//
// A Global custom definition is the owner's approved copy for every
// Workspace (global/workflows/<name>), proposed, reviewed and approved like
// a Workspace's but qualified like a built-in. Resolution by name for a
// Workspace: the Workspace's approved custom version, else the approved
// Global version, else the built-in while enabled. Every run still belongs
// to the Workspace it starts in, and passes its worker allowance and run
// admission at start and at every node.

var (
	// ErrBuiltinUnknown names a built-in this Evie does not have.
	ErrBuiltinUnknown = errors.New("no built-in workflow by that name")
)

// builtinCatalog is where a runtime reads its built-ins: the definitions
// embedded in Evie (tests substitute their own).
type builtinCatalog struct {
	list     func() []workflow.TemplateInfo
	source   func(name string) (workflow.Source, bool)
	fixtures func(name string) (workflow.Fixtures, error)
}

func embeddedBuiltins() builtinCatalog {
	return builtinCatalog{list: workflow.Builtins, source: workflow.BuiltinSource, fixtures: workflow.BuiltinFixtures}
}

// isBuiltin reports whether this Evie has a built-in of that name.
func (r *Runtime) isBuiltin(name string) bool {
	_, ok := r.builtinSet.source(name)
	return ok
}

// builtinEntry is one embedded built-in, validated once.
type builtinEntry struct {
	v        *workflow.Validated
	problem  error
	recorded *eviedb.WorkflowBuiltinVersion
}

// standaloneEnvironment reviews a built-in or Global definition: the
// compiled catalog and the operator's worker ceiling, outside any
// Workspace's access (they call no capability; QualifyBuiltin refuses one).
func (r *Runtime) standaloneEnvironment() workflow.Environment {
	return workflow.Environment{Capabilities: r.catalog, Workers: r.workers, AccessUnchecked: true}
}

// qualify validates a built-in or Global definition and requires it to
// qualify as built-in.
func (r *Runtime) qualify(src workflow.Source) (*workflow.Validated, error) {
	v, err := workflow.Validate(src, r.standaloneEnvironment())
	if err != nil {
		return nil, err
	}
	if problems := workflow.QualifyBuiltin(v.Report()); len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s", eviedb.ErrWorkflowNotStandalone, strings.Join(problems, "; "))
	}
	return v, nil
}

// builtin is this Evie's built-in of name, validated and qualified once.
func (r *Runtime) builtin(name string) (*workflow.Validated, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.builtins[name]; ok {
		return e.v, e.problem
	}
	src, ok := r.builtinSet.source(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrBuiltinUnknown, name)
	}
	v, err := r.qualify(src)
	if err != nil {
		err = fmt.Errorf("the built-in %s does not validate in this Evie: %w", name, err)
	}
	r.builtins[name] = &builtinEntry{v: v, problem: err}
	return v, err
}

// recordBuiltin records this Evie's built-in version of name, once: its
// exact files committed to procedural Git (a commit is made only when they
// changed there), then the version in SQLite (the same hash is the same
// version). A crash between the two leaves a commit nothing names.
func (r *Runtime) recordBuiltin(ctx context.Context, name string) (eviedb.WorkflowBuiltinVersion, error) {
	v, err := r.builtin(name)
	if err != nil {
		return eviedb.WorkflowBuiltinVersion{}, err
	}
	r.mu.Lock()
	if e := r.builtins[name]; e.recorded != nil {
		recorded := *e.recorded
		r.mu.Unlock()
		return recorded, nil
	}
	r.mu.Unlock()
	path := eviedb.BuiltinWorkflowDefinitionPath(name)
	authority := workflow.RequirementsOf(v.Report()).AuthorityHash
	message := fmt.Sprintf("Record built-in workflow %s\n\nExecutable-Hash: %s\nAuthority-Hash: %s\nBuilt-In: shipped with Evie; runs need no approval (it holds no Standing Authority)\n",
		name, v.Hash(), authority)
	commit, _, err := r.repo.Commit(ctx, path, v.Files(), message)
	if err != nil {
		return eviedb.WorkflowBuiltinVersion{}, err
	}
	committed, err := r.qualify(r.repo.Source(ctx, commit, path))
	if err != nil {
		return eviedb.WorkflowBuiltinVersion{}, fmt.Errorf("the committed built-in %s did not validate: %w", name, err)
	}
	if committed.Hash() != v.Hash() {
		return eviedb.WorkflowBuiltinVersion{}, fmt.Errorf("the committed built-in %s hashes to %s, not %s", name, committed.Hash(), v.Hash())
	}
	recorded, err := r.store.RecordWorkflowBuiltinVersion(ctx, eviedb.WorkflowBuiltinVersion{Name: name, Commit: commit, Path: path,
		ExecutableHash: committed.Hash(), AuthorityHash: authority, Report: committed.Report()})
	if err != nil {
		return recorded, err
	}
	r.mu.Lock()
	r.builtins[name].recorded = &recorded
	r.mu.Unlock()
	return recorded, nil
}

// resolve is the version a new run of name in the Workspace pins: its
// custom version, the Global custom version, or this Evie's built-in
// (recorded first). Without any, the Workspace's own reason is returned
// (never approved, deactivated, or not found).
func (r *Runtime) resolve(ctx context.Context, ws memory.WorkspaceID, name string) (eviedb.WorkflowActivation, error) {
	res, err := r.store.ResolveWorkflow(ctx, ws, name)
	if err != nil {
		return eviedb.WorkflowActivation{}, err
	}
	switch res.Source {
	case eviedb.WorkflowSourceWorkspace, eviedb.WorkflowSourceGlobal:
		return *res.Activation, nil
	}
	if !r.isBuiltin(name) {
		if _, err := r.store.ActiveWorkflow(ctx, ws, name); err != nil {
			return eviedb.WorkflowActivation{}, err
		}
		return eviedb.WorkflowActivation{}, fmt.Errorf("%w: %s", eviedb.ErrWorkflowNotFound, name)
	}
	if !res.BuiltinEnabled {
		return eviedb.WorkflowActivation{}, fmt.Errorf("%w (%s); David enables it with evie workflows builtin enable %s or in Settings", eviedb.ErrWorkflowBuiltinDisabled, name, name)
	}
	v, err := r.recordBuiltin(ctx, name)
	if err != nil {
		return eviedb.WorkflowActivation{}, err
	}
	return v.Activation(ws), nil
}

// BuiltinView is one built-in as the owner's surfaces show it: this Evie's
// version, whether it is enabled for every Workspace, the versions runs
// pinned so far and, for a Workspace, what a new run there would pin.
type BuiltinView struct {
	Name           string                          `json:"name"`
	Description    string                          `json:"description"`
	Inputs         []string                        `json:"inputs"`
	Workers        []string                        `json:"workers"`
	MaxTokens      int64                           `json:"max_tokens"`
	ExecutableHash string                          `json:"executable_hash,omitempty"`
	Version        int64                           `json:"version,omitempty"`
	Problem        string                          `json:"problem,omitempty"`
	Enabled        bool                            `json:"enabled"`
	Control        *eviedb.WorkflowBuiltinControl  `json:"control,omitempty"`
	Versions       []BuiltinVersionView            `json:"versions,omitempty"`
	Global         *eviedb.WorkflowDefinitionState `json:"global,omitempty"`
	Workspace      *BuiltinInWorkspace             `json:"workspace,omitempty"`
}

// BuiltinVersionView is one recorded built-in version, without its report.
type BuiltinVersionView struct {
	Version        int64  `json:"version"`
	ExecutableHash string `json:"executable_hash"`
	Commit         string `json:"commit"`
	RecordedAt     string `json:"recorded_at"`
	Current        bool   `json:"current,omitempty"`
}

// BuiltinInWorkspace is what a new run of the name in one Workspace would
// pin (source workspace, global or builtin), or why it cannot start.
type BuiltinInWorkspace struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	Source      string             `json:"source"`
	Version     int64              `json:"version,omitempty"`
	Blocked     []string           `json:"blocked,omitempty"`
}

// Builtins lists this Evie's built-ins with their state, and, given a
// Workspace, what a run of each would pin there.
func (r *Runtime) Builtins(ctx context.Context, ws memory.WorkspaceID) ([]BuiltinView, error) {
	out := []BuiltinView{}
	for _, info := range r.builtinSet.list() {
		view := BuiltinView{Name: info.Name, Description: info.Description, Inputs: info.Inputs, Workers: []string{}}
		v, err := r.builtin(info.Name)
		if err != nil {
			view.Problem = err.Error()
		} else {
			report := v.Report()
			view.ExecutableHash, view.MaxTokens, view.Workers = v.Hash(), report.Limits.MaxTokens, workflow.RequirementsOf(report).Workers
		}
		state, err := r.store.WorkflowBuiltinState(ctx, info.Name)
		if err != nil {
			return nil, err
		}
		view.Enabled, view.Control = state.Enabled, state.Control
		for _, recorded := range state.Versions {
			current := recorded.ExecutableHash == view.ExecutableHash
			if current {
				view.Version = recorded.Version
			}
			view.Versions = append(view.Versions, BuiltinVersionView{Version: recorded.Version, ExecutableHash: recorded.ExecutableHash, Commit: recorded.Commit,
				RecordedAt: recorded.RecordedAt.Format(timeLayout), Current: current})
		}
		if g, err := r.store.GlobalWorkflowDefinition(ctx, info.Name); err == nil {
			g.Versions, g.Proposals = nil, nil
			view.Global = &g
		} else if !errors.Is(err, eviedb.ErrWorkflowNotFound) {
			return nil, err
		}
		if ws != "" {
			res, err := r.store.ResolveWorkflow(ctx, ws, info.Name)
			in := &BuiltinInWorkspace{WorkspaceID: ws, Source: string(res.Source)}
			var blocked *eviedb.WorkflowBlockedError
			switch {
			case errors.As(err, &blocked):
				in.Blocked = blocked.Reasons
			case err != nil:
				return nil, err
			}
			if res.Activation != nil {
				in.Version = res.Activation.Version
			}
			if res.Source == eviedb.WorkflowSourceBuiltin {
				in.Version = view.Version
				switch {
				case !res.BuiltinEnabled:
					in.Blocked = append(in.Blocked, "David disabled this built-in for every Workspace")
				case view.Problem != "":
					in.Blocked = append(in.Blocked, view.Problem)
				default:
					reasons, err := r.store.WorkspaceWorkerBlocks(ctx, ws, view.Workers)
					if err != nil {
						return nil, err
					}
					in.Blocked = append(in.Blocked, reasons...)
				}
			}
			view.Workspace = in
		}
		out = append(out, view)
	}
	return out, nil
}

// BuiltinControl is the owner disabling or enabling a built-in for every
// Workspace.
type BuiltinControl struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
	Surface string `json:"-"`
}

// ControlBuiltin records the owner's control of a built-in this Evie has
// or one an earlier Evie recorded. Only the owner's CLI and web surfaces
// reach it; no model tool does.
func (r *Runtime) ControlBuiltin(ctx context.Context, c BuiltinControl) (eviedb.WorkflowBuiltinControl, error) {
	if !ownerSurface(c.Surface) {
		return eviedb.WorkflowBuiltinControl{}, eviedb.ErrWorkflowApprover
	}
	if !r.isBuiltin(c.Name) {
		state, err := r.store.WorkflowBuiltinState(ctx, c.Name)
		if err != nil {
			return eviedb.WorkflowBuiltinControl{}, err
		}
		if len(state.Versions) == 0 {
			return eviedb.WorkflowBuiltinControl{}, fmt.Errorf("%w: %s (evie workflows builtin list lists them)", ErrBuiltinUnknown, c.Name)
		}
	}
	return r.store.ControlWorkflowBuiltin(ctx, eviedb.WorkflowBuiltinControlRequest{Name: c.Name, Enabled: c.Enabled, Reason: c.Reason, Surface: c.Surface})
}

// Customization copies a built-in into the procedural repository as a
// proposal of a Workspace's custom definition or of a Global one, with its
// bundled dry run, for the owner's normal approval. An approved custom
// version of the same name then overrides the built-in in its scope.
type Customization struct {
	Name        string
	WorkspaceID memory.WorkspaceID
	Global      bool
	Fixtures    *workflow.Fixtures
	Surface     string
}

// Customize proposes a copy of the built-in and returns its approval card.
func (r *Runtime) Customize(ctx context.Context, c Customization) (Card, error) {
	if !r.isBuiltin(c.Name) {
		return Card{}, fmt.Errorf("%w: %s (evie workflows builtin list lists them)", ErrBuiltinUnknown, c.Name)
	}
	if c.Global == (c.WorkspaceID != "") {
		return Card{}, errors.New("customize a built-in either for one Workspace or as a Global definition")
	}
	return r.Propose(ctx, Proposal{WorkspaceID: c.WorkspaceID, Global: c.Global, Template: c.Name, Fixtures: c.Fixtures, Surface: c.Surface})
}

// proposeGlobal is Propose for a Global custom definition: validated
// outside any Workspace and qualified like a built-in, simulated with its
// fixtures, committed at global/workflows/<name>, validated again from the
// commit and recorded with its review and dry run.
func (r *Runtime) proposeGlobal(ctx context.Context, req Proposal, src workflow.Source, origin string) (Card, error) {
	local, err := r.qualify(src)
	if err != nil {
		return Card{}, err
	}
	if req.Fixtures != nil {
		if _, err := local.Simulate(*req.Fixtures); err != nil {
			return Card{}, err
		}
	}
	path := eviedb.GlobalWorkflowDefinitionPath(local.Name())
	authority := workflow.RequirementsOf(local.Report()).AuthorityHash
	message := fmt.Sprintf("Propose Global workflow %s\n\nExecutable-Hash: %s\nAuthority-Hash: %s\nProposed-By: owner (%s)\n", local.Name(), local.Hash(), authority, req.Surface)
	if origin != "" {
		message += "Template: " + origin + " (built into Evie)\n"
	}
	commit, _, err := r.repo.Commit(ctx, path, local.Files(), message)
	if err != nil {
		return Card{}, err
	}
	committed, err := r.qualify(r.repo.Source(ctx, commit, path))
	if err != nil {
		return Card{}, fmt.Errorf("the committed definition did not validate: %w", err)
	}
	if committed.Hash() != local.Hash() {
		return Card{}, fmt.Errorf("the committed definition hashes to %s, not %s", committed.Hash(), local.Hash())
	}
	p := eviedb.WorkflowProposal{Name: committed.Name(), Commit: commit, Path: path, ExecutableHash: committed.Hash(), AuthorityHash: authority,
		Report: committed.Report(), ProposedBy: "owner", Surface: req.Surface}
	if req.Fixtures != nil {
		if p.Simulation, err = committed.Simulate(*req.Fixtures); err != nil {
			return Card{}, err
		}
	}
	recorded, err := r.store.RecordGlobalWorkflowProposal(ctx, p)
	if err != nil {
		return Card{}, err
	}
	card, err := r.card(ctx, recorded)
	card.Template = origin
	return card, err
}

// proposal reads a Workspace or Global proposal by ID.
func (r *Runtime) proposal(ctx context.Context, id string) (eviedb.WorkflowProposal, error) {
	p, err := r.store.WorkflowProposal(ctx, id)
	if errors.Is(err, eviedb.ErrWorkflowProposalNotFound) {
		return r.store.GlobalWorkflowProposal(ctx, id)
	}
	return p, err
}

// ListGlobal lists the Global custom definitions.
func (r *Runtime) ListGlobal(ctx context.Context) ([]eviedb.WorkflowDefinitionState, error) {
	return r.store.ListGlobalWorkflowDefinitions(ctx)
}

// ShowGlobal is one Global definition's status and history.
func (r *Runtime) ShowGlobal(ctx context.Context, name string) (eviedb.WorkflowDefinitionState, error) {
	return r.store.GlobalWorkflowDefinition(ctx, name)
}

// pinnedReport is the review report of the version a run pinned, from its
// source's record.
func (r *Runtime) pinnedReport(ctx context.Context, run eviedb.WorkflowRun) (workflow.Report, error) {
	switch run.Source {
	case eviedb.WorkflowSourceBuiltin:
		v, err := r.store.WorkflowBuiltinVersion(ctx, run.Workflow, run.Version)
		return v.Report, err
	case eviedb.WorkflowSourceGlobal:
		state, err := r.store.GlobalWorkflowDefinition(ctx, run.Workflow)
		if err != nil {
			return workflow.Report{}, err
		}
		i := slices.IndexFunc(state.Versions, func(a eviedb.WorkflowActivation) bool { return a.Version == run.Version })
		if i < 0 {
			return workflow.Report{}, fmt.Errorf("Global version %d of %s is not recorded", run.Version, run.Workflow)
		}
		p, err := r.store.GlobalWorkflowProposal(ctx, state.Versions[i].ProposalID)
		return p.Report, err
	}
	state, err := r.store.WorkflowDefinition(ctx, run.WorkspaceID, run.Workflow)
	if err != nil {
		return workflow.Report{}, err
	}
	i := slices.IndexFunc(state.Versions, func(a eviedb.WorkflowActivation) bool { return a.Version == run.Version })
	if i < 0 {
		return workflow.Report{}, fmt.Errorf("version %d of %s is not recorded", run.Version, run.Workflow)
	}
	p, err := r.store.WorkflowProposal(ctx, state.Versions[i].ProposalID)
	return p.Report, err
}
