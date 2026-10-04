package task

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Stage 13 (2026-10-04): Task dependencies, expected paths and the
// deterministic todo_ready read model.

func TestNormalizePathsCleansAndRefusesUnsafeOrVaguePaths(t *testing.T) {
	got, err := NormalizePaths([]string{"pkg/a/", "./pkg/b", "pkg/a", "docs/README.md"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"pkg/a", "pkg/b", "docs/README.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizePaths = %q, want %q", got, want)
	}
	if got, err := NormalizePaths(nil); err != nil || got != nil {
		t.Fatalf("no paths = %q %v, want nil", got, err)
	}
	tooMany := make([]string, MaxTaskPaths+1)
	for i := range tooMany {
		tooMany[i] = "p" + strings.Repeat("x", i)
	}
	for _, bad := range [][]string{
		{""}, {"."}, {"./"}, {"/etc/passwd"}, {"../outside"}, {"pkg/../../outside"}, {"pkg/../x"},
		{"pkg/*.go"}, {"pkg/?"}, {"pkg/[ab]"}, {"~/notes"}, {" pkg"}, {"pkg "}, {"pkg\nx"}, {"pkg\x00"},
		{strings.Repeat("a", MaxTaskPathBytes+1)}, tooMany,
	} {
		_, err := NormalizePaths(bad)
		var inputErr *InputError
		if !errors.As(err, &inputErr) || inputErr.Field != "paths" {
			t.Errorf("NormalizePaths(%q) = %v, want a paths input error", bad, err)
		}
	}
}

func TestPathsOverlapByPrefixNestingCaseAndTheNoPathsRule(t *testing.T) {
	for _, c := range []struct {
		a, b    []string
		overlap bool
	}{
		{[]string{"pkg/a"}, []string{"pkg/a"}, true},
		{[]string{"pkg/a"}, []string{"pkg/a/b.go"}, true},
		{[]string{"pkg/a/b/c.go"}, []string{"pkg"}, true},
		{[]string{"pkg/a"}, []string{"pkg/ab"}, false},
		{[]string{"pkg/a", "docs"}, []string{"cmd", "docs/x.md"}, true},
		{[]string{"pkg/a"}, []string{"pkg/b", "cmd"}, false},
		{[]string{"Pkg/A"}, []string{"pkg/a/x.go"}, true},
		{nil, []string{"pkg/a"}, true},
		{[]string{"pkg/a"}, nil, true},
		{nil, nil, true},
	} {
		if got := PathsOverlap(c.a, c.b); got.Overlap != c.overlap {
			t.Errorf("PathsOverlap(%q, %q) = %+v, want %v", c.a, c.b, got, c.overlap)
		}
		if got := PathsOverlap(c.b, c.a); got.Overlap != c.overlap {
			t.Errorf("PathsOverlap(%q, %q) is not symmetric", c.b, c.a)
		}
	}
	if got := PathsOverlap([]string{"pkg"}, []string{"cmd", "pkg/a/x.go"}); got.Left != "pkg" || got.Right != "pkg/a/x.go" {
		t.Fatalf("overlap names %+v", got)
	}
	if got := PathsOverlap(nil, []string{"pkg"}); got.Left != "" || !got.Overlap {
		t.Fatalf("no-paths overlap names %+v", got)
	}
}

