package eviedb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

// InspectCompilerHistory pages content-free exact intervals for one receipt
// range. Indexed coordinates and persisted manifests determine progress; a
// scanning cursor or accepted/rejected candidate never establishes completion.
// Exact session totals use bounded persisted indexing; ErrCompilerStatusIndexing
// asks the caller to repeat this authorized request without returning partial counts.
func (s *Store) InspectCompilerHistory(ctx context.Context, owners []memory.ScopeContext, id string, rangeIndex int, afterSequence int64, limit int) (result memory.CompilerHistoryProgress, err error) {
	result.Intervals = []memory.CompilerHistoryInterval{}
	if limit < 1 || limit > 64 || rangeIndex < 0 || afterSequence < 0 {
		return result, errors.New("history progress requires a range index, nonnegative cursor, and limit 1 through 64")
	}
	indexing := false
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		receipt, state, err := loadCompilerHistory(ctx, conn, owners, id)
		if err != nil {
			return err
		}
		if rangeIndex >= len(receipt.Request.Ranges) {
			return errors.New("history range index outside receipt")
		}
		result.Receipt = receipt
		result.RequestState = state
		result.RangeIndex = rangeIndex
		r := receipt.Request.Ranges[rangeIndex]
		// All rows below are bounded by the receipt's <=10,000 event selection.
		// Event ID joins preserve holes/control members and interleaved ancestry.
		// nextCompilerInterval assigns nonoverlapping immutable ownership for
		// each generation/destination/session/root. One indexed predecessor
		// therefore identifies an event's only possible owner; its endpoint
		// check preserves gaps without scanning retained later segments.
		rows, err := conn.QueryContext(ctx, `SELECT e.sequence,
  CASE WHEN c.outcome IS NOT NULL AND EXISTS(SELECT 1 FROM json_each(c.event_ids) WHERE value=e.id) THEN c.outcome
       WHEN h.state='excluded' AND h.selection_id='' THEN 'excluded'
       WHEN j.job_id IS NOT NULL THEN j.state
       WHEN s.state='failed' OR h.state='failed' THEN 'failed'
       WHEN ? THEN 'cancelled'
       WHEN s.selection_id IS NOT NULL THEN s.state
       ELSE COALESCE(h.state,'selected_unmaterialized') END,
  COALESCE(j.reason,s.reason,h.reason,''),COALESCE(j.job_id,''),COALESCE(s.selection_id,''),COALESCE(j.attempts,0),COALESCE(j.retry_at,0)
 FROM events e
 LEFT JOIN memory_compiler_history_events m ON m.request_id=? AND m.range_ordinal=? AND m.event_id=e.id
 LEFT JOIN memory_compiler_history_roots h ON h.request_id=m.request_id AND h.range_ordinal=m.range_ordinal AND h.root_id=m.root_id
 LEFT JOIN memory_compiler_selections s ON s.selection_id=(
  SELECT candidate.selection_id FROM memory_compiler_selections candidate WHERE candidate.generation_id=? AND candidate.destination=? AND candidate.session_id=e.session_id AND candidate.root_id=m.root_id AND candidate.first_sequence<=e.sequence ORDER BY candidate.first_sequence DESC LIMIT 1
 ) AND e.sequence<=s.last_sequence
 LEFT JOIN memory_compiler_jobs j ON j.job_id=s.job_id
 LEFT JOIN memory_compiler_coverage c ON c.job_id=j.job_id
 WHERE e.session_id=? AND e.sequence BETWEEN ? AND ? ORDER BY e.sequence`, state.Cancelled, id, rangeIndex, receipt.GenerationID, r.Destination, r.SessionID, r.FirstSequence, r.LastSequence)
		if err != nil {
			return err
		}
		frontier := r.FirstSequence - 1
		blocked := false
		var all []memory.CompilerHistoryInterval
		for rows.Next() {
			var item memory.CompilerHistoryInterval
			if err := rows.Scan(&item.FirstSequence, &item.State, &item.Reason, &item.JobID, &item.SelectionID, &item.Attempts, &item.RetryAt); err != nil {
				rows.Close()
				return err
			}
			item.LastSequence = item.FirstSequence
			item.Reason = safeHistoryReason(item.Reason)
			completed := item.State == "completed_candidates" || item.State == "completed_empty" || item.State == "excluded"
			if !blocked && completed {
				frontier = item.LastSequence
			} else if !completed {
				blocked = true
			}
			if len(all) > 0 {
				last := &all[len(all)-1]
				if last.LastSequence+1 == item.FirstSequence && last.State == item.State && last.JobID == item.JobID && last.SelectionID == item.SelectionID && last.Reason == item.Reason {
					last.LastSequence = item.LastSequence
					continue
				}
			}
			all = append(all, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		result.ContiguousFrontier = frontier
		for _, item := range all {
			if item.LastSequence <= afterSequence {
				continue
			}
			if item.FirstSequence <= afterSequence {
				item.FirstSequence = afterSequence + 1
			}
			if len(result.Intervals) == limit {
				result.NextSequence = result.Intervals[len(result.Intervals)-1].LastSequence
				break
			}
			result.Intervals = append(result.Intervals, item)
		}
		total, selected, ready, err := compilerStatusEventCounts(ctx, conn, r.SourceScope, r.SessionID, receipt.GenerationID, r.Destination)
		if err != nil {
			return err
		}
		result.SelectedSessionEvents, result.OutsideSelectionEvents = selected, total-selected
		indexing = !ready
		return nil
	})
	if err == nil && indexing {
		return memory.CompilerHistoryProgress{}, ErrCompilerStatusIndexing
	}
	return
}

func safeHistoryReason(reason string) string {
	switch reason {
	case "source_scope_unavailable", "history_cancelled", "job_capacity", "unfinished_live_turn", "invalid_source_ancestry", "source_inspection_limit", "prohibited_source", "no_admitted_support", "secret_field":
		return reason
	default:
		return safeCompilerReason(reason)
	}
}
