package web

import (
	"net/http"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// Chat status (multi-chat.spec.md Stage 4): which chats run a turn, which
// wait on an approval, and when each chat's latest turn finished, streamed to
// the sidebar whenever any of it changes. Following it counts as presence.

type chatStatus struct {
	SessionID memory.SessionID `json:"sessionId"`
	Running   bool             `json:"running,omitempty"`
	// Waiting counts approvals the chat's turn is waiting on.
	Waiting int `json:"waiting,omitempty"`
	// FinishedAt is when the chat's latest turn in this server ended, in
	// Unix milliseconds.
	FinishedAt int64 `json:"finishedAt,omitempty"`
}

// statusChanged wakes every status follower. It never blocks or takes a
// lock other followers wait on, so it is safe under sessionMu or mu.
func (s *Server) statusChanged() {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	for wake := range s.statusFollowers {
		select {
		case wake <- struct{}{}:
		default: // already woken; it reads the latest status anyway
		}
	}
}

// chatStatuses reads the current status of every chat that has one.
func (s *Server) chatStatuses() []chatStatus {
	byID := map[memory.SessionID]*chatStatus{}
	entry := func(id memory.SessionID) *chatStatus {
		if byID[id] == nil {
			byID[id] = &chatStatus{SessionID: id}
		}
		return byID[id]
	}
	s.sessionMu.RLock()
	for id := range s.turns {
		entry(id).Running = true
	}
	for id, at := range s.finishedAt {
		entry(id).FinishedAt = at
	}
	s.sessionMu.RUnlock()
	s.mu.Lock()
	for _, id := range s.pendingChat {
		entry(id).Waiting++
	}
	s.mu.Unlock()
	list := make([]chatStatus, 0, len(byID))
	for _, status := range byID {
		if status.SessionID != "" {
			list = append(list, *status)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].SessionID < list[j].SessionID })
	return list
}

// handleChatStatus streams chat_status events: the current status, then the
// new status after every change, with heartbeats between.
func (s *Server) handleChatStatus(w http.ResponseWriter, r *http.Request) {
	var req struct{}
	if status, err := decodeManagementJSON(w, r, &req); err != nil {
		jsonError(w, status, "body must be an empty JSON object")
		return
	}
	stream, err := newSSEEvents(w)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	wake := make(chan struct{}, 1)
	s.statusMu.Lock()
	s.statusFollowers[wake] = struct{}{}
	s.statusMu.Unlock()
	defer func() {
		s.statusMu.Lock()
		delete(s.statusFollowers, wake)
		s.statusMu.Unlock()
	}()
	s.arrive()
	defer s.leave()
	send := func() {
		stream.emit("chat_status", struct {
			Chats []chatStatus `json:"chats"`
		}{s.chatStatuses()})
	}
	send()
	heartbeat := time.NewTicker(feedHeartbeat)
	defer heartbeat.Stop()
	for stream.err == nil {
		select {
		case <-r.Context().Done():
			return
		case <-wake:
			send()
		case <-heartbeat.C:
			stream.comment("ping")
		}
	}
}
