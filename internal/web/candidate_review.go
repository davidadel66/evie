package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// CandidateReviewKernel is the local owner surface; no source conversation or
// agent turn is consulted for authentication. Only guarded routes mint authority.
type CandidateReviewKernel interface {
	ListLocalOwnerCandidateScopes(context.Context, memory.OwnerCandidateScopeQuery) (memory.OwnerCandidateScopes, error)
	LocalOwnerReviewContext(context.Context, string) (eviedb.OwnerReviewContext, error)
	ListOwnerCandidates(context.Context, eviedb.OwnerReviewContext, memory.OwnerCandidateQuery) (memory.OwnerCandidatePage, error)
	InspectOwnerCandidate(context.Context, eviedb.OwnerReviewContext, string) (memory.OwnerCandidate, error)
	PrepareOwnerCandidateReview(context.Context, eviedb.OwnerReviewContext, memory.CandidateRef, string) (memory.ReviewPreview, error)
	ResolveOwnerCandidateReview(context.Context, eviedb.OwnerReviewContext, memory.ReviewDecision) (memory.ReviewResult, error)
	InspectOwnerReviewOperation(context.Context, eviedb.OwnerReviewContext, memory.SemanticID) (memory.OwnerReviewOperation, error)
}

func WithCandidateReview(server *Server, kernel CandidateReviewKernel) *Server {
	server.candidateReview = kernel
	return server
}

func (s *Server) registerCandidateReviewRoutes(mux *http.ServeMux) {
	if s.candidateReview == nil {
		return
	}
	for path, handler := range map[string]http.HandlerFunc{
		"scopes": s.handleCandidateScopes, "list": s.handleCandidateList, "inspect": s.handleCandidateInspect,
		"prepare": s.handleCandidatePrepare, "resolve": s.handleCandidateResolve, "operation": s.handleCandidateOperation,
	} {
		mux.Handle("/api/memory/candidates/"+path, s.managementRoute(handler))
	}
}

func decodeCandidateRequest(w http.ResponseWriter, r *http.Request, request any) bool {
	// Evidence and accepted operation bodies must not enter browser caches.
	w.Header().Set("Cache-Control", "no-store")
	// The 4 KiB owner reason plus its exact preview envelope needs more than
	// the legacy 4 KiB management-body budget. The complete request caps at 8 KiB.
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	status := http.StatusBadRequest
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		status = http.StatusRequestEntityTooLarge
	}
	if err == nil && strings.HasPrefix(strings.TrimSpace(string(b)), "{") {
		err = memory.ValidateCompilerJSON(b)
		if err == nil {
			decoder := json.NewDecoder(bytes.NewReader(b))
			decoder.DisallowUnknownFields()
			err = decoder.Decode(request)
		}
	} else if err == nil {
		err = errors.New("expected object")
	}
	if err != nil {
		managementJSONError(w, status, "invalid_review_request", "provide one bounded JSON review request with only the documented fields")
		return false
	}
	return true
}

func (s *Server) candidateAuthority(w http.ResponseWriter, r *http.Request, scope string) (eviedb.OwnerReviewContext, bool) {
	if scope == "" || len(scope) > 512 || strings.TrimSpace(scope) != scope {
		managementJSONError(w, http.StatusBadRequest, "review_scope_required", "select one exact memory scope")
		return eviedb.OwnerReviewContext{}, false
	}
	a, err := s.candidateReview.LocalOwnerReviewContext(r.Context(), scope)
	if err != nil {
		writeCandidateError(w, err)
		return a, false
	}
	return a, true
}

