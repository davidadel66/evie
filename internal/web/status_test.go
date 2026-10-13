package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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
