package workflowruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/workflow"
)

// Capability effects (Stage 14 sub-stage 6, 2026-10-04; ADR 0008, 0009,
// 0010, 0015, 0017, 0027, 0029, 0043).
//
// Every effect follows the durable intent-call-receipt-checkpoint
// sequence. The run's input was accepted and checkpointed, and its
// definition, authority and composition pinned, when it started. Before
// each call the pinned provider must be ready (otherwise the run pauses
// before anything is recorded); then one fenced transaction applies
// per-node admission (run admission, the operation within the run's own
// approved Standing Authority, the connection and resource within the
// Workspace's current access), counts the run's Effect Intents of the
// operation against its max_calls_per_run, takes the effect's Resource
// Conflict Keys (another run holding one makes this run wait, visibly) and
// records the node attempt with its Effect Intent: the stable idempotency
// key every attempt of the node shares (derived from the run, the node and
// its deterministic input), the keys, the provider and the authority
// evaluation. The lease is checked once more immediately before the call.
// The provider's answer is recorded as the Effect Receipt with the
// normalized output and the next checkpoint in one transaction (a proven
// failure records a failed receipt and retries within the step's Retry
// Policy, sending the same intent identity again).
//
// Anything else leaves the outcome unknown: a call without an answer in
// time, an error that does not prove nothing happened, a panic, a response
// outside the contract, or a runtime that stopped between intent and
// receipt (the next claim records it). The run then needs reconciliation:
// no dependent node starts, nothing pauses or resumes it, and it is never
// retried by policy. A provider that reconciles is asked at once (and
// again later while it is unavailable or cannot answer): by lookup or by
// idempotent replay, it says whether the effect with the intent's key
// happened. Its answer, or the owner's (Reconcile, with evidence), becomes
// the intent's immutable receipt; then the run continues: an effect that
// happened supplies the step's output from its receipt without another
// call, and one that did not retries only within the Retry Policy.

var (
	// ErrReconcileRefused refuses an owner's reconciliation the run does
	// not need or whose outcome does not fit.
	ErrReconcileRefused = errors.New("the reconciliation is refused")
)

// Reconciliation is the owner reconciling an effect whose outcome is
// unknown: the run and step, whether the effect happened or did not
// happen, the evidence and, when it happened, its output (an object within
// the step's contract). Detach records it and returns at once; otherwise
// Reconcile waits until the run finishes or stops again.
type Reconciliation struct {
	RunID    string          `json:"run_id"`
	Step     string          `json:"step"`
	Outcome  string          `json:"outcome"`
	Evidence string          `json:"evidence,omitempty"`
	Output   json.RawMessage `json:"output,omitempty"`
	Detach   bool            `json:"detach,omitempty"`
	Surface  string          `json:"-"`
}

// Owner reconciliation outcomes.
const (
	OutcomeHappened     = "happened"
	OutcomeDidNotHappen = "did-not-happen"
	// MaxReconcileEvidence bounds the owner's evidence.
	MaxReconcileEvidence = 2000
	// maxEffectConflictKeys bounds the Resource Conflict Keys one request
	// touches.
	maxEffectConflictKeys = 16
	// maxProviderText bounds the provider's own text (errors, evidence)
	// quoted in a receipt, an unknown outcome or a run's error, which the
	// store caps at 4,000 bytes.
	maxProviderText = 1000
)

