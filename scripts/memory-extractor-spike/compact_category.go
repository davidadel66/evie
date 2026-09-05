package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
)

// The schema changes only the locations the model may nominate. The existing
// adapter independently enforces evidence categories and all semantic boundaries.
func categorySchema(seal compactSeal, template []byte) ([]byte, error) {
	if digest(template) != compactV3SchemaSHA256 {
		return nil, errors.New("unfrozen compact category schema template")
	}
	support, context := []string{}, []string{}
	for _, field := range seal.Sources {
		switch field.Source.Ownership {
		case "new", "overlap":
			support = append(support, field.Alias)
		case "context":
			context = append(context, field.Alias)
		default:
			return nil, errors.New("unknown compact category ownership")
		}
	}
	var schema map[string]any
	if err := json.Unmarshal(template, &schema); err != nil {
		return nil, err
	}
	// These assertions describe the exact hash-checked template above; changed
	// templates fail closed before their shape is accessed.
	root := schema["properties"].(map[string]any)
	candidate := root["candidates"].(map[string]any)["items"].(map[string]any)
	properties := candidate["properties"].(map[string]any)
	emptyArray := func() map[string]any { return map[string]any{"type": "array", "const": []any{}} }
	if len(support) == 0 {
		root["candidates"] = emptyArray()
	} else {
		for _, category := range []struct {
			name    string
			aliases []string
		}{{"sources", support}, {"context", context}} {
			if len(category.aliases) == 0 {
				properties[category.name] = emptyArray()
				continue
			}
			alternatives := properties[category.name].(map[string]any)["items"].(map[string]any)["anyOf"].([]any)
			for _, alternative := range alternatives {
				ref := alternative.(map[string]any)["properties"].(map[string]any)["ref"].(map[string]any)
				ref["enum"] = category.aliases
			}
		}
	}
	return json.Marshal(schema)
}

// Offline scoring reconstructs from the fixed base configuration, never from
// model-returned aliases or a self-rehashed schema supplied by a recorded run.
func categoryBaseFiles(expectedBudget string) ([]byte, []byte, error) {
	base := filepath.Join(root(), "cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v3")
	prompt, err := readBounded(filepath.Join(base, "prompt.txt"), 12<<10)
	if err != nil {
		return nil, nil, err
	}
	schema, err := readBounded(filepath.Join(base, "output.schema.json"), 12<<10)
	if err != nil {
		return nil, nil, err
	}
	budgetBytes, err := readBounded(filepath.Join(base, "token-budgets.json"), 128<<10)
	if err != nil || digest(budgetBytes) != expectedBudget {
		return nil, nil, errors.New("compact category budget identity mismatch")
	}
	if _, err := readBudget(filepath.Join(base, "token-budgets.json"), prompt, schema); err != nil {
		return nil, nil, err
	}
	return prompt, schema, nil
}
