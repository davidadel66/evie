package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

// extraTool builds a per-turn tool for tests, the way a frontend would:
// a closure constructed at call time, not a registry entry.
func extraTool(name string, gated bool, execute func(context.Context, string) (string, error)) Tool {
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

func TestExecuteWithApprovalCancellationMatrix(t *testing.T) {
	tests := []struct {
		name    string
		tool    func(cancel context.CancelFunc, ran *bool) Tool
		approve func(context.CancelFunc) Approver
		observe func(context.CancelFunc) ApprovalObserver
	}{
		{
			name: "after prepare prevents approval",
			tool: func(cancel context.CancelFunc, ran *bool) Tool {
				return Tool{Schema: callSchema("matrix"), NeedsApproval: true,
					Prepare: func(context.Context, string) (PreparedTool, error) {
						cancel()
						return PreparedTool{Execute: func(context.Context) (string, error) { *ran = true; return "ran", nil }}, nil
					}}
			},
			approve: func(context.CancelFunc) Approver {
				return func(context.Context, string, string, *FileChangePreview) Decision {
					t.Fatal("approval started after cancellation")
					return Approved
				}
			},
		},
		{
			name: "after approval prevents observation",
			tool: func(context.CancelFunc, *bool) Tool {
				return Tool{Schema: callSchema("matrix"), NeedsApproval: true,
					Execute: func(context.Context, string) (string, error) { return "ran", nil }}
			},
			approve: func(cancel context.CancelFunc) Approver {
				return func(context.Context, string, string, *FileChangePreview) Decision { cancel(); return Approved }
			},
			observe: func(context.CancelFunc) ApprovalObserver {
				return func(context.Context, Decision) error { t.Fatal("observer started after cancellation"); return nil }
			},
		},
		{
			name: "after observation prevents prepared execution",
			tool: func(context.CancelFunc, *bool) Tool {
				return Tool{Schema: callSchema("matrix"), NeedsApproval: true,
					Prepare: func(context.Context, string) (PreparedTool, error) {
						return PreparedTool{Execute: func(context.Context) (string, error) { t.Fatal("execution started after cancellation"); return "", nil }}, nil
					}}
			},
			approve: func(context.CancelFunc) Approver {
				return func(context.Context, string, string, *FileChangePreview) Decision { return Approved }
			},
			observe: func(cancel context.CancelFunc) ApprovalObserver {
				return func(context.Context, Decision) error { cancel(); return nil }
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ran := false
			var approve Approver
			if tc.approve != nil {
				approve = tc.approve(cancel)
			}
			var observe ApprovalObserver
			if tc.observe != nil {
				observe = tc.observe(cancel)
			}
			_, _, err := ExecuteWithApproval(ctx, []Tool{tc.tool(cancel, &ran)}, callFor("matrix", "{}"), approve, observe)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
			if ran {
				t.Fatal("tool executed after cancellation")
			}
		})
	}
}

func TestExecuteWithApprovalCancellationDuringExecuteIsLifecycleError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	extra := extraTool("blocking", false, func(ctx context.Context, _ string) (string, error) {
		cancel()
		<-ctx.Done()
		return "", ctx.Err()
	})
	msg, isErr, err := ExecuteWithApproval(ctx, []Tool{extra}, callFor("blocking", "{}"), nil, nil)
	if !errors.Is(err, context.Canceled) || isErr || msg.Role != "" {
		t.Fatalf("result = (%+v, %v, %v), want lifecycle cancellation", msg, isErr, err)
	}
}

func TestExecuteWithApprovalCanceledContextStartsNoLifecyclePhase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	extra := extraTool("never", false, func(context.Context, string) (string, error) { ran = true; return "", nil })
	_, _, err := ExecuteWithApproval(ctx, []Tool{extra}, callFor("never", "{}"), nil, nil)
	if !errors.Is(err, context.Canceled) || ran {
		t.Fatalf("error = %v, ran = %v", err, ran)
	}
}

