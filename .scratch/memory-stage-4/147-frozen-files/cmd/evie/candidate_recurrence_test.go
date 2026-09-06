package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewCLIGenerationLineageClosedSessionAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "recurrence-cli", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	seedEvent := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink tea."})
	seed, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000147", SourceEventID: seedEvent.ID, Predicate: "drink", PredicateLabel: "Drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ApplyRememberLiteral(ctx, lease, seed); err != nil {
		t.Fatal(err)
	}
	root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer coffee and tea."})
	last := appendEvent(memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
	selection := memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}
	extractor := &batchCLIExtractor{CompilerExtractor: reviewCLIExtractor{seed.Subject.ID, seed.Predicate.ID}}
	first, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), selection, reviewCLIGeneration(), extractor)
	if err != nil {
		t.Fatal(err)
	}
	run := func(args []string, target any) {
		t.Helper()
		var out bytes.Buffer
		handled, err := runOwnerReviewManagement(ctx, args, &out, store)
		if !handled || err != nil {
			t.Fatalf("%v: %s %v", args, out.String(), err)
		}
		if err = json.Unmarshal(out.Bytes(), target); err != nil {
			t.Fatal(err)
		}
	}
	var original memory.OwnerCandidate
	run([]string{"memory-review", "inspect", "--scope", "global", "--id", first.Candidates[0].ID}, &original)
	proposal := original.Candidate.Proposal
	proposal.Proposition.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}
	var edited memory.OwnerCandidate
	run([]string{"memory-review", "edit", "--scope", "global", "--id", original.Ref.ID, "--proposal", batchCLIJSON(t, proposal), "--reason", "Preserve the owner interpretation."}, &edited)
	var preview memory.ReviewPreview
	run([]string{"memory-review", "prepare", "--scope", "global", "--id", edited.Ref.ID, "--revision", strconv.FormatInt(edited.Ref.ReviewRevision, 10), "--interpretation", strconv.FormatInt(edited.Ref.InterpretationRevision, 10), "--action", "reject"}, &preview)
	var resolved memory.ReviewResult
	run([]string{"memory-review", "resolve", "--scope", "global", "--preview", preview.ID, "--digest", preview.SHA256, "--delivery", "idem:v1:91000000-0000-4000-8000-000000000148", "--action", "reject", "--reason", "Do not store this extracted memory."}, &resolved)
	generation := reviewCLIGeneration()
	generation.Decoding.Seed++
	second, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), selection, generation, extractor)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	var lineage memory.CandidateLineage
	run([]string{"memory-review", "lineage", "--scope", "global", "--id", second.Candidates[0].ID}, &lineage)
	if !lineage.Suppressed || lineage.Candidate.Candidate.ReviewState != "unresolved" || lineage.Origin == nil || lineage.Origin.Edit == nil || lineage.Origin.Candidate.Proposal.Proposition.Object.Literal.Value != "tea" || lineage.Candidate.Candidate.Proposal.Proposition.Object.Literal.Value != "coffee" || lineage.Resolution == nil || lineage.Resolution.AuditID != resolved.AuditID || lineage.Selection != selection || lineage.Generation.Decoding.Seed != generation.Decoding.Seed {
		t.Fatalf("CLI lineage %+v", lineage)
	}
	reopened, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	store = eviedb.NewStore(reopened)
	var after memory.CandidateLineage
	run([]string{"memory-review", "lineage", "--scope", "global", "--id", second.Candidates[0].ID}, &after)
	if batchCLIJSON(t, after) != batchCLIJSON(t, lineage) {
		t.Fatal("lineage changed on reopen")
	}
	verification, err := store.VerifySemanticProjection(ctx)
	if err != nil || !verification.Valid || extractor.calls != 2 {
		t.Fatalf("replay depended on extraction %+v %v calls=%d", verification, err, extractor.calls)
	}
}
