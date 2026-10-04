package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/task"
)

// Stage 13 (2026-10-04): Task dependencies and paths, persisted with
// revision checks and idempotency, cycle-checked against the tree, and read
// back through todo_ready's read model.

func planStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db), path
}

// fourTasks is the Stage 13 acceptance tree: A and B are independent, C
// depends on both, D's paths overlap A's.
func fourTasks(t *testing.T, store *Store, ctx context.Context) (task.Task, []task.Task) {
	t.Helper()
	root, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "Feature", IdempotencyKey: "root"})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := store.DecomposeGlobalTask(ctx, root.ID, task.DecomposeInput{ExpectedRevision: root.Revision, IdempotencyKey: "plan", Children: []task.ChildInput{
		{Title: "A", Paths: []string{"pkg/a/"}},
		{Title: "B", Paths: []string{"./pkg/b"}},
		{Title: "C", Paths: []string{"pkg/c"}, DependsOnChildren: []int{0, 1}},
		{Title: "D", Paths: []string{"pkg/a/extra.go"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return tree.Parent, tree.Children
}

func TestTaskPlanFieldsPersistReplayAndKeepRevisionChecks(t *testing.T) {
	store, path := planStore(t)
	ctx := attributedTaskContext()
	root, children := fourTasks(t, store, ctx)
	a, b, c, d := children[0], children[1], children[2], children[3]
	if !reflect.DeepEqual(a.Paths, []string{"pkg/a"}) || !reflect.DeepEqual(b.Paths, []string{"pkg/b"}) ||
		!reflect.DeepEqual(c.DependsOn, []task.ID{a.ID, b.ID}) || d.DependsOn != nil {
		t.Fatalf("decomposed plan = %+v %+v %+v %+v", a, b, c, d)
	}
	// An exact retry replays the same children, plan included.
	replayed, err := store.DecomposeGlobalTask(ctx, root.ID, task.DecomposeInput{ExpectedRevision: 1, IdempotencyKey: "plan", Children: []task.ChildInput{
		{Title: "A", Paths: []string{"pkg/a/"}}, {Title: "B", Paths: []string{"./pkg/b"}},
		{Title: "C", Paths: []string{"pkg/c"}, DependsOnChildren: []int{0, 1}}, {Title: "D", Paths: []string{"pkg/a/extra.go"}},
	}})
	if err != nil || !reflect.DeepEqual(replayed.Children, children) {
		t.Fatalf("replayed decomposition = %+v %v", replayed.Children, err)
	}
	// Updates replace the lists under revision checks; empty clears.
	deps := []task.ID{a.ID}
	updated, err := store.UpdateGlobalTask(ctx, d.ID, task.UpdateInput{ExpectedRevision: d.Revision, DependsOn: &deps, IdempotencyKey: "d-deps"})
	if err != nil || !reflect.DeepEqual(updated.DependsOn, deps) || updated.Revision != d.Revision+1 {
		t.Fatalf("dependency update = %+v %v", updated, err)
	}
	if _, err := store.UpdateGlobalTask(ctx, d.ID, task.UpdateInput{ExpectedRevision: d.Revision, DependsOn: &deps, IdempotencyKey: "d-stale"}); !errors.Is(err, task.ErrConflict) {
		t.Fatalf("stale dependency update = %v, want a revision conflict", err)
	}
	again, err := store.UpdateGlobalTask(ctx, d.ID, task.UpdateInput{ExpectedRevision: d.Revision, DependsOn: &deps, IdempotencyKey: "d-deps"})
	if err != nil || !reflect.DeepEqual(again, updated) {
		t.Fatalf("replayed dependency update = %+v %v", again, err)
	}
	empty := []string{}
	cleared, err := store.UpdateGlobalTask(ctx, d.ID, task.UpdateInput{ExpectedRevision: updated.Revision, Paths: &empty, IdempotencyKey: "d-paths"})
	if err != nil || cleared.Paths != nil || !reflect.DeepEqual(cleared.DependsOn, deps) {
		t.Fatalf("cleared paths = %+v %v", cleared, err)
	}
	if _, err := store.UpdateGlobalTask(ctx, d.ID, task.UpdateInput{ExpectedRevision: cleared.Revision, Paths: &empty, IdempotencyKey: "d-noop"}); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("a no-op plan patch = %v", err)
	}
	// A child created alone carries its plan; the tree read shows it.
	child, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "E", ParentID: root.ID, ExpectedParentRevision: replayed.Parent.Revision,
		DependsOn: []task.ID{c.ID}, Paths: []string{"docs"}, IdempotencyKey: "e"})
	if err != nil || !reflect.DeepEqual(child.DependsOn, []task.ID{c.ID}) || !reflect.DeepEqual(child.Paths, []string{"docs"}) {
		t.Fatalf("planned child = %+v %v", child, err)
	}
	tree, err := store.GetGlobalTaskTree(context.Background(), root.ID, task.TreeQuery{})
	if err != nil || len(tree.Children) != 5 || !reflect.DeepEqual(tree.Children[2].Task.DependsOn, c.DependsOn) {
		t.Fatalf("tree = %+v %v", tree, err)
	}
	// Plans survive a reopen.
	reopened, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := NewStore(reopened).GetGlobalTask(context.Background(), c.ID)
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("reopened C = %+v %v, want %+v", got, err, c)
	}
}

