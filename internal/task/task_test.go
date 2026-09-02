package task

import (
	"errors"
	"strings"
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
		{ExpectedRevision: 1, IdempotencyKey: "request-1"},
	} {
		if err := ValidateUpdateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ValidateUpdateInput(%+v) = %v, want invalid input", input, err)
		}
	}
	title := "updated"
	if err := ValidateUpdateInput(UpdateInput{ExpectedRevision: 1, IdempotencyKey: "request-1", Title: &title}); err != nil {
		t.Fatalf("valid metadata patch rejected: %v", err)
	}
	status := StatusCompleted
	if err := ValidateUpdateInput(UpdateInput{ExpectedRevision: 1, IdempotencyKey: "request-1", Status: &status}); err != nil {
		t.Fatalf("valid lifecycle patch rejected: %v", err)
	}
}

func TestMutationInputsRequireBoundedIdempotencyIdentity(t *testing.T) {
	if err := ValidateCreateInput(CreateInput{Title: "missing identity"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing create identity error = %v", err)
	}
	if err := ValidateCreateInput(CreateInput{Title: "valid", IdempotencyKey: "request-1"}); err != nil {
		t.Fatalf("valid create rejected: %v", err)
	}
	title := "change"
	if err := ValidateUpdateInput(UpdateInput{ExpectedRevision: 1, Title: &title}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing update identity error = %v", err)
	}
	tooLong := strings.Repeat("x", 257)
	if err := ValidateIdempotencyKey(IdempotencyKey(tooLong)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("long identity error = %v", err)
	}
}

func TestMutationAttributionIsTrustedContextOnly(t *testing.T) {
	want := MutationAttribution{ActorID: "local", SessionID: "session-1", RunID: "run-1", ParentSessionID: "parent-1"}
	ctx := WithMutationAttribution(t.Context(), want)
	got, err := MutationAttributionFromContext(ctx)
	if err != nil || got != want {
		t.Fatalf("attribution = %+v, %v, want %+v", got, err, want)
	}
	if _, err := MutationAttributionFromContext(t.Context()); !errors.Is(err, ErrMissingAttribution) {
		t.Fatalf("missing attribution error = %v", err)
	}
}

func TestValidateCreateInputRequiresParentRevisionOnlyForChildren(t *testing.T) {
	validRoot := CreateInput{Title: "root", IdempotencyKey: "root"}
	if err := ValidateCreateInput(validRoot); err != nil {
		t.Fatalf("valid root: %v", err)
	}
	validChild := CreateInput{
		Title: "child", ParentID: "parent", ExpectedParentRevision: 2, IdempotencyKey: "child",
	}
	if err := ValidateCreateInput(validChild); err != nil {
		t.Fatalf("valid child: %v", err)
	}
	for _, input := range []CreateInput{
		{Title: "child", ParentID: "parent", IdempotencyKey: "missing-parent-revision"},
		{Title: "root", ExpectedParentRevision: 1, IdempotencyKey: "root-with-parent-revision"},
	} {
		if err := ValidateCreateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ValidateCreateInput(%+v) error = %v, want invalid input", input, err)
		}
	}
}

func TestValidateDecomposeInput(t *testing.T) {
	valid := DecomposeInput{
		ExpectedRevision: 3,
		Children:         []ChildInput{{Title: "research"}, {Title: "implement", Priority: 4, DueDate: "2026-09-03"}},
		IdempotencyKey:   "decompose",
	}
	if err := ValidateDecomposeInput(valid); err != nil {
		t.Fatalf("valid decomposition: %v", err)
	}
	for _, tt := range []struct {
		name  string
		input DecomposeInput
		field string
	}{
		{name: "revision", input: DecomposeInput{Children: []ChildInput{{Title: "child"}}, IdempotencyKey: "k"}, field: "expected_revision"},
		{name: "children", input: DecomposeInput{ExpectedRevision: 1, IdempotencyKey: "k"}, field: "children"},
		{name: "child", input: DecomposeInput{ExpectedRevision: 1, Children: []ChildInput{{Title: "ok"}, {Title: " "}}, IdempotencyKey: "k"}, field: "children[1].title"},
		{name: "identity", input: DecomposeInput{ExpectedRevision: 1, Children: []ChildInput{{Title: "child"}}}, field: "idempotency_key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDecomposeInput(tt.input)
			var inputErr *InputError
			if !errors.As(err, &inputErr) || inputErr.Field != tt.field {
				t.Fatalf("error = %#v, want field %q", err, tt.field)
			}
		})
	}
}

func TestValidateTreeQueryBoundsDepth(t *testing.T) {
	for _, depth := range []int{1, MaxTreeDepth} {
		if err := ValidateTreeQuery(TreeQuery{MaxDepth: depth}); err != nil {
			t.Fatalf("depth %d: %v", depth, err)
		}
	}
	for _, depth := range []int{-1, MaxTreeDepth + 1} {
		if err := ValidateTreeQuery(TreeQuery{MaxDepth: depth}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("depth %d error = %v, want invalid input", depth, err)
		}
	}
}
