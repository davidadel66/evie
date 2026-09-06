package main

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type compilerForegroundClient struct{}

func (compilerForegroundClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "Noted."}}}}, nil
}

func compilerForegroundSession(f *compilerRuntimeFixture) *agent.Session {
	return agent.New(compilerForegroundClient{}, evieTestContextProfile("fixture"),
		f.store.BindHistory(f.session.ID, "foreground"), f.session.ScopeContext(),
		f.store.BindTurnOwner(f.session.ID, "foreground"))
}

func runCompilerForegroundTurn(t *testing.T, ctx context.Context, session *agent.Session, input string) {
	t.Helper()
	finished := make(chan struct{})
	started := time.Now()
	go func() {
		defer close(finished)
		runREPLContextIO(ctx, session, bufio.NewScanner(strings.NewReader(input+"\n")), io.Discard)
	}()
	select {
	case <-finished:
		t.Logf("REPL foreground finalization=%s", time.Since(started))
	case <-time.After(2 * time.Second):
		t.Fatal("foreground REPL awaited background extraction")
	}
}

func TestCompilerConfiguredHostDoesNotBlockREPLDuringStalledExtraction(t *testing.T) {
	extractionStarted := make(chan struct{})
	extractionCancelled := make(chan struct{})
	release := make(chan struct{})
	f := newCompilerRuntimeFixture(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(extractionStarted)
		select {
		case <-r.Context().Done():
			close(extractionCancelled)
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	f.activate(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stop, err := startConfiguredCompilerHost(ctx, f.configPath, f.store)
	if err != nil {
		t.Fatal(err)
	}
	stop = sync.OnceValue(stop)
	t.Cleanup(func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	})
	session := compilerForegroundSession(f)
	runCompilerForegroundTurn(t, ctx, session, "I prefer tea.")
	select {
	case <-extractionStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("configured host did not begin selected extraction")
	}
	runCompilerForegroundTurn(t, ctx, session, "I also prefer coffee.")
	var finals int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM events WHERE session_id=? AND event_type='assistant_message'`, f.session.ID).Scan(&finals); err != nil {
		t.Fatal(err)
	}
	if finals != 2 || f.inferences.Load() != 1 {
		t.Fatalf("foreground finalization=%d inference requests=%d", finals, f.inferences.Load())
	}
	cancel()
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-extractionCancelled:
	case <-time.After(time.Second):
		t.Fatal("host shutdown did not cancel active extractor HTTP request")
	}
}

func TestCompilerUnconfiguredHostLeavesREPLAvailableWithoutPendingJobs(t *testing.T) {
	f := newCompilerRuntimeFixture(t, nil)
	stop, err := startConfiguredCompilerHost(context.Background(), "", f.store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	runCompilerForegroundTurn(t, context.Background(), compilerForegroundSession(f), "I prefer tea.")
	var finals, jobs, activations int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type='assistant_message'`).Scan(&finals); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_jobs`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_activations`).Scan(&activations); err != nil {
		t.Fatal(err)
	}
	if finals != 1 || jobs != 0 || activations != 0 || f.inferences.Load() != 0 {
		t.Fatalf("unconfigured foreground finals=%d jobs=%d activations=%d inference=%d", finals, jobs, activations, f.inferences.Load())
	}
}

func TestCompilerUnavailableConfiguredHostLeavesREPLAvailable(t *testing.T) {
	f := newCompilerRuntimeFixture(t, nil)
	f.activate(t)
	f.server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := startConfiguredCompilerHost(ctx, f.configPath, f.store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	runCompilerForegroundTurn(t, ctx, compilerForegroundSession(f), "I prefer tea.")
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var retrying int
		if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_jobs WHERE state='retry_wait' AND attempts=1`).Scan(&retrying); err != nil {
			t.Fatal(err)
		}
		if retrying == 1 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("configured unavailable endpoint did not leave bounded retry state")
		}
	}
	var finals int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type='assistant_message'`).Scan(&finals); err != nil {
		t.Fatal(err)
	}
	if finals != 1 || f.inferences.Load() != 0 {
		t.Fatalf("unavailable foreground finals=%d inference=%d", finals, f.inferences.Load())
	}
}

func TestRuntimeCancellationReleasesREPLScannerWithoutMoreInput(t *testing.T) {
	for _, waiting := range []string{"prompt", "approval"} {
		t.Run(waiting, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			barrier := newScanReadBarrier(reader)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stopInput := closeInputOnCancellation(ctx, reader)
			defer stopInput()
			history := &recordingREPLHistory{}
			var client agent.Client = &replCancellationClient{}
			if waiting == "approval" {
				path := filepath.Join(t.TempDir(), "note.txt")
				if err := os.WriteFile(path, []byte("hello world"), 0o600); err != nil {
					t.Fatal(err)
				}
				client = &replApprovalClient{called: make(chan struct{}), path: path}
			}
			session := agent.New(client, evieTestContextProfile("fixture"), history,
				memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"}, testTurnOwner{})
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				runREPLContextIO(ctx, session, bufio.NewScanner(barrier), io.Discard)
			}()
			waitForScannerRead(t, barrier, 1)
			if waiting == "approval" {
				if _, err := io.WriteString(writer, "edit it\n"); err != nil {
					t.Fatal(err)
				}
				waitForScannerRead(t, barrier, 2)
			}
			cancel()
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("runtime cancellation left scanner blocked")
			}
			for _, event := range history.events {
				if event.Type == memory.EventApproval || event.Type == memory.EventToolSucceeded {
					t.Fatal("cancelled input produced an approval or tool success")
				}
			}
		})
	}
}
