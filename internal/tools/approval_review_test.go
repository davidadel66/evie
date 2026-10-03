package tools

import (
	"context"
	"errors"
	"testing"
)

// A prepared effect routed to the owner's review inbox asks for no approval,
// records no approval decision and never executes; the model receives the
// queueing result. Queueing is still an authorized tool start.
func TestPreparedReviewQueuesWithoutApproval(t *testing.T) {
	var executed, queued bool
	var boundaries []AuthorizationBoundary
	tool := Tool{Schema: callSchema("semantic"), NeedsApproval: true, Prepare: func(context.Context, string) (PreparedTool, error) {
		return PreparedTool{
			Approval: ApprovalMetadata{Arguments: `{"proposal":1}`, ParentEventID: "owner-event", ExecutionID: "60000000-0000-4000-8000-000000000002",
				ProposalSHA256: "sha256:p", PreparedSHA256: "sha256:q"},
			Execute: func(context.Context) (string, error) { executed = true; return "applied", nil },
			Review:  func(context.Context) (string, error) { queued = true; return "queued for review", nil },
		}, nil
	}}
	message, isErr, err := ExecuteWithApprovalAuthorized(context.Background(), []Tool{tool}, callFor("semantic", `{}`),
		func(context.Context, string, string, *FileChangePreview) Decision {
			t.Fatal("a queued proposal asked for approval")
			return Declined
		},
		func(context.Context, Decision, ApprovalMetadata) error {
			t.Fatal("a queued proposal recorded an approval decision")
			return nil
		},
		func(_ context.Context, boundary AuthorizationBoundary) error {
			boundaries = append(boundaries, boundary)
			return nil
		})
	if err != nil || isErr || message.Content != "queued for review" || !queued || executed {
		t.Fatalf("review = (%+v, %v, %v) queued=%v executed=%v", message, isErr, err, queued, executed)
	}
	if len(boundaries) != 2 || boundaries[0] != AuthorizePreparation || boundaries[1] != AuthorizeExecution {
		t.Fatalf("authorization boundaries = %v", boundaries)
	}

	tool.Prepare = func(context.Context, string) (PreparedTool, error) {
		return PreparedTool{Review: func(context.Context) (string, error) { return "", errors.New("inbox unavailable") }}, nil
	}
	message, isErr, err = ExecuteWithApprovalAuthorized(context.Background(), []Tool{tool}, callFor("semantic", `{}`),
		func(context.Context, string, string, *FileChangePreview) Decision { return Approved }, nil,
		func(_ context.Context, boundary AuthorizationBoundary) error {
			if boundary == AuthorizeExecution {
				return errors.New("lease lost")
			}
			return nil
		})
	if err == nil {
		t.Fatalf("queueing ran after its execution authorization failed: %+v %v", message, isErr)
	}
	message, isErr, err = ExecuteWithApprovalAuthorized(context.Background(), []Tool{tool}, callFor("semantic", `{}`),
		func(context.Context, string, string, *FileChangePreview) Decision { return Approved }, nil, nil)
	if err != nil || !isErr || message.Content != "tool call came back with error inbox unavailable" {
		t.Fatalf("failed queueing = (%+v, %v, %v)", message, isErr, err)
	}
}
