package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

var _ task.Service = (*Store)(nil)

func (s *Store) CreateGlobalTask(ctx context.Context, input task.CreateInput) (task.Task, error) {
	if err := ctx.Err(); err != nil {
		return task.Task{}, err
	}
	if err := task.ValidateIdempotencyKey(input.IdempotencyKey); err != nil {
		return task.Task{}, err
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.Task{}, err
	}
	requestSHA256, err := canonicalCreateRequestSHA256(input)
	if err != nil {
		return task.Task{}, err
	}
	identitySHA256 := idempotencySHA256(input.IdempotencyKey)
	var created task.Task
	var businessErr error
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		prior, found, err := readMutationResult(ctx, conn, attribution, identitySHA256)
		if err != nil {
			return err
		}
		if found {
			if prior.RequestSHA256 != requestSHA256 || prior.Operation != task.OperationCreate {
				if err := insertIdempotencyConflict(ctx, conn, attribution, identitySHA256, prior.RequestSHA256,
					requestSHA256, task.OperationCreate, prior.TaskID, s.now().UTC()); err != nil {
					return err
				}
				businessErr = &task.IdempotencyConflictError{Operation: task.OperationCreate}
				return nil
			}
			if prior.OutcomeCode == mutationAccepted && prior.ResultingRevision != nil {
				created, err = getTaskRevision(ctx, conn, prior.TaskID, *prior.ResultingRevision)
				return err
			}
			businessErr = replayCreateError(input, prior)
			return nil
		}
		now := s.now().UTC()
		if validationErr := task.ValidateCreateInput(input); validationErr != nil {
			field := inputErrorField(validationErr)
			if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
				Operation: task.OperationCreate, OutcomeCode: mutationInvalidInput, DiagnosticField: field,
			}, now); err != nil {
				return err
			}
			businessErr = validationErr
			return nil
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return fmt.Errorf("generate task ID: %w", err)
		}
		created = task.Task{
			ID: task.ID(id.String()), Scope: task.ScopeGlobal,
			Title: input.Title, Description: input.Description, Priority: input.Priority, DueDate: input.DueDate,
			Status: task.StatusOpen, Revision: 1, CreatedAt: now, UpdatedAt: now,
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO tasks (
				id, scope, title, description, priority, due_date,
				status, revision, created_at, updated_at
			) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''), ?, ?, ?, ?)
		`, created.ID, created.Scope, created.Title, created.Description, created.Priority, created.DueDate,
			created.Status, created.Revision, formatTaskTime(created.CreatedAt), formatTaskTime(created.UpdatedAt)); err != nil {
			return fmt.Errorf("insert global task: %w", err)
		}
		eventID, err := appendTaskEvent(ctx, conn, task.Event{
			TaskID: created.ID, Operation: task.OperationCreate,
			ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
			RecordedAt: now, PreviousRevision: 0, ResultingRevision: 1, Outcome: task.MutationAccepted,
		})
		if err != nil {
			return err
		}
		if err := insertTaskRevision(ctx, conn, created); err != nil {
			return err
		}
		previous, resulting := uint64(0), uint64(1)
		return insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
			Operation: task.OperationCreate, TaskID: created.ID, EventID: eventID, OutcomeCode: mutationAccepted,
			PreviousRevision: &previous, ResultingRevision: &resulting,
		}, now)
	})
	if err != nil {
		return task.Task{}, err
	}
	if businessErr != nil {
		return task.Task{}, businessErr
	}
	return created, nil
}

func replayCreateError(input task.CreateInput, prior mutationResult) error {
	if prior.OutcomeCode == mutationInvalidInput {
		if err := task.ValidateCreateInput(input); err != nil {
			return err
		}
		return &task.InputError{Field: prior.DiagnosticField, Message: "original mutation was rejected"}
	}
	return fmt.Errorf("replay Task create outcome %q", prior.OutcomeCode)
}

func inputErrorField(err error) string {
	var inputErr *task.InputError
	if errors.As(err, &inputErr) {
		return inputErr.Field
	}
	return "mutation"
}

func (s *Store) ListOpenGlobalTasks(ctx context.Context) ([]task.Task, error) {
	return s.ListGlobalTasks(ctx, task.ListFilter{})
}

func (s *Store) ListGlobalTasks(ctx context.Context, filter task.ListFilter) ([]task.Task, error) {
	statuses, err := listStatuses(filter)
	if err != nil {
		return nil, err
	}
	placeholders := make([]string, len(statuses))
	arguments := make([]any, 0, len(statuses)+1)
	arguments = append(arguments, task.ScopeGlobal)
	for i, status := range statuses {
		placeholders[i] = "?"
		arguments = append(arguments, status)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, scope, title, description, priority, due_date,
		       status, revision, created_at, updated_at
		FROM tasks
		WHERE scope = ? AND status IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY created_at, id
	`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list global tasks: %w", err)
	}
	defer rows.Close()

	var values []task.Task
	for rows.Next() {
		value, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("list global tasks: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list global tasks: %w", err)
	}
	return values, nil
}

