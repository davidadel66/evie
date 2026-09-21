package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestWorkspaceCreationReconcilesCommitBeforeCleaningFolder(t *testing.T) {
	for _, tc := range []struct {
		name            string
		landed          bool
		cancelCaller    bool
		unavailableRead bool
	}{
		{name: "committed after error", landed: true},
		{name: "committed with cancelled caller", landed: true, cancelCaller: true},
		{name: "proven rollback"},
		{name: "uncertain read retains folder", landed: true, unavailableRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			store := NewStore(db)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			unavailableDB := newTestDB(t)
			if err := unavailableDB.Close(); err != nil {
				t.Fatal(err)
			}
			store.resolveImmediateTransaction = func(resolveCtx context.Context, conn *sql.Conn, statement string) (sql.Result, error) {
				if statement != "COMMIT" {
					return executeImmediateTransactionStatement(resolveCtx, conn, statement)
				}
				var result sql.Result
				if tc.landed {
					var err error
					result, err = executeImmediateTransactionStatement(resolveCtx, conn, statement)
					if err != nil {
						return result, err
					}
				}
				if tc.cancelCaller {
					cancel()
				}
				if tc.unavailableRead {
					store.db = unavailableDB
				}
				return result, errors.New("synthetic ambiguous COMMIT result")
			}
			folderPath := filepath.Join(t.TempDir(), "workspace")
			workspace, err := store.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Recovered", PresetID: "research", FolderPath: folderPath, CreateFolder: true})
			if tc.unavailableRead {
				if !errors.Is(err, ErrWorkspaceCreationUncertain) {
					t.Fatalf("uncertain result = %+v, %v", workspace, err)
				}
			} else if tc.landed {
				if err != nil || workspace.ID == "" {
					t.Fatalf("landed commit reported failure = %+v, %v", workspace, err)
				}
			} else if err == nil {
				t.Fatal("rollback reported success")
			}
			store.db = db
			listed, listErr := store.ListWorkspaces(context.Background(), false)
			if listErr != nil {
				t.Fatal(listErr)
			}
			if tc.landed {
				if len(listed) != 1 || listed[0].DefaultPresetID != "research" || listed[0].Folder.Revision != 1 {
					t.Fatalf("persisted Workspace = %+v", listed)
				}
				if stat, statErr := os.Stat(folderPath); statErr != nil || !stat.IsDir() {
					t.Fatalf("persisted folder removed: %v, %v", stat, statErr)
				}
				if !tc.unavailableRead && !reflect.DeepEqual(workspace, listed[0]) {
					t.Fatalf("recovered result = %+v, want %+v", workspace, listed[0])
				}
			} else {
				if len(listed) != 0 {
					t.Fatalf("rollback persisted Workspace: %+v", listed)
				}
				if _, statErr := os.Stat(folderPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("rollback left folder: %v", statErr)
				}
			}
		})
	}
}

func TestWorkspaceCreationPersistsPresetAndFolderTogether(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	folder := filepath.Join(t.TempDir(), "new-workspace")
	ws, err := store.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Study", PresetID: "research", FolderPath: folder, CreateFolder: true})
	if err != nil {
		t.Fatal(err)
	}
	if ws.DefaultPresetID != "research" || !reflect.DeepEqual(ws.AllowedPresetIDs, []string{"research"}) || ws.Folder.Revision != 1 {
		t.Fatalf("created workspace = %+v", ws)
	}
	if stat, err := os.Stat(folder); err != nil || !stat.IsDir() {
		t.Fatalf("created folder = %v, %v", stat, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	listed, err := store.ListWorkspaces(ctx, false)
	if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0], ws) {
		t.Fatalf("reopened = %+v, %v", listed, err)
	}
	if preset, err := store.WorkspaceDefaultPreset(ctx, ws.ID, ws.CurrentRevisionID); err != nil || preset != "research" {
		t.Fatalf("default = %q, %v", preset, err)
	}
	if _, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, standardReceipt(t)); !errors.Is(err, ErrWorkspacePresetNotAllowed) {
		t.Fatalf("broader receipt accepted: %v", err)
	}
	assertSessionCount(t, db, 0)
	receipt := standardReceipt(t)
	receipt.Preset.ID = "research"
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, receipt)
	if err != nil || session.WorkspaceRevisionSnapshot != ws.CurrentRevisionID {
		t.Fatalf("allowed session = %+v, %v", session, err)
	}
	if _, err := db.Exec(`UPDATE workspace_preset_revisions SET default_preset_id='standard'`); err == nil {
		t.Fatal("preset revision mutated")
	}
}

func TestWorkspaceCreationPreservesChosenFolderWhitespace(t *testing.T) {
	store := NewStore(newTestDB(t))
	path := filepath.Join(t.TempDir(), " selected folder \t")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspaceWithOptions(context.Background(), WorkspaceRegistration{DisplayName: "Exact folder", FolderPath: path})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := memory.CanonicalProjectRoot(path)
	if err != nil || workspace.Folder.Path != canonical {
		t.Fatalf("selected path changed: %q, want %q (%v)", workspace.Folder.Path, canonical, err)
	}
}

func TestWorkspaceCreationRejectsInvalidFoldersWithoutPartialWorkspace(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	parent := t.TempDir()
	file := filepath.Join(parent, "existing-file")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []WorkspaceRegistration{
		{FolderPath: "relative"}, {FolderPath: file}, {FolderPath: parent, CreateFolder: true},
		{FolderPath: filepath.Join(parent, "missing", "child"), CreateFolder: true}, {CreateFolder: true},
	} {
		if _, err := store.RegisterWorkspaceWithOptions(ctx, tc); !errors.Is(err, ErrWorkspaceFolderInvalid) {
			t.Fatalf("options %+v: %v", tc, err)
		}
	}
	listed, err := store.ListWorkspaces(ctx, false)
	if err != nil || len(listed) != 0 {
		t.Fatalf("partial Workspaces = %+v, %v", listed, err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "keep" {
		t.Fatalf("existing file changed: %q, %v", data, err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_initial_folder BEFORE INSERT ON workspace_folder_revisions BEGIN SELECT RAISE(ABORT,'test folder persistence failure'); END`); err != nil {
		t.Fatal(err)
	}
	createdPath := filepath.Join(parent, "rollback")
	if _, err := store.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Rollback", FolderPath: createdPath, CreateFolder: true}); err == nil {
		t.Fatal("persistence failure accepted")
	}
	if _, err := os.Stat(createdPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed creation left folder: %v", err)
	}
	listed, err = store.ListWorkspaces(ctx, false)
	if err != nil || len(listed) != 0 {
		t.Fatalf("failed creation left Workspace: %+v, %v", listed, err)
	}
	var presets int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspace_preset_revisions`).Scan(&presets); err != nil || presets != 0 {
		t.Fatalf("orphan presets = %d, %v", presets, err)
	}
}

func TestWorkspacePresetUpgradeKeepsExistingSessionsAndDefaultsToStandard(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "Existing")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE workspace_preset_revisions`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	listed, err := store.ListWorkspaces(ctx, false)
	if err != nil || len(listed) != 1 || listed[0].DefaultPresetID != "standard" || !reflect.DeepEqual(listed[0].AllowedPresetIDs, []string{"standard"}) {
		t.Fatalf("upgraded = %+v, %v", listed, err)
	}
	stored, err := store.GetActiveSession(ctx, session.ID)
	if err != nil || stored != session || stored.WorkspaceID != memory.WorkspaceID(ws.ID) {
		t.Fatalf("existing session changed = %+v, %v", stored, err)
	}
}
