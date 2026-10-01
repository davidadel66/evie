package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

const memoryDataPrefix = "EVIE_MEMORY_DATA\n"

// memoryUnit is one distinct piece of memory delivery: an evidence item of
// the synthetic memory message, charged for the escaped bytes it adds there,
// or one replayed memory-tool outcome message. The key is its exact encoding.
type memoryUnit struct {
	key   [sha256.Size]byte
	bytes int
}

func evidenceUnit(raw []byte) (memoryUnit, error) {
	escaped, err := json.Marshal(string(raw))
	if err != nil {
		return memoryUnit{}, err
	}
	return memoryUnit{key: sha256.Sum256(append([]byte("evidence\x00"), raw...)), bytes: len(escaped) - 2}, nil
}

// blockEvidenceUnits lists the evidence items of one memory message. The
// envelope (status, gaps, reading guide) is harness metadata, not evidence.
func blockEvidenceUnits(content string) ([]memoryUnit, error) {
	var block struct {
		Evidence []json.RawMessage `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(content, memoryDataPrefix)), &block); err != nil {
		return nil, fmt.Errorf("decode memory message: %w", err)
	}
	units := make([]memoryUnit, 0, len(block.Evidence))
	for _, raw := range block.Evidence {
		unit, err := evidenceUnit(raw)
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

// undelivered keeps the units this turn has not yet delivered, once each.
func (r *retrievalTurn) undelivered(units []memoryUnit) ([]memoryUnit, int) {
	var fresh []memoryUnit
	total := 0
	for _, unit := range units {
		if _, delivered := r.deliveredUnits[unit.key]; delivered || slices.ContainsFunc(fresh, func(seen memoryUnit) bool {
			return seen.key == unit.key
		}) {
			continue
		}
		fresh = append(fresh, unit)
		total += unit.bytes
	}
	return fresh, total
}

func (r *retrievalTurn) deliver(units ...[]memoryUnit) {
	if r.deliveredUnits == nil {
		r.deliveredUnits = make(map[[sha256.Size]byte]struct{})
	}
	for _, list := range units {
		for _, unit := range list {
			r.deliveredUnits[unit.key] = struct{}{}
			r.delivered += unit.bytes
		}
	}
}

// newEvidenceBytes is what delivering evidence would add to this turn's
// cumulative memory delivery.
func (r *retrievalTurn) newEvidenceBytes(evidence []memory.RetrievalEvidence) int {
	units := make([]memoryUnit, 0, len(evidence))
	for _, item := range evidence {
		raw, err := json.Marshal(item)
		if err != nil {
			return math.MaxInt
		}
		unit, err := evidenceUnit(raw)
		if err != nil {
			return math.MaxInt
		}
		units = append(units, unit)
	}
	_, total := r.undelivered(units)
	return total
}

// Admission charges each distinct memory byte once per turn: every evidence
// item of the memory message and every replayed outcome of this turn's memory
// tools, the first time its exact bytes reach the provider. Re-sending
// already-delivered evidence is not new delivery. Ordinary durable history and
// all other request fields remain covered by the composer's complete-request
// accounting. When new evidence would exceed the budget, the request keeps
// what was delivered and admits only the new evidence that still fits, and
// investigation is closed. A hard overflow cannot authorize another request.
func (r *retrievalTurn) admitRequest(request openrouter.ChatRequest) (string, *memory.RetrievalReceipt, bool, error) {
	var evidence, outcomes []memoryUnit
	for _, message := range request.Messages {
		switch {
		case message.Role == "user" && strings.HasPrefix(message.Content, memoryDataPrefix):
			units, err := blockEvidenceUnits(message.Content)
			if err != nil {
				return "", nil, false, err
			}
			evidence = append(evidence, units...)
		case message.Role == "tool" && r.toolIDs[message.ToolCallID]:
			encoded, err := json.Marshal(message)
			if err != nil {
				return "", nil, false, err
			}
			outcomes = append(outcomes, memoryUnit{
				key: sha256.Sum256(append([]byte("outcome\x00"), encoded...)), bytes: len(encoded),
			})
		}
	}
	evidence, evidenceBytes := r.undelivered(evidence)
	outcomes, outcomeBytes := r.undelivered(outcomes)
	if evidenceBytes+outcomeBytes <= retrievalTurnBytes-r.delivered {
		r.deliver(evidence, outcomes)
		return "", nil, false, nil
	}
	r.status = memory.RetrievalExhausted
	r.requestReserve = outcomeBytes
	defer func() { r.requestReserve = 0 }()
	data, receipt := r.renderProjection()
	units, err := blockEvidenceUnits(data)
	if err != nil {
		return "", nil, false, err
	}
	evidence, evidenceBytes = r.undelivered(units)
	if outcomeBytes+evidenceBytes > retrievalTurnBytes-r.delivered {
		return "", nil, false, fmt.Errorf("%w: cumulative memory budget exhausted; earlier sources remain inspectable, but further investigation is unavailable", ErrContextOverflow)
	}
	r.deliver(evidence, outcomes)
	return data, receipt, true, nil
}

// Trimming a projection never fabricates support for an orphaned graph path.
// Held evidence is unchanged; only this request's selected view is reduced.
func projectionSupportedEvidence(evidence []memory.RetrievalEvidence) []memory.RetrievalEvidence {
	selected := make(map[memory.SemanticID]memory.RetrievalEvidence)
	for _, item := range evidence {
		if item.ClaimID != "" {
			selected[item.ClaimID] = item
		}
	}
	var supported []memory.RetrievalEvidence
	for _, item := range evidence {
		paths := item.GraphPaths
		item.GraphPaths = nil
		for _, path := range paths {
			complete := true
			for _, id := range path.ClaimIDs {
				claim, found := selected[id]
				complete = complete && found && claim.Intent == item.Intent && claim.ValidAt.Equal(item.ValidAt) && claim.AsKnownAt.Equal(item.AsKnownAt) && claim.ValidAtConstrained == item.ValidAtConstrained
			}
			if complete {
				item.GraphPaths = append(item.GraphPaths, path)
			}
		}
		graph, direct := false, false
		for _, reason := range item.Paths {
			if strings.HasPrefix(reason, "graph_") {
				graph = true
			} else {
				direct = true
			}
		}
		if !graph || direct || len(item.GraphPaths) > 0 {
			supported = append(supported, item)
		}
	}
	selected = make(map[memory.SemanticID]memory.RetrievalEvidence)
	for _, item := range supported {
		if item.ClaimID != "" {
			selected[item.ClaimID] = item
		}
	}
	for i := range supported {
		item := &supported[i]
		conflicts := item.Conflicts
		item.Conflicts = nil
		for _, conflict := range conflicts {
			complete := true
			for _, id := range conflict.ClaimIDs {
				_, found := selected[id]
				complete = complete && found
			}
			if complete {
				item.Conflicts = append(item.Conflicts, conflict)
			}
		}
		related := item.RelatedClaimIDs
		item.RelatedClaimIDs = nil
		for _, id := range related {
			if _, found := selected[id]; found {
				item.RelatedClaimIDs = append(item.RelatedClaimIDs, id)
			}
		}
	}
	return supported
}
