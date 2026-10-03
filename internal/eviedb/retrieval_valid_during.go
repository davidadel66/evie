package eviedb

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"

	"github.com/davidadel66/evie/internal/memory"
)

// retrievalValidDuringCandidates bounds the Claims the time-window generator
// reads.
const retrievalValidDuringCandidates = 8

// validDuringGenerator applies a request's explicit past time ("in March",
// "last year") to accepted memory (memory architecture Stage 6). Among the
// Claims matching the request's other words, those with a recorded Valid
// Time (their own, or the interval a "changed" correction closed) that
// overlaps the window rank first. When the window has ended, a Claim that was
// valid inside it but is not current now is read as a model-directed
// valid_at read would read it: at the latest instant of the overlap,
// valid-at constrained, with its status (superseded) shown. Retired and
// erroneous Claims stay excluded, nothing outside the reader's scopes is
// read, and Claims without a recorded Valid Time are neither boosted nor
// excluded: observation time cannot stand in for an unknown fact date. A
// model's explicit valid_at takes precedence.
func (c *retrievalCandidates) validDuringGenerator(ctx context.Context) error {
	query := c.plan.query
	if query.ValidAt != nil {
		return nil
	}
	reference, window, ok := c.store.recallTimeWindow(query)
	if !ok {
		return nil
	}
	c.timeWindow = &memory.RetrievalTimeWindow{Expression: window.Expression, From: window.From, To: window.To}
	words := withoutTimeWords(strings.FieldsFunc(query.Text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), reference)
	if len(words) == 0 {
		return nil
	}
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = `"` + word + `"`
	}
	keys := c.plan.scopes
	rows, err := c.tx.QueryContext(ctx, `SELECT claim_id,valid_from,valid_to FROM (
 SELECT c.claim_id AS claim_id,
 CASE WHEN k.old_claim_id IS NULL THEN c.valid_from ELSE k.old_effective_from END AS valid_from,
 CASE WHEN k.old_claim_id IS NULL THEN c.valid_to ELSE k.old_effective_to END AS valid_to,
 bm25(memory_retrieval_fts_v3) AS rank
 FROM memory_retrieval_fts_v3 f JOIN semantic_claims c ON c.claim_id=f.claim_id
 LEFT JOIN semantic_claim_corrections k ON k.old_claim_id=c.claim_id AND k.mode='changed' AND k.transaction_time<=?
 WHERE memory_retrieval_fts_v3 MATCH ? AND f.generation=? AND f.scope_key IN (?,?,?)
 AND EXISTS(SELECT 1 FROM memory_retrieval_generations g WHERE g.generation=f.generation AND g.state='active')
 AND f.claim_id NOT IN (SELECT claim_id FROM memory_retrieval_dirty))
 WHERE (valid_from IS NOT NULL OR valid_to IS NOT NULL) AND (valid_from IS NULL OR valid_from<?) AND (valid_to IS NULL OR valid_to>?)
 ORDER BY rank,claim_id LIMIT ?`, formatSemanticTime(c.plan.metadata.AsKnownAt), strings.Join(quoted, " OR "), memoryIndexGeneration, keys[0], keys[1], keys[2],
		formatSemanticTime(window.To), formatSemanticTime(window.From), retrievalValidDuringCandidates+1)
	if err != nil {
		return err
	}
	type bounded struct {
		id       memory.SemanticID
		from, to sql.NullString
	}
	var found []bounded
	for rows.Next() {
		var item bounded
		if err := rows.Scan(&item.id, &item.from, &item.to); err != nil {
			rows.Close()
			return err
		}
		if len(found) == retrievalValidDuringCandidates {
			c.truncated = true
			break
		}
		found = append(found, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	ended := !window.To.After(c.store.now())
	rank := 0
	for _, item := range found {
		candidate, err := c.read(ctx, item.id)
		if err != nil {
			return err
		}
		if candidate == nil && ended {
			// The latest instant inside both the window and the Claim's
			// recorded interval.
			at := window.To.Add(-time.Nanosecond)
			if item.to.Valid {
				to, err := parseSemanticTime(item.to.String)
				if err != nil {
					return err
				}
				if to.Before(window.To) {
					at = to.Add(-time.Nanosecond)
				}
			}
			if candidate, err = c.readValidAt(ctx, item.id, at); err != nil {
				return err
			}
		}
		if candidate == nil {
			continue
		}
		rank++
		candidate.direct = true
		addRetrievalRank(candidate, "valid_during", rank)
		c.timeWindow.Matched = true
	}
	return nil
}

// readValidAt reads a Claim that is not eligible now as it was valid at an
// earlier instant, under the read's own intent and authority rules.
func (c *retrievalCandidates) readValidAt(ctx context.Context, id memory.SemanticID, at time.Time) (*retrievalCandidate, error) {
	metadata := c.plan.metadata
	metadata.ValidAt = at
	evidence, eligible, err := c.store.retrievalClaim(ctx, c.tx, metadata, id, c.plan.query.Intent, true)
	if err != nil || !eligible {
		return nil, err
	}
	for _, source := range evidence.Sources {
		allowed := false
		for _, authority := range c.plan.authorities {
			allowed = allowed || source.Authority == authority
		}
		if !allowed {
			return nil, nil
		}
	}
	item := &retrievalCandidate{evidence: evidence}
	c.eligible[id] = item
	return item, nil
}
