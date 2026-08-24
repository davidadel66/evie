package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

var (
	ErrChooserStateChanged = errors.New("eviedb: chooser state changed")
	ErrProjectNotActive    = errors.New("eviedb: project is missing or archived")
	ErrSessionNotActive    = errors.New("eviedb: session is missing or inactive")
)

func (s *Store) GetSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, project_root_snapshot, parent_session_id, COALESCE(title, ''), status, created_at, updated_at FROM sessions WHERE id = ?
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

func (s *Store) GetActiveSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	session, err := scanSession(s.db.QueryRowContext(ctx, `
		SELECT id, project_id, project_root_snapshot, parent_session_id, COALESCE(title, ''), status, created_at, updated_at
		FROM sessions
		WHERE id = ? AND status = ?
	`, id, memory.SessionActive))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Session{}, fmt.Errorf("%w: session %q", ErrSessionNotActive, id)
	}
	if err != nil {
		return memory.Session{}, fmt.Errorf("read active session: %w", err)
	}
	return session, nil
}

// GetActiveSessionForChooser serializes the rendered cwd-owner expectation
// with the active-session read. Registration after this commit is later state,
// not a scope silently adopted by this selection.
func (s *Store) GetActiveSessionForChooser(
	ctx context.Context,
	id memory.SessionID,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	var session memory.Session
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := requireChooserCWDProjectOwner(ctx, conn, cwdRoot, expectedCWDProjectID); err != nil {
			return err
		}
		var err error
		session, err = scanSession(conn.QueryRowContext(ctx, `
			SELECT id, project_id, project_root_snapshot, parent_session_id,
			       COALESCE(title, ''), status, created_at, updated_at
			FROM sessions
			WHERE id = ? AND status = ?
		`, id, memory.SessionActive))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: session %q", ErrSessionNotActive, id)
		}
		if err != nil {
			return fmt.Errorf("read active session: %w", err)
		}
		return nil
	})
	return session, err
}

