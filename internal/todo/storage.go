package todo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadPath computes the JSON file's absolute path from the list's Dir and
// Name — the one place that mapping exists, used by both Save and Load.
func (list *TaskList) LoadPath() (string, error) {
	if list.Dir == "" {
		return "", errors.New("directory path is not set")
	}
	if list.Name == "" {
		return "", errors.New("name of task list is not set")
	}

	homePath, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to load user home directory: %w", err)
	}
	return filepath.Join(homePath, list.Dir, list.Name+".json"), nil
}

// Save writes the whole list back to disk as indented JSON, creating the
// directory if needed. Whole-file rewrite every time — at personal-list
// scale, simplicity beats incremental updates.
func (list *TaskList) Save() error {
	jsonBytes, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return fmt.Errorf("failed to convert tasks into JSON: %w", err)
	}

	localPath, err := list.LoadPath()
	if err != nil {
		return fmt.Errorf("failed to constuct local path: %w", err)
	}

	makeDirError := os.MkdirAll(filepath.Dir(localPath), 0o755)
	if makeDirError != nil {
		return fmt.Errorf("failed to create the directory: %w", makeDirError)
	}

	err = os.WriteFile(localPath, jsonBytes, 0o644)
	if err != nil {
		return fmt.Errorf("failed to save JSON: %w", err)
	}

	return nil
}

// Load reads the list from disk into the receiver. A missing file is not
// an error — it's the legitimate first-run state, and the zero-value list
// is correct: empty tasks, NextID starting fresh.
func (list *TaskList) Load() error {
	localPath, err := list.LoadPath()
	if err != nil {
		return fmt.Errorf("failed to constuct local path: %w", err)
	}
	data, err := os.ReadFile(localPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read tasklist: %w", err)
	}

	return json.Unmarshal(data, list)
}