func TestTaskDependenciesStayInOneTreeAndAcyclic(t *testing.T) {
	store, _ := planStore(t)
	ctx := attributedTaskContext()
	root, children := fourTasks(t, store, ctx)
	a, c := children[0], children[2]
	other, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "Other tree", IdempotencyKey: "other"})
	if err != nil {
		t.Fatal(err)
	}
	group, err := store.DecomposeGlobalTask(ctx, a.ID, task.DecomposeInput{ExpectedRevision: a.Revision, IdempotencyKey: "a-steps", Children: []task.ChildInput{{Title: "A1"}}})
	if err != nil {
		t.Fatal(err)
	}
	a1 := group.Children[0]
	a = group.Parent
	for name, c := range map[string]struct {
		id      task.ID
		deps    []task.ID
		message string
	}{
		"self":        {a.ID, []task.ID{a.ID}, "the Task itself"},
		"other tree":  {a.ID, []task.ID{other.ID}, "not in this Task Tree"},
		"ancestor":    {a1.ID, []task.ID{a.ID}, "ancestor"},
		"descendant":  {a.ID, []task.ID{a1.ID}, "descendant"},
		"root":        {a.ID, []task.ID{root.ID}, "ancestor"},
		"cycle":       {a.ID, []task.ID{c.ID}, "cycle"},
		"unknown":     {a.ID, []task.ID{"missing"}, "not in this Task Tree"},
		"inheritance": {a1.ID, []task.ID{c.ID}, "cycle"},
	} {
		current, err := store.GetGlobalTask(context.Background(), c.id)
		if err != nil {
			t.Fatal(err)
		}
		deps := c.deps
		_, err = store.UpdateGlobalTask(ctx, c.id, task.UpdateInput{ExpectedRevision: current.Revision, DependsOn: &deps, IdempotencyKey: task.IdempotencyKey("bad-" + name)})
		var inputErr *task.InputError
		if !errors.As(err, &inputErr) || inputErr.Field != "depends_on" || !strings.Contains(err.Error(), c.message) {
			t.Errorf("%s: %v, want a depends_on error naming %q", name, err, c.message)
		}
		after, _ := store.GetGlobalTask(context.Background(), c.id)
		if after.Revision != current.Revision || !reflect.DeepEqual(after.DependsOn, current.DependsOn) {
			t.Errorf("%s: a refused dependency changed the Task: %+v", name, after)
		}
	}
	// A batch whose children wait for each other is refused whole.
	current, _ := store.GetGlobalTask(context.Background(), root.ID)
	_, err = store.DecomposeGlobalTask(ctx, root.ID, task.DecomposeInput{ExpectedRevision: current.Revision, IdempotencyKey: "loop", Children: []task.ChildInput{
		{Title: "X", DependsOnChildren: []int{1}}, {Title: "Y", DependsOnChildren: []int{0}},
	}})
	if !errors.Is(err, task.ErrInvalidInput) || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cyclic batch = %v", err)
	}
	if listed, _ := store.ListGlobalTasks(context.Background(), task.ListFilter{RootID: root.ID, IncludeHistory: true}); len(listed) != 6 {
		t.Fatalf("a refused batch wrote Tasks: %d", len(listed))
	}
	// A root cannot depend on anything.
	if _, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "R", DependsOn: []task.ID{a.ID}, IdempotencyKey: "r"}); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("dependent root = %v", err)
	}
}

