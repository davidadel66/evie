package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/task"
)

func TestTaskStoreCreatesListsGetsAndReopensGlobalTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	store.now = func() time.Time {
		return time.Date(2026, time.September, 2, 16, 30, 0, 123, time.FixedZone("owner", -4*60*60))
	}

	created, err := store.CreateGlobalTask(context.Background(), task.CreateInput{
		Title:       "Ship the durable path",
		Description: "Keep the contract narrow",
		Priority:    5,
		DueDate:     "2026-09-03",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Scope != task.ScopeGlobal || created.Status != task.StatusOpen ||
		created.Revision != 1 || created.CreatedAt.Location() != time.UTC ||
		!created.CreatedAt.Equal(store.now().UTC()) || !created.UpdatedAt.Equal(created.CreatedAt) {
		t.Fatalf("created Task metadata = %+v", created)
	}

	listed, err := store.ListOpenGlobalTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(listed, []task.Task{created}) {
		t.Fatalf("listed Tasks = %#v, want %#v", listed, []task.Task{created})
	}
	got, err := store.GetGlobalTask(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Fatalf("got Task = %#v, want %#v", got, created)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reopened, err := NewStore(db).GetGlobalTask(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened, created) {
		t.Fatalf("reopened Task = %#v, want %#v", reopened, created)
	}
}

func TestTaskStoreRejectsInvalidCreateWithoutPartialRows(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)

	tests := []struct {
		name  string
		input task.CreateInput
		field string
	}{
		{name: "empty title", input: task.CreateInput{}, field: "title"},
		{name: "blank title", input: task.CreateInput{Title: "  \t"}, field: "title"},
		{name: "negative priority", input: task.CreateInput{Title: "x", Priority: -1}, field: "priority"},
		{name: "priority above five", input: task.CreateInput{Title: "x", Priority: 6}, field: "priority"},
		{name: "malformed due date", input: task.CreateInput{Title: "x", DueDate: "09/03/2026"}, field: "due"},
		{name: "impossible due date", input: task.CreateInput{Title: "x", DueDate: "2026-02-30"}, field: "due"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.CreateGlobalTask(context.Background(), tt.input)
			var inputErr *task.InputError
			if !errors.Is(err, task.ErrInvalidInput) || !errors.As(err, &inputErr) || inputErr.Field != tt.field {
				t.Fatalf("CreateGlobalTask error = %v, want typed %s input error", err, tt.field)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "cancelled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CreateGlobalTask error = %v", err)
	}
	listed, err := store.ListOpenGlobalTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("invalid creates left Tasks: %+v", listed)
	}
}

func TestTaskStoreMissingIdentityReturnsTypedError(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	if _, err := store.GetGlobalTask(context.Background(), ""); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("empty GetGlobalTask identity error = %v", err)
	}

	_, err = store.GetGlobalTask(context.Background(), task.ID("opaque-missing-task"))
	var notFound *task.NotFoundError
	if !errors.Is(err, task.ErrNotFound) || !errors.As(err, &notFound) || notFound.ID != "opaque-missing-task" {
		t.Fatalf("GetGlobalTask error = %v, want typed not-found", err)
	}
}

func TestTaskTableRejectsForeignScopeAndNonIntegerStateAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tests := []struct {
		name     string
		scope    any
		priority any
		revision any
	}{
		{name: "foreign scope", scope: "project:forged", priority: nil, revision: 1},
		{name: "fractional priority", scope: "global", priority: 1.5, revision: 1},
		{name: "fractional revision", scope: "global", priority: nil, revision: 1.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Exec(`
				INSERT INTO tasks (
					id, scope, title, priority, status, revision, created_at, updated_at
				) VALUES (?, ?, 'forged', ?, 'open', ?, ?, ?)
			`, tt.name, tt.scope, tt.priority, tt.revision, now, now)
			if err == nil {
				t.Fatal("invalid raw Task row was accepted")
			}
		})
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	listed, err := NewStore(db).ListOpenGlobalTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("reopen exposed invalid Task rows: %+v", listed)
	}
}
