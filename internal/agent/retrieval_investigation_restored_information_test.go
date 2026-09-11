package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

func investigationContinuationTool() tools.Tool {
	return tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "continue_check", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) {
		return "Continue with the requested evidence view.", nil
	}}
}

func investigationReadView(view string) string {
	query := map[string]any{"query": "Boston"}
	if view == "knowledge pin" {
		query["as_known_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if view == "historical" {
		query["intent"] = "historical"
	}
	if view == "valid pin" {
		query["valid_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	raw, _ := json.Marshal(query)
	return string(raw)
}

func TestMemoryInvestigationRefreshesRestoredConflictsWithoutAdvancingPinnedViews(t *testing.T) {
	for _, restored := range []string{"claim", "source"} {
		for _, view := range []string{"current", "knowledge pin", "historical", "valid pin"} {
			t.Run(restored+"/"+view, func(t *testing.T) {
				f := newRetrievalFixture(t)
				source, reader := f.global(), f.global()
				boston := readerRemember(t, f, source, "live_in", "Remember that I live in Boston.", "Boston")
				chicago := readerRemember(t, f, source, "live_in", "Remember that I live in Chicago.", "Chicago")
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
							t.Fatalf("initial read did not isolate the eligible Boston Claim: %+v", first)
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
						return assistantStep("The requested evidence view remains available.", nil)
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
						t.Fatalf("restoring an older %s did not refresh the current conflicting view: %+v", restored, final)
					}
					for _, item := range final {
						if !slices.Contains([]memory.SemanticID{boston.ClaimID, chicago.ClaimID}, item.ClaimID) || len(item.Conflicts) != 1 || !item.AsKnownAt.After(first[0].AsKnownAt) {
							t.Fatalf("restored conflict lacks its supported new read view: %+v", item)
						}
						if view == "valid pin" && (!item.ValidAtConstrained || !item.ValidAt.Equal(first[0].ValidAt)) {
							t.Fatalf("restoration advanced the explicit world-validity pin: %+v", item)
						}
					}
				} else if len(final) != 1 || !reflect.DeepEqual(first[0].Reference(), final[0].Reference()) {
					t.Fatalf("restoration advanced the explicit %s view: %+v", view, final)
				}
				receipts := investigationReceipts(t, f, reader)
				if len(receipts) != 3 || !reflect.DeepEqual(receipts[0].Evidence, []memory.RetrievalReference{first[0].Reference()}) || !reflect.DeepEqual(receipts[1].Evidence, receipts[2].Evidence) {
					t.Fatalf("restoration rewrote original receipts or refreshed unchanged continuation: %+v", receipts)
				}
				if last := receipts[2].Investigation; last == nil || last.RefreshAttempts != wantRefreshes || last.SearchAttempts != 1+wantRefreshes {
					t.Fatalf("unexpected refresh work: %+v", last)
				}
			})
		}
	}
}

func TestMemoryInvestigationReusesEvidenceAfterNonconflictingPeerRestoration(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	boston := f.remember(source, memory.MemoryEverywhere, "Boston keepsake")
	chicago := f.remember(source, memory.MemoryEverywhere, "Chicago keepsake")
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, chicago.ClaimID)
	f.refresh()
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("initial", "memory_search", `{"query":"Boston"}`))
		case 1:
			first := expansionBoundaryEvidence(t, request)
			if len(first) != 1 || first[0].ClaimID != boston.ClaimID {
				t.Fatalf("initial many-cardinality view lacks Boston: %+v", first)
			}
			f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, chicago.ClaimID)
			return assistantStep("", nil, toolCall("continue", "continue_check", `{}`))
		default:
			return assistantStep("Both independent keepsakes may remain accepted.", nil)
		}
	}}
	if err := f.session(reader, client, investigationContinuationTool()).Send(context.Background(), "Inspect Boston and continue.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	receipts := investigationReceipts(t, f, reader)
	if len(receipts) != 2 || !reflect.DeepEqual(receipts[0].Evidence, receipts[1].Evidence) || receipts[1].Investigation == nil || receipts[1].Investigation.RefreshAttempts != 0 || receipts[1].Investigation.SearchAttempts != 1 {
		t.Fatalf("nonconflicting restoration needlessly repeated the search: %+v", receipts)
	}
}

