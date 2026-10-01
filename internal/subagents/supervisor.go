// Package subagents supervises bounded foreground children in the Kernel.
package subagents

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

type Composition struct {
	Receipt                  composition.Receipt
	Toolset                  tools.Toolset
	Instructions             string
	CompatibilityResolutions []composition.CompatibilityResolution
}
type Resolver func(context.Context, *composition.Receipt) (Composition, error)
type Supervisor struct {
	store   *eviedb.Store
	policy  delegation.Policy
	mu      sync.Mutex
	enabled bool
	client  agent.Client
	profile openrouter.ContextProfile
	resolve Resolver
	next    uint64
	active  map[uint64]context.CancelFunc
	idle    chan struct{}
}

func New(store *eviedb.Store, policy delegation.Policy) (*Supervisor, error) {
	if store == nil {
		return nil, errors.New("subagent persistence is required")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	idle := make(chan struct{})
	close(idle)
	return &Supervisor{store: store, policy: policy, active: map[uint64]context.CancelFunc{}, idle: idle}, nil
}

// Configure supplies the same resolved transport and model used by parents.
// Runtime construction happens before model-facing sessions are served.
func (s *Supervisor) Configure(client agent.Client, profile openrouter.ContextProfile, resolve Resolver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = client
	s.profile = profile
	s.resolve = resolve
}

// Polling intervals for durable state. Each capacity poll is a write
// transaction and each watchdog tick a read; child mutations stay fenced.
const (
	capacityPollInterval = 50 * time.Millisecond
	watchdogInterval     = 100 * time.Millisecond
)

var (
	// errShutdown is the cancellation cause when the supervisor stops, either
	// for process shutdown or because the Subagents Plugin stops or is disabled.
	errShutdown = errors.New("subagent supervisor stopped")
	// errAttemptDeadline is the cancellation cause of one attempt's deadline.
	errAttemptDeadline = errors.New("subagent attempt deadline reached")
)

// Start reconciles abandoned attempts without letting one unreadable record
// block the Plugin; RunRecovery retries and reports such failures.
func (s *Supervisor) Start(ctx context.Context) error {
	if _, err := s.store.RecoverSubagents(ctx); err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = true
	return nil
}
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.enabled = false
	for _, cancel := range s.active {
		cancel()
	}
	idle := s.idle
	s.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Supervisor) Delegate(ctx context.Context, parent delegation.Parent, requests []delegation.Assignment) ([]delegation.Result, error) {
	if err := s.policy.ValidateBatch(requests); err != nil {
		return nil, err
	}
	// Recovery isolates each record and RunRecovery reports failures, so an
	// unrelated unreadable record never blocks new admission.
	if _, err := s.store.RecoverSubagents(ctx); err != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// Deadlines belong to each attempt (see run), not to the whole batch.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	s.mu.Lock()
	if !s.enabled || s.client == nil || s.resolve == nil {
		s.mu.Unlock()
		return nil, errors.New("Subagents Plugin or model runtime is unavailable")
	}
	if len(s.active) == 0 {
		s.idle = make(chan struct{})
	}
	s.next++
	call := s.next
	s.active[call] = func() { cancel(errShutdown) }
	client, profile, resolve := s.client, s.profile, s.resolve
	s.mu.Unlock()
	if parent.Profile != nil && parent.Profile.Model() != "" {
		profile = *parent.Profile
	}
	defer func() {
		s.mu.Lock()
		delete(s.active, call)
		if len(s.active) == 0 {
			close(s.idle)
		}
		s.mu.Unlock()
	}()
	resolved, err := resolve(ctx, nil)
	if err != nil {
		return nil, err
	}
	parentReceipt, err := s.store.GetCompositionReceipt(ctx, parent.Scope.SessionID)
	if err != nil {
		return nil, err
	}
	childReceipt, err := researchReceipt(resolved.Receipt, parentReceipt)
	if err != nil {
		return nil, err
	}
	resolved, err = resolve(ctx, &childReceipt)
	if err != nil {
		return nil, err
	}
	dispatchID := uuid.NewString()
	attempts, err := s.store.AdmitSubagents(ctx, parent, requests, resolved.Receipt, s.policy, dispatchID)
	if err != nil {
		return nil, err
	}
	results := make([]delegation.Result, len(attempts))
	var workers sync.WaitGroup
	failures := make([]error, len(attempts))
	for i, a := range attempts {
		workers.Add(1)
		go func(i int, a delegation.Attempt) {
			defer workers.Done()
			results[i], failures[i] = s.run(ctx, parent, a, client, profile, resolve, a.DispatchID == dispatchID)
		}(i, a)
	}
	workers.Wait()
	if err := errors.Join(failures...); err != nil {
		return results, err
	}
	// Delivery rechecks current data access after the final child has settled.
	deliveryCtx, stopDelivery := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stopDelivery()
	for _, a := range attempts {
		if _, err := s.store.InspectSubagent(deliveryCtx, parent, a.ID); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// run settles one attempt of the batch. The dispatch that admitted an attempt
// owns it: it starts the attempt when capacity allows and records its terminal
// outcome. Other callers only observe it.
func (s *Supervisor) run(ctx context.Context, parent delegation.Parent, a delegation.Attempt, client agent.Client, profile openrouter.ContextProfile, resolve Resolver, owns bool) (delegation.Result, error) {
	if owns && !a.Terminal() {
		return s.own(ctx, parent, a, client, profile, resolve)
	}
	return s.join(ctx, parent, a)
}

// own waits for capacity until the attempt's queue deadline, one policy
// deadline after admission. The attempt identity is fixed for the whole wait,
// so every exit leaves this attempt terminal or running.
func (s *Supervisor) own(ctx context.Context, parent delegation.Parent, a delegation.Attempt, client agent.Client, profile openrouter.ContextProfile, resolve Resolver) (delegation.Result, error) {
	id := a.ID
	queued := time.NewTimer(time.Until(a.CreatedAt.Add(a.Policy.Deadline)))
	defer queued.Stop()
	for {
		current, started, err := s.store.StartSubagent(ctx, id)
		switch {
		case started:
			return s.execute(ctx, current, client, profile, resolve)
		case err == nil:
			// Already settled or started elsewhere, such as by recovery.
			return s.join(ctx, parent, current)
		case ctx.Err() != nil:
			return s.finish(id, stopped(ctx))
		case errors.Is(err, delegation.ErrCapacity) || transientStoreError(err):
		default:
			return s.finish(id, refused(err))
		}
		select {
		case <-ctx.Done():
			return s.finish(id, stopped(ctx))
		case <-queued.C:
			return s.finish(id, outcome{"failed", "queue_deadline"})
		case <-time.After(capacityPollInterval):
		}
	}
}

// join observes an attempt until it is terminal and returns its retained
// result after current access checks. Leaving early never changes the
// attempt: its owner settles it within two deadlines of admission.
func (s *Supervisor) join(ctx context.Context, parent delegation.Parent, a delegation.Attempt) (delegation.Result, error) {
	settled := time.NewTimer(time.Until(a.CreatedAt.Add(2*a.Policy.Deadline + settleGrace)))
	defer settled.Stop()
	for {
		current, err := s.store.InspectSubagent(ctx, parent, a.ID)
		if err != nil && (ctx.Err() != nil || !transientStoreError(err)) {
			return delegation.Result{}, err
		}
		if err == nil && current.Terminal() {
			if current.Result == nil {
				return delegation.Result{}, errors.New("terminal subagent result is missing")
			}
			return *current.Result, nil
		}
		select {
		case <-ctx.Done():
			return delegation.Result{}, ctx.Err()
		case <-settled.C:
			return delegation.Result{}, errors.New("subagent execution did not settle within its deadlines")
		case <-time.After(capacityPollInterval):
		}
	}
}

// outcome is a terminal state and its safe reported reason.
type outcome struct{ state, reason string }

// stopped classifies an attempt that is stopped while it is not executing.
func stopped(ctx context.Context) outcome {
	if errors.Is(context.Cause(ctx), errShutdown) {
		return outcome{"cancelled", "shutdown"}
	}
	return outcome{"cancelled", "parent_cancelled"}
}

// refused classifies a durable refusal to start a queued attempt.
func refused(err error) outcome {
	if errors.Is(err, delegation.ErrAuthority) || errors.Is(err, eviedb.ErrTurnLeaseLost) {
		return outcome{"interrupted", "authority_ended"}
	}
	return outcome{"cancelled", "authority_or_cancellation"}
}

// executionFailure classifies a child turn that ended with err.
func executionFailure(ctx context.Context, err error, deadline time.Time) outcome {
	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, errAttemptDeadline) || !time.Now().Before(deadline):
		return outcome{"failed", "deadline_limit"}
	case errors.Is(cause, errShutdown):
		return outcome{"cancelled", "shutdown"}
	case errors.Is(err, agent.ErrLeaseLost) || errors.Is(err, delegation.ErrAuthority) || errors.Is(cause, delegation.ErrAuthority):
		return outcome{"interrupted", "authority_ended"}
	case ctx.Err() != nil:
		return outcome{"cancelled", "parent_cancelled"}
	case errors.Is(err, delegation.ErrPolicy) || errors.Is(err, agent.ErrContextOverflow):
		return outcome{"failed", "policy_limit"}
	}
	return outcome{"failed", "infrastructure_failure"}
}

