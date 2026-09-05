package main_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

func compactV3Args(endpoint, only, out string) []string {
	args := compactArgs(endpoint, only, out)
	for i := range args {
		args[i] = strings.ReplaceAll(args[i], "compact-v1", "compact-v3")
	}
	return args
}
func objectHash(v any) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf("sha256:%x", sha256.Sum256(b))
}
func textHash(v string) string { return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(v))) }

func TestCompactV3PublicDerivedSchemaIdentityAndCategories(t *testing.T) {
	out := filepath.Join(t.TempDir(), "plan.json")
	args := append(compactV3Args("http://127.0.0.1:1", "N01-a,N01-b,N02-a,N02-b,N03-b,N04-b,N05-b,N06-a,N08-b,N09-a", out), "-preflight-only")
	if b, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		t.Fatalf("preflight: %v %s", err, b)
	}
	plan := readJSON(t, out)
	old := readJSON(t, filepath.Join(compactBase(), "qwen-compact-v2", "reports", "predispatch.json"))
	prefix, err := os.ReadFile(filepath.Join(compactBase(), "qwen-compact-v3", "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if plan["wire_version"] != "compact-v3" || plan["schema_derivation_version"] != "compact-category-v1" || len(plan["runs"].([]any)) != 0 {
		t.Fatal("lost pinned identity or dispatched inference")
	}
	maximum := 0
	for i, value := range plan["prepared_requests"].([]any) {
		record := value.(map[string]any)
		previous := old["prepared_requests"].([]any)[i].(map[string]any)
		if !reflect.DeepEqual(record["seal"], previous["seal"]) || record["seal_sha256"] != previous["seal_sha256"] {
			t.Fatal("source selection/projection/authority changed")
		}
		var req, prior map[string]any
		_ = json.Unmarshal([]byte(record["request"].(string)), &req)
		_ = json.Unmarshal([]byte(previous["request"].(string)), &prior)
		if req["prompt"] != prior["prompt"] || !reflect.DeepEqual(req["options"], prior["options"]) {
			t.Fatal("source/decoding changed")
		}
		actualSchema, _ := json.Marshal(req["format"])
		if req["system"] != string(prefix)+string(actualSchema) {
			t.Fatal("system and decoding schema differ")
		}
		if record["system_sha256"] != textHash(req["system"].(string)) || record["schema_sha256"] != objectHash(req["format"]) || record["schema_derivation_version"] != "compact-category-v1" {
			t.Fatal("derived request identities differ")
		}
		bound := len(req["system"].(string)) + len(req["prompt"].(string)) + 80 + 2 + 768 + 64
		if bound > maximum {
			maximum = bound
		}
		aliases := map[string][]any{"sources": {}, "context": {}}
		for _, v := range record["seal"].(map[string]any)["sources"].([]any) {
			f := v.(map[string]any)
			axis := "sources"
			if f["source"].(map[string]any)["ownership"] == "context" {
				axis = "context"
			}
			aliases[axis] = append(aliases[axis], f["alias"])
		}
		properties := req["format"].(map[string]any)["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		for _, axis := range []string{"sources", "context"} {
			field := properties[axis].(map[string]any)
			if len(aliases[axis]) == 0 {
				if !reflect.DeepEqual(field, map[string]any{"type": "array", "const": []any{}}) {
					t.Fatalf("empty context must use runtime-proven const[], got%v", field)
				}
				continue
			}
			refSchema := field["items"].(map[string]any)
			for _, alternative := range refSchema["anyOf"].([]any) {
				enum := alternative.(map[string]any)["properties"].(map[string]any)["ref"].(map[string]any)["enum"]
				if !reflect.DeepEqual(enum, aliases[axis]) {
					t.Fatalf("%s alias set widened/narrowed: %v", axis, enum)
				}
			}
			for _, alias := range aliases[axis] {
				for _, ref := range []map[string]any{{"ref": alias}, {"ref": alias, "selector": "whole"}, {"ref": alias, "selector": "date"}, {"ref": alias, "selector": "range", "start": float64(0), "end": float64(1)}} {
					if !referenceSchemaAllows(t, refSchema, ref) {
						t.Fatalf("intended ref denied: %v", ref)
					}
				}
			}
			other := "sources"
			if axis == "sources" {
				other = "context"
			}
			for _, alias := range append(append([]any{}, aliases[other]...), "unknown") {
				if referenceSchemaAllows(t, refSchema, map[string]any{"ref": alias}) {
					t.Fatalf("%s admitted cross-category/unknown alias %v", axis, alias)
				}
			}
		}
	}
	if maximum != 7750 {
		t.Fatalf("actual full rendered context bound=%d want7750", maximum)
	}
}

