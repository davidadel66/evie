package main_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func compactBase() string {
	return filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "memory-stage-4-spike", "v1")
}
func compactArgs(endpoint, only, out string) []string {
	base := filepath.Join(compactBase(), "qwen-compact-v1")
	return []string{"run", ".", "-wire", "compact-v1", "-endpoint", endpoint, "-prompt", filepath.Join(base, "prompt.txt"), "-schema", filepath.Join(base, "output.schema.json"), "-budgets", filepath.Join(base, "token-budgets.json"), "-model", "qwen2.5:7b-instruct-q4_K_M", "-context", "8192", "-only", only, "-repetitions", "1", "-stop-on-failure", "-output", out}
}
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func saveJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func compactProposal() map[string]any {
	return map[string]any{"subject_type": "owner", "subject_name": "", "subject_entity_ref": "", "predicate": "preference", "object_kind": "text", "object": "tea", "polarity": "affirmed", "kind": "fact", "temporal": "", "identity": "resolved", "effect": "assert", "sources": []any{map[string]any{"ref": "s1"}}, "context": []any{}}
}
func compactStub(t *testing.T, only string, p []any, modify func(map[string]any)) (string, map[string]any) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "report.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			t.Error("invalid request")
		}
		var in map[string]any
		if json.Unmarshal([]byte(req["prompt"].(string)), &in) != nil {
			t.Error("invalid input")
		}
		response := map[string]any{"window_id": in["window_id"], "candidates": p}
		if modify != nil {
			modify(response)
		}
		raw, _ := json.Marshal(response)
		_ = json.NewEncoder(w).Encode(map[string]any{"response": string(raw), "done": true, "done_reason": "stop"})
	}))
	defer server.Close()
	if b, err := exec.Command("go", compactArgs(server.URL, only, out)...).CombinedOutput(); err != nil {
		t.Fatalf("compact run: %v %s", err, b)
	}
	return out, readJSON(t, out)
}
func TestCompactPublicProjectionAndAllRequestPlan(t *testing.T) {
	out := filepath.Join(t.TempDir(), "plan.json")
	args := append(compactArgs("http://127.0.0.1:1", "N01-a,N01-b,N02-a,N02-b,N03-b,N04-b,N05-b,N06-a,N08-b,N09-a", out), "-preflight-only")
	if b, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		t.Fatalf("preflight: %v %s", err, b)
	}
	plan := readJSON(t, out)
	records := plan["prepared_requests"].([]any)
	if len(records) != 10 || len(plan["runs"].([]any)) != 0 {
		t.Fatal("preflight lost plan or dispatched")
	}
	source := readJSON(t, filepath.Join(compactBase(), "development.json"))
	windows := map[string]map[string]any{}
	for _, hv := range source["histories"].([]any) {
		for _, wv := range hv.(map[string]any)["windows"].([]any) {
			w := wv.(map[string]any)
			windows[w["id"].(string)] = w
		}
	}
	for _, rv := range records {
		record := rv.(map[string]any)
		seal := record["seal"].(map[string]any)
		var req, in map[string]any
		_ = json.Unmarshal([]byte(record["request"].(string)), &req)
		_ = json.Unmarshal([]byte(req["prompt"].(string)), &in)
		supplied := map[string]map[string]any{}
		original := windows[in["window_id"].(string)]["input"].(map[string]any)
		for _, category := range []string{"support", "context"} {
			for _, fv := range original[category].([]any) {
				f := fv.(map[string]any)
				supplied[f["event_id"].(string)] = f
			}
		}
		fields := in["fields"].([]any)
		aliases := seal["sources"].([]any)
		previous := float64(0)
		if len(fields) != len(supplied) || len(aliases) != len(fields) {
			t.Fatal("source selection changed")
		}
		for i, fv := range fields {
			field := fv.(map[string]any)
			alias := aliases[i].(map[string]any)
			canonical := alias["source"].(map[string]any)
			expected := supplied[canonical["event_id"].(string)]
			if field["sequence"].(float64) <= previous {
				t.Fatal("not chronological")
			}
			previous = field["sequence"].(float64)
			if field["text"] != expected["text"] || field["ownership"] != expected["ownership"] || field["authority"] != expected["authority"] || field["start"] != expected["start"] || field["end"] != expected["end"] || field["ref"] != fmt.Sprintf("s%d", i+1) || alias["observed_at"] == nil {
				t.Fatalf("lossy projection: %v", field)
			}
			if strings.Contains(req["prompt"].(string), canonical["event_id"].(string)) || strings.Contains(req["prompt"].(string), canonical["scope"].(string)) {
				t.Fatal("opaque source/destination identity leaked into wire")
			}
		}
		if len(req["system"].(string))+len(req["prompt"].(string))+80+2+768+64 > 8192 {
			t.Fatal("unproven full rendered budget")
		}
	}
}
func TestCompactPublicExpansionRejectsWithoutRepair(t *testing.T) {
	for _, tc := range []struct {
		name, only, rejection string
		mutate                func(map[string]any)
		root                  func(map[string]any)
	}{
		{"exact owner", "N01-a", "", func(p map[string]any) {}, nil},
		{"unknown alias", "N01-a", "unknown_alias", func(p map[string]any) { p["sources"] = []any{map[string]any{"ref": "s99"}} }, nil},
		{"wrong reference category", "N03-b", "reference_category", func(p map[string]any) { p["sources"] = []any{map[string]any{"ref": "s2"}} }, nil},
		{"support cannot become context", "N01-a", "reference_category", func(p map[string]any) { p["context"] = []any{map[string]any{"ref": "s1"}} }, nil},
		{"overlap alone", "N01-b", "no newly owned support", func(p map[string]any) {}, nil},
		{"UTF8 scalar cut", "N09-a", "UTF-8 scalar", func(p map[string]any) {
			p["sources"] = []any{map[string]any{"ref": "s1", "selector": "range", "start": 0, "end": 20}}
		}, nil},
		{"range outside projection", "N01-a", "outside projected", func(p map[string]any) {
			p["sources"] = []any{map[string]any{"ref": "s1", "selector": "range", "start": 0, "end": 99}}
		}, nil},
		{"explicit empty selector", "N01-a", "invalid_selector", func(p map[string]any) { p["sources"] = []any{map[string]any{"ref": "s1", "selector": ""}} }, nil},
		{"unknown selector", "N01-a", "invalid_selector", func(p map[string]any) { p["sources"] = []any{map[string]any{"ref": "s1", "selector": "sentence"}} }, nil},
		{"range does not fall back", "N01-a", "coordinates required", func(p map[string]any) { p["sources"] = []any{map[string]any{"ref": "s1", "selector": "range"}} }, nil},
		{"incompatible subject", "N01-a", "invalid_subject", func(p map[string]any) { p["subject_name"] = "tea" }, nil},
		{"literal placeholder", "N02-b", "name absent", func(p map[string]any) {
			p["subject_type"] = "new_entity"
			p["subject_name"] = "Name"
			p["identity"] = "unresolved"
		}, nil},
		{"new identity cannot resolve", "N02-b", "unresolved identity", func(p map[string]any) { p["subject_type"] = "new_entity"; p["subject_name"] = "Maya" }, nil},
		{"unoffered accepted identity", "N01-a", "unknown accepted entity", func(p map[string]any) { p["subject_type"] = "accepted_entity"; p["subject_entity_ref"] = "a1" }, nil},
		{"forbidden effect", "N01-a", "schema enum", func(p map[string]any) { p["effect"] = "approve" }, nil},
		{"wrong window", "N01-a", "response_binding", func(p map[string]any) {}, func(r map[string]any) { r["window_id"] = "N01-b" }},
		{"closed subject shape", "N01-a", "closed compact candidate", func(p map[string]any) { p["scope"] = "global" }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := compactProposal()
			tc.mutate(p)
			path, r := compactStub(t, tc.only, []any{p}, tc.root)
			run := r["runs"].([]any)[0].(map[string]any)
			proposals := run["proposals"].([]any)
			if run["raw_count"] != float64(1) || len(proposals) != 1 {
				t.Fatal("raw rejected object lost")
			}
			proposal := proposals[0].(map[string]any)
			if tc.name == "wrong window" || tc.name == "explicit empty selector" {
				scorePath := filepath.Join(t.TempDir(), "score.json")
				if b, err := exec.Command("go", "run", ".", "-score", path, "-output", scorePath).CombinedOutput(); err != nil {
					t.Fatalf("score shape rejection: %v %s", err, b)
				}
				score := readJSON(t, scorePath)
				panel := score["reports"].([]any)[0].(map[string]any)["raw"].(map[string]any)
				if panel["proposals"] != float64(1) || panel["supported_useful"] != float64(0) || panel["unadjudicated"] != float64(1) {
					t.Fatal("invalid shape gained credit or lost raw denominator")
				}
				if tc.name == "wrong window" && score["unadjudicated"].([]any)[0].(map[string]any)["candidate"].(map[string]any)["sources"] != nil {
					t.Fatal("scorer expanded wrong-window references")
				}
			}

			if tc.name == "wrong window" && (proposal["expanded"] == true || proposal["candidate"].(map[string]any)["sources"] != nil) {
				t.Fatal("wrong window response expanded canonical references")
			}
			if tc.rejection == "" {
				if run["retained_count"] != float64(1) {
					t.Fatalf("exact candidate rejected: %v", run)
				}
			} else if run["retained_count"] != float64(0) || !strings.Contains(proposal["rejection"].(string), tc.rejection) {
				t.Fatalf("repaired/incorrect rejection: %v", run)
			}
		})
	}
}
func TestCompactCanonicalScoreCountsRejectedRawAndChecksSeal(t *testing.T) {
	good := compactProposal()
	bad := compactProposal()
	bad["sources"] = []any{map[string]any{"ref": "s404"}}
	path, r := compactStub(t, "N01-a", []any{good, bad}, nil)
	for _, variant := range []string{"valid", "edited alias", "edited expansion"} {
		t.Run(variant, func(t *testing.T) {
			dir := t.TempDir()
			reportPath := path
			if variant != "valid" {
				copy := readJSON(t, path)
				run := copy["runs"].([]any)[0].(map[string]any)
				if variant == "edited alias" {
					seal := run["compact"].(map[string]any)["seal"].(map[string]any)
					seal["sources"].([]any)[0].(map[string]any)["source"].(map[string]any)["ownership"] = "overlap"
				} else {
					run["proposals"].([]any)[0].(map[string]any)["candidate"].(map[string]any)["object"] = "coffee"
				}
				reportPath = filepath.Join(dir, "edited.json")
				saveJSON(t, reportPath, copy)
			}
			scorePath := filepath.Join(dir, "score.json")
			b, err := exec.Command("go", "run", ".", "-score", reportPath, "-output", scorePath).CombinedOutput()
			if variant != "valid" {
				if err == nil {
					t.Fatalf("tampered report scored: %s", b)
				}
				return
			}
			if err != nil {
				t.Fatalf("score: %v %s", err, b)
			}
			score := readJSON(t, scorePath)
			summary := score["reports"].([]any)[0].(map[string]any)
			raw := summary["raw"].(map[string]any)
			retained := summary["retained"].(map[string]any)
			if raw["proposals"] != float64(2) || raw["supported_useful"] != float64(1) || raw["unadjudicated"] != float64(1) || retained["proposals"] != float64(1) || retained["supported_useful"] != float64(1) {
				t.Fatalf("wire/canonical denominators differ: %v", summary)
			}
			unknown := score["unadjudicated"].([]any)[0].(map[string]any)
			wire, _ := json.Marshal(bad)
			expected := fmt.Sprintf("sha256:%x", sha256.Sum256(wire))
			if unknown["candidate_sha256"] != expected {
				t.Fatal("raw wire identity lost")
			}
		})
	}
	_ = r
}
func TestCompactClockDateAndInterpretationContext(t *testing.T) {
	for _, id := range []string{"N03-b", "N08-b"} {
		t.Run(id, func(t *testing.T) {
			p := compactProposal()
			p["sources"] = []any{map[string]any{"ref": "s3"}}
			if id == "N03-b" {
				p["object"] = "tea over coffee"
				p["context"] = []any{map[string]any{"ref": "s2"}}
			} else {
				p["predicate"] = "habit"
				p["object"] = "drinking coffee"
				p["polarity"] = "denied"
				p["kind"] = "world_change"
				p["temporal"] = "2026-09-04"
				p["sources"] = []any{map[string]any{"ref": "s3"}, map[string]any{"ref": "s2", "selector": "date"}}
			}
			reportPath, r := compactStub(t, id, []any{p}, nil)
			run := r["runs"].([]any)[0].(map[string]any)
			if run["retained_count"] != float64(1) {
				t.Fatalf("valid expansion rejected: %v", run)
			}
			scorePath := filepath.Join(t.TempDir(), "score.json")
			if b, err := exec.Command("go", "run", ".", "-score", reportPath, "-output", scorePath).CombinedOutput(); err != nil {
				t.Fatalf("score: %v %s", err, b)
			}
			score := readJSON(t, scorePath)
			if score["reports"].([]any)[0].(map[string]any)["raw"].(map[string]any)["required_matches"] != float64(1) {
				t.Fatal("exact canonical meaning/source credit missing")
			}
		})
	}
}
func TestCompactPreflightRejectsWholeBatchAndHandlesHugeGap(t *testing.T) {
	for _, variant := range []string{"later oversized", "unknown case", "huge cutoff", "missing proof", "changed prompt"} {
		t.Run(variant, func(t *testing.T) {
			dir := t.TempDir()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			defer server.Close()
			args := compactArgs(server.URL, "N01-a,N01-b", filepath.Join(dir, "plan.json"))
			if variant == "huge cutoff" {
				args = append(args, "-preflight-only")
			}
			if variant == "later oversized" || variant == "huge cutoff" {
				c := readJSON(t, filepath.Join(compactBase(), "development.json"))
				h := c["histories"].([]any)[0].(map[string]any)
				w := h["windows"].([]any)[1].(map[string]any)
				if variant == "huge cutoff" {
					w["captured_sequence"] = float64(1000000000)
				} else {
					f := w["input"].(map[string]any)["support"].([]any)[0].(map[string]any)
					content := strings.Repeat("x", 6000)
					f["text"] = content
					f["end"] = len(content)
					f["sha256"] = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content)))
					for _, ev := range h["events"].([]any) {
						e := ev.(map[string]any)
						if e["id"] == f["event_id"] {
							e["content"] = content
						}
					}
				}
				path := filepath.Join(dir, "corpus.json")
				saveJSON(t, path, c)
				args = append(args, "-corpus", path)
			}
			if variant == "unknown case" {
				args = append(args, "-only", "N01-a,unknown")
			}
			if variant == "missing proof" {
				args = append(args, "-budgets", filepath.Join(dir, "missing.json"))
			}
			if variant == "changed prompt" {
				path := filepath.Join(dir, "prompt.txt")
				_ = os.WriteFile(path, []byte("changed"), 0600)
				args = append(args, "-prompt", path)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			b, err := exec.CommandContext(ctx, "go", args...).CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("unbounded preflight: %s", b)
			}
			if calls.Load() != 0 {
				t.Fatal("dispatched before entire batch checked")
			}
			if variant == "huge cutoff" {
				if err != nil {
					t.Fatalf("bounded gap representation failed: %v %s", err, b)
				}
			} else {
				want := map[string]string{"later oversized": "pre-dispatch: proven prompt byte bound", "unknown case": "unknown/oversized case selection", "missing proof": "verified token budget required", "changed prompt": "prompt/schema identity mismatch"}[variant]
				if err == nil || !strings.Contains(string(b), want) {
					t.Fatalf("unproven batch accepted or wrong rejection: %v %s", err, b)
				}
			}
		})
	}
}

