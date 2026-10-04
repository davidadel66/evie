package memorybackground

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// Stage 10 (memory flush before compaction): these tests drive the real
// manual compaction (the REPL and web /compact path) against SQLite with the
// runner as the session's flush, and a scripted pass model.

// summaryCompactor answers every compactor call with a valid summary.
type summaryCompactor struct{}

func (summaryCompactor) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	var summary strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&summary, "## %s\nkept\n\n", heading)
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: summary.String()}}}}, nil
}

// compactWith runs one manual compaction of the conversation and returns the
// committed compaction's payload. A nil flush models a process whose flush
// request was lost (a crash right after the commit).
func (f *fixture) compactWith(session memory.Session, flush agent.CompactionMemoryFlush) (memory.ContextCompactedPayload, error) {
	var payload memory.ContextCompactedPayload
	profile, err := openrouter.NewExplicitContextProfile("test/model", 262_144, 262_144, 16_384)
	if err != nil {
		return payload, err
	}
	const holder = memory.LeaseHolderID("compaction-fixture")
	var options []agent.SessionOption
	if flush != nil {
		options = append(options, agent.WithCompactionMemoryFlush(flush))
	}
	runtime := agent.NewWithToolset(summaryCompactor{}, profile, f.store.BindHistory(session.ID, holder), session.ScopeContext(),
		f.store.BindTurnOwner(session.ID, holder), tools.BuiltinToolset(), options...)
	result, err := runtime.Compact(f.ctx)
	if err != nil {
		return payload, err
	}
	events, err := f.store.LoadEvents(f.ctx, session.ID)
	if err != nil {
		return payload, err
	}
	for _, event := range events {
		if event.ID == result.CompactionEventID {
			return payload, json.Unmarshal(event.Payload, &payload)
		}
	}
	return payload, errors.New("the compaction event is missing")
}

func (f *fixture) compact(session memory.Session, flush agent.CompactionMemoryFlush) memory.ContextCompactedPayload {
	f.t.Helper()
	payload, err := f.compactWith(session, flush)
	if err != nil {
		f.t.Fatalf("compact: %v", err)
	}
	return payload
}

// fourTurns is a conversation whose manual compaction summarizes the first
// two turns (it always keeps the two newest); it returns the owner messages.
func (f *fixture) fourTurns(session memory.Session) []memory.Event {
	f.t.Helper()
	return []memory.Event{
		f.turn(session, "I'm vegetarian.", "Noted."),
		f.turn(session, "I live in Denver.", "Noted."),
		f.turn(session, "What's a good lentil recipe?", "Here is one."),
		f.turn(session, "Thanks. Unrelated: I'm a night owl.", "Noted."),
	}
}

func dietAndCity() []map[string]any {
	return []map[string]any{
		memoryItem(0, "profile_fact", "profile", "diet", "diet", "vegetarian"),
		memoryItem(1, "profile_fact", "profile", "home_city", "home city", "Denver"),
	}
}

// The flush covers exactly the owner messages up to the compaction's last
// summarized event, without waiting for the idle or turn triggers; its
// proposals land in the review inbox; the cursor ends at that frontier, so
// nothing is processed twice and newer owner messages wait for the ordinary
// triggers.
func TestACompactionFlushCoversTheSummarizedOwnerMessagesWithoutWaitingForTriggers(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	said := f.fourTurns(session)
	if due := f.due(); len(due) != 0 {
		t.Fatalf("a conversation was due before its compaction: %v", due)
	}
	payload := f.compact(session, f.runner)
	if payload.CoveredLastSequence != said[1].Sequence+1 {
		t.Fatalf("the compaction covered through %d, want the second turn's reply %d", payload.CoveredLastSequence, said[1].Sequence+1)
	}
	if due := f.due(); due[session.ID] != memory.BackgroundTriggerCompaction {
		t.Fatalf("the compacted conversation was not due for its flush: %v", due)
	}

	f.model.extract(dietAndCity()...)
	result, err := f.runner.Tick(f.ctx)
	if err != nil || len(result.Runs) != 1 {
		t.Fatalf("flush tick = %+v, %v", result, err)
	}
	run := result.Runs[0]
	if run.Trigger != memory.BackgroundTriggerCompaction || run.Status != memory.BackgroundRunSucceeded ||
		run.FromSequence != 0 || run.ThroughSequence != said[1].Sequence || run.Messages != 2 || run.Queued != 2 {
		t.Fatalf("flush run = %+v", run)
	}
	sent := f.model.calls("extract")[0].User
	if !strings.Contains(sent, "vegetarian") || !strings.Contains(sent, "Denver") || strings.Contains(sent, "lentil") || strings.Contains(sent, "night owl") {
		t.Fatalf("the flush read outside the compacted range: %s", sent)
	}
	if items := backgroundItems(f.inbox()); len(items) != 2 || f.cursor(session) != said[1].Sequence {
		t.Fatalf("inbox = %+v cursor = %d", items, f.cursor(session))
	}

	// No double processing: another request for the same compaction, and
	// another tick, find nothing owed.
	f.runner.RequestCompactionFlush(session.ID, payload.CoveredLastSequence)
	if result, err := f.runner.Tick(f.ctx); err != nil || len(result.Runs) != 0 || len(f.model.calls("")) != 1 {
		t.Fatalf("a covered flush ran again: %+v, %v, calls = %d", result, err, len(f.model.calls("")))
	}
	if runs, err := f.runner.PassThrough(f.ctx, session.ID, payload.CoveredLastSequence); err != nil || len(runs) != 0 {
		t.Fatalf("a covered frontier was passed again: %+v, %v", runs, err)
	}

	// The retained turns' owner messages are read once, by the idle trigger.
	f.offset = f.config.Idle + time.Second
	f.model.extract()
	if result, err := f.runner.Tick(f.ctx); err != nil || len(result.Runs) != 1 || result.Runs[0].Trigger != memory.BackgroundTriggerIdle {
		t.Fatalf("idle tick = %+v, %v", result, err)
	}
	if later := f.model.calls("extract")[1].User; strings.Contains(later, "vegetarian") || !strings.Contains(later, "lentil") || !strings.Contains(later, "night owl") {
		t.Fatalf("the idle pass reread the flushed messages: %s", later)
	}
	if len(backgroundItems(f.inbox())) != 2 {
		t.Fatalf("inbox after the idle pass = %+v", f.inbox())
	}
}

