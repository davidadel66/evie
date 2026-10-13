package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workspacetabs"
)

// tabHelper is Evie's helper script, served to every tab at _evie/tab.js.
//
//go:embed tab_helper.js
var tabHelper []byte

// workspaceTabsController finds the folder whose .evie/tabs a Workspace
// shows (workspace-tabs.spec.md). A missing or archived Workspace is
// eviedb.ErrWorkspaceNotFound; one without a folder is "".
type workspaceTabsController interface {
	ActiveWorkspaceFolder(context.Context, memory.WorkspaceID) (string, error)
}

// workspaceTabDataController keeps the owner's approvals of what each tab
// may read and reads registered databases for them (story 2).
type workspaceTabDataController interface {
	WorkspaceTabDataApprovals(context.Context, memory.WorkspaceID) (map[string]string, error)
	ApproveWorkspaceTabData(context.Context, memory.WorkspaceID, string, string) error
	WithdrawWorkspaceTabData(context.Context, memory.WorkspaceID, string) error
	ReadTabDatabase(ctx context.Context, db, query string) (any, error)
}

// workspaceTabProblemsController keeps what went wrong in each tab (story
// 3) so Evie and the owner can see it.
type workspaceTabProblemsController interface {
	RecordWorkspaceTabProblem(context.Context, memory.WorkspaceID, workspacetabs.Problem) error
	WorkspaceTabProblems(context.Context, memory.WorkspaceID) ([]workspacetabs.Problem, error)
	ClearWorkspaceTabProblems(context.Context, memory.WorkspaceID, string) error
}

// tabListing is a tab as the owner manages it: Approved when its current
// data list is the one the owner approved, and its latest problems.
type tabListing struct {
	workspacetabs.Tab
	Approved bool `json:"approved,omitempty"`
	// ApprovalStored is any approval on record, so the owner can withdraw
	// one even while the tab has a problem.
	ApprovalStored bool                    `json:"approvalStored,omitempty"`
	Recent         []workspacetabs.Problem `json:"recentProblems,omitempty"`
}

// tabProblemKinds are the problems a tab's page reports.
var tabProblemKinds = map[string]bool{"error": true, "blocked": true}

// maxTabQueryBytes bounds one SQL text a tab sends, within the data route's
// own body limit; maxTabRunsBytes bounds the runs a tab reads; and
// maxTabReads is how many tab reads run at once across Evie.
const (
	maxTabQueryBytes   = 16 << 10
	maxTabRequestBytes = 32 << 10
	maxTabRunsBytes    = 1 << 20
	maxTabReads        = 4
)

