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
	queries        []retrievalQueryRecord
	origins        map[string]int
	refreshes      int
	toolIDs        map[string]bool
	requestReserve int
	contextBytes   int
	outcomes       []string
	withheld       bool
	lastReceipt    *memory.RetrievalReceipt
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
	} else if r.status == memory.RetrievalExhausted || r.searches > retrievalSearchLimit || r.work >= retrievalTurnWork || retrievalTurnBytes-r.delivered < 512 {
		result.Status = "exhausted"
	} else if r.kernel != nil && os.Getenv("EVIE_REMOTE_MEMORY") == "on" {
		if query.Limit <= 0 || query.Limit > retrievalResultLimit {
			query.Limit = retrievalResultLimit
		}
		if query.MaxBytes <= 0 || query.MaxBytes > retrievalResultBytes {
			query.MaxBytes = retrievalResultBytes
		}
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
	if len(query.RefreshReferences) > 0 {
		// Refresh only lost evidence. A broad old query cannot replace the
		// explicit temporal view or unchanged pin of another held result.
		var refreshed []memory.RetrievalEvidence
		for _, item := range result.Evidence {
			held := false
			for _, current := range r.evidence {
				held = held || current.ID == item.ID
			}
			if !held {
				refreshed = append(refreshed, item)
			}
		}
		result.Evidence = refreshed
	}
	// Validate the internal result before retaining it. Its copied source text
	// is not a tool result; only the next revalidated memory projection sends it.
	encoded, encodeErr := json.Marshal(result)
	if encodeErr != nil || memory.HasRetrievalSecret(encoded) {
		result = memory.RetrievalResult{Status: memory.RetrievalUnavailable}
	}
	if !slices.Contains(r.outcomes, result.Status) {
		r.outcomes = append(r.outcomes, result.Status)
	}
	r.status = combinedRecallStatus(r.outcomes)
	if slices.Contains(r.outcomes, memory.RetrievalExhausted) {
		r.status = memory.RetrievalExhausted
	}
	if slices.Contains(r.outcomes, memory.RetrievalCancelled) {
		r.status = memory.RetrievalCancelled
	}
	if len(result.Evidence) > 0 {
		r.rememberQuery(query, result.Evidence)
	}
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
		if !found {
			if len(r.evidence) == retrievalResultLimit {
				// New requested evidence can replace old context. Keep the incoming
				// result's support group intact and invalidate an evicted older plan
				// so automatic refresh cannot undo the explicit follow-up.
				for i := len(r.evidence) - 1; i >= 0; i-- {
					incoming := false
					for _, item := range result.Evidence {
						incoming = incoming || item.ID == r.evidence[i].ID
					}
					if !incoming {
						r.origins[r.evidence[i].ID] = -1
						r.evidence = append(r.evidence[:i], r.evidence[i+1:]...)
						break
					}
				}
			}
			if len(r.evidence) < retrievalResultLimit {
				r.evidence = append(r.evidence, evidence)
			}
		}
	}
	if len(result.Evidence) > 0 {
		r.withheld = false
	}
	return result, nil
}

// Bind actual call IDs so each serialized replay is charged when a request is
// admitted. Executing a tool is not itself delivery to the provider.
func (r *retrievalTurn) searchForTool(callID string) func(context.Context, memory.RetrievalQuery) (memory.RetrievalResult, error) {
	return func(ctx context.Context, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
		if r.toolIDs == nil {
			r.toolIDs = make(map[string]bool)
		}
		r.toolIDs[callID] = true
		return r.search(ctx, query)
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
			prior := r.evidence
			r.evidence = valid
			if len(valid) < len(prior) {
				r.withheld = true
				r.status = memory.RetrievalPartial
				if len(valid) == 0 {
					r.status = memory.RetrievalUnavailable
				}
				r.refreshInvalidated(ctx, prior, valid)
			}
		}
	}
	if os.Getenv("EVIE_REMOTE_MEMORY") != "on" {
		r.evidence, r.status = nil, memory.RetrievalUnavailable
	}
	if r.withheld && len(r.evidence) == 0 && r.status != memory.RetrievalFailed && r.status != memory.RetrievalCancelled && r.status != memory.RetrievalExhausted {
		r.status = memory.RetrievalUnavailable
	}
	return r.renderProjection()
}

// A sizing preview is local only: compaction sees original durable messages,
// never this synthetic evidence. Revalidate and charge the final projection
// after compaction, immediately before composing the provider-bound request.
func (r *retrievalTurn) renderProjection() (string, *memory.RetrievalReceipt) {
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
	if err != nil || memory.HasRetrievalSecret(encoded) {
		receipt.Evidence = nil
		receipt.Status = "unavailable"
		data.Status, data.Evidence = receipt.Status, nil
		data.ReadingGuide, data.HistoricalOnly = "", nil
		encoded, _ = json.Marshal(data)
	}
	limit := retrievalTurnBytes - r.delivered - r.requestReserve
	if r.contextBytes > 0 {
		limit = min(limit, r.contextBytes)
	}
	for {
		content = "EVIE_MEMORY_DATA\n" + string(encoded)
		serialized, _ = json.Marshal(openrouter.Message{Role: "user", Content: content})
		if len(serialized) <= limit || len(data.Evidence) == 0 {
			break
		}
		receipt.Status, data.Status = memory.RetrievalExhausted, memory.RetrievalExhausted
		remove := len(data.Evidence) - 1
		if r.contextBytes > 0 {
			// A large first finding must not crowd out a smaller original that
			// can still support an answer within the request's actual headroom.
			for i, evidence := range data.Evidence {
				candidate := data
				candidate.Evidence = []memory.RetrievalEvidence{evidence}
				candidate.HistoricalOnly = nil
				if evidence.CurrentStatus == memory.SemanticStatusRetired {
					candidate.HistoricalOnly = []string{evidence.ID}
				}
				body, _ := json.Marshal(candidate)
				message, _ := json.Marshal(openrouter.Message{Role: "user", Content: "EVIE_MEMORY_DATA\n" + string(body)})
				if len(message) > limit {
					remove = i
					break
				}
			}
		}
		data.Evidence = projectionSupportedEvidence(slices.Delete(slices.Clone(data.Evidence), remove, remove+1))
		receipt.Evidence, data.HistoricalOnly = nil, nil
		for _, item := range data.Evidence {
			receipt.Evidence = append(receipt.Evidence, item.Reference())
			if item.CurrentStatus == memory.SemanticStatusRetired {
				data.HistoricalOnly = append(data.HistoricalOnly, item.ID)
			}
		}
		if len(data.Evidence) == 0 {
			data.ReadingGuide = ""
		}
		encoded, _ = json.Marshal(data)
	}
	content = "EVIE_MEMORY_DATA\n" + string(encoded)
	serialized, _ = json.Marshal(openrouter.Message{Role: "user", Content: content})
	return content, receipt
}