func (s *Server) handleCandidateScopes(w http.ResponseWriter, r *http.Request) {
	var request memory.OwnerCandidateScopeQuery
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	out, err := s.candidateReview.ListLocalOwnerCandidateScopes(r.Context(), request)
	if err != nil {
		writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCandidateList(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Scope  string `json:"scope_key"`
		Limit  int    `json:"limit"`
		Cursor string `json:"cursor"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	a, ok := s.candidateAuthority(w, r, request.Scope)
	if !ok {
		return
	}
	if request.Limit < 0 || request.Limit > 100 || len(request.Cursor) > 2048 {
		managementJSONError(w, http.StatusBadRequest, "invalid_review_request", "page limit must be 1–100 and the cursor must come from the preceding page")
		return
	}
	out, err := s.candidateReview.ListOwnerCandidates(r.Context(), a, memory.OwnerCandidateQuery{Limit: request.Limit, Cursor: request.Cursor})
	if err != nil {
		writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type candidateInspectRequest struct {
	Scope string `json:"scope_key"`
	ID    string `json:"id"`
}

func (s *Server) handleCandidateInspect(w http.ResponseWriter, r *http.Request) {
	var request candidateInspectRequest
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	a, ok := s.candidateAuthority(w, r, request.Scope)
	if !ok {
		return
	}
	out, err := s.candidateReview.InspectOwnerCandidate(r.Context(), a, request.ID)
	if err != nil {
		writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCandidateOperation(w http.ResponseWriter, r *http.Request) {
	var request candidateInspectRequest
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	a, ok := s.candidateAuthority(w, r, request.Scope)
	if !ok {
		return
	}
	out, err := s.candidateReview.InspectOwnerReviewOperation(r.Context(), a, memory.SemanticID(request.ID))
	if err != nil {
		writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCandidatePrepare(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Scope  string              `json:"scope_key"`
		Ref    memory.CandidateRef `json:"candidate"`
		Action string              `json:"action"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	a, ok := s.candidateAuthority(w, r, request.Scope)
	if !ok {
		return
	}
	if request.Ref.ID == "" || request.Ref.InterpretationRevision < 0 || request.Ref.ReviewRevision < 0 || !candidateAction(request.Action) {
		managementJSONError(w, http.StatusBadRequest, "invalid_review_request", "an exact candidate revision and explicit accept or reject action are required")
		return
	}
	out, err := s.candidateReview.PrepareOwnerCandidateReview(r.Context(), a, request.Ref, request.Action)
	if err != nil {
		writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCandidateResolve(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Scope    string                `json:"scope_key"`
		Decision memory.ReviewDecision `json:"decision"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	a, ok := s.candidateAuthority(w, r, request.Scope)
	if !ok {
		return
	}
	if !candidateAction(request.Decision.Action) || request.Decision.PreviewID == "" || request.Decision.PreviewSHA256 == "" || request.Decision.DeliveryKey == "" {
		managementJSONError(w, http.StatusBadRequest, "invalid_review_request", "resolution requires the exact preview, digest, delivery key and explicit action")
		return
	}
	out, err := s.candidateReview.ResolveOwnerCandidateReview(r.Context(), a, request.Decision)
	if errors.Is(err, eviedb.ErrReviewResolved) {
		writeJSON(w, http.StatusConflict, struct {
			Code   string              `json:"code"`
			Error  string              `json:"error"`
			Result memory.ReviewResult `json:"result"`
		}{"already_resolved", "this candidate already has an owner decision", out})
		return
	}
	if err != nil {
		writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func candidateAction(action string) bool { return action == "accept" || action == "reject" }

func writeCandidateError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusServiceUnavailable, "review_retryable", "the review service could not confirm this request; retry the same decision if one was submitted"
	switch {
	case errors.Is(err, eviedb.ErrOwnerReviewUnauthorized):
		status, code, message = http.StatusForbidden, "review_unauthorized", "this memory scope or candidate is not available"
	case errors.Is(err, eviedb.ErrReviewStale), errors.Is(err, eviedb.ErrStaleScopeRevision):
		status, code, message = http.StatusConflict, "stale_preview", "memory or candidate state changed; refresh and review a new preview before deciding"
	case errors.Is(err, eviedb.ErrReviewResolved):
		status, code, message = http.StatusConflict, "already_resolved", "this candidate already has an owner decision; refresh its details"
	case errors.Is(err, eviedb.ErrIdempotencyConflict):
		status, code, message = http.StatusConflict, "idempotency_conflict", "this delivery key belongs to a different decision; inspect the recorded result before trying again"
	case errors.Is(err, eviedb.ErrReviewInvalidSource):
		status, code, message = http.StatusConflict, "source_ineligible", "supporting evidence is no longer eligible; acceptance is blocked, but you may inspect and reject the candidate"
	case errors.Is(err, eviedb.ErrInvalidCursor):
		status, code, message = http.StatusBadRequest, "invalid_cursor", "restart this paginated listing"
	case errors.Is(err, eviedb.ErrSemanticScopeQuarantined):
		status, code, message = http.StatusLocked, "review_scope_quarantined", "this memory scope needs local verification or repair before review"
	case strings.HasPrefix(err.Error(), "needs_choice:"):
		status, code, message = http.StatusConflict, "needs_choice", "this candidate needs an explicit identity choice through local review before acceptance"
	}
	managementJSONError(w, status, code, message)
}
