package plugins

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/davidadel66/evie/internal/workflow"
)

// Workflow capability providers (Stage 14 sub-stage 5 of
// docs/memory-architecture.md, 2026-10-04; ADR 0015, 0016, 0035, 0037,
// 0039). A plugin contributes capability reads the Workflow Runtime may
// call through this focused provider role, separate from the tool role:
// each read has a canonical plugin-namespaced Capability ID declared in the
// manifest, a contract version, typed object input and output (whose schema
// hash a run pins), and the exact Connection IDs and resource selectors it
// serves. The Manager validates contributions once, lists the compiled
// contracts for definition validation whatever a plugin's state, and
// resolves a callable provider only while its plugin is ready, so disabling,
// stopping or failing a plugin makes its reads unavailable (a run waits for
// them) without changing any pinned record. This role carries reads only:
// effects, with idempotency, reconciliation and Resource Conflict Keys,
// come through the separate WorkflowEffectProvider role (sub-stage 6,
// workflow_effect.go).

// WorkflowCapabilityProvider is the focused role through which a plugin
// contributes workflow capability reads.
type WorkflowCapabilityProvider interface {
	WorkflowCapabilities() []WorkflowCapability
}

// WorkflowCapability is one capability read a plugin contributes.
type WorkflowCapability struct {
	ID              CapabilityID
	ContractVersion string
	// Input and Output are the contract's closed object types.
	Input  *workflow.Type
	Output *workflow.Type
	// Connections and Resources are the exact Connection IDs and resource
	// selectors the read serves.
	Connections []string
	Resources   []string
	// Read performs one read. It must honor ctx: the runtime cancels it
	// when the run's lease is lost, the runtime stops, or its time is up.
	Read WorkflowRead
}

// WorkflowRead performs one capability read and returns its output as
// JSON-encodable values; the runtime normalizes and validates it against
// the contract before recording it.
type WorkflowRead func(ctx context.Context, req WorkflowReadRequest) (any, error)

// WorkflowReadRequest is one read: the step's connection and resource,
// already admitted by the runtime, and its input, already checked against
// the contract.
type WorkflowReadRequest struct {
	Connection string
	Resource   string
	Input      map[string]any
}

var (
	// ErrWorkflowCapabilityUnknown is a Capability ID no compiled plugin
	// contributes as a workflow capability.
	ErrWorkflowCapabilityUnknown = errors.New("no compiled plugin provides this workflow capability")
	// ErrWorkflowCapabilityUnavailable is a compiled workflow capability
	// whose plugin or connection is not ready now.
	ErrWorkflowCapabilityUnavailable = errors.New("the workflow capability's provider is not available")
)

// WorkflowProvider is a workflow capability as the Manager resolves it now:
// its contract and schema hash, the plugin and implementation version that
// provide it, the evidence it declares for continuing earlier
// implementations' pinned work, and its behavior: a read's Read, or an
// effect's Perform, Reconcile and ConflictKeys (each nil unless ready, and
// Reconcile and ConflictKeys nil when the effect declares none).
type WorkflowProvider struct {
	Contract     workflow.CapabilityContract
	SchemaSHA256 string
	Plugin       PluginID
	Version      string
	// ResumableFrom maps an earlier implementation version of the plugin
	// to the exact contract and schema this implementation declares it
	// continues for this capability (ADR 0037, 0041).
	ResumableFrom map[string]CapabilityCompatibility
	Read          WorkflowRead
	Perform       WorkflowEffect
	Reconcile     WorkflowEffectReconcile
	ConflictKeys  WorkflowConflictKeys
}

func (c WorkflowCapability) contract() workflow.CapabilityContract {
	return workflow.CapabilityContract{ID: string(c.ID), Contract: c.ContractVersion, Input: c.Input, Output: c.Output,
		Connections: slices.Clone(c.Connections), Resources: slices.Clone(c.Resources)}
}

// snapshotWorkflowCapabilities validates a plugin's workflow capability
// contributions against its manifest. tools are the IDs it contributes as
// tools: one Capability is contributed by exactly one role.
func snapshotWorkflowCapabilities(manifest Manifest, supplied []WorkflowCapability, tools map[CapabilityID]bool) ([]WorkflowCapability, error) {
	contracts := make(map[CapabilityID]string, len(manifest.Capabilities))
	for _, contract := range manifest.Capabilities {
		contracts[contract.ID] = contract.Version
	}
	out := make([]WorkflowCapability, 0, len(supplied))
	seen := make(map[CapabilityID]bool, len(supplied))
	for _, c := range supplied {
		version, declared := contracts[c.ID]
		switch {
		case !declared:
			return nil, fmt.Errorf("plugin %q contributes undeclared Capability %q", manifest.ID, c.ID)
		case seen[c.ID]:
			return nil, fmt.Errorf("plugin %q contributes Capability %q more than once", manifest.ID, c.ID)
		case tools[c.ID]:
			return nil, fmt.Errorf("plugin %q contributes Capability %q both as a tool and as a workflow capability", manifest.ID, c.ID)
		case c.ContractVersion != version:
			return nil, fmt.Errorf("plugin %q contributes Capability %q at contract version %q, manifest declares %q", manifest.ID, c.ID, c.ContractVersion, version)
		case c.Input == nil || c.Input.Kind != workflow.KindObject || c.Output == nil || c.Output.Kind != workflow.KindObject:
			return nil, fmt.Errorf("workflow capability %q needs an object input and output type", c.ID)
		case len(c.Connections) == 0 || len(c.Resources) == 0:
			return nil, fmt.Errorf("workflow capability %q needs at least one connection and one resource it serves", c.ID)
		case c.Read == nil:
			return nil, fmt.Errorf("workflow capability %q has no read", c.ID)
		}
		served := workflow.WorkspaceAccess{}
		for _, connection := range c.Connections {
			served.Connections = append(served.Connections, workflow.ConnectionAccess{ID: connection, Resources: c.Resources})
		}
		if _, err := workflow.NormalizeAccess(served); err != nil {
			return nil, fmt.Errorf("workflow capability %q: %w", c.ID, err)
		}
		seen[c.ID] = true
		c.Connections, c.Resources = slices.Clone(c.Connections), slices.Clone(c.Resources)
		out = append(out, c)
	}
	return out, nil
}

