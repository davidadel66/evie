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
	return s.setWorkspaceWorker(ctx, id, expected, delegation.WorkerResearch, enabled)
}

// SetWorkspaceExplore allows or revokes read-only explore workers in a
// Standard Workspace, exactly as SetWorkspaceResearch does for research
// workers: a new revision, applying to new chats, and a durable revocation of
// every earlier revision that allowed it (Stage 7, 2026-10-03).
func (s *Store) SetWorkspaceExplore(ctx context.Context, id memory.WorkspaceID, expected memory.WorkspaceRevisionID, enabled bool) (memory.Workspace, error) {
	return s.setWorkspaceWorker(ctx, id, expected, delegation.WorkerExplore, enabled)
}

func (s *Store) setWorkspaceWorker(ctx context.Context, id memory.WorkspaceID, expected memory.WorkspaceRevisionID, preset string, enabled bool) (memory.Workspace, error) {
	var workspace memory.Workspace
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		workspace, err = scanWorkspace(conn.QueryRowContext(ctx, `SELECT id,display_name,lifecycle_state,current_revision_id,created_at,updated_at FROM workspaces WHERE id=?`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkspaceNotFound
		}
		if err != nil {
			return err
		}
		if expected == "" || workspace.State != memory.WorkspaceActive || workspace.CurrentRevisionID != expected {
			return ErrChooserStateChanged
		}
		var encoded string
		if err := conn.QueryRowContext(ctx, `SELECT default_preset_id,allowed_preset_ids FROM workspace_preset_revisions WHERE workspace_id=? AND revision_id=?`, id, expected).Scan(&workspace.DefaultPresetID, &encoded); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(encoded), &workspace.AllowedPresetIDs); err != nil {
			return err
		}
		if workspace.DefaultPresetID != "standard" {
			if preset == delegation.WorkerExplore {
				return ErrWorkspaceExplorePreset
			}
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
		workspace.CurrentRevisionID = memory.WorkspaceRevisionID(uuid.NewString())
		workspace.UpdatedAt = now
		allowed, err := json.Marshal(workspace.AllowedPresetIDs)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_preset_revisions(workspace_id,revision_id,default_preset_id,allowed_preset_ids) VALUES(?,?,?,?)`, id, workspace.CurrentRevisionID, workspace.DefaultPresetID, string(allowed)); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE workspaces SET current_revision_id=?,updated_at=? WHERE id=? AND current_revision_id=?`, workspace.CurrentRevisionID, now.Format(time.RFC3339Nano), id, expected)
		return err
	})
	if err != nil {
		return memory.Workspace{}, err
	}
	workspace.Folder, err = s.WorkspaceFolder(ctx, id)
	if err != nil {
		return memory.Workspace{}, err
	}
	workspace.Instructions, err = s.RepositoryInstructionSettings(ctx, id)
	return workspace, err
}

// authorizeWorkspacePreset requires the worker preset in the parent's pinned
// and the Workspace's current reviewed allowances, unrevoked for the pinned
// revision. Research and explore are allowed separately (2026-10-03).
func authorizeWorkspacePreset(ctx context.Context, conn *sql.Conn, scope memory.ScopeContext, preset string) error {
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
		if preset == delegation.WorkerExplore {
			return fmt.Errorf("%w: Workspace explore delegation requires reviewed permission: the Workspace must allow the explore preset, and the chat must start after it was allowed", delegation.ErrAuthority)
		}
		return fmt.Errorf("%w: Workspace research requires reviewed permission; enable research delegation in Workspace settings and start a new chat", delegation.ErrAuthority)
	}
	return nil
}