func TestCompactV3PublicWholeBatchUsesDerivedPromptBudget(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	for _, variant := range []string{"generated schema crosses bound", "unknown later case", "empty support"} {
		t.Run(variant, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "report.json")
			args := compactV3Args(server.URL, "N01-a,N01-b", out)
			if variant == "unknown later case" {
				args = append(args, "-only", "N01-a,unknown")
			} else {
				c := readJSON(t, filepath.Join(compactBase(), "development.json"))
				h := c["histories"].([]any)[0].(map[string]any)
				w := h["windows"].([]any)[1].(map[string]any)
				in := w["input"].(map[string]any)
				if variant == "empty support" {
					in["support"] = []any{}
					args = append(args, "-preflight-only")
				} else {
					f := in["support"].([]any)[0].(map[string]any)
					content := strings.Repeat("x", 1400)
					f["text"] = content
					f["end"] = len(content)
					f["sha256"] = textHash(content)
					for _, v := range h["events"].([]any) {
						e := v.(map[string]any)
						if e["id"] == f["event_id"] {
							e["content"] = content
						}
					}
				}
				path := filepath.Join(dir, "corpus.json")
				saveJSON(t, path, c)
				args = append(args, "-corpus", path)
			}
			b, err := exec.Command("go", args...).CombinedOutput()
			if calls.Load() != 0 {
				t.Fatal("dispatched before complete derived batch checked")
			}
			if variant == "empty support" {
				if err != nil {
					t.Fatalf("empty-support preflight:%v %s", err, b)
				}
				plan := readJSON(t, out)
				record := plan["prepared_requests"].([]any)[1].(map[string]any)
				var req map[string]any
				_ = json.Unmarshal([]byte(record["request"].(string)), &req)
				candidate := req["format"].(map[string]any)["properties"].(map[string]any)["candidates"]
				if !reflect.DeepEqual(candidate, map[string]any{"type": "array", "const": []any{}}) {
					t.Fatal("empty support permits candidates")
				}
			} else {
				want := "unknown/oversized case selection"
				if variant == "generated schema crosses bound" {
					want = "proven prompt byte bound"
				}
				if err == nil || !strings.Contains(string(b), want) {
					t.Fatalf("bad preflight:%v %s", err, b)
				}
			}
		})
	}
}

