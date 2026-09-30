package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

func ensureSubagentSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS subagent_executions (
 id TEXT PRIMARY KEY, parent_session_id TEXT NOT NULL REFERENCES sessions(id),
 idempotency_key TEXT NOT NULL, request_digest TEXT NOT NULL,
 child_session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id),
 state TEXT NOT NULL CHECK(state IN ('admitted','running','succeeded','failed','cancelled','interrupted')),
 record_json TEXT NOT NULL, UNIQUE(parent_session_id,idempotency_key)
 ); CREATE INDEX IF NOT EXISTS subagent_execution_state ON subagent_executions(state,parent_session_id);`)
	return err
}

// AdmitSubagents validates the entire batch and atomically reserves every new
// child, receipt and attempt. Retained keys never create runnable work again.
func (s *Store) AdmitSubagents(ctx context.Context, p delegation.Parent, requests []delegation.Assignment, receipt composition.Receipt, policy delegation.Policy, dispatchID string) ([]delegation.Attempt, error) {
	if dispatchID == "" {
		return nil, errors.New("subagent dispatch identity is required")
	}
	if err := policy.ValidateBatch(requests); err != nil {
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
		var payload string
		if err := conn.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE id=?`, p.IntentEventID).Scan(&payload); err != nil {
			return err
		}
		var intent memory.ToolIntentPayload
		if err := json.Unmarshal([]byte(payload), &intent); err != nil {
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

		for _, r := range requests {
			if err := authorizeSubagentTask(ctx, conn, p, r.TaskID); err != nil {
				return err
			}
			a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE parent_session_id=? AND idempotency_key=?`, p.Scope.SessionID, r.Key))
			if err == nil {
				if a.Digest != delegation.Digest(r) {
					return delegation.ErrConflict
				}
				attempts = append(attempts, a)
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			now := s.now().UTC()
			child := memory.Session{ID: memory.SessionID(uuid.NewString()), ParentSessionID: p.Scope.SessionID, Status: memory.SessionActive, CreatedAt: now, UpdatedAt: now,
				WorkspaceID: p.Scope.WorkspaceID, WorkspaceRevisionSnapshot: p.Scope.WorkspaceRevision, ProjectID: p.Scope.ProjectID, ProjectRootSnapshot: p.Scope.ProjectRoot}
			if _, err = conn.ExecContext(ctx, `INSERT INTO sessions(id,workspace_id,workspace_revision_snapshot,project_id,project_root_snapshot,parent_session_id,status,created_at,updated_at)
    VALUES (?,NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),?,?,?,?)`, child.ID, child.WorkspaceID, child.WorkspaceRevisionSnapshot, child.ProjectID, child.ProjectRootSnapshot, child.ParentSessionID, child.Status, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				return err
			}
			if err = insertCompositionReceipt(ctx, conn, child.ID, encoded, now); err != nil {
				return err
			}
			a = delegation.Attempt{ID: uuid.NewString(), DispatchID: dispatchID, Parent: p, Assignment: r, Digest: delegation.Digest(r), Child: child, Receipt: receipt, Policy: policy, PolicyID: delegation.Digest(policy), State: "admitted", CreatedAt: now}
			data, err := json.Marshal(a)
			if err != nil {
				return err
			}
			if _, err = conn.ExecContext(ctx, `INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`, a.ID, p.Scope.SessionID, r.Key, a.Digest, child.ID, a.State, string(data)); err != nil {
				return err
			}
			attempts = append(attempts, a)
		}
		return s.authorizeSubagentParent(ctx, conn, p, receipt)
	})
	return attempts, err
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

func (s *Store) authorizeSubagentParent(ctx context.Context, conn *sql.Conn, p delegation.Parent, child composition.Receipt) error {
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

	if p.Scope.WorkspaceID != "" {
		if err := authorizeWorkspaceResearch(ctx, conn, p.Scope); err != nil {
			return err
		}
	}
	if err := fenceTurnLeaseWrite(ctx, conn, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, s.now().UTC().Format(turnLeaseTimeFormat)); err != nil {
		return err
	}
	for _, plugin := range []string{"subagents", "web"} {
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
	if _, ok := caps[delegation.CapabilityID]; !ok {
		return errors.New("parent composition does not permit delegation")
	}
	if child.Preset.ID != "research" || len(child.Capabilities) != 2 {
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
	var payload string
	if err := conn.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE id=? AND session_id=? AND event_type='tool_intent'`, p.IntentEventID, p.Scope.SessionID).Scan(&payload); err != nil {
		return errors.New("delegation requires a committed source invocation")
	}
	var intent memory.ToolIntentPayload
	if err = json.Unmarshal([]byte(payload), &intent); err != nil || intent.Call.Name != delegation.ToolName {
		return delegation.ErrAuthority
	}
	if intent.Lease == nil || intent.Lease.SessionID != p.Lease.SessionID || intent.Lease.HolderID != p.Lease.HolderID || intent.Lease.FencingToken != p.Lease.FencingToken {
		return delegation.ErrAuthority
	}
	var linked bool
	if err = conn.QueryRowContext(ctx, `WITH RECURSIVE chain(id,parent_id,depth) AS (
 SELECT id,parent_id,0 FROM events WHERE id=? AND session_id=?
 UNION ALL SELECT e.id,e.parent_id,chain.depth+1 FROM events e JOIN chain ON e.id=chain.parent_id WHERE e.session_id=? AND chain.depth<128
 ) SELECT EXISTS(SELECT 1 FROM chain WHERE id=?) AND NOT EXISTS(
 SELECT 1 FROM events WHERE session_id=? AND sequence>(SELECT sequence FROM events WHERE id=?) AND event_type IN ('user_message','turn_failed','turn_interrupted')
 ) AND NOT EXISTS(SELECT 1 FROM events WHERE session_id=? AND execution_id=(SELECT execution_id FROM events WHERE id=?) AND event_type IN ('tool_succeeded','tool_failed','tool_cancelled'))`, p.IntentEventID, p.Scope.SessionID, p.Scope.SessionID, p.SourceEventID, p.Scope.SessionID, p.SourceEventID, p.Scope.SessionID, p.IntentEventID).Scan(&linked); err != nil {
		return err
	}
	if !linked {
		return delegation.ErrAuthority
	}
	var validRoot bool
	if err = conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events WHERE id=? AND session_id=? AND event_type='user_message' AND parent_id IS NULL)`, p.SourceEventID, p.Scope.SessionID).Scan(&validRoot); err != nil {
		return err
	}
	if !validRoot {
		return delegation.ErrAuthority
	}
	return nil
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
func (s *Store) InspectSubagent(ctx context.Context, p delegation.Parent, id string) (delegation.Attempt, error) {
	var a delegation.Attempt
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		a, err = readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=? AND parent_session_id=?`, id, p.Scope.SessionID))
		if err != nil {
			return err
		}
		if p.Scope.OwnerID != memory.LocalOwnerID || p.Scope.ParentSessionID != "" {
			return delegation.ErrAuthority
		}
		if err = validateSessionScope(ctx, conn, p.Scope); err != nil {
			return err
		}
		if p.Scope.WorkspaceID != "" {
			var active bool
			if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id=? AND lifecycle_state='active')`, p.Scope.WorkspaceID).Scan(&active); err != nil {
				return err
			}
			if !active || p.Scope.WorkspaceID != a.Parent.Scope.WorkspaceID || p.Scope.WorkspaceRevision != a.Parent.Scope.WorkspaceRevision {
				return delegation.ErrAuthority
			}
		}
		if p.Scope.ProjectID != "" {
			var archived bool
			if err = conn.QueryRowContext(ctx, `SELECT archived FROM projects WHERE id=?`, p.Scope.ProjectID).Scan(&archived); err != nil {
				return err
			}
			if archived {
				return ErrProjectNotActive
			}
		}

		return authorizeSubagentTask(ctx, conn, p, a.Assignment.TaskID)
	})
	return a, err
}
