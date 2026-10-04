package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflow"
)

// Durable interruptions (Stage 14 sub-stage 7). An interrupt step records
// its typed question and the run waits for the owner, across restarts,
// without a lease; nothing resumes it by itself. Any later surface or
// session answers it; the answer is validated against the step's schema,
// recorded once with its surface and session, and validated again before
// the graph advances. An effect whose outcome is unknown is the same kind
// of durable interruption: answering it and `evie workflows reconcile`
// record one answer with the Effect Receipt.

const lowTotal = `{"total_cents":1150,"confidence":"low"}`

func TestWorkflowInterruptionWaitsForTheOwnerAndIsAnsweredFromAnyLaterSurface(t *testing.T) {
	ctx := context.Background()
	chat := newFakeChat(reply(lowTotal, 120, 15))
	h := newAIHarness(t, chat)
	w := h.approveReceiptSplit(receiptSplitDir)
	origin, later := h.sessionIn(w), h.sessionIn(w)
	res, err := h.startReceipt(h.rt, w.ID, "r-2", origin.ID)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunWaitingForHuman || d.Run.NotBefore != nil || len(d.Interruptions) != 1 {
		t.Fatalf("run = %+v interruptions %+v", d.Run, d.Interruptions)
	}
	q := d.Interruptions[0]
	if q.Kind != eviedb.WorkflowInterruptionQuestion || q.Step != "confirm" || q.Checkpoint != 3 || q.SessionID != origin.ID || !q.Open() ||
		q.Message != "Evie read this receipt's total with low confidence. Reply with the total in cents." ||
		string(q.Request) != `{"input":{"proposed_total_cents":1150,"receipt_id":"r-2"}}` || !strings.Contains(string(q.ResponseSchema), `"note"`) {
		t.Fatalf("question = %+v", q)
	}
	if got := summarize(d); len(got) != 3 || got[2].Step != "sure" || got[2].Next != "confirm" {
		t.Fatalf("attempts = %+v", got)
	}
	if text := RunText(d); !strings.Contains(text, "Waiting for you: steps.confirm asks") ||
		!strings.Contains(text, "evie workflows answer "+id+" confirm --answer") {
		t.Fatalf("inspection:\n%s", text)
	}
	if mine, err := h.rt.Interruptions(ctx, InterruptionFilter{SessionID: origin.ID}); err != nil || len(mine) != 1 || mine[0].RunID != id {
		t.Fatalf("the originating session's interruptions = %+v, %v", mine, err)
	}

	// Nothing resumes it, however long and often schedulers poll, and it
	// waits across a restart.
	p := h.process()
	stop := p.rt.Start(ctx)
	for range 3 {
		h.clock.Advance(time.Hour)
		time.Sleep(20 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	h.reopen()
	h.advanceOnSleep(h.rt)
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunWaitingForHuman || len(d.Attempts) != 3 || len(d.Events) != 0 || len(chat.captured()) != 1 {
		t.Fatalf("after polling = %+v attempts %+v events %+v", d.Run, summarize(d), d.Events)
	}
	if due, err := h.store.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 0 {
		t.Fatalf("due = %v, %v", due, err)
	}

	// Answers are refused unless they come from the owner, name the
	// waiting step, fit its schema and come from a session of the
	// Workspace.
	global, err := h.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]AnswerRequest{
		"chat":            {RunID: id, Step: "confirm", Answer: json.RawMessage(`{"total_cents":1300}`), Surface: "chat"},
		"wrong type":      {RunID: id, Step: "confirm", Answer: json.RawMessage(`{"total_cents":"thirteen"}`), Surface: "web"},
		"unknown field":   {RunID: id, Step: "confirm", Answer: json.RawMessage(`{"total_cents":1300,"tip":1}`), Surface: "web"},
		"out of range":    {RunID: id, Step: "confirm", Answer: json.RawMessage(`{"total_cents":-1}`), Surface: "web"},
		"not an object":   {RunID: id, Step: "confirm", Answer: json.RawMessage(`[1300]`), Surface: "web"},
		"another step":    {RunID: id, Step: "split", Answer: json.RawMessage(`{"total_cents":1300}`), Surface: "web"},
		"foreign session": {RunID: id, Step: "confirm", Answer: json.RawMessage(`{"total_cents":1300}`), Surface: "web", SessionID: global.ID},
	} {
		if _, err := h.rt.Answer(ctx, bad); err == nil {
			t.Fatalf("%s: the answer was accepted", name)
		} else if name == "wrong type" && (!errors.Is(err, ErrAnswerRefused) || !strings.Contains(err.Error(), "total_cents: expected integer")) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if d := h.inspect(id); !d.Interruptions[0].Open() || d.Run.State != eviedb.WorkflowRunWaitingForHuman {
		t.Fatalf("a refused answer changed the run: %+v", d.Interruptions[0])
	}

	// Waiting for the owner does not count toward limits.max_duration
	// (30m): two hours later the answer still completes the run.
	h.clock.Advance(2 * time.Hour)
	answered, err := h.rt.Answer(ctx, AnswerRequest{RunID: id, Step: "confirm", Answer: json.RawMessage(`{"note":"checked the paper copy","total_cents":1300}`),
		SessionID: later.ID, Surface: "web"})
	if err != nil {
		t.Fatal(err)
	}
	wantOutputs := map[string]any{"proposed_total_cents": int64(1150), "confirmed_total_cents": int64(1300),
		"confirmed_shares": []any{int64(434), int64(433), int64(433)}, "tip_shares": []any{int64(0), int64(0), int64(0)}}
	if answered.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(decoded(t, answered.Run.Outputs), wantOutputs) {
		t.Fatalf("answered run = %+v", answered.Run)
	}
	d = h.inspect(id)
	res2 := d.Interruptions[0].Resolution
	if res2 == nil || res2.Kind != eviedb.WorkflowInterruptionAnswered || res2.Surface != "web" || res2.SessionID != later.ID || res2.Actor != "owner" ||
		string(res2.Answer) != `{"note":"checked the paper copy","total_cents":1300}` {
		t.Fatalf("resolution = %+v", res2)
	}
	// The proposal is kept; the answer is appended as the interruption's
	// output, and the deterministic step validated it again.
	confirm := d.Attempts[3]
	if string(d.Attempts[1].Output) != lowTotal2 || confirm.Step != "confirm" || confirm.Kind != workflow.KindInterrupt ||
		string(confirm.Output) != `{"note":"checked the paper copy","total_cents":1300}` || string(confirm.Input) != `{"proposed_total_cents":1150,"receipt_id":"r-2"}` ||
		!strings.Contains(confirm.Note, "interruption #1 answered by the owner via web") {
		t.Fatalf("attempts = %+v", d.Attempts)
	}
	checkOrdering(t, d)
	if _, err := h.rt.Answer(ctx, AnswerRequest{RunID: id, Step: "confirm", Answer: json.RawMessage(`{"total_cents":1}`), Surface: "cli"}); !errors.Is(err, ErrAnswerRefused) {
		t.Fatalf("answering a finished run again: %v", err)
	}
	if open, err := h.rt.Interruptions(ctx, InterruptionFilter{WorkspaceID: w.ID}); err != nil || len(open) != 0 {
		t.Fatalf("still open = %+v, %v", open, err)
	}
}

// lowTotal2 is lowTotal as recorded: canonical JSON.
const lowTotal2 = `{"confidence":"low","total_cents":1150}`

// An effect whose outcome is unknown is presented as a durable
// interruption; the owner's answer to it and `evie workflows reconcile`
// are the same durable record: the interruption's answer and the Effect
// Receipt, written together.
func TestWorkflowUnknownEffectIsReconciledThroughItsInterruption(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger(timeoutAfterAcceptance)
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	first, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	id := first.Run.Run.ID
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunNeedsReconciliation || len(d.Interruptions) != 1 {
		t.Fatalf("run = %+v interruptions %+v", d.Run, d.Interruptions)
	}
	q := d.Interruptions[0]
	if q.Kind != eviedb.WorkflowInterruptionEffect || q.IntentSeq != 1 || q.Step != "post" || !q.Open() ||
		!strings.Contains(string(q.Request), d.Effects[0].IdempotencyKey) || !strings.Contains(string(q.Request), "40ms") {
		t.Fatalf("interruption = %+v", q)
	}
	if text := RunText(d); !strings.Contains(text, "Waiting for you: did steps.post's effect happen?") ||
		!strings.Contains(text, "evie workflows answer "+id+" post --answer") {
		t.Fatalf("inspection:\n%s", text)
	}
	for name, bad := range map[string]string{
		"no outcome":       `{"evidence":"x"}`,
		"unknown outcome":  `{"outcome":"maybe"}`,
		"unknown field":    `{"outcome":"happened","output":{"entry_id":"entry-1","balance_cents":250},"amount":1}`,
		"outside contract": `{"outcome":"happened","output":{"entry_id":"entry-1"}}`,
		"output for none":  `{"outcome":"did-not-happen","output":{}}`,
	} {
		if _, err := h.rt.Answer(ctx, AnswerRequest{RunID: id, Step: "post", Answer: json.RawMessage(bad), Surface: "web"}); !errors.Is(err, ErrReconcileRefused) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	answer := `{"evidence":"the ledger shows entry-1","outcome":"happened","output":{"balance_cents":250,"entry_id":"entry-1"}}`
	done, err := h.rt.Answer(ctx, AnswerRequest{RunID: id, Step: "post", Answer: json.RawMessage(answer), Surface: "web"})
	if err != nil || done.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("answered = %+v, %v", done.Run, err)
	}
	d = h.inspect(id)
	res, receipt := d.Interruptions[0].Resolution, d.Effects[0].Receipt
	if res == nil || res.Kind != eviedb.WorkflowInterruptionAnswered || string(res.Answer) != answer || res.Surface != "web" || receipt == nil ||
		receipt.Source != eviedb.WorkflowEffectFromOwner || receipt.Surface != "web" || receipt.Evidence != "the ledger shows entry-1" ||
		!receipt.RecordedAt.Equal(res.RecordedAt) {
		t.Fatalf("resolution %+v receipt %+v", res, receipt)
	}

	// The reconcile command records the same answer.
	second, err := h.startPost(h.rt, w.ID, "dinner-2", false)
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := h.rt.Reconcile(ctx, Reconciliation{RunID: second.Run.Run.ID, Step: "post", Outcome: OutcomeHappened, Evidence: "the ledger shows entry-2",
		Output: json.RawMessage(`{"entry_id":"entry-2","balance_cents":500}`), Surface: "cli"})
	if err != nil || reconciled.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("reconciled = %+v, %v", reconciled.Run, err)
	}
	d = h.inspect(second.Run.Run.ID)
	res = d.Interruptions[0].Resolution
	if res == nil || res.Kind != eviedb.WorkflowInterruptionAnswered || res.Surface != "cli" ||
		string(res.Answer) != `{"evidence":"the ledger shows entry-2","outcome":"happened","output":{"balance_cents":500,"entry_id":"entry-2"}}` ||
		d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromOwner {
		t.Fatalf("reconcile resolution %+v receipt %+v", res, d.Effects[0].Receipt)
	}
}

// When the provider's reconciliation settles an unknown effect first, it
// closes the interruption as reconciled by the runtime, with its evidence.
func TestWorkflowProviderReconciliationClosesTheInterruption(t *testing.T) {
	ledger := newFakeLedger(timeoutAfterAcceptance)
	h, _ := newEffectHarness(t, ledger, true, ledger.lookupByKey)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Interruptions) != 1 {
		t.Fatalf("run = %+v interruptions %+v", d.Run, d.Interruptions)
	}
	r := d.Interruptions[0].Resolution
	if r == nil || r.Kind != eviedb.WorkflowInterruptionReconciled || r.Actor != "runtime" || r.Surface != "" ||
		string(r.Answer) != `{"evidence":"ledger entry entry-1 carries the key","outcome":"happened","output":{"balance_cents":250,"entry_id":"entry-1"}}` {
		t.Fatalf("resolution = %+v", r)
	}
}
