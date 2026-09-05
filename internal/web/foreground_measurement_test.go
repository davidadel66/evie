package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
)

type foregroundHTTPHistory struct {
	*fakeHistory
	records []memory.CompilerForegroundMeasurement
}

func (h *foregroundHTTPHistory) RecordCompilerForeground(_ context.Context, m memory.CompilerForegroundMeasurement) error {
	h.records = append(h.records, m)
	return nil
}

type failingForegroundResponse struct {
	*httptest.ResponseRecorder
	writeEvent, flushEvent, lastEvent string
	short                             bool
}

func (w *failingForegroundResponse) Write(p []byte) (int, error) {
	w.lastEvent = string(p)
	if w.writeEvent != "" && strings.Contains(string(p), "event: "+w.writeEvent+"\n") {
		if w.short {
			return len(p) - 1, nil
		}
		return 0, errors.New("fixture output unavailable")
	}
	return w.ResponseRecorder.Write(p)
}

func (w *failingForegroundResponse) FlushError() error {
	if w.flushEvent != "" && strings.Contains(w.lastEvent, "event: "+w.flushEvent+"\n") {
		return errors.New("fixture flush unavailable")
	}
	w.ResponseRecorder.Flush()
	return nil
}

func TestForegroundHTTPMeasurementRequiresSuccessfulOutput(t *testing.T) {
	for _, tt := range []struct {
		name, writeEvent, flushEvent string
		short, providerFailure       bool
	}{
		{name: "successful output"},
		{name: "early write fails", writeEvent: "delta"},
		{name: "final write fails", writeEvent: "turn_done"},
		{name: "short write", writeEvent: "assistant_done", short: true},
		{name: "early flush fails", flushEvent: "delta"},
		{name: "final flush fails", flushEvent: "turn_done"},
		{name: "provider failure output fails", writeEvent: "error", providerFailure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			history := &foregroundHTTPHistory{fakeHistory: &fakeHistory{}}
			step := fakeStep{deltas: []string{"done"}, content: "done"}
			outcome := "success"
			if tt.providerFailure {
				step.err = errors.New("fixture provider failed")
				outcome = "failed"
			}
			session := agent.New(&fakeClient{steps: []fakeStep{step}}, webTestContextProfile("test-model"), history,
				memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, webTestTurnOwner{})
			writer := &failingForegroundResponse{ResponseRecorder: httptest.NewRecorder(), writeEvent: tt.writeEvent, flushEvent: tt.flushEvent, short: tt.short}
			NewServer(session).Handler().ServeHTTP(writer, chatRequest(`{"message":"start"}`))
			if len(history.records) != 1 {
				t.Fatalf("measurements %d", len(history.records))
			}
			m := history.records[0]
			terminal := history.events[len(history.events)-1]
			wantType := memory.EventAssistantMessage
			if tt.providerFailure {
				wantType = memory.EventTurnFailed
			}
			if m.Outcome != outcome || m.TerminalCommittedAtUnixMS == nil || m.TerminalCommitNanos == nil || terminal.Type != wantType {
				t.Fatalf("output failure changed committed turn: %+v, terminal=%+v", m, terminal)
			}
			complete := tt.writeEvent == "" && tt.flushEvent == ""
			if (m.ResponseFinalizedAtUnixMS != nil) != complete || (m.ResponseFinalizationNanos != nil) != complete {
				t.Fatalf("output completion=%v, measurement %+v", complete, m)
			}
		})
	}
}
