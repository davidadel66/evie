package memorybackground

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// Store is the durable state the pass needs. The pass owns this interface;
// *eviedb.Store implements it.
type Store interface {
	EnsureMemoryBackgroundActivation(context.Context) (time.Time, error)
	AcquireMemoryBackgroundRunner(context.Context, string, time.Duration) (bool, error)
	ReleaseMemoryBackgroundRunner(context.Context, string) error
	DueMemoryBackgroundSessions(context.Context, memory.BackgroundDueQuery) ([]memory.BackgroundDue, error)
	MemoryBackgroundBatch(context.Context, memory.SessionID, memory.BackgroundBatchLimits) (memory.BackgroundBatch, error)
	MemoryBackgroundPool(context.Context, memory.ScopeContext) (memory.BackgroundPool, error)
	PrepareRememberLiteral(context.Context, memory.ScopeContext, memory.RememberLiteralRequest) (memory.RememberLiteralProposal, error)
	PrepareCorrectClaim(context.Context, memory.ScopeContext, memory.CorrectClaimRequest) (memory.CorrectClaimProposal, error)
	PrepareMemoryLifecycle(context.Context, memory.ScopeContext, memory.MemoryLifecycleRequest) (memory.MemoryLifecycleProposal, error)
	AcquireTurnLease(context.Context, memory.SessionID, memory.LeaseHolderID, time.Duration) (memory.TurnLease, error)
	ReleaseTurnLease(context.Context, memory.SessionID, memory.LeaseHolderID, memory.FencingToken) error
	CommitMemoryBackgroundPass(context.Context, memory.TurnLease, memory.BackgroundCommit) (memory.BackgroundCommitResult, error)
	FailMemoryBackgroundPass(context.Context, memory.BackgroundFailure) error
	MemoryBackgroundSpend(context.Context, time.Time) (memory.BackgroundSpend, error)
	MemoryBackgroundProgress(context.Context, time.Time, int) (memory.BackgroundProgress, error)
}

// Searcher is the optional full-text and dense retriever. When the store
// provides it, its accepted-memory results raise a candidate's similarity
// to those Claims; when it is unavailable, lexical comparison stands alone.
type Searcher interface {
	SearchMemory(context.Context, memory.ScopeContext, memory.RetrievalQuery) (memory.RetrievalResult, error)
}

// Model is one structured-output completion. Implementations must not
// retry internally; the pass owns retry, budget and backoff.
type Model interface {
	Identity() string
	Complete(context.Context, ModelRequest) (ModelResponse, error)
}

// ModelResponse is the model's text and the tokens the provider reported.
// UsageReported false means the counters are missing, so the pass charges
// its estimate instead.
type ModelResponse struct {
	Text          string
	InputTokens   int64
	OutputTokens  int64
	UsageReported bool
}

// ProviderError classifies a failed call. Retryable failures proved nothing
// was generated (no response, 429, 502, 503, 504), the Stage 2 provider
// robustness rule; MayHaveBilled is false only when no generation started.
type ProviderError struct {
	Err           error
	Retryable     bool
	RetryAfter    time.Duration
	MayHaveBilled bool
}

func (e *ProviderError) Error() string { return e.Err.Error() }
func (e *ProviderError) Unwrap() error { return e.Err }

// Runner runs the background pass. One process runs at most one pass at a
// time, and cooperating processes share one durable runner lease.
type Runner struct {
	store   Store
	model   Model
	config  Config
	pricing Pricing
	holder  string

	now   func() time.Time
	sleep func(context.Context, time.Duration) error
	newID func() string
	logf  func(string, ...any)

	mu        sync.Mutex
	activated bool
	pricingMu sync.RWMutex
	// flush wakes Run early when a compaction asks for a flush (Stage 10).
	flush chan struct{}
}

// Option adjusts a Runner, for tests and hosts.
type Option func(*Runner)

func WithClock(now func() time.Time) Option { return func(r *Runner) { r.now = now } }
func WithSleep(sleep func(context.Context, time.Duration) error) Option {
	return func(r *Runner) { r.sleep = sleep }
}
func WithLogger(logf func(string, ...any)) Option { return func(r *Runner) { r.logf = logf } }
func WithPricing(pricing Pricing) Option          { return func(r *Runner) { r.pricing = pricing } }

