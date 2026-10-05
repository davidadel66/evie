package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/google/uuid"
)

// Workflow activation and Standing Authority (Stage 14 sub-stage 2 of
// docs/memory-architecture.md, 2026-10-04; workflow-runtime.spec.md, ADR
// 0008, 0011, 0020, 0027, 0033). Procedural Git holds each proposed
// definition's files at a commit; SQLite holds what was proposed with its
// validation report and dry run, and what the owner approved:
//
//   - workspace_access_revisions: the owner's Workspace Access, the ceiling
//     of Connection IDs and exact resources, one immutable revision per
//     change, the latest applying at once;
//   - workflow_proposals: one row per proposal (commit, executable and
//     authority hashes, report, simulation), immutable;
//   - workflow_activations: one row per approval, version 1, 2, ... per
//     definition, binding one proposal's exact hashes, requested authority,
//     worker presets and schedule, pinned to the Workspace revision and
//     access revision it was checked against, with the approver's surface;
//     a later version supersedes an earlier one, and nothing is rewritten;
//   - workflow_deactivations: the owner ending one version (revocation).
//
// Blocking is derived, never stored: an activation is blocked while its
// Workspace is archived, while the Workspace's worker allowance (pinned and
// current revision, and every revocation of the pinned one) no longer
// covers its agent steps, or once any later access revision failed to cover
// its authority. Every input is immutable history, so a revocation stays in
// force after access is restored, exactly as for chats (ADR 0033): only a
// new approval clears it. Run admission (sub-stage 3) calls
// authorizeWorkflowActivation in its own transaction.

var (
	ErrWorkspaceInactive          = errors.New("the Workspace is archived")
	ErrWorkspaceAccessChanged     = errors.New("Workspace Access changed; refresh and try again")
	ErrWorkflowProposal           = errors.New("invalid workflow proposal")
	ErrWorkflowProposalNotFound   = errors.New("workflow proposal not found")
	ErrWorkflowNotFound           = errors.New("workflow definition not found")
	ErrWorkflowChanged            = errors.New("the workflow's current version changed since this review; review it again")
	ErrWorkflowHashMismatch       = errors.New("approval must name the proposal's exact executable hash and authority hash")
	ErrWorkflowSimulationRequired = errors.New("approval needs a dry run of this exact proposal that succeeded; propose it with fixtures")
	ErrWorkflowAlreadyActive      = errors.New("this exact version is already active")
	ErrWorkflowApprover           = errors.New("only the owner approves or deactivates workflows, from the CLI or the web")
	ErrWorkflowRefused            = errors.New("workflow approval refused")
	ErrWorkflowNotActive          = errors.New("workflow is not active")
	ErrWorkflowBlocked            = errors.New("workflow is blocked")
)

// WorkflowRefusal lists why an approval cannot activate: requested
// authority outside Workspace Access, a worker preset the Workspace does not
// allow, or an archived Workspace.
type WorkflowRefusal struct{ Reasons []string }

func (e *WorkflowRefusal) Error() string {
	return "workflow approval refused: " + strings.Join(e.Reasons, "; ")
}
func (e *WorkflowRefusal) Unwrap() error { return ErrWorkflowRefused }

// WorkflowBlockedError lists why an approved version cannot start runs.
type WorkflowBlockedError struct{ Reasons []string }

func (e *WorkflowBlockedError) Error() string {
	return "workflow is blocked: " + strings.Join(e.Reasons, "; ")
}
func (e *WorkflowBlockedError) Unwrap() error { return ErrWorkflowBlocked }

// WorkflowStatus is a definition's current state.
type WorkflowStatus string

const (
	// WorkflowProposed: proposals exist and nothing was ever approved.
	WorkflowProposed WorkflowStatus = "proposed"
	// WorkflowActive: the latest version is approved and may start runs.
	WorkflowActive WorkflowStatus = "active"
	// WorkflowBlocked: the latest version is approved but a revocation or
	// archive stops new runs; a new approval is needed.
	WorkflowBlocked WorkflowStatus = "blocked"
	// WorkflowInactive: the owner deactivated the latest version.
	WorkflowInactive WorkflowStatus = "inactive"
)

// WorkspaceAccessRevision is one revision of a Workspace's access ceiling.
// Revision 0 is the empty access every Workspace starts with.
type WorkspaceAccessRevision struct {
	WorkspaceID memory.WorkspaceID       `json:"workspace_id"`
	Revision    int64                    `json:"revision"`
	Access      workflow.WorkspaceAccess `json:"access"`
	RecordedAt  time.Time                `json:"recorded_at,omitzero"`
}

