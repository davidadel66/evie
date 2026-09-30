package openrouter

import "errors"

// WithWorkerLimits gives a worker its own request allowance without changing
// the parent or exceeding the resolved model's route-safe input/output limits.
func (p ContextProfile) WithWorkerLimits(requestBytes, outputTokens int64) (ContextProfile, error) {
	if requestBytes <= 0 || outputTokens <= 0 {
		return ContextProfile{}, errors.New("worker context limits must be positive")
	}
	d := p.Diagnostics()
	if _, err := newContextProfile(d); err != nil {
		return ContextProfile{}, err
	}
	output := min(outputTokens, d.OutputReserveTokens)
	if UsesResponses(d.ConfiguredModel) {
		// The resolved Responses ceiling can encode max_prompt_tokens plus
		// the parent's reserve. Lowering output must not enlarge that prompt cap.
		d.HardWindowTokens -= d.OutputReserveTokens - output
	}
	d.OutputReserveTokens = output
	d.WorkingTokens = d.HardWindowTokens
	if requestBytes < d.HardWindowTokens-output-d.EstimationMarginTokens {
		d.WorkingTokens = requestBytes + output + d.EstimationMarginTokens
	}
	return newContextProfile(d)
}
