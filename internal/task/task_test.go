package task

import (
	"errors"
	"testing"
)

func TestValidateStatusTransitionCompleteMatrix(t *testing.T) {
	statuses := []Status{StatusOpen, StatusInProgress, StatusBlocked, StatusCompleted, StatusCancelled}
	allowed := map[Status]map[Status]bool{
		StatusOpen:       {StatusInProgress: true, StatusBlocked: true, StatusCompleted: true, StatusCancelled: true},
		StatusInProgress: {StatusOpen: true, StatusBlocked: true, StatusCompleted: true, StatusCancelled: true},
		StatusBlocked:    {StatusOpen: true, StatusInProgress: true, StatusCompleted: true, StatusCancelled: true},
		StatusCompleted:  {StatusOpen: true},
		StatusCancelled:  {StatusOpen: true},
	}
	for _, from := range statuses {
		for _, to := range statuses {
			err := ValidateStatusTransition(from, to)
			if allowed[from][to] && err != nil {
				t.Errorf("%s -> %s rejected: %v", from, to, err)
			}
			if !allowed[from][to] {
				var transitionErr *TransitionError
				if !errors.Is(err, ErrInvalidTransition) || !errors.As(err, &transitionErr) {
					t.Errorf("%s -> %s error = %v, want typed transition error", from, to, err)
				}
			}
		}
	}
}

func TestValidateUpdateInputRequiresRevisionAndRealPatch(t *testing.T) {
	for _, input := range []UpdateInput{
		{},
		{ExpectedRevision: 1},
	} {
		if err := ValidateUpdateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ValidateUpdateInput(%+v) = %v, want invalid input", input, err)
		}
	}
	title := "updated"
	if err := ValidateUpdateInput(UpdateInput{ExpectedRevision: 1, Title: &title}); err != nil {
		t.Fatalf("valid metadata patch rejected: %v", err)
	}
	status := StatusCompleted
	if err := ValidateUpdateInput(UpdateInput{ExpectedRevision: 1, Status: &status}); err != nil {
		t.Fatalf("valid lifecycle patch rejected: %v", err)
	}
}

func TestMutationAttributionIsTrustedContextOnly(t *testing.T) {
	want := MutationAttribution{ActorID: "local", SessionID: "session-1", RunID: "run-1"}
	ctx := WithMutationAttribution(t.Context(), want)
	got, err := MutationAttributionFromContext(ctx)
	if err != nil || got != want {
		t.Fatalf("attribution = %+v, %v, want %+v", got, err, want)
	}
	if _, err := MutationAttributionFromContext(t.Context()); !errors.Is(err, ErrMissingAttribution) {
		t.Fatalf("missing attribution error = %v", err)
	}
}
