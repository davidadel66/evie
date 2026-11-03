package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type fakeSubagents struct {
	progress eviedb.SubagentProgress
	events   []memory.Event
	sessions []memory.SessionID
}

func (f *fakeSubagents) SubagentsOfCall(_ context.Context, session memory.SessionID, turn memory.EventID, callID string) ([]eviedb.SubagentProgress, error) {
	f.sessions = append(f.sessions, session)
	if callID != "call" || turn != "turn" {
		return nil, nil
	}
	return []eviedb.SubagentProgress{f.progress}, nil
}

func (f *fakeSubagents) SubagentEvents(_ context.Context, session memory.SessionID, executionID string) (eviedb.SubagentProgress, []memory.Event, error) {
	f.sessions = append(f.sessions, session)
	if executionID != f.progress.ExecutionID {
		return eviedb.SubagentProgress{}, nil, sql.ErrNoRows
	}
	return f.progress, f.events, nil
}

func (f *fakeSubagents) SubagentReport(_ context.Context, session memory.SessionID, executionID string) (eviedb.SubagentReport, error) {
	f.sessions = append(f.sessions, session)
	if executionID != f.progress.ExecutionID {
		return eviedb.SubagentReport{}, sql.ErrNoRows
	}
	text := "## Summary\nHorizons double."
	return eviedb.SubagentReport{ExecutionID: executionID, Worker: f.progress.Worker, Status: f.progress.State, Text: &text, TotalBytes: len(text)}, nil
}

func newSubagentServer(state string) (*Server, *fakeSubagents) {
	reader := &fakeSubagents{
		progress: eviedb.SubagentProgress{ExecutionID: "child-1", Worker: "research", Objective: "find evidence", State: state, Actions: 2},
		events: []memory.Event{
			{ID: "u", Sequence: 1, Type: memory.EventUserMessage, Content: "assignment"},
			{ID: "i1", Sequence: 2, Type: memory.EventToolIntent, ExecutionID: "e1", Payload: []byte(`{"call":{"id":"c1","name":"web_fetch","arguments":"{\"url\":\"https://example.com\"}"}}`)},
			{ID: "r1", Sequence: 3, Type: memory.EventToolSucceeded, ExecutionID: "e1", Content: strings.Repeat("page ", 4000)},
			{ID: "i2", Sequence: 4, Type: memory.EventToolIntent, ExecutionID: "e2", Payload: []byte(`{"call":{"id":"c2","name":"web_search","arguments":"{\"query\":\"scaling\"}"}}`)},
			{ID: "i3", Sequence: 5, Type: memory.EventToolIntent, ExecutionID: "e3", Payload: []byte(`{"call":{"id":"c3","name":"write_file","arguments":"{\"path\":\"docs/notes.md\",\"content\":\"` + strings.Repeat("x", 9000) + `\"}"}}`)},
			{ID: "r3", Sequence: 6, Type: memory.EventToolSucceeded, ExecutionID: "e3", Content: "written"},
		},
	}
	server := WithSubagents(NewServer(&agent.Session{}), reader)
	server.activeSession = memory.Session{ID: "parent"}
	return server, reader
}

