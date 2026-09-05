package main_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func compactV2Args(endpoint, only, out string) []string {
	args := compactArgs(endpoint, only, out)
	for i := range args {
		args[i] = strings.ReplaceAll(args[i], "compact-v1", "compact-v2")
	}
	return args
}

// Evaluate only the JSON Schema keywords used by the reference subtree. Unknown
// keywords fail the test so a new unsupported construct cannot appear to pass.
// This checks the actual public request schema, separately from the adapter.
func referenceSchemaAllows(t *testing.T, schema map[string]any, value any) bool {
	t.Helper()
	for key := range schema {
		switch key {
		case "anyOf", "type", "properties", "required", "additionalProperties", "enum", "minimum":
		default:
			t.Fatalf("unsupported reference schema keyword %q", key)
		}
	}
	if alternatives, ok := schema["anyOf"].([]any); ok {
		matched := false
		for _, alternative := range alternatives {
			matched = referenceSchemaAllows(t, alternative.(map[string]any), value) || matched
		}
		if !matched {
			return false
		}
	}
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		properties := schema["properties"].(map[string]any)
		for _, name := range schema["required"].([]any) {
			if _, exists := object[name.(string)]; !exists {
				return false
			}
		}
		for name, v := range object {
			property, exists := properties[name]
			if !exists {
				if schema["additionalProperties"] == false {
					return false
				}
			} else if !referenceSchemaAllows(t, property.(map[string]any), v) {
				return false
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return false
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || math.Trunc(number) != number {
			return false
		}
		if min, exists := schema["minimum"].(float64); exists && number < min {
			return false
		}
	case nil:
	default:
		t.Fatalf("unsupported reference type %v", schema["type"])
	}
	if values, ok := schema["enum"].([]any); ok {
		for _, permitted := range values {
			if reflect.DeepEqual(value, permitted) {
				return true
			}
		}
		return false
	}
	return true
}

func TestCompactV2PublicClosedReferenceSchema(t *testing.T) {
	out := filepath.Join(t.TempDir(), "plan.json")
	args := append(compactV2Args("http://127.0.0.1:1", "N01-a,N01-b,N02-a,N02-b,N03-b,N04-b,N05-b,N06-a,N08-b,N09-a", out), "-preflight-only")
	if b, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		t.Fatalf("preflight: %v %s", err, b)
	}
	plan := readJSON(t, out)
	if plan["wire_version"] != "compact-v2" || len(plan["runs"].([]any)) != 0 {
		t.Fatal("configuration identity lost or inference dispatched")
	}
	old := readJSON(t, filepath.Join(compactBase(), "qwen-compact-v1", "reports", "preflight.json"))
	oldRecords := old["prepared_requests"].([]any)
	for i, raw := range plan["prepared_requests"].([]any) {
		record := raw.(map[string]any)
		oldRecord := oldRecords[i].(map[string]any)
		if !reflect.DeepEqual(record["seal"], oldRecord["seal"]) || record["seal_sha256"] != oldRecord["seal_sha256"] {
			t.Fatal("source projection/authority/window seal changed")
		}
		var req, oldReq map[string]any
		_ = json.Unmarshal([]byte(record["request"].(string)), &req)
		_ = json.Unmarshal([]byte(oldRecord["request"].(string)), &oldReq)
		if req["prompt"] != oldReq["prompt"] || !reflect.DeepEqual(req["options"], oldReq["options"]) {
			t.Fatal("source input/decoding options changed")
		}
		bound := len(req["system"].(string)) + len(req["prompt"].(string)) + 80 + 2 + 768 + 64
		if bound > 8192 {
			t.Fatalf("complete context bound %d exceeds8192", bound)
		}
		properties := req["format"].(map[string]any)["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		for _, axis := range []string{"sources", "context"} {
			schema := properties[axis].(map[string]any)["items"].(map[string]any)
			// Exhaust every selector/coordinate-presence combination, including
			// all dangling starts seen in the immutable compact-v1 run.
			for _, selector := range []string{"omitted", "whole", "date", "range", "", "unknown", "null"} {
				for mask := 0; mask < 4; mask++ {
					ref := map[string]any{"ref": "s1"}
					if selector != "omitted" {
						ref["selector"] = selector
						if selector == "null" {
							ref["selector"] = nil
						}
					}
					if mask&1 != 0 {
						ref["start"] = float64(0)
					}
					if mask&2 != 0 {
						ref["end"] = float64(10)
					}
					want := (mask == 0 && (selector == "omitted" || selector == "whole" || selector == "date")) || (mask == 3 && selector == "range")
					if got := referenceSchemaAllows(t, schema, ref); got != want {
						t.Fatalf("%s selector=%q coordinates=%d allowed=%t want=%t", axis, selector, mask, got, want)
					}
				}
			}
			for _, malformed := range []map[string]any{
				{}, {"ref": nil}, {"ref": "s1", "extra": true},
				{"ref": "s1", "selector": "range", "start": float64(-1), "end": float64(10)},
				{"ref": "s1", "selector": "range", "start": float64(0), "end": float64(0)},
				{"ref": "s1", "selector": "range", "start": float64(0.5), "end": float64(10)},
			} {
				if referenceSchemaAllows(t, schema, malformed) {
					t.Fatalf("malformed ref allowed: %v", malformed)
				}
			}
		}
	}
}

