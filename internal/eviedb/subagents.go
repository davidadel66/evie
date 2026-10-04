package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

// subagentExecutionsTable is the attempt table. The 2026-10-01 amendments add
// the terminal state 'partial' for a child that wrapped up at its budget, and
// let one child session hold several attempts: its original assignment and
// each continuation, of which at most one is unfinished.
const subagentExecutionsTable = `CREATE TABLE IF NOT EXISTS subagent_executions (
 id TEXT PRIMARY KEY, parent_session_id TEXT NOT NULL REFERENCES sessions(id),
 idempotency_key TEXT NOT NULL, request_digest TEXT NOT NULL,
 child_session_id TEXT NOT NULL REFERENCES sessions(id),
 state TEXT NOT NULL CHECK(state IN ('admitted','running','succeeded','partial','failed','cancelled','interrupted')),
 record_json TEXT NOT NULL, UNIQUE(parent_session_id,idempotency_key)
 )`

const subagentExecutionsIndexes = `CREATE INDEX IF NOT EXISTS subagent_execution_state ON subagent_executions(state,parent_session_id);
 CREATE INDEX IF NOT EXISTS subagent_execution_child ON subagent_executions(child_session_id);
 CREATE UNIQUE INDEX IF NOT EXISTS subagent_execution_live_child ON subagent_executions(child_session_id) WHERE state IN ('admitted','running');`

// Shapes of earlier tables: before continuation each child session had
// exactly one attempt, and before 'partial' the state check was narrower.
const (
	continuableChildColumn   = "child_session_id TEXT NOT NULL REFERENCES sessions(id)"
	singleAttemptChildColumn = "child_session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id)"
	preBudgetSubagentStates  = `CHECK(state IN ('admitted','running','succeeded','failed','cancelled','interrupted'))`
)

// ensureSubagentSchema creates the attempt table or rebuilds an earlier one.
// SQLite cannot drop a column's UNIQUE or alter a CHECK, so an earlier table
// is rebuilt once with every row and rowid copied unchanged. The shape is
// re-read under the write lock so competing startups migrate once; an
// unknown shape fails closed.
func ensureSubagentSchema(ctx context.Context, db *sql.DB) error {
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		var definition string
		err := conn.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='subagent_executions'`).Scan(&definition)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err = conn.ExecContext(ctx, subagentExecutionsTable); err != nil {
				return err
			}
			_, err = conn.ExecContext(ctx, subagentExecutionsIndexes)
			return err
		}
		if err != nil {
			return err
		}
		if strings.Contains(definition, "'partial'") && strings.Contains(definition, continuableChildColumn) {
			_, err = conn.ExecContext(ctx, subagentExecutionsIndexes)
			return err
		}
		if !strings.Contains(definition, singleAttemptChildColumn) ||
			(!strings.Contains(definition, "'partial'") && !strings.Contains(definition, preBudgetSubagentStates)) {
			return errors.New("unsupported subagent execution table schema")
		}
		var references int
		if err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master AS m JOIN pragma_foreign_key_list(m.name) AS f WHERE m.type='table' AND f."table"='subagent_executions'`).Scan(&references); err != nil {
			return err
		}
		if references != 0 {
			return errors.New("unsupported subagent execution foreign reference")
		}
		replacement := strings.Replace(subagentExecutionsTable, "IF NOT EXISTS subagent_executions", "subagent_executions_continuable", 1)
		if _, err = conn.ExecContext(ctx, replacement); err != nil {
			return fmt.Errorf("create subagent execution table: %w", err)
		}
		if _, err = conn.ExecContext(ctx, `INSERT INTO subagent_executions_continuable(rowid,id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json)
 SELECT rowid,id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json FROM subagent_executions;
 DROP TABLE subagent_executions;
 ALTER TABLE subagent_executions_continuable RENAME TO subagent_executions;`); err != nil {
			return fmt.Errorf("migrate subagent executions: %w", err)
		}
		_, err = conn.ExecContext(ctx, subagentExecutionsIndexes)
		return err
	})
}

