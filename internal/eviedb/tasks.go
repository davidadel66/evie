package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

var _ task.Service = (*Store)(nil)

func (s *Store) CreateGlobalTask(ctx context.Context, input task.CreateInput) (task.Task, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityAdd, task.AccessContribute)
	if err := ctx.Err(); err != nil {
		return task.Task{}, err
	}
	if err := task.ValidateIdempotencyKey(input.IdempotencyKey); err != nil {
		return task.Task{}, err
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.Task{}, err
	}
	if input.ParentID == "" && attribution.ParentSessionID != "" {
		return task.Task{}, task.ErrRootCreationDenied
	}
	access, err := taskAccessFromContext(ctx, s.db)
	if err != nil {
		return task.Task{}, err
	}
	if input.ParentID == "" && access.delegated {
		return task.Task{}, task.ErrRootCreationDenied
	}
	targetScope, err := access.selected(input.Scope)
	if err != nil {
		return task.Task{}, err
	}
	requestSHA256, err := canonicalCreateRequestSHA256(input, targetScope)
	if err != nil {
		return task.Task{}, err
	}
	identitySHA256 := idempotencySHA256(input.IdempotencyKey)
	var created task.Task
	var businessErr error
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		transactionAccess, err := taskAccessFromContext(ctx, conn)
		if err != nil {
			return err
		}
		if input.ParentID == "" && transactionAccess.delegated {
			return task.ErrRootCreationDenied
		}
		if input.ParentID != "" && transactionAccess.delegated {
			if _, err := getGlobalTask(ctx, conn, input.ParentID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return &task.NotFoundError{ID: input.ParentID}
				}
				return err
			}
		}
		prior, found, err := readMutationResult(ctx, conn, attribution, identitySHA256)
		if err != nil {
			return err
		}
		if found {
			if prior.RequestSHA256 != requestSHA256 || prior.Operation != task.OperationCreate {
				if err := insertIdempotencyConflict(ctx, conn, attribution, identitySHA256, prior.RequestSHA256,
					requestSHA256, task.OperationCreate, prior.TaskID, s.now().UTC()); err != nil {
					return err
				}
				businessErr = &task.IdempotencyConflictError{Operation: task.OperationCreate}
				return nil
			}
			if prior.OutcomeCode == mutationAccepted && prior.ResultingRevision != nil {
				created, err = getTaskRevision(ctx, conn, prior.TaskID, *prior.ResultingRevision)
				return err
			}
			businessErr = replayCreateError(input, prior)
			return nil
		}
		now := s.now().UTC()
		if validationErr := task.ValidateCreateInput(input); validationErr != nil {
			field := inputErrorField(validationErr)
			if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
				Operation: task.OperationCreate, OutcomeCode: mutationInvalidInput, DiagnosticField: field,
			}, now); err != nil {
				return err
			}
			businessErr = validationErr
			return nil
		}
		if input.ParentID != "" {
			var createErr error
			created, businessErr, createErr = s.createChildTask(
				ctx, conn, attribution, identitySHA256, requestSHA256, input, now,
			)
			if createErr != nil || businessErr != nil || !input.Focus {
				return createErr
			}
			return upsertTaskFocus(ctx, conn, transactionAccess.sessionID, created.ID, now)
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return fmt.Errorf("generate task ID: %w", err)
		}
		created = task.Task{
			ID: task.ID(id.String()), RootID: task.ID(id.String()), Scope: targetScope,
			Title: input.Title, Description: input.Description, Priority: input.Priority, DueDate: input.DueDate,
			Status: task.StatusOpen, Revision: 1, CreatedAt: now, UpdatedAt: now,
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO tasks (
				id, scope, title, description, priority, due_date, result_summary,
				status, revision, created_at, updated_at
			) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)
		`, created.ID, created.Scope, created.Title, created.Description, created.Priority, created.DueDate,
			created.ResultSummary, created.Status, created.Revision, formatTaskTime(created.CreatedAt), formatTaskTime(created.UpdatedAt)); err != nil {
			return fmt.Errorf("insert global task: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO task_hierarchy (task_id, parent_id, root_id, sibling_order)
			VALUES (?, NULL, ?, 0)
		`, created.ID, created.ID); err != nil {
			return fmt.Errorf("insert root Task hierarchy: %w", err)
		}
		eventID, err := appendTaskEvent(ctx, conn, task.Event{
			TaskID: created.ID, Operation: task.OperationCreate,
			ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
			RecordedAt: now, PreviousRevision: 0, ResultingRevision: 1, Outcome: task.MutationAccepted,
		})
		if err != nil {
			return err
		}
		if err := linkTaskEventIdempotency(ctx, conn, eventID, identitySHA256); err != nil {
			return err
		}
		if err := insertTaskRevision(ctx, conn, created); err != nil {
			return err
		}
		previous, resulting := uint64(0), uint64(1)
		if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
			Operation: task.OperationCreate, TaskID: created.ID, EventID: eventID, OutcomeCode: mutationAccepted,
			PreviousRevision: &previous, ResultingRevision: &resulting,
		}, now); err != nil {
			return err
		}
		if input.Focus {
			return upsertTaskFocus(ctx, conn, transactionAccess.sessionID, created.ID, now)
		}
		return nil
	})
	if err != nil {
		return task.Task{}, err
	}
	if businessErr != nil {
		return task.Task{}, businessErr
	}
	return access.project(created), nil
}

