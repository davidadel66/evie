package eviedb

import (
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerTemporalAdmissionAndMeaningBoundaries(t *testing.T) {
	subject := memory.SemanticID("90000000-0000-4000-8000-000000000342")
	predicate := memory.SemanticPredicate{ID: "90000000-0000-4000-8000-000000000343", Token: "age", Label: "age", ObjectConstraint: "integer", Cardinality: memory.CardinalityOne}
	request := memory.CompilerRequest{IdentityPolicy: memory.CompilerTemporalPolicyV3, Entities: []memory.SemanticEntity{{ID: subject}}, Predicates: []memory.SemanticPredicate{predicate}}
	for _, tc := range []struct {
		name   string
		policy string
		adapt  func(*memory.ExtractorCandidate)
		valid  bool
	}{
		{"canonical integer", memory.CompilerTemporalPolicyV3, func(*memory.ExtractorCandidate) {}, true},
		{"noncanonical integer", memory.CompilerTemporalPolicyV3, func(c *memory.ExtractorCandidate) { c.Proposition.Object.Literal.Value = "01" }, false},
		{"wrong literal type", memory.CompilerTemporalPolicyV3, func(c *memory.ExtractorCandidate) { c.Proposition.Object.Literal.Kind = memory.LiteralText }, false},
		{"older policy", memory.CompilerIdentityPolicyV2, func(*memory.ExtractorCandidate) {}, false},
		{"missing typed meaning", memory.CompilerTemporalPolicyV3, func(c *memory.ExtractorCandidate) { c.Temporal = nil }, false},
		{"qualification only", memory.CompilerTemporalPolicyV3, func(c *memory.ExtractorCandidate) { c.TemporalQualification = "maybe" }, false},
		{"plan as actual Predicate", memory.CompilerTemporalPolicyV3, func(c *memory.ExtractorCandidate) { c.Temporal.Meaning = "plan" }, false},
		{"duplicate correction modes", memory.CompilerTemporalPolicyV3, func(c *memory.ExtractorCandidate) {
			c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError, memory.CorrectionError}}
		}, false},
		{"noncanonical mode order", memory.CompilerTemporalPolicyV3, func(c *memory.ExtractorCandidate) {
			c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionChanged, memory.CorrectionError}}
		}, false},
		{"ambiguous correction", memory.CompilerTemporalPolicyV3, func(c *memory.ExtractorCandidate) {
			c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError, memory.CorrectionChanged}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request
			r.IdentityPolicy = tc.policy
			c := memory.ExtractorCandidate{Temporal: &memory.CandidateTemporalProposal{Meaning: "assertion"}, Proposition: memory.ClaimProposition{SubjectEntityID: subject, PredicateID: predicate.ID, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralInteger, Value: "1"}}, Polarity: memory.PolarityAffirmed}}
			tc.adapt(&c)
			err := validateCompilerTemporal(r, c)
			if err == nil {
				err = validateCompilerProposition(r, c)
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	future := at.Add(time.Hour)
	c := memory.MemoryCandidate{Proposal: memory.ExtractorCandidate{Temporal: &memory.CandidateTemporalProposal{Meaning: "assertion"}, ValidTime: memory.ValidTime{From: &future}}, Support: []memory.CompilerSource{{ObservedAt: at.Format(time.RFC3339Nano)}}}
	if err := validateTemporalObserved(c); err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatal("future actual assertion admitted")
	}
	c.Proposal.ValidTime = memory.ValidTime{}
	c.Proposal.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionChanged}, EffectiveTime: &future}
	if err := validateTemporalObserved(c); err == nil {
		t.Fatal("future correction admitted")
	}
}

func TestReviewTemporalObservedInstantCompatibility(t *testing.T) {
	canonical := "2025-01-02T03:04:05.123000000Z"
	for _, source := range []string{"2025-01-02T03:04:05.123Z", "2025-01-02T03:04:05.123000000Z", "2025-01-02T04:04:05.123+01:00"} {
		if !reviewObservedTimeMatches("owner-review-preview-v3", canonical, source) {
			t.Fatalf("same instant rejected: %s", source)
		}
	}
	for _, source := range []string{"2025-01-02 03:04:05", "2025-01-02T03:04:05.124Z", "2025-01-02", "not a time"} {
		if reviewObservedTimeMatches("owner-review-preview-v3", canonical, source) {
			t.Fatalf("ambiguous or different instant accepted: %s", source)
		}
	}
	if reviewObservedTimeMatches("owner-review-preview-v1", canonical, "2025-01-02T03:04:05.123Z") {
		t.Fatal("v1 exact encoding policy changed")
	}
	if reviewObservedTimeMatches("owner-review-preview-v3", "2025-01-02T03:04:05.123Z", "2025-01-02T03:04:05.123Z") {
		t.Fatal("noncanonical v3 accepted timestamp")
	}
}
