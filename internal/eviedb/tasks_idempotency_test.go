package eviedb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/task"
)

func mutationContext(actor, session, run string) context.Context {
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: actor, SessionID: session, RunID: run,
	})
}

func TestTaskMutationsReplayCanonicalOutcomeSequentiallyAndAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	ctx1 := mutationContext("local", "session-replay", "run-1")
	ctx2 := mutationContext("local", "session-replay", "run-2")
	createInput := task.CreateInput{Title: "canonical", Description: "first state", IdempotencyKey: "create-replay"}
	created, err := store.CreateGlobalTask(ctx1, createInput)
	if err != nil {
		t.Fatal(err)
	}
	replayedCreate, err := store.CreateGlobalTask(ctx2, createInput)
	if err != nil || !reflect.DeepEqual(replayedCreate, created) {
		t.Fatalf("create replay = %+v, %v; want %+v", replayedCreate, err, created)
	}
	title := "revision two"
	updateInput := task.UpdateInput{ExpectedRevision: 1, Title: &title, IdempotencyKey: "update-replay"}
	updated, err := store.UpdateGlobalTask(ctx1, created.ID, updateInput)
	if err != nil {
		t.Fatal(err)
	}
	replayedUpdate, err := store.UpdateGlobalTask(ctx2, created.ID, updateInput)
	if err != nil || !reflect.DeepEqual(replayedUpdate, updated) {
		t.Fatalf("update replay = %+v, %v; want %+v", replayedUpdate, err, updated)
	}
	laterTitle := "revision three"
	later, err := store.UpdateGlobalTask(ctx1, created.ID, task.UpdateInput{
		ExpectedRevision: 2, Title: &laterTitle, IdempotencyKey: "later-update",
	})
	if err != nil || later.Revision != 3 {
		t.Fatalf("later update = %+v, %v", later, err)
	}
	replayedUpdate, err = store.UpdateGlobalTask(ctx2, created.ID, updateInput)
	if err != nil || !reflect.DeepEqual(replayedUpdate, updated) {
		t.Fatalf("historical replay = %+v, %v; want exact revision-two %+v", replayedUpdate, err, updated)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 3 {
		t.Fatalf("events = %+v, %v, want one per accepted effect", events, err)
	}
	for _, event := range events {
		if len(event.IdempotencySHA256) != 64 {
			t.Fatalf("event lacks hashed idempotency identity: %+v", event)
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
	reopened := NewStore(db)
	gotCreate, err := reopened.CreateGlobalTask(ctx2, createInput)
	if err != nil || !reflect.DeepEqual(gotCreate, created) {
		t.Fatalf("reopened create replay = %+v, %v; want %+v", gotCreate, err, created)
	}
	gotUpdate, err := reopened.UpdateGlobalTask(ctx2, created.ID, updateInput)
	if err != nil || !reflect.DeepEqual(gotUpdate, updated) {
		t.Fatalf("reopened update replay = %+v, %v; want %+v", gotUpdate, err, updated)
	}
}

func TestTaskMutationsRejectConflictingReuseButBindActorAndSession(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("actor-a", "session-a", "run-a")
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "original", IdempotencyKey: "shared-key"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateGlobalTask(ctx, task.CreateInput{Title: "different", IdempotencyKey: "shared-key"})
	var idemConflict *task.IdempotencyConflictError
	if !errors.Is(err, task.ErrIdempotencyConflict) || !errors.As(err, &idemConflict) {
		t.Fatalf("conflicting create error = %v", err)
	}
	title := "operation conflict"
	_, err = store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{
		ExpectedRevision: 1, Title: &title, IdempotencyKey: "shared-key",
	})
	if !errors.Is(err, task.ErrIdempotencyConflict) {
		t.Fatalf("cross-operation reuse error = %v", err)
	}
	for _, identity := range []struct{ actor, session string }{
		{actor: "actor-a", session: "session-b"},
		{actor: "actor-b", session: "session-a"},
	} {
		if _, err := store.CreateGlobalTask(mutationContext(identity.actor, identity.session, "run"), task.CreateInput{
			Title: "independent binding", IdempotencyKey: "shared-key",
		}); err != nil {
			t.Fatalf("binding %+v reused key: %v", identity, err)
		}
	}
	listed, err := store.ListOpenGlobalTasks(context.Background())
	if err != nil || len(listed) != 3 {
		t.Fatalf("bound creates = %+v, %v", listed, err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 1 || events[0].ResultingRevision != 1 {
		t.Fatalf("conflicting reuse changed Task history: %+v, %v", events, err)
	}
	var conflicts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_idempotency_conflicts`).Scan(&conflicts); err != nil || conflicts != 2 {
		t.Fatalf("conflict records = %d, %v", conflicts, err)
	}
}

func TestRejectedTaskMutationReplayReturnsOriginalTypedOutcome(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "session-rejected-replay", "run")
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "unchanged", IdempotencyKey: "rejected-replay-create",
	})
	if err != nil {
		t.Fatal(err)
	}
	title := created.Title
	input := task.UpdateInput{ExpectedRevision: 1, Title: &title, IdempotencyKey: "rejected-replay-update"}
	_, firstErr := store.UpdateGlobalTask(ctx, created.ID, input)
	_, replayErr := store.UpdateGlobalTask(mutationContext("local", "session-rejected-replay", "retry"), created.ID, input)
	for _, gotErr := range []error{firstErr, replayErr} {
		var inputErr *task.InputError
		if !errors.As(gotErr, &inputErr) || inputErr.Field != "patch" || inputErr.Message != "must change task state" {
			t.Fatalf("rejected replay error = %#v, want original typed patch outcome", gotErr)
		}
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 2 {
		t.Fatalf("rejected replay events = %+v, %v, want create plus one rejection", events, err)
	}
}

type concurrentMutationResult struct {
	task task.Task
	err  error
}

func runConcurrentMutations(
	t *testing.T,
	first func() (task.Task, error),
	second func() (task.Task, error),
) [2]concurrentMutationResult {
	t.Helper()
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan concurrentMutationResult, 2)
	var wg sync.WaitGroup
	for _, operation := range []func() (task.Task, error){first, second} {
		wg.Add(1)
		go func(operation func() (task.Task, error)) {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			value, err := operation()
			results <- concurrentMutationResult{task: value, err: err}
		}(operation)
	}
	<-ready
	<-ready
	close(start)
	wg.Wait()
	close(results)
	var got [2]concurrentMutationResult
	i := 0
	for result := range results {
		got[i] = result
		i++
	}
	return got
}

func openIndependentTaskStores(t *testing.T) (*Store, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbA.Close() })
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbB.Close() })
	dbA.SetMaxOpenConns(1)
	dbB.SetMaxOpenConns(1)
	return NewStore(dbA), NewStore(dbB)
}

func TestConcurrentIdenticalTaskMutationsCommitOneEffect(t *testing.T) {
	storeA, storeB := openIndependentTaskStores(t)
	storeA.now = func() time.Time { return time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC) }
	storeB.now = func() time.Time { return time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC) }
	input := task.CreateInput{Title: "one effect", IdempotencyKey: "concurrent-create"}
	results := runConcurrentMutations(t,
		func() (task.Task, error) {
			return storeA.CreateGlobalTask(mutationContext("local", "session-concurrent", "run-a"), input)
		},
		func() (task.Task, error) {
			return storeB.CreateGlobalTask(mutationContext("local", "session-concurrent", "run-b"), input)
		},
	)
	if results[0].err != nil || results[1].err != nil || !reflect.DeepEqual(results[0].task, results[1].task) {
		t.Fatalf("concurrent create results = %+v", results)
	}
	created := results[0].task
	title := "updated once"
	update := task.UpdateInput{ExpectedRevision: 1, Title: &title, IdempotencyKey: "concurrent-update"}
	results = runConcurrentMutations(t,
		func() (task.Task, error) {
			return storeA.UpdateGlobalTask(mutationContext("local", "session-concurrent", "run-c"), created.ID, update)
		},
		func() (task.Task, error) {
			return storeB.UpdateGlobalTask(mutationContext("local", "session-concurrent", "run-d"), created.ID, update)
		},
	)
	if results[0].err != nil || results[1].err != nil || !reflect.DeepEqual(results[0].task, results[1].task) || results[0].task.Revision != 2 {
		t.Fatalf("concurrent update results = %+v", results)
	}
	events, err := storeA.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 2 {
		t.Fatalf("concurrent retry events = %+v, %v", events, err)
	}
}

func TestConcurrentSameRevisionUpdatesHaveOneWinnerAndStableLoser(t *testing.T) {
	storeA, storeB := openIndependentTaskStores(t)
	ctx := mutationContext("local", "session-race", "seed")
	created, err := storeA.CreateGlobalTask(ctx, task.CreateInput{Title: "race", IdempotencyKey: "race-create"})
	if err != nil {
		t.Fatal(err)
	}
	titleA, titleB := "winner-a", "winner-b"
	inputA := task.UpdateInput{ExpectedRevision: 1, Title: &titleA, IdempotencyKey: "race-a"}
	inputB := task.UpdateInput{ExpectedRevision: 1, Title: &titleB, IdempotencyKey: "race-b"}
	results := runConcurrentMutations(t,
		func() (task.Task, error) {
			return storeA.UpdateGlobalTask(mutationContext("local", "session-race", "run-a"), created.ID, inputA)
		},
		func() (task.Task, error) {
			return storeB.UpdateGlobalTask(mutationContext("local", "session-race", "run-b"), created.ID, inputB)
		},
	)
	var winner task.Task
	var loserInput task.UpdateInput
	winners, losers := 0, 0
	for _, result := range results {
		if result.err == nil {
			winners++
			winner = result.task
			continue
		}
		var conflict *task.ConflictError
		if !errors.As(result.err, &conflict) || conflict.ID != created.ID || conflict.Expected != 1 || conflict.Current != 2 {
			t.Fatalf("loser error = %v", result.err)
		}
		losers++
		if winner.Title == titleA {
			loserInput = inputB
		} else {
			loserInput = inputA
		}
	}
	if winners != 1 || losers != 1 || winner.Revision != 2 {
		t.Fatalf("same-revision results = %+v", results)
	}
	// Result arrival order can put the loser first; select its input from durable winner state.
	if winner.Title == titleA {
		loserInput = inputB
	} else {
		loserInput = inputA
	}
	laterTitle := "later"
	if _, err := storeA.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{
		ExpectedRevision: 2, Title: &laterTitle, IdempotencyKey: "race-later",
	}); err != nil {
		t.Fatal(err)
	}
	_, err = storeB.UpdateGlobalTask(mutationContext("local", "session-race", "retry"), created.ID, loserInput)
	var replayed *task.ConflictError
	if !errors.As(err, &replayed) || replayed.Current != 2 {
		t.Fatalf("replayed loser error = %v, want original current revision 2", err)
	}
	events, err := storeA.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 4 {
		t.Fatalf("same-revision events = %+v, %v", events, err)
	}
}

func TestConcurrentDifferentTaskUpdatesBothProgress(t *testing.T) {
	storeA, storeB := openIndependentTaskStores(t)
	ctx := mutationContext("local", "session-different", "seed")
	first, err := storeA.CreateGlobalTask(ctx, task.CreateInput{Title: "first", IdempotencyKey: "different-create-a"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := storeA.CreateGlobalTask(ctx, task.CreateInput{Title: "second", IdempotencyKey: "different-create-b"})
	if err != nil {
		t.Fatal(err)
	}
	titleA, titleB := "first done", "second done"
	results := runConcurrentMutations(t,
		func() (task.Task, error) {
			return storeA.UpdateGlobalTask(mutationContext("local", "session-different", "run-a"), first.ID,
				task.UpdateInput{ExpectedRevision: 1, Title: &titleA, IdempotencyKey: "different-update-a"})
		},
		func() (task.Task, error) {
			return storeB.UpdateGlobalTask(mutationContext("local", "session-different", "run-b"), second.ID,
				task.UpdateInput{ExpectedRevision: 1, Title: &titleB, IdempotencyKey: "different-update-b"})
		},
	)
	if results[0].err != nil || results[1].err != nil || results[0].task.Revision != 2 || results[1].task.Revision != 2 {
		t.Fatalf("different Task results = %+v", results)
	}
}

func TestTaskMutationCancellationAndFailureRollBackIdempotency(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "session-rollback", "run")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	input := task.CreateInput{Title: "retry after cancel", IdempotencyKey: "cancelled-key"}
	if _, err := store.CreateGlobalTask(cancelled, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled create error = %v", err)
	}
	created, err := store.CreateGlobalTask(ctx, input)
	if err != nil {
		t.Fatalf("live retry after cancellation: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER reject_task_revision_insert BEFORE INSERT ON task_revisions
		BEGIN SELECT RAISE(ABORT, 'forced revision failure'); END;
	`); err != nil {
		t.Fatal(err)
	}
	title := "rolled back"
	update := task.UpdateInput{ExpectedRevision: 1, Title: &title, IdempotencyKey: "rollback-key"}
	if _, err := store.UpdateGlobalTask(ctx, created.ID, update); err == nil || !strings.Contains(err.Error(), "forced revision failure") {
		t.Fatalf("forced failure error = %v", err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_task_revision_insert`); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetGlobalTask(context.Background(), created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("failed mutation changed Task: %+v, %v", got, err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("failed mutation left event: %+v, %v", events, err)
	}
	updated, err := store.UpdateGlobalTask(ctx, created.ID, update)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("retry after rollback = %+v, %v", updated, err)
	}
}

func TestTaskIdempotencyRecordsContainOnlySafeMetadata(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "session-safe", "run-safe")
	rawKey := "credential-like-secret-identity"
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "task-content-marker", Description: "description-marker", IdempotencyKey: task.IdempotencyKey(rawKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "different-content-marker", IdempotencyKey: task.IdempotencyKey(rawKey),
	}); !errors.Is(err, task.ErrIdempotencyConflict) {
		t.Fatalf("conflicting reuse error = %v", err)
	}
	for _, table := range []string{"task_mutation_results", "task_idempotency_conflicts"} {
		rows, err := db.Query(`SELECT * FROM ` + table)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		for rows.Next() {
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			for _, value := range values {
				text := stringValue(value)
				for _, forbidden := range []string{rawKey, "task-content-marker", "description-marker", "different-content-marker"} {
					if strings.Contains(text, forbidden) {
						rows.Close()
						t.Fatalf("%s persisted forbidden content %q in columns %v", table, forbidden, columns)
					}
				}
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.GetGlobalTask(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
}

func stringValue(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(value)
	}
}

func TestTaskIdempotencyHistoryIsAppendOnly(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "session-append-only", "run")
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "append-only", IdempotencyKey: "append-only-create",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "different", IdempotencyKey: "append-only-create",
	}); !errors.Is(err, task.ErrIdempotencyConflict) {
		t.Fatalf("conflicting reuse error = %v", err)
	}

	for _, statement := range []string{
		`UPDATE task_revisions SET title = 'changed' WHERE task_id = '` + string(created.ID) + `'`,
		`DELETE FROM task_revisions WHERE task_id = '` + string(created.ID) + `'`,
		`UPDATE task_mutation_results SET run_id = 'changed'`,
		`DELETE FROM task_mutation_results`,
		`UPDATE task_idempotency_conflicts SET operation = 'update'`,
		`DELETE FROM task_idempotency_conflicts`,
	} {
		if _, err := db.Exec(statement); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("statement %q error = %v, want append-only rejection", statement, err)
		}
	}
}
