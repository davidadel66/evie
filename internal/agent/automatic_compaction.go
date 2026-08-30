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

// selectAutomaticCompaction first measures the complete projected request at
// the active summary frontier. Under pressure it selects the smallest legal
// contiguous prefix whose replacement by a maximum-sized summary leaves the
// canonical request at or below the target.
func selectAutomaticCompaction(
	input ContextComposeInput,
	composer *ContextComposer,
) (compactionPlan, bool, error) {
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
	if !automaticCompactionRequired(projection.estimate.SerializedBytes, profile.WorkingTokens) {
		return compactionPlan{}, false, nil
	}

	activeSummary, chain, err := reconstructCompactionChain(input.Events)
	if err != nil {
		return compactionPlan{}, true, err
	}
	compactionTurns, err := compactionRootTurns(input.Events)
	if err != nil {
		return compactionPlan{}, true, err
	}
	if len(compactionTurns) != len(turns) {
		return compactionPlan{}, true, errors.New("automatic compaction turn projection is inconsistent")
	}
	compactorUsable, err := compactionUsableInputBytes(profile)
	if err != nil {
		return compactionPlan{}, true, ErrNoLegalAutomaticCompaction
	}
	target := percentageFloor(profile.WorkingTokens, automaticCompactionTargetPercent)
	planningSummary := maximumCanonicalCompactionSummary()
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
			Request: request, Generation: generation, PriorCompactionEventID: priorID,
			CoveredFirst:  covered[0].events[0],
			CoveredLast:   covered[len(covered)-1].events[len(covered[len(covered)-1].events)-1],
			FirstRetained: compactionTurns[retainedIndex].events[0],
		}, true, nil
	}
	return compactionPlan{}, true, ErrNoLegalAutomaticCompaction
}

// maximumCanonicalCompactionSummary realizes the largest canonical JSON
// encoding permitted by the summary byte limit. encoding/json expands '<' to
// six bytes, so this bounds every accepted summary rather than only typical
// prose containing no escaped characters.
func maximumCanonicalCompactionSummary() string {
	var summary strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&summary, "## %s\nkept\n\n", heading)
	}
	if summary.Len() < CompactionSummaryMaxBytes {
		summary.WriteString(strings.Repeat("<", CompactionSummaryMaxBytes-summary.Len()))
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
	if err != nil {
		return nil, memory.Event{}, &automaticCompactionFailure{
			category: memory.ContextCompactionSummaryInvalid,
			cause:    causeProviderInvalid,
			err:      err,
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
