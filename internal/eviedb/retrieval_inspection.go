package eviedb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

// InspectMemoryEvidence resolves an original immutable read, never a new
// relevance search. Later lifecycle state is separate from the original state;
// current source restrictions are applied even to an old reference.
func (s *Store) InspectMemoryEvidence(ctx context.Context, scope memory.ScopeContext, refs []memory.RetrievalReference) ([]memory.RetrievalInspection, error) {
	if len(refs) > 64 {
		return nil, ErrInvalidRetrievalQuery
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = validateSessionScopeIdentity(ctx, tx, scope); err != nil {
		return nil, err
	}
	result := make([]memory.RetrievalInspection, 0, len(refs))
	for _, ref := range refs {
		item := memory.RetrievalInspection{Reference: ref}
		if ref.Kind == memory.RetrievalConversationExcerpt {
			evidence, eligible, err := s.resolveConversationReference(ctx, tx, scope, ref)
			if err != nil {
				return nil, err
			}
			if eligible {
				item.Evidence = &evidence
				item.Available = true
				item.CurrentStatus = memory.SemanticStatusActive
			}
			result = append(result, item)
			continue
		}
		if ref.Kind != memory.RetrievalAcceptedMemory || ref.ClaimID == "" || ref.ClaimOperationID == "" || ref.AsKnownAt.IsZero() || ref.ValidAt.IsZero() {
			result = append(result, item)
			continue
		}
		metadata, err := s.exactReadMetadata(ctx, tx, scope, memory.ClaimQuery{ValidAt: &ref.ValidAt, AsKnownAt: &ref.AsKnownAt}, nil, true)
		if err != nil {
			return nil, err
		}
		if !containsString(metadata.AllowedScopes, ref.ScopeKey) {
			result = append(result, item)
			continue
		}
		state, err := tx.QueryContext(ctx, `SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1`, ref.ClaimID)
		if err != nil {
			return nil, err
		}
		if state.Next() {
			var value memory.SemanticStateValue
			if err = state.Scan(&value); err != nil {
				state.Close()
				return nil, err
			}
			item.CurrentStatus = semanticStatus(value)
		}
		if err = state.Err(); err != nil {
			state.Close()
			return nil, err
		}
		state.Close()
		evidence, eligible, err := s.retrievalClaim(ctx, tx, metadata, ref.ClaimID)
		if err != nil {
			return nil, err
		}
		if eligible && evidence.ID == ref.ID && evidence.ScopeKey == ref.ScopeKey && evidence.ClaimOperationID == ref.ClaimOperationID && sameRetrievalSources(evidence.Reference().Sources, ref.Sources) {
			visible := true
			for _, source := range evidence.Sources {
				visible = visible && source.Evidence != ""
			}
			if visible {
				evidence.Paths = append([]string(nil), ref.Paths...)
				item.Evidence = &evidence
				item.Available = true
			}
		}
		result = append(result, item)
	}
	return result, tx.Commit()
}

func (h *SessionHistory) SearchMemory(ctx context.Context, scope memory.ScopeContext, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
	if scope.SessionID != h.sessionID {
		return memory.RetrievalResult{}, errors.New("retrieval session differs from bound history")
	}
	return h.store.SearchMemory(ctx, scope, query)
}

func (h *SessionHistory) RevalidateMemoryEvidence(ctx context.Context, scope memory.ScopeContext, evidence []memory.RetrievalEvidence) ([]memory.RetrievalEvidence, error) {
	if scope.SessionID != h.sessionID {
		return nil, errors.New("retrieval session differs from bound history")
	}
	return h.store.RevalidateMemoryEvidence(ctx, scope, evidence)
}
