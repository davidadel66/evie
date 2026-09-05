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
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecutableSendsOnlyProjectedSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		payload, _ := json.Marshal(req)
		for _, forbidden := range []string{"required_useful", "annotation_status", "I ate a pear", "no_memory_label"} {
			if strings.Contains(string(payload), forbidden) {
				t.Errorf("request leaked %q", forbidden)
			}
		}
		if !strings.Contains(string(payload), "I prefer tea.") {
			t.Error("missing exact source")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"{\"candidates\":[]}","done":true,"done_reason":"stop","eval_count":4}`))
	}))
	defer server.Close()
	out := filepath.Join(t.TempDir(), "report.json")
	cmd := exec.Command("go", "run", ".", "-endpoint", server.URL, "-only", "N01-a", "-repetitions", "1", "-output", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Runs []struct {
			Status string `json:"status"`
		} `json:"runs"`
		QualityStatus string `json:"quality_status"`
	}
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 || report.Runs[0].Status != "ok" {
		t.Fatalf("unexpected report: %s", b)
	}
	if report.QualityStatus != "unadjudicated" {
		t.Fatal("unreviewed gold became a quality claim")
	}
}

func TestTimeoutFencesLateOutputAndStopsFurtherRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte(`{"response":"{\"candidates\":[]}","done":true}`))
	}))
	defer server.Close()
	out := filepath.Join(t.TempDir(), "report.json")
	cmd := exec.Command("go", "run", ".", "-endpoint", server.URL, "-only", "N01-a", "-repetitions", "2", "-timeout", "40ms", "-output", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run: %v %s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Runs []struct {
			Status        string  `json:"status"`
			ServerRelease string  `json:"server_release"`
			RetainedCount int     `json:"retained_count"`
			LatencyMS     float64 `json:"latency_ms"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(report.Runs) != 1 {
		t.Fatalf("new inference after uncertain release: calls=%d report=%s", calls.Load(), b)
	}
	r := report.Runs[0]
	if r.Status != "cancelled_or_timeout" || r.ServerRelease != "unknown" || r.RetainedCount != 0 || r.LatencyMS > 1000 {
		t.Fatalf("late effect/client return boundary: %s", b)
	}
}

func TestExecutableRejectsMalformedAndUnboundOutput(t *testing.T) {
	valid := `{"candidates":[{"subject":"owner","predicate":"preference","object_kind":"text","object":"tea","polarity":"affirmed","kind":"fact","temporal":"","identity":"resolved","effect":"assert","scope":"project:553ecf4c-6a4f-50d4-94e1-8c37985464a7","sources":[{"event_id":"0d8102f9-c956-5110-82d9-4853195285fd","start":0,"end":13}],"context":[]}]}`
	for _, tc := range []struct {
		name, raw, status string
		done              bool
		retained          int
	}{
		{"exact whole source", valid, "ok", true, 1},
		{"wrong source", strings.ReplaceAll(valid, "0d8102f9-c956-5110-82d9-4853195285fd", "missing"), "ok", true, 0},
		{"wrong scope", strings.ReplaceAll(valid, "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7", "global"), "ok", true, 0},
		{"malformed", `{"candidates":[`, "schema_error", true, 0},
		{"truncated", valid, "truncated_output", false, 0},
		{"unknown field", `{"candidates":[],"approve":true}`, "schema_error", true, 0},
		{"missing field", strings.ReplaceAll(valid, `"temporal":"",`, ""), "schema_error", true, 0},
		{"duplicate key", `{"candidates":[],"candidates":[]}`, "schema_error", true, 0},
		{"combined enum", strings.ReplaceAll(valid, `"affirmed"`, `"affirmed|denied"`), "ok", true, 0},
		{"null support start", strings.ReplaceAll(valid, `"start":0`, `"start":null`), "schema_error", true, 0},
		{"null context start", strings.ReplaceAll(valid, `"context":[]`, `"context":[{"event_id":"missing","start":null,"end":13}]`), "schema_error", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"response": tc.raw, "done": tc.done})
			}))
			defer server.Close()
			out := filepath.Join(t.TempDir(), "report.json")
			cmd := exec.Command("go", "run", ".", "-endpoint", server.URL, "-only", "N01-a", "-repetitions", "1", "-output", out)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("run: %v %s", err, b)
			}
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				Runs []struct {
					Status        string `json:"status"`
					RetainedCount int    `json:"retained_count"`
				} `json:"runs"`
			}
			if err := json.Unmarshal(b, &report); err != nil {
				t.Fatal(err)
			}
			if len(report.Runs) != 1 || report.Runs[0].Status != tc.status || report.Runs[0].RetainedCount != tc.retained {
				t.Fatalf("unexpected result: %s", b)
			}
		})
	}
}

