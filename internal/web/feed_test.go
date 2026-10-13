package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type sseFrame struct {
	event string
	data  string
}

// followFeed follows a chat's feed over a real connection and returns its
// events as they arrive; closing the returned function closes the tab.
func followFeed(t *testing.T, base string, session memory.SessionID) (<-chan sseFrame, func()) {
	t.Helper()
	return followStream(t, base, "/api/chats/follow", `{"sessionId":"`+string(session)+`"}`)
}

func followStream(t *testing.T, base, path, body string) (<-chan sseFrame, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", base)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("follow: %d", response.StatusCode)
	}
	frames := make(chan sseFrame, 256)
	go func() {
		defer close(frames)
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		var frame sseFrame
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				frame.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				frame.data = strings.TrimPrefix(line, "data: ")
			case line == "" && frame.event != "":
				frames <- frame
				frame = sseFrame{}
			}
		}
	}()
	return frames, cancel
}

// until reads frames up to and including the first named one.
func until(t *testing.T, frames <-chan sseFrame, event string) []sseFrame {
	t.Helper()
	var seen []sseFrame
	timeout := time.After(5 * time.Second)
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatalf("feed closed before %s; saw %v", event, seen)
			}
			seen = append(seen, frame)
			if frame.event == event {
				return seen
			}
		case <-timeout:
			t.Fatalf("no %s; saw %v", event, seen)
		}
	}
}

func names(frames []sseFrame) string {
	out := make([]string, len(frames))
	for i, frame := range frames {
		out[i] = frame.event
	}
	return strings.Join(out, " ")
}

func postJSON(t *testing.T, base, path, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", base)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return response
}

// multi-chat.spec.md Stage 3: a detached turn reaches every tab following its
// chat; a tab joining midway receives the turn so far, text merged; a tab
// closing does not end the turn or its approval; a tab joining after the end
// receives nothing to replay, since history holds the turn.
func TestDetachedTurnReachesFollowersAndLateJoiners(t *testing.T) {
	path := writeTempFile(t)
	client := &fakeClient{steps: []fakeStep{
		{deltas: []string{"Let me ", "edit ", "it."}, toolCalls: []openrouter.ToolCall{{
			ID: "c1", Type: "function",
			Function: openrouter.FunctionCall{Name: "edit_file", Arguments: fmt.Sprintf(`{"path":%q,"old_string":"world","new_string":"evie"}`, path)},
		}}},
		{content: "done"},
	}}
	runtime := agent.New(client, webTestContextProfile("test-model"), &fakeHistory{}, memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "one"}, webTestTurnOwner{})
	server := NewContextServer(runtime, nil, nil, &fakeContextSessionController{})
	server.activeSession = memory.Session{ID: "one"}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	first, closeFirst := followFeed(t, ts.URL, "one")
	if state := until(t, first, "feed_state"); state[0].data != `{"running":false}` {
		t.Fatalf("idle feed state %s", state[0].data)
	}
	if response := postJSON(t, ts.URL, "/api/chat", `{"sessionId":"one","message":"edit it","detach":true}`); response.StatusCode != http.StatusAccepted {
		t.Fatalf("detached send: %d", response.StatusCode)
	}
	live := until(t, first, "approval_request")
	if !strings.HasPrefix(names(live), "user_message turn_started") || !strings.Contains(live[0].data, `"text":"edit it"`) {
		t.Fatalf("live start %s %s", names(live), live[0].data)
	}

	second, closeSecond := followFeed(t, ts.URL, "one")
	defer closeSecond()
	replay := until(t, second, "approval_request")
	if replay[0].data != `{"running":true}` || replay[1].event != "user_message" || replay[2].event != "turn_started" {
		t.Fatalf("replay start %s", names(replay))
	}
	deltas := 0
	for _, frame := range replay {
		if frame.event == "delta" {
			deltas++
			if frame.data != `{"text":"Let me edit it."}` {
				t.Fatalf("replayed text %s", frame.data)
			}
		}
	}
	if deltas != 1 {
		t.Fatalf("replay should merge the text into one delta: %s", names(replay))
	}
	var approval struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(replay[len(replay)-1].data), &approval); err != nil {
		t.Fatal(err)
	}

	closeFirst() // the tab that sent the message closes
	time.Sleep(20 * time.Millisecond)
	if response := postJSON(t, ts.URL, "/api/approve", `{"id":"`+approval.ID+`","approve":true}`); response.StatusCode != http.StatusNoContent {
		t.Fatalf("approve from the other tab: %d", response.StatusCode)
	}
	if end := until(t, second, "turn_done"); !strings.Contains(names(end), "tool_result") {
		t.Fatalf("rest of the turn %s", names(end))
	}
	server.detached.Wait()
	if got, _ := os.ReadFile(path); string(got) != "hello evie" {
		t.Fatalf("file = %q", got)
	}

	late, closeLate := followFeed(t, ts.URL, "one")
	defer closeLate()
	if state := until(t, late, "feed_state"); state[0].data != `{"running":false}` {
		t.Fatalf("finished feed state %s", state[0].data)
	}
	select {
	case frame := <-late:
		t.Fatalf("a finished turn replayed %s", frame.event)
	case <-time.After(100 * time.Millisecond):
	}
}

