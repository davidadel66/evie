package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// The merge gate's records (Stage 12 of docs/memory-architecture.md,
// 2026-10-04). Each build attempt records its gate's last step on its own
// record: the setup run when the harness created its worktree, then the
// verify runs, rebase, approval and merge of its latest merge proposal. An
// in-progress step names the parent turn that runs it, so recovery settles
// it only once that turn is gone.

// RecordBuildGate applies update to attempt id's gate record (creating it)
// under the write lock and stamps the step's time.
func (s *Store) RecordBuildGate(ctx context.Context, id string, update func(*delegation.BuildGate)) (delegation.Attempt, error) {
	var a delegation.Attempt
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		a, err = readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
		if err != nil {
			return err
		}
		if a.Worktree == nil {
			return fmt.Errorf("execution %q is not a build execution", id)
		}
		if a.Gate == nil {
			a.Gate = &delegation.BuildGate{}
		}
		update(a.Gate)
		a.Gate.At = s.now().UTC()
		return writeSubagent(ctx, conn, a)
	})
	return a, err
}

// RecordBuildRebase records that the harness rebased the worker's branch
// onto commit, the head of target: every attempt of the child now reports
// its branch against that commit, and the move that made the rebase
// necessary is settled.
func (s *Store) RecordBuildRebase(ctx context.Context, id, target, commit string) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
		if err != nil {
			return err
		}
		attempts, err := childAttempts(ctx, conn, a.Child.ID)
		if err != nil {
			return err
		}
		for _, attempt := range attempts {
			if attempt.Worktree == nil {
				continue
			}
			attempt.Worktree.Base, attempt.Worktree.BaseCommit = target, commit
			if attempt.Gate != nil {
				attempt.Gate.NeedsRebase = nil
			}
			if err := writeSubagent(ctx, conn, attempt); err != nil {
				return err
			}
		}
		return nil
	})
}

// childAttempts reads every attempt of a child, oldest first.
func childAttempts(ctx context.Context, conn *sql.Conn, child memory.SessionID) ([]delegation.Attempt, error) {
	rows, err := conn.QueryContext(ctx, `SELECT record_json FROM subagent_executions WHERE child_session_id=? ORDER BY rowid`, child)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var attempts []delegation.Attempt
	for rows.Next() {
		a, err := readSubagent(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

// ErrNoVerifyCommand refuses a merge proposal for a Workspace or project
// without a verify command.
var ErrNoVerifyCommand = errors.New("no verify command is set")

// PrepareBuildGate authorizes a propose_build_merge call on one read
// snapshot: parent's live turn committed the call, its pinned composition
// has the merge capability, its Workspace still allows build workers, and
// execution id is its own build attempt, finished, its child's latest, with
// its worktree still the parent's folder's, not removed or merged. It
// returns the attempt and the commands that apply to the parent's scope.
func (s *Store) PrepareBuildGate(ctx context.Context, p delegation.Parent, id string) (delegation.Attempt, memory.BuildVerify, error) {
	var a delegation.Attempt
	var verify memory.BuildVerify
	err := s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		if a, err = s.authorizeBuildGate(ctx, conn, p, id, checkTurnLeaseLive); err != nil {
			return err
		}
		switch {
		case p.Scope.WorkspaceID != "":
			verify, err = buildVerify(ctx, conn, verifyScopeWorkspace, string(p.Scope.WorkspaceID))
		case p.Scope.ProjectID != "":
			verify, err = buildVerify(ctx, conn, verifyScopeProject, string(p.Scope.ProjectID))
		}
		return err
	})
	return a, verify, err
}

// AuthorizeBuildGate repeats PrepareBuildGate's checks before each gate step
// that changes something, without reading the commands.
func (s *Store) AuthorizeBuildGate(ctx context.Context, p delegation.Parent, id string) (delegation.Attempt, error) {
	var a delegation.Attempt
	err := s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		a, err = s.authorizeBuildGate(ctx, conn, p, id, checkTurnLeaseLive)
		return err
	})
	return a, err
}

