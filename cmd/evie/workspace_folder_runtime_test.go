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

type folderReadClient struct{ results []string }

func (c *folderReadClient) ChatStream(_ context.Context, req openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	last := req.Messages[len(req.Messages)-1]
	message := openrouter.Message{Role: "assistant", Content: "Read the local file."}
	if last.Role == "tool" {
		c.results = append(c.results, last.Content)
	} else {
		message.Content = ""
		message.ToolCalls = []openrouter.ToolCall{{ID: "read-local-note", Type: "function", Function: openrouter.FunctionCall{Name: "read_file", Arguments: `{"path":"note.txt"}`}}}
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: message}}}, nil
}

func TestExistingWorkspaceChatUsesReplacedFolderOnNextTurn(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "Project")
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
	client := &folderReadClient{}
	runtime := agent.NewWithToolset(client, evieTestContextProfile("folder-test"), store.BindHistory(record.ID, "folder-test"), record.ScopeContext(), store.BindTurnOwner(record.ID, "folder-test"), tools.BuiltinToolset(), agent.WithAutomaticMemoryRecall(false))
	for index, text := range []string{"first folder", "replacement folder"} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetWorkspaceFolder(ctx, ws.ID, int64(index), root); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Send(ctx, "Read note.txt", clockCLIEvents{}, nil); err != nil {
			t.Fatal(err)
		}
		if len(client.results) != index+1 || !strings.Contains(client.results[index], text) {
			t.Fatalf("turn %d tool results=%v", index, client.results)
		}
	}
	saved, err := store.GetActiveSession(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ScopeContext() != record.ScopeContext() {
		t.Fatal("working folder changed the conversation memory scope")
	}
	if saved.WorkspaceID != memory.WorkspaceID(ws.ID) {
		t.Fatal("session left its Workspace")
	}
}