// WorkflowProposal is one proposed definition version: its files at a
// procedural Git commit, with the review report and dry run that attach to
// it. DocumentsVersion is set when the executable hash equals the active
// version's: the proposal changes documentation only and needs no approval.
type WorkflowProposal struct {
	ID                string                     `json:"id"`
	WorkspaceID       memory.WorkspaceID         `json:"workspace_id"`
	Name              string                     `json:"name"`
	Commit            string                     `json:"commit"`
	Path              string                     `json:"path"`
	ExecutableHash    string                     `json:"executable_hash"`
	AuthorityHash     string                     `json:"authority_hash"`
	Report            workflow.Report            `json:"report"`
	Simulation        *workflow.Simulation       `json:"simulation,omitempty"`
	ProposedBy        string                     `json:"proposed_by"`
	Surface           string                     `json:"surface"`
	WorkspaceRevision memory.WorkspaceRevisionID `json:"workspace_revision"`
	AccessRevision    int64                      `json:"access_revision"`
	DocumentsVersion  int64                      `json:"documents_version,omitempty"`
	ProposedAt        time.Time                  `json:"proposed_at"`
	// Source is "global" for a Global custom definition's proposal (no
	// Workspace); empty for a Workspace's.
	Source WorkflowSource `json:"source,omitempty"`
}

// WorkflowApprover is the evidence of who approved, and where. Only the
// owner approves, from the CLI or the web management API; no model tool or
// chat surface exists for it.
type WorkflowApprover struct {
	Actor   string `json:"actor"`
	Surface string `json:"surface"`
	Detail  string `json:"detail,omitempty"`
}

func (a WorkflowApprover) valid() bool {
	return a.Actor == "owner" && (a.Surface == "cli" || a.Surface == "web") && len(a.Detail) <= 200
}

// WorkflowApproval names exactly what the owner reviewed: the proposal, its
// executable and authority hashes, and the version the review was compared
// with (0 when none was approved).
type WorkflowApproval struct {
	ProposalID      string           `json:"proposal_id"`
	ExecutableHash  string           `json:"executable_hash"`
	AuthorityHash   string           `json:"authority_hash"`
	ExpectedVersion int64            `json:"expected_version"`
	Approver        WorkflowApprover `json:"approver"`
}

// WorkflowActivation is one approved version. Its Standing Authority,
// worker presets and schedule are bound with it; the schedule is recorded
// and starts runs once schedules ship (sub-stage 11).
type WorkflowActivation struct {
	WorkspaceID       memory.WorkspaceID         `json:"workspace_id"`
	Name              string                     `json:"name"`
	Version           int64                      `json:"version"`
	ProposalID        string                     `json:"proposal_id"`
	Commit            string                     `json:"commit"`
	Path              string                     `json:"path"`
	ExecutableHash    string                     `json:"executable_hash"`
	AuthorityHash     string                     `json:"authority_hash"`
	Authority         []workflow.AuthorityReport `json:"authority"`
	Workers           []string                   `json:"workers"`
	Schedule          *workflow.ScheduleReport   `json:"schedule,omitempty"`
	WorkspaceRevision memory.WorkspaceRevisionID `json:"workspace_revision"`
	AccessRevision    int64                      `json:"access_revision"`
	Approver          WorkflowApprover           `json:"approver"`
	ApprovedAt        time.Time                  `json:"approved_at"`
	Deactivated       *WorkflowDeactivation      `json:"deactivated,omitempty"`
	// Source is "global" for a Global custom version and "builtin" for a
	// built-in version (approved by no one: it holds no authority); empty
	// for a Workspace's approved version.
	Source WorkflowSource `json:"source,omitempty"`
}

// WorkflowDeactivation is the owner ending one version.
type WorkflowDeactivation struct {
	Reason  string    `json:"reason"`
	Surface string    `json:"surface"`
	At      time.Time `json:"at"`
}

// WorkflowProposalSummary is one proposal in a definition's history.
type WorkflowProposalSummary struct {
	ID               string    `json:"id"`
	Commit           string    `json:"commit"`
	ExecutableHash   string    `json:"executable_hash"`
	AuthorityHash    string    `json:"authority_hash"`
	SimulationStatus string    `json:"simulation_status"`
	DocumentsVersion int64     `json:"documents_version,omitempty"`
	ApprovedVersions []int64   `json:"approved_versions,omitempty"`
	Pending          bool      `json:"pending"`
	ProposedBy       string    `json:"proposed_by"`
	ProposedAt       time.Time `json:"proposed_at"`
}

// WorkflowDefinitionState is a definition's status, with its history when
// read on its own (newest first).
type WorkflowDefinitionState struct {
	WorkspaceID memory.WorkspaceID        `json:"workspace_id"`
	Name        string                    `json:"name"`
	Status      WorkflowStatus            `json:"status"`
	Blocked     []string                  `json:"blocked,omitempty"`
	Current     *WorkflowActivation       `json:"current,omitempty"`
	Pending     int                       `json:"pending"`
	Versions    []WorkflowActivation      `json:"versions,omitempty"`
	Proposals   []WorkflowProposalSummary `json:"proposals,omitempty"`
	// Source is "global" for a Global custom definition; empty for a
	// Workspace's.
	Source WorkflowSource `json:"source,omitempty"`
}

