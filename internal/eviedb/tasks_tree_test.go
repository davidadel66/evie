package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/task"
)

func TestTaskTreeDecompositionIsOrderedIdempotentAndDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	fixed := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixed }
	ctx := mutationContext("local", "tree-session", "tree-run")
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "root-create"})
	if err != nil {
		t.Fatal(err)
	}
	input := task.DecomposeInput{
		ExpectedRevision: 1,
		Children: []task.ChildInput{
			{Title: "research"},
			{Title: "implement", Priority: 4},
			{Title: "verify", DueDate: "2026-09-03"},
		},
		IdempotencyKey: "root-decompose",
	}
	decomposed, err := store.DecomposeGlobalTask(ctx, root.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if decomposed.Parent.Revision != 2 || titles(decomposed.Children) != "research,implement,verify" {
		t.Fatalf("decomposition = %+v", decomposed)
	}
	for i, child := range decomposed.Children {
		if child.ParentID != root.ID || child.RootID != root.ID || child.SiblingOrder != uint64(i+1) ||
			child.Scope != task.ScopeGlobal || child.Revision != 1 {
			t.Fatalf("child %d = %+v", i, child)
		}
	}
	rootEvents, err := store.ListTaskEvents(context.Background(), root.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantIdentity := task.IdempotencySHA256(input.IdempotencyKey)
	if len(rootEvents) != 2 || rootEvents[1].Operation != task.OperationDecompose ||
		rootEvents[1].PreviousRevision != 1 || rootEvents[1].ResultingRevision != 2 ||
		rootEvents[1].IdempotencySHA256 != wantIdentity {
		t.Fatalf("root decomposition events = %+v", rootEvents)
	}
	for _, child := range decomposed.Children {
		events, err := store.ListTaskEvents(context.Background(), child.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 1 || events[0].Operation != task.OperationCreate ||
			events[0].ResultingRevision != 1 || events[0].IdempotencySHA256 != wantIdentity {
			t.Fatalf("child %q decomposition events = %+v", child.ID, events)
		}
	}
	replayed, err := store.DecomposeGlobalTask(
		mutationContext("local", "tree-session", "retry-run"), root.ID, input,
	)
	if err != nil || !reflect.DeepEqual(replayed, decomposed) {
		t.Fatalf("decomposition replay = %+v, %v; want %+v", replayed, err, decomposed)
	}
	replayedEvents, err := store.ListTaskEvents(context.Background(), root.ID)
	if err != nil || !reflect.DeepEqual(replayedEvents, rootEvents) {
		t.Fatalf("decomposition replay events = %+v, %v; want %+v", replayedEvents, err, rootEvents)
	}
	conflicting := input
	conflicting.Children = []task.ChildInput{{Title: "different"}}
	if _, err := store.DecomposeGlobalTask(ctx, root.ID, conflicting); !errors.Is(err, task.ErrIdempotencyConflict) {
		t.Fatalf("conflicting decomposition reuse error = %v", err)
	}
	changedTitle := "research done"
	if _, err := preClaimLifecycleUpdate(store, ctx, decomposed.Children[0].ID, task.UpdateInput{
		ExpectedRevision: 1, Title: &changedTitle, IdempotencyKey: "change-child",
	}); err != nil {
		t.Fatal(err)
	}
	replayed, err = store.DecomposeGlobalTask(ctx, root.ID, input)
	if err != nil || !reflect.DeepEqual(replayed, decomposed) {
		t.Fatalf("historical decomposition replay = %+v, %v; want %+v", replayed, err, decomposed)
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
	replayed, err = reopened.DecomposeGlobalTask(ctx, root.ID, input)
	if err != nil || !reflect.DeepEqual(replayed, decomposed) {
		t.Fatalf("reopened decomposition replay = %+v, %v; want %+v", replayed, err, decomposed)
	}
}

func TestTaskTreeSupportsDeepChildrenTraversalFiltersAndBounds(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	store.now = func() time.Time { return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC) }
	ctx := mutationContext("local", "deep-tree", "run")
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "deep-root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "child", ParentID: root.ID, ExpectedParentRevision: 1, IdempotencyKey: "deep-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "grandchild", ParentID: child.ID, ExpectedParentRevision: 1, IdempotencyKey: "deep-grandchild",
	})
	if err != nil {
		t.Fatal(err)
	}
	greatGrandchild, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "great-grandchild", ParentID: grandchild.ID, ExpectedParentRevision: 1, IdempotencyKey: "deep-great-grandchild",
	})
	if err != nil {
		t.Fatal(err)
	}
	if greatGrandchild.RootID != root.ID || greatGrandchild.ParentID != grandchild.ID {
		t.Fatalf("deep descendant = %+v", greatGrandchild)
	}
	rootEvents, err := store.ListTaskEvents(context.Background(), root.ID)
	if err != nil || len(rootEvents) != 2 || rootEvents[1].Operation != task.OperationCreate {
		t.Fatalf("single-child parent events = %+v, %v", rootEvents, err)
	}
	listed, err := store.ListGlobalTasks(context.Background(), task.ListFilter{RootID: root.ID})
	if err != nil || titles(listed) != "root,child,grandchild,great-grandchild" {
		t.Fatalf("root traversal = %+v, %v", listed, err)
	}
	direct, err := store.ListGlobalTasks(context.Background(), task.ListFilter{ParentID: child.ID})
	if err != nil || len(direct) != 1 || direct[0].ID != grandchild.ID {
		t.Fatalf("parent traversal = %+v, %v", direct, err)
	}
	tree, err := store.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{MaxDepth: 2})
	if err != nil || !tree.Truncated || len(tree.Children) != 1 || len(tree.Children[0].Children) != 1 ||
		len(tree.Children[0].Children[0].Children) != 0 {
		t.Fatalf("bounded tree = %+v, %v", tree, err)
	}
	full, err := store.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{MaxDepth: 4})
	if err != nil || full.Truncated || full.Children[0].Children[0].Children[0].Task.ID != greatGrandchild.ID {
		t.Fatalf("full tree = %+v, %v", full, err)
	}
	completed := task.StatusCompleted
	if _, err := preClaimLifecycleUpdate(store, ctx, greatGrandchild.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &completed, IdempotencyKey: "complete-great-grandchild",
	}); err != nil {
		t.Fatal(err)
	}
	listed, err = store.ListGlobalTasks(context.Background(), task.ListFilter{RootID: root.ID})
	if err != nil || titles(listed) != "root,child,grandchild" {
		t.Fatalf("open-only tree traversal = %+v, %v", listed, err)
	}
	history, err := store.ListGlobalTasks(context.Background(), task.ListFilter{RootID: root.ID, IncludeHistory: true})
	if err != nil || titles(history) != "root,child,grandchild,great-grandchild" {
		t.Fatalf("history tree traversal = %+v, %v", history, err)
	}
	openTree, err := store.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{MaxDepth: 4})
	if err != nil || openTree.Truncated || len(openTree.Children[0].Children[0].Children) != 0 {
		t.Fatalf("open-only recursive tree = %+v, %v", openTree, err)
	}
	historyTree, err := store.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{
		MaxDepth: 4, IncludeHistory: true,
	})
	if err != nil || historyTree.Truncated ||
		historyTree.Children[0].Children[0].Children[0].Task.ID != greatGrandchild.ID {
		t.Fatalf("history recursive tree = %+v, %v", historyTree, err)
	}
}

