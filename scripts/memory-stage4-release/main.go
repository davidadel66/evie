// memory-stage4-release validates frozen evaluation metadata offline. It never
// reads a holdout corpus, invokes a model, or enables ongoing compilation.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memoryeval"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("memory-stage4-release", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "metadata-only release submission JSON")
	artifactIndex := flags.String("artifacts", "", "optional receipt/output hash index; required for readiness")
	output := flags.String("output", "", "new immutable report JSON path")
	if flags.Parse(args) != nil {
		return 1
	}
	if *input == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "-input and a new -output path are required")
		return 1
	}
	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(stderr, "cannot open submission:", err)
		return 1
	}
	data, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil || len(data) > 16<<20 {
		fmt.Fprintln(stderr, "submission exceeds 16 MiB or could not be read")
		return 1
	}
	submission, err := decodeSubmission(data)
	if err != nil {
		fmt.Fprintln(stderr, "invalid submission:", err)
		return 1
	}
	report := memoryeval.AssessStage4Release(submission)
	if *artifactIndex != "" {
		verified, err := verifyArtifacts(submission, *artifactIndex)
		if err != nil {
			fmt.Fprintln(stderr, "artifact verification failed:", err)
			return 1
		}
		report = memoryeval.AssessStage4Release(submission, verified)
	}
	envelope := struct {
		InputBytesSHA256         string                         `json:"input_bytes_sha256"`
		FinalRunEvidenceSupplied bool                           `json:"final_run_evidence_supplied"`
		ToolingOnly              bool                           `json:"tooling_only"`
		Report                   memoryeval.Stage4ReleaseReport `json:"report"`
	}{fmt.Sprintf("sha256:%x", sha256.Sum256(data)), submission.Execution != nil, submission.Execution == nil, report}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "cannot encode report:", err)
		return 1
	}
	encoded = append(encoded, '\n')
	if err := writeNewReport(*output, encoded); err != nil {
		fmt.Fprintln(stderr, "report was not published:", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: ready=%t; holdout_run_authorized=false; report=%s\n", report.Status, report.Ready, *output)
	if !report.Ready {
		return 2
	}
	return 0
}

func decodeSubmission(data []byte) (memoryeval.Stage4Submission, error) {
	var submission memoryeval.Stage4Submission
	if !utf8.Valid(data) {
		return submission, errors.New("JSON must be valid UTF-8")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	tokens.UseNumber()
	if err := closedJSONValue(tokens, 0); err != nil {
		return submission, err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return submission, errors.New("submission must contain exactly one JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&submission); err != nil {
		return submission, err
	}
	return submission, nil
}

func closedJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON nesting exceeds 64 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter != '{' && delimiter != '[' {
		return errors.New("unexpected JSON delimiter")
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("JSON object contains a duplicate or invalid key")
			}
			seen[name] = true
		}
		if err := closedJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil {
		return err
	}
	expected := json.Delim(']')
	if delimiter == '{' {
		expected = '}'
	}
	if end != expected {
		return errors.New("mismatched JSON delimiter")
	}
	return nil
}

// A fully written, synced temporary file is hard-linked into its final path.
// The containing directory is synced before success is reported. Link refuses an
// existing destination, so failed evaluations cannot be silently
// overwritten. Temporary files live on the same filesystem as the destination.
func writeNewReport(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".memory-release-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Link(tempName, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