// ListActiveSessions excludes closed sessions at the persistence boundary and
// returns one globally ordered stream for consumers to partition without
// re-sorting. Activity is the parsed timestamp attached to the greatest accepted
// sequence, with creation time as the empty-history fallback.
func (s *Store) ListActiveSessions(ctx context.Context) ([]memory.SessionListing, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sessions.id, sessions.project_id, sessions.project_root_snapshot,
		       sessions.parent_session_id, COALESCE(sessions.title, ''), sessions.status,
		       sessions.created_at, sessions.updated_at,
		       (
		           SELECT events.recorded_at
		           FROM events
		           WHERE events.session_id = sessions.id
		           ORDER BY events.sequence DESC
		           LIMIT 1
		       )
		FROM sessions
		WHERE sessions.status = ?
	`, memory.SessionActive)
	if err != nil {
		return nil, fmt.Errorf("query active sessions: %w", err)
	}
	defer rows.Close()

	var listings []memory.SessionListing
	for rows.Next() {
		var activityText sql.NullString
		session, err := scanSessionWithActivity(rows, &activityText)
		if err != nil {
			return nil, err
		}
		activityAt := session.CreatedAt
		if activityText.Valid {
			activityAt, err = time.Parse(time.RFC3339Nano, activityText.String)
			if err != nil {
				return nil, fmt.Errorf("parse session activity timestamp: %w", err)
			}
		}
		listings = append(listings, memory.SessionListing{Session: session, ActivityAt: activityAt})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read active sessions: %w", err)
	}
	sort.Slice(listings, func(i, j int) bool {
		if !listings[i].ActivityAt.Equal(listings[j].ActivityAt) {
			return listings[i].ActivityAt.After(listings[j].ActivityAt)
		}
		return listings[i].ID < listings[j].ID
	})
	return listings, nil
}

func scanSessionWithActivity(scanner rowScanner, activity *sql.NullString) (memory.Session, error) {
	var (
		id, title, status, createdText, updatedText string
		projectID, rootSnapshot, parentID           sql.NullString
	)
	if err := scanner.Scan(
		&id, &projectID, &rootSnapshot, &parentID, &title, &status,
		&createdText, &updatedText, activity,
	); err != nil {
		return memory.Session{}, err
	}
	return sessionFromScanned(id, projectID, rootSnapshot, parentID, title, status, createdText, updatedText)
}

func scanSession(scanner rowScanner) (memory.Session, error) {
	var (
		id, title, status, createdText, updatedText string
		projectID, rootSnapshot, parentID           sql.NullString
	)

	if err := scanner.Scan(
		&id,
		&projectID,
		&rootSnapshot,
		&parentID,
		&title,
		&status,
		&createdText,
		&updatedText,
	); err != nil {
		return memory.Session{}, err
	}

	return sessionFromScanned(id, projectID, rootSnapshot, parentID, title, status, createdText, updatedText)
}

func sessionFromScanned(
	id string,
	projectID, rootSnapshot, parentID sql.NullString,
	title, status, createdText, updatedText string,
) (memory.Session, error) {
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
		Title:     title,
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
	return s.createProjectSession(ctx, projectID, "", "", "", false)
}

// CreateProjectSessionForChooser atomically requires the rendered project root,
// active state, and rendered cwd owner before freezing the session scope.
func (s *Store) CreateProjectSessionForChooser(
	ctx context.Context,
	projectID memory.ProjectID,
	expectedProjectRoot string,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	return s.createProjectSession(ctx, projectID, expectedProjectRoot, cwdRoot, expectedCWDProjectID, true)
}

func (s *Store) createProjectSession(
	ctx context.Context,
	projectID memory.ProjectID,
	expectedProjectRoot string,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
	guarded bool,
) (memory.Session, error) {
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

	if guarded {
		err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
			if err := requireChooserCWDProjectOwner(ctx, conn, cwdRoot, expectedCWDProjectID); err != nil {
				return err
			}
			var storedProjectID string
			if err := conn.QueryRowContext(ctx, `
				INSERT INTO sessions (
					id, project_id, project_root_snapshot, status, created_at, updated_at
				)
				SELECT ?, id, canonical_root, ?, ?, ?
				FROM projects
				WHERE id = ? AND archived = 0 AND canonical_root = ?
				RETURNING project_id, project_root_snapshot
			`,
				session.ID,
				session.Status,
				session.CreatedAt.Format(time.RFC3339Nano),
				session.UpdatedAt.Format(time.RFC3339Nano),
				projectID,
				expectedProjectRoot,
			).Scan(&storedProjectID, &session.ProjectRootSnapshot); errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: project %q", ErrChooserStateChanged, projectID)
			} else if err != nil {
				return fmt.Errorf("insert project session: %w", err)
			}
			session.ProjectID = memory.ProjectID(storedProjectID)
			return nil
		})
		if err != nil {
			return memory.Session{}, err
		}
		return session, nil
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
		return memory.Session{}, fmt.Errorf("%w: project %q", ErrProjectNotActive, projectID)
	}

	if err != nil {
		return memory.Session{}, fmt.Errorf("insert project session: %w", err)
	}

	session.ProjectID = memory.ProjectID(storedProjectID)
	return session, nil
}

func (s *Store) CreateGlobalSession(ctx context.Context) (memory.Session, error) {
	return s.createGlobalSession(ctx, "", "", false)
}

func (s *Store) CreateGlobalSessionForChooser(
	ctx context.Context,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	return s.createGlobalSession(ctx, cwdRoot, expectedCWDProjectID, true)
}

func (s *Store) createGlobalSession(
	ctx context.Context,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
	guarded bool,
) (memory.Session, error) {
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

	if guarded {
		err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
			if err := requireChooserCWDProjectOwner(ctx, conn, cwdRoot, expectedCWDProjectID); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `
				INSERT INTO sessions (id, status, created_at, updated_at)
				VALUES (?, ?, ?, ?)
			`,
				session.ID,
				session.Status,
				session.CreatedAt.Format(time.RFC3339Nano),
				session.UpdatedAt.Format(time.RFC3339Nano),
			); err != nil {
				return fmt.Errorf("insert global session: %w", err)
			}
			return nil
		})
		if err != nil {
			return memory.Session{}, err
		}
		return session, nil
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?)
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

// requireChooserCWDProjectOwner is the authoritative transaction-backed check
// for every action derived from a rendered chooser snapshot.
func requireChooserCWDProjectOwner(
	ctx context.Context,
	conn *sql.Conn,
	root string,
	expectedProjectID memory.ProjectID,
) error {
	matches, err := projectRootOwnerMatches(ctx, conn, root, expectedProjectID)
	if err != nil {
		return err
	}
	if !matches {
		return ErrChooserStateChanged
	}
	return nil
}

func projectRootOwnerMatches(
	ctx context.Context,
	conn *sql.Conn,
	root string,
	expectedProjectID memory.ProjectID,
) (bool, error) {
	var matches bool
	if err := conn.QueryRowContext(ctx, `
		SELECT COALESCE((
			SELECT projects.id FROM projects WHERE projects.canonical_root = ?
		), '') = ?
	`, root, expectedProjectID).Scan(&matches); err != nil {
		return false, fmt.Errorf("check chooser cwd ownership: %w", err)
	}
	return matches, nil
}
