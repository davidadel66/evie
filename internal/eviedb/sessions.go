package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

var (
	ErrChooserStateChanged = errors.New("eviedb: chooser state changed")
	ErrProjectNotActive    = errors.New("eviedb: project is missing or archived")
	ErrWorkspaceNotActive  = errors.New("eviedb: Workspace is missing or archived")
	ErrSessionNotActive    = errors.New("eviedb: session is missing or inactive")
)

func (s *Store) GetSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot,
		       parent_session_id, COALESCE(title, ''), status, created_at, updated_at FROM sessions WHERE id = ?
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
		SELECT id, workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot,
		       parent_session_id, COALESCE(title, ''), status, created_at, updated_at
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

// EnsureGlobalSession returns one durable active Global primary session for a
// caller-owned stable identity. It never reopens or repurposes an existing
// session, so a collision cannot silently change scope or lifecycle history.
func (s *Store) EnsureGlobalSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	if id == "" {
		return memory.Session{}, errors.New("global session ID is required")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	var session memory.Session
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO sessions (id, status, created_at, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING
		`, id, memory.SessionActive, now, now); err != nil {
			return fmt.Errorf("ensure global session %q: %w", id, err)
		}
		var err error
		session, err = scanSession(conn.QueryRowContext(ctx, `
			SELECT id, workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot,
			       parent_session_id, COALESCE(title, ''), status, created_at, updated_at
			FROM sessions WHERE id = ?
		`, id))
		if err != nil {
			return fmt.Errorf("read ensured global session %q: %w", id, err)
		}
		if session.Status != memory.SessionActive || session.WorkspaceID != "" ||
			session.ProjectID != "" || session.ParentSessionID != "" {
			return fmt.Errorf("global session identity %q is already reserved by an incompatible session", id)
		}
		return nil
	})
	return session, err
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
			SELECT id, workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot, parent_session_id,
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
		SELECT sessions.id, sessions.workspace_id, sessions.workspace_revision_snapshot,
		       sessions.project_id, sessions.project_root_snapshot,
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
		id, title, status, createdText, updatedText                       string
		workspaceID, workspaceRevision, projectID, rootSnapshot, parentID sql.NullString
	)
	if err := scanner.Scan(
		&id, &workspaceID, &workspaceRevision, &projectID, &rootSnapshot, &parentID, &title, &status,
		&createdText, &updatedText, activity,
	); err != nil {
		return memory.Session{}, err
	}
	return sessionFromScanned(id, workspaceID, workspaceRevision, projectID, rootSnapshot, parentID, title, status, createdText, updatedText)
}

func scanSession(scanner rowScanner) (memory.Session, error) {
	var (
		id, title, status, createdText, updatedText                       string
		workspaceID, workspaceRevision, projectID, rootSnapshot, parentID sql.NullString
	)

	if err := scanner.Scan(
		&id,
		&workspaceID,
		&workspaceRevision,
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

	return sessionFromScanned(id, workspaceID, workspaceRevision, projectID, rootSnapshot, parentID, title, status, createdText, updatedText)
}

func sessionFromScanned(
	id string,
	workspaceID, workspaceRevision, projectID, rootSnapshot, parentID sql.NullString,
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
	if workspaceID.Valid {
		session.WorkspaceID = memory.WorkspaceID(workspaceID.String)
	}
	if workspaceRevision.Valid {
		session.WorkspaceRevisionSnapshot = memory.WorkspaceRevisionID(workspaceRevision.String)
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

func (s *Store) CreateWorkspaceSessionWithComposition(
	ctx context.Context,
	workspaceID memory.WorkspaceID,
	revisionID memory.WorkspaceRevisionID,
	receipt composition.Receipt,
) (memory.Session, error) {
	return s.createWorkspaceSession(ctx, workspaceID, revisionID, receipt, false)
}

func (s *Store) CreateWorkspaceSessionForChooserWithComposition(
	ctx context.Context,
	workspaceID memory.WorkspaceID,
	expectedRevisionID memory.WorkspaceRevisionID,
	receipt composition.Receipt,
) (memory.Session, error) {
	return s.createWorkspaceSession(ctx, workspaceID, expectedRevisionID, receipt, true)
}

func (s *Store) createWorkspaceSession(
	ctx context.Context,
	workspaceID memory.WorkspaceID,
	revisionID memory.WorkspaceRevisionID,
	receipt composition.Receipt,
	chooser bool,
) (memory.Session, error) {
	encodedReceipt, err := composition.Marshal(receipt)
	if err != nil {
		return memory.Session{}, fmt.Errorf("validate Composition Receipt: %w", err)
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return memory.Session{}, fmt.Errorf("generate session ID: %w", err)
	}
	now := s.now().UTC()
	session := memory.Session{ID: memory.SessionID(id.String()), Status: memory.SessionActive, CreatedAt: now, UpdatedAt: now}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var storedWorkspaceID, storedRevisionID string
		err := conn.QueryRowContext(ctx, `
			INSERT INTO sessions (
				id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at
			)
			SELECT ?, id, current_revision_id, ?, ?, ?
			FROM workspaces
			WHERE id = ? AND lifecycle_state = ? AND current_revision_id = ?
			RETURNING workspace_id, workspace_revision_snapshot
		`, session.ID, session.Status, session.CreatedAt.Format(time.RFC3339Nano),
			session.UpdatedAt.Format(time.RFC3339Nano), workspaceID, memory.WorkspaceActive, revisionID,
		).Scan(&storedWorkspaceID, &storedRevisionID)
		if errors.Is(err, sql.ErrNoRows) {
			if chooser {
				return fmt.Errorf("%w: Workspace %q", ErrChooserStateChanged, workspaceID)
			}
			return fmt.Errorf("%w: Workspace %q", ErrWorkspaceNotActive, workspaceID)
		}
		if err != nil {
			return fmt.Errorf("insert Workspace session: %w", err)
		}
		session.WorkspaceID = memory.WorkspaceID(storedWorkspaceID)
		session.WorkspaceRevisionSnapshot = memory.WorkspaceRevisionID(storedRevisionID)
		if err := insertCompositionReceipt(ctx, conn, session.ID, encodedReceipt, now); err != nil {
			return fmt.Errorf("insert Workspace session Composition Receipt: %w", err)
		}
		return nil
	})
	if err != nil {
		return memory.Session{}, err
	}
	return session, nil
}

func (s *Store) CreateProjectSession(ctx context.Context, projectID memory.ProjectID) (memory.Session, error) {
	return s.createProjectSession(ctx, projectID, "", "", "", false, nil)
}

func (s *Store) CreateProjectSessionWithComposition(
	ctx context.Context,
	projectID memory.ProjectID,
	receipt composition.Receipt,
) (memory.Session, error) {
	return s.createProjectSession(ctx, projectID, "", "", "", false, &receipt)
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
	return s.createProjectSession(ctx, projectID, expectedProjectRoot, cwdRoot, expectedCWDProjectID, true, nil)
}

func (s *Store) CreateProjectSessionForChooserWithComposition(
	ctx context.Context,
	projectID memory.ProjectID,
	expectedProjectRoot string,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
	receipt composition.Receipt,
) (memory.Session, error) {
	return s.createProjectSession(
		ctx, projectID, expectedProjectRoot, cwdRoot, expectedCWDProjectID, true, &receipt,
	)
}

func (s *Store) createProjectSession(
	ctx context.Context,
	projectID memory.ProjectID,
	expectedProjectRoot string,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
	guarded bool,
	receipt *composition.Receipt,
) (memory.Session, error) {
	var encodedReceipt []byte
	if receipt != nil {
		var err error
		encodedReceipt, err = composition.Marshal(*receipt)
		if err != nil {
			return memory.Session{}, fmt.Errorf("validate Composition Receipt: %w", err)
		}
	}
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
			if receipt != nil {
				if err := insertCompositionReceipt(ctx, conn, session.ID, encodedReceipt, now); err != nil {
					return fmt.Errorf("insert project session Composition Receipt: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return memory.Session{}, err
		}
		return session, nil
	}
	if receipt != nil {
		err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
			var storedProjectID string
			if err := conn.QueryRowContext(ctx, `
				INSERT INTO sessions (
					id, project_id, project_root_snapshot, status, created_at, updated_at
				)
				SELECT ?, id, canonical_root, ?, ?, ?
				FROM projects
				WHERE id = ? AND archived = 0
				RETURNING project_id, project_root_snapshot
			`, session.ID, session.Status, session.CreatedAt.Format(time.RFC3339Nano),
				session.UpdatedAt.Format(time.RFC3339Nano), projectID,
			).Scan(&storedProjectID, &session.ProjectRootSnapshot); errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: project %q", ErrProjectNotActive, projectID)
			} else if err != nil {
				return fmt.Errorf("insert project session: %w", err)
			}
			session.ProjectID = memory.ProjectID(storedProjectID)
			if err := insertCompositionReceipt(ctx, conn, session.ID, encodedReceipt, now); err != nil {
				return fmt.Errorf("insert project session Composition Receipt: %w", err)
			}
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
	return s.createGlobalSession(ctx, "", "", false, nil)
}

func (s *Store) CreateGlobalSessionWithComposition(
	ctx context.Context,
	receipt composition.Receipt,
) (memory.Session, error) {
	return s.createGlobalSession(ctx, "", "", false, &receipt)
}

func (s *Store) CreateGlobalSessionForChooser(
	ctx context.Context,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
) (memory.Session, error) {
	return s.createGlobalSession(ctx, cwdRoot, expectedCWDProjectID, true, nil)
}

func (s *Store) CreateGlobalSessionForChooserWithComposition(
	ctx context.Context,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
	receipt composition.Receipt,
) (memory.Session, error) {
	return s.createGlobalSession(ctx, cwdRoot, expectedCWDProjectID, true, &receipt)
}

func (s *Store) createGlobalSession(
	ctx context.Context,
	cwdRoot string,
	expectedCWDProjectID memory.ProjectID,
	guarded bool,
	receipt *composition.Receipt,
) (memory.Session, error) {
	var encodedReceipt []byte
	if receipt != nil {
		var err error
		encodedReceipt, err = composition.Marshal(*receipt)
		if err != nil {
			return memory.Session{}, fmt.Errorf("validate Composition Receipt: %w", err)
		}
	}
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
			if receipt != nil {
				if err := insertCompositionReceipt(ctx, conn, session.ID, encodedReceipt, now); err != nil {
					return fmt.Errorf("insert global session Composition Receipt: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return memory.Session{}, err
		}
		return session, nil
	}
	if receipt != nil {
		err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
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
			if err := insertCompositionReceipt(ctx, conn, session.ID, encodedReceipt, now); err != nil {
				return fmt.Errorf("insert global session Composition Receipt: %w", err)
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
