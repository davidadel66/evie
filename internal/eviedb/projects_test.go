package eviedb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectRegistryRegisterListAndRelocate(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	root := t.TempDir()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("canonicalize expected root: %v", err)
	}

	project, err := store.RegisterProject(ctx, "Evie", root)
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
	if project.ID == "" || project.DisplayName != "Evie" ||
		project.CanonicalRoot != canonicalRoot || project.Archived {
		t.Errorf("registered project = %+v", project)
	}
	if project.CreatedAt.IsZero() || project.UpdatedAt.IsZero() ||
		project.CreatedAt.Location() != time.UTC || project.UpdatedAt.Location() != time.UTC {
		t.Errorf("project timestamps are not nonzero UTC values: created=%v updated=%v", project.CreatedAt, project.UpdatedAt)
	}

	projects, err := store.ListProjects(ctx, false)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 1 || projects[0] != project {
		t.Fatalf("listed projects = %+v, want %+v", projects, project)
	}

	alias := filepath.Join(t.TempDir(), "evie-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatalf("create project alias: %v", err)
	}
	if _, err := store.RegisterProject(ctx, "Duplicate", alias); err == nil {
		t.Error("registering a symlink alias of an existing root succeeded")
	}

	newRoot := t.TempDir()
	canonicalNewRoot, err := filepath.EvalSymlinks(newRoot)
	if err != nil {
		t.Fatalf("canonicalize relocated root: %v", err)
	}
	relocated, err := store.RelocateProject(ctx, project.ID, newRoot)
	if err != nil {
		t.Fatalf("relocate project: %v", err)
	}
	if relocated.ID != project.ID || relocated.DisplayName != project.DisplayName ||
		relocated.CanonicalRoot != canonicalNewRoot || relocated.CreatedAt != project.CreatedAt ||
		!relocated.UpdatedAt.After(project.UpdatedAt) {
		t.Errorf("relocated project = %+v, original = %+v", relocated, project)
	}
}

func TestProjectRegistryListExcludesArchivedByDefault(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	active, err := store.RegisterProject(ctx, "Active", t.TempDir())
	if err != nil {
		t.Fatalf("register active project: %v", err)
	}
	archived, err := store.RegisterProject(ctx, "Archived", t.TempDir())
	if err != nil {
		t.Fatalf("register archived project: %v", err)
	}
	if _, err := db.Exec(`UPDATE projects SET archived = 1 WHERE id = ?`, archived.ID); err != nil {
		t.Fatalf("archive project fixture: %v", err)
	}

	projects, err := store.ListProjects(ctx, false)
	if err != nil {
		t.Fatalf("list active projects: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != active.ID {
		t.Errorf("active projects = %+v, want only %q", projects, active.ID)
	}

	projects, err = store.ListProjects(ctx, true)
	if err != nil {
		t.Fatalf("list all projects: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("all projects count = %d, want 2", len(projects))
	}
}

func TestFindActiveProjectByRootCanonicalizesAndExcludesArchived(t *testing.T) {
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
		t.Fatalf("create project alias: %v", err)
	}
	found, err := store.FindActiveProjectByRoot(ctx, alias)
	if err != nil {
		t.Fatalf("find project through alias: %v", err)
	}
	if found != project {
		t.Errorf("found project = %+v, want %+v", found, project)
	}

	if _, err := store.FindActiveProjectByRoot(ctx, t.TempDir()); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("unregistered root error = %v, want ErrProjectNotFound", err)
	}

	if _, err := db.Exec(`UPDATE projects SET archived = 1 WHERE id = ?`, project.ID); err != nil {
		t.Fatalf("archive project: %v", err)
	}
	if _, err := store.FindActiveProjectByRoot(ctx, root); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("archived root error = %v, want ErrProjectNotFound", err)
	}

	archived, err := store.FindProjectByRoot(ctx, root)
	if err != nil {
		t.Fatalf("find archived project by root: %v", err)
	}
	if archived.ID != project.ID || !archived.Archived {
		t.Errorf("archived lookup = %+v, want archived project %q", archived, project.ID)
	}
}
