package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

	path, err := captureShell(shell)
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

	snap, err := captureShell(shell)
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
