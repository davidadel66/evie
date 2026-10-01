package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func automaticPressureHistory(size int) []memory.Event {
	third := size / 3
	events := completedCompactionTurn("turn-1", 1, strings.Repeat("a", third), "answer")
	events = append(events, completedCompactionTurn("turn-2", 3, strings.Repeat("b", third), "answer")...)
	events = append(events, completedCompactionTurn("turn-3", 5, strings.Repeat("c", size-2*third), "answer")...)
	return events
}

// withByteBudgets pins the original one-token-per-byte estimator, so tests of
// compaction and projection mechanics keep their byte-denominated budgets.
// Calibrated budgets have their own tests in context_budget_test.go.
func withByteBudgets(session *Session) *Session {
	session.composer = NewContextComposer(CanonicalRequestEstimator{})
	return session
}

func automaticTestProfile(t *testing.T, working int64) openrouter.ContextProfile {
	t.Helper()
	profile, err := openrouter.NewExplicitContextProfile("test/model", working, working, 1)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestAutomaticCompactionPressureBoundary(t *testing.T) {
	for _, test := range []struct {
		name string
		size int64
		want bool
	}{
		{name: "just below", size: 79_999, want: false},
		{name: "at", size: 80_000, want: true},
		{name: "above", size: 80_001, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := automaticCompactionRequired(test.size, 100_000); got != test.want {
				t.Fatalf("automaticCompactionRequired(%d)=%v, want %v", test.size, got, test.want)
			}
		})
	}
}

func TestAutomaticCompactionSelectsWholePrefixToSixtyPercent(t *testing.T) {
	profile := automaticTestProfile(t, 230_000)
	// Covering turn-1 alone leaves turn-2 plus the reserved summary above
	// the 60 percent target, so the smallest legal prefix is two turns.
	events := completedCompactionTurn("turn-1", 1, strings.Repeat("a", 85_000), "answer")
	events = append(events, completedCompactionTurn("turn-2", 3, strings.Repeat("b", 100_000), "answer")...)
	events = append(events, completedCompactionTurn("turn-3", 5, strings.Repeat("c", 10_000), "answer")...)
	events = append(events, memory.Event{
		ID: "active", Sequence: 7, Type: memory.EventUserMessage, Role: memory.RoleUser,
		Content: strings.Repeat("d", 10_000),
	})
	input := ContextComposeInput{
		Profile: profile, Events: events, ActiveRootID: "active", TriggerEventID: "active", Iteration: 1,
	}
	plan, required, err := selectAutomaticCompaction(input, NewContextComposer(CanonicalRequestEstimator{}))
	if err != nil {
		t.Fatal(err)
	}
	if !required {
		t.Fatal("pressure did not trigger automatic compaction")
	}
	if plan.CoveredFirst.ID != "turn-1" || plan.CoveredLast.ID != "turn-2-assistant" ||
		plan.FirstRetained.ID != "turn-3" {
		t.Fatalf("plan=%+v", plan)
	}
	input.Summary = &ContextSummary{
		CompactionEventID: "planning", FirstRetainedEventID: plan.FirstRetained.ID,
		Content: planningCompactionSummary(),
	}
	composed, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(input)
	if err != nil {
		t.Fatal(err)
	}
	if composed.Snapshot.SerializedBytes > percentageFloor(composed.Snapshot.WorkingCeilingTokens, automaticCompactionTargetPercent) {
		t.Fatalf("reserved-summary planned request=%d, target=%d", composed.Snapshot.SerializedBytes,
			percentageFloor(composed.Snapshot.WorkingCeilingTokens, automaticCompactionTargetPercent))
	}
}

func TestAutomaticCompactionKeepsFittingActiveTurnUnderPressure(t *testing.T) {
	profile := automaticTestProfile(t, 100_000)
	events := []memory.Event{{
		ID: "active", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser,
		Content: strings.Repeat("x", 81_000),
	}}
	_, required, err := selectAutomaticCompaction(ContextComposeInput{
		Profile: profile, Events: events, ActiveRootID: "active", TriggerEventID: "active", Iteration: 1,
	}, NewContextComposer(CanonicalRequestEstimator{}))
	if required || err != nil {
		t.Fatalf("required=%v error=%v", required, err)
	}
}

