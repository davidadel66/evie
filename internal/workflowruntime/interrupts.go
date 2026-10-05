package workflowruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Durable interruptions (Stage 14 sub-stage 7, 2026-10-04; ADR 0009, 0029,
// 0044).
//
// An interrupt step records its typed question (its message, its declared
// input as the request and its answer's schema) under the fenced lease and
// the run waits for the owner (waiting_for_human), holding no lease: no
// scheduler claims it, and nothing pauses, resumes, fails or advances it
// meanwhile. The question carries the run's originating session, which
// lists it; any later session or surface of the owner answers it. An
// answer is validated against the step's schema (from the run's approved
// definition) before it is recorded, once, with its surface and session,
// under the answering surface's claim of the run's lease; the run then
// continues, in the foreground unless detached, and the answer becomes the
// step's output through an attempt that validates it again, so a human
// answer cannot bypass the schema. A proposal the owner corrects stays
// recorded: the answer is appended, never written over it (ADR 0044), and
// later deterministic steps revalidate it. Time spent waiting for an
// answer does not count toward limits.max_duration.
//
// An effect whose outcome is unknown is the same kind of durable
// interruption (effect_reconciliation): answering it with happened or
// did-not-happen, the evidence and, when it happened, its output is the
// owner's reconciliation, recorded with the Effect Receipt in one
// transaction; `evie workflows reconcile` records exactly that answer.

var (
	// ErrAnswerRefused refuses an answer the run is not waiting for or
	// whose value does not fit the step's schema.
	ErrAnswerRefused = errors.New("the answer is refused")
)

// AnswerRequest is the owner answering the interruption a run waits for at
// a step: a question's typed answer, or an unknown effect's
// reconciliation ({"outcome": "happened"|"did-not-happen", "evidence":
// "...", "output": {...}}). SessionID names the session answered from, if
// any; any session of the run's Workspace may answer. Detach records it
// and returns at once; otherwise Answer waits until the run finishes or
// stops again.
type AnswerRequest struct {
	RunID     string           `json:"run_id"`
	Step      string           `json:"step"`
	Answer    json.RawMessage  `json:"answer"`
	SessionID memory.SessionID `json:"session_id,omitempty"`
	Detach    bool             `json:"detach,omitempty"`
	Surface   string           `json:"-"`
}

// InterruptionFilter narrows a listing of interruptions: one Workspace,
// one originating session or one run; the open ones unless All.
type InterruptionFilter struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id,omitempty"`
	SessionID   memory.SessionID   `json:"session_id,omitempty"`
	RunID       string             `json:"run_id,omitempty"`
	All         bool               `json:"all,omitempty"`
	Limit       int                `json:"limit,omitempty"`
}

// Interruptions lists interruptions, by default the open ones: what the
// owner is asked now, for a Workspace or the session that started a run.
func (r *Runtime) Interruptions(ctx context.Context, f InterruptionFilter) ([]eviedb.WorkflowInterruption, error) {
	list, err := r.store.ListWorkflowInterruptions(ctx, eviedb.WorkflowInterruptionFilter{WorkspaceID: f.WorkspaceID, SessionID: f.SessionID, RunID: f.RunID, All: f.All, Limit: f.Limit})
	if err != nil {
		return nil, err
	}
	// A finished run's questions and answers are masked (sub-stage 12).
	return r.presentInterruptions(ctx, list)
}

// maxAnswerBytes bounds an answer document.
const maxAnswerBytes = 64 << 10

