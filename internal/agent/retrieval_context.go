package agent

import (
	"encoding/json"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// fitContext keeps supplemental memory within the complete request's actual
// headroom. It never evicts original discussion to make space for retrieved
// copies. Pressure already present without memory keeps the existing automatic
// compaction contract; memory alone must not make that otherwise fitting turn
// require an impossible compaction of its active root.
func (r *retrievalTurn) fitContext(input ContextComposeInput, composer *ContextComposer) (ContextComposeInput, error) {
	r.contextBytes = 0
	input.MemoryData, input.MemoryReceipt = r.renderProjection()
	if input.MemoryData == "" {
		return input, nil
	}
	prepared, err := composer.prepare(input)
	if err != nil {
		return input, err
	}
	original := input
	original.MemoryData, original.MemoryReceipt = "", nil
	baseline, err := composer.projectAtStart(original, prepared, prepared.start)
	if err != nil {
		return input, err
	}
	ceiling := min(prepared.usable, percentageFloor(prepared.profile.WorkingTokens, automaticCompactionThresholdPercent)-1)
	if baseline.estimate.SerializedBytes > ceiling {
		return input, nil
	}
	// Each reduction removes at least one whole evidence item. Source bytes,
	// locators and support groups are never truncated to satisfy the budget.
	for attempts := 0; attempts <= len(r.evidence)+1; attempts++ {
		projection, err := composer.projectAtStart(input, prepared, prepared.start)
		if err != nil {
			return input, err
		}
		if projection.estimate.SerializedBytes <= ceiling {
			return input, nil
		}
		message, err := json.Marshal(openrouter.Message{Role: "user", Content: input.MemoryData})
		if err != nil {
			return input, err
		}
		limit := int64(len(message)) - (projection.estimate.SerializedBytes - ceiling)
		if limit <= 0 || len(input.MemoryReceipt.Evidence) == 0 {
			return input, fmt.Errorf("%w: no room for the memory limit notice while preserving the original request", ErrContextOverflow)
		}
		r.contextBytes = int(limit)
		r.status = memory.RetrievalExhausted
		input.MemoryData, input.MemoryReceipt = r.renderProjection()
	}
	return input, fmt.Errorf("%w: memory projection could not fit the original request headroom", ErrContextOverflow)
}
