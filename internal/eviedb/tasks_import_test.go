package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/task"
)

const legacyTodoFixture = `{
 "Tasks": [
  {
   "ID": 9,
   "Title": "preserve open",
   "Description": "first description",
   "CreatedAt": "2026-07-01T10:30:00-04:00",
   "Priority": 4,
   "Status": false,
   "DueDate": "2026-09-03T00:00:00-04:00"
  },
  {
   "ID": 0,
   "Title": "preserve completed",
   "Description": "done description",
   "CreatedAt": "2026-07-02T11:45:00-04:00",
   "Priority": 0,
   "Status": true,
   "DueDate": "0001-01-01T00:00:00Z"
  }
 ],
 "NextID": 10
}`

func writeLegacyTodoFixture(t *testing.T, dir, body string) (string, []byte) {
	t.Helper()
	path := filepath.Join(dir, "DayToDay.json")
	bytes := []byte(body)
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, bytes
}

func TestLegacyTodoImportMapsProvenanceExactlyOnceAndSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	importedAt := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return importedAt }
	legacyPath, sourceBytes := writeLegacyTodoFixture(t, t.TempDir(), legacyTodoFixture)
	existingCtx := mutationContext("local", "existing", "create")
	existing, err := store.CreateGlobalTask(existingCtx, task.CreateInput{
		Title: "preserve open", Description: "first description", Priority: 4, DueDate: "2026-09-03",
		IdempotencyKey: "existing-identical-content",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.importLegacyTodoList(context.Background(), legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.MigrationID != legacyTodoMigrationID || len(result.Items) != 2 {
		t.Fatalf("first import = %+v", result)
	}
	if result.Items[0].SourceList != "DayToDay" || result.Items[0].LegacyID != 9 ||
		result.Items[0].Task.ID == "" || result.Items[0].Task.ID == task.ID("9") ||
		result.Items[0].Task.ID == existing.ID ||
		result.Items[0].Task.ParentID != "" || result.Items[0].Task.RootID != result.Items[0].Task.ID ||
		result.Items[0].Task.Title != "preserve open" || result.Items[0].Task.Description != "first description" ||
		result.Items[0].Task.Priority != 4 || result.Items[0].Task.DueDate != "2026-09-03" ||
		result.Items[0].Task.Status != task.StatusOpen || result.Items[0].Task.Revision != 1 ||
		!result.Items[0].Task.CreatedAt.Equal(importedAt) || !result.Items[0].Task.UpdatedAt.Equal(importedAt) {
		t.Fatalf("open import = %+v", result.Items[0])
	}
	if result.Items[1].SourceList != "DayToDay" || result.Items[1].LegacyID != 0 ||
		result.Items[1].Task.Title != "preserve completed" || result.Items[1].Task.Description != "done description" ||
		result.Items[1].Task.Priority != 0 || result.Items[1].Task.DueDate != "" ||
		result.Items[1].Task.Status != task.StatusCompleted || result.Items[1].Task.Revision != 1 {
		t.Fatalf("completed import = %+v", result.Items[1])
	}
	for _, item := range result.Items {
		events, err := store.ListTaskEvents(context.Background(), item.Task.ID)
		if err != nil || len(events) != 1 || events[0].Operation != task.OperationCreate ||
			events[0].ActorID != "kernel" || events[0].RunID != legacyTodoMigrationID ||
			events[0].IdempotencySHA256 == "" {
			t.Fatalf("import events for %d = %+v, %v", item.LegacyID, events, err)
		}
	}
	listed, err := store.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true})
	if err != nil || len(listed) != 3 {
		t.Fatalf("Task list after import = %+v, %v", listed, err)
	}
	gotExisting, err := store.GetGlobalTask(context.Background(), existing.ID)
	if err != nil || !reflect.DeepEqual(gotExisting, existing) {
		t.Fatalf("existing Task changed: got=%+v err=%v want=%+v", gotExisting, err, existing)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, sourceBytes) {
		t.Fatalf("source after import changed: err=%v got=%q want=%q", err, after, sourceBytes)
	}
	for _, statement := range []string{
		`UPDATE task_legacy_todo_imports SET source_list = 'changed'`,
		`DELETE FROM task_legacy_todo_provenance WHERE legacy_id = 9`,
	} {
		if _, err := db.Exec(statement); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("provenance mutation %q error = %v, want append-only rejection", statement, err)
		}
	}

	beforeRetryEvents, err := store.ListTaskEvents(context.Background(), result.Items[0].Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := store.importLegacyTodoList(context.Background(), legacyPath)
	if err != nil || retry.Applied || !reflect.DeepEqual(retry.Items, result.Items) {
		t.Fatalf("sequential retry = %+v, %v; want items %+v", retry, err, result.Items)
	}
	afterRetryEvents, err := store.ListTaskEvents(context.Background(), result.Items[0].Task.ID)
	if err != nil || !reflect.DeepEqual(afterRetryEvents, beforeRetryEvents) {
		t.Fatalf("sequential retry events = %+v, %v; want %+v", afterRetryEvents, err, beforeRetryEvents)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	corruptedSource := []byte(`{"not":"the legacy format"}`)
	if err := os.WriteFile(legacyPath, corruptedSource, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	reopened, err := store.importLegacyTodoList(context.Background(), legacyPath)
	if err != nil || reopened.Applied || !reflect.DeepEqual(reopened.Items, result.Items) {
		t.Fatalf("reopened retry = %+v, %v; want items %+v", reopened, err, result.Items)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, corruptedSource) {
		t.Fatalf("source after reopen changed: err=%v got=%q want=%q", err, after, corruptedSource)
	}

	open := task.StatusOpen
	reopenedCompleted, err := store.UpdateGlobalTask(
		mutationContext("local", "reopen-completed", "update"), result.Items[1].Task.ID,
		task.UpdateInput{ExpectedRevision: 1, Status: &open, IdempotencyKey: "reopen-imported-completed"},
	)
	if err != nil || reopenedCompleted.Status != task.StatusOpen {
		t.Fatalf("reopen imported completed Task = %+v, %v", reopenedCompleted, err)
	}
}

func TestLegacyTodoImportConcurrentAttemptsProduceOneEffect(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "evie.db")
	dbA, err := OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	storeA, storeB := NewStore(dbA), NewStore(dbB)
	legacyPath, sourceBytes := writeLegacyTodoFixture(t, dir, legacyTodoFixture)

	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan LegacyTodoImportResult, 2)
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for _, store := range []*Store{storeA, storeB} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			result, err := store.importLegacyTodoList(context.Background(), legacyPath)
			results <- result
			errors <- err
		}()
	}
	<-ready
	<-ready
	close(start)
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent import error = %v", err)
		}
	}
	applied := 0
	var want []LegacyTodoImportedTask
	for result := range results {
		if result.Applied {
			applied++
		}
		if want == nil {
			want = result.Items
		} else if !reflect.DeepEqual(result.Items, want) {
			t.Fatalf("concurrent results differ: got=%+v want=%+v", result.Items, want)
		}
	}
	if applied != 1 || len(want) != 2 {
		t.Fatalf("concurrent applied=%d items=%+v", applied, want)
	}
	listed, err := storeA.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true})
	if err != nil || len(listed) != 2 {
		t.Fatalf("concurrent imported Tasks = %+v, %v", listed, err)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, sourceBytes) {
		t.Fatalf("concurrent source changed: err=%v got=%q want=%q", err, after, sourceBytes)
	}
}

