package web

import (
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
)

// chatSkill is one skill the owner can invoke in the open chat with /name,
// as the composer lists it (skills.spec.md, Owner invocation).
type chatSkill struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ArgumentHint string `json:"argumentHint,omitempty"`
	OwnerOnly    bool   `json:"ownerOnly"`
}

// handleSkillList lists the open chat's pinned skills, owner-only ones
// included, and whether the chat can use skills at all: a chat without the
// skill capability lists none and is not available, so the composer sends
// its slash text as an ordinary message.
func (s *Server) handleSkillList(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID memory.SessionID `json:"sessionId"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must identify a session")
		return
	}
	s.sessionMu.RLock()
	var session *agent.Session
	if chat := s.chatReadLocked(request.SessionID); chat != nil {
		session = chat.agent
	}
	s.sessionMu.RUnlock()
	if session == nil {
		managementJSONError(w, http.StatusConflict, "skills_session_changed", "Chat changed; refresh and try again")
		return
	}
	pinned, err := session.Skills(r.Context())
	if err != nil {
		managementJSONError(w, http.StatusInternalServerError, "skills_unavailable", "This chat's skills could not be loaded.")
		return
	}
	skills := make([]chatSkill, 0, len(pinned))
	for _, p := range pinned {
		skills = append(skills, chatSkill{Name: p.Name, Description: p.Metadata.Description, ArgumentHint: p.Metadata.ArgumentHint, OwnerOnly: !p.ModelInvocable()})
	}
	writeJSON(w, http.StatusOK, struct {
		Available bool        `json:"available"`
		Skills    []chatSkill `json:"skills"`
	}{session.CanUseSkills(), skills})
}

// skillRefusal is why a chat refuses to run a message with skill, given
// what resolving it returned, or "".
func skillRefusal(err error, skill string) (int, string) {
	switch {
	case err == nil:
		return 0, ""
	case errors.Is(err, agent.ErrNoSkills):
		return http.StatusUnprocessableEntity, "This chat can't use skills."
	case errors.Is(err, agent.ErrSkillNotPinned):
		return http.StatusUnprocessableEntity, "This chat has no skill /" + skill + "."
	}
	return http.StatusInternalServerError, "This chat's skills could not be loaded."
}