// compiledWorkflow is one compiled workflow capability: a read or an
// effect.
type compiledWorkflow struct {
	id       CapabilityID
	contract workflow.CapabilityContract
	read     WorkflowRead
	effect   *WorkflowEffectCapability
}

// workflowCapabilityLocked finds a compiled workflow capability, read or
// effect, by ID.
func (m *Manager) workflowCapabilityLocked(id CapabilityID) (*compiledPlugin, compiledWorkflow, bool) {
	entry, exists := m.plugins[providerIDForCapability(id)]
	if !exists {
		return nil, compiledWorkflow{}, false
	}
	for _, c := range entry.workflowCapabilities {
		if c.ID == id {
			return entry, compiledWorkflow{id: c.ID, contract: c.contract(), read: c.Read}, true
		}
	}
	for i := range entry.workflowEffects {
		if e := &entry.workflowEffects[i]; e.ID == id {
			return entry, compiledWorkflow{id: e.ID, contract: e.contract(), effect: e}, true
		}
	}
	return nil, compiledWorkflow{}, false
}

// WorkflowContract is the compiled contract of one workflow capability,
// whatever its plugin's state: what definitions validate against.
func (m *Manager) WorkflowContract(id string) (workflow.CapabilityContract, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, c, ok := m.workflowCapabilityLocked(CapabilityID(id))
	if !ok {
		return workflow.CapabilityContract{}, false
	}
	return c.contract, true
}

// ResolveWorkflowCapability resolves one workflow capability's provider
// now, after applying the owner's latest enabled choices. A capability no
// compiled plugin contributes is ErrWorkflowCapabilityUnknown. One whose
// plugin is not ready, whose optional dependency is not loaded or whose
// connection is not ready is ErrWorkflowCapabilityUnavailable, returned
// with the provider it would be (without a read), so a pinned run can
// still be compared with it.
func (m *Manager) ResolveWorkflowCapability(ctx context.Context, id string) (WorkflowProvider, error) {
	if err := m.RefreshEnabledState(ctx); err != nil {
		return WorkflowProvider{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, c, ok := m.workflowCapabilityLocked(CapabilityID(id))
	if !ok {
		return WorkflowProvider{}, fmt.Errorf("%w: %s", ErrWorkflowCapabilityUnknown, id)
	}
	contract := c.contract
	p := WorkflowProvider{Contract: contract, SchemaSHA256: contract.SchemaSHA256(), Plugin: entry.manifest.ID,
		Version: entry.manifest.ImplementationVersion, ResumableFrom: map[string]CapabilityCompatibility{}}
	for _, declaration := range entry.manifest.ResumableFrom {
		for _, evidence := range declaration.Capabilities {
			if evidence.ID == c.id {
				p.ResumableFrom[declaration.ImplementationVersion] = evidence
			}
		}
	}
	unavailable := func(format string, args ...any) (WorkflowProvider, error) {
		return p, fmt.Errorf("%w: %s: %s", ErrWorkflowCapabilityUnavailable, id, fmt.Sprintf(format, args...))
	}
	if entry.state != StateReady {
		detail := string(entry.state)
		if entry.state == StateDisabled {
			detail += fmt.Sprintf("; enable plugin %q", entry.manifest.ID)
		}
		if entry.diagnostic != "" {
			detail += ": " + entry.diagnostic
		}
		return unavailable("plugin %q is %s", entry.manifest.ID, detail)
	}
	for _, dependency := range capabilityContract(entry.manifest, c.id).OptionalDependencies {
		if !entry.loadedOptional[dependency] {
			return unavailable("its optional dependency %q is not ready", dependency)
		}
	}
	if entry.connectionReadiness.State == ConnectionNotReady {
		return unavailable("plugin %q's connection is not ready; set it up and the capability becomes available", entry.manifest.ID)
	}
	p.Read = c.read
	if e := c.effect; e != nil {
		p.Perform, p.Reconcile, p.ConflictKeys = e.Perform, e.Reconcile, e.ConflictKeys
	}
	return p, nil
}
