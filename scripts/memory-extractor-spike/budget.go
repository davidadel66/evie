package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"text/template"
	"unicode/utf8"
)

type tokenBudget struct {
	Version  string            `json:"version"`
	Files    map[string]string `json:"file_sha256"`
	Reserve  int               `json:"reserve_tokens"`
	template string
	Entries  []struct {
		RequestSHA256   string `json:"request_sha256"`
		PromptTokens    int    `json:"prompt_tokens"`
		ContextTokens   int    `json:"context_tokens"`
		MaxOutputTokens int    `json:"max_output_tokens"`
		Model           string `json:"model"`
		Mode            string `json:"mode"`
		Report          string `json:"report"`
		ReportSHA256    string `json:"report_sha256"`
		RunIndex        int    `json:"run_index"`
	} `json:"entries"`
}

func readBudget(path string, prompt, schema []byte) (tokenBudget, error) {
	b, err := readBounded(path, 128<<10)
	if err != nil {
		return tokenBudget{}, fmt.Errorf("verified token budget required before inference: %w", err)
	}
	var budget tokenBudget
	if json.Unmarshal(b, &budget) != nil || (budget.Version != "empirical-frozen-request-budgets-v1" && budget.Version != "pinned-mistral-spm-byte-bound-v1" && !isQwenByteBudget(budget.Version)) || budget.Reserve != 64 {
		return budget, errors.New("invalid token budget version/reserve")
	}
	if budget.Files["prompt.txt"] != digest(prompt) || budget.Files["output.schema.json"] != digest(schema) {
		return budget, errors.New("token budget prompt/schema identity mismatch")
	}
	for _, file := range []string{"runtime-manifest.json", "runtime-api-metadata.json"} {
		b, err := readBounded(filepath.Join(filepath.Dir(path), file), 32<<10)
		if err != nil || budget.Files[file] != digest(b) {
			return budget, errors.New("token budget runtime/model/template identity mismatch")
		}
	}
	if isQwenByteBudget(budget.Version) {
		return readQwenBudget(path, budget, prompt)
	}
	if budget.Version == "pinned-mistral-spm-byte-bound-v1" {
		return readByteBudget(path, budget, prompt)
	}
	for _, entry := range budget.Entries {
		if filepath.IsAbs(entry.Report) || filepath.Clean(entry.Report) != entry.Report || filepath.Dir(entry.Report) != "reports" {
			return budget, errors.New("invalid budget evidence path")
		}
		b, err := readBounded(filepath.Join(filepath.Dir(path), entry.Report), 4<<20)
		if err != nil || digest(b) != entry.ReportSHA256 {
			return budget, errors.New("token budget evidence changed")
		}
		var r report
		if json.Unmarshal(b, &r) != nil || entry.RunIndex < 0 || entry.RunIndex >= len(r.Runs) {
			return budget, errors.New("invalid budget evidence run")
		}
		run := r.Runs[entry.RunIndex]
		if run.RequestSHA256 != entry.RequestSHA256 || run.PromptTokens != entry.PromptTokens || run.ServerRelease != "finished_response" || entry.PromptTokens <= 0 || entry.PromptTokens >= r.ContextTokens || entry.ContextTokens != r.ContextTokens || entry.MaxOutputTokens != r.MaxOutputTokens || entry.Model != r.Model || entry.Mode != r.Mode {
			return budget, errors.New("token budget not proven by original completed request")
		}
	}
	return budget, nil
}
func (b tokenBudget) check(body []byte, model, mode string, contextTokens, maxOutput int) error {
	if b.Version == "pinned-mistral-spm-byte-bound-v1" || isQwenByteBudget(b.Version) {
		return b.checkBytes(body, model, mode, contextTokens, maxOutput)
	}
	requestSHA := digest(body)
	for _, entry := range b.Entries {
		if entry.RequestSHA256 == requestSHA && entry.Model == model && entry.Mode == mode && entry.ContextTokens == contextTokens && entry.MaxOutputTokens == maxOutput {
			if entry.PromptTokens+maxOutput+b.Reserve > contextTokens {
				return errors.New("verified prompt plus output reserve exceeds context")
			}
			return nil
		}
	}
	return errors.New("unmeasured or changed request has no verified token budget")
}

