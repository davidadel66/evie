package eviedb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

type compilerInterval struct {
	First, Last               int64
	SelectionID, JobID, State string
}

// nextCompilerInterval returns one earliest unowned interval, capped before the
// next immutable owner. Endpoints are ledger coordinates, not source reads.
// A later owned suffix cannot advance the beginning of an earlier selection.
// If every requested coordinate is already owned, return its final existing
// unit; no completion or new ownership is inferred from that reuse.
func nextCompilerInterval(ctx context.Context, conn compilerQueryer, generation string, selection memory.CompilationSelection, first int64) (result compilerInterval, err error) {
	if first > selection.Cutoff {
		return result, errors.New("invalid selected interval bounds")
	}
	args := []any{generation, selection.Destination, selection.SessionID, selection.RootID, first, selection.Cutoff}
	var uncovered sql.NullInt64
	err = conn.QueryRowContext(ctx, `WITH starts(value) AS (
 SELECT ?5 UNION SELECT last_sequence+1 FROM memory_compiler_selections
 WHERE generation_id=?1 AND destination=?2 AND session_id=?3 AND root_id=?4 AND last_sequence>=?5 AND last_sequence<?6
) SELECT MIN(value) FROM starts WHERE value BETWEEN ?5 AND ?6 AND NOT EXISTS (
 SELECT 1 FROM memory_compiler_selections s WHERE s.generation_id=?1 AND s.destination=?2 AND s.session_id=?3 AND s.root_id=?4 AND s.first_sequence<=value AND s.last_sequence>=value
)`, args...).Scan(&uncovered)
	if err != nil {
		return result, err
	}
	if !uncovered.Valid {
		err = conn.QueryRowContext(ctx, `SELECT first_sequence,last_sequence,selection_id,COALESCE(job_id,''),state FROM memory_compiler_selections WHERE generation_id=?1 AND destination=?2 AND session_id=?3 AND root_id=?4 AND first_sequence<=?6 AND last_sequence>=?6 ORDER BY first_sequence DESC LIMIT 1`, args...).Scan(&result.First, &result.Last, &result.SelectionID, &result.JobID, &result.State)
		return result, err
	}
	result.First = uncovered.Int64
	result.Last = selection.Cutoff
	var next sql.NullInt64
	err = conn.QueryRowContext(ctx, `SELECT MIN(first_sequence) FROM memory_compiler_selections WHERE generation_id=? AND destination=? AND session_id=? AND root_id=? AND first_sequence>? AND first_sequence<=?`, generation, selection.Destination, selection.SessionID, selection.RootID, result.First, result.Last).Scan(&next)
	if err != nil {
		return result, err
	}
	if next.Valid {
		result.Last = next.Int64 - 1
	}
	return result, nil
}
