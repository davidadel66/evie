package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/workflow"
)

// Capability reads (Stage 14 sub-stage 5, 2026-10-04; ADR 0015, 0016, 0017,
// 0025, 0029, 0033, 0037, 0041).
//
// A run resolves the providers of its capability steps when it starts and
// pins each one in its Execution Composition: the canonical Capability ID,
// its contract version and schema hash, the plugin and implementation
// version that provide it, and the Connection IDs its steps use. A start is
// refused while a provider is unavailable. Every claim checks the pins
// again: a provider that is missing, changed its contract or schema, or was
// replaced by another implementation pauses the run visibly, never
// substituting behavior. A replacement that declares it continues the
// pinned implementation with the exact same contract and schema may
// continue the run once the owner approves an audited substitution (ADR
// 0041); the pin itself never changes. Before each read the runtime checks
// its provider is ready (a disabled plugin or unready connection pauses the
// run until it is, ADR 0029), applies per-node admission (the run's own
// version, its Standing Authority and the current Workspace Access, ADR
// 0033), records the attempt under the fenced lease, checks the lease once
// more, and calls the provider with a context that ends when the lease is
// lost, the runtime stops or the read's time is up. The provider's output
// is normalized and validated against the contract before it is recorded
// with the next checkpoint; a failed, timed-out or malformed read is a
// failed attempt retried within the step's Retry Policy after its backoff,
// a durable not-before time.

// CapabilityProviders is the workflow capability seam the runtime consumes
// (ADR 0015): the compiled contracts definitions validate against, and the
// provider of one capability as it is now. The Plugin Manager implements
// it.
type CapabilityProviders interface {
	WorkflowContract(id string) (workflow.CapabilityContract, bool)
	ResolveWorkflowCapability(ctx context.Context, id string) (plugins.WorkflowProvider, error)
}

// ContractsOnly is a catalog of contracts without providers: definitions
// validate against it, and no run can call any of them.
func ContractsOnly(catalog workflow.Catalog) CapabilityProviders { return contractsOnly{catalog} }

type contractsOnly struct{ catalog workflow.Catalog }

func (c contractsOnly) WorkflowContract(id string) (workflow.CapabilityContract, bool) {
	return c.catalog.Capability(id)
}

func (c contractsOnly) ResolveWorkflowCapability(_ context.Context, id string) (plugins.WorkflowProvider, error) {
	return plugins.WorkflowProvider{}, fmt.Errorf("%w: %s", plugins.ErrWorkflowCapabilityUnknown, id)
}

// providerCatalog is the providers' contracts as a workflow.Catalog.
type providerCatalog struct{ providers CapabilityProviders }

func (c providerCatalog) Capability(id string) (workflow.CapabilityContract, bool) {
	return c.providers.WorkflowContract(id)
}

var (
	// ErrCapabilityUnavailable refuses a start while a capability the
	// definition reads has no ready provider; nothing is recorded.
	ErrCapabilityUnavailable = errors.New("a workflow capability is unavailable")
	// ErrSubstitutionRefused refuses a substitution the replacement
	// provider's evidence does not permit.
	ErrSubstitutionRefused = errors.New("the provider substitution is not permitted")
)

// CapabilityUnavailableError names the capability a start needs and why it
// is unavailable.
type CapabilityUnavailableError struct {
	Workflow   string
	Capability string
	Reason     string
}

func (e *CapabilityUnavailableError) Error() string {
	return fmt.Sprintf("workflow %s calls %s, which is unavailable: %s; no run was started", e.Workflow, e.Capability, e.Reason)
}

func (e *CapabilityUnavailableError) Unwrap() error { return ErrCapabilityUnavailable }

// pinnedProvider is one capability's provider as a run pinned it. An
// effect's entry (sub-stage 6) also records its idempotency,
// reconciliation and Resource Conflict Key rule, which its schema hash
// covers, so a changed declaration pauses the run like a changed schema.
type pinnedProvider struct {
	Capability   string   `json:"capability"`
	Contract     string   `json:"contract"`
	SchemaSHA256 string   `json:"schema_sha256"`
	Plugin       string   `json:"plugin"`
	Version      string   `json:"version"`
	Connections  []string `json:"connections"`
	Effect       bool     `json:"effect,omitempty"`
	Idempotent   bool     `json:"idempotent,omitempty"`
	Reconciles   bool     `json:"reconciles,omitempty"`
	ConflictKeys string   `json:"conflict_keys,omitempty"`
}

