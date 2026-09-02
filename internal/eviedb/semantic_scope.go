package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// InspectArchivedSessionClaims is an explicit local audit operation. It derives
// the immutable Scope Context from the closed session row and exposes only that
// session's Semantic Memory; ordinary session-bound reads never use this path.
func (s *Store) InspectArchivedSessionClaims(
	ctx context.Context,
	sessionID memory.SessionID,
	query memory.ClaimQuery,
) (memory.ClaimsInspection, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return memory.ClaimsInspection{}, err
	}
	defer tx.Rollback()

	var workspaceID, projectID sql.NullString
	var status memory.SessionStatus
	if err := tx.QueryRowContext(ctx, `
		SELECT workspace_id, project_id, status FROM sessions WHERE id = ?
	`, sessionID).Scan(&workspaceID, &projectID, &status); errors.Is(err, sql.ErrNoRows) {
		return memory.ClaimsInspection{}, fmt.Errorf("archived Semantic Memory session %q does not exist", sessionID)
	} else if err != nil {
		return memory.ClaimsInspection{}, err
	}
	if status != memory.SessionClosed {
		return memory.ClaimsInspection{}, errors.New("archived Semantic Memory inspection requires a closed session")
	}
	if err := requireSemanticScopeKeysAvailable(ctx, tx, []string{"session:" + string(sessionID)}); err != nil {
		return memory.ClaimsInspection{}, err
	}
	scope := memory.ScopeContext{SessionID: sessionID}
	if workspaceID.Valid {
		scope.WorkspaceID = memory.WorkspaceID(workspaceID.String)
	}
	if projectID.Valid {
		scope.ProjectID = memory.ProjectID(projectID.String)
	}
	validAt, asKnownAt, err := s.semanticQueryTimes(ctx, tx, query)
	if err != nil {
		return memory.ClaimsInspection{}, err
	}
	allowed := make(map[string]struct{})
	for _, key := range allowedSemanticReadScopeKeys(scope) {
		allowed[key] = struct{}{}
	}
	result, found, err := s.inspectClaimsInScope(
		ctx, tx, "session:"+string(sessionID), validAt, asKnownAt, false, allowed,
	)
	if err != nil {
		return memory.ClaimsInspection{}, err
	}
	if !found {
		result.Scope.Key = "session:" + string(sessionID)
	}
	if err := tx.Commit(); err != nil {
		return memory.ClaimsInspection{}, err
	}
	return result, nil
}

func (s *Store) semanticQueryTimes(
	ctx context.Context,
	queryer semanticInspectionQueryer,
	query memory.ClaimQuery,
) (time.Time, time.Time, error) {
	captured := s.now().UTC()
	validAt, asKnownAt := captured, captured
	if query.ValidAt != nil {
		validAt = query.ValidAt.UTC()
	}
	if query.AsKnownAt != nil {
		return validAt, query.AsKnownAt.UTC(), nil
	}
	var latest sql.NullString
	if err := queryer.QueryRowContext(ctx, `SELECT MAX(transaction_time) FROM semantic_operations`).Scan(&latest); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if latest.Valid {
		latestAccepted, err := parseSemanticTime(latest.String)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if latestAccepted.After(asKnownAt) {
			asKnownAt = latestAccepted
		}
	}
	return validAt, asKnownAt, nil
}
