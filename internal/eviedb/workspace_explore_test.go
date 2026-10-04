package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// authorizeExplore runs the explore ceiling check every admission, start,
// child write and continuation runs, for a chat pinned to revision.
func authorizeExplore(t *testing.T, s *Store, id memory.WorkspaceID, revision memory.WorkspaceRevisionID) error {
	t.Helper()
	ctx := context.Background()
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		return authorizeWorkspacePreset(ctx, conn, memory.ScopeContext{WorkspaceID: id, WorkspaceRevision: revision}, delegation.WorkerExplore)
	})
}

func wantExplore(t *testing.T, got memory.Workspace, setting memory.WorkspaceExploreSetting, allowed bool) {
	t.Helper()
	if got.Explore.Setting != setting || got.Explore.Allowed != allowed {
		t.Fatalf("explore = %+v, want setting %q allowed %v", got.Explore, setting, allowed)
	}
}

func listedWorkspace(t *testing.T, s *Store, id memory.WorkspaceID) memory.Workspace {
	t.Helper()
	listed, err := s.ListWorkspaces(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range listed {
		if w.ID == id {
			return w
		}
	}
	t.Fatalf("Workspace %s not listed", id)
	return memory.Workspace{}
}

// With no explicit setting, a Workspace allows explore exactly while it has a
// local folder: attaching one enables it for chats pinned to that default
// revision, detaching disables it at once, and no revision is written.
func TestWorkspaceExploreDefaultFollowsTheFolder(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w, err := s.RegisterWorkspace(ctx, "No folder")
	if err != nil {
		t.Fatal(err)
	}
	wantExplore(t, w, memory.WorkspaceExploreDefault, false)
	wantExplore(t, listedWorkspace(t, s, w.ID), memory.WorkspaceExploreDefault, false)
	if err := authorizeExplore(t, s, w.ID, w.CurrentRevisionID); !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("default without a folder allowed explore: %v", err)
	}
	folder := t.TempDir()
	if _, err := s.SetWorkspaceFolder(ctx, w.ID, 0, folder); err != nil {
		t.Fatal(err)
	}
	attached := listedWorkspace(t, s, w.ID)
	wantExplore(t, attached, memory.WorkspaceExploreDefault, true)
	if attached.CurrentRevisionID != w.CurrentRevisionID {
		t.Fatal("attaching a folder wrote a Workspace revision")
	}
	if err := authorizeExplore(t, s, w.ID, w.CurrentRevisionID); err != nil {
		t.Fatalf("default with a folder refused explore: %v", err)
	}
	if _, err := s.SetWorkspaceFolder(ctx, w.ID, 1, ""); err != nil {
		t.Fatal(err)
	}
	wantExplore(t, listedWorkspace(t, s, w.ID), memory.WorkspaceExploreDefault, false)
	if err := authorizeExplore(t, s, w.ID, w.CurrentRevisionID); !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("detaching the folder left explore allowed: %v", err)
	}

	// A Workspace created with a folder allows explore from the start; one
	// whose default preset cannot delegate never does.
	withFolder, err := s.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Folder", FolderPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	wantExplore(t, withFolder, memory.WorkspaceExploreDefault, true)
	if err := authorizeExplore(t, s, withFolder.ID, withFolder.CurrentRevisionID); err != nil {
		t.Fatalf("new Workspace with a folder refused explore: %v", err)
	}
	research, err := s.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Reader", PresetID: "research", FolderPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	wantExplore(t, research, memory.WorkspaceExploreDefault, false)
	if err := authorizeExplore(t, s, research.ID, research.CurrentRevisionID); !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("Research-preset Workspace allowed explore: %v", err)
	}
	if _, err := s.SetWorkspaceExplore(ctx, research.ID, research.CurrentRevisionID, memory.WorkspaceExploreOn); !errors.Is(err, ErrWorkspaceExplorePreset) {
		t.Fatalf("explore on for a Research-preset Workspace: %v", err)
	}
}

