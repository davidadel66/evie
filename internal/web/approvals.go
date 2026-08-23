// The approval handshake: a gated tool call pauses the turn mid-Send
// while the answer arrives as a separate HTTP request. The approver
// blocks on a channel; /api/approve finds that channel by id and sends
// into it. Context-done (the browser went away) resolves to Expired so
// the model is never told David declined something he never saw.
package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/davidadel66/evie/internal/tools"
)

// newPending registers one waiting approval and returns its id and the
// channel its answer will arrive on. The channel is buffered so the
// approve handler can send without waiting for the approver to be at
// the receive — they race when a context cancels at the same moment.
func (s *Server) newPending() (string, chan bool) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	id := hex.EncodeToString(b)
	ch := make(chan bool, 1)

	s.mu.Lock()
	s.pending[id] = ch
	s.mu.Unlock()
	return id, ch
}

func (s *Server) expirePending(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.pending[id]; !ok {
		return false
	}
	delete(s.pending, id)
	return true
}

// approver builds the per-turn gate handed to Send: emit the request on
// this turn's stream, then block until the browser answers or the
// request dies. This is the pause the REPL gets from scanner.Scan().
func (s *Server) approver(visibilityCtx context.Context, ev *sseEvents) tools.Approver {
	return func(lifecycleCtx context.Context, name, args string, preview *tools.FileChangePreview) tools.Decision {
		id, ch := s.newPending()

		ev.ApprovalRequest(id, name, args, preview)

		select {
		case approved := <-ch:
			if approved {
				return tools.Approved
			}
			return tools.Declined
		case <-visibilityCtx.Done():
			if s.expirePending(id) {
				return tools.Expired
			}
			// A POST removed the entry first under the same lock. Honor that
			// already-claimed decision even though the disconnect is now visible.
			approved := <-ch
			if approved {
				return tools.Approved
			}
			return tools.Declined
		case <-lifecycleCtx.Done():
			if s.expirePending(id) {
				return tools.Expired
			}
			// A POST claimed the approval first. Return its decision; the
			// dispatcher resamples lifecycleCtx and still reports cancellation
			// before observation or execution.
			approved := <-ch
			if approved {
				return tools.Approved
			}
			return tools.Declined
		}
	}
}

// handleApprove resolves one pending approval. Taking the channel out
// of the map under the lock means exactly one answer can ever win; a
// second POST for the same id finds nothing and 404s, same as an id
// that already expired.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Approve bool   `json:"approve"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON with id and approve fields")
		return
	}

	s.mu.Lock()
	ch, ok := s.pending[req.ID]
	if ok {
		delete(s.pending, req.ID)
	}
	s.mu.Unlock()

	if !ok {
		jsonError(w, http.StatusNotFound, "unknown or expired approval id")
		return
	}

	ch <- req.Approve
	w.WriteHeader(http.StatusNoContent)
}
