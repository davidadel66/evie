package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

// A wrap-up that shortens tool results to fit its request can still need
// automatic compaction. The summary must be written from the stored events,
// never from the shortened projection, so it does not record excerpts or
// omission markers as the earlier turns.
func TestWrapUpCompactionSummarizesStoredEventsNotFittedProjection(t *testing.T) {
	const sentinel = "alpha-sentinel "
	page := strings.Repeat(sentinel, 25_000/len(sentinel))
	call := memory.ToolCall{ID: "call-1", Name: "large", Arguments: `{}`}
	history := &fakeHistory{events: []memory.Event{
		{ID: "turn-1", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: strings.Repeat("u", 60_000)},
		{ID: "turn-1-call", Sequence: 2, ParentID: "turn-1", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Payload: historyPayload(t, memory.AssistantMessagePayload{ToolCalls: []memory.ToolCall{call}})},
		{ID: "turn-1-result", Sequence: 3, ParentID: "turn-1-call", Type: memory.EventToolSucceeded, Role: memory.RoleTool, Content: page,
			Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: call.ID})},
		{ID: "turn-1-answer", Sequence: 4, ParentID: "turn-1-result", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: "answer", Payload: json.RawMessage(`{}`)},
	}}
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	conversation := &fakeClient{steps: []step{assistantStep("## Summary\nwrapped", nil)}}
	signal := func(WrapUpState) (string, bool) { return "Evie harness notice: wrap up now (test).", true }
	session := withByteBudgets(NewWithCompactorAndToolset(conversation, compactor, automaticTestProfile(t, 100_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner(), tools.NewToolset(nil), WithWrapUp(signal)))

	if err := session.Send(context.Background(), strings.Repeat("d", 12_000), &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(compactor.reqs) != 1 || len(conversation.reqs) != 1 || conversation.reqs[0].ToolChoice != "none" {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	summarized, err := json.Marshal(compactor.reqs[0].Messages)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Count(string(summarized), sentinel), strings.Count(page, sentinel); got != want ||
		strings.Contains(string(summarized), "older tool result projected") || strings.Contains(string(summarized), "tool result omitted") {
		t.Fatalf("compactor summarized a shortened tool result: %d of %d page repeats", got, want)
	}
}

// A delegated worker's supervisor can end the turn early through the same
// tool-free final call as the step limit, with its own harness notice. The
// signal is consulted before every model response and also replaces the
// step-limit notice when the limit is reached.
func TestWrapUpSignalRequestsTheFinalToolFreeResponse(t *testing.T) {
	for _, tc := range []struct {
		name, limit string
		wrapAt      int
	}{{"budget", "100", 1}, {"step_limit", "1", 100}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(TurnStepLimitEnv, tc.limit)
			ctx := context.Background()
			db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := eviedb.NewStore(db)
			parent, err := store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := standardManager(t, store).ResolvePreset(plugins.ResearchPresetID)
			if err != nil {
				t.Fatal(err)
			}
			child, err := store.CreateDelegatedSessionWithComposition(ctx, parent.ID, resolved.Receipt)
			if err != nil {
				t.Fatal(err)
			}
			const notice = "Evie harness notice: wrap up now (test)."
			var asked []WrapUpState
			signal := func(state WrapUpState) (string, bool) {
				asked = append(asked, state)
				if state.Step >= tc.wrapAt || state.Step >= state.StepLimit {
					return notice, true
				}
				return "", false
			}
			client := &fakeClient{steps: []step{assistantStep("## Summary\nwrapped", nil)}}
			session := NewDelegatedWithToolset(client, testContextProfile("test-model"), store.BindHistory(child.ID, "child"), child.ScopeContext(), store.BindTurnOwner(child.ID, "child"), resolved.Toolset, plugins.ResearchInstructions, WithWrapUp(signal))
			if err := session.Send(ctx, "Assignment: research.", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			if len(client.reqs) != 1 || client.reqs[0].ToolChoice != "none" || len(client.reqs[0].Tools) == 0 {
				t.Fatalf("requests=%+v", client.reqs)
			}
			last := client.reqs[0].Messages[len(client.reqs[0].Messages)-1]
			// The signal sees the measured request and its budget.
			if last.Role != "user" || last.Content != notice || len(asked) != 1 || asked[0].Step != 1 ||
				asked[0].RequestBytes <= 0 || asked[0].RequestBytes > asked[0].UsableBytes {
				t.Fatalf("final message=%+v asked=%v", last, asked)
			}
			events, err := store.LoadEvents(ctx, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			if final := events[len(events)-1]; final.Type != memory.EventAssistantMessage || final.Content != "## Summary\nwrapped" {
				t.Fatalf("final event=%+v", final)
			}
		})
	}
}
