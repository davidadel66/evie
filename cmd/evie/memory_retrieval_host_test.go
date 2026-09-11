package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

type retrievalRuntimeClient struct{ requests []openrouter.ChatRequest }

func (c *retrievalRuntimeClient) ChatStream(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	message := openrouter.Message{Role: "assistant", Content: "Evidence received."}
	if len(c.requests) == 1 {
		message = openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{
			ID: "lookup-diet", Type: "function", Function: openrouter.FunctionCall{
				Name: "memory_search", Arguments: `{"query":"vegetarian"}`,
			},
		}}}
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: message}}}, nil
}

func TestRetrievalRuntimeMaintainsNewMemoryForCompleteREPLTurn(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	stop := startMemoryRetrievalHost(ctx, store)
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	waitForIndex := func() {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			coverage, err := store.MemoryIndexCoverage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.State == "active" && coverage.Pending == 0 {
				return
			}
			select {
			case <-ticker.C:
			case <-deadline.C:
				t.Fatalf("background index did not cover accepted memory: %+v", coverage)
			}
		}
	}
	waitForIndex()
	source, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := agent.NewWithToolset(nil, evieTestContextProfile("fixture"), store.BindHistory(source.ID, "owner"), source.ScopeContext(), store.BindTurnOwner(source.ID, "owner"), tools.NewToolset(nil))
	proposal, err := owner.PrepareRememberLiteral(ctx, store, "Remember that I am vegetarian.", memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:54000000-0000-4000-8000-000000000001", Predicate: "diet", PredicateLabel: "diet",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "vegetarian"},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := owner.ResolveRememberLiteral(ctx, store, proposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	waitForIndex()
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	reader, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	client := &retrievalRuntimeClient{}
	session := agent.NewWithToolset(client, evieTestContextProfile("fixture"), store.BindHistory(reader.ID, "reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "reader"), tools.NewToolset(definitions))
	var output bytes.Buffer
	runREPLContextIO(ctx, session, bufio.NewScanner(strings.NewReader("Look up my dietary preference.\n")), &output)
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2; REPL output: %s", len(client.requests), output.String())
	}
	payload, err := json.Marshal(client.requests[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"vegetarian", string(accepted.ClaimID), string(proposal.Source.EventID)} {
		if !strings.Contains(string(payload), expected) {
			t.Fatalf("provider request omitted maintained evidence %q: %s", expected, payload)
		}
	}
}
