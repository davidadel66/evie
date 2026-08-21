// The server half of the package: the Server struct owns the session and
// the route table. Handler() returns the mux so tests can drive it
// through httptest with a fake provider — no port, no real model.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"

	"github.com/davidadel66/evie/internal/agent"
)

// Serve runs the web frontend until the process exits. Loopback only:
// there is no auth story yet, and off loopback the approval gate stops
// being a defense (docs/decisions.md) — so a non-loopback EVIE_ADDR
// is refused outright rather than warned about.
func Serve(session *agent.Session) error {
	addr, err := listenAddr()
	if err != nil {
		return err
	}
	log.Printf("evie serve listening on http://%s", addr)
	return http.ListenAndServe(addr, NewServer(session).Handler())
}

// listenAddr resolves EVIE_ADDR (default 127.0.0.1:6687) and enforces
// the loopback-only rule.
func listenAddr() (string, error) {
	addr := os.Getenv("EVIE_ADDR")
	if addr == "" {
		return "127.0.0.1:6687", nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("EVIE_ADDR %q is not host:port: %w", addr, err)
	}
	if !isLoopbackHost(host) {
		return "", fmt.Errorf("refusing to bind %q: no auth story yet, loopback only — see docs/decisions.md", addr)
	}
	return addr, nil
}

// Server is the web frontend's whole state: one session (one
// conversation, v1), plus the approvals waiting for a browser answer.
type Server struct {
	session *agent.Session

	mu      sync.Mutex
	pending map[string]chan bool
}

// NewServer wires a server around an existing session. The session is
// shared with any other frontend holding it; agent.Session's own lock
// arbitrates.
func NewServer(session *agent.Session) *Server {
	return &Server{
		session: session,
		pending: make(map[string]chan bool),
	}
}

// Handler is the route table. Every /api route sits behind the
// cross-origin guard — bash is ungated, so a drive-by form POST from a
// malicious page must die here, not in the handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", s.guard(s.handleChat))
	mux.HandleFunc("POST /api/approve", s.guard(s.handleApprove))
	mux.Handle("/", s.staticHandler())
	return mux
}

// guard rejects requests that a browser could be tricked into sending
// cross-origin. Two checks: the exact JSON content type (an HTML form
// cannot produce it), and Origin/Host must be loopback (a foreign page's
// fetch carries its own Origin; DNS rebinding shows up as a foreign
// Host). 403 on any failure.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			jsonError(w, http.StatusForbidden, "content type must be application/json")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isLoopbackHost(u.Hostname()) {
				jsonError(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}
		}
		if host, _, err := net.SplitHostPort(r.Host); err == nil {
			if !isLoopbackHost(host) {
				jsonError(w, http.StatusForbidden, "unexpected host")
				return
			}
		} else if !isLoopbackHost(r.Host) {
			jsonError(w, http.StatusForbidden, "unexpected host")
			return
		}
		next(w, r)
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// handleChat runs one turn: decode the message, stream the turn's events
// into the response, always finish with turn_done. A busy session is the
// one case that answers with a status instead of a stream — nothing has
// been written yet when TryLock fails, so the response is still free for
// a plain 409.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON with a message field")
		return
	}

	ev, err := newSSEEvents(w)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendErr := s.session.Send(context.Background(), req.Message, ev, s.approver(r.Context(), ev))

	if errors.Is(sendErr, agent.ErrBusy) && !ev.wrote {
		jsonError(w, http.StatusConflict, "a turn is already in progress")
		return
	}
	if sendErr != nil {
		ev.Error(sendErr.Error())
	}
	ev.TurnDone()
}

// jsonError writes the {"error": ...} body every non-stream failure
// uses. Content-Type is reset in case SSE headers were already staged.
func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Del("Cache-Control")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