func TestDelegatedWorkersCannotPlanAndSeeNoDependencies(t *testing.T) {
	store, _ := planStore(t)
	owner, err := store.CreateGlobalSessionWithComposition(context.Background(), standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx := scopedTaskContext(owner, "seed")
	_, children := fourTasks(t, store, ownerCtx)
	c := children[2]
	worker, err := store.CreateDelegatedSessionWithComposition(context.Background(), owner.ID,
		todoCapabilityReceipt(t, task.CapabilityGet, task.CapabilityUpdate, task.CapabilityDecompose))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.IssueTaskAccessGrant(scopedTaskContext(owner, "delegate"), task.GrantInput{GranteeSessionID: string(worker.ID), RootID: c.ID, Level: task.AccessContribute}); err != nil {
		t.Fatal(err)
	}
	workerCtx := scopedTaskContext(worker, "work")
	seen, err := store.GetGlobalTask(workerCtx, c.ID)
	if err != nil || seen.DependsOn != nil || !reflect.DeepEqual(seen.Paths, c.Paths) {
		t.Fatalf("worker's view of its Task = %+v %v", seen, err)
	}
	paths := []string{"elsewhere"}
	if _, err := store.UpdateGlobalTask(workerCtx, c.ID, task.UpdateInput{ExpectedRevision: c.Revision, Paths: &paths, IdempotencyKey: "w-paths"}); !strings.Contains(fmtErr(err), "orchestrating conversation") {
		t.Fatalf("worker planning paths = %v", err)
	}
	if _, err := store.DecomposeGlobalTask(workerCtx, c.ID, task.DecomposeInput{ExpectedRevision: c.Revision, IdempotencyKey: "w-steps",
		Children: []task.ChildInput{{Title: "1"}, {Title: "2", DependsOnChildren: []int{0}}}}); !strings.Contains(fmtErr(err), "orchestrating conversation") {
		t.Fatalf("worker planning subtask dependencies = %v", err)
	}
	if _, err := store.DecomposeGlobalTask(workerCtx, c.ID, task.DecomposeInput{ExpectedRevision: c.Revision, IdempotencyKey: "w-plain",
		Children: []task.ChildInput{{Title: "1"}}}); err != nil {
		t.Fatalf("worker decomposing without a plan = %v", err)
	}
	if _, err := store.ReadyGlobalTasks(workerCtx, task.ReadyQuery{TreeID: c.ID, Limits: testLimits}); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("worker reading readiness = %v", err)
	}
}

func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

var testLimits = task.DispatchLimits{MaxBatch: 8, PerParent: 2, Runtime: 4, PerTurn: 16}

