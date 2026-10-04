package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

var ErrWorkspaceResearchPreset = errors.New("research delegation requires the Standard Agent Preset")

// ErrWorkspaceExplorePreset refuses explore permission for a Workspace whose
// default preset cannot delegate.
var ErrWorkspaceExplorePreset = errors.New("explore delegation requires the Standard Agent Preset")

// SetWorkspaceResearch is an explicit owner configuration operation, not a
// model capability. Existing pins and revocations are never rewritten.
func (s *Store) SetWorkspaceResearch(ctx context.Context, id memory.WorkspaceID, expected memory.WorkspaceRevisionID, enabled bool) (memory.Workspace, error) {
	preset := delegation.WorkerResearch
	var workspace memory.Workspace
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var explore memory.WorkspaceExploreSetting
		var err error
		workspace, explore, err = lockWorkspaceRevision(ctx, conn, id, expected)
		if err != nil {
			return err
		}
		if workspace.DefaultPresetID != "standard" {
			return ErrWorkspaceResearchPreset
		}
		if slices.Contains(workspace.AllowedPresetIDs, preset) == enabled {
			return nil
		}
		if enabled {
			workspace.AllowedPresetIDs = append(workspace.AllowedPresetIDs, preset)
		} else {
			workspace.AllowedPresetIDs = slices.DeleteFunc(workspace.AllowedPresetIDs, func(id string) bool { return id == preset })
		}
		now := s.now().UTC()
		if !enabled {
			// A later addition grants only new revisions. It cannot resurrect
			// any older session whose allowance has been revoked.
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_preset_revocations(workspace_id,revision_id,preset_id,revoked_at)
 SELECT p.workspace_id,p.revision_id,?,? FROM workspace_preset_revisions p
 WHERE p.workspace_id=? AND EXISTS(SELECT 1 FROM json_each(p.allowed_preset_ids) WHERE value=?)`, preset, now.Format(time.RFC3339Nano), id, preset); err != nil {
				return err
			}
		}
		// The explore setting is part of the revision and carries forward.
		return insertWorkspaceRevision(ctx, conn, &workspace, expected, explore, now)
	})
	if err != nil {
		return memory.Workspace{}, err
	}
	return s.completeWorkspace(ctx, workspace)
}

// lockWorkspaceRevision reads an active Workspace whose current revision is
// still expected, with that revision's presets and explore setting, inside a
// write transaction.
func lockWorkspaceRevision(ctx context.Context, conn *sql.Conn, id memory.WorkspaceID, expected memory.WorkspaceRevisionID) (memory.Workspace, memory.WorkspaceExploreSetting, error) {
	workspace, err := scanWorkspace(conn.QueryRowContext(ctx, `SELECT id,display_name,lifecycle_state,current_revision_id,created_at,updated_at FROM workspaces WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return workspace, "", ErrWorkspaceNotFound
	}
	if err != nil {
		return workspace, "", err
	}
	if expected == "" || workspace.State != memory.WorkspaceActive || workspace.CurrentRevisionID != expected {
		return workspace, "", ErrChooserStateChanged
	}
	var encoded string
	if err := conn.QueryRowContext(ctx, `SELECT default_preset_id,allowed_preset_ids FROM workspace_preset_revisions WHERE workspace_id=? AND revision_id=?`, id, expected).Scan(&workspace.DefaultPresetID, &encoded); err != nil {
		return workspace, "", err
	}
	if err := json.Unmarshal([]byte(encoded), &workspace.AllowedPresetIDs); err != nil {
		return workspace, "", err
	}
	explore, err := workspaceExplore(ctx, conn, id, expected)
	return workspace, explore.Setting, err
}

// insertWorkspaceRevision makes a new revision with workspace's presets and
// the explore setting current, if expected is still current. The explore
// worker is governed by its setting alone, so a preset list copied from a
// Stage 7 revision drops the explore entry.
func insertWorkspaceRevision(ctx context.Context, conn *sql.Conn, workspace *memory.Workspace, expected memory.WorkspaceRevisionID, explore memory.WorkspaceExploreSetting, now time.Time) error {
	workspace.AllowedPresetIDs = slices.DeleteFunc(workspace.AllowedPresetIDs, func(id string) bool { return id == delegation.WorkerExplore })
	workspace.CurrentRevisionID = memory.WorkspaceRevisionID(uuid.NewString())
	workspace.UpdatedAt = now
	allowed, err := json.Marshal(workspace.AllowedPresetIDs)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_preset_revisions(workspace_id,revision_id,default_preset_id,allowed_preset_ids) VALUES(?,?,?,?)`, workspace.ID, workspace.CurrentRevisionID, workspace.DefaultPresetID, string(allowed)); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_explore_settings(workspace_id,revision_id,setting) VALUES(?,?,?)`, workspace.ID, workspace.CurrentRevisionID, explore); err != nil {
		return err
	}
	result, err := conn.ExecContext(ctx, `UPDATE workspaces SET current_revision_id=?,updated_at=? WHERE id=? AND current_revision_id=?`, workspace.CurrentRevisionID, now.Format(time.RFC3339Nano), workspace.ID, expected)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(ErrChooserStateChanged, err)
	}
	return nil
}

// completeWorkspace adds the operational folder and instruction settings and
// the current revision's explore state to a Workspace read in a transaction.
func (s *Store) completeWorkspace(ctx context.Context, workspace memory.Workspace) (memory.Workspace, error) {
	var err error
	if workspace.Folder, err = s.WorkspaceFolder(ctx, workspace.ID); err != nil {
		return memory.Workspace{}, err
	}
	if workspace.Instructions, err = s.RepositoryInstructionSettings(ctx, workspace.ID); err != nil {
		return memory.Workspace{}, err
	}
	if workspace.Explore, err = workspaceExplore(ctx, s.db, workspace.ID, workspace.CurrentRevisionID); err != nil {
		return memory.Workspace{}, err
	}
	return workspace, nil
}

// authorizeWorkspacePreset requires the worker preset in the parent's pinned
// and the Workspace's current reviewed allowances, unrevoked for the pinned
// revision. Research and explore are allowed separately; explore by its own
// setting, whose default follows the Workspace's folder (2026-10-03).
func authorizeWorkspacePreset(ctx context.Context, conn *sql.Conn, scope memory.ScopeContext, preset string) error {
	if preset == delegation.WorkerExplore {
		return authorizeWorkspaceExplore(ctx, conn, scope)
	}
	var allowed bool
	err := conn.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM workspaces w
 JOIN workspace_preset_revisions pinned ON pinned.workspace_id=w.id AND pinned.revision_id=?
 JOIN workspace_preset_revisions current ON current.workspace_id=w.id AND current.revision_id=w.current_revision_id
 WHERE w.id=? AND w.lifecycle_state='active'
 AND EXISTS(SELECT 1 FROM json_each(pinned.allowed_preset_ids) WHERE value=?)
 AND EXISTS(SELECT 1 FROM json_each(current.allowed_preset_ids) WHERE value=?)
 AND NOT EXISTS(SELECT 1 FROM workspace_preset_revocations r WHERE r.workspace_id=w.id AND r.revision_id=pinned.revision_id AND r.preset_id=?))`,
		scope.WorkspaceRevision, scope.WorkspaceID, preset, preset, preset).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%w: Workspace research requires reviewed permission; enable research delegation in Workspace settings and start a new chat", delegation.ErrAuthority)
	}
	return nil
}
