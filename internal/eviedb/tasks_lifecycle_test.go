package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/task"
)

func TestTaskLifecycleStateEventsFiltersAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	next := time.Date(2026, time.September, 2, 16, 0, 0, 0, time.UTC)
	store.now = func() time.Time {
		value := next
		next = next.Add(time.Minute)
		return value
	}
	ctx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: "local", SessionID: "session-120", RunID: "run-120",
	})

	created, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "Retain lifecycle", Description: "before", Priority: 4, DueDate: "2026-09-05",
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].TaskID != created.ID || events[0].Sequence != 1 ||
		events[0].Operation != task.OperationCreate || events[0].ActorID != "local" ||
		events[0].SessionID != "session-120" || events[0].RunID != "run-120" ||
		events[0].PreviousRevision != 0 || events[0].ResultingRevision != 1 ||
		events[0].Outcome != task.MutationAccepted || events[0].DiagnosticCode != "" {
		t.Fatalf("create events = %+v", events)
	}

	title, description, priority, due := "Retained", "", 0, ""
	updated, err := store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{
		ExpectedRevision: 1, Title: &title, Description: &description, Priority: &priority, DueDate: &due,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != title || updated.Description != "" || updated.Priority != 0 || updated.DueDate != "" ||
		updated.Revision != 2 || updated.Status != task.StatusOpen || !updated.CreatedAt.Equal(created.CreatedAt) ||
		!updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("metadata update = %+v", updated)
	}

	completed := task.StatusCompleted
	completedTask, err := store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: 2, Status: &completed})
	if err != nil {
		t.Fatal(err)
	}
	if completedTask.Status != task.StatusCompleted || completedTask.Revision != 3 {
		t.Fatalf("completed Task = %+v", completedTask)
	}
	if open, err := store.ListGlobalTasks(context.Background(), task.ListFilter{}); err != nil || len(open) != 0 {
		t.Fatalf("default open list = %+v, %v", open, err)
	}
	if history, err := store.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true}); err != nil ||
		!reflect.DeepEqual(history, []task.Task{completedTask}) {
		t.Fatalf("history list = %+v, %v", history, err)
	}
	if completedOnly, err := store.ListGlobalTasks(context.Background(), task.ListFilter{Statuses: []task.Status{task.StatusCompleted}}); err != nil ||
		!reflect.DeepEqual(completedOnly, []task.Task{completedTask}) {
		t.Fatalf("completed list = %+v, %v", completedOnly, err)
	}

	blocked := task.StatusBlocked
	_, err = store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: 3, Status: &blocked})
	var transitionErr *task.TransitionError
	if !errors.Is(err, task.ErrInvalidTransition) || !errors.As(err, &transitionErr) {
		t.Fatalf("completed -> blocked error = %v", err)
	}
	_, err = store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: 2, Title: &title})
	var conflict *task.ConflictError
	if !errors.Is(err, task.ErrConflict) || !errors.As(err, &conflict) || conflict.Expected != 2 || conflict.Current != 3 {
		t.Fatalf("stale update error = %#v", err)
	}

	reopenedStatus := task.StatusOpen
	reopened, err := store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: 3, Status: &reopenedStatus})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != task.StatusOpen || reopened.Revision != 4 {
		t.Fatalf("reopened Task = %+v", reopened)
	}
	events, err = store.ListTaskEvents(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 {
		t.Fatalf("events = %+v, want create + 3 accepted updates + 2 rejected diagnostics", events)
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) || event.ActorID != "local" || event.SessionID != "session-120" || event.RunID != "run-120" {
			t.Fatalf("event[%d] = %+v", i, event)
		}
	}
	if got := events[3]; got.Outcome != task.MutationRejected || got.DiagnosticCode != task.DiagnosticInvalidTransition ||
		got.PreviousRevision != 3 || got.ResultingRevision != 3 {
		t.Fatalf("transition diagnostic = %+v", got)
	}
	if got := events[4]; got.Outcome != task.MutationRejected || got.DiagnosticCode != task.DiagnosticRevisionConflict ||
		got.PreviousRevision != 3 || got.ResultingRevision != 3 {
		t.Fatalf("conflict diagnostic = %+v", got)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reopenedStore := NewStore(db)
	gotTask, err := reopenedStore.GetGlobalTask(context.Background(), created.ID)
	if err != nil || !reflect.DeepEqual(gotTask, reopened) {
		t.Fatalf("Task after restart = %+v, %v; want %+v", gotTask, err, reopened)
	}
	gotEvents, err := reopenedStore.ListTaskEvents(context.Background(), created.ID)
	if err != nil || !reflect.DeepEqual(gotEvents, events) {
		t.Fatalf("events after restart = %+v, %v; want %+v", gotEvents, err, events)
	}
}

