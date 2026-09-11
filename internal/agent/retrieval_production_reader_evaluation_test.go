package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

// This opt-in development evaluation uses the existing production client and
// complete SQLite-backed turn. It never runs concurrently with another test.
// Only initial read selection is scripted; the provider answers and may request
// up to two further read iterations. No owner data enters these fixtures.
type productionReaderCapture struct {
	t                      *testing.T
	directory, phase, stem string
	metadataCalls          int
	transport              *http.Transport
}

func productionReaderWrite(t *testing.T, directory, name string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	productionReaderRaw(t, directory, name, append(data, '\n'))
}

func productionReaderRaw(t *testing.T, directory, name string, data []byte) {
	t.Helper()
	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" && bytes.Contains(data, []byte(key)) {
		t.Fatal("provider artifact contains a credential; artifact withheld")
	}
	file, err := os.OpenFile(filepath.Join(directory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("write reader artifact: %v; close: %v", writeErr, closeErr)
	}
}

func (c *productionReaderCapture) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" || request.URL.Host != "openrouter.ai" || request.Response != nil {
		return nil, errors.New("reader requires direct official HTTPS endpoint without redirects")
	}
	stem := c.stem
	if request.Method == http.MethodGet {
		c.metadataCalls++
		stem = fmt.Sprintf("%s-metadata-%02d", c.phase, c.metadataCalls)
	} else if request.Method != http.MethodPost || request.URL.Path != "/api/v1/responses" || stem == "" {
		return nil, errors.New("unexpected production reader endpoint")
	}
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(body)
		body.Close()
		if readErr != nil {
			return nil, readErr
		}
		productionReaderRaw(c.t, c.directory, stem+"-wire-request.json", raw)
	}
	response, err := c.transport.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	productionReaderWrite(c.t, c.directory, stem+"-http.json", map[string]any{
		"method": request.Method, "url": request.URL.String(), "status": response.StatusCode,
		"content_type": response.Header.Get("Content-Type"),
	})
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// The normal client classifies the status. Error bodies are not reader
		// evidence and can echo credentials, so they are never retained.
		return response, nil
	}
	response.Body = &productionReaderBody{ReadCloser: response.Body, capture: c, name: stem + "-wire-response.raw"}
	return response, nil
}

type productionReaderBody struct {
	io.ReadCloser
	capture *productionReaderCapture
	name    string
	buffer  bytes.Buffer
}

func (b *productionReaderBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	// Match the production response cap; even malformed data cannot create an
	// unbounded diagnostic artifact. The caller receives the original stream.
	remaining := (16 << 20) + 1 - b.buffer.Len()
	if remaining > 0 {
		b.buffer.Write(data[:min(n, remaining)])
	}
	return n, err
}

func (b *productionReaderBody) Close() error {
	err := b.ReadCloser.Close()
	productionReaderRaw(b.capture.t, b.capture.directory, b.name, b.buffer.Bytes())
	return err
}

func productionReaderSetup(t *testing.T, phase string) (*openrouter.Client, openrouter.ContextProfile, *productionReaderCapture) {
	t.Helper()
	directory := os.Getenv("EVIE_MEMORY_READER_ARTIFACTS")
	if !filepath.IsAbs(directory) {
		t.Fatal("absolute reader artifact directory required")
	}
	if DefaultModel != "openai/gpt-6-astra" {
		t.Fatal("production reader model changed")
	}
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "24576")
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "768")
	t.Setenv("EVIE_REASONING", "low")
	prior := http.DefaultTransport
	transport := prior.(*http.Transport).Clone()
	transport.Proxy = nil
	capture := &productionReaderCapture{t: t, directory: directory, phase: phase, transport: transport}
	http.DefaultTransport = capture
	t.Cleanup(func() { http.DefaultTransport = prior; transport.CloseIdleConnections() })
	client, err := openrouter.NewClient(os.Getenv("OPENROUTER_API_KEY"))
	if err != nil {
		t.Fatal("production reader credential unavailable")
	}
	profile, err := client.ResolveContextProfile(context.Background(), DefaultModel)
	if err != nil {
		t.Fatalf("production profile discovery failed: %v", err)
	}
	if profile.Diagnostics().Source != openrouter.ContextProfileRemoteMetadata {
		t.Fatal("reader requires discovered production metadata without fallback")
	}
	productionReaderWrite(t, directory, phase+"-context-profile.json", profile.Diagnostics())
	return client, profile, capture
}

func TestMemoryStage5ProductionReaderPreflight(t *testing.T) {
	if os.Getenv("EVIE_RUN_PRODUCTION_READER_PREFLIGHT") != "1" {
		t.Skip("production reader metadata discovery is opt-in")
	}
	productionReaderSetup(t, "preflight")
}

type productionReaderClient struct {
	validator *localReaderClient
	provider  *openrouter.Client
	capture   *productionReaderCapture
	answers   []string
	calls     int
}