func ensureWorkflowSchema(ctx context.Context, db *sql.DB) error {
	immutable := func(table, what string) string {
		return fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_no_update BEFORE UPDATE ON %[1]s
 BEGIN SELECT RAISE(ABORT,'%[2]s are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS %[1]s_no_delete BEFORE DELETE ON %[1]s
 BEGIN SELECT RAISE(ABORT,'%[2]s are immutable'); END;`, table, what)
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workspace_access_revisions (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 revision INTEGER NOT NULL CHECK(revision > 0),
 access TEXT NOT NULL CHECK(json_valid(access)),
 recorded_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id, revision));
 `+immutable("workspace_access_revisions", "Workspace Access revisions")+`
 CREATE TABLE IF NOT EXISTS workflow_proposals (
 id TEXT PRIMARY KEY,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
 commit_id TEXT NOT NULL,
 tree_path TEXT NOT NULL,
 executable_hash TEXT NOT NULL,
 authority_hash TEXT NOT NULL,
 report TEXT NOT NULL CHECK(json_valid(report)),
 simulation TEXT CHECK(simulation IS NULL OR json_valid(simulation)),
 simulation_status TEXT NOT NULL CHECK(simulation_status IN ('none','succeeded','failed','waiting_for_human')),
 proposed_by TEXT NOT NULL CHECK(proposed_by IN ('owner','evie')),
 surface TEXT NOT NULL CHECK(surface IN ('cli','web','chat')),
 workspace_revision_id TEXT NOT NULL,
 access_revision INTEGER NOT NULL CHECK(access_revision >= 0),
 documents_version INTEGER NOT NULL CHECK(documents_version >= 0),
 proposed_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS workflow_proposals_by_definition ON workflow_proposals(workspace_id, name, proposed_at);
 `+immutable("workflow_proposals", "workflow proposals")+`
 CREATE TABLE IF NOT EXISTS workflow_activations (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 name TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version > 0),
 proposal_id TEXT NOT NULL REFERENCES workflow_proposals(id),
 commit_id TEXT NOT NULL,
 tree_path TEXT NOT NULL,
 executable_hash TEXT NOT NULL,
 authority_hash TEXT NOT NULL,
 authority TEXT NOT NULL CHECK(json_valid(authority) AND json_type(authority) = 'array'),
 workers TEXT NOT NULL CHECK(json_valid(workers) AND json_type(workers) = 'array'),
 schedule TEXT CHECK(schedule IS NULL OR json_valid(schedule)),
 workspace_revision_id TEXT NOT NULL,
 access_revision INTEGER NOT NULL CHECK(access_revision >= 0),
 approver TEXT NOT NULL CHECK(approver = 'owner'),
 approval_surface TEXT NOT NULL CHECK(approval_surface IN ('cli','web')),
 approval_detail TEXT NOT NULL,
 approved_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id, name, version));
 `+immutable("workflow_activations", "workflow activations")+`
 CREATE TABLE IF NOT EXISTS workflow_deactivations (
 workspace_id TEXT NOT NULL,
 name TEXT NOT NULL,
 version INTEGER NOT NULL,
 reason TEXT NOT NULL,
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 deactivated_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id, name, version),
 FOREIGN KEY(workspace_id, name, version) REFERENCES workflow_activations(workspace_id, name, version));
 `+immutable("workflow_deactivations", "workflow deactivations"))
	if err != nil {
		return fmt.Errorf("create workflow activation schema: %w", err)
	}
	return ensureWorkflowBuiltinSchema(ctx, db)
}

var (
	workflowNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	workflowHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	gitCommitPattern    = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
)

// WorkflowDefinitionPath is where a definition lives in the procedural
// repository: workspaces/<workspace-id>/workflows/<name>.
func WorkflowDefinitionPath(ws memory.WorkspaceID, name string) string {
	return "workspaces/" + string(ws) + "/workflows/" + name
}

type workspaceRow struct {
	state    memory.WorkspaceState
	revision memory.WorkspaceRevisionID
}

func readWorkspaceRow(ctx context.Context, q queryRower, id memory.WorkspaceID) (workspaceRow, error) {
	var w workspaceRow
	err := q.QueryRowContext(ctx, `SELECT lifecycle_state,current_revision_id FROM workspaces WHERE id=?`, id).Scan(&w.state, &w.revision)
	if errors.Is(err, sql.ErrNoRows) {
		return w, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
	}
	return w, err
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(text string) (time.Time, error) { return time.Parse(time.RFC3339Nano, text) }

// WorkspaceAccess is the Workspace's current access ceiling.
func (s *Store) WorkspaceAccess(ctx context.Context, id memory.WorkspaceID) (WorkspaceAccessRevision, error) {
	if _, err := readWorkspaceRow(ctx, s.db, id); err != nil {
		return WorkspaceAccessRevision{}, err
	}
	return workspaceAccess(ctx, s.db, id)
}

func workspaceAccess(ctx context.Context, q queryRower, id memory.WorkspaceID) (WorkspaceAccessRevision, error) {
	out := WorkspaceAccessRevision{WorkspaceID: id, Access: workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{}}}
	var encoded, recorded string
	err := q.QueryRowContext(ctx, `SELECT revision,access,recorded_at FROM workspace_access_revisions
 WHERE workspace_id=? ORDER BY revision DESC LIMIT 1`, id).Scan(&out.Revision, &encoded, &recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read Workspace Access: %w", err)
	}
	if err := json.Unmarshal([]byte(encoded), &out.Access); err != nil {
		return out, fmt.Errorf("decode Workspace Access: %w", err)
	}
	out.RecordedAt, err = parseTime(recorded)
	return out, err
}

// SetWorkspaceAccess records the owner's Workspace Access as a new revision
// when expected is still the latest. It applies at once: a removal blocks
// every approved workflow whose authority it no longer covers. Access grants
// nothing by itself; only a Workflow Approval within it does.
func (s *Store) SetWorkspaceAccess(ctx context.Context, id memory.WorkspaceID, expected int64, access workflow.WorkspaceAccess) (WorkspaceAccessRevision, error) {
	normalized, err := workflow.NormalizeAccess(access)
	if err != nil {
		return WorkspaceAccessRevision{}, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return WorkspaceAccessRevision{}, err
	}
	var out WorkspaceAccessRevision
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		w, err := readWorkspaceRow(ctx, conn, id)
		if err != nil {
			return err
		}
		if w.state != memory.WorkspaceActive {
			return ErrWorkspaceInactive
		}
		current, err := workspaceAccess(ctx, conn, id)
		if err != nil {
			return err
		}
		if current.Revision != expected {
			return ErrWorkspaceAccessChanged
		}
		now := s.now().UTC()
		if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_access_revisions(workspace_id,revision,access,recorded_at) VALUES(?,?,?,?)`,
			id, expected+1, string(encoded), formatTime(now)); err != nil {
			return fmt.Errorf("record Workspace Access: %w", err)
		}
		out = WorkspaceAccessRevision{WorkspaceID: id, Revision: expected + 1, Access: normalized, RecordedAt: now}
		return nil
	})
	return out, err
}

func (p WorkflowProposal) check() error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrWorkflowProposal, fmt.Sprintf(format, args...))
	}
	switch {
	case !workflowNamePattern.MatchString(p.Name) || p.Name != p.Report.Workflow:
		return bad("name %q is not the report's %q", p.Name, p.Report.Workflow)
	case !workflowHashPattern.MatchString(p.ExecutableHash) || p.ExecutableHash != p.Report.ExecutableHash:
		return bad("executable hash %s is not the report's", p.ExecutableHash)
	case p.AuthorityHash != workflow.RequirementsOf(p.Report).AuthorityHash:
		return bad("authority hash %s is not the report's", p.AuthorityHash)
	case !gitCommitPattern.MatchString(p.Commit):
		return bad("%q is not a procedural Git commit", p.Commit)
	case p.Path != WorkflowDefinitionPath(p.WorkspaceID, p.Name):
		return bad("path %q is not %s", p.Path, WorkflowDefinitionPath(p.WorkspaceID, p.Name))
	case p.ProposedBy != "owner" && p.ProposedBy != "evie":
		return bad("proposer %q is not the owner or Evie", p.ProposedBy)
	case p.Surface != "cli" && p.Surface != "web" && p.Surface != "chat":
		return bad("surface %q is unknown", p.Surface)
	case p.Simulation != nil && (p.Simulation.ExecutableHash != p.ExecutableHash || p.Simulation.Workflow != p.Name):
		return bad("the simulation was not run on this definition")
	}
	return nil
}

// RecordWorkflowProposal records a proposal whose files are at p.Commit in
// procedural Git. It assigns the ID and time, pins the Workspace revision
// and access it was proposed under, and marks it as documentation of the
// active version when its executable hash is that version's.
func (s *Store) RecordWorkflowProposal(ctx context.Context, p WorkflowProposal) (WorkflowProposal, error) {
	if err := p.check(); err != nil {
		return WorkflowProposal{}, err
	}
	report, err := json.Marshal(p.Report)
	if err != nil {
		return WorkflowProposal{}, err
	}
	var simulation any
	status := "none"
	if p.Simulation != nil {
		data, err := json.Marshal(p.Simulation)
		if err != nil {
			return WorkflowProposal{}, err
		}
		simulation, status = string(data), p.Simulation.Status
	}
	p.ID = uuid.NewString()
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		w, err := readWorkspaceRow(ctx, conn, p.WorkspaceID)
		if err != nil {
			return err
		}
		if w.state != memory.WorkspaceActive {
			return ErrWorkspaceInactive
		}
		access, err := workspaceAccess(ctx, conn, p.WorkspaceID)
		if err != nil {
			return err
		}
		latest, ok, err := latestActivation(ctx, conn, p.WorkspaceID, p.Name)
		if err != nil {
			return err
		}
		p.DocumentsVersion = 0
		if ok && latest.Deactivated == nil && latest.ExecutableHash == p.ExecutableHash {
			p.DocumentsVersion = latest.Version
		}
		p.WorkspaceRevision, p.AccessRevision = w.revision, access.Revision
		p.ProposedAt = s.now().UTC()
		_, err = conn.ExecContext(ctx, `INSERT INTO workflow_proposals(id,workspace_id,name,commit_id,tree_path,executable_hash,authority_hash,report,simulation,simulation_status,proposed_by,surface,workspace_revision_id,access_revision,documents_version,proposed_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, p.WorkspaceID, p.Name, p.Commit, p.Path, p.ExecutableHash, p.AuthorityHash, string(report), simulation, status,
			p.ProposedBy, p.Surface, p.WorkspaceRevision, p.AccessRevision, p.DocumentsVersion, formatTime(p.ProposedAt))
		if err != nil {
			return fmt.Errorf("record workflow proposal: %w", err)
		}
		return nil
	})
	if err != nil {
		return WorkflowProposal{}, err
	}
	return p, nil
}

