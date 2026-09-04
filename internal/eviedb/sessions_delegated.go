package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// CreateDelegatedSessionWithComposition creates the existing parent/child
// session shape through a narrow Kernel seam. It inherits the parent's stable
// Context Scope and pins its own immutable composition; it does not spawn or
// run a subagent.
func (s *Store) CreateDelegatedSessionWithComposition(
	ctx context.Context,
	parentID memory.SessionID,
	receipt composition.Receipt,
) (memory.Session, error) {
	encodedReceipt, err := composition.Marshal(receipt)
	if err != nil {
		return memory.Session{}, fmt.Errorf("validate delegated Composition Receipt: %w", err)
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return memory.Session{}, fmt.Errorf("generate delegated session ID: %w", err)
	}
	now := s.now().UTC()
	created := memory.Session{
		ID: memory.SessionID(id.String()), ParentSessionID: parentID,
		Status: memory.SessionActive, CreatedAt: now, UpdatedAt: now,
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var workspaceID, workspaceRevision, projectID, projectRoot sql.NullString
		var status string
		if err := conn.QueryRowContext(ctx, `
			SELECT workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot, status
			FROM sessions WHERE id = ?
		`, parentID).Scan(&workspaceID, &workspaceRevision, &projectID, &projectRoot, &status); errors.Is(err, sql.ErrNoRows) {
			return ErrSessionNotActive
		} else if err != nil {
			return fmt.Errorf("read delegated parent session: %w", err)
		}
		if status != string(memory.SessionActive) {
			return ErrSessionNotActive
		}
		if workspaceID.Valid {
			created.WorkspaceID = memory.WorkspaceID(workspaceID.String)
		}
		if workspaceRevision.Valid {
			created.WorkspaceRevisionSnapshot = memory.WorkspaceRevisionID(workspaceRevision.String)
		}
		if projectID.Valid {
			created.ProjectID = memory.ProjectID(projectID.String)
		}
		if projectRoot.Valid {
			created.ProjectRootSnapshot = projectRoot.String
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO sessions (
				id, workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot,
				parent_session_id, status, created_at, updated_at
			) VALUES (?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)
		`, created.ID, created.WorkspaceID, created.WorkspaceRevisionSnapshot, created.ProjectID,
			created.ProjectRootSnapshot, created.ParentSessionID, created.Status,
			created.CreatedAt.Format(time.RFC3339Nano), created.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert delegated session: %w", err)
		}
		if err := insertCompositionReceipt(ctx, conn, created.ID, encodedReceipt, now); err != nil {
			return fmt.Errorf("insert delegated Composition Receipt: %w", err)
		}
		return nil
	})
	if err != nil {
		return memory.Session{}, err
	}
	return created, nil
}
