package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

var integratedConditions = []string{"no_recall", "recent_context", "tool_only", "automatic", "automatic_deeper", "oracle"}

type integratedKernelCall struct {
	Query                 memory.RetrievalQuery       `json:"query"`
	Status                string                      `json:"status"`
	ElapsedNS             int64                       `json:"elapsed_ns"`
	Error                 string                      `json:"error,omitempty"`
	Evidence              []memory.RetrievalReference `json:"evidence"`
	Oracle                bool                        `json:"oracle_exact_selector"`
	Result                json.RawMessage             `json:"result"`
	ResultSerializedBytes int                         `json:"result_reported_serialized_bytes"`
	ResultMarshaledBytes  int                         `json:"result_marshaled_bytes"`
	ResultEvidenceCount   int                         `json:"result_evidence_count"`
	ResultEncodingError   string                      `json:"result_encoding_error,omitempty"`
	Truncated             bool                        `json:"truncated"`
	Coverage              memory.RetrievalCoverage    `json:"coverage"`
	DenseCoverage         *memory.RetrievalCoverage   `json:"dense_coverage,omitempty"`
}

// This observer delegates the production history, current revalidation and
// leases unchanged. Only the declared oracle condition replaces candidate
// selection, using approved exact references rather than search-derived gold.
type integratedHistory struct {
	*eviedb.SessionHistory
	store          *eviedb.Store
	oracle         bool
	gold           []memory.RetrievalReference
	coverage       memory.RetrievalCoverage
	calls          []integratedKernelCall
	pending        []integratedToolCall
	chargedToolIDs map[string]bool
}

type integratedToolCall struct {
	call    openrouter.ToolCall
	started bool
}

