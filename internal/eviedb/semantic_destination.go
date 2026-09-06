package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"github.com/davidadel66/evie/internal/memory"
	"sort"
)

func validateRecommendedDestination(ctx context.Context, writer turnLeaseWriteExecutor, sessionID memory.SessionID, destination memory.MemoryDestination, legacySession bool, target string, source memory.SemanticSource, scopes []memory.SemanticScope) error {
	var workspace, project sql.NullString
	if err := writer.queryRowContext(ctx, `SELECT workspace_id,project_id FROM sessions WHERE id=? AND status=?`, sessionID, memory.SessionActive).Scan(&workspace, &project); err != nil {
		return err
	}
	bound := memory.ScopeContext{SessionID: sessionID, WorkspaceID: memory.WorkspaceID(workspace.String), ProjectID: memory.ProjectID(project.String)}
	expected, err := memory.ResolveMemoryDestination(bound, destination, legacySession)
	if err != nil {
		return err
	}
	if target != expected {
		return errors.New("memory destination differs from its approved recommendation")
	}
	sourceKey := scopeKeyForContext(bound)
	expectedScopes := []string{"global"}
	if sourceKey != "global" {
		expectedScopes = append(expectedScopes, sourceKey)
	}
	if expected != "global" && expected != sourceKey {
		expectedScopes = append(expectedScopes, expected)
	}
	sort.Strings(expectedScopes)
	return validateAuthorizedSemanticScopes(sourceKey, expectedScopes, sessionID, source.SessionID, source.ScopeKey, scopes)
}
