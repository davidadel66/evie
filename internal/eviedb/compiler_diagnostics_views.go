package eviedb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

func diagnosticJobs(ctx context.Context, tx *sql.Tx, a OwnerReviewContext, q memory.CompilerDiagnosticsQuery, limit int, c diagnosticCursor, out *memory.CompilerDiagnostics) error {
	rows, err := tx.QueryContext(ctx, `SELECT j.job_id,j.generation_id,j.session_id,j.first_sequence,j.last_sequence,j.state,j.reason,j.attempts,COALESCE(j.retry_at,0),s.pause_reason,s.lane,d.queued_at,d.published_at,d.publication_ns,COALESCE(json_array_length(j.request,'$.window.new_event_ids'),0),COALESCE(json_array_length(v.event_ids),0) FROM memory_compiler_jobs j JOIN memory_compiler_job_schedule s USING(job_id) LEFT JOIN memory_compiler_diagnostic_jobs d USING(job_id) LEFT JOIN memory_compiler_coverage v USING(job_id) WHERE j.destination=? AND j.session_id=? AND j.job_id>? ORDER BY j.job_id LIMIT ?`, a.scope, q.SessionID, c.Key, limit)
	if err != nil {
		return err
	}
	for rows.Next() {
		var job memory.CompilerDiagnosticJob
		job.Measurements = []memory.CompilerAttemptMeasurement{}
		if err = rows.Scan(&job.JobID, &job.GenerationID, &job.SessionID, &job.FirstSequence, &job.LastSequence, &job.State, &job.Reason, &job.Attempts, &job.RetryAt, &job.PauseReason, &job.Lane, &job.QueuedAtUnixMS, &job.PublishedAtUnixMS, &job.PublicationNanos, &job.SelectedNewEvents, &job.CompletedNewEvents); err != nil {
			rows.Close()
			return err
		}
		job.State = safeDiagnosticState(job.State)
		job.Reason = safeHistoryReason(job.Reason)
		job.PauseReason = safeHistoryReason(job.PauseReason)
		job.Recovery = diagnosticRecovery(job.State, job.PauseReason)
		out.Jobs = append(out.Jobs, job)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i := range out.Jobs {
		var terminalAt sql.NullInt64
		e := tx.QueryRowContext(ctx, `SELECT f.terminal_at FROM memory_compiler_diagnostic_foreground f JOIN memory_compiler_jobs j ON j.root_id=f.root_id AND j.session_id=f.session_id WHERE j.job_id=?`, out.Jobs[i].JobID).Scan(&terminalAt)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if terminalAt.Valid && out.Jobs[i].PublishedAtUnixMS != nil && *out.Jobs[i].PublishedAtUnixMS >= terminalAt.Int64 {
			freshness := (*out.Jobs[i].PublishedAtUnixMS - terminalAt.Int64) * 1000000
			out.Jobs[i].CandidateFreshnessNanos = &freshness
		}
		measurements, e := tx.QueryContext(ctx, `SELECT attempt,fence,claimed_at,queue_wait_ns,inference_ns,validation_ns,database_ns,outcome FROM memory_compiler_diagnostic_attempts WHERE job_id=? ORDER BY attempt LIMIT 5`, out.Jobs[i].JobID)
		if e != nil {
			return e
		}
		for measurements.Next() {
			var m memory.CompilerAttemptMeasurement
			if e = measurements.Scan(&m.Attempt, &m.Fence, &m.ClaimedAtUnixMS, &m.QueueWaitNanos, &m.InferenceNanos, &m.ValidationNanos, &m.DatabaseCompletionNanos, &m.ObservedOutcome); e != nil {
				measurements.Close()
				return e
			}
			m.ObservedOutcome = safeDiagnosticOutcome(m.ObservedOutcome)
			out.Jobs[i].Measurements = append(out.Jobs[i].Measurements, m)
		}
		e = measurements.Err()
		measurements.Close()
		if e != nil {
			return e
		}
	}
	if len(out.Jobs) == limit {
		c.Key = out.Jobs[len(out.Jobs)-1].JobID
		out.NextCursor = diagnosticNext(a, q, c)
	}
	return nil
}

func diagnosticCandidates(ctx context.Context, tx *sql.Tx, a OwnerReviewContext, q memory.CompilerDiagnosticsQuery, limit int, c diagnosticCursor, out *memory.CompilerDiagnostics) error {
	// Navigate jobs as well as candidates: a page visits at most two jobs and
	// 32 candidate rows, even if millions of old jobs produced no candidates.
	jobLimit := limit / 16
	if jobLimit < 1 {
		jobLimit = 1
	}
	rows, err := tx.QueryContext(ctx, `SELECT job_id,generation_id FROM memory_compiler_jobs WHERE destination=? AND session_id=? AND job_id>=? ORDER BY job_id LIMIT ?`, a.scope, q.SessionID, c.Key, jobLimit)
	if err != nil {
		return err
	}
	type job struct{ id, generation string }
	jobs := []job{}
	for rows.Next() {
		var j job
		if err = rows.Scan(&j.id, &j.generation); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Sequence is the last ordinal in Key; -1 starts the following job. A short
	// candidate page can still carry a cursor after an empty job.
	for _, j := range jobs {
		after := int64(-1)
		if j.id == c.Key {
			after = c.Sequence
		}
		r, e := tx.QueryContext(ctx, `SELECT c.candidate_id,c.ordinal,c.review_revision,c.review_state,COALESCE(c.equivalent_to,''),d.published_at,x.decided_at,
 MAX(COALESCE((SELECT MAX(revision) FROM memory_review_edit_revisions WHERE candidate_id=c.candidate_id),0),COALESCE((SELECT MAX(revision) FROM memory_review_identity_revisions WHERE candidate_id=c.candidate_id),0),COALESCE((SELECT MAX(revision) FROM memory_review_temporal_revisions WHERE candidate_id=c.candidate_id),0)),EXISTS(SELECT 1 FROM memory_review_edit_revisions WHERE candidate_id=c.candidate_id)
 FROM memory_compiler_candidates c LEFT JOIN memory_compiler_diagnostic_jobs d USING(job_id) LEFT JOIN memory_compiler_diagnostic_decisions x USING(candidate_id) WHERE c.job_id=? AND c.ordinal>? ORDER BY c.ordinal LIMIT ?`, j.id, after, limit-len(out.Candidates))
		if e != nil {
			return e
		}
		last := after
		for r.Next() {
			item := memory.CompilerDiagnosticCandidate{JobID: j.id, GenerationID: j.generation}
			if e = r.Scan(&item.Ref.ID, &last, &item.Ref.ReviewRevision, &item.ReviewState, &item.EquivalentTo, &item.PublishedAtUnixMS, &item.DecidedAtUnixMS, &item.Ref.InterpretationRevision, &item.Edited); e != nil {
				r.Close()
				return e
			}
			item.ReviewState = safeDiagnosticState(item.ReviewState)
			out.Candidates = append(out.Candidates, item)
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return e
		}
		c.Key = j.id
		c.Sequence = last
		if len(out.Candidates) == limit {
			out.NextCursor = diagnosticNext(a, q, c)
			return nil
		}
		c.Key = j.id + "\x00"
		c.Sequence = -1
	}
	if len(jobs) == jobLimit {
		out.NextCursor = diagnosticNext(a, q, c)
	}
	return nil
}

func diagnosticActivations(ctx context.Context, tx *sql.Tx, a OwnerReviewContext, owner memory.ScopeContext, q memory.CompilerDiagnosticsQuery, limit int, c diagnosticCursor, out *memory.CompilerDiagnostics) error {
	rows, err := tx.QueryContext(ctx, `SELECT activation_id,source_scope,source_session,destination,generation_id,revision,after_position,through_position,work_paused FROM (
 SELECT * FROM (SELECT * FROM memory_compiler_activations WHERE destination=?1 AND source_scope=?2 AND source_session='' AND activation_id>?3 ORDER BY activation_id LIMIT ?5)
 UNION ALL SELECT * FROM (SELECT * FROM memory_compiler_activations WHERE destination=?1 AND source_scope=?2 AND source_session=?4 AND activation_id>?3 ORDER BY activation_id LIMIT ?5)
 ) ORDER BY activation_id LIMIT ?5`, a.scope, scopeKeyForContext(owner), c.Key, q.SessionID, limit)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item memory.CompilerActivation
		if err = scanCompilerActivation(rows, &item); err != nil {
			rows.Close()
			return err
		}
		out.Activations = append(out.Activations, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(out.Activations) == limit {
		c.Key = out.Activations[len(out.Activations)-1].ID
		out.NextCursor = diagnosticNext(a, q, c)
	}
	return nil
}
func diagnosticHistory(ctx context.Context, tx *sql.Tx, a OwnerReviewContext, q memory.CompilerDiagnosticsQuery, limit int, c diagnosticCursor, out *memory.CompilerDiagnostics) error {
	if q.Cursor == "" {
		c.Sequence = -1
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.request_id,r.ordinal,h.generation_id,r.first_sequence,r.last_sequence,r.scanned_sequence,h.revision,h.cancelled FROM memory_compiler_history_ranges r JOIN memory_compiler_history_requests h USING(request_id) WHERE r.destination=? AND r.session_id=? AND r.request_id>=? AND (r.request_id>? OR r.ordinal>?) ORDER BY r.request_id,r.ordinal LIMIT ?`, a.scope, q.SessionID, c.Key, c.Key, c.Sequence, limit)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item memory.CompilerDiagnosticHistory
		if err = rows.Scan(&item.RequestID, &item.RangeIndex, &item.GenerationID, &item.FirstSequence, &item.LastSequence, &item.ScannedSequence, &item.Revision, &item.Cancelled); err != nil {
			rows.Close()
			return err
		}
		out.History = append(out.History, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(out.History) == limit {
		last := out.History[len(out.History)-1]
		c.Key = last.RequestID
		c.Sequence = int64(last.RangeIndex)
		out.NextCursor = diagnosticNext(a, q, c)
	}
	return nil
}
func diagnosticSelection(ctx context.Context, tx *sql.Tx, a OwnerReviewContext, owner memory.ScopeContext, q memory.CompilerDiagnosticsQuery, limit int, c diagnosticCursor, out *memory.CompilerDiagnostics) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM memory_compiler_generations WHERE generation_id=?`, q.GenerationID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrReviewInvalidRequest
		}
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.sequence,p.commit_position FROM events e LEFT JOIN memory_compiler_event_positions p ON p.event_id=e.id WHERE e.session_id=? AND e.sequence>? ORDER BY e.sequence LIMIT ?`, q.SessionID, c.Sequence, limit)
	if err != nil {
		return err
	}
	type entry struct {
		item     memory.CompilerDiagnosticSelection
		position sql.NullInt64
	}
	entries := []entry{}
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.item.EventID, &e.item.Sequence, &e.position); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		e.item.Membership = "outside_selection"
		var selected int
		err = tx.QueryRowContext(ctx, `SELECT 1 FROM memory_compiler_history_selection_refs WHERE generation_id=? AND destination=? AND session_id=? AND event_id=?`, q.GenerationID, a.scope, q.SessionID, e.item.EventID).Scan(&selected)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if selected == 1 {
			e.item.Membership = "selected_history"
		}
		if e.position.Valid {
			for _, sourceSession := range []memory.SessionID{"", q.SessionID} {
				var through sql.NullInt64
				err = tx.QueryRowContext(ctx, `SELECT through_position FROM memory_compiler_activations WHERE generation_id=? AND destination=? AND source_scope=? AND source_session=? AND (through_position IS NULL OR through_position>after_position) AND after_position<? ORDER BY after_position DESC LIMIT 1`, q.GenerationID, a.scope, scopeKeyForContext(owner), sourceSession, e.position.Int64).Scan(&through)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				if err == nil && (!through.Valid || e.position.Int64 <= through.Int64) {
					e.item.Membership = "selected_live"
					break
				}
			}
		}
		out.Selection = append(out.Selection, e.item)
	}
	if len(out.Selection) == limit {
		c.Sequence = out.Selection[len(out.Selection)-1].Sequence
		out.NextCursor = diagnosticNext(a, q, c)
	}
	return nil
}
func diagnosticForeground(ctx context.Context, tx *sql.Tx, a OwnerReviewContext, q memory.CompilerDiagnosticsQuery, limit int, c diagnosticCursor, out *memory.CompilerDiagnostics) error {
	rows, err := tx.QueryContext(ctx, `SELECT root_id,started_at,terminal_at,terminal_ns,finalized_at,finalization_ns,outcome FROM memory_compiler_diagnostic_foreground WHERE session_id=? AND root_id>? ORDER BY root_id LIMIT ?`, q.SessionID, c.Key, limit)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item memory.CompilerForegroundMeasurement
		if err = rows.Scan(&item.RootID, &item.StartedAtUnixMS, &item.TerminalCommittedAtUnixMS, &item.TerminalCommitNanos, &item.ResponseFinalizedAtUnixMS, &item.ResponseFinalizationNanos, &item.Outcome); err != nil {
			rows.Close()
			return err
		}
		item.Outcome = safeDiagnosticOutcome(item.Outcome)
		out.Foreground = append(out.Foreground, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(out.Foreground) == limit {
		c.Key = string(out.Foreground[len(out.Foreground)-1].RootID)
		out.NextCursor = diagnosticNext(a, q, c)
	}
	return nil
}
func safeDiagnosticState(state string) string {
	switch state {
	case "selected_unmaterialized", "deferred_live", "configuration_paused", "authorization_paused", "queued", "running", "retry_wait", "staged", "cancelled", "failed", "completed_candidates", "completed_empty", "excluded", "unresolved", "accepted", "rejected":
		return state
	}
	return "unavailable"
}
func safeDiagnosticOutcome(state string) string {
	switch state {
	case "incomplete", "completed", "failed", "cancelled", "stale", "success", "interrupted":
		return state
	}
	return "incomplete"
}
func diagnosticRecovery(state, pause string) string {
	if pause != "" {
		return "restore the pinned configuration or selected scope, then explicitly resume"
	}
	switch state {
	case "queued":
		return "waiting for configured generation and shared capacity"
	case "retry_wait":
		return "automatic retry when due; inspect the configured endpoint"
	case "staged":
		return "publish the saved stage without inference"
	case "cancelled":
		return "explicit resume required; attempt budget is retained"
	case "failed":
		return "retained coverage gap; same-generation retry cannot reset attempts"
	}
	return ""
}

func diagnosticUnits(ctx context.Context, tx *sql.Tx, a OwnerReviewContext, q memory.CompilerDiagnosticsQuery, limit int, c diagnosticCursor, out *memory.CompilerDiagnostics) error {
	rows, err := tx.QueryContext(ctx, `SELECT s.selection_id,s.generation_id,COALESCE(s.job_id,''),s.first_sequence,s.last_sequence,COALESCE(j.state,s.state),COALESCE(j.reason,s.reason),COALESCE(json_array_length(s.window,'$.new_event_ids'),0) FROM memory_compiler_selections s LEFT JOIN memory_compiler_jobs j ON j.job_id=s.job_id WHERE s.destination=? AND s.session_id=? AND s.selection_id>? ORDER BY s.selection_id LIMIT ?`, a.scope, q.SessionID, c.Key, limit)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item memory.CompilerDiagnosticUnit
		if err = rows.Scan(&item.SelectionID, &item.GenerationID, &item.JobID, &item.FirstSequence, &item.LastSequence, &item.State, &item.Reason, &item.SelectedNewEvents); err != nil {
			rows.Close()
			return err
		}
		item.State = safeDiagnosticState(item.State)
		item.Reason = safeHistoryReason(item.Reason)
		out.Selections = append(out.Selections, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(out.Selections) == limit {
		c.Key = out.Selections[len(out.Selections)-1].SelectionID
		out.NextCursor = diagnosticNext(a, q, c)
	}
	return nil
}
func diagnosticRoots(ctx context.Context, tx *sql.Tx, a OwnerReviewContext, q memory.CompilerDiagnosticsQuery, limit int, c diagnosticCursor, out *memory.CompilerDiagnostics) error {
	rows, err := tx.QueryContext(ctx, `SELECT r.activation_id,r.root_id,r.first_sequence,r.last_sequence,r.state,r.reason,r.selection_id,a.destination,COALESCE(j.state,''),COALESCE(j.reason,'') FROM (SELECT * FROM memory_compiler_activation_roots WHERE session_id=? AND activation_id>=? AND (activation_id>? OR root_id>?) ORDER BY activation_id,root_id LIMIT ?) r JOIN memory_compiler_activations a USING(activation_id) LEFT JOIN memory_compiler_selections s ON s.selection_id=r.selection_id LEFT JOIN memory_compiler_jobs j ON j.job_id=s.job_id ORDER BY r.activation_id,r.root_id`, q.SessionID, c.Key, c.Key, c.Second, limit)
	if err != nil {
		return err
	}
	visited := 0
	for rows.Next() {
		var item memory.CompilerDiagnosticRoot
		var destination, jobState, jobReason string
		if err = rows.Scan(&item.ActivationID, &item.RootID, &item.FirstSequence, &item.LastSequence, &item.State, &item.Reason, &item.SelectionID, &destination, &jobState, &jobReason); err != nil {
			rows.Close()
			return err
		}
		visited++
		c.Key = item.ActivationID
		c.Second = string(item.RootID)
		if destination != a.scope {
			continue
		}
		if item.State != "selected_unmaterialized" && item.State != "deferred_live" && jobState != "" {
			item.State, item.Reason = jobState, jobReason
		}
		item.State = safeDiagnosticState(item.State)
		item.Reason = safeHistoryReason(item.Reason)
		out.LiveRoots = append(out.LiveRoots, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if visited == limit {
		out.NextCursor = diagnosticNext(a, q, c)
	}
	return nil
}
