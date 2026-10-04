package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflow"
)

// Map steps (Stage 14 sub-stage 9, 2026-10-04; workflow-runtime.spec.md
// amendment, ADR 0017).
//
// A map step runs its item step once for each element of a list a prior
// step produced. A list longer than max_items fails the step before any
// item starts (recorded as the map's one failed attempt); nothing is
// truncated. Otherwise the map's own attempt is recorded and stays in
// flight while its items run: at most max_parallel at once, never more
// than the run's limits.max_parallel and, for agent items, the operator's
// worker caps. Items are started in index order, in batches that apply run
// admission once per batch, each an item attempt recorded under the run's
// lease before it runs and finished with its output (its durable
// boundary). An item retries within its own Retry Policy after its
// backoff; one interrupted as many times as the interruption bound fails.
// With on_item_failure: fail, the first item that fails after its retries
// stops the map: unstarted items never start, running ones are cancelled
// and recorded interrupted, and the run fails. With partial, the map
// succeeds when at least min_succeeded items succeeded. Its output lists
// the succeeded items in input order, then the failed ones, so it never
// depends on completion order; it is recorded with the next checkpoint only
// after every item ended.
//
// Every item attempt counts against limits.max_attempts, and every item's
// spend against limits.max_tokens: an item whose bound would pass it is
// not started, and when nothing else is running the run fails visibly.
// A map attempt that runs again from the same checkpoint (after a crash,
// a stop, a pause or a lost lease) takes its items' recorded outcomes and
// runs only what did not finish. The owner's cancellation, a stop and a
// lost lease reach every item in flight through its context; the run's
// cancellation finishes only after every item ended.

// mapItem is one element's progress at the map's checkpoint.
type mapItem struct {
	index       int
	element     any
	made        int
	counted     int
	interrupted int
	lastError   string
	readyAt     time.Time
	running     bool
	done        bool
	outcome     workflow.ItemOutcome
}

// itemResult is how one item attempt ended, as it is recorded.
type itemResult struct {
	item  *mapItem
	end   eviedb.WorkflowItemEnd
	value any
	lost  bool
	pause string
}

// mapRun is one map attempt in flight.
type mapRun struct {
	r       *Runtime
	pc      *pass
	p       eviedb.WorkflowRunProgress
	step    workflow.StepPlan
	spec    workflow.MapPlan
	inputs  map[string]any
	state   map[string]any
	list    []any
	items   []*mapItem
	attempt eviedb.WorkflowNodeAttempt
	agent   workflow.AgentPlan
	ai      workflow.AIStep
	// parallel is how many items run at once.
	parallel int
	ctx      context.Context
	cancel   context.CancelCauseFunc
	results  chan itemResult
	running  int
	// recheck, when set, is how long a pause waits before the run is
	// checked again: until the next UTC day for the daily cap
	// (sub-stage 11).
	recheck time.Duration
}

// Why a map stops starting items.
const (
	mapComplete  = ""
	mapCancelled = "cancelled"
	mapStopped   = "stopped"
	mapLost      = "lost"
	mapFailed    = "failed"
	mapLimit     = "limit"
	mapPaused    = "paused"
	mapError     = "error"
)

var (
	errItemFailedMap = errors.New("an item failed and the map stops at its first failed item")
	errMapCancelled  = errors.New("the owner cancelled the run")
	errMapStopped    = errors.New("the runtime stopped")
	errMapLeaseLost  = errors.New("the run's lease was lost")
)