func (c composition) pin(capability string) (pinnedProvider, bool) {
	for _, p := range c.Providers {
		if p.Capability == capability {
			return p, true
		}
	}
	return pinnedProvider{}, false
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// pinProviders resolves the provider of every capability the definition
// reads, for a new run's Execution Composition. Each must be ready now.
func (r *Runtime) pinProviders(ctx context.Context, v *workflow.Validated) ([]pinnedProvider, error) {
	pins := map[string]*pinnedProvider{}
	for _, s := range v.CapabilitySteps() {
		pin := pins[s.Capability]
		if pin == nil {
			p, err := r.providers.ResolveWorkflowCapability(ctx, s.Capability)
			switch {
			case errors.Is(err, plugins.ErrWorkflowCapabilityUnknown), errors.Is(err, plugins.ErrWorkflowCapabilityUnavailable):
				return nil, &CapabilityUnavailableError{Workflow: v.Name(), Capability: s.Capability, Reason: err.Error()}
			case err != nil:
				return nil, err
			case p.Contract.Contract != s.Contract:
				return nil, &CapabilityUnavailableError{Workflow: v.Name(), Capability: s.Capability,
					Reason: fmt.Sprintf("%s %s offers contract %s, not %s", p.Plugin, p.Version, p.Contract.Contract, s.Contract)}
			}
			pin = &pinnedProvider{Capability: s.Capability, Contract: p.Contract.Contract, SchemaSHA256: p.SchemaSHA256, Plugin: string(p.Plugin), Version: p.Version}
			if p.Contract.Effect {
				pin.Effect, pin.Idempotent, pin.Reconciles, pin.ConflictKeys = true, p.Contract.Idempotent, p.Contract.Reconciles, p.Contract.ConflictKeys
			}
			pins[s.Capability] = pin
		}
		if !slices.Contains(pin.Connections, s.Connection) {
			pin.Connections = append(pin.Connections, s.Connection)
		}
	}
	out := make([]pinnedProvider, 0, len(pins))
	for _, pin := range pins {
		sort.Strings(pin.Connections)
		out = append(out, *pin)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Capability < out[j].Capability })
	return out, nil
}

// checkPinned says why the run's pinned provider of one capability cannot
// serve it now, or "" when it can, with the provider as resolved. With
// ready, the provider must also be available now (before a read); without
// it, only compatibility is checked (at every claim). Nothing is ever
// substituted without a recorded substitution.
func (r *Runtime) checkPinned(ctx context.Context, runID string, pin pinnedProvider, ready bool) (plugins.WorkflowProvider, string, error) {
	p, err := r.providers.ResolveWorkflowCapability(ctx, pin.Capability)
	unavailable := errors.Is(err, plugins.ErrWorkflowCapabilityUnavailable)
	switch {
	case errors.Is(err, plugins.ErrWorkflowCapabilityUnknown):
		return p, fmt.Sprintf("capability %s is missing: no plugin in this Evie provides it (the run pinned %s %s); continue the run with an Evie that does, or migrate it to a newer approved version (evie workflows migrate <run-id> --to-version <n>)",
			pin.Capability, pin.Plugin, pin.Version), nil
	case err != nil && !unavailable:
		return p, "", err
	}
	if p.Contract.Contract != pin.Contract || p.SchemaSHA256 != pin.SchemaSHA256 {
		return p, fmt.Sprintf("capability %s changed incompatibly: the run pinned contract %s (schema %s) from %s %s, and %s %s offers contract %s (schema %s); a changed contract needs a new approved definition or a migrated run",
			pin.Capability, pin.Contract, shortHash(pin.SchemaSHA256), pin.Plugin, pin.Version, p.Plugin, p.Version, p.Contract.Contract, shortHash(p.SchemaSHA256)), nil
	}
	for _, connection := range pin.Connections {
		if len(p.Contract.Connections) > 0 && !slices.Contains(p.Contract.Connections, connection) {
			return p, fmt.Sprintf("capability %s changed incompatibly: %s %s no longer serves connection %s", pin.Capability, p.Plugin, p.Version, connection), nil
		}
	}
	if p.Version != pin.Version {
		subs, err := r.store.WorkflowRunSubstitutions(ctx, runID)
		if err != nil {
			return p, "", err
		}
		approved := slices.ContainsFunc(subs, func(s eviedb.WorkflowRunSubstitution) bool {
			return s.Capability == pin.Capability && s.PinnedVersion == pin.Version && s.ReplacementVersion == p.Version &&
				s.Contract == pin.Contract && s.SchemaSHA256 == pin.SchemaSHA256
		})
		if !approved {
			if declaresContinuation(p, pin) {
				return p, fmt.Sprintf("capability %s: the run pinned %s %s, and this Evie has %s %s, which declares it continues %s with the same contract %s and schema; approve the substitution with: evie workflows substitute %s %s %s",
					pin.Capability, pin.Plugin, pin.Version, p.Plugin, p.Version, pin.Version, pin.Contract, runID, pin.Capability, p.Version), nil
			}
			return p, fmt.Sprintf("capability %s: the run pinned %s %s, and %s %s does not declare that it continues %s with contract %s and schema %s; continue the run with an Evie that has %s %s, or migrate it to a newer approved version (evie workflows migrate <run-id> --to-version <n>)",
				pin.Capability, pin.Plugin, pin.Version, p.Plugin, p.Version, pin.Version, pin.Contract, shortHash(pin.SchemaSHA256), pin.Plugin, pin.Version), nil
		}
	}
	if ready && unavailable {
		return p, fmt.Sprintf("%v; the run continues by itself once it is available", err), nil
	}
	return p, "", nil
}