// AdmitSubagents validates the entire batch and atomically reserves every new
// child, receipt and attempt. Retained keys never create runnable work again.
func (s *Store) AdmitSubagents(ctx context.Context, p delegation.Parent, requests []delegation.Assignment, receipt composition.Receipt, policy delegation.Policy, dispatchID string) ([]delegation.Attempt, error) {
	if receipt.Preset.ID != delegation.WorkerResearch {
		return nil, errors.New("only the restricted research preset is allowed")
	}
	return s.admitSubagents(ctx, p, requests, admission{}, receipt, policy, dispatchID)
}

// AdmitExploreSubagents admits a delegate_explore batch like AdmitSubagents.
// roots[i] is the folder the harness resolved for requests[i]; each must be
// inside the parent's current Workspace folder or project root, and is pinned
// on the attempt (Stage 7, 2026-10-03).
func (s *Store) AdmitExploreSubagents(ctx context.Context, p delegation.Parent, requests []delegation.Assignment, roots []string, receipt composition.Receipt, policy delegation.Policy, dispatchID string) ([]delegation.Attempt, error) {
	if receipt.Preset.ID != delegation.WorkerExplore {
		return nil, errors.New("only the read-only explore preset is allowed")
	}
	if len(roots) != len(requests) {
		return nil, errors.New("every explore assignment needs a resolved root")
	}
	return s.admitSubagents(ctx, p, requests, admission{roots: roots}, receipt, policy, dispatchID)
}

// admission is a batch's worker-specific harness inputs: explore roots, or
// build worktree plans and why a plan could not be made.
type admission struct {
	roots    []string
	plans    []delegation.Worktree
	planErrs []error
}

