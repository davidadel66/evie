package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

// Live feed per chat (multi-chat.spec.md Stage 3). Every turn's SSE events
// are also published to its chat's feed. A follower first receives whether a
// turn is running and that turn's events so far, then live ones, so leaving a
// chat or reloading the page never loses a running turn. Followers are also
// Evie's presence: an approval in a detached turn waits while any tab follows.

const (
	// followerBuffer bounds the events queued for one follower; a follower
	// that falls this far behind is dropped and reconnects.
	followerBuffer = 1024
	// feedHeartbeat keeps an idle follow connection alive and notices a
	// closed tab.
	feedHeartbeat = 15 * time.Second
	// approvalGrace is how long a detached turn's approval waits once no tab
	// follows any chat (multi-chat.decisions.md).
	approvalGrace = 2 * time.Minute
)

type feedFrame struct {
	event string
	data  []byte
}

// chatFeed is one chat's live feed. generation identifies the running turn,
// so a finishing turn's late events never land in the next turn's replay.
type chatFeed struct {
	mu         sync.Mutex
	generation uint64
	running    bool
	frames     []feedFrame
	followers  map[chan feedFrame]struct{}
}

// begin starts a turn's replay buffer and returns the turn's generation.
func (f *chatFeed) begin() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.generation++
	f.running, f.frames = true, nil
	return f.generation
}

// end clears the replay buffer of the turn of generation, once its last
// event (turn_done) has been published: history now holds the whole turn.
func (f *chatFeed) end(generation uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.generation == generation {
		f.running, f.frames = false, nil
	}
}

// publish records one event of the turn of generation for replay and hands
// it to every follower. It never blocks: a follower whose queue is full is
// dropped, and its connection ends so the browser reconnects and catches up.
func (f *chatFeed) publish(generation uint64, event string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.generation != generation || !f.running {
		return
	}
	frame := feedFrame{event: event, data: data}
	f.frames = append(f.frames, frame)
	for follower := range f.followers {
		select {
		case follower <- frame:
		default:
			delete(f.followers, follower)
			close(follower)
		}
	}
}

// follow registers a follower and returns whether a turn is running, that
// turn's events so far (consecutive text fragments merged), and the queue of
// later events. Registering and copying under one lock means nothing is
// missed or repeated between the replay and the live events.
func (f *chatFeed) follow() (bool, []feedFrame, chan feedFrame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	follower := make(chan feedFrame, followerBuffer)
	if f.followers == nil {
		f.followers = map[chan feedFrame]struct{}{}
	}
	f.followers[follower] = struct{}{}
	return f.running, coalesceFrames(f.frames), follower
}

func (f *chatFeed) unfollow(follower chan feedFrame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.followers[follower]; ok {
		delete(f.followers, follower)
		close(follower)
	}
}

// coalesceFrames merges runs of delta and of reasoning fragments, so a
// follower joining mid-turn receives the text so far in one piece.
func coalesceFrames(frames []feedFrame) []feedFrame {
	out := make([]feedFrame, 0, len(frames))
	var text bytes.Buffer
	run := ""
	flush := func() {
		if run == "" {
			return
		}
		data, _ := json.Marshal(struct {
			Text string `json:"text"`
		}{text.String()})
		out = append(out, feedFrame{event: run, data: data})
		run = ""
		text.Reset()
	}
	for _, frame := range frames {
		if frame.event == "delta" || frame.event == "reasoning" {
			var fragment struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(frame.data, &fragment) == nil {
				if run != frame.event {
					flush()
					run = frame.event
				}
				text.WriteString(fragment.Text)
				continue
			}
		}
		flush()
		out = append(out, frame)
	}
	flush()
	return out
}

// feedLocked returns the chat's feed, creating it. Requires sessionMu.
func (s *Server) feedLocked(id memory.SessionID) *chatFeed {
	feed := s.feeds[id]
	if feed == nil {
		feed = &chatFeed{}
		s.feeds[id] = feed
	}
	return feed
}

// handleFollow streams one open chat's feed: a feed_state event, the running
// turn's events so far, then live events, with heartbeats between.
func (s *Server) handleFollow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID memory.SessionID `json:"sessionId"`
	}
	if status, err := decodeManagementJSON(w, r, &req); err != nil || !boundedID(string(req.SessionID)) {
		if err == nil {
			status = http.StatusBadRequest
		}
		jsonError(w, status, "A conversation is required")
		return
	}
	s.sessionMu.Lock()
	if s.chatLocked(req.SessionID) == nil {
		s.sessionMu.Unlock()
		managementJSONError(w, http.StatusConflict, "chat_not_open", "this conversation is not open; reload it")
		return
	}
	feed := s.feedLocked(req.SessionID)
	s.sessionMu.Unlock()
	stream, err := newSSEEvents(w)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	running, replay, follower := feed.follow()
	defer feed.unfollow(follower)
	s.arrive()
	defer s.leave()
	stream.emit("feed_state", struct {
		Running bool `json:"running"`
	}{running})
	for _, frame := range replay {
		stream.emitRaw(frame.event, frame.data)
	}
	heartbeat := time.NewTicker(feedHeartbeat)
	defer heartbeat.Stop()
	for stream.err == nil {
		select {
		case <-r.Context().Done():
			return
		case frame, ok := <-follower:
			if !ok {
				return // dropped as too slow; the browser reconnects
			}
			stream.emitRaw(frame.event, frame.data)
		case <-heartbeat.C:
			stream.comment("ping")
		}
	}
}

// arrive and leave count the tabs following a feed.
func (s *Server) arrive() {
	s.presenceMu.Lock()
	defer s.presenceMu.Unlock()
	s.followers++
}

func (s *Server) leave() {
	s.presenceMu.Lock()
	defer s.presenceMu.Unlock()
	s.followers--
	if s.followers == 0 {
		s.aloneSince = s.now()
	}
}

// unattended reports whether no tab has followed any feed for the grace.
func (s *Server) unattended() bool {
	s.presenceMu.Lock()
	defer s.presenceMu.Unlock()
	return s.followers == 0 && s.now().Sub(s.aloneSince) >= s.approvalGrace
}

// followedApprover asks through the feed for a detached turn. The approval
// waits for an answer from any tab, and expires only when the turn ends or
// no tab has been connected for the grace.
func (s *Server) followedApprover(ev *sseEvents) tools.Approver {
	return func(lifecycleCtx context.Context, name, args string, preview *tools.FileChangePreview) tools.Decision {
		id, ch := s.newPending()
		ev.ApprovalRequest(id, name, args, preview)
		check := time.NewTicker(s.presenceCheck)
		defer check.Stop()
		expire := func() tools.Decision {
			if _, ok := s.claimPending(id); ok {
				return tools.Expired
			}
			// An answer claimed it first; honor that decision.
			return <-ch
		}
		decision := func() tools.Decision {
			for {
				select {
				case decision := <-ch:
					return decision
				case <-lifecycleCtx.Done():
					return expire()
				case <-check.C:
					if s.unattended() {
						return expire()
					}
				}
			}
		}()
		ev.approvalDecided(id, decision)
		return decision
	}
}

// emitRaw writes an already-encoded event; comment writes an SSE comment,
// which clients ignore.
func (e *sseEvents) emitRaw(event string, data []byte) {
	e.write(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

func (e *sseEvents) comment(text string) {
	e.write(": " + text + "\n\n")
}
