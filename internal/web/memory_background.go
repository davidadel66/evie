package web

import (
	"context"
	"net/http"

	"github.com/davidadel66/evie/internal/memory"
)

// MemoryBackgroundStatus reports the always-on background memory pass
// (memory architecture Stage 4): whether it is on and why not, its model,
// triggers, caps, today's spend, last run and conversation cursors. It holds
// no message, proposal or memory text; conversations appear by title.
type MemoryBackgroundStatus interface {
	MemoryBackgroundStatus(context.Context) (memory.BackgroundStatus, error)
}

func WithMemoryBackground(server *Server, status MemoryBackgroundStatus) *Server {
	server.memoryBackground = status
	return server
}

func (s *Server) registerMemoryBackgroundRoutes(mux *http.ServeMux) {
	if s.memoryBackground == nil {
		return
	}
	mux.Handle("/api/memory/background/status", s.managementRoute(s.handleMemoryBackgroundStatus))
}

func (s *Server) handleMemoryBackgroundStatus(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	status, err := s.memoryBackground.MemoryBackgroundStatus(r.Context())
	if err != nil {
		managementJSONError(w, http.StatusServiceUnavailable, "background_status_unavailable", "the background memory pass status could not be read; try again")
		return
	}
	writeJSON(w, http.StatusOK, status)
}
