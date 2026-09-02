package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	if err := task.ValidateCreateInput(input); err != nil {
		return task.Task{}, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return task.Task{}, fmt.Errorf("generate task ID: %w", err)
	}
	now := s.now().UTC()
	created := task.Task{
		ID:          task.ID(id.String()),
		Scope:       task.ScopeGlobal,
		Title:       input.Title,
		Description: input.Description,
		Priority:    input.Priority,
		DueDate:     input.DueDate,
		Status:      task.StatusOpen,
		Revision:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (
			id, scope, title, description, priority, due_date,
			status, revision, created_at, updated_at
		) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''), ?, ?, ?, ?)
	`,
		created.ID, created.Scope, created.Title, created.Description, created.Priority, created.DueDate,
		created.Status, created.Revision, formatTaskTime(created.CreatedAt), formatTaskTime(created.UpdatedAt),
	); err != nil {
		return task.Task{}, fmt.Errorf("insert global task: %w", err)
	}
	return created, nil
}

func (s *Store) ListOpenGlobalTasks(ctx context.Context) ([]task.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, scope, title, description, priority, due_date,
		       status, revision, created_at, updated_at
		FROM tasks
		WHERE scope = ? AND status = ?
		ORDER BY created_at, id
	`, task.ScopeGlobal, task.StatusOpen)
	if err != nil {
		return nil, fmt.Errorf("list open global tasks: %w", err)
	}
	defer rows.Close()

	var tasks []task.Task
	for rows.Next() {
		value, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("list open global tasks: %w", err)
		}
		tasks = append(tasks, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list open global tasks: %w", err)
	}
	return tasks, nil
}

func (s *Store) GetGlobalTask(ctx context.Context, id task.ID) (task.Task, error) {
	if strings.TrimSpace(string(id)) == "" {
		return task.Task{}, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	value, err := scanTask(s.db.QueryRowContext(ctx, `
		SELECT id, scope, title, description, priority, due_date,
		       status, revision, created_at, updated_at
		FROM tasks
		WHERE id = ? AND scope = ?
	`, id, task.ScopeGlobal))
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, &task.NotFoundError{ID: id}
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("get global task: %w", err)
	}
	return value, nil
}

func scanTask(scanner rowScanner) (task.Task, error) {
	var (
		value                    task.Task
		description, dueDate     sql.NullString
		priority                 sql.NullInt64
		createdText, updatedText string
	)
	if err := scanner.Scan(
		&value.ID, &value.Scope, &value.Title, &description, &priority, &dueDate,
		&value.Status, &value.Revision, &createdText, &updatedText,
	); err != nil {
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

func formatTaskTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