func TestReadyGlobalTasksBatchesTheTreeAndReleasesDeadClaims(t *testing.T) {
	store, _ := planStore(t)
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	lease := createClaimLease(t, store, "orchestrator", time.Minute)
	ctx := claimMutationContext(lease, "turn")
	root, children := fourTasks(t, store, ctx)
	a, b, c, d := children[0], children[1], children[2], children[3]
	ready, err := store.ReadyGlobalTasks(ctx, task.ReadyQuery{TreeID: root.ID, Limits: testLimits})
	if err != nil {
		t.Fatal(err)
	}
	if got := readyBatches(ready); !reflect.DeepEqual(got, [][]task.ID{{a.ID, b.ID}, {d.ID}}) {
		t.Fatalf("batches = %v", got)
	}
	if len(ready.Blocked) != 1 || ready.Blocked[0].TaskID != c.ID || ready.Blocked[0].Reasons[0].Code != task.ReasonWaitingOnDependencies {
		t.Fatalf("blocked = %+v", ready.Blocked)
	}
	// This conversation claims B: B waits until it releases it.
	if _, err := store.ClaimGlobalTask(ctx, b.ID, task.ClaimInput{IdempotencyKey: "claim-b"}); err != nil {
		t.Fatal(err)
	}
	ready, _ = store.ReadyGlobalTasks(ctx, task.ReadyQuery{TreeID: root.ID, Limits: testLimits})
	if got := readyBatches(ready); !reflect.DeepEqual(got, [][]task.ID{{a.ID}, {d.ID}}) || ready.Blocked[0].TaskID != b.ID || ready.Blocked[0].Reasons[0].Code != task.ReasonClaimed {
		t.Fatalf("with B claimed: batches %v blocked %+v", got, ready.Blocked)
	}
	// The claiming turn dies (its lease expires): B is ready again, marked
	// as holding a stale claim that dispatch releases.
	now = now.Add(2 * time.Minute)
	later := createClaimLease(t, store, "next-turn", time.Minute)
	ready, err = store.ReadyGlobalTasks(claimMutationContext(later, "next"), task.ReadyQuery{TreeID: root.ID, Limits: testLimits})
	if err != nil {
		t.Fatal(err)
	}
	if got := readyBatches(ready); !reflect.DeepEqual(got, [][]task.ID{{a.ID, b.ID}, {d.ID}}) || !ready.Batches[0][1].StaleClaim {
		t.Fatalf("after the claim's turn died: %+v", ready.Batches)
	}
	// Narrowing and subtrees.
	ready, _ = store.ReadyGlobalTasks(ctx, task.ReadyQuery{TreeID: root.ID, Max: 1, Limits: testLimits})
	if ready.Capacity.BatchSize != 1 || len(ready.Batches) != 3 {
		t.Fatalf("max 1 = %+v", ready)
	}
	sub, err := store.ReadyGlobalTasks(ctx, task.ReadyQuery{TreeID: c.ID, Limits: testLimits})
	if err != nil || sub.TreeID != c.ID || len(sub.Blocked) != 1 || len(sub.Batches) != 0 {
		t.Fatalf("C's subtree = %+v %v", sub, err)
	}
	if _, err := store.ReadyGlobalTasks(ctx, task.ReadyQuery{TreeID: "missing", Limits: testLimits}); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("unknown tree = %v", err)
	}
}

func readyBatches(r task.Readiness) [][]task.ID {
	out := [][]task.ID{}
	for _, batch := range r.Batches {
		var ids []task.ID
		for _, item := range batch {
			ids = append(ids, item.TaskID)
		}
		out = append(out, ids)
	}
	return out
}

// A database written before Stage 13 gains the plan columns once; its Tasks
// read with no plan and its revision history still replays.
func TestTaskPlanUpgradeKeepsEarlierTasks(t *testing.T) {
	store, path := planStore(t)
	ctx := attributedTaskContext()
	earlier, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "earlier", IdempotencyKey: "earlier"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`ALTER TABLE tasks DROP COLUMN depends_on`, `ALTER TABLE tasks DROP COLUMN paths`,
		`ALTER TABLE task_revisions DROP COLUMN depends_on`, `ALTER TABLE task_revisions DROP COLUMN paths`,
	} {
		if _, err := raw.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	upgraded := NewStore(db)
	got, err := upgraded.GetGlobalTask(context.Background(), earlier.ID)
	if err != nil || !reflect.DeepEqual(got, earlier) {
		t.Fatalf("earlier Task after upgrade = %+v %v", got, err)
	}
	replayed, err := upgraded.CreateGlobalTask(ctx, task.CreateInput{Title: "earlier", IdempotencyKey: "earlier"})
	if err != nil || !reflect.DeepEqual(replayed, earlier) {
		t.Fatalf("replayed earlier create = %+v %v", replayed, err)
	}
	for _, table := range []string{"tasks", "task_revisions"} {
		for _, column := range []string{"depends_on", "paths"} {
			if present, err := tableHasColumn(context.Background(), db, table, column); err != nil || !present {
				t.Fatalf("%s.%s missing after upgrade: %v", table, column, err)
			}
		}
	}
}
