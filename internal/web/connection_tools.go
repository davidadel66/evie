package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/mcpchat"
	"github.com/davidadel66/evie/internal/mcpconn"
	"github.com/davidadel66/evie/internal/memory"
)

// The owner's MCP tool review (workspace-mcp-connections.spec.md story 2):
// list one connection's live tools as a Workspace sees them, and enable or
// disable one. Serve lists the tools because only serve holds tokens.

// MCPToolReview is the review side of serve's MCP connections.
type MCPToolReview interface {
	Review(ctx context.Context, ws memory.WorkspaceID, name string) (mcpchat.Review, error)
	SetToolEnabled(ctx context.Context, ws memory.WorkspaceID, name, tool string, enabled bool) (mcpchat.Review, error)
}

// WithMCPToolReview adds the tool review routes.
func WithMCPToolReview(server *Server, review MCPToolReview) *Server {
	server.mcpToolReview = review
	return server
}

func (s *Server) registerConnectionToolRoutes(mux *http.ServeMux) {
	if s.mcpToolReview == nil {
		return
	}
	review := s.mcpToolReview
	mux.Handle("/api/connections/tools", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspace_id"`
			Name        string             `json:"name"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if !boundedID(string(request.WorkspaceID)) {
			managementJSONError(w, http.StatusBadRequest, "invalid_connection_request", "tools takes a Workspace ID and a connection name")
			return
		}
		result, err := review.Review(r.Context(), request.WorkspaceID, request.Name)
		if err != nil {
			toolReviewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.Handle("/api/connections/tools/set", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspace_id"`
			Name        string             `json:"name"`
			Tool        string             `json:"tool"`
			Enabled     *bool              `json:"enabled"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if !boundedID(string(request.WorkspaceID)) || request.Tool == "" || request.Enabled == nil {
			managementJSONError(w, http.StatusBadRequest, "invalid_connection_request", "set takes a Workspace ID, a connection name, a tool and enabled")
			return
		}
		result, err := review.SetToolEnabled(r.Context(), request.WorkspaceID, request.Name, request.Tool, *request.Enabled)
		if err != nil {
			toolReviewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
}

func toolReviewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, eviedb.ErrWorkspaceAccessChanged):
		managementJSONError(w, http.StatusConflict, "access_changed", "the Workspace Access changed; nothing was written, try again")
	case errors.Is(err, eviedb.ErrWorkspaceInactive):
		managementJSONError(w, http.StatusConflict, "workspace_archived", "the Workspace is archived; restore it to change its tools")
	case errors.Is(err, eviedb.ErrWorkspaceNotFound):
		managementJSONError(w, http.StatusNotFound, "workspace_not_found", "no such Workspace")
	case errors.Is(err, mcpchat.ErrToolNotOffered), errors.Is(err, mcpchat.ErrToolUnselectable):
		managementJSONError(w, http.StatusBadRequest, "invalid_tool", err.Error())
	case errors.Is(err, mcpconn.ErrNotConnected):
		managementJSONError(w, http.StatusConflict, "not_signed_in", "the connection is not signed in; run evie connections connect first")
	default:
		connectionError(w, err)
	}
}
