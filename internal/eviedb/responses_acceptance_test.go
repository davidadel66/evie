package eviedb_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

func TestResponsesPublicHistorySurvivesSQLiteRestartAndRollback(t *testing.T) {
	t.Setenv("EVIE_REASONING", "low")
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := eviedb.NewStore(db)
	record, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const holder memory.LeaseHolderID = "responses-acceptance"
	runs := 0
	toolset := tools.NewToolset([]tools.Tool{{
		Schema:  openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "echo", Parameters: openrouter.Parameter{Type: "object"}}},
		Execute: func(context.Context, string) (string, error) { runs++; return "echo result", nil },
	}})
	newSession := func(client *compactionAcceptanceClient, model string) *agent.Session {
		return agent.NewWithToolset(client, compactionAcceptanceProfile(t, model, 262144),
			store.BindHistory(record.ID, holder), record.ScopeContext(), store.BindTurnOwner(record.ID, holder), toolset)
	}
	legacy := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{compactionAcceptanceResponse("Legacy answer.")}}
	if err := newSession(legacy, openrouter.BuiltinModel).Send(ctx, "legacy question", nilAgentEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	message := openrouter.Message{Role: "assistant", Content: "Before.Between.After.",
		TextParts: []openrouter.TextPart{{Text: "Before.", Phase: "commentary"}, {Text: "Between.", Phase: "final_answer", AfterToolCalls: 1}, {Text: "After.", Phase: "commentary", AfterToolCalls: 2}},
		ToolCalls: []openrouter.ToolCall{
			{ID: "call-a", Type: "function", Function: openrouter.FunctionCall{Name: "echo", Arguments: `{}`}},
			{ID: "call-b", Type: "function", Function: openrouter.FunctionCall{Name: "echo", Arguments: `{}`}},
		},
		ResponseItems: []json.RawMessage{
			json.RawMessage(`{"type":"reasoning","id":"rs_opaque","summary":[],"encrypted_content":"opaque-restart-canary"}`),
			json.RawMessage(`{"type":"message","id":"msg_before","role":"assistant","phase":"commentary","status":"completed","content":[{"type":"output_text","text":"Before."}]}`),
			json.RawMessage(`{"type":"function_call","id":"fc_a","call_id":"call-a","name":"echo","arguments":"{}","status":"completed"}`),
			json.RawMessage(`{"type":"message","id":"msg_between","role":"assistant","phase":"final_answer","status":"completed","content":[{"type":"output_text","text":"Between."}]}`),
			json.RawMessage(`{"type":"function_call","id":"fc_b","call_id":"call-b","name":"echo","arguments":"{}","status":"completed"}`),
			json.RawMessage(`{"type":"message","id":"msg_after","role":"assistant","phase":"commentary","status":"completed","content":[{"type":"output_text","text":"After."}]}`),
		},
	}
	client := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		{Choices: []openrouter.Choice{{Message: message}}}, compactionAcceptanceResponse("Finished."),
	}}
	session := newSession(client, openrouter.AstraModel)
	if err := session.Send(ctx, "check twice", nilAgentEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	before, err := session.InspectContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	restartedClient := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{compactionAcceptanceResponse("Resumed.")}}
	restarted := newSession(restartedClient, openrouter.AstraModel)
	after, err := restarted.InspectContext(ctx)
	if err != nil || before.Projection.RequestSHA256 != after.Projection.RequestSHA256 {
		t.Fatalf("restart changed projection: %v", err)
	}
	if err := restarted.Send(ctx, "continue", nilAgentEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	body, err := openrouter.RequestBytes(restartedClient.requests[0])
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Input []map[string]json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatal(err)
	}
	var sequence []string
	for _, item := range input.Input {
		if string(item["phase"]) != "" {
			sequence = append(sequence, string(item["phase"]))
		} else if string(item["type"]) == `"function_call"` {
			sequence = append(sequence, string(item["call_id"]))
		}
	}
	if strings.Join(sequence, ",") != `"commentary","call-a","final_answer","call-b","commentary"` {
		t.Fatalf("public item order=%v", sequence)
	}
	for _, text := range []string{"Before.", "Between.", "After.", "Legacy answer."} {
		if strings.Count(string(body), text) != 1 {
			t.Fatalf("replay should contain %q once", text)
		}
	}
	if strings.Contains(string(body), "opaque-restart-canary") || strings.Contains(string(body), "msg_before") {
		t.Fatal("restart retained opaque state or provider identity")
	}
	rollback := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{compactionAcceptanceResponse("Rolled back.")}}
	if err := newSession(rollback, openrouter.BuiltinModel).Send(ctx, "continue on Kimi", nilAgentEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	chatBody, err := openrouter.RequestBytes(rollback.requests[0])
	if err != nil || !strings.Contains(string(chatBody), `"messages"`) || !strings.Contains(string(chatBody), "Before.Between.After.") || runs != 2 {
		t.Fatalf("rollback error=%v tool runs=%d", err, runs)
	}
	events, err := store.LoadEvents(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(event.Content+string(event.Payload), "opaque-restart-canary") || strings.Contains(string(event.Payload), "rs_opaque") {
			t.Fatal("transport state persisted")
		}
	}
}
