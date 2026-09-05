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
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
)

func ensureCandidateReviewNavigationSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS memory_review_navigation_reconcile (
	 singleton INTEGER PRIMARY KEY CHECK(singleton=1), last_candidate TEXT NOT NULL, cutoff TEXT NOT NULL, complete INTEGER NOT NULL CHECK(complete IN (0,1))
	);
	INSERT OR IGNORE INTO memory_review_navigation_reconcile SELECT 1,'',COALESCE(MAX(candidate_id),''),CASE WHEN MAX(candidate_id) IS NULL THEN 1 ELSE 0 END FROM memory_compiler_candidates;`)
	return err
}

// Candidate publication already inserts/advances the per-scope inbox ledger.
// This bounded, restartable pass covers candidates published before that ledger
// existed. It never changes candidate bytes or existing inbox revisions.
func reconcileCandidateNavigation(ctx context.Context, q *sql.Conn) (bool, error) {
	var last, cutoff string
	var complete bool
	if err := q.QueryRowContext(ctx, `SELECT last_candidate,cutoff,complete FROM memory_review_navigation_reconcile WHERE singleton=1`).Scan(&last, &cutoff, &complete); err != nil {
		return false, err
	}
	if complete {
		return false, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT c.candidate_id,j.destination FROM memory_compiler_candidates c JOIN memory_compiler_jobs j ON j.job_id=c.job_id WHERE c.candidate_id>? AND c.candidate_id<=? ORDER BY c.candidate_id LIMIT 63`, last, cutoff)
	if err != nil {
		return false, err
	}
	type entry struct{ id, scope string }
	entries := []entry{}
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.id, &e.scope); err != nil {
			rows.Close()
			return false, err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if _, err = q.ExecContext(ctx, `INSERT OR IGNORE INTO memory_review_inbox_revisions(scope_key,revision) VALUES(?,0)`, e.scope); err != nil {
			return false, err
		}
		last = e.id
	}
	complete = len(entries) < 63 || last == cutoff
	_, err = q.ExecContext(ctx, `UPDATE memory_review_navigation_reconcile SET last_candidate=?,complete=? WHERE singleton=1`, last, complete)
	return !complete, err
}

// ListLocalOwnerCandidateScopes is a trusted local-host navigation seam, never
// a model-visible tool. It discloses registry labels, not candidates or source
// bytes. Candidate access separately mints one exact OwnerReviewContext.
func (s *Store) ListLocalOwnerCandidateScopes(ctx context.Context, query memory.OwnerCandidateScopeQuery) (memory.OwnerCandidateScopes, error) {
	out := memory.OwnerCandidateScopes{Scopes: []memory.OwnerCandidateScope{}}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 100 || len(query.Cursor) > 2048 {
		return out, errors.New("invalid review scope page")
	}
	err := s.withImmediateTransaction(ctx, func(tx *sql.Conn) error {
		var err error
		if out.Indexing, err = reconcileCandidateNavigation(ctx, tx); err != nil {
			return err
		}
		var key []byte
		var revision int64
		if err = tx.QueryRowContext(ctx, `SELECT authentication_key,revision FROM memory_review_authorization WHERE singleton=1`).Scan(&key, &revision); err != nil {
			return err
		}
		seal := func(last string) string {
			mac := hmac.New(sha256.New, key)
			mac.Write([]byte(fmt.Sprintf("owner-review-scope-page-v1:%d:%s", revision, last)))
			return hex.EncodeToString(mac.Sum(nil))
		}
		var cursor struct {
			Last string
			Seal string
		}
		if query.Cursor != "" {
			b, e := base64.RawURLEncoding.DecodeString(query.Cursor)
			if e != nil || json.Unmarshal(b, &cursor) != nil || !hmac.Equal([]byte(cursor.Seal), []byte(seal(cursor.Last))) {
				return ErrInvalidCursor
			}
		}
		// Bound inspected destinations, including inaccessible rows. A page may be
		// empty with a next cursor; archived registries never force an unbounded scan.
		rows, err := tx.QueryContext(ctx, `SELECT scope_key FROM (SELECT scope_key FROM (SELECT scope_key FROM memory_review_inbox_revisions WHERE scope_key>? ORDER BY scope_key LIMIT ?) UNION SELECT 'global' WHERE 'global'>?) ORDER BY scope_key LIMIT ?`, cursor.Last, query.Limit+1, cursor.Last, query.Limit+1)
		if err != nil {
			return err
		}
		keys := []string{}
		for rows.Next() {
			var key string
			if err = rows.Scan(&key); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, key)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		more := len(keys) > query.Limit
		if more {
			keys = keys[:query.Limit]
		}
		for _, scope := range keys {
			if _, err = reviewScopeKeys(ctx, tx, scope); err != nil {
				continue
			}
			kind, id, err := splitScopeKey(scope)
			if err != nil {
				continue
			}
			label := "Global memory"
			switch kind {
			case "workspace":
				err = tx.QueryRowContext(ctx, `SELECT display_name FROM workspaces WHERE id=?`, id).Scan(&label)
			case "project":
				err = tx.QueryRowContext(ctx, `SELECT display_name FROM projects WHERE id=?`, id).Scan(&label)
			case "session":
				err = tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(title,''),'Conversation from ' || substr(created_at,1,16)) FROM sessions WHERE id=?`, id).Scan(&label)
			}
			if err != nil {
				return err
			}
			out.Scopes = append(out.Scopes, memory.OwnerCandidateScope{ScopeKey: scope, Label: label, Kind: kind})
		}
		if more {
			cursor.Last = keys[len(keys)-1]
			cursor.Seal = seal(cursor.Last)
			b, _ := json.Marshal(cursor)
			out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
		}
		return nil
	})
	if err != nil {
		return memory.OwnerCandidateScopes{}, err
	}
	return out, nil
}