// clip bounds provider text to maxProviderText bytes, on a rune boundary.
func clip(text string) string {
	if len(text) <= maxProviderText {
		return text
	}
	cut := maxProviderText
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

// stepIntents lists the Effect Intents a step sent from one checkpoint.
func (r *Runtime) stepIntents(ctx context.Context, runID string, checkpoint int64, step string) ([]eviedb.WorkflowEffectIntent, error) {
	all, err := r.store.WorkflowEffectIntents(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := []eviedb.WorkflowEffectIntent{}
	for _, i := range all {
		if i.Checkpoint == checkpoint && i.Step == step {
			out = append(out, i)
		}
	}
	return out, nil
}

// idempotencyKey is the Effect Intent's stable identity, derived from the
// run's logical execution (its Workspace and Run Key, ADR 0021), the node
// and its deterministic input: every attempt and reconciliation of the
// node shares it, whichever provider implementation serves it, and so does
// a later run of the same logical execution (a run started again after a
// failure), so an idempotent provider never repeats an effect that already
// happened.
func idempotencyKey(run eviedb.WorkflowRun, step workflow.StepPlan, input map[string]any) (string, error) {
	data, err := workflow.EncodeValue(map[string]any{"workspace": string(run.WorkspaceID), "run_key": run.RunKey, "step": step.ID,
		"capability": step.Capability, "connection": step.Connection, "resource": step.Resource, "input": input})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// conflictKeys derives a request's Resource Conflict Keys, each scoped by
// its connection: the provider's own, or one per resource.
func conflictKeys(p plugins.WorkflowProvider, req plugins.WorkflowEffectRequest) (keys []string, err error) {
	derived := []string{req.Resource}
	if p.ConflictKeys != nil {
		defer func() {
			if v := recover(); v != nil {
				keys, err = nil, fmt.Errorf("the provider's Resource Conflict Keys panicked: %v", v)
			}
		}()
		if derived, err = p.ConflictKeys(req); err != nil {
			return nil, fmt.Errorf("the provider could not derive its Resource Conflict Keys: %v", err)
		}
	}
	if len(derived) == 0 || len(derived) > maxEffectConflictKeys {
		return nil, fmt.Errorf("the provider derived %d Resource Conflict Keys; an effect touches 1 to %d", len(derived), maxEffectConflictKeys)
	}
	for _, k := range derived {
		if k == "" || len(k) > 256 || strings.ContainsFunc(k, func(r rune) bool { return r < ' ' || r == 0x7f }) {
			return nil, fmt.Errorf("the provider derived an invalid Resource Conflict Key %q", k)
		}
		keys = append(keys, req.Connection+"/"+k)
	}
	slices.Sort(keys)
	return slices.Compact(keys), nil
}

// prepareEffect resolves the step's input, idempotency key and Resource
// Conflict Keys with its ready provider.
func prepareEffect(run eviedb.WorkflowRun, v *workflow.Validated, step workflow.StepPlan, inputs, state map[string]any, provider plugins.WorkflowProvider) (plugins.WorkflowEffectRequest, []string, error) {
	input, err := v.CapabilityInput(step.ID, inputs, state)
	if err != nil {
		return plugins.WorkflowEffectRequest{}, nil, err
	}
	key, err := idempotencyKey(run, step, input)
	if err != nil {
		return plugins.WorkflowEffectRequest{}, nil, err
	}
	req := plugins.WorkflowEffectRequest{Connection: step.Connection, Resource: step.Resource, Input: input, IdempotencyKey: key}
	keys, err := conflictKeys(provider, req)
	if err != nil {
		return req, nil, fmt.Errorf("steps.%s: %v", step.ID, err)
	}
	return req, keys, nil
}

// pendingConflict reports another run's unresolved intent holding a
// Resource Conflict Key the run's next effect would take, or nil.
func (r *Runtime) pendingConflict(ctx context.Context, p eviedb.WorkflowRunProgress, v *workflow.Validated, step workflow.StepPlan, provider plugins.WorkflowProvider) (*eviedb.WorkflowEffectConflictError, error) {
	inputs, inputsErr := decodeObject(p.Run.Inputs)
	state, stateErr := decodeObject(p.Checkpoint.State)
	if inputsErr != nil || stateErr != nil {
		// The step itself reports the problem when it runs.
		return nil, nil
	}
	_, keys, err := prepareEffect(p.Run, v, step, inputs, state, provider)
	if err != nil {
		return nil, nil
	}
	return r.store.WorkflowEffectConflict(ctx, p.Run.ID, keys)
}

// waitForConflict pauses the run, visibly, before an effect whose Resource
// Conflict Key another run's unresolved intent holds: rechecked at the
// scheduler's pace while that intent is in flight, and at the pause recheck
// while it waits for reconciliation.
func (r *Runtime) waitForConflict(ctx context.Context, lease eviedb.WorkflowRunLease, step workflow.StepPlan, held *eviedb.WorkflowEffectConflictError) error {
	recheck := r.sched.Poll
	if held.Unknown {
		recheck = r.sched.Recheck
	}
	reason := fmt.Sprintf("steps.%s waits: %s; effects sharing a Resource Conflict Key are serialized, and this one is sent once the key is free", step.ID, held.Error())
	_, err := r.store.PauseWorkflowRun(ctx, lease, reason, recheck)
	return err
}

// effect takes one step of a capability effect from the run's latest
// checkpoint: it continues from a reconciled receipt, or sends a new
// Effect Intent within the step's Retry Policy, the run's limits and its
// Standing Authority.
func (r *Runtime) effect(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, inputs, state map[string]any, intents []eviedb.WorkflowEffectIntent, fail, pause func(string) error) error {
	if n := len(intents); n > 0 {
		last := intents[n-1]
		switch last.State() {
		case eviedb.WorkflowEffectSucceeded:
			// Reconciled as happened: the receipt is the step's output.
			return r.settleFromReceipt(pc, p, step, inputs, state, last, fail)
		case eviedb.WorkflowEffectFailed:
			if n >= step.Attempts {
				return fail(fmt.Sprintf("steps.%s: Effect Intent #%d did not happen (%s); its Retry Policy allows no further attempt (%s)",
					step.ID, last.Seq, receiptSummary(last.Receipt), attempts(step.Attempts)))
			}
		default:
			// The store refuses this too: an unresolved intent is never
			// sent again by policy.
			return fmt.Errorf("%w: steps.%s Effect Intent #%d is %s", eviedb.ErrWorkflowEffectUnresolved, step.ID, last.Seq, last.State())
		}
	}
	limits := pc.v.Limits()
	switch {
	case p.Counted >= limits.Attempts:
		return fail(fmt.Sprintf("steps.%s: the run reached limits.max_attempts %d", step.ID, limits.Attempts))
	case r.elapsed(p) > limits.Duration:
		return fail(fmt.Sprintf("steps.%s: the run exceeded limits.max_duration %s", step.ID, limits.Duration))
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
	if provider.Perform == nil || !provider.Contract.Effect {
		return fail(fmt.Sprintf("steps.%s: %s %s does not perform %s as an effect", step.ID, provider.Plugin, provider.Version, step.Capability))
	}
	req, keys, err := prepareEffect(p.Run, pc.v, step, inputs, state, provider)
	if err != nil {
		return fail(err.Error())
	}
	encoded, err := workflow.EncodeValue(req.Input)
	if err != nil {
		return err
	}
	reconcile := eviedb.WorkflowEffectReconcileByOwner
	if provider.Reconcile != nil {
		reconcile = eviedb.WorkflowEffectReconcileByProvider
	}
	intent, err := r.store.BeginWorkflowEffect(pc.writes, pc.lease, eviedb.WorkflowEffectStart{
		Attempt:    eviedb.WorkflowAttemptStart{RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Made + 1, Input: encoded},
		Capability: step.Capability, Contract: pin.Contract, SchemaSHA256: pin.SchemaSHA256, Plugin: string(provider.Plugin), Version: provider.Version,
		Connection: step.Connection, Resource: step.Resource, IdempotencyKey: req.IdempotencyKey, Idempotent: provider.Contract.Idempotent,
		Reconcile: reconcile, ConflictKeys: keys,
	})
	var held *eviedb.WorkflowEffectConflictError
	switch {
	case errors.As(err, &held):
		return r.waitForConflict(pc.writes, pc.lease, step, held)
	case errors.Is(err, eviedb.ErrWorkflowEffectLimit):
		return fail(err.Error())
	case err != nil && nodeRefused(err):
		// Out of bounds: stopped before the provider is called.
		return pause(err.Error())
	case err != nil:
		return err
	}
	// The lease is checked immediately before the provider is called: a
	// holder that lost it calls nothing, and the next holder finds the
	// intent without a receipt.
	if pc.lost.Err() != nil {
		return fmt.Errorf("%w: lost before steps.%s was sent", eviedb.ErrWorkflowRunLeaseLost, step.ID)
	}
	if err := r.store.CheckWorkflowRunLease(pc.writes, pc.lease); err != nil {
		return err
	}
	answer := r.callEffect(pc, func(ctx context.Context) (any, error) { return provider.Perform(ctx, req) })
	if pc.lost.Err() != nil {
		// A stale holder records nothing; the next holder finds the
		// intent without a receipt, so its outcome is unknown.
		return fmt.Errorf("%w: lost while steps.%s was sent", eviedb.ErrWorkflowRunLeaseLost, step.ID)
	}
	return r.recordAnswer(pc, p, step, inputs, state, intent, intents, answer)
}

// callAnswer is what one effect or reconciliation call returned.
type callAnswer struct {
	out      any
	err      error
	panicked bool
	// late: the call ran past its time without an answer of its own.
	late bool
}

// callEffect calls a provider with the effect timeout. Its context ends
// when the run's lease is lost or the time is up, but not when the
// runtime stops: an effect in flight completes or ends unknown. After the
// time is up the provider still has a short grace to say whether it
// accepted the request; without an answer the outcome is unknown.
func (r *Runtime) callEffect(pc *pass, call func(ctx context.Context) (any, error)) callAnswer {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(pc.stop), r.sched.EffectTimeout)
	defer cancel()
	stopLost := context.AfterFunc(pc.lost, cancel)
	defer stopLost()
	done := make(chan callAnswer, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- callAnswer{err: fmt.Errorf("the provider panicked: %v", v), panicked: true}
			}
		}()
		out, err := call(ctx)
		done <- callAnswer{out: out, err: err}
	}()
	select {
	case a := <-done:
		return a
	case <-ctx.Done():
	}
	grace := time.NewTimer(min(r.sched.EffectTimeout, time.Second))
	defer grace.Stop()
	select {
	case a := <-done:
		// Within the grace only the provider's proof that nothing
		// happened, or its actual response, settles the outcome.
		if a.err != nil && !a.panicked && !errors.Is(a.err, plugins.ErrWorkflowEffectNotPerformed) {
			a.late = true
		}
		return a
	case <-grace.C:
		return callAnswer{err: ctx.Err(), late: true}
	}
}

// recordAnswer records how an effect call ended: its receipt with the
// output and next checkpoint, a proven failure with its retry or the run's
// failure, or an unknown outcome.
func (r *Runtime) recordAnswer(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, inputs, state map[string]any,
	intent eviedb.WorkflowEffectIntent, earlier []eviedb.WorkflowEffectIntent, a callAnswer) error {
	unknown := func(reason string, response []byte) error {
		// A request that timed out may still be landing: automatic
		// reconciliation waits one effect timeout before it asks.
		_, err := r.store.MarkWorkflowEffectUnknown(pc.writes, pc.lease, eviedb.WorkflowEffectUnknownMark{RunID: p.Run.ID, IntentSeq: intent.Seq,
			Reason: unknownReason(intent, reason), Response: response, RunError: unknownRunError(intent, unknownReason(intent, reason)),
			Settle: r.sched.EffectTimeout})
		if err == nil {
			pc.settling = true
		}
		return err
	}
	switch {
	case a.err != nil && !a.panicked && errors.Is(a.err, plugins.ErrWorkflowEffectNotPerformed):
		message := clip(a.err.Error())
		end := eviedb.WorkflowAttemptEnd{Error: message}
		if sent := len(earlier) + 1; sent >= step.Attempts {
			end.RunError = fmt.Sprintf("steps.%s failed after %s: %s", step.ID, attempts(step.Attempts), message)
		} else {
			end.RetryAfter = step.Backoff
		}
		_, err := r.store.FinishWorkflowEffect(pc.writes, pc.lease, eviedb.WorkflowEffectEnd{RunID: p.Run.ID, IntentSeq: intent.Seq,
			Outcome: eviedb.WorkflowEffectFailed, Error: message, Attempt: end})
		return err
	case a.late:
		return unknown(fmt.Sprintf("the provider did not answer within %s, after the request may have been accepted", r.sched.EffectTimeout), nil)
	case a.panicked:
		return unknown(clip(a.err.Error())+", after the request may have been sent", nil)
	case a.err != nil:
		message := clip(a.err.Error())
		if message == "" {
			message = "the provider failed without a message"
		}
		return unknown("the provider failed without proving that nothing happened: "+message, nil)
	}
	value, output, normErr := normalizeOutput(a.out)
	if normErr == nil {
		normErr = pc.v.CheckCapabilityOutput(step.ID, value)
	}
	if normErr != nil {
		var response []byte
		if data, err := json.Marshal(a.out); err == nil && len(data) <= maxReadOutputBytes {
			if v, err := workflow.DecodeValue(data); err == nil {
				response, _ = workflow.EncodeValue(v)
			}
		}
		return unknown("malformed response: "+clip(normErr.Error()), response)
	}
	end, err := successEnd(pc, p, step, intent.AttemptSeq, inputs, state, value, output)
	if err != nil {
		return err
	}
	_, err = r.store.FinishWorkflowEffect(pc.writes, pc.lease, eviedb.WorkflowEffectEnd{RunID: p.Run.ID, IntentSeq: intent.Seq,
		Outcome: eviedb.WorkflowEffectSucceeded, Response: output, Attempt: end})
	return err
}

func unknownReason(i eviedb.WorkflowEffectIntent, why string) string {
	return fmt.Sprintf("steps.%s: the outcome of Effect Intent #%d (%s on %s %s, idempotency key %s) is unknown: %s",
		i.Step, i.Seq, i.Capability, i.Connection, i.Resource, i.IdempotencyKey, why)
}

// unknownRunError is the run's error while an intent's outcome is unknown.
func unknownRunError(i eviedb.WorkflowEffectIntent, reason string) string {
	if i.Reconcile == eviedb.WorkflowEffectReconcileByProvider {
		return reason + "; it is never retried by policy: Evie reconciles it with the provider before the run continues"
	}
	return reason + "; it is never retried by policy, and the provider cannot reconcile it, so " + eviedb.ReconcileHint(i.RunID, i.Step)
}

// receiptSummary describes how a receipt was recorded.
func receiptSummary(rc *eviedb.WorkflowEffectReceipt) string {
	if rc == nil {
		return "no receipt"
	}
	text := "answered by the provider"
	switch rc.Source {
	case eviedb.WorkflowEffectFromReconcile:
		text = "found by the provider's reconciliation"
	case eviedb.WorkflowEffectFromOwner:
		text = "reconciled by the owner via " + rc.Surface
	}
	if rc.Evidence != "" {
		text += ": " + rc.Evidence
	} else if rc.Error != "" {
		text += ": " + rc.Error
	}
	return text
}

// settleFromReceipt continues a step whose effect was reconciled as having
// happened: an attempt that calls nothing records the receipt's response
// as the step's output with the next checkpoint.
func (r *Runtime) settleFromReceipt(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, inputs, state map[string]any, i eviedb.WorkflowEffectIntent, fail func(string) error) error {
	value, err := workflow.DecodeValue(i.Receipt.Response)
	if err == nil {
		err = pc.v.CheckCapabilityOutput(step.ID, value)
	}
	if err != nil {
		return fail(fmt.Sprintf("steps.%s: Effect Intent #%d's receipt does not fit the contract: %v", step.ID, i.Seq, err))
	}
	output, err := workflow.EncodeValue(value)
	if err != nil {
		return err
	}
	a, err := r.store.BeginWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptStart{
		RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Made + 1, Input: i.Input,
	})
	if err != nil {
		return err
	}
	end, err := successEnd(pc, p, step, a.Seq, inputs, state, value, output)
	if err != nil {
		return err
	}
	end.Note = fmt.Sprintf("Effect Intent #%d happened (%s); its receipt's response is the step's output, and the provider was not called again",
		i.Seq, receiptSummary(i.Receipt))
	_, err = r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
	return err
}

