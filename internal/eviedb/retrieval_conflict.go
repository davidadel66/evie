package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/davidadel66/evie/internal/memory"
)

// Evidence relations name only records that survived selection. An attributed
// newer statement is a candidate discrepancy, never an accepted contradiction.
func decorateRetrievalRelations(evidence []memory.RetrievalEvidence) {
	var claims []memory.ClaimInspection
	selected := make(map[memory.SemanticID]bool)
	for i := range evidence {
		e := &evidence[i]
		e.Conflicts = nil
		if e.Claim == nil || e.Kind != memory.RetrievalAcceptedMemory {
			continue
		}
		selected[e.ClaimID] = true
		if e.Status == memory.SemanticStatusActive && e.CurrentStatus == memory.SemanticStatusActive && e.EffectiveValidTime != nil {
			claims = append(claims, memory.ClaimInspection{SemanticClaim: *e.Claim, EffectiveValidTime: *e.EffectiveValidTime})
		}
	}
	warnings := exactClaimConflictWarnings(claims)
	for i := range evidence {
		e := &evidence[i]
		for _, warning := range warnings {
			if e.Kind == memory.RetrievalAcceptedMemory && containsSemanticID(warning.ClaimIDs, e.ClaimID) {
				e.Conflicts = append(e.Conflicts, warning)
			}
		}
		var related []memory.SemanticID
		for _, id := range e.RelatedClaimIDs {
			if selected[id] && !containsSemanticID(related, id) {
				related = append(related, id)
			}
		}
		e.RelatedClaimIDs = related
	}
}

func retrievalClaimPairConflicts(left, right memory.RetrievalEvidence) bool {
	if left.Claim == nil || right.Claim == nil || left.EffectiveValidTime == nil || right.EffectiveValidTime == nil ||
		left.Status != memory.SemanticStatusActive || right.Status != memory.SemanticStatusActive ||
		left.CurrentStatus != memory.SemanticStatusActive || right.CurrentStatus != memory.SemanticStatusActive {
		return false
	}
	return len(exactClaimConflictWarnings([]memory.ClaimInspection{
		{SemanticClaim: *left.Claim, EffectiveValidTime: *left.EffectiveValidTime},
		{SemanticClaim: *right.Claim, EffectiveValidTime: *right.EffectiveValidTime},
	})) > 0
}