func listStatuses(filter task.ListFilter) ([]task.Status, error) {
	if len(filter.Statuses) == 0 {
		if filter.IncludeHistory {
			return []task.Status{task.StatusOpen, task.StatusInProgress, task.StatusBlocked, task.StatusCompleted, task.StatusCancelled}, nil
		}
		return []task.Status{task.StatusOpen}, nil
	}
	seen := make(map[task.Status]struct{}, len(filter.Statuses))
	statuses := make([]task.Status, 0, len(filter.Statuses))
	for _, status := range filter.Statuses {
		if !task.ValidStatus(status) {
			return nil, &task.InputError{Field: "statuses", Message: fmt.Sprintf("contains invalid status %q", status)}
		}
		if _, exists := seen[status]; exists {
			continue
		}
		seen[status] = struct{}{}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (s *Store) GetGlobalTask(ctx context.Context, id task.ID) (task.Task, error) {
	if strings.TrimSpace(string(id)) == "" {
		return task.Task{}, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	value, err := getGlobalTask(ctx, s.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, &task.NotFoundError{ID: id}
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("get global task: %w", err)
	}
	return value, nil
}

func (s *Store) UpdateGlobalTask(ctx context.Context, id task.ID, input task.UpdateInput) (task.Task, error) {
	if err := ctx.Err(); err != nil {
		return task.Task{}, err
	}
	if strings.TrimSpace(string(id)) == "" {
		return task.Task{}, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if err := task.ValidateIdempotencyKey(input.IdempotencyKey); err != nil {
		return task.Task{}, err
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.Task{}, err
	}
	requestSHA256, err := canonicalUpdateRequestSHA256(id, input)
	if err != nil {
		return task.Task{}, err
	}
	identitySHA256 := idempotencySHA256(input.IdempotencyKey)
	var result task.Task
	var businessErr error
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		prior, found, err := readMutationResult(ctx, conn, attribution, identitySHA256)
		if err != nil {
			return err
		}
		if found {
			if prior.RequestSHA256 != requestSHA256 || prior.Operation != task.OperationUpdate {
				conflictTaskID := prior.TaskID
				if conflictTaskID == "" {
					conflictTaskID = id
				}
				if err := insertIdempotencyConflict(ctx, conn, attribution, identitySHA256, prior.RequestSHA256,
					requestSHA256, task.OperationUpdate, conflictTaskID, s.now().UTC()); err != nil {
					return err
				}
				businessErr = &task.IdempotencyConflictError{Operation: task.OperationUpdate}
				return nil
			}
			if prior.OutcomeCode == mutationAccepted && prior.ResultingRevision != nil {
				result, err = getTaskRevision(ctx, conn, prior.TaskID, *prior.ResultingRevision)
				return err
			}
			businessErr = replayUpdateError(id, input, prior)
			return nil
		}
		current, err := getGlobalTask(ctx, conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			now := s.now().UTC()
			if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
				Operation: task.OperationUpdate, TaskID: id, OutcomeCode: mutationNotFound,
			}, now); err != nil {
				return err
			}
			businessErr = &task.NotFoundError{ID: id}
			return nil
		}
		if err != nil {
			return fmt.Errorf("get global task for update: %w", err)
		}
		result = current
		reject := func(cause error, code task.DiagnosticCode, outcomeCode, field string, from, to task.Status) error {
			businessErr = cause
			now := s.now().UTC()
			eventID, err := appendTaskEvent(ctx, conn, task.Event{
				TaskID: id, Operation: task.OperationUpdate,
				ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
				RecordedAt: now, PreviousRevision: current.Revision, ResultingRevision: current.Revision,
				Outcome: task.MutationRejected, DiagnosticCode: code,
			})
			if err != nil {
				return err
			}
			previous, resulting := current.Revision, current.Revision
			return insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
				Operation: task.OperationUpdate, TaskID: id, EventID: eventID, OutcomeCode: outcomeCode,
				DiagnosticField: field, PreviousRevision: &previous, ResultingRevision: &resulting,
				FromStatus: from, ToStatus: to,
			}, now)
		}
		if err := task.ValidateUpdateInput(input); err != nil {
			return reject(err, task.DiagnosticInvalidInput, mutationInvalidInput, inputErrorField(err), current.Status, "")
		}
		if input.ExpectedRevision != current.Revision {
			return reject(
				&task.ConflictError{ID: id, Expected: input.ExpectedRevision, Current: current.Revision},
				task.DiagnosticRevisionConflict, mutationRevisionConflict, "", current.Status, "",
			)
		}
		if input.Status != nil {
			if *input.Status == current.Status && !hasTaskMetadataPatch(input) {
				return reject(&task.TransitionError{From: current.Status, To: *input.Status},
					task.DiagnosticInvalidTransition, mutationInvalidTransition, "", current.Status, *input.Status)
			}
			if *input.Status != current.Status {
				if err := task.ValidateStatusTransition(current.Status, *input.Status); err != nil {
					return reject(err, task.DiagnosticInvalidTransition, mutationInvalidTransition, "", current.Status, *input.Status)
				}
			}
		}
		updated := applyTaskPatch(current, input)
		if taskStateEqual(current, updated) {
			return reject(&task.InputError{Field: "patch", Message: "must change task state"},
				task.DiagnosticInvalidInput, mutationInvalidInput, "patch", current.Status, "")
		}
		updated.Revision++
		updated.UpdatedAt = s.now().UTC()
		outcome, err := conn.ExecContext(ctx, `
			UPDATE tasks
			SET title = ?, description = NULLIF(?, ''), priority = NULLIF(?, 0), due_date = NULLIF(?, ''),
			    status = ?, revision = ?, updated_at = ?
			WHERE id = ? AND scope = ? AND revision = ?
		`, updated.Title, updated.Description, updated.Priority, updated.DueDate, updated.Status,
			updated.Revision, formatTaskTime(updated.UpdatedAt), id, task.ScopeGlobal, current.Revision)
		if err != nil {
			return fmt.Errorf("update global task: %w", err)
		}
		rows, err := outcome.RowsAffected()
		if err != nil || rows != 1 {
			return fmt.Errorf("update global task revision predicate affected %d rows: %w", rows, err)
		}
		eventID, err := appendTaskEvent(ctx, conn, task.Event{
			TaskID: id, Operation: task.OperationUpdate,
			ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
			RecordedAt: updated.UpdatedAt, PreviousRevision: current.Revision, ResultingRevision: updated.Revision,
			Outcome: task.MutationAccepted,
		})
		if err != nil {
			return err
		}
		if err := insertTaskRevision(ctx, conn, updated); err != nil {
			return err
		}
		previous, resulting := current.Revision, updated.Revision
		if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
			Operation: task.OperationUpdate, TaskID: id, EventID: eventID, OutcomeCode: mutationAccepted,
			PreviousRevision: &previous, ResultingRevision: &resulting,
			FromStatus: current.Status, ToStatus: updated.Status,
		}, updated.UpdatedAt); err != nil {
			return err
		}
		result = updated
		return nil
	})
	if err != nil {
		return task.Task{}, err
	}
	if businessErr != nil {
		return task.Task{}, businessErr
	}
	return result, nil
}

