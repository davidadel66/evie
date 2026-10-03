package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// MemoryProposalInbox is the owner's review inbox for memories Evie proposed
// without being asked (docs/memory-architecture.md, Stage 2). An approval
// applies the exact listed proposal through the source conversation's
// durable approval path; no agent turn is involved.
type MemoryProposalInbox interface {
	ListMemoryProposals(context.Context) (memory.MemoryProposalPage, error)
	PendingMemoryProposalCount(context.Context, ...string) (int, error)
	ApproveMemoryProposal(context.Context, memory.MemoryProposalApproval) (memory.MemoryProposalResult, error)
	DeclineMemoryProposal(context.Context, string) error
}

func (s *Server) memoryProposalInbox() (MemoryProposalInbox, bool) {
	inbox, ok := s.candidateReview.(MemoryProposalInbox)
	return inbox, ok
}

func (s *Server) registerMemoryProposalRoutes(mux *http.ServeMux) {
	if _, ok := s.memoryProposalInbox(); !ok {
		return
	}
	mux.Handle("/api/memory/proposals/list", s.managementRoute(s.handleMemoryProposalList))
	mux.Handle("/api/memory/proposals/count", s.managementRoute(s.handleMemoryProposalCount))
	mux.Handle("/api/memory/proposals/approve", s.managementRoute(s.handleMemoryProposalApprove))
	mux.Handle("/api/memory/proposals/decline", s.managementRoute(s.handleMemoryProposalDecline))
}

func (s *Server) handleMemoryProposalList(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	inbox, _ := s.memoryProposalInbox()
	page, err := inbox.ListMemoryProposals(r.Context())
	if err != nil {
		writeMemoryProposalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleMemoryProposalCount(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	inbox, _ := s.memoryProposalInbox()
	pending, err := inbox.PendingMemoryProposalCount(r.Context())
	if err != nil {
		writeMemoryProposalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Pending int `json:"pending"`
	}{pending})
}

func (s *Server) handleMemoryProposalApprove(w http.ResponseWriter, r *http.Request) {
	var request memory.MemoryProposalApproval
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if request.ProposalID == "" || len(request.ProposalID) > 64 || request.PreparedSHA256 == "" || len(request.PreparedSHA256) > 128 {
		managementJSONError(w, http.StatusBadRequest, "invalid_proposal_request", "approval requires the proposal ID and the exact prepared hash it was listed with")
		return
	}
	inbox, _ := s.memoryProposalInbox()
	result, err := inbox.ApproveMemoryProposal(r.Context(), request)
	if err != nil {
		writeMemoryProposalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMemoryProposalDecline(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ProposalID string `json:"proposal_id"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if request.ProposalID == "" || len(request.ProposalID) > 64 {
		managementJSONError(w, http.StatusBadRequest, "invalid_proposal_request", "decline requires the proposal ID")
		return
	}
	inbox, _ := s.memoryProposalInbox()
	if err := inbox.DeclineMemoryProposal(r.Context(), request.ProposalID); err != nil {
		writeMemoryProposalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ProposalID string `json:"proposal_id"`
		Declined   bool   `json:"declined"`
	}{request.ProposalID, true})
}

func writeMemoryProposalError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusServiceUnavailable, "proposal_retryable", "the memory inbox could not complete this request; refresh and try again"
	switch {
	case errors.Is(err, memory.ErrStandingMemoryFull):
		// The message lists the full tier's current memories so the owner
		// can drop or merge one, or approve with Always applies off.
		status, code, message = http.StatusConflict, "memory_full", err.Error()
	case errors.Is(err, eviedb.ErrMemoryProposalNotFound):
		status, code, message = http.StatusNotFound, "proposal_not_found", "this proposal no longer exists"
	case errors.Is(err, eviedb.ErrMemoryProposalResolved):
		status, code, message = http.StatusConflict, "proposal_resolved", "this proposal was already approved or declined"
	case errors.Is(err, eviedb.ErrMemoryProposalChanged):
		status, code, message = http.StatusConflict, "proposal_changed", "memory changed since this proposal was shown; review the updated proposal"
	case errors.Is(err, eviedb.ErrMemoryProposalBusy):
		status, code, message = http.StatusConflict, "proposal_busy", "its conversation is running a turn; approve it when the turn finishes"
	case errors.Is(err, eviedb.ErrMemoryProposalUnavailable):
		status, code, message = http.StatusConflict, "proposal_unavailable", err.Error()
	case errors.Is(err, eviedb.ErrSemanticScopeQuarantined):
		status, code, message = http.StatusLocked, "proposal_scope_quarantined", "this memory scope needs local verification or repair before review"
	}
	managementJSONError(w, status, code, message)
}
