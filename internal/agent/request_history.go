package agent

import (
	"context"
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
	events []memory.Event
	// decodes is the turn's, shared by all of its requests.
	decodes         *turnDecodes
	turnCompactions map[memory.EventID]*acceptedTurnCompaction
	// ratios are calibrated ratios by canonical model, before any bound a
	// rejected request sets.
	ratios map[string]tokenRatio
}

// verifyRequestHistory makes every reuse also derive the value afresh and
// panic on any difference. The package's tests turn it on, so each existing
// turn scenario checks the reuse against the uncached derivation.
var verifyRequestHistory = false

func newRequestHistory(events []memory.Event, decodes *turnDecodes) (*requestHistory, error) {
	compactions, err := decodedTurnCompactions(events, decodes)
	if verifyRequestHistory {
		fresh, freshErr := durableTurnCompactions(events)
		if fmt.Sprint(freshErr) != fmt.Sprint(err) || !reflect.DeepEqual(fresh, compactions) {
			panic(fmt.Sprintf("agent: in-turn compactions from reused decodes differ from fresh ones (%v, %v)", err, freshErr))
		}
	}
	if err != nil {
		return nil, err
	}
	return &requestHistory{events: events, decodes: decodes, turnCompactions: compactions, ratios: make(map[string]tokenRatio)}, nil
}

// holds reports whether events is the very history h was derived from: the
// same slice, not merely equal contents. Appending a compaction makes a new
// history, which is derived afresh.
func (h *requestHistory) holds(events []memory.Event) bool {
	return h != nil && len(events) == len(h.events) && (len(events) == 0 || &events[0] == &h.events[0])
}

func (h *requestHistory) compactions(events []memory.Event) (map[memory.EventID]*acceptedTurnCompaction, error) {
	if !h.holds(events) {
		if h == nil {
			return durableTurnCompactions(events)
		}
		return decodedTurnCompactions(events, h.decodes)
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
	if h == nil {
		return calibrated.bytesPerToken(events, canonicalModel, nil)
	}
	ratio, ok := h.ratios[canonicalModel]
	if !ok || !h.holds(events) {
		ratio = calibrated.bytesPerToken(events, canonicalModel, h.decodes)
		if h.holds(events) {
			h.ratios[canonicalModel] = ratio
		}
	}
	if verifyRequestHistory && calibrated.bytesPerToken(events, canonicalModel, nil) != ratio {
		panic("agent: reused token ratio differs from a fresh one")
	}
	return ratio
}

// turnEvents is the durable history for a turn's next request. The turn keeps
// what it loaded and, when the history can list them, reads only the events
// committed since; the first request, or a history without a tail, loads it
// whole. The result never shares spare capacity with loaded, so an append to
// it cannot write into what the turn keeps.
func (s *Session) turnEvents(ctx context.Context, loaded []memory.Event) ([]memory.Event, error) {
	tail, ok := s.history.(historyTail)
	if !ok || len(loaded) == 0 {
		return s.history.Events(ctx)
	}
	newer, err := tail.EventsAfter(ctx, loaded[len(loaded)-1].Sequence)
	if err != nil {
		return nil, err
	}
	events := append(loaded[:len(loaded):len(loaded)], newer...)
	if verifyRequestHistory {
		full, err := s.history.Events(ctx)
		if err != nil || !reflect.DeepEqual(full, events) {
			panic(fmt.Sprintf("agent: incrementally loaded history differs from a full load (%v)", err))
		}
	}
	return events, nil
}
