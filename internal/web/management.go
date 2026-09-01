package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

const maxManagementBodyBytes = 4096

func (s *Server) handlePluginList(w http.ResponseWriter, r *http.Request) {
	if status, err := decodeManagementEmptyObject(w, r); err != nil {
		jsonError(w, status, "body must be one empty JSON object")
		return
	}
	inspection, err := s.manager.InspectContext(r.Context())
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "plugin enabled configuration is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, inspection)
}

func (s *Server) handlePluginLifecycle(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID      plugins.PluginID `json:"id"`
		Enabled *bool            `json:"enabled"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must be one JSON object with id and enabled fields")
		return
	}
	if request.ID == "" || request.Enabled == nil {
		jsonError(w, http.StatusBadRequest, "body must be one JSON object with id and enabled fields")
		return
	}
	transition, err := s.manager.SetEnabledWithTransition(r.Context(), request.ID, *request.Enabled)
	if err != nil {
		if errors.Is(err, plugins.ErrEnabledStateUnavailable) {
			jsonError(w, http.StatusServiceUnavailable, "plugin enabled configuration is unavailable")
			return
		}
		jsonError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, transition)
}

func (s *Server) handlePresetList(w http.ResponseWriter, r *http.Request) {
	if status, err := decodeManagementEmptyObject(w, r); err != nil {
		jsonError(w, status, "body must be one empty JSON object")
		return
	}
	presets, err := s.manager.InspectPresetsContext(r.Context())
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "plugin enabled configuration is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": presets})
}

func (s *Server) handlePresetValidate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID plugins.PresetID `json:"id"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must be one JSON object with an id field")
		return
	}
	if request.ID == "" {
		jsonError(w, http.StatusBadRequest, "body must be one JSON object with an id field")
		return
	}
	report, err := s.manager.ValidatePresetContext(r.Context(), request.ID)
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "plugin enabled configuration is unavailable")
		return
	}
	status := http.StatusOK
	if !report.Valid {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, report)
}

func (s *Server) handleSessionInspect(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID memory.SessionID `json:"sessionId"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must be one JSON object with a sessionId field")
		return
	}
	if strings.TrimSpace(string(request.SessionID)) == "" {
		managementJSONError(w, http.StatusBadRequest, "invalid_session_id", "session ID is required")
		return
	}
	receipt, err := s.receipts.GetCompositionReceipt(r.Context(), request.SessionID)
	if err != nil {
		if errors.Is(err, eviedb.ErrCompositionReceiptNotFound) {
			managementJSONError(w, http.StatusNotFound, "session_not_found", "session was not found")
			return
		}
		managementJSONError(w, http.StatusInternalServerError, "session_inspection_unavailable", "session inspection is unavailable")
		return
	}
	resolutions, err := s.receipts.GetCompatibilityResolutions(r.Context(), request.SessionID)
	if err != nil {
		if errors.Is(err, eviedb.ErrCompositionReceiptNotFound) {
			managementJSONError(w, http.StatusNotFound, "session_not_found", "session was not found")
			return
		}
		managementJSONError(w, http.StatusInternalServerError, "session_inspection_unavailable", "session inspection is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		SessionID                memory.SessionID                  `json:"sessionId"`
		Receipt                  plugins.CompositionReceipt        `json:"receipt"`
		CompatibilityResolutions []plugins.CompatibilityResolution `json:"compatibilityResolutions"`
	}{request.SessionID, receipt, resolutions})
}

func managementJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}{code, message})
}

func decodeManagementEmptyObject(w http.ResponseWriter, r *http.Request) (int, error) {
	var object map[string]json.RawMessage
	status, err := decodeManagementJSON(w, r, &object)
	if err != nil {
		return status, err
	}
	if object == nil || len(object) != 0 {
		return http.StatusBadRequest, errors.New("body must be an empty JSON object")
	}
	return http.StatusOK, nil
}

func decodeManagementJSON(w http.ResponseWriter, r *http.Request, destination any) (int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxManagementBodyBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, errors.New("body must contain exactly one JSON value")
	}
	return http.StatusOK, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
