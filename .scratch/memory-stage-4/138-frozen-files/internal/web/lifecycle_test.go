package web

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type lifecycleClient struct {
	started       chan struct{}
	cancelled     chan struct{}
	activeContext context.Context
}

func (c *lifecycleClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.activeContext = ctx
	close(c.started)
	<-ctx.Done()
	close(c.cancelled)
	return openrouter.ChatResponse{}, ctx.Err()
}

func TestServeListenerContextKeepsTurnAliveAfterBrowserDisconnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client := &lifecycleClient{started: make(chan struct{}), cancelled: make(chan struct{})}
	session := agent.New(client, webTestContextProfile("lifecycle"), &fakeHistory{}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	disconnected := make(chan struct{})
	handler := NewServer(session).Handler()
	served := make(chan error, 1)
	go func() {
		served <- serveListenerContext(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			go func() { <-r.Context().Done(); close(disconnected) }()
			handler.ServeHTTP(w, r)
		}))
	}()
	requestCtx, disconnect := context.WithCancel(context.Background())
	defer disconnect()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, "http://"+listener.Addr().String()+"/api/chat", strings.NewReader(`{"message":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response := make(chan error, 1)
	go func() {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
		if resp != nil {
			_ = resp.Body.Close()
		}
		response <- err
	}()
	select {
	case <-client.started:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP chat did not reach provider")
	}
	disconnect()
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("server did not observe browser disconnect")
	}
	if err := client.activeContext.Err(); err != nil {
		t.Fatalf("browser disconnect cancelled independent turn: %v", err)
	}
	if err := <-response; !errors.Is(err, context.Canceled) {
		t.Fatalf("browser request cancellation: %v", err)
	}
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runtime failed to cancel disconnected turn during shutdown")
	}
}

func TestServeListenerContextCancelsActiveChatAndClosesListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client := &lifecycleClient{started: make(chan struct{}), cancelled: make(chan struct{})}
	history := &fakeHistory{}
	session := agent.New(client, webTestContextProfile("lifecycle"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serveListenerContext(ctx, listener, NewServer(session).Handler()) }()
	response := make(chan error, 1)
	go func() {
		httpClient := &http.Client{Timeout: 3 * time.Second}
		resp, err := httpClient.Post("http://"+listener.Addr().String()+"/api/chat", "application/json", strings.NewReader(`{"message":"hello"}`))
		if err != nil {
			response <- err
			return
		}
		_, err = io.Copy(io.Discard, resp.Body)
		response <- errors.Join(err, resp.Body.Close())
	}()
	select {
	case <-client.started:
	case err := <-response:
		t.Fatalf("HTTP chat ended before reaching provider: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP chat did not reach provider")
	}
	cancel()
	select {
	case <-client.cancelled:
	case <-time.After(time.Second):
		t.Fatal("runtime cancellation did not reach active provider")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP server did not finish shutdown")
	}
	select {
	case err := <-response:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("active HTTP response was not completed")
	}
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("shutdown listener still accepts connections")
	}
	for _, event := range history.events {
		if event.Type == memory.EventAssistantMessage {
			t.Fatal("cancelled provider persisted a late final response")
		}
	}
}

func TestServeWithContextPreservesLoopbackValidation(t *testing.T) {
	t.Setenv("EVIE_ADDR", "0.0.0.0:0")
	if err := ServeWithContext(context.Background(), NewServer(nil)); err == nil || !strings.Contains(err.Error(), "loopback only") {
		t.Fatalf("non-loopback runtime listener: %v", err)
	}
}

func TestServeWithContextReturnsListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("EVIE_ADDR", listener.Addr().String())
	if err := ServeWithContext(context.Background(), NewServer(nil)); err == nil {
		t.Fatal("runtime hid listener startup failure")
	}
}