const proposalColumns = `id,workspace_id,name,commit_id,tree_path,executable_hash,authority_hash,report,simulation,proposed_by,surface,workspace_revision_id,access_revision,documents_version,proposed_at`

func scanWorkflowProposal(scanner rowScanner) (WorkflowProposal, error) {
	var p WorkflowProposal
	var report, proposed string
	var simulation sql.NullString
	if err := scanner.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Commit, &p.Path, &p.ExecutableHash, &p.AuthorityHash, &report, &simulation,
		&p.ProposedBy, &p.Surface, &p.WorkspaceRevision, &p.AccessRevision, &p.DocumentsVersion, &proposed); err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(report), &p.Report); err != nil {
		return p, fmt.Errorf("decode workflow report: %w", err)
	}
	if simulation.Valid {
		p.Simulation = &workflow.Simulation{}
		if err := json.Unmarshal([]byte(simulation.String), p.Simulation); err != nil {
			return p, fmt.Errorf("decode workflow simulation: %w", err)
		}
	}
	var err error
	p.ProposedAt, err = parseTime(proposed)
	return p, err
}

// WorkflowProposal reads one proposal.
func (s *Store) WorkflowProposal(ctx context.Context, id string) (WorkflowProposal, error) {
	return workflowProposal(ctx, s.db, id)
}