// NewRunner builds a runner. A disabled configuration, or a nil model, makes
// a runner that only reports status.
func NewRunner(store Store, model Model, config Config, options ...Option) *Runner {
	r := &Runner{store: store, model: model, config: config, pricing: FallbackPricing, holder: "memory-background:" + uuid.NewString(),
		now: time.Now, sleep: sleepContext, newID: uuid.NewString, logf: func(string, ...any) {}, flush: make(chan struct{}, 1)}
	if config.Provider == ProviderLocal {
		r.pricing = Pricing{Source: "local"}
	}
	for _, option := range options {
		option(r)
	}
	if model == nil && r.config.Enabled {
		r.config.Enabled, r.config.Reason = false, "no model is available"
	}
	return r
}

func (r *Runner) Config() Config { return r.config }

// UsePricing replaces the price the USD cap is measured with, for example
// once the host has read the model's catalog price.
func (r *Runner) UsePricing(pricing Pricing) {
	r.pricingMu.Lock()
	defer r.pricingMu.Unlock()
	r.pricing = pricing
}

func (r *Runner) currentPricing() Pricing {
	r.pricingMu.RLock()
	defer r.pricingMu.RUnlock()
	return r.pricing
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runnerLeaseTTL covers a whole pass: two calls with retries plus the
// commit wait. A crashed process's lease expires within it.
func (r *Runner) runnerLeaseTTL() time.Duration {
	return time.Duration(2*(1+r.config.ProviderRetries))*r.config.CallTimeout + time.Duration(r.config.CommitAttempts)*r.config.CommitWait + time.Minute
}

// Run activates the pass once and then ticks until ctx ends. Shutdown
// cancels any in-flight model call; nothing is committed after it, and the
// durable cursor lets the next process redo the batch.
func (r *Runner) Run(ctx context.Context) error {
	if !r.config.Enabled {
		<-ctx.Done()
		return ctx.Err()
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = r.store.ReleaseMemoryBackgroundRunner(releaseCtx, r.holder)
	}()
	for {
		if _, err := r.Tick(ctx); err != nil && ctx.Err() == nil {
			r.logf("memory background pass: %v", err)
		}
		if err := r.wait(ctx); err != nil {
			return err
		}
	}
}

// wait sleeps until the next tick, or less when a compaction asks for a
// flush. It returns an error only when ctx ends.
func (r *Runner) wait(ctx context.Context) error {
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-r.flush:
			cancel()
		case <-waitCtx.Done():
		}
	}()
	_ = r.sleep(waitCtx, r.config.Tick)
	return ctx.Err()
}

// RequestCompactionFlush is the memory flush before compaction (memory
// architecture Stage 10): a compaction of the conversation has committed a
// summary of its events through sequence through. It never blocks and never
// touches the store or a model; it only wakes a running pass so the flush
// starts on this tick rather than the next one.
//
// The request is a shortcut, not the guarantee. A committed compaction
// whose summarized owner messages lie past the conversation's cursor makes
// the conversation due by itself (DueMemoryBackgroundSessions, trigger
// compaction), so a lost request (a crash right after the commit, a process
// that runs no pass, another process holding the runner lease) is flushed
// by the next tick that runs anywhere. The flush keeps every rule of a pass:
// owner messages only, the live-turn fence, backoff, and the caps. A
// disabled pass ignores the request, and status reports the skipped flush.
func (r *Runner) RequestCompactionFlush(sessionID memory.SessionID, through int64) {
	if !r.config.Enabled || sessionID == "" || through < 1 {
		return
	}
	select {
	case r.flush <- struct{}{}:
	default:
	}
}

// TickResult says what one scheduling pass did.
type TickResult struct {
	Runs   []memory.BackgroundRun
	Paused bool
	Leased bool
}