func TestDelegatedSessionCannotCreateTaskTreeRoot(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: "local", SessionID: "child-session", RunID: "run", ParentSessionID: "parent-session",
	})
	if _, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "forbidden root", IdempotencyKey: "delegated-root",
	}); !errors.Is(err, task.ErrRootCreationDenied) {
		t.Fatalf("delegated root error = %v", err)
	}
	listed, err := store.ListGlobalTasks(context.Background(), task.ListFilter{IncludeHistory: true})
	if err != nil || len(listed) != 0 {
		t.Fatalf("delegated root changed Tasks: %+v, %v", listed, err)
	}
}

func TestChildCreationRejectsMissingStaleAndTerminalParents(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "child-rejections", "run")
	if _, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "orphan", ParentID: "missing", ExpectedParentRevision: 1, IdempotencyKey: "missing-parent",
	}); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("missing parent error = %v", err)
	}
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "rejection-root"})
	if err != nil {
		t.Fatal(err)
	}
	staleInput := task.CreateInput{
		Title: "stale", ParentID: root.ID, ExpectedParentRevision: 2, IdempotencyKey: "stale-parent",
	}
	if _, err := store.CreateGlobalTask(ctx, staleInput); !errors.Is(err, task.ErrConflict) {
		t.Fatalf("stale parent error = %v", err)
	}
	if _, err := store.CreateGlobalTask(ctx, staleInput); !errors.Is(err, task.ErrConflict) {
		t.Fatalf("replayed stale parent error = %v", err)
	}
	completed := task.StatusCompleted
	root, err = preClaimLifecycleUpdate(store, ctx, root.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &completed, IdempotencyKey: "terminal-root",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "late child", ParentID: root.ID, ExpectedParentRevision: root.Revision, IdempotencyKey: "terminal-child",
	}); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("terminal parent error = %v", err)
	}
	listed, err := store.ListGlobalTasks(context.Background(), task.ListFilter{RootID: root.ID, IncludeHistory: true})
	if err != nil || len(listed) != 1 {
		t.Fatalf("rejected child creation changed tree = %+v, %v", listed, err)
	}
}

