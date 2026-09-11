package eviedb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

// A held current read can follow its accepted correction chain before an
// asynchronous index catches up. Neither the reference nor the replacement ID
// grants scope or source authority; the original and every new candidate are
// checked against canonical SQLite records inside this read snapshot.
func (c *retrievalCandidates) correctionRefreshGenerator(ctx context.Context) error {
	query := c.plan.query
	if query.Intent == memory.RetrievalHistorical || query.AsKnownAt != nil {
		return nil
	}
	refs := query.RefreshReferences
	if len(refs) > 8 {
		refs = refs[:8]
		c.truncated = true
	}
	rank := 0
	for _, ref := range refs {
		if ref.Kind != memory.RetrievalAcceptedMemory || ref.Intent != memory.RetrievalCurrent || ref.ClaimID == "" || ref.ID != "claim:"+string(ref.ClaimID) || ref.AsKnownAt.IsZero() || ref.ValidAt.IsZero() {
			continue
		}
		originalPin := c.plan.metadata
		originalPin.AsKnownAt, originalPin.ValidAt = ref.AsKnownAt, ref.ValidAt
		original, eligible, err := c.store.retrievalClaim(ctx, c.tx, originalPin, ref.ClaimID, memory.RetrievalHistorical, ref.ValidAtConstrained)
		if err != nil {
			return err
		}
		if !eligible || original.Claim == nil || original.ClaimOperationID != ref.ClaimOperationID || original.ScopeKey != ref.ScopeKey || !sameRetrievalSources(original.Reference().Sources, ref.Sources) {
			continue
		}
		// An exact alias was part of the original interpretation, not permanent
		// authority to follow that entity after the mapping is retired/revoked.
		identities, err := resolveRetrievalIdentityMatches(ctx, c.tx, originalPin, original, ref.IdentityMatches)
		if err != nil {
			return err
		}
		if len(identities) != len(ref.IdentityMatches) {
			continue
		}
		id := ref.ClaimID
		visited := map[memory.SemanticID]bool{id: true}
		for depth := 0; depth < 8; depth++ {
			var replacement memory.SemanticID
			err := c.tx.QueryRowContext(ctx, `SELECT correction.replacement_claim_id
 FROM semantic_claim_corrections correction
 JOIN semantic_claims candidate ON candidate.claim_id=correction.replacement_claim_id
 JOIN semantic_scopes scope ON scope.scope_id=candidate.scope_id
 WHERE correction.old_claim_id=? AND correction.transaction_time<=?
 AND scope.scope_key=? AND candidate.subject_entity_id=? AND candidate.predicate_id=?`,
				id, formatSemanticTime(c.plan.metadata.AsKnownAt), original.ScopeKey, original.Claim.SubjectEntityID, original.Claim.Predicate.ID).Scan(&replacement)
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				return err
			}
			if visited[replacement] {
				break
			}
			visited[replacement] = true
			candidate, err := c.read(ctx, replacement)
			if err != nil {
				return err
			}
			if candidate != nil {
				rank++
				candidate.direct = true
				addRetrievalRank(candidate, "accepted_correction_refresh", rank)
			}
			id = replacement
			if depth == 7 {
				c.truncated = true
			}
		}
	}
	return nil
}
