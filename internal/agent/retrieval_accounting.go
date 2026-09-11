package agent

import (
	"reflect"

	"github.com/davidadel66/evie/internal/memory"
)

func (r *retrievalTurn) recordAccounting(receipt *memory.RetrievalReceipt) {
	if receipt == nil {
		return
	}
	reused := 0
	if r.lastReceipt != nil {
		for _, current := range receipt.Evidence {
			for _, previous := range r.lastReceipt.Evidence {
				if reflect.DeepEqual(current, previous) {
					reused++
					break
				}
			}
		}
	}
	receipt.Investigation = &memory.RetrievalInvestigation{
		Version: "turn-evidence-v1", SearchAttempts: r.searches, RefreshAttempts: r.refreshes,
		ReusedEvidence: reused, CumulativeMemoryBytes: r.delivered, KernelWorkNanoseconds: r.work.Nanoseconds(),
		Outcomes: append([]string(nil), r.outcomes...),
	}
	r.lastReceipt = receipt
}