func TestCompactAcceptedAlternativesAndNonzeroProjection(t *testing.T) {
	for _, variant := range []string{"accepted alternatives", "nonzero projection"} {
		t.Run(variant, func(t *testing.T) {
			dir := t.TempDir()
			data := readJSON(t, filepath.Join(compactBase(), "development.json"))
			h := data["histories"].([]any)[0].(map[string]any)
			window := h["windows"].([]any)[0].(map[string]any)
			in := window["input"].(map[string]any)
			p := compactProposal()
			proposals := []any{p}
			if variant == "accepted alternatives" {
				// Temporary structural fixture: two same-name alternatives remain distinct;
				// no human meaning judgment is manufactured or scored from this fixture.
				in["accepted_context"] = []any{map[string]any{"entity_id": "entity-one", "aliases": []any{"tea"}}, map[string]any{"entity_id": "entity-two", "aliases": []any{"tea"}}}
				p["subject_type"] = "accepted_entity"
				p["subject_entity_ref"] = "a2"
				object := compactProposal()
				object["object_kind"] = "entity"
				object["object"] = "a1"
				unresolved := compactProposal()
				unresolved["subject_type"] = "new_entity"
				unresolved["subject_name"] = "tea"
				unresolved["identity"] = "unresolved"
				proposals = append(proposals, object, unresolved)
			} else {
				f := in["support"].([]any)[0].(map[string]any)
				f["start"] = 2
				f["text"] = "prefer tea."
				f["sha256"] = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("prefer tea.")))
			}
			corpusPath := filepath.Join(dir, "sources.json")
			saveJSON(t, corpusPath, data)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				_ = json.NewDecoder(r.Body).Decode(&req)
				if strings.Contains(req["prompt"].(string), "entity-one") || strings.Contains(req["prompt"].(string), "entity-two") {
					t.Error("accepted IDs leaked")
				}
				raw, _ := json.Marshal(map[string]any{"window_id": "N01-a", "candidates": proposals})
				_ = json.NewEncoder(w).Encode(map[string]any{"response": string(raw), "done": true, "done_reason": "stop"})
			}))
			defer server.Close()
			out := filepath.Join(dir, "report.json")
			args := append(compactArgs(server.URL, "N01-a", out), "-corpus", corpusPath)
			if b, err := exec.Command("go", args...).CombinedOutput(); err != nil {
				t.Fatalf("run: %v %s", err, b)
			}
			r := readJSON(t, out)
			run := r["runs"].([]any)[0].(map[string]any)
			if run["retained_count"] != float64(len(proposals)) {
				t.Fatalf("conversion rejected: %v", run)
			}
			expanded := run["proposals"].([]any)
			if variant == "accepted alternatives" {
				if expanded[0].(map[string]any)["candidate"].(map[string]any)["subject"] != "entity-two" || expanded[1].(map[string]any)["candidate"].(map[string]any)["object"] != "entity-one" || expanded[2].(map[string]any)["candidate"].(map[string]any)["subject"] != "new:tea" {
					t.Fatal("identities merged or guessed")
				}
			} else {
				if expanded[0].(map[string]any)["candidate"].(map[string]any)["sources"].([]any)[0].(map[string]any)["start"] != float64(2) {
					t.Fatal("whole alias lost original offset")
				}
			}
		})
	}
}
func TestCompactFailedAttemptsRemainScoreableWithoutRetainedData(t *testing.T) {
	for _, variant := range []string{"timeout", "HTTP", "truncation"} {
		t.Run(variant, func(t *testing.T) {
			dir := t.TempDir()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch variant {
				case "timeout":
					time.Sleep(70 * time.Millisecond)
				case "HTTP":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "truncation":
					_ = json.NewEncoder(w).Encode(map[string]any{"response": "{\"window_id\":\"N01-a\",\"candidates\":[", "done": true, "done_reason": "length"})
				}
			}))
			defer server.Close()
			out := filepath.Join(dir, "report.json")
			args := compactArgs(server.URL, "N01-a,N01-b", out)
			if variant == "timeout" {
				args = append(args, "-timeout", "20ms")
			}
			if b, err := exec.Command("go", args...).CombinedOutput(); err != nil {
				t.Fatalf("run: %v %s", err, b)
			}
			for _, tamper := range []bool{false, true} {
				r := readJSON(t, out)
				path := out
				if tamper {
					run := r["runs"].([]any)[0].(map[string]any)
					run["retained_count"] = 1
					path = filepath.Join(dir, "tampered.json")
					saveJSON(t, path, r)
				}
				scorePath := filepath.Join(dir, fmt.Sprintf("score-%t.json", tamper))
				b, err := exec.Command("go", "run", ".", "-score", path, "-output", scorePath).CombinedOutput()
				if tamper {
					if err == nil {
						t.Fatal("failed status bypassed retained validation")
					}
					continue
				}
				if err != nil {
					t.Fatalf("failed attempt could not score: %v %s", err, b)
				}
				summary := readJSON(t, scorePath)["reports"].([]any)[0].(map[string]any)
				if summary["attempted_runs"] != float64(1) || summary["unexecuted_runs"] != float64(1) || summary["raw"].(map[string]any)["required_denominator"] != float64(1) || summary["retained"].(map[string]any)["proposals"] != float64(0) {
					t.Fatalf("failed denominator lost: %v", summary)
				}
			}
		})
	}
}