// This proof is intentionally specific to the inspected model/runtime artifacts.
// It is not a general tokens-per-byte heuristic; see input-budget-proof.md.
func readByteBudget(path string, b tokenBudget, prompt []byte) (tokenBudget, error) {
	pinned := map[string]string{
		"runtime-manifest.json":     "sha256:56d8dbd0fc53d4ee9ae42f7170e78c0b48f808c7308b9969b1afae383cad882d",
		"runtime-api-metadata.json": "sha256:0cb7b51fd0ed73a46c209049a1c9bd742573d6d2721df7e0d47e555c618b4b4e",
		"tokenizer-proof.json":      "sha256:68a54396c5509c3fe1184ab344a50fadc4c635e9048405c2c0c011b8e94975d8",
	}
	for name, hash := range pinned {
		data, err := readBounded(filepath.Join(filepath.Dir(path), name), 32<<10)
		if err != nil || digest(data) != hash || b.Files[name] != hash {
			return b, errors.New("unproven tokenizer/runtime identity")
		}
		if name == "runtime-api-metadata.json" {
			var metadata struct {
				Template string `json:"template"`
			}
			if json.Unmarshal(data, &metadata) != nil {
				return b, errors.New("invalid pinned template")
			}
			b.template = metadata.Template
		}
	}
	if digest(prompt) != "sha256:f6f280093b15e1a1928db2737ec9bde8b2e0471ef66cbeab11de47b0ca57c832" {
		return b, errors.New("unfrozen corrected prompt")
	}
	return b, nil
}
func (b tokenBudget) checkBytes(body []byte, model, mode string, contextTokens, maxOutput int) error {
	expectedModel := "mistral:latest"
	if isQwenByteBudget(b.Version) {
		expectedModel = "qwen2.5:7b-instruct-q4_K_M"
		if mode != "schema" {
			return errors.New("unproven Qwen format")
		}
	}
	if model != expectedModel || (mode != "schema" && mode != "json") || contextTokens != 8192 || maxOutput != 768 {
		return errors.New("unproven byte-budget configuration")
	}
	var req struct {
		System string `json:"system"`
		Prompt string `json:"prompt"`
	}
	if json.Unmarshal(body, &req) != nil || req.System == "" || req.Prompt == "" || !utf8.ValidString(req.System) || !utf8.ValidString(req.Prompt) {
		return errors.New("missing complete prompt")
	}
	// Ollama v0.6.3 routes.go adds system then user; template.collate keeps both
	// messages, and also collects System. No model Messages layer is installed.
	tmpl, err := template.New("").Option("missingkey=zero").Parse(b.template)
	if err != nil {
		return err
	}
	var rendered bytes.Buffer
	values := map[string]any{"System": req.System, "Messages": []map[string]any{{"Role": "system", "Content": req.System}, {"Role": "user", "Content": req.Prompt}}, "Tools": nil, "Response": ""}
	if err := tmpl.Execute(&rendered, values); err != nil {
		return err
	}
	expected := "[INST] " + req.System + "\n\n" + req.Prompt + "[/INST] "
	if isQwenByteBudget(b.Version) {
		expected = "<|im_start|>system\n" + req.System + "<|im_end|>\n<|im_start|>user\n" + req.Prompt + "<|im_end|>\n<|im_start|>assistant\n"
	}
	if rendered.String() != expected {
		return errors.New("unproven full template rendering")
	}
	if rendered.Len()+2+maxOutput+b.Reserve > contextTokens {
		return errors.New("proven prompt byte bound plus output reserve exceeds context")
	}
	return nil
}