func TestTaskLifecycleRejectsEveryIllegalEdgeWithoutChangingState(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := attributedTaskContext()
	statuses := []task.Status{task.StatusOpen, task.StatusInProgress, task.StatusBlocked, task.StatusCompleted, task.StatusCancelled}
	for _, from := range statuses {
		for _, to := range statuses {
			if task.ValidateStatusTransition(from, to) == nil {
				continue
			}
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "edge " + string(from) + " " + string(to)})
				if err != nil {
					t.Fatal(err)
				}
				if from != task.StatusOpen {
					seed := from
					created, err = store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: created.Revision, Status: &seed})
					if err != nil {
						t.Fatal(err)
					}
				}
				before := created
				target := to
				_, err = store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: created.Revision, Status: &target})
				if !errors.Is(err, task.ErrInvalidTransition) {
					t.Fatalf("UpdateGlobalTask error = %v", err)
				}
				after, getErr := store.GetGlobalTask(context.Background(), created.ID)
				if getErr != nil || !reflect.DeepEqual(after, before) {
					t.Fatalf("rejected edge changed Task: before=%+v after=%+v err=%v", before, after, getErr)
				}
			})
		}
	}
}

func TestTaskLifecyclePersistsEveryAllowedEdge(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := attributedTaskContext()
	statuses := []task.Status{task.StatusOpen, task.StatusInProgress, task.StatusBlocked, task.StatusCompleted, task.StatusCancelled}
	for _, from := range statuses {
		for _, to := range statuses {
			if task.ValidateStatusTransition(from, to) != nil {
				continue
			}
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "allowed " + string(from) + " " + string(to)})
				if err != nil {
					t.Fatal(err)
				}
				if from != task.StatusOpen {
					seed := from
					created, err = store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: 1, Status: &seed})
					if err != nil {
						t.Fatal(err)
					}
				}
				target := to
				updated, err := store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: created.Revision, Status: &target})
				if err != nil || updated.Status != to || updated.Revision != created.Revision+1 {
					t.Fatalf("allowed update = %+v, %v", updated, err)
				}
				events, err := store.ListTaskEvents(context.Background(), created.ID)
				if err != nil || events[len(events)-1].Outcome != task.MutationAccepted ||
					events[len(events)-1].PreviousRevision != created.Revision ||
					events[len(events)-1].ResultingRevision != updated.Revision {
					t.Fatalf("allowed edge events = %+v, %v", events, err)
				}
			})
		}
	}
}

func TestTaskEventsAreAppendOnlyTasksCannotBeDeletedAndEventFailureRollsBack(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := attributedTaskContext()
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "immutable history"})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE task_events SET outcome = 'rejected' WHERE task_id = ?`,
		`DELETE FROM task_events WHERE task_id = ?`,
		`DELETE FROM tasks WHERE id = ?`,
	} {
		if _, err := db.Exec(statement, created.ID); err == nil {
			t.Fatalf("ordinary writable DB accepted %q", statement)
		}
	}
	if _, err := db.Exec(`
		CREATE TRIGGER reject_task_event_insert BEFORE INSERT ON task_events
		BEGIN SELECT RAISE(ABORT, 'forced event insert failure'); END;
	`); err != nil {
		t.Fatal(err)
	}
	title := "must roll back"
	_, err = store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: 1, Title: &title})
	if err == nil || !strings.Contains(err.Error(), "forced event insert failure") {
		t.Fatalf("event failure update error = %v", err)
	}
	got, err := store.GetGlobalTask(context.Background(), created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("event failure left Task mutation: %+v, %v", got, err)
	}
}

func TestTaskUpdateCancellationAndInvalidFiltersLeaveNoEvidence(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := attributedTaskContext()
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "cancel update"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	title := "not written"
	if _, err := store.UpdateGlobalTask(cancelled, created.ID, task.UpdateInput{ExpectedRevision: 1, Title: &title}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled update error = %v", err)
	}
	if _, err := store.ListGlobalTasks(context.Background(), task.ListFilter{Statuses: []task.Status{"forged"}}); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("invalid filter error = %v", err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("cancelled update events = %+v, %v", events, err)
	}
}

func TestTaskMutationRequiresTrustedAttributionAndInvalidPatchRecordsSafeDiagnostic(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	if _, err := store.CreateGlobalTask(context.Background(), task.CreateInput{Title: "unattributed"}); !errors.Is(err, task.ErrMissingAttribution) {
		t.Fatalf("unattributed create error = %v", err)
	}
	ctx := attributedTaskContext()
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "safe diagnostic"})
	if err != nil {
		t.Fatal(err)
	}
	blank := " \t"
	if _, err := store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{ExpectedRevision: 1, Title: &blank}); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("invalid patch error = %v", err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 2 || events[1].DiagnosticCode != task.DiagnosticInvalidInput ||
		events[1].PreviousRevision != 1 || events[1].ResultingRevision != 1 {
		t.Fatalf("invalid patch diagnostic = %+v, %v", events, err)
	}
	got, err := store.GetGlobalTask(context.Background(), created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("invalid patch changed Task: %+v, %v", got, err)
	}
}