// mapStep runs one attempt of a map step.
func (r *Runtime) mapStep(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, inputs, state map[string]any, fail, pause func(string) error) error {
	spec, ok := pc.v.MapStep(step.ID)
	if !ok {
		return fail(fmt.Sprintf("steps.%s is not a map step this runtime can run", step.ID))
	}
	limits := pc.v.Limits()
	if r.elapsed(p) > limits.Duration {
		return fail(fmt.Sprintf("steps.%s: the run exceeded limits.max_duration %s", step.ID, limits.Duration))
	}
	list, err := pc.v.MapList(step.ID, inputs, state)
	if err != nil {
		return fail(err.Error())
	}
	m := &mapRun{r: r, pc: pc, p: p, step: step, spec: spec, inputs: inputs, state: state, list: list, parallel: spec.Parallel, results: make(chan itemResult)}
	switch spec.Item.Kind {
	case workflow.KindAgent:
		if m.agent, ok = pc.v.AgentStep(step.ID); !ok {
			return fail(fmt.Sprintf("steps.%s runs an agent item this runtime cannot run", step.ID))
		}
		if r.agents == nil {
			return pause(r.agentsPause(step.ID, r.agentsOff))
		}
		if err := r.agents.RunWorkerAvailable(pc.stop, m.agent.Worker); err != nil {
			return pause(r.agentsPause(step.ID, fmt.Sprintf("its %s workers cannot run now: %v", m.agent.Worker, err)))
		}
		m.parallel = max(1, min(m.parallel, r.agents.RunCapacity()))
	case workflow.KindAI:
		if m.ai, ok = pc.v.ItemAIStep(step.ID); !ok {
			return fail(fmt.Sprintf("steps.%s runs an AI item this runtime cannot run", step.ID))
		}
		if r.models == nil {
			return pause(r.modelsPause(step.ID))
		}
	}
	encoded, err := workflow.EncodeValue(map[string]any{"items": list})
	if err != nil {
		return err
	}
	if len(list) > spec.MaxItems {
		// A longer list fails the step before any item starts; nothing is
		// truncated.
		a, err := r.store.BeginWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptStart{RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID,
			Kind: step.Kind, Attempt: p.Made + 1, Input: encoded})
		if err != nil {
			return err
		}
		reason := fmt.Sprintf("the list has %d items; max_items %d, so no item started", len(list), spec.MaxItems)
		_, err = r.store.FinishWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptEnd{RunID: p.Run.ID, Seq: a.Seq, Error: reason,
			RunError: fmt.Sprintf("steps.%s: %s", step.ID, reason)})
		return err
	}
	if err := m.load(); err != nil {
		return err
	}
	if m.attempt, err = r.store.BeginWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptStart{RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID,
		Kind: step.Kind, Attempt: p.Made + 1, Input: encoded}); err != nil {
		return err
	}
	return m.run()
}

// load reads what earlier attempts of this map at this checkpoint left:
// each item's outcome, or how many attempts it made, failed and was
// interrupted, and when its retry may start.
func (m *mapRun) load() error {
	m.items = make([]*mapItem, len(m.list))
	for i, element := range m.list {
		m.items[i] = &mapItem{index: i, element: element}
	}
	prior, err := m.r.store.WorkflowItemAttempts(m.pc.writes, m.p.Run.ID, m.p.Checkpoint.Seq, m.step.ID)
	if err != nil {
		return err
	}
	for _, a := range prior {
		if a.Item == nil || *a.Item >= len(m.items) {
			return fmt.Errorf("%w: steps.%s has an item attempt #%d outside its %d-item list", eviedb.ErrWorkflowRunConflict, m.step.ID, a.Seq, len(m.items))
		}
		it := m.items[*a.Item]
		it.made++
		switch a.State {
		case eviedb.WorkflowAttemptSucceeded:
			value, err := workflow.DecodeValue(a.Output)
			if err != nil {
				return fmt.Errorf("steps.%s item %d's recorded output cannot be read: %v", m.step.ID, it.index, err)
			}
			it.done, it.outcome = true, workflow.ItemOutcome{Index: it.index, Succeeded: true, Output: value}
		case eviedb.WorkflowAttemptFailed:
			it.counted++
			it.lastError = a.Error
			if a.FinishedAt != nil {
				it.readyAt = a.FinishedAt.Add(m.spec.Item.Backoff)
			}
		case eviedb.WorkflowAttemptInterrupted:
			it.interrupted++
			it.lastError = a.Error
		case eviedb.WorkflowAttemptRunning:
			return fmt.Errorf("%w: steps.%s item %d attempt %d is in flight under lease %d", eviedb.ErrWorkflowRunConflict, m.step.ID, it.index, a.Attempt, a.LeaseToken)
		}
	}
	for _, it := range m.items {
		m.settle(it)
	}
	return nil
}

