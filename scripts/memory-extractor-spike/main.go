// Command memory-extractor-spike runs bounded synthetic extraction experiments.
// It has no connection to Evie's event database or accepted-memory write paths.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type source struct {
	EventID   string `json:"event_id"`
	SessionID string `json:"session_id"`
	Scope     string `json:"scope"`
	EventPart string `json:"event_part"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Text      string `json:"text"`
	SHA256    string `json:"sha256"`
	Authority string `json:"authority"`
	Ownership string `json:"ownership"`
}
type input struct {
	Scope           string            `json:"scope"`
	Support         []source          `json:"support"`
	Context         []source          `json:"context"`
	AcceptedContext []json.RawMessage `json:"accepted_context"`
}
type window struct {
	ID               string `json:"id"`
	Closure          string `json:"closure"`
	CapturedSequence int    `json:"captured_sequence"`
	Input            input  `json:"input"`
}
type event struct {
	Sequence      int             `json:"sequence"`
	RecordedAt    string          `json:"recorded_at"`
	ID            string          `json:"id"`
	SessionID     string          `json:"session_id"`
	Scope         string          `json:"scope"`
	Type          string          `json:"type"`
	Role          string          `json:"role"`
	ParentID      string          `json:"parent_id"`
	ExecutionID   string          `json:"execution_id"`
	Content       string          `json:"content"`
	Payload       json.RawMessage `json:"payload"`
	FormatVersion int             `json:"format_version"`
}
type history struct {
	ID             string   `json:"id"`
	Split          string   `json:"split"`
	VariantLineage string   `json:"variant_lineage"`
	Events         []event  `json:"events"`
	Windows        []window `json:"windows"`
}
type corpus struct {
	SchemaVersion  string    `json:"schema_version"`
	SyntheticOnly  bool      `json:"synthetic_only"`
	EvidencePolicy string    `json:"evidence_policy"`
	Histories      []history `json:"histories"`
}
type reference struct {
	EventID string `json:"event_id"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}
type meaning struct {
	Subject    string `json:"subject"`
	Predicate  string `json:"predicate"`
	ObjectKind string `json:"object_kind"`
	Object     string `json:"object"`
	Polarity   string `json:"polarity"`
	Kind       string `json:"kind"`
	Temporal   string `json:"temporal"`
	Identity   string `json:"identity"`
	Effect     string `json:"effect"`
}
type candidate struct {
	meaning
	Scope   string      `json:"scope"`
	Sources []reference `json:"sources"`
	Context []reference `json:"context"`
}
type proposalResult struct {
	WireRaw   json.RawMessage `json:"wire_raw,omitempty"`
	Expanded  bool            `json:"expanded,omitempty"`
	Candidate candidate       `json:"candidate"`
	Retained  bool            `json:"retained"`
	Rejection string          `json:"rejection,omitempty"`
}
type run struct {
	Compact               *compactRecord   `json:"compact,omitempty"`
	OriginalStatus        string           `json:"original_status,omitempty"`
	OriginalRetainedCount int              `json:"original_retained_count,omitempty"`
	ServerRelease         string           `json:"server_release"`
	CaseID                string           `json:"case_id"`
	Repetition            int              `json:"repetition"`
	Seed                  int              `json:"seed"`
	Status                string           `json:"status"`
	Error                 string           `json:"error,omitempty"`
	LatencyMS             float64          `json:"latency_ms"`
	RequestBytes          int              `json:"request_bytes"`
	RequestSHA256         string           `json:"request_sha256"`
	Raw                   string           `json:"raw,omitempty"`
	RawSHA256             string           `json:"raw_sha256,omitempty"`
	RawCount              int              `json:"raw_count"`
	RetainedCount         int              `json:"retained_count"`
	Proposals             []proposalResult `json:"proposals"`
	PromptTokens          int              `json:"prompt_tokens"`
	OutputTokens          int              `json:"output_tokens"`
	LoadNS                int64            `json:"load_ns"`
	PromptNS              int64            `json:"prompt_ns"`
	EvaluationNS          int64            `json:"evaluation_ns"`
}
type report struct {
	PreparedRequests        []*compactRecord `json:"prepared_requests,omitempty"`
	PreflightOnly           bool             `json:"preflight_only,omitempty"`
	WireVersion             string           `json:"wire_version,omitempty"`
	SchemaDerivationVersion string           `json:"schema_derivation_version,omitempty"`
	StopOnFailure           bool             `json:"stop_on_failure,omitempty"`
	ExecutableSHA256        string           `json:"executable_sha256,omitempty"`
	BudgetSHA256            string           `json:"budget_sha256,omitempty"`
	OriginalReport          string           `json:"original_report,omitempty"`
	OriginalReportSHA256    string           `json:"original_report_sha256,omitempty"`
	ValidationVersion       string           `json:"validation_version,omitempty"`
	ValidationCodeSHA256    string           `json:"validation_code_sha256,omitempty"`
	PlannedCases            []string         `json:"planned_cases"`
	Repetitions             int              `json:"repetitions"`
	Version                 string           `json:"version"`
	StartedAt               string           `json:"started_at"`
	QualityStatus           string           `json:"quality_status"`
	CorpusSHA256            string           `json:"corpus_sha256"`
	PromptSHA256            string           `json:"prompt_sha256"`
	SchemaSHA256            string           `json:"schema_sha256"`
	EvidencePolicy          string           `json:"evidence_policy"`
	Model                   string           `json:"model"`
	Mode                    string           `json:"mode"`
	ContextTokens           int              `json:"context_tokens"`
	MaxOutputTokens         int              `json:"max_output_tokens"`
	Temperature             float64          `json:"temperature"`
	Endpoint                string           `json:"endpoint"`
	GoVersion               string           `json:"go_version"`
	Runs                    []run            `json:"runs"`
}

