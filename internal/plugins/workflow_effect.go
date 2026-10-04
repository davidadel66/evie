package plugins

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/workflow"
)

// Workflow capability effects (Stage 14 sub-stage 6 of
// docs/memory-architecture.md, 2026-10-04; ADR 0009, 0015, 0017, 0043).
// A plugin contributes effects the Workflow Runtime may perform through
// this focused provider role, separate from reads and tools: each effect
// has a canonical plugin-namespaced Capability ID declared in the manifest,
// a contract version, closed object input and output, the exact Connection
// IDs and resource selectors it serves, and four declarations the runtime
// relies on:
//
//   - Idempotent: the provider applies one idempotency key at most once,
//     so the same Effect Intent may be sent again (a retry within the
//     step's Retry Policy, or reconciliation by replay). The runtime
//     derives the key from the run, the node and its deterministic input
//     and passes it with every request.
//   - ConflictKeys: the Resource Conflict Keys a request touches. Without
//     a function the runtime uses one key per connection and resource, so
//     every effect on one resource is serialized; a provider that can
//     protect narrower parts names its rule (ConflictRule, such as
//     "account") and derives the keys from the request.
//   - Perform: sends the request. An error wrapping
//     ErrWorkflowEffectNotPerformed (see EffectNotPerformed) proves nothing
//     happened; every other error, a timeout, a panic or a response outside
//     the contract leaves the outcome unknown.
//   - Reconcile (optional): answers, for an Effect Intent whose outcome is
//     unknown, whether the effect with its idempotency key happened and its
//     result, by lookup or by idempotent replay. It must never perform the
//     effect a second time: a replay relies on a key the provider applies
//     at most once. The runtime admits each reconciliation like the effect
//     itself. Without it only the owner reconciles.
//
// The Manager validates contributions once, lists the compiled contracts
// whatever a plugin's state, and resolves the behavior only while the
// plugin is ready, exactly as for reads. Each declaration is part of the
// effect's schema hash, so a run pinned to an effect notices a change.

// WorkflowEffectProvider is the focused role through which a plugin
// contributes workflow capability effects.
type WorkflowEffectProvider interface {
	WorkflowEffects() []WorkflowEffectCapability
}

// WorkflowEffectCapability is one capability effect a plugin contributes.
type WorkflowEffectCapability struct {
	ID              CapabilityID
	ContractVersion string
	// Input and Output are the contract's closed object types.
	Input  *workflow.Type
	Output *workflow.Type
	// Connections and Resources are the exact Connection IDs and resource
	// selectors the effect serves.
	Connections []string
	Resources   []string
	// Idempotent declares that the provider applies one idempotency key at
	// most once.
	Idempotent bool
	// ConflictRule names what one Resource Conflict Key ConflictKeys
	// derives covers, such as "account"; both are empty for the default,
	// one key per connection and resource.
	ConflictRule string
	ConflictKeys WorkflowConflictKeys
	// Perform sends one request. It must honor ctx; the runtime does not
	// wait past its effect timeout for an answer.
	Perform WorkflowEffect
	// Reconcile answers whether an effect with the request's idempotency
	// key happened; nil leaves an unknown outcome to the owner.
	Reconcile WorkflowEffectReconcile
}

// WorkflowEffectRequest is one effect request: the step's connection and
// resource, already admitted by the runtime, its input, already checked
// against the contract, and the Effect Intent's idempotency key.
type WorkflowEffectRequest struct {
	Connection     string
	Resource       string
	Input          map[string]any
	IdempotencyKey string
}

// WorkflowEffect performs one effect request and returns its response as
// JSON-encodable values; the runtime normalizes and validates it against
// the contract before recording it with the Effect Receipt.
type WorkflowEffect func(ctx context.Context, req WorkflowEffectRequest) (any, error)

// WorkflowEffectReconcile answers what happened to an Effect Intent whose
// outcome is unknown. A returned error means it could not answer now; the
// runtime asks again later.
type WorkflowEffectReconcile func(ctx context.Context, req WorkflowEffectRequest) (WorkflowEffectLookup, error)

// WorkflowConflictKeys derives the Resource Conflict Keys one request
// touches, under the effect's ConflictRule. It must be deterministic.
type WorkflowConflictKeys func(req WorkflowEffectRequest) ([]string, error)

// WorkflowEffectOutcome is what reconciliation found.
type WorkflowEffectOutcome string

const (
	// WorkflowEffectHappened: the effect happened; Output is its response.
	WorkflowEffectHappened WorkflowEffectOutcome = "happened"
	// WorkflowEffectDidNotHappen: the provider proves it did not happen.
	WorkflowEffectDidNotHappen WorkflowEffectOutcome = "did_not_happen"
	// WorkflowEffectUnknown: the provider cannot prove either; the owner
	// decides.
	WorkflowEffectUnknown WorkflowEffectOutcome = "unknown"
)

