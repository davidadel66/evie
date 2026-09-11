package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
)

func investigationWorkspaceSessions(t *testing.T, f *retrievalFixture) (memory.Session, memory.Session) {
	t.Helper()
	ctx := context.Background()
	workspace, err := f.store.RegisterWorkspace(ctx, "Cross-scope investigation")
	if err != nil {
		t.Fatal(err)
	}
	composition, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	create := func() memory.Session {
		session, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	return create(), create()
}

func TestMemoryInvestigationRefreshesRestoredConflictsAcrossAuthorizedScopes(t *testing.T) {
	for _, restored := range []string{"claim", "source"} {
		for _, view := range []string{"current", "knowledge pin", "historical", "valid pin"} {
			t.Run(restored+"/"+view, func(t *testing.T) {
				f := newRetrievalFixture(t)
				source, reader := investigationWorkspaceSessions(t, f)
				boston := readerRemember(t, f, f.global(), "live_in", "Remember that I live in Boston.", "Boston")
				chicago := readerRemember(t, f, source, "live_in", "Remember that I live in Chicago.", "Chicago")
				if boston.Scope.Key != "global" || chicago.Scope.Key == boston.Scope.Key || chicago.Subject.ID != boston.Subject.ID || chicago.Predicate.ID != boston.Predicate.ID {
					t.Fatal("fixture requires Global and Workspace Claims about the same owner and single-valued predicate")
				}
				if restored == "claim" {
					f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, chicago.ClaimID)
				} else {
					f.lifecycle(source, "memory_retract_source", memory.SemanticObjectSourceLink, chicago.SourceLinkID)
				}
				f.refresh()
				query := investigationReadView(view)
				client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
					switch call {
					case 0:
						return assistantStep("", nil, toolCall("initial", "memory_search", query))
					case 1:
						first := expansionBoundaryEvidence(t, request)
						if len(first) != 1 || first[0].ClaimID != boston.ClaimID || len(first[0].Conflicts) != 0 {
							t.Fatalf("initial view must contain only the eligible Global Claim: %+v", first)
						}
						if restored == "claim" {
							f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, chicago.ClaimID)
						} else {
							f.lifecycle(source, "memory_restore_source", memory.SemanticObjectSourceLink, chicago.SourceLinkID)
						}
						return assistantStep("", nil, toolCall("continue-1", "continue_check", `{}`))
					case 2:
						return assistantStep("", nil, toolCall("continue-2", "continue_check", `{}`))
					default:
						return assistantStep("The conflicting available sources remain distinct.", nil)
					}
				}}
				if err := f.session(reader, client, investigationContinuationTool()).Send(context.Background(), "Inspect Boston, then continue twice.", &recorder{}, nil); err != nil {
					t.Fatal(err)
				}
				first := expansionBoundaryEvidence(t, client.reqs[1])
				final := expansionBoundaryEvidence(t, client.reqs[2])
				wantRefreshes := 0
				if view == "current" || view == "valid pin" {
					wantRefreshes = 1
					if len(final) != 2 {
						t.Fatalf("restoring an authorized Workspace %s must refresh the held Global interpretation: %+v", restored, final)
					}
					wantPair := []memory.SemanticID{boston.ClaimID, chicago.ClaimID}
					slices.Sort(wantPair)
					for _, item := range final {
						if !slices.Contains(wantPair, item.ClaimID) || len(item.Conflicts) != 1 || !reflect.DeepEqual(item.Conflicts[0].ClaimIDs, wantPair) || !item.AsKnownAt.After(first[0].AsKnownAt) {
							t.Fatalf("refreshed evidence must include the authorized conflict at its new knowledge date: %+v", item)
						}
						if item.ClaimID == chicago.ClaimID && (item.ScopeKey != chicago.Scope.Key || len(item.Sources) != 1 || item.Sources[0].EventID != chicago.Source.EventID || item.Sources[0].Evidence != chicago.Source.Evidence) {
							t.Fatalf("Workspace conflict lost its original scoped source: %+v", item)
						}
						if view == "valid pin" && (!item.ValidAtConstrained || !item.ValidAt.Equal(first[0].ValidAt)) {
							t.Fatalf("refresh advanced the explicit world-validity pin: %+v", item)
						}
					}
				} else if len(final) != 1 || !reflect.DeepEqual(first[0].Reference(), final[0].Reference()) {
					t.Fatalf("cross-scope restoration advanced the explicit %s view: %+v", view, final)
				}
				receipts := investigationReceipts(t, f, reader)
				if len(receipts) != 3 || !reflect.DeepEqual(receipts[0].Evidence, []memory.RetrievalReference{first[0].Reference()}) || !reflect.DeepEqual(receipts[1].Evidence, receipts[2].Evidence) {
					t.Fatalf("refresh changed the original receipt or repeated unchanged retrieval: %+v", receipts)
				}
				if last := receipts[2].Investigation; last == nil || last.RefreshAttempts != wantRefreshes || last.SearchAttempts != 1+wantRefreshes {
					t.Fatalf("cross-scope restoration must refresh only the changed current view: %+v", last)
				}
			})
		}
	}
}

