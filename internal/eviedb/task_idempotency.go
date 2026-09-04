package eviedb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

const (
	mutationAccepted          = "accepted"
	mutationInvalidInput      = "invalid_input"
	mutationNotFound          = "not_found"
	mutationRevisionConflict  = "revision_conflict"
	mutationInvalidTransition = "invalid_transition"
)

type mutationResult struct {
	RequestSHA256     string
	Operation         task.Operation
	TaskID            task.ID
	EventID           string
	OutcomeCode       string
	DiagnosticField   string
	PreviousRevision  *uint64
	ResultingRevision *uint64
	FromStatus        task.Status
	ToStatus          task.Status
}

func readMutationResult(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256 string,
) (mutationResult, bool, error) {
	claim, found, err := readIdempotencyClaim(ctx, conn, attribution, identitySHA256)
	if err != nil || !found {
		return mutationResult{}, found, err
	}
	if claim.Operation != task.OperationCreate && claim.Operation != task.OperationUpdate {
		return mutationResult{RequestSHA256: claim.RequestSHA256, Operation: claim.Operation}, true, nil
	}
	var (
		result                                       mutationResult
		taskID, eventID, field, fromStatus, toStatus sql.NullString
		previous, resulting                          sql.NullInt64
	)
	err = conn.QueryRowContext(ctx, `
		SELECT request_sha256, operation, task_id, event_id, outcome_code, diagnostic_field,
		       previous_revision, resulting_revision, from_status, to_status
		FROM task_mutation_results
		WHERE actor_id = ? AND session_id = ? AND identity_sha256 = ?
	`, attribution.ActorID, attribution.SessionID, identitySHA256).Scan(
		&result.RequestSHA256, &result.Operation, &taskID, &eventID, &result.OutcomeCode, &field,
		&previous, &resulting, &fromStatus, &toStatus,
	)
	if err != nil {
		return mutationResult{}, false, fmt.Errorf("read Task mutation result: %w", err)
	}
	result.TaskID = task.ID(taskID.String)
	result.EventID = eventID.String
	result.DiagnosticField = field.String
	result.FromStatus = task.Status(fromStatus.String)
	result.ToStatus = task.Status(toStatus.String)
	if previous.Valid {
		value := uint64(previous.Int64)
		result.PreviousRevision = &value
	}
	if resulting.Valid {
		value := uint64(resulting.Int64)
		result.ResultingRevision = &value
	}
	return result, true, nil
}

type idempotencyClaim struct {
	RequestSHA256 string
	Operation     task.Operation
}

func readIdempotencyClaim(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256 string,
) (idempotencyClaim, bool, error) {
	var claim idempotencyClaim
	err := conn.QueryRowContext(ctx, `
		SELECT request_sha256, operation FROM (
			SELECT request_sha256, operation
			FROM task_idempotency_claims
			WHERE actor_id = ? AND session_id = ? AND identity_sha256 = ?
			UNION ALL
			SELECT request_sha256, operation
			FROM task_coordination_results
			WHERE actor_id = ? AND session_id = ? AND identity_sha256 = ?
		) LIMIT 1
	`, attribution.ActorID, attribution.SessionID, identitySHA256,
		attribution.ActorID, attribution.SessionID, identitySHA256).Scan(&claim.RequestSHA256, &claim.Operation)
	if err == sql.ErrNoRows {
		return idempotencyClaim{}, false, nil
	}
	if err != nil {
		return idempotencyClaim{}, false, fmt.Errorf("read Task idempotency claim: %w", err)
	}
	return claim, true, nil
}

func insertIdempotencyClaim(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256, requestSHA256 string,
	operation task.Operation,
	recordedAt time.Time,
) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_idempotency_claims (
			actor_id, session_id, identity_sha256, request_sha256, operation, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, attribution.ActorID, attribution.SessionID, identitySHA256, requestSHA256, operation, formatTaskTime(recordedAt))
	if err != nil {
		return fmt.Errorf("insert Task idempotency claim: %w", err)
	}
	return nil
}

