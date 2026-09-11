package eviedb

import (
	"context"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// A passage has two lifecycle views: the explicit read pin and the current
// policy. Source retraction is always evaluated under current access rules.
type conversationReadSpan struct {
	evidenceSpan
	status, currentStatus memory.SemanticObjectStatus
}

// Event append stores UTC RFC3339Nano, whose fractional digits are trimmed.
// Pad only for comparison; preserve the original observed_at receipt value.
const conversationObservedTimeSQL = `(substr(e.recorded_at,1,19) || CASE WHEN substr(e.recorded_at,20,1)='.'
 THEN substr(substr(e.recorded_at,20,length(e.recorded_at)-20) || '000000000',1,10)
 ELSE '.000000000' END || 'Z')`

func conversationReadSpans(ctx context.Context, q semanticInspectionQueryer, e conversationEvidence, intent string, known time.Time) ([]conversationReadSpan, error) {
	knownAt := ""
	if !known.IsZero() {
		knownAt = formatSemanticTime(known)
	}
	rows, err := q.QueryContext(ctx, `WITH associations AS (
 SELECT sl.event_part,sl.locator_kind,sl.locator_value,sl.evidence_sha256,
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=sl.claim_id
 AND (?='' OR transaction_time<=?) ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),'active') AS pinned,
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=sl.claim_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),'active') AS current,
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='source_link' AND object_id=sl.source_link_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),'eligible') AS source_state
 FROM semantic_source_links sl WHERE sl.event_id=?)
 SELECT event_part,locator_kind,locator_value,evidence_sha256,pinned,current,source_state FROM associations
 WHERE pinned='retired' OR current='retired' OR source_state='retracted' LIMIT 257`, knownAt, knownAt, e.id)
	if err != nil {
		return nil, err
	}
	type association struct {
		span                    evidenceSpan
		pinned, current, source memory.SemanticStateValue
	}
	var associations []association
	boundaries := []int{0, len(e.content)}
	for rows.Next() {
		var locator memory.EvidenceLocator
		locator.EventID = e.id
		var a association
		if err = rows.Scan(&locator.EventPart, &locator.LocatorKind, &locator.LocatorValue, &locator.EvidenceSHA256, &a.pinned, &a.current, &a.source); err != nil {
			rows.Close()
			return nil, err
		}
		if len(associations) >= 256 {
			rows.Close()
			return nil, ErrConversationAssociation
		}
		a.span, err = conversationLocatorSpan(e, locator)
		if err != nil {
			rows.Close()
			return nil, ErrConversationAssociation
		}
		associations = append(associations, a)
		boundaries = append(boundaries, a.span.start, a.span.end)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	sort.Ints(boundaries)
	var result []conversationReadSpan
	for i := 1; i < len(boundaries); i++ {
		span := conversationReadSpan{evidenceSpan{boundaries[i-1], boundaries[i]}, memory.SemanticStatusActive, memory.SemanticStatusActive}
		if span.start == span.end {
			continue
		}
		suppressed := false
		for _, a := range associations {
			if a.span.end <= span.start || a.span.start >= span.end {
				continue
			}
			suppressed = suppressed || a.source == memory.SemanticStateRetracted
			if a.pinned == memory.SemanticStateRetired {
				span.status = memory.SemanticStatusRetired
			}
			if a.current == memory.SemanticStateRetired {
				span.currentStatus = memory.SemanticStatusRetired
			}
		}
		if suppressed || intent != memory.RetrievalHistorical && (span.status == memory.SemanticStatusRetired || span.currentStatus == memory.SemanticStatusRetired) {
			continue
		}
		if len(result) > 0 {
			prior := &result[len(result)-1]
			if prior.end == span.start && prior.status == span.status && prior.currentStatus == span.currentStatus {
				prior.end = span.end
				continue
			}
		}
		result = append(result, span)
	}
	return result, nil
}

func chooseConversationReadExcerpt(content, query string, spans []conversationReadSpan) (conversationReadSpan, bool) {
	for _, span := range spans {
		if selected, matched := chooseConversationExcerpt(content, query, []evidenceSpan{span.evidenceSpan}); matched {
			span.evidenceSpan = selected
			return span, true
		}
	}
	return conversationReadSpan{}, false
}

func conversationTypedExcerpt(e conversationEvidence, span conversationReadSpan, known, valid time.Time, intent string) memory.RetrievalEvidence {
	result := conversationExcerpt(e, span.evidenceSpan, known, valid)
	if intent == "" {
		intent = memory.RetrievalCurrent
	}
	result.Intent, result.Status, result.CurrentStatus = intent, span.status, span.currentStatus
	return result
}