func replayCreateError(input task.CreateInput, prior mutationResult) error {
	switch prior.OutcomeCode {
	case mutationInvalidInput:
		if err := task.ValidateCreateInput(input); err != nil {
			return err
		}
		return &task.InputError{Field: prior.DiagnosticField, Message: "original mutation was rejected"}
	case mutationNotFound:
		return &task.NotFoundError{ID: input.ParentID}
	case mutationRevisionConflict:
		current := uint64(0)
		if prior.ResultingRevision != nil {
			current = *prior.ResultingRevision
		}
		return &task.ConflictError{ID: input.ParentID, Expected: input.ExpectedParentRevision, Current: current}
	case mutationInvalidTransition:
		return &task.TransitionError{From: prior.FromStatus, To: task.StatusOpen}
	default:
		return fmt.Errorf("replay Task create outcome %q", prior.OutcomeCode)
	}
}

func (s *Store) createChildTask(
	ctx context.Context,
	conn *sql.Conn,
	attribution task.MutationAttribution,
	identitySHA256, requestSHA256 string,
	input task.CreateInput,
	now time.Time,
) (task.Task, error, error) {
	parent, err := getGlobalTask(ctx, conn, input.ParentID)
	if errors.Is(err, sql.ErrNoRows) {
		if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
			Operation: task.OperationCreate, TaskID: input.ParentID, OutcomeCode: mutationNotFound,
		}, now); err != nil {
			return task.Task{}, nil, err
		}
		return task.Task{}, &task.NotFoundError{ID: input.ParentID}, nil
	}
	if err != nil {
		return task.Task{}, nil, fmt.Errorf("get child Task parent: %w", err)
	}
	reject := func(cause error, code task.DiagnosticCode, outcome, field string, to task.Status) (task.Task, error, error) {
		eventID, err := appendHierarchyEvent(ctx, conn, task.Event{
			TaskID: parent.ID, Operation: task.OperationCreate,
			ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
			RecordedAt: now, PreviousRevision: parent.Revision, ResultingRevision: parent.Revision,
			Outcome: task.MutationRejected, DiagnosticCode: code,
		})
		if err != nil {
			return task.Task{}, nil, err
		}
		if err := linkTaskEventIdempotency(ctx, conn, eventID, identitySHA256); err != nil {
			return task.Task{}, nil, err
		}
		previous, resulting := parent.Revision, parent.Revision
		if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
			Operation: task.OperationCreate, TaskID: parent.ID, OutcomeCode: outcome,
			DiagnosticField: field, PreviousRevision: &previous, ResultingRevision: &resulting,
			FromStatus: parent.Status, ToStatus: to,
		}, now); err != nil {
			return task.Task{}, nil, err
		}
		return task.Task{}, cause, nil
	}
	if input.ExpectedParentRevision != parent.Revision {
		return reject(
			&task.ConflictError{ID: parent.ID, Expected: input.ExpectedParentRevision, Current: parent.Revision},
			task.DiagnosticRevisionConflict, mutationRevisionConflict, "", "",
		)
	}
	if parent.Status == task.StatusCompleted || parent.Status == task.StatusCancelled {
		return reject(
			&task.TransitionError{From: parent.Status, To: task.StatusOpen},
			task.DiagnosticInvalidTransition, mutationInvalidTransition, "parent_id", task.StatusOpen,
		)
	}

	order, err := nextSiblingOrder(ctx, conn, parent.ID)
	if err != nil {
		return task.Task{}, nil, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return task.Task{}, nil, fmt.Errorf("generate child Task ID: %w", err)
	}
	created := task.Task{
		ID: task.ID(id.String()), ParentID: parent.ID, RootID: parent.RootID, SiblingOrder: order,
		Scope: parent.Scope, Title: input.Title, Description: input.Description, Priority: input.Priority,
		DueDate: input.DueDate, Status: task.StatusOpen, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := insertTaskState(ctx, conn, created); err != nil {
		return task.Task{}, nil, err
	}
	if err := insertTaskHierarchy(ctx, conn, created); err != nil {
		return task.Task{}, nil, err
	}
	childEventID, err := appendTaskEvent(ctx, conn, task.Event{
		TaskID: created.ID, Operation: task.OperationCreate,
		ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
		RecordedAt: now, PreviousRevision: 0, ResultingRevision: 1, Outcome: task.MutationAccepted,
	})
	if err != nil {
		return task.Task{}, nil, err
	}
	if err := linkTaskEventIdempotency(ctx, conn, childEventID, identitySHA256); err != nil {
		return task.Task{}, nil, err
	}
	if err := insertTaskRevision(ctx, conn, created); err != nil {
		return task.Task{}, nil, err
	}
	if _, _, err := bumpTaskForHierarchy(
		ctx, conn, parent, task.OperationCreate, attribution, identitySHA256, now,
	); err != nil {
		return task.Task{}, nil, err
	}
	previous, resulting := uint64(0), uint64(1)
	if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
		Operation: task.OperationCreate, TaskID: created.ID, EventID: childEventID, OutcomeCode: mutationAccepted,
		PreviousRevision: &previous, ResultingRevision: &resulting,
	}, now); err != nil {
		return task.Task{}, nil, err
	}
	return created, nil, nil
}

func inputErrorField(err error) string {
	var inputErr *task.InputError
	if errors.As(err, &inputErr) {
		return inputErr.Field
	}
	return "mutation"
}

func (s *Store) ListOpenGlobalTasks(ctx context.Context) ([]task.Task, error) {
	return s.ListGlobalTasks(ctx, task.ListFilter{})
}