func TestMemoryInvestigationIgnoresRestoredConflictsOutsideAuthorizedScopes(t *testing.T) {
	for _, restored := range []string{"claim", "source"} {
		t.Run(restored, func(t *testing.T) {
			f := newRetrievalFixture(t)
			_, reader := investigationWorkspaceSessions(t, f)
			boston := readerRemember(t, f, f.global(), "live_in", "Remember that I live in Boston.", "Boston")
			project, err := f.store.RegisterProject(context.Background(), "Excluded investigation area", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			private, err := f.store.CreateProjectSession(context.Background(), project.ID)
			if err != nil {
				t.Fatal(err)
			}
			// More than eight excluded peers must neither consume the authorized
			// candidate allowance nor trigger the overflow refresh sentinel.
			var peers []memory.RememberLiteralProposal
			for i := 0; i < 9; i++ {
				city := fmt.Sprintf("Osaka-secluded-%d", i)
				peer := readerRemember(t, f, private, "live_in", "Remember that I live in "+city+".", city)
				if peer.Subject.ID != boston.Subject.ID || peer.Predicate.ID != boston.Predicate.ID {
					t.Fatal("excluded peers must share the Global subject and predicate")
				}
				if restored == "claim" {
					f.lifecycle(private, "memory_retire", memory.SemanticObjectClaim, peer.ClaimID)
				} else {
					f.lifecycle(private, "memory_retract_source", memory.SemanticObjectSourceLink, peer.SourceLinkID)
				}
				peers = append(peers, peer)
			}
			f.refresh()
			client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
				switch call {
				case 0:
					return assistantStep("", nil, toolCall("initial", "memory_search", `{"query":"Boston"}`))
				case 1:
					first := expansionBoundaryEvidence(t, request)
					if len(first) != 1 || first[0].ClaimID != boston.ClaimID || len(first[0].Conflicts) != 0 {
						t.Fatalf("initial view must contain only the authorized Global Claim: %+v", first)
					}
					for _, peer := range peers {
						if restored == "claim" {
							f.lifecycle(private, "memory_restore", memory.SemanticObjectClaim, peer.ClaimID)
						} else {
							f.lifecycle(private, "memory_restore_source", memory.SemanticObjectSourceLink, peer.SourceLinkID)
						}
					}
					return assistantStep("", nil, toolCall("continue", "continue_check", `{}`))
				default:
					return assistantStep("The same authorized evidence remains available.", nil)
				}
			}}
			if err := f.session(reader, client, investigationContinuationTool()).Send(context.Background(), "Inspect Boston, then continue.", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			receipts := investigationReceipts(t, f, reader)
			if len(receipts) != 2 || !reflect.DeepEqual(receipts[0].Evidence, receipts[1].Evidence) || receipts[1].Investigation == nil || receipts[1].Investigation.RefreshAttempts != 0 || receipts[1].Investigation.SearchAttempts != 1 {
				t.Fatalf("restoring excluded peers caused needless refresh or changed the authorized receipt: %+v", receipts)
			}
			for _, request := range client.reqs {
				encoded, err := json.Marshal(request)
				if err != nil {
					t.Fatal(err)
				}
				for _, peer := range peers {
					for _, forbidden := range []string{peer.Literal.Value, string(peer.ClaimID), string(peer.SourceLinkID), string(peer.Source.EventID), peer.Scope.Key} {
						if strings.Contains(string(encoded), forbidden) {
							t.Fatalf("restored peer outside authorized scopes leaked through the complete provider request: %q", forbidden)
						}
					}
				}
			}
		})
	}
}