// settle decides an item that spent its Retry Policy or its interruption
// bound.
func (m *mapRun) settle(it *mapItem) {
	switch {
	case it.done:
	case it.counted >= m.spec.Item.Attempts:
		it.done = true
		it.outcome = workflow.ItemOutcome{Index: it.index, Error: fmt.Sprintf("steps.%s.item failed after %s: %s", m.step.ID, attempts(m.spec.Item.Attempts), it.lastError)}
	case it.interrupted >= m.r.sched.Interruptions:
		it.done = true
		it.outcome = workflow.ItemOutcome{Index: it.index, Error: fmt.Sprintf("steps.%s.item was interrupted %s (each time its runtime crashed, stopped or lost its lease, or the map stopped it, before recording the outcome): %s",
			m.step.ID, times(it.interrupted), it.lastError)}
	}
}

// failedItem is the first item that failed for good, in index order.
func (m *mapRun) failedItem() *mapItem {
	for _, it := range m.items {
		if it.done && !it.outcome.Succeeded {
			return it
		}
	}
	return nil
}

// run starts items as slots free up and records each as it ends, until
// every item ended or the map stops starting them; then it records the
// map's own attempt.
func (m *mapRun) run() error {
	watch, stopWatch := m.r.cancellable(m.pc, m.pc.stop)
	defer stopWatch()
	m.ctx, m.cancel = context.WithCancelCause(context.Background())
	defer m.cancel(nil)
	stopWatching := context.AfterFunc(watch, func() {
		if m.pc.cancelled() {
			m.cancel(errMapCancelled)
		} else {
			m.cancel(errMapStopped)
		}
	})
	defer stopWatching()
	stopLost := context.AfterFunc(m.pc.lost, func() { m.cancel(errMapLeaseLost) })
	defer stopLost()
	stopping, reason := mapComplete, ""
	var failure error
	// stop stops starting items; with a cause it also stops the items in
	// flight (the owner's cancellation, a stop, a lost lease, the first
	// failed item), otherwise they finish (a pause, a limit, an error).
	stop := func(why, detail string, cause error) {
		if stopping == mapComplete {
			stopping, reason = why, detail
			if cause != nil {
				m.cancel(cause)
			}
		}
	}
	for {
		if stopping == mapComplete {
			switch {
			case m.pc.lost.Err() != nil:
				stop(mapLost, "", errMapLeaseLost)
			case m.pc.cancelled():
				stop(mapCancelled, "", errMapCancelled)
			case m.pc.stop.Err() != nil:
				stop(mapStopped, "", errMapStopped)
			case m.spec.OnItemFailure == "fail" && m.failedItem() != nil:
				stop(mapFailed, "", errItemFailedMap)
			}
		}
		if stopping == mapComplete {
			why, detail, err := m.startItems()
			var capped *eviedb.WorkflowDailyCapError
			switch {
			case err != nil && errors.As(err, &capped):
				// Today's cap refused the batch (sub-stage 11): the items
				// in flight finish, then the run pauses until tomorrow.
				m.recheck = m.r.untilReset(capped)
				stop(mapPaused, capped.Error(), nil)
			case err != nil && errors.Is(err, eviedb.ErrWorkflowRunCancelling):
				stop(mapCancelled, "", errMapCancelled)
			case err != nil && (errors.Is(err, eviedb.ErrWorkflowRunLeaseLost)):
				stop(mapLost, "", errMapLeaseLost)
			case err != nil && admissionRefused(err):
				stop(mapPaused, err.Error(), nil)
			case err != nil:
				failure = err
				stop(mapError, "", nil)
			case why == mapPaused:
				stop(mapPaused, detail, nil)
			case why == mapLimit && m.running == 0:
				stop(mapLimit, detail, nil)
			}
		}
		if m.running == 0 {
			if stopping != mapComplete || m.allDone() {
				break
			}
			// Only items waiting for their retry's backoff are left.
			if err := m.r.sleep(m.ctx, max(m.nextReady().Sub(m.r.now()), time.Millisecond)); err != nil && m.pc.lost.Err() == nil && !m.pc.cancelled() && m.pc.stop.Err() == nil {
				failure = err
				stop(mapError, "", nil)
			}
			continue
		}
		var timer *time.Timer
		var ready <-chan time.Time
		if stopping == mapComplete && m.running < m.parallel {
			if next := m.nextReady(); !next.IsZero() {
				timer = time.NewTimer(max(next.Sub(m.r.now()), time.Millisecond))
				ready = timer.C
			}
		}
		var signals <-chan struct{}
		if stopping == mapComplete {
			signals = m.ctx.Done()
		}
		select {
		case res := <-m.results:
			m.running--
			if err := m.record(res); err != nil {
				switch {
				case errors.Is(err, eviedb.ErrWorkflowRunLeaseLost):
					stop(mapLost, "", errMapLeaseLost)
				default:
					if failure == nil {
						failure = err
					}
					stop(mapError, "", nil)
				}
			} else if res.pause != "" {
				stop(mapPaused, res.pause, nil)
			}
		case <-signals:
		case <-ready:
		}
		if timer != nil {
			timer.Stop()
		}
	}
	return m.finish(stopping, reason, failure)
}

