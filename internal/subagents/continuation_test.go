package subagents_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

// callParentTool runs one Subagents Plugin tool as the parent's live
// invocation, exactly as the parent's dispatcher would.
func callParentTool(t *testing.T, f *fixture, p delegation.Parent, name, args string) (string, error) {
	t.Helper()
	for _, capability := range plugins.NewSubagents(f.supervisor).ToolCapabilities() {
		if capability.Tool.Schema.Function.Name == name {
			ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Profile: f.profile, Scope: p.Scope, Lease: p.Lease, SourceEventID: p.SourceEventID, IntentEventID: p.IntentEventID})
			return capability.Tool.Execute(ctx, args)
		}
	}
	t.Fatalf("Subagents Plugin does not provide %s", name)
	return "", nil
}

// commitContinue commits the parent's continue_research intent in its
// current turn and returns the invocation and its arguments.
func (f *fixture) commitContinue(t *testing.T, p delegation.Parent, executionID, message string) (delegation.Parent, string) {
	t.Helper()
	args, _ := json.Marshal(delegation.Continuation{ExecutionID: executionID, Message: message})
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-" + uuid.NewString(), Name: delegation.ContinueToolName, Arguments: string(args)}})
	intent, err := f.store.AppendEventWithLease(context.Background(), p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: p.SourceEventID, ExecutionID: memory.ExecutionID(uuid.NewString()), Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	p.IntentEventID = intent.ID
	return p, string(args)
}

func (f *fixture) callContinue(t *testing.T, p delegation.Parent, args string) (delegation.Result, error) {
	t.Helper()
	out, err := callParentTool(t, f, p, delegation.ContinueToolName, args)
	if err != nil {
		return delegation.Result{}, err
	}
	return unframedResult(t, out), nil
}

// unframedResult decodes one parent-view result and, after checking that the
// child's text arrived sealed in its data frame (G4), restores the summary
// and the child's limitations, so assertions can compare them exactly.
func unframedResult(t *testing.T, out string) delegation.Result {
	t.Helper()
	var r delegation.Result
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("parent-view result %q: %v", out, err)
	}
	if r.Summary == "" {
		return r
	}
	payload := sealedResearchFrame(t, r.Summary)
	summary, limitations, found := strings.Cut(payload, "\n\nLimitations:\n- ")
	if !found {
		summary, limitations, found = "", strings.TrimPrefix(payload, "Limitations:\n- "), strings.HasPrefix(payload, "Limitations:\n- ")
		if !found {
			summary = payload
		}
	}
	r.Summary = summary
	if found {
		r.Limitations = strings.Split(limitations, "\n- ")
	}
	return r
}

// continueResearch is one parent continue_research call in p's turn.
func (f *fixture) continueResearch(t *testing.T, p delegation.Parent, executionID, message string) (delegation.Result, error) {
	t.Helper()
	p, args := f.commitContinue(t, p, executionID, message)
	return f.callContinue(t, p, args)
}

func (f *fixture) attemptCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM subagent_executions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func reportPage(t *testing.T, f *fixture, p delegation.Parent, executionID string) delegation.ReportPage {
	t.Helper()
	out, err := callParentTool(t, f, p, delegation.ReportToolName, fmt.Sprintf(`{"execution_id":%q}`, executionID))
	if err != nil {
		t.Fatal(err)
	}
	var page delegation.ReportPage
	if err = json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatal(err)
	}
	// Pages are framed child output (G4); these reports contain no frame
	// markers, so the payload is the stored text.
	page.Text = sealedResearchFrame(t, page.Text)
	return page
}

