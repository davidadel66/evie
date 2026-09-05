package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
)

// Independent BPE proof: inspect_qwen_gguf.py verifies actual model bytes,
// all256 byte symbols, every parsed merge result, and nonempty specials.
func readQwenBudget(path string, b tokenBudget, prompt []byte) (tokenBudget, error) {
	pinned := map[string]string{
		"runtime-manifest.json":     "sha256:25f22801682b5301596397b3113b1a5be0b27341324187193ac731546b5bf14b",
		"runtime-api-metadata.json": "sha256:100a0ae981b35bffc4f387a16404b17e6b2b8ff1d21a64201b880f14834cb720",
		"tokenizer-proof.json":      "sha256:9023a4b4dfaf703dae5e5eb1bee9545d0860bd6ea506950b47fd3567c71acf5d",
	}
	for name, hash := range pinned {
		data, err := readBounded(filepath.Join(filepath.Dir(path), name), 32<<10)
		if err != nil || digest(data) != hash || b.Files[name] != hash {
			return b, errors.New("unproven Qwen tokenizer/runtime identity")
		}
		if name == "runtime-api-metadata.json" {
			var metadata struct {
				Template string `json:"template"`
			}
			if json.Unmarshal(data, &metadata) != nil {
				return b, errors.New("invalid pinned Qwen template")
			}
			b.template = metadata.Template
		}
	}
	expectedPrompt := "sha256:f6f280093b15e1a1928db2737ec9bde8b2e0471ef66cbeab11de47b0ca57c832"
	if b.Version == "pinned-qwen2-compact-v1-byte-bound-v1" {
		expectedPrompt = compactPromptSHA256
		if b.Files["output.schema.json"] != compactSchemaSHA256 {
			return b, errors.New("unfrozen compact schema")
		}
	}
	if b.Version == compactBudgetVersion("compact-v2") {
		expectedPrompt = compactV2PromptSHA256
		if b.Files["output.schema.json"] != compactV2SchemaSHA256 {
			return b, errors.New("unfrozen compact-v2 schema")
		}
	}
	if b.Version == compactBudgetVersion("compact-v3") {
		expectedPrompt = compactV3PromptSHA256
		if b.Files["output.schema.json"] != compactV3SchemaSHA256 {
			return b, errors.New("unfrozen compact-v3 schema template")
		}
		policy, err := readBounded(filepath.Join(filepath.Dir(path), "generator-policy.json"), 8<<10)
		if err != nil || digest(policy) != compactCategoryPolicySHA256 || b.Files["generator-policy.json"] != compactCategoryPolicySHA256 {
			return b, errors.New("unfrozen compact-v3 generator policy")
		}
	}
	if digest(prompt) != expectedPrompt {
		return b, errors.New("unfrozen corrected Qwen prompt")
	}
	return b, nil
}