// unknownIntent is the run's latest intent whose outcome is unknown.
func unknownIntent(intents []eviedb.WorkflowEffectIntent) (eviedb.WorkflowEffectIntent, bool) {
	for i := len(intents) - 1; i >= 0; i-- {
		if intents[i].State() == eviedb.WorkflowEffectUnknown {
			return intents[i], true
		}
	}
	return eviedb.WorkflowEffectIntent{}, false
}

// continueReconciliation is a claim of a run that needs reconciliation:
// when automatic reconciliation is due and the run's pins still hold, it
// reconciles and continues; otherwise it records why automatic
// reconciliation waits, and the run never pauses or resumes meanwhile.
func (r *Runtime) continueReconciliation(ctx, writes context.Context, lease eviedb.WorkflowRunLease, lost context.Context, p eviedb.WorkflowRunProgress) (passEnd, error) {
	run := p.Run
	if run.NotBefore == nil || run.NotBefore.After(r.now()) {
		return passParked, nil
	}
	intents, err := r.store.WorkflowEffectIntents(writes, run.ID)
	if err != nil {
		return 0, err
	}
	i, ok := unknownIntent(intents)
	if !ok {
		return 0, fmt.Errorf("workflow run %s needs reconciliation but has no Effect Intent whose outcome is unknown", run.ID)
	}
	later := func(reason string) (passEnd, error) {
		_, err := r.store.DeferWorkflowReconciliation(writes, lease, r.deferred(i, reason, r.sched.Recheck), r.sched.Recheck)
		return passParked, err
	}
	comp, reason := r.incompatible(run.Composition)
	if reason != "" {
		return later(reason)
	}
	if reason, err := r.providersCompatible(writes, run.ID, comp); err != nil || reason != "" {
		if err != nil {
			return 0, err
		}
		return later(reason)
	}
	access, err := r.store.WorkspaceAccess(writes, run.WorkspaceID)
	if err != nil {
		return 0, err
	}
	v, err := r.approvedDefinition(writes, pinnedActivation(run), access.Access)
	if errors.Is(err, ErrDefinitionMismatch) {
		return later(err.Error())
	}
	if err != nil {
		return 0, err
	}
	return r.advance(&pass{stop: ctx, writes: writes, lost: lost, lease: lease, v: v, comp: comp})
}

