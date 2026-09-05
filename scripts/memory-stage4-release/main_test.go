package main

import (
	"bytes"
	"encoding/json"
	"github.com/davidadel66/evie/internal/memoryeval"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandPublishesPendingReportWithoutOpeningMissingHoldout(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "submission.json")
	output := filepath.Join(dir, "report.json")
	if err := os.WriteFile(input, []byte(`{"version":"memory-stage4-release-submission-v1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-input", input, "-output", output}, &stdout, &stderr); code != 2 {
		t.Fatalf("code %d: %s", code, stderr.String())
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		FinalRunEvidenceSupplied bool `json:"final_run_evidence_supplied"`
		ToolingOnly              bool `json:"tooling_only"`
		Report                   struct {
			Ready  bool   `json:"ready_for_ongoing_compilation"`
			Status string `json:"status"`
		} `json:"report"`
	}
	if err = json.Unmarshal(before, &got); err != nil {
		t.Fatal(err)
	}
	if got.FinalRunEvidenceSupplied || !got.ToolingOnly || got.Report.Ready || got.Report.Status != "pending" {
		t.Fatalf("pending tooling was misrepresented: %s", before)
	}
	if code := run([]string{"-input", input, "-output", output}, &stdout, &stderr); code != 1 {
		t.Fatalf("existing report overwritten: %d", code)
	}
	after, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("immutable report bytes changed")
	}
}

func TestCommandRejectsAmbiguousOrUnknownMetadataWithoutPublishing(t *testing.T) {
	cases := []string{
		`{"version":"memory-stage4-release-submission-v1","version":"other"}`,
		`{"version":"memory-stage4-release-submission-v1","holdout_source_path":"must-not-open"}`,
		`{"version":"memory-stage4-release-submission-v1"} {}`,
		`{"version":"memory-stage4-release-submission-v1","plan":{"id":"a","id":"b"}}`,
		"{\"version\":\"\xff\"}",
		strings.Repeat("[", 66) + strings.Repeat("]", 66),
	}
	for _, data := range cases {
		t.Run(data[:min(40, len(data))], func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input.json")
			output := filepath.Join(dir, "out.json")
			if err := os.WriteFile(input, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if code := run([]string{"-input", input, "-output", output}, &stdout, &stderr); code != 1 {
				t.Fatalf("invalid input code=%d", code)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("invalid input published a report")
			}
		})
	}
}

func TestReceiptVerifierChecksFileBytesAndContainsRelativePaths(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`"approval receipt"`)
	hash := memoryeval.Stage4Digest("approval receipt")
	s := memoryeval.Stage4Submission{Version: "memory-stage4-release-submission-v1", Approvals: []memoryeval.Stage4Approval{{EvidenceSHA256: hash}}}
	if err := os.WriteFile(filepath.Join(dir, "receipt.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, "index.json")
	writeIndex := func(path string) {
		t.Helper()
		b, _ := json.Marshal([]artifactEntry{{SHA256: hash, Path: path}})
		if err := os.WriteFile(index, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeIndex("receipt.json")
	if _, err := verifyArtifacts(s, index); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "receipt.json"), []byte("changed bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyArtifacts(s, index); err == nil {
		t.Fatal("changed artifact bytes passed")
	}
	writeIndex("../outside.json")
	if _, err := verifyArtifacts(s, index); err == nil {
		t.Fatal("parent traversal passed")
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape.json")); err != nil {
		t.Fatal(err)
	}
	writeIndex("escape.json")
	if _, err := verifyArtifacts(s, index); err == nil {
		t.Fatal("outside symlink passed")
	}
	s.Plan = &memoryeval.Stage4ReleasePlan{GoldSHA256: hash}
	writeIndex("does-not-exist.json")
	if _, err := verifyArtifacts(s, index); err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("protected gold identity must be rejected before opening any path: %v", err)
	}
}

func TestReceiptRootRemainsBoundAfterItsDirectoryPathIsReplaced(t *testing.T) {
	parent := t.TempDir()
	original := filepath.Join(parent, "receipts")
	outside := filepath.Join(parent, "outside")
	for _, dir := range []string{original, outside} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(original, "receipt.json"), []byte("original receipt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "receipt.json"), []byte("outside receipt"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(original)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = os.Rename(original, filepath.Join(parent, "moved-receipts")); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, original); err != nil {
		t.Fatal(err)
	}
	data, err := readRootedArtifactFile(root, "receipt.json", 100)
	if err != nil || string(data) != "original receipt" {
		t.Fatalf("receipt read escaped the descriptor-rooted directory: %q %v", data, err)
	}
	if err = os.Symlink(outside, filepath.Join(parent, "moved-receipts", "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err = readRootedArtifactFile(root, "escape/receipt.json", 100); err == nil {
		t.Fatal("new symlink escaped the opened receipt root")
	}
}