func TestAutomaticCompactionNoLegalCutRespectsUsableBoundary(t *testing.T) {
	profile := automaticTestProfile(t, 100_000)
	composer := NewContextComposer(CanonicalRequestEstimator{})
	for _, extra := range []int64{-1, 0, 1} {
		t.Run(fmt.Sprintf("usable%+d", extra), func(t *testing.T) {
			input := ContextComposeInput{
				Profile: profile, ActiveRootID: "active", TriggerEventID: "active", Iteration: 1,
				Events: []memory.Event{{ID: "active", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "x"}},
			}
			prepared, err := composer.prepare(input)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := composer.projectAtStart(input, prepared, prepared.start)
			if err != nil {
				t.Fatal(err)
			}
			input.Events[0].Content += strings.Repeat("x", int(prepared.usable-projection.estimate.SerializedBytes+extra))
			_, required, err := selectAutomaticCompaction(input, composer)
			if extra > 0 {
				if !required || !errors.Is(err, ErrNoLegalAutomaticCompaction) {
					t.Fatalf("required=%v error=%v", required, err)
				}
			} else if required || err != nil {
				t.Fatalf("required=%v error=%v", required, err)
			}
		})
	}
}

func TestAutomaticCompactionRejectsOneUnfitEligibleTurn(t *testing.T) {
	profile := automaticTestProfile(t, 100_000)
	events := completedCompactionTurn("turn-1", 1, strings.Repeat("x", 96_000), "answer")
	events = append(events, memory.Event{
		ID: "active", Sequence: 3, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "continue",
	})
	_, required, err := selectAutomaticCompaction(ContextComposeInput{
		Profile: profile, Events: events, ActiveRootID: "active", TriggerEventID: "active", Iteration: 1,
	}, NewContextComposer(CanonicalRequestEstimator{}))
	if !required || !errors.Is(err, ErrNoLegalAutomaticCompaction) {
		t.Fatalf("required=%v error=%v", required, err)
	}
}

func TestSendAutomaticallyCompactsBeforeConversationAndSnapshotsAcceptedSummary(t *testing.T) {
	history := &fakeHistory{events: automaticPressureHistory(180_000)}
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	conversation := &fakeClient{steps: []step{assistantStep("done", nil)}}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))

	if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(compactor.reqs) != 1 || len(conversation.reqs) != 1 {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	events := history.allEvents()
	var compacted memory.Event
	var snapshot memory.ContextSnapshotPayload
	var compactedIndex, snapshotIndex = -1, -1
	for i, event := range events {
		switch event.Type {
		case memory.EventContextCompacted:
			compacted, compactedIndex = event, i
		case memory.EventContextSnapshot:
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			snapshotIndex = i
		}
	}
	if compactedIndex < 0 || snapshotIndex != compactedIndex+1 {
		t.Fatalf("event ordering=%v", eventTypes(events))
	}
	var payload memory.ContextCompactedPayload
	if err := json.Unmarshal(compacted.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Trigger != memory.ContextCompactionAutomatic || snapshot.ActiveCompactionEventID != compacted.ID ||
		snapshot.RetainedFirstEventID != payload.FirstRetainedEventID || snapshot.CompactionFailureCategory != "" {
		t.Fatalf("compaction=%+v snapshot=%+v", payload, snapshot)
	}
	if len(conversation.reqs[0].Messages) < 3 || conversation.reqs[0].Messages[1].Content != validCompactionSummary() {
		t.Fatalf("conversation request=%+v", conversation.reqs[0])
	}
}

func TestSendProceedsWithStableAutomaticFailureCategoryWhenUnchangedRequestFits(t *testing.T) {
	for _, test := range []struct {
		name     string
		step     step
		category memory.ContextCompactionFailureCategory
	}{
		{name: "transport", step: step{err: errors.New("summary unavailable")}, category: memory.ContextCompactionSummaryProvider},
		{name: "invalid", step: assistantStep("not a valid summary", nil), category: memory.ContextCompactionSummaryInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistory{events: automaticPressureHistory(190_000)}
			compactor := &fakeClient{steps: []step{test.step}}
			conversation := &fakeClient{steps: []step{assistantStep("done", nil)}}
			session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
				memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
			if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			if len(compactor.reqs) != 1 || len(conversation.reqs) != 1 {
				t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
			}
			var snapshot memory.ContextSnapshotPayload
			for _, event := range history.allEvents() {
				if event.Type == memory.EventContextSnapshot {
					if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
						t.Fatal(err)
					}
				}
				if event.Type == memory.EventContextCompacted {
					t.Fatalf("failed summary became durable: %+v", event)
				}
			}
			if snapshot.CompactionFailureCategory != test.category || snapshot.ActiveCompactionEventID != "" {
				t.Fatalf("snapshot=%+v", snapshot)
			}
		})
	}
}

