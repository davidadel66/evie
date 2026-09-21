package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/davidadel66/evie/internal/finance"
)

const spendingRefreshTimeout = 2 * time.Minute

// SpendingService exposes the typed owner view and explicit bank refresh without
// exposing bank credentials or the finance database to the HTTP layer.
type SpendingService interface {
	InspectSpending(context.Context, int) (finance.SpendingReport, error)
	InspectSpendingTransactions(context.Context, finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error)
	InspectSpendingCashFlow(context.Context, finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error)
	RefreshSpending(context.Context) (finance.SpendingRefresh, error)
}

func WithSpending(server *Server, service SpendingService) *Server {
	server.spendingService = service
	return server
}

func (s *Server) handleSpendingSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var query struct {
		Year int `json:"year"`
	}
	if status, err := decodeManagementJSON(w, r, &query); err != nil {
		managementJSONError(w, status, "invalid_spending_query", "body must be one valid spending query")
		return
	}
	if query.Year < 1 || query.Year > 9999 {
		managementJSONError(w, http.StatusBadRequest, "invalid_spending_year", "choose a year from 1 to 9999")
		return
	}
	report, err := s.spendingService.InspectSpending(r.Context(), query.Year)
	if err != nil {
		spendingJSONError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleSpendingTransactions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var query finance.SpendingTransactionsQuery
	if status, err := decodeManagementJSON(w, r, &query); err != nil {
		managementJSONError(w, status, "invalid_spending_transactions_query", "body must be one valid spending transactions query")
		return
	}
	query, err := finance.ValidateSpendingTransactionsQuery(query)
	if err != nil {
		spendingJSONError(w, err, false)
		return
	}
	report, err := s.spendingService.InspectSpendingTransactions(r.Context(), query)
	if err != nil {
		spendingJSONError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleSpendingCashFlow(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var query finance.SpendingCashFlowQuery
	if status, err := decodeManagementJSON(w, r, &query); err != nil {
		managementJSONError(w, status, "invalid_spending_cash_flow_query", "body must be one valid cash-flow query")
		return
	}
	query, err := finance.ValidateSpendingCashFlowQuery(query)
	if err != nil {
		spendingJSONError(w, err, false)
		return
	}
	report, err := s.spendingService.InspectSpendingCashFlow(r.Context(), query)
	if err != nil {
		spendingJSONError(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleSpendingRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if status, err := decodeManagementEmptyObject(w, r); err != nil {
		managementJSONError(w, status, "invalid_spending_refresh", "body must be an empty JSON object")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), spendingRefreshTimeout)
	defer cancel()
	result, err := s.spendingService.RefreshSpending(ctx)
	if err != nil {
		spendingJSONError(w, err, true)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func spendingJSONError(w http.ResponseWriter, err error, refresh bool) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		managementJSONError(w, http.StatusGatewayTimeout, "spending_timeout", "spending request timed out; try again")
	case errors.Is(err, context.Canceled):
		managementJSONError(w, http.StatusRequestTimeout, "spending_cancelled", "spending request was cancelled")
	case errors.Is(err, finance.ErrSpendingRefreshInProgress):
		managementJSONError(w, http.StatusConflict, "spending_refresh_in_progress", "transactions are already being refreshed")
	case errors.Is(err, finance.ErrSpendingYear):
		managementJSONError(w, http.StatusBadRequest, "invalid_spending_year", "choose a year from 1 to 9999")
	case errors.Is(err, finance.ErrSpendingTransactionsQuery):
		managementJSONError(w, http.StatusBadRequest, "invalid_spending_transactions_query", "choose a valid month, classification filter, and page of up to 100 transactions")
	case errors.Is(err, finance.ErrSpendingCashFlowQuery):
		managementJSONError(w, http.StatusBadRequest, "invalid_spending_cash_flow_query", "choose a month within the history range and a valid cutoff date")
	case refresh:
		managementJSONError(w, http.StatusUnprocessableEntity, "spending_refresh_unavailable", "transactions could not be refreshed; try again")
	default:
		managementJSONError(w, http.StatusUnprocessableEntity, "spending_unavailable", "spending could not be read")
	}
}
