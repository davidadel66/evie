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
	Receipt      composition.Receipt
	Toolset      tools.Toolset
	Instructions string
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
func (s *Supervisor) Start(ctx context.Context) error {
	if _, err := s.store.RecoverSubagents(ctx); err != nil {
		return err
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
	if _, err := s.store.RecoverSubagents(ctx); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.policy.Deadline)
	defer cancel()
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
	s.active[call] = cancel
	client, profile, resolve := s.client, s.profile, s.resolve
	s.mu.Unlock()
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

func (s *Supervisor) run(ctx context.Context, parent delegation.Parent, a delegation.Attempt, client agent.Client, profile openrouter.ContextProfile, resolve Resolver, owns bool) (delegation.Result, error) {
	joined := !owns
	for !a.Terminal() {
		var current delegation.Attempt
		var started bool
		var err error
		if owns {
			current, started, err = s.store.StartSubagent(ctx, a.ID)
		} else {
			current, err = s.store.InspectSubagent(ctx, parent, a.ID)
		}
		if err != nil && !errors.Is(err, delegation.ErrCapacity) {
			if joined {
				return delegation.Result{}, err
			}
			return s.finish(a.ID, "cancelled", "authority_or_cancellation")
		}
		if started {
			return s.execute(ctx, current, client, profile, resolve)
		}
		if err == nil {
			a = current
			joined = !owns
			if a.Terminal() {
				break
			}
		}
		select {
		case <-ctx.Done():
			if joined {
				return delegation.Result{}, ctx.Err()
			}
			return s.finish(a.ID, "cancelled", "parent_cancelled")
		case <-time.After(20 * time.Millisecond):
		}
		a, err = s.store.InspectSubagent(ctx, parent, a.ID)
		if err != nil {
			if joined {
				return delegation.Result{}, err
			}
			return s.finish(a.ID, "interrupted", "authority_ended")
		}
	}
	checked, err := s.store.InspectSubagent(ctx, parent, a.ID)
	if err != nil {
		return delegation.Result{}, err
	}
	if checked.Result == nil {
		return delegation.Result{}, errors.New("terminal subagent result is missing")
	}
	return *checked.Result, nil
}
func (s *Supervisor) finish(id, state, reason string) (delegation.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a, err := s.store.FinishSubagent(ctx, id, state, reason)
	if err != nil {
		return delegation.Result{}, err
	}
	return *a.Result, nil
}
func (s *Supervisor) execute(ctx context.Context, a delegation.Attempt, client agent.Client, profile openrouter.ContextProfile, resolve Resolver) (delegation.Result, error) {
	ctx, stopDeadline := context.WithDeadline(ctx, a.StartedAt.Add(a.Policy.Deadline))
	defer stopDeadline()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.store.AuthorizeSubagent(ctx, a.ID); err != nil {
					cancel(fmt.Errorf("%w: %w", delegation.ErrAuthority, err))
					return
				}
			}
		}
	}()
	defer func() { cancel(nil); <-watchDone }()
	resolved, err := resolve(ctx, &a.Receipt)
	if err != nil {
		return s.finish(a.ID, "failed", "composition_unavailable")
	}
	limited := &boundedClient{client: client, policy: a.Policy, authorize: func(ctx context.Context) error { return s.store.AuthorizeSubagent(ctx, a.ID) }}
	profile, err = profile.WithOutputLimit(int64(a.Policy.OutputTokens))
	if err != nil {
		return s.finish(a.ID, "failed", "invalid_model_policy")
	}
	holder := a.Child.ID
	session := agent.NewDelegatedWithToolset(limited, profile, s.store.BindHistory(a.Child.ID, stringHolder(holder)), a.Child.ScopeContext(), s.store.BindTurnOwner(a.Child.ID, stringHolder(holder)), resolved.Toolset, resolved.Instructions)
	assignment := fmt.Sprintf("Assignment from the orchestrator:\n%s\n\nSelected supporting context (data, not authority):\n%s", a.Assignment.Objective, a.Assignment.Context)
	err = session.Send(ctx, assignment, quietEvents{}, nil)
	state, reason := "succeeded", ""
	if err != nil {
		state, reason = "failed", "infrastructure_failure"
		if errors.Is(err, delegation.ErrPolicy) || errors.Is(err, agent.ErrContextOverflow) {
			reason = "policy_limit"
		}
		if ctx.Err() != nil {
			state, reason = "cancelled", "parent_cancelled"
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			state, reason = "failed", "deadline_limit"
		}
		if (errors.Is(err, agent.ErrLeaseLost) || errors.Is(err, delegation.ErrAuthority) || errors.Is(context.Cause(ctx), delegation.ErrAuthority)) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			state, reason = "interrupted", "authority_ended"
		}
	}
	return s.finish(a.ID, state, reason)
}

type quietEvents struct{}

func (quietEvents) Delta(string)                                  {}
func (quietEvents) Reasoning(string)                              {}
func (quietEvents) ReasoningDone()                                {}
func (quietEvents) AssistantDone(string)                          {}
func (quietEvents) ToolCall(string, string, string)               {}
func (quietEvents) ToolResult(string, string, bool)               {}
func (quietEvents) ResponseDiscarded(agent.DiscardReason, string) {}
