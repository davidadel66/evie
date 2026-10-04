package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Durable interruptions and AI model calls (Stage 14 sub-stage 7): a
// question waits for the owner without a lease and is answered once,
// immutably, with the answering surface and session; nothing resumes,
// pauses, fails or advances its run meanwhile. An effect whose outcome
// becomes unknown opens an effect_reconciliation interruption that its
// receipt closes in the same transaction. Each AI attempt records its
// model call before the call and its usage with the attempt's outcome.

const askSchema = `{"additionalProperties":false,"properties":{"expected":{"type":"boolean"},"note":{"maxLength":500,"type":"string"}},"required":["expected"],"type":"object"}`

// notesAtAsk starts a notes-digest run under a lease and advances it to
// its ask step (checkpoint 2).
func notesAtAsk(t *testing.T, s *Store) (WorkflowRun, WorkflowRunLease) {
	t.Helper()
	_, run := notesRun(t, s)
	claim, err := s.AcquireWorkflowRunLease(context.Background(), run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	advanceTo(t, s, claim.Lease, run.ID, []string{"read", "check"}, []string{"check", "ask"})
	return run, claim.Lease
}

func askQuestion(runID string) WorkflowQuestion {
	return WorkflowQuestion{RunID: runID, Checkpoint: 2, Step: "ask", Message: "No notes were found for this day. Reply whether that is expected.",
		Request: json.RawMessage(`{"input":{"day":"2026-10-05"}}`), ResponseSchema: json.RawMessage(askSchema)}
}

func interruptionsOf(t *testing.T, s *Store, runID string) []WorkflowInterruption {
	t.Helper()
	out, err := s.WorkflowInterruptions(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func listInterruptions(t *testing.T, s *Store, f WorkflowInterruptionFilter) []WorkflowInterruption {
	t.Helper()
	out, err := s.ListWorkflowInterruptions(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWorkflowQuestionWaitsForTheOwnerAndRecordsTheAnswerOnce(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	run, lease := notesAtAsk(t, r.s)
	if _, err := r.s.AskWorkflowQuestion(ctx, lease, WorkflowQuestion{RunID: run.ID, Checkpoint: 1, Step: "ask", Message: "x",
		Request: json.RawMessage(`{}`), ResponseSchema: json.RawMessage(askSchema)}); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("a question from a stale checkpoint: %v", err)
	}
	q, err := r.s.AskWorkflowQuestion(ctx, lease, askQuestion(run.ID))
	if err != nil {
		t.Fatal(err)
	}
	if q.Seq != 1 || q.Kind != WorkflowInterruptionQuestion || q.Step != "ask" || q.Checkpoint != 2 || q.Resolution != nil || q.WorkspaceID != run.WorkspaceID ||
		q.Workflow != "notes-digest" || q.LeaseToken != lease.Token || string(q.ResponseSchema) != askSchema {
		t.Fatalf("question = %+v", q)
	}
	p, err := r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil || p.Run.State != WorkflowRunWaitingForHuman || p.Run.NotBefore != nil || p.Question == nil || p.Question.Seq != 1 {
		t.Fatalf("progress = %+v, %v", p, err)
	}
	if _, err := r.s.AskWorkflowQuestion(ctx, lease, askQuestion(run.ID)); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("asking twice: %v", err)
	}

	// Nothing advances, pauses, resumes or fails a run waiting for a human,
	// and no scheduler finds it due, however long it waits.
	if _, err := r.s.BeginWorkflowAttempt(ctx, lease, WorkflowAttemptStart{RunID: run.ID, Checkpoint: 2, Step: "ask", Kind: workflow.KindInterrupt, Attempt: 1, Input: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("a node attempt began while the run waits for a human")
	}
	if _, err := r.s.PauseWorkflowRun(ctx, lease, "provider away", time.Minute); !errors.Is(err, ErrWorkflowInterruptionOpen) {
		t.Fatalf("pause while waiting for a human: %v", err)
	}
	if resumed, err := r.s.ResumeWorkflowRun(ctx, lease); err != nil || resumed.State != WorkflowRunWaitingForHuman {
		t.Fatalf("resume while waiting for a human = %+v, %v", resumed, err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, lease, 2, "limits"); !errors.Is(err, ErrWorkflowInterruptionOpen) {
		t.Fatalf("fail while waiting for a human: %v", err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, lease, ""); err != nil {
		t.Fatal(err)
	}
	clock.Advance(73 * time.Hour)
	if due, err := r.s.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 0 {
		t.Fatalf("due = %v, %v", due, err)
	}
	if open := listInterruptions(t, r.s, WorkflowInterruptionFilter{WorkspaceID: run.WorkspaceID}); len(open) != 1 || open[0].RunID != run.ID {
		t.Fatalf("open interruptions = %+v", open)
	}

	// Any later surface answers, under its own claim of the run.
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "owner-web", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	other, err := r.s.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	answer := WorkflowAnswer{RunID: run.ID, Seq: q.Seq, Answer: json.RawMessage(`{"expected":true}`), Surface: "web"}
	for name, bad := range map[string]WorkflowAnswer{
		"chat surface":    {RunID: run.ID, Seq: q.Seq, Answer: answer.Answer, Surface: "chat"},
		"not an object":   {RunID: run.ID, Seq: q.Seq, Answer: json.RawMessage(`[true]`), Surface: "web"},
		"foreign session": {RunID: run.ID, Seq: q.Seq, Answer: answer.Answer, Surface: "web", SessionID: other.ID},
		"no question":     {RunID: run.ID, Seq: 7, Answer: answer.Answer, Surface: "web"},
		"stale lease":     {RunID: run.ID, Seq: q.Seq, Answer: answer.Answer, Surface: "web"},
	} {
		use := claim.Lease
		if name == "stale lease" {
			use = lease
		}
		if _, err := r.s.AnswerWorkflowQuestion(ctx, use, bad); err == nil {
			t.Fatalf("%s: the answer was recorded", name)
		}
	}
	answered, err := r.s.AnswerWorkflowQuestion(ctx, claim.Lease, answer)
	if err != nil || answered.State != WorkflowRunRunning || answered.Error != "" {
		t.Fatalf("answered run = %+v, %v", answered, err)
	}
	if _, err := r.s.AnswerWorkflowQuestion(ctx, claim.Lease, answer); !errors.Is(err, ErrWorkflowInterruptionClosed) {
		t.Fatalf("answering twice: %v", err)
	}
	got := interruptionsOf(t, r.s, run.ID)
	res := got[0].Resolution
	if len(got) != 1 || res == nil || res.Kind != WorkflowInterruptionAnswered || string(res.Answer) != `{"expected":true}` || res.Actor != "owner" ||
		res.Surface != "web" || res.LeaseHolder != "owner-web" || !res.RecordedAt.Equal(q.OpenedAt.Add(73*time.Hour)) {
		t.Fatalf("resolution = %+v", res)
	}
	p, err = r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil || p.HumanWait != 73*time.Hour || p.Question == nil || p.Question.Resolution == nil {
		t.Fatalf("progress after the answer = %+v, %v", p, err)
	}
	if open := listInterruptions(t, r.s, WorkflowInterruptionFilter{WorkspaceID: run.WorkspaceID}); len(open) != 0 {
		t.Fatalf("still open = %+v", open)
	}
	if all := listInterruptions(t, r.s, WorkflowInterruptionFilter{RunID: run.ID, All: true}); len(all) != 1 {
		t.Fatalf("all = %+v", all)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil || len(d.Interruptions) != 1 || d.Interruptions[0].Resolution == nil {
		t.Fatalf("detail = %+v, %v", d.Interruptions, err)
	}
	for _, statement := range []string{
		`UPDATE workflow_interruptions SET message='changed'`, `DELETE FROM workflow_interruptions`,
		`UPDATE workflow_interruption_resolutions SET answer='{"expected":false}'`, `DELETE FROM workflow_interruption_resolutions`,
	} {
		if _, err := r.db.ExecContext(ctx, statement); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

// A question carries the run's originating session, so that session can
// show it; any session of the Workspace may answer it.
func TestWorkflowQuestionIsSurfacedToTheOriginatingSession(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	w := notesWorkspace(t, r.s, "notebook:inbox", "notebook:digest")
	p := recordProposal(t, r.s, testProposal(t, w.ID, "../workflow/testdata/notes-digest", notesFixtures))
	a := activate(t, r.s, p, 0)
	started, err := r.s.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	later, err := r.s.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := r.s.StartWorkflowRun(ctx, WorkflowRunStart{WorkspaceID: w.ID, Workflow: a.Name, Version: a.Version, ExecutableHash: a.ExecutableHash,
		AuthorityHash: a.AuthorityHash, Composition: json.RawMessage(`{"runtime":"test"}`), RunKey: "notes-digest:k9", Inputs: json.RawMessage(`{"day":"2026-10-05"}`),
		Start: "read", Surface: "web", SessionID: started.ID})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	advanceTo(t, r.s, claim.Lease, run.ID, []string{"read", "check"}, []string{"check", "ask"})
	q, err := r.s.AskWorkflowQuestion(ctx, claim.Lease, askQuestion(run.ID))
	if err != nil || q.SessionID != started.ID {
		t.Fatalf("question = %+v, %v", q, err)
	}
	if mine := listInterruptions(t, r.s, WorkflowInterruptionFilter{SessionID: started.ID}); len(mine) != 1 || mine[0].Seq != q.Seq {
		t.Fatalf("the originating session's interruptions = %+v", mine)
	}
	if theirs := listInterruptions(t, r.s, WorkflowInterruptionFilter{SessionID: later.ID}); len(theirs) != 0 {
		t.Fatalf("another session's interruptions = %+v", theirs)
	}
	if _, err := r.s.AnswerWorkflowQuestion(ctx, claim.Lease, WorkflowAnswer{RunID: run.ID, Seq: q.Seq, Answer: json.RawMessage(`{"expected":false,"note":"holiday"}`),
		Surface: "cli", SessionID: later.ID}); err != nil {
		t.Fatal(err)
	}
	res := interruptionsOf(t, r.s, run.ID)[0].Resolution
	if res == nil || res.SessionID != later.ID || res.Surface != "cli" {
		t.Fatalf("resolution = %+v", res)
	}
}

// The hook cancellation uses: an open question of a run the owner
// cancelled is withdrawn, never answered (sub-stage 8 requires the
// owner's recorded cancellation first: only it moves the run to
// cancelling); an effect whose outcome is unknown can never be withdrawn,
// only reconciled.
func TestWorkflowQuestionWithdrawalIsTheCancellationHook(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	run, lease := notesAtAsk(t, r.s)
	q, err := r.s.AskWorkflowQuestion(ctx, lease, askQuestion(run.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.WithdrawWorkflowQuestion(ctx, lease, run.ID, q.Seq, "the owner cancelled the run"); !errors.Is(err, ErrWorkflowInterruptionClosed) {
		t.Fatalf("withdrawing without the owner's cancellation: %v", err)
	}
	if _, err := r.s.RequestWorkflowRunCancellation(ctx, WorkflowRunCancellationRequest{RunID: run.ID, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	withdrawn, err := r.s.WithdrawWorkflowQuestion(ctx, lease, run.ID, q.Seq, "the owner cancelled the run")
	if err != nil || withdrawn.State != WorkflowRunCancelling {
		t.Fatalf("withdrawn = %+v, %v", withdrawn, err)
	}
	res := interruptionsOf(t, r.s, run.ID)[0].Resolution
	if res == nil || res.Kind != WorkflowInterruptionWithdrawn || res.Detail != "the owner cancelled the run" || len(res.Answer) != 0 {
		t.Fatalf("resolution = %+v", res)
	}
	if _, err := r.s.AnswerWorkflowQuestion(ctx, lease, WorkflowAnswer{RunID: run.ID, Seq: q.Seq, Answer: json.RawMessage(`{"expected":true}`), Surface: "cli"}); err == nil {
		t.Fatal("a withdrawn question was answered")
	}

	r2, _ := openLeasedRunDB(t)
	effectRun, effectLease := notesAtWrite(t, r2.s)
	intent, err := r2.s.BeginWorkflowEffect(ctx, effectLease, writeEffect(effectRun.ID, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r2.s.MarkWorkflowEffectUnknown(ctx, effectLease, WorkflowEffectUnknownMark{RunID: effectRun.ID, IntentSeq: intent.Seq, Reason: "timed out"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r2.s.WithdrawWorkflowQuestion(ctx, effectLease, effectRun.ID, 1, "cancel"); !errors.Is(err, ErrWorkflowEffectUnresolved) {
		t.Fatalf("withdrawing an effect reconciliation: %v", err)
	}
}

// An unknown effect outcome is presented as a durable interruption; the
// owner's reconciliation answers it and records the Effect Receipt in one
// transaction, so the answer and the receipt are one durable record.
func TestWorkflowEffectUnknownOpensAReconciliationInterruption(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.MarkWorkflowEffectUnknown(ctx, lease, WorkflowEffectUnknownMark{RunID: run.ID, IntentSeq: intent.Seq,
		Reason: "steps.write: the provider did not answer within 30s"}); err != nil {
		t.Fatal(err)
	}
	got := interruptionsOf(t, r.s, run.ID)
	if len(got) != 1 || got[0].Kind != WorkflowInterruptionEffect || got[0].IntentSeq != intent.Seq || got[0].Step != "write" || got[0].Resolution != nil ||
		!strings.Contains(string(got[0].Request), `"idempotency_key":"`+intent.IdempotencyKey+`"`) || !strings.Contains(string(got[0].Request), "30s") ||
		!strings.Contains(string(got[0].ResponseSchema), "did-not-happen") {
		t.Fatalf("interruptions = %+v", got)
	}
	if p, err := r.s.WorkflowRunProgress(ctx, run.ID); err != nil || p.Run.State != WorkflowRunNeedsReconciliation || p.Question != nil {
		t.Fatalf("progress = %+v, %v", p, err)
	}
	if _, err := r.s.AnswerWorkflowQuestion(ctx, lease, WorkflowAnswer{RunID: run.ID, Seq: 1, Answer: json.RawMessage(`{"outcome":"happened"}`), Surface: "cli"}); err == nil {
		t.Fatal("an effect reconciliation was answered as a question, without its receipt")
	}
	session := workspaceSession(t, r.s, run.WorkspaceID)
	answer := json.RawMessage(`{"evidence":"entry e7 is in the digest","outcome":"happened","output":{"entry_id":"e7"}}`)
	if _, err := r.s.ResolveWorkflowEffect(ctx, lease, WorkflowEffectResolution{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectSucceeded,
		Response: json.RawMessage(`{"entry_id":"e7"}`), Source: WorkflowEffectFromOwner, Evidence: "entry e7 is in the digest", Surface: "web",
		SessionID: session.ID, Answer: answer}); err != nil {
		t.Fatal(err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	res, receipt := d.Interruptions[0].Resolution, d.Effects[0].Receipt
	if res == nil || res.Kind != WorkflowInterruptionAnswered || string(res.Answer) != string(answer) || res.Surface != "web" || res.SessionID != session.ID ||
		receipt == nil || receipt.Source != WorkflowEffectFromOwner || receipt.Evidence != "entry e7 is in the digest" || !receipt.RecordedAt.Equal(res.RecordedAt) ||
		d.Run.State != WorkflowRunRunning {
		t.Fatalf("resolution %+v receipt %+v run %+v", res, receipt, d.Run)
	}
}

// Each AI attempt records its model call (pinned policy, prompt, schema
// and request digests, the estimate) with the attempt, before the call,
// and its usage and cost with the attempt's outcome. A call whose attempt
// was interrupted counts at its estimate: it may have been billed.
func TestWorkflowModelCallsRecordUsageWithTheAttempt(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, run := notesRun(t, r.s)
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	lease := claim.Lease
	advanceTo(t, r.s, lease, run.ID, []string{"read", "check"}, []string{"check", "summarize"})
	start := func(attempt int) WorkflowModelCallStart {
		return WorkflowModelCallStart{
			Attempt: WorkflowAttemptStart{RunID: run.ID, Checkpoint: 2, Step: "summarize", Kind: workflow.KindAI, Attempt: attempt, Input: json.RawMessage(`{"notes":[]}`)},
			Policy:  "summarizer", PolicyVersion: 2, Provider: "openrouter", Model: "deepseek/deepseek-v4.1-flash", Parameters: json.RawMessage(`{"max_output_tokens":2000,"temperature":0}`),
			PromptSHA256: "sha256:" + strings.Repeat("1", 64), SchemaSHA256: "sha256:" + strings.Repeat("2", 64), RequestSHA256: "sha256:" + strings.Repeat("3", 64),
			EstimatedInputTokens: 300, EstimatedOutputTokens: 2000, EstimatedMicroUSD: 17_000,
		}
	}
	first, err := r.s.BeginWorkflowModelCall(ctx, lease, start(1))
	if err != nil || first.State != WorkflowAttemptRunning || first.Kind != workflow.KindAI {
		t.Fatalf("first = %+v, %v", first, err)
	}
	p, err := r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil || p.ModelTokens != 2300 || p.ModelMicroUSD != 17_000 {
		t.Fatalf("spend while in flight = %d tokens %d µ$, %v", p.ModelTokens, p.ModelMicroUSD, err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: first.Seq, Error: "x"}); !errors.Is(err, ErrWorkflowModelCallOpen) {
		t.Fatalf("finishing an AI attempt without its model call: %v", err)
	}
	// A crash: the next holder records the attempt interrupted; its call
	// counts at the estimate.
	clock.Advance(leaseFor + time.Second)
	claim, err = r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	lease = claim.Lease
	second, err := r.s.BeginWorkflowModelCall(ctx, lease, start(2))
	if err != nil {
		t.Fatal(err)
	}
	finished, err := r.s.FinishWorkflowModelCall(ctx, lease, WorkflowModelCallEnd{RunID: run.ID, AttemptSeq: second.Seq, Requests: 2, InputTokens: 280, OutputTokens: 40,
		UsageReported: true, MicroUSD: 132, PricingSource: "catalog", Response: `{"digest":"- x"}`,
		Attempt: WorkflowAttemptEnd{Output: json.RawMessage(`{"digest":"- x"}`), Next: "write", State: json.RawMessage(`{"check":{},"read":{},"summarize":{"digest":"- x"}}`)}})
	if err != nil || finished.Checkpoint != 3 || finished.NextStep != "write" {
		t.Fatalf("finished = %+v, %v", finished, err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.ModelCalls) != 2 || d.ModelCalls[0].State() != WorkflowModelCallInterrupted || d.ModelCalls[1].State() != WorkflowModelCallFinished ||
		d.ModelCalls[1].InputTokens != 280 || d.ModelCalls[1].Requests != 2 || d.ModelCalls[1].Response != `{"digest":"- x"}` || d.ModelCalls[1].Model != "deepseek/deepseek-v4.1-flash" {
		t.Fatalf("model calls = %+v", d.ModelCalls)
	}
	p, err = r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil || p.ModelTokens != 2300+320 || p.ModelMicroUSD != 17_000+132 {
		t.Fatalf("spend = %d tokens %d µ$, %v", p.ModelTokens, p.ModelMicroUSD, err)
	}
	for _, statement := range []string{`UPDATE workflow_model_calls SET input_tokens=1 WHERE attempt_seq=` + jsonInt(second.Seq), `DELETE FROM workflow_model_calls`} {
		if _, err := r.db.ExecContext(ctx, statement); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func jsonInt(n int64) string {
	data, _ := json.Marshal(n)
	return string(data)
}

// workspaceSession creates a session of the Workspace.
func workspaceSession(t *testing.T, s *Store, ws memory.WorkspaceID) memory.Session {
	t.Helper()
	workspaces, err := s.ListWorkspaces(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range workspaces {
		if w.ID == ws {
			session, err := s.CreateWorkspaceSessionWithComposition(context.Background(), ws, w.CurrentRevisionID, standardReceipt(t))
			if err != nil {
				t.Fatal(err)
			}
			return session
		}
	}
	t.Fatalf("no Workspace %s", ws)
	return memory.Session{}
}
