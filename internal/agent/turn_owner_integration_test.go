package agent_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type blockingAgentClient struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (c *blockingAgentClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.calls++
	if c.entered != nil {
		close(c.entered)
	}
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return openrouter.ChatResponse{}, ctx.Err()
		}
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "done",
	}}}}, nil
}

type noOpAgentEvents struct{}

func (noOpAgentEvents) Delta(string)                                  {}
func (noOpAgentEvents) Reasoning(string)                              {}
func (noOpAgentEvents) ReasoningDone()                                {}
func (noOpAgentEvents) AssistantDone(string)                          {}
func (noOpAgentEvents) ToolCall(string, string, string)               {}
func (noOpAgentEvents) ToolResult(string, string, bool)               {}
func (noOpAgentEvents) ResponseDiscarded(agent.DiscardReason, string) {}

func TestTwoStoresAllowOnlyOneLiveAgentTurnForSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	storeA, storeB := eviedb.NewStore(dbA), eviedb.NewStore(dbB)
	storedSession, err := storeA.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	clientA := &blockingAgentClient{entered: make(chan struct{}), release: make(chan struct{})}
	clientB := &blockingAgentClient{}
	newSession := func(store *eviedb.Store, holder memory.LeaseHolderID, client agent.Client) *agent.Session {
		return agent.New(client, "test", store.BindHistory(storedSession.ID, holder), storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, holder))
	}
	sessionA := newSession(storeA, "holder-a", clientA)
	sessionB := newSession(storeB, "holder-b", clientB)

	done := make(chan error, 1)
	go func() { done <- sessionA.Send(context.Background(), "first", noOpAgentEvents{}, nil) }()
	select {
	case <-clientA.entered:
	case <-time.After(time.Second):
		t.Fatal("first owner did not reach provider")
	}

	err = sessionB.Send(context.Background(), "competing", noOpAgentEvents{}, nil)
	if !errors.Is(err, agent.ErrLeaseConflict) {
		t.Fatalf("competing Send error=%v, want ErrLeaseConflict", err)
	}
	if clientB.calls != 0 {
		t.Fatalf("competing provider calls=%d", clientB.calls)
	}
	close(clientA.release)
	if err := <-done; err != nil {
		t.Fatalf("first Send: %v", err)
	}
	events, err := storeA.LoadEvents(context.Background(), storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Content != "first" || events[1].Type != memory.EventAssistantMessage {
		t.Fatalf("accepted events=%+v", events)
	}
}
