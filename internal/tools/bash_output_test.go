package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func bashSpillPath(t *testing.T, output string) string {
	t.Helper()
	_, rest, found := strings.Cut(output, " saved to ")
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

// Final pass: the spill file has a ceiling. Past it the collector stops
// writing but keeps accepting output, so the command is drained rather than
// blocked or killed, and the note says where saving stopped.
func TestBashOutputSpillStopsAtTheCeiling(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	var collected boundedOutput
	chunk := bytes.Repeat([]byte("a"), 1<<20)
	written := int64(0)
	for written <= maxBashSpill+4<<20 {
		n, err := collected.Write(chunk)
		if n != len(chunk) || err != nil {
			t.Fatalf("Write past the ceiling = (%d, %v), want (%d, nil) so the command keeps draining", n, err, len(chunk))
		}
		written += int64(n)
	}
	collected.finish()
	rendered := collected.render()
	info, err := os.Stat(bashSpillPath(t, rendered))
	if err != nil {
		t.Fatalf("stat spill: %v", err)
	}
	if info.Size() != maxBashSpill {
		t.Fatalf("spill holds %d bytes after %d written, want the %d-byte ceiling", info.Size(), written, maxBashSpill)
	}
	if !strings.Contains(rendered, "of "+strconv.FormatInt(written, 10)+" characters shown") {
		t.Errorf("note does not count all %d bytes the command wrote: %q", written, rendered[maxBashOutput:])
	}
	if !strings.Contains(rendered, "first "+strconv.FormatInt(maxBashSpill>>20, 10)+" MiB") {
		t.Errorf("note does not say the saved output stops at the ceiling: %q", rendered[maxBashOutput:])
	}
}

// Final pass end to end: a command that writes past the ceiling still runs
// to completion with its real exit status, and the spill is bounded.
func TestRunBashSpillCeilingDrainsTheCommand(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	resetSessionCwd(t)
	restore := maxBashSpill
	maxBashSpill = 1 << 20
	t.Cleanup(func() { maxBashSpill = restore })

	got, err := runBashCommand(t, map[string]any{
		"command": "head -c 5000000 /dev/zero | tr '\\0' a; exit 3",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasSuffix(got, "exit status: 3\n") {
		t.Fatalf("command did not run to completion past the ceiling: %q", got[max(len(got)-300, 0):])
	}
	info, err := os.Stat(bashSpillPath(t, got))
	if err != nil {
		t.Fatalf("stat spill: %v", err)
	}
	if info.Size() != maxBashSpill {
		t.Fatalf("spill holds %d bytes, want the %d-byte ceiling", info.Size(), maxBashSpill)
	}
	if !strings.Contains(got, "of 5000000 characters shown") {
		t.Errorf("note does not count all output: %q", got[maxBashOutput:])
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