// The compaction never waits for the pass: a flush request returns while
// another pass is in the middle of a model call.
func TestACompactionNeverWaitsForARunningPass(t *testing.T) {
	f := newFixture(t)
	busy := f.globalSession()
	said := f.turn(busy, "I'm left-handed.")
	compacting := f.globalSession()
	f.fourTurns(compacting)
	release := make(chan struct{})
	f.model.script("extract", scriptedReply{text: `{"memories":[]}`, wait: release, usage: [2]int64{100, 5}})
	passed := make(chan error, 1)
	go func() {
		_, err := f.runner.PassThrough(f.ctx, busy.ID, said.Sequence)
		passed <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for len(f.model.calls("extract")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the running pass never reached its model call")
		}
		time.Sleep(5 * time.Millisecond)
	}
	compacted := make(chan error, 1)
	go func() {
		_, err := f.compactWith(compacting, f.runner)
		compacted <- err
	}()
	select {
	case err := <-compacted:
		if err != nil {
			t.Fatalf("compact: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the compaction waited for the running pass")
	}
	close(release)
	if err := <-passed; err != nil {
		t.Fatalf("the running pass: %v", err)
	}
}

// A running pass wakes for a flush request instead of waiting out its tick.
func TestTheRunningPassWakesForACompactionFlush(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	said := f.fourTurns(session)
	waiting := make(chan struct{}, 8)
	runner := NewRunner(f.store, f.model, f.config, WithPricing(Pricing{InputPerMillion: 0.3, OutputPerMillion: 1.2, Source: "test"}),
		WithSleep(func(ctx context.Context, _ time.Duration) error {
			waiting <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		}))
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- runner.Run(ctx) }()
	awaitWaiting := func(what string) {
		t.Helper()
		select {
		case <-waiting:
		case <-time.After(10 * time.Second):
			t.Fatalf("the runner never %s", what)
		}
	}
	awaitWaiting("finished its first tick")
	if len(f.model.calls("")) != 0 {
		t.Fatal("the first tick ran a pass before any compaction")
	}

	f.model.extract(dietAndCity()...)
	f.compact(session, runner)
	awaitWaiting("woke for the flush")
	if items := backgroundItems(f.inbox()); len(items) != 2 || f.cursor(session) != said[1].Sequence || len(f.model.calls("extract")) != 1 {
		t.Fatalf("after the wake: inbox = %+v cursor = %d", items, f.cursor(session))
	}
	cancel()
	if err := <-stopped; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v", err)
	}
}

