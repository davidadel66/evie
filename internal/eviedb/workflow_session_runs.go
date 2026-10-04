package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// Session starts (Stage 14 sub-stage 10 of docs/memory-architecture.md,
// 2026-10-04; workflow-runtime.spec.md: "Manual/session start is the first
// delivery slice", user story 18). Evie starts a run of a Workspace's
// active workflow from the owner's chat in that Workspace through its
// workflow_start tool. Such a run records origin surface "chat" and its
// session, and StartWorkflowRun admits it, in the transaction that records
// it, only for a top-level, active session of the run's Workspace whose
// pinned composition exposes workflows.start (the spec: the initiating
// preset must expose the capability used to start workflows), while the
// session's turn lease is live and that turn's committed workflow_start
// call has no outcome yet: the same position proof the merge gate uses.
// Run admission and everything else about a run are unchanged. A chat
// cannot rerun a succeeded key (the override stays the owner's), and every
// other owner record (approvals, cancellations, reconciliations, answers,
// overrides) keeps refusing the chat surface in its own check.

// The session-start capability and tool, which the Workflows plugin
// exposes; the plugin's tests pin that its names match.
const (
	WorkflowStartCapabilityID  = "workflows.start"
	WorkflowStatusCapabilityID = "workflows.status"
	WorkflowStartToolName      = "workflow_start"
)

// ErrWorkflowChatStart refuses a chat start whose session, turn or call
// does not prove it.
var ErrWorkflowChatStart = errors.New("a workflow run starts from a chat only through that chat's live workflow_start call")

// WorkflowChatOrigin is the chat turn that starts a run: its turn lease and
// the user message and committed workflow_start call of that turn.
type WorkflowChatOrigin struct {
	Lease         memory.TurnLease
	SourceEventID memory.EventID
	IntentEventID memory.EventID
}

// workflowRunSurfacesBeforeChat is the origin check of sub-stages 3 to 9;
// workflowRunSurfacesChat (sub-stage 10) adds chat, which always names its
// session; workflowRunSurfaces (sub-stage 11) adds schedule, which never
// does.
const (
	workflowRunSurfacesBeforeChat = "origin_surface TEXT NOT NULL CHECK(origin_surface IN ('cli','web'))"
	workflowRunSurfacesChat       = "origin_surface TEXT NOT NULL CHECK(origin_surface IN ('cli','web','chat') AND (origin_surface <> 'chat' OR session_id IS NOT NULL))"
	workflowRunSurfaces           = "origin_surface TEXT NOT NULL CHECK(origin_surface IN ('cli','web','chat','schedule') AND (origin_surface <> 'chat' OR session_id IS NOT NULL) AND (origin_surface <> 'schedule' OR session_id IS NULL))"
)

// validRunSurface checks a start's surface before its transaction: the
// owner's CLI or web without a chat turn, a chat with its session and turn
// and no rerun override, or (sub-stage 11) a schedule with its slot and
// neither a session nor an override.
func validRunSurface(req WorkflowRunStart) error {
	switch req.Surface {
	case "cli", "web":
		if req.Chat != nil || req.Slot != nil {
			return ErrWorkflowRunSurface
		}
	case "chat":
		if req.Chat == nil || req.Slot != nil || req.SessionID == "" || req.RerunOf != "" || strings.TrimSpace(req.Reason) != "" {
			return ErrWorkflowRunSurface
		}
	case "schedule":
		if req.Slot == nil || req.Chat != nil || req.SessionID != "" || req.RerunOf != "" || strings.TrimSpace(req.Reason) != "" {
			return ErrWorkflowRunSurface
		}
		return validSlotStart(req.Slot)
	default:
		return ErrWorkflowRunSurface
	}
	return nil
}

// authorizeChatStart proves a chat start inside the run's transaction.
func authorizeChatStart(ctx context.Context, conn *sql.Conn, req WorkflowRunStart, nowText string) error {
	o := req.Chat
	refuse := func(why string) error { return fmt.Errorf("%w: %s", ErrWorkflowChatStart, why) }
	if o.Lease.SessionID != req.SessionID || o.Lease.Generation != memory.LeaseGeneration(o.Lease.FencingToken) {
		return refuse("the turn lease is not the session's")
	}
	var parent, workspace sql.NullString
	var status string
	err := conn.QueryRowContext(ctx, `SELECT parent_session_id,workspace_id,status FROM sessions WHERE id=?`, req.SessionID).Scan(&parent, &workspace, &status)
	switch {
	case errors.Is(err, sql.ErrNoRows) || err == nil && workspace.String != string(req.WorkspaceID):
		return ErrWorkflowRunSession
	case err != nil:
		return err
	}
	if parent.Valid || status != string(memory.SessionActive) {
		return refuse("only an active top-level chat starts runs")
	}
	if err := checkTurnLeaseLive(ctx, conn, req.SessionID, o.Lease.HolderID, o.Lease.FencingToken, nowText); err != nil {
		return refuse("the chat's turn has ended")
	}
	receipt, err := getCompositionReceipt(ctx, conn, req.SessionID)
	if err != nil {
		return err
	}
	if !hasCapability(receipt, WorkflowStartCapabilityID) {
		return refuse("the chat's pinned composition does not expose " + WorkflowStartCapabilityID)
	}
	var payload string
	if err := conn.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE id=? AND session_id=? AND event_type='tool_intent'`, o.IntentEventID, req.SessionID).Scan(&payload); err != nil {
		return refuse("no committed workflow_start call")
	}
	var intent memory.ToolIntentPayload
	if err := json.Unmarshal([]byte(payload), &intent); err != nil || intent.Call.Name != WorkflowStartToolName {
		return refuse("the committed call is not workflow_start")
	}
	// The call belongs to the live turn and has no outcome yet.
	var linked bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM events source JOIN events intent ON intent.session_id=source.session_id
 WHERE source.id=? AND source.session_id=? AND source.event_type='user_message' AND source.parent_id IS NULL
 AND intent.id=? AND intent.event_type='tool_intent' AND intent.sequence>source.sequence
 ) AND NOT EXISTS(
 SELECT 1 FROM events WHERE session_id=? AND sequence>(SELECT sequence FROM events WHERE id=?) AND event_type IN ('user_message','turn_failed','turn_interrupted')
 ) AND NOT EXISTS(SELECT 1 FROM events WHERE session_id=? AND execution_id=(SELECT execution_id FROM events WHERE id=?) AND event_type IN ('tool_succeeded','tool_failed','tool_cancelled'))`,
		o.SourceEventID, req.SessionID, o.IntentEventID, req.SessionID, o.SourceEventID, req.SessionID, o.IntentEventID).Scan(&linked); err != nil {
		return err
	}
	if !linked {
		return refuse("the workflow_start call is not the live turn's or already has an outcome")
	}
	return nil
}