func (s *Store) authorizeBuildGate(ctx context.Context, conn *sql.Conn, p delegation.Parent, id string, fence subagentLeaseFence) (delegation.Attempt, error) {
	if err := s.authorizeGateParent(ctx, conn, p, fence); err != nil {
		return delegation.Attempt{}, err
	}
	a, err := s.inspectSubagent(ctx, conn, p, id)
	if errors.Is(err, sql.ErrNoRows) {
		a, err = inspectRunBuild(ctx, conn, p, id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return a, fmt.Errorf("%w: %q", delegation.ErrNotFound, id)
	}
	if err != nil {
		return a, err
	}
	if a.Worker() != delegation.WorkerBuild || a.Worktree == nil {
		return a, fmt.Errorf("execution %q is not a build execution; only a build worker's branch can be merged", id)
	}
	if !a.Terminal() {
		return a, fmt.Errorf("execution %q is still %s; wait for its result before proposing a merge", id, a.State)
	}
	var latest string
	if err := conn.QueryRowContext(ctx, `SELECT id FROM subagent_executions WHERE child_session_id=? ORDER BY rowid DESC LIMIT 1`, a.Child.ID).Scan(&latest); err != nil {
		return a, err
	}
	if latest != a.ID {
		return a, fmt.Errorf("execution %q was continued as execution %q; propose the merge with the child's latest execution", id, latest)
	}
	if a.Gate != nil && a.Gate.Merged != nil {
		return a, fmt.Errorf("execution %q was already merged into %s", id, a.Gate.Merged.Target)
	}
	if a.Worktree.RemovedAt != nil {
		return a, fmt.Errorf("the worktree of execution %q was removed, so there is no branch to verify", id)
	}
	if err := authorizeBuildWorktree(ctx, conn, a.Parent.Scope, *a.Worktree); err != nil {
		return a, err
	}
	return a, nil
}

// authorizeGateParent proves the calling parent: its live turn committed
// a propose_build_merge call that has no outcome yet, its pinned
// composition has the merge capability, it is a top-level Global, project
// or Workspace chat whose scope is unchanged, and its Workspace still allows
// build workers. A gate step is the parent's own action, approved by the
// owner, not delegated work, so no worker preset is involved.
func (s *Store) authorizeGateParent(ctx context.Context, conn *sql.Conn, p delegation.Parent, fence subagentLeaseFence) error {
	if p.Scope.ParentSessionID != "" || p.Scope.SessionID != p.Lease.SessionID || p.Scope.OwnerID != memory.LocalOwnerID || p.Lease.Generation != memory.LeaseGeneration(p.Lease.FencingToken) {
		return delegation.ErrAuthority
	}
	if err := validateSessionScope(ctx, conn, p.Scope); err != nil {
		return err
	}
	var ancestor, root, revision sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT parent_session_id,project_root_snapshot,workspace_revision_snapshot FROM sessions WHERE id=?`, p.Scope.SessionID).Scan(&ancestor, &root, &revision); err != nil {
		return err
	}
	if ancestor.Valid || root.String != p.Scope.ProjectRoot || revision.String != string(p.Scope.WorkspaceRevision) {
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
	if p.Scope.WorkspaceID != "" {
		if err := authorizeWorkspaceBuild(ctx, conn, p.Scope); err != nil {
			return err
		}
	}
	if err := fence(ctx, conn, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, s.now().UTC().Format(turnLeaseTimeFormat)); err != nil {
		return err
	}
	var enabled bool
	err := conn.QueryRowContext(ctx, `SELECT enabled FROM plugin_enabled_configuration WHERE plugin_id='subagents'`).Scan(&enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && !enabled {
		return delegation.ErrAuthority
	}
	receipt, err := getCompositionReceipt(ctx, conn, p.Scope.SessionID)
	if err != nil {
		return err
	}
	if !hasCapability(receipt, delegation.MergeCapabilityID) {
		return errors.New("parent composition does not permit proposing build merges")
	}
	var payload string
	if err := conn.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE id=? AND session_id=? AND event_type='tool_intent'`, p.IntentEventID, p.Scope.SessionID).Scan(&payload); err != nil {
		return errors.New("a merge proposal requires a committed source invocation")
	}
	var intent memory.ToolIntentPayload
	if err = json.Unmarshal([]byte(payload), &intent); err != nil || intent.Call.Name != delegation.MergeToolName {
		return delegation.ErrAuthority
	}
	if intent.Lease == nil || intent.Lease.SessionID != p.Lease.SessionID || intent.Lease.HolderID != p.Lease.HolderID || intent.Lease.FencingToken != p.Lease.FencingToken {
		return delegation.ErrAuthority
	}
	// The same position proof as delegation: the intent belongs to the
	// live turn and has no outcome yet.
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

func hasCapability(receipt composition.Receipt, id string) bool {
	for _, c := range receipt.Capabilities {
		if c.ID == id {
			return true
		}
	}
	return false
}

// MarkBuildTargetMoved records, on the latest attempt of every other build
// child in repo whose work is unmerged and still has its worktree, and
// whose target (its last proposal's, or its base branch) is move.Target,
// that the target moved: its next merge proposal rebases, and a running
// attempt's result says so. It returns the attempts it marked.
func (s *Store) MarkBuildTargetMoved(ctx context.Context, repo string, move delegation.TargetMove, except memory.SessionID) ([]delegation.Attempt, error) {
	var marked []delegation.Attempt
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		rows, err := conn.QueryContext(ctx, `SELECT record_json FROM subagent_executions WHERE CASE WHEN json_valid(record_json) THEN json_type(record_json,'$.worktree') IS NOT NULL END ORDER BY rowid DESC`)
		if err != nil {
			return err
		}
		seen := map[memory.SessionID]bool{}
		var latest []delegation.Attempt
		for rows.Next() {
			a, err := readSubagent(rows)
			if err != nil || a.Worktree == nil || seen[a.Child.ID] {
				continue
			}
			seen[a.Child.ID] = true
			latest = append(latest, a)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, a := range latest {
			if a.Child.ID == except || a.Worktree.Repo != repo || a.Worktree.RemovedAt != nil || (a.Gate != nil && a.Gate.Merged != nil) {
				continue
			}
			target := a.Worktree.Base
			if a.Gate != nil && a.Gate.Target != "" {
				target = a.Gate.Target
			}
			if target != move.Target {
				continue
			}
			if a.Gate == nil {
				a.Gate = &delegation.BuildGate{}
			}
			moved := move
			a.Gate.NeedsRebase = &moved
			if err := writeSubagent(ctx, conn, a); err != nil {
				return err
			}
			marked = append(marked, a)
		}
		return nil
	})
	return marked, err
}

