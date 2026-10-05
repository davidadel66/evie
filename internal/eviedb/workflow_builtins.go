package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/google/uuid"
)

// Built-in and Global workflows (owner decision 2026-10-04; the "Built-in
// workflows" amendment of workflow-runtime.spec.md and its entry in
// workflow-runtime.decisions.md). Workflow Approval exists to grant
// Standing Authority for unattended effects (ADR 0008); a definition that
// qualifies as built-in (workflow.QualifyBuiltin: no capability, no
// authority, no schedule, only the read-only research and explore workers)
// has nothing to grant, so it needs no approval:
//
//   - workflow_builtin_versions: each version of a built-in definition an
//     Evie ran, identified by its executable hash, with the procedural Git
//     commit holding its exact files (builtin/workflows/<name>), its review
//     report and its workers. Evie records it before the first run that
//     pins it; it is immutable, and a trigger refuses one with requested
//     authority, a capability, a schedule or a worker other than research
//     and explore.
//   - workflow_builtin_controls: the owner disabling and enabling a built-in
//     for every Workspace (CLI or web), immutable; the latest applies, and
//     none means enabled.
//   - workflow_global_proposals, _activations, _deactivations: the owner's
//     Global custom definitions (global/workflows/<name>), proposed,
//     approved and deactivated like a Workspace's, and qualified like a
//     built-in (the same trigger and checks).
//
// A run pins its source (workflow_runs.source: workspace, global or
// builtin), and a trigger requires its pinned version to exist in that
// source's table, in place of the foreign key to workflow_activations.
// Runs still belong to the Workspace they start in: its worker allowance
// and run admission apply at start and at every node.
//
// Resolution by name for a Workspace: the Workspace's custom version while
// its latest approval is not deactivated (a blocked one is the answer, with
// its blocks: it never falls through silently), else the Global custom
// version on the same terms, else the built-in when enabled.

// WorkflowSource is where the version a run pins comes from.
type WorkflowSource string

const (
	WorkflowSourceWorkspace WorkflowSource = "workspace"
	WorkflowSourceGlobal    WorkflowSource = "global"
	WorkflowSourceBuiltin   WorkflowSource = "builtin"
)

// workspaceSource reads an unset source as a Workspace version.
func workspaceSource(s WorkflowSource) WorkflowSource {
	if s == "" {
		return WorkflowSourceWorkspace
	}
	return s
}

var (
	// ErrWorkflowBuiltinDisabled refuses a run of a built-in the owner
	// disabled; it unwraps to ErrWorkflowNotActive.
	ErrWorkflowBuiltinDisabled = fmt.Errorf("%w: the owner disabled this built-in workflow", ErrWorkflowNotActive)
	// ErrWorkflowBuiltinUnchanged refuses a control that changes nothing.
	ErrWorkflowBuiltinUnchanged = errors.New("the built-in workflow is already in that state")
	// ErrWorkflowNotStandalone refuses a built-in or Global definition that
	// does not qualify (workflow.QualifyBuiltin).
	ErrWorkflowNotStandalone = errors.New("built-in and Global workflows hold no Standing Authority")
)

// GlobalWorkflowDefinitionPath is where a Global custom definition lives
// in the procedural repository.
func GlobalWorkflowDefinitionPath(name string) string { return "global/workflows/" + name }

// BuiltinWorkflowDefinitionPath is where Evie records a built-in version's
// exact files in the procedural repository.
func BuiltinWorkflowDefinitionPath(name string) string { return "builtin/workflows/" + name }

// WorkflowBuiltinVersion is one recorded version of a built-in.
type WorkflowBuiltinVersion struct {
	Name           string          `json:"name"`
	Version        int64           `json:"version"`
	Commit         string          `json:"commit"`
	Path           string          `json:"path"`
	ExecutableHash string          `json:"executable_hash"`
	AuthorityHash  string          `json:"authority_hash"`
	Workers        []string        `json:"workers"`
	Report         workflow.Report `json:"report"`
	RecordedAt     time.Time       `json:"recorded_at"`
}

// Activation is the version as a run start reads it.
func (v WorkflowBuiltinVersion) Activation(ws memory.WorkspaceID) WorkflowActivation {
	return WorkflowActivation{WorkspaceID: ws, Name: v.Name, Version: v.Version, Commit: v.Commit, Path: v.Path, ExecutableHash: v.ExecutableHash,
		AuthorityHash: v.AuthorityHash, Authority: []workflow.AuthorityReport{}, Workers: slices.Clone(v.Workers), Source: WorkflowSourceBuiltin,
		Approver: WorkflowApprover{Actor: "evie", Surface: "builtin", Detail: "built into Evie"}, ApprovedAt: v.RecordedAt}
}

// WorkflowBuiltinControl is the owner disabling or enabling a built-in.
type WorkflowBuiltinControl struct {
	Seq        int64     `json:"seq"`
	Name       string    `json:"name"`
	Enabled    bool      `json:"enabled"`
	Reason     string    `json:"reason,omitempty"`
	Surface    string    `json:"surface"`
	RecordedAt time.Time `json:"recorded_at"`
}

// WorkflowBuiltinControlRequest is the owner's control.
type WorkflowBuiltinControlRequest struct {
	Name    string
	Enabled bool
	Reason  string
	Surface string
}

