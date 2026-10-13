package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path"
	"regexp"
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

// workspaceIDPattern bounds a Workspace ID in a tab URL before it is used
// in a redirect or a policy.
var workspaceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// tabContentSecurityPolicy seals a tab's page: the document is sandboxed with
// scripts but without same-origin, so it has an opaque origin and cannot use
// Evie's routes or storage; it loads scripts, styles, images and fonts only
// from its own folder (folder, an absolute URL ending in "/") or inline; it
// opens no network connections, frames, workers, forms or links out; and
// only Evie's own page may frame it. Evie's page adds frame-src 'self', so
// the tab cannot navigate its frame away either.
func tabContentSecurityPolicy(folder string) string {
	return "sandbox allow-scripts; default-src 'none'; " +
		"script-src " + folder + " 'unsafe-inline'; style-src " + folder + " 'unsafe-inline'; " +
		"img-src " + folder + " data:; font-src " + folder + " data:; media-src " + folder + "; " +
		"connect-src 'none'; frame-src 'none'; worker-src 'none'; form-action 'none'; " +
		"base-uri 'none'; frame-ancestors 'self'"
}

// tabErrorPolicy seals an error page: no scripts or anything else.
const tabErrorPolicy = "sandbox; default-src 'none'; frame-ancestors 'self'"

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
		if errors.Is(err, workspacetabs.ErrTooMany) {
			managementJSONError(w, http.StatusUnprocessableEntity, "workspace_tabs_unreadable", "the Workspace folder's "+err.Error())
			return
		}
		if err != nil {
			managementJSONError(w, http.StatusUnprocessableEntity, "workspace_tabs_unreadable", "the Workspace folder's .evie/tabs cannot be read")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Tabs []workspacetabs.Tab `json:"tabs"`
		}{tabs})
	}))
	// A tab's relative links need the trailing slash. The Location is built
	// only from checked IDs, so it can never point off Evie.
	mux.HandleFunc("GET /workspace-tabs/{workspace}/{tab}", func(w http.ResponseWriter, r *http.Request) {
		sealTabResponse(w, tabErrorPolicy)
		workspace, tab := r.PathValue("workspace"), r.PathValue("tab")
		if !workspaceIDPattern.MatchString(workspace) || !workspacetabs.ValidID(tab) {
			http.Error(w, "This file is not part of the tab.", http.StatusNotFound)
			return
		}
		w.Header().Set("Location", "/workspace-tabs/"+workspace+"/"+tab+"/")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("GET /workspace-tabs/{workspace}/{tab}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		sealTabResponse(w, tabErrorPolicy)
		origin, ok := tabOrigin(r)
		if !ok {
			http.Error(w, "unexpected host", http.StatusForbidden)
			return
		}
		workspace, tab := r.PathValue("workspace"), r.PathValue("tab")
		if !workspaceIDPattern.MatchString(workspace) || !workspacetabs.ValidID(tab) {
			http.Error(w, "This file is not part of the tab.", http.StatusNotFound)
			return
		}
		folder, err := controller.ActiveWorkspaceFolder(r.Context(), memory.WorkspaceID(workspace))
		if err != nil {
			http.Error(w, "This Workspace is not available.", http.StatusNotFound)
			return
		}
		rel := r.PathValue("path")
		file, info, err := workspacetabs.Open(folder, tab, rel)
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
		w.Header().Set("Content-Security-Policy", tabContentSecurityPolicy(origin+"/workspace-tabs/"+workspace+"/"+tab+"/"))
		w.Header().Set("Content-Type", tabContentType(rel))
		http.ServeContent(w, r, "", info.ModTime(), file)
	})
}

// sealTabResponse sets the headers every tab response carries, errors
// included; a served file replaces the policy with its folder's.
func sealTabResponse(w http.ResponseWriter, policy string) {
	header := w.Header()
	header.Set("Content-Security-Policy", policy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "no-store")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-DNS-Prefetch-Control", "off")
}

// tabOrigin is "http://<host>[:<port>]" for a request whose Host is this
// machine with a numeric port, so it is safe to write into a policy.
func tabOrigin(r *http.Request) (string, bool) {
	if r.TLS != nil {
		return "", false
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, port = r.Host, ""
	}
	if !isLoopbackHost(host) || len(port) > 5 || strings.Trim(port, "0123456789") != "" {
		return "", false
	}
	if port == "" {
		if strings.Contains(host, ":") {
			return "http://[" + host + "]", true
		}
		return "http://" + host, true
	}
	return "http://" + net.JoinHostPort(host, port), true
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