func workflowProposal(ctx context.Context, q queryRower, id string) (WorkflowProposal, error) {
	p, err := scanWorkflowProposal(q.QueryRowContext(ctx, `SELECT `+proposalColumns+` FROM workflow_proposals WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("%w: %s", ErrWorkflowProposalNotFound, id)
	}
	return p, err
}

const activationColumns = `a.workspace_id,a.name,a.version,a.proposal_id,a.commit_id,a.tree_path,a.executable_hash,a.authority_hash,a.authority,a.workers,a.schedule,
 a.workspace_revision_id,a.access_revision,a.approver,a.approval_surface,a.approval_detail,a.approved_at,d.reason,d.surface,d.deactivated_at`

const activationFrom = ` FROM workflow_activations a LEFT JOIN workflow_deactivations d
 ON d.workspace_id=a.workspace_id AND d.name=a.name AND d.version=a.version`

func scanWorkflowActivation(scanner rowScanner) (WorkflowActivation, error) {
	var a WorkflowActivation
	var authority, workers, approved string
	var schedule, reason, surface, deactivated sql.NullString
	if err := scanner.Scan(&a.WorkspaceID, &a.Name, &a.Version, &a.ProposalID, &a.Commit, &a.Path, &a.ExecutableHash, &a.AuthorityHash, &authority, &workers, &schedule,
		&a.WorkspaceRevision, &a.AccessRevision, &a.Approver.Actor, &a.Approver.Surface, &a.Approver.Detail, &approved, &reason, &surface, &deactivated); err != nil {
		return a, err
	}
	if err := json.Unmarshal([]byte(authority), &a.Authority); err != nil {
		return a, fmt.Errorf("decode workflow authority: %w", err)
	}
	if err := json.Unmarshal([]byte(workers), &a.Workers); err != nil {
		return a, fmt.Errorf("decode workflow workers: %w", err)
	}
	if schedule.Valid {
		a.Schedule = &workflow.ScheduleReport{}
		if err := json.Unmarshal([]byte(schedule.String), a.Schedule); err != nil {
			return a, fmt.Errorf("decode workflow schedule: %w", err)
		}
	}
	var err error
	if a.ApprovedAt, err = parseTime(approved); err != nil {
		return a, err
	}
	if deactivated.Valid {
		at, err := parseTime(deactivated.String)
		if err != nil {
			return a, err
		}
		a.Deactivated = &WorkflowDeactivation{Reason: reason.String, Surface: surface.String, At: at}
	}
	return a, nil
}

func latestActivation(ctx context.Context, q queryRower, ws memory.WorkspaceID, name string) (WorkflowActivation, bool, error) {
	a, err := scanWorkflowActivation(q.QueryRowContext(ctx, `SELECT `+activationColumns+activationFrom+`
 WHERE a.workspace_id=? AND a.name=? ORDER BY a.version DESC LIMIT 1`, ws, name))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkflowActivation{}, false, nil
	}
	return a, err == nil, err
}

func activationAt(ctx context.Context, q queryRower, ws memory.WorkspaceID, name string, version int64) (WorkflowActivation, error) {
	return scanWorkflowActivation(q.QueryRowContext(ctx, `SELECT `+activationColumns+activationFrom+`
 WHERE a.workspace_id=? AND a.name=? AND a.version=?`, ws, name, version))
}

// workerReason explains a worker preset the Workspace does not allow.
func workerReason(worker string, now bool) string {
	if now {
		return fmt.Sprintf("agent steps run %s workers, which this Workspace does not allow; allow them in the Workspace's settings first", worker)
	}
	return fmt.Sprintf("agent steps run %s workers, which this Workspace no longer allows for this approval; allow them and approve again", worker)
}

// activationBlocks lists why an approved version cannot start runs now. Every
// input is immutable history or current state, so the answer cannot be
// missed by any path that changes a Workspace.
func activationBlocks(ctx context.Context, conn *sql.Conn, a WorkflowActivation) ([]string, error) {
	w, err := readWorkspaceRow(ctx, conn, a.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if w.state != memory.WorkspaceActive {
		return []string{"the Workspace is archived"}, nil
	}
	var reasons []string
	for _, worker := range a.Workers {
		err := authorizeWorkspacePreset(ctx, conn, memory.ScopeContext{WorkspaceID: a.WorkspaceID, WorkspaceRevision: a.WorkspaceRevision}, worker)
		if errors.Is(err, delegation.ErrAuthority) {
			reasons = append(reasons, workerReason(worker, false))
		} else if err != nil {
			return nil, err
		}
	}
	if len(a.Authority) == 0 {
		return reasons, nil
	}
	rows, err := conn.QueryContext(ctx, `SELECT revision,access FROM workspace_access_revisions WHERE workspace_id=? AND revision>? ORDER BY revision`, a.WorkspaceID, a.AccessRevision)
	if err != nil {
		return nil, err
	}
	type revision struct {
		number int64
		access string
	}
	var later []revision
	for rows.Next() {
		var r revision
		if err := rows.Scan(&r.number, &r.access); err != nil {
			rows.Close()
			return nil, err
		}
		later = append(later, r)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, r := range later {
		var access workflow.WorkspaceAccess
		if err := json.Unmarshal([]byte(r.access), &access); err != nil {
			return nil, err
		}
		if outside := workflow.AuthorityOutside(a.Authority, access); len(outside) > 0 {
			reasons = append(reasons, fmt.Sprintf("Workspace Access revision %d revoked part of its authority (%s); approve it again within the current access", r.number, strings.Join(outside, "; ")))
			break
		}
	}
	return reasons, nil
}

// ActivateWorkflow records the owner's approval of one exact proposal as the
// definition's next version. In one transaction it requires the reviewed
// hashes, a dry run of the proposal that succeeded, the version the review
// compared with, an active Workspace, the Workspace's current allowance for
// every worker preset its agent steps run, and requested authority within
// current Workspace Access. It starts no run.
func (s *Store) ActivateWorkflow(ctx context.Context, approval WorkflowApproval) (WorkflowActivation, error) {
	if !approval.Approver.valid() {
		return WorkflowActivation{}, ErrWorkflowApprover
	}
	var out WorkflowActivation
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		p, err := workflowProposal(ctx, conn, approval.ProposalID)
		if err != nil {
			return err
		}
		if approval.ExecutableHash != p.ExecutableHash || approval.AuthorityHash != p.AuthorityHash {
			return ErrWorkflowHashMismatch
		}
		latest, ok, err := latestActivation(ctx, conn, p.WorkspaceID, p.Name)
		if err != nil {
			return err
		}
		if latest.Version != approval.ExpectedVersion {
			return ErrWorkflowChanged
		}
		if p.Simulation == nil || p.Simulation.Status != workflow.SimulationSucceeded || p.Simulation.ExecutableHash != p.ExecutableHash {
			return ErrWorkflowSimulationRequired
		}
		w, err := readWorkspaceRow(ctx, conn, p.WorkspaceID)
		if err != nil {
			return err
		}
		if w.state != memory.WorkspaceActive {
			return &WorkflowRefusal{Reasons: []string{"the Workspace is archived"}}
		}
		if ok && latest.Deactivated == nil && latest.ExecutableHash == p.ExecutableHash && latest.AuthorityHash == p.AuthorityHash {
			blocks, err := activationBlocks(ctx, conn, latest)
			if err != nil {
				return err
			}
			if len(blocks) == 0 {
				return ErrWorkflowAlreadyActive
			}
		}
		req := workflow.RequirementsOf(p.Report)
		var reasons []string
		for _, worker := range req.Workers {
			err := authorizeWorkspacePreset(ctx, conn, memory.ScopeContext{WorkspaceID: p.WorkspaceID, WorkspaceRevision: w.revision}, worker)
			if errors.Is(err, delegation.ErrAuthority) {
				reasons = append(reasons, workerReason(worker, true))
			} else if err != nil {
				return err
			}
		}
		access, err := workspaceAccess(ctx, conn, p.WorkspaceID)
		if err != nil {
			return err
		}
		reasons = append(reasons, workflow.AuthorityOutside(req.Authority, access.Access)...)
		if len(reasons) > 0 {
			return &WorkflowRefusal{Reasons: reasons}
		}
		authority, err := json.Marshal(req.Authority)
		if err != nil {
			return err
		}
		workers, err := json.Marshal(req.Workers)
		if err != nil {
			return err
		}
		var schedule any
		if req.Schedule != nil {
			data, err := json.Marshal(req.Schedule)
			if err != nil {
				return err
			}
			schedule = string(data)
		}
		version := latest.Version + 1
		_, err = conn.ExecContext(ctx, `INSERT INTO workflow_activations(workspace_id,name,version,proposal_id,commit_id,tree_path,executable_hash,authority_hash,authority,workers,schedule,workspace_revision_id,access_revision,approver,approval_surface,approval_detail,approved_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.WorkspaceID, p.Name, version, p.ID, p.Commit, p.Path, p.ExecutableHash, p.AuthorityHash, string(authority), string(workers), schedule,
			w.revision, access.Revision, approval.Approver.Actor, approval.Approver.Surface, approval.Approver.Detail, formatTime(s.now()))
		if err != nil {
			return fmt.Errorf("record workflow activation: %w", err)
		}
		out, err = activationAt(ctx, conn, p.WorkspaceID, p.Name, version)
		return err
	})
	return out, err
}

