package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

// Promotion preserves the original evidence identity. Following its exact
// source-Claim edge recovers review origin even though copied Source Links are
// created by a v4 operation. The allowed scope path has at most two Promotions:
// session -> Context Scope -> global. This traversal never searches by prose.
func reviewSourceOrigin(ctx context.Context, q historicalReviewQuery, id memory.SemanticID) (memory.SemanticID, error) {
	for depth := 0; depth < 3; depth++ {
		rows, err := q.QueryContext(ctx, `SELECT o.operation_id,o.schema_version,o.operation_kind,parent.source_link_id
   FROM semantic_source_links child JOIN semantic_operations o ON o.operation_id=child.created_operation_id
   LEFT JOIN semantic_promotions p ON p.operation_id=o.operation_id AND p.destination_claim_id=child.claim_id
   LEFT JOIN semantic_source_links parent ON parent.claim_id=p.source_claim_id
    AND parent.event_id=child.event_id AND parent.source_session_id=child.source_session_id
    AND parent.source_scope_key=child.source_scope_key AND parent.event_part=child.event_part
    AND parent.locator_kind=child.locator_kind AND parent.locator_value=child.locator_value
    AND parent.evidence_sha256=child.evidence_sha256 AND parent.source_actor=child.source_actor
    AND parent.source_type=child.source_type AND parent.authority=child.authority AND parent.observed_at=child.observed_at
   WHERE child.source_link_id=?`, id)
		if err != nil {
			return "", err
		}
		var op memory.SemanticID
		var version int
		var kind string
		var parent sql.NullString
		if !rows.Next() {
			err = rows.Err()
			rows.Close()
			if err != nil {
				return "", err
			}
			return "", ErrReviewInvalidSource
		}
		err = rows.Scan(&op, &version, &kind, &parent)
		duplicate := rows.Next()
		rowErr := rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
		if rowErr != nil {
			return "", rowErr
		}
		if duplicate {
			return "", ErrReviewInvalidSource
		}
		if version == 6 && kind == "owner_candidate_review" {
			return op, nil
		}
		if version != 4 || kind != "promote_claim" {
			return "", nil
		}
		if !parent.Valid {
			return "", ErrReviewInvalidSource
		}
		id = memory.SemanticID(parent.String)
	}
	return "", errors.New("review source promotion lineage exceeds scope path")
}

type promotionReviewQuery struct{ writer turnLeaseWriteExecutor }

func (q promotionReviewQuery) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return q.writer.queryContext(ctx, query, args...)
}

type historicalPromotionReviewKey struct{}

func withHistoricalPromotionReview(ctx context.Context) context.Context {
	return context.WithValue(ctx, historicalPromotionReviewKey{}, true)
}
func isHistoricalPromotionReview(ctx context.Context) bool {
	v, _ := ctx.Value(historicalPromotionReviewKey{}).(bool)
	return v
}

func validateReviewOriginVisibility(ctx context.Context, q historicalReviewQuery, id memory.SemanticID) error {
	rows, err := q.QueryContext(ctx, `SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=? AND schema_version=6 AND operation_kind='owner_candidate_review'`, id)
	if err != nil {
		return err
	}
	var raw string
	if !rows.Next() {
		rows.Close()
		return ErrReviewInvalidSource
	}
	err = rows.Scan(&raw)
	rows.Close()
	if err != nil {
		return err
	}
	var op memory.OwnerReviewOperation
	if json.Unmarshal([]byte(raw), &op) != nil || validateOwnerReviewOperation(op) != nil {
		return ErrReviewInvalidSource
	}
	if err = validateOwnerReviewHistoricalSources(ctx, q, op); err != nil {
		return err
	}
	if isHistoricalPromotionReview(ctx) {
		return nil
	}
	rows, err = q.QueryContext(ctx, `SELECT source_policy FROM memory_review_authorization WHERE singleton=1`)
	if err != nil {
		return err
	}
	var policy string
	if !rows.Next() {
		rows.Close()
		return ErrReviewInvalidSource
	}
	err = rows.Scan(&policy)
	rows.Close()
	if err != nil {
		return err
	}
	if policy != op.Preview.SourcePolicy || policy != memory.CompilerPolicyVersion {
		return ErrReviewInvalidSource
	}
	for _, candidate := range op.Preview.Candidates {
		for _, sources := range [][]memory.CompilerSource{candidate.Candidate.Support, candidate.Candidate.Context} {
			for _, source := range sources {
				rows, err = q.QueryContext(ctx, `SELECT e.content,COALESCE(w.lifecycle_state,'active'),COALESCE(p.archived,0),
    EXISTS(SELECT 1 FROM semantic_projection_quarantine quarantine JOIN semantic_scopes scope ON scope.scope_id=quarantine.scope_id WHERE scope.scope_key IN (?,?))
    FROM events e JOIN sessions s ON s.id=e.session_id LEFT JOIN workspaces w ON w.id=s.workspace_id LEFT JOIN projects p ON p.id=s.project_id WHERE e.id=?`, source.ScopeKey, op.Preview.ScopeKey, source.Locator.EventID)
				if err != nil {
					return err
				}
				var content, state string
				var archived, quarantined int
				if !rows.Next() {
					rows.Close()
					return ErrReviewInvalidSource
				}
				err = rows.Scan(&content, &state, &archived, &quarantined)
				rows.Close()
				if err != nil {
					return err
				}
				if state != "active" || archived != 0 || quarantined != 0 || compilerHasSecret(content) {
					return ErrReviewInvalidSource
				}
			}
		}
	}
	return nil
}

func projectSourceWithReviewOrigin(ctx context.Context, q historicalReviewQuery, source memory.SemanticSource) (memory.SemanticSource, bool, error) {
	origin, err := reviewSourceOrigin(ctx, q, source.ID)
	if err != nil {
		return source, false, err
	}
	if origin != "" {
		if err = validateReviewOriginVisibility(ctx, q, origin); err != nil {
			return source, true, err
		}
	}
	if isHistoricalPromotionReview(ctx) {
		locator := memory.EvidenceLocator{EventID: source.EventID, EventPart: source.EventPart, LocatorKind: source.LocatorKind, LocatorValue: source.LocatorValue, EvidenceSHA256: strings.TrimPrefix(source.EvidenceSHA256, "sha256:")}
		projected, err := projectHistoricalCompilerSource(acceptedCompilerSource(source), locator)
		if err != nil {
			return source, origin != "", err
		}
		source.Evidence = projected.Evidence
		return source, origin != "", nil
	}
	source = projectAcceptedSource(source, origin != "", true)
	if origin != "" && source.Evidence == "" {
		return source, true, ErrReviewInvalidSource
	}
	return source, origin != "", nil
}

func renderSourceWithReviewOrigin(ctx context.Context, q historicalReviewQuery, source memory.SemanticSource) memory.SemanticSource {
	projected, _, err := projectSourceWithReviewOrigin(ctx, q, source)
	if err != nil {
		source.Evidence = ""
		return source
	}
	return projected
}

func requirePromotionReviewDisclosure(ctx context.Context, q historicalReviewQuery, sources []memory.SemanticSource) error {
	for _, source := range sources {
		origin, err := reviewSourceOrigin(ctx, q, source.ID)
		if err != nil {
			return err
		}
		if origin != "" {
			if source.Evidence == "" {
				return ErrReviewInvalidSource
			}
			if err = validateReviewOriginVisibility(ctx, q, origin); err != nil {
				return err
			}
		}
	}
	return nil
}
