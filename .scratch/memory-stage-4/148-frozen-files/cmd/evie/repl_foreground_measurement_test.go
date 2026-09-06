package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type measuredREPLClient struct{ fail bool }

func (c measuredREPLClient) ChatStream(_ context.Context, _ openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	h.OnContent("done")
	if c.fail {
		return openrouter.ChatResponse{}, errors.New("fixture provider failed")
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "done"}}}}, nil
}

type measuredREPLOutput struct {
	bytes.Buffer
	failText                   string
	short, failFlush, failOnce bool
	flushes                    int
}

func (w *measuredREPLOutput) Write(p []byte) (int, error) {
	if w.failText != "" && strings.Contains(string(p), w.failText) {
		if w.failOnce {
			w.failText = ""
		}
		if w.short {
			return len(p) - 1, nil
		}
		return 0, errors.New("fixture output unavailable")
	}
	return w.Buffer.Write(p)
}

func (w *measuredREPLOutput) Flush() error {
	w.flushes++
	if w.failFlush {
		return errors.New("fixture flush unavailable")
	}
	return nil
}

func TestForegroundREPLMeasurementRequiresSuccessfulOutput(t *testing.T) {
	for _, tt := range []struct {
		name, failText                                     string
		short, failFlush, providerFailure, recoverNextTurn bool
	}{
		{name: "successful buffered output"},
		{name: "smooth printer write fails", failText: "done"},
		{name: "short write", failText: "done", short: true},
		{name: "final output write fails", failText: "\n"},
		{name: "flush fails", failFlush: true},
		{name: "provider failure output fails", failText: "request failed:", providerFailure: true},
		{name: "next turn recovers", failText: "done", recoverNextTurn: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "measurement.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := eviedb.NewStore(db)
			bound, err := store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			session := agent.New(measuredREPLClient{fail: tt.providerFailure}, evieTestContextProfile("fixture"),
				store.BindHistory(bound.ID, "measurement"), bound.ScopeContext(), store.BindTurnOwner(bound.ID, "measurement"))
			writer := &measuredREPLOutput{failText: tt.failText, short: tt.short, failFlush: tt.failFlush, failOnce: tt.recoverNextTurn}
			input, turns := "start\n", 1
			if tt.recoverNextTurn {
				input, turns = "start\nagain\n", 2
			}
			runREPLContextIO(ctx, session, bufio.NewScanner(strings.NewReader(input)), writer)
			authority, err := store.LocalOwnerReviewContext(ctx, "global")
			if err != nil {
				t.Fatal(err)
			}
			projection, err := store.InspectOwnerCompilerDiagnostics(ctx, authority, memory.CompilerDiagnosticsQuery{SessionID: bound.ID, View: "foreground"})
			if err != nil {
				t.Fatal(err)
			}
			if len(projection.Foreground) != turns {
				t.Fatalf("measurements %+v", projection.Foreground)
			}
			outcome := "success"
			if tt.providerFailure {
				outcome = "failed"
			}
			completed := 0
			for _, m := range projection.Foreground {
				if m.Outcome != outcome || m.TerminalCommittedAtUnixMS == nil || m.TerminalCommitNanos == nil {
					t.Fatalf("output failure changed committed turn: %+v", m)
				}
				if (m.ResponseFinalizedAtUnixMS != nil) != (m.ResponseFinalizationNanos != nil) {
					t.Fatalf("partial finalization observation %+v", m)
				}
				if m.ResponseFinalizedAtUnixMS != nil {
					completed++
				}
			}
			wantCompleted := 0
			if (tt.failText == "" && !tt.failFlush) || tt.recoverNextTurn {
				wantCompleted = 1
			}
			if completed != wantCompleted {
				t.Fatalf("completed outputs=%d, want %d: %+v", completed, wantCompleted, projection.Foreground)
			}
			if writer.flushes == 0 {
				t.Fatal("buffered host output was not flushed")
			}
		})
	}
}
