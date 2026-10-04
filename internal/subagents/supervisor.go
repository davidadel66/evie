// Package subagents supervises bounded foreground children in the Kernel.
package subagents

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/worktree"
	"github.com/google/uuid"
)

type Composition struct {
	Receipt                  composition.Receipt
	Toolset                  tools.Toolset
	Instructions             string
	CompatibilityResolutions []composition.CompatibilityResolution
}

// Resolver composes a worker child: the named worker preset ("research" or
// "explore") freshly when receipt is nil, otherwise exactly the pinned
// receipt, whose preset is that worker. Instructions are the worker preset's
// pinned instructions.
type Resolver func(ctx context.Context, worker string, receipt *composition.Receipt) (Composition, error)
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
	// now measures each child's time budget; tests inject a clock. The hard
	// deadline itself is a real-time context deadline.
	now func() time.Time
	// settleWindow bounds an owner's retries of its attempt's terminal write.
	settleWindow time.Duration
	// unsettled holds the outcome each owner decided but could not record
	// within settleWindow. Recovery passes and same-key retries record it.
	unsettled map[string]outcome
	// commits holds the harness's commit report of each build attempt until
	// its outcome is recorded with it (Stage 11).
	commits map[string]*delegation.BuildReport
	// commitTimeout bounds the harness commit at the end of a build attempt.
	commitTimeout time.Duration
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
	return &Supervisor{store: store, policy: policy, active: map[uint64]context.CancelFunc{}, idle: idle, now: time.Now,
		settleWindow: settleGrace, unsettled: map[string]outcome{}, commits: map[string]*delegation.BuildReport{}, commitTimeout: buildCommitTimeout}, nil
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

// childRuntime is one admitting call's model transport and composition resolver.
type childRuntime struct {
	client  agent.Client
	profile openrouter.ContextProfile
	resolve Resolver
}

// enter registers one admitting call so Stop cancels it, after reconciling
// abandoned attempts. The returned context is cancelled by Stop; leave must
// be called when the call returns.
func (s *Supervisor) enter(ctx context.Context, parent delegation.Parent) (context.Context, childRuntime, func(), error) {
	// Recovery isolates each record and RunRecovery reports failures, so an
	// unrelated unreadable record never blocks new admission.
	if _, err := s.store.RecoverSubagents(ctx); err != nil && ctx.Err() != nil {
		return nil, childRuntime{}, nil, ctx.Err()
	}
	// Deadlines belong to each attempt (see run), not to the whole call.
	ctx, cancel := context.WithCancelCause(ctx)
	s.mu.Lock()
	if !s.enabled || s.client == nil || s.resolve == nil {
		s.mu.Unlock()
		cancel(nil)
		return nil, childRuntime{}, nil, errors.New("Subagents Plugin or model runtime is unavailable")
	}
	if len(s.active) == 0 {
		s.idle = make(chan struct{})
	}
	s.next++
	call := s.next
	s.active[call] = func() { cancel(errShutdown) }
	rt := childRuntime{client: s.client, profile: s.profile, resolve: s.resolve}
	s.mu.Unlock()
	if parent.Profile != nil && parent.Profile.Model() != "" {
		rt.profile = *parent.Profile
	}
	leave := func() {
		cancel(nil)
		s.mu.Lock()
		delete(s.active, call)
		if len(s.active) == 0 {
			close(s.idle)
		}
		s.mu.Unlock()
	}
	return ctx, rt, leave, nil
}

// Delegate runs a delegate_research batch of web research children.
func (s *Supervisor) Delegate(ctx context.Context, parent delegation.Parent, requests []delegation.Assignment) ([]delegation.Result, error) {
	return s.delegate(ctx, parent, delegation.WorkerResearch, requests)
}

// Explore runs a delegate_explore batch of read-only explore children (Stage
// 7, 2026-10-03), each confined to a folder the harness resolves from the
// parent's Workspace folder or project root and the assignment's path. They
// are admitted, budgeted, wrapped up, settled, recovered and delivered exactly
// like research children.
func (s *Supervisor) Explore(ctx context.Context, parent delegation.Parent, requests []delegation.Assignment) ([]delegation.Result, error) {
	return s.delegate(ctx, parent, delegation.WorkerExplore, requests)
}

