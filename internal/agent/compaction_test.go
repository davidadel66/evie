package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func completedCompactionTurn(rootID string, sequence int64, user, assistant string) []memory.Event {
	return []memory.Event{
		{ID: memory.EventID(rootID), Sequence: sequence, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: user},
		{ID: memory.EventID(rootID + "-assistant"), Sequence: sequence + 1, ParentID: memory.EventID(rootID), Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: assistant, Payload: json.RawMessage(`{}`)},
	}
}

func TestSessionCompactPersistsAcceptedGenerationAndUsesItForLaterConversation(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	history := &fakeHistory{events: events}
	compactorResponse := openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}
	compactor := &fakeClient{steps: []step{{res: compactorResponse}}}
	conversation := &fakeClient{steps: []step{assistantStep("continued", nil)}}
	session := NewWithCompactor(
		conversation, compactor, testContextProfile("configured/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner(),
	)

	result, err := session.Compact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.CompactionEventID == "" || len(compactor.reqs) != 1 {
		t.Fatalf("result=%+v requests=%d", result, len(compactor.reqs))
	}
	request := compactor.reqs[0]
	if request.Model != "configured/model" || !request.Stream || request.MaxTokens != 4096 ||
		request.Reasoning != nil || request.Temperature == nil || *request.Temperature != 0 || len(request.Tools) != 0 {
		t.Fatalf("compactor request=%+v", request)
	}
	var compacted memory.Event
	for _, event := range history.allEvents() {
		if event.Type == memory.EventContextCompacted {
			compacted = event
		}
	}
	if compacted.ID != result.CompactionEventID || compacted.Content != validCompactionSummary() ||
		compacted.ParentID != "" || compacted.Role != "" || compacted.ExecutionID != "" {
		t.Fatalf("compaction event=%+v", compacted)
	}
	var payload memory.ContextCompactedPayload
	if err := json.Unmarshal(compacted.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Generation != 1 || payload.Trigger != memory.ContextCompactionManual ||
		payload.CoveredFirstEventID != "turn-1" || payload.CoveredLastEventID != "turn-1-assistant" ||
		payload.FirstRetainedEventID != "turn-2" || payload.CanonicalModel != "configured/model" ||
		payload.PromptVersion != CompactionPromptVersion {
		t.Fatalf("payload=%+v", payload)
	}
	if _, err := session.Compact(context.Background()); !errors.Is(err, ErrCompactionChainPending) {
		t.Fatalf("second Compact error=%v, want ErrCompactionChainPending", err)
	}
	if len(compactor.reqs) != 1 {
		t.Fatalf("second Compact made a provider call; requests=%d", len(compactor.reqs))
	}

	if err := session.Send(context.Background(), "four", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(conversation.reqs) != 1 {
		t.Fatalf("conversation requests=%d", len(conversation.reqs))
	}
	messages := conversation.reqs[0].Messages
	if len(messages) < 3 || messages[1].Content != validCompactionSummary() {
		t.Fatalf("conversation messages=%+v", messages)
	}
	for _, message := range messages {
		if strings.Contains(message.Content, "answer one") || message.Content == "one" {
			t.Fatalf("covered turn leaked into conversational request: %+v", messages)
		}
	}
}

func TestSessionCompactNothingEligibleCallsNoProviderAndWritesNoEvent(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	history := &fakeHistory{events: events}
	compactor := &fakeClient{}
	session := NewWithCompactor(compactor, compactor, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())

	_, err := session.Compact(context.Background())
	if !errors.Is(err, ErrNothingEligibleForCompaction) {
		t.Fatalf("error=%v", err)
	}
	if len(compactor.reqs) != 0 || history.appendAttempts != 0 {
		t.Fatalf("provider requests=%d append attempts=%d", len(compactor.reqs), history.appendAttempts)
	}
}

func TestSessionCompactMapsProviderAndMalformedResponseFailuresWithoutWriting(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	tests := []struct {
		name           string
		step           step
		classification memory.TurnClassification
	}{
		{name: "transport", step: step{err: &openrouter.StreamError{Kind: openrouter.StreamProviderError, Err: errors.New("down")}}, classification: memory.ClassificationProviderError},
		{name: "missing sections", step: step{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "bad"}}}}}, classification: memory.ClassificationProviderResponseInvalid},
		{name: "tool call", step: step{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
			Role: "assistant", Content: validCompactionSummary(),
			ToolCalls: []openrouter.ToolCall{{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "bad"}}},
		}}}}}, classification: memory.ClassificationProviderResponseInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistory{events: events}
			client := &fakeClient{steps: []step{test.step}}
			session := NewWithCompactor(client, client, testContextProfile("test/model"), history,
				memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
			_, err := session.Compact(context.Background())
			var compactErr *CompactionError
			if !errors.As(err, &compactErr) || compactErr.Classification != test.classification ||
				compactErr.Stage != memory.StageContextCompaction {
				t.Fatalf("error=%v", err)
			}
			if history.appendAttempts != 0 {
				t.Fatalf("append attempts=%d", history.appendAttempts)
			}
		})
	}
}

