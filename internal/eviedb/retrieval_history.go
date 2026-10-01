package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// historicalCandidateLimit bounds the retired or superseded Claims a reader
// can see that one excerpt is compared with. Beyond it an excerpt cannot be
// classified, so callers withhold it and report incomplete coverage rather
// than present a possible restatement as current.
const historicalCandidateLimit = 1024

// annotatedConversationExcerpt builds an excerpt and links it to accepted
// Claims that are no longer current (harness review M2).
func annotatedConversationExcerpt(ctx context.Context, q semanticInspectionQueryer, scope memory.ScopeContext, e conversationEvidence, span conversationReadSpan, known, valid time.Time, intent string) (memory.RetrievalEvidence, error) {
	evidence := conversationTypedExcerpt(e, span, known, valid, intent)
	err := annotateConversationHistory(ctx, q, scope, e, span.evidenceSpan, known, &evidence)
	return evidence, err
}

// annotateConversationHistory labels, never suppresses. Retirement
// suppression stays in conversationReadSpans.
//   - source: the excerpt overlaps an eligible Source Link of a retired or
//     superseded Claim. A superseded Claim also marks the excerpt superseded,
//     and the link names the correction mode and replacement (memory decision
//     10: corrected evidence stays retrievable as history).
//   - restatement: the excerpt is not a Source of the Claim, and one sentence
//     repeats the Claim's saved value with a Predicate word or a non-owner
//     subject's name. An event that is a Source of an active Claim in the same
//     subject and Predicate family states the current value and is exempt.
//
// Claim identities are named only for scopes the reader may read.
func annotateConversationHistory(ctx context.Context, q semanticInspectionQueryer, scope memory.ScopeContext, e conversationEvidence, span evidenceSpan, known time.Time, evidence *memory.RetrievalEvidence) error {
	readable := []string{"global", scopeKeyForContext(scope), "session:" + string(scope.SessionID)}
	knownAt := ""
	if !known.IsZero() {
		knownAt = formatSemanticTime(known)
	}
	rows, err := q.QueryContext(ctx, `SELECT sl.claim_id,sl.event_part,sl.locator_kind,sl.locator_value,sl.evidence_sha256,sc.scope_key,c.subject_entity_id,c.predicate_token,
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=sl.claim_id
 AND (?='' OR transaction_time<=?) ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),'active'),
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=sl.claim_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),'active'),
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='source_link' AND object_id=sl.source_link_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),'eligible')
 FROM semantic_source_links sl JOIN semantic_claims c ON c.claim_id=sl.claim_id JOIN semantic_scopes sc ON sc.scope_id=c.scope_id
 WHERE sl.event_id=? ORDER BY sl.source_link_id LIMIT 257`, knownAt, knownAt, e.id)
	if err != nil {
		return err
	}
	type sourceLink struct {
		claim                   memory.SemanticID
		locator                 memory.EvidenceLocator
		scopeKey, family        string
		pinned, current, source memory.SemanticStateValue
	}
	var links []sourceLink
	for rows.Next() {
		var link sourceLink
		var subject memory.SemanticID
		var token string
		link.locator.EventID = e.id
		if err = rows.Scan(&link.claim, &link.locator.EventPart, &link.locator.LocatorKind, &link.locator.LocatorValue, &link.locator.EvidenceSHA256,
			&link.scopeKey, &subject, &token, &link.pinned, &link.current, &link.source); err != nil {
			rows.Close()
			return err
		}
		link.family = string(subject) + "\x00" + token
		links = append(links, link)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(links) > 256 {
		return ErrConversationAssociation
	}
	var historical []memory.RetrievalHistoricalClaim
	sourced := map[memory.SemanticID]bool{}
	current := map[string]bool{}
	for _, link := range links {
		sourced[link.claim] = true
		if link.source == memory.SemanticStateRetracted {
			continue
		}
		if link.current == memory.SemanticStateActive {
			current[link.family] = true
		}
		if !lifecycleEnded(link.pinned) && !lifecycleEnded(link.current) {
			continue
		}
		located, err := conversationLocatorSpan(e, link.locator)
		if err != nil {
			return ErrConversationAssociation
		}
		if located.end <= span.start || located.start >= span.end {
			continue
		}
		if link.pinned == memory.SemanticStateSuperseded && evidence.Status == memory.SemanticStatusActive {
			evidence.Status = memory.SemanticStatusSuperseded
		}
		if link.current == memory.SemanticStateSuperseded && evidence.CurrentStatus == memory.SemanticStatusActive {
			evidence.CurrentStatus = memory.SemanticStatusSuperseded
		}
		if !containsString(readable, link.scopeKey) {
			continue
		}
		state := link.current
		if !lifecycleEnded(state) {
			state = link.pinned
		}
		entry, err := historicalClaimLink(ctx, q, memory.RetrievalHistoricalSource, link.claim, state)
		if err != nil {
			return err
		}
		historical = append(historical, entry)
	}
	restated, err := restatedHistoricalClaims(ctx, q, readable, e, span, sourced, current)
	if err != nil {
		return err
	}
	historical = append(historical, restated...)
	sort.SliceStable(historical, func(i, j int) bool {
		if historical[i].Relation != historical[j].Relation {
			return historical[i].Relation == memory.RetrievalHistoricalSource
		}
		return historical[i].ClaimID < historical[j].ClaimID
	})
	evidence.HistoricalClaims = historical
	return nil
}

func lifecycleEnded(state memory.SemanticStateValue) bool {
	return state == memory.SemanticStateRetired || state == memory.SemanticStateSuperseded
}

func historicalClaimLink(ctx context.Context, q semanticInspectionQueryer, relation string, claim memory.SemanticID, state memory.SemanticStateValue) (memory.RetrievalHistoricalClaim, error) {
	link := memory.RetrievalHistoricalClaim{Relation: relation, ClaimID: claim, Status: semanticStatus(state)}
	if state != memory.SemanticStateSuperseded {
		return link, nil
	}
	err := q.QueryRowContext(ctx, `SELECT mode,replacement_claim_id FROM semantic_claim_corrections WHERE old_claim_id=?`, claim).Scan(&link.CorrectionMode, &link.ReplacementClaimID)
	if errors.Is(err, sql.ErrNoRows) {
		return link, nil
	}
	return link, err
}

// restatedHistoricalClaims compares the excerpt with every readable affirmed
// Claim that has ever been retired or superseded and is still not current.
func restatedHistoricalClaims(ctx context.Context, q semanticInspectionQueryer, readable []string, e conversationEvidence, span evidenceSpan, sourced map[memory.SemanticID]bool, current map[string]bool) ([]memory.RetrievalHistoricalClaim, error) {
	rows, err := q.QueryContext(ctx, `SELECT c.claim_id,c.subject_entity_id,COALESCE(se.anchor_kind,''),se.canonical_name,c.predicate_token,p.label,
 COALESCE(c.literal_kind,''),COALESCE(c.literal_value,oe.canonical_name,'')
 FROM semantic_claims c JOIN semantic_scopes sc ON sc.scope_id=c.scope_id JOIN semantic_predicates p ON p.predicate_id=c.predicate_id
 JOIN semantic_entities se ON se.entity_id=c.subject_entity_id LEFT JOIN semantic_entities oe ON oe.entity_id=c.object_entity_id
 WHERE sc.scope_key IN (?,?,?) AND c.polarity='affirmed'
 AND EXISTS(SELECT 1 FROM semantic_state_events st WHERE st.object_kind='claim' AND st.object_id=c.claim_id AND st.state IN ('retired','superseded'))
 ORDER BY c.claim_id LIMIT ?`, readable[0], readable[1], readable[2], historicalCandidateLimit+1)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id      memory.SemanticID
		family  string
		wording claimWording
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var subject memory.SemanticID
		var anchor, name, token, label, kind, value string
		if err = rows.Scan(&c.id, &subject, &anchor, &name, &token, &label, &kind, &value); err != nil {
			rows.Close()
			return nil, err
		}
		c.family = string(subject) + "\x00" + token
		c.wording.addSubject(name, anchor == "owner")
		c.wording.addValue(kind, value)
		c.wording.addPredicate(token)
		c.wording.addPredicate(label)
		candidates = append(candidates, c)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(candidates) > historicalCandidateLimit {
		return nil, ErrConversationAssociation
	}
	var result []memory.RetrievalHistoricalClaim
	for _, c := range candidates {
		if sourced[c.id] || current[c.family] {
			continue
		}
		if _, ok := firstSentence(e.content, span.start, span.end, c.wording.restatement); !ok {
			continue
		}
		var state memory.SemanticStateValue
		if err := q.QueryRowContext(ctx, `SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1`, c.id).Scan(&state); err != nil {
			return nil, err
		}
		if !lifecycleEnded(state) {
			continue // restored after retirement: current again
		}
		link, err := historicalClaimLink(ctx, q, memory.RetrievalHistoricalRestatement, c.id, state)
		if err != nil {
			return nil, err
		}
		result = append(result, link)
	}
	return result, nil
}
