package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func folderPickerRequest() *http.Request {
	r := managementRequest("/api/workspaces/choose-folder", `{}`)
	r.RemoteAddr = "127.0.0.1:42000"
	return r
}

func TestWorkspaceFolderPickerHTTPSelectionCancelAndSafeFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		choice workspaceFolderChoice
		err    error
		want   int
	}{
		{name: "chosen", choice: workspaceFolderChoice{Path: "/tmp/ selected folder \t"}, want: 200},
		{name: "cancelled", choice: workspaceFolderChoice{Cancelled: true}, want: 200},
		{name: "failure", err: errors.New("private-secret /private/folder"), want: 503},
		{name: "unsupported", err: errNativeFolderPickerUnsupported, want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewContextServer(nil, nil, nil, &fakeContextSessionController{})
			defer s.Close()
			s.folderPicker = func(context.Context) (workspaceFolderChoice, error) { return tc.choice, tc.err }
			r := httptest.NewRecorder()
			s.Handler().ServeHTTP(r, folderPickerRequest())
			if r.Code != tc.want || strings.Contains(r.Body.String(), "private-secret") || strings.Contains(r.Body.String(), "/private/") {
				t.Fatalf("response = %d %s", r.Code, r.Body.String())
			}
			if tc.err == nil {
				var got workspaceFolderChoice
				if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil || got != tc.choice {
					t.Fatalf("choice = %+v, %v", got, err)
				}
			}
			s.folderPicker = func(context.Context) (workspaceFolderChoice, error) {
				return workspaceFolderChoice{Cancelled: true}, nil
			}
			r = httptest.NewRecorder()
			s.Handler().ServeHTTP(r, folderPickerRequest())
			if r.Code != 200 {
				t.Fatalf("picker lock not released = %d %s", r.Code, r.Body.String())
			}
		})
	}
}

func TestWorkspaceFolderPickerRejectsRemoteOrInvalidRequests(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*http.Request)
		want   int
	}{
		{name: "remote", change: func(r *http.Request) { r.RemoteAddr = "192.0.2.1:1234" }, want: 403},
		{name: "nonlocal origin", change: func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, want: 403},
		{name: "other local app", change: func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:8899") }, want: 403},
		{name: "nonlocal host", change: func(r *http.Request) { r.Host = "evil.example" }, want: 403},
		{name: "form", change: func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, want: 403},
		{name: "get", change: func(r *http.Request) { r.Method = http.MethodGet }, want: 405},
		{name: "extra body", change: func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(`{"path":"/tmp"}`)) }, want: 400},
		{name: "missing body", change: func(r *http.Request) { r.Body = http.NoBody; r.ContentLength = 0 }, want: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewContextServer(nil, nil, nil, &fakeContextSessionController{})
			defer s.Close()
			s.folderPicker = func(context.Context) (workspaceFolderChoice, error) {
				t.Fatal("invalid request launched picker")
				return workspaceFolderChoice{}, nil
			}
			req := folderPickerRequest()
			tc.change(req)
			response := httptest.NewRecorder()
			s.Handler().ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestWorkspaceFolderPickerAcceptsSameOriginDevelopmentProxy(t *testing.T) {
	s := NewContextServer(nil, nil, nil, &fakeContextSessionController{})
	defer s.Close()
	s.folderPicker = func(context.Context) (workspaceFolderChoice, error) {
		return workspaceFolderChoice{Cancelled: true}, nil
	}
	req := folderPickerRequest()
	// Vite's existing proxy preserves the browser Host by default.
	req.Host = "localhost:5173"
	req.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("same origin proxy = %d %s", response.Code, response.Body.String())
	}
}

func TestWorkspaceFolderPickerSerializesAndReleasesOnRequestCancellation(t *testing.T) {
	s := NewContextServer(nil, nil, nil, &fakeContextSessionController{})
	defer s.Close()
	entered := make(chan struct{})
	finished := make(chan struct{})
	var calls atomic.Int32
	s.folderPicker = func(ctx context.Context) (workspaceFolderChoice, error) {
		if calls.Add(1) > 1 {
			return workspaceFolderChoice{Cancelled: true}, nil
		}
		close(entered)
		<-ctx.Done()
		return workspaceFolderChoice{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response := httptest.NewRecorder()
	go func() { defer close(finished); s.Handler().ServeHTTP(response, folderPickerRequest().WithContext(ctx)) }()
	awaitPicker(t, entered)
	busy := httptest.NewRecorder()
	s.Handler().ServeHTTP(busy, folderPickerRequest())
	if busy.Code != 409 || calls.Load() != 1 {
		t.Fatalf("busy=%d calls=%d", busy.Code, calls.Load())
	}
	chatLock := make(chan struct{})
	go func() { s.sessionMu.Lock(); s.sessionMu.Unlock(); close(chatLock) }()
	awaitPicker(t, chatLock)
	cancel()
	awaitPicker(t, finished)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"cancelled":true`) {
		t.Fatalf("request cancellation=%d %s", response.Code, response.Body.String())
	}
	next := httptest.NewRecorder()
	s.Handler().ServeHTTP(next, folderPickerRequest())
	if next.Code != 200 || calls.Load() != 2 {
		t.Fatalf("next=%d calls=%d", next.Code, calls.Load())
	}
}

func TestWorkspaceFolderPickerShutdownAndDeadlineStopActivePicker(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "shutdown"}[shutdown], func(t *testing.T) {
			s := NewContextServer(nil, nil, nil, &fakeContextSessionController{})
			defer s.Close()
			entered := make(chan struct{})
			finished := make(chan struct{})
			s.folderPicker = func(ctx context.Context) (workspaceFolderChoice, error) {
				close(entered)
				<-ctx.Done()
				return workspaceFolderChoice{}, ctx.Err()
			}
			ctx := context.Background()
			cancel := func() {}
			if !shutdown {
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
			}
			defer cancel()
			response := httptest.NewRecorder()
			go func() { defer close(finished); s.Handler().ServeHTTP(response, folderPickerRequest().WithContext(ctx)) }()
			awaitPicker(t, entered)
			if shutdown {
				s.Close()
			}
			awaitPicker(t, finished)
			if shutdown {
				next := httptest.NewRecorder()
				s.Handler().ServeHTTP(next, folderPickerRequest())
				if next.Code != 503 {
					t.Fatalf("closed picker admitted request: %d", next.Code)
				}
			} else if response.Code != 504 {
				t.Fatalf("timeout=%d %s", response.Code, response.Body.String())
			}
		})
	}
}

func awaitPicker(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("folder picker did not finish")
	}
}