// WorkflowBuiltinState is a built-in name's enabled state, its controls
// (newest first) and the versions recorded so far (newest first).
type WorkflowBuiltinState struct {
	Name     string                   `json:"name"`
	Enabled  bool                     `json:"enabled"`
	Control  *WorkflowBuiltinControl  `json:"control,omitempty"`
	Controls []WorkflowBuiltinControl `json:"controls,omitempty"`
	Versions []WorkflowBuiltinVersion `json:"versions,omitempty"`
}

// WorkflowResolution is what a new run of a name in a Workspace would pin:
// the Workspace's or the Global custom version, or (Source builtin) the
// built-in of that name, if Evie has one, while BuiltinEnabled.
type WorkflowResolution struct {
	Source         WorkflowSource          `json:"source"`
	Activation     *WorkflowActivation     `json:"activation,omitempty"`
	BuiltinEnabled bool                    `json:"builtin_enabled,omitempty"`
	BuiltinControl *WorkflowBuiltinControl `json:"builtin_control,omitempty"`
}

const standaloneTrigger = `CREATE TRIGGER IF NOT EXISTS %[1]s_standalone BEFORE INSERT ON %[1]s
 WHEN EXISTS(SELECT 1 FROM json_each(NEW.workers) WHERE value NOT IN ('research','explore'))%[2]s
 BEGIN SELECT RAISE(ABORT,'built-in and Global workflows run only research and explore workers and hold no Standing Authority, capability or schedule'); END;`

// reportStandalone refuses, in a trigger, a report that requests
// authority, calls a capability or declares a schedule.
const reportStandalone = `
 OR COALESCE(json_array_length(NEW.report,'$.requested_authority'),0) > 0
 OR COALESCE(json_array_length(NEW.report,'$.capabilities'),0) > 0
 OR json_extract(NEW.report,'$.schedule') IS NOT NULL`