func TestExecuteWrappersPropagateParentCancellation(t *testing.T) {
	t.Run("Execute", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		msg, err := Execute(ctx, callFor("get_time", `{}`), nil)
		if !errors.Is(err, context.Canceled) || msg.Role != "" {
			t.Fatalf("result = (%+v, %v), want lifecycle cancellation", msg, err)
		}
	})

	t.Run("ExecuteWith extra", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		extra := extraTool("blocking-wrapper", false, func(got context.Context, _ string) (string, error) {
			if got != ctx {
				t.Fatal("extra tool did not receive the caller context")
			}
			cancel()
			return "", got.Err()
		})

		msg, isErr, err := ExecuteWith(ctx, []Tool{extra}, callFor("blocking-wrapper", `{}`), nil)
		if !errors.Is(err, context.Canceled) || isErr || msg.Role != "" {
			t.Fatalf("result = (%+v, %v, %v), want lifecycle cancellation", msg, isErr, err)
		}
	})
}

func TestExecuteWithApprovalCancellationDuringPreparedExecuteIsLifecycleError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	extra := Tool{
		Schema:        callSchema("prepared-blocking"),
		NeedsApproval: true,
		Prepare: func(got context.Context, _ string) (PreparedTool, error) {
			if got != ctx {
				t.Fatal("prepare did not receive the caller context")
			}
			return PreparedTool{Execute: func(executeCtx context.Context) (string, error) {
				if executeCtx != ctx {
					t.Fatal("prepared execute did not receive the caller context")
				}
				cancel()
				return "", executeCtx.Err()
			}}, nil
		},
	}

	msg, isErr, err := ExecuteWithApproval(
		ctx,
		[]Tool{extra},
		callFor("prepared-blocking", `{}`),
		func(got context.Context, _ string, _ string, _ *FileChangePreview) Decision {
			if got != ctx {
				t.Fatal("approver did not receive the caller context")
			}
			return Approved
		},
		func(got context.Context, _ Decision) error {
			if got != ctx {
				t.Fatal("observer did not receive the caller context")
			}
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) || isErr || msg.Role != "" {
		t.Fatalf("result = (%+v, %v, %v), want lifecycle cancellation", msg, isErr, err)
	}
}

func TestExecuteWithApprovalChildDeadlineIsToolError(t *testing.T) {
	parent := context.Background()
	extra := extraTool("local-timeout", false, func(ctx context.Context, _ string) (string, error) {
		child, cancel := context.WithTimeout(ctx, time.Nanosecond)
		defer cancel()
		<-child.Done()
		return "", child.Err()
	})
	msg, isErr, err := ExecuteWithApproval(parent, []Tool{extra}, callFor("local-timeout", "{}"), nil, nil)
	if err != nil || !isErr || !strings.Contains(msg.Content, context.DeadlineExceeded.Error()) {
		t.Fatalf("result = (%+v, %v, %v), want model-visible child timeout", msg, isErr, err)
	}
}

func callSchema(name string) openrouter.Tool {
	return openrouter.Tool{Type: "function", Function: openrouter.Function{Name: name, Parameters: openrouter.Parameter{Type: "object"}}}
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

func TestToolsetCopiesSchemasAndExecutionAtConstruction(t *testing.T) {
	definitions := []Tool{{
		Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{
			Name: "pinned",
			Parameters: openrouter.Parameter{
				Type:       "object",
				Required:   []string{"value"},
				Properties: map[string]openrouter.Property{"value": {Type: "string", Enum: []string{"original"}}},
			},
		}},
		Execute: func(context.Context, string) (string, error) { return "original", nil },
	}}
	toolset := NewToolset(definitions)

	definitions[0].Schema.Function.Name = "changed"
	definitions[0].Schema.Function.Parameters.Required[0] = "changed"
	property := definitions[0].Schema.Function.Parameters.Properties["value"]
	property.Enum[0] = "changed"
	definitions[0].Schema.Function.Parameters.Properties["value"] = property
	definitions[0].Execute = func(context.Context, string) (string, error) { return "changed", nil }

	firstSchemas := toolset.Schemas()
	firstSchemas[0].Function.Name = "returned-copy-changed"
	firstSchemas[0].Function.Parameters.Required[0] = "returned-copy-changed"
	returnedProperty := firstSchemas[0].Function.Parameters.Properties["value"]
	returnedProperty.Enum[0] = "returned-copy-changed"
	firstSchemas[0].Function.Parameters.Properties["value"] = returnedProperty

	schemas := toolset.Schemas()
	if len(schemas) != 1 || schemas[0].Function.Name != "pinned" ||
		schemas[0].Function.Parameters.Required[0] != "value" ||
		schemas[0].Function.Parameters.Properties["value"].Enum[0] != "original" {
		t.Fatalf("Toolset schemas changed after construction: %+v", schemas)
	}
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(), callFor("pinned", `{}`), nil, nil, nil, nil,
	)
	if err != nil || isErr || message.Content != "original" {
		t.Fatalf("Toolset execution = (%+v, %v, %v), want original", message, isErr, err)
	}

	nilProperties := NewToolset([]Tool{{Schema: callSchema("empty")}}).Schemas()[0].Function.Parameters.Properties
	if nilProperties != nil {
		t.Fatalf("nil schema properties changed to an empty map: %+v", nilProperties)
	}
}

