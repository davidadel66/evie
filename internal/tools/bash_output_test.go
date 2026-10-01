package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func bashSpillPath(t *testing.T, output string) string {
	t.Helper()
	_, rest, found := strings.Cut(output, "full output saved to ")
	if !found {
		t.Fatalf("no spill path in %q", output[max(len(output)-300, 0):])
	}
	path, _, _ := strings.Cut(rest, " ")
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func runBashCommand(t *testing.T, params map[string]any) (string, error) {
	t.Helper()
	args, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return runBash(context.Background(), string(args))
}

// T5: the output collector holds at most the cap in memory and streams the
// rest to the spill file, which ends up with every byte.
func TestBashOutputHoldsAtMostTheCapInMemory(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	var collected boundedOutput
	chunk := bytes.Repeat([]byte("0123456789"), 1000)
	var want bytes.Buffer
	for i := 0; i < 100; i++ {
		n, err := collected.Write(chunk)
		if n != len(chunk) || err != nil {
			t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(chunk))
		}
		want.Write(chunk)
		if cap(collected.head) > maxBashOutput {
			t.Fatalf("collector holds %d bytes in memory after %d bytes written; the cap is %d", cap(collected.head), want.Len(), maxBashOutput)
		}
	}
	collected.finish()
	rendered := collected.render()
	if len(rendered) > maxBashOutput+500 {
		t.Fatalf("rendered output is %d bytes, want the cap plus a note", len(rendered))
	}
	data, err := os.ReadFile(bashSpillPath(t, rendered))
	if err != nil {
		t.Fatalf("read spill: %v", err)
	}
	if !bytes.Equal(data, want.Bytes()) {
		t.Fatalf("spill holds %d bytes, want all %d", len(data), want.Len())
	}
}

// T5 end to end: a command killed at its timeout returns bounded partial
// output with a pointer to the rest, not everything it printed.
func TestRunBashTimeoutOutputIsBounded(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	resetSessionCwd(t)
	_, err := runBashCommand(t, map[string]any{
		"command":         "head -c 200000 /dev/zero | tr '\\0' x; sleep 5",
		"timeout_seconds": 1,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want a timeout", err)
	}
	if len(err.Error()) > maxBashOutput+1000 {
		t.Fatalf("timeout error carries %d bytes of output, want at most the cap plus a note", len(err.Error()))
	}
	data, readErr := os.ReadFile(bashSpillPath(t, err.Error()))
	if readErr != nil || len(data) != 200000 {
		t.Fatalf("spill holds %d bytes (%v), want 200000", len(data), readErr)
	}
}

// T6: every capped call gets its own spill file, so a second call cannot
// overwrite the first one's output while the model is still reading it.
func TestRunBashSpillFilesAreUniquePerCall(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	resetSessionCwd(t)
	first, err := runBashCommand(t, map[string]any{"command": "head -c 40000 /dev/zero | tr '\\0' a"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := runBashCommand(t, map[string]any{"command": "head -c 40000 /dev/zero | tr '\\0' b"})
	if err != nil {
		t.Fatal(err)
	}
	firstPath, secondPath := bashSpillPath(t, first), bashSpillPath(t, second)
	if firstPath == secondPath {
		t.Fatalf("both calls spilled to %s", firstPath)
	}
	data, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != strings.Repeat("a", 40000) {
		t.Fatalf("first spill was overwritten: starts %q", data[:min(len(data), 20)])
	}
}

// T6: the inline cut never splits a UTF-8 rune.
func TestRunBashCutIsUTF8Safe(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	resetSessionCwd(t)
	// "é" (two bytes) starts at maxBashOutput-1, so a byte cut at the cap
	// slices it in half.
	got, err := runBashCommand(t, map[string]any{
		"command": "head -c 29999 /dev/zero | tr '\\0' a; printf '\\303\\251'; head -c 5000 /dev/zero | tr '\\0' b",
	})
	if err != nil {
		t.Fatal(err)
	}
	bashSpillPath(t, got)
	if !utf8.ValidString(got) || strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("bash output split a rune at the cap: %q", got[maxBashOutput-10:maxBashOutput+10])
	}
}
