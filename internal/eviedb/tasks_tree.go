package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

type decompositionResult struct {
	ParentID          task.ID
	EventID           string
	OutcomeCode       string
	DiagnosticField   string
	PreviousRevision  *uint64
	ResultingRevision *uint64
	FromStatus        task.Status
	ToStatus          task.Status
}

func (s *Store) DecomposeGlobalTask(
	ctx context.Context,
	id task.ID,
	input task.DecomposeInput,
) (task.Decomposition, error) {
	if err := ctx.Err(); err != nil {
		return task.Decomposition{}, err
	}
	if strings.TrimSpace(string(id)) == "" {
		return task.Decomposition{}, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if err := task.ValidateIdempotencyKey(input.IdempotencyKey); err != nil {
		return task.Decomposition{}, err
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.Decomposition{}, err
	}
	requestSHA256, err := canonicalDecomposeRequestSHA256(id, input)
	if err != nil {
		return task.Decomposition{}, err
	}
	identitySHA256 := idempotencySHA256(input.IdempotencyKey)
	var result task.Decomposition
	var businessErr error
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		claim, found, err := readIdempotencyClaim(ctx, conn, attribution, identitySHA256)
		if err != nil {
			return err
		}
		if found {
			if claim.RequestSHA256 != requestSHA256 || claim.Operation != task.OperationDecompose {
				if err := insertIdempotencyConflict(ctx, conn, attribution, identitySHA256, claim.RequestSHA256,
					requestSHA256, task.OperationDecompose, id, s.now().UTC()); err != nil {
					return err
				}
				businessErr = &task.IdempotencyConflictError{Operation: task.OperationDecompose}
				return nil
			}
			stored, err := readDecompositionResult(ctx, conn, attribution, identitySHA256)
			if err != nil {
				return err
			}
			if stored.OutcomeCode == mutationAccepted && stored.ResultingRevision != nil {
				result, err = loadDecompositionResult(ctx, conn, attribution, identitySHA256, stored)
				return err
			}
			businessErr = replayDecompositionError(id, input, stored)
			return nil
		}

		now := s.now().UTC()
		if validationErr := task.ValidateDecomposeInput(input); validationErr != nil {
			if err := insertDecompositionResult(ctx, conn, attribution, identitySHA256, requestSHA256, decompositionResult{
				ParentID: id, OutcomeCode: mutationInvalidInput, DiagnosticField: inputErrorField(validationErr),
			}, now); err != nil {
				return err
			}
			businessErr = validationErr
			return nil
		}
		parent, err := getGlobalTask(ctx, conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			if err := insertDecompositionResult(ctx, conn, attribution, identitySHA256, requestSHA256,
				decompositionResult{ParentID: id, OutcomeCode: mutationNotFound}, now); err != nil {
				return err
			}
			businessErr = &task.NotFoundError{ID: id}
			return nil
		}
		if err != nil {
			return fmt.Errorf("get Task decomposition parent: %w", err)
		}
		reject := func(cause error, code task.DiagnosticCode, outcome, field string, to task.Status) error {
			eventID, err := appendHierarchyEvent(ctx, conn, task.Event{
				TaskID: parent.ID, Operation: task.OperationDecompose,
				ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
				RecordedAt: now, PreviousRevision: parent.Revision, ResultingRevision: parent.Revision,
				Outcome: task.MutationRejected, DiagnosticCode: code,
			})
			if err != nil {
				return err
			}
			if err := linkTaskEventIdempotency(ctx, conn, eventID, identitySHA256); err != nil {
				return err
			}
			previous, resulting := parent.Revision, parent.Revision
			if err := insertDecompositionResult(ctx, conn, attribution, identitySHA256, requestSHA256, decompositionResult{
				ParentID: parent.ID, EventID: eventID, OutcomeCode: outcome, DiagnosticField: field,
				PreviousRevision: &previous, ResultingRevision: &resulting,
				FromStatus: parent.Status, ToStatus: to,
			}, now); err != nil {
				return err
			}
			businessErr = cause
			return nil
		}
		if input.ExpectedRevision != parent.Revision {
			return reject(
				&task.ConflictError{ID: id, Expected: input.ExpectedRevision, Current: parent.Revision},
				task.DiagnosticRevisionConflict, mutationRevisionConflict, "", "",
			)
		}
		if parent.Status == task.StatusCompleted || parent.Status == task.StatusCancelled {
			return reject(
				&task.TransitionError{From: parent.Status, To: task.StatusOpen},
				task.DiagnosticInvalidTransition, mutationInvalidTransition, "parent_id", task.StatusOpen,
			)
		}

		firstOrder, err := nextSiblingOrder(ctx, conn, parent.ID)
		if err != nil {
			return err
		}
		children := make([]task.Task, len(input.Children))
		for i, childInput := range input.Children {
			childID, err := uuid.NewRandom()
			if err != nil {
				return fmt.Errorf("generate decomposed Task ID: %w", err)
			}
			child := task.Task{
				ID: task.ID(childID.String()), ParentID: parent.ID, RootID: parent.RootID,
				SiblingOrder: firstOrder + uint64(i), Scope: parent.Scope,
				Title: childInput.Title, Description: childInput.Description, Priority: childInput.Priority,
				DueDate: childInput.DueDate, Status: task.StatusOpen, Revision: 1, CreatedAt: now, UpdatedAt: now,
			}
			if err := insertTaskState(ctx, conn, child); err != nil {
				return err
			}
			if err := insertTaskHierarchy(ctx, conn, child); err != nil {
				return err
			}
			eventID, err := appendTaskEvent(ctx, conn, task.Event{
				TaskID: child.ID, Operation: task.OperationCreate,
				ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
				RecordedAt: now, PreviousRevision: 0, ResultingRevision: 1, Outcome: task.MutationAccepted,
			})
			if err != nil {
				return err
			}
			if err := linkTaskEventIdempotency(ctx, conn, eventID, identitySHA256); err != nil {
				return err
			}
			if err := insertTaskRevision(ctx, conn, child); err != nil {
				return err
			}
			children[i] = child
		}
		updatedParent, parentEventID, err := bumpTaskForHierarchy(
			ctx, conn, parent, task.OperationDecompose, attribution, identitySHA256, now,
		)
		if err != nil {
			return err
		}
		previous, resulting := parent.Revision, updatedParent.Revision
		if err := insertDecompositionResult(ctx, conn, attribution, identitySHA256, requestSHA256, decompositionResult{
			ParentID: parent.ID, EventID: parentEventID, OutcomeCode: mutationAccepted,
			PreviousRevision: &previous, ResultingRevision: &resulting,
			FromStatus: parent.Status, ToStatus: parent.Status,
		}, now); err != nil {
			return err
		}
		for i, child := range children {
			if err := insertDecompositionChild(ctx, conn, attribution, identitySHA256, i, child); err != nil {
				return err
			}
		}
		result = task.Decomposition{Parent: updatedParent, Children: children}
		return nil
	})
	if err != nil {
		return task.Decomposition{}, err
	}
	if businessErr != nil {
		return task.Decomposition{}, businessErr
	}
	return result, nil
}

