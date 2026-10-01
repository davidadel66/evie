package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// T7: oversized-output files are not kept forever. Creating a new spill
// removes this tool's spills older than the retention window; recent ones,
// which a model may still be reading, and unrelated files are left alone.
func TestSpillFilesOlderThanRetentionAreRemoved(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	old := time.Now().Add(-spillRetention - time.Hour)
	file := func(name string, modified time.Time) string {
		t.Helper()
		path := filepath.Join(tmp, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
		return path
	}
	staleFetch := file("evie-fetch-stale.txt", old)
	freshFetch := file("evie-fetch-fresh.txt", time.Now())
	staleOutput := file("evie-output-stale.txt", old)
	freshOutput := file("evie-output-fresh.txt", time.Now())
	unrelated := file("notes-stale.txt", old)

	capped := capText(strings.Repeat("x", maxFetchOutput+10))
	newFetch := regexp.MustCompile(regexp.QuoteMeta(tmp) + `[^\s\]]+`).FindString(capped)
	if newFetch == "" {
		t.Fatalf("no spill path in %q", capped[maxFetchOutput:])
	}

	resetSessionCwd(t)
	out, err := runBashCommand(t, map[string]any{"command": "head -c 40000 /dev/zero | tr '\\0' z"})
	if err != nil {
		t.Fatal(err)
	}
	newOutput := bashSpillPath(t, out)

	for _, gone := range []string{staleFetch, staleOutput} {
		if _, err := os.Stat(gone); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("stale spill %s survived (stat err %v)", filepath.Base(gone), err)
		}
	}
	for _, kept := range []string{freshFetch, freshOutput, unrelated, newFetch, newOutput} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was removed: %v", filepath.Base(kept), err)
		}
	}
}