func ensureWorkflowBuiltinSchema(ctx context.Context, db *sql.DB) error {
	immutable := func(table, what string) string {
		return fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_no_update BEFORE UPDATE ON %[1]s
 BEGIN SELECT RAISE(ABORT,'%[2]s are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS %[1]s_no_delete BEFORE DELETE ON %[1]s
 BEGIN SELECT RAISE(ABORT,'%[2]s are immutable'); END;`, table, what)
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_builtin_versions (
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
 version INTEGER NOT NULL CHECK(version > 0),
 commit_id TEXT NOT NULL,
 tree_path TEXT NOT NULL,
 executable_hash TEXT NOT NULL,
 authority_hash TEXT NOT NULL,
 authority TEXT NOT NULL CHECK(authority = '[]'),
 workers TEXT NOT NULL CHECK(json_valid(workers) AND json_type(workers) = 'array'),
 report TEXT NOT NULL CHECK(json_valid(report)),
 recorded_by TEXT NOT NULL CHECK(recorded_by = 'evie'),
 recorded_at TEXT NOT NULL,
 PRIMARY KEY(name, version),
 UNIQUE(name, executable_hash));
 `+fmt.Sprintf(standaloneTrigger, "workflow_builtin_versions", reportStandalone)+`
 `+immutable("workflow_builtin_versions", "built-in workflow versions")+`
 CREATE TABLE IF NOT EXISTS workflow_builtin_controls (
 seq INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
 enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 reason TEXT NOT NULL CHECK(length(reason) <= 1000),
 actor TEXT NOT NULL CHECK(actor = 'owner'),
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 recorded_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS workflow_builtin_controls_by_name ON workflow_builtin_controls(name, seq);
 `+immutable("workflow_builtin_controls", "built-in workflow controls")+`
 CREATE TABLE IF NOT EXISTS workflow_global_proposals (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
 commit_id TEXT NOT NULL,
 tree_path TEXT NOT NULL,
 executable_hash TEXT NOT NULL,
 authority_hash TEXT NOT NULL,
 report TEXT NOT NULL CHECK(json_valid(report)),
 simulation TEXT CHECK(simulation IS NULL OR json_valid(simulation)),
 simulation_status TEXT NOT NULL CHECK(simulation_status IN ('none','succeeded','failed','waiting_for_human')),
 proposed_by TEXT NOT NULL CHECK(proposed_by = 'owner'),
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 documents_version INTEGER NOT NULL CHECK(documents_version >= 0),
 proposed_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS workflow_global_proposals_by_name ON workflow_global_proposals(name, proposed_at);
 `+immutable("workflow_global_proposals", "Global workflow proposals")+`
 CREATE TABLE IF NOT EXISTS workflow_global_activations (
 name TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version > 0),
 proposal_id TEXT NOT NULL REFERENCES workflow_global_proposals(id),
 commit_id TEXT NOT NULL,
 tree_path TEXT NOT NULL,
 executable_hash TEXT NOT NULL,
 authority_hash TEXT NOT NULL,
 authority TEXT NOT NULL CHECK(authority = '[]'),
 workers TEXT NOT NULL CHECK(json_valid(workers) AND json_type(workers) = 'array'),
 approver TEXT NOT NULL CHECK(approver = 'owner'),
 approval_surface TEXT NOT NULL CHECK(approval_surface IN ('cli','web')),
 approval_detail TEXT NOT NULL,
 approved_at TEXT NOT NULL,
 PRIMARY KEY(name, version));
 `+fmt.Sprintf(standaloneTrigger, "workflow_global_activations", "")+`
 `+immutable("workflow_global_activations", "Global workflow activations")+`
 CREATE TABLE IF NOT EXISTS workflow_global_deactivations (
 name TEXT NOT NULL,
 version INTEGER NOT NULL,
 reason TEXT NOT NULL,
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 deactivated_at TEXT NOT NULL,
 PRIMARY KEY(name, version),
 FOREIGN KEY(name, version) REFERENCES workflow_global_activations(name, version));
 `+immutable("workflow_global_deactivations", "Global workflow deactivations"))
	if err != nil {
		return fmt.Errorf("create built-in and Global workflow schema: %w", err)
	}
	return nil
}

// Runs pin their source (2026-10-04). workflowRunActivationKey is the
// foreign key of sub-stages 3 to 12, which only a Workspace version
// satisfies; workflowRunSource replaces it, and the trigger below checks
// the pinned version in its source's table. Every pinned table is
// immutable, so the check at insert holds for good.
const (
	workflowRunActivationKey = "FOREIGN KEY(workspace_id, workflow, version) REFERENCES workflow_activations(workspace_id, name, version),"
	workflowRunSource        = "source TEXT NOT NULL DEFAULT 'workspace' CHECK(source IN ('workspace','global','builtin')),"
	workflowRunSourceChecks  = `CREATE TRIGGER IF NOT EXISTS workflow_runs_pinned_version BEFORE INSERT ON workflow_runs
 WHEN NOT CASE NEW.source
 WHEN 'workspace' THEN EXISTS(SELECT 1 FROM workflow_activations a WHERE a.workspace_id=NEW.workspace_id AND a.name=NEW.workflow AND a.version=NEW.version)
 WHEN 'global' THEN EXISTS(SELECT 1 FROM workflow_global_activations a WHERE a.name=NEW.workflow AND a.version=NEW.version)
 WHEN 'builtin' THEN EXISTS(SELECT 1 FROM workflow_builtin_versions b WHERE b.name=NEW.workflow AND b.version=NEW.version)
 ELSE 0 END
 BEGIN SELECT RAISE(ABORT,'a workflow run pins an approved Workspace or Global version or a recorded built-in version'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_source_pinned BEFORE UPDATE OF source ON workflow_runs
 BEGIN SELECT RAISE(ABORT,'a workflow run''s pinned fields are immutable'); END;`
)

// upgradeWorkflowRunSources rebuilds a workflow_runs table from before
// built-in workflows once (its foreign key becomes the source column,
// every existing run a Workspace run), then adds the source checks.
func upgradeWorkflowRunSources(ctx context.Context, db *sql.DB) error {
	if err := rebuildWorkflowRunsWith(ctx, db, [][2]string{{workflowRunActivationKey, workflowRunSource}}); err != nil {
		return fmt.Errorf("upgrade workflow runs for built-in and Global workflows: %w", err)
	}
	if _, err := db.ExecContext(ctx, workflowRunSourceChecks); err != nil {
		return fmt.Errorf("create workflow run source checks: %w", err)
	}
	return nil
}

// --- Built-in versions and controls ---

const builtinVersionColumns = `name,version,commit_id,tree_path,executable_hash,authority_hash,workers,report,recorded_at`

func scanBuiltinVersion(scanner rowScanner) (WorkflowBuiltinVersion, error) {
	var v WorkflowBuiltinVersion
	var workers, report, recorded string
	if err := scanner.Scan(&v.Name, &v.Version, &v.Commit, &v.Path, &v.ExecutableHash, &v.AuthorityHash, &workers, &report, &recorded); err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(workers), &v.Workers); err != nil {
		return v, fmt.Errorf("decode built-in workflow workers: %w", err)
	}
	if err := json.Unmarshal([]byte(report), &v.Report); err != nil {
		return v, fmt.Errorf("decode built-in workflow report: %w", err)
	}
	var err error
	v.RecordedAt, err = parseTime(recorded)
	return v, err
}

func builtinVersionAt(ctx context.Context, q queryRower, name string, version int64) (WorkflowBuiltinVersion, error) {
	v, err := scanBuiltinVersion(q.QueryRowContext(ctx, `SELECT `+builtinVersionColumns+` FROM workflow_builtin_versions WHERE name=? AND version=?`, name, version))
	if errors.Is(err, sql.ErrNoRows) {
		return v, fmt.Errorf("%w: built-in %s version %d is not recorded", ErrWorkflowNotFound, name, version)
	}
	return v, err
}