func TestValidateDependenciesRefusesBlankDuplicateAndUnboundedLists(t *testing.T) {
	if err := ValidateDependencyIDs([]ID{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	many := make([]ID, MaxTaskDependencies+1)
	for i := range many {
		many[i] = ID(strings.Repeat("d", i+1))
	}
	for _, bad := range [][]ID{{""}, {" "}, {"a", "a"}, {ID(strings.Repeat("x", 129))}, many} {
		var inputErr *InputError
		if err := ValidateDependencyIDs(bad); !errors.As(err, &inputErr) || inputErr.Field != "depends_on" {
			t.Errorf("ValidateDependencyIDs(%q) = %v", bad, err)
		}
	}
}

func TestDecomposeAndUpdateInputsValidatePlanFields(t *testing.T) {
	valid := DecomposeInput{ExpectedRevision: 1, IdempotencyKey: "k", Children: []ChildInput{
		{Title: "A", Paths: []string{"pkg/a"}}, {Title: "B", DependsOnChildren: []int{0}},
	}}
	if err := ValidateDecomposeInput(valid); err != nil {
		t.Fatal(err)
	}
	for _, children := range [][]ChildInput{
		{{Title: "A", DependsOnChildren: []int{0}}},
		{{Title: "A"}, {Title: "B", DependsOnChildren: []int{2}}},
		{{Title: "A"}, {Title: "B", DependsOnChildren: []int{-1}}},
		{{Title: "A"}, {Title: "B", DependsOnChildren: []int{0, 0}}},
		{{Title: "A", Paths: []string{"/abs"}}},
		{{Title: "A", DependsOn: []ID{""}}},
	} {
		if err := ValidateDecomposeInput(DecomposeInput{ExpectedRevision: 1, IdempotencyKey: "k", Children: children}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("decompose %+v = %v, want invalid", children, err)
		}
	}
	empty := []ID{}
	if err := ValidateUpdateInput(UpdateInput{ExpectedRevision: 1, IdempotencyKey: "k", DependsOn: &empty}); err != nil {
		t.Fatalf("clearing dependencies is a real patch: %v", err)
	}
	paths := []string{"../x"}
	if err := ValidateUpdateInput(UpdateInput{ExpectedRevision: 1, IdempotencyKey: "k", Paths: &paths}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsafe update paths = %v", err)
	}
	if err := ValidateCreateInput(CreateInput{Title: "Root", IdempotencyKey: "k", DependsOn: []ID{"x"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a root depending on another Task = %v", err)
	}
}

func TestPlanFieldsChangeCanonicalRequestsOnlyWhenPresent(t *testing.T) {
	plain := CreateInput{Title: "A", ParentID: "p", ExpectedParentRevision: 1, IdempotencyKey: "k"}
	before, err := CanonicalCreateRequestSHA256(plain, ScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	planned := plain
	planned.Paths = []string{"pkg/a"}
	after, err := CanonicalCreateRequestSHA256(planned, ScopeGlobal)
	if err != nil || after == before {
		t.Fatalf("planned create digest %q equals plain %q (%v)", after, before, err)
	}
	title := "T"
	update := UpdateInput{ExpectedRevision: 1, Title: &title}
	u1, _ := CanonicalUpdateRequestSHA256("t", update)
	deps := []ID{"x"}
	update.DependsOn = &deps
	u2, _ := CanonicalUpdateRequestSHA256("t", update)
	if u1 == u2 {
		t.Fatal("dependencies do not change the update digest")
	}
	decompose := DecomposeInput{ExpectedRevision: 1, Children: []ChildInput{{Title: "A"}}}
	d1, _ := CanonicalDecomposeRequestSHA256("t", decompose)
	decompose.Children[0].Paths = []string{"a"}
	d2, _ := CanonicalDecomposeRequestSHA256("t", decompose)
	if d1 == d2 {
		t.Fatal("child paths do not change the decomposition digest")
	}
}

func TestCanonicalRequestsWithoutPlanFieldsKeepTheirDigests(t *testing.T) {
	// Digests recorded before Stage 13: a retry of an earlier request must
	// still replay, so these never change.
	create, _ := CanonicalCreateRequestSHA256(CreateInput{Title: "A", IdempotencyKey: "k"}, ScopeGlobal)
	focus, _ := CanonicalCreateRequestSHA256(CreateInput{Title: "A", IdempotencyKey: "k", Focus: true}, ScopeGlobal)
	update, _ := CanonicalUpdateRequestSHA256("t", UpdateInput{ExpectedRevision: 1, Title: ptr("T")})
	updateResult, _ := CanonicalUpdateRequestSHA256("t", UpdateInput{ExpectedRevision: 1, Title: ptr("T"), ResultSummary: ptr("r")})
	decompose, _ := CanonicalDecomposeRequestSHA256("t", DecomposeInput{ExpectedRevision: 1, Children: []ChildInput{{Title: "A"}}})
	for name, got := range map[string]string{"create": create, "focus": focus, "update": update, "updateResult": updateResult, "decompose": decompose} {
		if want := preStage13Digests[name]; got != want {
			t.Errorf("%s digest = %s, want %s", name, got, want)
		}
	}
}

var preStage13Digests = map[string]string{
	"create":       "55e810e0fba228c3514ba2c20b7445a44b79349352cc90cbcbe0124f14121a71",
	"focus":        "4f279d83721bee213b9cee8f720d6d3e9b495a213f830b288e333a7c351c435a",
	"update":       "b75267cd1037723ce5b35b239068c1d7223ed41fe3b83a9f3607a9a3730f11a0",
	"updateResult": "c9d30b6a36c930d990ad279a3abe2cf88ec776a4a957c64ce0f10bd54c032730",
	"decompose":    "8d4a82ec61ce0c9b2ccef4b395eb568654e50548896d5a9c1c68039d805d978f",
}

func ptr[T any](v T) *T { return &v }

func TestDependencyCycleFindsDirectInheritedAncestorAndDescendantCycles(t *testing.T) {
	// root
	// ├── g (group) ── c1, c2
	// ├── x
	// └── y
	base := func() []DependencyNode {
		return []DependencyNode{
			{ID: "root"}, {ID: "g", ParentID: "root"}, {ID: "c1", ParentID: "g"}, {ID: "c2", ParentID: "g"},
			{ID: "x", ParentID: "root"}, {ID: "y", ParentID: "root"},
		}
	}
	with := func(deps map[ID][]ID) []DependencyNode {
		nodes := base()
		for i := range nodes {
			nodes[i].DependsOn = deps[nodes[i].ID]
		}
		return nodes
	}
	for name, c := range map[string]struct {
		deps  map[ID][]ID
		cycle bool
	}{
		"chain":                     {map[ID][]ID{"y": {"x"}, "c2": {"c1"}, "g": {"y"}}, false},
		"direct":                    {map[ID][]ID{"x": {"y"}, "y": {"x"}}, true},
		"self":                      {map[ID][]ID{"x": {"x"}}, true},
		"ancestor":                  {map[ID][]ID{"c1": {"g"}}, true},
		"descendant":                {map[ID][]ID{"g": {"c1"}}, true},
		"inherited through a group": {map[ID][]ID{"g": {"x"}, "x": {"c2"}}, true},
		"group waits for its child": {map[ID][]ID{"y": {"g"}, "c1": {"y"}}, true},
		"siblings in order":         {map[ID][]ID{"c2": {"c1"}, "x": {"g"}, "y": {"x", "c2"}}, false},
	} {
		cycle := DependencyCycle(with(c.deps))
		if (cycle != nil) != c.cycle {
			t.Errorf("%s: cycle = %q, want cycle %v", name, cycle, c.cycle)
		}
		if cycle != nil && cycle[0] != cycle[len(cycle)-1] {
			t.Errorf("%s: cycle %q does not close", name, cycle)
		}
	}
}

// fourTaskTree is the Stage 13 acceptance tree: A and B are independent, C
// depends on both, and D's paths overlap A's.
func fourTaskTree() []PlanNode {
	return []PlanNode{
		{Task: Task{ID: "root", Title: "Feature", Status: StatusOpen, Revision: 2}},
		{Task: Task{ID: "a", ParentID: "root", Title: "A", Status: StatusOpen, Revision: 1, Paths: []string{"pkg/a"}}},
		{Task: Task{ID: "b", ParentID: "root", Title: "B", Status: StatusOpen, Revision: 1, Paths: []string{"pkg/b"}}},
		{Task: Task{ID: "c", ParentID: "root", Title: "C", Status: StatusOpen, Revision: 1, Paths: []string{"pkg/c"}, DependsOn: []ID{"a", "b"}}},
		{Task: Task{ID: "d", ParentID: "root", Title: "D", Status: StatusOpen, Revision: 1, Paths: []string{"pkg/a/extra.go"}}},
	}
}

func batchIDs(r Readiness) [][]ID {
	out := [][]ID{}
	for _, batch := range r.Batches {
		var ids []ID
		for _, item := range batch {
			ids = append(ids, item.TaskID)
		}
		out = append(out, ids)
	}
	return out
}

func blockedCodes(r Readiness) map[ID][]string {
	out := map[ID][]string{}
	for _, b := range r.Blocked {
		for _, reason := range b.Reasons {
			out[b.TaskID] = append(out[b.TaskID], reason.Code)
		}
	}
	return out
}

func capacityOf(size int) DispatchCapacity {
	return DispatchLimits{MaxBatch: 8, PerParent: size, Runtime: 4, PerTurn: 16}.Capacity(0, 0, 0, 0)
}

func TestReadinessBatchesIndependentTasksAndHoldsDependentsAndConflicts(t *testing.T) {
	r := NewPlan(fourTaskTree()).Readiness("root", nil, capacityOf(2))
	if got, want := batchIDs(r), [][]ID{{"a", "b"}, {"d"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("batches = %v, want %v", got, want)
	}
	if got := blockedCodes(r); !reflect.DeepEqual(got, map[ID][]string{"c": {ReasonWaitingOnDependencies}}) {
		t.Fatalf("blocked = %v", got)
	}
	if r.Capacity.BatchSize != 2 || r.Counts[StatusOpen] != 5 {
		t.Fatalf("capacity %+v counts %v", r.Capacity, r.Counts)
	}
	// One worker at a time: every batch has one Task.
	one := NewPlan(fourTaskTree()).Readiness("root", nil, capacityOf(1))
	if got, want := batchIDs(one), [][]ID{{"a"}, {"b"}, {"d"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("batches at capacity 1 = %v, want %v", got, want)
	}
	// No capacity: nothing is dispatchable now, and each ready Task says why.
	none := NewPlan(fourTaskTree()).Readiness("root", nil, DispatchLimits{MaxBatch: 8, PerParent: 2, Runtime: 4, PerTurn: 3}.Capacity(0, 0, 3, 0))
	if len(none.Batches) != 0 || none.Capacity.Note == "" {
		t.Fatalf("no-capacity readiness = %+v", none)
	}
	if got := blockedCodes(none); len(got["a"]) != 1 || got["a"][0] != ReasonNoCapacity || got["c"][0] != ReasonWaitingOnDependencies {
		t.Fatalf("no-capacity blocked = %v", got)
	}
	// max narrows the batch size.
	narrowed := NewPlan(fourTaskTree()).Readiness("root", nil, DispatchLimits{MaxBatch: 8, PerParent: 4, Runtime: 4, PerTurn: 16}.Capacity(0, 0, 0, 1))
	if narrowed.Capacity.BatchSize != 1 {
		t.Fatalf("max did not narrow the batch: %+v", narrowed.Capacity)
	}
}

func TestReadinessTreatsTasksWithoutPathsAsTouchingEverything(t *testing.T) {
	nodes := fourTaskTree()
	nodes[2].Task.Paths = nil                // B declares no paths
	nodes[4].Task.Paths = []string{"pkg/d/"} // D no longer overlaps A
	r := NewPlan(nodes).Readiness("root", nil, capacityOf(4))
	if got, want := batchIDs(r), [][]ID{{"a", "d"}, {"b"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("batches = %v, want %v", got, want)
	}
	for _, batch := range r.Batches {
		for i := range batch {
			for j := i + 1; j < len(batch); j++ {
				if PathsOverlap(batch[i].Paths, batch[j].Paths).Overlap {
					t.Fatalf("batch %v holds conflicting Tasks", batch)
				}
			}
		}
	}
	for _, batch := range r.Batches {
		if len(batch) > 1 && slicesContainsID(batch, "b") {
			t.Fatalf("a Task without paths shares batch %v", batch)
		}
	}
}

func slicesContainsID(batch []ReadyTask, id ID) bool {
	for _, item := range batch {
		if item.TaskID == id {
			return true
		}
	}
	return false
}

func TestReadinessFollowsBuildsThroughMergeBeforeDependentsStart(t *testing.T) {
	nodes := fourTaskTree()
	nodes[1].Build = &PlanBuild{ExecutionID: "ea", State: "running", Branch: "evie/ea-a"}
	nodes[1].Claim = &PlanClaim{Live: true}
	nodes[1].Task.Status = StatusInProgress
	occupancy := []PlanOccupancy{{TaskID: "a", ExecutionID: "ea", Phase: BuildRunning, Paths: []string{"pkg/a"}, InTree: true, Title: "A"}}
	r := NewPlan(nodes).Readiness("root", occupancy, capacityOf(2))
	if got, want := batchIDs(r), [][]ID{{"b"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("batches while A builds = %v, want %v", got, want)
	}
	if len(r.InFlight) != 1 || r.InFlight[0].TaskID != "a" || r.InFlight[0].Build.Phase != BuildRunning {
		t.Fatalf("in flight = %+v", r.InFlight)
	}
	codes := blockedCodes(r)
	if !reflect.DeepEqual(codes["d"], []string{ReasonConflictsWithBuild}) || !reflect.DeepEqual(codes["c"], []string{ReasonWaitingOnDependencies}) {
		t.Fatalf("blocked = %v", codes)
	}
	// A's worker completed A, but its branch is not merged: C still waits,
	// D still conflicts, and A says to propose the merge.
	nodes[1].Task.Status = StatusCompleted
	nodes[1].Claim = nil
	nodes[1].Build = &PlanBuild{ExecutionID: "ea", State: "succeeded", Branch: "evie/ea-a", HasCommits: true, NeedsRebase: true}
	occupancy[0].Phase = BuildAwaitingMerge
	r = NewPlan(nodes).Readiness("root", occupancy, capacityOf(2))
	if len(r.InFlight) != 1 || r.InFlight[0].Build.Phase != BuildAwaitingMerge || !strings.Contains(r.InFlight[0].Next, "propose_build_merge") || !r.InFlight[0].Build.NeedsRebase {
		t.Fatalf("awaiting merge = %+v", r.InFlight)
	}
	codes = blockedCodes(r)
	if !reflect.DeepEqual(codes["c"], []string{ReasonWaitingOnDependencies}) || !reflect.DeepEqual(codes["d"], []string{ReasonConflictsWithBuild}) {
		t.Fatalf("blocked while A awaits merge = %v", codes)
	}
	if detail := r.Blocked[0].Reasons[0].Detail + r.Blocked[1].Reasons[0].Detail; !strings.Contains(detail, "awaiting merge") {
		t.Fatalf("details do not say A awaits its merge: %q", detail)
	}
	// After the merge (and B's), C and D are ready together.
	nodes[1].Build = &PlanBuild{ExecutionID: "ea", State: "succeeded", Merged: true, HasCommits: true}
	nodes[2].Task.Status = StatusCompleted
	r = NewPlan(nodes).Readiness("root", nil, capacityOf(2))
	if got, want := batchIDs(r), [][]ID{{"c", "d"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("batches after merges = %v, want %v", got, want)
	}
}

func TestReadinessReportsClaimsStaleClaimsAndResumableBuilds(t *testing.T) {
	nodes := fourTaskTree()
	nodes[1].Claim = &PlanClaim{Live: true, Mine: true}
	nodes[2].Claim = &PlanClaim{Live: false}
	nodes[2].Task.Status = StatusInProgress
	nodes[2].Build = &PlanBuild{ExecutionID: "eb", State: "interrupted", Branch: "evie/eb-b", Uncommitted: true}
	r := NewPlan(nodes).Readiness("root", nil, capacityOf(2))
	codes := blockedCodes(r)
	if !reflect.DeepEqual(codes["a"], []string{ReasonClaimed}) || !strings.Contains(r.Blocked[0].Reasons[0].Detail, "this conversation") {
		t.Fatalf("A claimed by this conversation: %+v", r.Blocked)
	}
	var b *ReadyTask
	for _, batch := range r.Batches {
		for i := range batch {
			if batch[i].TaskID == "b" {
				b = &batch[i]
			}
		}
	}
	if b == nil || !b.StaleClaim || b.LastBuild == nil || b.LastBuild.Phase != BuildResumable || !strings.Contains(b.Note, `base "evie/eb-b"`) {
		t.Fatalf("B after a crash = %+v", b)
	}
}

func TestReadinessHoldsSubtasksOfRunningWorkAndOffersFinishedGroups(t *testing.T) {
	nodes := []PlanNode{
		{Task: Task{ID: "root", Title: "Feature", Status: StatusOpen}},
		{Task: Task{ID: "w", ParentID: "root", Title: "Worker's Task", Status: StatusInProgress, Paths: []string{"pkg/w"}},
			Build: &PlanBuild{ExecutionID: "ew", State: "running"}, Claim: &PlanClaim{Live: true}},
		{Task: Task{ID: "w1", ParentID: "w", Title: "Worker's step", Status: StatusOpen, Paths: []string{"pkg/w/x.go"}}},
		{Task: Task{ID: "g", ParentID: "root", Title: "Group", Status: StatusOpen}},
		{Task: Task{ID: "g1", ParentID: "g", Title: "Done", Status: StatusCompleted}},
		{Task: Task{ID: "g2", ParentID: "g", Title: "Dropped", Status: StatusCancelled}},
		{Task: Task{ID: "m", ParentID: "root", Title: "Merged", Status: StatusOpen},
			Build: &PlanBuild{ExecutionID: "em", State: "succeeded", Merged: true, HasCommits: true}},
		{Task: Task{ID: "s", ParentID: "root", Title: "Stuck", Status: StatusBlocked}},
	}
	r := NewPlan(nodes).Readiness("root", nil, capacityOf(2))
	if len(r.Batches) != 0 {
		t.Fatalf("batches = %v", batchIDs(r))
	}
	codes := blockedCodes(r)
	if !reflect.DeepEqual(codes["w1"], []string{ReasonInsideRunningWork}) || !reflect.DeepEqual(codes["s"], []string{ReasonMarkedBlocked}) {
		t.Fatalf("blocked = %v", codes)
	}
	var completable []ID
	for _, c := range r.Completable {
		completable = append(completable, c.TaskID)
	}
	if !reflect.DeepEqual(completable, []ID{"g", "m"}) {
		t.Fatalf("completable = %v", completable)
	}
	// Restricting to a subtree considers only its Tasks.
	sub := NewPlan(nodes).Readiness("g", nil, capacityOf(2))
	if sub.TreeID != "g" || len(sub.Completable) != 1 || sub.Counts[StatusCompleted] != 1 || sub.Counts[StatusOpen] != 1 {
		t.Fatalf("subtree readiness = %+v", sub)
	}
}

func TestDependencyOfAnAncestorHoldsItsSubtasks(t *testing.T) {
	nodes := []PlanNode{
		{Task: Task{ID: "root", Status: StatusOpen}},
		{Task: Task{ID: "x", ParentID: "root", Title: "X", Status: StatusOpen, Paths: []string{"x"}}},
		{Task: Task{ID: "g", ParentID: "root", Title: "G", Status: StatusOpen, DependsOn: []ID{"x"}}},
		{Task: Task{ID: "g1", ParentID: "g", Title: "G1", Status: StatusOpen, Paths: []string{"g"}}},
	}
	p := NewPlan(nodes)
	if got := p.EffectiveDependencies("g1"); !reflect.DeepEqual(got, []ID{"x"}) {
		t.Fatalf("effective dependencies = %v", got)
	}
	r := p.Readiness("root", nil, capacityOf(2))
	if got := batchIDs(r); !reflect.DeepEqual(got, [][]ID{{"x"}}) {
		t.Fatalf("batches = %v", got)
	}
	if unfinished := p.UnfinishedDependencies("g1"); len(unfinished) != 1 || unfinished[0].TaskID != "x" {
		t.Fatalf("unfinished = %+v", unfinished)
	}
}