func (s *Store) ListGlobalTasks(ctx context.Context, filter task.ListFilter) ([]task.Task, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityList, task.AccessRead)
	access, err := taskAccessFromContext(ctx, s.db)
	if err != nil {
		return nil, err
	}
	statuses, err := listStatuses(filter)
	if err != nil {
		return nil, err
	}
	selectedScopes := access.scopes
	if filter.Scope != task.ScopeSelectionDefault {
		scope, err := access.selected(filter.Scope)
		if err != nil {
			return nil, err
		}
		selectedScopes = []task.Scope{scope}
	}
	if filter.Scope == task.ScopeSelectionDefault && filter.RootID == "" && filter.ParentID == "" {
		if focusID, found, err := s.focusedTaskID(ctx, access); err != nil {
			return nil, err
		} else if found {
			filter.FocusID = focusID
			if len(filter.Statuses) == 0 && !filter.IncludeHistory {
				statuses = []task.Status{task.StatusOpen, task.StatusInProgress, task.StatusBlocked}
			}
		}
	}
	placeholders := make([]string, len(statuses))
	scopeSQL, scopeArgs := scopePlaceholders(selectedScopes)
	arguments := make([]any, 0, len(statuses)+len(scopeArgs)+3)
	arguments = append(arguments, scopeArgs...)
	for i, status := range statuses {
		placeholders[i] = "?"
		arguments = append(arguments, status)
	}
	where := `t.scope IN (` + scopeSQL + `) AND t.status IN (` + strings.Join(placeholders, ",") + `)`
	order := `t.created_at, t.id`
	limit := ""
	if filter.RootID != "" {
		if access.delegated {
			if filter.RootID != access.grant.RootID {
				return []task.Task{}, nil
			}
		} else {
			where += ` AND h.root_id = ?`
			arguments = append(arguments, filter.RootID)
		}
		order = `tree.path, t.id`
	}
	if filter.ParentID != "" {
		where += ` AND h.parent_id = ?`
		arguments = append(arguments, filter.ParentID)
		if access.delegated {
			where += ` AND ? IN (
				WITH RECURSIVE granted_parent(task_id) AS (
					SELECT ?
					UNION ALL
					SELECT child.task_id FROM task_hierarchy child JOIN granted_parent ON child.parent_id = granted_parent.task_id
				)
				SELECT task_id FROM granted_parent
			)`
			arguments = append(arguments, filter.ParentID, access.grant.RootID)
		}
		order = `h.sibling_order, t.id`
	}
	if filter.FocusID != "" {
		where += ` AND t.id IN (
			WITH RECURSIVE focused(task_id) AS (
				SELECT ?
				UNION ALL
				SELECT child.task_id FROM task_hierarchy child JOIN focused ON child.parent_id = focused.task_id
			)
			SELECT task_id FROM focused
		)`
		arguments = append(arguments, filter.FocusID)
		order = `tree.path, t.id`
	}
	if access.delegated {
		where += ` AND t.id IN (
			WITH RECURSIVE granted(task_id) AS (
				SELECT ?
				UNION ALL
				SELECT child.task_id FROM task_hierarchy child JOIN granted ON child.parent_id = granted.task_id
			)
			SELECT task_id FROM granted
		)`
		arguments = append(arguments, access.grant.RootID)
		if filter.Limit == 0 || filter.Limit > task.MaxTreeNodes {
			filter.Limit = task.MaxTreeNodes
		}
	}
	activeGrantSQL, activeGrantArgs := access.activeGrantPredicate()
	where += activeGrantSQL
	arguments = append(arguments, activeGrantArgs...)
	if filter.Limit > 0 {
		limit = `LIMIT ?`
		arguments = append(arguments, filter.Limit)
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE tree(task_id, path) AS (
			SELECT task_id, task_id FROM task_hierarchy WHERE parent_id IS NULL
			UNION ALL
			SELECT child.task_id, tree.path || '/' || printf('%020d', child.sibling_order)
			FROM task_hierarchy child JOIN tree ON child.parent_id = tree.task_id
		)
		SELECT t.id, COALESCE(h.parent_id, ''), h.root_id, h.sibling_order,
		       t.scope, t.title, t.description, t.priority, t.due_date, t.result_summary,
		       t.status, t.revision, t.created_at, t.updated_at
		FROM tasks t
		JOIN task_hierarchy h ON h.task_id = t.id
		JOIN tree ON tree.task_id = t.id
		WHERE `+where+`
		ORDER BY `+order+` `+limit+`
	`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list global tasks: %w", err)
	}
	defer rows.Close()

	var values []task.Task
	for rows.Next() {
		value, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("list global tasks: %w", err)
		}
		values = append(values, access.project(value))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list global tasks: %w", err)
	}
	return values, nil
}

func listStatuses(filter task.ListFilter) ([]task.Status, error) {
	if len(filter.Statuses) == 0 {
		if filter.IncludeHistory {
			return []task.Status{task.StatusOpen, task.StatusInProgress, task.StatusBlocked, task.StatusCompleted, task.StatusCancelled}, nil
		}
		return []task.Status{task.StatusOpen}, nil
	}
	seen := make(map[task.Status]struct{}, len(filter.Statuses))
	statuses := make([]task.Status, 0, len(filter.Statuses))
	for _, status := range filter.Statuses {
		if !task.ValidStatus(status) {
			return nil, &task.InputError{Field: "statuses", Message: fmt.Sprintf("contains invalid status %q", status)}
		}
		if _, exists := seen[status]; exists {
			continue
		}
		seen[status] = struct{}{}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (s *Store) GetGlobalTask(ctx context.Context, id task.ID) (task.Task, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityGet, task.AccessRead)
	if strings.TrimSpace(string(id)) == "" {
		return task.Task{}, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	value, err := getGlobalTask(ctx, s.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, &task.NotFoundError{ID: id}
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("get global task: %w", err)
	}
	access, accessErr := taskAccessFromContext(ctx, s.db)
	if accessErr != nil {
		return task.Task{}, accessErr
	}
	return access.project(value), nil
}

func (s *Store) UpdateGlobalTask(ctx context.Context, id task.ID, input task.UpdateInput) (task.Task, error) {
	return s.updateGlobalTask(withTaskAuthorization(ctx, task.CapabilityUpdate, task.AccessContribute), id, input, "")
}

// ManagementUpdateGlobalTask is a Kernel-only compatibility and recovery seam.
// It is deliberately absent from task.Service so model-facing capabilities
// cannot request or forge an override. Every use is restricted to the primary
// owner and recorded in the append-only Task claim audit.
func (s *Store) ManagementUpdateGlobalTask(
	ctx context.Context,
	id task.ID,
	input task.UpdateInput,
	reason string,
) (task.Task, error) {
	if strings.TrimSpace(reason) == "" {
		return task.Task{}, &task.InputError{Field: "management_reason", Message: "must not be blank"}
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.Task{}, err
	}
	if attribution.ParentSessionID != "" {
		authorizedCtx := withTaskAuthorization(ctx, task.CapabilityUpdate, task.AccessManage)
		access, accessErr := taskAccessFromContext(authorizedCtx, s.db)
		if accessErr != nil {
			if errors.Is(accessErr, task.ErrAccessDenied) || errors.Is(accessErr, task.ErrScopeDenied) {
				return task.Task{}, task.ErrManagementOverrideDenied
			}
			return task.Task{}, accessErr
		}
		if !access.delegated || !grantAllows(access.grant.Level, task.AccessManage) {
			return task.Task{}, task.ErrManagementOverrideDenied
		}
	}
	return s.updateGlobalTask(withTaskAuthorization(ctx, task.CapabilityUpdate, task.AccessManage), id, input, reason)
}

func (s *Store) updateGlobalTask(ctx context.Context, id task.ID, input task.UpdateInput, managementReason string) (task.Task, error) {
	if err := ctx.Err(); err != nil {
		return task.Task{}, err
	}
	if strings.TrimSpace(string(id)) == "" {
		return task.Task{}, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if err := task.ValidateIdempotencyKey(input.IdempotencyKey); err != nil {
		return task.Task{}, err
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.Task{}, err
	}
	access, err := taskAccessFromContext(ctx, s.db)
	if err != nil {
		return task.Task{}, err
	}
	requestSHA256, err := canonicalUpdateRequestSHA256(id, input)
	if err != nil {
		return task.Task{}, err
	}
	identitySHA256 := idempotencySHA256(input.IdempotencyKey)
	var result task.Task
	var businessErr error
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		transactionAccess, err := taskAccessFromContext(ctx, conn)
		if err != nil {
			return err
		}
		if transactionAccess.delegated {
			if _, err := getGlobalTask(ctx, conn, id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return &task.NotFoundError{ID: id}
				}
				return err
			}
		}
		prior, found, err := readMutationResult(ctx, conn, attribution, identitySHA256)
		if err != nil {
			return err
		}
		if found {
			if prior.RequestSHA256 != requestSHA256 || prior.Operation != task.OperationUpdate {
				conflictTaskID := prior.TaskID
				if conflictTaskID == "" {
					conflictTaskID = id
				}
				if err := insertIdempotencyConflict(ctx, conn, attribution, identitySHA256, prior.RequestSHA256,
					requestSHA256, task.OperationUpdate, conflictTaskID, s.now().UTC()); err != nil {
					return err
				}
				businessErr = &task.IdempotencyConflictError{Operation: task.OperationUpdate}
				return nil
			}
			if prior.OutcomeCode == mutationAccepted && prior.ResultingRevision != nil {
				result, err = getTaskRevision(ctx, conn, prior.TaskID, *prior.ResultingRevision)
				return err
			}
			businessErr = replayUpdateError(id, input, prior)
			return nil
		}
		current, err := getGlobalTask(ctx, conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			now := s.now().UTC()
			if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
				Operation: task.OperationUpdate, TaskID: id, OutcomeCode: mutationNotFound,
			}, now); err != nil {
				return err
			}
			businessErr = &task.NotFoundError{ID: id}
			return nil
		}
		if err != nil {
			return fmt.Errorf("get global task for update: %w", err)
		}
		if transactionAccess.delegated && input.Status != nil &&
			(*input.Status == task.StatusCancelled ||
				(*input.Status == task.StatusOpen &&
					(current.Status == task.StatusCompleted || current.Status == task.StatusCancelled))) {
			manageCtx := withTaskAuthorization(ctx, task.CapabilityUpdate, task.AccessManage)
			if _, err := taskAccessFromContext(manageCtx, conn); err != nil {
				return err
			}
		}
		result = current
		reject := func(cause error, code task.DiagnosticCode, outcomeCode, field string, from, to task.Status) error {
			businessErr = cause
			now := s.now().UTC()
			eventID, err := appendTaskEvent(ctx, conn, task.Event{
				TaskID: id, Operation: task.OperationUpdate,
				ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
				RecordedAt: now, PreviousRevision: current.Revision, ResultingRevision: current.Revision,
				Outcome: task.MutationRejected, DiagnosticCode: code,
			})
			if err != nil {
				return err
			}
			if err := linkTaskEventIdempotency(ctx, conn, eventID, identitySHA256); err != nil {
				return err
			}
			previous, resulting := current.Revision, current.Revision
			return insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
				Operation: task.OperationUpdate, TaskID: id, EventID: eventID, OutcomeCode: outcomeCode,
				DiagnosticField: field, PreviousRevision: &previous, ResultingRevision: &resulting,
				FromStatus: from, ToStatus: to,
			}, now)
		}
		if err := task.ValidateUpdateInput(input); err != nil {
			return reject(err, task.DiagnosticInvalidInput, mutationInvalidInput, inputErrorField(err), current.Status, "")
		}
		if input.ExpectedRevision != current.Revision {
			return reject(
				&task.ConflictError{ID: id, Expected: input.ExpectedRevision, Current: current.Revision},
				task.DiagnosticRevisionConflict, mutationRevisionConflict, "", current.Status, "",
			)
		}
		activeClaim, activeClaimFound, claimErr := authorizeTaskUpdate(
			ctx, conn, current, input, attribution, s.now().UTC(), managementReason != "",
		)
		if claimErr != nil {
			field := "claim_required"
			diagnostic := task.DiagnosticClaimRequired
			switch {
			case errors.Is(claimErr, task.ErrClaimNotOwned):
				field = "claim_not_owned"
				diagnostic = task.DiagnosticClaimNotOwned
			case errors.Is(claimErr, task.ErrClaimExecutionInactive):
				field = "execution_inactive"
				diagnostic = task.DiagnosticExecutionInactive
			case errors.Is(claimErr, task.ErrMissingAttribution):
				field = "claim_attribution"
			}
			if err := reject(claimErr, task.DiagnosticInvalidTransition, mutationInvalidTransition,
				field, current.Status, statusPatchOrCurrent(input, current.Status)); err != nil {
				return err
			}
			_, err := appendClaimEvent(ctx, conn, task.Event{
				TaskID: id, Operation: task.OperationUpdate, ActorID: attribution.ActorID,
				SessionID: attribution.SessionID, RunID: attribution.RunID, RecordedAt: s.now().UTC(),
				PreviousRevision: current.Revision, ResultingRevision: current.Revision,
				Outcome: task.MutationRejected, DiagnosticCode: diagnostic, ClaimID: activeClaim.ID,
				IdempotencySHA256: identitySHA256,
			})
			return err
		}
		if input.Status != nil && *input.Status == task.StatusCompleted {
			active, err := countActiveDescendants(ctx, conn, current.ID)
			if err != nil {
				return err
			}
			if active > 0 {
				return reject(&task.ActiveDescendantsError{ID: current.ID}, task.DiagnosticInvalidTransition,
					mutationInvalidTransition, "active_descendants", current.Status, task.StatusCompleted)
			}
		}
		if input.Status != nil && *input.Status == task.StatusOpen &&
			(current.Status == task.StatusCompleted || current.Status == task.StatusCancelled) && current.ParentID != "" {
			blocked, err := hasTerminalAncestor(ctx, conn, current.ParentID)
			if err != nil {
				return err
			}
			if blocked {
				return reject(&task.InputError{Field: "status", Message: "reopen ancestors before descendants"},
					task.DiagnosticInvalidInput, mutationInvalidInput, "ancestor_status", current.Status, task.StatusOpen)
			}
		}
		if input.Status != nil {
			if *input.Status == current.Status && !hasTaskNonStatusPatch(input) {
				return reject(&task.TransitionError{From: current.Status, To: *input.Status},
					task.DiagnosticInvalidTransition, mutationInvalidTransition, "", current.Status, *input.Status)
			}
			if *input.Status != current.Status {
				if err := task.ValidateStatusTransition(current.Status, *input.Status); err != nil {
					return reject(err, task.DiagnosticInvalidTransition, mutationInvalidTransition, "", current.Status, *input.Status)
				}
			}
		}
		updated := applyTaskPatch(current, input)
		if taskStateEqual(current, updated) {
			return reject(&task.InputError{Field: "patch", Message: "must change task state"},
				task.DiagnosticInvalidInput, mutationInvalidInput, "patch", current.Status, "")
		}
		updated.Revision++
		updated.UpdatedAt = s.now().UTC()
		outcome, err := conn.ExecContext(ctx, `
			UPDATE tasks
			SET title = ?, description = NULLIF(?, ''), priority = NULLIF(?, 0), due_date = NULLIF(?, ''), result_summary = NULLIF(?, ''),
			    status = ?, revision = ?, updated_at = ?
			WHERE id = ? AND scope = ? AND revision = ?
		`, updated.Title, updated.Description, updated.Priority, updated.DueDate, updated.ResultSummary, updated.Status,
			updated.Revision, formatTaskTime(updated.UpdatedAt), id, current.Scope, current.Revision)
		if err != nil {
			return fmt.Errorf("update global task: %w", err)
		}
		rows, err := outcome.RowsAffected()
		if err != nil || rows != 1 {
			return fmt.Errorf("update global task revision predicate affected %d rows: %w", rows, err)
		}
		eventID, err := appendTaskEvent(ctx, conn, task.Event{
			TaskID: id, Operation: task.OperationUpdate,
			ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
			RecordedAt: updated.UpdatedAt, PreviousRevision: current.Revision, ResultingRevision: updated.Revision,
			Outcome: task.MutationAccepted,
		})
		if err != nil {
			return err
		}
		if err := linkTaskEventIdempotency(ctx, conn, eventID, identitySHA256); err != nil {
			return err
		}
		if err := insertTaskRevision(ctx, conn, updated); err != nil {
			return err
		}
		previous, resulting := current.Revision, updated.Revision
		if err := insertMutationResult(ctx, conn, attribution, identitySHA256, requestSHA256, mutationResult{
			Operation: task.OperationUpdate, TaskID: id, EventID: eventID, OutcomeCode: mutationAccepted,
			PreviousRevision: &previous, ResultingRevision: &resulting,
			FromStatus: current.Status, ToStatus: updated.Status,
		}, updated.UpdatedAt); err != nil {
			return err
		}
		if managementReason != "" {
			if _, err := appendClaimEvent(ctx, conn, task.Event{
				TaskID: id, Operation: task.OperationUpdate, ActorID: attribution.ActorID,
				SessionID: attribution.SessionID, RunID: attribution.RunID, RecordedAt: updated.UpdatedAt,
				PreviousRevision: updated.Revision, ResultingRevision: updated.Revision,
				Outcome: task.MutationAccepted, ClaimID: activeClaim.ID, ClaimReason: managementReason,
				ManagementOverride: true, ManagementReason: managementReason,
				IdempotencySHA256: identitySHA256,
			}); err != nil {
				return err
			}
		} else if taskUpdateNeedsClaim(current, input) {
			if _, err := appendClaimEvent(ctx, conn, task.Event{
				TaskID: id, Operation: task.OperationUpdate, ActorID: attribution.ActorID,
				SessionID: attribution.SessionID, RunID: attribution.RunID, RecordedAt: updated.UpdatedAt,
				PreviousRevision: updated.Revision, ResultingRevision: updated.Revision,
				Outcome: task.MutationAccepted, ClaimID: activeClaim.ID, ClaimReason: "authorized",
				IdempotencySHA256: identitySHA256,
			}); err != nil {
				return err
			}
		}
		if input.Status != nil && (*input.Status == task.StatusCompleted || *input.Status == task.StatusCancelled) && activeClaimFound {
			reason := "task_completed"
			if *input.Status == task.StatusCancelled {
				reason = "task_cancelled"
			}
			if _, _, err := releaseStoredTaskClaim(ctx, conn, activeClaim, task.Event{
				Operation: task.OperationRelease, ActorID: attribution.ActorID,
				SessionID: attribution.SessionID, RunID: attribution.RunID, RecordedAt: updated.UpdatedAt,
				ManagementReason: managementReason,
			}, reason, managementReason != ""); err != nil {
				return err
			}
		}
		result = updated
		return nil
	})
	if err != nil {
		return task.Task{}, err
	}
	if businessErr != nil {
		return task.Task{}, businessErr
	}
	return access.project(result), nil
}

func replayUpdateError(id task.ID, input task.UpdateInput, prior mutationResult) error {
	switch prior.OutcomeCode {
	case mutationInvalidInput:
		if err := task.ValidateUpdateInput(input); err != nil {
			return err
		}
		if prior.DiagnosticField == "patch" {
			return &task.InputError{Field: "patch", Message: "must change task state"}
		}
		if prior.DiagnosticField == "ancestor_status" {
			return &task.InputError{Field: "status", Message: "reopen ancestors before descendants"}
		}
		return &task.InputError{Field: prior.DiagnosticField, Message: "original mutation was rejected"}
	case mutationNotFound:
		return &task.NotFoundError{ID: id}
	case mutationRevisionConflict:
		current := uint64(0)
		if prior.ResultingRevision != nil {
			current = *prior.ResultingRevision
		}
		return &task.ConflictError{ID: id, Expected: input.ExpectedRevision, Current: current}
	case mutationInvalidTransition:
		if prior.DiagnosticField == "claim_attribution" {
			return task.ErrMissingAttribution
		}
		if prior.DiagnosticField == "claim_required" {
			return &task.ClaimRequiredError{TaskID: id}
		}
		if prior.DiagnosticField == "claim_not_owned" {
			return &task.ClaimNotOwnedError{TaskID: id}
		}
		if prior.DiagnosticField == "execution_inactive" {
			return &task.ClaimExecutionInactiveError{TaskID: id}
		}
		if prior.DiagnosticField == "active_descendants" {
			return &task.ActiveDescendantsError{ID: id}
		}
		return &task.TransitionError{From: prior.FromStatus, To: prior.ToStatus}
	default:
		return fmt.Errorf("replay Task update outcome %q", prior.OutcomeCode)
	}
}

func statusPatchOrCurrent(input task.UpdateInput, current task.Status) task.Status {
	if input.Status != nil {
		return *input.Status
	}
	return current
}

func countActiveDescendants(ctx context.Context, source queryRower, id task.ID) (uint64, error) {
	var count uint64
	err := source.QueryRowContext(ctx, `
		WITH RECURSIVE descendants(task_id) AS (
			SELECT task_id FROM task_hierarchy WHERE parent_id = ?
			UNION ALL
			SELECT child.task_id
			FROM task_hierarchy child JOIN descendants ON child.parent_id = descendants.task_id
		)
		SELECT COUNT(*) FROM descendants
		JOIN tasks ON tasks.id = descendants.task_id
		WHERE tasks.status IN ('open', 'in_progress', 'blocked')
	`, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active Task descendants: %w", err)
	}
	return count, nil
}

func hasTerminalAncestor(ctx context.Context, source queryRower, parentID task.ID) (bool, error) {
	var count uint64
	err := source.QueryRowContext(ctx, `
		WITH RECURSIVE ancestors(task_id, parent_id) AS (
			SELECT task_id, parent_id FROM task_hierarchy WHERE task_id = ?
			UNION ALL
			SELECT parent.task_id, parent.parent_id
			FROM task_hierarchy parent JOIN ancestors ON parent.task_id = ancestors.parent_id
		)
		SELECT COUNT(*) FROM ancestors
		JOIN tasks ON tasks.id = ancestors.task_id
		WHERE tasks.status IN ('completed', 'cancelled')
	`, parentID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check terminal Task ancestors: %w", err)
	}
	return count > 0, nil
}

func hasTaskNonStatusPatch(input task.UpdateInput) bool {
	return input.Title != nil || input.Description != nil || input.Priority != nil || input.DueDate != nil ||
		input.ResultSummary != nil
}

func applyTaskPatch(current task.Task, input task.UpdateInput) task.Task {
	updated := current
	if input.Title != nil {
		updated.Title = *input.Title
	}
	if input.Description != nil {
		updated.Description = *input.Description
	}
	if input.Priority != nil {
		updated.Priority = *input.Priority
	}
	if input.DueDate != nil {
		updated.DueDate = *input.DueDate
	}
	if input.ResultSummary != nil {
		updated.ResultSummary = *input.ResultSummary
	}
	if input.Status != nil {
		updated.Status = *input.Status
	}
	return updated
}

func taskStateEqual(left, right task.Task) bool {
	left.Revision, right.Revision = 0, 0
	left.UpdatedAt, right.UpdatedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(left, right)
}

func (s *Store) ListTaskEvents(ctx context.Context, id task.ID) ([]task.Event, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityGet, task.AccessRead)
	if strings.TrimSpace(string(id)) == "" {
		return nil, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if _, err := s.GetGlobalTask(ctx, id); err != nil {
		return nil, err
	}
	access, err := taskAccessFromContext(ctx, s.db)
	if err != nil {
		return nil, err
	}
	activeGrantSQL, activeGrantArgs := access.activeGrantPredicate()
	arguments := []any{id}
	arguments = append(arguments, activeGrantArgs...)
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.task_id, e.sequence, e.operation, e.actor_id, e.session_id, e.run_id, e.recorded_at,
		       e.previous_revision, e.resulting_revision, e.outcome, e.diagnostic_code,
		       COALESCE(e.identity_sha256, i.identity_sha256, ''), e.claim_id,
		       e.claim_reason, COALESCE(g.grant_id, ''), e.management_override, e.management_reason
		FROM (
			SELECT id, task_id, sequence, operation, actor_id, session_id, run_id, recorded_at,
			       previous_revision, resulting_revision, outcome, diagnostic_code,
			       NULL AS identity_sha256, '' AS claim_id, '' AS claim_reason, 0 AS management_override,
			       '' AS management_reason
			FROM task_events
			UNION ALL
			SELECT id, task_id, sequence, operation, actor_id, session_id, run_id, recorded_at,
			       previous_revision, resulting_revision, outcome, diagnostic_code,
			       NULL AS identity_sha256, '' AS claim_id, '' AS claim_reason, 0 AS management_override,
			       '' AS management_reason
			FROM task_hierarchy_events
			UNION ALL
			SELECT id, task_id, sequence, operation, actor_id, session_id, run_id, recorded_at,
			       previous_revision, resulting_revision, outcome, diagnostic_code,
			       identity_sha256, COALESCE(claim_id, ''), COALESCE(claim_reason, ''), management_override,
			       COALESCE(management_reason, '')
			FROM task_claim_events
		) e
		LEFT JOIN task_event_idempotency i ON i.event_id = e.id
		LEFT JOIN task_event_grants g ON g.event_id = e.id
		WHERE e.task_id = ?`+activeGrantSQL+`
		ORDER BY e.sequence
	`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	defer rows.Close()
	var events []task.Event
	for rows.Next() {
		var event task.Event
		var recorded string
		var diagnostic sql.NullString
		if err := rows.Scan(&event.ID, &event.TaskID, &event.Sequence, &event.Operation, &event.ActorID, &event.SessionID,
			&event.RunID, &recorded, &event.PreviousRevision, &event.ResultingRevision, &event.Outcome, &diagnostic,
			&event.IdempotencySHA256, &event.ClaimID, &event.ClaimReason, &event.GrantID, &event.ManagementOverride,
			&event.ManagementReason); err != nil {
			return nil, fmt.Errorf("list task events: %w", err)
		}
		event.DiagnosticCode = task.DiagnosticCode(diagnostic.String)
		event.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded)
		if err != nil {
			return nil, fmt.Errorf("parse task event recorded_at: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	return events, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getGlobalTask(ctx context.Context, source queryRower, id task.ID) (task.Task, error) {
	access, err := taskAccessFromContext(ctx, source)
	if err != nil {
		return task.Task{}, err
	}
	scopeSQL, scopeArgs := scopePlaceholders(access.scopes)
	args := []any{id}
	args = append(args, scopeArgs...)
	grantSQL := ""
	if access.delegated {
		grantSQL = ` AND t.id IN (
			WITH RECURSIVE granted(task_id) AS (
				SELECT ?
				UNION ALL
				SELECT child.task_id FROM task_hierarchy child JOIN granted ON child.parent_id = granted.task_id
			)
			SELECT task_id FROM granted
		)`
		args = append(args, access.grant.RootID)
	}
	activeGrantSQL, activeGrantArgs := access.activeGrantPredicate()
	grantSQL += activeGrantSQL
	args = append(args, activeGrantArgs...)
	return scanTask(source.QueryRowContext(ctx, `
		SELECT t.id, COALESCE(h.parent_id, ''), h.root_id, h.sibling_order,
		       t.scope, t.title, t.description, t.priority, t.due_date, t.result_summary,
		       t.status, t.revision, t.created_at, t.updated_at
		FROM tasks t
		JOIN task_hierarchy h ON h.task_id = t.id
		WHERE t.id = ? AND t.scope IN (`+scopeSQL+`)`+grantSQL+`
	`, args...))
}

func insertTaskState(ctx context.Context, conn *sql.Conn, value task.Task) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO tasks (
			id, scope, title, description, priority, due_date, result_summary,
			status, revision, created_at, updated_at
		) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)
	`, value.ID, value.Scope, value.Title, value.Description, value.Priority, value.DueDate,
		value.ResultSummary, value.Status, value.Revision, formatTaskTime(value.CreatedAt), formatTaskTime(value.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert Task state: %w", err)
	}
	return nil
}

func insertTaskHierarchy(ctx context.Context, conn *sql.Conn, value task.Task) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_hierarchy (task_id, parent_id, root_id, sibling_order)
		VALUES (?, NULLIF(?, ''), ?, ?)
	`, value.ID, value.ParentID, value.RootID, value.SiblingOrder)
	if err != nil {
		return fmt.Errorf("insert Task hierarchy: %w", err)
	}
	return nil
}