func TestExecutableRejectsBrokenClockAncestryBeforeRequest(t *testing.T) {
	base := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "memory-stage-4-spike", "v1", "development.json")
	for _, variant := range []string{"valid", "missing", "cycle", "foreign session", "foreign scope", "format"} {
		t.Run(variant, func(t *testing.T) {
			b, err := os.ReadFile(base)
			if err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			if json.Unmarshal(b, &data) != nil {
				t.Fatal("invalid fixture")
			}
			for _, hv := range data["histories"].([]any) {
				h := hv.(map[string]any)
				if h["id"] != "N08" {
					continue
				}
				for _, ev := range h["events"].([]any) {
					e := ev.(map[string]any)
					if e["type"] != "assistant_message" {
						continue
					}
					payload := e["payload"].(map[string]any)
					if _, ok := payload["tool_calls"]; !ok {
						continue
					}
					switch variant {
					case "missing":
						e["parent_id"] = "missing"
					case "cycle":
						e["parent_id"] = e["id"]
					case "foreign session":
						e["session_id"] = "other"
					case "foreign scope":
						e["scope"] = "global"
					case "format":
						e["format_version"] = 2
					}
				}
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "sources.json")
			b, _ = json.Marshal(data)
			if os.WriteFile(path, b, 0600) != nil {
				t.Fatal("write fixture")
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(`{"response":"{\"candidates\":[]}","done":true,"done_reason":"stop"}`))
			}))
			defer server.Close()
			cmd := exec.Command("go", "run", ".", "-endpoint", server.URL, "-corpus", path, "-only", "N08-b", "-repetitions", "1", "-output", filepath.Join(dir, "report.json"))
			b, err = cmd.CombinedOutput()
			if variant == "valid" {
				if err != nil || calls.Load() != 1 {
					t.Fatalf("valid clock did not dispatch: %v %s", err, b)
				}
				return
			}
			if err == nil || !strings.Contains(string(b), "clock ancestry") {
				t.Fatalf("missing specific clock ancestry rejection: %v %s", err, b)
			}
			if calls.Load() != 0 {
				t.Fatal("inference saw broken clock input")
			}
		})
	}
}

func TestExecutableLocalTransportFences(t *testing.T) {
	var escaped atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
	defer other.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer redirect.Close()
	oversize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 65537))) }))
	defer oversize.Close()
	unavailable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unavailableURL := unavailable.URL
	unavailable.Close()
	for _, tc := range []struct {
		name, endpoint, want string
		failure              bool
	}{
		{"redirect", redirect.URL, "transport_error", false},
		{"response bound", oversize.URL, "response_bound_or_read_error", false},
		{"unavailable", unavailableURL, "transport_error", false},
		{"nonlocal", "http://192.0.2.1:11434", "literal loopback", true},
		{"DNS hostname", "http://localhost:11434", "literal loopback", true},
		{"credentials", "http://user:password@127.0.0.1:11434", "without credentials", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "report.json")
			cmd := exec.Command("go", "run", ".", "-endpoint", tc.endpoint, "-only", "N01-a", "-repetitions", "1", "-output", out)
			cmd.Env = append(os.Environ(), "HTTP_PROXY="+other.URL, "HTTPS_PROXY="+other.URL, "ALL_PROXY="+other.URL)
			b, err := cmd.CombinedOutput()
			if tc.failure {
				if err == nil || !strings.Contains(string(b), tc.want) {
					t.Fatalf("expected rejection: %v %s", err, b)
				}
				return
			}
			if err != nil {
				t.Fatalf("run: %v %s", err, b)
			}
			b, err = os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), `"status": "`+tc.want+`"`) {
				t.Fatalf("wrong result: %s", b)
			}
		})
	}
	if escaped.Load() != 0 {
		t.Fatal("redirect/proxy/fallback received a request")
	}
}

