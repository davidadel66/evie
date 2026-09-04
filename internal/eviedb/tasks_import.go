package eviedb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

const legacyTodoMigrationID = "legacy-todo:DayToDay:v1"

type LegacyTodoImportedTask struct {
	Task        task.Task
	SourceList  string
	LegacyID    int
	MigrationID string
}

type LegacyTodoImportResult struct {
	MigrationID string
	Applied     bool
	Items       []LegacyTodoImportedTask
}

type legacyTodoList struct {
	Tasks  []legacyTodoTask
	NextID int
}

type legacyTodoTask struct {
	ID          int
	Title       string
	Description string
	CreatedAt   time.Time
	Priority    int
	Status      bool
	DueDate     time.Time
}

// DefaultLegacyTodoPath resolves the one supported source list. The importer
// has no arbitrary-list discovery or runtime fallback to the JSON store.
func DefaultLegacyTodoPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve legacy Todo home: %w", err)
	}
	return filepath.Join(home, ".todo", "DayToDay.json"), nil
}

// ImportDefaultLegacyTodoList runs the startup-owned migration against the
// single historical default list location.
func (s *Store) ImportDefaultLegacyTodoList(ctx context.Context) (LegacyTodoImportResult, error) {
	path, err := DefaultLegacyTodoPath()
	if err != nil {
		return LegacyTodoImportResult{}, err
	}
	return s.importLegacyTodoList(ctx, path)
}