func (s *Store) admitSubagents(ctx context.Context, p delegation.Parent, requests []delegation.Assignment, extra admission, receipt composition.Receipt, policy delegation.Policy, dispatchID string) ([]delegation.Attempt, error) {
	if dispatchID == "" {
		return nil, errors.New("subagent dispatch identity is required")
	}
	worker := receipt.Preset.ID
	if err := policy.ValidateWorkerBatch(worker, requests); err != nil {
		return nil, err
	}
	encoded, err := composition.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	var attempts []delegation.Attempt
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := s.authorizeSubagentParent(ctx, conn, p, receipt); err != nil {
			return err
		}
		intent, err := committedSubagentIntent(ctx, conn, p, delegation.DelegationToolName(worker))
		if err != nil {
			return err
		}
		var committed struct {
			Assignments []delegation.Assignment `json:"assignments"`
		}
		if err := json.Unmarshal([]byte(intent.Call.Arguments), &committed); err != nil {
			return err
		}
		if delegation.Digest(committed.Assignments) != delegation.Digest(requests) {
			return errors.New("assignments do not match committed invocation")
		}

		retained := make([]*delegation.Attempt, len(requests))
		fresh := 0
		var conflicts []string
		for i, r := range requests {
			if err := authorizeSubagentTask(ctx, conn, p, r.TaskID); err != nil {
				return err
			}
			a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE parent_session_id=? AND idempotency_key=?`, p.Scope.SessionID, r.Key))
			if err == nil {
				// A key names one assignment of one worker: reusing a
				// research key for explore work, or the reverse, conflicts.
				if a.Digest != delegation.Digest(r) || a.Worker() != worker {
					conflicts = append(conflicts, fmt.Sprintf("%q", r.Key))
				}
				retained[i] = &a
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			fresh++
		}
		if len(conflicts) > 0 {
			// The whole batch is refused, so name every key that caused it.
			return fmt.Errorf("%w: idempotency_key %s already ran in this conversation with a different objective, context, task_id, path or worker; reuse a key only to retrieve that result, and give a new assignment a new key", delegation.ErrConflict, strings.Join(conflicts, ", "))
		}
		if fresh > 0 {
			if err := admitWithinTurnLimit(ctx, conn, p, policy, fresh); err != nil {
				return err
			}
		}
		for i, r := range requests {
			if retained[i] != nil {
				attempts = append(attempts, *retained[i])
				continue
			}
			var root string
			var worktree *delegation.Worktree
			id := uuid.NewString()
			now := s.now().UTC()
			switch worker {
			case delegation.WorkerExplore:
				root = extra.roots[i]
				if err := authorizeExploreRoot(ctx, conn, p.Scope, root); err != nil {
					return fmt.Errorf("assignment %q: %w", r.Key, err)
				}
			case delegation.WorkerBuild:
				// The harness planned the worktree before admission; the
				// attempt takes its identity from it.
				if extra.planErrs[i] != nil {
					return fmt.Errorf("assignment %q: %w", r.Key, extra.planErrs[i])
				}
				plan := extra.plans[i]
				if err := authorizeBuildWorktree(ctx, conn, p.Scope, plan); err != nil {
					return fmt.Errorf("assignment %q: %w", r.Key, err)
				}
				if slices.ContainsFunc(requests[:i], func(other delegation.Assignment) bool { return other.TaskID == r.TaskID }) {
					return fmt.Errorf("assignment %q: Task %s is already assigned to another build worker in this batch", r.Key, r.TaskID)
				}
				if err := prepareBuildTask(ctx, conn, r.TaskID, now); err != nil {
					return fmt.Errorf("assignment %q: %w", r.Key, err)
				}
				id, worktree = filepath.Base(plan.Path), &plan
			}
			child := memory.Session{ID: memory.SessionID(uuid.NewString()), ParentSessionID: p.Scope.SessionID, Status: memory.SessionActive, CreatedAt: now, UpdatedAt: now,
				WorkspaceID: p.Scope.WorkspaceID, WorkspaceRevisionSnapshot: p.Scope.WorkspaceRevision, ProjectID: p.Scope.ProjectID, ProjectRootSnapshot: p.Scope.ProjectRoot}
			if _, err := conn.ExecContext(ctx, `INSERT INTO sessions(id,workspace_id,workspace_revision_snapshot,project_id,project_root_snapshot,parent_session_id,status,created_at,updated_at)
    VALUES (?,NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),?,?,?,?)`, child.ID, child.WorkspaceID, child.WorkspaceRevisionSnapshot, child.ProjectID, child.ProjectRootSnapshot, child.ParentSessionID, child.Status, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				return err
			}
			if err := insertCompositionReceipt(ctx, conn, child.ID, encoded, now); err != nil {
				return err
			}
			a := delegation.Attempt{ID: id, DispatchID: dispatchID, Parent: p, Assignment: r, Digest: delegation.Digest(r), Child: child, Receipt: receipt, Policy: policy, PolicyID: delegation.Digest(policy), State: "admitted", CreatedAt: now, Root: root, Worktree: worktree}
			data, err := json.Marshal(a)
			if err != nil {
				return err
			}
			if _, err = conn.ExecContext(ctx, `INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`, a.ID, p.Scope.SessionID, r.Key, a.Digest, child.ID, a.State, string(data)); err != nil {
				return err
			}
			if worker == delegation.WorkerBuild {
				// The worker's authority over its Task: that Task's subtree,
				// for this attempt only (Stage 11).
				if err := issueBuildGrant(ctx, conn, p, child.ID, r.TaskID, a.ID, now); err != nil {
					return err
				}
			}
			attempts = append(attempts, a)
		}
		return s.authorizeSubagentParent(ctx, conn, p, receipt)
	})
	return attempts, err
}

// admitWithinTurnLimit refuses fresh attempts beyond the per-turn limit.
// Every attempt admitted for this parent turn counts, whichever delegation or
// continuation call admitted it and however it ended. An unreadable record
// proves no turn and never blocks admission.
func admitWithinTurnLimit(ctx context.Context, conn *sql.Conn, p delegation.Parent, policy delegation.Policy, fresh int) error {
	var started int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM subagent_executions WHERE parent_session_id=?
 AND CASE WHEN json_valid(record_json) THEN json_extract(record_json,'$.parent.SourceEventID') END=?`, p.Scope.SessionID, p.SourceEventID).Scan(&started); err != nil {
		return err
	}
	if started+fresh > policy.PerTurn {
		return &delegation.TurnLimitError{Limit: policy.PerTurn, Started: started, Requested: fresh}
	}
	return nil
}