// Explicit settings are new revisions that survive a reopen and ignore the
// folder. Turning explore off durably revokes every earlier revision that
// allowed it or followed the folder; turning it on or back to the default
// applies to chats that start afterwards.
func TestWorkspaceExploreExplicitSettingsPersistAndRevoke(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	s := NewStore(db)
	w, err := s.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Code", FolderPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetWorkspaceExplore(ctx, w.ID, w.CurrentRevisionID, "sometimes"); !errors.Is(err, ErrWorkspaceExploreSetting) {
		t.Fatalf("invalid setting: %v", err)
	}
	noop, err := s.SetWorkspaceExplore(ctx, w.ID, w.CurrentRevisionID, memory.WorkspaceExploreDefault)
	if err != nil || noop.CurrentRevisionID != w.CurrentRevisionID {
		t.Fatalf("no-op default = %+v, %v", noop, err)
	}
	off, err := s.SetWorkspaceExplore(ctx, w.ID, w.CurrentRevisionID, memory.WorkspaceExploreOff)
	if err != nil {
		t.Fatal(err)
	}
	wantExplore(t, off, memory.WorkspaceExploreOff, false)
	if off.CurrentRevisionID == w.CurrentRevisionID || off.Folder.Path == "" {
		t.Fatalf("explicit off = %+v", off)
	}
	if _, err := s.SetWorkspaceExplore(ctx, w.ID, w.CurrentRevisionID, memory.WorkspaceExploreOn); !errors.Is(err, ErrChooserStateChanged) {
		t.Fatalf("stale explore change: %v", err)
	}
	for _, revision := range []memory.WorkspaceRevisionID{w.CurrentRevisionID, off.CurrentRevisionID} {
		if err := authorizeExplore(t, s, w.ID, revision); !errors.Is(err, delegation.ErrAuthority) {
			t.Fatalf("explicit off left revision %s allowed: %v", revision, err)
		}
	}
	// A research change carries the explore setting into its new revision.
	researched, err := s.SetWorkspaceResearch(ctx, w.ID, off.CurrentRevisionID, true)
	if err != nil {
		t.Fatal(err)
	}
	wantExplore(t, researched, memory.WorkspaceExploreOff, false)

	back, err := s.SetWorkspaceExplore(ctx, w.ID, researched.CurrentRevisionID, memory.WorkspaceExploreDefault)
	if err != nil {
		t.Fatal(err)
	}
	wantExplore(t, back, memory.WorkspaceExploreDefault, true)
	if err := authorizeExplore(t, s, w.ID, back.CurrentRevisionID); err != nil {
		t.Fatalf("chat started after returning to the default: %v", err)
	}
	if err := authorizeExplore(t, s, w.ID, w.CurrentRevisionID); !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("returning to the default resurrected a revoked chat: %v", err)
	}

	// Explicit on survives detaching the folder and a reopen.
	on, err := s.SetWorkspaceExplore(ctx, w.ID, back.CurrentRevisionID, memory.WorkspaceExploreOn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetWorkspaceFolder(ctx, w.ID, on.Folder.Revision, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if db, err = OpenDBAt(path); err != nil {
		t.Fatal(err)
	}
	s = NewStore(db)
	reopened := listedWorkspace(t, s, w.ID)
	wantExplore(t, reopened, memory.WorkspaceExploreOn, true)
	if reopened.CurrentRevisionID != on.CurrentRevisionID {
		t.Fatalf("reopen changed the revision: %s", reopened.CurrentRevisionID)
	}
	for revision, want := range map[memory.WorkspaceRevisionID]bool{on.CurrentRevisionID: true, back.CurrentRevisionID: false, w.CurrentRevisionID: false} {
		if err := authorizeExplore(t, s, w.ID, revision); want && err != nil || !want && !errors.Is(err, delegation.ErrAuthority) {
			t.Fatalf("revision %s allowed=%v after reopen: %v", revision, want, err)
		}
	}
	// Explicit off survives attaching a folder.
	off, err = s.SetWorkspaceExplore(ctx, w.ID, on.CurrentRevisionID, memory.WorkspaceExploreOff)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetWorkspaceFolder(ctx, w.ID, reopened.Folder.Revision, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	wantExplore(t, listedWorkspace(t, s, w.ID), memory.WorkspaceExploreOff, false)
	if err := authorizeExplore(t, s, w.ID, off.CurrentRevisionID); !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("explicit off allowed explore once a folder was attached: %v", err)
	}
}

// Workspaces that existed before the explore default keep what their owner
// chose: a revision that allowed explore stays explicitly on, and a Workspace
// whose explore was revoked stays explicitly off. Untouched Workspaces get
// the default. No revision or allowance list is rewritten, and the migration
// runs once.
func TestWorkspaceExploreMigrationKeepsExplicitSettings(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	s := NewStore(db)
	untouched, err := s.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Untouched", FolderPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := s.RegisterWorkspace(ctx, "Allowed")
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := s.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Revoked", FolderPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// Stage 7 as merged: SetWorkspaceExplore listed the explore preset in
	// the revision's allowance, and revoking it recorded a revocation.
	legacy := func(id memory.WorkspaceID, revision, presets string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `INSERT INTO workspace_preset_revisions(workspace_id,revision_id,default_preset_id,allowed_preset_ids) VALUES(?,?,'standard',?)`, id, revision, presets); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE workspaces SET current_revision_id=? WHERE id=?`, revision, id); err != nil {
			t.Fatal(err)
		}
	}
	legacy(allowed.ID, "allowed-on", `["standard","explore"]`)
	legacy(revoked.ID, "revoked-on", `["standard","explore"]`)
	legacy(revoked.ID, "revoked-off", `["standard","research"]`)
	if _, err := db.ExecContext(ctx, `INSERT INTO workspace_preset_revocations(workspace_id,revision_id,preset_id,revoked_at) VALUES(?,'revoked-on','explore','2026-10-03T00:00:00Z')`, revoked.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE workspace_explore_settings`); err != nil {
		t.Fatal(err)
	}
	var revisions int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_preset_revisions`).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	reopen := func() {
		t.Helper()
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if db, err = OpenDBAt(path); err != nil {
			t.Fatal(err)
		}
		s = NewStore(db)
	}
	reopen()
	wantExplore(t, listedWorkspace(t, s, untouched.ID), memory.WorkspaceExploreDefault, true)
	wantExplore(t, listedWorkspace(t, s, allowed.ID), memory.WorkspaceExploreOn, true)
	wantExplore(t, listedWorkspace(t, s, revoked.ID), memory.WorkspaceExploreOff, false)
	for _, tc := range []struct {
		id       memory.WorkspaceID
		revision memory.WorkspaceRevisionID
		want     bool
	}{
		{untouched.ID, untouched.CurrentRevisionID, true},
		{allowed.ID, "allowed-on", true},
		{allowed.ID, allowed.CurrentRevisionID, false}, // pinned before it was allowed, no folder
		{revoked.ID, "revoked-on", false},
		{revoked.ID, revoked.CurrentRevisionID, false},
	} {
		if err := authorizeExplore(t, s, tc.id, tc.revision); tc.want && err != nil || !tc.want && !errors.Is(err, delegation.ErrAuthority) {
			t.Fatalf("migrated revision %s allowed=%v: %v", tc.revision, tc.want, err)
		}
	}
	var after int
	var presets string
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*),(SELECT allowed_preset_ids FROM workspace_preset_revisions WHERE revision_id='revoked-on') FROM workspace_preset_revisions`).Scan(&after, &presets); err != nil {
		t.Fatal(err)
	}
	if after != revisions || presets != `["standard","explore"]` {
		t.Fatalf("migration rewrote revisions: %d→%d, %s", revisions, after, presets)
	}
	// The migration does not run again: a default chosen afterwards in a
	// Workspace with an old revocation stays the default.
	reset, err := s.SetWorkspaceExplore(ctx, revoked.ID, "revoked-off", memory.WorkspaceExploreDefault)
	if err != nil {
		t.Fatal(err)
	}
	reopen()
	wantExplore(t, listedWorkspace(t, s, revoked.ID), memory.WorkspaceExploreDefault, true)
	if err := authorizeExplore(t, s, revoked.ID, reset.CurrentRevisionID); err != nil {
		t.Fatalf("default after the migration: %v", err)
	}
}