func replayUpdateError(id task.ID, input task.UpdateInput, prior mutationResult) error {
	switch prior.OutcomeCode {
	case mutationInvalidInput:
		if err := task.ValidateUpdateInput(input); err != nil {
			return err
		}
		if prior.DiagnosticField == "patch" {
			return &task.InputError{Field: "patch", Message: "must change task state"}
		}
		return &task.InputError{Field: prior.DiagnosticField, Message: "original mutation was rejected"}
	case mutationNotFound:
		return &task.NotFoundError{ID: id}
	case mutationRevisionConflict:
		current := uint64(0)
		if prior.ResultingRevision != nil {
			current = *prior.ResultingRevision
		}
		return &task.ConflictError{ID: id, Expected: input.ExpectedRevision, Current: current}
	case mutationInvalidTransition:
		return &task.TransitionError{From: prior.FromStatus, To: prior.ToStatus}
	default:
		return fmt.Errorf("replay Task update outcome %q", prior.OutcomeCode)
	}
}

func hasTaskMetadataPatch(input task.UpdateInput) bool {
	return input.Title != nil || input.Description != nil || input.Priority != nil || input.DueDate != nil
}

func applyTaskPatch(current task.Task, input task.UpdateInput) task.Task {
	updated := current
	if input.Title != nil {
		updated.Title = *input.Title
	}
	if input.Description != nil {
		updated.Description = *input.Description
	}
	if input.Priority != nil {
		updated.Priority = *input.Priority
	}
	if input.DueDate != nil {
		updated.DueDate = *input.DueDate
	}
	if input.Status != nil {
		updated.Status = *input.Status
	}
	return updated
}