// Build runs a delegate_build batch of build children (Stage 11, 2026-10-03):
// each works on one Task in its own git worktree on a fresh branch, which the
// harness plans before admission, creates when the child starts and commits
// when it finishes. They are admitted, budgeted, wrapped up, settled,
// recovered and delivered exactly like research children.
func (s *Supervisor) Build(ctx context.Context, parent delegation.Parent, requests []delegation.Assignment) ([]delegation.Result, error) {
	return s.delegate(ctx, parent, delegation.WorkerBuild, requests)
}

func (s *Supervisor) delegate(ctx context.Context, parent delegation.Parent, worker string, requests []delegation.Assignment) ([]delegation.Result, error) {
	if err := s.policy.ValidateWorkerBatch(worker, requests); err != nil {
		return nil, err
	}
	ctx, rt, leave, err := s.enter(ctx, parent)
	if err != nil {
		return nil, err
	}
	defer leave()
	client, profile, resolve := rt.client, rt.profile, rt.resolve
	resolved, err := resolve(ctx, worker, nil)
	if err != nil {
		return nil, err
	}
	parentReceipt, err := s.store.GetCompositionReceipt(ctx, parent.Scope.SessionID)
	if err != nil {
		return nil, err
	}
	var childReceipt composition.Receipt
	var roots []string
	var plans []delegation.Worktree
	var planErrs []error
	switch worker {
	case delegation.WorkerExplore:
		if childReceipt, err = exploreReceipt(resolved.Receipt, parentReceipt); err != nil {
			return nil, err
		}
		if roots, err = s.exploreRoots(ctx, parent, requests); err != nil {
			return nil, err
		}
	case delegation.WorkerBuild:
		if childReceipt, err = buildReceipt(resolved.Receipt, parentReceipt); err != nil {
			return nil, err
		}
		if plans, planErrs, err = s.buildPlans(ctx, parent, requests); err != nil {
			return nil, err
		}
	default:
		if childReceipt, err = researchReceipt(resolved.Receipt, parentReceipt); err != nil {
			return nil, err
		}
	}
	resolved, err = resolve(ctx, worker, &childReceipt)
	if err != nil {
		return nil, err
	}
	dispatchID := uuid.NewString()
	var attempts []delegation.Attempt
	switch worker {
	case delegation.WorkerExplore:
		attempts, err = s.store.AdmitExploreSubagents(ctx, parent, requests, roots, resolved.Receipt, s.policy, dispatchID)
	case delegation.WorkerBuild:
		attempts, err = s.store.AdmitBuildSubagents(ctx, parent, requests, plans, planErrs, resolved.Receipt, s.policy, dispatchID)
	default:
		attempts, err = s.store.AdmitSubagents(ctx, parent, requests, resolved.Receipt, s.policy, dispatchID)
	}
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
	if ctx.Err() != nil {
		// The call itself was cancelled (a waiting duplicate, or shutdown):
		// it fails as a whole, as before. Owned children have already
		// settled as cancelled outcomes rather than failures.
		if err := errors.Join(failures...); err != nil {
			return results, err
		}
	}
	// Delivery rechecks current data access after the final child has
	// settled. A child that cannot be settled or delivered becomes an error
	// entry carrying none of its content; its siblings are still returned
	// (amended 2026-10-01; formerly one failure discarded the whole batch).
	deliveryCtx, stopDelivery := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stopDelivery()
	for i, a := range attempts {
		err := failures[i]
		if err == nil {
			_, err = s.store.InspectSubagent(deliveryCtx, parent, a.ID)
		}
		if err != nil {
			results[i] = undeliverable(a, err, s.policy.ResultBytes)
		}
	}
	return results, nil
}