func TestLegacyTodoImportFailuresAreAtomicAndPreserveSource(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"Tasks":[`},
		{name: "blank title", body: `{"Tasks":[{"ID":1,"Title":"  ","Priority":1}]}`},
		{name: "invalid priority", body: `{"Tasks":[{"ID":1,"Title":"bad","Priority":6}]}`},
		{name: "duplicate legacy ID", body: `{"Tasks":[{"ID":1,"Title":"first"},{"ID":1,"Title":"second"}]}`},
		{name: "invalid due date", body: `{"Tasks":[{"ID":1,"Title":"bad date","DueDate":"not-a-time"}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := NewStore(db)
			legacyPath, before := writeLegacyTodoFixture(t, t.TempDir(), test.body)
			if _, err := store.importLegacyTodoList(context.Background(), legacyPath); err == nil {
				t.Fatal("invalid import succeeded")
			} else if !strings.Contains(err.Error(), legacyPath) {
				t.Fatalf("invalid import error %q omits source path %q", err, legacyPath)
			}
			listed, err := store.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true})
			if err != nil || len(listed) != 0 {
				t.Fatalf("invalid import left Tasks = %+v, %v", listed, err)
			}
			if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid source changed: err=%v got=%q want=%q", err, after, before)
			}
		})
	}
}

