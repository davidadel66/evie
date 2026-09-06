package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memoryeval"
)

type artifactEntry struct {
	SHA256 string `json:"sha256"`
	Path   string `json:"path"`
}

func verifyArtifacts(submission memoryeval.Stage4Submission, indexPath string) (memoryeval.Stage4EvidenceVerification, error) {
	var zero memoryeval.Stage4EvidenceVerification
	root, err := os.OpenRoot(filepath.Dir(indexPath))
	if err != nil {
		return zero, err
	}
	defer root.Close()
	data, err := readRootedArtifactFile(root, filepath.Base(indexPath), 1<<20)
	if err != nil {
		return zero, err
	}
	if !utf8.Valid(data) {
		return zero, errors.New("artifact index must be valid UTF-8")
	}
	tokenDecoder := json.NewDecoder(bytes.NewReader(data))
	if err = closedJSONValue(tokenDecoder, 0); err != nil {
		return zero, err
	}
	if _, err = tokenDecoder.Token(); err != io.EOF {
		return zero, errors.New("artifact index must contain exactly one JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var entries []artifactEntry
	if err = decoder.Decode(&entries); err != nil {
		return zero, err
	}
	required := memoryeval.Stage4RequiredEvidence(submission)
	if len(entries) != len(required) {
		return zero, errors.New("artifact index must list exactly the referenced receipt and output hashes")
	}

	protected := []string{}
	if p := submission.Plan; p != nil {
		protected = []string{p.CorpusSHA256, p.GoldSHA256, p.Configuration.ModelSHA256, p.BaselineConfiguration.ModelSHA256}
	}
	artifacts := map[string][]byte{}
	total := 0
	for _, entry := range entries {
		if !slices.Contains(required, entry.SHA256) || artifacts[entry.SHA256] != nil || slices.Contains(protected, entry.SHA256) {
			return zero, errors.New("unknown, duplicate, or protected artifact in receipt index")
		}
		if entry.Path == "" || filepath.IsAbs(entry.Path) || filepath.Clean(entry.Path) != entry.Path || entry.Path == ".." || strings.HasPrefix(entry.Path, ".."+string(filepath.Separator)) {
			return zero, errors.New("artifact paths must be clean relative paths below the index directory")
		}
		data, err := readRootedArtifactFile(root, entry.Path, 16<<20)
		if err != nil {
			return zero, err
		}
		total += len(data)
		if total > 64<<20 {
			return zero, errors.New("receipt artifact bytes exceed the 64 MiB evaluation-input bound")
		}
		artifacts[entry.SHA256] = data
	}
	return memoryeval.VerifyStage4Evidence(submission, artifacts)
}

func readRootedArtifactFile(root *os.Root, path string, limit int64) ([]byte, error) {
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	return readOpenedArtifact(file, limit)
}

func readOpenedArtifact(file *os.File, limit int64) ([]byte, error) {
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("receipt artifacts must be regular files")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("artifact exceeds %d-byte bound", limit)
	}
	return data, nil
}