func taskStateEqual(left, right task.Task) bool {
	left.Revision, right.Revision = 0, 0
	left.UpdatedAt, right.UpdatedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(left, right)
}

func (s *Store) ListTaskEvents(ctx context.Context, id task.ID) ([]task.Event, error) {
	if strings.TrimSpace(string(id)) == "" {
		return nil, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.task_id, e.sequence, e.operation, e.actor_id, e.session_id, e.run_id, e.recorded_at,
		       e.previous_revision, e.resulting_revision, e.outcome, e.diagnostic_code,
		       COALESCE(r.identity_sha256, '')
		FROM task_events e
		LEFT JOIN task_mutation_results r ON r.event_id = e.id
		WHERE e.task_id = ?
		ORDER BY e.sequence
	`, id)
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	defer rows.Close()
	var events []task.Event
	for rows.Next() {
		var event task.Event
		var recorded string
		var diagnostic sql.NullString
		if err := rows.Scan(&event.ID, &event.TaskID, &event.Sequence, &event.Operation, &event.ActorID, &event.SessionID,
			&event.RunID, &recorded, &event.PreviousRevision, &event.ResultingRevision, &event.Outcome, &diagnostic,
			&event.IdempotencySHA256); err != nil {
			return nil, fmt.Errorf("list task events: %w", err)
		}
		event.DiagnosticCode = task.DiagnosticCode(diagnostic.String)
		event.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded)
		if err != nil {
			return nil, fmt.Errorf("parse task event recorded_at: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	return events, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getGlobalTask(ctx context.Context, source queryRower, id task.ID) (task.Task, error) {
	return scanTask(source.QueryRowContext(ctx, `
		SELECT id, scope, title, description, priority, due_date,
		       status, revision, created_at, updated_at
		FROM tasks
		WHERE id = ? AND scope = ?
	`, id, task.ScopeGlobal))
}

func appendTaskEvent(ctx context.Context, conn *sql.Conn, event task.Event) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate task event ID: %w", err)
	}
	event.ID = id.String()
	if err := conn.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sequence), 0) + 1 FROM task_events WHERE task_id = ?`, event.TaskID,
	).Scan(&event.Sequence); err != nil {
		return "", fmt.Errorf("allocate task event sequence: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO task_events (
			id, task_id, sequence, operation, actor_id, session_id, run_id, recorded_at,
			previous_revision, resulting_revision, outcome, diagnostic_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
	`, event.ID, event.TaskID, event.Sequence, event.Operation, event.ActorID, event.SessionID, event.RunID,
		formatTaskTime(event.RecordedAt), event.PreviousRevision, event.ResultingRevision, event.Outcome, event.DiagnosticCode); err != nil {
		return "", fmt.Errorf("append task event: %w", err)
	}
	return event.ID, nil
}

func scanTask(scanner rowScanner) (task.Task, error) {
	var (
		value                    task.Task
		description, dueDate     sql.NullString
		priority                 sql.NullInt64
		createdText, updatedText string
	)
	if err := scanner.Scan(&value.ID, &value.Scope, &value.Title, &description, &priority, &dueDate,
		&value.Status, &value.Revision, &createdText, &updatedText); err != nil {
		return task.Task{}, err
	}
	value.Description = description.String
	value.Priority = int(priority.Int64)
	value.DueDate = dueDate.String
	var err error
	value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return task.Task{}, fmt.Errorf("parse task created_at: %w", err)
	}
	value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedText)
	if err != nil {
		return task.Task{}, fmt.Errorf("parse task updated_at: %w", err)
	}
	return value, nil
}

func formatTaskTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
