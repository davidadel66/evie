package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// MemoryRetrieval is the read-only Kernel boundary consumed by a turn. Scope
// always comes from the durable session, never from model arguments.
type MemoryRetrieval interface {
	SearchMemory(context.Context, memory.ScopeContext, memory.RetrievalQuery) (memory.RetrievalResult, error)
	RevalidateMemoryEvidence(context.Context, memory.ScopeContext, []memory.RetrievalEvidence) ([]memory.RetrievalEvidence, error)
}

type MemoryActivityEvents interface {
	MemoryRetrieved(memory.Event)
}

const (
	retrievalVersion        = "memory-retrieval-v1"
	retrievalSearchLimit    = 8
	retrievalResultLimit    = 8
	retrievalResultBytes    = 12 * 1024
	retrievalTurnBytes      = 36 * 1024
	retrievalSearchDeadline = 750 * time.Millisecond
	retrievalTurnWork       = 3 * time.Second
)

type retrievalTurn struct {
	kernel    MemoryRetrieval
	scope     memory.ScopeContext
	searches  int
	work      time.Duration
	delivered int
	status    string
	evidence  []memory.RetrievalEvidence
}

func (s *Session) newRetrievalTurn() *retrievalTurn {
	kernel, _ := s.history.(MemoryRetrieval)
	return &retrievalTurn{kernel: kernel, scope: s.scope}
}

func (r *retrievalTurn) search(ctx context.Context, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
	r.searches++
	result := memory.RetrievalResult{Status: "unavailable"}
	// Only the turn can resolve model-visible IDs into previously held evidence.
	// Caller-supplied references and covered ranges never establish authority.
	query.Anchor, query.Covered = nil, nil
	if query.Kind == memory.RetrievalConversationExpansion {
		for _, evidence := range r.evidence {
			if evidence.Kind == memory.RetrievalConversationExcerpt {
				ref := evidence.Reference()
				query.Covered = append(query.Covered, ref)
				if evidence.ID == query.AnchorID {
					query.Anchor = &ref
				}
			}
		}
	}
	validAnchor := query.Kind != memory.RetrievalConversationExpansion || query.Anchor != nil
	if ctx.Err() != nil {
		result.Status = "cancelled"
	} else if !validAnchor {
		result.Status = memory.RetrievalUnavailable
	} else if r.searches > retrievalSearchLimit || r.work >= retrievalTurnWork || r.delivered >= retrievalTurnBytes || len(r.evidence) >= retrievalResultLimit {
		result.Status = "exhausted"
	} else if r.kernel != nil && os.Getenv("EVIE_REMOTE_MEMORY") == "on" {
		if query.Limit <= 0 || query.Limit > retrievalResultLimit {
			query.Limit = retrievalResultLimit
		}
		if query.MaxBytes <= 0 || query.MaxBytes > retrievalResultBytes {
			query.MaxBytes = retrievalResultBytes
		}
		query.Limit = min(query.Limit, retrievalResultLimit-len(r.evidence))
		query.MaxBytes = min(query.MaxBytes, retrievalTurnBytes-r.delivered)
		searchCtx, cancel := context.WithTimeout(ctx, min(retrievalSearchDeadline, retrievalTurnWork-r.work))
		started := time.Now()
		var err error
		result, err = r.kernel.SearchMemory(searchCtx, r.scope, query)
		r.work += time.Since(started)
		cancel()
		if err != nil {
			result = memory.RetrievalResult{Status: "failed"}
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				result.Status = "cancelled"
			}
			if errors.Is(err, context.DeadlineExceeded) {
				result.Status = "exhausted"
			}
		}
	}
	// Charge the complete search result even though the persisted tool outcome
	// contains only its smaller count/status projection. Mixed calls cannot avoid
	// the turn cap by postponing the next provider request.
	encoded, encodeErr := json.Marshal(result)
	if encodeErr != nil || memory.HasRetrievalSecret(encoded) {
		result = memory.RetrievalResult{Status: memory.RetrievalUnavailable}
	} else if len(encoded) > retrievalTurnBytes-r.delivered {
		result = memory.RetrievalResult{Status: memory.RetrievalExhausted}
	} else {
		r.delivered += len(encoded)
	}
	r.status = result.Status
	for _, evidence := range result.Evidence {
		found := false
		for _, existing := range r.evidence {
			if existing.ID == evidence.ID {
				found = true
				break
			}
		}
		if !found && len(r.evidence) < retrievalResultLimit {
			r.evidence = append(r.evidence, evidence)
		}
	}
	return result, nil
}

func (r *retrievalTurn) projection(ctx context.Context) (string, *memory.RetrievalReceipt) {
	if r.status == "" {
		return "", nil
	}
	if os.Getenv("EVIE_REMOTE_MEMORY") != "on" || r.kernel == nil {
		r.evidence = nil
		r.status = "unavailable"
	} else if r.work >= retrievalTurnWork {
		r.evidence = nil
		r.status = memory.RetrievalExhausted
	} else if len(r.evidence) > 0 {
		started := time.Now()
		checkCtx, cancel := context.WithTimeout(ctx, min(retrievalSearchDeadline, retrievalTurnWork-r.work))
		valid, err := r.kernel.RevalidateMemoryEvidence(checkCtx, r.scope, r.evidence)
		cancel()
		r.work += time.Since(started)
		if err != nil {
			r.evidence = nil
			r.status = "unavailable"
		} else {
			r.evidence = valid
		}
	}
	receipt := &memory.RetrievalReceipt{Version: retrievalVersion, Status: r.status, Evidence: []memory.RetrievalReference{}}
	for _, evidence := range r.evidence {
		receipt.Evidence = append(receipt.Evidence, evidence.Reference())
	}
	data := struct {
		Version  string                     `json:"version"`
		Status   string                     `json:"status"`
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}{retrievalVersion, r.status, r.evidence}
	encoded, err := json.Marshal(data)
	content := "EVIE_MEMORY_DATA\n" + string(encoded)
	serialized, _ := json.Marshal(openrouter.Message{Role: "user", Content: content})
	if err != nil || memory.HasRetrievalSecret(encoded) || len(serialized) > retrievalTurnBytes-r.delivered {
		receipt.Evidence = nil
		receipt.Status = "exhausted"
		if err != nil || memory.HasRetrievalSecret(encoded) {
			receipt.Status = "unavailable"
		}
		data.Status, data.Evidence = receipt.Status, nil
		encoded, _ = json.Marshal(data)
	}
	content = "EVIE_MEMORY_DATA\n" + string(encoded)
	serialized, _ = json.Marshal(openrouter.Message{Role: "user", Content: content})
	r.delivered += len(serialized)
	return content, receipt
}
