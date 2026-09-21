package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/repoinstructions"
)

var ErrRepositoryInstructionsChanged = memory.ErrRepositoryInstructionsChanged

func ensureRepositoryInstructions(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workspace_instruction_settings (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), revision INTEGER NOT NULL CHECK(revision>0), enabled INTEGER NOT NULL CHECK(enabled IN(0,1)), PRIMARY KEY(workspace_id,revision));
 CREATE TABLE IF NOT EXISTS repository_instruction_snapshots (
 session_id TEXT NOT NULL, turn_id TEXT NOT NULL, snapshot_json TEXT NOT NULL CHECK(json_valid(snapshot_json)),
 PRIMARY KEY(session_id,turn_id), FOREIGN KEY(turn_id,session_id) REFERENCES events(id,session_id));
 CREATE TRIGGER IF NOT EXISTS repository_instruction_snapshots_no_update BEFORE UPDATE ON repository_instruction_snapshots BEGIN SELECT RAISE(ABORT,'instruction snapshots are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS repository_instruction_snapshots_no_delete BEFORE DELETE ON repository_instruction_snapshots BEGIN SELECT RAISE(ABORT,'instruction snapshots are immutable'); END;`)
	return err
}
func (s *Store) RepositoryInstructionSettings(ctx context.Context, id memory.WorkspaceID) (memory.RepositoryInstructionSettings, error) {
	var result memory.RepositoryInstructionSettings
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT enabled FROM workspace_instruction_settings WHERE workspace_id=w.id ORDER BY revision DESC LIMIT 1),1), COALESCE((SELECT MAX(revision) FROM workspace_instruction_settings WHERE workspace_id=w.id),0) FROM workspaces w WHERE id=?`, id).Scan(&result.Enabled, &result.Revision)
	return result, err
}
func (s *Store) SetRepositoryInstructionSettings(ctx context.Context, id memory.WorkspaceID, expected int64, enabled bool) (memory.RepositoryInstructionSettings, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO workspace_instruction_settings(workspace_id,revision,enabled) SELECT id,?,? FROM workspaces WHERE id=? AND lifecycle_state='active' AND COALESCE((SELECT MAX(revision) FROM workspace_instruction_settings WHERE workspace_id=?),0)=?`, expected+1, enabled, id, id, expected)
	if err != nil {
		return memory.RepositoryInstructionSettings{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return memory.RepositoryInstructionSettings{}, err
	}
	if n != 1 {
		return memory.RepositoryInstructionSettings{}, ErrRepositoryInstructionsChanged
	}
	return memory.RepositoryInstructionSettings{Enabled: enabled, Revision: expected + 1}, nil
}
func (s *Store) PreviewRepositoryInstructions(ctx context.Context, id memory.WorkspaceID) (memory.RepositoryInstructionSnapshot, error) {
	folder, err := s.WorkspaceFolder(ctx, id)
	if err != nil {
		return memory.RepositoryInstructionSnapshot{}, err
	}
	settings, err := s.RepositoryInstructionSettings(ctx, id)
	if err != nil {
		return memory.RepositoryInstructionSnapshot{}, err
	}
	return repoinstructions.Load(id, folder, settings), nil
}
func (s *Store) RepositoryInstructionSnapshot(ctx context.Context, session memory.SessionID, turn memory.EventID) (memory.RepositoryInstructionSnapshot, error) {
	var encoded string
	var prepared bool
	err := s.db.QueryRowContext(ctx, `SELECT snapshot_json,EXISTS(SELECT 1 FROM events WHERE session_id=? AND event_type='context_snapshot' AND json_extract(payload_json,'$.repository_instructions_turn_id')=?) FROM repository_instruction_snapshots WHERE session_id=? AND turn_id=?`, session, turn, session, turn).Scan(&encoded, &prepared)
	if err != nil {
		return memory.RepositoryInstructionSnapshot{}, err
	}
	var result memory.RepositoryInstructionSnapshot
	err = json.Unmarshal([]byte(encoded), &result)
	result.Prepared = prepared
	return result, err
}
func (h *SessionHistory) PreviewRepositoryInstructions(ctx context.Context) (memory.RepositoryInstructionSnapshot, error) {
	session, err := h.store.GetActiveSession(ctx, h.sessionID)
	if err != nil {
		return memory.RepositoryInstructionSnapshot{}, err
	}
	if session.WorkspaceID == "" || session.ParentSessionID != "" {
		return memory.RepositoryInstructionSnapshot{}, nil
	}
	return h.store.PreviewRepositoryInstructions(ctx, session.WorkspaceID)
}
func (h *SessionHistory) RepositoryInstructions(ctx context.Context, lease memory.TurnLease, turn memory.EventID) (memory.RepositoryInstructionSnapshot, error) {
	if err := validateBoundTurnLease(lease, h.sessionID, h.holderID); err != nil {
		return memory.RepositoryInstructionSnapshot{}, err
	}
	// Disk reads happen outside the write transaction. Recheck operational
	// revisions under the lease fence before accepting this snapshot.
	snapshot, err := h.PreviewRepositoryInstructions(ctx)
	if err != nil || snapshot.WorkspaceID == "" {
		return snapshot, err
	}
	snapshot.SessionID = h.sessionID
	snapshot.TurnID = turn
	snapshot.CapturedAt = h.store.now().UTC().Format(time.RFC3339Nano)
	err = h.store.withTurnLeaseWrite(ctx, h.sessionID, h.holderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		var encoded string
		err := writer.queryRowContext(ctx, `SELECT snapshot_json FROM repository_instruction_snapshots WHERE session_id=? AND turn_id=?`, h.sessionID, turn).Scan(&encoded)
		if err == nil {
			return json.Unmarshal([]byte(encoded), &snapshot)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var valid bool
		err = writer.queryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events e JOIN sessions s ON s.id=e.session_id WHERE e.id=? AND e.session_id=? AND e.event_type='user_message' AND e.role='user' AND e.parent_id IS NULL AND s.parent_session_id IS NULL AND s.workspace_id=?)`, turn, h.sessionID, snapshot.WorkspaceID).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return errors.New("Repository instructions require this session's root user turn")
		}
		var folderRevision, settingsRevision int64
		err = writer.queryRowContext(ctx, `SELECT COALESCE((SELECT MAX(revision) FROM workspace_folder_revisions WHERE workspace_id=?),0),COALESCE((SELECT MAX(revision) FROM workspace_instruction_settings WHERE workspace_id=?),0)`, snapshot.WorkspaceID, snapshot.WorkspaceID).Scan(&folderRevision, &settingsRevision)
		if err != nil {
			return err
		}
		if folderRevision != snapshot.Folder.Revision || settingsRevision != snapshot.Settings.Revision {
			return ErrRepositoryInstructionsChanged
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		_, err = writer.execContext(ctx, `INSERT INTO repository_instruction_snapshots(session_id,turn_id,snapshot_json)VALUES(?,?,?)`, h.sessionID, turn, string(data))
		return err
	})
	if err != nil {
		return memory.RepositoryInstructionSnapshot{}, fmt.Errorf("capture repository instructions: %w", err)
	}
	return snapshot, nil
}
