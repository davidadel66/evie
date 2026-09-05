package eviedb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type diagnosticCursor struct {
	Second   string `json:"second,omitempty"`
	Key      string `json:"key"`
	Sequence int64  `json:"sequence"`
	Seal     string `json:"seal"`
}

func diagnosticPageLimit(limit int) (int, error) {
	if limit == 0 {
		limit = 32
	}
	if limit < 1 || limit > memory.CompilerDiagnosticsMaxPage {
		return 0, ErrReviewInvalidRequest
	}
	return limit, nil
}

func diagnosticPageCursor(a OwnerReviewContext, session memory.SessionID, view, generation, raw string) (diagnosticCursor, error) {
	var c diagnosticCursor
	if len(raw) > 2048 {
		return c, ErrInvalidCursor
	}
	if raw != "" {
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || json.Unmarshal(b, &c) != nil || !hmac.Equal([]byte(c.Seal), []byte(diagnosticCursorSeal(a, session, view, generation, c))) {
			return c, ErrInvalidCursor
		}
	}
	return c, nil
}
func diagnosticCursorSeal(a OwnerReviewContext, session memory.SessionID, view, generation string, c diagnosticCursor) string {
	mac := hmac.New(sha256.New, a.seal)
	mac.Write(compilerJSON(struct {
		Domain                        string
		Session                       memory.SessionID
		View, Generation, Key, Second string
		Sequence                      int64
	}{"compiler-diagnostics-v1", session, view, generation, c.Key, c.Second, c.Sequence}))
	return hex.EncodeToString(mac.Sum(nil))
}
func diagnosticNext(a OwnerReviewContext, q memory.CompilerDiagnosticsQuery, c diagnosticCursor) string {
	c.Seal = diagnosticCursorSeal(a, q.SessionID, q.View, q.GenerationID, c)
	return base64.RawURLEncoding.EncodeToString(compilerJSON(c))
}

func diagnosticSource(ctx context.Context, q reviewQuery, a OwnerReviewContext, id memory.SessionID) (memory.ScopeContext, error) {
	if err := checkReviewAuthority(ctx, q, a); err != nil {
		return memory.ScopeContext{}, err
	}
	owner, err := reviewSourceContext(ctx, q, id)
	if err != nil {
		return owner, err
	}
	source := scopeKeyForContext(owner)
	if a.scope != source && a.scope != "session:"+string(id) {
		return owner, ErrOwnerReviewUnauthorized
	}
	if _, err = reviewScopeKeys(ctx, q, source); err != nil {
		return owner, err
	}
	return owner, nil
}

