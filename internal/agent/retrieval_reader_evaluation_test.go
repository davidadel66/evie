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
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

const localReaderModel = "qwen2.5:7b-instruct-q4_K_M"
const localReaderManifest = "845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e"

type readerEvaluationCase struct {
	name, question string
	searches       []openrouter.ToolCall
	sources        []memory.EventID
	check          func(*testing.T, []memory.RetrievalEvidence)
}

func readerRemember(t *testing.T, f *retrievalFixture, source memory.Session, predicate, text, value string) memory.RememberLiteralProposal {
	t.Helper()
	a := f.session(source, nil)
	p, err := a.PrepareRememberLiteral(context.Background(), f.store, text, memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: predicate, PredicateLabel: strings.ReplaceAll(predicate, "_", " "), PredicateCardinality: memory.CardinalityOne,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: memory.PolarityAffirmed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ResolveRememberLiteral(context.Background(), f.store, p, tools.Approved); err != nil {
		t.Fatal(err)
	}
	return p
}

func readerCase(t *testing.T, f *retrievalFixture, name string) readerEvaluationCase {
	t.Helper()
	ctx := context.Background()
	source := f.global()
	switch name {
	case "historical_retired":
		p := readerRemember(t, f, source, "preferred_lunch", "Remember that my preferred lunch is a chickpea sandwich.", "chickpea sandwich")
		inspection, err := f.store.InspectClaims(ctx, source.ScopeContext(), memory.ClaimQuery{})
		if err != nil {
			t.Fatal(err)
		}
		var cutoff time.Time
		for _, c := range inspection.Claims {
			if c.ID == p.ClaimID {
				cutoff = c.TransactionTime
			}
		}
		if cutoff.IsZero() {
			t.Fatal("missing accepted transaction time")
		}
		f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, p.ClaimID)
		args, _ := json.Marshal(map[string]string{"query": "chickpea", "intent": "historical", "as_known_at": cutoff.Format(time.RFC3339Nano)})
		return readerEvaluationCase{name: name, question: "What lunch preference had I recorded at the time of that old record, and should it still be treated as my current preference? Include the original source event ID.", sources: []memory.EventID{p.Source.EventID},
			searches: []openrouter.ToolCall{toolCall("old-claim", "memory_search", string(args)), toolCall("old-words", "memory_search_conversations", string(args))},
			check: func(t *testing.T, evidence []memory.RetrievalEvidence) {
				claim, excerpt := false, false
				for _, e := range evidence {
					if !e.AsKnownAt.Equal(cutoff) || e.Intent != memory.RetrievalHistorical || e.CurrentStatus != memory.SemanticStatusRetired {
						t.Fatalf("historical pin/status lost: %+v", e)
					}
					if e.ClaimID == p.ClaimID {
						claim = e.Status == memory.SemanticStatusActive
					}
					if e.Kind == memory.RetrievalConversationExcerpt {
						excerpt = true
					}
				}
				if !claim || !excerpt {
					t.Fatal("historical reader requires old active Claim, current retirement, and original excerpt")
				}
			}}
	case "saved_boston_newer_chicago":
		p := readerRemember(t, f, source, "home_city", "Remember that I live in Boston.", "Boston")
		newer := f.converse(source, "An update: I now live in Chicago after moving from Boston.")
		return readerEvaluationCase{name: name, question: "Where do I live, and is there anything inconsistent in your saved memory? Attribute the saved record and the later statement, including both source event IDs.", sources: []memory.EventID{p.Source.EventID, newer.ID},
			searches: []openrouter.ToolCall{toolCall("saved-city", "memory_search", `{"query":"Boston Chicago","intent":"current"}`), toolCall("later-city", "memory_search_conversations", `{"query":"Chicago","intent":"current"}`)},
			check: func(t *testing.T, evidence []memory.RetrievalEvidence) {
				claim, newStatement := false, false
				for _, e := range evidence {
					if e.ClaimID == p.ClaimID {
						claim = e.CurrentStatus == memory.SemanticStatusActive && strings.Contains(e.Text, "Boston")
					}
					for _, s := range e.Sources {
						if s.EventID == newer.ID {
							newStatement = e.Kind == memory.RetrievalConversationExcerpt && s.Authority == memory.AuthorityOwnerStatement && strings.Contains(e.Text, "Chicago")
						}
					}
				}
				if !claim || !newStatement {
					t.Fatal("reader requires both the unchanged Boston Claim and attributed newer Chicago statement")
				}
			}}
	case "tentative_quote_and_inference":
		client := &fakeClient{steps: []step{assistantStep("I infer that the Kyoto trip could be confirmed.", nil)}}
		if err := f.session(source, client).Send(ctx, "I might visit Kyoto next spring. Nothing is booked. My colleague said, \"I'll definitely travel to Kyoto\", but that was her plan, not mine.", &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
		events, err := f.store.LoadEvents(ctx, source.ID)
		if err != nil {
			t.Fatal(err)
		}
		var ids []memory.EventID
		for _, event := range events {
			if event.Type == memory.EventUserMessage || event.Type == memory.EventAssistantMessage {
				ids = append(ids, event.ID)
			}
		}
		return readerEvaluationCase{name: name, question: "Is my Kyoto trip confirmed, and whose definite plan was mentioned? Distinguish the original statement from any assistant inference. Include source event IDs.", sources: ids,
			searches: []openrouter.ToolCall{toolCall("trip-statements", "memory_search_conversations", `{"query":"Kyoto","intent":"current"}`)},
			check: func(t *testing.T, evidence []memory.RetrievalEvidence) {
				owner, inference := false, false
				for _, e := range evidence {
					if e.Kind != memory.RetrievalConversationExcerpt || e.ClaimID != "" {
						t.Fatal("tentative evidence became accepted memory")
					}
					for _, s := range e.Sources {
						if s.Actor == memory.SemanticActorOwner {
							owner = s.Authority == memory.AuthorityOwnerStatement && strings.Contains(e.Text, "Nothing is booked") && strings.Contains(e.Text, "her plan, not mine")
						}
						if s.Actor == "assistant" {
							inference = s.Authority == "none" && strings.Contains(e.Text, "infer")
						}
					}
				}
				if !owner || !inference {
					t.Fatal("reader requires tentative owner speech, quotation, and separately attributed assistant inference")
				}
			}}
	default:
		t.Fatal("unknown reader case")
		return readerEvaluationCase{}
	}
}

type localReaderClient struct {
	t                   *testing.T
	test                readerEvaluationCase
	model               bool
	endpoint, directory string
	http                *http.Client
	calls, modelCalls   int
	requests            []openrouter.ChatRequest
	answers             []string
}

func (c *localReaderClient) ChatStream(ctx context.Context, request openrouter.ChatRequest, handlers openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	c.calls++
	if c.calls == 1 {
		return assistantStep("", nil, c.test.searches...).res, nil
	}
	data := retrievalData(c.t, request)
	var supplied struct {
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
		return openrouter.ChatResponse{}, err
	}
	c.test.check(c.t, supplied.Evidence)
	found := map[memory.EventID]bool{}
	for _, e := range supplied.Evidence {
		for _, s := range e.Sources {
			found[s.EventID] = true
		}
	}
	for _, id := range c.test.sources {
		if !found[id] {
			c.t.Fatalf("required source %s absent before model call", id)
		}
	}
	if len(found) != len(c.test.sources) {
		c.t.Fatalf("unexpected source before reader: %v", found)
	}
	if !c.model {
		return assistantStep("Provider-bound evidence contract verified; no real reader ran.", nil).res, nil
	}
	c.modelCalls++
	if c.modelCalls > 3 {
		return openrouter.ChatResponse{}, errors.New("reader exceeded three model-call evaluation bound")
	}
	messages := make([]map[string]any, 0, len(request.Messages))
	for _, m := range request.Messages {
		message := map[string]any{"role": m.Role, "content": m.Content}
		if m.ToolCallID != "" {
			message["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			var calls []map[string]any
			for _, call := range m.ToolCalls {
				var arguments any
				if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
					return openrouter.ChatResponse{}, err
				}
				calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Function.Name, "arguments": arguments}})
			}
			message["tool_calls"] = calls
		}
		messages = append(messages, message)
	}
	body := map[string]any{"model": localReaderModel, "messages": messages, "tools": request.Tools, "stream": false, "keep_alive": "30s",
		"options": map[string]any{"num_ctx": 32768, "num_predict": 768, "seed": 0, "temperature": 0, "num_thread": 4}}
	encoded, err := json.Marshal(body)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	stem := fmt.Sprintf("%s-%02d", c.test.name, c.modelCalls)
	c.write(stem+"-composed-request.json", request)
	c.write(stem+"-ollama-request.json", body)
	callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.endpoint+"/api/chat", bytes.NewReader(encoded))
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	started := time.Now()
	response, err := c.http.Do(req)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	elapsed := time.Since(started)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	mustReaderWrite(c.t, filepath.Join(c.directory, stem+"-raw-response.json"), raw)
	if response.StatusCode != http.StatusOK {
		return openrouter.ChatResponse{}, fmt.Errorf("local reader HTTP status %d", response.StatusCode)
	}
	var result struct {
		Model           string `json:"model"`
		Done            bool   `json:"done"`
		DoneReason      string `json:"done_reason"`
		PromptEvalCount int64  `json:"prompt_eval_count"`
		EvalCount       int64  `json:"eval_count"`
		Message         struct {
			Role, Content string
			ToolCalls     []struct {
				Function struct {
					Name      string
					Arguments json.RawMessage
				}
			} `json:"tool_calls"`
		} `json:"message"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return openrouter.ChatResponse{}, err
	}
	if !result.Done || result.Model != localReaderModel || result.Message.Role != "assistant" || result.DoneReason == "length" {
		return openrouter.ChatResponse{}, errors.New("reader incomplete, wrong model/role, or output truncated")
	}
	var calls []openrouter.ToolCall
	for i, call := range result.Message.ToolCalls {
		if call.Function.Name != "memory_search" && call.Function.Name != "memory_search_conversations" {
			return openrouter.ChatResponse{}, errors.New("reader requested unavailable capability")
		}
		calls = append(calls, toolCall(fmt.Sprintf("reader-%d-%d", c.modelCalls, i), call.Function.Name, string(call.Function.Arguments)))
	}
	c.answers = append(c.answers, result.Message.Content)
	c.write(stem+"-timing.json", map[string]any{"elapsed_ns": elapsed.Nanoseconds(), "ollama_request_bytes": len(encoded), "response_bytes": len(raw), "prompt_tokens": result.PromptEvalCount, "output_tokens": result.EvalCount})
	if handlers.OnContent != nil && result.Message.Content != "" {
		handlers.OnContent(result.Message.Content)
	}
	usage := testProviderUsage(result.PromptEvalCount, result.EvalCount, result.PromptEvalCount+result.EvalCount)
	return assistantUsageStep(result.Message.Content, usage, calls...).res, nil
}

func mustReaderWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
func (c *localReaderClient) write(name string, value any) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		c.t.Fatal(err)
	}
	mustReaderWrite(c.t, filepath.Join(c.directory, name), encoded)
}

func localReaderHTTP(t *testing.T, endpoint string) *http.Client {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	address, err := netip.ParseAddr(parsed.Hostname())
	if err != nil || !address.IsLoopback() || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		t.Fatal("literal loopback HTTP endpoint required")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("reader redirects denied") }}
}

func TestMemoryStage5ReaderEvidenceContract(t *testing.T) { runMemoryReaderCases(t, false) }
func TestMemoryStage5LocalReaderEvaluation(t *testing.T) {
	if os.Getenv("EVIE_RUN_MEMORY_READER_EVAL") != "1" {
		t.Skip("actual local reader evaluation requires EVIE_RUN_MEMORY_READER_EVAL=1 and pinned fixture directory")
	}
	runMemoryReaderCases(t, true)
}

func runMemoryReaderCases(t *testing.T, model bool) {
	endpoint, directory := os.Getenv("EVIE_MEMORY_READER_ENDPOINT"), os.Getenv("EVIE_MEMORY_READER_ARTIFACTS")
	var client *http.Client
	if model {
		if endpoint == "" {
			endpoint = "http://127.0.0.1:11566"
		}
		if !filepath.IsAbs(directory) {
			t.Fatal("absolute reader artifact directory required")
		}
		prior, err := filepath.Glob(filepath.Join(directory, "*-raw-response.json"))
		if err != nil || len(prior) > 0 {
			t.Fatal("reader outputs already exist; preserve this run and use a new versioned directory")
		}
		client = localReaderHTTP(t, endpoint)
		versionResponse, err := client.Get(endpoint + "/api/version")
		if err != nil {
			t.Fatal(err)
		}
		var runtimeVersion struct{ Version string }
		err = json.NewDecoder(versionResponse.Body).Decode(&runtimeVersion)
		versionResponse.Body.Close()
		if err != nil || runtimeVersion.Version != "0.6.3" {
			t.Fatalf("reader runtime pin mismatch: %+v %v", runtimeVersion, err)
		}
		freeze, err := os.ReadFile(filepath.Join(directory, "freeze.json"))
		if err != nil {
			t.Fatal(err)
		}
		var pin struct {
			TestSHA256          string `json:"test_sha256"`
			ModelManifestSHA256 string `json:"model_manifest_sha256"`
		}
		if err = json.Unmarshal(freeze, &pin); err != nil {
			t.Fatal(err)
		}
		source, err := os.ReadFile("retrieval_reader_evaluation_test.go")
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(source)
		if pin.TestSHA256 != hex.EncodeToString(hash[:]) || pin.ModelManifestSHA256 != localReaderManifest {
			t.Fatal("reader source/model pin mismatch")
		}
		response, err := client.Get(endpoint + "/api/tags")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var tags struct {
			Models []struct{ Name, Digest string }
		}
		if err = json.NewDecoder(response.Body).Decode(&tags); err != nil {
			t.Fatal(err)
		}
		matched := false
		for _, tag := range tags.Models {
			if tag.Name == localReaderModel && tag.Digest == localReaderManifest {
				matched = true
			}
		}
		if !matched {
			t.Fatal("local reader manifest changed")
		}
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
			c := &localReaderClient{t: t, test: test, model: model, endpoint: endpoint, directory: directory, http: client}
			profile, err := openrouter.NewExplicitContextProfile(localReaderModel, 32768, 24576, 768)
			if err != nil {
				t.Fatal(err)
			}
			var definitions []tools.Tool
			for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
				if capability.Tool.Schema.Function.Name == "memory_search" || capability.Tool.Schema.Function.Name == "memory_search_conversations" {
					definitions = append(definitions, capability.Tool)
				}
			}
			holder := memory.LeaseHolderID("reader-" + string(reader.ID))
			session := NewWithToolset(c, profile, f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions), WithAutomaticMemoryRecall(false))
			started := time.Now()
			err = session.Send(context.Background(), test.question, &recorder{}, nil)
			elapsed := time.Since(started)
			if model {
				c.write(name+"-all-composed-requests.json", c.requests)
				c.write(name+"-case.json", map[string]any{"name": name, "question": test.question, "expected_source_ids": test.sources, "scripted_initial_searches": test.searches, "whole_turn_elapsed_ns": elapsed.Nanoseconds(), "model_calls": c.modelCalls, "answers": c.answers, "error": fmt.Sprint(err)})
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(c.requests) < 2 {
				t.Fatal("reader did not receive evidence")
			}
			after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
			if err != nil || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) || len(before.Claims) != len(after.Claims) {
				t.Fatalf("reader changed accepted memory: %v", err)
			}
			if model {
				if len(c.answers) == 0 {
					t.Fatal("reader produced no answer")
				}
				answer := c.answers[len(c.answers)-1]
				checks := readerAnswerChecks(name, answer, test.sources)
				c.write(name+"-automated-checks.json", checks)
				for check, passed := range checks {
					if !passed {
						t.Errorf("predeclared reader check failed: %s; raw answer retained", check)
					}
				}
			}
		})
	}
}

func readerAnswerChecks(name, answer string, sources []memory.EventID) map[string]bool {
	lower := strings.ToLower(answer)
	has := func(words ...string) bool {
		for _, word := range words {
			if strings.Contains(lower, word) {
				return true
			}
		}
		return false
	}
	checks := map[string]bool{"nonempty_answer": strings.TrimSpace(answer) != ""}
	cited := true
	for _, id := range sources {
		cited = cited && strings.Contains(answer, string(id))
	}
	checks["requested_source_event_ids_present"] = cited
	switch name {
	case "historical_retired":
		checks["historical_preference_named"] = has("chickpea sandwich")
		checks["old_or_retired_status_expressed"] = has("retir", "historical", "previous", "past", "used to", "at that time")
		checks["does_not_assert_current_preference"] = !has("you currently prefer", "you still prefer", "your current preference is a chickpea")
	case "saved_boston_newer_chicago":
		checks["both_places_named"] = has("boston") && has("chicago")
		checks["saved_record_attributed"] = has("memory", "saved", "record", "stored")
		checks["later_statement_attributed"] = has("update", "later", "newer", "statement", "said", "moved")
		checks["discrepancy_expressed"] = has("conflict", "inconsist", "discrep", "outdated", "differ", "stale")
	case "tentative_quote_and_inference":
		checks["destination_named"] = has("kyoto")
		checks["uncertainty_expressed"] = has("not confirmed", "isn't confirmed", "unconfirmed", "uncertain", "tentative", "might", "not booked")
		checks["quoted_plan_attributed"] = has("colleague")
		checks["assistant_inference_distinguished"] = has("infer", "assistant", "speculat")
	}
	return checks
}