// deferred is a run's error while automatic reconciliation waits: the
// unknown outcome, why it waits and, when it is tried again, when; when it
// is not (recheck 0), how the owner reconciles it.
func (r *Runtime) deferred(i eviedb.WorkflowEffectIntent, why string, recheck time.Duration) string {
	base := ""
	if i.Unknown != nil {
		base = i.Unknown.Reason + "; "
	}
	why = clip(why)
	if recheck > 0 {
		return fmt.Sprintf("%sautomatic reconciliation waits: %s; it is tried again after %s, or %s", base, why, recheck, eviedb.ReconcileHint(i.RunID, i.Step))
	}
	return fmt.Sprintf("%sautomatic reconciliation cannot settle it: %s; %s", base, why, eviedb.ReconcileHint(i.RunID, i.Step))
}

// reconcileAutomatically asks the provider what happened to the run's
// unknown Effect Intent, under per-node admission and the lease, and
// records its answer as the intent's receipt (resolved) or why it waits:
// tried again later while the provider is unavailable or fails to answer,
// and left to the owner when it cannot prove the outcome or is not
// admitted.
func (r *Runtime) reconcileAutomatically(pc *pass, p eviedb.WorkflowRunProgress) (bool, error) {
	intents, err := r.store.WorkflowEffectIntents(pc.writes, p.Run.ID)
	if err != nil {
		return false, err
	}
	i, ok := unknownIntent(intents)
	if !ok {
		return false, fmt.Errorf("workflow run %s needs reconciliation but has no Effect Intent whose outcome is unknown", p.Run.ID)
	}
	wait := func(why string, recheck time.Duration) (bool, error) {
		_, err := r.store.DeferWorkflowReconciliation(pc.writes, pc.lease, r.deferred(i, why, recheck), recheck)
		return false, err
	}
	if i.Reconcile != eviedb.WorkflowEffectReconcileByProvider {
		return wait("its provider does not reconcile", 0)
	}
	step, ok := pc.v.Step(i.Step)
	if !ok || step.Kind != workflow.KindCapability || !step.Effect {
		return false, fmt.Errorf("Effect Intent #%d names steps.%s, which is not an effect of the definition", i.Seq, i.Step)
	}
	pin, ok := pc.comp.pin(i.Capability)
	if !ok {
		return wait(fmt.Sprintf("capability %s is not pinned in the run's Execution Composition", i.Capability), 0)
	}
	provider, reason, err := r.checkPinned(pc.writes, p.Run.ID, pin, true)
	switch {
	case err != nil:
		return false, err
	case reason != "":
		return wait(reason, r.sched.Recheck)
	case provider.Reconcile == nil:
		return wait(fmt.Sprintf("%s %s no longer reconciles %s", provider.Plugin, provider.Version, i.Capability), 0)
	}
	// Reconciliation may replay the request, so it is admitted like the
	// effect itself, and sent only under the current lease.
	if err := r.store.AdmitWorkflowNode(pc.writes, pc.lease, eviedb.WorkflowNodeAdmission{
		Step: i.Step, Capability: i.Capability, Connection: i.Connection, Resource: i.Resource,
	}); err != nil {
		if nodeRefused(err) {
			return wait(err.Error(), 0)
		}
		return false, err
	}
	input, err := decodeObject(i.Input)
	if err != nil {
		return false, err
	}
	req := plugins.WorkflowEffectRequest{Connection: i.Connection, Resource: i.Resource, Input: input, IdempotencyKey: i.IdempotencyKey}
	if pc.lost.Err() != nil {
		return false, fmt.Errorf("%w: lost before steps.%s was reconciled", eviedb.ErrWorkflowRunLeaseLost, i.Step)
	}
	if err := r.store.CheckWorkflowRunLease(pc.writes, pc.lease); err != nil {
		return false, err
	}
	answer := r.callEffect(pc, func(ctx context.Context) (any, error) { return provider.Reconcile(ctx, req) })
	if pc.lost.Err() != nil {
		return false, fmt.Errorf("%w: lost while steps.%s was reconciled", eviedb.ErrWorkflowRunLeaseLost, i.Step)
	}
	switch {
	case answer.late:
		return wait(fmt.Sprintf("the provider's reconciliation did not answer within %s", r.sched.EffectTimeout), r.sched.Recheck)
	case answer.err != nil:
		return wait("the provider's reconciliation failed: "+clip(answer.err.Error()), r.sched.Recheck)
	}
	lookup, _ := answer.out.(plugins.WorkflowEffectLookup)
	resolution := eviedb.WorkflowEffectResolution{RunID: p.Run.ID, IntentSeq: i.Seq, Source: eviedb.WorkflowEffectFromReconcile, Evidence: clip(lookup.Evidence)}
	switch lookup.Outcome {
	case plugins.WorkflowEffectHappened:
		value, output, err := normalizeOutput(lookup.Output)
		if err == nil {
			err = pc.v.CheckCapabilityOutput(i.Step, value)
		}
		if err != nil {
			return wait(fmt.Sprintf("the provider's reconciliation found that it happened but answered outside the contract: %s", clip(err.Error())), 0)
		}
		resolution.Outcome, resolution.Response = eviedb.WorkflowEffectSucceeded, output
	case plugins.WorkflowEffectDidNotHappen:
		resolution.Outcome, resolution.Error = eviedb.WorkflowEffectFailed, "the provider's reconciliation found that it did not happen"
	default:
		why := "the provider's reconciliation cannot prove the outcome"
		if lookup.Evidence != "" {
			why += " (" + clip(lookup.Evidence) + ")"
		}
		return wait(why, 0)
	}
	if _, err := r.store.ResolveWorkflowEffect(pc.writes, pc.lease, resolution); err != nil {
		return false, err
	}
	return true, nil
}