func TestExecuteWithDispatchesExtra(t *testing.T) {
	var gotArgs string
	extra := extraTool("show", false, func(_ context.Context, args string) (string, error) {
		gotArgs = args
		return "displayed to David", nil
	})

	msg, _, _ := ExecuteWith(context.Background(), []Tool{extra}, callFor("show", `{"kind":"svg"}`), nil)

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
	extra := extraTool("dangerous", true, func(_ context.Context, args string) (string, error) {
		ran = true
		return "should not run", nil
	})
	deny := func(_ context.Context, name, args string, _ *FileChangePreview) Decision { return Declined }

	msg, isErr, _ := ExecuteWith(context.Background(), []Tool{extra}, callFor("dangerous", "{}"), deny)

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
	msg, _, _ = ExecuteWith(context.Background(), []Tool{extra}, callFor("dangerous", "{}"), nil)
	if ran || !strings.Contains(msg.Content, "David declined") {
		t.Fatalf("nil approver did not decline, got %q", msg.Content)
	}
}

func TestExecuteWithExtraError(t *testing.T) {
	extra := extraTool("broken", false, func(_ context.Context, args string) (string, error) {
		return "", errors.New("boom")
	})

	msg, isErr, _ := ExecuteWith(context.Background(), []Tool{extra}, callFor("broken", "{}"), nil)

	if !strings.Contains(msg.Content, "boom") {
		t.Fatalf("error not surfaced to model, got %q", msg.Content)
	}
	if !isErr {
		t.Fatal("tool error not reported as failure")
	}
}

func TestExecuteWithBaseWinsOverExtra(t *testing.T) {
	extra := extraTool("get_time", false, func(_ context.Context, args string) (string, error) {
		return "shadowed", nil
	})

	msg, _, _ := ExecuteWith(context.Background(), []Tool{extra}, callFor("get_time", "{}"), nil)

	if msg.Content == "shadowed" {
		t.Fatal("extra shadowed a built-in tool; base registry must win")
	}
}

func TestExecuteWithUnknownTool(t *testing.T) {
	msg, isErr, _ := ExecuteWith(context.Background(), nil, callFor("nope", "{}"), nil)

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
	extra := extraTool("dangerous", true, func(_ context.Context, args string) (string, error) {
		ranAfterObservation = observed
		return "ran", nil
	})
	approve := func(_ context.Context, name, args string, _ *FileChangePreview) Decision { return Approved }
	observe := func(_ context.Context, decision Decision) error {
		if decision != Approved {
			t.Fatalf("observed decision = %v, want Approved", decision)
		}
		observed = true
		return nil
	}

	msg, isErr, err := ExecuteWithApproval(
		context.Background(),
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
	extra := extraTool("dangerous", true, func(_ context.Context, args string) (string, error) {
		ran = true
		return "must not run", nil
	})
	approve := func(_ context.Context, name, args string, _ *FileChangePreview) Decision { return Approved }

	_, _, err := ExecuteWithApproval(
		context.Background(),
		[]Tool{extra},
		callFor("dangerous", "{}"),
		approve,
		func(context.Context, Decision) error { return errors.New("persist approval: disk full") },
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
	extra := extraTool("dangerous", true, func(_ context.Context, args string) (string, error) {
		ran = true
		return "must not run", nil
	})
	var observed Decision
	observedDecision := false

	msg, isErr, err := ExecuteWithApproval(
		context.Background(),
		[]Tool{extra},
		callFor("dangerous", "{}"),
		func(context.Context, string, string, *FileChangePreview) Decision { return Declined },
		func(_ context.Context, decision Decision) error {
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