// G10: a child that wrapped up at its token budget is continued instead of
// restarted. The continuation is a new attempt on the same child session: the
// child sees its own earlier turn, runs on a fresh budget, and returns a new
// result whose sources and usage cover its own work.
func TestContinuePartialChildResumesSameSessionWithFreshBudget(t *testing.T) {
	p := delegation.DefaultPolicy()
	p.TokenBudget = 1000
	f := setupFixture(t, p, nil, evidenceWeb{})
	first := report("First pass found https://search.example/hit")
	second := report("Updated: https://search.example/hit agrees with https://fetched.example/late")
	client := &recordingClient{next: func(n int, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		switch n {
		case 1:
			return toolResponse("hit", "web_fetch", `{"url":"https://search.example/hit"}`, usageOf(400, 50)), nil
		case 2:
			return toolResponse("early", "web_fetch", `{"url":"https://fetched.example/early"}`, usageOf(400, 50)), nil
		case 3:
			return withUsage(response(first), usageOf(400, 50)), nil
		case 4:
			return toolResponse("late", "web_fetch", `{"url":"https://fetched.example/late"}`, usageOf(400, 50)), nil
		}
		return withUsage(response(second), usageOf(100, 10)), nil
	}}
	configure(t, f, client)
	ctx := context.Background()
	r, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "topic", Objective: "objective-sentinel: research the topic"}})
	if err != nil {
		t.Fatal(err)
	}
	original := r[0]
	if original.Status != "partial" || original.Reason != "token_budget" || !isWrapUpRequest(client.recorded()[2]) {
		t.Fatalf("original outcome: %+v", original)
	}
	if !strings.Contains(strings.Join(original.Notes, "\n"), "continue_research") {
		t.Fatalf("partial result does not say it can be continued: %q", original.Notes)
	}

	c, err := f.continueResearch(t, f.parent, original.ExecutionID, "followup-sentinel: check the late source too")
	if err != nil {
		t.Fatal(err)
	}
	requests := client.recorded()
	if len(requests) != 5 {
		t.Fatalf("continuation calls: %d", len(requests))
	}
	resumed := requests[3]
	encoded, _ := json.Marshal(resumed.Messages)
	// Same child history: the original assignment and report are visible.
	for _, want := range []string{"objective-sentinel", "First pass found", "followup-sentinel", "1000 model tokens"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("continued child request lacks %q: %s", want, encoded)
		}
	}
	// Fresh budget: 1350 tokens were spent before, yet the continuation's
	// first call is an ordinary tool-enabled call, not a wrap-up.
	if resumed.ToolChoice != "" || len(resumed.Tools) != 2 || isWrapUpRequest(requests[4]) {
		t.Fatalf("continuation did not get a fresh budget: %q tools=%d", resumed.ToolChoice, len(resumed.Tools))
	}
	if c.ExecutionID == original.ExecutionID || c.ChildSessionID != original.ChildSessionID || c.ContinuesExecutionID != original.ExecutionID {
		t.Fatalf("continuation identity: %+v", c)
	}
	if c.Status != "succeeded" || c.Summary != "Updated: https://search.example/hit agrees with https://fetched.example/late" || c.ReportBytes != len(second) {
		t.Fatalf("continuation outcome: %+v", c)
	}
	// Earlier pages verify citations; uncited earlier pages are not listed again.
	want := []delegation.Source{{URL: "https://search.example/hit", Fetched: true, Cited: true}, {URL: "https://fetched.example/late", Fetched: true, Cited: true}}
	if fmt.Sprint(c.Sources) != fmt.Sprint(want) || len(c.UnverifiedURLs) != 0 {
		t.Fatalf("continuation sources = %+v unverified=%q", c.Sources, c.UnverifiedURLs)
	}
	if c.Usage == nil || *c.Usage.InputTokens != 500 || *c.Usage.OutputTokens != 60 || c.Usage.Incomplete {
		t.Fatalf("continuation usage covers more than its own turn: %+v", c.Usage)
	}
	// Each attempt keeps its own report.
	if page := reportPage(t, f, f.parent, c.ExecutionID); page.Text != second {
		t.Fatalf("continuation report: %q", page.Text)
	}
	if page := reportPage(t, f, f.parent, original.ExecutionID); page.Text != first {
		t.Fatalf("original report changed: %q", page.Text)
	}
	stored, err := f.store.InspectSubagent(ctx, f.parent, original.ExecutionID)
	if err != nil || stored.State != "partial" || stored.Result.Summary != original.Summary {
		t.Fatalf("original attempt changed: %+v %v", stored, err)
	}
	child, err := f.store.GetSession(ctx, c.ChildSessionID)
	if err != nil || child.Status != memory.SessionClosed {
		t.Fatalf("child session left open: %+v %v", child, err)
	}
	events, err := f.store.LoadEvents(ctx, c.ChildSessionID)
	if err != nil {
		t.Fatal(err)
	}
	roots := 0
	for _, e := range events {
		if e.Type == memory.EventUserMessage {
			roots++
		}
	}
	if roots != 2 {
		t.Fatalf("child turns = %d, want 2", roots)
	}
}

