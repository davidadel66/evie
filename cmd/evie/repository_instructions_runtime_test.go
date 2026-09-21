package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type instructionClient struct {
	t        *testing.T
	path     string
	expected string
	requests int
}

func (c *instructionClient) ChatStream(_ context.Context, req openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if strings.HasPrefix(req.Messages[0].Content, "You are Evie's context compactor.") {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "## Goal / criteria / constraints\nTest instructions.\n## Current state / completed actions\nFixed the guide.\n## Decisions / discoveries\nRoot guide applies.\n## Durable paths / IDs / artifacts / tool outcomes\nAGENTS.md\n## Unresolved questions / blockers / risks\nNone.\n## Next steps\nContinue.\n## User preferences / commitments\nUse the guide."}}}}, nil
	}
	c.requests++
	found := ""
	for _, message := range req.Messages {
		if strings.HasPrefix(message.Content, "Repository instructions enabled by the owner") {
			found = message.Content
			if message.Role != "user" {
				c.t.Fatal("repo guide changed system priority")
			}
		}
	}
	if c.expected == "" && found != "" || c.expected != "" && !strings.Contains(found, c.expected) {
		c.t.Fatalf("instructions=%q expected=%q", found, c.expected)
	}
	message := openrouter.Message{Role: "assistant", Content: "done"}
	if req.Messages[len(req.Messages)-1].Role != "tool" && c.expected != "" {
		os.WriteFile(c.path, []byte("SECOND_GUIDE"), 0600)
		message.Content = ""
		message.ToolCalls = []openrouter.ToolCall{{ID: "read-guide", Type: "function", Function: openrouter.FunctionCall{Name: "read_file", Arguments: `{"path":"AGENTS.md"}`}}}
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: message}}}, nil
}
func TestRepositoryInstructionsFrozenPerTurnReloadedAndExcludedWhenDisabled(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "Repository")
	if err != nil {
		t.Fatal(err)
	}
	composition, err := sessionCompositionManager(t).ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, composition.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	os.WriteFile(path, []byte("FIRST_GUIDE"), 0600)
	if _, err = store.SetWorkspaceFolder(ctx, ws.ID, 0, root); err != nil {
		t.Fatal(err)
	}
	client := &instructionClient{t: t, path: path, expected: "FIRST_GUIDE"}
	runtime := agent.NewWithToolset(client, evieTestContextProfile("instructions-test"), store.BindHistory(record.ID, "test"), record.ScopeContext(), store.BindTurnOwner(record.ID, "test"), tools.BuiltinToolset(), agent.WithAutomaticMemoryRecall(false))
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 32769)), 0600); err != nil {
		t.Fatal(err)
	}
	if err = runtime.Send(ctx, "Invalid guide", clockCLIEvents{}, nil); err == nil || client.requests != 0 {
		t.Fatalf("invalid instructions reached provider or succeeded: %v", err)
	}
	var classification string
	if err = db.QueryRow(`SELECT json_extract(payload_json,'$.classification') FROM events WHERE event_type='turn_failed'`).Scan(&classification); err != nil || classification != string(memory.ClassificationRepositoryInstructions) {
		t.Fatalf("instruction failure did not complete durably: %s %v", classification, err)
	}
	if err := os.WriteFile(path, []byte("FIRST_GUIDE"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = runtime.Send(ctx, "Read instructions", clockCLIEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	if client.requests != 2 {
		t.Fatalf("requests=%d", client.requests)
	}
	client.expected = "SECOND_GUIDE"
	if err = runtime.Send(ctx, "Read again", clockCLIEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetRepositoryInstructionSettings(ctx, ws.ID, 0, false); err != nil {
		t.Fatal(err)
	}
	client.expected = ""
	if err = runtime.Send(ctx, "No guide", clockCLIEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	var captured, prepared int
	db.QueryRow(`SELECT count(*) FROM repository_instruction_snapshots`).Scan(&captured)
	db.QueryRow(`SELECT count(*) FROM events WHERE event_type='context_snapshot' AND json_extract(payload_json,'$.repository_instructions_turn_id') IS NOT NULL`).Scan(&prepared)
	if captured != 4 || prepared != 5 {
		t.Fatalf("snapshots=%d prepared requests=%d", captured, prepared)
	}
	if _, err = runtime.Compact(ctx); err != nil {
		t.Fatalf("instruction failure blocked later compaction: %v", err)
	}
}
