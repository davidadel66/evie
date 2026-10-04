package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/workflow"
)

// Capability reads (Stage 14 sub-stage 5 of docs/memory-architecture.md,
// 2026-10-04; workflow-runtime.spec.md, ADR 0016, 0017, 0025, 0033, 0041):
//
//   - Per-node admission: before a node acts through a capability, its
//     holder asks, in one fenced transaction, that the run's own version
//     still admits runs (deactivation and blocks, including every access
//     revocation since the run's pinned revision), that the operation is
//     within the run's pinned Standing Authority, and that the Workspace's
//     current access still covers its connection and resource. Effects
//     (sub-stage 6) use the same check before their Effect Intent.
//   - An attempt its own holder records interrupted: a runtime that stops
//     while a read is in flight records the attempt before it releases the
//     run, exactly as a later claim would.
//   - workflow_run_substitutions: the owner's audited approval that a run
//     continues with a replacement implementation of a pinned provider that
//     declares it continues the pinned one with the exact same contract and
//     schema (ADR 0041's Compatibility Resolution). The run's pinned
//     Execution Composition is never rewritten. Immutable.

var (
	// ErrWorkflowNodeRefused is a node its run may not perform now.
	ErrWorkflowNodeRefused = errors.New("the workflow node is not admitted")
)

// WorkflowNodeRefusal says why a node is not admitted.
type WorkflowNodeRefusal struct{ Reason string }

func (e *WorkflowNodeRefusal) Error() string { return e.Reason }
func (e *WorkflowNodeRefusal) Unwrap() error { return ErrWorkflowNodeRefused }

// WorkflowNodeAdmission is one node about to act through a capability on
// one connection's exact resource.
type WorkflowNodeAdmission struct {
	Step       string
	Capability string
	Connection string
	Resource   string
}

// AdmitWorkflowNode applies admission to one node its holder is about to
// run, changing nothing: run admission for the run (authorizeWorkflowRun),
// the operation within the run's own approved Standing Authority, and the
// connection and resource within the Workspace's current access. A refusal
// is ErrWorkflowNodeRefused, ErrWorkflowNotActive or ErrWorkflowBlocked;
// the runtime pauses the run before the node.
func (s *Store) AdmitWorkflowNode(ctx context.Context, lease WorkflowRunLease, n WorkflowNodeAdmission) error {
	return s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, _ time.Time) error {
		run, err := runRunnable(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		if err := authorizeWorkflowRun(ctx, conn, run); err != nil {
			return err
		}
		own, err := activationAt(ctx, conn, run.WorkspaceID, run.Workflow, run.Version)
		if err != nil {
			return err
		}
		covered := slices.ContainsFunc(own.Authority, func(op workflow.AuthorityReport) bool {
			return op.Capability == n.Capability && op.Connection == n.Connection && slices.Contains(op.Resources, n.Resource)
		})
		if !covered {
			return &WorkflowNodeRefusal{Reason: fmt.Sprintf("steps.%s: %s on %s %s is outside the Standing Authority approved with version %d",
				n.Step, n.Capability, n.Connection, n.Resource, run.Version)}
		}
		access, err := workspaceAccess(ctx, conn, run.WorkspaceID)
		if err != nil {
			return err
		}
		if !access.Access.Allows(n.Connection, n.Resource) {
			return &WorkflowNodeRefusal{Reason: fmt.Sprintf("steps.%s: %s %s is outside the Workspace's current Workspace Access (revision %d)",
				n.Step, n.Connection, n.Resource, access.Revision)}
		}
		return nil
	})
}

// CheckWorkflowRunLease reports whether lease is still the run's current,
// unexpired lease, changing nothing: the check a holder makes immediately
// before it calls a provider.
func (s *Store) CheckWorkflowRunLease(ctx context.Context, lease WorkflowRunLease) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return fenceWorkflowRun(ctx, conn, lease, s.now().UTC())
}

