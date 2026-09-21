package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

var ErrWorkspaceNotFound = errors.New("eviedb: workspace not found")
var ErrWorkspaceCreationUncertain = errors.New("Workspace creation could not be confirmed; refresh Workspaces before trying again")

func (s *Store) RegisterWorkspace(ctx context.Context, displayName string) (memory.Workspace, error) {
	return s.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: displayName})
}

// WorkspaceRegistration is an owner-selected initial configuration. Preset
// availability is validated by the runtime before persistence.
type WorkspaceRegistration struct {
	DisplayName  string `json:"displayName"`
	PresetID     string `json:"presetId,omitempty"`
	FolderPath   string `json:"folderPath,omitempty"`
	CreateFolder bool   `json:"createFolder,omitempty"`
}

func (s *Store) RegisterWorkspaceWithOptions(ctx context.Context, options WorkspaceRegistration) (memory.Workspace, error) {
	presetID := strings.TrimSpace(options.PresetID)
	if presetID == "" {
		presetID = "standard"
	}
	if !composition.ValidIdentity(presetID) {
		return memory.Workspace{}, errors.New("invalid Agent Preset ID")
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("generate Workspace ID: %w", err)
	}
	revisionID, err := uuid.NewRandom()
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("generate initial Workspace revision ID: %w", err)
	}
	now := s.now().UTC()
	workspace := memory.Workspace{
		ID:                memory.WorkspaceID(id.String()),
		Instructions:      memory.RepositoryInstructionSettings{Enabled: true},
		DisplayName:       memory.WorkspaceDisplayLabel(options.DisplayName, now),
		DefaultPresetID:   presetID,
		AllowedPresetIDs:  []string{presetID},
		State:             memory.WorkspaceActive,
		CurrentRevisionID: memory.WorkspaceRevisionID(revisionID.String()),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	folder, cleanup, err := prepareWorkspaceFolder(options.FolderPath, options.CreateFolder)
	if err != nil {
		return memory.Workspace{}, err
	}
	retainFolder := false
	defer func() { cleanup(retainFolder) }()
	workspace.Folder = folder
	allowed, err := json.Marshal(workspace.AllowedPresetIDs)
	if err != nil {
		return memory.Workspace{}, err
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, `
		INSERT INTO workspaces (
			id, display_name, lifecycle_state, current_revision_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, workspace.ID, workspace.DisplayName, workspace.State, workspace.CurrentRevisionID,
			workspace.CreatedAt.Format(time.RFC3339Nano), workspace.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert Workspace: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_preset_revisions(workspace_id,revision_id,default_preset_id,allowed_preset_ids)
 VALUES(?,?,?,?)`, workspace.ID, workspace.CurrentRevisionID, presetID, string(allowed)); err != nil {
			return fmt.Errorf("insert Workspace Agent Preset: %w", err)
		}
		if folder.Path != "" {
			if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_folder_revisions(workspace_id,revision,path,recorded_at)
 VALUES(?,1,?,?)`, workspace.ID, folder.Path, now.Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("insert Workspace folder: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		// COMMIT or connection close can report an error after the transaction
		// lands. Only remove our directory when a fresh read proves no Workspace
		// exists; cancellation must not prevent this bounded reconciliation.
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var persisted bool
		readErr := s.db.QueryRowContext(readCtx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id=?)`, workspace.ID).Scan(&persisted)
		if readErr != nil {
			retainFolder = true
			return memory.Workspace{}, errors.Join(ErrWorkspaceCreationUncertain, err, readErr)
		}
		if !persisted {
			return memory.Workspace{}, err
		}
	}
	// Both a clean commit and a confirmed commit return the original result.
	retainFolder = true
	return workspace, nil
}

func (s *Store) ListWorkspaces(ctx context.Context, includeArchived bool) ([]memory.Workspace, error) {
	query := `SELECT id, display_name, lifecycle_state, current_revision_id, created_at, updated_at FROM workspaces`
	if !includeArchived {
		query += ` WHERE lifecycle_state = 'active'`
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query Workspaces: %w", err)
	}
	defer rows.Close()
	var workspaces []memory.Workspace
	for rows.Next() {
		workspace, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Workspaces: %w", err)
	}
	// Close the cursor before additional reads: tests and embedded hosts may
	// deliberately use one SQLite connection.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range workspaces {
		folder, err := s.WorkspaceFolder(ctx, workspaces[i].ID)
		if err != nil {
			return nil, err
		}
		workspaces[i].Folder = folder
		settings, err := s.RepositoryInstructionSettings(ctx, workspaces[i].ID)
		if err != nil {
			return nil, err
		}
		workspaces[i].Instructions = settings
		if err := s.loadWorkspacePresets(ctx, &workspaces[i]); err != nil {
			return nil, err
		}
	}
	return workspaces, nil
}

func (s *Store) RenameWorkspace(ctx context.Context, id memory.WorkspaceID, displayName string) (memory.Workspace, error) {
	now := s.now().UTC()
	displayName = memory.WorkspaceDisplayLabel(displayName, now)
	workspace, err := scanWorkspace(s.db.QueryRowContext(ctx, `
		UPDATE workspaces SET display_name = ?, updated_at = ? WHERE id = ?
		RETURNING id, display_name, lifecycle_state, current_revision_id, created_at, updated_at
	`, displayName, now.Format(time.RFC3339Nano), id))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Workspace{}, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
	}
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("rename Workspace: %w", err)
	}
	settings, err := s.RepositoryInstructionSettings(ctx, workspace.ID)
	if err != nil {
		return memory.Workspace{}, err
	}
	workspace.Instructions = settings
	if err := s.loadWorkspacePresets(ctx, &workspace); err != nil {
		return memory.Workspace{}, err
	}
	return workspace, nil
}

func (s *Store) ArchiveWorkspace(ctx context.Context, id memory.WorkspaceID) (memory.Workspace, error) {
	now := s.now().UTC()
	workspace, err := scanWorkspace(s.db.QueryRowContext(ctx, `
		UPDATE workspaces SET lifecycle_state = ?, updated_at = ? WHERE id = ?
		RETURNING id, display_name, lifecycle_state, current_revision_id, created_at, updated_at
	`, memory.WorkspaceArchived, now.Format(time.RFC3339Nano), id))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Workspace{}, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
	}
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("archive Workspace: %w", err)
	}
	settings, err := s.RepositoryInstructionSettings(ctx, workspace.ID)
	if err != nil {
		return memory.Workspace{}, err
	}
	workspace.Instructions = settings
	if err := s.loadWorkspacePresets(ctx, &workspace); err != nil {
		return memory.Workspace{}, err
	}
	return workspace, nil
}

func scanWorkspace(scanner rowScanner) (memory.Workspace, error) {
	var id, displayName, state, revisionID, createdText, updatedText string
	if err := scanner.Scan(&id, &displayName, &state, &revisionID, &createdText, &updatedText); err != nil {
		return memory.Workspace{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("parse Workspace created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedText)
	if err != nil {
		return memory.Workspace{}, fmt.Errorf("parse Workspace updated_at: %w", err)
	}
	return memory.Workspace{
		ID: memory.WorkspaceID(id), DisplayName: displayName, State: memory.WorkspaceState(state),
		CurrentRevisionID: memory.WorkspaceRevisionID(revisionID), CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}