// Tick runs the passes that are due now, up to SessionsPerTick, one at a
// time. It does nothing when another process holds the runner lease or
// today's cap is reached.
func (r *Runner) Tick(ctx context.Context) (TickResult, error) {
	var result TickResult
	if !r.config.Enabled {
		return result, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.activated {
		if _, err := r.store.EnsureMemoryBackgroundActivation(ctx); err != nil {
			return result, fmt.Errorf("activate: %w", err)
		}
		r.activated = true
	}
	leased, err := r.store.AcquireMemoryBackgroundRunner(ctx, r.holder, r.runnerLeaseTTL())
	if err != nil || !leased {
		return result, err
	}
	result.Leased = true
	// Paused once today's spend reaches a cap: no room for even one token.
	if paused, err := r.capExceeded(ctx, 1, 1); err != nil || paused {
		result.Paused = paused
		return result, err
	}
	due, err := r.store.DueMemoryBackgroundSessions(ctx, memory.BackgroundDueQuery{Now: r.now(), Idle: r.config.Idle, Turns: r.config.Turns, Limit: r.config.SessionsPerTick})
	if err != nil {
		return result, err
	}
	for _, item := range due {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		// Renew before each pass; a process that lost the lease to another
		// stops, so two processes never run paid passes at once.
		if leased, err := r.store.AcquireMemoryBackgroundRunner(ctx, r.holder, r.runnerLeaseTTL()); err != nil || !leased {
			result.Leased = leased
			return result, err
		}
		var err error
		if item.Trigger == memory.BackgroundTriggerCompaction {
			// An owed compaction flush covers everything through the
			// compaction's frontier, batch after batch.
			var runs []memory.BackgroundRun
			var leased bool
			runs, leased, err = r.passThrough(ctx, item.SessionID, item.Through)
			result.Runs = append(result.Runs, runs...)
			if !leased {
				result.Leased = false
				return result, err
			}
		} else {
			var run memory.BackgroundRun
			run, err = r.runSession(ctx, item.SessionID, item.Trigger, 0)
			if run.RunID != "" && !run.FinishedAt.IsZero() {
				result.Runs = append(result.Runs, run)
			}
		}
		if errors.Is(err, memory.ErrBackgroundDailyCap) {
			result.Paused = true
			return result, nil
		}
		if err != nil && ctx.Err() == nil {
			r.logf("memory background pass for %s: %v", item.SessionID, err)
		}
	}
	return result, nil
}

// PassThrough runs passes over one conversation's owner messages up to
// event sequence through at once, regardless of the idle and turn triggers
// and of failure backoff. The compaction flush (Stage 10) reaches it through
// Tick, which runs it for a conversation an owed flush makes due, so the
// flush also waits for a live turn and honours backoff. Each pass still
// respects the caps, the cursor and the live-turn fence at commit.
func (r *Runner) PassThrough(ctx context.Context, sessionID memory.SessionID, through int64) ([]memory.BackgroundRun, error) {
	if !r.config.Enabled {
		return nil, errors.New("the memory background pass is disabled: " + r.config.Reason)
	}
	if through < 1 {
		return nil, errors.New("a compaction flush needs the frontier it summarizes")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.activated {
		if _, err := r.store.EnsureMemoryBackgroundActivation(ctx); err != nil {
			return nil, fmt.Errorf("activate: %w", err)
		}
		r.activated = true
	}
	runs, leased, err := r.passThrough(ctx, sessionID, through)
	if !leased {
		return runs, errors.Join(errors.New("another process is running the background pass"), err)
	}
	return runs, err
}

// passThrough runs passes over one conversation's owner messages up to
// event sequence through, batch after batch, renewing the runner lease
// before each. The caller holds r.mu and has activated the pass. leased is
// false when another process holds the runner lease.
func (r *Runner) passThrough(ctx context.Context, sessionID memory.SessionID, through int64) (runs []memory.BackgroundRun, leased bool, err error) {
	for range 64 {
		if leased, err := r.store.AcquireMemoryBackgroundRunner(ctx, r.holder, r.runnerLeaseTTL()); err != nil || !leased {
			return runs, false, err
		}
		run, err := r.runSession(ctx, sessionID, memory.BackgroundTriggerCompaction, through)
		if run.RunID != "" && !run.FinishedAt.IsZero() {
			runs = append(runs, run)
		}
		if err != nil {
			return runs, true, err
		}
		if run.FinishedAt.IsZero() || run.ThroughSequence >= through || run.Status != memory.BackgroundRunSucceeded && run.Status != memory.BackgroundRunSkipped {
			return runs, true, nil
		}
	}
	return runs, true, nil
}

// errPassBudget marks a call that cannot fit the per-pass token budget.
var errPassBudget = errors.New("pass budget exceeded")

// commitLeaseDuration bounds how long a crashed commit could keep the
// conversation's turn lease. A healthy commit releases it within one short
// transaction.
const commitLeaseDuration = 10 * time.Second

// passSpend accumulates one pass's spend across its calls.
type passSpend struct {
	input, output, micro int64
}

func (s passSpend) tokens() int64 { return s.input + s.output }

// capExceeded reports whether today's recorded spend (since midnight UTC)
// plus extra would pass a daily cap. The USD cap binds only a priced model.
func (r *Runner) capExceeded(ctx context.Context, extraTokens, extraMicro int64) (bool, error) {
	now := r.now().UTC()
	today, err := r.store.MemoryBackgroundSpend(ctx, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		return false, err
	}
	pricing := r.currentPricing()
	priced := pricing.InputPerMillion > 0 || pricing.OutputPerMillion > 0
	return today.Tokens()+extraTokens > r.config.DailyTokens || priced && today.CostMicroUSD+extraMicro > r.config.DailyMicroUSD, nil
}

// call makes one model call inside the pass and daily budgets, retrying
// only failures that proved nothing was generated.
func (r *Runner) call(ctx context.Context, request ModelRequest, spend *passSpend) (string, error) {
	estimatedInput, estimatedOutput := request.EstimatedTokens()
	pricing := r.currentPricing()
	estimatedMicro := pricing.MicroUSD(estimatedInput, estimatedOutput)
	if spend.tokens()+estimatedInput+estimatedOutput > r.config.PassTokens {
		return "", fmt.Errorf("%w: the %s call needs about %d tokens, beyond the pass budget of %d", errPassBudget, request.Purpose, estimatedInput+estimatedOutput, r.config.PassTokens)
	}
	if capped, err := r.capExceeded(ctx, spend.tokens()+estimatedInput+estimatedOutput, spend.micro+estimatedMicro); err != nil {
		return "", err
	} else if capped {
		return "", memory.ErrBackgroundDailyCap
	}
	for attempt := 0; ; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, r.config.CallTimeout)
		response, err := r.model.Complete(callCtx, request)
		cancel()
		if err == nil {
			input, output := response.InputTokens, response.OutputTokens
			if !response.UsageReported {
				input, output = estimatedInput, estimatedOutput
			}
			spend.input, spend.output = spend.input+input, spend.output+output
			spend.micro += pricing.MicroUSD(input, output)
			return response.Text, nil
		}
		var provider *ProviderError
		if !errors.As(err, &provider) || provider.MayHaveBilled {
			spend.input, spend.output = spend.input+estimatedInput, spend.output+estimatedOutput
			spend.micro += estimatedMicro
		}
		if provider == nil || !provider.Retryable || attempt >= r.config.ProviderRetries || ctx.Err() != nil {
			return "", err
		}
		delay := max(r.config.ProviderRetryGap<<attempt, provider.RetryAfter)
		if delay > 30*time.Second {
			return "", err
		}
		if err := r.sleep(ctx, delay); err != nil {
			return "", err
		}
	}
}

