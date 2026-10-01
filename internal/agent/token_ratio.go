package agent

import (
	"encoding/json"
	"math"

	"github.com/davidadel66/evie/internal/memory"
)

// Token budgets (window, working ceiling, reserves) are converted to canonical
// request bytes with a per-model bytes-per-token ratio, in thousandths. The
// ratio is a deterministic function of durable history, so every request and
// /context see the same value for the same stored samples.
const (
	// uncalibratedBytesPerTokenMilli caps the ratio until enough samples
	// exist. Typical text measures 4.2-4.7 canonical bytes per provider token,
	// so 3 keeps the estimate an over-estimate of tokens.
	uncalibratedBytesPerTokenMilli = int64(3000)
	// Provider usage samples are kept within one and six bytes per token, so
	// one bad counter can neither collapse nor inflate the budget.
	minimumUsageBytesPerTokenMilli = int64(1000)
	maximumBytesPerTokenMilli      = int64(6000)
	// tokenCalibrationMinSamples recorded requests lift the uncalibrated cap;
	// the ratio is the minimum over the newest tokenCalibrationWindow samples.
	tokenCalibrationMinSamples = 8
	tokenCalibrationWindow     = 32
)

// tokenRatio is the bytes-per-token ratio one request uses. A zero milli is
// the original one-byte-per-token estimate and is not recorded.
type tokenRatio struct {
	milli   int64
	samples int
}

// tokenRatioEstimator is implemented by estimators that calibrate the ratio
// from durable history. Other estimators charge one token per byte.
type tokenRatioEstimator interface {
	bytesPerToken(events []memory.Event, canonicalModel string) tokenRatio
}

// CalibratedRequestEstimator measures the same canonical request bytes as
// CanonicalRequestEstimator and converts token budgets to bytes with a
// per-model ratio calibrated from recorded provider usage.
type CalibratedRequestEstimator struct{ CanonicalRequestEstimator }

func (CalibratedRequestEstimator) Version() string { return CalibratedRequestEstimatorVersion }

// bytesPerToken pairs each assistant message's reported input tokens with the
// canonical bytes of the context snapshot sent for the same trigger. A second
// snapshot for one trigger is the single compact-and-retry after a provider
// context-length rejection; the rejected request proves at most
// bytes/(hard window - output reserve) bytes per token and is a sample too.
// Only requests to canonicalModel count. The ratio is the minimum over the
// newest samples, capped at the uncalibrated floor until enough exist.
func (CalibratedRequestEstimator) bytesPerToken(events []memory.Event, canonicalModel string) tokenRatio {
	pending := make(map[memory.EventID]memory.ContextSnapshotPayload)
	var samples []int64
	for _, event := range events {
		switch event.Type {
		case memory.EventContextSnapshot:
			var snapshot memory.ContextSnapshotPayload
			if json.Unmarshal(event.Payload, &snapshot) != nil {
				continue
			}
			if rejected, ok := pending[event.ParentID]; ok && rejected.CanonicalModel == canonicalModel {
				samples = append(samples, rejectionBytesPerTokenMilli(
					rejected.SerializedBytes, rejected.HardWindowTokens, rejected.OutputReserveTokens,
				))
			}
			pending[event.ParentID] = snapshot
		case memory.EventAssistantMessage:
			snapshot, ok := pending[event.ParentID]
			delete(pending, event.ParentID)
			if !ok || snapshot.CanonicalModel != canonicalModel || snapshot.SerializedBytes <= 0 {
				continue
			}
			var payload memory.AssistantMessagePayload
			if json.Unmarshal(event.Payload, &payload) != nil || payload.Usage == nil ||
				payload.Usage.InputTokens == nil || *payload.Usage.InputTokens <= 0 {
				continue
			}
			sample := bytesPerTokenMilli(snapshot.SerializedBytes, *payload.Usage.InputTokens)
			samples = append(samples, max(minimumUsageBytesPerTokenMilli, min(sample, maximumBytesPerTokenMilli)))
		}
	}
	if len(samples) > tokenCalibrationWindow {
		samples = samples[len(samples)-tokenCalibrationWindow:]
	}
	ratio := tokenRatio{milli: uncalibratedBytesPerTokenMilli, samples: len(samples)}
	if len(samples) >= tokenCalibrationMinSamples {
		ratio.milli = maximumBytesPerTokenMilli
	}
	for _, sample := range samples {
		ratio.milli = min(ratio.milli, sample)
	}
	return ratio
}

// rejectionBytesPerTokenMilli bounds the ratio after a provider rejected a
// request of the given bytes for context length: its input exceeded the hard
// window less the output reserve requested with it.
func rejectionBytesPerTokenMilli(bytes, hardWindowTokens, outputReserveTokens int64) int64 {
	limit := hardWindowTokens - outputReserveTokens
	if bytes <= 0 || limit <= 0 {
		return 1
	}
	return max(1, min(bytesPerTokenMilli(bytes, limit), maximumBytesPerTokenMilli))
}

// contextTokenRatio is the ratio a request composed from events uses. A
// rejected request in this turn caps it at the bound that rejection proved.
func contextTokenRatio(
	estimator RequestEstimator,
	events []memory.Event,
	canonicalModel string,
	hardWindowTokens, outputReserveTokens, rejectedRequestBytes int64,
) tokenRatio {
	var ratio tokenRatio
	if calibrated, ok := estimator.(tokenRatioEstimator); ok {
		ratio = calibrated.bytesPerToken(events, canonicalModel)
	}
	if rejectedRequestBytes > 0 {
		bound := rejectionBytesPerTokenMilli(rejectedRequestBytes, hardWindowTokens, outputReserveTokens)
		if ratio.milli == 0 || bound < ratio.milli {
			ratio.milli = bound
		}
	}
	return ratio
}

// bytesPerTokenMilli is bytes/tokens in thousandths, rounded down so the
// ratio never overstates how many bytes one token covers.
func bytesPerTokenMilli(bytes, tokens int64) int64 {
	if bytes > math.MaxInt64/1000 {
		return maximumBytesPerTokenMilli
	}
	return bytes * 1000 / tokens
}

func (r tokenRatio) budgetBytes(tokens int64) int64 {
	return memory.ContextBudgetBytes(tokens, r.milli)
}
