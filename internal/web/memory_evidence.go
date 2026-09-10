package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/davidadel66/evie/internal/memory"
)

// MemoryEvidenceInspector resolves the exact references retained with a
// request, applying the caller's current access before returning source text.
type MemoryEvidenceInspector interface {
	InspectMemoryEvidence(context.Context, memory.ScopeContext, []memory.RetrievalReference) ([]memory.RetrievalInspection, error)
}

func (s *Server) handleMemoryEvidence(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request struct {
		SessionID  memory.SessionID `json:"sessionId"`
		SnapshotID memory.EventID   `json:"snapshotId"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil || request.SessionID == "" || request.SnapshotID == "" {
		if status == 0 {
			status = http.StatusBadRequest
		}
		managementJSONError(w, status, "invalid_memory_evidence", "A selected conversation and request reference are required.")
		return
	}
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	if s.selectingSession || s.activeSession.ID != request.SessionID || s.session == nil {
		managementJSONError(w, http.StatusConflict, "memory_session_changed", "The selected conversation changed. Open its sources again.")
		return
	}
	events, err := s.session.HistoryEvents(r.Context())
	if err != nil {
		managementJSONError(w, http.StatusConflict, "memory_evidence_unavailable", "The original request could not be inspected.")
		return
	}
	for _, event := range events {
		if event.ID != request.SnapshotID || event.Type != memory.EventContextSnapshot || event.SessionID != request.SessionID {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if json.Unmarshal(event.Payload, &snapshot) != nil || snapshot.Memory == nil {
			break
		}
		inspected, err := s.memoryEvidence.InspectMemoryEvidence(r.Context(), s.activeSession.ScopeContext(), snapshot.Memory.Evidence)
		if err != nil {
			managementJSONError(w, http.StatusUnprocessableEntity, "memory_evidence_unavailable", "The original evidence is unavailable under current access.")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			SessionID  memory.SessionID             `json:"sessionId"`
			SnapshotID memory.EventID               `json:"snapshotId"`
			Version    string                       `json:"version"`
			Status     string                       `json:"status"`
			Evidence   []memory.RetrievalInspection `json:"evidence"`
		}{request.SessionID, request.SnapshotID, snapshot.Memory.Version, snapshot.Memory.Status, inspected})
		return
	}
	managementJSONError(w, http.StatusNotFound, "memory_evidence_unavailable", "No original evidence reference is available for this request.")
}

type memoryActivity struct {
	SnapshotID    memory.EventID `json:"snapshotId"`
	Status        string         `json:"status"`
	AcceptedCount int            `json:"acceptedCount"`
	ExcerptCount  int            `json:"excerptCount"`
}

func projectMemoryActivity(event memory.Event) (*memoryActivity, error) {
	var snapshot memory.ContextSnapshotPayload
	if len(event.Payload) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Memory == nil {
		return nil, nil
	}
	activity := &memoryActivity{SnapshotID: event.ID, Status: snapshot.Memory.Status}
	for _, ref := range snapshot.Memory.Evidence {
		switch ref.Kind {
		case memory.RetrievalAcceptedMemory:
			activity.AcceptedCount++
		case memory.RetrievalConversationExcerpt:
			activity.ExcerptCount++
		}
	}
	return activity, nil
}

func (e *sseEvents) MemoryRetrieved(event memory.Event) {
	activity, err := projectMemoryActivity(event)
	if err != nil {
		e.Error("Memory activity could not be displayed. Reload the conversation.")
		return
	}
	if activity != nil {
		e.emit("memory_activity", activity)
	}
}