// A detached turn's approval waits while any tab follows a chat, and expires
// only once none has for the grace period.
func TestDetachedApprovalWaitsForATab(t *testing.T) {
	for _, followed := range []bool{true, false} {
		t.Run(fmt.Sprintf("followed=%t", followed), func(t *testing.T) {
			path := writeTempFile(t)
			server, handler := newTestServerFull(editFileTurn(path))
			server.approvalGrace, server.presenceCheck = 40*time.Millisecond, 5*time.Millisecond
			if followed {
				server.arrive()
			} else {
				server.arrive()
				server.leave() // the last tab closed just now
			}
			chat := httptest.NewRecorder()
			handler.ServeHTTP(chat, chatRequest(`{"message":"edit it","detach":true}`))
			if chat.Code != http.StatusAccepted {
				t.Fatalf("detached send: %d %s", chat.Code, chat.Body)
			}
			id := pendingID(t, server)
			time.Sleep(4 * server.approvalGrace)
			answered := postApprove(t, handler, id, true)
			server.detached.Wait()
			got, _ := os.ReadFile(path)
			if followed && (answered.Code != http.StatusNoContent || string(got) != "hello evie") {
				t.Fatalf("followed: approve %d, file %q", answered.Code, got)
			}
			if !followed && (answered.Code != http.StatusNotFound || string(got) != "hello world") {
				t.Fatalf("unattended: approve %d, file %q", answered.Code, got)
			}
		})
	}
}

func TestChatFeedReplayAndSlowFollowers(t *testing.T) {
	feed := &chatFeed{}
	stale := feed.begin()
	feed.end(stale)
	turn := feed.begin()
	feed.publish(stale, "delta", []byte(`{"text":"late"}`))
	feed.publish(turn, "reasoning", []byte(`{"text":"think "}`))
	feed.publish(turn, "reasoning", []byte(`{"text":"more"}`))
	feed.publish(turn, "tool_call", []byte(`{"id":"a"}`))
	running, replay, slow := feed.follow()
	if !running || len(replay) != 2 || string(replay[0].data) != `{"text":"think more"}` || replay[1].event != "tool_call" {
		t.Fatalf("replay %t %v", running, replay)
	}
	for range followerBuffer + 1 {
		feed.publish(turn, "delta", []byte(`{"text":"x"}`))
	}
	drained := 0
	for range slow {
		drained++
	}
	if drained != followerBuffer {
		t.Fatalf("a follower that fell behind should be dropped after its buffer, got %d", drained)
	}
	feed.end(turn)
	if running, replay, _ := feed.follow(); running || len(replay) != 0 {
		t.Fatalf("after the turn: %t %v", running, replay)
	}
}