// Reconcile records the owner's reconciliation of an effect whose outcome
// is unknown, with its evidence, as the intent's immutable receipt, and
// lets the run continue: from the receipt's output when it happened (the
// provider is not called again), or, when it did not, with a retry only
// within the step's Retry Policy. Only the owner's CLI and web surfaces
// reconcile. The run's lease is taken for the write, so a run another
// runtime is advancing is refused until it stops. Since sub-stage 7 it is
// the owner's answer to the effect's durable interruption: the same answer
// Answer records, written with the receipt in one transaction.
func (r *Runtime) Reconcile(ctx context.Context, req Reconciliation) (eviedb.WorkflowRunDetail, error) {
	return r.reconcile(ctx, req, "")
}

// reconcile records the owner's reconciliation, answered from session
// when one is named.
func (r *Runtime) reconcile(ctx context.Context, req Reconciliation, session memory.SessionID) (eviedb.WorkflowRunDetail, error) {
	if !ownerSurface(req.Surface) {
		return eviedb.WorkflowRunDetail{}, eviedb.ErrWorkflowApprover
	}
	refuse := func(format string, args ...any) (eviedb.WorkflowRunDetail, error) {
		return eviedb.WorkflowRunDetail{}, fmt.Errorf("%w: %s", ErrReconcileRefused, fmt.Sprintf(format, args...))
	}
	var outcome eviedb.WorkflowEffectState
	switch req.Outcome {
	case OutcomeHappened:
		outcome = eviedb.WorkflowEffectSucceeded
	case OutcomeDidNotHappen:
		outcome = eviedb.WorkflowEffectFailed
		if len(req.Output) > 0 {
			return refuse("an effect that did not happen has no output")
		}
	default:
		return refuse("the outcome is %s or %s, not %q", OutcomeHappened, OutcomeDidNotHappen, req.Outcome)
	}
	if len(req.Evidence) > MaxReconcileEvidence {
		return refuse("the evidence is longer than %d bytes", MaxReconcileEvidence)
	}
	d, err := r.store.WorkflowRun(ctx, req.RunID)
	if err != nil {
		return eviedb.WorkflowRunDetail{}, err
	}
	switch d.Run.State {
	case eviedb.WorkflowRunNeedsReconciliation:
	case eviedb.WorkflowRunRunning, eviedb.WorkflowRunWaitingForDependency:
		// A runtime may have stopped between an intent and its receipt
		// (and an older Evie may have paused the run); the claim below
		// records that outcome unknown.
	default:
		return refuse("run %s does not need reconciliation (it is %s)", req.RunID, d.Run.State)
	}
	writes := context.WithoutCancel(ctx)
	claim, err := r.store.AcquireWorkflowRunLease(writes, req.RunID, r.holder, r.sched.Lease)
	switch {
	case errors.Is(err, eviedb.ErrWorkflowRunLeaseHeld):
		return refuse("another runtime is advancing run %s now (%v); try again once it stops", req.RunID, err)
	case errors.Is(err, eviedb.ErrWorkflowRunFinished), errors.Is(err, eviedb.ErrWorkflowRunNotSchedulable), errors.Is(err, eviedb.ErrWorkflowRunNotDue):
		return refuse("%v", err)
	case err != nil:
		return eviedb.WorkflowRunDetail{}, err
	}
	resolveErr := r.reconcileClaimed(writes, claim.Lease, req, outcome, session)
	if err := r.store.ReleaseWorkflowRunLease(writes, claim.Lease, ""); err != nil && !errors.Is(err, eviedb.ErrWorkflowRunLeaseLost) {
		resolveErr = errors.Join(resolveErr, fmt.Errorf("release workflow run lease: %w", err))
	}
	if resolveErr != nil {
		return eviedb.WorkflowRunDetail{}, resolveErr
	}
	r.wake()
	if req.Detach {
		return r.store.WorkflowRun(writes, req.RunID)
	}
	return r.await(ctx, req.RunID)
}