// committedSubagentIntent reads the parent's committed tool intent, which
// authorizeSubagentParent proved outstanding in the current turn, and
// requires it to be a call of tool.
func committedSubagentIntent(ctx context.Context, conn *sql.Conn, p delegation.Parent, tool string) (memory.ToolIntentPayload, error) {
	var payload string
	var intent memory.ToolIntentPayload
	if err := conn.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE id=? AND session_id=?`, p.IntentEventID, p.Scope.SessionID).Scan(&payload); err != nil {
		return intent, err
	}
	if err := json.Unmarshal([]byte(payload), &intent); err != nil {
		return intent, err
	}
	if intent.Call.Name != tool {
		return intent, fmt.Errorf("%w: the committed invocation is not %s", delegation.ErrAuthority, tool)
	}
	return intent, nil
}

// AdmitSubagentContinuation admits a continuation of one of p's finished
// attempts as a new attempt on the same child session. It is authorized like
// a fresh delegation, by the current parent turn's live lease and committed
// continue_research intent, never by the authority of the attempt it extends,
// and counts toward the current turn's limit. A retry of the same intent
// returns the attempt it admitted. Only the child's latest report can be
// extended, and only while none of its attempts is unfinished.
func (s *Store) AdmitSubagentContinuation(ctx context.Context, p delegation.Parent, executionID, message string, policy delegation.Policy, dispatchID string) (delegation.Attempt, error) {
	if dispatchID == "" {
		return delegation.Attempt{}, errors.New("subagent dispatch identity is required")
	}
	if err := policy.ValidateContinuation(executionID, message); err != nil {
		return delegation.Attempt{}, err
	}
	request := delegation.Continuation{ExecutionID: executionID, Message: message}
	digest := delegation.Digest(request)
	key := delegation.ContinuationKey(p.IntentEventID)
	var a delegation.Attempt
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		retained, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE parent_session_id=? AND idempotency_key=?`, p.Scope.SessionID, key))
		if err == nil {
			if retained.Continues == nil || retained.Continues.ExecutionID != executionID || retained.Digest != digest {
				return fmt.Errorf("%w: this continue_research call (key %q) already admitted execution %q with different arguments", delegation.ErrConflict, key, retained.ID)
			}
			if err = s.authorizeSubagentParent(ctx, conn, p, retained.Receipt); err != nil {
				return err
			}
			if err = authorizeAttemptRoot(ctx, conn, retained); err != nil {
				return err
			}
			if _, err = s.inspectSubagent(ctx, conn, p, retained.ID); err != nil {
				return err
			}
			a = retained
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		original, err := s.inspectSubagent(ctx, conn, p, executionID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", delegation.ErrNotFound, executionID)
		}
		if err != nil {
			return err
		}
		if err = s.authorizeSubagentParent(ctx, conn, p, original.Receipt); err != nil {
			return err
		}
		// An explore child resumes in its pinned folder, which must still be
		// inside the parent's current folder.
		if err = authorizeAttemptRoot(ctx, conn, original); err != nil {
			return err
		}
		intent, err := committedSubagentIntent(ctx, conn, p, delegation.ContinueToolName)
		if err != nil {
			return err
		}
		var committed delegation.Continuation
		if err = json.Unmarshal([]byte(intent.Call.Arguments), &committed); err != nil || committed != request {
			return errors.New("continuation does not match the committed invocation")
		}
		if err = resumableSubagent(ctx, conn, original); err != nil {
			return err
		}
		if err = admitWithinTurnLimit(ctx, conn, p, policy, 1); err != nil {
			return err
		}
		var after int64
		if err = conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM events WHERE session_id=?`, original.Child.ID).Scan(&after); err != nil {
			return err
		}
		now := s.now().UTC()
		assignment := delegation.Assignment{Key: key, Objective: message, TaskID: original.Assignment.TaskID}
		a = delegation.Attempt{ID: uuid.NewString(), DispatchID: dispatchID, Parent: p, Assignment: assignment, Digest: digest, Child: original.Child, Receipt: original.Receipt,
			Policy: policy, PolicyID: delegation.Digest(policy), State: "admitted", CreatedAt: now, Continues: &delegation.Continues{ExecutionID: original.ID, AfterSequence: after}, Root: original.Root,
			Worktree: original.Worktree}
		data, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if _, err = conn.ExecContext(ctx, `INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`, a.ID, p.Scope.SessionID, key, digest, a.Child.ID, a.State, string(data)); err != nil {
			return err
		}
		if a.Worker() == delegation.WorkerBuild {
			// A build child continues in the same worktree, on the same Task,
			// under a grant for this attempt only.
			if err = issueBuildGrant(ctx, conn, p, a.Child.ID, a.Assignment.TaskID, a.ID, now); err != nil {
				return err
			}
		}
		return s.authorizeSubagentParent(ctx, conn, p, original.Receipt)
	})
	return a, err
}

// resumableSubagent refuses to continue an attempt unless it ended with an
// accepted report, its child has no unfinished attempt, and no later attempt
// of the child has a newer report: a continuation always resumes the child's
// whole history, so it extends the child's latest report.
func resumableSubagent(ctx context.Context, conn *sql.Conn, a delegation.Attempt) error {
	var live, liveState string
	err := conn.QueryRowContext(ctx, `SELECT id,state FROM subagent_executions WHERE child_session_id=? AND state IN ('admitted','running')`, a.Child.ID).Scan(&live, &liveState)
	if err == nil {
		return fmt.Errorf("%w: its child is still %s as execution %q; wait for that result", delegation.ErrNotResumable, liveState, live)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var latest string
	err = conn.QueryRowContext(ctx, `SELECT id FROM subagent_executions WHERE child_session_id=? AND state IN ('succeeded','partial')
 AND CASE WHEN json_valid(record_json) THEN COALESCE(json_extract(record_json,'$.final_event_id'),'') END<>'' ORDER BY rowid DESC LIMIT 1`, a.Child.ID).Scan(&latest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	switch {
	case a.Resumable() && latest == a.ID:
		return nil
	case latest == "":
		return fmt.Errorf("%w: execution %q ended %s without a report to continue from; start a new attempt with a new idempotency key", delegation.ErrNotResumable, a.ID, a.State)
	case !a.Resumable():
		return fmt.Errorf("%w: execution %q ended %s without a report; continue execution %q, the child's latest report", delegation.ErrNotResumable, a.ID, a.State, latest)
	}
	return fmt.Errorf("%w: execution %q was already continued; continue execution %q, the child's latest report", delegation.ErrNotResumable, a.ID, latest)
}

func readSubagent(row rowScanner) (delegation.Attempt, error) {
	var data string
	var a delegation.Attempt
	if err := row.Scan(&data); err != nil {
		return a, err
	}
	err := json.Unmarshal([]byte(data), &a)
	return a, err
}
func writeSubagent(ctx context.Context, conn *sql.Conn, a delegation.Attempt) error {
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `UPDATE subagent_executions SET state=?,record_json=? WHERE id=?`, a.State, string(b), a.ID)
	return err
}

// subagentLeaseFence proves the parent's turn lease inside a transaction.
// Mutations use fenceTurnLeaseWrite under BEGIN IMMEDIATE; read-only checks
// use checkTurnLeaseLive on their snapshot and never take the write lock.
type subagentLeaseFence func(context.Context, *sql.Conn, memory.SessionID, memory.LeaseHolderID, memory.FencingToken, string) error

func (s *Store) authorizeSubagentParent(ctx context.Context, conn *sql.Conn, p delegation.Parent, child composition.Receipt) error {
	return s.authorizeSubagentParentWith(ctx, conn, p, child, fenceTurnLeaseWrite)
}

func (s *Store) authorizeSubagentParentWith(ctx context.Context, conn *sql.Conn, p delegation.Parent, child composition.Receipt, fence subagentLeaseFence) error {
	if p.Scope.ParentSessionID != "" || p.Scope.SessionID != p.Lease.SessionID || p.Scope.OwnerID != memory.LocalOwnerID || p.Lease.Generation != memory.LeaseGeneration(p.Lease.FencingToken) {
		return delegation.ErrAuthority
	}
	if err := validateSessionScope(ctx, conn, p.Scope); err != nil {
		return err
	}
	var ancestor sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT parent_session_id FROM sessions WHERE id=?`, p.Scope.SessionID).Scan(&ancestor); err != nil {
		return err
	}
	if ancestor.Valid {
		return errors.New("nested delegation is unavailable")
	}
	var root, revision sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT project_root_snapshot,workspace_revision_snapshot FROM sessions WHERE id=?`, p.Scope.SessionID).Scan(&root, &revision); err != nil {
		return err
	}
	if root.String != p.Scope.ProjectRoot || revision.String != string(p.Scope.WorkspaceRevision) {
		return delegation.ErrAuthority
	}
	if p.Scope.ProjectID != "" {
		var archived bool
		if err := conn.QueryRowContext(ctx, `SELECT archived FROM projects WHERE id=?`, p.Scope.ProjectID).Scan(&archived); err != nil {
			return err
		}
		if archived {
			return ErrProjectNotActive
		}
	}

	worker := child.Preset.ID
	if worker != delegation.WorkerResearch && worker != delegation.WorkerExplore && worker != delegation.WorkerBuild {
		return errors.New("only the restricted research, read-only explore and fenced build presets are allowed")
	}
	if p.Scope.WorkspaceID != "" {
		if err := authorizeWorkspacePreset(ctx, conn, p.Scope, worker); err != nil {
			return err
		}
	}
	if err := fence(ctx, conn, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, s.now().UTC().Format(turnLeaseTimeFormat)); err != nil {
		return err
	}
	// An explore worker uses no Web capability, so only Subagents must stay
	// enabled for it; a build worker needs Todo for its Task instead.
	required := []string{"subagents", "web"}
	switch worker {
	case delegation.WorkerExplore:
		required = []string{"subagents"}
	case delegation.WorkerBuild:
		required = []string{"subagents", "todo"}
	}
	for _, plugin := range required {
		var enabled bool
		err := conn.QueryRowContext(ctx, `SELECT enabled FROM plugin_enabled_configuration WHERE plugin_id=?`, plugin).Scan(&enabled)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && !enabled {
			return delegation.ErrAuthority
		}
	}
	receipt, err := getCompositionReceipt(ctx, conn, p.Scope.SessionID)
	if err != nil {
		return err
	}
	caps := map[string]composition.Capability{}
	for _, c := range receipt.Capabilities {
		caps[c.ID] = c
	}
	switch worker {
	case delegation.WorkerExplore:
		if _, ok := caps[delegation.ExploreCapabilityID]; !ok {
			return errors.New("parent composition does not permit explore delegation")
		}
		if err := authorizeExploreComposition(receipt, child); err != nil {
			return err
		}
	case delegation.WorkerBuild:
		if _, ok := caps[delegation.BuildCapabilityID]; !ok {
			return errors.New("parent composition does not permit build delegation")
		}
		if err := authorizeBuildComposition(receipt, child); err != nil {
			return err
		}
	default:
		if _, ok := caps[delegation.CapabilityID]; !ok {
			return errors.New("parent composition does not permit delegation")
		}
		if len(child.Capabilities) != 2 {
			return errors.New("only the restricted research preset is allowed")
		}
		for _, c := range child.Capabilities {
			if c.ID != "web.search" && c.ID != "web.fetch" {
				return delegation.ErrAuthority
			}
			parent, ok := caps[c.ID]
			if !ok || parent.ContractVersion != c.ContractVersion || parent.SchemaSHA256 != c.SchemaSHA256 {
				return errors.New("required research capability exceeds parent composition")
			}
		}
	}
	var payload string
	if err := conn.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE id=? AND session_id=? AND event_type='tool_intent'`, p.IntentEventID, p.Scope.SessionID).Scan(&payload); err != nil {
		return errors.New("delegation requires a committed source invocation")
	}
	var intent memory.ToolIntentPayload
	if err = json.Unmarshal([]byte(payload), &intent); err != nil || !delegation.AdmitsChildren(intent.Call.Name) {
		return delegation.ErrAuthority
	}
	// A fresh child is admitted only by its own worker's delegation tool.
	if intent.Call.Name != delegation.ContinueToolName && intent.Call.Name != delegation.DelegationToolName(worker) {
		return delegation.ErrAuthority
	}
	// A continuation is delegated work too: it needs both capabilities.
	if _, ok := caps[delegation.ContinueCapabilityID]; intent.Call.Name == delegation.ContinueToolName && !ok {
		return errors.New("parent composition does not permit continuing research")
	}
	if intent.Lease == nil || intent.Lease.SessionID != p.Lease.SessionID || intent.Lease.HolderID != p.Lease.HolderID || intent.Lease.FencingToken != p.Lease.FencingToken {
		return delegation.ErrAuthority
	}
	// A turn appends its root user message first and every later event of
	// that turn under the same fenced lease; the next turn, or the turn's
	// failure or interruption, appends a boundary. An outstanding intent that
	// was written under the live lease after the latest root, with no boundary
	// since, therefore descends from that root. Proving this by position uses
	// indexed lookups with no ancestry walk, so it has no turn-depth limit.
	var linked bool
	if err = conn.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM events source JOIN events intent ON intent.session_id=source.session_id
 WHERE source.id=? AND source.session_id=? AND source.event_type='user_message' AND source.parent_id IS NULL
 AND intent.id=? AND intent.event_type='tool_intent' AND intent.sequence>source.sequence
 ) AND NOT EXISTS(
 SELECT 1 FROM events WHERE session_id=? AND sequence>(SELECT sequence FROM events WHERE id=?) AND event_type IN ('user_message','turn_failed','turn_interrupted')
 ) AND NOT EXISTS(SELECT 1 FROM events WHERE session_id=? AND execution_id=(SELECT execution_id FROM events WHERE id=?) AND event_type IN ('tool_succeeded','tool_failed','tool_cancelled'))`,
		p.SourceEventID, p.Scope.SessionID, p.IntentEventID, p.Scope.SessionID, p.SourceEventID, p.Scope.SessionID, p.IntentEventID).Scan(&linked); err != nil {
		return err
	}
	if !linked {
		return delegation.ErrAuthority
	}
	return nil
}

// authorizeExploreComposition proves an explore child within the parent's
// ceiling: the explore preset selects no plugin capability, its tools are
// exactly the read-only explore tools, and the parent itself holds read_file,
// query_db and bash, of which the worker's tools are confined, read-only
// forms. A parent without them cannot delegate explore work.
func authorizeExploreComposition(parent, child composition.Receipt) error {
	if len(child.Capabilities) != 0 || len(child.Providers) != 0 {
		return errors.New("the read-only explore preset selects no plugin capabilities")
	}
	names := make([]string, len(child.ToolSchemas))
	for i, schema := range child.ToolSchemas {
		names[i] = schema.Name
	}
	if !slices.Equal(names, delegation.ExploreToolNames) {
		return errors.New("only the read-only explore tools are allowed")
	}
	for _, name := range delegation.ExploreToolNames {
		if !slices.ContainsFunc(parent.ToolSchemas, func(s composition.ToolSchema) bool { return s.Name == name }) {
			return fmt.Errorf("%w: parent lacks %s, so explore work would exceed its composition", delegation.ErrAuthority, name)
		}
	}
	return nil
}

// authorizeExploreRoot proves an explore attempt's pinned root is inside the
// parent's current folder: its Workspace's latest attached folder, or its
// project root. A parent with neither has no ceiling beyond the root it
// declared, which the harness resolved at admission. A Workspace without a
// folder has that ceiling only while its owner has explore explicitly on
// (2026-10-03); authorizeWorkspacePreset proves the full allowance first.
func authorizeExploreRoot(ctx context.Context, conn *sql.Conn, scope memory.ScopeContext, root string) error {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("%w: an explore attempt needs a pinned absolute root", delegation.ErrAuthority)
	}
	base := scope.ProjectRoot
	if scope.WorkspaceID != "" {
		var on bool
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE((SELECT path FROM workspace_folder_revisions WHERE workspace_id=w.id ORDER BY revision DESC LIMIT 1),''),
 EXISTS(SELECT 1 FROM workspace_explore_settings e WHERE e.workspace_id=w.id AND e.revision_id=w.current_revision_id AND e.setting='on')
 FROM workspaces w WHERE w.id=?`, scope.WorkspaceID).Scan(&base, &on); err != nil {
			return err
		}
		if base == "" && !on {
			return fmt.Errorf("%w: the Workspace has no attached folder for explore workers to read, and explore is not turned on", delegation.ErrAuthority)
		}
	}
	if base == "" || pathWithin(base, root) {
		return nil
	}
	if canonical, err := filepath.EvalSymlinks(base); err == nil && pathWithin(canonical, root) {
		return nil
	}
	return fmt.Errorf("%w: explore root %q is outside the conversation's folder %q", delegation.ErrAuthority, root, base)
}