func TestCompactV2PublicConfigurationIdentity(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	for _, variant := range []string{"v1 wire v2 files", "v2 wire v1 files", "canonical wire v2 files", "changed schema", "changed prompt", "changed timeout"} {
		t.Run(variant, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "report.json")
			args := compactV2Args(server.URL, "N01-a", out)
			switch variant {
			case "v1 wire v2 files":
				args[3] = "compact-v1"
			case "v2 wire v1 files":
				args = compactArgs(server.URL, "N01-a", out)
				args[3] = "compact-v2"
			case "canonical wire v2 files":
				args[3] = "canonical"
			case "changed timeout":
				args = append(args, "-timeout", "61s")
			default:
				tmp := t.TempDir()
				base := filepath.Join(compactBase(), "qwen-compact-v2")
				budget := readJSON(t, filepath.Join(base, "token-budgets.json"))
				for _, name := range []string{"prompt.txt", "output.schema.json", "runtime-manifest.json", "runtime-api-metadata.json", "tokenizer-proof.json"} {
					b, err := os.ReadFile(filepath.Join(base, name))
					if err != nil {
						t.Fatal(err)
					}
					if (variant == "changed schema" && name == "output.schema.json") || (variant == "changed prompt" && name == "prompt.txt") {
						b = append(b, ' ')
						budget["file_sha256"].(map[string]any)[name] = fmt.Sprintf("sha256:%x", sha256.Sum256(b))
					}
					if err = os.WriteFile(filepath.Join(tmp, name), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				saveJSON(t, filepath.Join(tmp, "token-budgets.json"), budget)
				for i := range args {
					args[i] = strings.ReplaceAll(args[i], base, tmp)
				}
			}
			b, err := exec.Command("go", args...).CombinedOutput()
			if err == nil || calls.Load() != 0 {
				t.Fatalf("unsafe config dispatched: %v %s calls=%d", err, b, calls.Load())
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("unsafe config wrote report")
			}
		})
	}
}

func TestCompactV2PublicAdapterAndScoreBinding(t *testing.T) {
	proposals := []any{}
	for _, ref := range []map[string]any{
		{"ref": "s1"},
		{"ref": "s1", "selector": "whole"},
		{"ref": "s1", "selector": "range", "start": 0, "end": 13},
		{"ref": "s1", "start": 0},
		{"ref": "s1", "selector": "whole", "start": 0, "end": 13},
	} {
		p := compactProposal()
		p["sources"] = []any{ref}
		proposals = append(proposals, p)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := json.Marshal(map[string]any{"window_id": "N01-a", "candidates": proposals})
		_ = json.NewEncoder(w).Encode(map[string]any{"response": string(raw), "done": true, "done_reason": "stop"})
	}))
	defer server.Close()
	out := filepath.Join(t.TempDir(), "report.json")
	if b, err := exec.Command("go", compactV2Args(server.URL, "N01-a", out)...).CombinedOutput(); err != nil {
		t.Fatalf("run: %v %s", err, b)
	}
	r := readJSON(t, out)
	run := r["runs"].([]any)[0].(map[string]any)
	if run["raw_count"] != float64(5) || run["retained_count"] != float64(3) {
		t.Fatalf("adapter changed complete-range/whole protocol or raw denominator: %v", run)
	}
	for _, version := range []string{"compact-v2", "compact-v1"} {
		r["wire_version"] = version
		input := filepath.Join(t.TempDir(), "report.json")
		saveJSON(t, input, r)
		score := filepath.Join(t.TempDir(), "score.json")
		b, err := exec.Command("go", "run", ".", "-score", input, "-output", score).CombinedOutput()
		if version == "compact-v2" && err != nil {
			t.Fatalf("score: %v %s", err, b)
		}
		if version == "compact-v1" && (err == nil || !strings.Contains(string(b), "configuration/prompt/schema identity mismatch")) {
			t.Fatalf("relabelled configuration accepted: %v %s", err, b)
		}
	}
}
