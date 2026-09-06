package usage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type localPosition struct {
	Offset  int64
	ModTime time.Time
	Size    int64
	Info    fs.FileInfo
}
type LocalSnapshot struct {
	Observations   []Observation
	Complete       bool
	Files          int
	FinishedFiles  int
	InvalidRecords int
	ScannedBytes   int64
	PendingFiles   int
}

// LocalIndex is a single-worker, in-memory index of token metadata only. Files
// remain the durable source; offsets accelerate polling without copying text.
type LocalIndex struct {
	home      string
	positions map[string]localPosition
	records   map[string]Observation
	conflicts map[string]bool
	invalid   int
}

func NewLocalIndex(home string) *LocalIndex {
	return &LocalIndex{home: home, positions: map[string]localPosition{}, records: map[string]Observation{}, conflicts: map[string]bool{}}
}

type localFile struct {
	path string
	info fs.FileInfo
}

func (index *LocalIndex) Scan(ctx context.Context, byteBudget int64) (LocalSnapshot, error) {
	files := []localFile{}
	for _, directory := range []string{"sessions", "archived_sessions"} {
		err := filepath.WalkDir(filepath.Join(index.home, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			files = append(files, localFile{path, info})
			if len(files) > 10000 {
				return ErrLimit
			}
			return nil
		})
		if err != nil {
			return index.snapshot(false, len(files), 0), err
		}
	}
	current := make(map[string]fs.FileInfo, len(files))
	for _, file := range files {
		current[file.path] = file.info
	}
	for path, position := range index.positions {
		info, exists := current[path]
		if !exists || info.Size() < position.Size ||
			(position.Info != nil && !os.SameFile(info, position.Info)) ||
			(info.Size() == position.Size && !info.ModTime().Equal(position.ModTime)) {
			// Removal, archive moves and rewrites invalidate inherited copies and
			// conflicts too. Rebuild within the usual scan budget from source files.
			index.positions = map[string]localPosition{}
			index.records = map[string]Observation{}
			index.conflicts = map[string]bool{}
			index.invalid = 0
			break
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].info.ModTime().Equal(files[j].info.ModTime()) {
			return files[i].path < files[j].path
		}
		return files[i].info.ModTime().After(files[j].info.ModTime())
	})
	finished := 0
	pending := 0
	var read int64
	for _, file := range files {
		position := index.positions[file.path]
		if position.Offset == file.info.Size() {
			finished++
			continue
		}
		if read >= byteBudget {
			continue
		}
		count, end, err := index.readFile(ctx, file.path, &position, byteBudget-read)
		position.Size = file.info.Size()
		position.ModTime = file.info.ModTime()
		position.Info = file.info
		index.positions[file.path] = position
		read += count
		if err != nil {
			return index.snapshot(false, len(files), finished), err
		}
		if position.Offset >= file.info.Size() || end {
			finished++
			if end && position.Offset < file.info.Size() {
				pending++
			}
		}
	}
	snapshot := index.snapshot(finished == len(files), len(files), finished)
	snapshot.PendingFiles = pending
	return snapshot, nil
}
func (index *LocalIndex) snapshot(complete bool, files, finished int) LocalSnapshot {
	values := make([]Observation, 0, len(index.records))
	for _, record := range index.records {
		values = append(values, record)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].At.Equal(values[j].At) {
			return values[i].ID < values[j].ID
		}
		return values[i].At.Before(values[j].At)
	})
	var scanned int64
	for _, position := range index.positions {
		scanned += position.Offset
	}
	return LocalSnapshot{Observations: values, Complete: complete, Files: files, FinishedFiles: finished, InvalidRecords: index.invalid, ScannedBytes: scanned}
}
func (index *LocalIndex) readFile(ctx context.Context, path string, position *localPosition, budget int64) (int64, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()
	if _, err := file.Seek(position.Offset, io.SeekStart); err != nil {
		return 0, false, err
	}
	reader := bufio.NewReaderSize(file, 64<<10)
	var consumed int64
	for consumed < budget {
		if err := ctx.Err(); err != nil {
			return consumed, false, err
		}
		// Unrelated long lines are streamed past in fixed-size fragments. Only
		// candidate usage records (never full prompts) can occupy the small buffer.
		var size int64
		var record []byte
		candidate := false
		oversized := false
		for {
			if err := ctx.Err(); err != nil {
				return consumed, false, err
			}
			part, readErr := reader.ReadSlice('\n')
			if size == 0 {
				prefix := part
				if len(prefix) > 256 {
					prefix = prefix[:256]
				}
				candidate = bytes.Contains(prefix, []byte(`"type":"token_usage_record"`)) || bytes.Contains(prefix, []byte(`"type": "token_usage_record"`))
			}
			size += int64(len(part))
			consumed += int64(len(part))
			if candidate && !oversized {
				if len(record)+len(part) > 256<<10 {
					oversized = true
					record = nil
				} else {
					record = append(record, part...)
				}
			}
			if errors.Is(readErr, bufio.ErrBufferFull) {
				continue
			}
			if errors.Is(readErr, io.EOF) {
				return consumed, true, nil
			} // Retry an unfinished line on the next scan.
			if readErr != nil {
				return consumed, false, readErr
			}
			if oversized {
				index.invalid++
			} else if candidate {
				if err := index.accept(record); err != nil {
					return consumed, false, err
				}
			}
			position.Offset += size
			break
		}
	}
	return consumed, false, nil
}
func (index *LocalIndex) accept(line []byte) error {
	var envelope struct {
		Timestamp string `json:"timestamp"`
		Type      string `json:"type"`
		Payload   struct {
			ID     string          `json:"response_id"`
			Thread string          `json:"thread_id"`
			Usage  json.RawMessage `json:"usage"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &envelope) != nil || envelope.Type != "token_usage_record" {
		index.invalid++
		return nil
	}
	at, err := time.Parse(time.RFC3339Nano, envelope.Timestamp)
	if err != nil || envelope.Payload.ID == "" || envelope.Payload.Thread == "" || len(envelope.Payload.ID) > 256 {
		index.invalid++
		return nil
	}
	var tokens Tokens
	if json.Unmarshal(envelope.Payload.Usage, &tokens) != nil {
		index.invalid++
		return nil
	}
	record := Observation{ID: envelope.Payload.ID, At: at, Tokens: tokens}
	if index.conflicts[record.ID] {
		return nil
	}
	if previous, exists := index.records[record.ID]; exists {
		if !sameObservation(previous, record) {
			delete(index.records, record.ID)
			index.conflicts[record.ID] = true
			index.invalid++
		}
		return nil
	}
	if len(index.records)+len(index.conflicts) >= MaxObservations {
		return ErrLimit
	}
	index.records[record.ID] = record
	return nil
}
func sameObservation(a, b Observation) bool {
	if !a.At.Equal(b.At) {
		return false
	}
	left := []*int64{a.Tokens.Input, a.Tokens.Cached, a.Tokens.Output, a.Tokens.Total, a.Tokens.Reasoning, a.Tokens.CacheWrite}
	right := []*int64{b.Tokens.Input, b.Tokens.Cached, b.Tokens.Output, b.Tokens.Total, b.Tokens.Reasoning, b.Tokens.CacheWrite}
	for i, x := range left {
		y := right[i]
		if x == nil || y == nil {
			if x != nil || y != nil {
				return false
			}
		} else if *x != *y {
			return false
		}
	}
	return true
}
