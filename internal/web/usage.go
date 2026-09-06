package web

import (
	"context"
	"net/http"

	"github.com/davidadel66/evie/internal/usage"
)

// UsageReader returns only typed operational measurements for the owner view.
type UsageReader interface {
	InspectUsage(context.Context, usage.Period) (usage.Report, error)
}

func WithUsage(server *Server, reader UsageReader) *Server {
	server.usageReader = reader
	return server
}
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var query struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Timezone string `json:"timezone"`
	}
	if status, err := decodeManagementJSON(w, r, &query); err != nil {
		jsonError(w, status, "body must be one valid usage query")
		return
	}
	period, err := usage.ParsePeriod(query.From, query.To, query.Timezone)
	if err != nil {
		managementJSONError(w, http.StatusBadRequest, "invalid_usage_range", "choose 1 to 90 calendar days and a valid timezone")
		return
	}
	report, err := s.usageReader.InspectUsage(r.Context(), period)
	if err != nil {
		managementJSONError(w, http.StatusUnprocessableEntity, "usage_unavailable", "usage could not be read")
		return
	}
	writeJSON(w, http.StatusOK, report)
}
