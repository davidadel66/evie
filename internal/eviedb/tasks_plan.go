package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/davidadel66/evie/internal/task"
)

// Task plans (Stage 13 of docs/memory-architecture.md, 2026-10-04). A Task
// may name the Tasks of its tree it depends on and the paths it expects to
// touch. Both are stored as JSON arrays on the Task's state and on each
// revision snapshot, NULL when empty, so earlier rows read as before.

// ensureTaskPlan adds the plan columns to tasks and task_revisions once. It
// runs after the Context Scope upgrade, which rebuilds those tables with
// their earlier columns.
func ensureTaskPlan(ctx context.Context, db *sql.DB) error {
	columns := []struct{ table, name string }{
		{"tasks", "depends_on"}, {"tasks", "paths"}, {"task_revisions", "depends_on"}, {"task_revisions", "paths"},
	}
	missing := false
	for _, column := range columns {
		present, err := tableHasColumn(ctx, db, column.table, column.name)
		if err != nil {
			return err
		}
		missing = missing || !present
	}
	if !missing {
		return nil
	}
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		for _, column := range columns {
			present, err := tableHasColumn(ctx, conn, column.table, column.name)
			if err != nil {
				return err
			}
			if present {
				continue
			}
			if _, err := conn.ExecContext(ctx, `ALTER TABLE `+column.table+` ADD COLUMN `+column.name+
				` TEXT CHECK (`+column.name+` IS NULL OR (json_valid(`+column.name+`) AND json_type(`+column.name+`) = 'array'))`); err != nil {
				return fmt.Errorf("add %s.%s: %w", column.table, column.name, err)
			}
		}
		return nil
	})
}

// encodeTaskList is a plan list as stored: NULL when empty.
func encodeTaskList[T any](values []T) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode Task plan: %w", err)
	}
	return string(encoded), nil
}

// encodeTaskPlan is a Task's dependencies and paths as stored.
func encodeTaskPlan(value task.Task) (any, any, error) {
	dependsOn, err := encodeTaskList(value.DependsOn)
	if err != nil {
		return nil, nil, err
	}
	paths, err := encodeTaskList(value.Paths)
	return dependsOn, paths, err
}

// decodeTaskList reads a stored plan list; NULL and an empty array are nil.
func decodeTaskList[T any](stored sql.NullString) ([]T, error) {
	if !stored.Valid || stored.String == "" {
		return nil, nil
	}
	var values []T
	if err := json.Unmarshal([]byte(stored.String), &values); err != nil {
		return nil, fmt.Errorf("decode Task plan: %w", err)
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

// treeDependencyNodes reads every Task of a tree with its parent and own
// dependencies, for the cycle check.
func treeDependencyNodes(ctx context.Context, conn *sql.Conn, rootID task.ID) ([]task.DependencyNode, map[task.ID]task.ID, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT h.task_id, COALESCE(h.parent_id, ''), t.depends_on
		FROM task_hierarchy h JOIN tasks t ON t.id = h.task_id
		WHERE h.root_id = ?
		ORDER BY h.task_id
	`, rootID)
	if err != nil {
		return nil, nil, fmt.Errorf("read Task Tree dependencies: %w", err)
	}
	defer rows.Close()
	var nodes []task.DependencyNode
	parents := map[task.ID]task.ID{}
	for rows.Next() {
		var node task.DependencyNode
		var dependsOn sql.NullString
		if err := rows.Scan(&node.ID, &node.ParentID, &dependsOn); err != nil {
			return nil, nil, err
		}
		if node.DependsOn, err = decodeTaskList[task.ID](dependsOn); err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, node)
		parents[node.ID] = node.ParentID
	}
	return nodes, parents, rows.Err()
}

// errDelegatedPlan refuses plan fields from a delegated session: the
// orchestrating conversation plans dependencies and paths; a worker
// contributes beneath its granted Task (Stage 13).
var errDelegatedPlan = &task.InputError{Field: "plan", Message: "dependencies and paths are planned by the orchestrating conversation, not by delegated workers"}

// checkTaskDependencies proves proposed dependencies against the tree: each
// names another Task of the same tree that is neither an ancestor nor a
// descendant, and the tree stays free of cycles. proposed maps each Task
// whose dependencies change (or that is being created, with parent) to its
// new list. It returns an input error naming the problem, or nil.
func checkTaskDependencies(ctx context.Context, conn *sql.Conn, rootID task.ID, created map[task.ID]task.ID, proposed map[task.ID][]task.ID) (*task.InputError, error) {
	nodes, parents, err := treeDependencyNodes(ctx, conn, rootID)
	if err != nil {
		return nil, err
	}
	for id, parent := range created {
		nodes = append(nodes, task.DependencyNode{ID: id, ParentID: parent})
		parents[id] = parent
	}
	slices.SortFunc(nodes, func(a, b task.DependencyNode) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	ancestorOf := func(ancestor, id task.ID) bool {
		for at, hops := parents[id], 0; at != "" && hops <= len(parents); at, hops = parents[at], hops+1 {
			if at == ancestor {
				return true
			}
		}
		return false
	}
	ids := make([]task.ID, 0, len(proposed))
	for id := range proposed {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		for _, dependency := range proposed[id] {
			_, inTree := parents[dependency]
			switch {
			case dependency == id:
				return &task.InputError{Field: "depends_on", Message: "must not name the Task itself"}, nil
			case !inTree:
				return &task.InputError{Field: "depends_on", Message: fmt.Sprintf("Task %s is not in this Task Tree; dependencies are between Tasks of one tree", dependency)}, nil
			case ancestorOf(dependency, id):
				return &task.InputError{Field: "depends_on", Message: fmt.Sprintf("must not name ancestor %s: a Task's subtasks finish before it does", dependency)}, nil
			case ancestorOf(id, dependency):
				return &task.InputError{Field: "depends_on", Message: fmt.Sprintf("must not name descendant %s: its subtasks already finish before it does", dependency)}, nil
			}
		}
	}
	for i := range nodes {
		if dependencies, changed := proposed[nodes[i].ID]; changed {
			nodes[i].DependsOn = dependencies
		}
	}
	if cycle := task.DependencyCycle(nodes); cycle != nil {
		return &task.InputError{Field: "depends_on", Message: "would create a dependency cycle: " + task.DescribeCycle(cycle)}, nil
	}
	return nil, nil
}

// taskRootID reads a Task's tree root.
func taskRootID(ctx context.Context, source queryRower, id task.ID) (task.ID, error) {
	var root task.ID
	err := source.QueryRowContext(ctx, `SELECT root_id FROM task_hierarchy WHERE task_id = ?`, id).Scan(&root)
	if errors.Is(err, sql.ErrNoRows) {
		return "", &task.NotFoundError{ID: id}
	}
	return root, err
}