func TestSendAutomaticFailureKeepsPriorAcceptedSummaryActive(t *testing.T) {
	turn1 := completedCompactionTurn("turn-1", 1, "old", "old answer")
	turn2 := completedCompactionTurn("turn-2", 3, strings.Repeat("b", 90_000), "answer two")
	priorSummary := strings.ReplaceAll(validCompactionSummary(), "kept", "prior accepted continuity")
	prior := contextCompactionEvent(
		t, "compaction-1", 5, 1, "", turn1[0], turn1[1], turn2[0], priorSummary,
		"old/model", CompactionPromptVersion,
	)
	turn3 := completedCompactionTurn("turn-3", 6, strings.Repeat("c", 90_000), "answer three")
	events := append(append(append([]memory.Event{}, turn1...), turn2...), prior)
	events = append(events, turn3...)
	history := &fakeHistory{events: events}
	compactor := &fakeClient{steps: []step{assistantStep("generated but invalid", nil)}}
	conversation := &fakeClient{steps: []step{assistantStep("done", nil)}}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(conversation.reqs) != 1 || len(conversation.reqs[0].Messages) < 3 ||
		conversation.reqs[0].Messages[1].Content != priorSummary {
		t.Fatalf("conversation request=%+v", conversation.reqs)
	}
	for _, message := range conversation.reqs[0].Messages {
		if strings.Contains(message.Content, "generated but invalid") {
			t.Fatalf("unaccepted summary reached conversation: %+v", conversation.reqs[0])
		}
	}
	var snapshot memory.ContextSnapshotPayload
	for _, event := range history.allEvents() {
		if event.Type == memory.EventContextSnapshot {
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
		}
	}
	if snapshot.ActiveCompactionEventID != prior.ID ||
		snapshot.CompactionFailureCategory != memory.ContextCompactionSummaryInvalid {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestSendMapsTerminalAutomaticCompactionFailureBeforeConversation(t *testing.T) {
	for _, test := range []struct {
		name           string
		step           step
		classification memory.TurnClassification
	}{
		{name: "transport", step: step{err: errors.New("summary unavailable")}, classification: memory.ClassificationProviderError},
		{name: "invalid", step: assistantStep("not a valid summary", nil), classification: memory.ClassificationProviderResponseInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistory{events: automaticPressureHistory(210_000)}
			compactor := &fakeClient{steps: []step{test.step}}
			conversation := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
			session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
				memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
			if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); err == nil {
				t.Fatal("Send unexpectedly succeeded")
			}
			if len(compactor.reqs) != 1 || len(conversation.reqs) != 0 {
				t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
			}
			var terminal memory.TurnTerminalPayload
			last := history.allEvents()[len(history.allEvents())-1]
			if err := json.Unmarshal(last.Payload, &terminal); err != nil {
				t.Fatal(err)
			}
			if last.Type != memory.EventTurnFailed || terminal.Classification != test.classification ||
				terminal.Stage != memory.StageContextCompaction {
				t.Fatalf("terminal event=%+v payload=%+v", last, terminal)
			}
		})
	}
}

func TestSendAttemptsAutomaticCompactionAgainOnPostToolIteration(t *testing.T) {
	// Leave room for the parent instructions so only the tool result triggers compaction.
	history := &fakeHistory{events: automaticPressureHistory(170_000)}
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	conversation := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "large", `{}`)),
		assistantStep("done", nil),
	}}
	large := echoTool("large", false, nil)
	large.Execute = func(context.Context, string) (string, error) { return strings.Repeat("z", 25_000), nil }
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 250_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	if err := session.Send(context.Background(), "run it", &recorder{}, nil, large); err != nil {
		t.Fatal(err)
	}
	if len(conversation.reqs) != 2 || len(compactor.reqs) != 1 {
		t.Fatalf("conversation requests=%d compactor requests=%d events=%v", len(conversation.reqs), len(compactor.reqs), eventTypes(history.allEvents()))
	}
	var snapshotIterations []int
	for _, event := range history.allEvents() {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.ActiveCompactionEventID != "" {
			snapshotIterations = append(snapshotIterations, snapshot.Iteration)
		}
	}
	if fmt.Sprint(snapshotIterations) != "[2]" {
		t.Fatalf("automatic compaction snapshot iterations=%v", snapshotIterations)
	}
}

