package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type identityCLIExtractor struct{ subject memory.SemanticID }

func (identityCLIExtractor) ServerIdentity() string { return "scripted:identity-cli" }
func (e identityCLIExtractor) Extract(_ context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	var source memory.EvidenceLocator
	for _, s := range r.Window.Sources {
		if s.Usage == "new_support" {
			source = s.Locator
			break
		}
	}
	candidate := memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: e.subject, Polarity: memory.PolarityAffirmed}, Identity: &memory.CandidateIdentityProposal{Object: &memory.EntityMention{Name: "Maya", EntityType: "person", Support: source}, Predicate: &memory.PredicateDefinition{Token: "works_with", Label: "works with", ObjectConstraint: memory.ConstraintEntity, Cardinality: memory.CardinalityMany}, Uncertainty: "Choose which Maya; name alone is not identity."}, Support: []memory.EvidenceLocator{source}, Context: []memory.EvidenceLocator{}}
	raw, err := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{candidate}})
	return eviedb.CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}, err
}

func TestOwnerReviewIdentityCLIClosedSessionCompletePath(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "identity-cli", time.Minute)
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
	source := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	seed, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000150", SourceEventID: source.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ApplyRememberLiteral(ctx, lease, seed); err != nil {
		t.Fatal(err)
	}
	root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I work with Maya."})
	last := appendEvent(memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Recorded."})
	generation := reviewCLIGeneration()
	generation.EntityPolicy = memory.CompilerIdentityPolicyV2
	generation.PredicatePolicy = generation.EntityPolicy
	generation.ValidationPolicy = generation.EntityPolicy
	generation.EquivalencePolicy = generation.EntityPolicy
	generation.EffectPolicy = generation.EntityPolicy
	compiled, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, generation, identityCLIExtractor{seed.Subject.ID})
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("compile %+v %v", compiled, err)
	}
	if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		handled, err := runOwnerReviewManagement(ctx, append([]string{"memory-review"}, args...), &out, store)
		if !handled || err != nil {
			t.Fatalf("CLI %v: %v", args, err)
		}
		return out.Bytes()
	}
	var item memory.OwnerCandidate
	if err = json.Unmarshal(run("inspect", "--scope", "global", "--id", compiled.Candidates[0].ID), &item); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(item.Candidate.Proposal.Identity.Uncertainty, "name alone") {
		t.Fatal("CLI hid uncertainty")
	}
	var options memory.ReviewIdentityOptions
	if err = json.Unmarshal(run("alternatives", "--scope", "global", "--id", item.Ref.ID), &options); err != nil {
		t.Fatal(err)
	}
	choices := `{"subject":null,"object":{"entity_id":"","create":true},"predicate":{"predicate_id":"","create":true}}`
	if err = json.Unmarshal(run("choose", "--scope", "global", "--id", item.Ref.ID, "--options", options.SHA256, "--choices", choices), &item); err != nil {
		t.Fatal(err)
	}
	var preview memory.ReviewPreview
	if err = json.Unmarshal(run("prepare", "--scope", "global", "--id", item.Ref.ID, "--revision", fmt.Sprint(item.Ref.ReviewRevision), "--interpretation", fmt.Sprint(item.Ref.InterpretationRevision), "--action", "accept"), &preview); err != nil {
		t.Fatal(err)
	}
	var result memory.ReviewResult
	if err = json.Unmarshal(run("resolve", "--scope", "global", "--preview", preview.ID, "--digest", preview.SHA256, "--delivery", "idem:v1:91000000-0000-4000-8000-000000000151", "--action", "accept"), &result); err != nil || result.Operation == nil {
		t.Fatalf("CLI acceptance %+v %v", result, err)
	}
	var operation memory.OwnerReviewOperation
	if err = json.Unmarshal(run("operation", "--scope", "global", "--id", string(result.Operation.OperationID)), &operation); err != nil {
		t.Fatal(err)
	}
	if operation.Preview.Effect.Claims[0].ObjectEntity.CanonicalName != "Maya" || operation.Preview.Effect.Identity.Revision.Choices.Object.Create != true {
		t.Fatal("CLI operation lost explicit identity choice")
	}
	var historical memory.ReviewIdentityRevision
	if err = json.Unmarshal(run("identity-revision", "--scope", "global", "--id", item.Ref.ID, "--interpretation", "1"), &historical); err != nil || historical.AuditID != item.Identity.AuditID {
		t.Fatalf("CLI identity audit %+v %v", historical, err)
	}
	verified, err := store.VerifySemanticProjection(ctx)
	if err != nil || !verified.Valid {
		t.Fatalf("CLI replay %+v %v", verified, err)
	}
	for _, args := range [][]string{{"choose", "--scope", "global", "--id", item.Ref.ID, "--choices", choices}, {"alternatives", "--scope", "global", "--id", item.Ref.ID, "--choices", choices}, {"choose", "--scope", "global", "--id", item.Ref.ID, "--options", options.SHA256, "--choices", `{"subject":null,"object":{"entity_id":"","create":true},"predicate":null,"scope":"global"}`}} {
		var out bytes.Buffer
		if _, err = runOwnerReviewManagement(ctx, append([]string{"memory-review"}, args...), &out, store); err == nil || out.Len() != 0 {
			t.Fatalf("invalid identity CLI accepted %v", args)
		}
	}
}
