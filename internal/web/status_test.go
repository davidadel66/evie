package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
)

// multi-chat.spec.md Stage 4: the sidebar's status feed reports a chat's
// running turn, the approval it waits on, and when the turn finished.
func TestChatStatusFollowsATurnThroughItsApproval(t *testing.T) {
	path := writeTempFile(t)
	runtime := agent.New(editFileTurn(path), webTestContextProfile("test-model"), &fakeHistory{}, memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "one"}, webTestTurnOwner{})
	server := NewContextServer(runtime, nil, nil, &fakeContextSessionController{})
	server.activeSession = memory.Session{ID: "one"}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	frames, closeStatus := followStream(t, ts.URL, "/api/chats/status", `{}`)
	defer closeStatus()
	next := func() []chatStatus {
		t.Helper()
		frame := until(t, frames, "chat_status")
		var status struct {
			Chats []chatStatus `json:"chats"`
		}
		if err := json.Unmarshal([]byte(frame[len(frame)-1].data), &status); err != nil {
			t.Fatal(err)
		}
		return status.Chats
	}
	if chats := next(); len(chats) != 0 {
		t.Fatalf("idle status %+v", chats)
	}
	if response := postJSON(t, ts.URL, "/api/chat", `{"sessionId":"one","message":"edit it","detach":true}`); response.StatusCode != http.StatusAccepted {
		t.Fatalf("detached send: %d", response.StatusCode)
	}
	var waiting chatStatus
	for waiting.Waiting == 0 {
		chats := next()
		if len(chats) != 1 || chats[0].SessionID != "one" || !chats[0].Running {
			t.Fatalf("running status %+v", chats)
		}
		waiting = chats[0]
	}
	id := pendingID(t, server)
	if response := postJSON(t, ts.URL, "/api/approve", `{"id":"`+id+`","approve":true}`); response.StatusCode != http.StatusNoContent {
		t.Fatalf("approve: %d", response.StatusCode)
	}
	var finished chatStatus
	for finished.FinishedAt == 0 || finished.Running {
		chats := next()
		if len(chats) != 1 || chats[0].Waiting != 0 {
			t.Fatalf("after the answer %+v", chats)
		}
		finished = chats[0]
	}
	server.detached.Wait()
	if got, _ := os.ReadFile(path); string(got) != "hello evie" {
		t.Fatalf("file = %q", got)
	}
}

// The stream sends the status after every change and only then: a wake with
// nothing changed sends nothing, so a status a follower has already acted on
// is not sent again afterwards. Frames stay snapshots: with several chats, one
// read before an answer can still arrive after it when another chat changed
// too, and the answer's own wake then sends the current status.
func TestChatStatusSendsNothingWhenNothingChanged(t *testing.T) {
	server := NewContextServer(nil, nil, nil, &fakeContextSessionController{})
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	frames, closeStatus := followStream(t, ts.URL, "/api/chats/status", `{}`)
	defer closeStatus()
	until(t, frames, "chat_status")

	server.statusChanged()
	select {
	case frame, ok := <-frames:
		if !ok {
			t.Fatal("status stream closed")
		}
		t.Fatalf("unchanged status sent again: %+v", frame)
	case <-time.After(200 * time.Millisecond):
	}

	server.sessionMu.Lock()
	server.finishedAt["one"] = 42
	server.sessionMu.Unlock()
	server.statusChanged()
	frame := until(t, frames, "chat_status")
	var status struct {
		Chats []chatStatus `json:"chats"`
	}
	if err := json.Unmarshal([]byte(frame[len(frame)-1].data), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Chats) != 1 || status.Chats[0] != (chatStatus{SessionID: "one", FinishedAt: 42}) {
		t.Fatalf("changed status %+v", status.Chats)
	}
}