// declaresContinuation reports whether a replacement provider declares, in
// its plugin's manifest, that it continues the pinned implementation's
// capability with exactly the pinned contract and schema (ADR 0037).
func declaresContinuation(p plugins.WorkflowProvider, pin pinnedProvider) bool {
	evidence, ok := p.ResumableFrom[pin.Version]
	return ok && string(evidence.ID) == pin.Capability && evidence.ContractVersion == pin.Contract && evidence.SchemaSHA256 == pin.SchemaSHA256
}

// providersCompatible says why the run's pinned providers cannot continue
// it, or "".
func (r *Runtime) providersCompatible(ctx context.Context, runID string, c composition) (string, error) {
	for _, pin := range c.Providers {
		if _, reason, err := r.checkPinned(ctx, runID, pin, false); err != nil || reason != "" {
			return reason, err
		}
	}
	return "", nil
}

// maxReadOutputBytes bounds one read's normalized output.
const maxReadOutputBytes = 1 << 20

// callRead calls one provider read with a context that ends when parent
// does (the runtime stops, or the owner cancels the run), the run's lease
// is lost or the read timeout passes. A provider that ignores its context
// is not waited for; a panic is a failure.
func (r *Runtime) callRead(pc *pass, parent context.Context, read plugins.WorkflowRead, req plugins.WorkflowReadRequest) (any, error) {
	ctx, cancel := context.WithTimeout(parent, r.sched.ReadTimeout)
	defer cancel()
	stopLost := context.AfterFunc(pc.lost, cancel)
	defer stopLost()
	type result struct {
		out any
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- result{err: fmt.Errorf("the provider panicked: %v", p)}
			}
		}()
		out, err := read(ctx, req)
		done <- result{out, err}
	}()
	select {
	case res := <-done:
		if res.err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return res.out, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// normalizeOutput turns a provider's output into canonical values.
func normalizeOutput(out any) (any, []byte, error) {
	data, err := json.Marshal(out)
	if err != nil {
		return nil, nil, err
	}
	if len(data) > maxReadOutputBytes {
		return nil, nil, fmt.Errorf("the output is larger than %d KiB", maxReadOutputBytes>>10)
	}
	value, err := workflow.DecodeValue(data)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := workflow.EncodeValue(value)
	return value, encoded, err
}

// nodeRefused reports the admission refusals that pause a run before a node.
func nodeRefused(err error) bool {
	return errors.Is(err, eviedb.ErrWorkflowNodeRefused) || admissionRefused(err)
}

// read makes one attempt of a capability read step, from the pinned
// provider's readiness and per-node admission to the recorded outcome.
func (r *Runtime) read(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, inputs, state map[string]any, fail, pause func(string) error) error {
	if reason := r.limitReached(pc.v, p, step); reason != "" {
		return fail(reason)
	}
	pin, ok := pc.comp.pin(step.Capability)
	if !ok {
		return fail(fmt.Sprintf("steps.%s: capability %s is not pinned in the run's Execution Composition", step.ID, step.Capability))
	}
	provider, reason, err := r.checkPinned(pc.writes, p.Run.ID, pin, true)
	if err != nil {
		return err
	}
	if reason != "" {
		return pause(fmt.Sprintf("steps.%s: %s", step.ID, reason))
	}
	if err := r.store.AdmitWorkflowNode(pc.writes, pc.lease, eviedb.WorkflowNodeAdmission{
		Step: step.ID, Capability: step.Capability, Connection: step.Connection, Resource: step.Resource,
	}); err != nil {
		if nodeRefused(err) {
			return pause(err.Error())
		}
		return err
	}
	input, err := pc.v.CapabilityInput(step.ID, inputs, state)
	if err != nil {
		return fail(err.Error())
	}
	encoded, err := workflow.EncodeValue(input)
	if err != nil {
		return err
	}
	a, err := r.store.BeginWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptStart{
		RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Made + 1, Input: encoded,
	})
	if err != nil {
		return err
	}
	// The lease is checked immediately before the provider is called: a
	// holder that lost it calls nothing, and the next holder records the
	// attempt interrupted.
	if pc.lost.Err() != nil {
		return fmt.Errorf("%w: lost before steps.%s was read", eviedb.ErrWorkflowRunLeaseLost, step.ID)
	}
	if err := r.store.CheckWorkflowRunLease(pc.writes, pc.lease); err != nil {
		return err
	}
	// The owner's cancellation stops the read through its context
	// (sub-stage 8).
	calls, stopWatch := r.cancellable(pc, pc.stop)
	out, callErr := r.callRead(pc, calls, provider.Read, plugins.WorkflowReadRequest{Connection: step.Connection, Resource: step.Resource, Input: input})
	stopWatch()
	switch {
	case pc.lost.Err() != nil:
		return fmt.Errorf("%w: lost while steps.%s was read", eviedb.ErrWorkflowRunLeaseLost, step.ID)
	case callErr != nil && pc.stop.Err() != nil:
		return r.store.InterruptWorkflowAttempt(pc.writes, pc.lease, a.Seq,
			"interrupted: the runtime stopped during the read; it is read again from the same checkpoint")
	case callErr != nil && pc.cancelled():
		return r.store.InterruptWorkflowAttempt(pc.writes, pc.lease, a.Seq,
			"interrupted: the owner cancelled the run during the read, which was stopped")
	case errors.Is(callErr, context.DeadlineExceeded):
		callErr = fmt.Errorf("the read did not finish within %s", r.sched.ReadTimeout)
	}
	var value any
	var output []byte
	if callErr == nil {
		var normErr error
		if value, output, normErr = normalizeOutput(out); normErr == nil {
			normErr = pc.v.CheckCapabilityOutput(step.ID, value)
		}
		if normErr != nil {
			callErr = fmt.Errorf("malformed response: %v", normErr)
		}
	}
	if callErr != nil && callErr.Error() == "" {
		callErr = errors.New("the read failed without a message")
	}
	return r.settle(pc, p, step, a.Seq, inputs, state, value, output, callErr)
}