func TestCompactV3PublicScoreReconstructsDerivedRequest(t *testing.T) {
	for _, binding := range []string{"bound", "wrong window"} {
		t.Run(binding, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				proposals := []any{}
				for _, variant := range []string{"valid", "owner in context", "assistant in support", "unknown alias", "identity"} {
					p := compactProposal()
					p["sources"] = []any{map[string]any{"ref": "s3"}}
					p["context"] = []any{map[string]any{"ref": "s2"}}
					switch variant {
					case "owner in context":
						p["context"] = []any{map[string]any{"ref": "s3"}}
					case "assistant in support":
						p["sources"] = []any{map[string]any{"ref": "s2"}}
					case "unknown alias":
						p["sources"] = []any{map[string]any{"ref": "unknown"}}
					case "identity":
						p["identity"] = "unresolved"
					}
					proposals = append(proposals, p)
				}
				id := "N03-b"
				if binding == "wrong window" {
					id = "another"
				}
				raw, _ := json.Marshal(map[string]any{"window_id": id, "candidates": proposals})
				_ = json.NewEncoder(w).Encode(map[string]any{"response": string(raw), "done": true, "done_reason": "stop"})
			}))
			defer server.Close()
			out := filepath.Join(t.TempDir(), "report.json")
			if b, err := exec.Command("go", compactV3Args(server.URL, "N03-b", out)...).CombinedOutput(); err != nil {
				t.Fatalf("run:%v %s", err, b)
			}
			report := readJSON(t, out)
			run := report["runs"].([]any)[0].(map[string]any)
			retained := float64(1)
			if binding == "wrong window" {
				retained = 0
			}
			if run["raw_count"] != float64(5) || run["retained_count"] != retained {
				t.Fatalf("strict adapter/binding or denominator changed:%v", run)
			}
			variants := []string{"unchanged", "derived schema rehashed", "derived system identity", "derivation version", "budget identity", "relabelled", "seed"}
			if binding == "wrong window" {
				variants = []string{"unchanged"}
			}
			for _, variant := range variants {
				t.Run(variant, func(t *testing.T) {
					r := readJSON(t, out)
					rr := r["runs"].([]any)[0].(map[string]any)
					record := rr["compact"].(map[string]any)
					switch variant {
					case "derived schema rehashed":
						var req map[string]any
						_ = json.Unmarshal([]byte(record["request"].(string)), &req)
						properties := req["format"].(map[string]any)["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
						for _, a := range properties["sources"].(map[string]any)["items"].(map[string]any)["anyOf"].([]any) {
							a.(map[string]any)["properties"].(map[string]any)["ref"].(map[string]any)["enum"] = []any{"s1", "s2", "s3", "unknown"}
						}
						prefix, err := os.ReadFile(filepath.Join(compactBase(), "qwen-compact-v3", "prompt.txt"))
						if err != nil {
							t.Fatal(err)
						}
						schema, _ := json.Marshal(req["format"])
						req["system"] = string(prefix) + string(schema)
						body, _ := json.Marshal(req)
						record["request"] = string(body)
						record["system_sha256"] = textHash(req["system"].(string))
						record["schema_sha256"] = objectHash(req["format"])
						rr["request_sha256"] = textHash(string(body))
						rr["request_bytes"] = len(body)
					case "derived system identity":
						record["system_sha256"] = textHash("changed")
					case "derivation version":
						r["schema_derivation_version"] = "unknown"
					case "budget identity":
						r["budget_sha256"] = textHash("changed")
					case "relabelled":
						r["wire_version"] = "compact-v2"
					case "seed":
						rr["seed"] = float64(18)
					}
					path := filepath.Join(t.TempDir(), "report.json")
					saveJSON(t, path, r)
					score := filepath.Join(t.TempDir(), "score.json")
					b, err := exec.Command("go", "run", ".", "-score", path, "-output", score).CombinedOutput()
					if variant == "unchanged" {
						if err != nil {
							t.Fatalf("offline score:%v %s", err, b)
						}
					} else if err == nil {
						t.Fatalf("tampered %s scored", variant)
					}
				})
			}
		})
	}
}

func TestCompactV3PublicGeneratorPolicyPin(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	for _, variant := range []string{"v2 wire v3 files", "v3 wire v2 files", "changed policy and updated budget"} {
		t.Run(variant, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "report.json")
			args := compactV3Args(server.URL, "N01-a", out)
			switch variant {
			case "v2 wire v3 files":
				args[3] = "compact-v2"
			case "v3 wire v2 files":
				args = compactV2Args(server.URL, "N01-a", out)
				args[3] = "compact-v3"
			default:
				dir := t.TempDir()
				base := filepath.Join(compactBase(), "qwen-compact-v3")
				budget := readJSON(t, filepath.Join(base, "token-budgets.json"))
				for _, name := range []string{"prompt.txt", "output.schema.json", "runtime-manifest.json", "runtime-api-metadata.json", "tokenizer-proof.json", "generator-policy.json"} {
					b, err := os.ReadFile(filepath.Join(base, name))
					if err != nil {
						t.Fatal(err)
					}
					if name == "generator-policy.json" {
						b = append(b, ' ')
						budget["file_sha256"].(map[string]any)[name] = textHash(string(b))
					}
					if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				saveJSON(t, filepath.Join(dir, "token-budgets.json"), budget)
				for i := range args {
					args[i] = strings.ReplaceAll(args[i], base, dir)
				}
			}
			b, err := exec.Command("go", args...).CombinedOutput()
			if err == nil || calls.Load() != 0 {
				t.Fatalf("unfrozen policy/configuration dispatched:%v %s calls%d", err, b, calls.Load())
			}
			if _, err = os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("unfrozen policy wrote report")
			}
		})
	}
}