// exploreRoots resolves each explore assignment's folder: its path inside the
// parent's Workspace folder or project root, or that folder itself; an
// absolute path when the parent has neither, as in a Workspace without a
// folder whose owner turned explore on (2026-10-03). Admission checks the
// Workspace's explore permission, then each root against the parent's
// current folder, again under the write lock.
func (s *Supervisor) exploreRoots(ctx context.Context, parent delegation.Parent, requests []delegation.Assignment) ([]string, error) {
	base := parent.Scope.ProjectRoot
	if parent.Scope.WorkspaceID != "" {
		folder, err := s.store.WorkspaceFolder(ctx, parent.Scope.WorkspaceID)
		if err != nil {
			return nil, err
		}
		base = folder.Path
	}
	roots := make([]string, len(requests))
	for i, r := range requests {
		root, err := tools.ResolveWorkerRoot(base, r.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid delegate_explore request: assignment %q path: %w", r.Key, err)
		}
		roots[i] = root
	}
	return roots, nil
}

// buildPlans plans each build assignment's worktree in the parent's
// Workspace folder or project root, without changing anything. A plan that
// cannot be made is kept as its error: only a fresh assignment needs one, so
// admission refuses it then, while a retained key still replays.
func (s *Supervisor) buildPlans(ctx context.Context, parent delegation.Parent, requests []delegation.Assignment) ([]delegation.Worktree, []error, error) {
	folder := parent.Scope.ProjectRoot
	if parent.Scope.WorkspaceID != "" {
		current, err := s.store.WorkspaceFolder(ctx, parent.Scope.WorkspaceID)
		if err != nil {
			return nil, nil, err
		}
		folder = current.Path
	}
	plans := make([]delegation.Worktree, len(requests))
	errs := make([]error, len(requests))
	for i, r := range requests {
		plans[i], errs[i] = worktree.Plan(ctx, folder, r.Base, uuid.NewString(), r.Objective)
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
	}
	return plans, errs, nil
}

// undeliverable is the error entry of one batch member whose result could
// not be settled or delivered. It names the assignment's key and the error,
// cut so the entry stays within the result limit, and nothing else.
func undeliverable(a delegation.Attempt, err error, limit int) delegation.Result {
	r := delegation.Result{ExecutionID: a.ID, ChildSessionID: a.Child.ID, Status: delegation.StatusError,
		Error: fmt.Sprintf("assignment %q: %v", a.Assignment.Key, err),
		Notes: []string{"Not delivered; the other results are unaffected. Repeat this assignment with the same idempotency_key to retry delivery without running it again."}}
	for r.ParentBytes() > limit && r.Error != "" {
		cut := len(r.Error) * 3 / 4
		for cut > 0 && !utf8.RuneStart(r.Error[cut]) {
			cut--
		}
		r.Error = r.Error[:cut]
	}
	if r.ParentBytes() > limit {
		r.Notes = nil
	}
	return r
}

// Continue extends one of parent's finished attempts: the child resumes its
// own session, history intact, with message as a follow-up assignment and a
// fresh time and token budget. The continuation is a new attempt admitted and
// supervised exactly like a fresh delegation under the current parent turn's
// authority. A retry of the same committed intent returns that attempt.
func (s *Supervisor) Continue(ctx context.Context, parent delegation.Parent, executionID, message string) (delegation.Result, error) {
	if err := s.policy.ValidateContinuation(executionID, message); err != nil {
		return delegation.Result{}, err
	}
	ctx, rt, leave, err := s.enter(ctx, parent)
	if err != nil {
		return delegation.Result{}, err
	}
	defer leave()
	dispatchID := uuid.NewString()
	a, err := s.store.AdmitSubagentContinuation(ctx, parent, executionID, message, s.policy, dispatchID)
	if err != nil {
		return delegation.Result{}, err
	}
	result, err := s.run(ctx, parent, a, rt.client, rt.profile, rt.resolve, a.DispatchID == dispatchID)
	if err != nil {
		return result, err
	}
	// Delivery rechecks current data access after the child has settled.
	deliveryCtx, stopDelivery := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stopDelivery()
	if _, err := s.store.InspectSubagent(deliveryCtx, parent, a.ID); err != nil {
		return delegation.Result{}, err
	}
	return result, nil
}

