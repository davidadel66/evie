package web_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

func TestCandidateAdvancedHTTPInputRefusalIsDefinitiveWithoutPersistence(t *testing.T) {
	for _, route := range []string{"resolve", "batch/resolve", "edit"} {
		for _, reason := range []struct{ name, value string }{
			{"secret", "password=synthetic-fixture-value"},
			{"oversized", strings.Repeat("x", 4097)},
		} {
			t.Run(route+"/"+reason.name, func(t *testing.T) {
				f := newWebReviewFixture(t)
				single := webPrepare(t, f, "reject")
				batch := reviewResponse[memory.ReviewBatchPreview](t, advancedPost(t, f, "batch/prepare", memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{{ID: "group", Action: "reject", Candidates: []memory.CandidateRef{f.candidate.Ref}}}}), 200)
				decision := webDecision(single)
				decision.Reason = reason.value
				batchDecision := memory.ReviewBatchDecision{DeliveryKey: decision.DeliveryKey, PreviewID: batch.ID, PreviewSHA256: batch.SHA256, Actions: []memory.ReviewBatchAction{{GroupID: "group", Action: "reject"}}, Reason: reason.value}
				edit := memory.ReviewEditDecision{Candidate: f.candidate.Ref, Proposal: f.candidate.Candidate.Proposal, Reason: reason.value}
				switch route {
				case "resolve":
					advancedError(t, reviewPost(t, f.handler, route, map[string]any{"scope_key": "global", "decision": decision}), 400, "invalid_review_request")
				case "batch/resolve":
					advancedError(t, advancedPost(t, f, route, batchDecision), 400, "invalid_review_request")
				case "edit":
					advancedError(t, advancedPost(t, f, route, edit), 400, "invalid_review_request")
				}
				var deliveries, edits, resolutions, operations int
				err := f.db.QueryRow(`SELECT
      (SELECT count(*) FROM memory_review_deliveries)+(SELECT count(*) FROM memory_review_batch_deliveries),
      (SELECT count(*) FROM memory_review_edit_revisions),
      (SELECT count(*) FROM memory_review_resolutions),
      (SELECT count(*) FROM semantic_operations WHERE operation_kind='owner_candidate_review')`).Scan(&deliveries, &edits, &resolutions, &operations)
				if err != nil || deliveries+edits+resolutions+operations != 0 {
					t.Fatalf("refused request changed durable state: %d/%d/%d/%d %v", deliveries, edits, resolutions, operations, err)
				}
				// The definitive refusal leaves the same preview/ref available for a new
				// explicit decision with a corrected reason; no unknown delivery is stored.
				switch route {
				case "resolve":
					decision.Reason = "Reviewed"
					reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, route, map[string]any{"scope_key": "global", "decision": decision}), 200)
				case "batch/resolve":
					batchDecision.Reason = "Reviewed"
					reviewResponse[memory.ReviewBatchResult](t, advancedPost(t, f, route, batchDecision), 200)
				case "edit":
					edit.Reason = "Reviewed"
					reviewResponse[memory.OwnerCandidate](t, advancedPost(t, f, route, edit), 200)
				}
			})
		}
	}
}

func TestCandidateAdvancedHTTPHistoryKindAndMissingRevision(t *testing.T) {
	f := newWebReviewFixture(t)
	edit := reviewResponse[memory.OwnerCandidate](t, advancedPost(t, f, "edit", memory.ReviewEditDecision{Candidate: f.candidate.Ref, Proposal: f.candidate.Candidate.Proposal, Reason: "Review history fixture"}), 200)
	for _, route := range []string{"edit/revision", "identity/revision", "temporal/revision"} {
		t.Run(route, func(t *testing.T) {
			input := map[string]any{"id": edit.Ref.ID, "revision": edit.Ref.InterpretationRevision}
			if route == "edit/revision" {
				recorded := reviewResponse[memory.ReviewEditRevision](t, advancedPost(t, f, route, input), 200)
				if recorded.Revision != edit.Ref.InterpretationRevision || recorded.CandidateID != edit.Ref.ID {
					t.Fatal("history returned a different binding")
				}
			} else {
				advancedError(t, advancedPost(t, f, route, input), 404, "review_revision_not_found")
			}
			input["revision"] = edit.Ref.InterpretationRevision + 1
			advancedError(t, advancedPost(t, f, route, input), 404, "review_revision_not_found")
			input["revision"] = 0
			advancedError(t, advancedPost(t, f, route, input), 400, "invalid_history_request")
			input["revision"] = 1
			input["id"] = ""
			advancedError(t, advancedPost(t, f, route, input), 400, "invalid_history_request")
			// Even a missing revision cannot reveal a candidate outside its scope.
			input["id"] = edit.Ref.ID
			advancedError(t, reviewPost(t, f.handler, route, map[string]any{"scope_key": "session:unavailable", "input": input}), 403, "review_unauthorized")
		})
	}
}

type historyReadFailureKernel struct {
	*eviedb.Store
	err error
}

func (k historyReadFailureKernel) InspectOwnerCandidateEditRevision(context.Context, eviedb.OwnerReviewContext, string, int64) (memory.ReviewEditRevision, error) {
	return memory.ReviewEditRevision{}, k.err
}
func (k historyReadFailureKernel) InspectOwnerCandidateIdentityRevision(context.Context, eviedb.OwnerReviewContext, string, int64) (memory.ReviewIdentityRevision, error) {
	return memory.ReviewIdentityRevision{}, k.err
}
func (k historyReadFailureKernel) InspectOwnerCandidateTemporalRevision(context.Context, eviedb.OwnerReviewContext, string, int64) (memory.ReviewTemporalRevision, error) {
	return memory.ReviewTemporalRevision{}, k.err
}

func TestCandidateAdvancedHTTPHistoryReadErrorsRetainTheirMeaning(t *testing.T) {
	f := newWebReviewFixture(t)
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"proven missing row", fmt.Errorf("history lookup: %w", sql.ErrNoRows), 404, "review_revision_not_found"},
		{"untyped similar text", errors.New(sql.ErrNoRows.Error()), 503, "review_retryable"},
		{"database read", errors.New("synthetic database failure"), 503, "review_retryable"},
		{"cancellation", context.Canceled, 503, "review_retryable"},
		{"current source", eviedb.ErrReviewInvalidSource, 409, "source_ineligible"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f.handler = web.WithCandidateReview(web.NewServer(nil), historyReadFailureKernel{Store: f.store, err: test.err}).Handler()
			for _, route := range []string{"edit/revision", "identity/revision", "temporal/revision"} {
				advancedError(t, advancedPost(t, f, route, map[string]any{"id": f.candidate.Ref.ID, "revision": 1}), test.status, test.code)
			}
		})
	}
}