// tabRunReader lists a Workspace's runs as a tab may see them: scrubbed of
// sensitive inputs as a chat sees them, without inputs.
type tabRunReader interface {
	TabRuns(context.Context, memory.WorkspaceID, int) ([]eviedb.WorkflowRun, error)
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
		approvals := map[string]string{}
		if data, ok := controller.(workspaceTabDataController); ok {
			if approvals, err = data.WorkspaceTabDataApprovals(r.Context(), request.WorkspaceID); err != nil {
				managementJSONError(w, http.StatusInternalServerError, "workspace_tabs_failed", "the Workspace's tab approvals could not be read")
				return
			}
			// A tab folder that is gone takes its approval with it, so a tab
			// made later under the same name starts unapproved.
			for id := range approvals {
				if !slices.ContainsFunc(tabs, func(tab workspacetabs.Tab) bool { return tab.ID == id }) {
					if err := data.WithdrawWorkspaceTabData(r.Context(), request.WorkspaceID, id); err == nil {
						delete(approvals, id)
					}
				}
			}
		}
		recent := map[string][]workspacetabs.Problem{}
		if problems, ok := controller.(workspaceTabProblemsController); ok {
			found, err := problems.WorkspaceTabProblems(r.Context(), request.WorkspaceID)
			if err != nil {
				managementJSONError(w, http.StatusInternalServerError, "workspace_tabs_failed", "the Workspace's tab problems could not be read")
				return
			}
			for _, problem := range found {
				if len(recent[problem.TabID]) < 3 {
					recent[problem.TabID] = append(recent[problem.TabID], problem)
				}
			}
		}
		listings := make([]tabListing, len(tabs))
		for i, tab := range tabs {
			_, stored := approvals[tab.ID]
			listings[i] = tabListing{Tab: tab, Approved: tab.Problem == "" && tab.DataKey != "" && approvals[tab.ID] == workspacetabs.ApprovalKey(folder, tab.DataKey),
				ApprovalStored: stored, Recent: recent[tab.ID]}
		}
		writeJSON(w, http.StatusOK, struct {
			Tabs []tabListing `json:"tabs"`
		}{listings})
	}))
	if data, ok := controller.(workspaceTabDataController); ok {
		s.registerWorkspaceTabDataRoutes(mux, controller, data)
	}
	if problems, ok := controller.(workspaceTabProblemsController); ok {
		s.registerWorkspaceTabProblemRoutes(mux, problems)
	}
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
		if rel == workspacetabs.Reserved+"/tab.js" {
			w.Header().Set("Content-Security-Policy", tabContentSecurityPolicy(origin+"/workspace-tabs/"+workspace+"/"+tab+"/"))
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write(tabHelper)
			return
		}
		if rel == workspacetabs.Reserved+"/evie.css" {
			css, err := appStylesheet()
			if err != nil {
				http.Error(w, "Evie's stylesheet is not built.", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Security-Policy", tabContentSecurityPolicy(origin+"/workspace-tabs/"+workspace+"/"+tab+"/"))
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			_, _ = w.Write(css)
			return
		}
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

// appStylesheet is the app's own built stylesheet: its theme tokens and
// utility classes, served to tabs at _evie/evie.css so they look like Evie
// (story 5). It references no fonts or other files.
var appStylesheet = sync.OnceValues(func() ([]byte, error) {
	matches, err := fs.Glob(distFS, "ui/dist/assets/index-*.css")
	if err != nil || len(matches) == 0 {
		return nil, errors.New("no built stylesheet")
	}
	return fs.ReadFile(distFS, matches[0])
})

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

// tabRequest is the body of the approve, withdraw and data routes.
type tabRequest struct {
	WorkspaceID memory.WorkspaceID `json:"workspaceId"`
	TabID       string             `json:"tabId"`
	DataKey     string             `json:"dataKey,omitempty"`
	Request     *tabDataRequest    `json:"request,omitempty"`
}

// tabDataRequest is one read a tab asks for through Evie's page.
type tabDataRequest struct {
	Kind string `json:"kind"`
	DB   string `json:"db,omitempty"`
	SQL  string `json:"sql,omitempty"`
	Path string `json:"path,omitempty"`
}

// tabRun is a run as a tab reads it.
type tabRun struct {
	ID         string          `json:"id"`
	Workflow   string          `json:"workflow"`
	State      string          `json:"state"`
	CreatedAt  time.Time       `json:"created_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Outputs    json.RawMessage `json:"outputs,omitempty"`
	Error      string          `json:"error,omitempty"`
}

func (s *Server) registerWorkspaceTabDataRoutes(mux *http.ServeMux, folders workspaceTabsController, data workspaceTabDataController) {
	// shownTab reads the request and the named tab as it is now; it has
	// answered the request when ok is false.
	shownTab := func(w http.ResponseWriter, r *http.Request) (request tabRequest, folder string, tab workspacetabs.Tab, ok bool) {
		if status, err := decodeManagementJSONUpTo(w, r, &request, maxTabRequestBytes); err != nil || !workspaceIDPattern.MatchString(string(request.WorkspaceID)) || !workspacetabs.ValidID(request.TabID) {
			if err == nil {
				status = http.StatusBadRequest
			}
			jsonError(w, status, "body must name a Workspace and one of its tabs")
			return request, "", tab, false
		}
		folder, err := folders.ActiveWorkspaceFolder(r.Context(), request.WorkspaceID)
		if err != nil {
			managementJSONError(w, http.StatusNotFound, "workspace_not_found", "Workspace not found; refresh and try again")
			return request, "", tab, false
		}
		tabs, err := workspacetabs.List(folder)
		if err != nil {
			managementJSONError(w, http.StatusUnprocessableEntity, "workspace_tabs_unreadable", "the Workspace folder's .evie/tabs cannot be read")
			return request, "", tab, false
		}
		for _, candidate := range tabs {
			if candidate.ID == request.TabID && candidate.Problem == "" {
				return request, folder, candidate, true
			}
		}
		managementJSONError(w, http.StatusNotFound, "workspace_tab_not_found", "This tab is not shown; Settings → Tabs says why")
		return request, "", tab, false
	}
	mux.Handle("/api/workspaces/tabs/approve", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		request, folder, tab, ok := shownTab(w, r)
		if !ok {
			return
		}
		if tab.DataKey == "" || request.DataKey != tab.DataKey {
			managementJSONError(w, http.StatusConflict, "workspace_tab_data_changed", "The tab's data list changed; review it again before allowing it")
			return
		}
		if err := data.ApproveWorkspaceTabData(r.Context(), request.WorkspaceID, tab.ID, workspacetabs.ApprovalKey(folder, tab.DataKey)); err != nil {
			managementJSONError(w, http.StatusInternalServerError, "workspace_tab_approve_failed", "The tab's data could not be allowed")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Approved bool `json:"approved"`
		}{true})
	}))
	mux.Handle("/api/workspaces/tabs/withdraw", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request tabRequest
		if status, err := decodeManagementJSON(w, r, &request); err != nil || !workspaceIDPattern.MatchString(string(request.WorkspaceID)) || !workspacetabs.ValidID(request.TabID) {
			if err == nil {
				status = http.StatusBadRequest
			}
			jsonError(w, status, "body must name a Workspace and one of its tabs")
			return
		}
		if err := data.WithdrawWorkspaceTabData(r.Context(), request.WorkspaceID, request.TabID); err != nil {
			managementJSONError(w, http.StatusInternalServerError, "workspace_tab_withdraw_failed", "The tab's data could not be withdrawn")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Approved bool `json:"approved"`
		}{false})
	}))
	reads := make(chan struct{}, maxTabReads)
	mux.Handle("/api/workspaces/tabs/data", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		select {
		case reads <- struct{}{}:
			defer func() { <-reads }()
		default:
			managementJSONError(w, http.StatusTooManyRequests, "workspace_tab_reads_busy", "Evie is already running its most tab reads at once; try again shortly")
			return
		}
		request, folder, tab, ok := shownTab(w, r)
		if !ok {
			return
		}
		refuse := func(message string) {
			// A refusal is a problem Evie should see, to fix the tab.
			if problems, ok := folders.(workspaceTabProblemsController); ok {
				_ = problems.RecordWorkspaceTabProblem(r.Context(), request.WorkspaceID, workspacetabs.Problem{TabID: tab.ID, Kind: "refused", Message: message})
			}
			managementJSONError(w, http.StatusForbidden, "workspace_tab_data_refused", message)
		}
		if request.Request == nil {
			jsonError(w, http.StatusBadRequest, "body must hold the tab's request")
			return
		}
		approvals, err := data.WorkspaceTabDataApprovals(r.Context(), request.WorkspaceID)
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "workspace_tabs_failed", "the Workspace's tab approvals could not be read")
			return
		}
		if tab.DataKey == "" || approvals[tab.ID] != workspacetabs.ApprovalKey(folder, tab.DataKey) {
			refuse("This tab may read nothing until you allow its data in Settings → Tabs")
			return
		}
		ask := request.Request
		var result any
		switch ask.Kind {
		case "query":
			if !slices.ContainsFunc(tab.Data, func(source workspacetabs.DataSource) bool { return source.DB != "" && source.DB == ask.DB }) {
				refuse("tab.json does not declare the database " + quoteName(ask.DB))
				return
			}
			if len(ask.SQL) > maxTabQueryBytes {
				refuse("the query is over 16 KB")
				return
			}
			if result, err = data.ReadTabDatabase(r.Context(), ask.DB, ask.SQL); err != nil {
				managementJSONError(w, http.StatusUnprocessableEntity, "workspace_tab_query_failed", err.Error())
				return
			}
		case "file":
			var globs []string
			for _, source := range tab.Data {
				if source.Files != "" {
					globs = append(globs, source.Files)
				}
			}
			text, err := workspacetabs.ReadFile(folder, ask.Path, globs)
			if errors.Is(err, workspacetabs.ErrNotFound) {
				managementJSONError(w, http.StatusNotFound, "workspace_tab_file_missing", "No such file")
				return
			}
			if errors.Is(err, workspacetabs.ErrRefused) {
				refuse(strings.TrimPrefix(err.Error(), workspacetabs.ErrRefused.Error()+": "))
				return
			}
			if err != nil {
				managementJSONError(w, http.StatusInternalServerError, "workspace_tab_file_failed", "The file could not be read")
				return
			}
			result = string(text)
		case "runs":
			if !slices.ContainsFunc(tab.Data, func(source workspacetabs.DataSource) bool { return source.Runs }) {
				refuse("tab.json does not declare runs")
				return
			}
			reader, ok := s.workflows.(tabRunReader)
			if !ok {
				managementJSONError(w, http.StatusUnprocessableEntity, "workflows_unavailable", "Workflows are unavailable")
				return
			}
			runs, err := reader.TabRuns(r.Context(), request.WorkspaceID, 100)
			if err != nil {
				writeWorkflowError(w, err)
				return
			}
			rows, size := []tabRun{}, 0
			for _, run := range runs {
				if size += len(run.Outputs) + len(run.Error); size > maxTabRunsBytes {
					break
				}
				rows = append(rows, tabRun{ID: run.ID, Workflow: run.Workflow, State: string(run.State), CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt, Outputs: run.Outputs, Error: run.Error})
			}
			result = rows
		default:
			jsonError(w, http.StatusBadRequest, `the request kind is "query", "file" or "runs"`)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Result any `json:"result"`
		}{result})
	}))
}

func quoteName(name string) string {
	if len(name) > 40 {
		name = name[:40]
	}
	return `"` + name + `"`
}

// registerWorkspaceTabProblemRoutes lets Evie's page record what went wrong
// in a tab it shows, and clear a tab's problems.
func (s *Server) registerWorkspaceTabProblemRoutes(mux *http.ServeMux, problems workspaceTabProblemsController) {
	mux.Handle("/api/workspaces/tabs/problems", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspaceId"`
			TabID       string             `json:"tabId"`
			Kind        string             `json:"kind"`
			Message     string             `json:"message"`
			Clear       bool               `json:"clear,omitempty"`
		}
		if status, err := decodeManagementJSON(w, r, &request); err != nil || !workspaceIDPattern.MatchString(string(request.WorkspaceID)) || !workspacetabs.ValidID(request.TabID) ||
			(!request.Clear && (!tabProblemKinds[request.Kind] || strings.TrimSpace(request.Message) == "")) {
			if err == nil {
				status = http.StatusBadRequest
			}
			jsonError(w, status, `body must name a Workspace, one of its tabs, and a problem of kind "error" or "blocked", or "clear": true`)
			return
		}
		var err error
		if request.Clear {
			err = problems.ClearWorkspaceTabProblems(r.Context(), request.WorkspaceID, request.TabID)
		} else {
			err = problems.RecordWorkspaceTabProblem(r.Context(), request.WorkspaceID, workspacetabs.Problem{TabID: request.TabID, Kind: request.Kind, Message: request.Message})
		}
		if errors.Is(err, eviedb.ErrWorkspaceNotFound) {
			managementJSONError(w, http.StatusNotFound, "workspace_not_found", "Workspace not found; refresh and try again")
			return
		}
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "workspace_tab_problem_failed", "The tab's problem could not be saved")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