func TestSubagentRoutesReadOnlyTheOpenConversation(t *testing.T) {
	server, reader := newSubagentServer("running")
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/api/subagents/list", `{"sessionId":"parent","turnId":"turn","callId":"call"}`, http.StatusOK},
		{"/api/subagents/list", `{"sessionId":"other","turnId":"turn","callId":"call"}`, http.StatusConflict},
		{"/api/subagents/list", `{"sessionId":"parent"}`, http.StatusBadRequest},
		{"/api/subagents/trace", `{"sessionId":"parent","executionId":"child-1"}`, http.StatusOK},
		{"/api/subagents/trace", `{"sessionId":"other","executionId":"child-1"}`, http.StatusConflict},
		{"/api/subagents/trace", `{"sessionId":"parent","executionId":"elsewhere"}`, http.StatusNotFound},
	} {
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, managementRequest(tc.path, tc.body))
		if rr.Code != tc.status {
			t.Fatalf("%s %s: status=%d want %d: %s", tc.path, tc.body, rr.Code, tc.status, rr.Body.String())
		}
	}
	for _, session := range reader.sessions {
		if session != "parent" {
			t.Fatalf("read another conversation's subagents: %v", reader.sessions)
		}
	}

	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/subagents/list", `{"sessionId":"parent","turnId":"turn","callId":"none"}`))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"subagents":[]`) {
		t.Fatalf("a call without children: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSubagentTraceLeavesARunningChildsActionRequestedAndBoundsResults(t *testing.T) {
	for _, tc := range []struct {
		state       string
		lastSettled bool
	}{{"running", false}, {"partial", true}} {
		server, _ := newSubagentServer(tc.state)
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, managementRequest("/api/subagents/trace", `{"sessionId":"parent","executionId":"child-1"}`))
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d: %s", rr.Code, rr.Body.String())
		}
		var body struct {
			Subagent eviedb.SubagentProgress `json:"subagent"`
			Items    []historyItem           `json:"items"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Subagent.State != tc.state || len(body.Items) != 4 || body.Items[0].Text != "assignment" {
			t.Fatalf("%s trace = %+v", tc.state, body)
		}
		fetch, search := body.Items[1], body.Items[2]
		if fetch.Result == nil || len(*fetch.Result) > subagentTraceToolBytes+64 || !strings.HasSuffix(*fetch.Result, "(cut for display)") {
			t.Fatalf("fetch result not bounded: %d bytes", len(*fetch.Result))
		}
		if (search.Result != nil) != tc.lastSettled {
			t.Fatalf("%s child: unfinished action result = %v", tc.state, search.Result)
		}
		var args struct {
			Path string `json:"path"`
		}
		if write := body.Items[3]; len(write.Args) > subagentTraceToolBytes || json.Unmarshal([]byte(write.Args), &args) != nil || args.Path != "docs/notes.md" {
			t.Fatalf("bounded arguments lost their subject: %d bytes %q", len(write.Args), write.Args[:min(len(write.Args), 80)])
		}
	}
}

func TestSubagentReportRouteReadsOnlyTheOpenConversation(t *testing.T) {
	server, reader := newSubagentServer("succeeded")
	call := func(r *http.Request) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, r)
		return rr
	}

	rr := call(managementRequest("/api/subagents/report", `{"sessionId":"other","executionId":"child-1"}`))
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), `"code":"subagents_session_changed"`) || len(reader.sessions) != 0 {
		t.Fatalf("a chat that is not open: %d %s, reads %v", rr.Code, rr.Body.String(), reader.sessions)
	}

	rr = call(managementRequest("/api/subagents/report", `{"sessionId":"parent","executionId":"child-1"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Report eviedb.SubagentReport `json:"report"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Report.ExecutionID != "child-1" || body.Report.Status != "succeeded" || body.Report.Text == nil || *body.Report.Text != "## Summary\nHorizons double." {
		t.Fatalf("report = %s", rr.Body.String())
	}

	rr = call(managementRequest("/api/subagents/report", `{"sessionId":"parent","executionId":"elsewhere"}`))
	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), `"code":"subagent_not_found"`) {
		t.Fatalf("unknown subagent: %d %s", rr.Code, rr.Body.String())
	}
	for _, session := range reader.sessions {
		if session != "parent" {
			t.Fatalf("read another conversation's report: %v", reader.sessions)
		}
	}
	if len(reader.sessions) != 2 {
		t.Fatalf("reads = %v", reader.sessions)
	}
}

func TestSubagentReportRouteKeepsTheManagementGuard(t *testing.T) {
	server, reader := newSubagentServer("succeeded")
	body := `{"sessionId":"parent","executionId":"child-1"}`
	get := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687/api/subagents/report", nil)
	form := managementRequest("/api/subagents/report", body)
	form.Header.Set("Content-Type", "text/plain")
	foreign := managementRequest("/api/subagents/report", body)
	foreign.Header.Set("Origin", "https://attacker.example")
	rebound := httptest.NewRequest(http.MethodPost, "http://attacker.example/api/subagents/report", strings.NewReader(body))
	rebound.Header.Set("Content-Type", "application/json")
	for _, tc := range []struct {
		name   string
		req    *http.Request
		status int
	}{
		{"GET", get, http.StatusMethodNotAllowed},
		{"not JSON", form, http.StatusForbidden},
		{"foreign origin", foreign, http.StatusForbidden},
		{"foreign host", rebound, http.StatusForbidden},
		{"no subagent", managementRequest("/api/subagents/report", `{"sessionId":"parent"}`), http.StatusBadRequest},
	} {
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, tc.req)
		if rr.Code != tc.status {
			t.Fatalf("%s: status=%d want %d: %s", tc.name, rr.Code, tc.status, rr.Body.String())
		}
	}
	if len(reader.sessions) != 0 {
		t.Fatalf("refused requests read reports: %v", reader.sessions)
	}
}
