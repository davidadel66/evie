package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

// gatedClient answers once its release channel closes; a nil release
// answers at once.
type gatedClient struct {
	entered chan struct{}
	release chan struct{}
	answer  string
}

func (c *gatedClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if c.entered != nil {
		c.entered <- struct{}{}
	}
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return openrouter.ChatResponse{}, ctx.Err()
		}
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: c.answer}}}}, nil
}

// multi-chat.spec.md Stage 2, composed with real SQLite: while one chat's
// turn waits on its model, another chat opens, answers and is archived, the
// waiting chat's history is readable and it reopens without being rebuilt.
func TestWebChatsRunSideBySide(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	profile, err := openrouter.NewExplicitContextProfile("test/model", 300000, 200000, 12000)
	if err != nil {
		t.Fatal(err)
	}
	slow := &gatedClient{entered: make(chan struct{}, 1), release: make(chan struct{}), answer: "slow answer"}
	clients := []*gatedClient{slow, {answer: "quick answer"}}
	var mu sync.Mutex
	built := 0
	controller := newWebContextSessionController(store, sessionCompositionManager(t), func(session memory.Session, composition plugins.ResolvedComposition) (*agent.Session, error) {
		mu.Lock()
		defer mu.Unlock()
		client := clients[built]
		built++
		holder := memory.LeaseHolderID("web-" + session.ID)
		return agent.NewWithToolset(client, profile, store.BindHistory(session.ID, holder), session.ScopeContext(), store.BindTurnOwner(session.ID, holder), composition.Toolset), nil
	})
	handler := web.NewContextServer(nil, nil, nil, controller).Handler()
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Host = "127.0.0.1:6687"
		request.Header.Set("Origin", "http://127.0.0.1:6687")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	open := func(body string) memory.SessionID {
		t.Helper()
		result := post("/api/context-sessions/select", body)
		var opened struct {
			Session memory.Session `json:"session"`
		}
		if result.Code != http.StatusOK || json.Unmarshal(result.Body.Bytes(), &opened) != nil {
			t.Fatalf("open %s: %d %s", body, result.Code, result.Body)
		}
		return opened.Session.ID
	}
	turnStatus := func(id memory.SessionID) string {
		t.Helper()
		result := post("/api/context-sessions/history", `{"sessionId":"`+string(id)+`"}`)
		var page struct {
			Items []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
				Turn *struct {
					Status string `json:"status"`
				} `json:"turn"`
			} `json:"items"`
		}
		if result.Code != http.StatusOK || json.Unmarshal(result.Body.Bytes(), &page) != nil || len(page.Items) == 0 || page.Items[0].Turn == nil {
			t.Fatalf("history of %s: %d %s", id, result.Code, result.Body)
		}
		return page.Items[0].Turn.Status
	}

	first := open(`{"unscoped":true}`)
	slowDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { slowDone <- post("/api/chat", `{"sessionId":"`+string(first)+`","message":"take your time"}`) }()
	<-slow.entered

	second := open(`{"unscoped":true}`)
	if quick := post("/api/chat", `{"sessionId":"`+string(second)+`","message":"quick one"}`); quick.Code != http.StatusOK || !strings.Contains(quick.Body.String(), "quick answer") {
		t.Fatalf("second chat while the first runs: %d %s", quick.Code, quick.Body)
	}
	if status := turnStatus(first); status != "working" {
		t.Fatalf("running chat's history turn = %q", status)
	}
	if again := post("/api/chat", `{"sessionId":"`+string(first)+`","message":"twice"}`); again.Code != http.StatusConflict {
		t.Fatalf("a second turn in the running chat: %d", again.Code)
	}
	if archive := post("/api/context-sessions/archive", `{"sessionId":"`+string(first)+`"}`); archive.Code != http.StatusConflict {
		t.Fatalf("archiving the running chat: %d", archive.Code)
	}
	if reopened := open(`{"sessionId":"` + string(first) + `"}`); reopened != first || built != 2 {
		t.Fatalf("reopening the running chat rebuilt it: %s, %d agents", reopened, built)
	}
	if archive := post("/api/context-sessions/archive", `{"sessionId":"`+string(second)+`"}`); archive.Code != http.StatusOK {
		t.Fatalf("archiving the idle chat: %d %s", archive.Code, archive.Body)
	}
	if closed := post("/api/context-sessions/history", `{"sessionId":"`+string(second)+`"}`); closed.Code != http.StatusConflict {
		t.Fatalf("archived chat still open: %d", closed.Code)
	}

	close(slow.release)
	if done := <-slowDone; done.Code != http.StatusOK || !strings.Contains(done.Body.String(), "slow answer") {
		t.Fatalf("the first chat's turn: %d %s", done.Code, done.Body)
	}
	if status := turnStatus(first); status != "complete" {
		t.Fatalf("finished chat's history turn = %q", status)
	}
}