// passOutcome is the per-pass bookkeeping that becomes the run record.
type passOutcome struct {
	candidates, noops, skipped int
}

// runSession runs one pass over one conversation: read the batch, extract,
// compare, decide, prepare, and commit under the turn lease. A run is
// recorded for every pass that reached the model or moved the cursor.
func (r *Runner) runSession(ctx context.Context, sessionID memory.SessionID, trigger memory.BackgroundTrigger, through int64) (memory.BackgroundRun, error) {
	run := memory.BackgroundRun{RunID: r.newID(), SessionID: sessionID, Trigger: trigger, Model: r.model.Identity() + "@" + PromptVersion, StartedAt: r.now()}
	batch, err := r.store.MemoryBackgroundBatch(ctx, sessionID, memory.BackgroundBatchLimits{MaxMessages: r.config.BatchMessages, MaxBytes: r.config.BatchBytes, Through: through})
	if err != nil {
		return run, err
	}
	run.FromSequence, run.ThroughSequence, run.Messages = batch.FromSequence, batch.ThroughSequence, len(batch.Messages)
	if batch.ThroughSequence == batch.FromSequence {
		return memory.BackgroundRun{}, nil
	}
	var spend passSpend
	var outcome passOutcome
	var proposals []memory.QueuedMemoryProposal
	if len(batch.Messages) > 0 {
		proposals, err = r.propose(ctx, batch, &spend, &outcome)
	}
	run.InputTokens, run.OutputTokens, run.CostMicroUSD = spend.input, spend.output, spend.micro
	run.Candidates, run.Noops, run.Skipped = outcome.candidates, outcome.noops, outcome.skipped
	if err != nil {
		return r.fail(ctx, run, batch, err)
	}
	return r.commit(ctx, run, batch, proposals)
}