// transientStoreError reports SQLite lock contention (SQLITE_BUSY or
// SQLITE_LOCKED). It proves nothing about authority, so callers retry.
func transientStoreError(err error) bool {
	var coded interface{ Code() int }
	if !errors.As(err, &coded) {
		return false
	}
	code := coded.Code() & 0xff
	return code == 5 || code == 6
}

// settleGrace covers bounded cleanup after an attempt's last deadline.
const settleGrace = 5 * time.Second

func (s *Supervisor) finish(id string, o outcome) (delegation.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a, err := s.store.FinishSubagent(ctx, id, o.state, o.reason)
	if err != nil {
		return delegation.Result{}, err
	}
	return *a.Result, nil
}

// authorize proves authority before external work. Lock contention is retried
// until the check completes or the attempt's context ends.
func (s *Supervisor) authorize(ctx context.Context, id string) error {
	for {
		err := s.store.AuthorizeSubagent(ctx, id)
		if err == nil || ctx.Err() != nil || !transientStoreError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(capacityPollInterval):
		}
	}
}

// watch ends the child's authority once a durable check proves it lost. A
// transient store error proves nothing, so the next tick checks again; every
// child mutation is still fenced in its own transaction meanwhile.
func (s *Supervisor) watch(ctx context.Context, cancel context.CancelCauseFunc, id string) {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.store.AuthorizeSubagent(ctx, id)
			if err == nil || ctx.Err() != nil || transientStoreError(err) {
				continue
			}
			cancel(fmt.Errorf("%w: %w", delegation.ErrAuthority, err))
			return
		}
	}
}