func TestParentCompletionRequiresEveryDescendantTerminal(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "completion-tree", "run")
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "complete-root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "child", ParentID: root.ID, ExpectedParentRevision: 1, IdempotencyKey: "complete-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "grandchild", ParentID: child.ID, ExpectedParentRevision: 1, IdempotencyKey: "complete-grandchild",
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled := task.StatusCancelled
	if _, err := preClaimLifecycleUpdate(store, ctx, child.ID, task.UpdateInput{
		ExpectedRevision: 2, Status: &cancelled, IdempotencyKey: "cancel-direct-child",
	}); err != nil {
		t.Fatal(err)
	}
	completed := task.StatusCompleted
	_, err = preClaimLifecycleUpdate(store, ctx, root.ID, task.UpdateInput{
		ExpectedRevision: 2, Status: &completed, IdempotencyKey: "complete-parent-too-early",
	})
	if !errors.Is(err, task.ErrActiveDescendants) {
		t.Fatalf("active descendant completion error = %v", err)
	}
	if _, err := preClaimLifecycleUpdate(store, ctx, grandchild.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &cancelled, IdempotencyKey: "cancel-grandchild-update",
	}); err != nil {
		t.Fatal(err)
	}
	open := task.StatusOpen
	_, err = preClaimLifecycleUpdate(store, ctx, grandchild.ID, task.UpdateInput{
		ExpectedRevision: 2, Status: &open, IdempotencyKey: "reopen-grandchild-too-early",
	})
	if !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("reopen beneath terminal ancestor error = %v", err)
	}
	parent, err := preClaimLifecycleUpdate(store, ctx, root.ID, task.UpdateInput{
		ExpectedRevision: 2, Status: &completed, IdempotencyKey: "complete-parent",
	})
	if err != nil || parent.Status != task.StatusCompleted {
		t.Fatalf("completed parent = %+v, %v", parent, err)
	}
	_, err = preClaimLifecycleUpdate(store, ctx, child.ID, task.UpdateInput{
		ExpectedRevision: 3, Status: &open, IdempotencyKey: "reopen-child-too-early",
	})
	if !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("reopen beneath terminal parent error = %v", err)
	}
}

func TestTaskTreeKeepsActiveWorkVisibleThroughTerminalStructuralAncestor(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "terminal-ancestor-tree", "run")
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "visible-root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "child", ParentID: root.ID, ExpectedParentRevision: 1, IdempotencyKey: "visible-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "grandchild", ParentID: child.ID, ExpectedParentRevision: 1, IdempotencyKey: "visible-grandchild",
	})
	if err != nil {
		t.Fatal(err)
	}
	blocked := task.StatusBlocked
	if _, err := preClaimLifecycleUpdate(store, ctx, grandchild.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &blocked, IdempotencyKey: "block-grandchild",
	}); err != nil {
		t.Fatal(err)
	}
	cancelled := task.StatusCancelled
	if _, err := preClaimLifecycleUpdate(store, ctx, child.ID, task.UpdateInput{
		ExpectedRevision: 2, Status: &cancelled, IdempotencyKey: "cancel-child",
	}); err != nil {
		t.Fatal(err)
	}
	tree, err := store.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{MaxDepth: 4})
	if err != nil || len(tree.Children) != 1 || tree.Children[0].Task.ID != child.ID ||
		len(tree.Children[0].Children) != 1 || tree.Children[0].Children[0].Task.ID != grandchild.ID ||
		tree.Children[0].Children[0].Task.Status != task.StatusBlocked {
		t.Fatalf("active work through terminal ancestor = %+v, %v", tree, err)
	}
}

