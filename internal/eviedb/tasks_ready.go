package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
)

// Task Tree orchestration (Stage 13 of docs/memory-architecture.md,
// 2026-10-04). The orchestrator is the conversational model inside one
// turn; todo_ready gives it the dispatchable set of a Task Tree, and build
// admission enforces the same dependency and conflict rules, so a model that
// ignores the read model still cannot start conflicting or premature work.
// Nothing here runs in the background: work that outlives a turn belongs to
// the Workflow Runtime (Stage 14).

// ReadyGlobalTasks returns query.TreeID's read model for the calling
// conversation: dispatchable Tasks in conflict-free batches sized to the
// subagent caps, Tasks whose builds run or await their merge, blocked Tasks
// with every reason, and Tasks that look finished. Claims of executions that
// are no longer running count as released; interrupted builds whose
// worktrees were kept are offered for resumption.
func (s *Store) ReadyGlobalTasks(ctx context.Context, query task.ReadyQuery) (task.Readiness, error) {
	_, readiness, err := s.TaskTreeProgress(ctx, query)
	return readiness, err
}

// TaskTreeProgress returns every Task of query.TreeID's tree in tree order,
// each with its claim and latest build (execution, state, branch, gate
// step), and the subtree's read model, from one snapshot. The owner's
// `todo show` renders it.
func (s *Store) TaskTreeProgress(ctx context.Context, query task.ReadyQuery) ([]task.PlanNode, task.Readiness, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityReady, task.AccessRead)
	if err := task.ValidateReadyQuery(query); err != nil {
		return nil, task.Readiness{}, err
	}
	access, err := taskAccessFromContext(ctx, s.db)
	if err != nil {
		return nil, task.Readiness{}, err
	}
	if access.delegated {
		// Only the orchestrating conversation reads a tree's dispatchable
		// set; a worker sees its own Task's subtree.
		return nil, task.Readiness{}, task.ErrAccessDenied
	}
	var readiness task.Readiness
	var progress []task.PlanNode
	err = s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		subtree, err := getGlobalTask(ctx, conn, query.TreeID)
		if errors.Is(err, sql.ErrNoRows) {
			return &task.NotFoundError{ID: query.TreeID}
		}
		if err != nil {
			return err
		}
		now := s.now().UTC()
		nodes, err := loadPlanNodes(ctx, conn, subtree.RootID, access.sessionID, now)
		if err != nil {
			return err
		}
		builds, err := loadBuildAttempts(ctx, conn)
		if err != nil {
			return err
		}
		attachLatestBuilds(nodes, builds)
		folder, err := sessionFolder(ctx, conn, access.sessionID)
		if err != nil {
			return err
		}
		occupancy, err := buildOccupancy(ctx, conn, nodes, builds, func(w *delegation.Worktree) bool { return sameFolder(w.Repo, folder) })
		if err != nil {
			return err
		}
		capacity, err := dispatchCapacity(ctx, conn, access.sessionID, query)
		if err != nil {
			return err
		}
		readiness = task.NewPlan(nodes).Readiness(subtree.ID, occupancy, capacity)
		progress = nodes
		return nil
	})
	return progress, readiness, err
}

