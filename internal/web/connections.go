package web

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/url"

	"github.com/davidadel66/evie/internal/mcpconn"
)

// MCP Connections (workspace-mcp-connections.spec.md story 1): serve owns
// every sign-in and token, and the CLI and web UI reach it through these
// owner-only routes. The OAuth callback is the one unguarded route: the
// authorization server's redirect is a cross-site browser navigation, so a
// single-use, unexpired state is what it accepts.

// MCPConnections is serve's connection manager as these routes use it.
type MCPConnections interface {
	List(ctx context.Context) ([]mcpconn.Connection, error)
	Add(ctx context.Context, req mcpconn.AddRequest) (mcpconn.Connection, error)
	BeginConnect(ctx context.Context, name string) (mcpconn.Attempt, error)
	Wait(ctx context.Context, attemptID string) (mcpconn.Connection, error)
	Callback(ctx context.Context, query url.Values) (mcpconn.Connection, error)
	Disconnect(ctx context.Context, name string) (mcpconn.Connection, error)
}

// WithMCPConnections adds the connection routes and the OAuth callback.
func WithMCPConnections(server *Server, connections MCPConnections) *Server {
	server.mcpConnections = connections
	return server
}

func (s *Server) registerConnectionRoutes(mux *http.ServeMux) {
	if s.mcpConnections == nil {
		return
	}
	connections := s.mcpConnections
	mux.Handle("/api/connections/list", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct{}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		list, err := connections.List(r.Context())
		if err != nil {
			connectionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Connections []mcpconn.Connection `json:"connections"`
		}{list})
	}))
	mux.Handle("/api/connections/add", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Name         string `json:"name"`
			URL          string `json:"url"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		connection, err := connections.Add(r.Context(), mcpconn.AddRequest{Name: request.Name, URL: request.URL, ClientID: request.ClientID, ClientSecret: request.ClientSecret})
		if err != nil {
			connectionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, connection)
	}))
	mux.Handle("/api/connections/connect", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Name string `json:"name"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		attempt, err := connections.BeginConnect(r.Context(), request.Name)
		if err != nil {
			connectionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, attempt)
	}))
	mux.Handle("/api/connections/wait", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			AttemptID string `json:"attempt_id"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if !boundedID(request.AttemptID) {
			managementJSONError(w, http.StatusBadRequest, "invalid_connection_request", "wait takes an attempt ID")
			return
		}
		connection, err := connections.Wait(r.Context(), request.AttemptID)
		if err != nil {
			connectionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, connection)
	}))
	mux.Handle("/api/connections/disconnect", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Name string `json:"name"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		connection, err := connections.Disconnect(r.Context(), request.Name)
		if err != nil {
			connectionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, connection)
	}))
	mux.HandleFunc("/api/connections/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "the OAuth callback takes GET", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		connection, err := connections.Callback(r.Context(), r.URL.Query())
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeCallbackPage(w, "Sign-in failed", err.Error())
			return
		}
		writeCallbackPage(w, "Connected to "+connection.Name, "You can close this tab.")
	})
}

func writeCallbackPage(w http.ResponseWriter, title, message string) {
	_, _ = w.Write([]byte("<!doctype html><meta charset=\"utf-8\"><title>Evie</title>" +
		"<body style=\"font-family:system-ui;margin:3rem\"><h1>" + html.EscapeString(title) + "</h1><p>" + html.EscapeString(message) + "</p></body>"))
}

func connectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mcpconn.ErrInvalid):
		managementJSONError(w, http.StatusBadRequest, "invalid_connection_request", err.Error())
	case errors.Is(err, mcpconn.ErrConnectionNotFound):
		managementJSONError(w, http.StatusNotFound, "connection_not_found", err.Error())
	case errors.Is(err, mcpconn.ErrUnknownAttempt):
		managementJSONError(w, http.StatusNotFound, "attempt_not_found", err.Error())
	case errors.Is(err, mcpconn.ErrConnectionExists):
		managementJSONError(w, http.StatusConflict, "connection_exists", err.Error())
	case errors.Is(err, mcpconn.ErrKeychainUnavailable):
		managementJSONError(w, http.StatusServiceUnavailable, "keychain_unavailable", err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		managementJSONError(w, http.StatusGatewayTimeout, "connection_timeout", err.Error())
	default:
		managementJSONError(w, http.StatusBadGateway, "connection_failed", err.Error())
	}
}
