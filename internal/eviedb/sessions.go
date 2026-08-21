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

func (s *Store) GetSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, project_root_snapshot, parent_session_id, status, created_at, updated_at FROM sessions WHERE id = ?
		`, id)
	session, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Session{}, fmt.Errorf("session %q not found: %w", id, err)
	}
	if err != nil {
		return memory.Session{}, fmt.Errorf("read session: %w", err)
	}
	return session, nil
}

func scanSession(scanner rowScanner) (memory.Session, error) {
	var (
		id, status, createdText, updatedText string
		projectID, rootSnapshot, parentID    sql.NullString
	)

	if err := scanner.Scan(
		&id,
		&projectID,
		&rootSnapshot,
		&parentID,
		&status,
		&createdText,
		&updatedText,
	); err != nil {
		return memory.Session{}, err
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return memory.Session{}, fmt.Errorf("parse session created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedText)
	if err != nil {
		return memory.Session{}, fmt.Errorf("parse session updated_at: %w", err)
	}

	session := memory.Session{
		ID:        memory.SessionID(id),
		Status:    memory.SessionStatus(status),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	if projectID.Valid {
		session.ProjectID = memory.ProjectID(projectID.String)
	}

	if rootSnapshot.Valid {
		session.ProjectRootSnapshot = rootSnapshot.String
	}

	if parentID.Valid {
		session.ParentSessionID = memory.SessionID(parentID.String)
	}
	return session, nil
}

func (s *Store) CreateProjectSession(ctx context.Context, projectID memory.ProjectID) (memory.Session, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return memory.Session{}, fmt.Errorf("generate session ID: %w", err)
	}

	now := time.Now().UTC()
	session := memory.Session{
		ID:        memory.SessionID(id.String()),
		Status:    memory.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	var storedProjectID string
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO sessions (
		id, project_id, project_root_snapshot, status, created_at, updated_at
		)
		SELECT ?, id, canonical_root, ?, ?, ?
		FROM projects
		WHERE id = ? AND archived = 0
		RETURNING project_id, project_root_snapshot
		`,
		session.ID,
		session.Status,
		session.CreatedAt.Format(time.RFC3339Nano),
		session.UpdatedAt.Format(time.RFC3339Nano),
		projectID,
	).Scan(&storedProjectID, &session.ProjectRootSnapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Session{}, fmt.Errorf("project %q is missing or archived: %w", projectID, err)
	}

	if err != nil {
		return memory.Session{}, fmt.Errorf("insert project session: %w", err)
	}

	session.ProjectID = memory.ProjectID(storedProjectID)
	return session, nil
}

func (s *Store) CreateGlobalSession(ctx context.Context) (memory.Session, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return memory.Session{}, fmt.Errorf("generate session ID: %w", err)
	}

	now := time.Now().UTC()
	session := memory.Session{
		ID:        memory.SessionID(id.String()),
		Status:    memory.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, status, created_at, updated_at) VALUES (?, ?, ?, ?)
		`,
		session.ID,
		session.Status,
		session.CreatedAt.Format(time.RFC3339Nano),
		session.UpdatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return memory.Session{}, fmt.Errorf("insert global session: %w", err)
	}

	return session, nil
}
