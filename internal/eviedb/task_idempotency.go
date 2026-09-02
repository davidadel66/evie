package eviedb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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

func idempotencySHA256(key task.IdempotencyKey) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func canonicalCreateRequestSHA256(input task.CreateInput) (string, error) {
	return canonicalMutationSHA256(struct {
		Version     int        `json:"version"`
		Operation   string     `json:"operation"`
		Scope       task.Scope `json:"scope"`
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Priority    int        `json:"priority"`
		DueDate     string     `json:"due_date"`
	}{1, string(task.OperationCreate), task.ScopeGlobal, input.Title, input.Description, input.Priority, input.DueDate})
}

func canonicalUpdateRequestSHA256(id task.ID, input task.UpdateInput) (string, error) {
	title, titleSet := pointerValue(input.Title)
	description, descriptionSet := pointerValue(input.Description)
	priority, prioritySet := pointerValue(input.Priority)
	dueDate, dueDateSet := pointerValue(input.DueDate)
	status, statusSet := pointerValue(input.Status)
	return canonicalMutationSHA256(struct {
		Version          int         `json:"version"`
		Operation        string      `json:"operation"`
		Scope            task.Scope  `json:"scope"`
		TaskID           task.ID     `json:"task_id"`
		ExpectedRevision uint64      `json:"expected_revision"`
		TitleSet         bool        `json:"title_set"`
		Title            string      `json:"title"`
		DescriptionSet   bool        `json:"description_set"`
		Description      string      `json:"description"`
		PrioritySet      bool        `json:"priority_set"`
		Priority         int         `json:"priority"`
		DueDateSet       bool        `json:"due_date_set"`
		DueDate          string      `json:"due_date"`
		StatusSet        bool        `json:"status_set"`
		Status           task.Status `json:"status"`
	}{
		1, string(task.OperationUpdate), task.ScopeGlobal, id, input.ExpectedRevision,
		titleSet, title, descriptionSet, description, prioritySet, priority,
		dueDateSet, dueDate, statusSet, status,
	})
}

func pointerValue[T any](value *T) (T, bool) {
	if value == nil {
		var zero T
		return zero, false
	}
	return *value, true
}

func canonicalMutationSHA256(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode canonical Task mutation: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func readMutationResult(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256 string,
) (mutationResult, bool, error) {
	var (
		result                                       mutationResult
		taskID, eventID, field, fromStatus, toStatus sql.NullString
		previous, resulting                          sql.NullInt64
	)
	err := conn.QueryRowContext(ctx, `
		SELECT request_sha256, operation, task_id, event_id, outcome_code, diagnostic_field,
		       previous_revision, resulting_revision, from_status, to_status
		FROM task_mutation_results
		WHERE actor_id = ? AND session_id = ? AND identity_sha256 = ?
	`, attribution.ActorID, attribution.SessionID, identitySHA256).Scan(
		&result.RequestSHA256, &result.Operation, &taskID, &eventID, &result.OutcomeCode, &field,
		&previous, &resulting, &fromStatus, &toStatus,
	)
	if err == sql.ErrNoRows {
		return mutationResult{}, false, nil
	}
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

func insertMutationResult(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256, requestSHA256 string,
	result mutationResult,
	recordedAt time.Time,
) error {
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

func nullableUint64(value *uint64) any {
	if value == nil {
		return nil
	}
	return *value
}

func insertTaskRevision(ctx context.Context, conn *sql.Conn, value task.Task) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_revisions (
			task_id, revision, scope, title, description, priority, due_date,
			status, created_at, updated_at
		) VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''), ?, ?, ?)
	`, value.ID, value.Revision, value.Scope, value.Title, value.Description, value.Priority, value.DueDate,
		value.Status, formatTaskTime(value.CreatedAt), formatTaskTime(value.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert Task revision: %w", err)
	}
	return nil
}

func getTaskRevision(ctx context.Context, conn *sql.Conn, id task.ID, revision uint64) (task.Task, error) {
	return scanTask(conn.QueryRowContext(ctx, `
		SELECT task_id, scope, title, description, priority, due_date,
		       status, revision, created_at, updated_at
		FROM task_revisions
		WHERE task_id = ? AND revision = ?
	`, id, revision))
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
