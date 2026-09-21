package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/localterminal"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

type terminalRequest struct {
	WorkspaceID memory.WorkspaceID `json:"workspaceId"`
	Revision    int64              `json:"revision"`
	ID          string             `json:"id"`
	Data        string             `json:"data"`
	Columns     int                `json:"columns"`
	Rows        int                `json:"rows"`
}
type terminalEntry struct {
	workspace memory.WorkspaceID
	revision  int64
	process   *localterminal.Session
	cancel    context.CancelFunc
}
type terminalEvent struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Data []byte `json:"data,omitempty"`
	Code int    `json:"code,omitempty"`
}

func (s *Server) registerTerminalRoutes(mux *http.ServeMux) {
	controller, ok := s.contextSessions.(WorkspaceFolderController)
	if !ok {
		return
	}
	for _, action := range []string{"open", "input", "resize", "close"} {
		mux.Handle("/api/terminal/"+action, s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
			var req terminalRequest
			if status, err := decodeManagementJSON(w, r, &req); err != nil {
				jsonError(w, status, "Invalid terminal request")
				return
			}
			if req.WorkspaceID == "" {
				jsonError(w, 400, "Choose a Workspace folder first")
				return
			}
			if (action == "open" || action == "resize") && !localterminal.ValidSize(req.Columns, req.Rows) {
				jsonError(w, 400, "Invalid terminal dimensions")
				return
			}
			if action == "open" {
				s.openTerminal(w, r, controller, req)
				return
			}
			s.sessionMu.RLock()
			defer s.sessionMu.RUnlock()
			folder, err := controller.WorkspaceFolder(r.Context(), req.WorkspaceID)
			if err != nil || folder.Revision != req.Revision || folder.Path == "" {
				jsonError(w, 409, "Workspace folder changed; reopen the terminal")
				return
			}
			s.terminalMu.Lock()
			entry := s.terminals[req.ID]
			s.terminalMu.Unlock()
			if entry == nil || entry.workspace != req.WorkspaceID || entry.revision != req.Revision {
				jsonError(w, 404, "Terminal is no longer connected")
				return
			}
			switch action {
			case "input":
				err = entry.process.Write(req.Data)
			case "resize":
				err = entry.process.Resize(req.Columns, req.Rows)
			case "close":
				entry.cancel()
			}
			if err != nil {
				entry.cancel()
				jsonError(w, 409, "Terminal input could not be delivered; reopen the terminal")
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
		}))
	}
}

func (s *Server) openTerminal(w http.ResponseWriter, r *http.Request, controller WorkspaceFolderController, req terminalRequest) {
	// Validate and register under the same lock used for folder replacement.
	// Streaming never holds this lock or prevents another chat from starting.
	s.sessionMu.RLock()
	folder, err := controller.WorkspaceFolder(r.Context(), req.WorkspaceID)
	if err != nil || folder.Path == "" || folder.Revision != req.Revision {
		s.sessionMu.RUnlock()
		jsonError(w, 409, "Workspace folder changed or is not attached")
		return
	}
	s.terminalMu.Lock()
	if s.terminalsClosed || len(s.terminals) >= 8 {
		s.terminalMu.Unlock()
		s.sessionMu.RUnlock()
		jsonError(w, 409, "Close an existing terminal before opening another")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	process, err := localterminal.Start(ctx, folder.Path, req.Columns, req.Rows)
	if err != nil {
		s.terminalMu.Unlock()
		s.sessionMu.RUnlock()
		cancel()
		jsonError(w, 422, err.Error())
		return
	}
	id := uuid.NewString()
	s.terminals[id] = &terminalEntry{workspace: req.WorkspaceID, revision: req.Revision, process: process, cancel: cancel}
	s.terminalMu.Unlock()
	s.sessionMu.RUnlock()
	defer func() { cancel(); process.Close(); s.terminalMu.Lock(); delete(s.terminals, id); s.terminalMu.Unlock() }()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writer := http.NewResponseController(w)
	write := func(event terminalEvent) bool {
		// A stalled browser must not keep a shell/output goroutine indefinitely.
		if err := writer.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return false
		}
		if err := json.NewEncoder(w).Encode(event); err != nil {
			return false
		}
		return writer.Flush() == nil
	}
	if !write(terminalEvent{Type: "ready", ID: id}) {
		return
	}
	output := make(chan []byte, 8)
	go func() {
		defer close(output)
		for {
			data := make([]byte, 8192)
			n, err := process.Read(data)
			if n > 0 {
				select {
				case output <- data[:n]:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	exited := process.Exited()
	var drain <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-exited:
			exited = nil
			timer = time.NewTimer(150 * time.Millisecond)
			drain = timer.C
		case <-drain:
			drain = nil
			process.Close()
		case data, ok := <-output:
			if !ok {
				process.Close()
				write(terminalEvent{Type: "exit", Code: process.ExitCode()})
				return
			}
			if !write(terminalEvent{Type: "output", Data: data}) {
				return
			}
		}
	}
}

// Called only after a successful folder revision update. Cancellation is
// immediate; bounded process teardown happens outside the selection lock.
func (s *Server) stopWorkspaceTerminals(id memory.WorkspaceID) {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	for _, entry := range s.terminals {
		if entry.workspace == id {
			entry.cancel()
		}
	}
}

// Close drains native pickers and terminals owned by direct Handler users as
// well as production listeners.
func (s *Server) Close() {
	s.closeFolderPicker()
	s.terminalMu.Lock()
	s.terminalsClosed = true
	entries := make([]*terminalEntry, 0, len(s.terminals))
	for _, entry := range s.terminals {
		entry.cancel()
		entries = append(entries, entry)
	}
	s.terminalMu.Unlock()
	var cleanup sync.WaitGroup
	for _, entry := range entries {
		cleanup.Go(entry.process.Close)
	}
	cleanup.Wait()
}
