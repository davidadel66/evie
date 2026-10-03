package web_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

type fakeBackgroundStatus struct {
	status memory.BackgroundStatus
	err    error
}

func (f fakeBackgroundStatus) MemoryBackgroundStatus(context.Context) (memory.BackgroundStatus, error) {
	return f.status, f.err
}

func backgroundStatusPost(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/memory/background/status", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// The Review area shows the background pass: on or off and why, its model,
// last run, today's spend against the caps.
func TestMemoryBackgroundStatusRoute(t *testing.T) {
	status := memory.BackgroundStatus{Enabled: true, State: memory.BackgroundStateOn, Provider: "openrouter", Model: "deepseek/deepseek-v4.1-flash",
		IdleMinutes: 10, TurnTrigger: 20, DailyTokenCap: 300_000, DailyMicroUSDCap: 250_000, RemainingTokens: 290_000}
	handler := web.WithMemoryBackground(web.NewServer(nil), fakeBackgroundStatus{status: status}).Handler()
	got := reviewResponse[memory.BackgroundStatus](t, backgroundStatusPost(t, handler), http.StatusOK)
	if got.Model != status.Model || got.State != memory.BackgroundStateOn || got.RemainingTokens != 290_000 {
		t.Fatalf("status = %+v", got)
	}
	failing := web.WithMemoryBackground(web.NewServer(nil), fakeBackgroundStatus{err: errors.New("database detail")}).Handler()
	if w := backgroundStatusPost(t, failing); w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "database detail") {
		t.Fatalf("failure = %d %s", w.Code, w.Body.String())
	}
	if w := backgroundStatusPost(t, web.NewServer(nil).Handler()); w.Code == http.StatusOK {
		t.Fatalf("a server without the pass answered status: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/memory/background/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", w.Code)
	}
}
