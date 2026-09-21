package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

type workspaceFolderChoice struct {
	Path      string `json:"path"`
	Cancelled bool   `json:"cancelled"`
}

func (s *Server) handleWorkspaceChooseFolder(w http.ResponseWriter, r *http.Request) {
	// This dialog appears on the server's desktop, so only its local owner may
	// launch it. The regular management guard also checks Host, Origin and JSON.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		managementJSONError(w, http.StatusForbidden, "folder_picker_local_only", "Choose a folder from Evie on this computer")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host != r.Host || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			managementJSONError(w, http.StatusForbidden, "folder_picker_origin_rejected", "Open the folder chooser from this Evie window")
			return
		}
	}
	if status, err := decodeManagementEmptyObject(w, r); err != nil {
		jsonError(w, status, "body must be one empty JSON object")
		return
	}
	s.folderPickerMu.Lock()
	if s.folderPickerClosed {
		s.folderPickerMu.Unlock()
		managementJSONError(w, http.StatusServiceUnavailable, "folder_picker_unavailable", "The folder chooser is unavailable")
		return
	}
	if s.folderPickerCancel != nil {
		s.folderPickerMu.Unlock()
		managementJSONError(w, http.StatusConflict, "folder_picker_busy", "Finish choosing a folder in the open dialog first")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	done := make(chan struct{})
	s.folderPickerCancel, s.folderPickerDone = cancel, done
	picker := s.folderPicker
	if picker == nil {
		picker = chooseNativeWorkspaceFolder
	}
	s.folderPickerMu.Unlock()
	defer func() {
		cancel()
		s.folderPickerMu.Lock()
		s.folderPickerCancel, s.folderPickerDone = nil, nil
		close(done)
		s.folderPickerMu.Unlock()
	}()
	choice, err := picker(ctx)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		managementJSONError(w, http.StatusGatewayTimeout, "folder_picker_timeout", "The folder chooser timed out; try again")
		return
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		writeJSON(w, http.StatusOK, workspaceFolderChoice{Cancelled: true})
		return
	}
	if err != nil {
		message := "The folder chooser could not be opened; try again"
		if errors.Is(err, errNativeFolderPickerUnsupported) {
			message = "The native folder chooser is available on macOS"
		}
		managementJSONError(w, http.StatusServiceUnavailable, "folder_picker_unavailable", message)
		return
	}
	writeJSON(w, http.StatusOK, choice)
}

func (s *Server) closeFolderPicker() {
	s.folderPickerMu.Lock()
	s.folderPickerClosed = true
	cancel, done := s.folderPickerCancel, s.folderPickerDone
	s.folderPickerMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