// run settles one attempt of the batch. The dispatch that admitted an attempt
// owns it: it starts the attempt when capacity allows and records its terminal
// outcome. Other callers only observe it, and their result is a replay of the
// earlier call's attempt, marked with the time that attempt ended.
func (s *Supervisor) run(ctx context.Context, parent delegation.Parent, a delegation.Attempt, client agent.Client, profile openrouter.ContextProfile, resolve Resolver, owns bool) (delegation.Result, error) {
	if owns && !a.Terminal() {
		return s.own(ctx, parent, a, client, profile, resolve)
	}
	settled, err := s.join(ctx, parent, a)
	if err != nil {
		return delegation.Result{}, err
	}
	result := *settled.Result
	if !owns {
		result.Replayed, result.CompletedAt = true, settled.EndedAt
	}
	return result, nil
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
			settled, err := s.join(ctx, parent, current)
			if err != nil {
				return delegation.Result{}, err
			}
			return *settled.Result, nil
		case ctx.Err() != nil:
			return s.finish(ctx, id, stopped(ctx))
		case errors.Is(err, delegation.ErrCapacity) || transientStoreError(err):
		default:
			return s.finish(ctx, id, refused(err))
		}
		select {
		case <-ctx.Done():
			return s.finish(ctx, id, stopped(ctx))
		case <-queued.C:
			return s.finish(ctx, id, outcome{"failed", "queue_deadline"})
		case <-time.After(capacityPollInterval):
		}
	}
}

