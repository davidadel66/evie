package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type contextRejectingClient struct {
	rejections int
	requests   []openrouter.ChatRequest
}

func (c *contextRejectingClient) ChatStream(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	if len(c.requests) <= c.rejections {
		return openrouter.ChatResponse{}, &openrouter.StreamError{
			Kind: openrouter.StreamProviderError, HTTPStatus: 400, ContextLengthExceeded: true,
			Err: errors.New("api returned status 400"),
		}
	}
	return compactionAcceptanceResponse("answered after compaction"), nil
}

// C8: a provider context-length rejection compacts once and retries against
// real SQLite persistence, and the resulting history stays valid.
func TestProviderContextRejectionCompactsAndRetriesDurably(t *testing.T) {
	ctx := context.Background()
	store := eviedb.NewStore(openAcceptanceDB(t))
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const holder = memory.LeaseHolderID("context-rejection-acceptance")
	history := store.BindHistory(sessionRecord.ID, holder)
	lease, err := store.AcquireTurnLease(ctx, sessionRecord.ID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		root, err := history.Append(ctx, lease, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser,
			Content: fmt.Sprintf("source-%d:", i+1) + strings.Repeat(string(rune('a'+i)), 60_000),
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

	profile, err := openrouter.NewExplicitContextProfile("retry/model", 262_144, 262_144, 16_384)
	if err != nil {
		t.Fatal(err)
	}
	conversation := &contextRejectingClient{rejections: 1}
	compactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		compactionAcceptanceResponse(compactionAcceptanceSummary("after provider rejection")),
	}}
	session := agent.NewWithCompactor(conversation, compactor, profile, history,
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder))
	if err := session.Send(ctx, "continue", nilAgentEvents{}, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(conversation.requests) != 2 || len(compactor.requests) != 1 {
		t.Fatalf("conversation requests=%d compactor requests=%d", len(conversation.requests), len(compactor.requests))
	}
	events, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	tail := acceptanceEventTypes(events[6:])
	if fmt.Sprint(tail) != fmt.Sprint([]memory.EventType{
		memory.EventUserMessage, memory.EventContextSnapshot, memory.EventContextCompacted,
		memory.EventContextSnapshot, memory.EventAssistantMessage,
	}) {
		t.Fatalf("durable shape=%v", tail)
	}
	if _, err := session.InspectContext(ctx); err != nil {
		t.Fatalf("/context after retry: %v", err)
	}
}