// propose extracts candidates from the batch, compares each with the most
// similar existing memories, decides ADD / UPDATE / INVALIDATE / NOOP, and
// prepares every non-NOOP result through the ordinary Prepare paths.
func (r *Runner) propose(ctx context.Context, batch memory.BackgroundBatch, spend *passSpend, outcome *passOutcome) ([]memory.QueuedMemoryProposal, error) {
	scope := batch.Session.ScopeContext()
	pool, err := r.store.MemoryBackgroundPool(ctx, scope)
	if err != nil {
		return nil, err
	}
	request, err := extractRequest(batch, pool, r.config)
	if err != nil {
		return nil, err
	}
	raw, err := r.call(ctx, request, spend)
	if err != nil {
		return nil, err
	}
	candidates, dropped, err := parseExtraction(raw, batch, pool, r.config.CandidateLimit)
	if err != nil {
		return nil, err
	}
	outcome.candidates = len(candidates) + dropped
	outcome.skipped += dropped

	dense := r.denseMatches(ctx, scope, candidates)
	var decisions []resolvedDecision
	var comparisons []comparison
	var compared []int
	decisions = make([]resolvedDecision, len(candidates))
	for i, c := range candidates {
		duplicateInBatch := false
		for _, earlier := range candidates[:i] {
			duplicateInBatch = duplicateInBatch || sameMemory(earlier, c)
		}
		similar := rankSimilar(c, pool.Memories, dense[i], r.config.SimilarLimit)
		duplicate := exactDuplicate(c, similar)
		switch {
		case duplicateInBatch:
			decisions[i] = resolvedDecision{op: memory.BackgroundNoop}
		case duplicate != nil:
			decisions[i] = resolvedDecision{op: memory.BackgroundNoop, target: duplicate}
		case len(similar) == 0:
			decisions[i] = resolvedDecision{op: memory.BackgroundAdd}
		default:
			comparisons = append(comparisons, comparison{candidate: c, similar: similar})
			compared = append(compared, i)
		}
	}
	if len(comparisons) > 0 {
		request, err := decideRequest(comparisons, r.config)
		if err != nil {
			return nil, err
		}
		raw, err := r.call(ctx, request, spend)
		if err != nil {
			return nil, err
		}
		resolved, err := parseDecisions(raw, comparisons)
		if err != nil {
			return nil, err
		}
		for j, i := range compared {
			decisions[i] = resolved[j]
		}
	}

	var proposals []memory.QueuedMemoryProposal
	changed := map[memory.SemanticID]bool{}
	current := map[memory.SemanticID]bool{}
	for _, m := range pool.Memories {
		if m.ClaimID != "" {
			current[m.ClaimID] = true
		}
	}
	for i, c := range candidates {
		d := decisions[i]
		if d.op != memory.BackgroundNoop && d.target != nil {
			if d.target.Pending || !d.target.Writable || changed[d.target.ClaimID] {
				// A pending proposal is never changed, a Claim outside this
				// conversation's own scope cannot be corrected from it, and
				// one Claim gets at most one change per pass.
				if d.target.Pending {
					outcome.noops++
				} else {
					outcome.skipped++
				}
				continue
			}
			changed[d.target.ClaimID] = true
		}
		queued, noop, err := r.prepare(ctx, scope, c, d, current)
		switch {
		case err != nil:
			r.logf("memory background pass could not prepare a %s proposal: %v", d.op, err)
			outcome.skipped++
		case noop:
			outcome.noops++
		default:
			proposals = append(proposals, queued)
		}
	}
	return proposals, nil
}

