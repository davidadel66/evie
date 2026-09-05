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
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

// Serve runs the web frontend until the process exits. Loopback only:
// there is no auth story yet, and off loopback the approval gate stops
// being a defense (docs/decisions.md) — so a non-loopback EVIE_ADDR
// is refused outright rather than warned about.
func Serve(session *agent.Session) error {
	return serveServer(NewServer(session))
}

// ServeManaged keeps Kernel-owned management available even when a degraded
// startup could not compose a chat session.
func ServeManaged(session *agent.Session, manager *plugins.Manager, receipts ReceiptInspector) error {
	return serveServer(NewManagedServer(session, manager, receipts))
}

func ServeContextManaged(
	manager *plugins.Manager,
	receipts ReceiptInspector,
	contextSessions ContextSessionController,
	semanticMemory agent.SemanticGraphMemory,
) error {
	return serveServer(NewContextMemoryServer(nil, manager, receipts, contextSessions, semanticMemory))
}

func serveServer(server *Server) error {
	addr, err := listenAddr()
	if err != nil {
		return err
	}
	log.Printf("evie serve listening on http://%s", addr)
	return http.ListenAndServe(addr, server.Handler())
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

// Server is the web frontend's selected conversation plus the approvals
// waiting for a browser answer. Context-managed servers may replace the
// selected conversation only between turns.
type Server struct {
	sessionMu        sync.RWMutex
	session          *agent.Session
	activeSession    memory.Session
	activeTurns      int
	selectingSession bool
	manager          *plugins.Manager
	receipts         ReceiptInspector
	contextSessions  ContextSessionController
	semanticMemory   agent.SemanticGraphMemory
	candidateReview  CandidateReviewKernel

	mu      sync.Mutex
	pending map[string]chan bool
}

// ReceiptInspector is the read-only, Kernel-owned session audit boundary.
// Its value types structurally exclude credentials.
type ReceiptInspector interface {
	GetCompositionReceipt(context.Context, memory.SessionID) (plugins.CompositionReceipt, error)
	GetCompatibilityResolutions(context.Context, memory.SessionID) ([]plugins.CompatibilityResolution, error)
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

func NewManagedServer(session *agent.Session, manager *plugins.Manager, receipts ReceiptInspector) *Server {
	server := NewServer(session)
	server.manager = manager
	server.receipts = receipts
	return server
}

func NewContextServer(
	session *agent.Session,
	manager *plugins.Manager,
	receipts ReceiptInspector,
	contextSessions ContextSessionController,
) *Server {
	server := NewManagedServer(session, manager, receipts)
	server.contextSessions = contextSessions
	return server
}

// NewContextMemoryServer composes the read-only Semantic Memory surface with
// the Context-managed web server without changing the legacy test seam.
func NewContextMemoryServer(
	session *agent.Session,
	manager *plugins.Manager,
	receipts ReceiptInspector,
	contextSessions ContextSessionController,
	semanticMemory agent.SemanticGraphMemory,
) *Server {
	server := NewContextServer(session, manager, receipts, contextSessions)
	server.semanticMemory = semanticMemory
	return server
}

// Handler is the route table. Every /api route sits behind the
// cross-origin guard — bash is ungated, so a drive-by form POST from a
// malicious page must die here, not in the handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", s.guard(s.handleChat))
	mux.HandleFunc("POST /api/approve", s.guard(s.handleApprove))
	if s.manager != nil {
		mux.Handle("/api/plugins/list", s.managementRoute(s.handlePluginList))
		mux.Handle("/api/plugins/lifecycle", s.managementRoute(s.handlePluginLifecycle))
		mux.Handle("/api/presets/list", s.managementRoute(s.handlePresetList))
		mux.Handle("/api/presets/validate", s.managementRoute(s.handlePresetValidate))
	}
	if s.receipts != nil {
		mux.Handle("/api/sessions/inspect", s.managementRoute(s.handleSessionInspect))
	}
	if s.contextSessions != nil {
		mux.Handle("/api/context-sessions/list", s.managementRoute(s.handleContextSessionList))
		mux.Handle("/api/context-sessions/select", s.managementRoute(s.handleContextSessionSelect))
		mux.Handle("/api/workspaces/register", s.managementRoute(s.handleWorkspaceRegister))
	}
	if s.semanticMemory != nil {
		mux.Handle("/api/memory/scopes", s.managementRoute(s.handleMemoryScopes))
		mux.Handle("/api/memory/objects", s.managementRoute(s.handleMemoryObjects))
		mux.Handle("/api/memory/inspect", s.managementRoute(s.handleMemoryInspect))
	}
	s.registerCandidateReviewRoutes(mux)
	mux.Handle("/", s.staticHandler())
	return mux
}

func (s *Server) managementRoute(next http.HandlerFunc) http.Handler {
	guarded := s.guard(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			jsonError(w, http.StatusMethodNotAllowed, "management routes require POST")
			return
		}
		guarded(w, r)
	})
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
	s.sessionMu.Lock()
	if s.selectingSession {
		s.sessionMu.Unlock()
		jsonError(w, http.StatusConflict, "a Context Scope selection is in progress")
		return
	}
	session := s.session
	if session != nil {
		s.activeTurns++
	}
	s.sessionMu.Unlock()
	if session == nil {
		jsonError(w, http.StatusServiceUnavailable, "chat is unavailable because the active Agent Preset is invalid; use plugin diagnostics to repair startup")
		return
	}
	defer func() {
		s.sessionMu.Lock()
		s.activeTurns--
		s.sessionMu.Unlock()
	}()
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

	measurementCtx, finalizeMeasurement := agent.BeginResponseMeasurement(turnLifecycleContext(r))
	sendErr := session.Send(measurementCtx, req.Message, ev, s.approver(r.Context(), ev))

	if (errors.Is(sendErr, agent.ErrBusy) || errors.Is(sendErr, agent.ErrLeaseConflict)) && !ev.wrote {
		jsonError(w, http.StatusConflict, "a turn is already in progress")
		return
	}
	if sendErr != nil {
		ev.Error(sendErr.Error())
	}
	outputErr := ev.TurnDone()
	if measurementErr := finalizeMeasurement(outputErr); measurementErr != nil {
		log.Printf("compiler foreground measurement unavailable")
	}
}

// jsonError writes the {"error": ...} body every non-stream failure
// uses. Content-Type is reset in case SSE headers were already staged.
func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Del("Cache-Control")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