// G10: continuation is scoped to the calling parent's own finished attempts.
func TestContinueRefusesOtherParentsRunningAndUnfinishedAttempts(t *testing.T) {
	ctx := context.Background()
	t.Run("another_parent", func(t *testing.T) {
		f := setup(t, delegation.DefaultPolicy())
		r, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "mine", Objective: "research"}})
		if err != nil {
			t.Fatal(err)
		}
		other := f.newParent(t)
		_, err = f.continueResearch(t, other, r[0].ExecutionID, "keep going")
		if !errors.Is(err, delegation.ErrNotFound) || f.client.calls != 1 || f.attemptCount(t) != 1 {
			t.Fatalf("another parent continued: %v calls=%d attempts=%d", err, f.client.calls, f.attemptCount(t))
		}
		if _, err = f.continueResearch(t, f.parent, "no-such-execution", "keep going"); !errors.Is(err, delegation.ErrNotFound) {
			t.Fatalf("unknown execution: %v", err)
		}
	})
	t.Run("still_running", func(t *testing.T) {
		f := setup(t, delegation.DefaultPolicy())
		g := &gatedProvider{entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
		configure(t, f, g)
		done := make(chan []delegation.Result, 1)
		go func() {
			r, _ := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "held", Objective: "research"}})
			done <- r
		}()
		<-g.entered
		var running string
		if err := f.db.QueryRow(`SELECT id FROM subagent_executions WHERE state='running'`).Scan(&running); err != nil {
			t.Fatal(err)
		}
		_, err := f.continueResearch(t, f.parent, running, "keep going")
		if !errors.Is(err, delegation.ErrNotResumable) || !strings.Contains(err.Error(), "running") || f.attemptCount(t) != 1 {
			t.Fatalf("continued a running attempt: %v", err)
		}
		g.release <- struct{}{}
		r := <-done
		if len(r) != 1 || r[0].Status != "succeeded" {
			t.Fatalf("original: %+v", r)
		}
		g.release <- struct{}{}
		c, err := f.continueResearch(t, f.parent, running, "keep going")
		if err != nil || c.Status != "succeeded" || c.ChildSessionID != r[0].ChildSessionID {
			t.Fatalf("continuation after finish: %+v %v", c, err)
		}
	})
	t.Run("failed", func(t *testing.T) {
		f := setup(t, delegation.DefaultPolicy())
		configure(t, f, clientFunc(func(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
			return openrouter.ChatResponse{}, errors.New("provider unavailable")
		}))
		r, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "broken", Objective: "research"}})
		if err != nil || r[0].Status != "failed" {
			t.Fatalf("original: %+v %v", r, err)
		}
		if _, err = f.continueResearch(t, f.parent, r[0].ExecutionID, "keep going"); !errors.Is(err, delegation.ErrNotResumable) || !strings.Contains(err.Error(), "new idempotency key") {
			t.Fatalf("continued a failed attempt: %v", err)
		}
	})
}

// G10: retrying the same committed intent returns the same continuation;
// only the child's latest report can be continued, so a new call naming an
// already-continued attempt is refused with the continuation to use instead.
func TestContinuationRetryOfSameIntentReturnsTheSameAttempt(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	r, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "topic", Objective: "research"}})
	if err != nil {
		t.Fatal(err)
	}
	p, args := f.commitContinue(t, f.parent, r[0].ExecutionID, "go deeper")
	first, err := f.callContinue(t, p, args)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.callContinue(t, p, args)
	if err != nil || again.ExecutionID != first.ExecutionID || f.client.calls != 2 || f.attemptCount(t) != 2 {
		t.Fatalf("retry reran the continuation: %+v %v calls=%d", again, err, f.client.calls)
	}
	// G5: the retry is reported as a replay of the attempt it returns.
	if first.Replayed || first.CompletedAt != nil || !again.Replayed || again.CompletedAt == nil {
		t.Fatalf("continuation replay not flagged: first=%+v again=%+v", first, again)
	}
	// Arguments that differ from the committed intent are refused.
	changed, _ := json.Marshal(delegation.Continuation{ExecutionID: r[0].ExecutionID, Message: "something else"})
	if _, err = f.callContinue(t, p, string(changed)); err == nil || f.client.calls != 2 {
		t.Fatalf("changed arguments accepted: %v", err)
	}
	_, err = f.continueResearch(t, f.parent, r[0].ExecutionID, "go deeper")
	if !errors.Is(err, delegation.ErrNotResumable) || !strings.Contains(err.Error(), first.ExecutionID) || f.client.calls != 2 {
		t.Fatalf("superseded attempt continued: %v", err)
	}
	third, err := f.continueResearch(t, f.parent, first.ExecutionID, "and further")
	if err != nil || third.ContinuesExecutionID != first.ExecutionID || third.ChildSessionID != r[0].ChildSessionID || f.client.calls != 3 {
		t.Fatalf("chained continuation: %+v %v", third, err)
	}
}