// Session navigation seeks one exact lineage's registry index. It inspects at
// most 32 registry rows, includes closed sessions, and never returns titles.
func (s *Store) ListOwnerCompilerSessions(ctx context.Context, a OwnerReviewContext, q memory.CompilerDiagnosticSessionQuery) (memory.CompilerDiagnosticSessions, error) {
	out := memory.CompilerDiagnosticSessions{SessionIDs: []memory.SessionID{}}
	limit, err := diagnosticPageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	c, err := diagnosticPageCursor(a, "", "sessions", "", q.Cursor)
	if err != nil {
		return out, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return out, err
	}
	kind, id, err := splitScopeKey(a.scope)
	if err != nil {
		return out, err
	}
	clause := "workspace_id IS NULL AND project_id IS NULL"
	args := []any{}
	switch kind {
	case "workspace":
		clause = "workspace_id=?"
		args = append(args, id)
	case "project":
		clause = "project_id=? AND workspace_id IS NULL"
		args = append(args, id)
	case "session":
		clause = "id=?"
		args = append(args, id)
	}
	args = append(args, c.Key, limit)
	rows, err := tx.QueryContext(ctx, `SELECT id FROM sessions WHERE `+clause+` AND id>? ORDER BY id LIMIT ?`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id memory.SessionID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		out.SessionIDs = append(out.SessionIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.SessionIDs) == limit {
		c.Key = string(out.SessionIDs[len(out.SessionIDs)-1])
		out.NextCursor = diagnosticNext(a, memory.CompilerDiagnosticsQuery{View: "sessions"}, c)
	}
	return out, tx.Commit()
}

// Every view has a keyset and a fixed work budget. Counts are incrementally
// maintained, never recomputed by scanning retained jobs/candidates/events.
// Pages are transaction snapshots; a cursor is not a cross-request snapshot.
func (s *Store) InspectOwnerCompilerDiagnostics(ctx context.Context, a OwnerReviewContext, q memory.CompilerDiagnosticsQuery) (memory.CompilerDiagnostics, error) {
	out := memory.CompilerDiagnostics{ScopeKey: a.scope, SessionID: q.SessionID, View: q.View, Counts: map[string]int64{}, Jobs: []memory.CompilerDiagnosticJob{}, Candidates: []memory.CompilerDiagnosticCandidate{}, Activations: []memory.CompilerActivation{}, History: []memory.CompilerDiagnosticHistory{}, Selection: []memory.CompilerDiagnosticSelection{}, Foreground: []memory.CompilerForegroundMeasurement{}, CapacityState: "available", Selections: []memory.CompilerDiagnosticUnit{}, LiveRoots: []memory.CompilerDiagnosticRoot{}}
	limit, err := diagnosticPageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	if q.View == "" {
		q.View = "jobs"
		out.View = q.View
	}
	if q.SessionID == "" || len(q.SessionID) > 512 || (q.GenerationID != "" && (len(q.GenerationID) != 64 || !isDiagnosticHash(q.GenerationID))) {
		return out, ErrReviewInvalidRequest
	}
	switch q.View {
	case "jobs", "candidates", "activations", "history", "foreground", "selections", "live_roots":
		if q.GenerationID != "" {
			return out, ErrReviewInvalidRequest
		}
	case "selection":
		if q.GenerationID == "" {
			return out, ErrReviewInvalidRequest
		}
	default:
		return out, ErrReviewInvalidRequest
	}
	c, err := diagnosticPageCursor(a, q.SessionID, q.View, q.GenerationID, q.Cursor)
	if err != nil {
		return out, err
	}
	// A bounded legacy pass is a separate transaction, so polling cannot enlarge
	// the snapshot's work with the size of a prior installation.
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if _, e := diagnosticSource(ctx, conn, a, q.SessionID); e != nil {
			return e
		}
		return reconcileCompilerDiagnostics(ctx, conn)
	})
	if err != nil {
		return out, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	owner, err := diagnosticSource(ctx, tx, a, q.SessionID)
	if err != nil {
		return out, err
	}
	out.AsOfUnixMS = time.Now().UnixMilli()
	var counts string
	err = tx.QueryRowContext(ctx, `SELECT revision,counts FROM memory_compiler_diagnostic_sessions WHERE destination=? AND session_id=?`, a.scope, q.SessionID).Scan(&out.Revision, &counts)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if counts != "" {
		var stored map[string]int64
		if json.Unmarshal([]byte(counts), &stored) != nil {
			return out, ErrReviewInvalidSource
		}
		for key, value := range stored {
			if value < 0 {
				return out, ErrReviewInvalidSource
			}
			if diagnosticCounterKey(key) {
				out.Counts[key] = value
			}
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT NOT complete FROM memory_compiler_diagnostic_reconcile WHERE singleton=1`).Scan(&out.Indexing); err != nil {
		return out, err
	}
	var capacity string
	err = tx.QueryRowContext(ctx, `SELECT state FROM memory_compiler_capacity WHERE singleton=1`).Scan(&capacity)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if capacity == "reserved" {
		out.CapacityState = "busy"
	}
	if capacity == "release_pending" {
		out.CapacityState = "capacity_blocked"
	}
	switch q.View {
	case "jobs":
		err = diagnosticJobs(ctx, tx, a, q, limit, c, &out)
	case "candidates":
		err = diagnosticCandidates(ctx, tx, a, q, limit, c, &out)
	case "activations":
		err = diagnosticActivations(ctx, tx, a, owner, q, limit, c, &out)
	case "history":
		err = diagnosticHistory(ctx, tx, a, q, limit, c, &out)
	case "selection":
		err = diagnosticSelection(ctx, tx, a, owner, q, limit, c, &out)
	case "selections":
		err = diagnosticUnits(ctx, tx, a, q, limit, c, &out)
	case "live_roots":
		err = diagnosticRoots(ctx, tx, a, q, limit, c, &out)
	case "foreground":
		err = diagnosticForeground(ctx, tx, a, q, limit, c, &out)
	}
	if err != nil {
		return memory.CompilerDiagnostics{}, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	if compilerHasSecret(string(b)) {
		return memory.CompilerDiagnostics{}, ErrReviewInvalidSource
	}
	if len(b) > 128*1024 {
		return memory.CompilerDiagnostics{}, ErrReviewTooLarge
	}
	return out, tx.Commit()
}
func isDiagnosticHash(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}
func diagnosticCounterKey(key string) bool {
	switch key {
	case "jobs_queued", "jobs_running", "jobs_retry_wait", "jobs_staged", "jobs_cancelled", "jobs_failed", "jobs_completed_candidates", "jobs_completed_empty", "jobs_excluded", "attempts", "cancellations", "candidates_unresolved", "candidates_accepted", "candidates_rejected", "candidates_suppressed":
		return true
	}
	return false
}

func reconcileCompilerDiagnostics(ctx context.Context, conn *sql.Conn) error {
	var last, cutoff string
	var complete bool
	if err := conn.QueryRowContext(ctx, `SELECT last_job,cutoff,complete FROM memory_compiler_diagnostic_reconcile WHERE singleton=1`).Scan(&last, &cutoff, &complete); err != nil {
		return err
	}
	if complete {
		return nil
	}
	rows, err := conn.QueryContext(ctx, `SELECT job_id FROM memory_compiler_jobs WHERE job_id>? AND job_id<=? ORDER BY job_id LIMIT 15`, last, cutoff)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		_, err = conn.ExecContext(ctx, `INSERT OR IGNORE INTO memory_compiler_diagnostic_jobs(job_id,destination,session_id,state,attempts,unresolved,accepted,rejected,suppressed) SELECT j.job_id,j.destination,j.session_id,j.state,j.attempts,(SELECT COUNT(*) FROM memory_compiler_candidates WHERE job_id=j.job_id AND review_state='unresolved' AND equivalent_to IS NULL),(SELECT COUNT(*) FROM memory_compiler_candidates WHERE job_id=j.job_id AND review_state='accepted'),(SELECT COUNT(*) FROM memory_compiler_candidates WHERE job_id=j.job_id AND review_state='rejected'),(SELECT COUNT(*) FROM memory_compiler_candidates WHERE job_id=j.job_id AND equivalent_to IS NOT NULL) FROM memory_compiler_jobs j WHERE j.job_id=?`, id)
		if err != nil {
			return err
		}
		last = id
	}
	_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_diagnostic_reconcile SET last_job=?,complete=? WHERE singleton=1`, last, len(ids) < 15 || last == cutoff)
	return err
}
