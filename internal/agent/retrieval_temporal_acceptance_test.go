package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestHistoricalMemorySearchHonorsHalfOpenValidityAndExactKnowledgePins(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	owner := f.session(source, nil)
	proposal, err := owner.PrepareRememberLiteral(context.Background(), f.store, "The agate residence applied from 2000 until 2005.", memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "residence", PredicateLabel: "residence", PredicateCardinality: memory.CardinalityOne,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "agate residence"}, Polarity: memory.PolarityAffirmed, ValidTime: memory.ValidTime{From: &from, To: &to},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := owner.ResolveRememberLiteral(context.Background(), f.store, proposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectClaim, accepted.ClaimID)
	if err != nil || inspection.Claim == nil {
		t.Fatalf("accepted temporal fixture: %+v %v", inspection, err)
	}
	known := inspection.Claim.TransactionTime
	f.refresh()
	current, _ := f.search(f.global(), "agate")
	if strings.Contains(retrievalData(t, current.reqs[1]), string(accepted.ClaimID)) {
		t.Fatal("ordinary current read returned expired Claim")
	}
	for _, test := range []struct {
		name    string
		options map[string]any
		want    bool
	}{
		{"historical without validity constraint", nil, true},
		{"inclusive valid start", map[string]any{"valid_at": from.Format(time.RFC3339Nano)}, true},
		{"last valid instant", map[string]any{"valid_at": to.Add(-time.Nanosecond).Format(time.RFC3339Nano)}, true},
		{"exclusive valid end", map[string]any{"valid_at": to.Format(time.RFC3339Nano)}, false},
		{"before accepted knowledge", map[string]any{"as_known_at": known.Add(-time.Nanosecond).Format(time.RFC3339Nano)}, false},
		{"inclusive accepted knowledge", map[string]any{"as_known_at": known.Format(time.RFC3339Nano)}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := f.historicalSearch(f.global(), "memory_search", "agate", test.options)
			data := retrievalData(t, client.reqs[1])
			if strings.Contains(data, string(accepted.ClaimID)) != test.want {
				t.Fatalf("temporal boundary mismatch: %s", data)
			}
		})
	}
}

func TestHistoricalMemorySearchPreservesUnknownValidity(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	accepted := f.remember(source, memory.MemoryEverywhere, "opal compass")
	f.refresh()
	client := f.historicalSearch(f.global(), "memory_search", "opal", nil)
	var data struct {
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, client.reqs[1]), "EVIE_MEMORY_DATA\n")), &data); err != nil {
		t.Fatal(err)
	}
	var found *memory.RetrievalEvidence
	for i := range data.Evidence {
		if data.Evidence[i].ClaimID == accepted.ClaimID {
			found = &data.Evidence[i]
		}
	}
	if found == nil || found.Claim == nil || found.EffectiveValidTime == nil || found.EffectiveValidTime.From != nil || found.EffectiveValidTime.To != nil || found.ValidAtConstrained {
		t.Fatalf("unknown world dates were invented: %+v", found)
	}
	if found.Claim.TransactionTime.IsZero() || found.AsKnownAt.IsZero() {
		t.Fatal("actual acceptance/read timestamps were lost")
	}
}