func insertMutationResult(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256, requestSHA256 string,
	result mutationResult,
	recordedAt time.Time,
) error {
	if err := insertIdempotencyClaim(ctx, conn, attribution, identitySHA256, requestSHA256, result.Operation, recordedAt); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_mutation_results (
			actor_id, session_id, run_id, identity_sha256, request_sha256, operation,
			task_id, event_id, outcome_code, diagnostic_field, previous_revision,
			resulting_revision, from_status, to_status, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)
	`, attribution.ActorID, attribution.SessionID, attribution.RunID, identitySHA256, requestSHA256, result.Operation,
		result.TaskID, result.EventID, result.OutcomeCode, result.DiagnosticField,
		nullableUint64(result.PreviousRevision), nullableUint64(result.ResultingRevision), result.FromStatus, result.ToStatus,
		formatTaskTime(recordedAt))
	if err != nil {
		return fmt.Errorf("insert Task mutation result: %w", err)
	}
	return nil
}

func linkTaskEventIdempotency(ctx context.Context, conn *sql.Conn, eventID, identitySHA256 string) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_event_idempotency (event_id, identity_sha256) VALUES (?, ?)
	`, eventID, identitySHA256)
	if err != nil {
		return fmt.Errorf("link Task event idempotency: %w", err)
	}
	return nil
}

func nullableUint64(value *uint64) any {
	if value == nil {
		return nil
	}
	return *value
}

func insertTaskRevision(ctx context.Context, conn *sql.Conn, value task.Task) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_revisions (
			task_id, revision, scope, title, description, priority, due_date, result_summary,
			status, created_at, updated_at
		) VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)
	`, value.ID, value.Revision, value.Scope, value.Title, value.Description, value.Priority, value.DueDate,
		value.ResultSummary, value.Status, formatTaskTime(value.CreatedAt), formatTaskTime(value.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert Task revision: %w", err)
	}
	return nil
}

func getTaskRevision(ctx context.Context, conn *sql.Conn, id task.ID, revision uint64) (task.Task, error) {
	access, err := taskAccessFromContext(ctx, conn)
	if err != nil {
		return task.Task{}, err
	}
	scopeSQL, scopeArgs := scopePlaceholders(access.scopes)
	args := []any{id, revision}
	args = append(args, scopeArgs...)
	return scanTask(conn.QueryRowContext(ctx, `
		SELECT r.task_id, COALESCE(h.parent_id, ''), h.root_id, h.sibling_order,
		       r.scope, r.title, r.description, r.priority, r.due_date, r.result_summary,
		       r.status, r.revision, r.created_at, r.updated_at
		FROM task_revisions r
		JOIN task_hierarchy h ON h.task_id = r.task_id
		WHERE r.task_id = ? AND r.revision = ? AND r.scope IN (`+scopeSQL+`)
	`, args...))
}

func insertIdempotencyConflict(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256, originalRequestSHA256, attemptedRequestSHA256 string,
	operation task.Operation,
	taskID task.ID,
	recordedAt time.Time,
) error {
	id, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("generate Task idempotency conflict ID: %w", err)
	}
	if operation == task.OperationDecompose {
		_, err = conn.ExecContext(ctx, `
			INSERT INTO task_decomposition_conflicts (
				id, actor_id, session_id, identity_sha256, original_request_sha256,
				attempted_request_sha256, parent_id, recorded_at
			) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)
		`, id.String(), attribution.ActorID, attribution.SessionID, identitySHA256, originalRequestSHA256,
			attemptedRequestSHA256, taskID, formatTaskTime(recordedAt))
		if err != nil {
			return fmt.Errorf("insert Task decomposition idempotency conflict: %w", err)
		}
		return nil
	}
	_, err = conn.ExecContext(ctx, `
		INSERT INTO task_idempotency_conflicts (
			id, actor_id, session_id, identity_sha256, original_request_sha256,
			attempted_request_sha256, operation, task_id, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)
	`, id.String(), attribution.ActorID, attribution.SessionID, identitySHA256, originalRequestSHA256,
		attemptedRequestSHA256, operation, taskID, formatTaskTime(recordedAt))
	if err != nil {
		return fmt.Errorf("insert Task idempotency conflict: %w", err)
	}
	return nil
}