// InterruptWorkflowAttempt records, under the holder's lease, that the
// holder's own attempt in flight was interrupted (the runtime stopped while
// it ran), with an attempt_interrupted event, before the holder releases
// the run. The next holder reruns it from the same checkpoint.
func (s *Store) InterruptWorkflowAttempt(ctx context.Context, lease WorkflowRunLease, seq int64, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("an interrupted attempt needs its reason")
	}
	return s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		var step string
		var attempt int
		var checkpoint int64
		err := conn.QueryRowContext(ctx, `UPDATE workflow_node_attempts SET state='interrupted',error=?,finished_at=?
 WHERE run_id=? AND seq=? AND state='running' AND lease_token=? RETURNING step_id,attempt,input_checkpoint`,
			reason, formatTime(now), lease.RunID, seq, lease.Token).Scan(&step, &attempt, &checkpoint)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkflowRunConflict
		}
		if err != nil {
			return fmt.Errorf("record interrupted workflow node attempt: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET updated_at=? WHERE id=?`, formatTime(now), lease.RunID); err != nil {
			return err
		}
		return recordWorkflowRunEvent(ctx, conn, WorkflowRunEvent{RunID: lease.RunID, Kind: WorkflowEventAttemptInterrupted, Holder: lease.Holder, Token: lease.Token,
			Attempt: seq, Detail: fmt.Sprintf("steps.%s attempt %d from checkpoint %d: %s", step, attempt, checkpoint, reason), At: now})
	})
}

// WorkflowRunSubstitution is the owner's audited approval that a run
// continues one pinned capability with a replacement implementation of its
// plugin: the pinned and replacement versions, and the exact contract and
// schema both share, with the replacement's declared evidence.
type WorkflowRunSubstitution struct {
	RunID              string    `json:"run_id"`
	Seq                int64     `json:"seq"`
	Capability         string    `json:"capability"`
	Plugin             string    `json:"plugin"`
	PinnedVersion      string    `json:"pinned_version"`
	ReplacementVersion string    `json:"replacement_version"`
	Contract           string    `json:"contract"`
	SchemaSHA256       string    `json:"schema_sha256"`
	Evidence           string    `json:"evidence"`
	Approver           string    `json:"approver"`
	Surface            string    `json:"surface"`
	ApprovedAt         time.Time `json:"approved_at"`
}

func ensureWorkflowRunSubstitutionSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_run_substitutions (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 seq INTEGER NOT NULL CHECK(seq > 0),
 capability TEXT NOT NULL CHECK(length(capability) BETWEEN 1 AND 128),
 plugin TEXT NOT NULL CHECK(length(plugin) BETWEEN 1 AND 128),
 pinned_version TEXT NOT NULL CHECK(length(pinned_version) BETWEEN 1 AND 64),
 replacement_version TEXT NOT NULL CHECK(length(replacement_version) BETWEEN 1 AND 64 AND replacement_version <> pinned_version),
 contract TEXT NOT NULL CHECK(length(contract) BETWEEN 1 AND 64),
 schema_sha256 TEXT NOT NULL CHECK(length(schema_sha256) = 64),
 evidence TEXT NOT NULL CHECK(length(evidence) BETWEEN 1 AND 2000),
 approver TEXT NOT NULL CHECK(approver = 'owner'),
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 approved_at TEXT NOT NULL,
 PRIMARY KEY(run_id, seq),
 UNIQUE(run_id, capability, pinned_version, replacement_version));
 CREATE TRIGGER IF NOT EXISTS workflow_run_substitutions_no_update BEFORE UPDATE ON workflow_run_substitutions
 BEGIN SELECT RAISE(ABORT,'workflow run substitutions are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_run_substitutions_no_delete BEFORE DELETE ON workflow_run_substitutions
 BEGIN SELECT RAISE(ABORT,'workflow run substitutions are immutable'); END;`)
	if err != nil {
		return fmt.Errorf("create workflow run substitution schema: %w", err)
	}
	return nil
}

const substitutionColumns = `run_id,seq,capability,plugin,pinned_version,replacement_version,contract,schema_sha256,evidence,approver,surface,approved_at`

func scanWorkflowRunSubstitution(scanner rowScanner) (WorkflowRunSubstitution, error) {
	var sub WorkflowRunSubstitution
	var approved string
	if err := scanner.Scan(&sub.RunID, &sub.Seq, &sub.Capability, &sub.Plugin, &sub.PinnedVersion, &sub.ReplacementVersion, &sub.Contract,
		&sub.SchemaSHA256, &sub.Evidence, &sub.Approver, &sub.Surface, &approved); err != nil {
		return sub, err
	}
	var err error
	sub.ApprovedAt, err = parseTime(approved)
	return sub, err
}

func workflowRunSubstitutions(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string) ([]WorkflowRunSubstitution, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+substitutionColumns+` FROM workflow_run_substitutions WHERE run_id=? ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowRunSubstitution{}
	for rows.Next() {
		sub, err := scanWorkflowRunSubstitution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// WorkflowRunSubstitutions lists a run's approved substitutions in order.
func (s *Store) WorkflowRunSubstitutions(ctx context.Context, runID string) ([]WorkflowRunSubstitution, error) {
	return workflowRunSubstitutions(ctx, s.db, runID)
}

// RecordWorkflowRunSubstitution records the owner's approval of one
// substitution for an unfinished run, or returns the one already recorded
// for the same capability and versions. The runtime checks the evidence
// against the provider it resolves; the store keeps the record immutable.
func (s *Store) RecordWorkflowRunSubstitution(ctx context.Context, sub WorkflowRunSubstitution) (WorkflowRunSubstitution, error) {
	switch {
	case sub.Surface != "cli" && sub.Surface != "web":
		return WorkflowRunSubstitution{}, ErrWorkflowApprover
	case sub.RunID == "" || sub.Capability == "" || sub.Plugin == "" || sub.PinnedVersion == "" || sub.ReplacementVersion == "" || sub.Contract == "":
		return WorkflowRunSubstitution{}, errors.New("a substitution names its run, capability, plugin, both versions and the contract")
	case sub.ReplacementVersion == sub.PinnedVersion:
		return WorkflowRunSubstitution{}, errors.New("a substitution replaces the pinned version with another")
	case len(sub.SchemaSHA256) != 64 || strings.TrimSpace(sub.Evidence) == "":
		return WorkflowRunSubstitution{}, errors.New("a substitution needs the exact schema hash and the replacement's evidence")
	}
	sub.Approver = "owner"
	var out WorkflowRunSubstitution
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if _, err := runRunnable(ctx, conn, sub.RunID); err != nil {
			return err
		}
		existing, err := scanWorkflowRunSubstitution(conn.QueryRowContext(ctx, `SELECT `+substitutionColumns+` FROM workflow_run_substitutions
 WHERE run_id=? AND capability=? AND pinned_version=? AND replacement_version=?`, sub.RunID, sub.Capability, sub.PinnedVersion, sub.ReplacementVersion))
		if err == nil {
			out = existing
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := s.now().UTC()
		if err := conn.QueryRowContext(ctx, `INSERT INTO workflow_run_substitutions(`+substitutionColumns+`)
 VALUES(?,(SELECT COALESCE(MAX(seq),0)+1 FROM workflow_run_substitutions WHERE run_id=?),?,?,?,?,?,?,?,?,?,?) RETURNING seq`,
			sub.RunID, sub.RunID, sub.Capability, sub.Plugin, sub.PinnedVersion, sub.ReplacementVersion, sub.Contract, sub.SchemaSHA256,
			sub.Evidence, sub.Approver, sub.Surface, formatTime(now)).Scan(&sub.Seq); err != nil {
			return fmt.Errorf("record workflow run substitution: %w", err)
		}
		sub.ApprovedAt = now
		out = sub
		return nil
	})
	return out, err
}
