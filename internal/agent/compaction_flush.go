package agent

import "github.com/davidadel66/evie/internal/memory"

// CompactionMemoryFlush is offered the frontier of every committed
// compaction of a root conversation: the memory flush before compaction
// (memory architecture Stage 10). through is the sequence of the last event
// the summary covers, so the background memory pass covers every owner
// message up to it. RequestCompactionFlush must return at once without
// waiting for a pass: the pass reads the stored events later and never holds
// the turn or its lease. The memory background runner implements it.
type CompactionMemoryFlush interface {
	RequestCompactionFlush(sessionID memory.SessionID, through int64)
}

// WithCompactionMemoryFlush offers the session's committed whole-turn,
// in-turn and manual compactions to the background memory pass. Delegated
// sessions never offer theirs.
func WithCompactionMemoryFlush(flush CompactionMemoryFlush) SessionOption {
	return func(session *Session) { session.compactionFlush = flush }
}

// requestCompactionFlush offers the owner messages through the last event a
// committed compaction summarized. A delegated child never flushes: its
// "user" messages are the orchestrator's task text, not the owner's words.
func (s *Session) requestCompactionFlush(through int64) {
	if s.compactionFlush == nil || through < 1 || s.scope.ParentSessionID != "" || s.workerInstructions != "" {
		return
	}
	s.compactionFlush.RequestCompactionFlush(s.scope.SessionID, through)
}