func readDecompositionResult(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256 string,
) (decompositionResult, error) {
	var result decompositionResult
	var eventID, field, fromStatus, toStatus sql.NullString
	var previous, resulting sql.NullInt64
	err := conn.QueryRowContext(ctx, `
		SELECT parent_id, event_id, outcome_code, diagnostic_field, previous_revision,
		       resulting_revision, from_status, to_status
		FROM task_decomposition_results
		WHERE actor_id = ? AND session_id = ? AND identity_sha256 = ?
	`, attribution.ActorID, attribution.SessionID, identitySHA256).Scan(
		&result.ParentID, &eventID, &result.OutcomeCode, &field, &previous, &resulting, &fromStatus, &toStatus,
	)
	if err != nil {
		return decompositionResult{}, fmt.Errorf("read Task decomposition result: %w", err)
	}
	result.EventID = eventID.String
	result.DiagnosticField = field.String
	result.FromStatus = task.Status(fromStatus.String)
	result.ToStatus = task.Status(toStatus.String)
	if previous.Valid {
		value := uint64(previous.Int64)
		result.PreviousRevision = &value
	}
	if resulting.Valid {
		value := uint64(resulting.Int64)
		result.ResultingRevision = &value
	}
	return result, nil
}

func insertDecompositionResult(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256, requestSHA256 string,
	result decompositionResult,
	recordedAt time.Time,
) error {
	if err := insertIdempotencyClaim(ctx, conn, attribution, identitySHA256, requestSHA256,
		task.OperationDecompose, recordedAt); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_decomposition_results (
			actor_id, session_id, identity_sha256, parent_id, event_id, outcome_code,
			diagnostic_field, previous_revision, resulting_revision, from_status, to_status
		) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?, NULLIF(?, ''), NULLIF(?, ''))
	`, attribution.ActorID, attribution.SessionID, identitySHA256, result.ParentID, result.EventID,
		result.OutcomeCode, result.DiagnosticField, nullableUint64(result.PreviousRevision),
		nullableUint64(result.ResultingRevision), result.FromStatus, result.ToStatus)
	if err != nil {
		return fmt.Errorf("insert Task decomposition result: %w", err)
	}
	return nil
}

func insertDecompositionChild(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256 string,
	order int,
	child task.Task,
) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_decomposition_children (
			actor_id, session_id, identity_sha256, child_order, child_task_id, child_revision
		) VALUES (?, ?, ?, ?, ?, ?)
	`, attribution.ActorID, attribution.SessionID, identitySHA256, order, child.ID, child.Revision)
	if err != nil {
		return fmt.Errorf("insert Task decomposition child: %w", err)
	}
	return nil
}