type compactionDeadlineClient struct {
	remaining time.Duration
}

func (c *compactionDeadlineClient) ChatStream(
	ctx context.Context,
	_ openrouter.ChatRequest,
	_ openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return openrouter.ChatResponse{}, errors.New("missing compactor deadline")
	}
	c.remaining = time.Until(deadline)
	return openrouter.ChatResponse{}, errors.New("stop after observing deadline")
}

func TestSessionCompactBoundsProviderByFixedTwoMinuteTimeout(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	client := &compactionDeadlineClient{}
	session := NewWithCompactor(client, client, testContextProfile("test/model"), &fakeHistory{events: events},
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	_, _ = session.Compact(context.Background())
	if client.remaining < 119*time.Second || client.remaining > 2*time.Minute {
		t.Fatalf("compactor timeout remaining=%s, want fixed two-minute bound", client.remaining)
	}
}

func TestSessionCompactStorageFailureDoesNotActivateGeneratedSummary(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	history := &fakeHistory{events: events, appendErr: errors.New("disk full")}
	response := openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}
	client := &fakeClient{steps: []step{{res: response}}}
	session := NewWithCompactor(client, client, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	_, err := session.Compact(context.Background())
	var compactErr *CompactionError
	if err == nil || errors.As(err, &compactErr) {
		t.Fatalf("storage error=%v, want local non-provider failure", err)
	}
	if session.acceptedSummary() != nil {
		t.Fatal("generated-but-unpersisted summary became active")
	}
}

func TestManualCompactionSelectsMaximalPrefixAndRetainsTwoNewestCompletedTurns(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("configured/model", 300000, 262144, 16384)
	if err != nil {
		t.Fatal(err)
	}
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	events = append(events, completedCompactionTurn("turn-4", 7, "four", "answer four")...)

	plan, err := selectManualCompaction(events, profile, CanonicalRequestEstimator{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CoveredFirst.ID != "turn-1" || plan.CoveredLast.ID != "turn-2-assistant" ||
		plan.FirstRetained.ID != "turn-3" {
		t.Fatalf("cut=%+v", plan)
	}
	request := plan.Request
	if request.Model != "configured/model" || !request.Stream || request.MaxTokens != CompactionOutputReserveTokens ||
		request.Reasoning != nil || request.Temperature == nil || *request.Temperature != 0 ||
		len(request.Tools) != 0 || len(request.Messages) != 2 {
		t.Fatalf("request=%+v", request)
	}
	transcript := request.Messages[1].Content
	for _, included := range []string{"one", "answer one", "two", "answer two", CompactionTranscriptOpen, CompactionTranscriptClose} {
		if !strings.Contains(transcript, included) {
			t.Fatalf("transcript %q does not contain %q", transcript, included)
		}
	}
	for _, excluded := range []string{"three", "answer three", "four", "answer four"} {
		if strings.Contains(transcript, excluded) {
			t.Fatalf("transcript %q contains retained text %q", transcript, excluded)
		}
	}
}

func TestManualCompactionNeedsThreeCompletedTurns(t *testing.T) {
	profile := testContextProfile("test/model")
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	_, err := selectManualCompaction(events, profile, CanonicalRequestEstimator{})
	if err != ErrNothingEligibleForCompaction {
		t.Fatalf("error=%v, want ErrNothingEligibleForCompaction", err)
	}
}

func TestManualCompactionRetainsEveryTurnAfterTheLargestFittingPrefix(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 12000, 12000, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := completedCompactionTurn("turn-1", 1, "one", "small")
	events = append(events, completedCompactionTurn("turn-2", 3, "two", strings.Repeat("x", 6000))...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	events = append(events, completedCompactionTurn("turn-4", 7, "four", "answer four")...)
	events = append(events, completedCompactionTurn("turn-5", 9, "five", "answer five")...)

	plan, err := selectManualCompaction(events, profile, CanonicalRequestEstimator{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CoveredLast.ID != "turn-1-assistant" || plan.FirstRetained.ID != "turn-2" {
		t.Fatalf("cut=%+v", plan)
	}
}

func TestCompactorRequestContainsCompleteToolGroupAndExcludesDurableMetadata(t *testing.T) {
	usage := int64(777777)
	events := []memory.Event{
		{ID: "turn-1", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "run tool"},
		{ID: "tool-assistant", Sequence: 2, ParentID: "turn-1", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: "calling", Payload: historyPayload(t, memory.AssistantMessagePayload{
				ToolCalls: []memory.ToolCall{{ID: "call-1", Name: "lookup", Arguments: `{"id":"artifact-42"}`}},
				Usage:     &memory.TokenUsage{TotalTokens: &usage},
			})},
		{ID: "intent", Sequence: 3, ParentID: "tool-assistant", Type: memory.EventToolIntent, Content: "INTENT_METADATA",
			Payload: historyPayload(t, memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-1", Name: "lookup", Arguments: `{}`}})},
		{ID: "approval", Sequence: 4, ParentID: "intent", Type: memory.EventApproval, Content: "APPROVAL_METADATA",
			Payload: historyPayload(t, memory.ApprovalPayload{Decision: memory.ApprovalApproved})},
		{ID: "result", Sequence: 5, ParentID: "tool-assistant", Type: memory.EventToolSucceeded, Role: memory.RoleTool,
			Content: "TOOL_RESULT_VISIBLE", Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: "call-1"})},
		{ID: "resolved", Sequence: 6, ParentID: "result", Type: memory.EventExecutionResolved, Content: "EXECUTION_METADATA"},
		{ID: "final", Sequence: 7, ParentID: "result", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: "final answer", Payload: json.RawMessage(`{}`)},
	}
	events = append(events, completedCompactionTurn("turn-2", 8, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 10, "three", "answer three")...)

	plan, err := selectManualCompaction(events, testContextProfile("test/model"), CanonicalRequestEstimator{})
	if err != nil {
		t.Fatal(err)
	}
	request := plan.Request
	if request.Model != "test/model" || !request.Stream || request.MaxTokens != 4096 || request.Reasoning != nil ||
		request.Temperature == nil || *request.Temperature != 0 || len(request.Tools) != 0 ||
		len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[0].Content != compactionSystemPrompt ||
		request.Messages[0].Reasoning != "" || len(request.Messages[0].ReasoningDetails) != 0 ||
		len(request.Messages[0].ToolCalls) != 0 || request.Messages[0].ToolCallID != "" {
		t.Fatalf("request=%+v", request)
	}
	encodedRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{"INTENT_METADATA", "APPROVAL_METADATA", "EXECUTION_METADATA", "777777", "usage"} {
		if strings.Contains(string(encodedRequest), excluded) {
			t.Fatalf("request leaked %q: %s", excluded, encodedRequest)
		}
	}
	userData := strings.TrimSuffix(strings.TrimPrefix(request.Messages[1].Content, CompactionTranscriptOpen+"\n"), "\n"+CompactionTranscriptClose)
	var transcript compactionTranscript
	if err := json.Unmarshal([]byte(userData), &transcript); err != nil {
		t.Fatal(err)
	}
	if transcript.PriorSummary != nil || len(transcript.Turns) != 1 || transcript.Turns[0].RootEventID != "turn-1" ||
		len(transcript.Turns[0].Messages) != 4 || transcript.Turns[0].Terminal != nil {
		t.Fatalf("transcript=%+v", transcript)
	}
	messages := transcript.Turns[0].Messages
	if messages[0].Role != "user" || messages[0].Content != "run tool" ||
		messages[1].Role != "assistant" || messages[1].Content != "calling" || len(messages[1].ToolCalls) != 1 ||
		messages[1].ToolCalls[0].ID != "call-1" || messages[1].ToolCalls[0].Function.Arguments != `{"id":"artifact-42"}` ||
		messages[2].Role != "tool" || messages[2].ToolCallID != "call-1" || messages[2].Content != "TOOL_RESULT_VISIBLE" ||
		messages[3].Role != "assistant" || messages[3].Content != "final answer" {
		t.Fatalf("messages=%+v", messages)
	}
}

func TestCompactorRequestRendersOnlySafeTerminalMarker(t *testing.T) {
	terminal := memory.TurnTerminalPayload{
		TurnID: "turn-1", Classification: memory.ClassificationProviderError, Stage: memory.StageProvider,
	}
	events := []memory.Event{
		{ID: "turn-1", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "try"},
		{ID: "failed", Sequence: 2, ParentID: "turn-1", Type: memory.EventTurnFailed,
			Content: terminal.SafeContent(), Payload: historyPayload(t, terminal)},
	}
	events = append(events, completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	plan, err := selectManualCompaction(events, testContextProfile("test/model"), CanonicalRequestEstimator{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Request.Messages[1].Content, terminal.SafeContent()) ||
		!strings.Contains(plan.Request.Messages[1].Content, `"classification":"provider_error"`) ||
		strings.Contains(plan.Request.Messages[1].Content, "raw provider") {
		t.Fatalf("terminal transcript=%q", plan.Request.Messages[1].Content)
	}
}

func TestManualCompactionOneEligibleTurnOverflow(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	_, err := selectManualCompaction(events, testContextProfile("test/model"), fixedRequestEstimator{bytes: 300000})
	if err == nil || !IsContextOverflow(err) {
		t.Fatalf("error=%v, want context overflow", err)
	}
}

type cancellingCompactor struct {
	entered chan struct{}
}

func (c *cancellingCompactor) ChatStream(
	ctx context.Context,
	_ openrouter.ChatRequest,
	_ openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	close(c.entered)
	<-ctx.Done()
	return openrouter.ChatResponse{}, ctx.Err()
}

func TestSessionCompactCallerCancellationWritesNothing(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	history := &fakeHistory{events: events}
	client := &cancellingCompactor{entered: make(chan struct{})}
	session := NewWithCompactor(client, client, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := session.Compact(ctx)
		done <- err
	}()
	<-client.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context canceled", err)
	}
	if history.appendAttempts != 0 || session.acceptedSummary() != nil {
		t.Fatalf("append attempts=%d summary=%+v", history.appendAttempts, session.acceptedSummary())
	}
}

var errCompactionLeaseLost = errors.New("compaction lease lost")

type compactionLeaseLossOwner struct{ *fakeTurnOwner }

func (*compactionLeaseLossOwner) Authorize(context.Context, memory.TurnLease) error {
	return errCompactionLeaseLost
}

func (*compactionLeaseLossOwner) IsLeaseLost(err error) bool {
	return errors.Is(err, errCompactionLeaseLost)
}

func TestSessionCompactLeaseLossBeforeProviderWritesNothing(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	history := &fakeHistory{events: events}
	client := &fakeClient{}
	owner := &compactionLeaseLossOwner{fakeTurnOwner: newFakeTurnOwner()}
	session := NewWithCompactor(client, client, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, owner)
	_, err := session.Compact(context.Background())
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("error=%v, want ErrLeaseLost", err)
	}
	if len(client.reqs) != 0 || history.appendAttempts != 0 {
		t.Fatalf("provider requests=%d append attempts=%d", len(client.reqs), history.appendAttempts)
	}
}

func validCompactionSummary() string {
	var b strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		b.WriteString("## ")
		b.WriteString(heading)
		b.WriteString("\nkept\n\n")
	}
	return b.String()
}

func TestValidateCompactionSummaryIsStrict(t *testing.T) {
	valid := validCompactionSummary()
	if err := validateCompactionSummary(valid); err != nil {
		t.Fatalf("valid summary: %v", err)
	}
	tests := []struct {
		name    string
		summary string
	}{
		{name: "blank", summary: " \n"},
		{name: "invalid UTF-8", summary: string([]byte{0xff})},
		{name: "oversized", summary: valid + strings.Repeat("x", CompactionSummaryMaxBytes)},
		{name: "missing section", summary: strings.Replace(valid, "## "+memory.ContextCompactionSectionHeadings()[3], "### missing", 1)},
		{name: "empty section", summary: strings.Replace(valid, "## "+memory.ContextCompactionSectionHeadings()[3]+"\nkept", "## "+memory.ContextCompactionSectionHeadings()[3], 1)},
		{name: "embedded heading", summary: strings.Replace(valid, "## "+memory.ContextCompactionSectionHeadings()[3], "prefix ## "+memory.ContextCompactionSectionHeadings()[3], 1)},
		{name: "heading with trailing text", summary: strings.Replace(valid, "## "+memory.ContextCompactionSectionHeadings()[3], "## "+memory.ContextCompactionSectionHeadings()[3]+" extra", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCompactionSummary(test.summary); err == nil {
				t.Fatalf("summary accepted (%d bytes, utf8=%v)", len(test.summary), utf8.ValidString(test.summary))
			}
		})
	}
}
