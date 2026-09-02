package main

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type noSemanticModelClient struct{ calls int }

func (c *noSemanticModelClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.calls++
	return openrouter.ChatResponse{}, nil
}

func TestREPLRememberApprovalAndEventlessInspectionUseSemanticInterface(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := &noSemanticModelClient{}
	holder := memory.LeaseHolderID("semantic-repl")
	session := agent.New(client, evieTestContextProfile("test"), store.BindHistory(storedSession.ID, holder),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, holder))
	var out bytes.Buffer
	runREPLContextIOWithMemory(ctx, session, bufio.NewScanner(strings.NewReader(
		"/remember timezone_name Detroit\ny\n/memory\n",
	)), &out, store)

	got := out.String()
	for _, want := range []string{
		"Memory proposal", "scope: global (expected revision 0)",
		"proposition: owner timezone_name \"Detroit\"", "evidence: event=",
		"Remembered Claim", "Semantic Memory — scope=global revision=1", "authority=owner_statement",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("REPL output %q does not contain %q", got, want)
		}
	}
	if client.calls != 0 {
		t.Fatalf("semantic commands made %d model calls", client.calls)
	}
	events, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Type != memory.EventUserMessage || events[1].Type != memory.EventApproval {
		t.Fatalf("events after remember and inspection = %+v", events)
	}
}

func TestREPLDeclinedRememberChangesNoSemanticState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := &noSemanticModelClient{}
	holder := memory.LeaseHolderID("semantic-repl-decline")
	session := agent.New(client, evieTestContextProfile("test"), store.BindHistory(storedSession.ID, holder),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, holder))
	var out bytes.Buffer
	runREPLContextIOWithMemory(ctx, session, bufio.NewScanner(strings.NewReader(
		"/remember timezone_name Detroit\nn\n/memory\n",
	)), &out, store)

	if got := out.String(); !strings.Contains(got, "Semantic Memory unchanged") ||
		!strings.Contains(got, "revision=0") || !strings.Contains(got, "No accepted Claims.") {
		t.Fatalf("declined output = %q", got)
	}
	if client.calls != 0 {
		t.Fatalf("declined semantic command made %d model calls", client.calls)
	}
}

func TestCancelledRememberApprovalChangesNoSemanticState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	holder := memory.LeaseHolderID("semantic-repl-cancel")
	session := agent.New(&noSemanticModelClient{}, evieTestContextProfile("test"), store.BindHistory(storedSession.ID, holder),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, holder))
	proposal, err := session.PrepareRememberLiteral(ctx, store, "/remember timezone_name Detroit", memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:70000000-0000-4000-8000-000000000050",
		Predicate:      "timezone_name", PredicateLabel: "timezone name",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ResolveRememberLiteral(ctx, store, proposal, tools.Expired); err != nil {
		t.Fatal(err)
	}
	inspection, err := session.InspectSemanticMemory(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 0 || len(inspection.Claims) != 0 {
		t.Fatalf("cancelled approval changed Semantic Memory: %+v", inspection)
	}
}
