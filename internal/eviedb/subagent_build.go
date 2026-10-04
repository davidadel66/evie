package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

// Build workers (Stage 11 of docs/memory-architecture.md, 2026-10-03). The
// store pins each build attempt's harness-planned git worktree, issues the
// child a Task Access Grant on the assignment's Task (contribute: read,
// claim, update, decompose that Task's subtree only) and ends it, with any
// claim the child still holds, when the attempt ends.

// Bounds of the commands a build result lists.
const (
	maxBuildCommands     = 20
	maxBuildCommandBytes = 300
)

// AdmitBuildSubagents admits a delegate_build batch like AdmitSubagents.
// plans[i] is the worktree the harness planned for requests[i], or
// planErrs[i] says why it could not; a plan only matters for a fresh
// assignment, so a retained key replays whatever its repository looks like
// now.
func (s *Store) AdmitBuildSubagents(ctx context.Context, p delegation.Parent, requests []delegation.Assignment, plans []delegation.Worktree, planErrs []error, receipt composition.Receipt, policy delegation.Policy, dispatchID string) ([]delegation.Attempt, error) {
	if receipt.Preset.ID != delegation.WorkerBuild {
		return nil, errors.New("only the build preset is allowed")
	}
	if len(plans) != len(requests) || len(planErrs) != len(requests) {
		return nil, errors.New("every build assignment needs a planned worktree")
	}
	return s.admitSubagents(ctx, p, requests, admission{plans: plans, planErrs: planErrs}, receipt, policy, dispatchID)
}

// authorizeBuildComposition proves a build child within the parent's
// ceiling: exactly the build Task Capabilities, each pinned at the parent's
// own contract and schema, and exactly the worktree-confined build tools,
// which are narrower forms of the parent's read_file, edit_file and bash.
func authorizeBuildComposition(parent, child composition.Receipt) error {
	if len(child.Capabilities) != len(delegation.BuildTaskCapabilityIDs) {
		return errors.New("only the build preset's Task capabilities are allowed")
	}
	parentCaps := map[string]composition.Capability{}
	for _, c := range parent.Capabilities {
		parentCaps[c.ID] = c
	}
	for i, c := range child.Capabilities {
		if c.ID != delegation.BuildTaskCapabilityIDs[i] {
			return delegation.ErrAuthority
		}
		own, ok := parentCaps[c.ID]
		if !ok || own.ContractVersion != c.ContractVersion || own.SchemaSHA256 != c.SchemaSHA256 {
			return fmt.Errorf("%w: build capability %s exceeds the parent composition", delegation.ErrAuthority, c.ID)
		}
	}
	names := make([]string, len(child.ToolSchemas))
	for i, schema := range child.ToolSchemas {
		names[i] = schema.Name
	}
	kernel := len(delegation.BuildKernelToolNames) - 1
	if len(names) > kernel && names[kernel] == "bash" {
		kernel++
	}
	if len(names) != kernel+len(child.Capabilities) || !slices.Equal(names[:kernel], delegation.BuildKernelToolNames[:kernel]) {
		return errors.New("only the worktree-confined build tools are allowed")
	}
	for _, name := range delegation.BuildParentToolNames {
		if !slices.ContainsFunc(parent.ToolSchemas, func(s composition.ToolSchema) bool { return s.Name == name }) {
			return fmt.Errorf("%w: parent lacks %s, so build work would exceed its composition", delegation.ErrAuthority, name)
		}
	}
	return nil
}

// conversationFolder is the parent's current folder: its Workspace's latest
// attached folder, or its project root.
func conversationFolder(ctx context.Context, conn *sql.Conn, scope memory.ScopeContext) (string, error) {
	if scope.WorkspaceID == "" {
		return scope.ProjectRoot, nil
	}
	var folder string
	err := conn.QueryRowContext(ctx, `SELECT COALESCE((SELECT path FROM workspace_folder_revisions WHERE workspace_id=w.id ORDER BY revision DESC LIMIT 1),'') FROM workspaces w WHERE w.id=?`, scope.WorkspaceID).Scan(&folder)
	return folder, err
}

