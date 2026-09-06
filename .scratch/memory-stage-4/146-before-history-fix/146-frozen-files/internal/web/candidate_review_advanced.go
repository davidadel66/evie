package web

import (
	"context"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// AdvancedCandidateReviewKernel owns all interpretation and atomic-group rules.
// The HTTP adapter only authenticates one exact scope and decodes bounded input.
type AdvancedCandidateReviewKernel interface {
	OwnerCandidateIdentityOptions(context.Context, eviedb.OwnerReviewContext, memory.CandidateRef) (memory.ReviewIdentityOptions, error)
	ChooseOwnerCandidateIdentity(context.Context, eviedb.OwnerReviewContext, memory.ReviewIdentityDecision) (memory.OwnerCandidate, error)
	OwnerCandidateTemporalOptions(context.Context, eviedb.OwnerReviewContext, memory.CandidateRef) (memory.ReviewTemporalOptions, error)
	ChooseOwnerCandidateTemporal(context.Context, eviedb.OwnerReviewContext, memory.ReviewTemporalDecision) (memory.OwnerCandidate, error)
	EditOwnerCandidate(context.Context, eviedb.OwnerReviewContext, memory.ReviewEditDecision) (memory.OwnerCandidate, error)
	InspectOwnerCandidateEditRevision(context.Context, eviedb.OwnerReviewContext, string, int64) (memory.ReviewEditRevision, error)
	InspectOwnerCandidateIdentityRevision(context.Context, eviedb.OwnerReviewContext, string, int64) (memory.ReviewIdentityRevision, error)
	InspectOwnerCandidateTemporalRevision(context.Context, eviedb.OwnerReviewContext, string, int64) (memory.ReviewTemporalRevision, error)
	PrepareOwnerCandidateBatch(context.Context, eviedb.OwnerReviewContext, memory.ReviewBatchRequest) (memory.ReviewBatchPreview, error)
	InspectOwnerCandidateBatch(context.Context, eviedb.OwnerReviewContext, string) (memory.ReviewBatchPreview, error)
	ResolveOwnerCandidateBatch(context.Context, eviedb.OwnerReviewContext, memory.ReviewBatchDecision) (memory.ReviewBatchResult, error)
}

type candidateRevisionRequest struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

// A complete edited proposal can approach 256 KiB. Reserve another 8 KiB for
// its request envelope/reason; the Kernel applies its canonical disclosure bound.
// Batch input has at most 64 references and 20 groups; 64 KiB admits its full
// closure. Responses are never truncated to either request-body limit.
func (s *Server) registerAdvancedCandidateReviewRoutes(mux *http.ServeMux) {
	k, ok := s.candidateReview.(AdvancedCandidateReviewKernel)
	if !ok {
		return
	}
	for path, handler := range map[string]http.HandlerFunc{
		"identity/options": advancedReviewRoute(s, 8192, k.OwnerCandidateIdentityOptions),
		"identity/choose":  advancedReviewRoute(s, 8192, k.ChooseOwnerCandidateIdentity),
		"temporal/options": advancedReviewRoute(s, 8192, k.OwnerCandidateTemporalOptions),
		"temporal/choose":  advancedReviewRoute(s, 8192, k.ChooseOwnerCandidateTemporal),
		"edit":             advancedReviewRoute(s, 264*1024, k.EditOwnerCandidate),
		"edit/revision": advancedReviewRoute(s, 8192, func(ctx context.Context, a eviedb.OwnerReviewContext, r candidateRevisionRequest) (memory.ReviewEditRevision, error) {
			return k.InspectOwnerCandidateEditRevision(ctx, a, r.ID, r.Revision)
		}),
		"identity/revision": advancedReviewRoute(s, 8192, func(ctx context.Context, a eviedb.OwnerReviewContext, r candidateRevisionRequest) (memory.ReviewIdentityRevision, error) {
			return k.InspectOwnerCandidateIdentityRevision(ctx, a, r.ID, r.Revision)
		}),
		"temporal/revision": advancedReviewRoute(s, 8192, func(ctx context.Context, a eviedb.OwnerReviewContext, r candidateRevisionRequest) (memory.ReviewTemporalRevision, error) {
			return k.InspectOwnerCandidateTemporalRevision(ctx, a, r.ID, r.Revision)
		}),
		"batch/prepare": advancedReviewRoute(s, 64*1024, k.PrepareOwnerCandidateBatch),
		"batch/inspect": advancedReviewRoute(s, 8192, func(ctx context.Context, a eviedb.OwnerReviewContext, r candidateRevisionRequest) (memory.ReviewBatchPreview, error) {
			return k.InspectOwnerCandidateBatch(ctx, a, r.ID)
		}),
		"batch/resolve": advancedReviewRoute(s, 32*1024, k.ResolveOwnerCandidateBatch),
	} {
		mux.Handle("/api/memory/candidates/"+path, s.managementRoute(handler))
	}
}

func advancedReviewRoute[Request, Response any](s *Server, limit int64, call func(context.Context, eviedb.OwnerReviewContext, Request) (Response, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Scope string   `json:"scope_key"`
			Input *Request `json:"input"`
		}
		if !decodeCandidateRequestLimit(w, r, &request, limit) {
			return
		}
		if request.Input == nil {
			managementJSONError(w, http.StatusBadRequest, "invalid_review_request", "provide the complete typed review input")
			return
		}
		a, ok := s.candidateAuthority(w, r, request.Scope)
		if !ok {
			return
		}
		out, err := call(r.Context(), a, *request.Input)
		if err != nil {
			writeCandidateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