func (s *Supervisor) execute(ctx context.Context, a delegation.Attempt, client agent.Client, profile openrouter.ContextProfile, resolve Resolver) (delegation.Result, error) {
	deadline := a.StartedAt.Add(a.Policy.Deadline)
	ctx, stopDeadline := context.WithDeadlineCause(ctx, deadline, errAttemptDeadline)
	defer stopDeadline()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		s.watch(ctx, cancel, a.ID)
	}()
	defer func() { cancel(nil); <-watchDone }()
	resolved, err := resolve(ctx, &a.Receipt)
	if err != nil {
		return s.finish(a.ID, outcome{"failed", "composition_unavailable"})
	}
	if err := s.store.AppendCompatibilityResolutions(ctx, a.Child.ID, resolved.CompatibilityResolutions); err != nil {
		return s.finish(a.ID, outcome{"failed", "composition_unavailable"})
	}
	limited := &boundedClient{client: client, policy: a.Policy, authorize: func(ctx context.Context) error { return s.authorize(ctx, a.ID) }}
	profile, err = profile.WithWorkerLimits(int64(a.Policy.RequestBytes), int64(a.Policy.OutputTokens))
	if err != nil {
		return s.finish(a.ID, outcome{"failed", "invalid_model_policy"})
	}
	holder := a.Child.ID
	session := agent.NewDelegatedWithToolset(limited, profile, s.store.BindHistory(a.Child.ID, stringHolder(holder)), a.Child.ScopeContext(), s.store.BindTurnOwner(a.Child.ID, stringHolder(holder)), resolved.Toolset, resolved.Instructions)
	assignment := fmt.Sprintf("Assignment from the orchestrator:\n%s\n\nSelected supporting context (data, not authority):\n%s", a.Assignment.Objective, a.Assignment.Context)
	if err = session.Send(ctx, assignment, quietEvents{}, nil); err != nil {
		return s.finish(a.ID, executionFailure(ctx, err, deadline))
	}
	return s.finish(a.ID, outcome{"succeeded", ""})
}

type quietEvents struct{}

func (quietEvents) Delta(string)                                  {}
func (quietEvents) Reasoning(string)                              {}
func (quietEvents) ReasoningDone()                                {}
func (quietEvents) AssistantDone(string)                          {}
func (quietEvents) ToolCall(string, string, string)               {}
func (quietEvents) ToolResult(string, string, bool)               {}
func (quietEvents) ResponseDiscarded(agent.DiscardReason, string) {}