// The live-turn fence and failure backoff hold a flush back exactly as they
// hold back any pass; the cursor stays until the flush commits.
func TestACompactionFlushWaitsForTheLiveTurnAndHonoursBackoff(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	said := f.fourTurns(session)
	f.compact(session, f.runner)
	lease, err := f.store.AcquireTurnLease(f.ctx, session.ID, "live-turn", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := f.runner.Tick(f.ctx); err != nil || len(result.Runs) != 0 || len(f.model.calls("")) != 0 {
		t.Fatalf("a flush ran while a live turn held the conversation: %+v, %v", result, err)
	}
	if err := f.store.ReleaseTurnLease(f.ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	f.model.script("extract", scriptedReply{err: &ProviderError{Err: errScripted, MayHaveBilled: true}})
	result, err := f.runner.Tick(f.ctx)
	if err != nil || len(result.Runs) != 1 || result.Runs[0].Status != memory.BackgroundRunFailed || f.cursor(session) != 0 {
		t.Fatalf("failed flush = %+v, %v, cursor = %d", result, err, f.cursor(session))
	}
	if due := f.due(); due[session.ID] != "" {
		t.Fatalf("a failed flush was due again before its backoff: %v", due)
	}
	f.offset += f.config.BaseBackoff + time.Second
	if due := f.due(); due[session.ID] != memory.BackgroundTriggerCompaction {
		t.Fatalf("the flush was not due after its backoff: %v", due)
	}
	f.model.extract(dietAndCity()...)
	if result, err := f.runner.Tick(f.ctx); err != nil || len(result.Runs) != 1 || result.Runs[0].Status != memory.BackgroundRunSucceeded {
		t.Fatalf("retried flush = %+v, %v", result, err)
	}
	if len(backgroundItems(f.inbox())) != 2 || f.cursor(session) != said[1].Sequence {
		t.Fatalf("inbox = %+v cursor = %d", f.inbox(), f.cursor(session))
	}
}

// A flush lost in a crash right after the compaction committed is still
// owed after a restart: the committed compaction, not the request, makes
// the conversation due.
func TestAFlushLostInACrashRunsAfterRestart(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	said := f.fourTurns(session)
	f.compact(session, nil)
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f.db, f.store = db, eviedb.NewStore(db)
	f.runner = NewRunner(f.store, f.model, f.config, WithClock(func() time.Time { return time.Now().Add(f.offset) }),
		WithSleep(func(ctx context.Context, d time.Duration) error { return ctx.Err() }),
		WithPricing(Pricing{InputPerMillion: 0.3, OutputPerMillion: 1.2, Source: "test"}))
	f.model.extract(dietAndCity()...)
	result, err := f.runner.Tick(f.ctx)
	if err != nil || len(result.Runs) != 1 || result.Runs[0].Trigger != memory.BackgroundTriggerCompaction ||
		result.Runs[0].ThroughSequence != said[1].Sequence {
		t.Fatalf("restarted tick = %+v, %v", result, err)
	}
	if len(backgroundItems(f.inbox())) != 2 {
		t.Fatalf("inbox = %+v", f.inbox())
	}
}

// A disabled pass, or one paused at today's cap, leaves the compaction
// exactly as it is without a flush, makes no call, and status shows the
// flush it skipped.
func TestADisabledOrCappedPassLeavesCompactionUnchangedAndReportsTheSkippedFlush(t *testing.T) {
	f := newFixtureWith(t, FromEnv(func(string) string { return "" }))
	flushed, plain := f.globalSession(), f.globalSession()
	f.fourTurns(flushed)
	f.fourTurns(plain)
	withFlush := f.compact(flushed, f.runner)
	without := f.compact(plain, nil)
	if withFlush.CoveredFirstSequence != without.CoveredFirstSequence || withFlush.CoveredLastSequence != without.CoveredLastSequence ||
		withFlush.SummarySHA256 != without.SummarySHA256 || withFlush.Generation != without.Generation {
		t.Fatalf("the compaction changed with a disabled flush: %+v vs %+v", withFlush, without)
	}
	if result, err := f.runner.Tick(f.ctx); err != nil || result.Leased || len(f.model.calls("")) != 0 {
		t.Fatalf("a disabled pass ran: %+v, %v", result, err)
	}
	status, err := f.runner.Status(f.ctx)
	if err != nil || status.State != memory.BackgroundStateDisabled ||
		status.Progress.CompactionFlush != (memory.BackgroundCompactionFlush{Conversations: 2, PendingOwnerMessages: 4}) ||
		!strings.Contains(status.CompactionFlushSkipped, "disabled") {
		t.Fatalf("disabled status = %+v, %v", status, err)
	}

	config := testConfig()
	config.DailyTokens = 8_000
	g := newFixtureWith(t, config)
	earlier := g.globalSession()
	g.turn(earlier, "I prefer tea.")
	g.offset = config.Idle + time.Second
	g.model.script("extract", scriptedReply{text: `{"memories":[]}`, usage: [2]int64{7_000, 1_000}})
	if result, err := g.runner.Tick(g.ctx); err != nil || len(result.Runs) != 1 {
		t.Fatalf("spending tick = %+v, %v", result, err)
	}
	session := g.globalSession()
	g.fourTurns(session)
	g.compact(session, g.runner)
	result, err := g.runner.Tick(g.ctx)
	if err != nil || !result.Paused || len(result.Runs) != 0 || len(g.model.calls("")) != 1 {
		t.Fatalf("a capped flush ran: %+v, %v", result, err)
	}
	status, err = g.runner.Status(g.ctx)
	if err != nil || status.State != memory.BackgroundStatePaused ||
		status.Progress.CompactionFlush != (memory.BackgroundCompactionFlush{Conversations: 1, PendingOwnerMessages: 2}) ||
		!strings.Contains(status.CompactionFlushSkipped, "cap") {
		t.Fatalf("capped status = %+v, %v", status, err)
	}
}