// reconcileClaimed records the owner's reconciliation under the claim:
// the receipt, and the answer to the effect's interruption it closes.
func (r *Runtime) reconcileClaimed(ctx context.Context, lease eviedb.WorkflowRunLease, req Reconciliation, outcome eviedb.WorkflowEffectState, session memory.SessionID) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrReconcileRefused, fmt.Sprintf(format, args...))
	}
	d, err := r.store.WorkflowRun(ctx, req.RunID)
	if err != nil {
		return err
	}
	if d.Run.State != eviedb.WorkflowRunNeedsReconciliation {
		return refuse("run %s does not need reconciliation (it is %s)", req.RunID, d.Run.State)
	}
	var intent *eviedb.WorkflowEffectIntent
	for k := range d.Effects {
		if e := &d.Effects[k]; e.Step == req.Step && e.State() == eviedb.WorkflowEffectUnknown {
			intent = e
		}
	}
	if intent == nil {
		return refuse("steps.%s of run %s has no Effect Intent whose outcome is unknown", req.Step, req.RunID)
	}
	resolution := eviedb.WorkflowEffectResolution{RunID: req.RunID, IntentSeq: intent.Seq, Outcome: outcome, Source: eviedb.WorkflowEffectFromOwner,
		Evidence: req.Evidence, Surface: req.Surface, SessionID: session}
	answer := map[string]any{"outcome": req.Outcome}
	if req.Evidence != "" {
		answer["evidence"] = req.Evidence
	}
	if outcome == eviedb.WorkflowEffectFailed {
		resolution.Error = "the owner reconciled it as not having happened"
	} else {
		output := req.Output
		if len(output) == 0 {
			output = json.RawMessage(`{}`)
		}
		value, err := workflow.DecodeValue(output)
		if err != nil {
			return refuse("the output is not JSON: %v", err)
		}
		if _, ok := value.(map[string]any); !ok {
			return refuse("the output must be a JSON object")
		}
		if err := r.checkReconciledOutput(ctx, d.Run, *intent, value); err != nil {
			return refuse("the output does not fit %s %s: %v", intent.Capability, intent.Contract, err)
		}
		if resolution.Response, err = workflow.EncodeValue(value); err != nil {
			return err
		}
		answer["output"] = value
	}
	if resolution.Answer, err = workflow.EncodeValue(answer); err != nil {
		return err
	}
	_, err = r.store.ResolveWorkflowEffect(ctx, lease, resolution)
	return err
}

