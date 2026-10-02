package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

var ErrNoLegalAutomaticCompaction = errors.New("agent: no legal automatic compaction can satisfy the target")

const (
	automaticCompactionThresholdPercent = int64(80)
	automaticCompactionTargetPercent    = int64(60)
)

func automaticCompactionRequired(serializedBytes, workingCeiling int64) bool {
	return serializedBytes >= percentageFloor(workingCeiling, automaticCompactionThresholdPercent)
}

// automaticCompactionSummaryReserveBytes is the encoded summary size planning
// reserves: the 16 KiB summary limit plus 25 percent for the JSON escaping of
// typical Markdown. Every accepted summary is measured again before it is
// committed, so an unusually escaped one cannot overrun the usable budget.
const automaticCompactionSummaryReserveBytes = CompactionSummaryMaxBytes * 5 / 4

// selectAutomaticCompaction first measures the complete projected request at
// the active summary frontier. Under pressure it selects the smallest legal
// contiguous prefix whose replacement by a reserved-size summary leaves the
// canonical request at or below the target. If no such prefix exists, an
// unchanged projection within the usable input budget can still proceed. A
// recovery from a provider context-length rejection is always under pressure.
func selectAutomaticCompaction(
	input ContextComposeInput,
	composer *ContextComposer,
) (compactionPlan, bool, error) {
	return selectAutomaticCompactionFrom(input, input.Events, composer)
}

// selectAutomaticCompactionFrom plans like selectAutomaticCompaction when
// input.Events is a projection of stored, the durable events, with tool-result
// content shortened (a worker's wrap-up fit). Pressure and every candidate
// are measured on input, the request actually sent, while the covered turns
// the compactor summarizes come from stored, so a summary never records
// excerpts or omission markers as the conversation (amended 2026-10-01).
func selectAutomaticCompactionFrom(
	input ContextComposeInput,
	stored []memory.Event,
	composer *ContextComposer,
) (compactionPlan, bool, error) {
	if len(stored) != len(input.Events) {
		return compactionPlan{}, true, errors.New("automatic compaction source does not match the projected history")
	}
	for i := range stored {
		if stored[i].ID != input.Events[i].ID {
			return compactionPlan{}, true, errors.New("automatic compaction source does not match the projected history")
		}
	}
	prepared, err := composer.prepare(input)
	if err != nil {
		return compactionPlan{}, false, err
	}
	profile, turns := prepared.profile, prepared.turns
	activeIndex, start := prepared.activeIndex, prepared.start
	projection, err := composer.projectAtStart(input, prepared, start)
	if err != nil {
		return compactionPlan{}, true, err
	}
	if input.RejectedRequestBytes <= 0 &&
		!automaticCompactionRequired(projection.estimate.SerializedBytes, prepared.workingBytes()) {
		return compactionPlan{}, false, nil
	}

	activeSummary, chain, err := reconstructCompactionChain(stored)
	if err != nil {
		return compactionPlan{}, true, err
	}
	compactionTurns, err := compactionRootTurns(stored)
	if err != nil {
		return compactionPlan{}, true, err
	}
	if len(compactionTurns) != len(turns) {
		return compactionPlan{}, true, errors.New("automatic compaction turn projection is inconsistent")
	}
	compactorUsable, err := compactionUsableInputBytes(profile, prepared.ratio)
	if err != nil {
		return compactionPlan{}, true, ErrNoLegalAutomaticCompaction
	}
	target := percentageFloor(prepared.workingBytes(), automaticCompactionTargetPercent)
	planningSummary := planningCompactionSummary()
	for retainedIndex := start + 1; retainedIndex <= activeIndex; retainedIndex++ {
		covered := compactionTurns[start:retainedIndex]
		if !covered[len(covered)-1].complete {
			break
		}
		request, err := renderCompactionRequest(input.Profile, activeSummary, covered)
		if err != nil {
			return compactionPlan{}, true, err
		}
		compactorEstimate, err := composer.estimator.Estimate(request)
		if err != nil {
			return compactionPlan{}, true, fmt.Errorf("estimate automatic compactor request: %w", err)
		}
		if compactorEstimate.SerializedBytes > compactorUsable {
			break
		}

		candidate := input
		candidate.Summary = &ContextSummary{
			CompactionEventID:    "automatic-planning",
			FirstRetainedEventID: turns[retainedIndex][0].ID,
			Content:              planningSummary,
		}
		candidateProjection, err := composer.projectAtStart(candidate, prepared, retainedIndex)
		if err != nil {
			return compactionPlan{}, true, err
		}
		if candidateProjection.estimate.SerializedBytes > target {
			continue
		}

		generation := int64(1)
		var priorID memory.EventID
		if len(chain) != 0 {
			generation = chain[len(chain)-1].Payload.Generation + 1
			priorID = chain[len(chain)-1].Event.ID
		}
		return compactionPlan{
			Request: request, PriorSummary: summaryContent(activeSummary), Generation: generation, PriorCompactionEventID: priorID,
			CoveredFirst:  covered[0].events[0],
			CoveredLast:   covered[len(covered)-1].events[len(covered[len(covered)-1].events)-1],
			FirstRetained: compactionTurns[retainedIndex].events[0],
			summaryFits: func(summary string) (bool, error) {
				actual := input
				actual.Summary = &ContextSummary{
					CompactionEventID: "automatic-planning", FirstRetainedEventID: turns[retainedIndex][0].ID, Content: summary,
				}
				projection, err := composer.projectAtStart(actual, prepared, retainedIndex)
				if err != nil {
					return false, err
				}
				return projection.estimate.SerializedBytes <= prepared.usable, nil
			},
		}, true, nil
	}
	// Pressure and the preferred compaction target are not hard input limits.
	// Keep the full projected history when it fits, even if the active turn or
	// reserved summary size makes the target unreachable.
	if projection.estimate.SerializedBytes <= prepared.usable {
		return compactionPlan{}, false, nil
	}
	return compactionPlan{}, true, ErrNoLegalAutomaticCompaction
}