// supplementAcceptedRetrieval shares the foreground candidate/result budgets
// with initial retrieval. It never runs a second unbounded relevance search.
func (s *Store) supplementAcceptedRetrieval(ctx context.Context, q *sql.Tx, scope memory.ScopeContext, query memory.RetrievalQuery, metadata memory.ExactReadMetadata, result *memory.RetrievalResult, seen map[memory.SemanticID]bool) (bool, error) {
	if len(result.Evidence) == 0 {
		return false, nil
	}
	remaining := retrievalCandidateLimit - len(seen)
	seeds := append([]memory.RetrievalEvidence(nil), result.Evidence...)
	for _, seed := range seeds {
		if seed.Claim == nil || seed.Status != memory.SemanticStatusActive || seed.CurrentStatus != memory.SemanticStatusActive {
			continue
		}
		if remaining <= 0 {
			result.Truncated = true
			break
		}
		rows, err := q.QueryContext(ctx, `SELECT c.claim_id FROM semantic_claims c JOIN semantic_scopes sc ON sc.scope_id=c.scope_id
 WHERE sc.scope_key IN (?,?,?) AND c.subject_entity_id=? AND c.predicate_id=? AND c.claim_id!=? AND c.transaction_time<=?
 ORDER BY c.claim_id LIMIT ?`, "global", scopeKeyForContext(scope), "session:"+string(scope.SessionID), seed.Claim.SubjectEntityID, seed.Claim.Predicate.ID, seed.ClaimID, formatSemanticTime(metadata.AsKnownAt), remaining+1)
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
		if err = rows.Err(); err != nil {
			rows.Close()
			return false, err
		}
		rows.Close()
		for _, id := range ids {
			if seen[id] {
				continue
			}
			if remaining <= 0 {
				result.Truncated = true
				break
			}
			seen[id], remaining = true, remaining-1
			peer, eligible, err := s.retrievalClaim(ctx, q, metadata, id, query.Intent, query.ValidAt != nil)
			if err != nil {
				return false, err
			}
			if !eligible || !retrievalClaimPairConflicts(seed, peer) {
				continue
			}
			if len(result.Evidence) >= query.Limit {
				result.Truncated = true
				continue
			}
			peer.Paths = []string{"conflicting_claim"}
			result.Evidence = append(result.Evidence, peer)
		}
	}
	coverage, err := conversationIndexCoverage(ctx, q)
	if err != nil {
		return false, err
	}
	if coverage.State != "active" {
		return true, nil
	}
	incomplete := coverage.Pending > 0
	groups := make(map[string]bool)
	eventsSeen := make(map[memory.EventID]bool)
	companions := 0
	for _, seed := range append([]memory.RetrievalEvidence(nil), result.Evidence...) {
		if seed.Claim == nil {
			continue
		}
		key := string(seed.Claim.SubjectEntityID) + ":" + string(seed.Claim.Predicate.ID)
		if groups[key] {
			continue
		}
		groups[key] = true
		if remaining <= 0 {
			result.Truncated = true
			break
		}
		var after time.Time
		var related []memory.SemanticID
		for _, item := range result.Evidence {
			if item.Claim == nil || item.Claim.SubjectEntityID != seed.Claim.SubjectEntityID || item.Claim.Predicate.ID != seed.Claim.Predicate.ID {
				continue
			}
			related = append(related, item.ClaimID)
			for _, source := range item.Sources {
				observed, err := time.Parse(time.RFC3339Nano, source.ObservedAt)
				if err != nil {
					return false, err
				}
				if observed.After(after) {
					after = observed
				}
			}
		}
		subject, err := loadSemanticEntityForInspection(ctx, q, seed.Claim.SubjectEntityID)
		if err != nil {
			return false, err
		}
		match := "(" + retrievalPhrase(seed.Claim.Predicate.Token) + " OR " + retrievalPhrase(seed.Claim.Predicate.Label) + " OR " + retrievalPhrase(query.Text) + ")"
		if subject.AnchorKind != "owner" {
			match += " AND " + retrievalPhrase(subject.CanonicalName)
		}
		rows, err := q.QueryContext(ctx, `SELECT e.id FROM memory_retrieval_event_fts f JOIN events e ON e.id=f.event_id
 WHERE memory_retrieval_event_fts MATCH ? AND f.generation=? AND f.scope_key=? AND e.event_type='user_message' AND e.role='user'
 AND `+conversationObservedTimeSQL+`>? AND `+conversationObservedTimeSQL+`<=?
 AND (e.session_id!=? OR e.sequence<COALESCE((SELECT MAX(sequence) FROM events WHERE session_id=? AND event_type='user_message'),0))
 AND (?=0 OR e.content!=COALESCE((SELECT content FROM events WHERE session_id=? AND event_type='user_message' ORDER BY sequence DESC LIMIT 1),''))
 ORDER BY `+conversationObservedTimeSQL+` DESC,bm25(memory_retrieval_event_fts),e.id LIMIT ?`, match, conversationIndexGeneration, scopeKeyForContext(scope), formatSemanticTime(after), formatSemanticTime(metadata.AsKnownAt), scope.SessionID, scope.SessionID, query.ExcludeCurrentRequestCopies, scope.SessionID, remaining+1)
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
		if err = rows.Err(); err != nil {
			rows.Close()
			return false, err
		}
		rows.Close()
		for _, id := range ids {
			if eventsSeen[id] {
				continue
			}
			if remaining <= 0 {
				result.Truncated = true
				break
			}
			eventsSeen[id], remaining = true, remaining-1
			e, eligible, err := loadConversationEvidence(ctx, q, id)
			if err != nil {
				return false, err
			}
			if !eligible || e.scope != scopeKeyForContext(scope) || e.actor != memory.SemanticActorOwner || e.authority != memory.AuthorityOwnerStatement {
				continue
			}
			spans, err := conversationReadSpans(ctx, q, e, query.Intent, metadata.AsKnownAt)
			if err != nil {
				return false, err
			}
			spans, err = unrepresentedConversationSpans(ctx, q, e, spans)
			if errors.Is(err, ErrConversationAssociation) {
				incomplete = true
				continue
			}
			if err != nil {
				return false, err
			}
			span, matched := chooseConversationReadExcerpt(e.content, seed.Claim.Predicate.Token+" "+seed.Claim.Predicate.Label+" "+query.Text, spans)
			if !matched {
				continue
			}
			if companions >= 2 || len(result.Evidence) >= query.Limit {
				result.Truncated = true
				continue
			}
			evidence := conversationTypedExcerpt(e, span, metadata.AsKnownAt, metadata.ValidAt, query.Intent)
			evidence.Paths = []string{"newer_owner_statement"}
			evidence.RelatedClaimIDs = append([]memory.SemanticID(nil), related...)
			result.Evidence = append(result.Evidence, evidence)
			result.Truncated = result.Truncated || span.start > 0 || span.end < len(e.content)
			companions++
		}
	}
	return incomplete, nil
}

func retrievalPhrase(value string) string {
	words := strings.FieldsFunc(value, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return `"` + strings.Join(words, " ") + `"`
}

// Existing accepted provenance is not a new independent owner observation.
// Subtract exact accepted Source Link intervals, retaining unrelated bytes.
func unrepresentedConversationSpans(ctx context.Context, q semanticInspectionQueryer, e conversationEvidence, spans []conversationReadSpan) ([]conversationReadSpan, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_part,locator_kind,locator_value,evidence_sha256 FROM semantic_source_links WHERE event_id=? ORDER BY source_link_id LIMIT 257`, e.id)
	if err != nil {
		return nil, err
	}
	var represented []evidenceSpan
	for rows.Next() {
		var locator memory.EvidenceLocator
		locator.EventID = e.id
		if err = rows.Scan(&locator.EventPart, &locator.LocatorKind, &locator.LocatorValue, &locator.EvidenceSHA256); err != nil {
			rows.Close()
			return nil, err
		}
		span, err := conversationLocatorSpan(e, locator)
		if err != nil || len(represented) >= 256 {
			rows.Close()
			return nil, ErrConversationAssociation
		}
		represented = append(represented, span)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	var result []conversationReadSpan
	for _, span := range spans {
		for _, part := range subtractCoveredEvidence([]evidenceSpan{span.evidenceSpan}, represented) {
			selected := span
			selected.evidenceSpan = part
			result = append(result, selected)
		}
	}
	return result, nil
}