func nextSiblingOrder(ctx context.Context, conn *sql.Conn, parentID task.ID) (uint64, error) {
	var order uint64
	if err := conn.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sibling_order), 0) + 1 FROM task_hierarchy WHERE parent_id = ?
	`, parentID).Scan(&order); err != nil {
		return 0, fmt.Errorf("allocate Task sibling order: %w", err)
	}
	return order, nil
}

func bumpTaskForHierarchy(
	ctx context.Context,
	conn *sql.Conn,
	current task.Task,
	operation task.Operation,
	attribution task.MutationAttribution,
	identitySHA256 string,
	now time.Time,
) (task.Task, string, error) {
	updated := current
	updated.Revision++
	updated.UpdatedAt = now
	result, err := conn.ExecContext(ctx, `
		UPDATE tasks SET revision = ?, updated_at = ? WHERE id = ? AND revision = ?
	`, updated.Revision, formatTaskTime(updated.UpdatedAt), current.ID, current.Revision)
	if err != nil {
		return task.Task{}, "", fmt.Errorf("update Task hierarchy revision: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return task.Task{}, "", fmt.Errorf("update Task hierarchy revision predicate affected %d rows: %w", rows, err)
	}
	eventID, err := appendHierarchyEvent(ctx, conn, task.Event{
		TaskID: current.ID, Operation: operation,
		ActorID: attribution.ActorID, SessionID: attribution.SessionID, RunID: attribution.RunID,
		RecordedAt: now, PreviousRevision: current.Revision, ResultingRevision: updated.Revision,
		Outcome: task.MutationAccepted,
	})
	if err != nil {
		return task.Task{}, "", err
	}
	if err := linkTaskEventIdempotency(ctx, conn, eventID, identitySHA256); err != nil {
		return task.Task{}, "", err
	}
	if err := insertTaskRevision(ctx, conn, updated); err != nil {
		return task.Task{}, "", err
	}
	return updated, eventID, nil
}

func appendHierarchyEvent(ctx context.Context, conn *sql.Conn, event task.Event) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate Task hierarchy event ID: %w", err)
	}
	event.ID = id.String()
	if err := conn.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1 FROM (
			SELECT sequence FROM task_events WHERE task_id = ?
			UNION ALL
			SELECT sequence FROM task_hierarchy_events WHERE task_id = ?
			UNION ALL
			SELECT sequence FROM task_claim_events WHERE task_id = ?
		)
	`, event.TaskID, event.TaskID, event.TaskID).Scan(&event.Sequence); err != nil {
		return "", fmt.Errorf("allocate Task hierarchy event sequence: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO task_hierarchy_events (
			id, task_id, sequence, operation, actor_id, session_id, run_id, recorded_at,
			previous_revision, resulting_revision, outcome, diagnostic_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
	`, event.ID, event.TaskID, event.Sequence, event.Operation, event.ActorID, event.SessionID, event.RunID,
		formatTaskTime(event.RecordedAt), event.PreviousRevision, event.ResultingRevision,
		event.Outcome, event.DiagnosticCode); err != nil {
		return "", fmt.Errorf("append Task hierarchy event: %w", err)
	}
	if err := linkActiveTaskGrant(ctx, conn, event.ID, event.SessionID); err != nil {
		return "", err
	}
	return event.ID, nil
}

func appendTaskEvent(ctx context.Context, conn *sql.Conn, event task.Event) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate task event ID: %w", err)
	}
	event.ID = id.String()
	if err := conn.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1 FROM (
			SELECT sequence FROM task_events WHERE task_id = ?
			UNION ALL
			SELECT sequence FROM task_hierarchy_events WHERE task_id = ?
			UNION ALL
			SELECT sequence FROM task_claim_events WHERE task_id = ?
		)
	`, event.TaskID, event.TaskID, event.TaskID,
	).Scan(&event.Sequence); err != nil {
		return "", fmt.Errorf("allocate task event sequence: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO task_events (
			id, task_id, sequence, operation, actor_id, session_id, run_id, recorded_at,
			previous_revision, resulting_revision, outcome, diagnostic_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))
	`, event.ID, event.TaskID, event.Sequence, event.Operation, event.ActorID, event.SessionID, event.RunID,
		formatTaskTime(event.RecordedAt), event.PreviousRevision, event.ResultingRevision, event.Outcome, event.DiagnosticCode); err != nil {
		return "", fmt.Errorf("append task event: %w", err)
	}
	if err := linkActiveTaskGrant(ctx, conn, event.ID, event.SessionID); err != nil {
		return "", err
	}
	return event.ID, nil
}

func linkActiveTaskGrant(ctx context.Context, conn *sql.Conn, eventID, sessionID string) error {
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO task_event_grants (event_id, grant_id)
		SELECT ?, id FROM task_access_grants
		WHERE grantee_session_id = ? AND ended_at IS NULL
	`, eventID, sessionID); err != nil {
		return fmt.Errorf("link Task event to access grant: %w", err)
	}
	return nil
}

func scanTask(scanner rowScanner) (task.Task, error) {
	var (
		value                               task.Task
		description, dueDate, resultSummary sql.NullString
		priority                            sql.NullInt64
		createdText, updatedText            string
	)
	if err := scanner.Scan(&value.ID, &value.ParentID, &value.RootID, &value.SiblingOrder,
		&value.Scope, &value.Title, &description, &priority, &dueDate, &resultSummary,
		&value.Status, &value.Revision, &createdText, &updatedText); err != nil {
		return task.Task{}, err
	}
	value.Description = description.String
	value.Priority = int(priority.Int64)
	value.DueDate = dueDate.String
	value.ResultSummary = resultSummary.String
	var err error
	value.CreatedAt, err = time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return task.Task{}, fmt.Errorf("parse task created_at: %w", err)
	}
	value.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedText)
	if err != nil {
		return task.Task{}, fmt.Errorf("parse task updated_at: %w", err)
	}
	return value, nil
}

func formatTaskTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