// denseMatches asks the optional retriever which accepted Claims match each
// candidate's text. Retrieval failures only lose the boost.
func (r *Runner) denseMatches(ctx context.Context, scope memory.ScopeContext, candidates []candidate) []map[memory.SemanticID]bool {
	matches := make([]map[memory.SemanticID]bool, len(candidates))
	searcher, ok := r.store.(Searcher)
	for i, c := range candidates {
		matches[i] = map[memory.SemanticID]bool{}
		if !ok {
			continue
		}
		result, err := searcher.SearchMemory(ctx, scope, memory.RetrievalQuery{Text: c.Label + " " + c.Literal.Value, Limit: r.config.SimilarLimit})
		if err != nil {
			continue
		}
		for _, evidence := range result.Evidence {
			if evidence.ClaimID != "" {
				matches[i][evidence.ClaimID] = true
			}
		}
	}
	return matches
}

// prepare turns one decision into an exact prepared proposal through the
// same Prepare paths an approval card uses, so source binding to the exact
// owner span (or evie_proposed), kind, standing default and scope rules
// apply unchanged. noop is true when Prepare shows the memory already
// exists and is current; current holds the current Claims the pass read.
func (r *Runner) prepare(ctx context.Context, scope memory.ScopeContext, c candidate, d resolvedDecision, current map[memory.SemanticID]bool) (memory.QueuedMemoryProposal, bool, error) {
	queued := memory.QueuedMemoryProposal{Reason: memory.MemoryProposalBackground}
	switch d.op {
	case memory.BackgroundNoop:
		return queued, true, nil
	case memory.BackgroundAdd:
		proposal, err := r.store.PrepareRememberLiteral(ctx, scope, memory.RememberLiteralRequest{
			Destination: c.Destination, IdempotencyKey: idempotencyKey(scope.SessionID, c, d), SourceEventID: c.Message.EventID,
			Predicate: c.Predicate, PredicateLabel: c.Label, PredicateCardinality: c.Cardinality, Literal: c.Literal,
			Polarity: c.Polarity, Kind: c.Kind,
		})
		if err != nil {
			return queued, false, err
		}
		if !proposal.ClaimCreate {
			// The identical Claim exists. Current: nothing to add. Retired
			// or superseded: bringing it back is a restore the owner asks
			// for, not a new source quietly attached to it.
			if current[proposal.ClaimID] {
				return queued, true, nil
			}
			return queued, false, errors.New("the same memory exists but is retired or superseded; restoring it is an explicit owner action")
		}
		queued.ToolName, queued.Literal = memory.RememberLiteralToolName, &proposal
	case memory.BackgroundUpdate:
		target := d.target
		if target.Literal == nil || target.SubjectEntityID == "" || target.PredicateID == "" {
			return queued, false, errors.New("only a literal memory is corrected in the background")
		}
		replacement := memory.TypedLiteral{Kind: target.Literal.Kind, Value: c.Literal.Value}
		if !canonicalValue(replacement) {
			return queued, false, fmt.Errorf("the new value is not a canonical %s", replacement.Kind)
		}
		if normalizedValue(replacement.Value) == normalizedValue(target.Literal.Value) && c.Polarity == target.Polarity {
			return queued, true, nil
		}
		request := memory.CorrectClaimRequest{IdempotencyKey: idempotencyKey(scope.SessionID, c, d), SourceEventID: c.Message.EventID,
			OldClaimID: target.ClaimID, Mode: d.mode,
			Replacement: memory.ClaimProposition{SubjectEntityID: target.SubjectEntityID, PredicateID: target.PredicateID,
				Object: memory.ClaimObject{Literal: &replacement}, Polarity: c.Polarity}}
		if d.mode == memory.CorrectionChanged {
			effective := c.Message.RecordedAt.UTC()
			request.EffectiveTime = &effective
		}
		proposal, err := r.store.PrepareCorrectClaim(ctx, scope, request)
		if err != nil {
			return queued, false, err
		}
		queued.ToolName, queued.Correction = memory.CorrectClaimToolName, &proposal
	case memory.BackgroundInvalidate:
		proposal, err := r.store.PrepareMemoryLifecycle(ctx, scope, memory.MemoryLifecycleRequest{
			IdempotencyKey: idempotencyKey(scope.SessionID, c, d), SourceEventID: c.Message.EventID,
			Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: d.target.ClaimID,
		})
		if err != nil {
			return queued, false, err
		}
		queued.ToolName, queued.Lifecycle = memory.RetireToolName, &proposal
	default:
		return queued, false, fmt.Errorf("unsupported operation %q", d.op)
	}
	return queued, false, nil
}