// join observes an attempt until it is terminal and returns it, with its
// retained result, after current access checks. Leaving early never changes
// the attempt: its owner settles it within two deadlines of admission. An
// attempt whose owner could not record its outcome is settled here with that
// outcome, or the join fails at once saying why (amended 2026-10-01).
func (s *Supervisor) join(ctx context.Context, parent delegation.Parent, a delegation.Attempt) (delegation.Attempt, error) {
	settled := time.NewTimer(time.Until(a.CreatedAt.Add(2*a.Policy.Deadline + settleGrace)))
	defer settled.Stop()
	for {
		current, err := s.store.InspectSubagent(ctx, parent, a.ID)
		if err != nil && (ctx.Err() != nil || !transientStoreError(err)) {
			return delegation.Attempt{}, err
		}
		if err == nil && current.Terminal() {
			if current.Result == nil {
				return delegation.Attempt{}, errors.New("terminal subagent result is missing")
			}
			return current, nil
		}
		if err == nil {
			if o, pending := s.pendingOutcome(a.ID); pending {
				if _, err := s.settleOnce(ctx, a.ID, o); err != nil {
					return delegation.Attempt{}, fmt.Errorf("execution %s has not settled: recording its outcome failed (%w); recovery keeps retrying, so repeat this request later", a.ID, err)
				}
				continue
			}
		}
		select {
		case <-ctx.Done():
			return delegation.Attempt{}, ctx.Err()
		case <-settled.C:
			return delegation.Attempt{}, errors.New("subagent execution did not settle within its deadlines")
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

// refused classifies a durable refusal to start a queued attempt: the parent's
// authority or scope ended (its lease, its project, or its active session), or
// the store failed. Amended 2026-10-01; formerly one cancelled/
// authority_or_cancellation for both.
func refused(err error) outcome {
	if errors.Is(err, delegation.ErrAuthority) || errors.Is(err, eviedb.ErrTurnLeaseLost) ||
		errors.Is(err, eviedb.ErrProjectNotActive) || errors.Is(err, sql.ErrNoRows) {
		return outcome{"interrupted", "authority_ended"}
	}
	return outcome{"failed", "infrastructure_failure"}
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
	case errors.Is(err, agent.ErrStepLimitExceeded):
		// The tool-free wrap-up response still requested tools, so nothing
		// from it was committed.
		return outcome{"failed", delegation.ReasonWrapUpFailed}
	// Policy limits name the limit reached (2026-10-01; formerly all
	// policy_limit).
	case errors.Is(err, errTokenBudgetSpent):
		return outcome{"failed", delegation.ReasonTokenBudgetSpent}
	case errors.Is(err, errResponseTooLarge):
		return outcome{"failed", delegation.ReasonResponseTooLarge}
	case errors.Is(err, errRequestTooLarge) || errors.Is(err, agent.ErrContextOverflow):
		return outcome{"failed", delegation.ReasonContextLimit}
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

// terminalWriteTimeout bounds one attempt at recording a terminal outcome.
const terminalWriteTimeout = 3 * time.Second

// finish records the owner's terminal outcome for id. call is the owning
// Delegate or Continue call, which Stop cancels. While call is live, lock
// contention, or a write that outlives its timeout, is retried with bounded
// backoff for up to settleWindow; like every other supervisor store call,
// contention proves nothing. Once call has ended (shutdown or a cancelled
// parent), the owner makes only the one bounded write its outcome needs, and
// a retry wait or write in progress stops at once. An outcome that cannot be
// recorded is handed to recovery (settlePending) and to same-key retries
// (join), so the attempt does not keep its running slot until its parent turn
// ends (amended 2026-10-01).
func (s *Supervisor) finish(call context.Context, id string, o outcome) (delegation.Result, error) {
	// A stopped attempt still records why it stopped: when call has already
	// ended, that one write is bounded only by its own timeout.
	write := call
	if call.Err() != nil {
		write = context.WithoutCancel(call)
	}
	deadline := time.Now().Add(s.settleWindow)
	delay := capacityPollInterval
	for {
		a, err := s.settleOnce(write, id, o)
		if err == nil {
			return *a.Result, nil
		}
		if !retryableTerminalWrite(err) || call.Err() != nil || time.Now().Add(delay).After(deadline) {
			return s.handOff(id, o, err)
		}
		retry := time.NewTimer(delay)
		select {
		case <-call.Done():
			retry.Stop()
			return s.handOff(id, o, err)
		case <-retry.C:
		}
		delay = min(2*delay, time.Second)
	}
}

// handOff keeps an outcome its owner could not record for recovery and
// same-key retries to record.
func (s *Supervisor) handOff(id string, o outcome, err error) (delegation.Result, error) {
	s.mu.Lock()
	s.unsettled[id] = o
	s.mu.Unlock()
	return delegation.Result{}, fmt.Errorf("recording the outcome failed; recovery keeps retrying: %w", err)
}

// settleOnce makes one attempt, bounded by terminalWriteTimeout and ended
// with ctx, to record o for id. Recording an attempt that is already terminal
// returns it unchanged, so an outcome recorded by recovery or another settler
// is never replaced.
func (s *Supervisor) settleOnce(ctx context.Context, id string, o outcome) (delegation.Attempt, error) {
	write, cancel := context.WithTimeout(ctx, terminalWriteTimeout)
	defer cancel()
	s.mu.Lock()
	commit := s.commits[id]
	s.mu.Unlock()
	var a delegation.Attempt
	var err error
	if commit != nil {
		a, err = s.store.FinishBuildSubagent(write, id, o.state, o.reason, commit)
	} else {
		a, err = s.store.FinishSubagent(write, id, o.state, o.reason)
	}
	if err == nil && a.Result == nil {
		err = errors.New("terminal subagent result is missing")
	}
	if err != nil {
		if write.Err() != nil && ctx.Err() == nil {
			err = fmt.Errorf("%w: %w", errTerminalWriteTimeout, err)
		}
		return delegation.Attempt{}, err
	}
	s.mu.Lock()
	delete(s.unsettled, id)
	delete(s.commits, id)
	s.mu.Unlock()
	return a, nil
}

// errTerminalWriteTimeout marks a terminal write that outlived its own
// timeout, which under SQLite's busy timeout means it waited on the lock.
var errTerminalWriteTimeout = errors.New("terminal write timed out")

func retryableTerminalWrite(err error) bool {
	return transientStoreError(err) || errors.Is(err, errTerminalWriteTimeout)
}

// pendingOutcome reports the outcome an owner decided for id but could not
// record.
func (s *Supervisor) pendingOutcome(id string) (outcome, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.unsettled[id]
	return o, ok
}

// settlePending makes one attempt to record every outcome an owner could not.
// Recovery runs it on each pass, so an unsettled attempt is settled as soon as
// the store accepts the write, while its parent turn is still live. It stops
// when ctx ends, leaving the rest pending.
func (s *Supervisor) settlePending(ctx context.Context) error {
	s.mu.Lock()
	pending := make(map[string]outcome, len(s.unsettled))
	for id, o := range s.unsettled {
		pending[id] = o
	}
	s.mu.Unlock()
	var failures []error
	for id, o := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := s.settleOnce(ctx, id, o); err != nil {
			failures = append(failures, fmt.Errorf("settle subagent execution %q: %w", id, err))
		}
	}
	return errors.Join(failures...)
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
	// The owning call outlives the attempt's own deadline and authority:
	// recording the outcome is bounded by the call (see finish).
	call := ctx
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
	resolved, err := resolve(ctx, a.Worker(), &a.Receipt)
	if err != nil {
		return s.finish(call, a.ID, outcome{"failed", "composition_unavailable"})
	}
	toolset := resolved.Toolset
	switch a.Worker() {
	case delegation.WorkerExplore:
		// The explore tools read only inside the attempt's pinned root.
		toolset = toolset.WithWorkerRoot(a.Root)
	case delegation.WorkerBuild:
		// The harness creates the worktree on its fresh branch, or finds the
		// one a continuation resumes, before the child runs; the build tools
		// write only there (Stage 11).
		if err := s.prepareWorktree(ctx, a); err != nil {
			return s.finish(call, a.ID, outcome{"failed", reasonWorktreeUnavailable})
		}
		toolset = toolset.WithBuildFence(tools.BuildFence{Worktree: a.Worktree.Path, Temp: a.Worktree.Temp})
	}
	if err := s.store.AppendCompatibilityResolutions(ctx, a.Child.ID, resolved.CompatibilityResolutions); err != nil {
		return s.finish(call, a.ID, outcome{"failed", "composition_unavailable"})
	}
	if a.Policy.Validate() != nil {
		// Policies pinned before the 2026-10-01 budgets cannot run. Distinct
		// from a model profile that cannot take the policy (below).
		return s.finish(call, a.ID, outcome{"failed", "pinned_policy_invalid"})
	}
	metered := &budget{client: client, policy: a.Policy, worker: a.Worker(), now: s.now, started: s.now(),
		authorize: func(ctx context.Context) error { return s.authorize(ctx, a.ID) },
		record:    func(reason string) { s.recordWrapUp(ctx, a.ID, reason) }}
	// The child gets the model's normal output reserve; only its request
	// bytes are narrowed.
	profile, err = profile.WithWorkerLimits(int64(a.Policy.RequestBytes), profile.OutputReserveTokens())
	if err != nil {
		return s.finish(call, a.ID, outcome{"failed", "invalid_model_policy"})
	}
	holder := a.Child.ID
	session := agent.NewDelegatedWithToolset(metered, profile, s.store.BindHistory(a.Child.ID, stringHolder(holder)), a.Child.ScopeContext(), s.store.BindTurnOwner(a.Child.ID, stringHolder(holder)), toolset, resolved.Instructions, agent.WithWrapUp(metered.wrapUp))
	var o outcome
	if err = session.Send(ctx, assignmentMessage(a), quietEvents{}, nil); err != nil {
		o = executionFailure(ctx, err, deadline)
		if o.reason == delegation.ReasonContextLimit && errors.Is(err, agent.ErrContextOverflow) && metered.wrapUpReason() != "" {
			// Even the smallest wrap-up request could not fit.
			o.reason = delegation.ReasonWrapUpFailed
		}
	} else if reason := metered.wrapUpReason(); reason != "" {
		o = outcome{delegation.StatePartial, reason}
	} else {
		o = outcome{"succeeded", ""}
	}
	if a.Worker() == delegation.WorkerBuild {
		// Whatever the outcome, the harness commits the worker's changes on
		// its branch before settling, so the result lists them.
		s.commitBuild(call, a)
	}
	return s.finish(call, a.ID, o)
}

// reasonWorktreeUnavailable is a build attempt whose worktree the harness
// could not create or find.
const reasonWorktreeUnavailable = "worktree_unavailable"

// buildCommitTimeout bounds the harness commit and report at the end of a
// build attempt.
const buildCommitTimeout = 30 * time.Second

// prepareWorktree creates a fresh build attempt's worktree, or verifies the
// existing one a continuation resumes. A failure is recorded in the attempt's
// build report so the parent learns why.
func (s *Supervisor) prepareWorktree(ctx context.Context, a delegation.Attempt) error {
	if a.Worktree == nil {
		return errors.New("the build attempt has no pinned worktree")
	}
	err := worktree.Ensure(ctx, *a.Worktree, a.Continues == nil)
	if err != nil {
		// Before the child runs, the harness's own words name only harness
		// paths, so the reason is kept, bounded.
		reason := err.Error()
		if len(reason) > 300 {
			reason = strings.ToValidUTF8(reason[:300], "") + "…"
		}
		w := a.Worktree
		s.mu.Lock()
		s.commits[a.ID] = &delegation.BuildReport{Branch: w.Branch, Base: w.Base, BaseCommit: w.BaseCommit, Worktree: w.Path,
			CommitError: "the harness could not prepare the worktree, so nothing ran in it: " + reason}
		s.mu.Unlock()
	}
	return err
}

// commitBuild commits the worker's changes on its branch as the harness and
// keeps the branch report for settlement. It runs after the child's turn,
// whatever ended it, bounded by commitTimeout even when the owning call was
// cancelled.
func (s *Supervisor) commitBuild(call context.Context, a delegation.Attempt) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(call), s.commitTimeout)
	defer cancel()
	w := *a.Worktree
	_, commitErr := worktree.Commit(ctx, w, buildCommitMessage(a))
	report, err := worktree.Report(ctx, w)
	if err != nil {
		report = delegation.BuildReport{Branch: w.Branch, Base: w.Base, BaseCommit: w.BaseCommit, Worktree: w.Path, Uncommitted: true,
			CommitError: "the branch could not be read: " + harnessWords(err)}
	}
	if commitErr != nil {
		report.Uncommitted = true
		report.CommitError = harnessWords(commitErr)
	}
	s.mu.Lock()
	s.commits[a.ID] = &report
	s.mu.Unlock()
}

// harnessWords is a harness error as the parent may read it outside the
// child-output frame: fixed wording, never worker-chosen names.
func harnessWords(err error) string {
	switch {
	case errors.Is(err, worktree.ErrNestedRepository):
		return "the worktree contains a nested .git folder, which the harness never commits"
	case errors.Is(err, context.DeadlineExceeded):
		return "git did not finish in time"
	}
	return "git failed; inspect the worktree"
}

// buildCommitMessage is the harness's plain commit message: the
// assignment's first line, or the follow-up's for a continuation, and the
// execution that made it.
func buildCommitMessage(a delegation.Attempt) string {
	subject, _, _ := strings.Cut(strings.TrimSpace(a.Assignment.Objective), "\n")
	subject = strings.TrimSpace(subject)
	if a.Continues != nil {
		subject = "Continue: " + subject
	}
	if runes := []rune(subject); len(runes) > 72 {
		subject = strings.TrimSpace(string(runes[:71])) + "…"
	}
	if subject == "" {
		subject = "Evie build"
	}
	return fmt.Sprintf("%s\n\nEvie build execution %s.", subject, a.ID)
}

// recordWrapUp persists the wrap-up before the child's final call. A failure
// leaves the in-memory reason, which still settles this live attempt as
// partial; only recovery after a crash depends on the durable mark.
func (s *Supervisor) recordWrapUp(ctx context.Context, id, reason string) {
	for {
		err := s.store.RecordSubagentWrapUp(ctx, id, reason)
		if err == nil || ctx.Err() != nil || !transientStoreError(err) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(capacityPollInterval):
		}
	}
}

// ReadReport pages the stored report of one of parent's own attempts.
func (s *Supervisor) ReadReport(ctx context.Context, parent delegation.Parent, executionID string, offset, limit int) (delegation.ReportPage, error) {
	return s.store.ReadSubagentReport(ctx, parent, executionID, offset, limit)
}

type quietEvents struct{}

func (quietEvents) Delta(string)                                  {}
func (quietEvents) Reasoning(string)                              {}
func (quietEvents) ReasoningDone()                                {}
func (quietEvents) AssistantDone(string)                          {}
func (quietEvents) ToolCall(string, string, string)               {}
func (quietEvents) ToolResult(string, string, bool)               {}
func (quietEvents) ResponseDiscarded(agent.DiscardReason, string) {}