// G6/G10: a continuation is one of the parent turn's child runs.
func TestContinuationsCountTowardThePerTurnLimit(t *testing.T) {
	p := delegation.DefaultPolicy()
	p.PerTurn = 2
	f := setup(t, p)
	ctx := context.Background()
	r, err := f.delegate(t, ctx, f.parent, batch("a", 2))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.continueResearch(t, f.parent, r[0].ExecutionID, "more")
	var limit *delegation.TurnLimitError
	if !errors.As(err, &limit) || limit.Started != 2 || limit.Requested != 1 || f.client.calls != 2 {
		t.Fatalf("continuation over the per-turn limit: %v calls=%d", err, f.client.calls)
	}
	next := f.nextTurn(t, f.parent)
	continued, args := f.commitContinue(t, next, r[0].ExecutionID, "more")
	c, err := f.callContinue(t, continued, args)
	if err != nil || c.Status != "succeeded" {
		t.Fatalf("continuation in the next turn: %+v %v", c, err)
	}
	if _, err = f.delegate(t, ctx, next, batch("b", 2)); !errors.As(err, &limit) || limit.Started != 1 {
		t.Fatalf("continuation not counted in its turn: %v", err)
	}
	if _, err = f.delegate(t, ctx, next, batch("b", 1)); err != nil {
		t.Fatal(err)
	}
	// Retrying the retained continuation admits nothing new.
	if again, err := f.callContinue(t, continued, args); err != nil || again.ExecutionID != c.ExecutionID || f.client.calls != 4 {
		t.Fatalf("retained continuation at the limit: %+v %v calls=%d", again, err, f.client.calls)
	}
	if _, err = f.continueResearch(t, next, c.ExecutionID, "more"); !errors.As(err, &limit) {
		t.Fatalf("third run of the turn: %v", err)
	}
}

// compactionSummary is a valid rolling summary with every required heading.
func compactionSummary() string {
	var b strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&b, "## %s\nsummary-sentinel\n\n", heading)
	}
	return b.String()
}