func (m *mapRun) allDone() bool {
	for _, it := range m.items {
		if !it.done {
			return false
		}
	}
	return true
}

// nextReady is the earliest time a waiting item may retry (zero when no
// item waits).
func (m *mapRun) nextReady() time.Time {
	var next time.Time
	for _, it := range m.items {
		if it.done || it.running || it.made == 0 && it.readyAt.IsZero() {
			continue
		}
		if next.IsZero() || it.readyAt.Before(next) {
			next = it.readyAt
		}
	}
	return next
}

// startItems starts every pending item that is due, in index order, while
// slots are free and the run's limits allow, in one batch. why is mapLimit
// (with the reason) when a limit held back the next item, or mapPaused
// when its workers cannot run now.
func (m *mapRun) startItems() (why, detail string, err error) {
	free := m.parallel - m.running
	if free <= 0 {
		return "", "", nil
	}
	now := m.r.now()
	var due []*mapItem
	for _, it := range m.items {
		if len(due) == free {
			break
		}
		if !it.done && !it.running && !it.readyAt.After(now) {
			due = append(due, it)
		}
	}
	if len(due) == 0 {
		return "", "", nil
	}
	if m.spec.Item.Kind == workflow.KindAgent {
		if err := m.r.agents.RunWorkerAvailable(m.pc.stop, m.agent.Worker); err != nil {
			return mapPaused, m.r.agentsPause(m.step.ID, fmt.Sprintf("its %s workers cannot run now: %v", m.agent.Worker, err)), nil
		}
	}
	spend, err := m.r.store.WorkflowRunSpend(m.pc.writes, m.p.Run.ID)
	if err != nil {
		return "", "", err
	}
	// Today's spend across all runs, for the operator's daily cap
	// (sub-stage 11); the store checks the batch again when it records it.
	var today eviedb.WorkflowDailySpend
	var reset time.Time
	if m.r.cap != nil {
		var day string
		day, reset = eviedb.WorkflowDay(now)
		if today, err = m.r.store.WorkflowDailySpend(m.pc.writes, day); err != nil {
			return "", "", err
		}
	}
	reservedMicro := int64(0)
	limits := m.pc.v.Limits()
	type prepared struct {
		it    *mapItem
		start eviedb.WorkflowItemStart
		input map[string]any
		plan  workerPlan
		req   ModelRequest
	}
	var batch []prepared
	reserved := int64(0)
	for _, it := range due {
		switch {
		case spend.Counted+len(batch) >= limits.Attempts:
			why, detail = mapLimit, fmt.Sprintf("steps.%s: the run reached limits.max_attempts %d", m.step.ID, limits.Attempts)
		case m.r.elapsed(m.p) > limits.Duration:
			why, detail = mapLimit, fmt.Sprintf("steps.%s: the run exceeded limits.max_duration %s", m.step.ID, limits.Duration)
		}
		if why != "" {
			break
		}
		input, err := m.pc.v.ItemInput(m.step.ID, m.inputs, m.state, it.element)
		if err != nil {
			return "", "", err
		}
		encoded, err := workflow.EncodeValue(input)
		if err != nil {
			return "", "", err
		}
		p := prepared{it: it, input: input, start: eviedb.WorkflowItemStart{Index: it.index, Kind: m.spec.Item.Kind, Attempt: it.made + 1, Input: encoded}}
		estimate, estimateMicro := int64(0), int64(0)
		switch m.spec.Item.Kind {
		case workflow.KindAgent:
			if p.plan, err = m.r.planWorker(m.pc, m.p.Run.ID, m.p.Checkpoint.Seq, m.step.ID, &it.index, m.agent.Worker); err != nil {
				return "", "", err
			}
			w := m.r.workerCallStart(m.pc.stop, m.agent, p.plan)
			if p.plan.adopt == nil {
				estimate, estimateMicro = m.agent.Tokens, w.EstimatedMicroUSD
			}
			p.start.Worker = &w
		case workflow.KindAI:
			if p.req, err = aiRequest(m.ai, input); err != nil {
				return "", "", err
			}
			estIn, estOut := estimateTokens(p.req)
			estimate = estIn + estOut
			pricing := m.r.models.Pricing(m.pc.stop, m.ai.Policy.Model)
			params, err := policyParameters(m.ai.Policy)
			if err != nil {
				return "", "", err
			}
			digest, err := requestDigest(p.req)
			if err != nil {
				return "", "", err
			}
			estimateMicro = pricing.MicroUSD(estIn, estOut)
			p.start.Model = &eviedb.WorkflowModelCallStart{Policy: m.ai.Policy.Name, PolicyVersion: m.ai.Policy.Version, Provider: m.ai.Policy.Provider, Model: m.ai.Policy.Model,
				Parameters: params, PromptSHA256: m.ai.PromptSHA256, SchemaSHA256: m.ai.SchemaSHA256, RequestSHA256: digest,
				EstimatedInputTokens: estIn, EstimatedOutputTokens: estOut, EstimatedMicroUSD: estimateMicro, Cap: m.r.cap}
		}
		if spend.Tokens+reserved+estimate > limits.Tokens || estimate == 0 && spend.Tokens > limits.Tokens {
			why, detail = mapLimit, fmt.Sprintf("steps.%s item %d: its next attempt could use up to %s tokens, and the run has used %s of limits.max_tokens %s; it was not started",
				m.step.ID, it.index, grouped(estimate), grouped(spend.Tokens+reserved), grouped(limits.Tokens))
			break
		}
		if c := m.r.cap; c != nil && estimate > 0 && (today.Tokens+reserved+estimate > c.Tokens || today.MicroUSD+reservedMicro+estimateMicro > c.MicroUSD) {
			// The operator's daily cap holds the item back (sub-stage 11):
			// the items started finish, then the run pauses until tomorrow.
			held := today
			held.Tokens, held.MicroUSD = today.Tokens+reserved, today.MicroUSD+reservedMicro
			capped := &eviedb.WorkflowDailyCapError{Day: today.Day, Spent: held, Cap: *c, NeedTokens: estimate, NeedMicroUSD: estimateMicro, ResetAt: reset}
			m.recheck = m.r.untilReset(capped)
			why, detail = mapPaused, capped.Error()
			break
		}
		reserved += estimate
		reservedMicro += estimateMicro
		batch = append(batch, p)
	}
	if len(batch) == 0 {
		return why, detail, nil
	}
	starts := make([]eviedb.WorkflowItemStart, len(batch))
	for i, p := range batch {
		starts[i] = p.start
	}
	attempts, err := m.r.store.BeginWorkflowItemAttempts(m.pc.writes, m.pc.lease, m.attempt.Seq, starts)
	if err != nil {
		return "", "", err
	}
	for i, a := range attempts {
		p := batch[i]
		p.it.made++
		p.it.running = true
		m.running++
		go func(it *mapItem, a eviedb.WorkflowNodeAttempt, input map[string]any, plan workerPlan, req ModelRequest) {
			m.results <- m.runItem(it, a, input, plan, req)
		}(p.it, a, p.input, p.plan, p.req)
	}
	return why, detail, nil
}

