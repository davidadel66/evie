package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/openrouter"
)

// workspace-mcp-connections.spec.md: a tool may decide per call whether it
// needs the owner's approval; any error from that decision asks.
func TestRequiresApprovalDecidesPerCallAndFallsBackToAsking(t *testing.T) {
	cases := []struct {
		name     string
		policy   func(context.Context, string) (bool, error)
		decision Decision
		asked    bool
		ran      bool
	}{
		{"allowed", func(context.Context, string) (bool, error) { return false, nil }, Declined, false, true},
		{"asked and approved", func(context.Context, string) (bool, error) { return true, nil }, Approved, true, true},
		{"asked and declined", func(context.Context, string) (bool, error) { return true, nil }, Declined, true, false},
		{"policy error asks", func(context.Context, string) (bool, error) { return false, errors.New("policy unreadable") }, Declined, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var asked, ran bool
			var seenArgs string
			tool := Tool{
				Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "mcp_call", Parameters: openrouter.Parameter{Type: "object"}}},
				Execute: func(context.Context, string) (string, error) {
					ran = true
					return "ok", nil
				},
				RequiresApproval: func(ctx context.Context, args string) (bool, error) {
					seenArgs = args
					return c.policy(ctx, args)
				},
			}
			approve := func(context.Context, string, string, *FileChangePreview) Decision {
				asked = true
				return c.decision
			}
			call := openrouter.ToolCall{ID: "call-1", Function: openrouter.FunctionCall{Name: "mcp_call", Arguments: `{"tool":"x"}`}}
			msg, _, err := NewToolset([]Tool{tool}).ExecuteWithApprovalAuthorizedCompletion(context.Background(), call, approve, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if asked != c.asked || ran != c.ran || seenArgs != `{"tool":"x"}` {
				t.Fatalf("asked=%v ran=%v args=%q, result %q", asked, ran, seenArgs, msg.Content)
			}
		})
	}
}

// The approval decision may reach a server, so a turn that can no longer
// prepare tools never runs it.
func TestRequiresApprovalRunsOnlyAfterPreparationIsAuthorized(t *testing.T) {
	var decided bool
	tool := Tool{
		Schema:  openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "mcp_call", Parameters: openrouter.Parameter{Type: "object"}}},
		Execute: func(context.Context, string) (string, error) { return "ok", nil },
		RequiresApproval: func(context.Context, string) (bool, error) {
			decided = true
			return false, nil
		},
	}
	var boundaries []AuthorizationBoundary
	refuse := func(_ context.Context, boundary AuthorizationBoundary) error {
		boundaries = append(boundaries, boundary)
		return errors.New("turn interrupted")
	}
	call := openrouter.ToolCall{ID: "call-1", Function: openrouter.FunctionCall{Name: "mcp_call", Arguments: `{}`}}
	if _, _, err := NewToolset([]Tool{tool}).ExecuteWithApprovalAuthorizedCompletion(context.Background(), call, nil, nil, refuse, nil); err == nil {
		t.Fatal("an unauthorized turn ran the tool")
	}
	if decided || len(boundaries) != 1 || boundaries[0] != AuthorizePreparation {
		t.Fatalf("decided=%v boundaries=%v", decided, boundaries)
	}
	// Allowed: preparation, then execution, in order.
	boundaries = nil
	allow := func(_ context.Context, boundary AuthorizationBoundary) error {
		boundaries = append(boundaries, boundary)
		return nil
	}
	if _, _, err := NewToolset([]Tool{tool}).ExecuteWithApprovalAuthorizedCompletion(context.Background(), call, nil, nil, allow, nil); err != nil {
		t.Fatal(err)
	}
	if !decided || len(boundaries) != 2 || boundaries[0] != AuthorizePreparation || boundaries[1] != AuthorizeExecution {
		t.Fatalf("decided=%v boundaries=%v", decided, boundaries)
	}
}