func digest(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }
func root() string {
	p, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
			return p
		}
		next := filepath.Dir(p)
		if next == p {
			return "."
		}
		p = next
	}
}
func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func execute() error {
	base := filepath.Join(root(), "cmd/evie/docs/fixtures/memory-stage-4-spike/v1")
	corpusPath := flag.String("corpus", filepath.Join(base, "development.json"), "source-only synthetic corpus")
	promptPath := flag.String("prompt", filepath.Join(base, "prompt.txt"), "frozen extraction prompt")
	schemaPath := flag.String("schema", filepath.Join(base, "output.schema.json"), "frozen output JSON schema")
	endpoint := flag.String("endpoint", "http://127.0.0.1:11434", "literal loopback HTTP origin; proxies and redirects disabled")
	model := flag.String("model", "mistral:latest", "cached model name; pin and verify artifacts separately")
	mode := flag.String("mode", "schema", "schema or json")
	wire := flag.String("wire", "canonical", "canonical or experimental compact-v1/compact-v2/compact-v3")
	preflightOnly := flag.Bool("preflight-only", false, "write verified compact request plan without inference")
	only := flag.String("only", "", "comma-separated exact case IDs; empty selects the corpus")
	output := flag.String("output", "", "new report file (required; never overwrite)")
	score := flag.String("score", "", "comma-separated recorded reports to score offline")
	gold := flag.String("gold", filepath.Join(base, "development.gold.json"), "separate reviewed gold for offline scoring only")
	annotation := flag.String("annotation", filepath.Join(base, "annotation-record.json"), "actual human approval record")
	adjudications := flag.String("adjudications", "", "optional separately human-approved output judgments")
	budgets := flag.String("budgets", filepath.Join(base, "token-budgets.json"), "verified exact-request context budgets; no unmeasured input is sent")
	revalidate := flag.String("revalidate", "", "recorded report to revalidate offline; raw inference remains unchanged")
	stopOnFailure := flag.Bool("stop-on-failure", false, "stop after first non-ok response; preserve attempted and unexecuted runs")
	repetitions := flag.Int("repetitions", 2, "1 to 3 repetitions, seeds 17 onward")
	contextTokens := flag.Int("context", 4096, "bounded context tokens, 2048 to 8192")
	maxTokens := flag.Int("max-tokens", 768, "bounded output tokens, 32 to 1024")
	timeout := flag.Duration("timeout", 60*time.Second, "per-request timeout, at most 90s")
	flag.Parse()
	if *wire != "canonical" && !isCompactWire(*wire) {
		return errors.New("unknown wire configuration")
	}
	if *revalidate != "" {
		if *output == "" {
			return errors.New("missing -output")
		}
		return revalidateReport(*revalidate, *corpusPath, *budgets, *promptPath, *schemaPath, *output)
	}
	if *score != "" {
		if *output == "" {
			return errors.New("missing -output")
		}
		return scoreReports(*score, *gold, *annotation, *adjudications, *output)
	}
	if *output == "" || *repetitions < 1 || *repetitions > 3 || *contextTokens < 2048 || *contextTokens > 8192 || *maxTokens < 32 || *maxTokens > 1024 || *timeout <= 0 || *timeout > 90*time.Second {
		return errors.New("invalid bounds or missing -output")
	}
	if *mode != "schema" && *mode != "json" {
		return errors.New("mode must be schema or json")
	}
	if isCompactWire(*wire) && (*mode != "schema" || *model != "qwen2.5:7b-instruct-q4_K_M" || *contextTokens != 8192 || *maxTokens != 768 || *repetitions != 1 || !*stopOnFailure) {
		return fmt.Errorf("%s requires pinned Qwen schema,8192 context,768 output,one repetition and stop-on-failure", *wire)
	}
	if (*wire == "compact-v2" || *wire == "compact-v3") && *timeout != 60*time.Second {
		return fmt.Errorf("%s requires pinned60s timeout", *wire)
	}
	if _, err := os.Stat(*output); err == nil {
		return errors.New("output already exists")
	}
	client, origin, err := localClient(*endpoint)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	corpusBytes, err := readBounded(*corpusPath, 2<<20)
	if err != nil {
		return err
	}
	var data corpus
	if err := json.Unmarshal(corpusBytes, &data); err != nil {
		return err
	}
	if !data.SyntheticOnly || data.SchemaVersion != "evie-extraction-spike-input-v1" {
		return errors.New("only the versioned synthetic spike corpus is permitted")
	}
	prompt, err := readBounded(*promptPath, 12<<10)
	if err != nil {
		return err
	}
	schema, err := readBounded(*schemaPath, 12<<10)
	if err != nil {
		return err
	}
	if !json.Valid(schema) {
		return errors.New("invalid output schema")
	}
	budget, err := readBudget(*budgets, prompt, schema)
	if err != nil {
		return err
	}
	if (isCompactWire(*wire) && budget.Version != compactBudgetVersion(*wire)) || (!isCompactWire(*wire) && (budget.Version == compactBudgetVersion("compact-v1") || budget.Version == compactBudgetVersion("compact-v2") || budget.Version == compactBudgetVersion("compact-v3"))) {
		return errors.New("wire/budget configuration mismatch")
	}
	r := report{Version: "evie-extraction-spike-report-v1", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), QualityStatus: "unadjudicated", CorpusSHA256: digest(corpusBytes), PromptSHA256: digest(prompt), SchemaSHA256: digest(schema), EvidencePolicy: data.EvidencePolicy, Model: *model, Mode: *mode, ContextTokens: *contextTokens, MaxOutputTokens: *maxTokens, Endpoint: origin, GoVersion: runtime.Version(), Runs: []run{}}
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err := readBounded(exePath, 64<<20)
	if err != nil {
		return err
	}
	budgetBytes, err := readBounded(*budgets, 128<<10)
	if err != nil {
		return err
	}
	if isCompactWire(*wire) {
		r.WireVersion = *wire
	}
	if *wire == "compact-v3" {
		r.SchemaDerivationVersion = compactCategoryVersion
	}
	r.ExecutableSHA256 = digest(executable)
	r.BudgetSHA256 = digest(budgetBytes)
	r.ValidationCodeSHA256, err = validationIdentity()
	if err != nil {
		return err
	}
	r.ValidationVersion = "standalone-source-checks-v2"
	r.StopOnFailure = *stopOnFailure
	r.Repetitions = *repetitions
	selected := map[string]bool{}
	if *only != "" {
		for _, id := range strings.Split(*only, ",") {
			if id == "" || selected[id] {
				return errors.New("empty/duplicate selected case")
			}
			selected[id] = true
		}
	}
	for _, h := range data.Histories {
		for _, w := range h.Windows {
			if *only == "" || selected[w.ID] {
				r.PlannedCases = append(r.PlannedCases, w.ID)
				if err := validateInput(h, w); err != nil {
					return fmt.Errorf("%s input: %w", w.ID, err)
				}
				for rep := 0; rep < *repetitions; rep++ {
					body, prepared, err := experimentRequest(h, w, data.EvidencePolicy, r.CorpusSHA256, *wire, prompt, schema, *mode, *model, *contextTokens, *maxTokens, 17+rep)
					if err != nil {
						return err
					}
					if len(body) > 32<<10 {
						return fmt.Errorf("%s pre-dispatch: serialized request exceeds32768 bytes", w.ID)
					}
					if err := budget.check(body, *model, *mode, *contextTokens, *maxTokens); err != nil {
						return fmt.Errorf("%s repetition%d pre-dispatch: %w", w.ID, rep+1, err)
					}
					if prepared != nil {
						r.PreparedRequests = append(r.PreparedRequests, prepared)
					}
				}
			}
		}
	}
	if len(r.PlannedCases) == 0 || len(r.PlannedCases) > 32 || (*only != "" && len(r.PlannedCases) != len(selected)) {
		return errors.New("unknown/oversized case selection")
	}
	if *preflightOnly {
		if !isCompactWire(*wire) {
			return errors.New("preflight-only requires a compact configuration")
		}
		r.PreflightOnly = true
		return writeReport(r, *output)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	count := 0
experiments:
	for _, h := range data.Histories {
		for _, w := range h.Windows {
			if *only != "" && !selected[w.ID] {
				continue
			}
			count++
			if count > 32 {
				return errors.New("corpus exceeds 32 selected windows")
			}
			if err := validateInput(h, w); err != nil {
				return fmt.Errorf("%s input: %w", w.ID, err)
			}
			for rep := 0; rep < *repetitions; rep++ {
				if ctx.Err() != nil {
					break
				}
				body, compact, err := experimentRequest(h, w, data.EvidencePolicy, r.CorpusSHA256, *wire, prompt, schema, *mode, *model, *contextTokens, *maxTokens, 17+rep)
				if err != nil {
					return err
				}
				result := run{Compact: compact, CaseID: w.ID, Repetition: rep + 1, Seed: 17 + rep, RequestBytes: len(body), RequestSHA256: digest(body), Proposals: []proposalResult{}}
				if len(body) > 32<<10 {
					result.Status = "input_bound"
					result.Error = "serialized request exceeds 32768 bytes"
				} else {
					requestCtx, cancel := context.WithTimeout(ctx, *timeout)
					start := time.Now()
					result = infer(requestCtx, client, origin, body, result, w.Input)
					result.LatencyMS = float64(time.Since(start).Microseconds()) / 1000
					cancel()
				}
				r.Runs = append(r.Runs, result)
				fmt.Fprintf(os.Stderr, "%s repetition %d: %s (%.0f ms, raw=%d retained=%d)\n", w.ID, rep+1, result.Status, result.LatencyMS, result.RawCount, result.RetainedCount)
				if result.ServerRelease == "unknown" || (*stopOnFailure && result.Status != "ok") {
					break experiments
				}
			}
		}
	}
	if count == 0 {
		return errors.New("no selected cases")
	}
	return writeReport(r, *output)
}

