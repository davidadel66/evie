package agent

import (
	"fmt"
	"reflect"

	"github.com/davidadel66/evie/internal/memory"
)

// requestHistory is what one loaded history yields to every composition pass
// of a request built from it: its validated in-turn compactions and its
// calibrated bytes-per-token ratio. A request composes several times
// (fitting memory, planning compaction, the final composition), and each pass
// used to decode and validate every context snapshot in the session again.
// It is used by one turn at a time.
type requestHistory struct {
	events          []memory.Event
	turnCompactions map[memory.EventID]*acceptedTurnCompaction
	// ratios are calibrated ratios by canonical model, before any bound a
	// rejected request sets.
	ratios map[string]tokenRatio
}

// verifyRequestHistory makes every reuse also derive the value afresh and
// panic on any difference. The package's tests turn it on, so each existing
// turn scenario checks the reuse against the uncached derivation.
var verifyRequestHistory = false

func newRequestHistory(events []memory.Event) (*requestHistory, error) {
	compactions, err := durableTurnCompactions(events)
	if err != nil {
		return nil, err
	}
	return &requestHistory{events: events, turnCompactions: compactions, ratios: make(map[string]tokenRatio)}, nil
}

// holds reports whether events is the very history h was derived from: the
// same slice, not merely equal contents. Appending a compaction makes a new
// history, which is derived afresh.
func (h *requestHistory) holds(events []memory.Event) bool {
	return h != nil && len(events) == len(h.events) && (len(events) == 0 || &events[0] == &h.events[0])
}

func (h *requestHistory) compactions(events []memory.Event) (map[memory.EventID]*acceptedTurnCompaction, error) {
	if !h.holds(events) {
		return durableTurnCompactions(events)
	}
	if verifyRequestHistory {
		fresh, err := durableTurnCompactions(events)
		if err != nil || !reflect.DeepEqual(fresh, h.turnCompactions) {
			panic(fmt.Sprintf("agent: reused in-turn compactions differ from the history's (%v)", err))
		}
	}
	return h.turnCompactions, nil
}

// calibratedRatio is estimator's ratio for canonicalModel over events, or the
// zero ratio for an estimator that does not calibrate.
func (h *requestHistory) calibratedRatio(estimator RequestEstimator, events []memory.Event, canonicalModel string) tokenRatio {
	calibrated, ok := estimator.(tokenRatioEstimator)
	if !ok {
		return tokenRatio{}
	}
	if !h.holds(events) {
		return calibrated.bytesPerToken(events, canonicalModel)
	}
	ratio, ok := h.ratios[canonicalModel]
	if !ok {
		ratio = calibrated.bytesPerToken(events, canonicalModel)
		h.ratios[canonicalModel] = ratio
	} else if verifyRequestHistory && calibrated.bytesPerToken(events, canonicalModel) != ratio {
		panic("agent: reused token ratio differs from the history's")
	}
	return ratio
}
