package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// CompilerDiagnosticsKernel owns bounded operational projections and current
// source authorization. Legacy review-only kernels need not implement this view.
type CompilerDiagnosticsKernel interface {
	ListOwnerCompilerSessions(context.Context, eviedb.OwnerReviewContext, memory.CompilerDiagnosticSessionQuery) (memory.CompilerDiagnosticSessions, error)
	InspectOwnerCompilerDiagnostics(context.Context, eviedb.OwnerReviewContext, memory.CompilerDiagnosticsQuery) (memory.CompilerDiagnostics, error)
}

func (s *Server) registerCompilerDiagnosticRoutes(mux *http.ServeMux) {
	k, ok := s.candidateReview.(CompilerDiagnosticsKernel)
	if !ok {
		return
	}
	for path, handler := range map[string]http.HandlerFunc{
		"sessions":    compilerDiagnosticRoute(s, k.ListOwnerCompilerSessions),
		"diagnostics": compilerDiagnosticRoute(s, k.InspectOwnerCompilerDiagnostics),
	} {
		guarded := s.managementRoute(handler)
		mux.Handle("/api/memory/compiler/"+path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			guarded.ServeHTTP(compilerDiagnosticResponse{w}, r)
		}))
	}
}

func compilerDiagnosticRoute[Request, Response any](s *Server, call func(context.Context, eviedb.OwnerReviewContext, Request) (Response, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Scope string   `json:"scope_key"`
			Input *Request `json:"input"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if request.Input == nil {
			writeCompilerDiagnosticError(w, eviedb.ErrReviewInvalidRequest)
			return
		}
		a, ok := s.candidateAuthority(w, r, request.Scope)
		if !ok {
			return
		}
		out, err := call(r.Context(), a, *request.Input)
		if err != nil {
			writeCompilerDiagnosticError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func writeCompilerDiagnosticError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, eviedb.ErrReviewInvalidRequest):
		managementJSONError(w, http.StatusBadRequest, "invalid_diagnostic_request", "select a session, a documented view, and a page limit from 1 to 32; selection also requires an exact generation ID")
	case errors.Is(err, eviedb.ErrReviewTooLarge):
		managementJSONError(w, http.StatusRequestEntityTooLarge, "diagnostics_too_large", "this diagnostic page exceeds its bound; request a smaller page")
	case errors.Is(err, eviedb.ErrOwnerReviewUnauthorized), errors.Is(err, eviedb.ErrInvalidCursor), errors.Is(err, eviedb.ErrSemanticScopeQuarantined), errors.Is(err, eviedb.ErrReviewInvalidSource):
		writeCandidateError(w, err)
	default:
		managementJSONError(w, http.StatusServiceUnavailable, "diagnostics_retryable", "compiler diagnostics are temporarily unavailable; refresh to request a new snapshot")
	}
}

// The shared guard also serves streaming endpoints and clears cache headers on
// errors. This bounded JSON surface remains non-cacheable on every response.
type compilerDiagnosticResponse struct{ http.ResponseWriter }

func (w compilerDiagnosticResponse) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}
