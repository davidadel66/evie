package tools

import (
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/openrouter"
)

// extraTool builds a per-turn tool for tests, the way a frontend would:
// a closure constructed at call time, not a registry entry.
func extraTool(name string, gated bool, execute func(args string) (string, error)) Tool {
	return Tool{
		Schema: openrouter.Tool{
			Type: "function",
			Function: openrouter.Function{
				Name:        name,
				Description: "test-only extra tool",
				Parameters:  openrouter.Parameter{Type: "object"},
			},
		},
		Execute:       execute,
		NeedsApproval: gated,
	}
}

func callFor(name, args string) openrouter.ToolCall {
	return openrouter.ToolCall{
		ID:   "call-1",
		Type: "function",
		Function: openrouter.FunctionCall{
			Name:      name,
			Arguments: args,
		},
	}
}

func TestSchemasWithAppendsExtras(t *testing.T) {
	base := Schemas()
	extra := extraTool("show", false, nil)

	got := SchemasWith([]Tool{extra})

	if len(got) != len(base)+1 {
		t.Fatalf("SchemasWith returned %d schemas, want %d", len(got), len(base)+1)
	}
	if got[len(got)-1].Function.Name != "show" {
		t.Fatalf("last schema is %q, want the extra appended at the end", got[len(got)-1].Function.Name)
	}
}

func TestSchemasMatchesSchemasWithNil(t *testing.T) {
	if len(Schemas()) != len(SchemasWith(nil)) {
		t.Fatal("Schemas and SchemasWith(nil) disagree")
	}
}

func TestExecuteWithDispatchesExtra(t *testing.T) {
	var gotArgs string
	extra := extraTool("show", false, func(args string) (string, error) {
		gotArgs = args
		return "displayed to David", nil
	})

	msg, _ := ExecuteWith([]Tool{extra}, callFor("show", `{"kind":"svg"}`), nil)

	if gotArgs != `{"kind":"svg"}` {
		t.Fatalf("extra received args %q", gotArgs)
	}
	if msg.Role != "tool" || msg.ToolCallID != "call-1" {
		t.Fatalf("wrapping wrong: role=%q id=%q", msg.Role, msg.ToolCallID)
	}
	if msg.Content != "displayed to David" {
		t.Fatalf("content = %q", msg.Content)
	}
}

func TestExecuteWithGatedExtraDeclined(t *testing.T) {
	ran := false
	extra := extraTool("dangerous", true, func(args string) (string, error) {
		ran = true
		return "should not run", nil
	})
	deny := func(name, args string, _ *FileChangePreview) Decision { return Declined }

	msg, isErr := ExecuteWith([]Tool{extra}, callFor("dangerous", "{}"), deny)

	if isErr {
		t.Fatal("a decline is the gate working, not a failure")
	}
	if ran {
		t.Fatal("gated extra executed despite decline")
	}
	if !strings.Contains(msg.Content, "David declined") {
		t.Fatalf("decline text missing, got %q", msg.Content)
	}

	// nil approver must fail closed too.
	msg, _ = ExecuteWith([]Tool{extra}, callFor("dangerous", "{}"), nil)
	if ran || !strings.Contains(msg.Content, "David declined") {
		t.Fatalf("nil approver did not decline, got %q", msg.Content)
	}
}

func TestExecuteWithExtraError(t *testing.T) {
	extra := extraTool("broken", false, func(args string) (string, error) {
		return "", errors.New("boom")
	})

	msg, isErr := ExecuteWith([]Tool{extra}, callFor("broken", "{}"), nil)

	if !strings.Contains(msg.Content, "boom") {
		t.Fatalf("error not surfaced to model, got %q", msg.Content)
	}
	if !isErr {
		t.Fatal("tool error not reported as failure")
	}
}

func TestExecuteWithBaseWinsOverExtra(t *testing.T) {
	extra := extraTool("get_time", false, func(args string) (string, error) {
		return "shadowed", nil
	})

	msg, _ := ExecuteWith([]Tool{extra}, callFor("get_time", "{}"), nil)

	if msg.Content == "shadowed" {
		t.Fatal("extra shadowed a built-in tool; base registry must win")
	}
}

func TestExecuteWithUnknownTool(t *testing.T) {
	msg, isErr := ExecuteWith(nil, callFor("nope", "{}"), nil)

	if !isErr {
		t.Fatal("unknown tool not reported as failure")
	}
	if !strings.Contains(msg.Content, "Unknown Tool Call: nope") {
		t.Fatalf("unknown-tool text changed, got %q", msg.Content)
	}
	if msg.ToolCallID != "call-1" {
		t.Fatalf("unknown-tool reply lost the call id, got %q", msg.ToolCallID)
	}
}

func TestExecuteWithApprovalObservesDecisionBeforeExecution(t *testing.T) {
	observed := false
	ranAfterObservation := false
	extra := extraTool("dangerous", true, func(args string) (string, error) {
		ranAfterObservation = observed
		return "ran", nil
	})
	approve := func(name, args string, _ *FileChangePreview) Decision { return Approved }
	observe := func(decision Decision) error {
		if decision != Approved {
			t.Fatalf("observed decision = %v, want Approved", decision)
		}
		observed = true
		return nil
	}

	msg, isErr, err := ExecuteWithApproval(
		[]Tool{extra}, callFor("dangerous", "{}"), approve, observe,
	)
	if err != nil {
		t.Fatalf("ExecuteWithApproval: %v", err)
	}
	if isErr || msg.Content != "ran" || !ranAfterObservation {
		t.Fatalf("result = (%+v, isErr=%v), ran after observation=%v", msg, isErr, ranAfterObservation)
	}
}

func TestExecuteWithApprovalObserverFailurePreventsExecution(t *testing.T) {
	ran := false
	extra := extraTool("dangerous", true, func(args string) (string, error) {
		ran = true
		return "must not run", nil
	})
	approve := func(name, args string, _ *FileChangePreview) Decision { return Approved }

	_, _, err := ExecuteWithApproval(
		[]Tool{extra},
		callFor("dangerous", "{}"),
		approve,
		func(Decision) error { return errors.New("persist approval: disk full") },
	)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("observer error = %v, want disk full", err)
	}
	if ran {
		t.Fatal("tool ran after approval observer failed")
	}
}

func TestExecuteWithApprovalObservesDecline(t *testing.T) {
	ran := false
	extra := extraTool("dangerous", true, func(args string) (string, error) {
		ran = true
		return "must not run", nil
	})
	var observed Decision
	observedDecision := false

	msg, isErr, err := ExecuteWithApproval(
		[]Tool{extra},
		callFor("dangerous", "{}"),
		func(name, args string, _ *FileChangePreview) Decision { return Declined },
		func(decision Decision) error {
			observed = decision
			observedDecision = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ExecuteWithApproval: %v", err)
	}
	if !observedDecision || observed != Declined || ran || isErr ||
		!strings.Contains(msg.Content, "David declined") {
		t.Fatalf("decision observed=%v/%v, ran=%v, result=(%+v, %v)",
			observedDecision, observed, ran, msg, isErr)
	}
}
