package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
)

func TestTaskScopeUpgradePreservesLegacyGlobalState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewStore(db).CreateGlobalTask(mutationContext("local", "legacy", "seed"), task.CreateInput{Title: "legacy", IdempotencyKey: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA foreign_keys=OFF;
		DROP TRIGGER IF EXISTS tasks_no_hard_delete;
		DROP TRIGGER IF EXISTS task_hierarchy_validate_child;
		DROP TRIGGER IF EXISTS session_task_focus_authorized_insert;
		DROP TRIGGER IF EXISTS session_task_focus_authorized_update;
		DROP TRIGGER IF EXISTS task_revisions_append_only_update;
		DROP TRIGGER IF EXISTS task_revisions_append_only_delete;
		CREATE TABLE old_tasks (id TEXT PRIMARY KEY, scope TEXT NOT NULL CHECK(scope='global'), title TEXT NOT NULL,
			description TEXT, priority INTEGER, due_date TEXT, result_summary TEXT, status TEXT NOT NULL, revision INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO old_tasks SELECT * FROM tasks; DROP TABLE tasks; ALTER TABLE old_tasks RENAME TO tasks;
		CREATE TABLE old_revisions (task_id TEXT NOT NULL REFERENCES tasks(id), revision INTEGER NOT NULL, scope TEXT NOT NULL CHECK(scope='global'),
			title TEXT NOT NULL, description TEXT, priority INTEGER, due_date TEXT, result_summary TEXT, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			PRIMARY KEY(task_id, revision));
		INSERT INTO old_revisions SELECT * FROM task_revisions; DROP TABLE task_revisions; ALTER TABLE old_revisions RENAME TO task_revisions;`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := NewStore(db).GetGlobalTask(context.Background(), legacy.ID)
	if err != nil || got.Scope != task.ScopeGlobal || got.Title != "legacy" {
		t.Fatalf("upgraded legacy Task = %+v, %v", got, err)
	}
	var definition string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='tasks'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "workspace:*") {
		t.Fatalf("Task schema was not widened: %s", definition)
	}
}

func scopedTaskContext(session memory.Session, run string) context.Context {
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(session.ID), RunID: run,
		WorkspaceID: string(session.WorkspaceID), ProjectID: string(session.ProjectID),
		ParentSessionID: string(session.ParentSessionID),
	})
}

func TestTaskScopesDefaultToContextAndRemainIsolated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	ctx := context.Background()
	workspaceA, err := store.RegisterWorkspace(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	workspaceB, err := store.RegisterWorkspace(ctx, "B")
	if err != nil {
		t.Fatal(err)
	}
	sessionA, err := store.CreateWorkspaceSessionWithComposition(ctx, workspaceA.ID, workspaceA.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	sessionB, err := store.CreateWorkspaceSessionWithComposition(ctx, workspaceB.ID, workspaceB.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}

	global, err := store.CreateGlobalTask(scopedTaskContext(globalSession, "global"), task.CreateInput{Title: "global", IdempotencyKey: "global"})
	if err != nil {
		t.Fatal(err)
	}
	rootA, err := store.CreateGlobalTask(scopedTaskContext(sessionA, "a"), task.CreateInput{Title: "workspace a", IdempotencyKey: "a"})
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := store.CreateGlobalTask(scopedTaskContext(sessionB, "b"), task.CreateInput{Title: "workspace b", IdempotencyKey: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if rootA.Scope != task.WorkspaceScope(string(workspaceA.ID)) || rootB.Scope != task.WorkspaceScope(string(workspaceB.ID)) {
		t.Fatalf("workspace scopes = %q, %q", rootA.Scope, rootB.Scope)
	}
	child, err := store.CreateGlobalTask(scopedTaskContext(sessionA, "child"), task.CreateInput{
		Title: "child", ParentID: rootA.ID, ExpectedParentRevision: rootA.Revision, IdempotencyKey: "child",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.Scope != rootA.Scope {
		t.Fatalf("child scope = %q, want %q", child.Scope, rootA.Scope)
	}

	visible, err := store.ListGlobalTasks(scopedTaskContext(sessionA, "list"), task.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got := taskTitles(visible); strings.Join(got, ",") != "global,workspace a,child" {
		t.Fatalf("visible titles = %v", got)
	}
	if _, err := store.GetGlobalTask(scopedTaskContext(sessionA, "get"), rootB.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("sibling get error = %v, want non-disclosing not found", err)
	}
	if _, err := store.CreateGlobalTask(scopedTaskContext(sessionA, "cross"), task.CreateInput{
		Title: "cross", ParentID: rootB.ID, ExpectedParentRevision: rootB.Revision, IdempotencyKey: "cross",
	}); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("cross-scope child error = %v, want not found", err)
	}
	explicitGlobal, err := store.CreateGlobalTask(scopedTaskContext(sessionA, "explicit-global"), task.CreateInput{
		Title: "owner wide", Scope: task.ScopeSelectionGlobal, IdempotencyKey: "explicit-global",
	})
	if err != nil || explicitGlobal.Scope != task.ScopeGlobal {
		t.Fatalf("explicit Global Task = %+v, %v", explicitGlobal, err)
	}
	globalOnly, err := store.ListGlobalTasks(scopedTaskContext(sessionA, "filter"), task.ListFilter{Scope: task.ScopeSelectionGlobal})
	if err != nil || len(globalOnly) != 2 || globalOnly[0].ID != global.ID || globalOnly[1].ID != explicitGlobal.ID {
		t.Fatalf("global filter = %+v, %v", globalOnly, err)
	}

	projectRoot := t.TempDir()
	project, err := store.RegisterProject(ctx, "P", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	projectSession, err := store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	projectTask, err := store.CreateGlobalTask(scopedTaskContext(projectSession, "project"), task.CreateInput{Title: "project", IdempotencyKey: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if projectTask.Scope != task.ProjectScope(string(project.ID)) {
		t.Fatalf("project scope = %q", projectTask.Scope)
	}
	otherProject, err := store.RegisterProject(ctx, "Other project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	otherProjectSession, err := store.CreateProjectSession(ctx, otherProject.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherProjectTask, err := store.CreateGlobalTask(scopedTaskContext(otherProjectSession, "other-project"), task.CreateInput{Title: "other project", IdempotencyKey: "other-project"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetGlobalTask(scopedTaskContext(sessionA, "workspace-project-get"), projectTask.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Workspace read of project Task = %v", err)
	}
	if _, err := store.GetGlobalTask(scopedTaskContext(projectSession, "project-workspace-get"), rootA.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("project read of Workspace Task = %v", err)
	}
	if _, err := store.GetGlobalTask(scopedTaskContext(projectSession, "sibling-project-get"), otherProjectTask.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("project read of sibling project Task = %v", err)
	}
	projectVisible, err := store.ListGlobalTasks(scopedTaskContext(projectSession, "project-list"), task.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range projectVisible {
		if value.ID == rootA.ID || value.ID == rootB.ID || value.ID == otherProjectTask.ID {
			t.Fatalf("project list leaked foreign Task: %+v", value)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reopened, err := NewStore(db).GetGlobalTask(scopedTaskContext(sessionA, "reopen"), rootA.ID)
	if err != nil || reopened.Scope != rootA.Scope {
		t.Fatalf("reopened = %+v, %v", reopened, err)
	}
}

func TestTaskFocusPersistsAndProjectsBoundedOpenTree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(context.Background(), "Focused")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(context.Background(), workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.RegisterWorkspace(context.Background(), "Other")
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := store.CreateWorkspaceSessionWithComposition(context.Background(), other.ID, other.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}

	root, err := store.CreateGlobalTask(scopedTaskContext(session, "root"), task.CreateInput{Title: "focus root", IdempotencyKey: "focus-root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalTask(scopedTaskContext(session, "child"), task.CreateInput{Title: "open child", ParentID: root.ID, ExpectedParentRevision: 1, IdempotencyKey: "focus-child"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := store.CreateGlobalTask(scopedTaskContext(otherSession, "foreign"), task.CreateInput{Title: "secret sibling", IdempotencyKey: "foreign"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SelectTaskFocus(scopedTaskContext(session, "select"), root.ID); err != nil {
		t.Fatal(err)
	}
	blocked := task.StatusBlocked
	child, err = store.ManagementUpdateGlobalTask(scopedTaskContext(session, "block-child"), child.ID, task.UpdateInput{
		ExpectedRevision: child.Revision, Status: &blocked, IdempotencyKey: "block-focused-child",
	}, "focus-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SelectTaskFocus(scopedTaskContext(session, "reject"), foreign.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("foreign focus error = %v", err)
	}
	focused, err := store.ListGlobalTasks(scopedTaskContext(session, "focused-list"), task.ListFilter{})
	if err != nil || len(focused) != 2 || focused[0].ID != root.ID || focused[1].ID != child.ID {
		t.Fatalf("focused list = %+v, %v", focused, err)
	}
	projection, err := store.BindHistory(session.ID, "holder").WorkingContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(projection, string(root.ID)) || !strings.Contains(projection, "open child") || strings.Contains(projection, "secret sibling") {
		t.Fatalf("projection = %q", projection)
	}
	if err := store.SelectTaskFocus(scopedTaskContext(session, "select-child"), child.ID); err != nil {
		t.Fatal(err)
	}
	focused, err = store.ListGlobalTasks(scopedTaskContext(session, "child-list"), task.ListFilter{})
	if err != nil || len(focused) != 1 || focused[0].ID != child.ID {
		t.Fatalf("child-focused list = %+v, %v", focused, err)
	}
	projection, err = store.BindHistory(session.ID, "holder").WorkingContext(context.Background())
	if err != nil || strings.Contains(projection, "focus root") || !strings.Contains(projection, "open child") {
		t.Fatalf("child projection = %q, %v", projection, err)
	}
	if err := store.SelectTaskFocus(scopedTaskContext(session, "reselect-root"), root.ID); err != nil {
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
	reopened := NewStore(db)
	projection, err = reopened.BindHistory(session.ID, "holder").WorkingContext(context.Background())
	if err != nil || !strings.Contains(projection, "focus root") {
		t.Fatalf("reopened projection = %q, %v", projection, err)
	}
	if err := reopened.ClearTaskFocus(scopedTaskContext(session, "clear")); err != nil {
		t.Fatal(err)
	}
	projection, err = reopened.BindHistory(session.ID, "holder").WorkingContext(context.Background())
	if err != nil || projection != "" {
		t.Fatalf("cleared projection = %q, %v", projection, err)
	}
}

func taskTitles(values []task.Task) []string {
	result := make([]string, len(values))
	for i := range values {
		result[i] = values[i].Title
	}
	return result
}
