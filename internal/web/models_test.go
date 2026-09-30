package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type fakeModelController struct {
	*fakeContextSessionController
	client *fakeClient
	err    error
	calls  int
}

func (c *fakeModelController) ListChatModels(context.Context) ([]openrouter.Model, error) {
	return []openrouter.Model{{ID: "anthropic/test", Name: "Claude Test", Provider: "anthropic"}}, c.err
}
func (c *fakeModelController) SelectModel(_ context.Context, id memory.SessionID, revision int64, model string) (OpenedContextSession, error) {
	c.calls++
	if c.err != nil {
		return OpenedContextSession{}, c.err
	}
	return OpenedContextSession{Session: memory.Session{ID: id, Status: memory.SessionActive}, Agent: agent.New(c.client, webTestContextProfile(model), &fakeHistory{}, memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: id}, webTestTurnOwner{}), ModelRevision: revision + 1}, nil
}
func modelTestServer() (*Server, *fakeModelController) {
	client := &fakeClient{steps: []fakeStep{{content: "selected model replied"}}}
	controller := &fakeModelController{fakeContextSessionController: &fakeContextSessionController{}, client: client}
	s := NewContextServer(agent.New(client, webTestContextProfile("openai/test"), &fakeHistory{}, memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session-1"}, webTestTurnOwner{}), nil, nil, controller)
	s.activeSession = memory.Session{ID: "session-1", Status: memory.SessionActive}
	return s, controller
}

func TestChatModelSelectionDrivesNextRequestAndRejectsOldBrowserModel(t *testing.T) {
	s, c := modelTestServer()
	handler := s.Handler()
	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, managementRequest("/api/models/select", `{"sessionId":"session-1","model":"anthropic/test","revision":0}`))
	if selected.Code != http.StatusOK || !strings.Contains(selected.Body.String(), `"revision":1`) {
		t.Fatalf("selection=%d %s", selected.Code, selected.Body.String())
	}
	stale := httptest.NewRecorder()
	handler.ServeHTTP(stale, managementRequest("/api/chat", `{"sessionId":"session-1","model":"openai/test","message":"hello"}`))
	if stale.Code != http.StatusConflict || len(c.client.reqs) != 0 {
		t.Fatalf("stale send=%d requests=%d", stale.Code, len(c.client.reqs))
	}
	chat := httptest.NewRecorder()
	handler.ServeHTTP(chat, managementRequest("/api/chat", `{"sessionId":"session-1","model":"anthropic/test","message":"hello"}`))
	if chat.Code != http.StatusOK || len(c.client.reqs) != 1 || c.client.reqs[0].Model != "anthropic/test" {
		t.Fatalf("chat=%d requests=%+v body=%s", chat.Code, c.client.reqs, chat.Body.String())
	}
}

func TestChatModelSelectionRejectsBusyStaleAndCrossOriginRequests(t *testing.T) {
	for _, variant := range []string{"busy", "selecting", "session", "revision", "origin", "missing-revision"} {
		t.Run(variant, func(t *testing.T) {
			s, c := modelTestServer()
			body := `{"sessionId":"session-1","model":"anthropic/test","revision":0}`
			if variant == "busy" {
				s.activeTurns = 1
			}
			if variant == "selecting" {
				s.selectingSession = true
			}
			if variant == "session" {
				body = strings.Replace(body, "session-1", "stale", 1)
			}
			if variant == "revision" {
				s.modelRevision = 2
			}
			if variant == "missing-revision" {
				body = `{"sessionId":"session-1","model":"anthropic/test"}`
			}
			r := managementRequest("/api/models/select", body)
			if variant == "origin" {
				r.Header.Set("Origin", "https://attacker.example")
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code < 400 || c.calls != 0 || s.session.ContextProfile().ConfiguredModel != "openai/test" {
				t.Fatalf("status=%d calls=%d", w.Code, c.calls)
			}
		})
	}
}

func TestChatModelFailureKeepsCurrentRuntimeAndSanitizesErrors(t *testing.T) {
	s, c := modelTestServer()
	c.err = errors.New("secret provider token")
	for _, route := range []string{"list", "select"} {
		body := `{"sessionId":"session-1"}`
		if route == "select" {
			body = `{"sessionId":"session-1","model":"anthropic/test","revision":0}`
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, managementRequest("/api/models/"+route, body))
		if strings.Contains(w.Body.String(), "secret") || s.session.ContextProfile().ConfiguredModel != "openai/test" || s.selectingSession {
			t.Fatalf("failure leaked or changed state: %s", w.Body.String())
		}
		if route == "list" && (w.Code != 200 || !strings.Contains(w.Body.String(), `"model":"openai/test"`) || !strings.Contains(w.Body.String(), `"problem"`)) {
			t.Fatalf("catalog failure lost current model: %d %s", w.Code, w.Body.String())
		}
	}
}
