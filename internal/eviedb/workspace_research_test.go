package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

func TestWorkspaceResearchOptInPreservesPinsAndRejectsStaleChanges(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	s := NewStore(db)
	w, err := s.RegisterWorkspace(ctx, "Research")
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := s.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if enabled.CurrentRevisionID == w.CurrentRevisionID || enabled.DefaultPresetID != "standard" || !reflect.DeepEqual(enabled.AllowedPresetIDs, []string{"standard", "research"}) {
		t.Fatalf("enabled=%+v", enabled)
	}
	if _, err := s.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, false); !errors.Is(err, ErrChooserStateChanged) {
		t.Fatalf("stale update=%v", err)
	}
	unchanged, err := s.GetSession(ctx, old.ID)
	if err != nil || unchanged.WorkspaceRevisionSnapshot != w.CurrentRevisionID {
		t.Fatalf("old pin=%+v error=%v", unchanged, err)
	}
	noop, err := s.SetWorkspaceResearch(ctx, w.ID, enabled.CurrentRevisionID, true)
	if err != nil || noop.CurrentRevisionID != enabled.CurrentRevisionID {
		t.Fatalf("no-op=%+v error=%v", noop, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	s = NewStore(db)
	listed, err := s.ListWorkspaces(ctx, false)
	if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0].AllowedPresetIDs, enabled.AllowedPresetIDs) {
		t.Fatalf("reopened=%+v error=%v", listed, err)
	}
	disabled, err := s.SetWorkspaceResearch(ctx, w.ID, enabled.CurrentRevisionID, false)
	if err != nil || !reflect.DeepEqual(disabled.AllowedPresetIDs, []string{"standard"}) {
		t.Fatalf("disabled=%+v error=%v", disabled, err)
	}
	reenabled, err := s.SetWorkspaceResearch(ctx, w.ID, disabled.CurrentRevisionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	s = NewStore(db)
	for _, tc := range []struct {
		revision    memory.WorkspaceRevisionID
		wantAllowed bool
	}{{enabled.CurrentRevisionID, false}, {reenabled.CurrentRevisionID, true}} {
		err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
			return authorizeWorkspaceResearch(ctx, conn, memory.ScopeContext{WorkspaceID: w.ID, WorkspaceRevision: tc.revision})
		})
		if tc.wantAllowed && err != nil || !tc.wantAllowed && !errors.Is(err, delegation.ErrAuthority) {
			t.Fatalf("reopened revision allowed=%v: %v", tc.wantAllowed, err)
		}
	}
}
