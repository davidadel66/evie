package eviedb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// rememberLegacyWhole accepts a remember operation in the pre-Stage-14 shape:
// the whole owner message cited as owner_statement. Accepted history still
// holds such operations, and Apply and replay still accept them.
func rememberLegacyWhole(t *testing.T, ctx context.Context, store *Store, session memory.Session, content, predicate, value, key string) memory.RememberLiteralProposal {
	t.Helper()
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID("legacy-"+key), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:" + key, SourceEventID: event.ID, Predicate: predicate, PredicateLabel: strings.ReplaceAll(predicate, "_", " "),
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal.Source.LocatorKind, proposal.Source.LocatorValue = memory.LocatorWhole, ""
	proposal.Source.Evidence, proposal.Source.EvidenceSHA256 = content, evidenceHash(content)
	proposal.Source.Authority = memory.AuthorityOwnerStatement
	if proposal.ProposalSHA256, _, err = semanticHash(canonicalRememberLiteralProposal(proposal)); err != nil {
		t.Fatal(err)
	}
	if proposal.PreparedSHA256, _, err = semanticHash(proposal); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberLiteral(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	return proposal
}

func TestLegacyWholeGlobalSourceShowsOnlyItsValueSentenceInOtherScopes(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	global, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	command := "My blood test came back and the clinic wants a follow-up. Remember that my favorite color is teal. The kids are sick."
	sentence := "Remember that my favorite color is teal."
	teal := rememberLegacyWhole(t, ctx, store, global, command, "favorite_color", "teal", "71400000-0000-4000-8000-000000000001")
	unmatched := "Read the article about gardening. Keep what matters."
	lisbon := rememberLegacyWhole(t, ctx, store, global, unmatched, "home_city", "Lisbon", "71400000-0000-4000-8000-000000000002")
	workspace, err := store.RegisterWorkspace(ctx, "Garden planning")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		coverage, err := store.RefreshMemoryIndex(ctx, 256)
		if err != nil {
			t.Fatal(err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			break
		}
	}

	type view struct {
		name   string
		reader memory.Session
		want   map[memory.SemanticID]string
	}
	for _, v := range []view{
		{"Workspace reader", reader, map[memory.SemanticID]string{teal.ClaimID: sentence, lisbon.ClaimID: ""}},
		{"Global reader keeps today's rendering", global, map[memory.SemanticID]string{teal.ClaimID: command, lisbon.ClaimID: unmatched}},
	} {
		t.Run(v.name, func(t *testing.T) {
			check := func(path string, claim memory.SemanticID, source memory.SemanticSource) {
				t.Helper()
				if source.LocatorKind != memory.LocatorWhole || source.Authority != memory.AuthorityOwnerStatement || source.EvidenceSHA256 != evidenceHash(map[memory.SemanticID]string{teal.ClaimID: command, lisbon.ClaimID: unmatched}[claim]) {
					t.Fatalf("%s: legacy source lost its identity or label: %+v", path, source)
				}
				if source.Evidence != v.want[claim] {
					t.Fatalf("%s: legacy source rendered %q, want %q", path, source.Evidence, v.want[claim])
				}
			}
			for _, query := range []string{"teal", "Lisbon"} {
				result, err := store.SearchMemory(ctx, v.reader.ScopeContext(), memory.RetrievalQuery{Text: query})
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, evidence := range result.Evidence {
					if evidence.ClaimID == teal.ClaimID || evidence.ClaimID == lisbon.ClaimID {
						found = true
						check("search "+query, evidence.ClaimID, evidence.Sources[0])
					}
				}
				if !found {
					t.Fatalf("search %q lost the legacy Claim: %+v", query, result.Evidence)
				}
			}
			for claim, sourceLink := range map[memory.SemanticID]memory.SemanticID{teal.ClaimID: teal.SourceLinkID, lisbon.ClaimID: lisbon.SourceLinkID} {
				inspected, err := store.InspectSemanticObject(ctx, v.reader.ScopeContext(), memory.SemanticObjectClaim, claim)
				if err != nil || len(inspected.Sources) != 1 {
					t.Fatalf("inspect Claim: %+v: %v", inspected.Sources, err)
				}
				check("Claim inspection", claim, inspected.Sources[0].Source)
				link, err := store.InspectSemanticObject(ctx, v.reader.ScopeContext(), memory.SemanticObjectSourceLink, sourceLink)
				if err != nil || link.Source == nil {
					t.Fatalf("inspect Source Link: %v", err)
				}
				check("Source Link inspection", claim, *link.Source)
			}
			claims, err := store.InspectClaims(ctx, v.reader.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			for _, claim := range claims.Claims {
				check("Claim query", claim.ID, claim.Sources[0])
			}
			page, err := store.ListSemanticObjects(ctx, v.reader.ScopeContext(), memory.SemanticObjectListQuery{Kinds: []memory.SemanticObjectKind{memory.SemanticObjectSourceLink}, PageSize: 100})
			if err != nil {
				t.Fatal(err)
			}
			listed := 0
			defer func() {
				if listed != 2 {
					t.Errorf("object listing returned %d legacy Source Links, want 2", listed)
				}
			}()
			for _, row := range page.Objects {
				if row.Source == nil {
					continue
				}
				listed++
				claim := teal.ClaimID
				if row.Source.EventID == lisbon.Source.EventID {
					claim = lisbon.ClaimID
				}
				check("object listing", claim, *row.Source)
			}
		})
	}
}

// The ambiguity marker is presentation for the reader. The dense document is
// built from the plain Claim text, so a later same-named Entity cannot make an
// indexed vector look stale (harness review M6).
func TestDenseClaimDocumentIgnoresAmbiguityMarkers(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	global, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	remember := func(key, content, object string) memory.RememberEntityProposal {
		t.Helper()
		lease, err := store.AcquireTurnLease(ctx, global.ID, memory.LeaseHolderID("dense-"+key), time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		event, err := store.AppendEventWithLease(ctx, global.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberEntity(ctx, global.ScopeContext(), memory.RememberEntityRequest{
			IdempotencyKey: "idem:v1:" + key, SourceEventID: event.ID, Predicate: "plays", PredicateLabel: "plays",
			Subject: memory.EntitySelector{Create: true, CanonicalName: "Sarah", EntityType: "person", Alias: "Sarah"},
			Object:  memory.EntitySelector{Create: true, CanonicalName: object, EntityType: "sport", Alias: object},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyRememberEntity(ctx, lease, proposal); err != nil {
			t.Fatal(err)
		}
		if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	first := remember("71400000-0000-4000-8000-000000000011", "My sister Sarah plays Tennis.", "Tennis")
	before, eligible, err := store.denseClaimDocument(ctx, db, first.Claim.ID)
	if err != nil || !eligible {
		t.Fatalf("dense document: %v", err)
	}
	remember("71400000-0000-4000-8000-000000000012", "My coworker Sarah plays Squash.", "Squash")
	after, eligible, err := store.denseClaimDocument(ctx, db, first.Claim.ID)
	if err != nil || !eligible {
		t.Fatalf("dense document: %v", err)
	}
	if after.text != before.text || after.contentHash != before.contentHash || after.text != "Sarah — plays: Tennis" {
		t.Fatalf("a new same-named Entity changed the indexed dense document: %q -> %q", before.text, after.text)
	}
}
