package web

import (
	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
)

// Open chats (multi-chat.spec.md Stage 2). The server keeps up to
// maxOpenChats conversations open, each with its own agent session and model
// revision, so a turn in one never blocks another. s.session, s.activeSession
// and s.modelRevision mirror the focused chat, the one opened last, which only
// supplies defaults such as the memory view's scope; per-chat requests name
// their chat. All of it is guarded by sessionMu.

const maxOpenChats = 8

type openChat struct {
	agent         *agent.Session
	session       memory.Session
	modelRevision int64
	// used orders chats by focus, for closing the least recently used.
	used uint64
	// compacting, changingModel and archiving hold the chat like a turn
	// does; a chat being archived or changing model refuses new turns.
	compacting    bool
	changingModel bool
	archiving     bool
}

// held reports why the chat refuses a new turn, or "".
func (c *openChat) held() string {
	switch {
	case c.changingModel:
		return "Chat model changed; refresh before sending"
	case c.archiving:
		return "this conversation is being archived"
	}
	return ""
}

// chatLocked returns the open chat id names, or nil. A focused chat known
// only through the mirror fields is adopted into the set.
func (s *Server) chatLocked(id memory.SessionID) *openChat {
	if id == "" {
		return nil
	}
	if chat := s.chats[id]; chat != nil {
		return chat
	}
	if s.session == nil || s.activeSession.ID != id {
		return nil
	}
	chat := &openChat{agent: s.session, session: s.activeSession, modelRevision: s.modelRevision}
	s.chats[id] = chat
	return chat
}

// chatReadLocked is chatLocked for a read lock: it never adopts, and returns
// the mirrored focused chat as a detached value instead.
func (s *Server) chatReadLocked(id memory.SessionID) *openChat {
	if id == "" {
		return nil
	}
	if chat := s.chats[id]; chat != nil {
		return chat
	}
	if s.session == nil || s.activeSession.ID != id {
		return nil
	}
	return &openChat{agent: s.session, session: s.activeSession, modelRevision: s.modelRevision}
}

// focusLocked adds chat to the set if needed and makes it the focused chat.
func (s *Server) focusLocked(chat *openChat) {
	s.chats[chat.session.ID] = chat
	s.chatUses++
	chat.used = s.chatUses
	s.session, s.activeSession, s.modelRevision = chat.agent, chat.session, chat.modelRevision
}

// mirrorLocked refreshes the focused mirror after chat's agent or model
// revision changed.
func (s *Server) mirrorLocked(chat *openChat) {
	if s.activeSession.ID == chat.session.ID {
		s.session, s.activeSession, s.modelRevision = chat.agent, chat.session, chat.modelRevision
	}
}

// busyLocked reports whether the chat has a running turn, compaction or model
// change. A pending approval belongs to a running turn.
func (s *Server) busyLocked(id memory.SessionID) bool {
	if _, running := s.turns[id]; running {
		return true
	}
	chat := s.chats[id]
	return chat != nil && (chat.compacting || chat.changingModel || chat.archiving)
}

// holdArchivingLocked marks the open chats an archival covers, so no turn
// starts in them while it runs, and returns the call that releases them.
func (s *Server) holdArchivingLocked(cover func(*openChat) bool) func() {
	if s.session != nil {
		s.chatLocked(s.activeSession.ID)
	}
	var held []*openChat
	for _, chat := range s.chats {
		if cover(chat) {
			chat.archiving = true
			held = append(held, chat)
		}
	}
	return func() {
		for _, chat := range held {
			chat.archiving = false
		}
	}
}

// workspaceBusyLocked reports whether any open chat of the Workspace is busy.
func (s *Server) workspaceBusyLocked(workspace memory.WorkspaceID) bool {
	if s.session != nil && s.activeSession.WorkspaceID == workspace && s.busyLocked(s.activeSession.ID) {
		return true
	}
	for id, chat := range s.chats {
		if chat.session.WorkspaceID == workspace && s.busyLocked(id) {
			return true
		}
	}
	return false
}

// closeLocked closes one chat; closing the focused chat leaves none focused.
func (s *Server) closeLocked(id memory.SessionID) {
	delete(s.chats, id)
	if s.activeSession.ID == id {
		s.session, s.activeSession, s.modelRevision = nil, memory.Session{}, 0
	}
}

// closeWorkspaceLocked closes every open chat of the Workspace.
func (s *Server) closeWorkspaceLocked(workspace memory.WorkspaceID) {
	for id, chat := range s.chats {
		if chat.session.WorkspaceID == workspace {
			s.closeLocked(id)
		}
	}
	if s.activeSession.WorkspaceID == workspace {
		s.closeLocked(s.activeSession.ID)
	}
}

// roomLocked reports whether another chat can open: below the limit, or with
// an idle chat to close.
func (s *Server) roomLocked() bool {
	return len(s.chats) < maxOpenChats || s.idlestLocked() != ""
}

// makeRoomLocked closes the least recently used idle chats until another can
// open. When every chat is busy it closes nothing: a turn that started after
// roomLocked was checked must not lose its chat.
func (s *Server) makeRoomLocked() {
	for len(s.chats) >= maxOpenChats {
		id := s.idlestLocked()
		if id == "" {
			return
		}
		s.closeLocked(id)
	}
}

func (s *Server) idlestLocked() memory.SessionID {
	var idlest memory.SessionID
	var used uint64
	for id, chat := range s.chats {
		if s.busyLocked(id) {
			continue
		}
		if idlest == "" || chat.used < used {
			idlest, used = id, chat.used
		}
	}
	return idlest
}

// isOpenChat reports whether id is an open chat.
func (s *Server) isOpenChat(id memory.SessionID) bool {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	return s.chatReadLocked(id) != nil
}
