package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

// projectAcceptedSource never expands a selected byte range to the full event.
// This is shared by review inspection and the older accepted read/promotion
// paths that can subsequently encounter a review-created Source Link.
func projectAcceptedSource(source memory.SemanticSource, reviewOrigin, policyCurrent bool) memory.SemanticSource {
	if !reviewOrigin && source.LocatorKind == memory.LocatorWhole {
		return source
	}
	if reviewOrigin && !policyCurrent {
		source.Evidence = ""
		return source
	}
	locator := memory.EvidenceLocator{EventID: source.EventID, EventPart: source.EventPart, LocatorKind: source.LocatorKind, LocatorValue: source.LocatorValue, EvidenceSHA256: strings.TrimPrefix(source.EvidenceSHA256, "sha256:")}
	if reviewOrigin && compilerHasSecret(source.Evidence) {
		source.Evidence = ""
		return source
	}
	projected, err := projectCompilerSource(memory.CompilerSource{Locator: memory.EvidenceLocator{EventID: source.EventID}, Evidence: source.Evidence}, locator)
	if err != nil {
		source.Evidence = ""
		return source
	}
	source.Evidence = projected.Evidence
	return source
}

func reviewSourcePolicyCurrent(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) bool {
	var policy string
	return q.QueryRowContext(ctx, `SELECT source_policy FROM memory_review_authorization WHERE singleton=1`).Scan(&policy) == nil && policy == memory.CompilerPolicyVersion
}

func loadReviewRecordedSource(ctx context.Context, q reviewQuery, id memory.SemanticID, evidence string) (memory.SemanticSource, error) {
	var source memory.SemanticSource
	err := q.QueryRowContext(ctx, `SELECT source_link_id,created_operation_id,event_id,source_session_id,source_scope_key,event_part,locator_kind,locator_value,evidence_sha256,source_actor,source_type,authority,observed_at,eligibility FROM semantic_source_links WHERE source_link_id=?`, id).Scan(&source.ID, &source.OperationID, &source.EventID, &source.SessionID, &source.ScopeKey, &source.EventPart, &source.LocatorKind, &source.LocatorValue, &source.EvidenceSHA256, &source.Actor, &source.SourceType, &source.Authority, &source.ObservedAt, &source.Eligibility)
	source.Evidence = evidence
	return source, err
}

// InspectOwnerReviewOperation preserves the original reviewed context and
// sources. Current policy applies to old envelopes as well as live candidates.
func (s *Store) InspectOwnerReviewOperation(ctx context.Context, a OwnerReviewContext, id memory.SemanticID) (memory.OwnerReviewOperation, error) {
	var op memory.OwnerReviewOperation
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return op, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return memory.OwnerReviewOperation{}, err
	}
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT o.prepared_proposal_json FROM semantic_operations o JOIN semantic_scopes s ON s.scope_id=o.target_scope_id WHERE o.operation_id=? AND o.schema_version=6 AND s.scope_key=?`, id, a.scope).Scan(&raw)
	if err != nil {
		return memory.OwnerReviewOperation{}, ErrOwnerReviewUnauthorized
	}
	if json.Unmarshal([]byte(raw), &op) != nil {
		return memory.OwnerReviewOperation{}, errors.New("invalid accepted review envelope")
	}
	if err = reviewOperationSourcesVisible(ctx, tx, op); err != nil {
		return memory.OwnerReviewOperation{}, ErrReviewInvalidSource
	}
	return op, tx.Commit()
}

func reviewOperationSourcesVisible(ctx context.Context, q reviewQuery, op memory.OwnerReviewOperation) error {
	if !reviewSourcePolicyCurrent(ctx, q) {
		return ErrReviewInvalidSource
	}
	for _, candidate := range op.Preview.Candidates {
		var requestRaw []byte
		var request memory.CompilerRequest
		if err := q.QueryRowContext(ctx, `SELECT request FROM memory_compiler_jobs WHERE job_id=?`, candidate.JobID).Scan(&requestRaw); err != nil {
			return err
		}
		if json.Unmarshal(requestRaw, &request) != nil {
			return ErrReviewInvalidSource
		}
		owner, err := reviewSourceContext(ctx, q, request.Window.Selection.SessionID)
		if err != nil {
			return err
		}
		if err = compilerAuthorize(ctx, q, owner, request.Window.Selection); err != nil {
			return err
		}
		for _, sources := range [][]memory.CompilerSource{candidate.Candidate.Support, candidate.Candidate.Context} {
			for _, source := range sources {
				if _, err = resolveCompilerSource(ctx, q, owner, request.Window.Selection, source); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