// loadPlanNodes reads every Task of the tree rooted at root in tree order
// (depth first, siblings in order), with each claim and whether its
// execution is still running; session is the querying conversation.
func loadPlanNodes(ctx context.Context, conn *sql.Conn, root task.ID, session memory.SessionID, now time.Time) ([]task.PlanNode, error) {
	rows, err := conn.QueryContext(ctx, `
		WITH RECURSIVE tree(task_id, path) AS (
			SELECT task_id, printf('%020d', sibling_order) FROM task_hierarchy WHERE task_id = ?
			UNION ALL
			SELECT child.task_id, tree.path || '/' || printf('%020d', child.sibling_order)
			FROM task_hierarchy child JOIN tree ON child.parent_id = tree.task_id
		)
		SELECT t.id, COALESCE(h.parent_id, ''), h.root_id, h.sibling_order,
		       t.scope, t.title, t.description, t.priority, t.due_date, t.result_summary,
		       t.status, t.revision, t.created_at, t.updated_at, t.depends_on, t.paths
		FROM tree JOIN tasks t ON t.id = tree.task_id JOIN task_hierarchy h ON h.task_id = t.id
		ORDER BY tree.path, t.id
	`, root)
	if err != nil {
		return nil, fmt.Errorf("read Task Tree plan: %w", err)
	}
	var nodes []task.PlanNode
	index := map[task.ID]int{}
	for rows.Next() {
		value, err := scanTask(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("read Task Tree plan: %w", err)
		}
		index[value.ID] = len(nodes)
		nodes = append(nodes, task.PlanNode{Task: value})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	claims, err := conn.QueryContext(ctx, `
		SELECT c.task_id, c.session_id, EXISTS (
			SELECT 1 FROM sessions s JOIN session_turn_leases l ON l.session_id = s.id
			WHERE s.id = c.session_id AND s.status = 'active' AND l.holder_id = c.lease_holder_id
			  AND l.fencing_token = c.lease_token AND l.lease_generation = c.lease_generation AND l.expires_at > ?
		)
		FROM task_claims c JOIN task_hierarchy h ON h.task_id = c.task_id
		WHERE h.root_id = ?
	`, now.Format(turnLeaseTimeFormat), root)
	if err != nil {
		return nil, fmt.Errorf("read Task Tree claims: %w", err)
	}
	defer claims.Close()
	for claims.Next() {
		var id task.ID
		var holder memory.SessionID
		var live bool
		if err := claims.Scan(&id, &holder, &live); err != nil {
			return nil, err
		}
		if i, ok := index[id]; ok {
			nodes[i].Claim = &task.PlanClaim{Live: live, Mine: holder == session}
		}
	}
	return nodes, claims.Err()
}

// loadBuildAttempts reads every build attempt, oldest first.
func loadBuildAttempts(ctx context.Context, conn *sql.Conn) ([]delegation.Attempt, error) {
	rows, err := conn.QueryContext(ctx, `SELECT record_json FROM subagent_executions
 WHERE CASE WHEN json_valid(record_json) THEN json_type(record_json,'$.worktree') IS NOT NULL END ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("read build attempts: %w", err)
	}
	defer rows.Close()
	var attempts []delegation.Attempt
	for rows.Next() {
		a, err := readSubagent(rows)
		if err != nil || a.Worktree == nil || a.Assignment.TaskID == "" {
			// An unreadable record proves nothing about any Task.
			continue
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

// latestBuilds maps each Task to its latest build attempt: a newer
// assignment or continuation supersedes earlier ones.
func latestBuilds(attempts []delegation.Attempt) map[task.ID]delegation.Attempt {
	latest := map[task.ID]delegation.Attempt{}
	for _, a := range attempts {
		latest[task.ID(a.Assignment.TaskID)] = a
	}
	return latest
}

// planBuild is an attempt as the orchestration read model sees it.
func planBuild(a delegation.Attempt) *task.PlanBuild {
	b := &task.PlanBuild{ExecutionID: a.ID, State: a.State, Branch: a.Worktree.Branch, Worktree: a.Worktree.Path,
		Removed: a.Worktree.RemovedAt != nil, Uncommitted: true}
	if a.Gate != nil {
		b.GateStep = a.Gate.Step
		b.NeedsRebase = a.Gate.NeedsRebase != nil
		b.Merged = a.Gate.Merged != nil
	}
	if a.Result != nil && a.Result.Build != nil {
		report := a.Result.Build
		b.HasCommits = len(report.Commits) > 0 || (report.Head != "" && report.Head != report.BaseCommit)
		b.Uncommitted = report.Uncommitted
	}
	return b
}

func attachLatestBuilds(nodes []task.PlanNode, attempts []delegation.Attempt) {
	latest := latestBuilds(attempts)
	for i := range nodes {
		if a, ok := latest[nodes[i].Task.ID]; ok {
			nodes[i].Build = planBuild(a)
		}
	}
}

// buildOccupancy lists the builds that hold paths in the conversation's
// repository (inRepo): each Task's latest build while it runs or awaits its
// merge. Builds on the tree's own Tasks always count.
func buildOccupancy(ctx context.Context, conn *sql.Conn, nodes []task.PlanNode, attempts []delegation.Attempt, inRepo func(*delegation.Worktree) bool) ([]task.PlanOccupancy, error) {
	inTree := map[task.ID]task.PlanNode{}
	for _, n := range nodes {
		inTree[n.Task.ID] = n
	}
	latest := latestBuilds(attempts)
	ids := make([]task.ID, 0, len(latest))
	for id := range latest {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var out []task.PlanOccupancy
	for _, id := range ids {
		a := latest[id]
		node, mine := inTree[id]
		if !mine && !inRepo(a.Worktree) {
			continue
		}
		phase := planBuild(a).Phase()
		if phase != task.BuildRunning && phase != task.BuildAwaitingMerge {
			continue
		}
		o := task.PlanOccupancy{TaskID: id, ExecutionID: a.ID, Phase: phase, InTree: mine}
		if mine {
			o.Paths, o.Title = node.Task.Paths, node.Task.Title
		} else {
			paths, err := storedTaskPaths(ctx, conn, id)
			if err != nil {
				return nil, err
			}
			o.Paths = paths
		}
		out = append(out, o)
	}
	return out, nil
}

// storedTaskPaths reads one Task's paths directly: the harness compares
// paths across scopes in one repository, so it reads them as the Kernel.
func storedTaskPaths(ctx context.Context, conn *sql.Conn, id task.ID) ([]string, error) {
	var stored sql.NullString
	err := conn.QueryRowContext(ctx, `SELECT paths FROM tasks WHERE id = ?`, id).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeTaskList[string](stored)
}

// sessionFolder is a conversation's current folder: its Workspace's latest
// attached folder, or its project root, or empty.
func sessionFolder(ctx context.Context, conn *sql.Conn, session memory.SessionID) (string, error) {
	var workspace, root sql.NullString
	err := conn.QueryRowContext(ctx, `SELECT workspace_id, project_root_snapshot FROM sessions WHERE id = ?`, session).Scan(&workspace, &root)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return conversationFolder(ctx, conn, memory.ScopeContext{WorkspaceID: memory.WorkspaceID(workspace.String), ProjectRoot: root.String})
}

// sameFolder reports whether repo is folder, literally or after resolving
// links.
func sameFolder(repo, folder string) bool {
	if repo == "" || folder == "" {
		return false
	}
	if repo == folder {
		return true
	}
	resolved, err := filepath.EvalSymlinks(folder)
	return err == nil && resolved == repo
}

// dispatchCapacity counts the workers that bound the next batch.
func dispatchCapacity(ctx context.Context, conn *sql.Conn, session memory.SessionID, query task.ReadyQuery) (task.DispatchCapacity, error) {
	var here, overall, started int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM subagent_executions WHERE state IN ('admitted','running') AND parent_session_id = ?`, session).Scan(&here); err != nil {
		return task.DispatchCapacity{}, err
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM subagent_executions WHERE state IN ('admitted','running')`).Scan(&overall); err != nil {
		return task.DispatchCapacity{}, err
	}
	if query.TurnSourceEventID != "" {
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM subagent_executions WHERE parent_session_id = ?
 AND CASE WHEN json_valid(record_json) THEN json_extract(record_json,'$.parent.SourceEventID') END = ?`, session, query.TurnSourceEventID).Scan(&started); err != nil {
			return task.DispatchCapacity{}, err
		}
	}
	return query.Limits.Capacity(here, overall, started, query.Max), nil
}