// BuildGateRecoveryCandidates lists build attempts whose gate is in an
// in-progress step (verify, rebase or merge) and whose step owner's turn is
// no longer live: a crash or a lost turn left them.
func (s *Store) BuildGateRecoveryCandidates(ctx context.Context) ([]delegation.Attempt, error) {
	var candidates []delegation.Attempt
	err := s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		rows, err := conn.QueryContext(ctx, `SELECT record_json FROM subagent_executions WHERE state NOT IN ('admitted','running')
 AND CASE WHEN json_valid(record_json) THEN json_extract(record_json,'$.gate.step') IN (?,?,?,?) END`,
			delegation.GateVerifying, delegation.GateRebasing, delegation.GateVerifyingAfter, delegation.GateMerging)
		if err != nil {
			return err
		}
		var found []delegation.Attempt
		for rows.Next() {
			a, err := readSubagent(rows)
			if err != nil {
				continue
			}
			found = append(found, a)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		now := s.now().UTC().Format(turnLeaseTimeFormat)
		for _, a := range found {
			owner := a.Gate.Owner
			if owner != nil {
				err := checkTurnLeaseLive(ctx, conn, owner.SessionID, owner.HolderID, owner.FencingToken, now)
				if err == nil {
					continue
				}
				if !errors.Is(err, ErrTurnLeaseLost) {
					return err
				}
			}
			candidates = append(candidates, a)
		}
		return nil
	})
	return candidates, err
}
