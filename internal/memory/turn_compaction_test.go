package memory

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func validTurnCompactionSummary() string {
	var summary strings.Builder
	for _, heading := range ContextCompactionSectionHeadings() {
		fmt.Fprintf(&summary, "## %s\nkept\n\n", heading)
	}
	return summary.String()
}

func validTurnCompactedPayload(summary string) ContextTurnCompactedPayload {
	digest := sha256.Sum256([]byte(summary))
	return ContextTurnCompactedPayload{
		SchemaVersion: ContextTurnCompactedSchemaVersion,
		TurnID:        "root", Generation: 1,
		CoveredFirstEventID: "assistant-1", CoveredFirstSequence: 3,
		CoveredLastEventID: "result-4", CoveredLastSequence: 12,
		FirstRetainedEventID: "assistant-5", FirstRetainedSequence: 14,
		CanonicalModel: "test/model", PromptVersion: ContextTurnCompactionPromptVersion,
		SummaryBytes: int64(len(summary)), SummarySHA256: fmt.Sprintf("%x", digest),
	}
}

// Stage 8: an in-turn compaction records an explicit frontier of whole tool
// groups inside one root turn, and the same strict summary rules as a
// rolling summary.
func TestContextTurnCompactedPayloadValidatesFrontierAndSummary(t *testing.T) {
	summary := validTurnCompactionSummary()
	valid := validTurnCompactedPayload(summary)
	if err := valid.Validate(summary); err != nil {
		t.Fatalf("valid payload: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*ContextTurnCompactedPayload)
	}{
		{name: "schema", mutate: func(p *ContextTurnCompactedPayload) { p.SchemaVersion = 2 }},
		{name: "turn", mutate: func(p *ContextTurnCompactedPayload) { p.TurnID = "" }},
		{name: "generation", mutate: func(p *ContextTurnCompactedPayload) { p.Generation = 0 }},
		{name: "first generation with prior", mutate: func(p *ContextTurnCompactedPayload) { p.PriorCompactionEventID = "prior" }},
		{name: "later generation without prior", mutate: func(p *ContextTurnCompactedPayload) { p.Generation = 2 }},
		{name: "covered first", mutate: func(p *ContextTurnCompactedPayload) { p.CoveredFirstEventID = "" }},
		{name: "covered last", mutate: func(p *ContextTurnCompactedPayload) { p.CoveredLastEventID = "" }},
		{name: "retained", mutate: func(p *ContextTurnCompactedPayload) { p.FirstRetainedEventID = "" }},
		{name: "group without result", mutate: func(p *ContextTurnCompactedPayload) { p.CoveredLastSequence = p.CoveredFirstSequence }},
		{name: "retained inside covered", mutate: func(p *ContextTurnCompactedPayload) { p.FirstRetainedSequence = p.CoveredLastSequence }},
		{name: "non-positive start", mutate: func(p *ContextTurnCompactedPayload) { p.CoveredFirstSequence = 0 }},
		{name: "model", mutate: func(p *ContextTurnCompactedPayload) { p.CanonicalModel = " " }},
		{name: "prompt", mutate: func(p *ContextTurnCompactedPayload) { p.PromptVersion = "" }},
		{name: "bytes", mutate: func(p *ContextTurnCompactedPayload) { p.SummaryBytes++ }},
		{name: "hash", mutate: func(p *ContextTurnCompactedPayload) { p.SummarySHA256 = strings.Repeat("0", 64) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := valid
			test.mutate(&payload)
			if err := payload.Validate(summary); err == nil {
				t.Fatalf("invalid payload validated: %+v", payload)
			}
		})
	}
	if err := valid.Validate("not a sectioned summary"); err == nil {
		t.Fatal("sectionless summary validated")
	}
}

func TestContextTurnCompactedPayloadValidatesExactAdvance(t *testing.T) {
	prior := ContextTurnCompactedPayload{
		TurnID: "root", Generation: 1, FirstRetainedEventID: "assistant-5", FirstRetainedSequence: 14,
	}
	next := ContextTurnCompactedPayload{
		TurnID: "root", Generation: 2, PriorCompactionEventID: "turn-compaction-1",
		CoveredFirstEventID: "assistant-5", CoveredFirstSequence: 14,
	}
	if err := next.ValidateAdvance("turn-compaction-1", prior); err != nil {
		t.Fatalf("valid advance: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*ContextTurnCompactedPayload)
	}{
		{name: "turn", mutate: func(p *ContextTurnCompactedPayload) { p.TurnID = "other" }},
		{name: "generation", mutate: func(p *ContextTurnCompactedPayload) { p.Generation++ }},
		{name: "prior link", mutate: func(p *ContextTurnCompactedPayload) { p.PriorCompactionEventID = "other" }},
		{name: "frontier", mutate: func(p *ContextTurnCompactedPayload) { p.CoveredFirstEventID = "assistant-4" }},
		{name: "frontier sequence", mutate: func(p *ContextTurnCompactedPayload) { p.CoveredFirstSequence = 13 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := next
			test.mutate(&candidate)
			if err := candidate.ValidateAdvance("turn-compaction-1", prior); err == nil {
				t.Fatalf("invalid advance accepted: %+v", candidate)
			}
		})
	}
	if err := next.ValidateAdvance("", prior); err == nil {
		t.Fatal("advance without a prior event accepted")
	}
}
