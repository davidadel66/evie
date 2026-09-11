package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

const expansionSequenceWindow = 64

// expandConversation is an exact, bounded read anchored to evidence selected
// by the current turn. It does not rely on an index or infer broader authority
// from an event ID supplied in model arguments.
func (s *Store) expandConversation(ctx context.Context, scope memory.ScopeContext, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
	result := memory.RetrievalResult{Status: memory.RetrievalFailed, Evidence: []memory.RetrievalEvidence{}}
	var err error
	query, err = normalizeRetrievalBounds(query)
	if err != nil || query.Before < 0 || query.Before > 2 || query.After < 0 || query.After > 2 || len(query.Covered) > 64 {
		return result, ErrInvalidRetrievalQuery
	}
	if query.Anchor == nil || query.AnchorID != query.Anchor.ID || query.Anchor.AsKnownAt.IsZero() || query.Anchor.ValidAt.IsZero() {
		result.Status = memory.RetrievalUnavailable
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, retrievalDeadline)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	defer tx.Rollback()
	if err = validateSessionScopeIdentity(ctx, tx, scope); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	anchor, eligible, err := s.resolveConversationReference(ctx, tx, scope, *query.Anchor)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	if !eligible {
		result.Status = memory.RetrievalUnavailable
		return result, nil
	}
	known, valid, intent := anchor.AsKnownAt, anchor.ValidAt, anchor.Intent
	anchorEvent, eligible, err := loadConversationEvidence(ctx, tx, anchor.Sources[0].EventID)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	if !eligible {
		result.Status = memory.RetrievalUnavailable
		return result, nil
	}
	cutoff := int64(0)
	if anchorEvent.session == scope.SessionID {
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM events WHERE session_id=? AND event_type='user_message'`, scope.SessionID).Scan(&cutoff); err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		if cutoff == 0 || anchorEvent.sequence >= cutoff {
			result.Status = memory.RetrievalUnavailable
			return result, nil
		}
	}
	result.Coverage, err = conversationIndexCoverage(ctx, tx)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	covered := map[memory.EventID][]evidenceSpan{}
	refs := append([]memory.RetrievalReference{*query.Anchor}, query.Covered...)
	for _, ref := range refs {
		if ref.Kind != memory.RetrievalConversationExcerpt || len(ref.Sources) != 1 || ref.Sources[0].SessionID != anchorEvent.session {
			continue
		}
		prior, eligible, err := s.resolveConversationReference(ctx, tx, scope, ref)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		if !eligible {
			continue
		}
		e, eligible, err := loadConversationEvidence(ctx, tx, prior.Sources[0].EventID)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		if !eligible {
			continue
		}
		span, err := conversationLocatorSpan(e, ref.Sources[0].EvidenceLocator)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		covered[e.id] = append(covered[e.id], span)
	}
	type candidate struct {
		id       memory.EventID
		sequence int64
	}
	candidates := []candidate{{id: anchorEvent.id, sequence: anchorEvent.sequence}}
	loadNeighbors := func(before bool, count int) error {
		if count == 0 {
			return nil
		}
		operator, order := ">", "ASC"
		bound := anchorEvent.sequence + expansionSequenceWindow
		if before {
			operator, order = "<", "DESC"
			bound = max(int64(0), anchorEvent.sequence-expansionSequenceWindow)
		}
		windowOperator := "<="
		if before {
			windowOperator = ">="
		}
		statement := `SELECT e.id,e.sequence FROM events e WHERE session_id=? AND sequence ` + operator + ` ? AND sequence ` + windowOperator + ` ?
 AND event_type IN ('user_message','assistant_message') AND content!='' AND (?=0 OR sequence<?)
 AND ` + conversationObservedTimeSQL + `<=?
 ORDER BY sequence ` + order + ` LIMIT ?`
		rows, err := tx.QueryContext(ctx, statement, anchorEvent.session, anchorEvent.sequence, bound, cutoff, cutoff, formatSemanticTime(known), count)
		if err != nil {
			return err
		}
		defer rows.Close()
		loaded := 0
		for rows.Next() {
			var c candidate
			if err = rows.Scan(&c.id, &c.sequence); err != nil {
				return err
			}
			candidates = append(candidates, c)
			loaded++
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if loaded < count {
			// An indexed sequence probe reports an incomplete bounded window
			// without scanning farther to discover the next public message.
			var outside bool
			statement = `SELECT EXISTS(SELECT 1 FROM events e WHERE session_id=? AND sequence ` + operator + ` ? AND (?=0 OR sequence<?) AND ` + conversationObservedTimeSQL + `<=?)`
			if err = tx.QueryRowContext(ctx, statement, anchorEvent.session, bound, cutoff, cutoff, formatSemanticTime(known)).Scan(&outside); err != nil {
				return err
			}
			result.Truncated = result.Truncated || outside
		}
		return nil
	}
	if err = loadNeighbors(true, query.Before); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	if err = loadNeighbors(false, query.After); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].sequence < candidates[j].sequence })
	anchorSpan, err := conversationLocatorSpan(anchorEvent, query.Anchor.Sources[0].EvidenceLocator)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	for _, c := range candidates {
		e, eligible, err := loadConversationEvidence(ctx, tx, c.id)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		if !eligible || e.scope != scopeKeyForContext(scope) || e.session != anchorEvent.session {
			result.Truncated = true
			continue
		}
		spans, err := conversationReadSpans(ctx, tx, e, intent, known)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		eligibleBytes := 0
		var additional []conversationReadSpan
		for _, span := range spans {
			eligibleBytes += span.end - span.start
			for _, uncovered := range subtractCoveredEvidence([]evidenceSpan{span.evidenceSpan}, covered[e.id]) {
				part := span
				part.evidenceSpan = uncovered
				additional = append(additional, part)
			}
		}
		if eligibleBytes < len(e.content) {
			result.Truncated = true
		}
		for _, span := range additional {
			if strings.TrimSpace(e.content[span.start:span.end]) == "" {
				continue
			}
			if len(result.Evidence) >= query.Limit {
				result.Truncated = true
				break
			}
			selected := span
			if span.end-span.start > conversationExcerptBytes {
				result.Truncated = true
				if e.sequence < anchorEvent.sequence || e.id == anchorEvent.id && span.end <= anchorSpan.start {
					selected.start = span.end - conversationExcerptBytes
					for selected.start < selected.end && !utf8.RuneStart(e.content[selected.start]) {
						selected.start++
					}
				} else {
					selected.end = span.start + conversationExcerptBytes
					for selected.end > selected.start && !utf8.ValidString(e.content[selected.start:selected.end]) {
						selected.end--
					}
				}
			}
			evidence := conversationTypedExcerpt(e, selected, known, valid, intent)
			evidence.Paths = []string{memory.RetrievalConversationExpansion}
			result.Evidence = append(result.Evidence, evidence)
		}
	}
	if err = tx.Commit(); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	result.Status = memory.RetrievalSuccess
	if len(result.Evidence) == 0 {
		result.Status = memory.RetrievalEmpty
	}
	for {
		encoded, err := json.Marshal(result)
		if err != nil {
			return result, err
		}
		result.SerializedBytes = len(encoded)
		encoded, _ = json.Marshal(result)
		result.SerializedBytes = len(encoded)
		if len(encoded) <= query.MaxBytes {
			return result, nil
		}
		result.Truncated = true
		if len(result.Evidence) == 0 {
			result.Status = memory.RetrievalExhausted
			return result, nil
		}
		result.Evidence = result.Evidence[:len(result.Evidence)-1]
		if len(result.Evidence) == 0 {
			result.Status = memory.RetrievalExhausted
		}
	}
}

func subtractCoveredEvidence(allowed, covered []evidenceSpan) []evidenceSpan {
	covered = append([]evidenceSpan(nil), covered...)
	sort.Slice(covered, func(i, j int) bool { return covered[i].start < covered[j].start })
	var result []evidenceSpan
	for _, span := range allowed {
		cursor := span.start
		for _, prior := range covered {
			if prior.end <= cursor || prior.start >= span.end {
				continue
			}
			if prior.start > cursor {
				result = append(result, evidenceSpan{cursor, min(prior.start, span.end)})
			}
			cursor = max(cursor, prior.end)
			if cursor >= span.end {
				break
			}
		}
		if cursor < span.end {
			result = append(result, evidenceSpan{cursor, span.end})
		}
	}
	return result
}
