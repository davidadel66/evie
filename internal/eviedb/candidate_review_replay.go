package eviedb

import (
	"context"
	"database/sql"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func ownerReviewReplayHandler(operation semanticAcceptedReplayOperation) (semanticReplayPreparedHandler, error) {
	var op memory.OwnerReviewOperation
	if err := strictSemanticProposal(operation.PreparedJSON, &op); err != nil {
		return semanticReplayPreparedHandler{}, err
	}
	if err := validateOwnerReviewOperation(op); err != nil {
		return semanticReplayPreparedHandler{}, err
	}
	effect, err := reviewReplayEffect(op)
	if err != nil {
		return semanticReplayPreparedHandler{}, err
	}
	return semanticReplayPreparedHandler{SchemaVersion: 6, Kind: op.Kind, OperationID: op.OperationID, IdempotencyKey: op.IdempotencyKey, Actor: op.Actor, SessionID: op.SessionID, TargetScope: effect.Scope, Scopes: effect.Scopes, PriorRevisions: effect.PriorRevisions, SourceEventID: op.SourceEventID, PreparedProposal: op, CanonicalProposal: canonicalOwnerReviewOperation(op), CanonicalEffect: canonicalOwnerReviewEffect(op.Preview.Effect), ValidateResult: func(record semanticAcceptedReplayOperation) error {
		return validateConcreteReplayResult(record, &memory.OwnerReviewOperationResult{}, func(r memory.OwnerReviewOperationResult) (memory.SemanticID, time.Time, []memory.ScopeRevision) {
			return r.OperationID, r.TransactionTime, r.ResultingRevisions
		}, func(r memory.OwnerReviewOperationResult) bool {
			claims, sources := []memory.SemanticID{}, []memory.SemanticID{}
			for _, item := range effect.Claims {
				claims = append(claims, item.Claim.ID)
				for _, source := range item.Sources {
					sources = append(sources, source.ID)
				}
			}
			return string(compilerJSON(claims)) == string(compilerJSON(r.ClaimIDs)) && string(compilerJSON(sources)) == string(compilerJSON(r.SourceLinkIDs))
		})
	}, Replay: func(ctx context.Context, s *Store, record semanticAcceptedReplayOperation) (any, error) {
		var result memory.OwnerReviewOperationResult
		err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
			if err := validateOwnerReviewHistoricalSources(ctx, conn, op); err != nil {
				return err
			}
			var err error
			result, err = s.applyOwnerReviewOperation(ctx, conn, op, record.TransactionTime)
			return err
		})
		return result, err
	}}, nil
}