// idempotencyKey is derived from what the proposal says and where it came
// from, so a pass that repeats after a crash proposes the identical change
// under the identical key, and the inbox reports it as a duplicate.
func idempotencyKey(sessionID memory.SessionID, c candidate, d resolvedDecision) string {
	target := ""
	if d.target != nil {
		target = string(d.target.ClaimID)
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{"evie-memory-background-v1", string(sessionID), string(c.Message.EventID), string(d.op),
		target, string(d.mode), c.Predicate, c.Label, string(c.Cardinality), string(c.Literal.Kind), c.Literal.Value, string(c.Polarity),
		string(c.Destination), string(c.Kind)}, "\x00")))
	var id uuid.UUID
	copy(id[:], digest[:16])
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	return "idem:v1:" + id.String()
}

// commit queues the proposals and advances the cursor under the
// conversation's turn lease. While a live turn holds the lease the commit
// waits briefly; if the turn outlasts the wait, the run is deferred and the
// batch runs again later (the cursor has not moved).
func (r *Runner) commit(ctx context.Context, run memory.BackgroundRun, batch memory.BackgroundBatch, proposals []memory.QueuedMemoryProposal) (memory.BackgroundRun, error) {
	holder := memory.LeaseHolderID(r.holder + ":" + run.RunID)
	var lease memory.TurnLease
	var err error
	for attempt := 0; attempt < r.config.CommitAttempts; attempt++ {
		lease, err = r.store.AcquireTurnLease(ctx, run.SessionID, holder, commitLeaseDuration)
		if err == nil || ctx.Err() != nil {
			break
		}
		if sleepErr := r.sleep(ctx, r.config.CommitWait); sleepErr != nil {
			err = sleepErr
			break
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return r.fail(ctx, run, batch, err)
		}
		run.Status = memory.BackgroundRunDeferred
		cause := fmt.Errorf("the conversation stayed busy: %w", err)
		recorded, recordErr := r.record(ctx, run, batch, cause, false)
		return recorded, errors.Join(cause, recordErr)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = r.store.ReleaseTurnLease(releaseCtx, lease.SessionID, lease.HolderID, lease.FencingToken)
	}()
	run.FinishedAt = r.now()
	result, err := r.store.CommitMemoryBackgroundPass(ctx, lease, memory.BackgroundCommit{Proposals: proposals, Run: run})
	if errors.Is(err, memory.ErrBackgroundCursorMoved) {
		return r.superseded(ctx, run)
	}
	if err != nil {
		if ctx.Err() != nil {
			return r.fail(ctx, run, batch, err)
		}
		run.Status = memory.BackgroundRunFailed
		recorded, recordErr := r.record(ctx, run, batch, err, false)
		return recorded, errors.Join(err, recordErr)
	}
	run.Status = memory.BackgroundRunSucceeded
	run.Queued = result.Queued
	run.Duplicates += result.Duplicates + result.PreviouslyDeclined + result.AlreadyApplied
	run.Skipped += result.Refused
	return run, nil
}

// fail records a pass that did not reach its commit.
func (r *Runner) fail(ctx context.Context, run memory.BackgroundRun, batch memory.BackgroundBatch, cause error) (memory.BackgroundRun, error) {
	switch {
	case ctx.Err() != nil:
		// Shutdown: keep the cursor; record spend already made so the
		// daily cap stays honest.
		if run.InputTokens+run.OutputTokens == 0 {
			return memory.BackgroundRun{}, ctx.Err()
		}
		run.Status = memory.BackgroundRunDeferred
		recorded, _ := r.record(context.WithoutCancel(ctx), run, batch, cause, false)
		return recorded, ctx.Err()
	case errors.Is(cause, memory.ErrBackgroundDailyCap) && run.InputTokens+run.OutputTokens == 0:
		return memory.BackgroundRun{}, cause
	case errors.Is(cause, memory.ErrBackgroundDailyCap):
		// The cap fell between the two calls: keep the cursor and the spend,
		// without counting a failure.
		run.Status = memory.BackgroundRunDeferred
		recorded, err := r.record(ctx, run, batch, cause, false)
		return recorded, errors.Join(cause, err)
	case errors.Is(cause, ErrMalformed) || errors.Is(cause, errPassBudget):
		// Output that fails validation, or a batch that cannot fit the pass
		// budget, will fail the same way again: count it toward skipping.
		run.Status, run.Malformed = memory.BackgroundRunFailed, true
		skip := batch.Malformed+1 >= r.config.MalformedSkip
		if skip {
			run.Status = memory.BackgroundRunSkipped
		}
		recorded, err := r.record(ctx, run, batch, cause, skip)
		return recorded, errors.Join(cause, err)
	default:
		run.Status = memory.BackgroundRunFailed
		recorded, err := r.record(ctx, run, batch, cause, false)
		return recorded, errors.Join(cause, err)
	}
}

