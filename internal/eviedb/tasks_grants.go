package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

// IssueTaskAccessGrant is a Kernel-only orchestration seam. It is intentionally
// absent from task.Service and every model-facing capability.
func (s *Store) IssueTaskAccessGrant(ctx context.Context, input task.GrantInput) (task.AccessGrant, error) {
	return s.issueTaskAccessGrant(ctx, input, false)
}

// IssueFocusedTaskAccessGrant atomically gives an existing direct child
// session one bounded subtree grant and selects that granted root as its Task
// Focus. It is a Kernel-only orchestration seam and is intentionally absent
// from model-facing Todo capabilities.
func (s *Store) IssueFocusedTaskAccessGrant(ctx context.Context, input task.GrantInput) (task.AccessGrant, error) {
	return s.issueTaskAccessGrant(ctx, input, true)
}

func (s *Store) issueTaskAccessGrant(ctx context.Context, input task.GrantInput, focus bool) (task.AccessGrant, error) {
	if strings.TrimSpace(input.GranteeSessionID) == "" {
		return task.AccessGrant{}, &task.InputError{Field: "grantee_session_id", Message: "must not be blank"}
	}
	if strings.TrimSpace(string(input.RootID)) == "" {
		return task.AccessGrant{}, &task.InputError{Field: "root_task_id", Message: "must not be blank"}
	}
	if err := task.ValidateAccessLevel(input.Level); err != nil {
		return task.AccessGrant{}, err
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.AccessGrant{}, err
	}
	if attribution.ActorID != string(memory.LocalOwnerID) {
		return task.AccessGrant{}, task.ErrAccessDenied
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return task.AccessGrant{}, fmt.Errorf("generate Task Access Grant ID: %w", err)
	}
	grant := task.AccessGrant{
		ID: id.String(), GranteeSessionID: input.GranteeSessionID, RootID: input.RootID, Level: input.Level,
		IssuerActorID: attribution.ActorID, IssuerSessionID: attribution.SessionID,
		IssuerRunID: attribution.RunID, IssuedAt: s.now().UTC(),
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		issuer, err := readGrantSession(ctx, conn, attribution.SessionID)
		if errors.Is(err, sql.ErrNoRows) {
			return task.ErrAccessDenied
		}
		if err != nil {
			return fmt.Errorf("read Task Access Grant issuer: %w", err)
		}
		if issuer.Status != memory.SessionActive || issuer.ParentSessionID != "" {
			return task.ErrAccessDenied
		}
		grantee, err := readGrantSession(ctx, conn, input.GranteeSessionID)
		if errors.Is(err, sql.ErrNoRows) {
			return task.ErrAccessDenied
		}
		if err != nil {
			return fmt.Errorf("read Task Access Grant grantee: %w", err)
		}
		if grantee.Status != memory.SessionActive || grantee.ParentSessionID != issuer.ID {
			return task.ErrAccessDenied
		}
		if issuer.WorkspaceID != grantee.WorkspaceID || issuer.ProjectID != grantee.ProjectID {
			return task.ErrAccessDenied
		}
		authorizedCtx := withTaskAuthorization(ctx, task.CapabilityGet, task.AccessRead)
		root, err := getGlobalTask(authorizedCtx, conn, input.RootID)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, task.ErrNotFound) {
			return &task.NotFoundError{ID: input.RootID}
		}
		if err != nil {
			return err
		}
		allowedScope := root.Scope == task.ScopeGlobal ||
			(issuer.WorkspaceID != "" && root.Scope == task.WorkspaceScope(string(issuer.WorkspaceID))) ||
			(issuer.ProjectID != "" && root.Scope == task.ProjectScope(string(issuer.ProjectID)))
		if !allowedScope {
			return &task.NotFoundError{ID: input.RootID}
		}
		_, err = conn.ExecContext(ctx, `
			INSERT INTO task_access_grants (
				id, grantee_session_id, root_task_id, access_level, issuer_actor_id,
				issuer_session_id, issuer_run_id, issued_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, grant.ID, grant.GranteeSessionID, grant.RootID, grant.Level, grant.IssuerActorID,
			grant.IssuerSessionID, grant.IssuerRunID, formatTaskTime(grant.IssuedAt))
		if err != nil {
			return fmt.Errorf("insert Task Access Grant: %w", err)
		}
		if focus {
			if err := upsertTaskFocus(ctx, conn, grantee.ID, grant.RootID, grant.IssuedAt); err != nil {
				return fmt.Errorf("focus granted Task subtree: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return task.AccessGrant{}, err
	}
	return grant, nil
}

func readGrantSession(ctx context.Context, source queryRower, id string) (memory.Session, error) {
	var session memory.Session
	var workspaceID, projectID, parentID sql.NullString
	err := source.QueryRowContext(ctx, `
		SELECT id, workspace_id, project_id, parent_session_id, status
		FROM sessions WHERE id = ?
	`, id).Scan(&session.ID, &workspaceID, &projectID, &parentID, &session.Status)
	if err != nil {
		return memory.Session{}, err
	}
	if workspaceID.Valid {
		session.WorkspaceID = memory.WorkspaceID(workspaceID.String)
	}
	if projectID.Valid {
		session.ProjectID = memory.ProjectID(projectID.String)
	}
	if parentID.Valid {
		session.ParentSessionID = memory.SessionID(parentID.String)
	}
	return session, nil
}

func (s *Store) GetTaskAccessGrant(ctx context.Context, id string) (task.AccessGrant, error) {
	if strings.TrimSpace(id) == "" {
		return task.AccessGrant{}, &task.InputError{Field: "grant_id", Message: "must not be blank"}
	}
	grant, err := scanTaskAccessGrant(s.db.QueryRowContext(ctx, `
		SELECT id, grantee_session_id, root_task_id, access_level, issuer_actor_id,
		       issuer_session_id, issuer_run_id, issued_at, ended_at, COALESCE(end_reason, ''),
		       COALESCE(ended_by_actor_id, ''), COALESCE(ended_by_session_id, ''), COALESCE(ended_by_run_id, '')
		FROM task_access_grants WHERE id = ?
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return task.AccessGrant{}, fmt.Errorf("Task Access Grant %q not found: %w", id, sql.ErrNoRows)
	}
	if err != nil {
		return task.AccessGrant{}, fmt.Errorf("read Task Access Grant: %w", err)
	}
	return grant, nil
}

