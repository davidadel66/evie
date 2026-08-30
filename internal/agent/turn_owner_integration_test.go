package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
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

func integrationContextProfile(model string) openrouter.ContextProfile {
	profile, err := openrouter.NewExplicitContextProfile(model, 300000, 262144, 16384)
	if err != nil {
		panic(err)
	}
	return profile
}

func (noOpAgentEvents) Delta(string)                                  {}
func (noOpAgentEvents) Reasoning(string)                              {}
func (noOpAgentEvents) ReasoningDone()                                {}
func (noOpAgentEvents) AssistantDone(string)                          {}
func (noOpAgentEvents) ToolCall(string, string, string)               {}
func (noOpAgentEvents) ToolResult(string, string, bool)               {}
func (noOpAgentEvents) ResponseDiscarded(agent.DiscardReason, string) {}

type failingAgentClient struct {
	err error
}

func (c failingAgentClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	return openrouter.ChatResponse{}, c.err
}

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
		return agent.New(client, integrationContextProfile("test"), store.BindHistory(storedSession.ID, holder), storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, holder))
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

func TestAgentTerminalSafeContentMatchesProductionStorageAuthority(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	holder := memory.LeaseHolderID("holder")
	providerErr := &openrouter.StreamError{
		Kind:       openrouter.StreamProviderError,
		HTTPStatus: 503,
		Err:        errors.New("secret provider detail"),
	}
	session := agent.New(
		failingAgentClient{err: providerErr},
		integrationContextProfile("test"),
		store.BindHistory(storedSession.ID, holder),
		storedSession.ScopeContext(),
		store.BindTurnOwner(storedSession.ID, holder),
	)

	err = session.Send(context.Background(), "hello", noOpAgentEvents{}, nil)
	if !errors.Is(err, providerErr) {
		t.Fatalf("Send error=%v, want provider error", err)
	}
	events, err := store.LoadEvents(context.Background(), storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Type != memory.EventTurnFailed {
		t.Fatalf("events=%+v", events)
	}
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if events[1].Content != payload.SafeContent() ||
		payload.Classification != memory.ClassificationProviderError ||
		payload.Stage != memory.StageProvider {
		t.Fatalf("terminal=%+v payload=%+v", events[1], payload)
	}
	if strings.Contains(events[1].Content, "secret") || strings.Contains(string(events[1].Payload), "secret") {
		t.Fatalf("terminal leaked provider detail: content=%q payload=%s", events[1].Content, events[1].Payload)
	}
}

type toolCycleAgentClient struct {
	mu        sync.Mutex
	calls     int
	secondErr error
}

func (c *toolCycleAgentClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call == 1 {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
			Role: "assistant",
			ToolCalls: []openrouter.ToolCall{{
				ID: "call-1", Type: "function",
				Function: openrouter.FunctionCall{Name: "echo", Arguments: `{}`},
			}},
		}}}}, nil
	}
	if c.secondErr != nil {
		return openrouter.ChatResponse{}, c.secondErr
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "done",
	}}}}, nil
}

func (c *toolCycleAgentClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type cancelToolResultEvents struct {
	noOpAgentEvents
	cancel context.CancelFunc
}

func (e cancelToolResultEvents) ToolResult(string, string, bool) { e.cancel() }

func integrationEchoTool() tools.Tool {
	return tools.Tool{
		Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "echo"}},
		Execute: func(context.Context, string) (string, error) {
			return "ok", nil
		},
	}
}

func openAgentStores(t *testing.T) (*eviedb.Store, *eviedb.Store, memory.Session) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbA.Close() })
	dbB, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbB.Close() })
	storeA, storeB := eviedb.NewStore(dbA), eviedb.NewStore(dbB)
	storedSession, err := storeA.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return storeA, storeB, storedSession
}

func TestFinalToolResultCancellationPersistsPriorProviderTriggerAcrossStores(t *testing.T) {
	storeA, storeB, storedSession := openAgentStores(t)
	holder := memory.LeaseHolderID("holder")
	client := &toolCycleAgentClient{}
	session := agent.New(
		client, integrationContextProfile("test"), storeA.BindHistory(storedSession.ID, holder),
		storedSession.ScopeContext(), storeB.BindTurnOwner(storedSession.ID, holder),
	)
	ctx, cancel := context.WithCancel(context.Background())
	err := session.Send(ctx, "hello", cancelToolResultEvents{cancel: cancel}, nil, integrationEchoTool())
	if !errors.Is(err, context.Canceled) || err.Error() != context.Canceled.Error() {
		t.Fatalf("Send error=%v, want unjoined context.Canceled", err)
	}
	if client.callCount() != 1 {
		t.Fatalf("provider calls=%d, want one", client.callCount())
	}
	events, err := storeB.LoadEvents(context.Background(), storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("events=%+v", events)
	}
	root, outcome, terminal := events[0], events[3], events[4]
	if outcome.Type != memory.EventToolSucceeded || terminal.Type != memory.EventTurnInterrupted || terminal.ParentID != root.ID {
		t.Fatalf("root=%+v outcome=%+v terminal=%+v", root, outcome, terminal)
	}
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TurnID != root.ID || payload.Stage != memory.StageToolCommit ||
		payload.Classification != memory.ClassificationCallerCancelled {
		t.Fatalf("terminal=%+v payload=%+v", terminal, payload)
	}
}

func TestSecondCycleProviderFailurePersistsFinalOutcomeTriggerAcrossStores(t *testing.T) {
	storeA, storeB, storedSession := openAgentStores(t)
	holder := memory.LeaseHolderID("holder")
	providerErr := &openrouter.StreamError{Kind: openrouter.StreamProviderError, Err: errors.New("provider down")}
	client := &toolCycleAgentClient{secondErr: providerErr}
	session := agent.New(
		client, integrationContextProfile("test"), storeA.BindHistory(storedSession.ID, holder),
		storedSession.ScopeContext(), storeB.BindTurnOwner(storedSession.ID, holder),
	)
	err := session.Send(context.Background(), "hello", noOpAgentEvents{}, nil, integrationEchoTool())
	if !errors.Is(err, providerErr) {
		t.Fatalf("Send error=%v, want provider error", err)
	}
	events, err := storeB.LoadEvents(context.Background(), storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("events=%+v", events)
	}
	root, outcome, terminal := events[0], events[3], events[4]
	if outcome.Type != memory.EventToolSucceeded || terminal.Type != memory.EventTurnFailed || terminal.ParentID != outcome.ID {
		t.Fatalf("root=%+v outcome=%+v terminal=%+v", root, outcome, terminal)
	}
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TurnID != root.ID || payload.Stage != memory.StageProvider ||
		payload.Classification != memory.ClassificationProviderError {
		t.Fatalf("terminal=%+v payload=%+v", terminal, payload)
	}
}
