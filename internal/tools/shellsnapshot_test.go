package tools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func resetSnapshotForTest(t *testing.T) {
	t.Helper()
	snapshotMu.Lock()
	oldPath := snapshotPath
	oldAttempted := snapshotAttempted
	oldCapture := snapshotCapture
	oldFunc := captureShellFunc
	snapshotPath = ""
	snapshotAttempted = false
	snapshotCapture = nil
	snapshotMu.Unlock()
	if oldCapture != nil {
		oldCapture.cancel()
		<-oldCapture.done
	}
	t.Cleanup(func() {
		snapshotMu.Lock()
		path := snapshotPath
		snapshotPath = oldPath
		snapshotAttempted = oldAttempted
		snapshotCapture = nil
		captureShellFunc = oldFunc
		snapshotMu.Unlock()
		if path != "" && path != oldPath {
			_ = os.Remove(path)
		}
	})
}

func TestCancelledLazySnapshotRemainsRetryable(t *testing.T) {
	resetSnapshotForTest(t)
	started := make(chan struct{})
	finished := make(chan struct{})
	captureShellFunc = func(ctx context.Context, _ string) (string, error) {
		close(started)
		defer close(finished)
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan string, 1)
	go func() { done <- snapshot(ctx) }()
	<-started
	cancel()
	if got := <-done; got != "" {
		t.Fatalf("cancelled snapshot = %q", got)
	}
	<-finished

	want := filepath.Join(t.TempDir(), "snapshot.sh")
	if err := os.WriteFile(want, []byte("export PATH=/bin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	captureShellFunc = func(context.Context, string) (string, error) { return want, nil }
	if got := snapshot(context.Background()); got != want {
		t.Fatalf("retry snapshot = %q, want %q", got, want)
	}
}

func TestCancelledCaptureDoesNotPublishSuccessfulBoundaryResult(t *testing.T) {
	resetSnapshotForTest(t)
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	produced := filepath.Join(t.TempDir(), "cancelled-success.sh")
	if err := os.WriteFile(produced, []byte("export PATH=/cancelled\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	captureShellFunc = func(context.Context, string) (string, error) {
		close(started)
		<-release
		close(finished)
		return produced, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan string, 1)
	go func() { done <- snapshot(ctx) }()
	<-started
	cancel()
	if got := <-done; got != "" {
		t.Fatalf("cancelled snapshot = %q", got)
	}
	close(release)
	<-finished
	waitForSnapshotCaptureToFinish(t)

	snapshotMu.Lock()
	gotPath := snapshotPath
	gotAttempted := snapshotAttempted
	snapshotMu.Unlock()
	if gotPath != "" || gotAttempted {
		t.Fatalf("cancelled capture published path=%q attempted=%v", gotPath, gotAttempted)
	}
	if _, err := os.Stat(produced); !os.IsNotExist(err) {
		t.Fatalf("cancelled capture output was not removed: %v", err)
	}

	want := filepath.Join(t.TempDir(), "retry.sh")
	if err := os.WriteFile(want, []byte("export PATH=/retry\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	captureShellFunc = func(context.Context, string) (string, error) { return want, nil }
	if got := snapshot(context.Background()); got != want {
		t.Fatalf("retry snapshot = %q, want %q", got, want)
	}
}

func TestOrdinarySnapshotFailureRemainsSticky(t *testing.T) {
	resetSnapshotForTest(t)
	calls := 0
	captureShellFunc = func(context.Context, string) (string, error) {
		calls++
		return "", errors.New("ordinary capture failure")
	}
	if got := snapshot(context.Background()); got != "" {
		t.Fatalf("first failed snapshot = %q", got)
	}
	if got := snapshot(context.Background()); got != "" {
		t.Fatalf("second failed snapshot = %q", got)
	}
	if calls != 1 {
		t.Fatalf("capture attempts = %d, want sticky single ordinary failure", calls)
	}
}

func TestSnapshotWaitersCancelIndependently(t *testing.T) {
	resetSnapshotForTest(t)
	started := make(chan struct{})
	release := make(chan struct{})
	want := filepath.Join(t.TempDir(), "snapshot.sh")
	captureShellFunc = func(ctx context.Context, _ string) (string, error) {
		close(started)
		select {
		case <-release:
			return want, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan string, 1)
	second := make(chan string, 1)
	go func() { first <- snapshot(ctx) }()
	<-started
	go func() { second <- snapshot(context.Background()) }()
	waitForSnapshotWaiters(t, 2)
	cancel()
	if got := <-first; got != "" {
		t.Fatalf("cancelled waiter = %q", got)
	}
	close(release)
	if got := <-second; got != want {
		t.Fatalf("live waiter = %q, want %q", got, want)
	}
	if ctx.Err() == nil {
		t.Fatal("test context was not cancelled")
	}
}

func waitForSnapshotWaiters(t *testing.T, want int) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			snapshotMu.Lock()
			capture := snapshotCapture
			got := 0
			if capture != nil {
				got = capture.waiters
			}
			snapshotMu.Unlock()
			if got >= want {
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("snapshot waiter count did not reach %d", want)
	}
}

func waitForSnapshotCaptureToFinish(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			snapshotMu.Lock()
			active := snapshotCapture != nil
			snapshotMu.Unlock()
			if !active {
				return
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("snapshot capture did not finish")
	}
}

func TestWarmCaptureContinuesAfterTurnWaiterCancels(t *testing.T) {
	resetSnapshotForTest(t)
	started := make(chan struct{})
	release := make(chan struct{})
	want := filepath.Join(t.TempDir(), "warm.sh")
	captureShellFunc = func(ctx context.Context, _ string) (string, error) {
		close(started)
		select {
		case <-release:
			return want, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	Warm()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := snapshot(ctx); got != "" {
		t.Fatalf("cancelled turn waiter = %q", got)
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshotMu.Lock()
		got := snapshotPath
		snapshotMu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("startup Warm capture was cancelled with the turn waiter")
}

// The snapshot exists to carry interactive-only definitions — aliases and
// functions from .zshrc / .bashrc — into non-interactive command shells.
// This drives a real shell against a throwaway HOME and asserts both make
// it into the file.
func TestCaptureShell(t *testing.T) {
	shell, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash not available: %v", err)
	}

	home := t.TempDir()
	rc := "alias evietest='echo aliased'\neviefunc() { echo functioned; }\nexport EVIE_PROBE=1\n"
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(rc), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("HOME", home)

	path, err := captureShell(context.Background(), shell)
	if err != nil {
		t.Fatalf("captureShell: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	got := string(data)

	if !strings.Contains(got, "evietest") {
		t.Errorf("snapshot missing the alias:\n%s", got)
	}
	if !strings.Contains(got, "eviefunc") {
		t.Errorf("snapshot missing the function:\n%s", got)
	}
	if !strings.Contains(got, "export PATH=") {
		t.Errorf("snapshot missing PATH:\n%s", got)
	}
	if !strings.Contains(got, "expand_aliases") {
		t.Errorf("snapshot does not re-enable aliases for non-interactive shells:\n%s", got)
	}
}

// A snapshot is only useful if sourcing it actually makes the definitions
// usable, which is what the eval second-parse trick buys.
func TestSnapshotDefinitionsAreUsable(t *testing.T) {
	shell, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash not available: %v", err)
	}

	home := t.TempDir()
	rc := "alias evietest='echo aliased'\neviefunc() { echo functioned; }\n"
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(rc), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("HOME", home)

	snap, err := captureShell(context.Background(), shell)
	if err != nil {
		t.Fatalf("captureShell: %v", err)
	}
	defer os.Remove(snap)

	for _, tc := range []struct{ command, want string }{
		{"evietest", "aliased"},
		{"eviefunc", "functioned"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			script := "source " + shellQuote(snap) + " 2>/dev/null || true\neval " + shellQuote(tc.command) + "\n"
			out, err := exec.Command(shell, "-l", "-c", script).CombinedOutput()
			if err != nil {
				t.Fatalf("running %q: %v\n%s", tc.command, err, out)
			}
			if !strings.Contains(string(out), tc.want) {
				t.Errorf("output %q does not contain %q", out, tc.want)
			}
		})
	}
}
