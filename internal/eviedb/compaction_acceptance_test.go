package eviedb_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type compactionAcceptanceClient struct {
	responses []openrouter.ChatResponse
	requests  []openrouter.ChatRequest
}

type compactionAcceptanceErrorClient struct {
	err      error
	requests []openrouter.ChatRequest
}

func (c *compactionAcceptanceErrorClient) ChatStream(
	_ context.Context,
	request openrouter.ChatRequest,
	_ openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	return openrouter.ChatResponse{}, c.err
}

type compactionAcceptanceCancellingClient struct {
	entered chan struct{}
}

func (c *compactionAcceptanceCancellingClient) ChatStream(
	ctx context.Context,
	_ openrouter.ChatRequest,
	_ openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	close(c.entered)
	<-ctx.Done()
	return openrouter.ChatResponse{}, ctx.Err()
}

func (c *compactionAcceptanceClient) ChatStream(
	_ context.Context,
	request openrouter.ChatRequest,
	_ openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	if len(c.responses) == 0 {
		return openrouter.ChatResponse{}, fmt.Errorf("acceptance client has no response")
	}
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
}

func compactionAcceptanceSummary(generation string) string {
	values := []string{
		"Preserve durable rolling continuity and all safety constraints.",
		"The early approved decision remains SQLite; " + generation + " is accepted.",
		"The stable decision is to preserve append-only evidence.",
		"Stable path /work/evie and ID artifact-42; old tool outcome is success.",
		"The unresolved blocker remains provider access.",
		"Next action: validate restart projection.",
		"The user committed to inspect every retained source event.",
	}
	var summary strings.Builder
	for i, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&summary, "## %s\n%s\n\n", heading, values[i])
	}
	return summary.String()
}

func compactionAcceptanceResponse(summary string) openrouter.ChatResponse {
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: summary,
	}}}}
}

