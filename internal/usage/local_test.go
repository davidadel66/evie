package usage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func localRecord(id string, input string) string {
	return `{"timestamp":"2026-09-01T12:00:00Z","type":"token_usage_record","payload":{"response_id":"` + id + `","thread_id":"task","usage":{"input_tokens":` + input + `,"cached_input_tokens":50,"output_tokens":10,"total_tokens":110}}}` + "\n"
}
func TestLocalIndexDeduplicatesCopiesAndResumesPartialWrites(t *testing.T) {
	home := t.TempDir()
	sessions := filepath.Join(home, "sessions")
	archive := filepath.Join(home, "archived_sessions")
	_ = os.MkdirAll(sessions, 0700)
	_ = os.MkdirAll(archive, 0700)
	path := filepath.Join(sessions, "one.jsonl")
	first := localRecord("one", "100")
	second := localRecord("two", "100")
	// Huge unrelated messages never become cached content; legacy snapshots are excluded.
	content := `{"type":"response_item","payload":"` + strings.Repeat("secret", 30000) + `"}` + "\n" + first + second[:len(second)-4]
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	index := NewLocalIndex(home)
	snapshot, err := index.Scan(context.Background(), 1<<20)
	if err != nil || !snapshot.Complete || snapshot.PendingFiles != 1 || len(snapshot.Observations) != 1 {
		t.Fatalf("%+v %v", snapshot, err)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString(second[len(second)-4:])
	_ = f.Close()
	_ = os.WriteFile(filepath.Join(archive, "copy.jsonl"), []byte(first), 0600)
	snapshot, err = index.Scan(context.Background(), 1<<20)
	if err != nil || !snapshot.Complete || len(snapshot.Observations) != 2 {
		t.Fatalf("%+v %v", snapshot, err)
	}
	snapshot, err = index.Scan(context.Background(), 1<<20)
	if err != nil || len(snapshot.Observations) != 2 {
		t.Fatal("poll duplicated records")
	}
	// A contradictory duplicate must not silently win by directory order.
	_ = os.WriteFile(filepath.Join(archive, "conflict.jsonl"), []byte(localRecord("one", "999")), 0600)
	snapshot, err = index.Scan(context.Background(), 1<<20)
	if err != nil || len(snapshot.Observations) != 1 || snapshot.InvalidRecords != 1 {
		t.Fatalf("%+v %v", snapshot, err)
	}
}
func TestLocalIndexBoundsWorkAndHonorsCancellation(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(filepath.Join(home, "sessions"), 0700)
	_ = os.WriteFile(filepath.Join(home, "sessions", "records.jsonl"), []byte(localRecord("one", "100")+localRecord("two", "100")), 0600)
	index := NewLocalIndex(home)
	snapshot, err := index.Scan(context.Background(), 1)
	if err != nil || snapshot.Complete || len(snapshot.Observations) != 1 {
		t.Fatalf("%+v %v", snapshot, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := index.Scan(ctx, 1<<20); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestLocalIndexRebuildsAfterSourceRemovalAndReplacement(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "sessions")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	first, copyPath := filepath.Join(directory, "first.jsonl"), filepath.Join(directory, "copy.jsonl")
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(first, localRecord("retained", "100")+localRecord("removed", "100"))
	write(copyPath, localRecord("retained", "100"))
	index := NewLocalIndex(home)
	assertIDs := func(want string) {
		t.Helper()
		snapshot, err := index.Scan(context.Background(), 1<<20)
		if err != nil || !snapshot.Complete {
			t.Fatalf("%+v %v", snapshot, err)
		}
		ids := []string{}
		for _, observation := range snapshot.Observations {
			ids = append(ids, observation.ID)
		}
		if strings.Join(ids, ",") != want {
			t.Fatalf("IDs %v, want %s", ids, want)
		}
	}
	assertIDs("removed,retained")
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	assertIDs("retained")
	// An atomic replacement can grow as well as shrink.
	replacement := filepath.Join(home, "replacement")
	write(replacement, localRecord("replacement-longer", "100")+localRecord("second", "100"))
	if err := os.Rename(replacement, copyPath); err != nil {
		t.Fatal(err)
	}
	assertIDs("replacement-longer,second")
	write(copyPath, localRecord("shorter", "100"))
	assertIDs("shorter")
}

func TestLocalIndexRetainsLimitAcrossPolls(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "sessions")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "full.jsonl")
	if err := os.WriteFile(path, []byte(localRecord("new", "100")), 0600); err != nil {
		t.Fatal(err)
	}
	index := NewLocalIndex(home)
	for i := 0; i < MaxObservations; i++ {
		index.conflicts[strconv.Itoa(i)] = true
	}
	for i := 0; i < 2; i++ {
		snapshot, err := index.Scan(context.Background(), 1<<20)
		if !errors.Is(err, ErrLimit) || snapshot.Complete || index.positions[path].Offset != 0 {
			t.Fatalf("poll %d lost limit or checkpointed rejected record: %+v %v", i, snapshot, err)
		}
	}
}