func (r *Runner) record(ctx context.Context, run memory.BackgroundRun, batch memory.BackgroundBatch, cause error, skip bool) (memory.BackgroundRun, error) {
	run.FinishedAt = r.now()
	if cause != nil {
		run.Error = cause.Error()
	}
	next := run.FinishedAt.Add(r.config.BaseBackoff)
	if run.Status == memory.BackgroundRunFailed {
		backoff := r.config.BaseBackoff << min(batch.Failures, 16)
		next = run.FinishedAt.Add(min(backoff, r.config.MaxBackoff))
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := r.store.FailMemoryBackgroundPass(recordCtx, memory.BackgroundFailure{Run: run, NextAttempt: next, SkipBatch: skip})
	if errors.Is(err, memory.ErrBackgroundCursorMoved) {
		return r.superseded(ctx, run)
	}
	return run, err
}

// superseded records a pass whose batch another pass committed first. Its
// spend still counts toward today's caps; the cursor is left alone.
func (r *Runner) superseded(ctx context.Context, run memory.BackgroundRun) (memory.BackgroundRun, error) {
	if run.InputTokens+run.OutputTokens == 0 {
		return memory.BackgroundRun{}, nil
	}
	run.Status, run.FinishedAt, run.Error = memory.BackgroundRunSuperseded, r.now(), "another pass covered this batch first"
	run.Queued, run.Malformed = 0, false
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return run, r.store.FailMemoryBackgroundPass(recordCtx, memory.BackgroundFailure{Run: run})
}

// Status reports configuration and durable progress.
func (r *Runner) Status(ctx context.Context) (memory.BackgroundStatus, error) {
	status := memory.BackgroundStatus{Enabled: r.config.Enabled, State: memory.BackgroundStateOn, Reason: r.config.Reason,
		IdleMinutes: int(r.config.Idle / time.Minute), TurnTrigger: r.config.Turns, PassTokenBudget: r.config.PassTokens,
		DailyTokenCap: r.config.DailyTokens, DailyMicroUSDCap: r.config.DailyMicroUSD}
	pricing := r.currentPricing()
	if r.config.Enabled {
		status.Provider, status.Model, status.PricingSource = r.config.Provider, r.config.Model, pricing.Source
	} else {
		status.State = memory.BackgroundStateDisabled
	}
	progress, err := r.store.MemoryBackgroundProgress(ctx, r.now(), 20)
	if err != nil {
		return status, err
	}
	status.Progress = progress
	status.RemainingTokens = max(0, r.config.DailyTokens-progress.Today.Tokens())
	status.RemainingMicroUSD = max(0, r.config.DailyMicroUSD-progress.Today.CostMicroUSD)
	priced := pricing.InputPerMillion > 0 || pricing.OutputPerMillion > 0
	if r.config.Enabled && (status.RemainingTokens == 0 || status.RemainingMicroUSD == 0 && priced) {
		status.State, status.Reason = memory.BackgroundStatePaused, "today's cap is reached; the pass resumes after midnight UTC"
	}
	// Stage 10: a compaction never waits for its flush, so a flush the pass
	// cannot run now stays owed and is shown with the reason it is skipped.
	if progress.CompactionFlush.PendingOwnerMessages > 0 {
		switch status.State {
		case memory.BackgroundStateDisabled:
			status.CompactionFlushSkipped = "the pass is disabled; compactions proceed without the memory flush"
		case memory.BackgroundStatePaused:
			status.CompactionFlushSkipped = "today's cap is reached; the owed flush runs after midnight UTC"
		}
	}
	return status, nil
}