func (h *integratedHistory) SearchMemory(ctx context.Context, scope memory.ScopeContext, query memory.RetrievalQuery) (result memory.RetrievalResult, err error) {
	started := time.Now()
	defer func() {
		call := integratedKernelCall{Query: query, Status: result.Status, ElapsedNS: time.Since(started).Nanoseconds(), Oracle: h.oracle}
		call.ResultSerializedBytes = result.SerializedBytes
		call.ResultEvidenceCount = len(result.Evidence)
		call.Truncated, call.Coverage = result.Truncated, result.Coverage
		if result.DenseCoverage != nil {
			coverage := *result.DenseCoverage
			call.DenseCoverage = &coverage
		}
		encoded, encodingErr := json.Marshal(result)
		if encodingErr != nil {
			call.ResultEncodingError = encodingErr.Error()
		} else {
			call.Result = encoded
			call.ResultMarshaledBytes = len(encoded)
		}
		if err != nil {
			call.Error = err.Error()
		}
		for _, e := range result.Evidence {
			call.Evidence = append(call.Evidence, e.Reference())
		}
		h.calls = append(h.calls, call)
	}()
	if !h.oracle {
		return h.SessionHistory.SearchMemory(ctx, scope, query)
	}
	if os.Getenv("EVIE_REMOTE_MEMORY") != "on" {
		return memory.RetrievalResult{Status: memory.RetrievalUnavailable}, nil
	}
	var refs []memory.RetrievalReference
	for _, ref := range h.gold {
		if ref.Kind == query.Kind {
			refs = append(refs, ref)
		}
	}
	if query.Limit <= 0 || query.Limit > 2 || query.MaxBytes <= 0 {
		return result, errors.New("oracle received a request outside its predeclared automatic budget")
	}
	if len(refs) > query.Limit {
		return result, errors.New("oracle gold does not fit requested result bound")
	}
	inspected, err := h.store.InspectMemoryEvidence(ctx, scope, refs)
	if err != nil {
		return result, err
	}
	result = memory.RetrievalResult{Status: memory.RetrievalEmpty, Coverage: h.coverage, Evidence: []memory.RetrievalEvidence{}}
	for _, item := range inspected {
		if !item.Available || item.Evidence == nil {
			return result, errors.New("independently declared oracle source is not currently inspectable")
		}
		result.Evidence = append(result.Evidence, *item.Evidence)
	}
	result.Evidence, err = h.SessionHistory.RevalidateMemoryEvidence(ctx, scope, result.Evidence)
	if err != nil {
		return result, err
	}
	if len(result.Evidence) != len(refs) {
		return result, errors.New("oracle gold failed current scope/source/lifecycle revalidation")
	}
	if len(result.Evidence) > 0 {
		result.Status = memory.RetrievalSuccess
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	result.SerializedBytes = len(encoded)
	// Account for the nonzero byte-count field too; this is conservative with
	// respect to the runtime's subsequent common result bounding.
	encoded, err = json.Marshal(result)
	if err != nil {
		return result, err
	}
	if len(encoded) > query.MaxBytes {
		return result, errors.New("oracle sufficient support does not fit requested serialized result bound")
	}
	return result, nil
}

func integratedReadDefinitions(store *eviedb.Store, history *integratedHistory) []tools.Tool {
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(store).ToolCapabilities() {
		switch capability.Tool.Schema.Function.Name {
		case "memory_search", "memory_search_conversations", "memory_expand_conversation":
			definition := capability.Tool
			execute := definition.Execute
			name := definition.Schema.Function.Name
			definition.Execute = func(ctx context.Context, raw string) (string, error) {
				// Attribute only invocations that actually enter the runtime's
				// public search callback, including admitted budget refusals.
				// Invalid arguments and unavailable grants do not become charges.
				callID := ""
				for i := range history.pending {
					pending := &history.pending[i]
					if !pending.started && pending.call.Function.Name == name && pending.call.Function.Arguments == raw {
						pending.started = true
						callID = pending.call.ID
						break
					}
				}
				if invocation, ok := tools.InvocationFromContext(ctx); ok && invocation.SearchMemory != nil {
					original := invocation.SearchMemory
					invocation.SearchMemory = func(ctx context.Context, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
						if callID == "" {
							return memory.RetrievalResult{}, errors.New("observer could not bind the actual model tool call")
						}
						history.chargedToolIDs[callID] = true
						return original(ctx, query)
					}
					ctx = tools.WithInvocationContext(ctx, invocation)
				}
				return execute(ctx, raw)
			}
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

func integratedSession(t *testing.T, f *retrievalFixture, seed integratedSeed, condition string, client Client, profile openrouter.ContextProfile) (*Session, *integratedHistory, memory.Session) {
	t.Helper()
	reader := seed.Readers["recent"]
	if condition == "no_recall" {
		reader = seed.Readers["empty"]
	}
	automatic := condition == "automatic" || condition == "automatic_deeper" || condition == "oracle"
	modelReads := condition == "tool_only" || condition == "automatic_deeper"
	holder := memory.LeaseHolderID("integrated-" + condition + "-" + string(reader.ID))
	history := &integratedHistory{SessionHistory: f.store.BindHistory(reader.ID, holder), store: f.store, oracle: condition == "oracle", coverage: seed.IndexCoverage, chargedToolIDs: map[string]bool{}}
	if history.oracle {
		for _, id := range seed.Case.Gold.SupportSets[0] {
			history.gold = append(history.gold, seed.Bindings[id].Reference)
		}
	}
	// Resolve the same grant while enabled. The measured unavailable condition
	// switches the existing egress gate afterward, before any measured turn.
	definitions := integratedReadDefinitions(f.store, history)
	if observer, ok := client.(*integratedClient); ok {
		observer.history = history
	}
	return NewWithToolset(client, profile, history, reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions), WithAutomaticMemoryRecall(automatic), WithModelMemoryRetrieval(modelReads)), history, reader
}

type integratedDispatch struct {
	Call                   int                           `json:"call"`
	Request                openrouter.ChatRequest        `json:"composed_request"`
	RequestSHA256          string                        `json:"request_sha256"`
	RequestBytes           int                           `json:"request_bytes"`
	MemoryBytes            int                           `json:"serialized_memory_message_bytes"`
	ProjectionContentBytes int                           `json:"projection_content_bytes"`
	SourceTextBytes        int                           `json:"source_text_bytes"`
	OutcomeReplayBytes     int                           `json:"serialized_outcome_replay_bytes"`
	CumulativeMemoryBytes  int                           `json:"cumulative_memory_delivery_bytes"`
	ChargedToolCallIDs     []string                      `json:"charged_tool_call_ids"`
	ElapsedToDispatchNS    int64                         `json:"elapsed_to_dispatch_ns"`
	Evidence               []memory.RetrievalEvidence    `json:"evidence"`
	Snapshot               memory.ContextSnapshotPayload `json:"snapshot"`
	BoundaryErrors         []string                      `json:"boundary_errors"`
}

type integratedClient struct {
	t                     *testing.T
	f                     *retrievalFixture
	seed                  integratedSeed
	reader                memory.Session
	condition             string
	provider              *openrouter.Client
	capture               *productionReaderCapture
	directory             string
	started               time.Time
	dispatches            []integratedDispatch
	responses             []openrouter.ChatResponse
	modelElapsedNS        []int64
	encodedRequests       []json.RawMessage
	scripted              func(integratedDispatch) openrouter.ChatResponse
	history               *integratedHistory
	cumulativeMemoryBytes int
}

func (c *integratedClient) ChatStream(ctx context.Context, request openrouter.ChatRequest, handlers openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	wire, err := openrouter.RequestBytes(request)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	d := integratedDispatch{Call: len(c.dispatches) + 1, Request: request, RequestSHA256: memory.CompilerHash(wire), RequestBytes: len(wire), ElapsedToDispatchNS: time.Since(c.started).Nanoseconds()}
	if c.history != nil {
		for id := range c.history.chargedToolIDs {
			d.ChargedToolCallIDs = append(d.ChargedToolCallIDs, id)
		}
		slices.Sort(d.ChargedToolCallIDs)
	}
	for _, message := range request.Messages {
		if message.Role == "tool" && c.history != nil && c.history.chargedToolIDs[message.ToolCallID] {
			encoded, err := json.Marshal(message)
			if err != nil {
				return openrouter.ChatResponse{}, err
			}
			d.OutcomeReplayBytes += len(encoded)
		}
		if !strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
			continue
		}
		d.ProjectionContentBytes += len(message.Content)
		encoded, err := json.Marshal(message)
		if err != nil {
			return openrouter.ChatResponse{}, err
		}
		d.MemoryBytes += len(encoded)
		var projection struct {
			Evidence []memory.RetrievalEvidence `json:"evidence"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(message.Content, "EVIE_MEMORY_DATA\n")), &projection); err != nil {
			return openrouter.ChatResponse{}, err
		}
		d.Evidence = append(d.Evidence, projection.Evidence...)
	}
	for _, evidence := range d.Evidence {
		for _, source := range evidence.Sources {
			d.SourceTextBytes += len(source.Evidence)
		}
	}
	c.cumulativeMemoryBytes += d.MemoryBytes + d.OutcomeReplayBytes
	d.CumulativeMemoryBytes = c.cumulativeMemoryBytes
	events, err := c.f.store.LoadEvents(ctx, c.reader.ID)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	found := false
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == memory.EventContextSnapshot {
			if err := json.Unmarshal(events[i].Payload, &d.Snapshot); err != nil {
				return openrouter.ChatResponse{}, err
			}
			found = true
			break
		}
	}
	if !found || d.Snapshot.RequestSHA256 != d.RequestSHA256 || d.Snapshot.SerializedBytes != int64(len(wire)) {
		d.BoundaryErrors = append(d.BoundaryErrors, "actual encoded request differs from persisted snapshot")
	}
	if d.Snapshot.Memory != nil && d.Snapshot.Memory.Investigation != nil && d.Snapshot.Memory.Investigation.CumulativeMemoryBytes != d.CumulativeMemoryBytes {
		d.BoundaryErrors = append(d.BoundaryErrors, "observed serialized memory/replayed outcomes differ from cumulative receipt accounting")
	}
	var refs []memory.RetrievalReference
	for _, e := range d.Evidence {
		refs = append(refs, e.Reference())
	}
	if d.Snapshot.Memory != nil && !reflect.DeepEqual(refs, d.Snapshot.Memory.Evidence) && !(len(refs) == 0 && len(d.Snapshot.Memory.Evidence) == 0) {
		d.BoundaryErrors = append(d.BoundaryErrors, "actual projected source references differ from persisted receipt")
	}
	for _, text := range c.seed.Case.Gold.ForbiddenText {
		if text != "" && strings.Contains(string(wire), text) {
			d.BoundaryErrors = append(d.BoundaryErrors, "forbidden source text in complete encoded request")
		}
	}
	for _, id := range c.seed.Case.Gold.ForbiddenCurrent {
		b := c.seed.Bindings[id]
		historical := b.Reference.Intent == memory.RetrievalHistorical
		if historical {
			for _, e := range d.Evidence {
				if (e.ClaimID == b.ClaimID || integratedHasEvent(e, b.Source.EventID)) && e.Intent != memory.RetrievalHistorical {
					d.BoundaryErrors = append(d.BoundaryErrors, "retired historical source supplied as current")
				}
			}
			continue
		}
		for _, value := range []string{string(b.ClaimID), string(b.OperationID), string(b.Source.ID), string(b.Source.EventID)} {
			if value != "" && strings.Contains(string(wire), value) {
				d.BoundaryErrors = append(d.BoundaryErrors, "forbidden source identifier in complete encoded request")
			}
		}
	}
	c.dispatches = append(c.dispatches, d)
	c.encodedRequests = append(c.encodedRequests, append(json.RawMessage(nil), wire...))
	if c.directory != "" {
		stem := fmt.Sprintf("%s-%s-%02d", c.seed.Case.ID, c.condition, d.Call)
		productionReaderWrite(c.t, c.directory, stem+"-dispatch.json", d)
		productionReaderRaw(c.t, c.directory, stem+"-encoded-request.json", wire)
		if c.capture != nil {
			c.capture.stem = stem
		}
	}
	if len(d.BoundaryErrors) > 0 {
		return openrouter.ChatResponse{}, errors.New("integrated provider boundary failure; retained dispatch contains details")
	}
	if d.Call > 6 {
		return openrouter.ChatResponse{}, errors.New("integrated reader exceeded frozen six-model-call limit")
	}
	if c.provider == nil {
		if c.scripted != nil {
			response := c.scripted(d)
			c.observeToolCalls(response)
			return response, nil
		}
		return assistantStep("Model-free fixture dispatch checked; no answer-quality claim.", nil).res, nil
	}
	started := time.Now()
	response, err := c.provider.ChatStream(ctx, request, handlers)
	c.observeToolCalls(response)
	c.responses = append(c.responses, response)
	c.modelElapsedNS = append(c.modelElapsedNS, time.Since(started).Nanoseconds())
	stem := fmt.Sprintf("%s-%s-%02d", c.seed.Case.ID, c.condition, d.Call)
	productionReaderWrite(c.t, c.directory, stem+"-answer.json", map[string]any{"response": response, "elapsed_ns": c.modelElapsedNS[len(c.modelElapsedNS)-1], "error": fmt.Sprint(err)})
	return response, err
}

func (c *integratedClient) observeToolCalls(response openrouter.ChatResponse) {
	if c.history == nil {
		return
	}
	for _, choice := range response.Choices {
		for _, call := range choice.Message.ToolCalls {
			c.history.pending = append(c.history.pending, integratedToolCall{call: call})
		}
	}
}

func (c *integratedClient) lastAccounting() *memory.RetrievalInvestigation {
	for i := len(c.dispatches) - 1; i >= 0; i-- {
		if receipt := c.dispatches[i].Snapshot.Memory; receipt != nil && receipt.Investigation != nil {
			return receipt.Investigation
		}
	}
	return nil
}

func integratedHasEvent(e memory.RetrievalEvidence, id memory.EventID) bool {
	for _, s := range e.Sources {
		if s.EventID == id {
			return true
		}
	}
	return false
}

func TestMemoryStage5IntegratedFixtureSmoke(t *testing.T) {
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	workload := integratedLoadWorkload(t, "")
	for _, test := range workload.Cases {
		t.Run(test.ID, func(t *testing.T) {
			directory := t.TempDir()
			seed := integratedBuildSeed(t, test, directory)
			for _, condition := range integratedConditions {
				t.Run(condition, func(t *testing.T) {
					t.Setenv("EVIE_REMOTE_MEMORY", "on")
					f := integratedCloneSeed(t, directory, seed)
					client := &integratedClient{t: t, f: f, seed: seed, condition: condition}
					if condition == "tool_only" || condition == "automatic_deeper" {
						client.scripted = func(d integratedDispatch) openrouter.ChatResponse {
							return integratedLocalResponse(condition, seed.Question, d)
						}
					}
					session, history, reader := integratedSession(t, f, seed, condition, client, testContextProfile("test-model"))
					client.reader = reader
					before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
					if err != nil {
						t.Fatal(err)
					}
					if test.MemoryMode == "unavailable" {
						t.Setenv("EVIE_REMOTE_MEMORY", "off")
					}
					client.started = time.Now()
					if err := session.Send(context.Background(), seed.Question, &recorder{}, nil); err != nil {
						t.Fatal(err)
					}
					if client.scripted == nil && len(client.dispatches) != 1 {
						t.Fatal("model-free fixture unexpectedly performed multiple dispatches")
					}
					if condition == "oracle" {
						for _, id := range test.Gold.SupportSets[0] {
							b := seed.Bindings[id]
							found := false
							for _, e := range client.dispatches[0].Evidence {
								if b.ClaimID != "" {
									found = found || e.ClaimID == b.ClaimID
								} else {
									found = found || integratedHasEvent(e, b.Source.EventID)
								}
							}
							if !found {
								t.Errorf("oracle did not supply independently bound source %s; kernel calls=%+v", id, history.calls)
							}
						}
					}
					after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
					if err != nil || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
						t.Fatal("fixture reader mutated accepted semantic revisions")
					}
				})
			}
		})
	}
}
