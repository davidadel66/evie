package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func validationIdentity() (string, error) {
	var combined []byte
	for _, name := range []string{"main.go", "checks.go", "budget.go", "qwen_budget.go", "compact_wire.go", "compact_config.go", "compact_category.go", "revalidate.go"} {
		b, err := readBounded(filepath.Join(root(), "scripts/memory-extractor-spike", name), 128<<10)
		if err != nil {
			return "", err
		}
		combined = append(combined, []byte(name+"\x00")...)
		combined = append(combined, b...)
	}
	return digest(combined), nil
}

func revalidateReport(path, corpusPath, budgetPath, promptPath, schemaPath, output string) error {
	original, err := readBounded(path, 4<<20)
	if err != nil {
		return err
	}
	var r report
	if json.Unmarshal(original, &r) != nil || r.Version != "evie-extraction-spike-report-v1" {
		return errors.New("invalid original report")
	}
	if r.WireVersion != "" {
		return errors.New("compact reports must be scored with seal re-expansion; offline revalidation is not enabled")
	}
	corpusBytes, err := readBounded(corpusPath, 2<<20)
	if err != nil || digest(corpusBytes) != r.CorpusSHA256 {
		return errors.New("original corpus hash differs")
	}
	var c corpus
	if json.Unmarshal(corpusBytes, &c) != nil {
		return errors.New("invalid corpus")
	}
	prompt, err := readBounded(promptPath, 12<<10)
	if err != nil {
		return err
	}
	schema, err := readBounded(schemaPath, 12<<10)
	if err != nil {
		return err
	}
	if digest(prompt) != r.PromptSHA256 || digest(schema) != r.SchemaSHA256 {
		return errors.New("original prompt/schema hash differs")
	}
	budget, err := readBudget(budgetPath, prompt, schema)
	if err != nil {
		return err
	}
	inputs := map[string]input{}
	for _, h := range c.Histories {
		for _, w := range h.Windows {
			if err := validateInput(h, w); err != nil {
				return fmt.Errorf("%s: %w", w.ID, err)
			}
			inputs[w.ID] = w.Input
		}
	}
	r.OriginalReportSHA256 = digest(original)
	r.OriginalReport = path
	r.ValidationCodeSHA256, err = validationIdentity()
	if err != nil {
		return err
	}
	r.ValidationVersion = "standalone-source-checks-v2"
	for i, run := range r.Runs {
		run.OriginalStatus = run.Status
		run.OriginalRetainedCount = run.RetainedCount
		body := requestBody(inputs[run.CaseID], prompt, schema, r.Mode, r.Model, r.ContextTokens, r.MaxOutputTokens, run.Seed)
		if digest(body) != run.RequestSHA256 {
			return errors.New("original request identity differs")
		}
		if err := budget.check(body, r.Model, r.Mode, r.ContextTokens, r.MaxOutputTokens); err != nil {
			return err
		}
		if run.Status == "ok" || run.Status == "schema_error" {
			if run.ServerRelease != "finished_response" || digest([]byte(run.Raw)) != run.RawSHA256 {
				return errors.New("raw result is incomplete/redacted or differs from recorded bytes")
			}
			in, ok := inputs[run.CaseID]
			if !ok {
				return errors.New("missing original case")
			}
			run = checkProposals(run.Raw, run, in)
		}
		r.Runs[i] = run
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return errors.Join(err, f.Close())
}