// G10: a continued child has a closed earlier turn, so automatic compaction
// applies to it like the primary chat: a large first turn is summarized
// before the continuation's request instead of overflowing its context.
func TestContinuedChildCompactsItsEarlierTurns(t *testing.T) {
	p := delegation.DefaultPolicy()
	p.RequestBytes = 96 * 1024
	f := setupFixture(t, p, nil, pageWeb{size: 72 * 1024})
	client := &recordingClient{next: func(n int, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if strings.Contains(r.Messages[0].Content, "context compactor") {
			return response(compactionSummary()), nil
		}
		switch n {
		case 1:
			return toolResponse("long", "web_fetch", `{"url":"https://pages.example/long"}`, nil), nil
		case 2:
			return response(report("The long page says X https://pages.example/long")), nil
		}
		return response(report("Follow-up answer citing https://pages.example/long")), nil
	}}
	configure(t, f, client)
	ctx := context.Background()
	r, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "long", Objective: "read the long page"}})
	if err != nil || r[0].Status != "succeeded" {
		t.Fatalf("original: %+v %v", r, err)
	}
	// A long follow-up pushes the next request past the compaction threshold.
	c, err := f.continueResearch(t, f.parent, r[0].ExecutionID, "Follow-up: "+strings.Repeat("compare carefully. ", 400))
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "succeeded" || !strings.Contains(c.Summary, "Follow-up answer") {
		t.Fatalf("continuation: %+v", c)
	}
	var compactions, conversational []openrouter.ChatRequest
	for _, request := range client.recorded()[2:] {
		if strings.Contains(request.Messages[0].Content, "context compactor") {
			compactions = append(compactions, request)
		} else {
			conversational = append(conversational, request)
		}
	}
	if len(compactions) != 1 || len(conversational) != 1 {
		t.Fatalf("continuation calls: %d compactions, %d conversational", len(compactions), len(conversational))
	}
	summarized, _ := json.Marshal(compactions[0].Messages)
	sent, _ := json.Marshal(conversational[0].Messages)
	if !strings.Contains(string(summarized), "page-sentinel") || strings.Contains(string(sent), "page-sentinel") || !strings.Contains(string(sent), "summary-sentinel") {
		t.Fatal("the earlier turn was not replaced by its summary")
	}
	events, err := f.store.LoadEvents(ctx, c.ChildSessionID)
	if err != nil {
		t.Fatal(err)
	}
	compacted := 0
	for _, e := range events {
		if e.Type == memory.EventContextCompacted {
			compacted++
		}
	}
	if compacted != 1 {
		t.Fatalf("durable compactions = %d", compacted)
	}
	want := []delegation.Source{{URL: "https://pages.example/long", Fetched: true, Cited: true}}
	if fmt.Sprint(c.Sources) != fmt.Sprint(want) {
		t.Fatalf("sources = %+v", c.Sources)
	}
}

// G10: a continuation is supervised like a fresh attempt: it wraps up at
// its own budget with its own durable wrap-up mark, and loses authority when
// the parent turn that authorized it does.
func TestContinuationWrapsUpAndLosesAuthorityLikeAFreshAttempt(t *testing.T) {
	ctx := context.Background()
	t.Run("wrap_up", func(t *testing.T) {
		t.Setenv(agent.TurnStepLimitEnv, "2")
		f := setupFixture(t, delegation.DefaultPolicy(), nil, evidenceWeb{})
		client := &recordingClient{next: func(n int, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
			if r.ToolChoice == "none" {
				return response(report(fmt.Sprintf("Pass %d https://search.example/hit", n))), nil
			}
			return toolResponse(fmt.Sprint(n), "web_fetch", `{"url":"https://search.example/hit"}`, nil), nil
		}}
		configure(t, f, client)
		r, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "loop", Objective: "keep researching"}})
		if err != nil || r[0].Status != "partial" {
			t.Fatalf("original: %+v %v", r, err)
		}
		c, err := f.continueResearch(t, f.parent, r[0].ExecutionID, "keep going")
		if err != nil || c.Status != "partial" || c.Reason != "step_limit" || c.Summary != "Pass 4 https://search.example/hit" {
			t.Fatalf("continuation wrap-up: %+v %v", c, err)
		}
		if requests := client.recorded(); len(requests) != 4 || requests[2].ToolChoice != "" || !isWrapUpRequest(requests[3]) {
			t.Fatalf("continuation requests: %d", len(requests))
		}
		stored, err := f.store.InspectSubagent(ctx, f.parent, c.ExecutionID)
		if err != nil || stored.WrapUp == nil || stored.WrapUp.Reason != "step_limit" || stored.Continues == nil || stored.Continues.ExecutionID != r[0].ExecutionID {
			t.Fatalf("stored continuation: %+v %v", stored, err)
		}
		if !strings.Contains(strings.Join(c.Notes, "\n"), "continue_research") {
			t.Fatalf("partial continuation does not say it can be continued: %q", c.Notes)
		}
	})
	t.Run("parent_authority", func(t *testing.T) {
		f := setup(t, delegation.DefaultPolicy())
		g := &gatedProvider{entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
		configure(t, f, g)
		g.release <- struct{}{}
		r, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "topic", Objective: "research"}})
		if err != nil || r[0].Status != "succeeded" {
			t.Fatalf("original: %+v %v", r, err)
		}
		<-g.entered
		p, _ := f.commitContinue(t, f.parent, r[0].ExecutionID, "more")
		type outcome struct {
			r   delegation.Result
			err error
		}
		done := make(chan outcome, 1)
		go func() {
			c, err := f.supervisor.Continue(ctx, p, r[0].ExecutionID, "more")
			done <- outcome{c, err}
		}()
		<-g.entered
		if err = f.store.ReleaseTurnLease(ctx, f.parent.Scope.SessionID, f.parent.Lease.HolderID, f.parent.Lease.FencingToken); err != nil {
			t.Fatal(err)
		}
		var got outcome
		select {
		case got = <-done:
		case <-time.After(gateTimeout):
			t.Fatal("continuation kept running after its parent turn ended")
		}
		if got.err != nil || got.r.Status != "interrupted" || got.r.Reason != "authority_ended" || got.r.ContinuesExecutionID != r[0].ExecutionID {
			t.Fatalf("continuation after parent authority ended: %+v %v", got.r, got.err)
		}
		child, err := f.store.GetSession(ctx, r[0].ChildSessionID)
		if err != nil || child.Status != memory.SessionClosed {
			t.Fatalf("child left open: %+v %v", child, err)
		}
		if n := unfinished(t, f); n != 0 {
			t.Fatalf("%d attempts left unfinished", n)
		}
	})
}

