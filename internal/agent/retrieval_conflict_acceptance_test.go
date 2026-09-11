package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemorySearchTurnSuppliesConflictingClaimsAndNewerOwnerStatementWithoutOverwriting(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	source := f.global()
	owner := f.session(source, nil)
	var accepted []memory.SemanticID
	for _, city := range []string{"Boston", "Portland"} {
		proposal, err := owner.PrepareRememberLiteral(ctx, f.store, "Remember that I live in "+city+".", memory.RememberLiteralRequest{
			Destination: memory.MemoryEverywhere, IdempotencyKey: "idem:v1:" + uuid.NewString(),
			Predicate: "live_in", PredicateLabel: "live in", PredicateCardinality: memory.CardinalityOne,
			Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: city}, Polarity: memory.PolarityAffirmed,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := owner.ResolveRememberLiteral(ctx, f.store, proposal, tools.Approved)
		if err != nil {
			t.Fatal(err)
		}
		accepted = append(accepted, result.ClaimID)
	}
	newer := f.converse(source, "I live in Chicago now.")
	project, err := f.store.RegisterProject(ctx, "Other area", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private, err := f.store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	privateStatement := f.converse(private, "I live in Kyoto now; this belongs to the other project.")
	f.refresh()
	before, err := f.store.InspectClaims(ctx, source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Claims) != 2 || before.Claims[0].SubjectEntityID != before.Claims[1].SubjectEntityID || before.Claims[0].Predicate.ID != before.Claims[1].Predicate.ID {
		t.Fatalf("fixture must retain two active Claims about the same subject and predicate: %+v", before.Claims)
	}
	for _, claim := range before.Claims {
		if claim.Predicate.Cardinality != memory.CardinalityOne || !newer.RecordedAt.After(claim.TransactionTime) {
			t.Fatalf("new owner statement must follow both accepted single-value Claims: %+v", claim)
		}
	}
	client, _ := f.search(f.global(), "Boston")
	data := retrievalData(t, client.reqs[1])
	var supplied struct {
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
		t.Fatal(err)
	}
	var gotClaims []memory.SemanticID
	var newerExcerpt *memory.RetrievalEvidence
	for i := range supplied.Evidence {
		evidence := &supplied.Evidence[i]
		if evidence.Kind == memory.RetrievalAcceptedMemory {
			gotClaims = append(gotClaims, evidence.ClaimID)
			if !slices.Contains(accepted, evidence.ClaimID) || evidence.Status != memory.SemanticStatusActive || evidence.CurrentStatus != memory.SemanticStatusActive || evidence.Claim == nil || evidence.Claim.Predicate.Cardinality != memory.CardinalityOne || evidence.Claim.TransactionTime.IsZero() {
				t.Fatalf("retrieval changed acceptance, lifecycle or Claim authority: %+v", evidence)
			}
			if len(evidence.Conflicts) != 1 || evidence.Conflicts[0].Code != memory.ConflictOneCardinality || evidence.Conflicts[0].PredicateToken != "live_in" {
				t.Fatalf("conflicting accepted Claim lacks explicit Kernel conflict metadata: %+v", evidence)
			}
			pair := append([]memory.SemanticID(nil), evidence.Conflicts[0].ClaimIDs...)
			slices.Sort(pair)
			wantPair := append([]memory.SemanticID(nil), accepted...)
			slices.Sort(wantPair)
			if !reflect.DeepEqual(pair, wantPair) {
				t.Fatalf("conflict metadata names evidence outside the supplied eligible pair: %+v", evidence.Conflicts)
			}
		}
		if evidence.Kind == memory.RetrievalConversationExcerpt && len(evidence.Sources) == 1 && evidence.Sources[0].EventID == newer.ID {
			newerExcerpt = evidence
		}
	}
	slices.Sort(gotClaims)
	wantClaims := append([]memory.SemanticID(nil), accepted...)
	slices.Sort(wantClaims)
	if !reflect.DeepEqual(gotClaims, wantClaims) {
		t.Fatalf("Boston lookup must expose both still-active conflicting Claims: got=%v want=%v; evidence=%s", gotClaims, wantClaims, data)
	}
	if newerExcerpt == nil || newerExcerpt.Text != newer.Content || newerExcerpt.ClaimID != "" || newerExcerpt.Claim != nil || len(newerExcerpt.Conflicts) != 0 || !slices.Contains(newerExcerpt.Paths, "newer_owner_statement") || !slices.Contains(newerExcerpt.RelatedClaimIDs, accepted[0]) {
		t.Fatalf("newer owner wording must accompany saved Claims as a potential discrepancy, without becoming accepted truth: %+v", newerExcerpt)
	}
	for _, id := range newerExcerpt.RelatedClaimIDs {
		if !slices.Contains(gotClaims, id) {
			t.Fatalf("newer statement refers to a Claim not supplied in the request: %s", id)
		}
	}
	statementSource := newerExcerpt.Sources[0]
	observed, err := time.Parse(time.RFC3339Nano, statementSource.ObservedAt)
	if err != nil || !observed.Equal(newer.RecordedAt) || statementSource.SessionID != source.ID || statementSource.Actor != memory.SemanticActorOwner || statementSource.Authority != memory.AuthorityOwnerStatement || statementSource.EvidenceSHA256 != memory.CompilerHash([]byte(newer.Content)) {
		t.Fatalf("newer owner attribution or original observation time changed: %+v %v", statementSource, err)
	}
	for _, forbidden := range []string{"Kyoto", string(privateStatement.ID), string(private.ID), string(project.ID)} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("matching companion disclosed another conversation area: %q", forbidden)
		}
	}
	after, err := f.store.InspectClaims(ctx, source.ScopeContext(), memory.ClaimQuery{})
	if err != nil || !reflect.DeepEqual(before.Claims, after.Claims) || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
		t.Fatalf("discrepancy retrieval overwrote or accepted memory: %v", err)
	}
}

func TestMemoryRepeatedConversationReadPreservesStillEligibleDiscrepancySupport(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	saved := readerRemember(t, f, source, "home_city", "Remember that I live in Boston.", "Boston")
	newer := f.converse(source, "An update: I now live in Chicago after moving from Boston.")
	f.refresh()
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("saved", "memory_search", `{"query":"Boston"}`)),
		assistantStep("", nil, toolCall("original", "memory_search_conversations", `{"query":"Chicago"}`)),
		assistantStep("The saved record and newer statement differ.", nil),
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Check my saved city and the newer statement.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	for _, request := range client.reqs[1:] {
		var data struct {
			Evidence []memory.RetrievalEvidence `json:"evidence"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, request), "EVIE_MEMORY_DATA\n")), &data); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, evidence := range data.Evidence {
			if evidence.Kind == memory.RetrievalConversationExcerpt && evidence.Sources[0].EventID == newer.ID {
				found = slices.Contains(evidence.RelatedClaimIDs, saved.ClaimID) && slices.Contains(evidence.Paths, "newer_owner_statement")
			}
		}
		if !found {
			t.Fatal("re-reading the same original statement erased its still-eligible discrepancy support")
		}
	}
}
