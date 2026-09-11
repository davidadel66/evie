package agent

import (
	"context"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// Query plans live only in the current turn. A later explicit temporal view
// supersedes the earlier plan for its shared evidence; refreshing the old plan
// must not silently undo that choice. Durable receipts remain immutable.
type retrievalQueryRecord struct {
	query memory.RetrievalQuery
	ids   []string
	refs  []memory.RetrievalReference
}

func (r *retrievalTurn) rememberQuery(query memory.RetrievalQuery, evidence []memory.RetrievalEvidence) {
	if len(r.queries) >= retrievalSearchLimit {
		return
	}
	query.Anchor, query.Covered = nil, nil
	query.RefreshReferences = nil
	record := retrievalQueryRecord{query: query}
	if r.origins == nil {
		r.origins = make(map[string]int)
	}
	for _, item := range evidence {
		record.ids = append(record.ids, item.ID)
		record.refs = append(record.refs, item.Reference())
		r.origins[item.ID] = len(r.queries)
	}
	r.queries = append(r.queries, record)
}

func (r *retrievalTurn) refreshInvalidated(ctx context.Context, prior, valid []memory.RetrievalEvidence) {
	lost := make(map[string]bool)
	for _, item := range prior {
		lost[item.ID] = true
	}
	for _, item := range valid {
		delete(lost, item.ID)
	}
	// Search appends a new plan; inspect only the plans held at entry so one
	// dispatch cannot repeatedly chase an unavailable or changing source.
	plans := append([]retrievalQueryRecord(nil), r.queries...)
	refreshed := false
	for index, record := range plans {
		for _, ref := range record.refs {
			if lost[ref.ID] && r.origins[ref.ID] == index {
				record.query.RefreshReferences = append(record.query.RefreshReferences, ref)
			}
		}
		if len(record.query.RefreshReferences) == 0 {
			continue
		}
		r.refreshes++
		result, _ := r.search(ctx, record.query)
		refreshed = refreshed || len(result.Evidence) > 0
		if len(result.Evidence) == 0 && result.Status != memory.RetrievalFailed && result.Status != memory.RetrievalCancelled && result.Status != memory.RetrievalExhausted {
			r.status = memory.RetrievalPartial
			if len(r.evidence) == 0 {
				r.status = memory.RetrievalUnavailable
			}
		}
	}
	if !refreshed || len(r.evidence) == 0 {
		return
	}
	// Refreshed sources must pass the same final current-state/egress check as
	// reused sources, within the remaining shared work budget.
	if r.work >= retrievalTurnWork {
		r.evidence, r.status = nil, memory.RetrievalExhausted
		return
	}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, min(retrievalSearchDeadline, retrievalTurnWork-r.work))
	checked, err := r.kernel.RevalidateMemoryEvidence(checkCtx, r.scope, r.evidence)
	cancel()
	r.work += time.Since(started)
	if err != nil {
		r.evidence, r.status = nil, memory.RetrievalUnavailable
		return
	}
	if len(checked) < len(r.evidence) {
		r.status = memory.RetrievalPartial
		if len(checked) == 0 {
			r.status = memory.RetrievalUnavailable
		}
	}
	r.evidence = checked
}