func TestSendContinuesAfterLargeToolGroupWhenNoCompactionTargetFits(t *testing.T) {
	history := &fakeHistory{events: automaticPressureHistory(60_000)}
	compactor := &fakeClient{}
	conversation := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("page-1", "first_page", `{}`), toolCall("page-2", "second_page", `{}`)),
		assistantStep("done", nil),
	}}
	first := echoTool("first_page", false, nil)
	first.Execute = func(context.Context, string) (string, error) { return strings.Repeat("a", 102_000), nil }
	second := echoTool("second_page", false, nil)
	second.Execute = func(context.Context, string) (string, error) { return strings.Repeat("b", 77_800), nil }
	profile, err := openrouter.NewExplicitContextProfile("test/model", 1_000_000, 262_144, 16_384)
	if err != nil {
		t.Fatal(err)
	}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, profile, history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	if err := session.Send(context.Background(), "read both pages", &recorder{}, nil, first, second); err != nil {
		t.Fatal(err)
	}
	if len(conversation.reqs) != 2 || len(compactor.reqs) != 0 {
		t.Fatalf("conversation requests=%d compactor requests=%d", len(conversation.reqs), len(compactor.reqs))
	}
	var snapshots []memory.ContextSnapshotPayload
	var durableResultBytes []int
	for _, event := range history.allEvents() {
		switch event.Type {
		case memory.EventContextSnapshot:
			var snapshot memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			snapshots = append(snapshots, snapshot)
		case memory.EventToolSucceeded:
			durableResultBytes = append(durableResultBytes, len(event.Content))
		case memory.EventContextCompacted, memory.EventTurnFailed:
			t.Fatalf("unexpected event: %s", event.Type)
		}
	}
	if len(snapshots) != 2 || fmt.Sprint(durableResultBytes) != "[102000 77800]" {
		t.Fatalf("snapshots=%d durable result bytes=%v", len(snapshots), durableResultBytes)
	}
	for i, snapshot := range snapshots {
		if snapshot.RetainedFirstEventID != "turn-1" || snapshot.ActiveCompactionEventID != "" || snapshot.CompactionFailureCategory != "" {
			t.Fatalf("iteration %d changed history frontier or recorded a compactor failure", i+1)
		}
		estimate, err := (CanonicalRequestEstimator{}).Estimate(conversation.reqs[i])
		if err != nil {
			t.Fatal(err)
		}
		if estimate.RequestSHA256 != snapshot.RequestSHA256 || estimate.SerializedBytes > snapshot.UsableInputBytes {
			t.Fatalf("iteration %d request does not match its safe snapshot", i+1)
		}
		pressured := automaticCompactionRequired(snapshot.SerializedBytes, snapshot.WorkingCeilingTokens)
		if pressured != (i == 1) {
			t.Fatalf("iteration %d pressure=%v bytes=%d", i+1, pressured, snapshot.SerializedBytes)
		}
	}
	var toolResults int
	for _, message := range conversation.reqs[1].Messages {
		if message.Role == "tool" {
			toolResults++
		}
	}
	if toolResults != 2 || len(snapshots[1].Placeholders) == 0 {
		t.Fatalf("projected tool results=%d placeholders=%d", toolResults, len(snapshots[1].Placeholders))
	}
}

// L6: a failed automatic compaction is remembered for the rest of its turn.
// Later pressured iterations that still fit proceed without calling the
// compactor again, and their snapshots keep the remembered failure category.
// The next turn starts fresh and may attempt compaction again.
func TestSendDoesNotRetryFailedAutomaticCompactionWithinTurn(t *testing.T) {
	history := &fakeHistory{events: automaticPressureHistory(190_000)}
	compactor := &fakeClient{steps: []step{
		{err: errors.New("summary unavailable one")},
		{err: errors.New("summary unavailable two")},
	}}
	conversation := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
		assistantStep("", nil, toolCall("call-2", "echo", `{}`)),
		assistantStep("done", nil),
		assistantStep("done again", nil),
	}}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))

	if err := session.Send(context.Background(), "repeat it", &recorder{}, nil, echoTool("echo", false, nil)); err != nil {
		t.Fatal(err)
	}
	if len(conversation.reqs) != 3 || len(compactor.reqs) != 1 {
		t.Fatalf("conversation requests=%d compactor attempts=%d", len(conversation.reqs), len(compactor.reqs))
	}
	var failureIterations []int
	for _, event := range history.allEvents() {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.CompactionFailureCategory != memory.ContextCompactionSummaryProvider {
			t.Fatalf("snapshot=%+v", snapshot)
		}
		failureIterations = append(failureIterations, snapshot.Iteration)
	}
	if fmt.Sprint(failureIterations) != "[1 2 3]" {
		t.Fatalf("failure snapshot iterations=%v", failureIterations)
	}

	if err := session.Send(context.Background(), "next turn", &recorder{}, nil, echoTool("echo", false, nil)); err != nil {
		t.Fatal(err)
	}
	if len(conversation.reqs) != 4 || len(compactor.reqs) != 2 {
		t.Fatalf("next turn: conversation requests=%d compactor attempts=%d", len(conversation.reqs), len(compactor.reqs))
	}
}

