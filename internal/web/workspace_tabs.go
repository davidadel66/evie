package web

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workspacetabs"
)

// workspaceTabsController finds the folder whose .evie/tabs a Workspace
// shows (workspace-tabs.spec.md). A missing or archived Workspace is
// eviedb.ErrWorkspaceNotFound; one without a folder is "".
type workspaceTabsController interface {
	ActiveWorkspaceFolder(context.Context, memory.WorkspaceID) (string, error)
}

// tabContentSecurityPolicy seals every response from a tab: the document is
// sandboxed with scripts but without same-origin, so it has an opaque
// origin and cannot use Evie's routes or storage; it loads scripts, styles,
// images and fonts only from its own folder or inline; it opens no network
// connections, frames, workers, forms or links out; and only Evie's own
// page may frame it.
const tabContentSecurityPolicy = "sandbox allow-scripts; default-src 'none'; " +
	"script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self' data:; media-src 'self'; " +
	"connect-src 'none'; frame-src 'none'; worker-src 'none'; form-action 'none'; " +
	"base-uri 'none'; frame-ancestors 'self'"

func (s *Server) registerWorkspaceTabRoutes(mux *http.ServeMux) {
	controller, ok := s.contextSessions.(workspaceTabsController)
	if !ok {
		return
	}
	mux.Handle("/api/workspaces/tabs", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspaceId"`
		}
		if status, err := decodeManagementJSON(w, r, &request); err != nil || strings.TrimSpace(string(request.WorkspaceID)) == "" {
			if err == nil {
				status = http.StatusBadRequest
			}
			jsonError(w, status, "body must be one JSON object with a nonblank workspaceId field")
			return
		}
		folder, err := controller.ActiveWorkspaceFolder(r.Context(), request.WorkspaceID)
		if errors.Is(err, eviedb.ErrWorkspaceNotFound) {
			managementJSONError(w, http.StatusNotFound, "workspace_not_found", "Workspace not found; refresh and try again")
			return
		}
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "workspace_tabs_failed", "the Workspace's tabs could not be read")
			return
		}
		tabs, err := workspacetabs.List(folder)
		if err != nil {
			managementJSONError(w, http.StatusUnprocessableEntity, "workspace_tabs_unreadable", "the Workspace folder's .evie/tabs cannot be read")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Tabs []workspacetabs.Tab `json:"tabs"`
		}{tabs})
	}))
	// A tab's relative links need the trailing slash.
	mux.HandleFunc("GET /workspace-tabs/{workspace}/{tab}", func(w http.ResponseWriter, r *http.Request) {
		sealTabResponse(w)
		http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
	})
	mux.HandleFunc("GET /workspace-tabs/{workspace}/{tab}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		sealTabResponse(w)
		if !loopbackRequestHost(r) {
			http.Error(w, "unexpected host", http.StatusForbidden)
			return
		}
		folder, err := controller.ActiveWorkspaceFolder(r.Context(), memory.WorkspaceID(r.PathValue("workspace")))
		if err != nil {
			http.Error(w, "This Workspace is not available.", http.StatusNotFound)
			return
		}
		rel := r.PathValue("path")
		file, info, err := workspacetabs.Open(folder, r.PathValue("tab"), rel)
		switch {
		case errors.Is(err, workspacetabs.ErrInvalid):
			http.Error(w, "This tab has a problem: "+strings.TrimPrefix(err.Error(), workspacetabs.ErrInvalid.Error()+": "), http.StatusNotFound)
			return
		case errors.Is(err, workspacetabs.ErrNotFound), errors.Is(err, workspacetabs.ErrPath):
			http.Error(w, "This file is not part of the tab.", http.StatusNotFound)
			return
		case err != nil:
			http.Error(w, "The tab could not be read.", http.StatusInternalServerError)
			return
		}
		defer file.Close()
		if rel == "" {
			rel = "index.html"
		}
		w.Header().Set("Content-Type", tabContentType(rel))
		http.ServeContent(w, r, "", info.ModTime(), file)
	})
}

// sealTabResponse sets the headers every tab response carries, errors
// included.
func sealTabResponse(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Content-Security-Policy", tabContentSecurityPolicy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "no-store")
	header.Set("Referrer-Policy", "no-referrer")
}

// tabContentType is the type a tab file is served as; anything unknown is
// opaque bytes, which nosniff keeps from running.
func tabContentType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".ico":
		return "image/x-icon"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	default:
		return "application/octet-stream"
	}
}