// orchestratingParent reports whether a parent was composed with the Task
// Tree orchestration contract (todo_ready, Stage 13). Its admissions apply
// the conservative rule: a Task without paths may touch any file, and a
// Task inside another build's running work is that worker's. A parent
// pinned before Stage 13 cannot declare paths, so its builds keep running
// in parallel as before; declared overlaps and dependencies bind everyone.
func orchestratingParent(ctx context.Context, conn *sql.Conn, session memory.SessionID) (bool, error) {
	receipt, err := getCompositionReceipt(ctx, conn, session)
	if err != nil {
		return false, err
	}
	return hasCapability(receipt, string(task.CapabilityReady)), nil
}

// buildOverlap applies the conflict rule for one admission: declared paths
// that overlap always conflict; a Task without paths conflicts only under
// the conservative rule.
func buildOverlap(left, right []string, conservative bool) task.Overlap {
	if !conservative && (len(left) == 0 || len(right) == 0) {
		return task.Overlap{}
	}
	return task.PathsOverlap(left, right)
}

// checkBuildOrchestration enforces the Task Tree rules on one fresh build
// assignment inside its admission transaction (Stage 13): the Task is not
// part of another Task's running work, its dependencies (its own and its
// ancestors') are done, and its paths overlap neither a build running in
// the same repository nor an earlier assignment of the same call.
func checkBuildOrchestration(ctx context.Context, conn *sql.Conn, id task.ID, plan delegation.Worktree, earlier []task.ID, conservative bool, now time.Time) error {
	root, err := taskRootID(ctx, conn, id)
	if err != nil {
		return err
	}
	nodes, err := loadPlanNodes(ctx, conn, root, "", now)
	if err != nil {
		return err
	}
	attempts, err := loadBuildAttempts(ctx, conn)
	if err != nil {
		return err
	}
	attachLatestBuilds(nodes, attempts)
	p := task.NewPlan(nodes)
	node, _ := p.Node(id)
	if cover, covered := p.CoveringWork(id); covered && conservative {
		return fmt.Errorf("Task %s is part of Task %s, whose build %s is running; that worker's Task Access Grant covers its subtasks, so dispatch it after that build finishes",
			id, cover.Task.ID, cover.Build.ExecutionID)
	}
	if unfinished := p.UnfinishedDependencies(id); len(unfinished) > 0 {
		descriptions := make([]string, len(unfinished))
		for i, d := range unfinished {
			descriptions[i] = d.Describe()
		}
		return fmt.Errorf("Task %s depends on unfinished work: %s; dispatch it once those are completed and their builds merged (todo_ready lists what is ready)",
			id, strings.Join(descriptions, ", "))
	}
	for _, other := range earlier {
		otherPaths := nodePaths(p, other)
		if !p.Has(other) {
			if otherPaths, err = storedTaskPaths(ctx, conn, other); err != nil {
				return err
			}
		}
		if overlap := buildOverlap(node.Task.Paths, otherPaths, conservative); overlap.Overlap {
			return fmt.Errorf("Task %s conflicts with Task %s in this same delegate_build call (%s); conflicting Tasks never run at once, so dispatch them in separate batches (todo_ready batches conflict-free Tasks)",
				id, other, overlap.Describe())
		}
	}
	return checkRunningBuildConflicts(ctx, conn, p, id, node.Task.Paths, plan, attempts, "", conservative)
}