// DeactivateWorkflow is the owner revoking a definition's Standing
// Authority: it ends version, which must be the latest and still active.
// No new run starts; the version stays in history.
func (s *Store) DeactivateWorkflow(ctx context.Context, ws memory.WorkspaceID, name string, version int64, reason, surface string) (WorkflowActivation, error) {
	if surface != "cli" && surface != "web" {
		return WorkflowActivation{}, ErrWorkflowApprover
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 1000 {
		return WorkflowActivation{}, errors.New("a deactivation reason is at most 1000 bytes")
	}
	var out WorkflowActivation
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		latest, ok, err := latestActivation(ctx, conn, ws, name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %s", ErrWorkflowNotFound, name)
		}
		if latest.Version != version {
			return ErrWorkflowChanged
		}
		if latest.Deactivated != nil {
			return ErrWorkflowNotActive
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_deactivations(workspace_id,name,version,reason,surface,deactivated_at) VALUES(?,?,?,?,?,?)`,
			ws, name, version, reason, surface, formatTime(s.now())); err != nil {
			return fmt.Errorf("record workflow deactivation: %w", err)
		}
		out, err = activationAt(ctx, conn, ws, name, version)
		return err
	})
	return out, err
}

// ActiveWorkflow returns the version a new run of the definition would pin,
// or why there is none: ErrWorkflowNotFound, ErrWorkflowNotActive, or a
// *WorkflowBlockedError.
func (s *Store) ActiveWorkflow(ctx context.Context, ws memory.WorkspaceID, name string) (WorkflowActivation, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return WorkflowActivation{}, err
	}
	defer conn.Close()
	return authorizeWorkflowActivation(ctx, conn, ws, name)
}

// authorizeWorkflowActivation is the run-admission check: run start
// (sub-stage 3) calls it inside the transaction that records the run.
func authorizeWorkflowActivation(ctx context.Context, conn *sql.Conn, ws memory.WorkspaceID, name string) (WorkflowActivation, error) {
	latest, ok, err := latestActivation(ctx, conn, ws, name)
	if err != nil {
		return WorkflowActivation{}, err
	}
	if !ok {
		var proposed bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_proposals WHERE workspace_id=? AND name=?)`, ws, name).Scan(&proposed); err != nil {
			return WorkflowActivation{}, err
		}
		if proposed {
			return WorkflowActivation{}, fmt.Errorf("%w: %s has never been approved", ErrWorkflowNotActive, name)
		}
		return WorkflowActivation{}, fmt.Errorf("%w: %s", ErrWorkflowNotFound, name)
	}
	if latest.Deactivated != nil {
		return WorkflowActivation{}, fmt.Errorf("%w: version %d was deactivated", ErrWorkflowNotActive, latest.Version)
	}
	blocks, err := activationBlocks(ctx, conn, latest)
	if err != nil {
		return WorkflowActivation{}, err
	}
	if len(blocks) > 0 {
		return WorkflowActivation{}, &WorkflowBlockedError{Reasons: blocks}
	}
	return latest, nil
}

