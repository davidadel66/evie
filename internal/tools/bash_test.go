package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBash(t *testing.T) {
	call := func(params map[string]any) (string, error) {
		args, _ := json.Marshal(params)
		return runBash(string(args))
	}

	t.Run("returns stdout and a zero exit status", func(t *testing.T) {
		got, err := call(map[string]any{"command": "echo hello"})
		if err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		if !strings.Contains(got, "hello") {
			t.Errorf("output %q missing stdout", got)
		}
		if !strings.Contains(got, "exit status: 0") {
			t.Errorf("output %q missing exit status 0", got)
		}
	})

	// The load-bearing case: a failing command is a result the model reads,
	// not a Go error the dispatcher buries behind "tool call came back with".
	t.Run("a failing command is a result, not an error", func(t *testing.T) {
		got, err := call(map[string]any{"command": "echo oops >&2; exit 3"})
		if err != nil {
			t.Fatalf("runBash returned error for a non-zero exit: %v", err)
		}
		if !strings.Contains(got, "oops") {
			t.Errorf("output %q missing stderr", got)
		}
		if !strings.Contains(got, "exit status: 3") {
			t.Errorf("output %q missing exit status 3", got)
		}
	})

	t.Run("stderr and stdout are combined", func(t *testing.T) {
		got, err := call(map[string]any{"command": "echo out; echo err >&2"})
		if err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		if !strings.Contains(got, "out") || !strings.Contains(got, "err") {
			t.Errorf("output %q missing one of the streams", got)
		}
	})

	t.Run("shell features work", func(t *testing.T) {
		got, err := call(map[string]any{"command": "printf 'a\\nb\\nc\\n' | grep -c ."})
		if err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		if !strings.Contains(got, "3") {
			t.Errorf("output %q — pipe did not work", got)
		}
	})

	t.Run("cwd runs the command elsewhere", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "marker.txt"), nil, 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		got, err := call(map[string]any{"command": "ls", "cwd": dir})
		if err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		if !strings.Contains(got, "marker.txt") {
			t.Errorf("output %q — cwd was not honoured", got)
		}
	})

	t.Run("cwd expands a leading tilde", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home directory: %v", err)
		}

		got, err := call(map[string]any{"command": "pwd", "cwd": "~"})
		if err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		if !strings.Contains(got, home) {
			t.Errorf("output %q does not report %q", got, home)
		}
	})

	// The session working directory follows the shell, so the model can cd
	// into a project once and stay there.
	t.Run("cd persists into the next call", func(t *testing.T) {
		resetSessionCwd(t)

		dir := t.TempDir()
		if _, err := call(map[string]any{"command": "cd " + dir}); err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}

		got, err := call(map[string]any{"command": "pwd"})
		if err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		// t.TempDir() on macOS lives under /var, a symlink to /private/var;
		// pwd -P reports the physical path, so compare on the suffix.
		if !strings.Contains(got, filepath.Base(dir)) {
			t.Errorf("output %q — cd did not persist into the next call", got)
		}
	})

	t.Run("an explicit cwd becomes the new session directory", func(t *testing.T) {
		resetSessionCwd(t)

		dir := t.TempDir()
		if _, err := call(map[string]any{"command": "pwd", "cwd": dir}); err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}

		got, err := call(map[string]any{"command": "pwd"})
		if err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		if !strings.Contains(got, filepath.Base(dir)) {
			t.Errorf("output %q — explicit cwd did not stick", got)
		}
	})

	// A remembered directory that has since been deleted must not poison
	// every later command.
	t.Run("a deleted session directory is discarded", func(t *testing.T) {
		resetSessionCwd(t)

		dir, err := os.MkdirTemp("", "moussa-gone-*")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if _, err := call(map[string]any{"command": "pwd", "cwd": dir}); err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("cleanup: %v", err)
		}

		got, err := call(map[string]any{"command": "pwd"})
		if err != nil {
			t.Fatalf("runBash returned error after its directory vanished: %v", err)
		}
		if !strings.Contains(got, "exit status: 0") {
			t.Errorf("output %q — recovery did not produce a clean run", got)
		}
	})

	t.Run("oversized output is trimmed and spilled to a file", func(t *testing.T) {
		resetSessionCwd(t)

		got, err := call(map[string]any{
			"command": fmt.Sprintf("printf 'x%%.0s' $(seq 1 %d)", maxBashOutput+5000),
		})
		if err != nil {
			t.Fatalf("runBash returned error: %v", err)
		}
		if !strings.Contains(got, "output trimmed") {
			t.Errorf("output was not trimmed")
		}

		_, rest, found := strings.Cut(got, "full output saved to ")
		if !found {
			t.Fatalf("no spill path in %q", got[len(got)-200:])
		}
		path, _, _ := strings.Cut(rest, " ")
		defer os.Remove(path)

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("spill file missing: %v", err)
		}
		if info.Size() <= maxBashOutput {
			t.Errorf("spill file is %d bytes, want more than %d", info.Size(), maxBashOutput)
		}
	})

	t.Run("timeout kills the command and reports partial output", func(t *testing.T) {
		_, err := call(map[string]any{
			"command":         "echo starting; sleep 5",
			"timeout_seconds": 1,
		})
		if err == nil {
			t.Fatal("runBash succeeded on a command that should have timed out")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("error %v does not mention the timeout", err)
		}
		if !strings.Contains(err.Error(), "starting") {
			t.Errorf("error %v drops the partial output", err)
		}
	})

	t.Run("empty command errors", func(t *testing.T) {
		if _, err := call(map[string]any{"command": "   "}); err == nil {
			t.Fatal("runBash succeeded on an empty command")
		}
	})

	t.Run("unusable cwd errors", func(t *testing.T) {
		if _, err := call(map[string]any{"command": "pwd", "cwd": "~alice/projects"}); err == nil {
			t.Fatal("runBash succeeded on a ~user cwd")
		}
	})

	t.Run("malformed arguments error", func(t *testing.T) {
		if _, err := runBash("not json"); err == nil {
			t.Fatal("runBash succeeded on malformed arguments")
		}
	})
}

// resetSessionCwd clears the persistent working directory so a subtest
// starts from moussa's own directory regardless of what ran before it.
func resetSessionCwd(t *testing.T) {
	t.Helper()
	sessionCwdMu.Lock()
	sessionCwd = ""
	sessionCwdMu.Unlock()
}
