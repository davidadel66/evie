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
	"github.com/davidadel66/evie/internal/eviedb"
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
	databaseInspector DatabaseInspector,
) error {
	return serveServer(NewContextDataServer(nil, manager, receipts, contextSessions, semanticMemory, databaseInspector))
}

func serveServer(server *Server) error {
	defer server.Close()
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
	sessionMu         sync.RWMutex
	session           *agent.Session
	activeSession     memory.Session
	activeTurns       int
	modelRevision     int64
	selectingSession  bool
	manager           *plugins.Manager
	receipts          ReceiptInspector
	contextSessions   ContextSessionController
	semanticMemory    agent.SemanticGraphMemory
	memoryEvidence    MemoryEvidenceInspector
	databaseInspector DatabaseInspector
	candidateReview   CandidateReviewKernel
	usageReader       UsageReader
	spendingService   SpendingService
	terminalMu        sync.Mutex
	terminals         map[string]*terminalEntry
	terminalsClosed   bool

	folderPickerMu     sync.Mutex
	folderPicker       func(context.Context) (workspaceFolderChoice, error)
	folderPickerCancel context.CancelFunc
	folderPickerDone   chan struct{}
	folderPickerClosed bool

	mu      sync.Mutex
	pending map[string]chan bool

	// turns holds the stop control of each running web turn, keyed by its
	// conversation. Guarded by sessionMu.
	turns map[memory.SessionID]*webTurn
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
		session:   session,
		pending:   make(map[string]chan bool),
		terminals: make(map[string]*terminalEntry),
		turns:     make(map[memory.SessionID]*webTurn),
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
	server.memoryEvidence, _ = semanticMemory.(MemoryEvidenceInspector)
	return server
}

// NewContextDataServer adds the physical database-inspection seam to the
// Context and Semantic Memory server. Keeping it separate preserves focused
// fakes for callers that only need one of those owner surfaces.
func NewContextDataServer(
	session *agent.Session,
	manager *plugins.Manager,
	receipts ReceiptInspector,
	contextSessions ContextSessionController,
	semanticMemory agent.SemanticGraphMemory,
	databaseInspector DatabaseInspector,
) *Server {
	server := NewContextMemoryServer(session, manager, receipts, contextSessions, semanticMemory)
	server.databaseInspector = databaseInspector
	return server
}

