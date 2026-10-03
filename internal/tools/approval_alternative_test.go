package tools

import (
	"context"
	"testing"
)

func alternativeTool(executed *string) Tool {
	metadata := func(name string) ApprovalMetadata {
		return ApprovalMetadata{Arguments: `{"standing":` + name + `}`, ParentEventID: "owner-event-1",
			ExecutionID: "60000000-0000-4000-8000-000000000001", ProposalSHA256: "sha256:" + name, PreparedSHA256: "sha256:prepared-" + name}
	}
	return Tool{
		Schema: callSchema("semantic"), NeedsApproval: true,
		Prepare: func(context.Context, string) (PreparedTool, error) {
			return PreparedTool{
				Approval: metadata("true"),
				Execute:  func(context.Context) (string, error) { *executed = "default"; return "default", nil },
				Alternative: &PreparedAlternative{
					Approval: metadata("false"),
					Execute:  func(context.Context) (string, error) { *executed = "alternative"; return "alternative", nil },
				},
			}, nil
		},
	}
}

func TestApprovedAlternativeRecordsAndRunsThePreparedAlternative(t *testing.T) {
	var executed string
	var observed ApprovalMetadata
	var observedDecision Decision
	message, isErr, err := ExecuteWithApproval(context.Background(), []Tool{alternativeTool(&executed)}, callFor("semantic", `{}`),
		func(_ context.Context, _ string, args string, _ *FileChangePreview) Decision {
			if args != `{"standing":true}` {
				t.Fatalf("approval card arguments = %s", args)
			}
			return ApprovedAlternative
		},
		func(_ context.Context, decision Decision, metadata ApprovalMetadata) error {
			observedDecision, observed = decision, metadata
			return nil
		})
	if err != nil || isErr || message.Content != "alternative" || executed != "alternative" {
		t.Fatalf("alternative execution = (%+v, %v, %v), executed=%q", message, isErr, err, executed)
	}
	if observedDecision != Approved || observed.ProposalSHA256 != "sha256:false" || observed.Arguments != `{"standing":false}` {
		t.Fatalf("durable approval = %v %+v", observedDecision, observed)
	}

	executed = ""
	if _, _, err := ExecuteWithApproval(context.Background(), []Tool{alternativeTool(&executed)}, callFor("semantic", `{}`),
		func(context.Context, string, string, *FileChangePreview) Decision { return Approved },
		func(_ context.Context, _ Decision, metadata ApprovalMetadata) error { observed = metadata; return nil }); err != nil || executed != "default" || observed.ProposalSHA256 != "sha256:true" {
		t.Fatalf("plain approval ran %q with %+v (%v)", executed, observed, err)
	}
}

func TestApprovedAlternativeWithoutOneIsDeclined(t *testing.T) {
	executed := false
	tool := Tool{Schema: callSchema("plain"), NeedsApproval: true, Execute: func(context.Context, string) (string, error) {
		executed = true
		return "ran", nil
	}}
	var observed Decision = Approved
	message, _, err := ExecuteWithApproval(context.Background(), []Tool{tool}, callFor("plain", `{}`),
		func(context.Context, string, string, *FileChangePreview) Decision { return ApprovedAlternative },
		func(_ context.Context, decision Decision, _ ApprovalMetadata) error { observed = decision; return nil })
	if err != nil || executed || observed != Declined || message.Content == "ran" {
		t.Fatalf("alternative without one = %+v executed=%v observed=%v err=%v", message, executed, observed, err)
	}
}
