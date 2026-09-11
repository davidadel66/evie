package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// Admission accounts for each complete escaped memory message and each replay
// of a retrieval outcome produced in this turn. Ordinary durable history and
// all other request fields remain covered by the composer's complete-request
// accounting. A hard control-message overflow cannot authorize another request.
func (r *retrievalTurn) admitRequest(request openrouter.ChatRequest) (string, *memory.RetrievalReceipt, bool, error) {
	initial, replay := 0, 0
	for _, message := range request.Messages {
		memoryData := message.Role == "user" && strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n")
		outcome := message.Role == "tool" && r.toolIDs[message.ToolCallID]
		if !memoryData && !outcome {
			continue
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return "", nil, false, err
		}
		initial += len(encoded)
		if outcome {
			replay += len(encoded)
		}
	}
	remaining := retrievalTurnBytes - r.delivered
	// Close further investigation while a complete supported answer still
	// fits. Otherwise a successful continuation can consume the last useful
	// projection and leave only an empty exhausted block for the reader.
	finalSupportedRequest := len(r.evidence) > 0 && initial <= remaining && initial > remaining-initial
	if initial <= remaining && !finalSupportedRequest {
		r.delivered += initial
		return "", nil, false, nil
	}
	r.status = memory.RetrievalExhausted
	r.requestReserve = replay
	defer func() { r.requestReserve = 0 }()
	data, receipt := r.renderProjection()
	encoded, err := json.Marshal(openrouter.Message{Role: "user", Content: data})
	if err != nil {
		return "", nil, false, err
	}
	if replay+len(encoded) > retrievalTurnBytes-r.delivered {
		return "", nil, false, fmt.Errorf("%w: cumulative memory budget exhausted; earlier sources remain inspectable, but further investigation is unavailable", ErrContextOverflow)
	}
	r.delivered += replay + len(encoded)
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
