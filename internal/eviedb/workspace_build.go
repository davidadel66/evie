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

// ErrWorkspaceBuildSetting refuses a setting other than default, on or off.
var ErrWorkspaceBuildSetting = errors.New("build setting must be default, on or off")

// ErrWorkspaceBuildPreset refuses build permission for a Workspace whose
// default preset cannot delegate.
var ErrWorkspaceBuildPreset = errors.New("build delegation requires the Standard Agent Preset")

// workspaceBuildSchema records each Workspace revision's build setting
// (Stage 11, 2026-10-03), mirroring the explore settings: one immutable row
// per revision. Unlike explore, 'default' allows nothing: writing code is a
// bigger grant than reading it, so only the owner's explicit 'on' allows
// build workers. A revision without a row allows nothing either.
const workspaceBuildSchema = `CREATE TABLE workspace_build_settings (
 workspace_id TEXT NOT NULL, revision_id TEXT NOT NULL,
 setting TEXT NOT NULL CHECK(setting IN ('default','on','off')),
 PRIMARY KEY(workspace_id,revision_id),
 FOREIGN KEY(workspace_id,revision_id) REFERENCES workspace_preset_revisions(workspace_id,revision_id))`

const workspaceBuildTriggers = `CREATE TRIGGER IF NOT EXISTS workspace_build_settings_no_update BEFORE UPDATE ON workspace_build_settings
 BEGIN SELECT RAISE(ABORT,'Workspace build settings are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workspace_build_settings_no_delete BEFORE DELETE ON workspace_build_settings
 BEGIN SELECT RAISE(ABORT,'Workspace build settings are immutable'); END;`

// ensureWorkspaceBuild creates the build settings once and records 'default'
// (off) for every existing revision without changing any revision. The
// table's existence is re-read under the write lock, so competing startups
// migrate once.
func ensureWorkspaceBuild(ctx context.Context, db *sql.DB) error {
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		var exists bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='workspace_build_settings')`).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := conn.ExecContext(ctx, workspaceBuildSchema); err != nil {
				return fmt.Errorf("create Workspace build settings: %w", err)
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_build_settings(workspace_id,revision_id,setting)
 SELECT workspace_id,revision_id,'default' FROM workspace_preset_revisions`); err != nil {
				return fmt.Errorf("record Workspace build settings: %w", err)
			}
		}
		_, err := conn.ExecContext(ctx, workspaceBuildTriggers)
		return err
	})
}

// buildAllowedSQL is the build ceiling of preset revision p whose setting is
// the SQL expression setting: a Standard Workspace explicitly turned on.
func buildAllowedSQL(p, setting string) string {
	return fmt.Sprintf(`(%s.default_preset_id='standard' AND %s='on')`, p, setting)
}

// authorizeWorkspaceBuild requires both the parent's pinned revision and the
// Workspace's current revision to allow build workers, and no revocation of
// the pinned revision.
func authorizeWorkspaceBuild(ctx context.Context, conn *sql.Conn, scope memory.ScopeContext) error {
	var allowed bool
	err := conn.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM workspaces w
 JOIN workspace_preset_revisions pinned ON pinned.workspace_id=w.id AND pinned.revision_id=?
 JOIN workspace_build_settings pinned_build ON pinned_build.workspace_id=pinned.workspace_id AND pinned_build.revision_id=pinned.revision_id
 JOIN workspace_preset_revisions current ON current.workspace_id=w.id AND current.revision_id=w.current_revision_id
 JOIN workspace_build_settings current_build ON current_build.workspace_id=current.workspace_id AND current_build.revision_id=current.revision_id
 WHERE w.id=? AND w.lifecycle_state='active'
 AND `+buildAllowedSQL("pinned", "pinned_build.setting")+`
 AND `+buildAllowedSQL("current", "current_build.setting")+`
 AND NOT EXISTS(SELECT 1 FROM workspace_preset_revocations r WHERE r.workspace_id=w.id AND r.revision_id=pinned.revision_id AND r.preset_id=?))`,
		scope.WorkspaceRevision, scope.WorkspaceID, delegation.WorkerBuild).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%w: Workspace build delegation requires the owner's explicit permission: build workers are off by default, and the owner can turn them on with `evie workspaces build <workspace-id> on` or the Workspace API; a chat started before it was turned on, or one it was turned off for, needs a new chat", delegation.ErrAuthority)
	}
	return nil
}

// workspaceBuild reads revision's build setting and whether it allows build
// workers. A revision without a recorded setting allows nothing.
func workspaceBuild(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id memory.WorkspaceID, revision memory.WorkspaceRevisionID) (memory.WorkspaceBuild, error) {
	var build memory.WorkspaceBuild
	err := q.QueryRowContext(ctx, `SELECT COALESCE(b.setting,'off'),`+buildAllowedSQL("p", "COALESCE(b.setting,'off')")+`
 FROM workspace_preset_revisions p LEFT JOIN workspace_build_settings b ON b.workspace_id=p.workspace_id AND b.revision_id=p.revision_id
 WHERE p.workspace_id=? AND p.revision_id=?`, id, revision).Scan(&build.Setting, &build.Allowed)
	if err != nil {
		return build, fmt.Errorf("read Workspace build setting: %w", err)
	}
	return build, nil
}

// SetWorkspaceBuild records the owner's build setting for a Standard
// Workspace as a new revision, like SetWorkspaceExplore: it applies to chats
// started afterwards. Any setting other than on (off, or the default, which
// is off) durably revokes every earlier revision that was on, so no later
// setting restores those chats.
func (s *Store) SetWorkspaceBuild(ctx context.Context, id memory.WorkspaceID, expected memory.WorkspaceRevisionID, setting memory.WorkspaceBuildSetting) (memory.Workspace, error) {
	if !setting.Valid() {
		return memory.Workspace{}, ErrWorkspaceBuildSetting
	}
	var workspace memory.Workspace
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var explore memory.WorkspaceExploreSetting
		var err error
		workspace, explore, err = lockWorkspaceRevision(ctx, conn, id, expected)
		if err != nil {
			return err
		}
		if workspace.DefaultPresetID != "standard" {
			return ErrWorkspaceBuildPreset
		}
		current, err := workspaceBuild(ctx, conn, id, expected)
		if err != nil {
			return err
		}
		if current.Setting == setting {
			return nil
		}
		now := s.now().UTC()
		if setting != memory.WorkspaceBuildOn {
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_preset_revocations(workspace_id,revision_id,preset_id,revoked_at)
 SELECT workspace_id,revision_id,?,? FROM workspace_build_settings WHERE workspace_id=? AND setting='on'`,
				delegation.WorkerBuild, now.Format(time.RFC3339Nano), id); err != nil {
				return err
			}
		}
		return insertWorkspaceRevision(ctx, conn, &workspace, expected, explore, setting, now)
	})
	if err != nil {
		return memory.Workspace{}, err
	}
	return s.completeWorkspace(ctx, workspace)
}