func compactionAcceptanceProfile(t *testing.T, model string, working int64) openrouter.ContextProfile {
	t.Helper()
	profile, err := openrouter.NewExplicitContextProfile(model, 700000, working, 16384)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestDurableCompactionChainSurvivesSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const holder = memory.LeaseHolderID("compaction-acceptance")
	history := store.BindHistory(sessionRecord.ID, holder)

	appendSimpleTurn := func(user, assistant string) []memory.Event {
		t.Helper()
		lease, err := store.AcquireTurnLease(ctx, sessionRecord.ID, holder, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		root, err := history.Append(ctx, lease, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: user})
		if err != nil {
			t.Fatal(err)
		}
		answer, err := history.Append(ctx, lease, memory.EventInput{
			ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: assistant, Payload: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.ReleaseTurnLease(ctx, sessionRecord.ID, holder, lease.FencingToken); err != nil {
			t.Fatal(err)
		}
		return []memory.Event{root, answer}
	}

	var sourceEvents []memory.Event
	sourceEvents = append(sourceEvents, appendSimpleTurn(
		"Approved decision: use SQLite. Stable path /work/evie and ID artifact-42.",
		"Decision recorded; preserve append-only evidence.",
	)...)
	lease, err := store.AcquireTurnLease(ctx, sessionRecord.ID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	toolRoot, err := history.Append(ctx, lease, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Read the old tool outcome."})
	if err != nil {
		t.Fatal(err)
	}
	toolAssistant, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: toolRoot.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
		Content: "Checking artifact-42.",
		Payload: json.RawMessage(`{"tool_calls":[{"id":"call-42","name":"inspect","arguments":"{\"id\":\"artifact-42\"}"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: toolAssistant.ID, Type: memory.EventToolIntent, ExecutionID: "execution-42",
		Payload: json.RawMessage(`{"call":{"id":"call-42","name":"inspect","arguments":"{\"id\":\"artifact-42\"}"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	toolResult, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: toolAssistant.ID, Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: "execution-42",
		Content: "old tool outcome: artifact-42 is intact",
		Payload: json.RawMessage(`{"tool_call_id":"call-42","is_error":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	toolFinal, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: toolResult.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
		Content: "The old tool outcome succeeded.", Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, sessionRecord.ID, holder, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	sourceEvents = append(sourceEvents, toolRoot, toolAssistant, intent, toolResult, toolFinal)

	lease, err = store.AcquireTurnLease(ctx, sessionRecord.ID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	failedRoot, err := history.Append(ctx, lease, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Resolve provider access."})
	if err != nil {
		t.Fatal(err)
	}
	terminal := memory.TurnTerminalPayload{TurnID: failedRoot.ID, Classification: memory.ClassificationProviderError, Stage: memory.StageProvider}
	terminalJSON, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: failedRoot.ID, Type: memory.EventTurnFailed, Content: terminal.SafeContent(), Payload: terminalJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, sessionRecord.ID, holder, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	sourceEvents = append(sourceEvents, failedRoot, failed)
	sourceEvents = append(sourceEvents, appendSimpleTurn("What is the next action?", "Validate restart projection.")...)
	sourceEvents = append(sourceEvents, appendSimpleTurn("Keep the blocker visible.", "Provider access remains unresolved.")...)

	firstSummary := compactionAcceptanceSummary("generation one")
	firstCompactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{compactionAcceptanceResponse(firstSummary)}}
	firstAgent := agent.NewWithCompactor(
		firstCompactor, firstCompactor, compactionAcceptanceProfile(t, "old/model", 262144), history,
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder),
	)
	firstResult, err := firstAgent.Compact(ctx)
	if err != nil {
		t.Fatalf("first compaction: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	history = store.BindHistory(sessionRecord.ID, holder)

	sourceEvents = append(sourceEvents, appendSimpleTurn("Continue after compaction.", "Continuity retained.")...)
	sourceEvents = append(sourceEvents, appendSimpleTurn("Prepare restart.", "Restart is the next action.")...)
	secondSummary := compactionAcceptanceSummary("generation two")
	secondCompactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{compactionAcceptanceResponse(secondSummary)}}
	secondAgent := agent.NewWithCompactor(
		secondCompactor, secondCompactor, compactionAcceptanceProfile(t, "current/model", 250000), history,
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder),
	)
	secondResult, err := secondAgent.Compact(ctx)
	if err != nil {
		t.Fatalf("second compaction: %v", err)
	}
	if firstResult.CompactionEventID == secondResult.CompactionEventID {
		t.Fatal("second compaction did not append a replacement generation")
	}

	restartProfile := compactionAcceptanceProfile(t, "resume/model", 240000)
	beforeRestart := agent.NewWithCompactor(
		&compactionAcceptanceClient{}, &compactionAcceptanceClient{}, restartProfile, history,
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder),
	)
	beforeDiagnostics, err := beforeRestart.InspectContext(ctx)
	if err != nil {
		t.Fatalf("inspect before restart: %v", err)
	}
	if beforeDiagnostics.Projection.ActiveCompactionEventID != secondResult.CompactionEventID {
		t.Fatalf("active compaction before restart=%q", beforeDiagnostics.Projection.ActiveCompactionEventID)
	}

	acceptedBeforeRestart, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[memory.EventID]memory.Event, len(acceptedBeforeRestart))
	compactionCount := 0
	for _, event := range acceptedBeforeRestart {
		byID[event.ID] = event
		if event.Type == memory.EventContextCompacted {
			compactionCount++
		}
	}
	if compactionCount != 2 {
		t.Fatalf("compaction events=%d, want 2", compactionCount)
	}
	firstCompaction := byID[firstResult.CompactionEventID]
	secondCompaction := byID[secondResult.CompactionEventID]
	var firstPayload, secondPayload memory.ContextCompactedPayload
	if err := json.Unmarshal(firstCompaction.Payload, &firstPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(secondCompaction.Payload, &secondPayload); err != nil {
		t.Fatal(err)
	}
	if firstCompaction.Content != firstSummary || secondCompaction.Content != secondSummary ||
		firstPayload.Generation != 1 || secondPayload.Generation != 2 ||
		secondPayload.PriorCompactionEventID != firstCompaction.ID ||
		secondPayload.CoveredFirstEventID != firstPayload.FirstRetainedEventID ||
		secondPayload.CoveredFirstSequence <= firstPayload.CoveredLastSequence {
		t.Fatalf("durable compaction chain is inconsistent: first=%+v second=%+v", firstPayload, secondPayload)
	}
	for _, source := range sourceEvents {
		retained, ok := byID[source.ID]
		if !ok || retained.Sequence != source.Sequence || retained.Type != source.Type || retained.Content != source.Content ||
			string(retained.Payload) != string(source.Payload) {
			t.Fatalf("source event changed or disappeared: before=%+v after=%+v", source, retained)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	restartedDB, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restartedDB.Close()
	restartedStore := eviedb.NewStore(restartedDB)
	restartedHistory := restartedStore.BindHistory(sessionRecord.ID, holder)
	conversation := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "Restart continuity accepted.",
	}}}}}}
	restartedAgent := agent.NewWithCompactor(
		conversation, conversation, restartProfile, restartedHistory,
		sessionRecord.ScopeContext(), restartedStore.BindTurnOwner(sessionRecord.ID, holder),
	)
	afterDiagnostics, err := restartedAgent.InspectContext(ctx)
	if err != nil {
		t.Fatalf("inspect after restart: %v", err)
	}
	if afterDiagnostics.Projection.ActiveCompactionEventID != secondResult.CompactionEventID ||
		afterDiagnostics.Projection.RetainedFirstEventID != beforeDiagnostics.Projection.RetainedFirstEventID ||
		afterDiagnostics.Projection.RequestSHA256 != beforeDiagnostics.Projection.RequestSHA256 {
		t.Fatalf("restart projection changed: before=%+v after=%+v", beforeDiagnostics.Projection, afterDiagnostics.Projection)
	}
	if err := restartedAgent.Send(ctx, "Execute the next action.", nilAgentEvents{}, nil); err != nil {
		t.Fatalf("send after restart: %v", err)
	}
	requestJSON, err := json.Marshal(conversation.requests[0])
	if err != nil {
		t.Fatal(err)
	}
	if conversation.requests[0].Messages[1].Content != secondSummary ||
		strings.Contains(string(requestJSON), firstSummary) || strings.Contains(string(requestJSON), "artifact-42 is intact") ||
		!strings.Contains(string(requestJSON), "Execute the next action.") {
		t.Fatalf("provider projection after restart=%s", requestJSON)
	}
}

func TestAutomaticCompactionCrossesWindowWithToolHistoryAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const holder = memory.LeaseHolderID("automatic-compaction-acceptance")
	profile := compactionAcceptanceProfile(t, "automatic/model", 600_000)
	firstSummary := compactionAcceptanceSummary("automatic generation one")
	firstConversation := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		compactionAcceptanceResponse("first answer"),
		compactionAcceptanceResponse("second answer"),
	}}
	firstCompactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		compactionAcceptanceResponse(firstSummary),
	}}
	firstAgent := agent.NewWithCompactor(
		firstConversation, firstCompactor, profile, store.BindHistory(sessionRecord.ID, holder),
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder),
	)
	firstUser := "FIRST_RAW:" + strings.Repeat("a", 330_000)
	secondUser := "SECOND_RAW:" + strings.Repeat("b", 190_000)
	if err := firstAgent.Send(ctx, firstUser, nilAgentEvents{}, nil); err != nil {
		t.Fatalf("first large turn: %v", err)
	}
	if err := firstAgent.Send(ctx, secondUser, nilAgentEvents{}, nil); err != nil {
		t.Fatalf("second large turn: %v", err)
	}
	if len(firstCompactor.requests) != 1 || len(firstConversation.requests) != 2 {
		t.Fatalf("before restart compactor requests=%d conversation requests=%d", len(firstCompactor.requests), len(firstConversation.requests))
	}

	beforeRestart, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	var firstCompaction memory.Event
	for _, event := range beforeRestart {
		if event.Type == memory.EventContextCompacted {
			firstCompaction = event
		}
	}
	if firstCompaction.ID == "" {
		t.Fatalf("first automatic generation missing: %v", acceptanceEventTypes(beforeRestart))
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	secondSummary := compactionAcceptanceSummary("automatic generation two after restart")
	toolCall := openrouter.ToolCall{
		ID: "call-large", Type: "function",
		Function: openrouter.FunctionCall{Name: "large_result", Arguments: `{}`},
	}
	secondConversation := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{toolCall}}}}},
		compactionAcceptanceResponse("tool-heavy turn complete"),
	}}
	secondCompactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		compactionAcceptanceResponse(secondSummary),
	}}
	largeTool := tools.Tool{
		Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{
			Name: "large_result", Description: "Return a deterministic large result.",
			Parameters: openrouter.Parameter{Type: "object", Properties: map[string]openrouter.Property{}},
		}},
		Execute: func(context.Context, string) (string, error) {
			return "TOOL_RAW:" + strings.Repeat("z", 67_000), nil
		},
	}
	restarted := agent.NewWithCompactor(
		secondConversation, secondCompactor, compactionAcceptanceProfile(t, "automatic/model", 330_000), store.BindHistory(sessionRecord.ID, holder),
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder),
	)
	if err := restarted.Send(ctx, "Run the large tool after restart.", nilAgentEvents{}, nil, largeTool); err != nil {
		t.Fatalf("tool-heavy turn after restart: %v", err)
	}
	if len(secondConversation.requests) != 2 || len(secondCompactor.requests) != 1 {
		t.Fatalf("after restart conversation requests=%d compactor requests=%d", len(secondConversation.requests), len(secondCompactor.requests))
	}

	diagnostics, err := restarted.InspectContext(ctx)
	if err != nil {
		t.Fatalf("/context diagnostics: %v", err)
	}
	afterRestart, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	var compactions []memory.Event
	var toolAssistant, toolResult memory.Event
	for _, event := range afterRestart {
		switch event.Type {
		case memory.EventContextCompacted:
			compactions = append(compactions, event)
		case memory.EventAssistantMessage:
			var payload memory.AssistantMessagePayload
			if err := json.Unmarshal(event.Payload, &payload); err == nil && len(payload.ToolCalls) != 0 {
				toolAssistant = event
			}
		case memory.EventToolSucceeded:
			toolResult = event
		}
	}
	if len(compactions) != 2 || diagnostics.Projection.ActiveCompactionEventID != compactions[1].ID ||
		diagnostics.LatestSnapshot == nil || diagnostics.LatestSnapshot.Manifest.ActiveCompactionEventID != compactions[1].ID {
		t.Fatalf("compactions=%d diagnostics=%+v", len(compactions), diagnostics)
	}
	var firstPayload, secondPayload memory.ContextCompactedPayload
	if err := json.Unmarshal(compactions[0].Payload, &firstPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(compactions[1].Payload, &secondPayload); err != nil {
		t.Fatal(err)
	}
	if firstPayload.Trigger != memory.ContextCompactionAutomatic || secondPayload.Trigger != memory.ContextCompactionAutomatic ||
		firstPayload.Generation != 1 || secondPayload.Generation != 2 ||
		secondPayload.PriorCompactionEventID != compactions[0].ID ||
		secondPayload.CoveredFirstEventID != firstPayload.FirstRetainedEventID {
		t.Fatalf("automatic chain: first=%+v second=%+v", firstPayload, secondPayload)
	}
	var toolAssistantPayload memory.AssistantMessagePayload
	var toolResultPayload memory.ToolResultPayload
	if err := json.Unmarshal(toolAssistant.Payload, &toolAssistantPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(toolResult.Payload, &toolResultPayload); err != nil {
		t.Fatal(err)
	}
	assistantToolCallID := ""
	if len(toolAssistantPayload.ToolCalls) == 1 {
		assistantToolCallID = toolAssistantPayload.ToolCalls[0].ID
	}
	if toolAssistant.ID == "" || toolResult.ID == "" || len(toolAssistantPayload.ToolCalls) != 1 ||
		assistantToolCallID != toolResultPayload.ToolCallID ||
		!strings.HasPrefix(toolResult.Content, "TOOL_RAW:") || len(toolResult.Content) != len("TOOL_RAW:")+67_000 {
		t.Fatalf("tool pairing IDs assistant=%q result=%q", assistantToolCallID, toolResultPayload.ToolCallID)
	}
	seenFirst, seenSecond := false, false
	for _, event := range afterRestart {
		seenFirst = seenFirst || event.Type == memory.EventUserMessage && event.Content == firstUser
		seenSecond = seenSecond || event.Type == memory.EventUserMessage && event.Content == secondUser
	}
	if !seenFirst || !seenSecond || len(afterRestart) <= len(beforeRestart) {
		t.Fatalf("raw history was not retained: first=%v second=%v before=%d after=%d", seenFirst, seenSecond, len(beforeRestart), len(afterRestart))
	}
}

func TestAutomaticCompactionPersistsDeterministicFailureAndCancellationEvidence(t *testing.T) {
	for _, test := range []struct {
		name        string
		historySize int
		cancel      bool
		wantType    memory.EventType
		wantClass   memory.TurnClassification
	}{
		{name: "provider failure", historySize: 210_000, wantType: memory.EventTurnFailed, wantClass: memory.ClassificationProviderError},
		{name: "caller cancellation", historySize: 190_000, cancel: true, wantType: memory.EventTurnInterrupted, wantClass: memory.ClassificationCallerCancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := eviedb.NewStore(openAcceptanceDB(t))
			sessionRecord, err := store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			holder := memory.LeaseHolderID("automatic-evidence-" + strings.ReplaceAll(test.name, " ", "-"))
			history := store.BindHistory(sessionRecord.ID, holder)
			lease, err := store.AcquireTurnLease(ctx, sessionRecord.ID, holder, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			third := test.historySize / 3
			for i, size := range []int{third, third, test.historySize - 2*third} {
				root, err := history.Append(ctx, lease, memory.EventInput{
					Type: memory.EventUserMessage, Role: memory.RoleUser,
					Content: fmt.Sprintf("source-%d:", i+1) + strings.Repeat(string(rune('a'+i)), size),
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := history.Append(ctx, lease, memory.EventInput{
					ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
					Content: "accepted", Payload: json.RawMessage(`{}`),
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.ReleaseTurnLease(ctx, sessionRecord.ID, holder, lease.FencingToken); err != nil {
				t.Fatal(err)
			}

			conversation := &compactionAcceptanceClient{}
			var compactor agent.Client
			var cancel context.CancelFunc
			sendCtx := ctx
			if test.cancel {
				blocking := &compactionAcceptanceCancellingClient{entered: make(chan struct{})}
				compactor = blocking
				sendCtx, cancel = context.WithCancel(ctx)
				done := make(chan error, 1)
				session := agent.NewWithCompactor(
					conversation, compactor, compactionAcceptanceProfile(t, "evidence/model", 230_000), history,
					sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder),
				)
				go func() { done <- session.Send(sendCtx, strings.Repeat("d", 8_000), nilAgentEvents{}, nil) }()
				<-blocking.entered
				cancel()
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("Send error=%v, want context canceled", err)
				}
			} else {
				failing := &compactionAcceptanceErrorClient{err: errors.New("deterministic summary transport failure")}
				compactor = failing
				session := agent.NewWithCompactor(
					conversation, compactor, compactionAcceptanceProfile(t, "evidence/model", 230_000), history,
					sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder),
				)
				if err := session.Send(sendCtx, strings.Repeat("d", 8_000), nilAgentEvents{}, nil); err == nil {
					t.Fatal("Send unexpectedly succeeded")
				}
				if len(failing.requests) != 1 {
					t.Fatalf("compactor requests=%d", len(failing.requests))
				}
			}

			events, err := store.LoadEvents(ctx, sessionRecord.ID)
			if err != nil {
				t.Fatal(err)
			}
			last := events[len(events)-1]
			var terminal memory.TurnTerminalPayload
			if err := json.Unmarshal(last.Payload, &terminal); err != nil {
				t.Fatal(err)
			}
			if last.Type != test.wantType || last.Content != terminal.SafeContent() ||
				terminal.Classification != test.wantClass || terminal.Stage != memory.StageContextCompaction ||
				len(conversation.requests) != 0 {
				t.Fatalf("terminal=%+v event_type=%q conversation_requests=%d", terminal, last.Type, len(conversation.requests))
			}
		})
	}
}

func acceptanceEventTypes(events []memory.Event) []memory.EventType {
	types := make([]memory.EventType, len(events))
	for i := range events {
		types[i] = events[i].Type
	}
	return types
}

func TestStaleLeaseCannotAppendCompactionEvent(t *testing.T) {
	ctx := context.Background()
	store := eviedb.NewStore(openAcceptanceDB(t))
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const staleHolder = memory.LeaseHolderID("stale-compactor")
	staleHistory := store.BindHistory(sessionRecord.ID, staleHolder)
	staleLease, err := store.AcquireTurnLease(ctx, sessionRecord.ID, staleHolder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var turns [3][2]memory.Event
	for i := range turns {
		root, err := staleHistory.Append(ctx, staleLease, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: fmt.Sprintf("turn %d", i+1),
		})
		if err != nil {
			t.Fatal(err)
		}
		assistant, err := staleHistory.Append(ctx, staleLease, memory.EventInput{
			ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: "done", Payload: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		turns[i] = [2]memory.Event{root, assistant}
	}
	if err := store.ReleaseTurnLease(ctx, sessionRecord.ID, staleHolder, staleLease.FencingToken); err != nil {
		t.Fatal(err)
	}
	currentLease, err := store.AcquireTurnLease(ctx, sessionRecord.ID, "current-compactor", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseTurnLease(ctx, sessionRecord.ID, currentLease.HolderID, currentLease.FencingToken)

	summary := compactionAcceptanceSummary("stale generation")
	digest := sha256.Sum256([]byte(summary))
	payload, err := json.Marshal(memory.ContextCompactedPayload{
		SchemaVersion: memory.ContextCompactedSchemaVersion, Generation: 1,
		Trigger:             memory.ContextCompactionManual,
		CoveredFirstEventID: turns[0][0].ID, CoveredFirstSequence: turns[0][0].Sequence,
		CoveredLastEventID: turns[0][1].ID, CoveredLastSequence: turns[0][1].Sequence,
		FirstRetainedEventID: turns[1][0].ID, CanonicalModel: "model", PromptVersion: agent.CompactionPromptVersion,
		SummaryBytes: int64(len(summary)), SummarySHA256: fmt.Sprintf("%x", digest),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := staleHistory.Append(ctx, staleLease, memory.EventInput{
		Type: memory.EventContextCompacted, Content: summary, Payload: payload,
	}); !errors.Is(err, eviedb.ErrTurnLeaseLost) {
		t.Fatalf("stale compaction append error=%v, want ErrTurnLeaseLost", err)
	}
	events, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == memory.EventContextCompacted {
			t.Fatalf("stale owner appended compaction %+v", event)
		}
	}
}

func openAcceptanceDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type nilAgentEvents struct{}

func (nilAgentEvents) Delta(string)                                  {}
func (nilAgentEvents) Reasoning(string)                              {}
func (nilAgentEvents) ReasoningDone()                                {}
func (nilAgentEvents) AssistantDone(string)                          {}
func (nilAgentEvents) ToolCall(string, string, string)               {}
func (nilAgentEvents) ToolResult(string, string, bool)               {}
func (nilAgentEvents) ResponseDiscarded(agent.DiscardReason, string) {}