// L6: when a later iteration no longer fits after the turn's compaction
// failed, the turn fails with the remembered failure instead of calling the
// compactor again.
func TestSendFailsWithRememberedCompactionFailureWhenLaterIterationNoLongerFits(t *testing.T) {
	history := &fakeHistory{events: automaticPressureHistory(330_000)}
	compactor := &fakeClient{steps: []step{
		{err: errors.New("summary unavailable one")},
		{err: errors.New("summary unavailable two")},
	}}
	conversation := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "large", `{}`)),
		assistantStep("must not run", nil),
	}}
	large := echoTool("large", false, nil)
	large.Execute = func(context.Context, string) (string, error) { return strings.Repeat("z", 70_000), nil }
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 400_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))

	err := session.Send(context.Background(), "run it", &recorder{}, nil, large)
	if err == nil {
		t.Fatal("Send unexpectedly succeeded")
	}
	if len(conversation.reqs) != 1 || len(compactor.reqs) != 1 {
		t.Fatalf("conversation requests=%d compactor attempts=%d events=%v", len(conversation.reqs), len(compactor.reqs), eventTypes(history.allEvents()))
	}
	events := history.allEvents()
	last := events[len(events)-1]
	terminal := terminalPayloadOf(t, last)
	if last.Type != memory.EventTurnFailed || terminal.Classification != memory.ClassificationProviderError ||
		terminal.Stage != memory.StageContextCompose {
		t.Fatalf("terminal event=%+v payload=%+v", last, terminal)
	}
	if err := terminal.Validate(last.Type); err != nil {
		t.Fatal(err)
	}
}

func TestSendRecordsRecoverableAutomaticSummaryPersistenceFailure(t *testing.T) {
	history := &fakeHistory{
		events: automaticPressureHistory(190_000), appendErrAt: 2, appendErr: errors.New("disk full"),
	}
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	conversation := &fakeClient{steps: []step{assistantStep("done", nil)}}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(compactor.reqs) != 1 || len(conversation.reqs) != 1 {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	for _, event := range history.allEvents() {
		if event.Type == memory.EventContextCompacted {
			t.Fatalf("unpersisted summary became active: %+v", event)
		}
		if event.Type == memory.EventContextSnapshot {
			var snapshot memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.CompactionFailureCategory != memory.ContextCompactionSummaryPersistence {
				t.Fatalf("snapshot=%+v", snapshot)
			}
		}
	}
}

func TestSendCancellationBeforeAutomaticCompactionCommitKeepsOldSummary(t *testing.T) {
	history := &fakeHistory{events: automaticPressureHistory(190_000)}
	compactor := &cancellingCompactor{entered: make(chan struct{})}
	session := withByteBudgets(NewWithCompactor(&fakeClient{}, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Send(ctx, strings.Repeat("d", 8_000), &recorder{}, nil) }()
	<-compactor.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v, want context canceled", err)
	}
	events := history.allEvents()
	for _, event := range events {
		if event.Type == memory.EventContextCompacted || event.Type == memory.EventContextSnapshot {
			t.Fatalf("cancelled compaction committed later evidence: %v", eventTypes(events))
		}
	}
	var terminal memory.TurnTerminalPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &terminal); err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].Type != memory.EventTurnInterrupted || terminal.Stage != memory.StageContextCompaction {
		t.Fatalf("terminal=%+v event=%+v", terminal, events[len(events)-1])
	}
}

