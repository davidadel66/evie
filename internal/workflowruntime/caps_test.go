package workflowruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// The operator's daily cap (Stage 14 sub-stage 11): before every AI call
// and worker start, today's spend across all workflow runs (UTC day, open
// calls at their bound) plus the call's bound must stay within the cap in
// tokens and dollars. Otherwise nothing is sent: the run pauses visibly
// with the reason, its notification is cap_reached, and it continues by
// itself after midnight UTC. Each run's own limits.max_tokens still
// applies.

// processWithCap is a process over the harness's database whose runtime
// enforces cap.
func (h *runHarness) processWithCap(cap eviedb.WorkflowDailyCap) *process {
	h.t.Helper()
	h.cap = &cap
	defer func() { h.cap = nil }()
	return h.process()
}

func TestWorkflowDailyCapPausesBeforeAnAICallAndResumesNextDay(t *testing.T) {
	ctx := context.Background()
	chat := newFakeChat(reply(`{"total_cents":1200,"confidence":"high"}`, 100, 50))
	h := newAIHarness(t, chat)
	w := h.approveReceiptSplit(receiptSplitDir)
	first, err := h.startReceipt(h.rt, w.ID, "r1", "")
	if err != nil || first.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("first run = %+v, %v", first.Run.Run, err)
	}
	estimate := first.Run.ModelCalls[0].EstimatedInputTokens + first.Run.ModelCalls[0].EstimatedOutputTokens
	// Today has counted 150 tokens; the next call could use estimate more,
	// and the cap leaves one token less than that.
	p := h.processWithCap(eviedb.WorkflowDailyCap{Tokens: 150 + estimate - 1, MicroUSD: 1_000_000})
	sent := len(chat.captured())
	second, err := h.startReceipt(p.rt, w.ID, "r2", "")
	if err != nil {
		t.Fatal(err)
	}
	run := second.Run.Run
	if run.State != eviedb.WorkflowRunWaitingForDependency || !strings.HasPrefix(run.Error, eviedb.WorkflowDailyCapReason+" for 2026-10-04 (UTC)") ||
		run.NotBefore == nil || !run.NotBefore.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("capped run = %+v", run)
	}
	if len(chat.captured()) != sent || len(second.Run.ModelCalls) != 0 {
		t.Fatalf("a capped call was sent or recorded: %d requests, %+v", len(chat.captured())-sent, second.Run.ModelCalls)
	}
	notes, err := p.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{RunID: run.ID})
	if err != nil || len(notes) != 1 || notes[0].Kind != eviedb.WorkflowNotifyCapReached || !strings.Contains(notes[0].Text, "daily workflow cap") {
		t.Fatalf("notifications = %+v, %v", notes, err)
	}
	caps, err := p.rt.Caps(ctx)
	if err != nil || !caps.Enforced || caps.Day != "2026-10-04" || caps.Spent.Tokens != 150 || caps.Cap.Tokens != 150+estimate-1 ||
		len(caps.PausedRuns) != 1 || caps.PausedRuns[0] != run.ID || !caps.ResetAt.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("caps = %+v, %v", caps, err)
	}
	if text := caps.Text(); !strings.Contains(text, "150 of") || !strings.Contains(text, "paused at the cap: 1 run") {
		t.Fatalf("caps text:\n%s", text)
	}
	// Rechecks before midnight leave it paused without notifying again.
	if end, err := p.rt.execute(ctx, run.ID); err != nil || end != passWaiting {
		t.Fatalf("recheck before midnight = %v, %v", end, err)
	}
	// The next UTC day it continues by itself and succeeds.
	h.setClock("2026-10-05T00:00:01Z")
	d, err := p.rt.await(ctx, run.ID)
	if err != nil || d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("after midnight = %+v, %v", d.Run, err)
	}
	if notes, _ := p.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{RunID: run.ID, All: true}); len(notes) != 2 || notes[0].Kind != eviedb.WorkflowNotifySucceeded {
		t.Fatalf("notifications after resuming = %+v", notes)
	}
	// An Evie without a configured cap reports none.
	if caps, err := h.rt.Caps(ctx); err != nil || caps.Enforced {
		t.Fatalf("caps without one = %+v, %v", caps, err)
	}
}