// WorkflowDefinition reads one definition's status and full history.
func (s *Store) WorkflowDefinition(ctx context.Context, ws memory.WorkspaceID, name string) (WorkflowDefinitionState, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return WorkflowDefinitionState{}, err
	}
	defer conn.Close()
	return readDefinitionState(ctx, conn, ws, name, true)
}

// ListWorkflowDefinitions lists every definition in a Workspace, or in
// every Workspace when ws is empty, with its status, by Workspace and name.
func (s *Store) ListWorkflowDefinitions(ctx context.Context, ws memory.WorkspaceID) ([]WorkflowDefinitionState, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	query := `SELECT DISTINCT workspace_id,name FROM workflow_proposals`
	args := []any{}
	if ws != "" {
		query += ` WHERE workspace_id=?`
		args = append(args, ws)
	}
	rows, err := conn.QueryContext(ctx, query+` ORDER BY workspace_id,name`, args...)
	if err != nil {
		return nil, err
	}
	type key struct {
		ws   memory.WorkspaceID
		name string
	}
	var keys []key
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.ws, &k.name); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, k)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []WorkflowDefinitionState{}
	for _, k := range keys {
		d, err := readDefinitionState(ctx, conn, k.ws, k.name, false)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func readDefinitionState(ctx context.Context, conn *sql.Conn, ws memory.WorkspaceID, name string, detail bool) (WorkflowDefinitionState, error) {
	d := WorkflowDefinitionState{WorkspaceID: ws, Name: name}
	rows, err := conn.QueryContext(ctx, `SELECT `+activationColumns+activationFrom+` WHERE a.workspace_id=? AND a.name=? ORDER BY a.version DESC`, ws, name)
	if err != nil {
		return d, err
	}
	var versions []WorkflowActivation
	for rows.Next() {
		a, err := scanWorkflowActivation(rows)
		if err != nil {
			rows.Close()
			return d, err
		}
		versions = append(versions, a)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	if err := rows.Err(); err != nil {
		return d, err
	}
	rows, err = conn.QueryContext(ctx, `SELECT id,commit_id,executable_hash,authority_hash,simulation_status,documents_version,proposed_by,proposed_at
 FROM workflow_proposals WHERE workspace_id=? AND name=? ORDER BY proposed_at DESC, rowid DESC`, ws, name)
	if err != nil {
		return d, err
	}
	var proposals []WorkflowProposalSummary
	for rows.Next() {
		var p WorkflowProposalSummary
		var proposed string
		if err := rows.Scan(&p.ID, &p.Commit, &p.ExecutableHash, &p.AuthorityHash, &p.SimulationStatus, &p.DocumentsVersion, &p.ProposedBy, &proposed); err != nil {
			rows.Close()
			return d, err
		}
		if p.ProposedAt, err = parseTime(proposed); err != nil {
			rows.Close()
			return d, err
		}
		proposals = append(proposals, p)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	if err := rows.Err(); err != nil {
		return d, err
	}
	if len(proposals) == 0 && len(versions) == 0 {
		return d, fmt.Errorf("%w: %s", ErrWorkflowNotFound, name)
	}
	var lastApproval time.Time
	if len(versions) > 0 {
		lastApproval = versions[0].ApprovedAt
	}
	for i := range proposals {
		p := &proposals[i]
		for j := len(versions) - 1; j >= 0; j-- {
			if versions[j].ProposalID == p.ID {
				p.ApprovedVersions = append(p.ApprovedVersions, versions[j].Version)
			}
		}
		p.Pending = len(p.ApprovedVersions) == 0 && p.DocumentsVersion == 0 && p.ProposedAt.After(lastApproval)
		if p.Pending {
			d.Pending++
		}
	}
	switch {
	case len(versions) == 0:
		d.Status = WorkflowProposed
	case versions[0].Deactivated != nil:
		d.Status = WorkflowInactive
		d.Current = &versions[0]
	default:
		d.Current = &versions[0]
		blocks, err := activationBlocks(ctx, conn, versions[0])
		if err != nil {
			return d, err
		}
		d.Status, d.Blocked = WorkflowActive, blocks
		if len(blocks) > 0 {
			d.Status = WorkflowBlocked
		}
	}
	if detail {
		d.Versions, d.Proposals = versions, proposals
	}
	return d, nil
}