func scanTaskAccessGrant(scanner rowScanner) (task.AccessGrant, error) {
	var grant task.AccessGrant
	var issuedAt string
	var endedAt sql.NullString
	if err := scanner.Scan(&grant.ID, &grant.GranteeSessionID, &grant.RootID, &grant.Level,
		&grant.IssuerActorID, &grant.IssuerSessionID, &grant.IssuerRunID, &issuedAt,
		&endedAt, &grant.EndReason, &grant.EndedByActorID, &grant.EndedBySessionID,
		&grant.EndedByRunID); err != nil {
		return task.AccessGrant{}, err
	}
	var err error
	grant.IssuedAt, err = time.Parse(time.RFC3339Nano, issuedAt)
	if err != nil {
		return task.AccessGrant{}, fmt.Errorf("parse Task Access Grant issued_at: %w", err)
	}
	if endedAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, endedAt.String)
		if err != nil {
			return task.AccessGrant{}, fmt.Errorf("parse Task Access Grant ended_at: %w", err)
		}
		grant.EndedAt = &parsed
	}
	return grant, nil
}

// TerminateTaskAccessGrant permanently ends an active grant while retaining
// its immutable issuance and monotonic termination audit.
func (s *Store) TerminateTaskAccessGrant(ctx context.Context, id, reason string) (task.AccessGrant, error) {
	if strings.TrimSpace(reason) == "" {
		return task.AccessGrant{}, &task.InputError{Field: "termination_reason", Message: "must not be blank"}
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.AccessGrant{}, err
	}
	var ended task.AccessGrant
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		issuer, err := readGrantSession(ctx, conn, attribution.SessionID)
		if errors.Is(err, sql.ErrNoRows) {
			return task.ErrAccessDenied
		}
		if err != nil {
			return fmt.Errorf("read Task Access Grant terminator: %w", err)
		}
		if issuer.Status != memory.SessionActive || issuer.ParentSessionID != "" || attribution.ActorID != string(memory.LocalOwnerID) {
			return task.ErrAccessDenied
		}
		ended, err = scanTaskAccessGrant(conn.QueryRowContext(ctx, `
			SELECT id, grantee_session_id, root_task_id, access_level, issuer_actor_id,
			       issuer_session_id, issuer_run_id, issued_at, ended_at, COALESCE(end_reason, ''),
			       COALESCE(ended_by_actor_id, ''), COALESCE(ended_by_session_id, ''), COALESCE(ended_by_run_id, '')
			FROM task_access_grants WHERE id = ?
		`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return task.ErrAccessDenied
		}
		if err != nil {
			return fmt.Errorf("read Task Access Grant for termination: %w", err)
		}
		if ended.IssuerSessionID != attribution.SessionID || ended.EndedAt != nil {
			return task.ErrAccessDenied
		}
		now := s.now().UTC()
		if _, err := conn.ExecContext(ctx, `
			UPDATE task_access_grants
			SET ended_at = ?, end_reason = ?, ended_by_actor_id = ?, ended_by_session_id = ?, ended_by_run_id = ?
			WHERE id = ? AND ended_at IS NULL
		`, formatTaskTime(now), strings.TrimSpace(reason), attribution.ActorID, attribution.SessionID,
			attribution.RunID, id); err != nil {
			return fmt.Errorf("terminate Task Access Grant: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM session_task_focus WHERE session_id = ?`, ended.GranteeSessionID); err != nil {
			return fmt.Errorf("clear terminated Task focus: %w", err)
		}
		ended.EndedAt = &now
		ended.EndReason = strings.TrimSpace(reason)
		ended.EndedByActorID = attribution.ActorID
		ended.EndedBySessionID = attribution.SessionID
		ended.EndedByRunID = attribution.RunID
		return nil
	})
	if err != nil {
		return task.AccessGrant{}, err
	}
	return ended, nil
}