// authorizeAttemptRoot applies authorizeExploreRoot to an explore attempt
// and authorizeBuildWorktree to a build attempt; research attempts have no
// folder.
func authorizeAttemptRoot(ctx context.Context, conn *sql.Conn, a delegation.Attempt) error {
	switch a.Worker() {
	case delegation.WorkerExplore:
		return authorizeExploreRoot(ctx, conn, a.Parent.Scope, a.Root)
	case delegation.WorkerBuild:
		if a.Worktree == nil {
			return fmt.Errorf("%w: a build attempt needs a pinned worktree", delegation.ErrAuthority)
		}
		return authorizeBuildWorktree(ctx, conn, a.Parent.Scope, *a.Worktree)
	}
	return nil
}

// pathWithin reports whether p is root or below it.
func pathWithin(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func authorizeSubagentTask(ctx context.Context, conn *sql.Conn, p delegation.Parent, id string) error {
	if id == "" {
		return nil
	}
	receipt, err := getCompositionReceipt(ctx, conn, p.Scope.SessionID)
	if err != nil {
		return err
	}
	capable := false
	for _, c := range receipt.Capabilities {
		if c.ID == "todo.get" {
			capable = true
		}
	}
	if !capable {
		return task.ErrAccessDenied
	}
	bound := task.WithMutationAttribution(ctx, task.MutationAttribution{ActorID: string(p.Scope.OwnerID), SessionID: string(p.Scope.SessionID), WorkspaceID: string(p.Scope.WorkspaceID), ProjectID: string(p.Scope.ProjectID)})
	access, err := taskAccessFromContext(bound, conn)
	if err != nil {
		return err
	}
	var scope string
	if err = conn.QueryRowContext(ctx, `SELECT scope FROM tasks WHERE id=?`, id).Scan(&scope); err != nil {
		return task.ErrNotFound
	}
	for _, allowed := range access.scopes {
		if string(allowed) == scope {
			return nil
		}
	}
	return task.ErrNotFound
}

// InspectSubagent applies current parent and Task access even to retained results.
// It only reads, so waiting callers never contend for SQLite's write lock.
func (s *Store) InspectSubagent(ctx context.Context, p delegation.Parent, id string) (delegation.Attempt, error) {
	var a delegation.Attempt
	err := s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		a, err = s.inspectSubagent(ctx, conn, p, id)
		return err
	})
	return a, err
}