func TestTaskTreeDepthLookaheadDoesNotExposeTerminalOnlyHistory(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "terminal-lookahead", "run")
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "lookahead-root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "child", ParentID: root.ID, ExpectedParentRevision: 1, IdempotencyKey: "lookahead-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "grandchild", ParentID: child.ID, ExpectedParentRevision: 1, IdempotencyKey: "lookahead-grandchild",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := task.StatusCompleted
	if _, err := preClaimLifecycleUpdate(store, ctx, grandchild.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &completed, IdempotencyKey: "lookahead-complete-grandchild",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := preClaimLifecycleUpdate(store, ctx, child.ID, task.UpdateInput{
		ExpectedRevision: 2, Status: &completed, IdempotencyKey: "lookahead-complete-child",
	}); err != nil {
		t.Fatal(err)
	}
	for _, maxDepth := range []int{1, 2} {
		tree, err := store.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{MaxDepth: maxDepth})
		if err != nil || len(tree.Children) != 0 || tree.Truncated {
			t.Fatalf("terminal-only tree at depth %d = %+v, %v", maxDepth, tree, err)
		}
	}
	history, err := store.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{
		MaxDepth: 2, IncludeHistory: true,
	})
	if err != nil || len(history.Children) != 1 || len(history.Children[0].Children) != 1 || history.Truncated {
		t.Fatalf("terminal history tree = %+v, %v", history, err)
	}
}

func TestTaskTreeReadAndTruncationShareOneSnapshot(t *testing.T) {
	storeA, storeB := openIndependentTaskStores(t)
	ctx := mutationContext("local", "tree-read-snapshot", "run")
	root, err := storeA.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "snapshot-root"})
	if err != nil {
		t.Fatal(err)
	}
	readComplete := make(chan struct{})
	writeComplete := make(chan struct{})
	storeA.afterTaskTreeRead = func() {
		close(readComplete)
		<-writeComplete
	}
	result := make(chan struct {
		tree task.Tree
		err  error
	}, 1)
	go func() {
		tree, err := storeA.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{MaxDepth: 1})
		result <- struct {
			tree task.Tree
			err  error
		}{tree: tree, err: err}
	}()
	<-readComplete
	if _, err := storeB.DecomposeGlobalTask(ctx, root.ID, task.DecomposeInput{
		ExpectedRevision: 1, Children: []task.ChildInput{{Title: "later child"}}, IdempotencyKey: "snapshot-decompose",
	}); err != nil {
		t.Fatal(err)
	}
	close(writeComplete)
	got := <-result
	if got.err != nil || got.tree.Task.Revision != 1 || len(got.tree.Children) != 0 || got.tree.Truncated {
		t.Fatalf("tree snapshot = %+v, %v", got.tree, got.err)
	}
	storeA.afterTaskTreeRead = nil
	current, err := storeA.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{MaxDepth: 1})
	if err != nil || current.Task.Revision != 2 || len(current.Children) != 1 || current.Truncated {
		t.Fatalf("current tree = %+v, %v", current, err)
	}
}

func TestDecompositionRollsBackValidationCancellationAndPersistenceFailure(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "decompose-rollback", "run")
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "rollback-root"})
	if err != nil {
		t.Fatal(err)
	}
	invalid := task.DecomposeInput{
		ExpectedRevision: 1,
		Children:         []task.ChildInput{{Title: "valid"}, {Title: " "}, {Title: "also valid"}},
		IdempotencyKey:   "invalid-middle",
	}
	if _, err := store.DecomposeGlobalTask(ctx, root.ID, invalid); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("invalid decomposition error = %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	cancelInput := task.DecomposeInput{
		ExpectedRevision: 1, Children: []task.ChildInput{{Title: "cancelled"}}, IdempotencyKey: "cancelled-decompose",
	}
	if _, err := store.DecomposeGlobalTask(cancelled, root.ID, cancelInput); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled decomposition error = %v", err)
	}
	if _, err := store.DecomposeGlobalTask(ctx, root.ID, cancelInput); err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER reject_decomposed_child BEFORE INSERT ON tasks
		WHEN NEW.title = 'explode'
		BEGIN SELECT RAISE(ABORT, 'forced decomposition failure'); END;
	`); err != nil {
		t.Fatal(err)
	}
	failureInput := task.DecomposeInput{
		ExpectedRevision: 2,
		Children:         []task.ChildInput{{Title: "before"}, {Title: "explode"}, {Title: "after"}},
		IdempotencyKey:   "failed-decompose",
	}
	if _, err := store.DecomposeGlobalTask(ctx, root.ID, failureInput); err == nil || !strings.Contains(err.Error(), "forced decomposition failure") {
		t.Fatalf("forced decomposition error = %v", err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_decomposed_child`); err != nil {
		t.Fatal(err)
	}
	gotRoot, err := store.GetGlobalTask(context.Background(), root.ID)
	if err != nil || gotRoot.Revision != 2 {
		t.Fatalf("failed decompositions changed parent = %+v, %v", gotRoot, err)
	}
	listed, err := store.ListGlobalTasks(context.Background(), task.ListFilter{RootID: root.ID, IncludeHistory: true})
	if err != nil || titles(listed) != "root,cancelled" {
		t.Fatalf("failed decompositions left children = %+v, %v", listed, err)
	}
	succeeded, err := store.DecomposeGlobalTask(ctx, root.ID, failureInput)
	if err != nil || len(succeeded.Children) != 3 || succeeded.Parent.Revision != 3 {
		t.Fatalf("retry after persistence rollback = %+v, %v", succeeded, err)
	}
}