func TestSendCancellationAfterAutomaticCompactionCommitKeepsSummaryAtCompose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	history := &fakeHistory{events: automaticPressureHistory(190_000)}
	history.afterAppend = func(input memory.EventInput) {
		if input.Type == memory.EventContextCompacted {
			cancel()
		}
	}
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	conversation := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	if err := session.Send(ctx, strings.Repeat("d", 8_000), &recorder{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v, want context canceled", err)
	}
	if len(conversation.reqs) != 0 {
		t.Fatalf("conversation requests=%d", len(conversation.reqs))
	}
	events := history.allEvents()
	var compacted memory.Event
	for _, event := range events {
		if event.Type == memory.EventContextCompacted {
			compacted = event
		}
		if event.Type == memory.EventContextSnapshot {
			t.Fatalf("snapshot committed after cancellation: %v", eventTypes(events))
		}
	}
	if compacted.ID == "" {
		t.Fatalf("accepted compaction missing: %v", eventTypes(events))
	}
	var terminal memory.TurnTerminalPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Stage != memory.StageContextCompose {
		t.Fatalf("terminal=%+v", terminal)
	}
}

func TestSendCancellationAfterAutomaticSnapshotKeepsSnapshotDurable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	history := &fakeHistory{events: automaticPressureHistory(190_000)}
	history.afterAppend = func(input memory.EventInput) {
		if input.Type == memory.EventContextSnapshot {
			cancel()
		}
	}
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	conversation := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	if err := session.Send(ctx, strings.Repeat("d", 8_000), &recorder{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v, want context canceled", err)
	}
	if len(conversation.reqs) != 0 {
		t.Fatalf("conversation requests=%d", len(conversation.reqs))
	}
	var compactedID memory.EventID
	var snapshot memory.ContextSnapshotPayload
	var terminal memory.TurnTerminalPayload
	for _, event := range history.allEvents() {
		switch event.Type {
		case memory.EventContextCompacted:
			compactedID = event.ID
		case memory.EventContextSnapshot:
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
		case memory.EventTurnInterrupted:
			if err := json.Unmarshal(event.Payload, &terminal); err != nil {
				t.Fatal(err)
			}
		}
	}
	if compactedID == "" || snapshot.ActiveCompactionEventID != compactedID ||
		terminal.Classification != memory.ClassificationCallerCancelled || terminal.Stage != memory.StageProvider {
		t.Fatalf("compaction=%q snapshot=%+v terminal=%+v events=%v", compactedID, snapshot, terminal, eventTypes(history.allEvents()))
	}
}

func TestSendLeaseLossFencesAutomaticCompactionAndLaterWrites(t *testing.T) {
	history := &fakeHistory{events: automaticPressureHistory(190_000)}
	initialEvents := len(history.events)
	conversation := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	compactor := &fakeClient{steps: []step{assistantStep(validCompactionSummary(), nil)}}
	owner := &scriptedOwner{authorizeErrAt: 1, authorizeErr: errFakeLeaseLost}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, owner))
	if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Send error=%v, want lease loss", err)
	}
	if len(compactor.reqs) != 0 || len(conversation.reqs) != 0 {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	for _, event := range history.allEvents()[initialEvents:] {
		if event.Type == memory.EventContextCompacted || event.Type == memory.EventContextSnapshot ||
			event.Type == memory.EventAssistantMessage || event.Type == memory.EventTurnFailed ||
			event.Type == memory.EventTurnInterrupted {
			t.Fatalf("lease loss allowed a fenced write: %+v", event)
		}
	}
}

func TestSendNoLegalAutomaticCutRecordsSafeContextOverflow(t *testing.T) {
	history := &fakeHistory{}
	conversation := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	session := withByteBudgets(NewWithCompactor(conversation, conversation, automaticTestProfile(t, 100_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	err := session.Send(context.Background(), strings.Repeat("x", 96_000), &recorder{}, nil)
	if !errors.Is(err, ErrContextOverflow) || len(conversation.reqs) != 0 {
		t.Fatalf("Send error=%v conversation requests=%d", err, len(conversation.reqs))
	}
	events := history.allEvents()
	last := events[len(events)-1]
	var terminal memory.TurnTerminalPayload
	if err := json.Unmarshal(last.Payload, &terminal); err != nil {
		t.Fatal(err)
	}
	if last.Content != terminal.SafeContent() || terminal.Classification != memory.ClassificationContextOverflow ||
		terminal.Stage != memory.StageContextCompose {
		t.Fatalf("terminal event=%+v payload=%+v", last, terminal)
	}
}

func eventTypes(events []memory.Event) []memory.EventType {
	types := make([]memory.EventType, len(events))
	for i := range events {
		types[i] = events[i].Type
	}
	return types
}
