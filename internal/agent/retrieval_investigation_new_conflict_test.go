package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemoryInvestigationRefreshesNewConflictButPreservesExplicitKnowledgePin(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		name := "current"
		if pinned {
			name = "explicit knowledge pin"
		}
		t.Run(name, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source, reader := f.global(), f.global()
			saved := readerRemember(t, f, source, "live_in", "Remember that I live in Boston.", "Boston")
			f.refresh()
			query := map[string]any{"query": "Boston"}
			if pinned {
				query["as_known_at"] = time.Now().UTC().Format(time.RFC3339Nano)
			}
			initial, _ := json.Marshal(query)
			remember, _ := json.Marshal(map[string]any{"idempotency_key": "idem:v1:" + uuid.NewString(), "predicate": "live_in", "predicate_label": "live in", "cardinality": "one", "literal_kind": "text", "literal_value": "Portland", "polarity": "affirmed", "destination": "everywhere"})
			client := &fakeClient{steps: []step{
				assistantStep("", nil, toolCall("initial", "memory_search", string(initial))),
				assistantStep("", nil, toolCall("new-fact", "memory_remember_literal", string(remember))),
				assistantStep("The requested evidence view has been preserved.", nil),
			}}
			if err := f.session(reader, client).Send(context.Background(), "Read Boston, then remember that I live in Portland without replacing the existing record.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
				t.Fatal(err)
			}
			current, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{PredicateToken: "live_in"})
			if err != nil || len(current.Claims) != 2 {
				t.Fatalf("new conflicting Claim was not accepted: count=%d err=%v", len(current.Claims), err)
			}
			first := expansionBoundaryEvidence(t, client.reqs[1])
			final := expansionBoundaryEvidence(t, client.reqs[2])
			claims := 0
			for _, item := range final {
				if item.ClaimID != "" {
					claims++
				}
			}
			if pinned {
				if claims != 1 || final[0].ClaimID != saved.ClaimID || !final[0].AsKnownAt.Equal(first[0].AsKnownAt) || len(final[0].Conflicts) != 0 {
					t.Fatalf("new fact undid explicit knowledge pin: %+v", final)
				}
			} else {
				if claims != 2 {
					t.Fatalf("new conflicting accepted evidence did not refresh current query: %+v", final)
				}
				for _, item := range final {
					if item.ClaimID != "" && (len(item.Conflicts) != 1 || !item.AsKnownAt.After(first[0].AsKnownAt)) {
						t.Fatalf("new conflict was attached to stale read metadata: %+v", item)
					}
				}
			}
		})
	}
}