// runItem runs one item attempt and says how it ended.
func (m *mapRun) runItem(it *mapItem, a eviedb.WorkflowNodeAttempt, input map[string]any, plan workerPlan, req ModelRequest) itemResult {
	res := itemResult{item: it, end: eviedb.WorkflowItemEnd{RunID: a.RunID, Seq: a.Seq}}
	interrupted := func(format string, args ...any) itemResult {
		res.end.Interrupted, res.end.Error = true, "interrupted: "+fmt.Sprintf(format, args...)
		return res
	}
	succeeded := func(value any, output []byte, note string) itemResult {
		res.value, res.end.Output, res.end.Note = value, output, note
		return res
	}
	switch m.spec.Item.Kind {
	case workflow.KindDeterministic:
		var out any
		var callErr error
		if m.r.call != nil {
			out, callErr = m.r.call(a.RunID, m.pc.v, m.step.ID, input)
		} else {
			out, callErr = m.pc.v.CallItemFunction(m.step.ID, input)
		}
		if callErr == nil {
			output, err := workflow.EncodeValue(out)
			if err != nil {
				callErr = fmt.Errorf("its output cannot be recorded: %v", err)
			} else {
				return succeeded(out, output, "")
			}
		}
		res.end.Error = callErr.Error()
		if res.end.Error == "" {
			res.end.Error = "the function failed without a message"
		}
		return res
	case workflow.KindAI:
		estIn, estOut := estimateTokens(req)
		pricing := m.r.models.Pricing(m.pc.stop, m.ai.Policy.Model)
		if m.pc.lost.Err() != nil {
			res.lost = true
			return res
		}
		answer := m.r.callModel(m.pc, m.ctx, req, estIn, estOut)
		if m.pc.lost.Err() != nil {
			res.lost = true
			return res
		}
		res.end.Model = &eviedb.WorkflowModelCallEnd{Requests: answer.requests, InputTokens: answer.in, OutputTokens: answer.out, UsageReported: answer.reported,
			MicroUSD: pricing.MicroUSD(answer.in, answer.out), PricingSource: pricing.Source, Response: clipModelText(answer.text)}
		switch {
		case answer.cancelled:
			return interrupted("the owner cancelled the run during the model call, which was stopped; the call may have been billed and counts at its estimate")
		case answer.stopped:
			return interrupted("the runtime stopped during the model call; the item runs again, and the call may have been billed")
		case answer.err != nil && m.ctx.Err() != nil:
			res.end.Model.Error = clip(answer.err.Error())
			return interrupted("%v during the model call, which was stopped", context.Cause(m.ctx))
		case answer.err != nil:
			res.end.Model.Error = clip(answer.err.Error())
			res.end.Error = "the model request failed: " + res.end.Model.Error
			return res
		}
		value, output, failure := parseProposalWith(func(v any) error { return m.pc.v.CheckItemOutput(m.step.ID, v) }, answer.text)
		if failure != nil {
			res.end.Error = failure.Error()
			return res
		}
		return succeeded(value, output, "")
	case workflow.KindAgent:
		o := m.r.runAgent(m.pc, m.ctx, m.p.Run, a, m.agent, input, plan)
		if o.lost {
			res.lost = true
			return res
		}
		end := o.end
		res.end.Worker = &end
		switch {
		case o.interrupted != "":
			res.end.Interrupted, res.end.Error, res.pause = true, o.interrupted, o.pause
			return res
		case o.failure != nil:
			res.end.Error, res.end.Note = o.failure.Error(), o.note
			return res
		}
		return succeeded(o.value, o.output, o.note)
	}
	res.end.Error = fmt.Sprintf("a map item is a deterministic, AI or agent step, not %s", m.spec.Item.Kind)
	return res
}