// importLegacyTodoList copies the default legacy list into Global Tasks once.
// The explicit path is a deterministic test/startup seam; only DayToDay.json
// is accepted, and the source is opened read-only and never changed.
func (s *Store) importLegacyTodoList(ctx context.Context, path string) (LegacyTodoImportResult, error) {
	if err := ctx.Err(); err != nil {
		return LegacyTodoImportResult{}, err
	}
	if filepath.Base(path) != "DayToDay.json" {
		return LegacyTodoImportResult{}, &task.InputError{Field: "legacy_todo_path", Message: "must name DayToDay.json"}
	}
	prior, found, err := readLegacyTodoImport(ctx, s.db)
	if err != nil {
		return LegacyTodoImportResult{}, err
	}
	if found {
		return prior, nil
	}

	source, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return LegacyTodoImportResult{MigrationID: legacyTodoMigrationID}, nil
	}
	if err != nil {
		return LegacyTodoImportResult{}, fmt.Errorf("read legacy Todo %q: %w", path, err)
	}
	legacy, err := decodeLegacyTodoList(source)
	if err != nil {
		return LegacyTodoImportResult{}, fmt.Errorf("import legacy Todo %q: %w", path, err)
	}
	if err := validateLegacyTodoList(legacy); err != nil {
		return LegacyTodoImportResult{}, fmt.Errorf("import legacy Todo %q: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return LegacyTodoImportResult{}, err
	}

	digest := sha256.Sum256(source)
	sourceSHA256 := hex.EncodeToString(digest[:])
	result := LegacyTodoImportResult{MigrationID: legacyTodoMigrationID}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		prior, found, err := readLegacyTodoImport(ctx, conn)
		if err != nil {
			return err
		}
		if found {
			result = prior
			return nil
		}
		now := s.now().UTC()
		items := make([]LegacyTodoImportedTask, 0, len(legacy.Tasks))
		for _, legacyTask := range legacy.Tasks {
			if err := ctx.Err(); err != nil {
				return err
			}
			item, err := importLegacyTodoTask(ctx, conn, legacyTask, now)
			if err != nil {
				return err
			}
			items = append(items, item)
			if s.afterLegacyTodoItem != nil {
				s.afterLegacyTodoItem()
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO task_legacy_todo_imports (
				migration_id, source_list, source_sha256, item_count, applied_at
			) VALUES (?, 'DayToDay', ?, ?, ?)
		`, legacyTodoMigrationID, sourceSHA256, len(items), formatTaskTime(now)); err != nil {
			return fmt.Errorf("record legacy Todo migration: %w", err)
		}
		for sourceIndex, item := range items {
			if _, err := conn.ExecContext(ctx, `
				INSERT INTO task_legacy_todo_provenance (
					migration_id, source_list, legacy_id, source_index, task_id
				) VALUES (?, ?, ?, ?, ?)
			`, item.MigrationID, item.SourceList, item.LegacyID, sourceIndex, item.Task.ID); err != nil {
				return fmt.Errorf("record legacy Todo provenance for ID %d: %w", item.LegacyID, err)
			}
		}
		result.Applied = true
		result.Items = items
		return nil
	})
	if err != nil {
		return LegacyTodoImportResult{}, err
	}
	return result, nil
}

func decodeLegacyTodoList(source []byte) (legacyTodoList, error) {
	if !utf8.Valid(source) {
		return legacyTodoList{}, errors.New("decode legacy Todo: input is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	var legacy legacyTodoList
	if err := decoder.Decode(&legacy); err != nil {
		return legacyTodoList{}, fmt.Errorf("decode legacy Todo: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return legacyTodoList{}, fmt.Errorf("decode legacy Todo: %w", err)
	}
	return legacy, nil
}

func validateLegacyTodoList(legacy legacyTodoList) error {
	seen := make(map[int]struct{}, len(legacy.Tasks))
	for index, legacyTask := range legacy.Tasks {
		if legacyTask.ID < 0 {
			return &task.InputError{Field: fmt.Sprintf("Tasks[%d].ID", index), Message: "must not be negative"}
		}
		if _, found := seen[legacyTask.ID]; found {
			return &task.InputError{Field: fmt.Sprintf("Tasks[%d].ID", index), Message: "must be unique"}
		}
		seen[legacyTask.ID] = struct{}{}
		dueDate := ""
		if !legacyTask.DueDate.IsZero() {
			dueDate = legacyTask.DueDate.Format("2006-01-02")
		}
		if err := task.ValidateCreateInput(task.CreateInput{
			Title: legacyTask.Title, Description: legacyTask.Description,
			Priority: legacyTask.Priority, DueDate: dueDate, IdempotencyKey: "legacy-import-validation",
		}); err != nil {
			return fmt.Errorf("legacy Todo ID %d: %w", legacyTask.ID, err)
		}
	}
	return nil
}

func importLegacyTodoTask(
	ctx context.Context,
	conn *sql.Conn,
	legacy legacyTodoTask,
	importedAt time.Time,
) (LegacyTodoImportedTask, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return LegacyTodoImportedTask{}, fmt.Errorf("generate imported Task ID: %w", err)
	}
	status := task.StatusOpen
	if legacy.Status {
		status = task.StatusCompleted
	}
	dueDate := ""
	if !legacy.DueDate.IsZero() {
		dueDate = legacy.DueDate.Format("2006-01-02")
	}
	value := task.Task{
		ID: task.ID(id.String()), RootID: task.ID(id.String()), Scope: task.ScopeGlobal,
		Title: legacy.Title, Description: legacy.Description, Priority: legacy.Priority,
		DueDate: dueDate, Status: status, Revision: 1, CreatedAt: importedAt, UpdatedAt: importedAt,
	}
	if err := insertTaskState(ctx, conn, value); err != nil {
		return LegacyTodoImportedTask{}, fmt.Errorf("import legacy Todo ID %d: %w", legacy.ID, err)
	}
	if err := insertTaskHierarchy(ctx, conn, value); err != nil {
		return LegacyTodoImportedTask{}, fmt.Errorf("import legacy Todo ID %d: %w", legacy.ID, err)
	}
	eventID, err := appendTaskEvent(ctx, conn, task.Event{
		TaskID: value.ID, Operation: task.OperationCreate, ActorID: "kernel",
		SessionID: legacyTodoMigrationID, RunID: legacyTodoMigrationID,
		RecordedAt: importedAt, PreviousRevision: 0, ResultingRevision: 1, Outcome: task.MutationAccepted,
	})
	if err != nil {
		return LegacyTodoImportedTask{}, fmt.Errorf("audit legacy Todo ID %d: %w", legacy.ID, err)
	}
	identity := task.IdempotencySHA256(task.IdempotencyKey(legacyTodoMigrationID + ":" + strconv.Itoa(legacy.ID)))
	if err := linkTaskEventIdempotency(ctx, conn, eventID, identity); err != nil {
		return LegacyTodoImportedTask{}, err
	}
	if err := insertTaskRevision(ctx, conn, value); err != nil {
		return LegacyTodoImportedTask{}, err
	}
	return LegacyTodoImportedTask{
		Task: value, SourceList: "DayToDay", LegacyID: legacy.ID, MigrationID: legacyTodoMigrationID,
	}, nil
}

type legacyTodoImportQueryer interface {
	queryRower
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readLegacyTodoImport(ctx context.Context, source legacyTodoImportQueryer) (LegacyTodoImportResult, bool, error) {
	var migrationID string
	var itemCount int
	err := source.QueryRowContext(ctx, `
		SELECT migration_id, item_count
		FROM task_legacy_todo_imports
		WHERE migration_id = ?
	`, legacyTodoMigrationID).Scan(&migrationID, &itemCount)
	if errors.Is(err, sql.ErrNoRows) {
		return LegacyTodoImportResult{MigrationID: legacyTodoMigrationID}, false, nil
	}
	if err != nil {
		return LegacyTodoImportResult{}, false, fmt.Errorf("read legacy Todo migration: %w", err)
	}
	rows, err := source.QueryContext(ctx, `
		SELECT p.source_list, p.legacy_id,
		       t.id, COALESCE(h.parent_id, ''), h.root_id, h.sibling_order,
		       t.scope, t.title, t.description, t.priority, t.due_date, t.result_summary,
		       t.status, t.revision, t.created_at, t.updated_at
		FROM task_legacy_todo_provenance p
		JOIN tasks t ON t.id = p.task_id
		JOIN task_hierarchy h ON h.task_id = t.id
		WHERE p.migration_id = ?
		ORDER BY p.source_index
	`, legacyTodoMigrationID)
	if err != nil {
		return LegacyTodoImportResult{}, false, fmt.Errorf("read imported legacy Todo Tasks: %w", err)
	}
	defer rows.Close()
	var items []LegacyTodoImportedTask
	for rows.Next() {
		var sourceList string
		var legacyID int
		value, err := scanTaskWithPrefix(rows, &sourceList, &legacyID)
		if err != nil {
			return LegacyTodoImportResult{}, false, err
		}
		items = append(items, LegacyTodoImportedTask{
			Task: value, SourceList: sourceList, LegacyID: legacyID, MigrationID: migrationID,
		})
	}
	if err := rows.Err(); err != nil {
		return LegacyTodoImportResult{}, false, fmt.Errorf("read imported legacy Todo Tasks: %w", err)
	}
	if len(items) != itemCount {
		return LegacyTodoImportResult{}, false, fmt.Errorf(
			"read imported legacy Todo Tasks: migration records %d items but found %d provenance rows",
			itemCount, len(items),
		)
	}
	return LegacyTodoImportResult{MigrationID: migrationID, Items: items}, true, nil
}

func scanTaskWithPrefix(rows *sql.Rows, sourceList *string, legacyID *int) (task.Task, error) {
	var (
		value                               task.Task
		description, dueDate, resultSummary sql.NullString
		priority                            sql.NullInt64
		createdText, updatedText            string
	)
	if err := rows.Scan(sourceList, legacyID, &value.ID, &value.ParentID, &value.RootID, &value.SiblingOrder,
		&value.Scope, &value.Title, &description, &priority, &dueDate, &resultSummary,
		&value.Status, &value.Revision, &createdText, &updatedText); err != nil {
		return task.Task{}, fmt.Errorf("scan imported legacy Todo Task: %w", err)
	}
	value.Description = description.String
	value.Priority = int(priority.Int64)
	value.DueDate = dueDate.String
	value.ResultSummary = resultSummary.String
	var err error
	value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return task.Task{}, fmt.Errorf("parse imported legacy Todo created_at: %w", err)
	}
	value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedText)
	if err != nil {
		return task.Task{}, fmt.Errorf("parse imported legacy Todo updated_at: %w", err)
	}
	return value, nil
}
