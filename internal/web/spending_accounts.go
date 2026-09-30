package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/finance"
)

// SpendingAccountsService keeps provider credentials and connection recovery in
// finance. The browser supplies only an opaque, locally issued link handle.
type SpendingAccountsService interface {
	InspectAccounts(context.Context) (finance.SpendingAccountsReport, error)
	RefreshAccounts(context.Context) (finance.SpendingAccountsRefresh, error)
	StartAccountLink(context.Context) (finance.SpendingAccountLink, error)
	CompleteAccountLink(context.Context, string) (finance.SpendingAccountLinkResult, error)
	CancelAccountLink(context.Context, string) error
}

type spendingAccountsHandler struct{ service SpendingAccountsService }

func (s *Server) registerSpendingAccountRoutes(mux *http.ServeMux) {
	service, ok := s.spendingService.(SpendingAccountsService)
	if !ok {
		return
	}
	h := spendingAccountsHandler{service: service}
	mux.Handle("/api/data/spending/accounts", s.managementRoute(h.inspect))
	mux.Handle("/api/data/spending/accounts/refresh", s.managementRoute(h.refresh))
	mux.Handle("/api/data/spending/link/start", s.managementRoute(h.start))
	mux.Handle("/api/data/spending/link/status", s.managementRoute(h.status))
	mux.Handle("/api/data/spending/link/cancel", s.managementRoute(h.cancel))
}

func spendingAccountEmptyRequest(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if status, err := decodeManagementEmptyObject(w, r); err != nil {
		managementJSONError(w, status, "invalid_spending_accounts_request", "body must be one empty JSON object")
		return false
	}
	return true
}

func (h spendingAccountsHandler) inspect(w http.ResponseWriter, r *http.Request) {
	if !spendingAccountEmptyRequest(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	report, err := h.service.InspectAccounts(ctx)
	if err != nil {
		spendingAccountsJSONError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h spendingAccountsHandler) refresh(w http.ResponseWriter, r *http.Request) {
	if !spendingAccountEmptyRequest(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spendingRefreshTimeout)
	defer cancel()
	report, err := h.service.RefreshAccounts(ctx)
	if err != nil {
		spendingAccountsJSONError(w, errors.Join(finance.ErrSpendingAccountsRefreshUnavailable, err))
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h spendingAccountsHandler) start(w http.ResponseWriter, r *http.Request) {
	if !spendingAccountEmptyRequest(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	link, err := h.service.StartAccountLink(ctx)
	if err != nil {
		spendingAccountsJSONError(w, errors.Join(finance.ErrSpendingLinkUnavailable, err))
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func spendingAccountLinkID(w http.ResponseWriter, r *http.Request) (string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	var request struct {
		ID string `json:"id"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		managementJSONError(w, status, "invalid_spending_link_request", "body must contain a saved connection ID")
		return "", false
	}
	if request.ID == "" || len(request.ID) > 128 || strings.TrimSpace(request.ID) != request.ID {
		managementJSONError(w, http.StatusBadRequest, "invalid_spending_link_request", "choose a saved connection")
		return "", false
	}
	return request.ID, true
}

func (h spendingAccountsHandler) status(w http.ResponseWriter, r *http.Request) {
	id, ok := spendingAccountLinkID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := h.service.CompleteAccountLink(ctx, id)
	if err != nil {
		spendingAccountsJSONError(w, errors.Join(finance.ErrSpendingLinkUnavailable, err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h spendingAccountsHandler) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := spendingAccountLinkID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.service.CancelAccountLink(ctx, id); err != nil {
		spendingAccountsJSONError(w, errors.Join(finance.ErrSpendingLinkUnavailable, err))
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

func spendingAccountsJSONError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		managementJSONError(w, http.StatusGatewayTimeout, "spending_accounts_timeout", "bank request timed out; reload connections before trying again")
	case errors.Is(err, context.Canceled):
		managementJSONError(w, http.StatusRequestTimeout, "spending_accounts_cancelled", "bank request was cancelled; reload connections")
	case errors.Is(err, finance.ErrSpendingAccountsRefreshInProgress), errors.Is(err, finance.ErrSpendingLinkInProgress):
		managementJSONError(w, http.StatusConflict, "spending_accounts_busy", "another bank connection request is in progress")
	case errors.Is(err, finance.ErrSpendingLinkNotFound):
		managementJSONError(w, http.StatusNotFound, "spending_link_not_found", "this connection is no longer available; start again")
	case errors.Is(err, finance.ErrSpendingLinkUnavailable):
		managementJSONError(w, http.StatusUnprocessableEntity, "spending_link_unavailable", "bank connection could not be completed; reload connections before trying again")
	case errors.Is(err, finance.ErrSpendingAccountsRefreshUnavailable):
		managementJSONError(w, http.StatusUnprocessableEntity, "spending_accounts_refresh_unavailable", "account details could not be refreshed")
	default:
		managementJSONError(w, http.StatusUnprocessableEntity, "spending_accounts_unavailable", "connected accounts could not be loaded")
	}
}