// A map of AI items starts only the items that fit today's cap, lets
// those finish, then pauses; the next day it continues from its finished
// items and runs the rest.
func TestWorkflowDailyCapPausesAMapBetweenItems(t *testing.T) {
	ctx := context.Background()
	chat := newFakeChat(summarizeAnswers)
	h := newAIHarness(t, chat)
	w := h.approveSummarizeEach()
	first, err := h.startRun(h.rt, w.ID, "summarize-each", map[string]string{"question": "Q1"})
	if err != nil || first.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("first run = %+v, %v", first.Run.Run, err)
	}
	var spent, item int64
	for _, c := range first.Run.ModelCalls {
		spent += c.Tokens()
		if c.Item != nil {
			item = c.EstimatedInputTokens + c.EstimatedOutputTokens
		}
	}
	// Room for the plan (60 tokens used) and one item's estimate, not two.
	p := h.processWithCap(eviedb.WorkflowDailyCap{Tokens: spent + 60 + item + item/2, MicroUSD: 1_000_000})
	second, err := h.startRun(p.rt, w.ID, "summarize-each", map[string]string{"question": "Q2"})
	if err != nil {
		t.Fatal(err)
	}
	run := second.Run.Run
	if run.State != eviedb.WorkflowRunWaitingForDependency || !strings.HasPrefix(run.Error, eviedb.WorkflowDailyCapReason) {
		t.Fatalf("map run = %+v\n%s", run, RunText(second.Run))
	}
	items := 0
	for _, a := range second.Run.Attempts {
		if a.Item != nil {
			items++
			if a.State != eviedb.WorkflowAttemptSucceeded {
				t.Fatalf("item attempt = %+v", a)
			}
		}
	}
	if items != 1 {
		t.Fatalf("items started before the pause = %d\n%s", items, RunText(second.Run))
	}
	h.setClock("2026-10-05T00:00:01Z")
	d, err := p.rt.await(ctx, run.ID)
	if err != nil || d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("after midnight = %+v, %v\n%s", d.Run, err, RunText(d))
	}
	items = 0
	for _, a := range d.Attempts {
		if a.Item != nil {
			items++
		}
	}
	if items != 2 {
		t.Fatalf("item attempts = %d (a finished item ran again)\n%s", items, RunText(d))
	}
}

// summarizeAnswers plans two topics (60 tokens) and summarizes each (50).
func summarizeAnswers(ctx context.Context, req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
	user := req.Messages[len(req.Messages)-1].Content
	if strings.Contains(req.Messages[0].Content, "Plan topics") {
		return reply(`{"topics":["x","y"]}`, 50, 10)(ctx, req)
	}
	topic := "x"
	if strings.Contains(user, `"topic":"y"`) {
		topic = "y"
	}
	return reply(`{"answer":"summary of `+topic+`"}`, 40, 10)(ctx, req)
}

func (h *runHarness) approveSummarizeEach() memory.Workspace {
	h.t.Helper()
	w := h.workspace()
	dir := writeDefinition(h.t, "summarize-each", summarizeMap, map[string]string{"prompts/plan.md": "Plan topics for the question.\n",
		"prompts/summary.md": "Summarize the topic in one sentence.\n", "schemas/topics.schema.json": topicsSchema, "schemas/finding.schema.json": findingSchema})
	h.approve(h.propose(w.ID, dir, fixtures(h.t, `{"inputs": {"question": "Q"}, "steps": {"plan": {"output": {"topics": ["x"]}, "tokens": 10},
 "summaries": {"item_default": {"output": {"answer": "s"}, "tokens": 10}}}}`)))
	return w
}