func TestOfflineScoreKeepsUnknownMeaningSeparateFromGold(t *testing.T) {
	root := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "memory-stage-4-spike", "v1")
	b, err := os.ReadFile(filepath.Join(root, "development.json"))
	if err != nil {
		t.Fatal(err)
	}
	correct := map[string]any{"subject": "owner", "predicate": "preference", "object_kind": "text", "object": "tea", "polarity": "affirmed", "kind": "fact", "temporal": "", "identity": "resolved", "effect": "assert", "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7", "sources": []any{map[string]any{"event_id": "0d8102f9-c956-5110-82d9-4853195285fd", "start": 0, "end": 13}}, "context": []any{}}
	wrong := map[string]any{}
	for k, v := range correct {
		wrong[k] = v
	}
	wrong["subject"] = "project"
	raw, _ := json.Marshal(map[string]any{"candidates": []any{correct, wrong}})
	r := map[string]any{"version": "evie-extraction-spike-report-v1", "mode": "schema", "corpus_sha256": fmt.Sprintf("sha256:%x", sha256.Sum256(b)), "planned_cases": []string{"N01-a"}, "repetitions": 2, "runs": []any{map[string]any{"case_id": "N01-a", "status": "ok", "raw": string(raw), "latency_ms": 10, "proposals": []any{map[string]any{"candidate": correct, "retained": true}, map[string]any{"candidate": wrong, "retained": true}}}}}
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "input-report.json")
	data, _ := json.Marshal(r)
	if err := os.WriteFile(reportPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "score.json")
	cmd := exec.Command("go", "run", ".", "-score", reportPath, "-output", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("score: %v %s", err, b)
	}
	b, err = os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Status  string `json:"status"`
		Reports []struct {
			UnexecutedRuns int `json:"unexecuted_runs"`
			Raw            struct {
				Proposals           int `json:"proposals"`
				SupportedUseful     int `json:"supported_useful"`
				Unadjudicated       int `json:"unadjudicated"`
				RequiredMatches     int `json:"required_matches"`
				RequiredDenominator int `json:"required_denominator"`
			} `json:"raw"`
		} `json:"reports"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Status != "output_adjudication_pending" || len(s.Reports) != 1 {
		t.Fatalf("wrong score: %s", b)
	}
	x := s.Reports[0]
	if x.UnexecutedRuns != 1 || x.Raw.Proposals != 2 || x.Raw.SupportedUseful != 1 || x.Raw.Unadjudicated != 1 || x.Raw.RequiredMatches != 1 || x.Raw.RequiredDenominator != 1 {
		t.Fatalf("denominators or unreviewed meaning changed: %s", b)
	}
}

func TestEveryTokenBudgetIsCheckedBeforeAnyDispatch(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	for _, args := range [][]string{{"-only", "N01-a", "-repetitions", "3"}, {"-only", "N01-a,N03-a", "-repetitions", "1"}, {"-only", "N01-a", "-repetitions", "1", "-context", "2048"}, {"-only", "N01-a", "-budgets", "/nonexistent/spike-budget.json"}} {
		args = append([]string{"run", ".", "-endpoint", server.URL, "-output", filepath.Join(t.TempDir(), "report.json")}, args...)
		cmd := exec.Command("go", args...)
		if b, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("unmeasured request accepted: %s", b)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("dispatched%d requests before checking all budgets", calls.Load())
	}
}

func TestCorrectedPromptBudgetAndWholeBatchBoundary(t *testing.T) {
	base := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "memory-stage-4-spike", "v1")
	prompt, err := os.ReadFile(filepath.Join(base, "prompt-v2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"schema", "json", "later oversized input", "changed prompt"} {
		t.Run(variant, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request map[string]any
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("bad request")
				}
				if request["system"] != string(prompt) {
					t.Error("full common field contract missing")
				}
				_, _ = w.Write([]byte(`{"response":"{\"candidates\":[]}","done":true,"done_reason":"stop"}`))
			}))
			defer server.Close()
			dir := t.TempDir()
			args := []string{"run", ".", "-endpoint", server.URL, "-prompt", filepath.Join(base, "prompt-v2.txt"), "-budgets", filepath.Join(base, "token-budgets-v2.json"), "-context", "8192", "-only", "N01-a,N01-b", "-repetitions", "1", "-output", filepath.Join(dir, "report.json")}
			wantError := ""
			switch variant {
			case "json":
				args = append(args, "-mode", "json")
			case "changed prompt":
				p := filepath.Join(dir, "changed.txt")
				if err := os.WriteFile(p, append(prompt, 'x'), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "-prompt", p)
				wantError = "prompt/schema identity mismatch"
			case "later oversized input":
				b, err := os.ReadFile(filepath.Join(base, "development.json"))
				if err != nil {
					t.Fatal(err)
				}
				var c map[string]any
				if json.Unmarshal(b, &c) != nil {
					t.Fatal("bad sources")
				}
				h := c["histories"].([]any)[0].(map[string]any)
				window := h["windows"].([]any)[1].(map[string]any)
				fields := window["input"].(map[string]any)["support"].([]any)
				for _, f := range fields {
					field := f.(map[string]any)
					if field["ownership"] != "new" {
						continue
					}
					content := strings.Repeat("x", 6000)
					field["text"] = content
					field["end"] = len(content)
					field["sha256"] = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content)))
					for _, v := range h["events"].([]any) {
						e := v.(map[string]any)
						if e["id"] == field["event_id"] {
							e["content"] = content
						}
					}
				}
				p := filepath.Join(dir, "large.json")
				b, _ = json.Marshal(c)
				if os.WriteFile(p, b, 0600) != nil {
					t.Fatal("write source")
				}
				args = append(args, "-corpus", p)
				wantError = "proven prompt byte bound"
			}
			b, err := exec.Command("go", args...).CombinedOutput()
			if wantError != "" {
				if err == nil || !strings.Contains(string(b), wantError) || calls.Load() != 0 {
					t.Fatalf("batch was not rejected before all inference: %v calls=%d %s", err, calls.Load(), b)
				}
			} else if err != nil || calls.Load() != 2 {
				t.Fatalf("valid full prompt did not dispatch: %v calls=%d %s", err, calls.Load(), b)
			}
		})
	}
}

func TestOfflineAdjudicationRequiresExactApprovalProvenance(t *testing.T) {
	base := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "memory-stage-4-spike", "v1")
	for _, variant := range []string{"gold binding", "packet binding"} {
		t.Run(variant, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(base, "output-adjudications.proposed.json"))
			if err != nil {
				t.Fatal(err)
			}
			var a map[string]any
			if json.Unmarshal(b, &a) != nil {
				t.Fatal("bad proposed fixture")
			}
			// This deliberately fabricated approval exists only in a temporary negative test.
			a["status"] = "human_approved"
			a["reviewer"] = "test fixture"
			a["reviewed_at"] = "2000-01-01T00:00:00Z"
			want := "adjudication packet/source-score hash mismatch"
			if variant == "gold binding" {
				a["gold_sha256"] = "sha256:wrong"
				want = "adjudication gold binding mismatch"
			}
			dir := t.TempDir()
			p := filepath.Join(dir, "adjudications.json")
			b, _ = json.Marshal(a)
			if os.WriteFile(p, b, 0600) != nil {
				t.Fatal("write test fixture")
			}
			b, err = exec.Command("go", "run", ".", "-score", filepath.Join(base, "reports", "development-schema-v2.json"), "-adjudications", p, "-output", filepath.Join(dir, "score.json")).CombinedOutput()
			if err == nil || !strings.Contains(string(b), want) {
				t.Fatalf("provenance mismatch accepted: %v %s", err, b)
			}
		})
	}
}

func TestStopOnFailurePreservesFailedAttemptAndUnexecutedPlan(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"response":"{\"wrong\":[]}","done":true,"done_reason":"stop"}`))
	}))
	defer server.Close()
	out := filepath.Join(t.TempDir(), "report.json")
	b, err := exec.Command("go", "run", ".", "-endpoint", server.URL, "-only", "N01-a,N01-b", "-repetitions", "2", "-stop-on-failure", "-output", out).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v %s", err, b)
	}
	b, err = os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		PlannedCases []string `json:"planned_cases"`
		Repetitions  int      `json:"repetitions"`
		Runs         []struct {
			Status        string `json:"status"`
			ServerRelease string `json:"server_release"`
		} `json:"runs"`
	}
	if json.Unmarshal(b, &r) != nil {
		t.Fatal("bad report")
	}
	if calls.Load() != 1 || len(r.PlannedCases) != 2 || r.Repetitions != 2 || len(r.Runs) != 1 || r.Runs[0].Status != "schema_error" || r.Runs[0].ServerRelease != "finished_response" {
		t.Fatalf("failed attempt or unexecuted plan lost: calls=%d %s", calls.Load(), b)
	}
}

