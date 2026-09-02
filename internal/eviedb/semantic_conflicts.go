package eviedb

import (
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func literalConflictWarnings(claims []memory.LiteralClaimInspection) []memory.ClaimConflictWarning {
	candidates := make([]claimConflictCandidate, 0, len(claims))
	for _, claim := range claims {
		candidates = append(candidates, claimConflictCandidate{
			ID: claim.ID, SubjectID: claim.Subject.ID, PredicateID: claim.Predicate.ID,
			PredicateToken: claim.Predicate.Token, ObjectKey: string(claim.Literal.Kind) + "\x00" + claim.Literal.Value,
			Polarity: claim.Polarity, ValidTime: claim.ValidTime, Cardinality: claim.Predicate.Cardinality,
		})
	}
	return classifyClaimConflicts(candidates)
}

func exactClaimConflictWarnings(claims []memory.ClaimInspection) []memory.ClaimConflictWarning {
	candidates := make([]claimConflictCandidate, 0, len(claims))
	for _, claim := range claims {
		objectKey := ""
		if claim.Object.EntityID != "" {
			objectKey = "entity\x00" + string(claim.Object.EntityID)
		} else if claim.Object.Literal != nil {
			objectKey = "literal\x00" + string(claim.Object.Literal.Kind) + "\x00" + claim.Object.Literal.Value
		}
		candidates = append(candidates, claimConflictCandidate{
			ID: claim.ID, SubjectID: claim.SubjectEntityID, PredicateID: claim.Predicate.ID,
			PredicateToken: claim.Predicate.Token, ObjectKey: objectKey, Polarity: claim.Polarity,
			ValidTime: claim.EffectiveValidTime, Cardinality: claim.Predicate.Cardinality,
		})
	}
	return classifyClaimConflicts(candidates)
}

func sortConflictWarnings(warnings []memory.ClaimConflictWarning) {
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].PredicateToken != warnings[j].PredicateToken {
			return warnings[i].PredicateToken < warnings[j].PredicateToken
		}
		if warnings[i].ClaimIDs[0] != warnings[j].ClaimIDs[0] {
			return warnings[i].ClaimIDs[0] < warnings[j].ClaimIDs[0]
		}
		return warnings[i].ClaimIDs[1] < warnings[j].ClaimIDs[1]
	})
}

func validTimesOverlap(left, right memory.ValidTime) bool {
	return (left.To == nil || right.From == nil || left.To.After(*right.From)) &&
		(right.To == nil || left.From == nil || right.To.After(*left.From))
}

func validTimeContains(validTime memory.ValidTime, instant time.Time) bool {
	return (validTime.From == nil || !instant.Before(*validTime.From)) &&
		(validTime.To == nil || instant.Before(*validTime.To))
}

func conflictingClaimIDs(warnings []memory.ClaimConflictWarning) map[memory.SemanticID]struct{} {
	ids := make(map[memory.SemanticID]struct{})
	for _, warning := range warnings {
		for _, claimID := range warning.ClaimIDs {
			ids[claimID] = struct{}{}
		}
	}
	return ids
}

func selectLiteralConflictClaims(
	claims []memory.LiteralClaimInspection,
	warnings []memory.ClaimConflictWarning,
) []memory.LiteralClaimInspection {
	ids := conflictingClaimIDs(warnings)
	result := make([]memory.LiteralClaimInspection, 0, len(ids))
	for _, claim := range claims {
		if _, ok := ids[claim.ID]; ok {
			result = append(result, claim)
		}
	}
	return result
}

func selectEntityConflictClaims(
	claims []memory.EntityClaimInspection,
	warnings []memory.ClaimConflictWarning,
) []memory.EntityClaimInspection {
	ids := conflictingClaimIDs(warnings)
	result := make([]memory.EntityClaimInspection, 0, len(ids))
	for _, claim := range claims {
		if _, ok := ids[claim.Claim.ID]; ok {
			result = append(result, claim)
		}
	}
	return result
}

func entityConflictWarnings(claims []memory.EntityClaimInspection) []memory.ClaimConflictWarning {
	candidates := make([]claimConflictCandidate, 0, len(claims))
	for _, claim := range claims {
		candidates = append(candidates, claimConflictCandidate{
			ID: claim.Claim.ID, SubjectID: claim.Claim.SubjectEntityID, PredicateID: claim.Claim.PredicateID,
			PredicateToken: claim.Claim.PredicateToken, ObjectKey: string(claim.Claim.ObjectEntityID),
			Polarity: claim.Claim.Polarity, ValidTime: claim.Claim.ValidTime, Cardinality: claim.Predicate.Cardinality,
		})
	}
	return classifyClaimConflicts(candidates)
}

type claimConflictCandidate struct {
	ID             memory.SemanticID
	SubjectID      memory.SemanticID
	PredicateID    memory.SemanticID
	PredicateToken string
	ObjectKey      string
	Polarity       memory.ClaimPolarity
	ValidTime      memory.ValidTime
	Cardinality    memory.PredicateCardinality
}

func classifyClaimConflicts(claims []claimConflictCandidate) []memory.ClaimConflictWarning {
	var opposite, cardinality []memory.ClaimConflictWarning
	for leftIndex := range claims {
		for rightIndex := leftIndex + 1; rightIndex < len(claims); rightIndex++ {
			left, right := claims[leftIndex], claims[rightIndex]
			if left.SubjectID != right.SubjectID || left.PredicateID != right.PredicateID ||
				!validTimesOverlap(left.ValidTime, right.ValidTime) {
				continue
			}
			claimIDs := []memory.SemanticID{left.ID, right.ID}
			sort.Slice(claimIDs, func(i, j int) bool { return claimIDs[i] < claimIDs[j] })
			if left.ObjectKey == right.ObjectKey && left.Polarity != right.Polarity {
				opposite = append(opposite, memory.ClaimConflictWarning{
					Code: memory.ConflictOppositePolarity, PredicateToken: left.PredicateToken, ClaimIDs: claimIDs,
				})
			}
			if left.ObjectKey != right.ObjectKey && left.Polarity == memory.PolarityAffirmed &&
				right.Polarity == memory.PolarityAffirmed &&
				(left.Cardinality == memory.CardinalityOne || right.Cardinality == memory.CardinalityOne) {
				cardinality = append(cardinality, memory.ClaimConflictWarning{
					Code: memory.ConflictOneCardinality, PredicateToken: left.PredicateToken, ClaimIDs: claimIDs,
				})
			}
		}
	}
	sortConflictWarnings(opposite)
	sortConflictWarnings(cardinality)
	return append(opposite, cardinality...)
}
