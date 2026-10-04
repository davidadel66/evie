package task

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Task Tree orchestration (Stage 13 of docs/memory-architecture.md,
// 2026-10-04). A Task may name the Tasks of its tree it depends on and the
// files or folders it expects to touch. The orchestrator, the conversational
// model inside one turn, reads the dispatchable set with todo_ready; the
// harness enforces the same dependency and conflict rules when it admits a
// build worker. Everything here is pure and deterministic: persistence fills
// a Plan, which computes the read model.

const (
	// MaxTaskDependencies bounds one Task's depends_on list.
	MaxTaskDependencies = 32
	// MaxTaskPaths and MaxTaskPathBytes bound one Task's paths.
	MaxTaskPaths     = 32
	MaxTaskPathBytes = 256
	maxTaskIDBytes   = 128
)

// ValidateDependencyIDs checks the shape of a depends_on list. Whether each
// Task exists in the same tree, and whether the list would create a cycle,
// is decided by persistence against the tree.
func ValidateDependencyIDs(ids []ID) error {
	if len(ids) > MaxTaskDependencies {
		return &InputError{Field: "depends_on", Message: fmt.Sprintf("must not exceed %d Tasks", MaxTaskDependencies)}
	}
	seen := make(map[ID]bool, len(ids))
	for _, id := range ids {
		switch {
		case strings.TrimSpace(string(id)) == "":
			return &InputError{Field: "depends_on", Message: "must not contain a blank Task identity"}
		case len(id) > maxTaskIDBytes:
			return &InputError{Field: "depends_on", Message: fmt.Sprintf("Task identities must not exceed %d bytes", maxTaskIDBytes)}
		case seen[id]:
			return &InputError{Field: "depends_on", Message: fmt.Sprintf("names Task %q twice", id)}
		}
		seen[id] = true
	}
	return nil
}

// NormalizePaths validates a Task's expected paths and returns them cleaned
// ("./pkg/a/" becomes "pkg/a") without duplicates, in the given order, or nil
// for none. Paths are plain files or folders relative to the repository's top
// level; they are compared only to group conflicting work, never to grant or
// fence anything. Omitting paths means the Task may touch any file.
func NormalizePaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) > MaxTaskPaths {
		return nil, &InputError{Field: "paths", Message: fmt.Sprintf("must not exceed %d paths", MaxTaskPaths)}
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, raw := range paths {
		cleaned, err := normalizePath(raw)
		if err != nil {
			return nil, err
		}
		if !seen[cleaned] {
			seen[cleaned] = true
			out = append(out, cleaned)
		}
	}
	return out, nil
}