// record records how an item attempt ended and decides the item.
func (m *mapRun) record(res itemResult) error {
	it := res.item
	it.running = false
	if res.lost {
		return fmt.Errorf("%w: lost while steps.%s item %d ran", eviedb.ErrWorkflowRunLeaseLost, m.step.ID, it.index)
	}
	// An attempt the map itself stopped (an item failed with
	// on_item_failure: fail) did not fail on its own: it is recorded
	// interrupted.
	if !res.end.Interrupted && res.end.Error != "" && errors.Is(context.Cause(m.ctx), errItemFailedMap) {
		res.end.Interrupted = true
		res.end.Error = "interrupted: an item failed and the map stops at its first failed item (" + res.end.Error + ")"
	}
	if err := m.r.store.FinishWorkflowItemAttempt(m.pc.writes, m.pc.lease, res.end); err != nil {
		return err
	}
	switch {
	case res.end.Interrupted:
		it.interrupted++
		it.lastError = res.end.Error
	case res.end.Error != "":
		it.counted++
		it.lastError = res.end.Error
		it.readyAt = m.r.now().Add(m.spec.Item.Backoff)
	default:
		it.done, it.outcome = true, workflow.ItemOutcome{Index: it.index, Succeeded: true, Output: res.value}
	}
	m.settle(it)
	return nil
}

