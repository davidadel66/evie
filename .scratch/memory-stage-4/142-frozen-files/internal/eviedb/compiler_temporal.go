package eviedb

import (
	"errors"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func validateCompilerTemporal(request memory.CompilerRequest, proposal memory.ExtractorCandidate) error {
	if proposal.Temporal == nil {
		if request.IdentityPolicy == memory.CompilerTemporalPolicyV3 {
			return errors.New("temporal generation requires typed meaning")
		}
		return nil
	}
	if request.IdentityPolicy != memory.CompilerTemporalPolicyV3 {
		return errors.Join(ErrCompilerTerminalOutput, errors.New("generation does not admit temporal interpretations"))
	}
	// This slice resolves existing identities. General combined edits come later.
	if proposal.Identity != nil && (proposal.Identity.Subject != nil || proposal.Identity.Object != nil || proposal.Temporal.Correction != nil) {
		return errors.New("temporal interpretation requires existing unambiguous entities")
	}
	var predicate memory.SemanticPredicate
	for _, p := range request.Predicates {
		if p.ID == proposal.Proposition.PredicateID {
			predicate = p
		}
	}
	if proposal.Identity != nil && proposal.Identity.Predicate != nil {
		p := proposal.Identity.Predicate
		predicate = memory.SemanticPredicate{Token: p.Token, Label: p.Label, ObjectConstraint: p.ObjectConstraint, Cardinality: p.Cardinality}
	}
	return validateTemporalMeaning(proposal, predicate)
}

func validateTemporalMeaning(proposal memory.ExtractorCandidate, predicate memory.SemanticPredicate) error {
	temporal := proposal.Temporal
	if temporal == nil {
		return nil
	}
	if proposal.TemporalQualification != "" {
		return errors.New("typed temporal interpretation cannot carry an unbound qualification")
	}
	switch temporal.Meaning {
	case "assertion":
	case "plan", "possibility":
		token, label := memory.PlanPredicateToken, memory.PlanPredicateLabel
		if temporal.Meaning == "possibility" {
			token, label = memory.PossibilityPredicateToken, memory.PossibilityPredicateLabel
		}
		if predicate.Token != token || predicate.Label != label || predicate.ObjectConstraint != "text" || predicate.Cardinality != memory.CardinalityMany || proposal.Proposition.Object.Literal == nil || proposal.Proposition.Object.Literal.Kind != memory.LiteralText || strings.TrimSpace(proposal.Proposition.Object.Literal.Value) == "" {
			return errors.New("qualified meaning requires its exact intent/possibility Predicate contract")
		}
		if temporal.Correction != nil {
			return errors.New("plan or possibility cannot supersede an actual circumstance")
		}
	default:
		return errors.New("unsupported temporal meaning")
	}
	if correction := temporal.Correction; correction != nil {
		if len(correction.Modes) < 1 || len(correction.Modes) > 2 {
			return errors.New("correction needs bounded explicit mode alternatives")
		}
		seen := map[memory.CorrectionMode]bool{}
		for _, mode := range correction.Modes {
			if mode != memory.CorrectionError && mode != memory.CorrectionChanged || seen[mode] {
				return errors.New("invalid correction alternatives")
			}
			seen[mode] = true
		}
		if len(correction.Modes) == 2 && correction.Modes[0] != memory.CorrectionError {
			return errors.New("correction alternatives must be canonical error,changed order")
		}
		if correction.EffectiveTime != nil {
			normalized, err := normalizeValidTime(memory.ValidTime{From: correction.EffectiveTime})
			if err != nil || !validTimesEqual(normalized, memory.ValidTime{From: correction.EffectiveTime}) {
				return errors.New("noncanonical correction effective time")
			}
			if !seen[memory.CorrectionChanged] {
				return errors.New("error-only correction has no effective time")
			}
		}
	}
	return nil
}

// Observed Time bounds completed-change assertions independently of extraction
// or review wall clocks. Future decisions retain modal Predicate meaning.
func validateTemporalObserved(candidate memory.MemoryCandidate) error {
	temporal := candidate.Proposal.Temporal
	if temporal == nil || temporal.Meaning != "assertion" {
		return nil
	}
	var latest time.Time
	for _, source := range candidate.Support {
		at, err := time.Parse(time.RFC3339Nano, source.ObservedAt)
		if err != nil {
			return err
		}
		if at.After(latest) {
			latest = at
		}
	}
	if from := candidate.Proposal.ValidTime.From; from != nil && from.After(latest) {
		return errors.New("future assertion requires explicit plan or possibility meaning")
	}
	if temporal.Correction != nil && temporal.Correction.EffectiveTime != nil && temporal.Correction.EffectiveTime.After(latest) {
		return errors.New("future possibility is not a completed correction")
	}
	return nil
}