func normalizePath(raw string) (string, error) {
	refuse := func(message string) (string, error) {
		return "", &InputError{Field: "paths", Message: fmt.Sprintf("%q %s", raw, message)}
	}
	switch {
	case len(raw) > MaxTaskPathBytes:
		return "", &InputError{Field: "paths", Message: fmt.Sprintf("each path must not exceed %d bytes", MaxTaskPathBytes)}
	case raw != strings.TrimSpace(raw):
		return refuse("must not start or end with spaces")
	case strings.ContainsFunc(raw, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return refuse("must not contain control characters")
	case strings.ContainsAny(raw, "*?["):
		return refuse("is a pattern; name plain files or folders")
	case strings.HasPrefix(raw, "/"):
		return refuse("must be relative to the repository's top level")
	case strings.HasPrefix(raw, "~"):
		return refuse("must be relative to the repository's top level, not a home folder")
	}
	for _, component := range strings.Split(raw, "/") {
		if component == ".." {
			return refuse("must not contain ..")
		}
	}
	cleaned := path.Clean(raw)
	if cleaned == "." || raw == "" {
		return refuse("names the whole repository; omit paths for work that may touch any file")
	}
	return cleaned, nil
}

// Overlap names the first pair of overlapping paths of two Tasks. Left or
// Right is empty when that Task declared no paths, which overlaps
// everything.
type Overlap struct {
	Overlap     bool
	Left, Right string
}

// PathsOverlap applies the conflict rule: two Tasks conflict when one's path
// equals, contains or lies inside the other's, compared component-wise and
// case-insensitively (as on the owner's macOS file system), or when either
// declared no paths (it may touch any file).
func PathsOverlap(left, right []string) Overlap {
	if len(left) == 0 || len(right) == 0 {
		o := Overlap{Overlap: true}
		if len(left) > 0 {
			o.Left = left[0]
		}
		if len(right) > 0 {
			o.Right = right[0]
		}
		return o
	}
	for _, l := range left {
		a := strings.ToLower(strings.TrimSuffix(l, "/"))
		for _, r := range right {
			b := strings.ToLower(strings.TrimSuffix(r, "/"))
			if a == b || strings.HasPrefix(b, a+"/") || strings.HasPrefix(a, b+"/") {
				return Overlap{Overlap: true, Left: l, Right: r}
			}
		}
	}
	return Overlap{}
}

// Describe is the overlap in harness words, from the left Task's side.
func (o Overlap) Describe() string {
	switch {
	case o.Left == "" && o.Right == "":
		return "neither declares paths, so either may touch any file"
	case o.Left == "":
		return "it declares no paths, so it may touch any file"
	case o.Right == "":
		return "that Task declares no paths, so it may touch any file"
	case o.Left == o.Right:
		return fmt.Sprintf("both touch %s", o.Left)
	}
	return fmt.Sprintf("%s overlaps %s", o.Left, o.Right)
}

// DependencyNode is one Task's place in its tree and its own dependencies.
type DependencyNode struct {
	ID        ID
	ParentID  ID
	DependsOn []ID
}

// DependencyCycle returns a cycle of "waits for" edges in a Task Tree, first
// and last element equal, or nil when there is none. A Task waits for its
// dependencies, its subtasks wait for the dependencies of every ancestor
// (they cannot start before their parent could), and a parent waits for its
// subtasks (it cannot complete while they are active). So naming an ancestor
// or a descendant is a cycle, as is any loop of dependencies.
func DependencyCycle(nodes []DependencyNode) []ID {
	byID := make(map[ID]DependencyNode, len(nodes))
	children := make(map[ID][]ID, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
		if n.ParentID != "" {
			children[n.ParentID] = append(children[n.ParentID], n.ID)
		}
	}
	edges := make(map[ID][]ID, len(nodes))
	for _, n := range nodes {
		seen := map[ID]bool{}
		add := func(to ID) {
			if !seen[to] {
				seen[to] = true
				edges[n.ID] = append(edges[n.ID], to)
			}
		}
		for at, hops := n, 0; hops <= len(nodes); hops++ {
			for _, dependency := range at.DependsOn {
				add(dependency)
			}
			parent, ok := byID[at.ParentID]
			if !ok {
				break
			}
			at = parent
		}
		for _, child := range children[n.ID] {
			add(child)
		}
	}
	const (
		unvisited = iota
		visiting
		done
	)
	state := make(map[ID]int, len(nodes))
	var stack []ID
	var cycle []ID
	var visit func(ID) bool
	visit = func(id ID) bool {
		state[id] = visiting
		stack = append(stack, id)
		for _, next := range edges[id] {
			switch state[next] {
			case visiting:
				for i, at := range stack {
					if at == next {
						cycle = append(append([]ID(nil), stack[i:]...), next)
						return true
					}
				}
			case unvisited:
				if visit(next) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = done
		return false
	}
	for _, n := range nodes {
		if state[n.ID] == unvisited && visit(n.ID) {
			return cycle
		}
	}
	return nil
}

// DescribeCycle renders a cycle as "a → b → a".
func DescribeCycle(cycle []ID) string {
	parts := make([]string, len(cycle))
	for i, id := range cycle {
		parts[i] = string(id)
	}
	return strings.Join(parts, " → ")
}

// ReadyQuery asks for the read model of one Task Tree or subtree. Max
// optionally narrows the batch size. Limits and TurnSourceEventID are
// trusted: the runtime supplies them, never model arguments.
type ReadyQuery struct {
	TreeID            ID
	Max               int
	Limits            DispatchLimits
	TurnSourceEventID string
}

// ValidateReadyQuery checks a todo_ready request.
func ValidateReadyQuery(query ReadyQuery) error {
	switch {
	case strings.TrimSpace(string(query.TreeID)) == "":
		return &InputError{Field: "tree_id", Message: "must not be blank"}
	case query.Max < 0:
		return &InputError{Field: "max", Message: "must be zero (no narrowing) or positive"}
	case query.Limits.MaxBatch <= 0 || query.Limits.PerParent <= 0 || query.Limits.Runtime <= 0 || query.Limits.PerTurn <= 0:
		return &InputError{Field: "limits", Message: "dispatch limits must be positive"}
	}
	return nil
}

// DispatchLimits are the subagent concurrency caps a batch is sized to: at
// most MaxBatch assignments per delegation call, PerParent running at once
// for one conversation, Runtime running at once in this process, and
// PerTurn started by one parent turn.
type DispatchLimits struct {
	MaxBatch, PerParent, Runtime, PerTurn int
}

// DispatchCapacity is how many workers one conversation can start now.
type DispatchCapacity struct {
	BatchSize       int    `json:"batch_size"`
	MaxBatch        int    `json:"max_batch"`
	PerConversation int    `json:"per_conversation"`
	RunningHere     int    `json:"running_in_this_conversation"`
	Runtime         int    `json:"runtime"`
	RunningOverall  int    `json:"running_overall"`
	PerTurn         int    `json:"per_turn"`
	StartedThisTurn int    `json:"started_this_turn"`
	Note            string `json:"note,omitempty"`
}

// Capacity sizes the next batch: the smallest of the per-call limit, the
// conversation's free slots, the runtime limit and what this turn may still
// start, narrowed to max when it is positive. Workers of other
// conversations only delay a batch (it waits for free runtime slots), so
// they are reported, not subtracted.
func (l DispatchLimits) Capacity(runningHere, runningOverall, startedThisTurn, max int) DispatchCapacity {
	c := DispatchCapacity{MaxBatch: l.MaxBatch, PerConversation: l.PerParent, RunningHere: runningHere, Runtime: l.Runtime,
		RunningOverall: runningOverall, PerTurn: l.PerTurn, StartedThisTurn: startedThisTurn}
	size := l.MaxBatch
	for _, limit := range []int{l.PerParent - runningHere, l.Runtime, l.PerTurn - startedThisTurn} {
		size = min(size, limit)
	}
	if max > 0 {
		size = min(size, max)
	}
	c.BatchSize = maxInt(size, 0)
	switch {
	case l.PerTurn-startedThisTurn <= 0:
		c.Note = fmt.Sprintf("This turn already started %d workers (limit %d per turn); dispatch more in a new turn.", startedThisTurn, l.PerTurn)
	case l.PerParent-runningHere <= 0:
		c.Note = fmt.Sprintf("This conversation already runs %d workers (limit %d); wait for their results.", runningHere, l.PerParent)
	case runningOverall >= l.Runtime:
		c.Note = fmt.Sprintf("Other conversations' workers fill the runtime limit (%d running, limit %d); a batch waits for free slots.", runningOverall, l.Runtime)
	}
	return c
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// PlanClaim is a Task's claim as the read model needs it: whether the
// claiming execution is still running, and whether it is the querying
// conversation's own.
type PlanClaim struct {
	Live bool
	Mine bool
}

// PlanBuild is the latest build worker attempt on a Task, as the harness
// recorded it.
type PlanBuild struct {
	ExecutionID string
	State       string
	Branch      string
	Worktree    string
	GateStep    string
	NeedsRebase bool
	Merged      bool
	// Removed means the worktree was cleaned up without a merge.
	Removed bool
	// HasCommits means the branch has commits after its base to merge.
	HasCommits  bool
	Uncommitted bool
}

// BuildPhase is where a Task's latest build stands for orchestration.
type BuildPhase string

const (
	// BuildRunning: the worker is admitted or running.
	BuildRunning BuildPhase = "running"
	// BuildAwaitingMerge: the worker finished with a report and commits that
	// are not merged yet; propose_build_merge is next.
	BuildAwaitingMerge BuildPhase = "awaiting_merge"
	// BuildMerged: the owner approved and the harness merged the branch.
	BuildMerged BuildPhase = "merged"
	// BuildResumable: the worker failed, was cancelled or was interrupted,
	// and its worktree and branch were kept.
	BuildResumable BuildPhase = "resumable"
	// BuildSettled: nothing is pending (no commits to merge, or the
	// worktree was cleaned up).
	BuildSettled BuildPhase = "settled"
)

// Phase classifies the attempt.
func (b PlanBuild) Phase() BuildPhase {
	switch {
	case b.State == "admitted" || b.State == "running":
		return BuildRunning
	case b.Merged:
		return BuildMerged
	case b.Removed:
		return BuildSettled
	case (b.State == "succeeded" || b.State == "partial") && b.HasCommits:
		return BuildAwaitingMerge
	case b.State == "failed" || b.State == "cancelled" || b.State == "interrupted":
		return BuildResumable
	}
	return BuildSettled
}

// pending reports whether the build still holds its Task's work: running,
// or finished with commits that are not merged.
func (b *PlanBuild) pending() bool {
	if b == nil {
		return false
	}
	phase := b.Phase()
	return phase == BuildRunning || phase == BuildAwaitingMerge
}

// PlanNode is one Task of a Task Tree with its claim and latest build.
type PlanNode struct {
	Task  Task
	Claim *PlanClaim
	Build *PlanBuild
}

// PlanOccupancy is a build in the conversation's repository that holds
// paths: running, or finished and awaiting its merge. InTree marks a build
// on a Task of the queried tree, whose title may be shown.
type PlanOccupancy struct {
	TaskID      ID
	ExecutionID string
	Phase       BuildPhase
	Paths       []string
	InTree      bool
	Title       string
}

// Plan is a Task Tree in tree order (depth first, siblings in order).
type Plan struct {
	nodes    []PlanNode
	index    map[ID]int
	children map[ID][]ID
}

// NewPlan indexes nodes, which must be in tree order.
func NewPlan(nodes []PlanNode) *Plan {
	p := &Plan{nodes: nodes, index: make(map[ID]int, len(nodes)), children: make(map[ID][]ID, len(nodes))}
	for i, n := range nodes {
		p.index[n.Task.ID] = i
		if n.Task.ParentID != "" {
			p.children[n.Task.ParentID] = append(p.children[n.Task.ParentID], n.Task.ID)
		}
	}
	return p
}

// Node returns the Task id of the tree.
func (p *Plan) Node(id ID) (PlanNode, bool) {
	i, ok := p.index[id]
	if !ok {
		return PlanNode{}, false
	}
	return p.nodes[i], true
}

// Has reports whether id is a Task of the tree.
func (p *Plan) Has(id ID) bool {
	_, ok := p.index[id]
	return ok
}

// Ancestors lists id's ancestors, nearest first.
func (p *Plan) Ancestors(id ID) []ID {
	var out []ID
	node, ok := p.Node(id)
	for ok && node.Task.ParentID != "" && len(out) <= len(p.nodes) {
		out = append(out, node.Task.ParentID)
		node, ok = p.Node(node.Task.ParentID)
	}
	return out
}

// EffectiveDependencies are id's own dependencies, then those of each
// ancestor, without duplicates: a subtask cannot start before its parent
// could.
func (p *Plan) EffectiveDependencies(id ID) []ID {
	var out []ID
	seen := map[ID]bool{}
	for _, at := range append([]ID{id}, p.Ancestors(id)...) {
		node, _ := p.Node(at)
		for _, dependency := range node.Task.DependsOn {
			if !seen[dependency] {
				seen[dependency] = true
				out = append(out, dependency)
			}
		}
	}
	return out
}

// DependencyState is one unfinished dependency and why.
type DependencyState struct {
	TaskID ID
	Title  string
	Status Status
	Build  *PlanBuild
	// Missing marks a dependency that is not in the tree.
	Missing bool
}

// Describe renders the dependency in harness words.
func (d DependencyState) Describe() string {
	if d.Missing {
		return fmt.Sprintf("Task %s (not in this Task Tree)", d.TaskID)
	}
	state := string(d.Status)
	if d.Build != nil {
		switch d.Build.Phase() {
		case BuildRunning:
			state += fmt.Sprintf("; its build %s is running", d.Build.ExecutionID)
		case BuildAwaitingMerge:
			state += fmt.Sprintf("; branch %s awaiting merge", d.Build.Branch)
			if d.Build.NeedsRebase {
				state += ", needs rebase"
			}
		}
	}
	return fmt.Sprintf("Task %s %q (%s)", d.TaskID, d.Title, state)
}

// DependencyDone reports whether a dependency is finished for its
// dependents: completed, and its build (if any) merged or with nothing to
// merge, so dependent work starts from the merged result.
func DependencyDone(n PlanNode) bool {
	return n.Task.Status == StatusCompleted && !n.Build.pending()
}

// UnfinishedDependencies lists id's effective dependencies that are not
// done, in order.
func (p *Plan) UnfinishedDependencies(id ID) []DependencyState {
	var out []DependencyState
	for _, dependency := range p.EffectiveDependencies(id) {
		node, ok := p.Node(dependency)
		if !ok {
			out = append(out, DependencyState{TaskID: dependency, Missing: true})
			continue
		}
		if !DependencyDone(node) {
			out = append(out, DependencyState{TaskID: dependency, Title: node.Task.Title, Status: node.Task.Status, Build: node.Build})
		}
	}
	return out
}

// CoveringWork is the nearest ancestor of id whose build is running. That
// worker's live grant covers id, which is part of its work (it may have
// added id itself). A finished build covers nothing (a subtask may be
// stacked on its unmerged branch), and neither does a claim on an
// ancestor: the orchestrator claims a parent to track its progress.
func (p *Plan) CoveringWork(id ID) (PlanNode, bool) {
	for _, ancestor := range p.Ancestors(id) {
		node, _ := p.Node(ancestor)
		if node.Build != nil && node.Build.Phase() == BuildRunning {
			return node, true
		}
	}
	return PlanNode{}, false
}

// unmergedAncestor is the nearest ancestor of id whose build awaits its
// merge: work on id can be stacked on that branch.
func (p *Plan) unmergedAncestor(id ID) (PlanNode, bool) {
	for _, ancestor := range p.Ancestors(id) {
		node, _ := p.Node(ancestor)
		if node.Build != nil && node.Build.Phase() == BuildAwaitingMerge {
			return node, true
		}
	}
	return PlanNode{}, false
}

// Readiness reason codes.
const (
	ReasonWaitingOnDependencies = "waiting_on_dependencies"
	ReasonClaimed               = "claimed"
	ReasonInsideRunningWork     = "inside_running_work"
	ReasonConflictsWithBuild    = "conflicts_with_build"
	ReasonMarkedBlocked         = "marked_blocked"
	ReasonNoCapacity            = "no_capacity"
)

// Readiness is todo_ready's read model of one Task Tree (or subtree).
type Readiness struct {
	TreeID   ID               `json:"tree_id"`
	Capacity DispatchCapacity `json:"capacity"`
	// Batches are the dispatchable Tasks, partitioned so that no two Tasks
	// of a batch conflict, each at most Capacity.BatchSize. Dispatch the
	// first batch, then read again.
	Batches     [][]ReadyTask     `json:"batches"`
	InFlight    []InFlightTask    `json:"in_flight,omitempty"`
	Blocked     []BlockedTask     `json:"blocked,omitempty"`
	Completable []CompletableTask `json:"completable,omitempty"`
	Counts      map[Status]int    `json:"counts"`
}

// ReadyTask is one dispatchable Task.
type ReadyTask struct {
	TaskID   ID       `json:"task_id"`
	Title    string   `json:"title"`
	Status   Status   `json:"status"`
	Revision uint64   `json:"revision"`
	Paths    []string `json:"paths,omitempty"`
	// StaleClaim marks a claim left by an execution that is no longer
	// running; dispatching the Task releases it.
	StaleClaim bool        `json:"stale_claim,omitempty"`
	LastBuild  *BuildState `json:"last_build,omitempty"`
	Note       string      `json:"note,omitempty"`
}

// BuildState is a Task's latest build as the orchestrator sees it.
type BuildState struct {
	ExecutionID string     `json:"execution_id"`
	State       string     `json:"state"`
	Phase       BuildPhase `json:"phase"`
	Branch      string     `json:"branch,omitempty"`
	GateStep    string     `json:"gate_step,omitempty"`
	NeedsRebase bool       `json:"needs_rebase,omitempty"`
}

// InFlightTask is a Task whose build is running or awaiting its merge.
type InFlightTask struct {
	TaskID ID         `json:"task_id"`
	Title  string     `json:"title"`
	Status Status     `json:"status"`
	Build  BuildState `json:"build"`
	Next   string     `json:"next"`
}

// BlockedTask is an active Task that cannot be dispatched now, with every
// reason.
type BlockedTask struct {
	TaskID  ID            `json:"task_id"`
	Title   string        `json:"title"`
	Status  Status        `json:"status"`
	Reasons []BlockReason `json:"reasons"`
}

// BlockReason is one reason a Task waits, in harness words.
type BlockReason struct {
	Code         string   `json:"code"`
	Detail       string   `json:"detail"`
	TaskIDs      []ID     `json:"task_ids,omitempty"`
	ExecutionIDs []string `json:"execution_ids,omitempty"`
}

// CompletableTask is an active Task whose work looks finished: all its
// subtasks are completed or cancelled, or its build was merged.
type CompletableTask struct {
	TaskID   ID     `json:"task_id"`
	Title    string `json:"title"`
	Status   Status `json:"status"`
	Revision uint64 `json:"revision"`
	Reason   string `json:"reason"`
}

func buildState(b *PlanBuild) *BuildState {
	if b == nil {
		return nil
	}
	return &BuildState{ExecutionID: b.ExecutionID, State: b.State, Phase: b.Phase(), Branch: b.Branch, GateStep: b.GateStep, NeedsRebase: b.NeedsRebase}
}

func active(status Status) bool {
	return status == StatusOpen || status == StatusInProgress || status == StatusBlocked
}

// Readiness computes the read model for subtree (the tree's root or any of
// its Tasks). Occupancy lists the builds in the conversation's repository
// that hold paths.
func (p *Plan) Readiness(subtree ID, occupancy []PlanOccupancy, capacity DispatchCapacity) Readiness {
	r := Readiness{TreeID: subtree, Capacity: capacity, Batches: [][]ReadyTask{}, Counts: map[Status]int{}}
	inSubtree := map[ID]bool{subtree: true}
	var candidates []ReadyTask
	for _, n := range p.nodes {
		t := n.Task
		if !inSubtree[t.ID] && !inSubtree[t.ParentID] {
			continue
		}
		inSubtree[t.ID] = true
		r.Counts[t.Status]++
		phase := BuildSettled
		if n.Build != nil {
			phase = n.Build.Phase()
		}
		if phase == BuildRunning || phase == BuildAwaitingMerge {
			flight := InFlightTask{TaskID: t.ID, Title: t.Title, Status: t.Status, Build: *buildState(n.Build)}
			if phase == BuildRunning {
				flight.Next = "Wait for its delegate_build result."
			} else {
				flight.Next = fmt.Sprintf("propose_build_merge with execution_id %s", n.Build.ExecutionID)
				if n.Build.NeedsRebase {
					flight.Next += " (its target moved; the gate rebases first)"
				}
				flight.Next += "; if a check fails, continue_research that worker with the output."
				if t.Status != StatusCompleted {
					flight.Next += " Complete the Task once merged."
				}
			}
			r.InFlight = append(r.InFlight, flight)
			continue
		}
		if !active(t.Status) {
			continue
		}
		if cover, covered := p.CoveringWork(t.ID); covered {
			detail := fmt.Sprintf("part of Task %s %q, whose build %s is %s; that work covers its subtasks",
				cover.Task.ID, cover.Task.Title, cover.Build.ExecutionID, strings.ReplaceAll(string(cover.Build.Phase()), "_", " "))
			r.Blocked = append(r.Blocked, BlockedTask{TaskID: t.ID, Title: t.Title, Status: t.Status, Reasons: []BlockReason{{
				Code: ReasonInsideRunningWork, Detail: detail, TaskIDs: []ID{cover.Task.ID}, ExecutionIDs: []string{cover.Build.ExecutionID}}}})
			continue
		}
		if kids := p.children[t.ID]; len(kids) > 0 {
			finished := true
			for _, kid := range kids {
				node, _ := p.Node(kid)
				finished = finished && !active(node.Task.Status) && !node.Build.pending()
			}
			if finished {
				r.Completable = append(r.Completable, CompletableTask{TaskID: t.ID, Title: t.Title, Status: t.Status, Revision: t.Revision,
					Reason: "All its subtasks are completed or cancelled; review and complete it."})
			}
			continue
		}
		if phase == BuildMerged {
			r.Completable = append(r.Completable, CompletableTask{TaskID: t.ID, Title: t.Title, Status: t.Status, Revision: t.Revision,
				Reason: fmt.Sprintf("Its build %s was merged; complete it if the work is done.", n.Build.ExecutionID)})
			continue
		}
		var reasons []BlockReason
		if t.Status == StatusBlocked {
			reasons = append(reasons, BlockReason{Code: ReasonMarkedBlocked, Detail: "marked blocked; resolve what blocks it and set it open to dispatch it"})
		}
		if n.Claim != nil && n.Claim.Live {
			detail := "claimed by another active execution"
			if n.Claim.Mine {
				detail = "claimed by this conversation; release it (todo_release) before delegating it, or finish it yourself"
			}
			reasons = append(reasons, BlockReason{Code: ReasonClaimed, Detail: detail})
		}
		if unfinished := p.UnfinishedDependencies(t.ID); len(unfinished) > 0 {
			reason := BlockReason{Code: ReasonWaitingOnDependencies}
			descriptions := make([]string, len(unfinished))
			for i, d := range unfinished {
				descriptions[i] = d.Describe()
				reason.TaskIDs = append(reason.TaskIDs, d.TaskID)
			}
			reason.Detail = "depends on " + strings.Join(descriptions, ", ") + "; a dependency is done when it is completed and its build merged"
			reasons = append(reasons, reason)
		}
		ancestors := map[ID]bool{}
		for _, a := range p.Ancestors(t.ID) {
			ancestors[a] = true
		}
		var conflicts []string
		conflict := BlockReason{Code: ReasonConflictsWithBuild}
		for _, o := range occupancy {
			if o.TaskID == t.ID || ancestors[o.TaskID] {
				continue
			}
			overlap := PathsOverlap(t.Paths, o.Paths)
			if !overlap.Overlap {
				continue
			}
			who := fmt.Sprintf("build %s on another Task", o.ExecutionID)
			if o.InTree {
				who = fmt.Sprintf("Task %s %q, whose build %s", o.TaskID, o.Title, o.ExecutionID)
				conflict.TaskIDs = append(conflict.TaskIDs, o.TaskID)
			}
			conflicts = append(conflicts, fmt.Sprintf("%s is %s (%s)", who, strings.ReplaceAll(string(o.Phase), "_", " "), overlap.Describe()))
			conflict.ExecutionIDs = append(conflict.ExecutionIDs, o.ExecutionID)
		}
		if len(conflicts) > 0 {
			conflict.Detail = "its paths conflict with " + strings.Join(conflicts, "; ") + "; dispatch it after that build is merged or abandoned"
			reasons = append(reasons, conflict)
		}
		if len(reasons) > 0 {
			r.Blocked = append(r.Blocked, BlockedTask{TaskID: t.ID, Title: t.Title, Status: t.Status, Reasons: reasons})
			continue
		}
		ready := ReadyTask{TaskID: t.ID, Title: t.Title, Status: t.Status, Revision: t.Revision, Paths: t.Paths,
			StaleClaim: n.Claim != nil && !n.Claim.Live, LastBuild: buildState(n.Build)}
		if n.Build != nil {
			switch phase {
			case BuildResumable:
				ready.Note = fmt.Sprintf("Its last build %s ended %s; worktree %s and branch %s were kept", n.Build.ExecutionID, n.Build.State, n.Build.Worktree, n.Build.Branch)
				if n.Build.Uncommitted {
					ready.Note += " with uncommitted changes"
				}
				ready.Note += fmt.Sprintf(". To build on that work, delegate_build with base %q; or start fresh.", n.Build.Branch)
			case BuildSettled:
				ready.Note = fmt.Sprintf("Its last build %s ended %s with nothing left to merge.", n.Build.ExecutionID, n.Build.State)
			}
		}
		if parent, stacked := p.unmergedAncestor(t.ID); stacked && ready.Note == "" {
			ready.Note = fmt.Sprintf("Task %s's build %s awaits its merge; to build on that work before it merges, delegate_build with base %q.",
				parent.Task.ID, parent.Build.ExecutionID, parent.Build.Branch)
		}
		candidates = append(candidates, ready)
	}
	if capacity.BatchSize <= 0 {
		for _, c := range candidates {
			r.Blocked = append(r.Blocked, BlockedTask{TaskID: c.TaskID, Title: c.Title, Status: c.Status,
				Reasons: []BlockReason{{Code: ReasonNoCapacity, Detail: "ready, but no worker can start now: " + capacity.Note}}})
		}
		sort.SliceStable(r.Blocked, func(i, j int) bool { return p.index[r.Blocked[i].TaskID] < p.index[r.Blocked[j].TaskID] })
		return r
	}
	// First fit, in tree order: a Task joins the first batch that has room
	// and holds nothing it conflicts with.
	for _, c := range candidates {
		placed := false
		for i := range r.Batches {
			if len(r.Batches[i]) >= capacity.BatchSize {
				continue
			}
			clash := false
			for _, member := range r.Batches[i] {
				if PathsOverlap(c.Paths, member.Paths).Overlap {
					clash = true
					break
				}
			}
			if !clash {
				r.Batches[i] = append(r.Batches[i], c)
				placed = true
				break
			}
		}
		if !placed {
			r.Batches = append(r.Batches, []ReadyTask{c})
		}
	}
	return r
}
