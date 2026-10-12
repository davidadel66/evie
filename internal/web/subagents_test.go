package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	server := WithSubagents(NewServer(nil), reader)
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
