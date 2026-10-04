package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// ErrWorkspaceExploreSetting refuses a setting other than default, on or off.
var ErrWorkspaceExploreSetting = errors.New("explore setting must be default, on or off")

// workspaceExploreSchema records each Workspace revision's explore setting
// (2026-10-03). Like the revision it belongs to, a row is immutable. 'default'
// allows explore workers exactly while the Workspace has a local folder; 'on'
// and 'off' are the owner's explicit choice and ignore the folder. A revision
// without a row allows nothing.
const workspaceExploreSchema = `CREATE TABLE workspace_explore_settings (
 workspace_id TEXT NOT NULL, revision_id TEXT NOT NULL,
 setting TEXT NOT NULL CHECK(setting IN ('default','on','off')),
 PRIMARY KEY(workspace_id,revision_id),
 FOREIGN KEY(workspace_id,revision_id) REFERENCES workspace_preset_revisions(workspace_id,revision_id))`

const workspaceExploreTriggers = `CREATE TRIGGER IF NOT EXISTS workspace_explore_settings_no_update BEFORE UPDATE ON workspace_explore_settings
 BEGIN SELECT RAISE(ABORT,'Workspace explore settings are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workspace_explore_settings_no_delete BEFORE DELETE ON workspace_explore_settings
 BEGIN SELECT RAISE(ABORT,'Workspace explore settings are immutable'); END;`

// ensureWorkspaceExplore creates the explore settings once and records a
// setting for every existing revision without changing any revision: one
// that listed the explore preset (Stage 7 as merged) is explicitly on; one
// without it in a Workspace whose owner revoked explore is explicitly off;
// any other follows the folder. The table's existence is re-read under the
// write lock, so competing startups migrate once and a later startup never
// re-derives a setting the owner has since chosen.
func ensureWorkspaceExplore(ctx context.Context, db *sql.DB) error {
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		var exists bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='workspace_explore_settings')`).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := conn.ExecContext(ctx, workspaceExploreSchema); err != nil {
				return fmt.Errorf("create Workspace explore settings: %w", err)
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_explore_settings(workspace_id,revision_id,setting)
 SELECT p.workspace_id,p.revision_id,CASE
 WHEN EXISTS(SELECT 1 FROM json_each(p.allowed_preset_ids) WHERE value=?) THEN 'on'
 WHEN EXISTS(SELECT 1 FROM workspace_preset_revocations r WHERE r.workspace_id=p.workspace_id AND r.preset_id=?) THEN 'off'
 ELSE 'default' END
 FROM workspace_preset_revisions p`, delegation.WorkerExplore, delegation.WorkerExplore); err != nil {
				return fmt.Errorf("record Workspace explore settings: %w", err)
			}
		}
		_, err := conn.ExecContext(ctx, workspaceExploreTriggers)
		return err
	})
}

// exploreAllowedSQL is the explore ceiling of preset revision p whose
// setting is the SQL expression setting: a Standard Workspace, explicitly
// on, or on the default while the Workspace's latest folder is attached.
// The folder is read at each check, as every chat uses the current folder.
func exploreAllowedSQL(p, setting string) string {
	return fmt.Sprintf(`(%[1]s.default_preset_id='standard' AND (%[2]s='on' OR (%[2]s='default' AND COALESCE((SELECT f.path FROM workspace_folder_revisions f WHERE f.workspace_id=%[1]s.workspace_id ORDER BY f.revision DESC LIMIT 1),'')<>'')))`, p, setting)
}

// authorizeWorkspaceExplore requires both the parent's pinned revision and
// the Workspace's current revision to allow explore now, and no revocation
// of the pinned revision.
func authorizeWorkspaceExplore(ctx context.Context, conn *sql.Conn, scope memory.ScopeContext) error {
	var allowed bool
	err := conn.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM workspaces w
 JOIN workspace_preset_revisions pinned ON pinned.workspace_id=w.id AND pinned.revision_id=?
 JOIN workspace_explore_settings pinned_explore ON pinned_explore.workspace_id=pinned.workspace_id AND pinned_explore.revision_id=pinned.revision_id
 JOIN workspace_preset_revisions current ON current.workspace_id=w.id AND current.revision_id=w.current_revision_id
 JOIN workspace_explore_settings current_explore ON current_explore.workspace_id=current.workspace_id AND current_explore.revision_id=current.revision_id
 WHERE w.id=? AND w.lifecycle_state='active'
 AND `+exploreAllowedSQL("pinned", "pinned_explore.setting")+`
 AND `+exploreAllowedSQL("current", "current_explore.setting")+`
 AND NOT EXISTS(SELECT 1 FROM workspace_preset_revocations r WHERE r.workspace_id=w.id AND r.revision_id=pinned.revision_id AND r.preset_id=?))`,
		scope.WorkspaceRevision, scope.WorkspaceID, delegation.WorkerExplore).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%w: Workspace explore delegation requires reviewed permission: explore is on by default only while the Workspace has a local folder, and the owner can turn it on or off in Workspace settings; a chat started before it was turned on, or one it was turned off for, needs a new chat", delegation.ErrAuthority)
	}
	return nil
}

// workspaceExplore reads revision's explore setting and whether it allows
// explore now. A revision without a recorded setting allows nothing.
func workspaceExplore(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id memory.WorkspaceID, revision memory.WorkspaceRevisionID) (memory.WorkspaceExplore, error) {
	var explore memory.WorkspaceExplore
	err := q.QueryRowContext(ctx, `SELECT COALESCE(e.setting,'off'),`+exploreAllowedSQL("p", "COALESCE(e.setting,'off')")+`
 FROM workspace_preset_revisions p LEFT JOIN workspace_explore_settings e ON e.workspace_id=p.workspace_id AND e.revision_id=p.revision_id
 WHERE p.workspace_id=? AND p.revision_id=?`, id, revision).Scan(&explore.Setting, &explore.Allowed)
	if err != nil {
		return explore, fmt.Errorf("read Workspace explore setting: %w", err)
	}
	return explore, nil
}

// SetWorkspaceExplore records the owner's explore setting for a Standard
// Workspace as a new revision, like SetWorkspaceResearch: it applies to
// chats started afterwards, and turning explore off durably revokes every
// earlier revision that was on or followed the folder, so no later setting
// restores those chats. Returning to the default or turning explore on
// revokes nothing.
func (s *Store) SetWorkspaceExplore(ctx context.Context, id memory.WorkspaceID, expected memory.WorkspaceRevisionID, setting memory.WorkspaceExploreSetting) (memory.Workspace, error) {
	if !setting.Valid() {
		return memory.Workspace{}, ErrWorkspaceExploreSetting
	}
	var workspace memory.Workspace
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var current memory.WorkspaceExploreSetting
		var err error
		workspace, current, err = lockWorkspaceRevision(ctx, conn, id, expected)
		if err != nil {
			return err
		}
		if workspace.DefaultPresetID != "standard" {
			return ErrWorkspaceExplorePreset
		}
		if current == setting {
			return nil
		}
		now := s.now().UTC()
		if setting == memory.WorkspaceExploreOff {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_preset_revocations(workspace_id,revision_id,preset_id,revoked_at)
 SELECT workspace_id,revision_id,?,? FROM workspace_explore_settings WHERE workspace_id=? AND setting IN ('on','default')`,
				delegation.WorkerExplore, now.Format(time.RFC3339Nano), id); err != nil {
				return err
			}
		}
		return insertWorkspaceRevision(ctx, conn, &workspace, expected, setting, "", now)
	})
	if err != nil {
		return memory.Workspace{}, err
	}
	return s.completeWorkspace(ctx, workspace)
}
