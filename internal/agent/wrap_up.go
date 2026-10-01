package agent

import (
	"crypto/sha256"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
)

// WrapUpState is what a delegated worker's supervisor sees before each model
// response of the turn.
type WrapUpState struct {
	Step, StepLimit int
	// RequestBytes is the next request's canonical size after ordinary
	// tool-result projection and before any final-step notice; UsableBytes is
	// its context budget. A one-turn worker has no closed turns to compact,
	// so this is its remaining context.
	RequestBytes, UsableBytes int64
}

// WrapUpSignal lets a delegated worker's supervisor end a turn early through
// the step limit's final tool-free response. When it reports wrapUp, that
// response is requested with tool_choice "none" and the returned notice as its
// trailing harness message; at the step limit its notice also replaces the
// default one. Tool results are projected further when needed so that this
// final request fits the context budget. A wrap-up response that still asks
// for tools ends the turn exactly like the step limit.
type WrapUpSignal func(WrapUpState) (notice string, wrapUp bool)

// WithWrapUp installs a wrap-up signal on a session.
func WithWrapUp(signal WrapUpSignal) SessionOption {
	return func(session *Session) { session.wrapUp = signal }
}

// finalStepNotice reports whether this response is the turn's last and the
// harness notice that asks for the answer. The context measurement is made
// only for sessions with a wrap-up signal.
func (s *Session) finalStepNotice(step int, input ContextComposeInput) (string, bool, error) {
	if s.wrapUp != nil {
		requestBytes, usable, err := s.projectedRequestBytes(input)
		if err != nil {
			return "", false, err
		}
		state := WrapUpState{Step: step, StepLimit: s.stepLimit, RequestBytes: requestBytes, UsableBytes: usable}
		if notice, wrapUp := s.wrapUp(state); wrapUp && notice != "" {
			return notice, true, nil
		}
	}
	if step >= s.stepLimit {
		return stepLimitNote(s.stepLimit), true, nil
	}
	return "", false, nil
}

// projectedRequestBytes measures the request the composer would build from
// the retained frontier, with its ordinary tool-result projection.
func (s *Session) projectedRequestBytes(input ContextComposeInput) (int64, int64, error) {
	prepared, err := s.composer.prepare(input)
	if err != nil {
		return 0, 0, err
	}
	projection, err := s.composer.projectAtStart(input, prepared, prepared.start)
	if err != nil {
		return 0, 0, err
	}
	return projection.estimate.SerializedBytes, prepared.usable, nil
}

// fitWrapUpRequest makes a worker's final tool-free request fit its context
// budget. Ordinary projection keeps the newest tool-result groups whole; the
// wrap-up needs only enough to write the report, so it first reduces every
// large tool result to its head and tail excerpt, and then, if that is not
// enough, every tool result to a one-line marker. Both forms name the event
// and its original size and hash. It returns the input unchanged when it
// already fits or when nothing fits, in which case composition reports the
// overflow. reduced reports whether events were replaced.
func (s *Session) fitWrapUpRequest(input ContextComposeInput) (ContextComposeInput, bool, error) {
	fits := func(candidate ContextComposeInput) (bool, error) {
		requestBytes, usable, err := s.projectedRequestBytes(candidate)
		return requestBytes <= usable, err
	}
	if ok, err := fits(input); ok || err != nil {
		return input, false, err
	}
	for _, reduce := range []func(memory.Event) string{wrapUpExcerpt, wrapUpMarker} {
		candidate := input
		candidate.Events = reduceToolResults(input.Events, reduce)
		ok, err := fits(candidate)
		if err != nil {
			return input, false, err
		}
		if ok {
			return candidate, true, nil
		}
	}
	return input, false, nil
}

func isToolResult(event memory.Event) bool {
	return event.Type == memory.EventToolSucceeded || event.Type == memory.EventToolFailed || event.Type == memory.EventToolCancelled
}

// reduceToolResults copies events, replacing each tool result's content with
// reduce's when that is shorter.
func reduceToolResults(events []memory.Event, reduce func(memory.Event) string) []memory.Event {
	reduced := append([]memory.Event(nil), events...)
	for i, event := range reduced {
		if !isToolResult(event) {
			continue
		}
		if content := reduce(event); len(content) < len(event.Content) {
			reduced[i].Content = content
		}
	}
	return reduced
}

// wrapUpExcerpt is the ordinary older-result projection, applied to every
// large result. Its output stays below the projection threshold, so the
// composer never projects it again.
func wrapUpExcerpt(event memory.Event) string {
	if !isPressureProjectableToolResult(event) {
		return event.Content
	}
	return projectOldToolResult(event)
}

func wrapUpMarker(event memory.Event) string {
	return fmt.Sprintf("[tool result omitted to fit the final report: event_id=%s original_bytes=%d sha256=%x]",
		event.ID, len(event.Content), sha256.Sum256([]byte(event.Content)))
}

// wrapUpPlaceholders describes, against durable content, every retained tool
// result the wrap-up reduced. The composer saw only reduced content, which is
// below its projection threshold, so these are all of the request's
// placeholders.
func wrapUpPlaceholders(original, reduced []memory.Event, snapshot memory.ContextSnapshotPayload) []memory.ContextPlaceholderManifest {
	var retainedOriginal, retainedReduced []memory.Event
	for i := range original {
		if original[i].Sequence >= snapshot.RetainedFirstSequence && original[i].Sequence <= snapshot.RetainedLastSequence {
			retainedOriginal = append(retainedOriginal, original[i])
			retainedReduced = append(retainedReduced, reduced[i])
		}
	}
	return toolResultPlaceholderManifests(retainedOriginal, retainedReduced)
}