// inspectSubagent reads one attempt of p's session under current access.
// Another session's attempt is indistinguishable from a missing one.
func (s *Store) inspectSubagent(ctx context.Context, conn *sql.Conn, p delegation.Parent, id string) (delegation.Attempt, error) {
	a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=? AND parent_session_id=?`, id, p.Scope.SessionID))
	if err != nil {
		return a, err
	}
	if p.Scope.OwnerID != memory.LocalOwnerID || p.Scope.ParentSessionID != "" {
		return a, delegation.ErrAuthority
	}
	if err = validateSessionScope(ctx, conn, p.Scope); err != nil {
		return a, err
	}
	if p.Scope.WorkspaceID != "" {
		var active bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id=? AND lifecycle_state='active')`, p.Scope.WorkspaceID).Scan(&active); err != nil {
			return a, err
		}
		if !active || p.Scope.WorkspaceID != a.Parent.Scope.WorkspaceID || p.Scope.WorkspaceRevision != a.Parent.Scope.WorkspaceRevision {
			return a, delegation.ErrAuthority
		}
	}
	if p.Scope.ProjectID != "" {
		var archived bool
		if err = conn.QueryRowContext(ctx, `SELECT archived FROM projects WHERE id=?`, p.Scope.ProjectID).Scan(&archived); err != nil {
			return a, err
		}
		if archived {
			return a, ErrProjectNotActive
		}
	}
	return a, authorizeSubagentTask(ctx, conn, p, a.Assignment.TaskID)
}
