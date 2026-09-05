package eviedb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

// The explicit-resume queue contains only outstanding resume operations. A
// polling host never scans all retained cancelled jobs looking for permission.
func (s *Store) resumeCompilerHistoryStep(ctx context.Context, config CompilerSupervisorConfig) error {
	var selection string
	var owner memory.ScopeContext
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var request, generation string
		var resumeOrder int64
		err := conn.QueryRowContext(ctx, `SELECT z.request_id,q.generation_id,z.change_order FROM memory_compiler_history_resume_queue z JOIN memory_compiler_history_requests q USING(request_id) WHERE q.cancelled=0 ORDER BY z.checked_order,z.change_order,z.request_id LIMIT 1`).Scan(&request, &generation, &resumeOrder)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		check, err := nextHistoryCheck(ctx, conn)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_resume_queue SET checked_order=? WHERE request_id=?`, check, request); err != nil {
			return err
		}
		if extractor := config.Extractors[generation]; extractor == nil || extractor.ServerIdentity() == "" {
			return nil
		}
		var session string
		err = conn.QueryRowContext(ctx, `SELECT s.selection_id,j.session_id FROM memory_compiler_history_ranges r JOIN memory_compiler_jobs j ON j.generation_id=? AND j.destination=r.destination AND j.session_id=r.session_id AND j.first_sequence<=r.last_sequence AND j.last_sequence>=r.first_sequence JOIN memory_compiler_history_jobs h USING(job_id) JOIN memory_compiler_selections s USING(job_id) WHERE r.request_id=? AND j.state='cancelled' AND j.reason='history_cancelled' AND h.cancel_order<? AND (j.attempts<5 OR EXISTS(SELECT 1 FROM memory_compiler_stages st WHERE st.job_id=j.job_id AND st.consumed=0)) ORDER BY r.ordinal,j.last_sequence,j.first_sequence LIMIT 1`, generation, request, resumeOrder).Scan(&selection, &session)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = conn.ExecContext(ctx, `DELETE FROM memory_compiler_history_resume_queue WHERE request_id=?`, request)
			return err
		}
		if err != nil {
			return err
		}
		owner, err = reviewSourceContext(ctx, conn, memory.SessionID(session))
		return err
	})
	if err != nil || selection == "" {
		return err
	}
	// The existing API repeats current authority and pause checks in its own
	// serialized resource transaction, including a concurrent request cancellation.
	_, err = s.ResumeCompilation(ctx, owner, selection)
	if errors.Is(err, ErrCompilerCapacityBlocked) || errors.Is(err, ErrCompilerFence) {
		return nil
	}
	return err
}