// Handler is the route table. Every /api route sits behind the
// cross-origin guard — bash is ungated, so a drive-by form POST from a
// malicious page must die here, not in the handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", s.guard(s.handleChat))
	mux.HandleFunc("POST /api/approve", s.guard(s.handleApprove))
	mux.Handle("/api/cancel", s.managementRoute(s.handleCancel))
	mux.Handle("/api/compact", s.managementRoute(s.handleCompact))
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
		if _, ok := s.contextSessions.(ModelController); ok {
			mux.Handle("/api/models/list", s.managementRoute(s.handleModelList))
			mux.Handle("/api/models/select", s.managementRoute(s.handleModelSelect))
		}
		mux.Handle("/api/context-sessions/list", s.managementRoute(s.handleContextSessionList))
		mux.Handle("/api/context-sessions/history", s.managementRoute(s.handleContextSessionHistory))
		mux.Handle("/api/context-sessions/select", s.managementRoute(s.handleContextSessionSelect))
		if _, ok := s.contextSessions.(contextSessionArchiveController); ok {
			mux.Handle("/api/context-sessions/archive", s.managementRoute(s.handleContextSessionArchive))
			mux.Handle("/api/context-sessions/restore", s.managementRoute(s.handleContextSessionRestore))
		}
		mux.Handle("/api/workspaces/register", s.managementRoute(s.handleWorkspaceRegister))
		if _, ok := s.contextSessions.(workspaceResearchController); ok {
			mux.Handle("/api/workspaces/research", s.managementRoute(s.handleWorkspaceResearch))
		}
		mux.Handle("/api/workspaces/choose-folder", s.managementRoute(s.handleWorkspaceChooseFolder))
	}
	if s.semanticMemory != nil {
		mux.Handle("/api/memory/scopes", s.managementRoute(s.handleMemoryScopes))
		mux.Handle("/api/memory/objects", s.managementRoute(s.handleMemoryObjects))
		mux.Handle("/api/memory/inspect", s.managementRoute(s.handleMemoryInspect))
	}
	if s.memoryEvidence != nil {
		mux.Handle("/api/memory/evidence", s.managementRoute(s.handleMemoryEvidence))
	}
	if s.databaseInspector != nil {
		mux.Handle("/api/data/database/schema", s.managementRoute(s.handleDatabaseSchema))
		mux.Handle("/api/data/database/rows", s.managementRoute(s.handleDatabaseRows))
	}
	if s.usageReader != nil {
		mux.Handle("/api/data/usage/summary", s.managementRoute(s.handleUsage))
	}
	if s.spendingService != nil {
		mux.Handle("/api/data/spending/summary", s.managementRoute(s.handleSpendingSummary))
		mux.Handle("/api/data/spending/transactions", s.managementRoute(s.handleSpendingTransactions))
		mux.Handle("/api/data/spending/cash-flow", s.managementRoute(s.handleSpendingCashFlow))
		mux.Handle("/api/data/spending/day", s.managementRoute(s.handleSpendingDay))
		mux.Handle("/api/data/spending/category", s.managementRoute(s.handleSpendingCategory))
		mux.Handle("/api/data/spending/refresh", s.managementRoute(s.handleSpendingRefresh))
	}
	s.registerSpendingAccountRoutes(mux)
	s.registerCandidateReviewRoutes(mux)
	s.registerFolderRoutes(mux)
	s.registerRepositoryInstructionRoutes(mux)
	s.registerTerminalRoutes(mux)
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
	var req struct {
		Message   string           `json:"message"`
		SessionID memory.SessionID `json:"sessionId"`
		Model     string           `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "body must be JSON with a message field")
		return
	}

	s.sessionMu.Lock()
	if s.selectingSession {
		s.sessionMu.Unlock()
		jsonError(w, http.StatusConflict, "a Context Scope selection is in progress")
		return
	}
	if s.contextSessions != nil && (req.SessionID == "" || req.SessionID != s.activeSession.ID) {
		s.sessionMu.Unlock()
		jsonError(w, http.StatusConflict, "the active conversation changed; reload before sending")
		return
	}
	session := s.session
	if session != nil && req.Model != "" && req.Model != session.ContextProfile().ConfiguredModel {
		s.sessionMu.Unlock()
		jsonError(w, http.StatusConflict, "Chat model changed; refresh before sending")
		return
	}
	turnKey := s.activeSession.ID
	turnCtx, stopTurn := context.WithCancelCause(turnLifecycleContext(r))
	defer stopTurn(nil)
	turn := &webTurn{stop: stopTurn}
	if session != nil {
		if !s.beginWebTurn(turnKey, turn) {
			s.sessionMu.Unlock()
			jsonError(w, http.StatusConflict, "a turn is already in progress")
			return
		}
		s.activeTurns++
	}
	s.sessionMu.Unlock()
	if session == nil {
		jsonError(w, http.StatusServiceUnavailable, "chat is unavailable because the active Agent Preset is invalid; use plugin diagnostics to repair startup")
		return
	}
	defer func() {
		s.finishWebTurn(turnKey, turn)
		s.sessionMu.Lock()
		s.activeTurns--
		s.sessionMu.Unlock()
	}()

	ev, err := newSSEEvents(w)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	measurementCtx, finalizeMeasurement := agent.BeginResponseMeasurement(turnCtx)
	sendErr := session.Send(measurementCtx, req.Message, ev, s.approver(r.Context(), ev))
	s.finishWebTurn(turnKey, turn)

	if errors.Is(sendErr, eviedb.ErrSessionModelChanged) && !ev.wrote {
		jsonError(w, http.StatusConflict, "Chat model changed; refresh before sending")
		return
	}
	if (errors.Is(sendErr, agent.ErrBusy) || errors.Is(sendErr, agent.ErrLeaseConflict)) && !ev.wrote {
		jsonError(w, http.StatusConflict, "a turn is already in progress")
		return
	}
	if stoppedByOwner(turnCtx, sendErr) {
		ev.TurnStopped()
	} else if sendErr != nil {
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
