package eviedb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

// InspectCompilerStatus returns one exact source session's metadata without
// rendering protected evidence, generation prompts, or raw extractor errors.
// The job cursor is stable and each request visits at most 64 ledger rows.
func (s *Store) InspectCompilerStatus(ctx context.Context, owner memory.ScopeContext, afterJobID string, limit int) (memory.CompilerStatus, error) {
	result := memory.CompilerStatus{Jobs: []memory.CompilerWorkStatus{}, CapacityState: "available"}
	if limit < 1 || limit > 64 {
		return result, errors.New("compiler status limit must be 1 through 64")
	}
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		selection := memory.CompilationSelection{SessionID: owner.SessionID, Destination: scopeKeyForContext(owner)}
		if err := compilerAuthorize(ctx, conn, owner, selection); err != nil {
			return err
		}
		var capacity string
		err := conn.QueryRowContext(ctx, `SELECT state FROM memory_compiler_capacity WHERE singleton=1`).Scan(&capacity)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if capacity == "reserved" {
			result.CapacityState = "busy"
		}
		if capacity == "release_pending" {
			result.CapacityState = "capacity_blocked"
			result.CapacityRecovery = "await authenticated request release; this runtime may require a verified controlled server restart"
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(stage_bytes),0),COALESCE(SUM(candidate_positions),0) FROM memory_compiler_resources`).Scan(&result.ReservedStages, &result.ReservedStageBytes, &result.ReservedCandidates); err != nil {
			return err
		}
		rows, err := conn.QueryContext(ctx, `SELECT j.job_id,j.generation_id,j.session_id,j.first_sequence,j.last_sequence,j.state,j.reason,j.attempts,COALESCE(j.retry_at,0),q.pause_reason,q.lane FROM memory_compiler_jobs j JOIN memory_compiler_job_schedule q ON q.job_id=j.job_id WHERE j.session_id=? AND j.job_id>? ORDER BY j.job_id LIMIT ?`, owner.SessionID, afterJobID, limit)
		if err != nil {
			return err
		}
		for rows.Next() {
			var job memory.CompilerWorkStatus
			if err := rows.Scan(&job.JobID, &job.GenerationID, &job.SessionID, &job.FirstSequence, &job.LastSequence, &job.State, &job.Reason, &job.Attempts, &job.RetryAt, &job.PauseReason, &job.Lane); err != nil {
				rows.Close()
				return err
			}
			// Durable reasons are code-controlled; unexpected storage bytes are never
			// promoted into diagnostics when raw source integrity is already suspect.
			job.Reason = safeCompilerReason(job.Reason)
			job.PauseReason = safeCompilerReason(job.PauseReason)
			switch job.State {
			case "queued":
				job.Recovery = "waiting for configured generation and capacity"
			case "retry_wait":
				job.Recovery = "automatic retry when due and capacity is available"
			case "staged":
				job.Recovery = "publish saved stage without inference"
			case "cancelled":
				job.Recovery = "explicit resume required; attempt budget is retained"
			case "failed":
				job.Recovery = "retained coverage gap; same-generation retry cannot reset attempts"
			}
			if job.PauseReason != "" {
				job.Recovery = "restore pinned configuration or resolve the pause, then explicitly resume"
			}
			result.Jobs = append(result.Jobs, job)
		}
		err = rows.Err()
		rows.Close()
		if len(result.Jobs) == limit {
			result.NextJobID = result.Jobs[len(result.Jobs)-1].JobID
		}
		return err
	})
	return result, err
}

func safeCompilerReason(reason string) string {
	switch reason {
	case "", "endpoint_unavailable", "worker_interrupted", "worker_shutdown", "caller_cancelled", "owner_cancelled", "attempts_exhausted", "invalid_source", "invalid_source_or_effect", "invalid_configuration", "invalid_or_missing_output", "staging_failed", "invalid_stage", "oversized_input", "configuration_unavailable", "activation_disabled", "resource_capacity":
		return reason
	default:
		return "unavailable_detail"
	}
}