// G10 through the primary acceptance seam: the parent model delegates, then
// continues the child with the execution ID from the result, through the
// real Plugin, dispatcher and agent loop.
func TestComposedParentContinuesItsChildThroughThePlugin(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.CreateGlobalSessionWithComposition(ctx, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	toolResult := func(r openrouter.ChatRequest, id string, into any) error {
		for _, m := range r.Messages {
			if m.Role == "tool" && m.ToolCallID == id {
				return json.Unmarshal([]byte(m.Content), into)
			}
		}
		return fmt.Errorf("no %s result", id)
	}
	call := func(id, name, args string) openrouter.ChatResponse {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: id, Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: args}}}}}}}
	}
	var parentCalls, childCalls atomic.Int32
	var continued delegation.Result
	var first []delegation.Result
	client := clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		isParent := false
		for _, tool := range r.Tools {
			isParent = isParent || tool.Function.Name == delegation.ContinueToolName
		}
		if isParent {
			switch parentCalls.Add(1) {
			case 1:
				return call("research", delegation.ToolName, `{"assignments":[{"idempotency_key":"topic","objective":"objective-sentinel"}]}`), nil
			case 2:
				if err := toolResult(r, "research", &first); err != nil || len(first) != 1 {
					return openrouter.ChatResponse{}, fmt.Errorf("delegation result: %v", err)
				}
				args, _ := json.Marshal(delegation.Continuation{ExecutionID: first[0].ExecutionID, Message: "followup-sentinel"})
				return call("continue", delegation.ContinueToolName, string(args)), nil
			}
			if err := toolResult(r, "continue", &continued); err != nil {
				return openrouter.ChatResponse{}, err
			}
			return response("Owner-visible answer"), nil
		}
		encoded, _ := json.Marshal(r.Messages)
		if childCalls.Add(1) == 1 {
			return response(report("first report https://example.com/a")), nil
		}
		if !strings.Contains(string(encoded), "objective-sentinel") || !strings.Contains(string(encoded), "first report") || !strings.Contains(string(encoded), "followup-sentinel") {
			return openrouter.ChatResponse{}, errors.New("continued child lost its history or follow-up")
		}
		return response(report("second report https://example.com/a")), nil
	})
	configure(t, f, client)
	parent := agent.NewWithToolset(client, f.profile, f.store.BindHistory(stored.ID, "composed"), stored.ScopeContext(), f.store.BindTurnOwner(stored.ID, "composed"), resolved.Toolset)
	if err = parent.Send(ctx, "research and then dig deeper", &eventSink{}, nil); err != nil {
		t.Fatal(err)
	}
	if parentCalls.Load() != 3 || childCalls.Load() != 2 {
		t.Fatalf("calls: parent=%d child=%d", parentCalls.Load(), childCalls.Load())
	}
	// The parent read the child's report framed as untrusted data (G4).
	if continued.Status != "succeeded" || continued.ContinuesExecutionID != first[0].ExecutionID || continued.ChildSessionID != first[0].ChildSessionID ||
		!strings.HasPrefix(sealedResearchFrame(t, continued.Summary), "second report https://example.com/a\n\nLimitations:\n- Only one source was read.") {
		t.Fatalf("parent received continuation: %+v", continued)
	}
}
