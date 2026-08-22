package eviedb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestSessionStoreCreatesGlobalAndProjectScopes(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create global session: %v", err)
	}
	if globalSession.ID == "" || globalSession.ProjectID != "" ||
		globalSession.ProjectRootSnapshot != "" || globalSession.ParentSessionID != "" ||
		globalSession.Status != memory.SessionActive {
		t.Errorf("global session = %+v", globalSession)
	}
	if globalSession.CreatedAt.IsZero() || globalSession.CreatedAt.Location() != time.UTC ||
		globalSession.UpdatedAt != globalSession.CreatedAt {
		t.Errorf("global session timestamps = created %v, updated %v", globalSession.CreatedAt, globalSession.UpdatedAt)
	}
	if scope := globalSession.ScopeContext(); scope.OwnerID != memory.LocalOwnerID ||
		scope.SessionID != globalSession.ID || scope.ProjectID != "" || scope.ProjectRoot != "" {
		t.Errorf("global scope = %+v", scope)
	}

	project, err := store.RegisterProject(ctx, "Evie", t.TempDir())
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
	oldRoot := project.CanonicalRoot

	oldSession, err := store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatalf("create project session: %v", err)
	}
	if oldSession.ProjectID != project.ID || oldSession.ProjectRootSnapshot != oldRoot ||
		oldSession.Status != memory.SessionActive {
		t.Errorf("project session = %+v", oldSession)
	}

	relocated, err := store.RelocateProject(ctx, project.ID, t.TempDir())
	if err != nil {
		t.Fatalf("relocate project: %v", err)
	}
	newSession, err := store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatalf("create session after relocation: %v", err)
	}
	if newSession.ProjectRootSnapshot != relocated.CanonicalRoot ||
		newSession.ProjectRootSnapshot == oldSession.ProjectRootSnapshot {
		t.Errorf("new session root = %q, old = %q, relocated = %q",
			newSession.ProjectRootSnapshot, oldSession.ProjectRootSnapshot, relocated.CanonicalRoot)
	}

	loadedOldSession, err := store.GetSession(ctx, oldSession.ID)
	if err != nil {
		t.Fatalf("load old session: %v", err)
	}
	if loadedOldSession != oldSession {
		t.Errorf("loaded old session = %+v, want %+v", loadedOldSession, oldSession)
	}
}

func TestCreateProjectSessionRejectsUnavailableProject(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if _, err := store.CreateProjectSession(ctx, memory.ProjectID("missing")); err == nil {
		t.Error("session creation for missing project succeeded")
	}

	project, err := store.RegisterProject(ctx, "Archived", t.TempDir())
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
	if _, err := db.Exec(`UPDATE projects SET archived = 1 WHERE id = ?`, project.ID); err != nil {
		t.Fatalf("archive project fixture: %v", err)
	}
	if _, err := store.CreateProjectSession(ctx, project.ID); err == nil {
		t.Error("session creation for archived project succeeded")
	}
}

func TestCreateProjectSessionFromCanonicalRootLookupFreezesScope(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	root := t.TempDir()
	project, err := store.RegisterProject(ctx, "Evie", root)
	if err != nil {
		t.Fatalf("register project: %v", err)
	}

	alias := filepath.Join(t.TempDir(), "evie-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatalf("create root alias: %v", err)
	}
	discovered, err := store.FindActiveProjectByRoot(ctx, alias)
	if err != nil {
		t.Fatalf("discover project from launch root: %v", err)
	}

	session, err := store.CreateProjectSession(ctx, discovered.ID)
	if err != nil {
		t.Fatalf("create discovered project session: %v", err)
	}
	if session.ProjectID != project.ID || session.ProjectRootSnapshot != project.CanonicalRoot {
		t.Fatalf("created session = %+v, want project %q rooted at %q",
			session, project.ID, project.CanonicalRoot)
	}

	scope := session.ScopeContext()
	if scope.ProjectID != project.ID || scope.ProjectRoot != project.CanonicalRoot {
		t.Errorf("scope = %+v, want frozen project %q rooted at %q",
			scope, project.ID, project.CanonicalRoot)
	}
}