func writeReport(r report, output string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > 4<<20 {
		return errors.New("report exceeds 4 MiB")
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(b, '\n'))
	closeErr := f.Close()
	return errors.Join(writeErr, closeErr)
}

func requestBody(in any, prompt, schema []byte, mode, model string, contextTokens, maxTokens, seed int) []byte {
	data, _ := json.Marshal(in)
	var format any = "json"
	if mode == "schema" {
		format = json.RawMessage(schema)
	}
	body, _ := json.Marshal(map[string]any{"model": model, "system": string(prompt), "prompt": string(data), "format": format, "stream": false, "keep_alive": "1m", "options": map[string]any{"temperature": 0, "seed": seed, "num_ctx": contextTokens, "num_predict": maxTokens}})
	return body
}

func localClient(endpoint string) (*http.Client, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, "", errors.New("endpoint must be a loopback HTTP origin without credentials/path/query")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() || u.Port() == "" {
		return nil, "", errors.New("endpoint must use a literal loopback IP and explicit port")
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1, ResponseHeaderTimeout: 90 * time.Second}
	return &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}, strings.TrimSuffix(u.String(), "/"), nil
}
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(b)) > limit {
		return nil, errors.New("file exceeds bound")
	}
	return b, err
}
func infer(ctx context.Context, client *http.Client, origin string, body []byte, result run, in input) run {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/api/generate", bytes.NewReader(body))
	if err != nil {
		result.Status = "request_error"
		result.Error = err.Error()
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	result.ServerRelease = "unknown"
	resp, err := client.Do(req)
	if err != nil {
		result.Status = "transport_error"
		if ctx.Err() != nil {
			result.Status = "cancelled_or_timeout"
		}
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10+1))
	if err != nil || len(b) > 64<<10 {
		result.Status = "response_bound_or_read_error"
		return result
	}
	if resp.StatusCode != http.StatusOK {
		result.Status = "http_error"
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result
	}
	var envelope struct {
		Response           string `json:"response"`
		Done               bool   `json:"done"`
		DoneReason         string `json:"done_reason"`
		EvalCount          int    `json:"eval_count"`
		PromptEvalCount    int    `json:"prompt_eval_count"`
		LoadDuration       int64  `json:"load_duration"`
		PromptEvalDuration int64  `json:"prompt_eval_duration"`
		EvalDuration       int64  `json:"eval_duration"`
	}
	if err = json.Unmarshal(b, &envelope); err != nil {
		result.Status = "invalid_envelope"
		return result
	}
	if envelope.Done {
		result.ServerRelease = "finished_response"
	}
	result.OutputTokens = envelope.EvalCount
	result.PromptTokens = envelope.PromptEvalCount
	result.LoadNS = envelope.LoadDuration
	result.PromptNS = envelope.PromptEvalDuration
	result.EvaluationNS = envelope.EvalDuration
	result.RawSHA256 = digest([]byte(envelope.Response))
	if len(envelope.Response) > 16<<10 {
		result.Status = "output_bound"
		return result
	}
	result.Raw = strings.ReplaceAll(envelope.Response, "EVIE_SPIKE_SECRET_DO_NOT_SEND", "[excluded synthetic marker]")
	if ctx.Err() != nil {
		result.Status = "cancelled_or_timeout"
		result.Raw = ""
		return result
	}
	if !envelope.Done || envelope.DoneReason == "length" {
		result.Status = "truncated_output"
		return result
	}
	if result.Compact != nil {
		return checkCompactProposals(envelope.Response, result, in)
	}
	return checkProposals(envelope.Response, result, in)
}

func checkProposals(raw string, result run, in input) run {
	var err error
	result.Proposals = []proposalResult{}
	result.RetainedCount = 0
	result.RawCount = 0
	if err = validateShape(raw); err != nil {
		result.Status = "schema_error"
		result.Error = err.Error()
		return result
	}
	var proposed struct {
		Candidates []candidate `json:"candidates"`
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&proposed); err != nil {
		result.Status = "schema_error"
		result.Error = err.Error()
		return result
	}
	if err = d.Decode(new(any)); err != io.EOF || proposed.Candidates == nil || len(proposed.Candidates) > 8 {
		result.Status = "schema_error"
		return result
	}
	result.Status = "ok"
	result.RawCount = len(proposed.Candidates)
	for _, c := range proposed.Candidates {
		p := proposalResult{Candidate: c}
		if err = validateCandidate(c, in); err != nil {
			p.Rejection = err.Error()
		} else {
			p.Retained = true
			result.RetainedCount++
		}
		result.Proposals = append(result.Proposals, p)
	}
	return result
}