// Substitution is the owner approving that a run continues one pinned
// capability with the replacement implementation this Evie has.
type Substitution struct {
	RunID      string `json:"run_id"`
	Capability string `json:"capability"`
	Version    string `json:"version"`
	Surface    string `json:"-"`
}

// Substitute records the owner's audited approval that a paused run
// continues a pinned capability with the replacement provider this Evie
// has (ADR 0041): only when it is the named version, offers the pinned
// contract and schema exactly, still serves the run's connections, and its
// plugin declares that it continues the pinned implementation. The run
// continues at its next recheck.
func (r *Runtime) Substitute(ctx context.Context, s Substitution) (eviedb.WorkflowRunSubstitution, error) {
	if !ownerSurface(s.Surface) {
		return eviedb.WorkflowRunSubstitution{}, eviedb.ErrWorkflowApprover
	}
	refuse := func(format string, args ...any) (eviedb.WorkflowRunSubstitution, error) {
		return eviedb.WorkflowRunSubstitution{}, fmt.Errorf("%w: %s", ErrSubstitutionRefused, fmt.Sprintf(format, args...))
	}
	d, err := r.store.WorkflowRun(ctx, s.RunID)
	if err != nil {
		return eviedb.WorkflowRunSubstitution{}, err
	}
	if d.Run.State.Terminal() {
		return eviedb.WorkflowRunSubstitution{}, fmt.Errorf("%w: %s", eviedb.ErrWorkflowRunFinished, d.Run.State)
	}
	var c composition
	if err := json.Unmarshal(d.Run.Composition, &c); err != nil {
		return refuse("the run's Execution Composition cannot be read: %v", err)
	}
	pin, ok := c.pin(s.Capability)
	if !ok {
		return refuse("run %s pins no provider of %s", s.RunID, s.Capability)
	}
	p, err := r.providers.ResolveWorkflowCapability(ctx, s.Capability)
	switch {
	case errors.Is(err, plugins.ErrWorkflowCapabilityUnknown):
		return refuse("no plugin in this Evie provides %s", s.Capability)
	case err != nil && !errors.Is(err, plugins.ErrWorkflowCapabilityUnavailable):
		return eviedb.WorkflowRunSubstitution{}, err
	case p.Version == pin.Version:
		return refuse("the run's pinned %s %s is the provider this Evie has; nothing needs substituting", pin.Plugin, pin.Version)
	case p.Version != s.Version:
		return refuse("this Evie has %s %s, not %s", p.Plugin, p.Version, s.Version)
	case p.Contract.Contract != pin.Contract || p.SchemaSHA256 != pin.SchemaSHA256:
		return refuse("%s %s offers contract %s (schema %s), not the pinned %s (schema %s); a changed contract needs a new approved definition or a migrated run",
			p.Plugin, p.Version, p.Contract.Contract, shortHash(p.SchemaSHA256), pin.Contract, shortHash(pin.SchemaSHA256))
	case !declaresContinuation(p, pin):
		return refuse("%s %s does not declare that it continues %s's %s with contract %s and schema %s", p.Plugin, p.Version, pin.Version, pin.Capability, pin.Contract, shortHash(pin.SchemaSHA256))
	}
	for _, connection := range pin.Connections {
		if len(p.Contract.Connections) > 0 && !slices.Contains(p.Contract.Connections, connection) {
			return refuse("%s %s no longer serves connection %s", p.Plugin, p.Version, connection)
		}
	}
	sub, err := r.store.RecordWorkflowRunSubstitution(ctx, eviedb.WorkflowRunSubstitution{
		RunID: s.RunID, Capability: pin.Capability, Plugin: string(p.Plugin), PinnedVersion: pin.Version, ReplacementVersion: p.Version,
		Contract: pin.Contract, SchemaSHA256: pin.SchemaSHA256, Surface: s.Surface,
		Evidence: fmt.Sprintf("plugin %s %s declares in its manifest that it continues %s's %s with contract %s and schema sha256:%s",
			p.Plugin, p.Version, pin.Version, pin.Capability, pin.Contract, pin.SchemaSHA256),
	})
	if err == nil {
		r.wake()
	}
	return sub, err
}

// providersLine renders a run's pinned providers.
func providersLine(c composition) string {
	parts := make([]string, len(c.Providers))
	for i, p := range c.Providers {
		parts[i] = fmt.Sprintf("%s %s from %s %s on %s (schema %s)", p.Capability, p.Contract, p.Plugin, p.Version, strings.Join(p.Connections, ", "), shortHash(p.SchemaSHA256))
		if p.Effect {
			parts[i] += ", " + workflow.EffectSummary(true, p.Idempotent, p.Reconciles, p.ConflictKeys)
		}
	}
	return strings.Join(parts, "; ")
}
