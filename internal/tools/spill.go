package tools

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// spillRetention bounds how long an oversized-output file survives. Spills
// are unique per call, so without a bound they pile up in the temp directory
// at up to 10MB a fetch. The model is told to read a spill with grep, head,
// or tail right away rather than refetch or rerun; a day covers that and a
// resumed session, and an expired path simply means fetch or run it again.
const spillRetention = 24 * time.Hour

// createSpillFile makes a unique 0600 file for one oversized result, after
// removing this prefix's spills older than spillRetention. Sweeping at
// creation time needs no background goroutine and touches the temp
// directory only when a new spill is about to be written anyway.
func createSpillFile(prefix string) (*os.File, error) {
	removeStaleSpillFiles(prefix, time.Now().Add(-spillRetention))
	return os.CreateTemp("", prefix+"*.txt")
}

// removeStaleSpillFiles deletes regular files named like this prefix's
// spills whose modification time is before cutoff. Best effort: a file it
// cannot stat or remove (another user's, on a shared /tmp) is skipped.
func removeStaleSpillFiles(prefix string, cutoff time.Time) {
	dir := os.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".txt") || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}