// checkReconciledOutput checks an owner's output against the step's
// contract: through the run's approved definition when it still loads,
// otherwise through the provider's contract when it is still the pinned
// one.
func (r *Runtime) checkReconciledOutput(ctx context.Context, run eviedb.WorkflowRun, i eviedb.WorkflowEffectIntent, value any) error {
	access, err := r.store.WorkspaceAccess(ctx, run.WorkspaceID)
	if err != nil {
		return err
	}
	if v, err := r.approvedDefinition(ctx, pinnedActivation(run), access.Access); err == nil {
		return v.CheckCapabilityOutput(i.Step, value)
	}
	p, err := r.providers.ResolveWorkflowCapability(ctx, i.Capability)
	if err != nil && !errors.Is(err, plugins.ErrWorkflowCapabilityUnavailable) {
		return fmt.Errorf("its contract cannot be read now: %v", err)
	}
	if p.SchemaSHA256 != i.SchemaSHA256 {
		return errors.New("its contract cannot be checked now: neither the approved definition nor the pinned provider is available")
	}
	return p.Contract.CheckOutput(value)
}

// effectsText renders a run's Effect Intents for the terminal.
func effectsText(b *strings.Builder, effects []eviedb.WorkflowEffectIntent) {
	if len(effects) == 0 {
		return
	}
	line := func(format string, args ...any) { fmt.Fprintf(b, format+"\n", args...) }
	line("Effects")
	for _, e := range effects {
		var authority struct {
			Calls int `json:"calls"`
			Max   int `json:"max_calls_per_run"`
		}
		_ = json.Unmarshal(e.Authority, &authority)
		line("  Intent #%d for steps.%s attempt %d (checkpoint %d): %s %s from %s %s on %s %s, key %s, conflict keys %s, call %d of %d allowed, reconciled by the %s",
			e.Seq, e.Step, e.Attempt, e.Checkpoint, e.Capability, e.Contract, e.Plugin, e.Version, e.Connection, e.Resource, e.IdempotencyKey,
			strings.Join(e.ConflictKeys, ", "), authority.Calls, authority.Max, e.Reconcile)
		line("     input %s", e.Input)
		if u := e.Unknown; u != nil {
			line("     outcome unknown at %s: %s", u.RecordedAt.Format(timeLayout), u.Reason)
			if len(u.Response) > 0 {
				line("     unrecognized response %s", u.Response)
			}
		}
		rc := e.Receipt
		if rc == nil {
			if e.Unknown == nil {
				line("     pending: the call is in flight")
			}
			continue
		}
		from := "the provider"
		switch rc.Source {
		case eviedb.WorkflowEffectFromReconcile:
			from = "the provider's reconciliation"
		case eviedb.WorkflowEffectFromOwner:
			from = "the owner via " + rc.Surface
		}
		text := fmt.Sprintf("     receipt: %s from %s at %s", rc.Outcome, from, rc.RecordedAt.Format(timeLayout))
		if rc.Evidence != "" {
			text += fmt.Sprintf("; evidence %q", rc.Evidence)
		}
		if rc.Error != "" {
			text += "; " + rc.Error
		}
		if len(rc.Response) > 0 {
			text += "; response " + string(rc.Response)
		}
		line("%s", text)
	}
}
