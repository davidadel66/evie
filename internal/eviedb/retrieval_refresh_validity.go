package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// New or newly eligible support can change a current interpretation while the
// held Claim remains active. Explicit read views retain their knowledge pin.
func (s *Store) hasNewRetrievalInformation(ctx context.Context, tx *sql.Tx, scope memory.ScopeContext, metadata memory.ExactReadMetadata, current, prior memory.RetrievalEvidence) (bool, error) {
	if prior.AsKnownAtConstrained || prior.Intent == memory.RetrievalHistorical || current.Claim == nil {
		return false, nil
	}
	changed, err := s.hasNewRetrievalConflict(ctx, tx, scope, metadata, current, prior)
	if err != nil || changed {
		return changed, err
	}
	return s.hasNewRetrievalOwnerStatement(ctx, tx, scope, metadata, current, prior)
}

// Check at most eight changed peers per held Claim. State events include older
// Claims and Source Links restored after the original read. Current canonical
// eligibility and conflict semantics still decide whether a refresh is needed.
func (s *Store) hasNewRetrievalConflict(ctx context.Context, tx *sql.Tx, scope memory.ScopeContext, metadata memory.ExactReadMetadata, current, prior memory.RetrievalEvidence) (bool, error) {
	after, through := formatSemanticTime(prior.AsKnownAt), formatSemanticTime(metadata.AsKnownAt)
	rows, err := tx.QueryContext(ctx, `SELECT c.claim_id FROM semantic_claims c JOIN semantic_scopes sc ON sc.scope_id=c.scope_id
 WHERE sc.scope_key IN (?,?,?) AND c.subject_entity_id=? AND c.predicate_id=? AND c.transaction_time<=? AND c.claim_id!=?
 AND (c.transaction_time>? OR EXISTS(SELECT 1 FROM semantic_state_events st
 WHERE st.transaction_time>? AND st.transaction_time<=? AND
 ((st.object_kind='claim' AND st.object_id=c.claim_id) OR
 (st.object_kind='source_link' AND st.object_id IN (SELECT source_link_id FROM semantic_source_links WHERE claim_id=c.claim_id)))))
 ORDER BY c.transaction_time,c.claim_id LIMIT 9`, "global", scopeKeyForContext(scope), "session:"+string(scope.SessionID), current.Claim.SubjectEntityID, current.Claim.Predicate.ID, through, prior.ClaimID, after, after, through)
	if err != nil {
		return false, err
	}
	var ids []memory.SemanticID
	for rows.Next() {
		var id memory.SemanticID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(ids) > 8 {
		return true, nil
	}
	for _, id := range ids {
		peer, eligible, err := s.retrievalClaim(ctx, tx, metadata, id, memory.RetrievalCurrent, prior.ValidAtConstrained)
		if err != nil {
			return false, err
		}
		if eligible && retrievalClaimPairConflicts(current, peer) {
			return true, nil
		}
	}
	return false, nil
}

// A new original owner statement uses the same canonical subject/predicate
// relevance and source-span fences as the accepted-search companion path.
// Inspect at most eight suggestions, never raw payloads or a whole transcript.
func (s *Store) hasNewRetrievalOwnerStatement(ctx context.Context, tx *sql.Tx, scope memory.ScopeContext, metadata memory.ExactReadMetadata, current, prior memory.RetrievalEvidence) (bool, error) {
	coverage, err := conversationIndexCoverage(ctx, tx)
	if err != nil || coverage.State != "active" {
		return false, err
	}
	subject, err := loadSemanticEntityForInspection(ctx, tx, current.Claim.SubjectEntityID)
	if err != nil {
		return false, err
	}
	match := "(" + retrievalPhrase(current.Claim.Predicate.Token) + " OR " + retrievalPhrase(current.Claim.Predicate.Label) + ")"
	if subject.AnchorKind != "owner" {
		match += " AND " + retrievalPhrase(subject.CanonicalName)
	}
	after := prior.AsKnownAt
	for _, source := range current.Sources {
		observed, err := time.Parse(time.RFC3339Nano, source.ObservedAt)
		if err != nil {
			return false, err
		}
		if observed.After(after) {
			after = observed
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.id FROM memory_retrieval_event_fts_v3 f JOIN events e ON e.id=f.event_id
 WHERE memory_retrieval_event_fts_v3 MATCH ? AND f.generation=? AND f.scope_key=? AND e.event_type='user_message' AND e.role='user'
 AND `+conversationObservedTimeSQL+`>? AND `+conversationObservedTimeSQL+`<=?
 AND (e.session_id!=? OR e.sequence<COALESCE((SELECT MAX(sequence) FROM events WHERE session_id=? AND event_type='user_message'),0))
 ORDER BY `+conversationObservedTimeSQL+` DESC,e.id LIMIT 9`, match, conversationIndexGeneration, scopeKeyForContext(scope), formatSemanticTime(after), formatSemanticTime(metadata.AsKnownAt), scope.SessionID, scope.SessionID)
	if err != nil {
		return false, err
	}
	var ids []memory.EventID
	for rows.Next() {
		var id memory.EventID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(ids) > 8 {
		return true, nil
	}
	for _, id := range ids {
		event, eligible, err := loadConversationEvidence(ctx, tx, id)
		if err != nil {
			return false, err
		}
		if !eligible || event.scope != scopeKeyForContext(scope) || event.actor != memory.SemanticActorOwner || event.authority != memory.AuthorityOwnerStatement {
			continue
		}
		spans, err := conversationReadSpans(ctx, tx, event, memory.RetrievalCurrent, metadata.AsKnownAt)
		if err != nil {
			return false, err
		}
		spans, err = unrepresentedConversationSpans(ctx, tx, event, spans)
		if errors.Is(err, ErrConversationAssociation) {
			return true, nil // A bounded fresh read must report the unresolved association.
		}
		if err != nil {
			return false, err
		}
		if _, eligible := chooseConversationReadExcerpt(event.content, current.Claim.Predicate.Token+" "+current.Claim.Predicate.Label, spans); eligible {
			return true, nil
		}
	}
	return false, nil
}