// followedChat serves one open chat "one" around client and owner, over a
// real connection so its feed can be followed.
func followedChat(t *testing.T, client agent.Client, owner webTestTurnOwner) (*Server, *httptest.Server) {
	t.Helper()
	runtime := agent.New(client, webTestContextProfile("test-model"), &fakeHistory{}, memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "one"}, owner)
	server := NewContextServer(runtime, nil, nil, &fakeContextSessionController{})
	server.activeSession = memory.Session{ID: "one"}
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return server, ts
}

// A tab joining after an approval was answered shows the answer, not a
// pending card: the decision is published to the chat's feed.
func TestApprovalDecisionReachesTheFeed(t *testing.T) {
	path := writeTempFile(t)
	server, ts := followedChat(t, editFileTurn(path), webTestTurnOwner{})
	frames, closeFeed := followFeed(t, ts.URL, "one")
	defer closeFeed()
	until(t, frames, "feed_state")
	if response := postJSON(t, ts.URL, "/api/chat", `{"sessionId":"one","message":"edit it","detach":true}`); response.StatusCode != http.StatusAccepted {
		t.Fatal(response.StatusCode)
	}
	request := until(t, frames, "approval_request")
	var approval struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(request[len(request)-1].data), &approval); err != nil {
		t.Fatal(err)
	}
	postJSON(t, ts.URL, "/api/approve", `{"id":"`+approval.ID+`","approve":true}`)
	decided := until(t, frames, "approval_decided")
	if want := `{"id":"` + approval.ID + `","state":"approved"}`; decided[len(decided)-1].data != want {
		t.Fatalf("decision %s, want %s", decided[len(decided)-1].data, want)
	}
	until(t, frames, "turn_done")
	server.detached.Wait()
}

type panicClient struct{}

func (panicClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	panic("provider bug")
}

// A panic in a detached turn ends that turn, not the server.
func TestDetachedTurnPanicEndsOnlyThatTurn(t *testing.T) {
	server, ts := followedChat(t, panicClient{}, webTestTurnOwner{})
	frames, closeFeed := followFeed(t, ts.URL, "one")
	defer closeFeed()
	until(t, frames, "feed_state")
	if response := postJSON(t, ts.URL, "/api/chat", `{"sessionId":"one","message":"hi","detach":true}`); response.StatusCode != http.StatusAccepted {
		t.Fatal(response.StatusCode)
	}
	end := until(t, frames, "turn_done")
	if !strings.Contains(names(end), "error") {
		t.Fatalf("followers were not told the turn failed: %s", names(end))
	}
	server.detached.Wait()
	if response := postJSON(t, ts.URL, "/api/context-sessions/list", `{}`); response.StatusCode != http.StatusOK {
		t.Fatalf("server after the panic: %d", response.StatusCode)
	}
	if server.isOpenChat("one") {
		t.Fatal("the chat whose turn panicked is still open")
	}
}

// A detached turn the agent refuses tells followers with its own code, which
// the browser keeps so queued messages wait.
func TestRefusedDetachedTurnIsMarked(t *testing.T) {
	server, ts := followedChat(t, &fakeClient{}, webTestTurnOwner{acquireErr: errors.New("lease held elsewhere"), conflict: true})
	frames, closeFeed := followFeed(t, ts.URL, "one")
	defer closeFeed()
	until(t, frames, "feed_state")
	if response := postJSON(t, ts.URL, "/api/chat", `{"sessionId":"one","message":"hi","detach":true}`); response.StatusCode != http.StatusAccepted {
		t.Fatal(response.StatusCode)
	}
	end := until(t, frames, "turn_done")
	if names(end) != "error turn_done" || !strings.Contains(end[0].data, `"code":"turn_refused"`) {
		t.Fatalf("refusal frames %s %s", names(end), end[0].data)
	}
	server.detached.Wait()
}