// checkRunningBuildConflicts refuses work on id whose paths overlap a
// build that is running in the same repository on another Task. except is
// a child session whose own attempts are ignored (a continuation's).
func checkRunningBuildConflicts(ctx context.Context, conn *sql.Conn, p *task.Plan, id task.ID, paths []string, plan delegation.Worktree, attempts []delegation.Attempt, except memory.SessionID, conservative bool) error {
	for _, a := range attempts {
		if a.Terminal() || a.Child.ID == except || task.ID(a.Assignment.TaskID) == id || !sameRepository(*a.Worktree, plan) {
			continue
		}
		other := task.ID(a.Assignment.TaskID)
		otherPaths := nodePaths(p, other)
		if !p.Has(other) {
			var err error
			if otherPaths, err = storedTaskPaths(ctx, conn, other); err != nil {
				return err
			}
		}
		overlap := buildOverlap(paths, otherPaths, conservative)
		if !overlap.Overlap {
			continue
		}
		on := "another Task"
		if p.Has(other) {
			on = "Task " + string(other)
		}
		return fmt.Errorf("Task %s conflicts with build %s, which is running in this repository on %s (%s); conflicting Tasks never run at once, so dispatch it after that build finishes (todo_ready batches conflict-free Tasks)",
			id, a.ID, on, overlap.Describe())
	}
	return nil
}

// checkContinuationConflicts refuses to continue a build worker on Task id
// while a build on an overlapping Task runs in the same repository.
func checkContinuationConflicts(ctx context.Context, conn *sql.Conn, a delegation.Attempt, now time.Time) error {
	if a.Worktree == nil || a.Assignment.TaskID == "" {
		return nil
	}
	// A run-owned build (Stage 14 sub-stage 9) has no parent composition:
	// it always takes the conservative rule.
	conservative := a.Run != nil
	if !conservative {
		var err error
		if conservative, err = orchestratingParent(ctx, conn, a.Parent.Scope.SessionID); err != nil {
			return err
		}
	}
	id := task.ID(a.Assignment.TaskID)
	root, err := taskRootID(ctx, conn, id)
	if err != nil {
		return err
	}
	nodes, err := loadPlanNodes(ctx, conn, root, "", now)
	if err != nil {
		return err
	}
	attempts, err := loadBuildAttempts(ctx, conn)
	if err != nil {
		return err
	}
	p := task.NewPlan(nodes)
	return checkRunningBuildConflicts(ctx, conn, p, id, nodePaths(p, id), *a.Worktree, attempts, a.Child.ID, conservative)
}

func nodePaths(p *task.Plan, id task.ID) []string {
	node, _ := p.Node(id)
	return node.Task.Paths
}

func sameRepository(a, b delegation.Worktree) bool {
	if a.CommonDir != "" && b.CommonDir != "" {
		return a.CommonDir == b.CommonDir
	}
	return a.Repo != "" && a.Repo == b.Repo
}