func TestQwenPinnedBPEBudgetAtExactBoundary(t *testing.T) {
	base := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "memory-stage-4-spike", "v1")
	for _, variant := range []string{"at boundary", "one byte over", "changed model", "changed template", "missing proof"} {
		t.Run(variant, func(t *testing.T) {
			dir := t.TempDir()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request map[string]any
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request")
				}
				if len(request["prompt"].(string)) != 1690 {
					t.Error("boundary request was not exact")
				}
				_, _ = w.Write([]byte(`{"response":"{\"candidates\":[]}","done":true,"done_reason":"stop"}`))
			}))
			defer server.Close()
			// A temporary synthetic owner field exercises the complete public request,
			// not a private tokenizer helper or the frozen human-approved gold.
			b, err := os.ReadFile(filepath.Join(base, "development.json"))
			if err != nil {
				t.Fatal(err)
			}
			var c map[string]any
			if json.Unmarshal(b, &c) != nil {
				t.Fatal("bad source")
			}
			h := c["histories"].([]any)[0].(map[string]any)
			window := h["windows"].([]any)[0].(map[string]any)
			in := window["input"].(map[string]any)
			field := in["support"].([]any)[0].(map[string]any)
			target := 1690
			if variant == "one byte over" {
				target++
			}
			length := 1000
			for attempts := 0; attempts < 8; attempts++ {
				content := strings.Repeat("x", length)
				field["text"] = content
				field["end"] = len(content)
				field["sha256"] = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content)))
				encoded, _ := json.Marshal(in)
				if len(encoded) == target {
					break
				}
				length += target - len(encoded)
			}
			encoded, _ := json.Marshal(in)
			if len(encoded) != target {
				t.Fatal("could not create exact public input boundary")
			}
			for _, v := range h["events"].([]any) {
				e := v.(map[string]any)
				if e["id"] == field["event_id"] {
					e["content"] = field["text"]
				}
			}
			corpusPath := filepath.Join(dir, "boundary-source.json")
			b, _ = json.Marshal(c)
			if os.WriteFile(corpusPath, b, 0600) != nil {
				t.Fatal("write source")
			}
			budgetPath := filepath.Join(base, "qwen", "token-budgets.json")
			want := ""
			model := "qwen2.5:7b-instruct-q4_K_M"
			switch variant {
			case "one byte over":
				want = "proven prompt byte bound"
			case "changed model":
				model = "mistral:latest"
				want = "unproven byte-budget configuration"
			case "changed template", "missing proof":
				for _, name := range []string{"runtime-manifest.json", "runtime-api-metadata.json", "tokenizer-proof.json", "token-budgets.json"} {
					if variant == "missing proof" && name == "tokenizer-proof.json" {
						continue
					}
					data, err := os.ReadFile(filepath.Join(base, "qwen", name))
					if err != nil {
						t.Fatal(err)
					}
					if variant == "changed template" && name == "runtime-api-metadata.json" {
						data = append(data, ' ')
					}
					if os.WriteFile(filepath.Join(dir, name), data, 0600) != nil {
						t.Fatal("write altered proof fixture")
					}
				}
				budgetPath = filepath.Join(dir, "token-budgets.json")
				want = "identity"
			}
			b, err = exec.Command("go", "run", ".", "-endpoint", server.URL, "-corpus", corpusPath, "-prompt", filepath.Join(base, "prompt-v2.txt"), "-budgets", budgetPath, "-model", model, "-context", "8192", "-only", "N01-a", "-repetitions", "1", "-stop-on-failure", "-output", filepath.Join(dir, "report.json")).CombinedOutput()
			if want == "" {
				if err != nil || calls.Load() != 1 {
					t.Fatalf("valid full Qwen template/bound rejected: %v calls=%d %s", err, calls.Load(), b)
				}
			} else if err == nil || calls.Load() != 0 || !strings.Contains(string(b), want) {
				t.Fatalf("unproven input dispatched: %v calls=%d %s", err, calls.Load(), b)
			}
		})
	}
}