// RecordWorkflowBuiltinVersion records a built-in version whose exact files
// are at v.Commit in procedural Git, numbering it after the name's latest,
// or returns the version already recorded with its executable hash. Only a
// definition that qualifies as built-in is recorded.
func (s *Store) RecordWorkflowBuiltinVersion(ctx context.Context, v WorkflowBuiltinVersion) (WorkflowBuiltinVersion, error) {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrWorkflowProposal, fmt.Sprintf(format, args...))
	}
	req := workflow.RequirementsOf(v.Report)
	switch {
	case !workflowNamePattern.MatchString(v.Name) || v.Name != v.Report.Workflow:
		return v, bad("built-in name %q is not the report's %q", v.Name, v.Report.Workflow)
	case !workflowHashPattern.MatchString(v.ExecutableHash) || v.ExecutableHash != v.Report.ExecutableHash:
		return v, bad("executable hash %s is not the report's", v.ExecutableHash)
	case v.AuthorityHash != req.AuthorityHash:
		return v, bad("authority hash %s is not the report's", v.AuthorityHash)
	case !gitCommitPattern.MatchString(v.Commit):
		return v, bad("%q is not a procedural Git commit", v.Commit)
	case v.Path != BuiltinWorkflowDefinitionPath(v.Name):
		return v, bad("path %q is not %s", v.Path, BuiltinWorkflowDefinitionPath(v.Name))
	}
	if problems := workflow.QualifyBuiltin(v.Report); len(problems) > 0 {
		return v, fmt.Errorf("%w: %s", ErrWorkflowNotStandalone, strings.Join(problems, "; "))
	}
	workers, err := json.Marshal(req.Workers)
	if err != nil {
		return v, err
	}
	report, err := json.Marshal(v.Report)
	if err != nil {
		return v, err
	}
	var out WorkflowBuiltinVersion
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		existing, err := scanBuiltinVersion(conn.QueryRowContext(ctx, `SELECT `+builtinVersionColumns+` FROM workflow_builtin_versions WHERE name=? AND executable_hash=?`, v.Name, v.ExecutableHash))
		if err == nil {
			out = existing
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var latest int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM workflow_builtin_versions WHERE name=?`, v.Name).Scan(&latest); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_builtin_versions(name,version,commit_id,tree_path,executable_hash,authority_hash,authority,workers,report,recorded_by,recorded_at)
 VALUES(?,?,?,?,?,?,'[]',?,?,'evie',?)`, v.Name, latest+1, v.Commit, v.Path, v.ExecutableHash, v.AuthorityHash, string(workers), string(report), formatTime(s.now())); err != nil {
			return fmt.Errorf("record built-in workflow version: %w", err)
		}
		out, err = builtinVersionAt(ctx, conn, v.Name, latest+1)
		return err
	})
	return out, err
}

// WorkflowBuiltinVersion reads one recorded built-in version.
func (s *Store) WorkflowBuiltinVersion(ctx context.Context, name string, version int64) (WorkflowBuiltinVersion, error) {
	return builtinVersionAt(ctx, s.db, name, version)
}

func latestBuiltinControl(ctx context.Context, q queryRower, name string) (*WorkflowBuiltinControl, error) {
	var c WorkflowBuiltinControl
	var recorded string
	err := q.QueryRowContext(ctx, `SELECT seq,name,enabled,reason,surface,recorded_at FROM workflow_builtin_controls WHERE name=? ORDER BY seq DESC LIMIT 1`, name).
		Scan(&c.Seq, &c.Name, &c.Enabled, &c.Reason, &c.Surface, &recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if c.RecordedAt, err = parseTime(recorded); err != nil {
		return nil, err
	}
	return &c, nil
}

// WorkflowBuiltinState reads a built-in name's enabled state with its
// controls and recorded versions, newest first. A name never controlled is
// enabled.
func (s *Store) WorkflowBuiltinState(ctx context.Context, name string) (WorkflowBuiltinState, error) {
	out := WorkflowBuiltinState{Name: name, Enabled: true, Controls: []WorkflowBuiltinControl{}, Versions: []WorkflowBuiltinVersion{}}
	rows, err := s.db.QueryContext(ctx, `SELECT seq,name,enabled,reason,surface,recorded_at FROM workflow_builtin_controls WHERE name=? ORDER BY seq DESC`, name)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c WorkflowBuiltinControl
		var recorded string
		if err := rows.Scan(&c.Seq, &c.Name, &c.Enabled, &c.Reason, &c.Surface, &recorded); err != nil {
			rows.Close()
			return out, err
		}
		if c.RecordedAt, err = parseTime(recorded); err != nil {
			rows.Close()
			return out, err
		}
		out.Controls = append(out.Controls, c)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}
	if len(out.Controls) > 0 {
		latest := out.Controls[0]
		out.Control, out.Enabled = &latest, latest.Enabled
	}
	rows, err = s.db.QueryContext(ctx, `SELECT `+builtinVersionColumns+` FROM workflow_builtin_versions WHERE name=? ORDER BY version DESC`, name)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanBuiltinVersion(rows)
		if err != nil {
			return out, err
		}
		out.Versions = append(out.Versions, v)
	}
	return out, rows.Err()
}

// ControlWorkflowBuiltin records the owner disabling or enabling a built-in
// for every Workspace. A disabled built-in starts no run, and its runs in
// progress pause at their next node (ADR 0033); enabling it again lets them
// continue. Only the owner controls it, from the CLI or the web; a control
// that changes nothing is refused.
func (s *Store) ControlWorkflowBuiltin(ctx context.Context, req WorkflowBuiltinControlRequest) (WorkflowBuiltinControl, error) {
	if req.Surface != "cli" && req.Surface != "web" {
		return WorkflowBuiltinControl{}, ErrWorkflowApprover
	}
	if !workflowNamePattern.MatchString(req.Name) {
		return WorkflowBuiltinControl{}, fmt.Errorf("%w: %q", ErrWorkflowNotFound, req.Name)
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 1000 {
		return WorkflowBuiltinControl{}, errors.New("a built-in control reason is at most 1000 bytes")
	}
	var out WorkflowBuiltinControl
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		latest, err := latestBuiltinControl(ctx, conn, req.Name)
		if err != nil {
			return err
		}
		if (latest == nil || latest.Enabled) == req.Enabled {
			return ErrWorkflowBuiltinUnchanged
		}
		now := s.now().UTC()
		res, err := conn.ExecContext(ctx, `INSERT INTO workflow_builtin_controls(name,enabled,reason,actor,surface,recorded_at) VALUES(?,?,?,'owner',?,?)`,
			req.Name, req.Enabled, reason, req.Surface, formatTime(now))
		if err != nil {
			return fmt.Errorf("record built-in workflow control: %w", err)
		}
		seq, err := res.LastInsertId()
		if err != nil {
			return err
		}
		out = WorkflowBuiltinControl{Seq: seq, Name: req.Name, Enabled: req.Enabled, Reason: reason, Surface: req.Surface, RecordedAt: now}
		return nil
	})
	return out, err
}

