package memorybackground

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func (f *fixture) due() map[memory.SessionID]memory.BackgroundTrigger {
	f.t.Helper()
	items, err := f.store.DueMemoryBackgroundSessions(f.ctx, memory.BackgroundDueQuery{Now: f.runner.now(), Idle: f.config.Idle, Turns: f.config.Turns, Limit: 10})
	if err != nil {
		f.t.Fatal(err)
	}
	due := map[memory.SessionID]memory.BackgroundTrigger{}
	for _, item := range items {
		due[item.SessionID] = item.Trigger
	}
	return due
}

// Triggers and cursor: a conversation is due once it has been idle for the
// idle window with new owner messages, or once it has N new owner messages
// even while active; never while a live turn holds it; and only for owner
// messages after its durable cursor.
func TestPassIsDueAtIdleOrEveryNOwnerTurnsSinceTheCursor(t *testing.T) {
	config := testConfig()
	config.Turns = 3
	f := newFixtureWith(t, config)
	quiet := f.globalSession()
	busy := f.globalSession()
	f.turn(quiet, "I prefer tea.", "Noted, tea.")
	for i := range 3 {
		f.turn(busy, fmt.Sprintf("message %d", i))
	}
	if due := f.due(); len(due) != 1 || due[busy.ID] != memory.BackgroundTriggerTurns {
		t.Fatalf("before the idle window: %v", due)
	}
	f.offset = config.Idle + time.Second
	if due := f.due(); due[quiet.ID] != memory.BackgroundTriggerIdle || due[busy.ID] != memory.BackgroundTriggerTurns {
		t.Fatalf("after the idle window: %v", due)
	}
	// A live turn holds the conversation: not due.
	lease, err := f.store.AcquireTurnLease(f.ctx, quiet.ID, "live-turn", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if due := f.due(); due[quiet.ID] != "" {
		t.Fatalf("a conversation with a live turn was due: %v", due)
	}
	if err := f.store.ReleaseTurnLease(f.ctx, quiet.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	// After a pass the cursor covers the batch; only new owner messages count.
	f.model.extract()
	f.mustRun(quiet)
	if due := f.due(); due[quiet.ID] != "" {
		t.Fatalf("a covered conversation was due again: %v", due)
	}
	f.turn(quiet, "Also, I'm a night owl.")
	if due := f.due(); due[quiet.ID] != memory.BackgroundTriggerIdle {
		t.Fatalf("a new owner message did not make the conversation due: %v", due)
	}
	f.model.extract(memoryItem(0, "profile_fact", "profile", "sleep_pattern", "sleep pattern", "night owl"))
	f.mustRun(quiet)
	calls := f.model.calls("extract")
	if last := calls[len(calls)-1].User; strings.Contains(last, "I prefer tea.") || !strings.Contains(last, "night owl") {
		t.Fatalf("the second pass reread covered messages: %s", last)
	}
}

// Conversations created after activation are read from their start;
// delegated child conversations are never read: their "user" messages are
// the orchestrator's task text, not the owner's words.
func TestOnlyRootConversationsAreRead(t *testing.T) {
	f := newFixture(t)
	root := f.globalSession()
	f.turn(root, "A message after activation.")
	f.offset = f.config.Idle + time.Second
	if due := f.due(); due[root.ID] != memory.BackgroundTriggerIdle {
		t.Fatalf("a conversation created after activation must be read from its start: %v", due)
	}
	child, err := f.store.CreateDelegatedSessionWithComposition(f.ctx, root.ID, fixtureReceipt())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(f.ctx, child.ID, "worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AppendEventWithLease(f.ctx, child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "delegated task text"}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseTurnLease(f.ctx, child.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if due := f.due(); due[child.ID] != "" {
		t.Fatalf("a delegated child conversation was due: %v", due)
	}
	if _, err := f.store.MemoryBackgroundBatch(f.ctx, child.ID, memory.BackgroundBatchLimits{MaxMessages: 5, MaxBytes: 1000}); err == nil {
		t.Fatal("a delegated child conversation was readable")
	}
}

// An archived Workspace's conversations are not read, and a pass already
// running when the Workspace was archived queues nothing and keeps the
// cursor; after a restore the same messages are read again
// (workspaces.decisions.md, 2026-10-11).
func TestArchivedWorkspaceConversationsAreNotReadAndQueueNothing(t *testing.T) {
	f := newFixture(t)
	session := f.workspaceSession("Old project")
	f.turn(session, "I'm a vegetarian.")
	start := f.cursor(session)
	f.offset = f.config.Idle + time.Second
	if due := f.due(); due[session.ID] != memory.BackgroundTriggerIdle {
		t.Fatalf("before the archive: %v", due)
	}
	if _, err := f.store.ArchiveWorkspace(f.ctx, session.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if due := f.due(); due[session.ID] != "" {
		t.Fatalf("a conversation in an archived Workspace was due: %v", due)
	}
	f.model.extract(memoryItem(0, "profile_fact", "profile", "diet", "diet", "vegetarian"))
	run, err := f.run(session)
	if !errors.Is(err, eviedb.ErrMemoryProposalWorkspaceArchived) || run.Status != memory.BackgroundRunFailed ||
		f.cursor(session) != start || f.inbox().Pending != 0 {
		t.Fatalf("pass in flight at the archive: run = %+v err = %v cursor = %d", run, err, f.cursor(session))
	}
	if _, err := f.store.RestoreWorkspace(f.ctx, session.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	f.model.extract(memoryItem(0, "profile_fact", "profile", "diet", "diet", "vegetarian"))
	f.mustRun(session)
	if f.inbox().Pending != 1 {
		t.Fatalf("inbox after the restore = %+v", f.inbox())
	}
}

func TestActivationStartsExistingConversationsAtTheirLatestEvent(t *testing.T) {
	f := newFixture(t)
	db := f.store
	session := f.globalSession()
	f.turn(session, "Written before the pass existed.")
	// Reset activation to model a database that predates the pass.
	if err := resetActivationForTest(f); err != nil {
		t.Fatal(err)
	}
	if _, err := db.EnsureMemoryBackgroundActivation(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.offset = f.config.Idle + time.Second
	if due := f.due(); due[session.ID] != "" {
		t.Fatalf("history from before activation was selected: %v", due)
	}
	f.turn(session, "Written after.")
	f.model.extract()
	f.mustRun(session)
	if user := f.model.calls("extract")[0].User; strings.Contains(user, "before the pass") || !strings.Contains(user, "Written after.") {
		t.Fatalf("activation frontier ignored: %s", user)
	}
}

// Budgets: a pass whose call would pass the per-pass budget or today's cap
// makes no model call; the tick reports paused once the cap is reached.
func TestCapsStopModelCallsBeforeTheyHappen(t *testing.T) {
	config := testConfig()
	// Any extraction call reserves its whole 4,000-token output allowance
	// plus its input, so this cap leaves no room for one.
	config.DailyTokens = 4_100
	f := newFixtureWith(t, config)
	session := f.globalSession()
	f.turn(session, strings.Repeat("I like long walks on the beach. ", 20))
	f.offset = config.Idle + time.Second
	f.model.extract()
	result, err := f.runner.Tick(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Paused || len(f.model.calls("")) != 0 {
		t.Fatalf("the daily cap did not stop the call: %+v, calls = %d", result, len(f.model.calls("")))
	}
	status, err := f.runner.Status(f.ctx)
	if err != nil || status.State != memory.BackgroundStateOn || status.RemainingTokens != 4_100 {
		t.Fatalf("status = %+v, %v", status, err)
	}

	small := testConfig()
	small.PassTokens = 1_000
	g := newFixtureWith(t, small)
	other := g.globalSession()
	g.turn(other, "I prefer tea.")
	run, err := g.run(other)
	if err == nil || !strings.Contains(err.Error(), "pass budget") || len(g.model.calls("")) != 0 || run.Status != memory.BackgroundRunFailed {
		t.Fatalf("pass budget: run = %+v err = %v", run, err)
	}
}

func TestSpendReachingTheDailyCapPausesTheNextTick(t *testing.T) {
	config := testConfig()
	config.DailyTokens = 8_000
	f := newFixtureWith(t, config)
	session := f.globalSession()
	f.turn(session, "I prefer tea.")
	f.offset = config.Idle + time.Second
	f.model.script("extract", scriptedReply{text: `{"memories":[]}`, usage: [2]int64{7_000, 1_000}})
	if result, err := f.runner.Tick(f.ctx); err != nil || len(result.Runs) != 1 || result.Paused {
		t.Fatalf("first tick = %+v, %v", result, err)
	}
	f.turn(session, "I also prefer green tea.")
	f.offset = 2*config.Idle + time.Second
	result, err := f.runner.Tick(f.ctx)
	if err != nil || !result.Paused || len(result.Runs) != 0 {
		t.Fatalf("a tick at the cap = %+v, %v", result, err)
	}
	status, err := f.runner.Status(f.ctx)
	if err != nil || status.State != memory.BackgroundStatePaused || status.RemainingTokens != 0 || status.Progress.Today.Tokens() != 8_000 {
		t.Fatalf("status = %+v, %v", status, err)
	}
}

// Live-turn fence: a commit waits for the conversation's turn lease; when a
// turn outlasts the wait the run is deferred and the cursor does not move.
func TestCommitNeverWritesWhileALiveTurnHoldsTheConversation(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	f.turn(session, "I'm a vegetarian.")
	start := f.cursor(session)
	lease, err := f.store.AcquireTurnLease(f.ctx, session.ID, "live-turn", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.model.extract(memoryItem(0, "profile_fact", "profile", "diet", "diet", "vegetarian"))
	run, err := f.run(session)
	if err == nil || run.Status != memory.BackgroundRunDeferred || f.cursor(session) != start || f.inbox().Pending != 0 {
		t.Fatalf("run = %+v err = %v cursor = %d", run, err, f.cursor(session))
	}
	if err := f.store.ReleaseTurnLease(f.ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	f.model.extract(memoryItem(0, "profile_fact", "profile", "diet", "diet", "vegetarian"))
	f.mustRun(session)
	if f.inbox().Pending != 1 {
		t.Fatalf("inbox after the turn ended = %+v", f.inbox())
	}
}

// Crash recovery: a pass whose commit never happened leaves the cursor in
// place, and the rerun proposes the identical change once.
func TestCrashBeforeCommitIsRecoveredByTheDurableCursor(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	f.turn(session, "I'm left-handed.")
	start := f.cursor(session)
	release := make(chan struct{})
	f.model.script("extract", scriptedReply{text: `{"memories":[{"message":0,"kind":"profile_fact","applies":"profile","predicate":"handedness","label":"handedness","cardinality":"one","value_type":"text","value":"left-handed","polarity":"affirmed"}]}`, wait: release, usage: [2]int64{500, 50}})
	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() {
		_, err := f.runner.runSession(ctx, session.ID, memory.BackgroundTriggerIdle, 0)
		done <- err
	}()
	cancel() // shutdown while the model call is in flight
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled pass = %v", err)
	}
	close(release)
	if f.cursor(session) != start || f.inbox().Pending != 0 {
		t.Fatal("a cancelled pass committed")
	}
	// The same batch runs again after restart.
	for range 2 {
		f.model.extract(memoryItem(0, "profile_fact", "profile", "handedness", "handedness", "left-handed"))
	}
	first := f.mustRun(session)
	// A second runner that read the same batch before the first committed
	// cannot commit it again (cursor compare-and-set) nor queue it twice.
	stale := memory.BackgroundRun{RunID: "stale", SessionID: session.ID, Trigger: memory.BackgroundTriggerIdle, Model: "fake", FromSequence: start,
		ThroughSequence: first.ThroughSequence, StartedAt: time.Now(), FinishedAt: time.Now()}
	lease, err := f.store.AcquireTurnLease(f.ctx, session.ID, "stale-runner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CommitMemoryBackgroundPass(f.ctx, lease, memory.BackgroundCommit{Run: stale}); !errors.Is(err, memory.ErrBackgroundCursorMoved) {
		t.Fatalf("stale commit = %v", err)
	}
	_ = f.store.ReleaseTurnLease(f.ctx, session.ID, lease.HolderID, lease.FencingToken)
	if f.inbox().Pending != 1 {
		t.Fatalf("inbox = %+v", f.inbox())
	}
}

// Provider robustness: a failure that proved nothing was generated is
// retried with backoff; anything else fails the pass, keeps the cursor and
// backs the conversation off.
func TestProviderFailuresRetryOnlyWhenNothingWasGenerated(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	f.turn(session, "I'm a morning person.")
	f.model.script("extract",
		scriptedReply{err: &ProviderError{Err: errScripted, Retryable: true}},
		scriptedReply{text: `{"memories":[]}`, usage: [2]int64{100, 5}},
	)
	if run := f.mustRun(session); run.InputTokens != 100 || len(f.slept) != 1 {
		t.Fatalf("run = %+v slept = %v", run, f.slept)
	}
	f.turn(session, "And I hate mornings now.")
	start := f.cursor(session)
	f.offset = f.config.Idle + time.Second
	f.model.script("extract", scriptedReply{err: &ProviderError{Err: errScripted, MayHaveBilled: true}})
	run, err := f.run(session)
	if err == nil || run.Status != memory.BackgroundRunFailed || run.Malformed || f.cursor(session) != start || run.InputTokens == 0 {
		t.Fatalf("run = %+v err = %v", run, err)
	}
	if due := f.due(); due[session.ID] != "" {
		t.Fatalf("a failed conversation was due again before its backoff: %v", due)
	}
	f.offset += f.config.BaseBackoff + time.Second
	if due := f.due(); due[session.ID] != memory.BackgroundTriggerIdle {
		t.Fatalf("the backoff did not end: %v", due)
	}
}

// Disabled states: a disabled runner makes no calls and reports why.
func TestDisabledRunnerMakesNoCallsAndReportsWhy(t *testing.T) {
	config := FromEnv(func(string) string { return "" })
	f := newFixtureWith(t, config)
	session := f.globalSession()
	f.turn(session, "I prefer tea.")
	f.offset = time.Hour
	result, err := f.runner.Tick(f.ctx)
	if err != nil || result.Leased || len(f.model.calls("")) != 0 {
		t.Fatalf("a disabled runner ran: %+v, %v", result, err)
	}
	status, err := f.runner.Status(f.ctx)
	if err != nil || status.Enabled || status.State != memory.BackgroundStateDisabled || !strings.Contains(status.Reason, "EVIE_REMOTE_MEMORY=on") || status.Model != "" {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if _, err := f.runner.PassThrough(f.ctx, session.ID, 1); err == nil {
		t.Fatal("a disabled runner accepted a compaction flush")
	}
}

// Stage 10 seam: PassThrough reads exactly the owner messages up to the
// frontier about to be compacted, ignoring the idle and turn triggers.
func TestPassThroughCoversExactlyTheFrontierAboutToBeCompacted(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	first := f.turn(session, "I'm vegetarian.", "Noted.")
	f.turn(session, "Later: I moved to Denver.")
	f.model.extract()
	runs, err := f.runner.PassThrough(f.ctx, session.ID, first.Sequence+1)
	if err != nil || len(runs) != 1 || runs[0].Trigger != memory.BackgroundTriggerCompaction || runs[0].ThroughSequence != first.Sequence {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	if user := f.model.calls("extract")[0].User; !strings.Contains(user, "vegetarian") || strings.Contains(user, "Denver") {
		t.Fatalf("the flush read past its frontier: %s", user)
	}
}