// authorizeBuildWorktree proves a build attempt's pinned worktree still
// belongs to the parent's current folder: the repository is that folder,
// the worktree is the harness's place under its .evie folder, and the owner
// has not removed it. A Workspace whose folder moves ends the child's
// authority, as for explore.
func authorizeBuildWorktree(ctx context.Context, conn *sql.Conn, scope memory.ScopeContext, w delegation.Worktree) error {
	if w.RemovedAt != nil {
		return fmt.Errorf("%w: worktree %s was removed", delegation.ErrAuthority, w.Path)
	}
	folder, err := conversationFolder(ctx, conn, scope)
	if err != nil {
		return err
	}
	if folder == "" {
		return fmt.Errorf("%w: the conversation has no Workspace folder or project root for build work", delegation.ErrAuthority)
	}
	canonical := folder
	if resolved, err := filepath.EvalSymlinks(folder); err == nil {
		canonical = resolved
	}
	if w.Repo == "" || (w.Repo != folder && w.Repo != canonical) {
		return fmt.Errorf("%w: repository %q is not the conversation's folder %q", delegation.ErrAuthority, w.Repo, folder)
	}
	id := filepath.Base(w.Path)
	if w.Path != filepath.Join(w.Repo, ".evie", "worktrees", id) || w.Temp != filepath.Join(w.Repo, ".evie", "tmp", id) {
		return fmt.Errorf("%w: worktree %q is not the harness's place in %s", delegation.ErrAuthority, w.Path, w.Repo)
	}
	return nil
}

