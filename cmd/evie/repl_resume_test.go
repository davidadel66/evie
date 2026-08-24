package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type resumeCaptureClient struct{ requests []openrouter.ChatRequest }

func (c *resumeCaptureClient) ChatStream(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "resumed",
	}}}}, nil
}

func TestSelectedGlobalAndRelocatedProjectSessionsResumeStoredScopeAndOrderedHistory(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := eviedb.NewStore(db)
	oldRoot, newRoot := t.TempDir(), t.TempDir()
	project, err := store.RegisterProject(context.Background(), "Project", oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	projectSession, err := store.CreateProjectSession(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	globalSession, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seedResumeHistory(t, store, projectSession)
	seedResumeHistory(t, store, globalSession)
	if _, err := store.RelocateProject(context.Background(), project.ID, newRoot); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		input     string
		want      memory.Session
		wantScope memory.ScopeContext
	}{
		{name: "project", input: "2\n", want: projectSession, wantScope: projectSession.ScopeContext()},
		{name: "global", input: "4\n", want: globalSession, wantScope: globalSession.ScopeContext()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var chooser bytes.Buffer
			selected, err := selectREPLSession(context.Background(), store, newRoot, bufio.NewScanner(strings.NewReader(tt.input)), &chooser)
			if err != nil {
				t.Fatal(err)
			}
			if selected.ID != tt.want.ID || selected.ScopeContext() != tt.wantScope {
				t.Fatalf("selected=%+v scope=%+v, want ID=%q scope=%+v", selected, selected.ScopeContext(), tt.want.ID, tt.wantScope)
			}
			if tt.name == "project" && !strings.Contains(chooser.String(), "stored root: "+escapedREPLPath(projectSession.ProjectRootSnapshot)) {
				t.Fatalf("relocation snapshot missing: %q", chooser.String())
			}

			client := &resumeCaptureClient{}
			holder := memory.LeaseHolderID("restart-" + tt.name)
			resumed := agent.New(client, "test", store.BindHistory(selected.ID, holder), selected.ScopeContext(), store.BindTurnOwner(selected.ID, holder))
			if err := resumed.Send(context.Background(), "after restart", &replEvents{out: io.Discard}, nil); err != nil {
				t.Fatal(err)
			}
			if len(client.requests) != 1 {
				t.Fatalf("provider requests=%d", len(client.requests))
			}
			messages := client.requests[0].Messages
			wantRoles := []string{"system", "user", "assistant", "user", "user"}
			wantContent := []string{"", "earlier", "old answer", "unfinished tools", "after restart"}
			if len(messages) != len(wantRoles) {
				t.Fatalf("resumed messages=%+v", messages)
			}
			for i := range wantRoles {
				if messages[i].Role != wantRoles[i] || (i > 0 && messages[i].Content != wantContent[i]) {
					t.Fatalf("message[%d]=%+v, want role=%q content=%q", i, messages[i], wantRoles[i], wantContent[i])
				}
			}
		})
	}
}

func seedResumeHistory(t *testing.T, store *eviedb.Store, session memory.Session) {
	t.Helper()
	holder := memory.LeaseHolderID("seed-" + session.ID)
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	history := store.BindHistory(session.ID, holder)
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		event, err := history.Append(context.Background(), lease, input)
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "earlier"})
	appendEvent(memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "old answer"})
	toolRoot := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "unfinished tools"})
	calls := []memory.ToolCall{{ID: "call-1", Name: "first", Arguments: `{}`}, {ID: "call-2", Name: "second", Arguments: `{}`}}
	payload, err := json.Marshal(memory.AssistantMessagePayload{ToolCalls: calls})
	if err != nil {
		t.Fatal(err)
	}
	assistant := appendEvent(memory.EventInput{ParentID: toolRoot.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: payload})
	resultPayload, err := json.Marshal(memory.ToolResultPayload{ToolCallID: "call-1"})
	if err != nil {
		t.Fatal(err)
	}
	appendEvent(memory.EventInput{ParentID: assistant.ID, Type: memory.EventToolSucceeded, Role: memory.RoleTool, Content: "partial", Payload: resultPayload})
	if err := store.ReleaseTurnLease(context.Background(), session.ID, holder, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
}
