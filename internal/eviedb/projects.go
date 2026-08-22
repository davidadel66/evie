package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

var ErrProjectNotFound = errors.New("eviedb: project not found")

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) RegisterProject(ctx context.Context, displayName, root string) (memory.Project, error) {
	if strings.TrimSpace(displayName) == "" {
		return memory.Project{}, errors.New("project display name must not be empty")
	}

	canonicalRoot, err := memory.CanonicalProjectRoot(root)
	if err != nil {
		return memory.Project{}, err
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return memory.Project{}, fmt.Errorf("generate project ID: %w", err)
	}

	now := time.Now().UTC()
	project := memory.Project{
		ID:            memory.ProjectID(id.String()),
		DisplayName:   displayName,
		CanonicalRoot: canonicalRoot,
		Archived:      false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (
			id, display_name, canonical_root, archived, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`,
		project.ID,
		project.DisplayName,
		project.CanonicalRoot,
		project.Archived,
		project.CreatedAt.Format(time.RFC3339Nano),
		project.UpdatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return memory.Project{}, fmt.Errorf("insert project: %w", err)
	}
	return project, nil
}

func (s *Store) ListProjects(ctx context.Context, includeArchived bool) ([]memory.Project, error) {
	query := `
		SELECT id, display_name, canonical_root, archived, created_at, updated_at
		FROM projects
	`
	if !includeArchived {
		query += ` WHERE archived = 0`
	}
	query += ` ORDER BY created_at, id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	var projects []memory.Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read projects: %w", err)
	}
	return projects, nil
}

func (s *Store) FindActiveProjectByRoot(ctx context.Context, root string) (memory.Project, error) {
	canonicalRoot, err := memory.CanonicalProjectRoot(root)
	if err != nil {
		return memory.Project{}, err
	}

	project, err := scanProject(s.db.QueryRowContext(ctx, `
		SELECT id, display_name, canonical_root, archived, created_at, updated_at
		FROM projects
		WHERE canonical_root = ? AND archived = 0
	`, canonicalRoot))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Project{}, fmt.Errorf("%w: %s", ErrProjectNotFound, canonicalRoot)
	}
	if err != nil {
		return memory.Project{}, fmt.Errorf("find active project: %w", err)
	}
	return project, nil
}

func (s *Store) FindProjectByRoot(ctx context.Context, root string) (memory.Project, error) {
	canonicalRoot, err := memory.CanonicalProjectRoot(root)
	if err != nil {
		return memory.Project{}, err
	}

	project, err := scanProject(s.db.QueryRowContext(ctx, `
		SELECT id, display_name, canonical_root, archived, created_at, updated_at
		FROM projects
		WHERE canonical_root = ?
	`, canonicalRoot))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Project{}, fmt.Errorf("%w: %s", ErrProjectNotFound, canonicalRoot)
	}
	if err != nil {
		return memory.Project{}, fmt.Errorf("find project: %w", err)
	}
	return project, nil
}

func (s *Store) RelocateProject(ctx context.Context, id memory.ProjectID, root string) (memory.Project, error) {
	canonicalRoot, err := memory.CanonicalProjectRoot(root)
	if err != nil {
		return memory.Project{}, err
	}

	row := s.db.QueryRowContext(ctx, `
		UPDATE projects
		SET canonical_root = ?, updated_at = ?
		WHERE id = ?
		RETURNING id, display_name, canonical_root, archived, created_at, updated_at
	`, canonicalRoot, time.Now().UTC().Format(time.RFC3339Nano), id)

	project, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Project{}, fmt.Errorf("project %q not found: %w", id, err)
	}
	if err != nil {
		return memory.Project{}, fmt.Errorf("relocate project: %w", err)
	}
	return project, nil
}

func scanProject(scanner rowScanner) (memory.Project, error) {
	var (
		id, displayName, canonicalRoot string
		createdText, updatedText       string
		archived                       int
	)
	if err := scanner.Scan(&id, &displayName, &canonicalRoot, &archived, &createdText, &updatedText); err != nil {
		return memory.Project{}, err
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return memory.Project{}, fmt.Errorf("parse project created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedText)
	if err != nil {
		return memory.Project{}, fmt.Errorf("parse project updated_at: %w", err)
	}

	return memory.Project{
		ID:            memory.ProjectID(id),
		DisplayName:   displayName,
		CanonicalRoot: canonicalRoot,
		Archived:      archived == 1,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}