// prepareBuildTask requires a build assignment's Task to be open and free
// for the worker to claim: a claim held by a live execution refuses the
// assignment (the worker claims its own Task); a claim left by an execution
// that is no longer running is released, as recovery would. One unfinished
// build attempt works on a Task at a time.
func prepareBuildTask(ctx context.Context, conn *sql.Conn, id string, now time.Time) error {
	var status task.Status
	if err := conn.QueryRowContext(ctx, `SELECT status FROM tasks WHERE id=?`, id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &task.NotFoundError{ID: task.ID(id)}
		}
		return err
	}
	if status == task.StatusCompleted || status == task.StatusCancelled {
		return fmt.Errorf("Task %s is %s; reopen it or choose an open Task for build work", id, status)
	}
	claim, found, err := getStoredTaskClaim(ctx, conn, task.ID(id))
	if err != nil {
		return err
	}
	if found {
		live, err := claimExecutionActive(ctx, conn, task.MutationAttribution{SessionID: claim.SessionID, LeaseHolderID: claim.LeaseHolderID,
			LeaseToken: claim.LeaseToken, LeaseGeneration: claim.LeaseGeneration}, now)
		if err != nil {
			return err
		}
		if live {
			return fmt.Errorf("Task %s is claimed by an active execution; release that claim (todo_release) before delegating build work on it, because the build worker claims its own Task", id)
		}
		if err := releaseClaimsAsKernel(ctx, conn, `task_id = ?`, []any{id}, now, "recovery"); err != nil {
			return err
		}
	}
	var busy string
	err = conn.QueryRowContext(ctx, `SELECT id FROM subagent_executions WHERE state IN ('admitted','running')
 AND CASE WHEN json_valid(record_json) THEN json_type(record_json,'$.worktree') IS NOT NULL AND json_extract(record_json,'$.assignment.task_id')=? END LIMIT 1`, id).Scan(&busy)
	if err == nil {
		return fmt.Errorf("Task %s already has an unfinished build execution %s", id, busy)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

// issueBuildGrant gives a build child a contribute grant on its Task's
// subtree for the attempt's lifetime. The parent issues it; the grant never
// reveals the Task's ancestors or siblings.
func issueBuildGrant(ctx context.Context, conn *sql.Conn, p delegation.Parent, child memory.SessionID, taskID, attemptID string, now time.Time) error {
	if _, err := conn.ExecContext(ctx, `INSERT INTO task_access_grants(id,grantee_session_id,root_task_id,access_level,issuer_actor_id,issuer_session_id,issuer_run_id,issued_at)
 VALUES(?,?,?,?,?,?,?,?)`, uuid.NewString(), child, taskID, task.AccessContribute, p.Scope.OwnerID, p.Scope.SessionID, attemptID, formatTaskTime(now)); err != nil {
		return fmt.Errorf("issue the build worker's Task Access Grant: %w", err)
	}
	return nil
}

// endBuildAccess ends a build child's grant and releases any claim it still
// holds, so the orchestrator can claim the Task again.
func endBuildAccess(ctx context.Context, conn *sql.Conn, a delegation.Attempt, now time.Time) error {
	if _, err := conn.ExecContext(ctx, `UPDATE task_access_grants SET ended_at=?,end_reason='execution_ended',ended_by_actor_id='kernel',ended_by_session_id=?,ended_by_run_id=?
 WHERE grantee_session_id=? AND ended_at IS NULL`, formatTaskTime(now), a.Parent.Scope.SessionID, a.ID, a.Child.ID); err != nil {
		return fmt.Errorf("end the build worker's Task Access Grant: %w", err)
	}
	return releaseClaimsAsKernel(ctx, conn, `session_id = ?`, []any{a.Child.ID}, now, "execution_ended")
}

// releaseClaimsAsKernel releases the claims matching where with the same
// audit event as any release, reading each Task's revision directly: it runs
// in settlement and recovery, which act for no session's Task scope.
func releaseClaimsAsKernel(ctx context.Context, conn *sql.Conn, where string, args []any, now time.Time, reason string) error {
	rows, err := conn.QueryContext(ctx, `SELECT c.claim_id,c.task_id,c.session_id,t.revision FROM task_claims c JOIN tasks t ON t.id=c.task_id WHERE c.`+where+` ORDER BY c.task_id`, args...)
	if err != nil {
		return fmt.Errorf("list Task claims to release: %w", err)
	}
	type held struct {
		claimID, taskID, sessionID string
		revision                   uint64
	}
	var claims []held
	for rows.Next() {
		var h held
		if err := rows.Scan(&h.claimID, &h.taskID, &h.sessionID, &h.revision); err != nil {
			rows.Close()
			return err
		}
		claims = append(claims, h)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, h := range claims {
		if _, err := conn.ExecContext(ctx, `DELETE FROM task_claims WHERE task_id=? AND claim_id=?`, h.taskID, h.claimID); err != nil {
			return fmt.Errorf("release Task claim: %w", err)
		}
		if _, err := appendClaimEvent(ctx, conn, task.Event{TaskID: task.ID(h.taskID), Operation: task.OperationRelease, ActorID: "kernel", SessionID: h.sessionID,
			RunID: "build-cleanup", RecordedAt: now, PreviousRevision: h.revision, ResultingRevision: h.revision, Outcome: task.MutationAccepted,
			ClaimID: h.claimID, ClaimReason: reason}); err != nil {
			return err
		}
	}
	return nil
}

var exitStatusLine = regexp.MustCompile(`\nexit status: (-?[0-9]+)\n?$`)

// subagentBuildCommands lists the bash commands the child ran in the
// attempt's events with how each ended, the latest maxBuildCommands of them.
// Commands are child-written; the parent view frames them.
func subagentBuildCommands(ctx context.Context, conn *sql.Conn, child memory.SessionID, after int64) ([]delegation.BuildCommand, int, error) {
	rows, err := conn.QueryContext(ctx, `SELECT i.payload_json,COALESCE(o.event_type,''),COALESCE(o.content,'') FROM events i
 LEFT JOIN events o ON o.session_id=i.session_id AND o.execution_id=i.execution_id AND o.event_type IN ('tool_succeeded','tool_failed','tool_cancelled')
 WHERE i.session_id=? AND i.sequence>? AND i.event_type='tool_intent' ORDER BY i.sequence`, child, after)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var commands []delegation.BuildCommand
	for rows.Next() {
		var payload []byte
		var outcome, content string
		if err := rows.Scan(&payload, &outcome, &content); err != nil {
			return nil, 0, err
		}
		var intent memory.ToolIntentPayload
		if json.Unmarshal(payload, &intent) != nil || intent.Call.Name != "bash" {
			continue
		}
		var args struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal([]byte(intent.Call.Arguments), &args)
		command := strings.TrimSpace(args.Command)
		if len(command) > maxBuildCommandBytes {
			command = utf8Prefix(command, maxBuildCommandBytes) + "…"
		}
		c := delegation.BuildCommand{Command: command}
		switch {
		case outcome == "tool_succeeded":
			if m := exitStatusLine.FindStringSubmatch(content); m != nil {
				c.Outcome = "exit"
				c.ExitStatus, _ = strconv.Atoi(m[1])
			} else {
				c.Outcome = "error"
			}
		case strings.Contains(content, "timed out"):
			c.Outcome = "timeout"
		case strings.Contains(content, "refused"):
			c.Outcome = "refused"
		case outcome == "":
			c.Outcome = "no_result"
		default:
			c.Outcome = "error"
		}
		commands = append(commands, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	omitted := 0
	if len(commands) > maxBuildCommands {
		omitted = len(commands) - maxBuildCommands
		commands = commands[omitted:]
	}
	return commands, omitted, nil
}

// buildNotes are the harness notes of a build result: where the work is and
// how to resume or clean it up when the harness could not commit it.
func buildNotes(a delegation.Attempt, report *delegation.BuildReport, harnessReported bool) []string {
	notes := worktreeNotes(a, report, harnessReported)
	// Stage 12 (2026-10-04): how setup went when it did not pass, and a
	// target that moved while the worker ran.
	if setup := report.Setup; setup != nil && setup.Outcome != delegation.RunPassed {
		note := fmt.Sprintf("Setup command `%s` %s", setup.Command, setup.Outcome)
		if setup.Outcome == delegation.RunFailed {
			note += fmt.Sprintf(" (exit %d)", setup.ExitStatus)
		}
		if setup.Detail != "" {
			note += ": " + setup.Detail
		}
		if setup.Log != "" {
			note += "; its output is in " + setup.Log
		}
		notes = append(notes, note+".")
	}
	if moved := report.TargetMoved; moved != nil {
		notes = append(notes, fmt.Sprintf("While this worker ran, %s moved from %s to %s (the merge of execution %s); propose_build_merge rebases this branch onto it before verifying.",
			moved.Target, shortSHA(moved.From), shortSHA(moved.To), moved.ByExecution))
	}
	return notes
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// worktreeNotes are where the work is and how to resume or clean it up.
func worktreeNotes(a delegation.Attempt, report *delegation.BuildReport, harnessReported bool) []string {
	w := a.Worktree
	cleanup := fmt.Sprintf("`evie worktrees clean %s` removes the worktree", a.ID)
	switch {
	case !harnessReported:
		return []string{fmt.Sprintf("Evie's harness recorded no commit for this attempt; worktree %s on branch %s was kept for inspection with any uncommitted changes. To resume, commit or discard them and delegate a new build with base %q; %s.", w.Path, w.Branch, w.Branch, cleanup)}
	case report.Head == "" && report.CommitError != "":
		return []string{"Worktree problem: " + report.CommitError + "."}
	case report.Uncommitted && report.CommitError != "":
		return []string{fmt.Sprintf("Changes in worktree %s were not committed on branch %s (%s); they were kept for inspection; %s.", w.Path, w.Branch, report.CommitError, cleanup)}
	case report.Uncommitted:
		return []string{fmt.Sprintf("Worktree %s has changes the harness did not commit (ignored or added after its commit); %s.", w.Path, cleanup)}
	}
	return nil
}

// FinishBuildSubagent settles a build attempt like FinishSubagent, recording
// the harness's commit report: the branch, its commits and the files they
// changed. A nil report means the harness did not commit (the attempt never
// got its worktree, or recovery settles it).
func (s *Store) FinishBuildSubagent(ctx context.Context, id, state, reason string, report *delegation.BuildReport) (delegation.Attempt, error) {
	return s.finishSubagentWith(ctx, id, state, reason, report)
}

// BuildWorktree is one build attempt's worktree record, for the owner's
// cleanup command.
type BuildWorktree struct {
	ExecutionID string              `json:"execution_id"`
	State       string              `json:"state"`
	Reason      string              `json:"reason,omitempty"`
	TaskID      string              `json:"task_id,omitempty"`
	EndedAt     *time.Time          `json:"ended_at,omitempty"`
	Worktree    delegation.Worktree `json:"worktree"`
	// Gate is the latest attempt's merge-gate record and Setup the setup
	// run that prepared the worktree (Stage 12, 2026-10-04).
	Gate  *delegation.BuildGate `json:"gate,omitempty"`
	Setup *delegation.GateRun   `json:"setup,omitempty"`
}

// ListBuildWorktrees lists the worktree of every build attempt, original
// assignments only (a continuation shares its original's worktree), newest
// first, with the state of the latest attempt on it.
func (s *Store) ListBuildWorktrees(ctx context.Context) ([]BuildWorktree, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT record_json FROM subagent_executions WHERE CASE WHEN json_valid(record_json) THEN json_type(record_json,'$.worktree') IS NOT NULL END ORDER BY rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	latest := map[string]BuildWorktree{}
	var order []string
	for rows.Next() {
		a, err := readSubagent(rows)
		if err != nil || a.Worktree == nil {
			continue
		}
		key := a.Worktree.Path
		if entry, seen := latest[key]; seen {
			if entry.Setup == nil && a.Gate != nil && a.Gate.Setup != nil {
				entry.Setup = a.Gate.Setup
				latest[key] = entry
			}
			continue
		}
		entry := BuildWorktree{ExecutionID: a.ID, State: a.State, TaskID: a.Assignment.TaskID, EndedAt: a.EndedAt, Worktree: *a.Worktree, Gate: a.Gate}
		if a.Gate != nil {
			entry.Setup = a.Gate.Setup
		}
		if a.Result != nil {
			entry.Reason = a.Result.Reason
		}
		latest[key] = entry
		order = append(order, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]BuildWorktree, 0, len(order))
	for _, key := range order {
		out = append(out, latest[key])
	}
	return out, nil
}

// ErrBuildWorktreeBusy refuses to clean up a worktree while an attempt on
// it is unfinished.
var ErrBuildWorktreeBusy = errors.New("the build worktree has an unfinished execution")

// BuildWorktreeFor returns the worktree of execution id, refusing one that
// any unfinished attempt still uses.
func (s *Store) BuildWorktreeFor(ctx context.Context, id string) (delegation.Worktree, error) {
	a, err := readSubagent(s.db.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return delegation.Worktree{}, fmt.Errorf("no execution %q", id)
	}
	if err != nil {
		return delegation.Worktree{}, err
	}
	if a.Worktree == nil {
		return delegation.Worktree{}, fmt.Errorf("execution %q is not a build execution", id)
	}
	var busy bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM subagent_executions WHERE child_session_id=? AND state IN ('admitted','running'))`, a.Child.ID).Scan(&busy); err != nil {
		return delegation.Worktree{}, err
	}
	if busy {
		return delegation.Worktree{}, fmt.Errorf("%w: execution %q", ErrBuildWorktreeBusy, id)
	}
	return *a.Worktree, nil
}

// MarkBuildWorktreeRemoved records that the owner removed the worktree of
// execution id's child, on every attempt of that child, so no continuation
// can run in it again.
func (s *Store) MarkBuildWorktreeRemoved(ctx context.Context, id string) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
		if err != nil {
			return err
		}
		if a.Worktree == nil {
			return fmt.Errorf("execution %q is not a build execution", id)
		}
		rows, err := conn.QueryContext(ctx, `SELECT record_json FROM subagent_executions WHERE child_session_id=?`, a.Child.ID)
		if err != nil {
			return err
		}
		var attempts []delegation.Attempt
		for rows.Next() {
			attempt, err := readSubagent(rows)
			if err != nil {
				rows.Close()
				return err
			}
			attempts = append(attempts, attempt)
		}
		rows.Close()
		now := s.now().UTC()
		for _, attempt := range attempts {
			if !attempt.Terminal() {
				return fmt.Errorf("%w: execution %q", ErrBuildWorktreeBusy, attempt.ID)
			}
			if attempt.Worktree == nil || attempt.Worktree.RemovedAt != nil {
				continue
			}
			attempt.Worktree.RemovedAt = &now
			if err := writeSubagent(ctx, conn, attempt); err != nil {
				return err
			}
		}
		return nil
	})
}
