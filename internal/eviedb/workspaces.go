package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

var ErrWorkspaceNotFound = errors.New("eviedb: workspace not found")

func (s *Store) RegisterWorkspace(ctx context.Context, displayName string) (memory.Workspace, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("generate Workspace ID: %w", err)
	}
	revisionID, err := uuid.NewRandom()
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("generate initial Workspace revision ID: %w", err)
	}
	now := s.now().UTC()
	displayName = memory.WorkspaceDisplayLabel(displayName, now)
	workspace := memory.Workspace{
		ID:                memory.WorkspaceID(id.String()),
		DisplayName:       displayName,
		State:             memory.WorkspaceActive,
		CurrentRevisionID: memory.WorkspaceRevisionID(revisionID.String()),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO workspaces (
			id, display_name, lifecycle_state, current_revision_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, workspace.ID, workspace.DisplayName, workspace.State, workspace.CurrentRevisionID,
		workspace.CreatedAt.Format(time.RFC3339Nano), workspace.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
		return memory.Workspace{}, fmt.Errorf("insert Workspace: %w", err)
	}
	return workspace, nil
}

func (s *Store) ListWorkspaces(ctx context.Context, includeArchived bool) ([]memory.Workspace, error) {
	query := `SELECT id, display_name, lifecycle_state, current_revision_id, created_at, updated_at FROM workspaces`
	if !includeArchived {
		query += ` WHERE lifecycle_state = 'active'`
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query Workspaces: %w", err)
	}
	defer rows.Close()
	var workspaces []memory.Workspace
	for rows.Next() {
		workspace, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Workspaces: %w", err)
	}
	return workspaces, nil
}

func (s *Store) RenameWorkspace(ctx context.Context, id memory.WorkspaceID, displayName string) (memory.Workspace, error) {
	now := s.now().UTC()
	displayName = memory.WorkspaceDisplayLabel(displayName, now)
	workspace, err := scanWorkspace(s.db.QueryRowContext(ctx, `
		UPDATE workspaces SET display_name = ?, updated_at = ? WHERE id = ?
		RETURNING id, display_name, lifecycle_state, current_revision_id, created_at, updated_at
	`, displayName, now.Format(time.RFC3339Nano), id))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Workspace{}, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
	}
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("rename Workspace: %w", err)
	}
	return workspace, nil
}

func (s *Store) ArchiveWorkspace(ctx context.Context, id memory.WorkspaceID) (memory.Workspace, error) {
	now := s.now().UTC()
	workspace, err := scanWorkspace(s.db.QueryRowContext(ctx, `
		UPDATE workspaces SET lifecycle_state = ?, updated_at = ? WHERE id = ?
		RETURNING id, display_name, lifecycle_state, current_revision_id, created_at, updated_at
	`, memory.WorkspaceArchived, now.Format(time.RFC3339Nano), id))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Workspace{}, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
	}
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("archive Workspace: %w", err)
	}
	return workspace, nil
}

func scanWorkspace(scanner rowScanner) (memory.Workspace, error) {
	var id, displayName, state, revisionID, createdText, updatedText string
	if err := scanner.Scan(&id, &displayName, &state, &revisionID, &createdText, &updatedText); err != nil {
		return memory.Workspace{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("parse Workspace created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedText)
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("parse Workspace updated_at: %w", err)
	}
	return memory.Workspace{
		ID: memory.WorkspaceID(id), DisplayName: displayName, State: memory.WorkspaceState(state),
		CurrentRevisionID: memory.WorkspaceRevisionID(revisionID), CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}