func TestMemoryInvestigationRefreshesNewOwnerStatementWithoutAdvancingPinnedViews(t *testing.T) {
	for _, view := range []string{"current", "knowledge pin", "historical", "other area", "unrelated"} {
		t.Run(view, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source, reader := f.global(), f.global()
			boston := readerRemember(t, f, source, "live_in", "Remember that I live in Boston.", "Boston")
			f.refresh()
			query := investigationReadView(view)
			var appended memory.Event
			client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
				switch call {
				case 0:
					return assistantStep("", nil, toolCall("initial", "memory_search", query))
				case 1:
					first := expansionBoundaryEvidence(t, request)
					if len(first) != 1 || first[0].ClaimID != boston.ClaimID {
						t.Fatalf("initial view lacks accepted Boston evidence: %+v", first)
					}
					area := source
					if view == "other area" {
						project, err := f.store.RegisterProject(context.Background(), "Other owner area", t.TempDir())
						if err != nil {
							t.Fatal(err)
						}
						area, err = f.store.CreateProjectSession(context.Background(), project.ID)
						if err != nil {
							t.Fatal(err)
						}
					}
					text := "I live in Chicago now."
					if view == "unrelated" {
						text = "The garden gate is green."
					}
					appended = f.converse(area, text)
					if !appended.RecordedAt.After(first[0].AsKnownAt) {
						t.Fatal("new original statement must follow the held read")
					}
					return assistantStep("", nil, toolCall("continue-1", "continue_check", `{}`))
				case 2:
					return assistantStep("", nil, toolCall("continue-2", "continue_check", `{}`))
				default:
					return assistantStep("The requested source context remains available.", nil)
				}
			}}
			if err := f.session(reader, client, investigationContinuationTool()).Send(context.Background(), "Inspect Boston, then continue twice.", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			first := expansionBoundaryEvidence(t, client.reqs[1])
			final := expansionBoundaryEvidence(t, client.reqs[2])
			wantRefreshes := 0
			if view == "current" {
				wantRefreshes = 1
				if len(final) != 2 {
					t.Fatalf("new same-area owner statement did not refresh the current view: %+v", final)
				}
				var owner *memory.RetrievalEvidence
				for i := range final {
					if final[i].Kind == memory.RetrievalConversationExcerpt {
						owner = &final[i]
					}
				}
				if owner == nil || owner.ClaimID != "" || owner.Text != appended.Content || len(owner.Sources) != 1 || owner.Sources[0].EventID != appended.ID || owner.Sources[0].SessionID != source.ID || owner.Sources[0].Authority != memory.AuthorityOwnerStatement || owner.Sources[0].EvidenceSHA256 != memory.CompilerHash([]byte(appended.Content)) || !slices.Contains(owner.Paths, "newer_owner_statement") || !reflect.DeepEqual(owner.RelatedClaimIDs, []memory.SemanticID{boston.ClaimID}) || !owner.AsKnownAt.After(first[0].AsKnownAt) {
					t.Fatalf("new owner wording lost exact source/discrepancy attribution: %+v", owner)
				}
			} else if len(final) != 1 || !reflect.DeepEqual(first[0].Reference(), final[0].Reference()) {
				t.Fatalf("%s append advanced or broadened the held view: %+v", view, final)
			}
			receipts := investigationReceipts(t, f, reader)
			if len(receipts) != 3 || !reflect.DeepEqual(receipts[0].Evidence, []memory.RetrievalReference{first[0].Reference()}) || !reflect.DeepEqual(receipts[1].Evidence, receipts[2].Evidence) {
				t.Fatalf("owner append rewrote or repeatedly refreshed request receipts: %+v", receipts)
			}
			if last := receipts[2].Investigation; last == nil || last.RefreshAttempts != wantRefreshes || last.SearchAttempts != 1+wantRefreshes {
				t.Fatalf("unexpected owner-statement refresh work: %+v", last)
			}
		})
	}
}
