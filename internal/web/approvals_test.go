package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

// pendingID polls until the in-flight turn registers an approval and
// returns its id — the test's stand-in for the browser receiving the
// approval_request event.
func pendingID(t *testing.T, s *Server) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for id := range s.pending {
			s.mu.Unlock()
			return id
		}
		s.mu.Unlock()
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("no approval registered in time")
	return ""
}

// editFileTurn scripts a turn where the model edits path via the real
// (gated) edit_file tool, then wraps up.
func editFileTurn(path string) *fakeClient {
	args := fmt.Sprintf(`{"path":%q,"old_string":"world","new_string":"evie"}`, path)
	return &fakeClient{steps: []fakeStep{
		{toolCalls: []openrouter.ToolCall{{
			ID: "c1", Type: "function",
			Function: openrouter.FunctionCall{Name: "edit_file", Arguments: args},
		}}},
		{content: "done"},
	}}
}

func writeTempFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func postApprove(t *testing.T, h http.Handler, id string, approve bool) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"id": id, "approve": approve})
	req := httptest.NewRequest("POST", "http://127.0.0.1:6687/api/approve", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestApprovalApprovedRunsTool(t *testing.T) {
	path := writeTempFile(t)
	srv, h := newTestServerFull(editFileTurn(path))

	rec := httptest.NewRecorder()
	turnDone := make(chan struct{})
	go func() {
		defer close(turnDone)
		h.ServeHTTP(rec, chatRequest(`{"message":"edit it"}`))
	}()

	id := pendingID(t, srv)
	if res := postApprove(t, h, id, true); res.Code != http.StatusNoContent {
		t.Fatalf("approve status = %d", res.Code)
	}
	<-turnDone

	got, _ := os.ReadFile(path)
	if string(got) != "hello evie" {
		t.Fatalf("file = %q, want the approved edit applied", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: approval_request\n") {
		t.Fatalf("stream missing approval_request:\n%s", body)
	}
	if !strings.Contains(body, `"oldText":"hello world","newText":"hello evie"`) {
		t.Fatalf("approval request missing full-file preview:\n%s", body)
	}
	if !strings.Contains(body, "event: turn_done\n") {
		t.Fatalf("stream missing turn_done:\n%s", body)
	}
}

func TestApprovalDeclinedLeavesFileAlone(t *testing.T) {
	path := writeTempFile(t)
	srv, h := newTestServerFull(editFileTurn(path))

	rec := httptest.NewRecorder()
	turnDone := make(chan struct{})
	go func() {
		defer close(turnDone)
		h.ServeHTTP(rec, chatRequest(`{"message":"edit it"}`))
	}()

	id := pendingID(t, srv)
	if res := postApprove(t, h, id, false); res.Code != http.StatusNoContent {
		t.Fatalf("approve status = %d", res.Code)
	}
	<-turnDone

	got, _ := os.ReadFile(path)
	if string(got) != "hello world" {
		t.Fatalf("file = %q, declined edit must not apply", got)
	}
	if !strings.Contains(rec.Body.String(), "David declined") {
		t.Fatalf("decline text missing from stream:\n%s", rec.Body.String())
	}
}

func TestApprovalExpiresOnDisconnect(t *testing.T) {
	path := writeTempFile(t)
	srv, h := newTestServerFull(editFileTurn(path))

	ctx, cancel := context.WithCancel(context.Background())
	req := chatRequest(`{"message":"edit it"}`).WithContext(ctx)
	rec := httptest.NewRecorder()
	turnDone := make(chan struct{})
	go func() {
		defer close(turnDone)
		h.ServeHTTP(rec, req)
	}()

	pendingID(t, srv) // wait until the approver is blocked
	cancel()          // the browser goes away
	<-turnDone

	got, _ := os.ReadFile(path)
	if string(got) != "hello world" {
		t.Fatalf("file = %q, expired approval must not apply", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "expired before David saw it") {
		t.Fatalf("expired text missing (model must not think David declined):\n%s", body)
	}
	if strings.Contains(body, "David declined") {
		t.Fatalf("disconnect wrongly reported as a decline:\n%s", body)
	}
}

func TestApproveUnknownIDIs404(t *testing.T) {
	_, h := newTestServerFull(&fakeClient{})
	if res := postApprove(t, h, "deadbeef", true); res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.Code)
	}
}