func TestLegacyTodoImportPersistenceFailureAndCancellationRollBackCompletely(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	existing, err := store.CreateGlobalTask(mutationContext("local", "preexisting", "create"), task.CreateInput{
		Title: "pre-existing state", IdempotencyKey: "pre-existing-before-import-failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	existingEvents, err := store.ListTaskEvents(context.Background(), existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath, before := writeLegacyTodoFixture(t, t.TempDir(), legacyTodoFixture)
	if _, err := db.Exec(`
		CREATE TRIGGER fail_second_legacy_import
		BEFORE INSERT ON tasks WHEN NEW.title = 'preserve completed'
		BEGIN SELECT RAISE(ABORT, 'forced legacy import failure'); END;
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.importLegacyTodoList(context.Background(), legacyPath); err == nil ||
		!strings.Contains(err.Error(), "forced legacy import failure") {
		t.Fatalf("persistence failure = %v", err)
	}
	listed, err := store.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true})
	if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0], existing) {
		t.Fatalf("failed import left Tasks = %+v, %v", listed, err)
	}
	if _, err := db.Exec(`DROP TRIGGER fail_second_legacy_import`); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	itemsStaged := 0
	store.afterLegacyTodoItem = func() {
		itemsStaged++
		if itemsStaged == 1 {
			cancel()
		}
	}
	if _, err := store.importLegacyTodoList(cancelled, legacyPath); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled import error = %v", err)
	}
	store.afterLegacyTodoItem = nil
	listed, err = store.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true})
	if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0], existing) {
		t.Fatalf("cancelled import left Tasks = %+v, %v", listed, err)
	}
	afterEvents, err := store.ListTaskEvents(context.Background(), existing.ID)
	if err != nil || !reflect.DeepEqual(afterEvents, existingEvents) {
		t.Fatalf("failed imports changed pre-existing events: got=%+v err=%v want=%+v", afterEvents, err, existingEvents)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("failed import source changed: err=%v got=%q want=%q", err, after, before)
	}
	retry, err := store.importLegacyTodoList(context.Background(), legacyPath)
	if err != nil || !retry.Applied || len(retry.Items) != 2 {
		t.Fatalf("retry after rolled-back failures = %+v, %v", retry, err)
	}
}

func TestLegacyTodoImportMissingIsNoOpAndExistingUnreadableInputErrors(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	dir := t.TempDir()
	missing := filepath.Join(dir, "DayToDay.json")
	result, err := store.importLegacyTodoList(context.Background(), missing)
	if err != nil || result.Applied || len(result.Items) != 0 {
		t.Fatalf("missing import = %+v, %v", result, err)
	}
	legacyPath, _ := writeLegacyTodoFixture(t, dir, legacyTodoFixture)
	result, err = store.importLegacyTodoList(context.Background(), legacyPath)
	if err != nil || !result.Applied || len(result.Items) != 2 {
		t.Fatalf("import after missing source = %+v, %v", result, err)
	}

	otherDB, err := OpenDBAt(filepath.Join(t.TempDir(), "other.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	otherStore := NewStore(otherDB)
	unreadable := filepath.Join(t.TempDir(), "DayToDay.json")
	if err := os.Mkdir(unreadable, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := otherStore.importLegacyTodoList(context.Background(), unreadable); err == nil ||
		!strings.Contains(err.Error(), "read legacy Todo") {
		t.Fatalf("unreadable import error = %v", err)
	}
	listed, err := otherStore.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true})
	if err != nil || len(listed) != 0 {
		t.Fatalf("unreadable import left Tasks = %+v, %v", listed, err)
	}
}

func TestLegacyTodoImportEmptyListCompletesTheOneTimeMigration(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	dir := t.TempDir()
	legacyPath, emptyBytes := writeLegacyTodoFixture(t, dir, `{"Tasks":[],"NextID":0}`)
	first, err := store.importLegacyTodoList(context.Background(), legacyPath)
	if err != nil || !first.Applied || len(first.Items) != 0 {
		t.Fatalf("empty import = %+v, %v", first, err)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, emptyBytes) {
		t.Fatalf("empty source changed: err=%v got=%q want=%q", err, after, emptyBytes)
	}
	_, replacementBytes := writeLegacyTodoFixture(t, dir, legacyTodoFixture)
	retry, err := store.importLegacyTodoList(context.Background(), legacyPath)
	if err != nil || retry.Applied || len(retry.Items) != 0 {
		t.Fatalf("retry after completed empty import = %+v, %v", retry, err)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, replacementBytes) {
		t.Fatalf("replacement source changed: err=%v got=%q want=%q", err, after, replacementBytes)
	}
}

func TestLegacyTodoImportResolvesCommittedButUncertainRetryWithoutDuplicates(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	legacyPath, before := writeLegacyTodoFixture(t, t.TempDir(), legacyTodoFixture)
	store.resolveImmediateTransaction = func(
		ctx context.Context,
		conn *sql.Conn,
		statement string,
	) (sql.Result, error) {
		result, execErr := executeImmediateTransactionStatement(ctx, conn, statement)
		if statement == `COMMIT` && execErr == nil {
			return result, errors.New("forced uncertain legacy Todo commit")
		}
		return result, execErr
	}
	if _, err := store.importLegacyTodoList(context.Background(), legacyPath); err == nil ||
		!strings.Contains(err.Error(), "forced uncertain legacy Todo commit") {
		t.Fatalf("uncertain import error = %v", err)
	}
	store.resolveImmediateTransaction = executeImmediateTransactionStatement
	retry, err := store.importLegacyTodoList(context.Background(), legacyPath)
	if err != nil || retry.Applied || len(retry.Items) != 2 {
		t.Fatalf("retry after uncertain commit = %+v, %v", retry, err)
	}
	listed, err := store.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true})
	if err != nil || len(listed) != 2 {
		t.Fatalf("Tasks after uncertain retry = %+v, %v", listed, err)
	}
	for _, item := range retry.Items {
		events, err := store.ListTaskEvents(context.Background(), item.Task.ID)
		if err != nil || len(events) != 1 {
			t.Fatalf("events after uncertain retry for %d = %+v, %v", item.LegacyID, events, err)
		}
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("uncertain retry source changed: err=%v got=%q want=%q", err, after, before)
	}
}

func TestDefaultLegacyTodoPathAndImportUseOnlyTheHistoricalDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".todo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyPath, before := writeLegacyTodoFixture(t, dir, legacyTodoFixture)
	resolved, err := DefaultLegacyTodoPath()
	if err != nil || resolved != legacyPath {
		t.Fatalf("default path = %q, %v; want %q", resolved, err, legacyPath)
	}
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	result, err := NewStore(db).ImportDefaultLegacyTodoList(context.Background())
	if err != nil || !result.Applied || len(result.Items) != 2 {
		t.Fatalf("default import = %+v, %v", result, err)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("default source changed: err=%v got=%q want=%q", err, after, before)
	}
}
