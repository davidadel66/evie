package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

// Stage 13 (2026-10-04): `todo show` prints a Task Tree's orchestration
// progress: each Task's plan, claim and latest build, then todo_ready's
// batches, blocked Tasks with reasons and Tasks that look finished.
func TestTodoShowPrintsTheTreesOrchestrationProgress(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	bound, err := ownerContext(ctx, store, "", uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateGlobalTask(bound, task.CreateInput{Title: "Ship the feature", IdempotencyKey: "root"})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := store.DecomposeGlobalTask(bound, root.ID, task.DecomposeInput{ExpectedRevision: root.Revision, IdempotencyKey: "plan", Children: []task.ChildInput{
		{Title: "A", Paths: []string{"pkg/a"}}, {Title: "B", Paths: []string{"pkg/b"}},
		{Title: "C", Paths: []string{"pkg/c"}, DependsOnChildren: []int{0, 1}}, {Title: "D", Paths: []string{"pkg/a/extra.go"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	a, b, c, d := tree.Children[0], tree.Children[1], tree.Children[2], tree.Children[3]
	var stdout, stderr bytes.Buffer
	if exit := runShow(ctx, store, []string{string(root.ID)}, &stdout, &stderr); exit != 0 {
		t.Fatalf("show exit=%d stderr=%s", exit, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		`- status=open id=` + string(root.ID) + ` title="Ship the feature" claim=- build=-`,
		`  - status=open id=` + string(c.ID) + ` title="C" claim=- build=- depends_on=` + string(a.ID) + `,` + string(b.ID) + ` paths=pkg/c`,
		"Counts: open=5",
		"Capacity: batch=2",
		`  1. "A" (` + string(a.ID) + `), "B" (` + string(b.ID) + `)`,
		`  2. "D" (` + string(d.ID) + `)`,
		`Blocked: "C" (` + string(c.ID) + `) waiting_on_dependencies: depends on Task ` + string(a.ID),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show output lacks %q:\n%s", want, out)
		}
	}
	// todo get shows the plan too.
	stdout.Reset()
	if exit := runGet(ctx, store, []string{string(c.ID)}, &stdout, &stderr); exit != 0 || !strings.Contains(stdout.String(), "depends_on="+string(a.ID)+","+string(b.ID)+" paths=pkg/c") {
		t.Fatalf("get = %d %s", exit, stdout.String())
	}
	if exit := runShow(ctx, store, []string{"missing"}, &stdout, &stderr); exit == 0 || !strings.Contains(stderr.String(), "not found") {
		t.Fatalf("show of an unknown tree = %d %s", exit, stderr.String())
	}
}