type concurrentDecompositionResult struct {
	value task.Decomposition
	err   error
}

func runConcurrentDecompositions(
	t *testing.T,
	first func() (task.Decomposition, error),
	second func() (task.Decomposition, error),
) [2]concurrentDecompositionResult {
	t.Helper()
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan concurrentDecompositionResult, 2)
	var wait sync.WaitGroup
	for _, operation := range []func() (task.Decomposition, error){first, second} {
		wait.Add(1)
		go func(operation func() (task.Decomposition, error)) {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			value, err := operation()
			results <- concurrentDecompositionResult{value: value, err: err}
		}(operation)
	}
	<-ready
	<-ready
	close(start)
	wait.Wait()
	close(results)
	var got [2]concurrentDecompositionResult
	i := 0
	for result := range results {
		got[i] = result
		i++
	}
	return got
}

func TestConcurrentDecompositionRetriesCommitOneOrderedBatch(t *testing.T) {
	storeA, storeB := openIndependentTaskStores(t)
	ctx := mutationContext("local", "decompose-concurrent", "seed")
	root, err := storeA.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "concurrent-root"})
	if err != nil {
		t.Fatal(err)
	}
	input := task.DecomposeInput{
		ExpectedRevision: 1,
		Children:         []task.ChildInput{{Title: "one"}, {Title: "two"}, {Title: "three"}},
		IdempotencyKey:   "concurrent-batch",
	}
	results := runConcurrentDecompositions(t,
		func() (task.Decomposition, error) {
			return storeA.DecomposeGlobalTask(mutationContext("local", "decompose-concurrent", "run-a"), root.ID, input)
		},
		func() (task.Decomposition, error) {
			return storeB.DecomposeGlobalTask(mutationContext("local", "decompose-concurrent", "run-b"), root.ID, input)
		},
	)
	if results[0].err != nil || results[1].err != nil || !reflect.DeepEqual(results[0].value, results[1].value) {
		t.Fatalf("concurrent decomposition results = %+v", results)
	}
	listed, err := storeA.ListGlobalTasks(context.Background(), task.ListFilter{RootID: root.ID})
	if err != nil || titles(listed) != "root,one,two,three" {
		t.Fatalf("concurrent decomposition tree = %+v, %v", listed, err)
	}
	parent, err := storeA.GetGlobalTask(context.Background(), root.ID)
	if err != nil || parent.Revision != 2 {
		t.Fatalf("concurrent decomposition parent = %+v, %v", parent, err)
	}
}

func TestCompetingDecompositionsFromSameRevisionHaveOneWinner(t *testing.T) {
	storeA, storeB := openIndependentTaskStores(t)
	ctx := mutationContext("local", "decompose-race", "seed")
	root, err := storeA.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "race-root"})
	if err != nil {
		t.Fatal(err)
	}
	inputA := task.DecomposeInput{ExpectedRevision: 1, Children: []task.ChildInput{{Title: "a"}}, IdempotencyKey: "batch-a"}
	inputB := task.DecomposeInput{ExpectedRevision: 1, Children: []task.ChildInput{{Title: "b"}}, IdempotencyKey: "batch-b"}
	results := runConcurrentDecompositions(t,
		func() (task.Decomposition, error) { return storeA.DecomposeGlobalTask(ctx, root.ID, inputA) },
		func() (task.Decomposition, error) { return storeB.DecomposeGlobalTask(ctx, root.ID, inputB) },
	)
	winners, losers := 0, 0
	for _, result := range results {
		if result.err == nil {
			winners++
			continue
		}
		var conflict *task.ConflictError
		if !errors.As(result.err, &conflict) || conflict.Expected != 1 || conflict.Current != 2 {
			t.Fatalf("competing decomposition loser = %v", result.err)
		}
		losers++
	}
	if winners != 1 || losers != 1 {
		t.Fatalf("competing decomposition results = %+v", results)
	}
	listed, err := storeA.ListGlobalTasks(context.Background(), task.ListFilter{RootID: root.ID})
	if err != nil || len(listed) != 2 {
		t.Fatalf("competing decomposition tree = %+v, %v", listed, err)
	}
}