// WorkflowEffectLookup is reconciliation's answer, with the evidence the
// receipt records.
type WorkflowEffectLookup struct {
	Outcome  WorkflowEffectOutcome
	Output   any
	Evidence string
}

// ErrWorkflowEffectNotPerformed marks an effect error that proves nothing
// happened: the request was refused or never accepted.
var ErrWorkflowEffectNotPerformed = errors.New("the effect was not performed")

// EffectNotPerformed wraps err as the provider's proof that nothing
// happened.
func EffectNotPerformed(err error) error {
	if err == nil {
		return ErrWorkflowEffectNotPerformed
	}
	return fmt.Errorf("%w: %w", ErrWorkflowEffectNotPerformed, err)
}

func (e WorkflowEffectCapability) contract() workflow.CapabilityContract {
	rule := e.ConflictRule
	if rule == "" {
		rule = workflow.DefaultConflictKeys
	}
	return workflow.CapabilityContract{ID: string(e.ID), Contract: e.ContractVersion, Effect: true, Idempotent: e.Idempotent,
		Reconciles: e.Reconcile != nil, ConflictKeys: rule, Input: e.Input, Output: e.Output,
		Connections: slices.Clone(e.Connections), Resources: slices.Clone(e.Resources)}
}

// snapshotWorkflowEffects validates a plugin's workflow effect
// contributions against its manifest. taken are the IDs it contributes
// through another role: one Capability is contributed by exactly one role.
func snapshotWorkflowEffects(manifest Manifest, supplied []WorkflowEffectCapability, taken map[CapabilityID]bool) ([]WorkflowEffectCapability, error) {
	contracts := make(map[CapabilityID]string, len(manifest.Capabilities))
	for _, contract := range manifest.Capabilities {
		contracts[contract.ID] = contract.Version
	}
	out := make([]WorkflowEffectCapability, 0, len(supplied))
	seen := make(map[CapabilityID]bool, len(supplied))
	for _, e := range supplied {
		version, declared := contracts[e.ID]
		switch {
		case !declared:
			return nil, fmt.Errorf("plugin %q contributes undeclared Capability %q", manifest.ID, e.ID)
		case seen[e.ID]:
			return nil, fmt.Errorf("plugin %q contributes Capability %q more than once", manifest.ID, e.ID)
		case taken[e.ID]:
			return nil, fmt.Errorf("plugin %q contributes Capability %q both as a workflow effect and through another role", manifest.ID, e.ID)
		case e.ContractVersion != version:
			return nil, fmt.Errorf("plugin %q contributes Capability %q at contract version %q, manifest declares %q", manifest.ID, e.ID, e.ContractVersion, version)
		case e.Input == nil || e.Input.Kind != workflow.KindObject || e.Output == nil || e.Output.Kind != workflow.KindObject:
			return nil, fmt.Errorf("workflow effect %q needs an object input and output type", e.ID)
		case len(e.Connections) == 0 || len(e.Resources) == 0:
			return nil, fmt.Errorf("workflow effect %q needs at least one connection and one resource it serves", e.ID)
		case e.Perform == nil:
			return nil, fmt.Errorf("workflow effect %q has no perform function", e.ID)
		case (e.ConflictKeys == nil) != (strings.TrimSpace(e.ConflictRule) == ""):
			return nil, fmt.Errorf("workflow effect %q must name its conflict key rule exactly when it derives its own Resource Conflict Keys", e.ID)
		case e.ConflictRule == workflow.DefaultConflictKeys:
			return nil, fmt.Errorf("workflow effect %q: conflict rule %q is the default; derive no keys to use it", e.ID, e.ConflictRule)
		case len(e.ConflictRule) > 64 || strings.ContainsFunc(e.ConflictRule, func(r rune) bool { return r < ' ' }):
			return nil, fmt.Errorf("workflow effect %q: its conflict rule must be a short name", e.ID)
		}
		served := workflow.WorkspaceAccess{}
		for _, connection := range e.Connections {
			served.Connections = append(served.Connections, workflow.ConnectionAccess{ID: connection, Resources: e.Resources})
		}
		if _, err := workflow.NormalizeAccess(served); err != nil {
			return nil, fmt.Errorf("workflow effect %q: %w", e.ID, err)
		}
		seen[e.ID] = true
		e.Connections, e.Resources = slices.Clone(e.Connections), slices.Clone(e.Resources)
		out = append(out, e)
	}
	return out, nil
}