// inspectRunBuild reads a workflow run's build attempt for the merge gate
// (sub-stage 10). Workflows cannot merge (the spec's v1 rule): a run's
// build branch waits for the owner's Stage 12 gate "from a session". So a
// top-level chat of the run's Workspace, whose own checks authorizeGateParent
// already applied (its live propose_build_merge call, the merge capability,
// the Workspace's build allowance), may take a run-owned build attempt to
// the gate once its run has finished: until then the workflow may still
// continue that worker in its worktree. The owner still approves the merge
// on the gate's card. Another Workspace's chat finds nothing.
func inspectRunBuild(ctx context.Context, conn *sql.Conn, p delegation.Parent, id string) (delegation.Attempt, error) {
	if p.Scope.WorkspaceID == "" || p.Scope.ParentSessionID != "" || p.Scope.OwnerID != memory.LocalOwnerID {
		return delegation.Attempt{}, sql.ErrNoRows
	}
	a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT e.record_json FROM subagent_executions e
 JOIN workflow_worker_sessions s ON s.session_id=e.parent_session_id JOIN workflow_runs r ON r.id=s.run_id
 WHERE e.id=? AND r.workspace_id=?`, id, p.Scope.WorkspaceID))
	if err != nil {
		return a, err
	}
	if a.Run == nil {
		return a, sql.ErrNoRows
	}
	var state WorkflowRunState
	if err := conn.QueryRowContext(ctx, `SELECT state FROM workflow_runs WHERE id=?`, a.Run.RunID).Scan(&state); err != nil {
		return a, err
	}
	if !state.Terminal() {
		return a, fmt.Errorf("execution %q is a build of workflow run %s, which is still %s; its branch can go to the merge gate once the run has finished", id, a.Run.RunID, state)
	}
	return a, authorizeSubagentTask(ctx, conn, p, a.Assignment.TaskID)
}

var workflowRunsTableName = regexp.MustCompile(`^CREATE TABLE\s+"?workflow_runs"?\s*\(`)

// rebuildWorkflowRuns rebuilds the workflow_runs table when its definition
// holds from, with to in its place: SQLite cannot change a check in place.
// Under the write lock, with foreign keys off, it copies every row with its
// rowid into the new table, drops the old one, renames the new one, and
// recreates exactly the indexes and triggers the old one had (read from the
// schema first), then checks foreign keys before committing. Competing
// startups rebuild it once: each re-reads the definition under the lock.
func rebuildWorkflowRuns(ctx context.Context, db *sql.DB, from, to string) error {
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_runs'`).Scan(&definition); err != nil {
		return err
	}
	if !strings.Contains(definition, from) {
		return nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys=ON`)
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`)
		}
	}()
	if err := conn.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_runs'`).Scan(&definition); err != nil {
		return err
	}
	if !strings.Contains(definition, from) {
		return nil
	}
	if !workflowRunsTableName.MatchString(definition) {
		return errors.New("unsupported workflow run table schema")
	}
	rebuilt := workflowRunsTableName.ReplaceAllString(strings.Replace(definition, from, to, 1), "CREATE TABLE workflow_runs_rebuilt (")
	var dependents []string
	rows, err := conn.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE tbl_name='workflow_runs' AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		dependents = append(dependents, s)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var columns []string
	rows, err = conn.QueryContext(ctx, `SELECT name FROM pragma_table_info('workflow_runs') ORDER BY cid`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		columns = append(columns, `"`+name+`"`)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	list := strings.Join(columns, ",")
	for _, statement := range append([]string{rebuilt,
		`INSERT INTO workflow_runs_rebuilt(rowid,` + list + `) SELECT rowid,` + list + ` FROM workflow_runs`,
		`DROP TABLE workflow_runs`,
		`ALTER TABLE workflow_runs_rebuilt RENAME TO workflow_runs`,
	}, dependents...) {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild workflow runs: %w", err)
		}
	}
	rows, err = conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	violated := rows.Next()
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		return scanErr
	}
	if violated {
		return errors.New("the workflow run rebuild would violate a foreign key")
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}
