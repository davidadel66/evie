package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
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
	retrievalVersion        = "memory-retrieval-v2"
	retrievalSearchLimit    = 8
	retrievalResultLimit    = 8
	retrievalResultBytes    = 12 * 1024
	retrievalTurnBytes      = 36 * 1024
	retrievalSearchDeadline = 750 * time.Millisecond
	retrievalTurnWork       = 3 * time.Second
)

type retrievalTurn struct {
	interpretation *memory.RetrievalInterpretation
	kernel         MemoryRetrieval
	scope          memory.ScopeContext
	searches       int
	work           time.Duration
	delivered      int
	status         string
	evidence       []memory.RetrievalEvidence
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
	// Validate the internal result before retaining it. Its copied source text
	// is not a tool result; only the next revalidated memory projection sends it.
	encoded, encodeErr := json.Marshal(result)
	if encodeErr != nil || memory.HasRetrievalSecret(encoded) {
		result = memory.RetrievalResult{Status: memory.RetrievalUnavailable}
	}
	r.status = result.Status
	for _, evidence := range result.Evidence {
		found := false
		for i, existing := range r.evidence {
			if existing.ID == evidence.ID {
				// A second current read of the same immutable excerpt must not erase
				// its already-discovered relation to held Claims. Dispatch revalidation
				// still prunes every relation whose supporting Claim is no longer valid.
				// Historical or explicitly constrained reads select an independent view.
				if existing.Intent == memory.RetrievalCurrent && evidence.Intent == memory.RetrievalCurrent &&
					!existing.ValidAtConstrained && !evidence.ValidAtConstrained && slices.Contains(existing.Paths, "newer_owner_statement") {
					for _, id := range existing.RelatedClaimIDs {
						if !slices.Contains(evidence.RelatedClaimIDs, id) {
							evidence.RelatedClaimIDs = append(evidence.RelatedClaimIDs, id)
						}
					}
					if !slices.Contains(evidence.Paths, "newer_owner_statement") {
						evidence.Paths = append(evidence.Paths, "newer_owner_statement")
					}
				}
				// A targeted temporal read may intentionally select a different view of
				// the same immutable Claim or excerpt. Its new request receipt must carry
				// the requested view; older receipts remain untouched.
				r.evidence[i] = evidence
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

// Bind the actual call ID so accounting includes the exact serialized tool
// outcome, not an internal source-bearing object that never reaches a provider.
func (r *retrievalTurn) searchForTool(callID string) func(context.Context, memory.RetrievalQuery) (memory.RetrievalResult, error) {
	return func(ctx context.Context, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
		result, err := r.search(ctx, query)
		if err != nil {
			return result, err
		}
		content, renderErr := memory.RenderRetrievalOutcome(result)
		encoded, encodeErr := json.Marshal(openrouter.Message{Role: "tool", ToolCallID: callID, Content: content})
		if renderErr != nil || encodeErr != nil || len(encoded) > retrievalTurnBytes-r.delivered {
			result = memory.RetrievalResult{Status: memory.RetrievalExhausted}
			r.status = result.Status
		} else {
			r.delivered += len(encoded)
		}
		return result, nil
	}
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
			if len(valid) < len(r.evidence) {
				r.status = memory.RetrievalPartial
				if len(valid) == 0 {
					r.status = memory.RetrievalUnavailable
				}
			}
			r.evidence = valid
		}
	}
	return r.renderProjection(true)
}

// A sizing preview is local only: compaction sees original durable messages,
// never this synthetic evidence. Revalidate and charge the final projection
// after compaction, immediately before composing the provider-bound request.
func (r *retrievalTurn) renderProjection(charge bool) (string, *memory.RetrievalReceipt) {
	if r.status == "" {
		return "", nil
	}
	receipt := &memory.RetrievalReceipt{Version: retrievalVersion, Status: r.status, Evidence: []memory.RetrievalReference{}, Interpretation: r.interpretation}
	for _, evidence := range r.evidence {
		receipt.Evidence = append(receipt.Evidence, evidence.Reference())
	}
	data := struct {
		Version        string                     `json:"version"`
		Status         string                     `json:"status"`
		ReadingGuide   string                     `json:"reading_guide,omitempty"`
		HistoricalOnly []string                   `json:"historical_only,omitempty"`
		Evidence       []memory.RetrievalEvidence `json:"evidence"`
	}{Version: retrievalVersion, Status: r.status, Evidence: r.evidence}
	if len(r.evidence) > 0 {
		data.ReadingGuide = "current_status:retired cannot establish a current fact, even with status:active at as_known_at. Prefer paraphrases with original event citations. Use quotation marks only for verbatim source text, preserving case and punctuation; keep formatting outside the quotation. Cite that source entry's event_id and actor, never a nearby result. Assistant inference and reported speech are not owner confirmation."
	}
	if r.interpretation != nil {
		data.ReadingGuide += " " + automaticReferenceReadingGuide
	}
	for _, evidence := range r.evidence {
		if evidence.CurrentStatus == memory.SemanticStatusRetired {
			data.HistoricalOnly = append(data.HistoricalOnly, evidence.ID)
		}
	}
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
		data.ReadingGuide, data.HistoricalOnly = "", nil
		encoded, _ = json.Marshal(data)
	}
	content = "EVIE_MEMORY_DATA\n" + string(encoded)
	serialized, _ = json.Marshal(openrouter.Message{Role: "user", Content: content})
	if charge {
		r.delivered += len(serialized)
	}
	return content, receipt
}