// --- Global custom definitions ---

func (p WorkflowProposal) checkGlobal() error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrWorkflowProposal, fmt.Sprintf(format, args...))
	}
	switch {
	case p.WorkspaceID != "":
		return bad("a Global proposal names no Workspace")
	case !workflowNamePattern.MatchString(p.Name) || p.Name != p.Report.Workflow:
		return bad("name %q is not the report's %q", p.Name, p.Report.Workflow)
	case !workflowHashPattern.MatchString(p.ExecutableHash) || p.ExecutableHash != p.Report.ExecutableHash:
		return bad("executable hash %s is not the report's", p.ExecutableHash)
	case p.AuthorityHash != workflow.RequirementsOf(p.Report).AuthorityHash:
		return bad("authority hash %s is not the report's", p.AuthorityHash)
	case !gitCommitPattern.MatchString(p.Commit):
		return bad("%q is not a procedural Git commit", p.Commit)
	case p.Path != GlobalWorkflowDefinitionPath(p.Name):
		return bad("path %q is not %s", p.Path, GlobalWorkflowDefinitionPath(p.Name))
	case p.ProposedBy != "owner" || (p.Surface != "cli" && p.Surface != "web"):
		return ErrWorkflowApprover
	case p.Simulation != nil && (p.Simulation.ExecutableHash != p.ExecutableHash || p.Simulation.Workflow != p.Name):
		return bad("the simulation was not run on this definition")
	}
	if problems := workflow.QualifyBuiltin(p.Report); len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrWorkflowNotStandalone, strings.Join(problems, "; "))
	}
	return nil
}

