package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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
		"/remember timezone_name Detroit\ny\n/memory --scope global --page-size 20\n",
	)), &out, store)

	got := out.String()
	for _, want := range []string{
		"Memory proposal", "scope: global (expected revision 0)",
		"proposition: owner timezone_name \"Detroit\"", "evidence: event=",
		"Remembered Claim", "Semantic Memory — scope=global", "mode=current", "valid_at=", "as_known_at=",
		"Entity ", "Claim ", "Predicate timezone_name@1", "Source Link ", "authority=owner_statement",
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

func TestREPLMemoryInspectVerifyAndRebuildAreEventlessOwnerCommands(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := &noSemanticModelClient{}
	holder := memory.LeaseHolderID("semantic-owner-inspection")
	session := agent.New(client, evieTestContextProfile("test"), store.BindHistory(storedSession.ID, holder),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, holder))
	proposal, err := session.PrepareRememberLiteral(ctx, store, "/remember timezone_name Detroit", memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:70000000-0000-4000-8000-000000000060",
		Predicate:      "timezone_name", PredicateLabel: "timezone name",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ResolveRememberLiteral(ctx, store, proposal, tools.Approved); err != nil {
		t.Fatal(err)
	}
	before, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	runREPLContextIOWithMemory(ctx, session, bufio.NewScanner(strings.NewReader(
		fmt.Sprintf("/memory inspect claim %s --scope global --history\n/memory verify\n/memory rebuild\n", proposal.ClaimID),
	)), &out, store)

	got := out.String()
	for _, want := range []string{
		"Memory detail — scope=global kind=claim", "status=active", "lifecycle", "sources", "operations",
		"schema_version", "proposal_sha256", "valid_at", "as_known_at",
		"Memory verification progress: replaying accepted operations", "verification outcome: valid",
		"Memory rebuild progress: acquiring owner maintenance fence", "rebuild outcome: recovered", "fencing token:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("owner memory output %q does not contain %q", got, want)
		}
	}
	if client.calls != 0 {
		t.Fatalf("owner memory commands made %d model calls", client.calls)
	}
	after, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("owner memory commands appended Episodic events: before=%d after=%d", len(before), len(after))
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	inspection, err := eviedb.NewStore(reopened).InspectSemanticObjectAt(ctx, storedSession.ScopeContext(), memory.SemanticObjectClaim, proposal.ClaimID, memory.ClaimQuery{ScopeKey: "global"})
	if err != nil || inspection.Status != memory.SemanticStatusActive || len(inspection.Operations) != 1 {
		t.Fatalf("reopened owner detail: result=%+v error=%v", inspection, err)
	}
}

func TestREPLMemorySurfacesStaleCursorAndQuarantineWithoutEvents(t *testing.T) {
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
	holder := memory.LeaseHolderID("semantic-owner-diagnostics")
	session := agent.New(client, evieTestContextProfile("test"), store.BindHistory(storedSession.ID, holder),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, holder))
	seed := func(sequence int, value string) memory.RememberLiteralProposal {
		t.Helper()
		proposal, err := session.PrepareRememberLiteral(ctx, store, "/remember timezone_name "+value, memory.RememberLiteralRequest{
			IdempotencyKey: fmt.Sprintf("idem:v1:70000000-0000-4000-8000-%012d", sequence),
			Predicate:      "timezone_name", PredicateLabel: "timezone name",
			Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := session.ResolveRememberLiteral(ctx, store, proposal, tools.Approved); err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	firstProposal := seed(61, "Detroit")
	_ = seed(62, "Chicago")

	var out bytes.Buffer
	if !handleMemoryCommand(ctx, session, store, "/memory --scope global --kind claim --page-size 1", &out) {
		t.Fatal("memory listing command was not handled")
	}
	cursorMarker := "next cursor: "
	cursorStart := strings.Index(out.String(), cursorMarker)
	if cursorStart < 0 {
		t.Fatalf("first page omitted cursor: %q", out.String())
	}
	cursor := strings.TrimSpace(strings.SplitN(out.String()[cursorStart+len(cursorMarker):], "\n", 2)[0])
	_ = seed(63, "Cleveland")
	before, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}

	out.Reset()
	handleMemoryCommand(ctx, session, store, "/memory --scope global --kind claim --page-size 1 --cursor "+cursor, &out)
	if !strings.Contains(out.String(), "stale cursor") {
		t.Fatalf("stale cursor output = %q", out.String())
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_claims_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE semantic_claims SET literal_value = 'projection mismatch' WHERE claim_id = ?`, firstProposal.ClaimID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER semantic_claims_append_only_update BEFORE UPDATE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END`); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	handleMemoryCommand(ctx, session, store, "/memory verify", &out)
	for _, want := range []string{"Memory verification progress", "verification outcome: mismatch", "quarantined=true", "mismatch table=semantic_claims"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("verification mismatch output %q does not contain %q", out.String(), want)
		}
	}
	out.Reset()
	handleMemoryCommand(ctx, session, store, "/memory --scope global", &out)
	if !strings.Contains(out.String(), "scope is quarantined") || !strings.Contains(out.String(), "canonical replay mismatch") {
		t.Fatalf("quarantine output = %q", out.String())
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_operations_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA ignore_check_constraints = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE semantic_operations SET schema_version = 99 WHERE operation_id = ?`, firstProposal.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA ignore_check_constraints = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER semantic_operations_append_only_update BEFORE UPDATE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END`); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	handleMemoryCommand(ctx, session, store, "/memory verify", &out)
	if !strings.Contains(out.String(), "schema version 99") || !strings.Contains(out.String(), "unknown operation kind") {
		t.Fatalf("unsupported-version output = %q", out.String())
	}
	after, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) || client.calls != 0 {
		t.Fatalf("diagnostic commands changed events or called model: before=%d after=%d model_calls=%d", len(before), len(after), client.calls)
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