// answerRun records the owner's answer to the interruption the run waits for
// at the step and lets the run continue. Only the owner's CLI and web
// surfaces answer; no model-facing tool does.
func (r *Runtime) answerRun(ctx context.Context, req AnswerRequest) (eviedb.WorkflowRunDetail, error) {
	if !ownerSurface(req.Surface) {
		return eviedb.WorkflowRunDetail{}, eviedb.ErrWorkflowApprover
	}
	refuse := func(format string, args ...any) (eviedb.WorkflowRunDetail, error) {
		return eviedb.WorkflowRunDetail{}, fmt.Errorf("%w: %s", ErrAnswerRefused, fmt.Sprintf(format, args...))
	}
	if len(req.Answer) > maxAnswerBytes {
		return refuse("the answer is larger than %d KiB", maxAnswerBytes>>10)
	}
	d, err := r.store.WorkflowRun(ctx, req.RunID)
	if err != nil {
		return eviedb.WorkflowRunDetail{}, err
	}
	for _, e := range d.Effects {
		if e.Step == req.Step && e.State() == eviedb.WorkflowEffectUnknown {
			rec, err := parseEffectAnswer(req)
			if err != nil {
				return eviedb.WorkflowRunDetail{}, err
			}
			return r.reconcile(ctx, rec, req.SessionID)
		}
	}
	var q *eviedb.WorkflowInterruption
	for i := range d.Interruptions {
		if it := &d.Interruptions[i]; it.Kind == eviedb.WorkflowInterruptionQuestion && it.Step == req.Step && it.Open() {
			q = it
		}
	}
	if q == nil || d.Run.State != eviedb.WorkflowRunWaitingForHuman {
		return refuse("steps.%s of run %s waits for no answer (the run is %s)", req.Step, req.RunID, d.Run.State)
	}
	value, err := workflow.DecodeValue(req.Answer)
	if err != nil {
		return refuse("the answer is not one JSON object: %v", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return refuse("the answer is not one JSON object")
	}
	access, err := r.store.WorkspaceAccess(ctx, d.Run.WorkspaceID)
	if err != nil {
		return eviedb.WorkflowRunDetail{}, err
	}
	v, err := r.approvedDefinition(ctx, pinnedActivation(d.Run), access.Access)
	if err != nil {
		return refuse("the answer cannot be checked now: %v", err)
	}
	if err := v.CheckStepOutput(req.Step, value); err != nil {
		return refuse("the answer does not fit steps.%s's schema: %v", req.Step, err)
	}
	answer, err := workflow.EncodeValue(value)
	if err != nil {
		return eviedb.WorkflowRunDetail{}, err
	}
	writes := context.WithoutCancel(ctx)
	claim, err := r.store.AcquireWorkflowRunLease(writes, req.RunID, r.holder, r.sched.Lease)
	switch {
	case errors.Is(err, eviedb.ErrWorkflowRunLeaseHeld):
		return refuse("another runtime holds run %s now (%v); try again once it stops", req.RunID, err)
	case errors.Is(err, eviedb.ErrWorkflowRunFinished), errors.Is(err, eviedb.ErrWorkflowRunNotSchedulable), errors.Is(err, eviedb.ErrWorkflowRunNotDue):
		return refuse("%v", err)
	case err != nil:
		return eviedb.WorkflowRunDetail{}, err
	}
	_, answerErr := r.store.AnswerWorkflowQuestion(writes, claim.Lease, eviedb.WorkflowAnswer{RunID: req.RunID, Seq: q.Seq, Answer: answer, Surface: req.Surface, SessionID: req.SessionID})
	if errors.Is(answerErr, eviedb.ErrWorkflowInterruptionClosed) {
		answerErr = fmt.Errorf("%w: %v", ErrAnswerRefused, answerErr)
	}
	if err := r.store.ReleaseWorkflowRunLease(writes, claim.Lease, ""); err != nil && !errors.Is(err, eviedb.ErrWorkflowRunLeaseLost) {
		answerErr = errors.Join(answerErr, fmt.Errorf("release workflow run lease: %w", err))
	}
	if answerErr != nil {
		return eviedb.WorkflowRunDetail{}, answerErr
	}
	r.wake()
	if req.Detach {
		return r.store.WorkflowRun(writes, req.RunID)
	}
	return r.await(ctx, req.RunID)
}

// effectAnswerDoc is the owner's answer to an unknown effect.
type effectAnswerDoc struct {
	Outcome  string          `json:"outcome"`
	Evidence string          `json:"evidence,omitempty"`
	Output   json.RawMessage `json:"output,omitempty"`
}

// parseEffectAnswer reads the answer to an effect_reconciliation strictly.
func parseEffectAnswer(req AnswerRequest) (Reconciliation, error) {
	refuse := func(format string, args ...any) (Reconciliation, error) {
		return Reconciliation{}, fmt.Errorf("%w: %s", ErrReconcileRefused, fmt.Sprintf(format, args...))
	}
	dec := json.NewDecoder(bytes.NewReader(req.Answer))
	dec.DisallowUnknownFields()
	var doc effectAnswerDoc
	if err := dec.Decode(&doc); err != nil {
		return refuse(`the answer to an unknown effect is {"outcome": "happened" or "did-not-happen", "evidence": "...", "output": {...}}: %v`, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return refuse("the answer is one JSON object")
	}
	return Reconciliation{RunID: req.RunID, Step: req.Step, Outcome: doc.Outcome, Evidence: doc.Evidence, Output: doc.Output, Detach: req.Detach, Surface: req.Surface}, nil
}

// interrupt takes an interrupt step: it asks its question, or turns the
// owner's answer into the step's output after validating it again.
func (r *Runtime) interrupt(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, inputs, state map[string]any, fail func(string) error) error {
	q := p.Question
	if q == nil {
		if reason := r.limitReached(pc.v, p, step); reason != "" {
			return fail(reason)
		}
		spec, ok := pc.v.InterruptStep(step.ID)
		if !ok {
			return fail(fmt.Sprintf("steps.%s is not an interruption this runtime can ask", step.ID))
		}
		input, err := pc.v.StepInput(step.ID, inputs, state)
		if err != nil {
			return fail(err.Error())
		}
		request, err := workflow.EncodeValue(map[string]any{"input": input})
		if err != nil {
			return err
		}
		_, err = r.store.AskWorkflowQuestion(pc.writes, pc.lease, eviedb.WorkflowQuestion{RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID,
			Message: spec.Message, Request: request, ResponseSchema: spec.Schema})
		return err
	}
	res := q.Resolution
	if res == nil || res.Kind != eviedb.WorkflowInterruptionAnswered {
		return fmt.Errorf("%w: steps.%s interruption #%d has no answer", eviedb.ErrWorkflowRunConflict, step.ID, q.Seq)
	}
	value, err := workflow.DecodeValue(res.Answer)
	if err == nil {
		err = pc.v.CheckStepOutput(step.ID, value)
	}
	if err != nil {
		return fail(fmt.Sprintf("steps.%s: the owner's answer to interruption #%d does not fit the step's schema: %v", step.ID, q.Seq, err))
	}
	output, err := workflow.EncodeValue(value)
	if err != nil {
		return err
	}
	var request struct {
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(q.Request, &request); err != nil || len(request.Input) == 0 {
		request.Input = json.RawMessage(`{}`)
	}
	a, err := r.store.BeginWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptStart{
		RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Made + 1, Input: request.Input,
	})
	if err != nil {
		return err
	}
	end, err := successEnd(pc, p, step, a.Seq, inputs, state, value, output)
	if err != nil {
		return err
	}
	from := ""
	if res.SessionID != "" {
		from = " from session " + string(res.SessionID)
	}
	end.Note = fmt.Sprintf("interruption #%d answered by the owner via %s%s at %s; the answer passed the step's schema again", q.Seq, res.Surface, from, res.RecordedAt.Format(timeLayout))
	_, err = r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
	return err
}

// answerHint is how the owner answers an open interruption from the CLI.
func answerHint(i eviedb.WorkflowInterruption) string {
	if i.Kind == eviedb.WorkflowInterruptionEffect {
		return fmt.Sprintf(`evie workflows answer %s %s --answer '{"outcome":"happened|did-not-happen","evidence":"...","output":{...}}' (or evie workflows reconcile)`, i.RunID, i.Step)
	}
	return fmt.Sprintf("evie workflows answer %s %s --answer '<JSON object>'", i.RunID, i.Step)
}

// InterruptionText renders one interruption for the terminal.
func InterruptionText(i eviedb.WorkflowInterruption) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	if i.Open() {
		if i.Kind == eviedb.WorkflowInterruptionEffect {
			line("Waiting for you: did steps.%s's effect happen? Effect Intent #%d's outcome is unknown (interruption #%d, opened %s)", i.Step, i.IntentSeq, i.Seq, i.OpenedAt.Format(timeLayout))
		} else {
			line("Waiting for you: steps.%s asks: %s (interruption #%d, opened %s)", i.Step, i.Message, i.Seq, i.OpenedAt.Format(timeLayout))
		}
	} else {
		line("Interruption #%d (%s at steps.%s, checkpoint %d): %s", i.Seq, i.Kind, i.Step, i.Checkpoint, resolutionText(*i.Resolution))
	}
	line("     request %s", i.Request)
	if i.Open() {
		line("     answer schema %s", i.ResponseSchema)
		line("     answer with: %s", answerHint(i))
	}
	return b.String()
}

func resolutionText(res eviedb.WorkflowInterruptionResolution) string {
	var text string
	switch res.Kind {
	case eviedb.WorkflowInterruptionAnswered:
		text = "answered by the owner via " + res.Surface
		if res.SessionID != "" {
			text += " from session " + string(res.SessionID)
		}
	case eviedb.WorkflowInterruptionReconciled:
		text = "settled by the provider's reconciliation"
	default:
		text = "withdrawn"
	}
	text += " at " + res.RecordedAt.Format(timeLayout)
	if len(res.Answer) > 0 {
		text += ": " + string(res.Answer)
	}
	if res.Detail != "" {
		text += " (" + res.Detail + ")"
	}
	return text
}

// interruptionsText renders a run's interruptions.
func interruptionsText(b *strings.Builder, interruptions []eviedb.WorkflowInterruption) {
	if len(interruptions) == 0 {
		return
	}
	b.WriteString("Interruptions\n")
	for _, i := range interruptions {
		for _, l := range strings.Split(strings.TrimSuffix(InterruptionText(i), "\n"), "\n") {
			b.WriteString("  " + l + "\n")
		}
	}
}