func (c *productionReaderClient) ChatStream(ctx context.Context, request openrouter.ChatRequest, handlers openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	// Reuse the original exact evidence/source/status assertions unchanged.
	scripted, err := c.validator.ChatStream(ctx, request, openrouter.StreamHandlers{})
	if err != nil || c.validator.calls == 1 {
		return scripted, err
	}
	c.calls++
	if c.calls > 3 {
		return openrouter.ChatResponse{}, errors.New("reader exceeded three model-call evaluation bound")
	}
	stem := fmt.Sprintf("%s-%02d", c.validator.test.name, c.calls)
	c.capture.stem = stem
	productionReaderWrite(c.validator.t, c.capture.directory, stem+"-composed-request.json", request)
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	started := time.Now()
	response, err := c.provider.ChatStream(ctx, request, handlers)
	productionReaderWrite(c.validator.t, c.capture.directory, stem+"-timing.json", map[string]any{"elapsed_ns": time.Since(started).Nanoseconds(), "usage": response.Usage, "error": fmt.Sprint(err)})
	if err != nil {
		return response, err
	}
	productionReaderWrite(c.validator.t, c.capture.directory, stem+"-normalized-response.json", response)
	if len(response.Choices) != 1 {
		return openrouter.ChatResponse{}, errors.New("unexpected reader choice count")
	}
	for _, call := range response.Choices[0].Message.ToolCalls {
		if call.Function.Name != "memory_search" && call.Function.Name != "memory_search_conversations" {
			return openrouter.ChatResponse{}, errors.New("reader requested unavailable capability")
		}
	}
	c.answers = append(c.answers, response.Choices[0].Message.Content)
	return response, nil
}

func TestMemoryStage5ProductionReaderEvaluation(t *testing.T) {
	if os.Getenv("EVIE_RUN_PRODUCTION_READER_EVAL") != "1" {
		t.Skip("actual production reader evaluation is opt-in")
	}
	directory := os.Getenv("EVIE_MEMORY_READER_ARTIFACTS")
	freezeRaw, err := os.ReadFile(filepath.Join(directory, "freeze.json"))
	if err != nil {
		t.Fatal(err)
	}
	var freeze struct {
		AdapterSHA256 string                               `json:"adapter_sha256"`
		TestSHA256    string                               `json:"test_sha256"`
		Profile       openrouter.ContextProfileDiagnostics `json:"profile"`
	}
	if err = json.Unmarshal(freezeRaw, &freeze); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"retrieval_production_reader_evaluation_test.go": freeze.AdapterSHA256, "retrieval_reader_evaluation_test.go": freeze.TestSHA256} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(raw)
		if hex.EncodeToString(hash[:]) != want {
			t.Fatal("frozen reader fixture source changed")
		}
	}
	provider, profile, capture := productionReaderSetup(t, "evaluation")
	if profile.Diagnostics() != freeze.Profile {
		t.Fatal("production model/context profile changed since freeze")
	}
	for _, name := range []string{"historical_retired", "saved_boston_newer_chicago", "tentative_quote_and_inference"} {
		t.Run(name, func(t *testing.T) {
			f := newRetrievalFixture(t)
			test := readerCase(t, f, name)
			f.refresh()
			reader := f.global()
			before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			validator := &localReaderClient{t: t, test: test}
			client := &productionReaderClient{validator: validator, provider: provider, capture: capture}
			var definitions []tools.Tool
			for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
				if capability.Tool.Schema.Function.Name == "memory_search" || capability.Tool.Schema.Function.Name == "memory_search_conversations" {
					definitions = append(definitions, capability.Tool)
				}
			}
			holder := memory.LeaseHolderID("production-reader-" + string(reader.ID))
			session := NewWithToolset(client, profile, f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions))
			started := time.Now()
			err = session.Send(context.Background(), test.question, &recorder{}, nil)
			productionReaderWrite(t, directory, name+"-case.json", map[string]any{"name": name, "question": test.question, "expected_source_ids": test.sources, "scripted_initial_searches": test.searches, "whole_turn_elapsed_ns": time.Since(started).Nanoseconds(), "model_calls": client.calls, "answers": client.answers, "error": fmt.Sprint(err)})
			productionReaderWrite(t, directory, name+"-all-composed-requests.json", validator.requests)
			if err != nil {
				t.Fatal(err)
			}
			after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
			if err != nil || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) || len(before.Claims) != len(after.Claims) {
				t.Fatalf("reader changed accepted memory: %v", err)
			}
			if len(client.answers) == 0 {
				t.Fatal("reader produced no answer")
			}
			checks := readerAnswerChecks(name, client.answers[len(client.answers)-1], test.sources)
			productionReaderWrite(t, directory, name+"-automated-checks.json", checks)
			for check, passed := range checks {
				if !passed {
					t.Errorf("predeclared reader check failed: %s; raw answer retained", check)
				}
			}
		})
	}
}