// planningCompactionSummary stands in for the unknown summary while planning:
// the required section headings filled to the reserved encoded size with text
// that JSON encodes byte for byte.
func planningCompactionSummary() string {
	var summary strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&summary, "## %s\nkept\n\n", heading)
	}
	if summary.Len() < automaticCompactionSummaryReserveBytes {
		summary.WriteString(strings.Repeat("x", automaticCompactionSummaryReserveBytes-summary.Len()))
	}
	return summary.String()
}

type automaticCompactionFailure struct {
	category   memory.ContextCompactionFailureCategory
	cause      causeKind
	httpStatus int
	err        error
}

func (s *Session) performAutomaticCompaction(
	coordinator *turnCoordinator,
	lease memory.TurnLease,
	plan compactionPlan,
) (*ContextSummary, memory.Event, *automaticCompactionFailure) {
	if err := s.owner.Authorize(coordinator.ctx, lease); err != nil {
		return nil, memory.Event{}, &automaticCompactionFailure{err: s.classifyLocalError(
			coordinator, fmt.Errorf("authorize automatic compactor start: %w", err),
		)}
	}
	if err := s.observeTurnContext(coordinator); err != nil {
		return nil, memory.Event{}, &automaticCompactionFailure{err: err}
	}

	callCtx, cancelCall := context.WithTimeout(coordinator.ctx, 2*time.Minute)
	response, err := s.compactor.ChatStream(callCtx, plan.Request, openrouter.StreamHandlers{})
	cancelCall()
	if err != nil {
		if cause := coordinator.result(); cause.kind != causeNone {
			return nil, memory.Event{}, &automaticCompactionFailure{err: cause.err}
		}
		if ctxErr := coordinator.ctx.Err(); ctxErr != nil {
			coordinator.selectCause(callerCause(ctxErr), ctxErr, 0)
			return nil, memory.Event{}, &automaticCompactionFailure{err: ctxErr}
		}
		var streamErr *openrouter.StreamError
		if errors.As(err, &streamErr) && streamErr.Kind == openrouter.StreamProviderResponseInvalid {
			return nil, memory.Event{}, &automaticCompactionFailure{
				category: memory.ContextCompactionSummaryInvalid,
				cause:    causeProviderInvalid,
				err:      fmt.Errorf("automatic compactor response invalid: %w", err),
			}
		}
		httpStatus := 0
		if errors.As(err, &streamErr) {
			httpStatus = streamErr.HTTPStatus
		}
		return nil, memory.Event{}, &automaticCompactionFailure{
			category:   memory.ContextCompactionSummaryProvider,
			cause:      causeProviderError,
			httpStatus: httpStatus,
			err:        fmt.Errorf("automatic compactor request failed: %w", err),
		}
	}
	if err := s.observeTurnContext(coordinator); err != nil {
		return nil, memory.Event{}, &automaticCompactionFailure{err: err}
	}
	summary, err := validatedCompactionSummary(response)
	if err == nil {
		summary, err = carryForwardCompactionSections(plan.PriorSummary, summary)
	}
	if err != nil {
		return nil, memory.Event{}, &automaticCompactionFailure{
			category: memory.ContextCompactionSummaryInvalid,
			cause:    causeProviderInvalid,
			err:      err,
		}
	}
	// Planning reserved a typical summary encoding; measure the real one
	// before it can become the active generation.
	if plan.summaryFits != nil {
		fits, err := plan.summaryFits(summary)
		if err != nil {
			return nil, memory.Event{}, &automaticCompactionFailure{err: s.classifyLocalError(
				coordinator, fmt.Errorf("measure automatic summary: %w", err),
			)}
		}
		if !fits {
			return nil, memory.Event{}, &automaticCompactionFailure{
				category: memory.ContextCompactionSummaryInvalid,
				cause:    causeProviderInvalid,
				err:      errors.New("automatic summary does not fit the usable input budget"),
			}
		}
	}

	input, err := compactionEventInput(s.profile, plan, summary, memory.ContextCompactionAutomatic)
	if err != nil {
		return nil, memory.Event{}, &automaticCompactionFailure{err: err}
	}
	if !coordinator.beginCommitBoundary() {
		return nil, memory.Event{}, &automaticCompactionFailure{err: s.observeTurnContext(coordinator)}
	}
	compacted, err := s.history.Append(coordinator.ctx, lease, input)
	if err != nil {
		coordinator.abortCommitBoundary()
		if cause := coordinator.result(); cause.kind != causeNone {
			return nil, memory.Event{}, &automaticCompactionFailure{err: cause.err}
		}
		if s.owner.IsLeaseLost(err) || coordinator.ctx.Err() != nil {
			return nil, memory.Event{}, &automaticCompactionFailure{err: s.classifyLocalError(
				coordinator, fmt.Errorf("persist automatic context compaction: %w", err),
			)}
		}
		return nil, memory.Event{}, &automaticCompactionFailure{
			category: memory.ContextCompactionSummaryPersistence,
			err:      fmt.Errorf("persist automatic context compaction: %w", err),
		}
	}
	coordinator.finishCommitBoundary(memory.StageContextCompose)
	return &ContextSummary{
		CompactionEventID: compacted.ID, FirstRetainedEventID: plan.FirstRetained.ID, Content: summary,
	}, compacted, nil
}

func completeAutomaticProjectionFits(input ContextComposeInput, composer *ContextComposer) (bool, error) {
	prepared, err := composer.prepare(input)
	if err != nil {
		return false, err
	}
	projection, err := composer.projectAtStart(input, prepared, prepared.start)
	if err != nil {
		return false, err
	}
	return projection.estimate.SerializedBytes <= prepared.usable, nil
}