// finish records the map's own attempt once no item is in flight: its
// output with the next checkpoint, its failure (the first failed item, too
// few succeeded, a limit), or its interruption (the owner's cancellation,
// a stop, a pause), or nothing when the lease was lost.
func (m *mapRun) finish(stopping, reason string, failure error) error {
	pc, p, step := m.pc, m.p, m.step
	counts := m.counts()
	end := eviedb.WorkflowAttemptEnd{RunID: p.Run.ID, Seq: m.attempt.Seq}
	if m.allDone() && (stopping == mapCancelled || stopping == mapStopped || stopping == mapPaused) {
		// Every item ended anyway: the map's work all happened, so its
		// output is recorded; a cancellation or pause takes effect at the
		// next node boundary.
		stopping = mapComplete
	}
	switch stopping {
	case mapLost:
		return fmt.Errorf("%w: lost during steps.%s", eviedb.ErrWorkflowRunLeaseLost, step.ID)
	case mapError:
		return failure
	case mapCancelled:
		return m.r.store.InterruptWorkflowAttempt(pc.writes, pc.lease, m.attempt.Seq,
			"interrupted: the owner cancelled the run during the map ("+counts+"); no further item starts")
	case mapStopped:
		return m.r.store.InterruptWorkflowAttempt(pc.writes, pc.lease, m.attempt.Seq,
			"interrupted: the runtime stopped during the map ("+counts+"); it continues from its finished items")
	case mapPaused:
		if err := m.r.store.InterruptWorkflowAttempt(pc.writes, pc.lease, m.attempt.Seq, "interrupted: "+reason+" ("+counts+")"); err != nil {
			return err
		}
		recheck := m.r.sched.Recheck
		if m.recheck > 0 {
			recheck = m.recheck
		}
		_, err := m.r.store.PauseWorkflowRun(pc.writes, pc.lease, reason, recheck)
		return err
	case mapLimit:
		end.Error, end.RunError = reason+" ("+counts+")", reason
		_, err := m.r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
		return err
	case mapFailed:
		it := m.failedItem()
		end.Error = fmt.Sprintf("item %d failed: %s (%s)", it.index, it.outcome.Error, counts)
		end.RunError = fmt.Sprintf("steps.%s: item %d failed: %s", step.ID, it.index, it.outcome.Error)
		_, err := m.r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
		return err
	}
	var outcomes []workflow.ItemOutcome
	succeeded := 0
	for _, it := range m.items {
		outcomes = append(outcomes, it.outcome)
		if it.outcome.Succeeded {
			succeeded++
		}
	}
	value, err := pc.v.MapOutput(step.ID, m.list, outcomes)
	if err != nil {
		end.Error = err.Error()
		end.RunError = fmt.Sprintf("steps.%s: %v", step.ID, err)
		_, err := m.r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
		return err
	}
	if m.spec.OnItemFailure == "partial" && succeeded < m.spec.MinSucceeded {
		end.Error = fmt.Sprintf("%d of %d items succeeded; min_succeeded %d", succeeded, len(m.list), m.spec.MinSucceeded)
		end.RunError = fmt.Sprintf("steps.%s: %s", step.ID, end.Error)
		_, err := m.r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
		return err
	}
	output, err := workflow.EncodeValue(value)
	if err != nil {
		return err
	}
	if end, err = successEnd(pc, p, step, m.attempt.Seq, m.inputs, m.state, value, output); err != nil {
		return err
	}
	end.Note = fmt.Sprintf("%d of %d items succeeded, at most %d at once", succeeded, len(m.list), m.parallel)
	_, err = m.r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
	return err
}

// counts summarizes the items for a note.
func (m *mapRun) counts() string {
	var succeeded, failed, started, never int
	for _, it := range m.items {
		switch {
		case it.done && it.outcome.Succeeded:
			succeeded++
		case it.done:
			failed++
		case it.made > 0:
			started++
		default:
			never++
		}
	}
	parts := []string{fmt.Sprintf("%d of %d items succeeded", succeeded, len(m.items))}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if started > 0 {
		parts = append(parts, fmt.Sprintf("%d stopped before finishing", started))
	}
	if never > 0 {
		parts = append(parts, fmt.Sprintf("%d never started", never))
	}
	return strings.Join(parts, ", ")
}
