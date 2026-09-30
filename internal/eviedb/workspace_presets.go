package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrWorkspacePresetNotAllowed = errors.New("Agent Preset is not allowed in this Workspace")

// Preset choices belong to the pinned Workspace revision. Folder and instruction
// settings have independent operational revisions.
func ensureWorkspacePresets(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workspace_preset_revisions (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), revision_id TEXT NOT NULL,
 default_preset_id TEXT NOT NULL CHECK(length(trim(default_preset_id)) > 0),
 allowed_preset_ids TEXT NOT NULL CHECK(json_valid(allowed_preset_ids) AND json_type(allowed_preset_ids) = 'array'),
 PRIMARY KEY(workspace_id, revision_id));
 INSERT OR IGNORE INTO workspace_preset_revisions(workspace_id,revision_id,default_preset_id,allowed_preset_ids)
 SELECT id,current_revision_id,'standard','["standard"]' FROM workspaces;
 CREATE TRIGGER IF NOT EXISTS workspace_preset_revisions_no_update BEFORE UPDATE ON workspace_preset_revisions
 BEGIN SELECT RAISE(ABORT,'Workspace preset revisions are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workspace_preset_revisions_no_delete BEFORE DELETE ON workspace_preset_revisions
 BEGIN SELECT RAISE(ABORT,'Workspace preset revisions are immutable'); END;
 CREATE TABLE IF NOT EXISTS workspace_preset_revocations (
 workspace_id TEXT NOT NULL, revision_id TEXT NOT NULL, preset_id TEXT NOT NULL,
 revoked_at TEXT NOT NULL, PRIMARY KEY(workspace_id,revision_id,preset_id),
 FOREIGN KEY(workspace_id,revision_id) REFERENCES workspace_preset_revisions(workspace_id,revision_id));
 CREATE TRIGGER IF NOT EXISTS workspace_preset_revocations_no_update BEFORE UPDATE ON workspace_preset_revocations
 BEGIN SELECT RAISE(ABORT,'Workspace preset revocations are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workspace_preset_revocations_no_delete BEFORE DELETE ON workspace_preset_revocations
 BEGIN SELECT RAISE(ABORT,'Workspace preset revocations are immutable'); END;`)
	return err
}

// WorkspaceDefaultPreset requires the same active revision the owner selected.
// Session insertion checks its allowlist again in the receipt transaction.
func (s *Store) WorkspaceDefaultPreset(ctx context.Context, id memory.WorkspaceID, revision memory.WorkspaceRevisionID) (string, error) {
	var preset string
	err := s.db.QueryRowContext(ctx, `SELECT p.default_preset_id FROM workspaces w
 JOIN workspace_preset_revisions p ON p.workspace_id=w.id AND p.revision_id=w.current_revision_id
 WHERE w.id=? AND w.current_revision_id=? AND w.lifecycle_state='active'`, id, revision).Scan(&preset)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrChooserStateChanged
	}
	return preset, err
}

func (s *Store) loadWorkspacePresets(ctx context.Context, workspace *memory.Workspace) error {
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT default_preset_id,allowed_preset_ids FROM workspace_preset_revisions
 WHERE workspace_id=? AND revision_id=?`, workspace.ID, workspace.CurrentRevisionID).Scan(&workspace.DefaultPresetID, &encoded)
	if err != nil {
		return fmt.Errorf("read Workspace Agent Preset: %w", err)
	}
	return json.Unmarshal([]byte(encoded), &workspace.AllowedPresetIDs)
}
