package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

// scriptedThinker is the browser fixture's model. A message containing
// "slow" thinks aloud for several seconds, one sentence at a time, before
// answering; one containing "approve" asks to edit note; anything else is
// answered at once, echoing the message.
type scriptedThinker struct{ note string }

func (m scriptedThinker) ChatStream(ctx context.Context, request openrouter.ChatRequest, handlers openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	last := ""
	for _, message := range request.Messages {
		if message.Role == "user" {
			last = message.Content
		}
	}
	if final := request.Messages[len(request.Messages)-1]; final.Role == "tool" {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "Edited the note."}}}}, nil
	}
	if strings.Contains(last, "approve") {
		args := fmt.Sprintf(`{"path":%q,"old_string":"world","new_string":"evie"}`, m.note)
		call := openrouter.ToolCall{ID: "edit-" + fmt.Sprint(time.Now().UnixNano()), Type: "function", Function: openrouter.FunctionCall{Name: "edit_file", Arguments: args}}
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{call}}}}}, nil
	}
	answer := "Quick answer to: " + last
	if strings.Contains(last, "slow") {
		for _, sentence := range []string{"I need to read the notes first. ", "Let me compare the two lists. ", "Now I will draft the summary. ", "Checking the totals once more. "} {
			for _, word := range strings.SplitAfter(sentence, " ") {
				if handlers.OnReasoning != nil && word != "" {
					handlers.OnReasoning(word)
				}
				select {
				case <-time.After(250 * time.Millisecond):
				case <-ctx.Done():
					return openrouter.ChatResponse{}, ctx.Err()
				}
			}
		}
		answer = "Here is the slow summary."
	}
	if handlers.OnContent != nil {
		handlers.OnContent(answer)
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: answer}}}}, nil
}

// Optional real-browser fixture for multi-chat.spec.md: a temporary database
// with one Workspace and two titled chats, served with the scripted model.
func TestMultiChatBrowserFixture(t *testing.T) {
	if os.Getenv("EVIE_MULTICHAT_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in multi-chat browser fixture")
	}
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "browser.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	profile, err := openrouter.NewExplicitContextProfile("test/model", 300000, 200000, 12000)
	if err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(note, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	controller := newWebContextSessionController(store, sessionCompositionManager(t), func(session memory.Session, composition plugins.ResolvedComposition) (*agent.Session, error) {
		holder := memory.LeaseHolderID("web-" + session.ID)
		return agent.NewWithToolset(scriptedThinker{note: note}, profile, store.BindHistory(session.ID, holder), session.ScopeContext(), store.BindTurnOwner(session.ID, holder), composition.Toolset), nil
	})
	workspace, err := store.RegisterWorkspace(ctx, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	var ids []memory.SessionID
	for _, title := range []string{"Notes chat", "Groceries chat"} {
		opened, err := controller.SelectSession(ctx, web.ContextSessionSelection{WorkspaceID: workspace.ID, WorkspaceRevision: workspace.CurrentRevisionID})
		if err != nil {
			t.Fatal(err)
		}
		if err := opened.Agent.Send(ctx, title, &replEvents{out: os.Stderr}, nil); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, opened.Session.ID)
	}
	server := web.NewContextServer(nil, nil, store, controller)
	defer server.Close()
	host := httptest.NewServer(server.Handler())
	defer host.Close()
	fmt.Printf("MULTICHAT_BROWSER_READY=%s %s %s\n", host.URL, ids[0], ids[1])
	var stop [1]byte
	_, _ = os.Stdin.Read(stop[:])
}
