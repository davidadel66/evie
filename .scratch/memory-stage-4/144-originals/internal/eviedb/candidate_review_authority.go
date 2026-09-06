package eviedb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"sort"

	"github.com/davidadel66/evie/internal/memory"
)

var (
	ErrOwnerReviewUnauthorized = errors.New("owner review unauthorized")
	ErrReviewStale             = errors.New("stale_preview")
	ErrReviewResolved          = errors.New("already_resolved")
	ErrReviewInvalidSource     = errors.New("source_ineligible")
)

// OwnerReviewContext is an opaque capability. Only the trusted local owner
// entry point creates one. Deserializing request JSON yields no authority.
type OwnerReviewContext struct {
	scope    string
	revision int64
	binding  string
	seal     []byte
}

func (a OwnerReviewContext) ScopeKey() string { return a.scope }

type reviewQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func reviewCapabilityMAC(key []byte, scope string, revision int64) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(compilerJSON(struct {
		Domain, Scope string
		Revision      int64
	}{"evie-owner-review-context-v1", scope, revision}))
	return mac.Sum(nil)
}

// LocalOwnerReviewContext is a trusted host seam, not a model-visible tool.
// Web callers must invoke it only after their existing owner authentication.
func (s *Store) LocalOwnerReviewContext(ctx context.Context, scope string) (OwnerReviewContext, error) {
	var a OwnerReviewContext
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if _, err := reviewScopeKeys(ctx, conn, scope); err != nil {
			return ErrOwnerReviewUnauthorized
		}
		var key []byte
		if err := conn.QueryRowContext(ctx, `SELECT revision,authentication_key FROM memory_review_authorization WHERE singleton=1`).Scan(&a.revision, &key); err != nil {
			return err
		}
		a.scope = scope
		a.binding = memory.CompilerHash(key)
		a.seal = reviewCapabilityMAC(key, scope, a.revision)
		return nil
	})
	return a, err
}

func checkReviewAuthority(ctx context.Context, q reviewQuery, a OwnerReviewContext) error {
	var revision int64
	var key []byte
	if err := q.QueryRowContext(ctx, `SELECT revision,authentication_key FROM memory_review_authorization WHERE singleton=1`).Scan(&revision, &key); err != nil {
		return err
	}
	if a.scope == "" || a.revision != revision || a.binding != memory.CompilerHash(key) || !hmac.Equal(a.seal, reviewCapabilityMAC(key, a.scope, revision)) {
		return ErrOwnerReviewUnauthorized
	}
	if _, err := reviewScopeKeys(ctx, q, a.scope); err != nil {
		return ErrOwnerReviewUnauthorized
	}
	return nil
}

func reviewScopeKeys(ctx context.Context, q reviewQuery, key string) ([]string, error) {
	kind, id, err := splitScopeKey(key)
	if err != nil {
		return nil, err
	}
	keys := []string{"global"}
	switch kind {
	case "global":
	case "workspace":
		var state string
		if err := q.QueryRowContext(ctx, `SELECT lifecycle_state FROM workspaces WHERE id=?`, id).Scan(&state); err != nil || state != "active" {
			return nil, ErrOwnerReviewUnauthorized
		}
		keys = append(keys, key)
	case "project":
		var archived int
		if err := q.QueryRowContext(ctx, `SELECT archived FROM projects WHERE id=?`, id).Scan(&archived); err != nil || archived != 0 {
			return nil, ErrOwnerReviewUnauthorized
		}
		keys = append(keys, key)
	case "session":
		owner, err := reviewSourceContext(ctx, q, memory.SessionID(id))
		if err != nil {
			return nil, err
		}
		contextKey := scopeKeyForContext(owner)
		if contextKey != "global" {
			if _, err := reviewScopeKeys(ctx, q, contextKey); err != nil {
				return nil, err
			}
			keys = append(keys, contextKey)
		}
		keys = append(keys, key)
	default:
		return nil, ErrOwnerReviewUnauthorized
	}
	sort.Strings(keys)
	if err := requireSemanticScopeKeysAvailable(ctx, q, keys); err != nil {
		return nil, err
	}
	return keys, nil
}

func reviewSourceContext(ctx context.Context, q reviewQuery, id memory.SessionID) (memory.ScopeContext, error) {
	owner := memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: id}
	var workspace, project sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT workspace_id,project_id FROM sessions WHERE id=?`, id).Scan(&workspace, &project); err != nil {
		return owner, err
	}
	owner.WorkspaceID = memory.WorkspaceID(workspace.String)
	owner.ProjectID = memory.ProjectID(project.String)
	return owner, nil
}

func reviewCursor(a OwnerReviewContext, revision int64, last string) string {
	mac := hmac.New(sha256.New, a.seal)
	mac.Write(compilerJSON(struct {
		Revision int64
		Last     string
	}{revision, last}))
	return hex.EncodeToString(mac.Sum(nil))
}