// RecordGlobalWorkflowProposal records the owner's proposal of a Global
// custom definition whose files are at p.Commit in procedural Git; it is
// qualified like a built-in (no authority, capability or schedule, only
// research and explore workers). It is documentation of the active Global
// version when its executable hash is that version's.
func (s *Store) RecordGlobalWorkflowProposal(ctx context.Context, p WorkflowProposal) (WorkflowProposal, error) {
	if err := p.checkGlobal(); err != nil {
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
	p.ID, p.Source = uuid.NewString(), WorkflowSourceGlobal
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		latest, ok, err := latestGlobalActivation(ctx, conn, p.Name)
		if err != nil {
			return err
		}
		p.DocumentsVersion = 0
		if ok && latest.Deactivated == nil && latest.ExecutableHash == p.ExecutableHash {
			p.DocumentsVersion = latest.Version
		}
		p.ProposedAt = s.now().UTC()
		_, err = conn.ExecContext(ctx, `INSERT INTO workflow_global_proposals(id,name,commit_id,tree_path,executable_hash,authority_hash,report,simulation,simulation_status,proposed_by,surface,documents_version,proposed_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, p.Name, p.Commit, p.Path, p.ExecutableHash, p.AuthorityHash, string(report), simulation, status,
			p.ProposedBy, p.Surface, p.DocumentsVersion, formatTime(p.ProposedAt))
		if err != nil {
			return fmt.Errorf("record Global workflow proposal: %w", err)
		}
		return nil
	})
	if err != nil {
		return WorkflowProposal{}, err
	}
	return p, nil
}

const globalProposalColumns = `id,name,commit_id,tree_path,executable_hash,authority_hash,report,simulation,proposed_by,surface,documents_version,proposed_at`

func scanGlobalProposal(scanner rowScanner) (WorkflowProposal, error) {
	p := WorkflowProposal{Source: WorkflowSourceGlobal}
	var report, proposed string
	var simulation sql.NullString
	if err := scanner.Scan(&p.ID, &p.Name, &p.Commit, &p.Path, &p.ExecutableHash, &p.AuthorityHash, &report, &simulation,
		&p.ProposedBy, &p.Surface, &p.DocumentsVersion, &proposed); err != nil {
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

// GlobalWorkflowProposal reads one Global proposal.
func (s *Store) GlobalWorkflowProposal(ctx context.Context, id string) (WorkflowProposal, error) {
	return globalProposal(ctx, s.db, id)
}

func globalProposal(ctx context.Context, q queryRower, id string) (WorkflowProposal, error) {
	p, err := scanGlobalProposal(q.QueryRowContext(ctx, `SELECT `+globalProposalColumns+` FROM workflow_global_proposals WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("%w: %s", ErrWorkflowProposalNotFound, id)
	}
	return p, err
}

const globalActivationColumns = `a.name,a.version,a.proposal_id,a.commit_id,a.tree_path,a.executable_hash,a.authority_hash,a.workers,
 a.approver,a.approval_surface,a.approval_detail,a.approved_at,d.reason,d.surface,d.deactivated_at`

const globalActivationFrom = ` FROM workflow_global_activations a LEFT JOIN workflow_global_deactivations d ON d.name=a.name AND d.version=a.version`

func scanGlobalActivation(scanner rowScanner) (WorkflowActivation, error) {
	a := WorkflowActivation{Source: WorkflowSourceGlobal, Authority: []workflow.AuthorityReport{}}
	var workers, approved string
	var reason, surface, deactivated sql.NullString
	if err := scanner.Scan(&a.Name, &a.Version, &a.ProposalID, &a.Commit, &a.Path, &a.ExecutableHash, &a.AuthorityHash, &workers,
		&a.Approver.Actor, &a.Approver.Surface, &a.Approver.Detail, &approved, &reason, &surface, &deactivated); err != nil {
		return a, err
	}
	if err := json.Unmarshal([]byte(workers), &a.Workers); err != nil {
		return a, fmt.Errorf("decode workflow workers: %w", err)
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

func latestGlobalActivation(ctx context.Context, q queryRower, name string) (WorkflowActivation, bool, error) {
	a, err := scanGlobalActivation(q.QueryRowContext(ctx, `SELECT `+globalActivationColumns+globalActivationFrom+` WHERE a.name=? ORDER BY a.version DESC LIMIT 1`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkflowActivation{}, false, nil
	}
	return a, err == nil, err
}

func globalActivationAt(ctx context.Context, q queryRower, name string, version int64) (WorkflowActivation, error) {
	a, err := scanGlobalActivation(q.QueryRowContext(ctx, `SELECT `+globalActivationColumns+globalActivationFrom+` WHERE a.name=? AND a.version=?`, name, version))
	if errors.Is(err, sql.ErrNoRows) {
		return a, fmt.Errorf("%w: Global %s version %d", ErrWorkflowNotFound, name, version)
	}
	return a, err
}

// ActivateGlobalWorkflow records the owner's approval of one exact Global
// proposal as the name's next Global version: the reviewed hashes, the
// version the review compared with, a dry run of it that succeeded, and
// the built-in qualification. It needs no Workspace: each Workspace's
// worker allowance applies when a run starts there.
func (s *Store) ActivateGlobalWorkflow(ctx context.Context, approval WorkflowApproval) (WorkflowActivation, error) {
	if !approval.Approver.valid() {
		return WorkflowActivation{}, ErrWorkflowApprover
	}
	var out WorkflowActivation
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		p, err := globalProposal(ctx, conn, approval.ProposalID)
		if err != nil {
			return err
		}
		if approval.ExecutableHash != p.ExecutableHash || approval.AuthorityHash != p.AuthorityHash {
			return ErrWorkflowHashMismatch
		}
		latest, ok, err := latestGlobalActivation(ctx, conn, p.Name)
		if err != nil {
			return err
		}
		if latest.Version != approval.ExpectedVersion {
			return ErrWorkflowChanged
		}
		if p.Simulation == nil || p.Simulation.Status != workflow.SimulationSucceeded || p.Simulation.ExecutableHash != p.ExecutableHash {
			return ErrWorkflowSimulationRequired
		}
		if problems := workflow.QualifyBuiltin(p.Report); len(problems) > 0 {
			return &WorkflowRefusal{Reasons: problems}
		}
		if ok && latest.Deactivated == nil && latest.ExecutableHash == p.ExecutableHash && latest.AuthorityHash == p.AuthorityHash {
			return ErrWorkflowAlreadyActive
		}
		workers, err := json.Marshal(workflow.RequirementsOf(p.Report).Workers)
		if err != nil {
			return err
		}
		version := latest.Version + 1
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_global_activations(name,version,proposal_id,commit_id,tree_path,executable_hash,authority_hash,authority,workers,approver,approval_surface,approval_detail,approved_at)
 VALUES(?,?,?,?,?,?,?,'[]',?,?,?,?,?)`, p.Name, version, p.ID, p.Commit, p.Path, p.ExecutableHash, p.AuthorityHash, string(workers),
			approval.Approver.Actor, approval.Approver.Surface, approval.Approver.Detail, formatTime(s.now())); err != nil {
			return fmt.Errorf("record Global workflow activation: %w", err)
		}
		out, err = globalActivationAt(ctx, conn, p.Name, version)
		return err
	})
	return out, err
}

// DeactivateGlobalWorkflow is the owner ending the Global version: no new
// run starts from it (a Workspace without its own custom version falls
// back to the built-in, if any), and its runs pause at their next node.
func (s *Store) DeactivateGlobalWorkflow(ctx context.Context, name string, version int64, reason, surface string) (WorkflowActivation, error) {
	if surface != "cli" && surface != "web" {
		return WorkflowActivation{}, ErrWorkflowApprover
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 1000 {
		return WorkflowActivation{}, errors.New("a deactivation reason is at most 1000 bytes")
	}
	var out WorkflowActivation
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		latest, ok, err := latestGlobalActivation(ctx, conn, name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: Global %s", ErrWorkflowNotFound, name)
		}
		if latest.Version != version {
			return ErrWorkflowChanged
		}
		if latest.Deactivated != nil {
			return ErrWorkflowNotActive
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_global_deactivations(name,version,reason,surface,deactivated_at) VALUES(?,?,?,?,?)`,
			name, version, reason, surface, formatTime(s.now())); err != nil {
			return fmt.Errorf("record Global workflow deactivation: %w", err)
		}
		out, err = globalActivationAt(ctx, conn, name, version)
		return err
	})
	return out, err
}

// GlobalWorkflowDefinition reads one Global definition's status and full
// history. Blocking is per Workspace (its worker allowance), so a Global
// definition is proposed, active or inactive.
func (s *Store) GlobalWorkflowDefinition(ctx context.Context, name string) (WorkflowDefinitionState, error) {
	return readGlobalDefinition(ctx, s.db, name, true)
}

// ListGlobalWorkflowDefinitions lists every Global definition by name.
func (s *Store) ListGlobalWorkflowDefinitions(ctx context.Context) ([]WorkflowDefinitionState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT name FROM workflow_global_proposals ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := []WorkflowDefinitionState{}
	for _, name := range names {
		d, err := readGlobalDefinition(ctx, s.db, name, false)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func readGlobalDefinition(ctx context.Context, q interface {
	queryRower
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, name string, detail bool) (WorkflowDefinitionState, error) {
	d := WorkflowDefinitionState{Name: name, Source: WorkflowSourceGlobal}
	rows, err := q.QueryContext(ctx, `SELECT `+globalActivationColumns+globalActivationFrom+` WHERE a.name=? ORDER BY a.version DESC`, name)
	if err != nil {
		return d, err
	}
	var versions []WorkflowActivation
	for rows.Next() {
		a, err := scanGlobalActivation(rows)
		if err != nil {
			rows.Close()
			return d, err
		}
		versions = append(versions, a)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	rows, err = q.QueryContext(ctx, `SELECT id,commit_id,executable_hash,authority_hash,simulation_status,documents_version,proposed_by,proposed_at
 FROM workflow_global_proposals WHERE name=? ORDER BY proposed_at DESC, rowid DESC`, name)
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
	if len(proposals) == 0 && len(versions) == 0 {
		return d, fmt.Errorf("%w: Global %s", ErrWorkflowNotFound, name)
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
		d.Status, d.Current = WorkflowInactive, &versions[0]
	default:
		d.Status, d.Current = WorkflowActive, &versions[0]
	}
	if detail {
		d.Versions, d.Proposals = versions, proposals
	}
	return d, nil
}

// --- Resolution and admission ---

// standaloneBlocks lists why a Workspace cannot run a built-in or Global
// version's workers now: an archived Workspace, or a worker preset its
// allowance (at revision, pinned, current and every revocation of the
// pinned) does not cover.
func standaloneBlocks(ctx context.Context, conn *sql.Conn, ws memory.WorkspaceID, revision memory.WorkspaceRevisionID, workers []string, starting bool) ([]string, error) {
	w, err := readWorkspaceRow(ctx, conn, ws)
	if err != nil {
		return nil, err
	}
	if w.state != memory.WorkspaceActive {
		return []string{"the Workspace is archived"}, nil
	}
	if revision == "" {
		revision = w.revision
	}
	var reasons []string
	for _, worker := range workers {
		err := authorizeWorkspacePreset(ctx, conn, memory.ScopeContext{WorkspaceID: ws, WorkspaceRevision: revision}, worker)
		if errors.Is(err, delegation.ErrAuthority) {
			if starting {
				reasons = append(reasons, standaloneWorkerReason(worker))
			} else {
				reasons = append(reasons, fmt.Sprintf("agent steps run %s workers, which this Workspace no longer allows for this run; allow them again in the Workspace's settings and start a new run", worker))
			}
		} else if err != nil {
			return nil, err
		}
	}
	return reasons, nil
}

// WorkspaceWorkerBlocks lists why the Workspace cannot run a built-in's or
// Global version's workers now (its current revision), for review.
func (s *Store) WorkspaceWorkerBlocks(ctx context.Context, ws memory.WorkspaceID, workers []string) ([]string, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return standaloneBlocks(ctx, conn, ws, "", workers, true)
}

// standaloneWorkerReason tells the owner, and a chat, exactly what to
// enable for a built-in or Global workflow's workers.
func standaloneWorkerReason(worker string) string {
	switch worker {
	case workflow.WorkerResearch:
		return "agent steps run research workers, which this Workspace does not allow: David turns on research delegation in the Workspace's settings (the Workspace page), then starts it again"
	case workflow.WorkerExplore:
		return "agent steps run explore workers, which this Workspace does not allow: David allows explore in the Workspace's settings (it follows the Workspace folder by default), then starts it again"
	}
	return workerReason(worker, true)
}

// ResolveWorkflow says what a new run of name in the Workspace would pin:
// the Workspace's custom version, the Global custom version, or the
// built-in (whether one exists is Evie's to know). A custom version that
// applies but is blocked is returned with its *WorkflowBlockedError.
func (s *Store) ResolveWorkflow(ctx context.Context, ws memory.WorkspaceID, name string) (WorkflowResolution, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return WorkflowResolution{}, err
	}
	defer conn.Close()
	return resolveWorkflow(ctx, conn, ws, name)
}

func resolveWorkflow(ctx context.Context, conn *sql.Conn, ws memory.WorkspaceID, name string) (WorkflowResolution, error) {
	w, err := readWorkspaceRow(ctx, conn, ws)
	if err != nil {
		return WorkflowResolution{}, err
	}
	custom, ok, err := latestActivation(ctx, conn, ws, name)
	if err != nil {
		return WorkflowResolution{}, err
	}
	if ok && custom.Deactivated == nil {
		res := WorkflowResolution{Source: WorkflowSourceWorkspace, Activation: &custom}
		blocks, err := activationBlocks(ctx, conn, custom)
		if err != nil {
			return res, err
		}
		if len(blocks) > 0 {
			return res, &WorkflowBlockedError{Reasons: blocks}
		}
		return res, nil
	}
	global, ok, err := latestGlobalActivation(ctx, conn, name)
	if err != nil {
		return WorkflowResolution{}, err
	}
	if ok && global.Deactivated == nil {
		global.WorkspaceID, global.WorkspaceRevision = ws, w.revision
		res := WorkflowResolution{Source: WorkflowSourceGlobal, Activation: &global}
		blocks, err := standaloneBlocks(ctx, conn, ws, w.revision, global.Workers, true)
		if err != nil {
			return res, err
		}
		if len(blocks) > 0 {
			return res, &WorkflowBlockedError{Reasons: blocks}
		}
		return res, nil
	}
	control, err := latestBuiltinControl(ctx, conn, name)
	if err != nil {
		return WorkflowResolution{}, err
	}
	return WorkflowResolution{Source: WorkflowSourceBuiltin, BuiltinEnabled: control == nil || control.Enabled, BuiltinControl: control}, nil
}

// admitWorkflowStart is run admission for a new run, inside the
// transaction that records it: the name must still resolve to the source
// and version the runtime validated, which must still admit runs. It
// returns the version to pin.
func admitWorkflowStart(ctx context.Context, conn *sql.Conn, req WorkflowRunStart) (WorkflowActivation, error) {
	source := workspaceSource(req.Source)
	res, err := resolveWorkflow(ctx, conn, req.WorkspaceID, req.Workflow)
	if res.Source != source {
		if res.Source == "" {
			return WorkflowActivation{}, err
		}
		if source == WorkflowSourceWorkspace {
			// The Workspace version no longer applies: say why, as before
			// built-ins existed.
			if _, err := authorizeWorkflowActivation(ctx, conn, req.WorkspaceID, req.Workflow); err != nil {
				return WorkflowActivation{}, err
			}
		}
		return WorkflowActivation{}, ErrWorkflowChanged
	}
	if err != nil {
		return WorkflowActivation{}, err
	}
	switch source {
	case WorkflowSourceWorkspace, WorkflowSourceGlobal:
		return *res.Activation, nil
	case WorkflowSourceBuiltin:
		if !res.BuiltinEnabled {
			return WorkflowActivation{}, fmt.Errorf("%w (%s); David enables it with evie workflows builtin enable %s or in Settings", ErrWorkflowBuiltinDisabled, req.Workflow, req.Workflow)
		}
		v, err := builtinVersionAt(ctx, conn, req.Workflow, req.Version)
		if errors.Is(err, ErrWorkflowNotFound) {
			return WorkflowActivation{}, ErrWorkflowChanged
		}
		if err != nil {
			return WorkflowActivation{}, err
		}
		w, err := readWorkspaceRow(ctx, conn, req.WorkspaceID)
		if err != nil {
			return WorkflowActivation{}, err
		}
		blocks, err := standaloneBlocks(ctx, conn, req.WorkspaceID, w.revision, v.Workers, true)
		if err != nil {
			return WorkflowActivation{}, err
		}
		if len(blocks) > 0 {
			return WorkflowActivation{}, &WorkflowBlockedError{Reasons: blocks}
		}
		return v.Activation(req.WorkspaceID), nil
	}
	return WorkflowActivation{}, fmt.Errorf("unknown workflow source %q", source)
}

// authorizeStandaloneRun is run admission for a built-in or Global run
// that continues (ADR 0033: revocations apply despite the pin): its
// built-in must still be enabled, or its Global definition's latest
// version and its own still approved and not deactivated; the Workspace
// must be active and still allow its workers at the run's pinned and
// current revision. A run of an earlier built-in or Global version
// continues under its own pins.
func authorizeStandaloneRun(ctx context.Context, conn *sql.Conn, run WorkflowRun) error {
	var workers []string
	switch run.Source {
	case WorkflowSourceBuiltin:
		control, err := latestBuiltinControl(ctx, conn, run.Workflow)
		if err != nil {
			return err
		}
		if control != nil && !control.Enabled {
			return fmt.Errorf("%w (%s, at %s); David enables it with evie workflows builtin enable %s or in Settings", ErrWorkflowBuiltinDisabled, run.Workflow,
				control.RecordedAt.Format(time.RFC3339), run.Workflow)
		}
		v, err := builtinVersionAt(ctx, conn, run.Workflow, run.Version)
		if err != nil {
			return err
		}
		workers = v.Workers
	case WorkflowSourceGlobal:
		latest, ok, err := latestGlobalActivation(ctx, conn, run.Workflow)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: Global %s", ErrWorkflowNotFound, run.Workflow)
		}
		if latest.Deactivated != nil {
			return fmt.Errorf("%w: Global version %d was deactivated", ErrWorkflowNotActive, latest.Version)
		}
		own, err := globalActivationAt(ctx, conn, run.Workflow, run.Version)
		if err != nil {
			return err
		}
		if own.Deactivated != nil {
			return fmt.Errorf("%w: Global version %d was deactivated", ErrWorkflowNotActive, own.Version)
		}
		workers = own.Workers
	default:
		return fmt.Errorf("unknown workflow source %q", run.Source)
	}
	blocks, err := standaloneBlocks(ctx, conn, run.WorkspaceID, run.WorkspaceRevision, workers, false)
	if err != nil {
		return err
	}
	if len(blocks) > 0 {
		return &WorkflowBlockedError{Reasons: blocks}
	}
	return nil
}