func TestTaskHierarchyRejectsOrdinaryCorruption(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "hierarchy-corruption", "run")
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "corruption-root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalTask(ctx, task.CreateInput{
		Title: "child", ParentID: root.ID, ExpectedParentRevision: 1, IdempotencyKey: "corruption-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE task_hierarchy SET parent_id = '` + string(child.ID) + `' WHERE task_id = '` + string(root.ID) + `'`,
		`UPDATE task_hierarchy SET parent_id = task_id WHERE task_id = '` + string(child.ID) + `'`,
		`DELETE FROM task_hierarchy WHERE task_id = '` + string(child.ID) + `'`,
	} {
		if _, err := db.Exec(statement); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("hierarchy corruption statement %q error = %v", statement, err)
		}
	}
}

func TestTaskDecompositionAuditMetadataIsAppendOnlyAndSecretFree(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "decomposition-audit", "run")
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "root", IdempotencyKey: "audit-root"})
	if err != nil {
		t.Fatal(err)
	}
	rawKey := task.IdempotencyKey("decomposition-secret-key")
	input := task.DecomposeInput{
		ExpectedRevision: 1,
		Children:         []task.ChildInput{{Title: "sensitive-child-title", Description: "sensitive-description"}},
		IdempotencyKey:   rawKey,
	}
	if _, err := store.DecomposeGlobalTask(ctx, root.ID, input); err != nil {
		t.Fatal(err)
	}
	conflict := input
	conflict.Children = []task.ChildInput{{Title: "different-sensitive-title"}}
	if _, err := store.DecomposeGlobalTask(ctx, root.ID, conflict); !errors.Is(err, task.ErrIdempotencyConflict) {
		t.Fatalf("conflicting decomposition = %v", err)
	}

	for _, table := range []string{
		"task_idempotency_claims",
		"task_event_idempotency",
		"task_decomposition_results",
		"task_decomposition_children",
		"task_decomposition_conflicts",
	} {
		assertTableOmitsStrings(t, db, table, []string{
			string(rawKey), "sensitive-child-title", "sensitive-description", "different-sensitive-title",
		})
	}

	for _, statement := range []string{
		`UPDATE task_idempotency_claims SET operation = 'update' WHERE operation = 'decompose'`,
		`DELETE FROM task_idempotency_claims WHERE operation = 'decompose'`,
		`UPDATE task_event_idempotency SET identity_sha256 = '` + strings.Repeat("a", 64) + `'`,
		`DELETE FROM task_event_idempotency`,
		`UPDATE task_hierarchy_events SET run_id = 'changed'`,
		`DELETE FROM task_hierarchy_events`,
		`UPDATE task_decomposition_results SET diagnostic_field = 'changed'`,
		`DELETE FROM task_decomposition_results`,
		`UPDATE task_decomposition_children SET child_order = 2`,
		`DELETE FROM task_decomposition_children`,
		`UPDATE task_decomposition_conflicts SET recorded_at = 'changed'`,
		`DELETE FROM task_decomposition_conflicts`,
	} {
		if _, err := db.Exec(statement); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("statement %q error = %v, want append-only rejection", statement, err)
		}
	}
}

func assertTableOmitsStrings(t *testing.T, db queryDatabase, table string, forbidden []string) {
	t.Helper()
	rows, err := db.Query(`SELECT * FROM ` + table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		for _, value := range values {
			text := stringValue(value)
			for _, candidate := range forbidden {
				if strings.Contains(text, candidate) {
					t.Fatalf("%s persisted forbidden content %q in columns %v", table, candidate, columns)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

type queryDatabase interface {
	Query(string, ...any) (*sql.Rows, error)
}

func titles(values []task.Task) string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = value.Title
	}
	return strings.Join(result, ",")
}