func loadDecompositionResult(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256 string,
	stored decompositionResult,
) (task.Decomposition, error) {
	parent, err := getTaskRevision(ctx, conn, stored.ParentID, *stored.ResultingRevision)
	if err != nil {
		return task.Decomposition{}, fmt.Errorf("load Task decomposition parent revision: %w", err)
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT child_task_id, child_revision
		FROM task_decomposition_children
		WHERE actor_id = ? AND session_id = ? AND identity_sha256 = ?
		ORDER BY child_order
	`, attribution.ActorID, attribution.SessionID, identitySHA256)
	if err != nil {
		return task.Decomposition{}, fmt.Errorf("list Task decomposition children: %w", err)
	}
	defer rows.Close()
	var children []task.Task
	for rows.Next() {
		var id task.ID
		var revision uint64
		if err := rows.Scan(&id, &revision); err != nil {
			return task.Decomposition{}, err
		}
		child, err := getTaskRevision(ctx, conn, id, revision)
		if err != nil {
			return task.Decomposition{}, err
		}
		children = append(children, child)
	}
	if err := rows.Err(); err != nil {
		return task.Decomposition{}, err
	}
	return task.Decomposition{Parent: parent, Children: children}, nil
}

func replayDecompositionError(id task.ID, input task.DecomposeInput, stored decompositionResult) error {
	switch stored.OutcomeCode {
	case mutationInvalidInput:
		if err := task.ValidateDecomposeInput(input); err != nil {
			return err
		}
		return &task.InputError{Field: stored.DiagnosticField, Message: "original mutation was rejected"}
	case mutationNotFound:
		return &task.NotFoundError{ID: id}
	case mutationRevisionConflict:
		current := uint64(0)
		if stored.ResultingRevision != nil {
			current = *stored.ResultingRevision
		}
		return &task.ConflictError{ID: id, Expected: input.ExpectedRevision, Current: current}
	case mutationInvalidTransition:
		return &task.TransitionError{From: stored.FromStatus, To: stored.ToStatus}
	default:
		return fmt.Errorf("replay Task decomposition outcome %q", stored.OutcomeCode)
	}
}

func (s *Store) GetGlobalTaskTree(ctx context.Context, id task.ID, query task.TreeQuery) (task.Tree, error) {
	if strings.TrimSpace(string(id)) == "" {
		return task.Tree{}, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if err := task.ValidateTreeQuery(query); err != nil {
		return task.Tree{}, err
	}
	root, err := s.GetGlobalTask(ctx, id)
	if err != nil {
		return task.Tree{}, err
	}
	maxDepth := query.MaxDepth
	if maxDepth == 0 {
		maxDepth = task.DefaultTreeDepth
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE descendants(task_id, depth, path) AS (
			SELECT task_id, 0, printf('%020d', sibling_order) FROM task_hierarchy WHERE task_id = ?
			UNION ALL
			SELECT child.task_id, descendants.depth + 1,
			       descendants.path || '/' || printf('%020d', child.sibling_order)
			FROM task_hierarchy child JOIN descendants ON child.parent_id = descendants.task_id
			WHERE descendants.depth < ?
		)
		SELECT t.id, COALESCE(h.parent_id, ''), h.root_id, h.sibling_order,
		       t.scope, t.title, t.description, t.priority, t.due_date,
		       t.status, t.revision, t.created_at, t.updated_at, descendants.depth,
		       CASE WHEN descendants.depth = ? THEN EXISTS (
		           WITH RECURSIVE below(task_id) AS (
		               SELECT task_id FROM task_hierarchy WHERE parent_id = t.id
		               UNION ALL
		               SELECT child.task_id
		               FROM task_hierarchy child JOIN below ON child.parent_id = below.task_id
		           )
		           SELECT 1 FROM below
		           JOIN tasks below_task ON below_task.id = below.task_id
		           WHERE ? OR below_task.status IN ('open', 'in_progress', 'blocked')
		       ) ELSE 0 END
		FROM descendants
		JOIN tasks t ON t.id = descendants.task_id
		JOIN task_hierarchy h ON h.task_id = t.id
		ORDER BY descendants.path, t.id
		LIMIT ?
	`, id, maxDepth, maxDepth, query.IncludeHistory, task.MaxTreeNodes+1)
	if err != nil {
		return task.Tree{}, fmt.Errorf("read Task tree: %w", err)
	}
	defer rows.Close()
	type flatNode struct {
		value                  task.Task
		depth                  int
		hasRelevantDescendants bool
	}
	var flat []flatNode
	for rows.Next() {
		var node flatNode
		var description, dueDate sql.NullString
		var priority sql.NullInt64
		var createdText, updatedText string
		if err := rows.Scan(&node.value.ID, &node.value.ParentID, &node.value.RootID, &node.value.SiblingOrder,
			&node.value.Scope, &node.value.Title, &description, &priority, &dueDate, &node.value.Status,
			&node.value.Revision, &createdText, &updatedText, &node.depth, &node.hasRelevantDescendants); err != nil {
			return task.Tree{}, err
		}
		node.value.Description = description.String
		node.value.Priority = int(priority.Int64)
		node.value.DueDate = dueDate.String
		node.value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdText)
		if err != nil {
			return task.Tree{}, err
		}
		node.value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedText)
		if err != nil {
			return task.Tree{}, err
		}
		flat = append(flat, node)
	}
	if err := rows.Err(); err != nil {
		return task.Tree{}, err
	}
	if err := rows.Close(); err != nil {
		return task.Tree{}, err
	}
	if s.afterTaskTreeRead != nil {
		s.afterTaskTreeRead()
	}
	if len(flat) == 0 {
		return task.Tree{Task: root}, nil
	}
	truncatedByNodes := len(flat) > task.MaxTreeNodes
	if truncatedByNodes {
		flat = flat[:task.MaxTreeNodes]
	}
	values := make(map[task.ID]task.Task, len(flat))
	childrenByParent := make(map[task.ID][]task.ID, len(flat))
	depths := make(map[task.ID]int, len(flat))
	hasRelevantDescendants := make(map[task.ID]bool, len(flat))
	var rootID task.ID
	for _, flatNode := range flat {
		values[flatNode.value.ID] = flatNode.value
		depths[flatNode.value.ID] = flatNode.depth
		hasRelevantDescendants[flatNode.value.ID] = flatNode.hasRelevantDescendants
		if flatNode.depth == 0 {
			rootID = flatNode.value.ID
		} else {
			childrenByParent[flatNode.value.ParentID] = append(childrenByParent[flatNode.value.ParentID], flatNode.value.ID)
		}
	}
	if rootID == "" {
		return task.Tree{}, fmt.Errorf("Task tree root %q was not returned", id)
	}
	var build func(task.ID) (task.Tree, bool)
	build = func(id task.ID) (task.Tree, bool) {
		node := task.Tree{Task: values[id], Children: []task.Tree{}}
		for _, childID := range childrenByParent[id] {
			child, include := build(childID)
			if include {
				node.Children = append(node.Children, child)
			}
		}
		active := node.Task.Status == task.StatusOpen || node.Task.Status == task.StatusInProgress ||
			node.Task.Status == task.StatusBlocked
		structural := len(node.Children) > 0 || (depths[id] == maxDepth && hasRelevantDescendants[id])
		return node, id == rootID || query.IncludeHistory || active || structural
	}
	tree, _ := build(rootID)
	truncatedByDepth := false
	for _, node := range flat {
		if node.depth == maxDepth && node.hasRelevantDescendants {
			truncatedByDepth = true
			break
		}
	}
	tree.Truncated = truncatedByNodes || truncatedByDepth
	return tree, nil
}
