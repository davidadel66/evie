package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

type receiptHTTPFixture struct {
	t              *testing.T
	db             *sql.DB
	path           string
	store          *eviedb.Store
	source, reader memory.Session
	accepted       memory.RememberLiteralProposal
	conversation   memory.Event
}

func newReceiptHTTPFixture(t *testing.T) *receiptHTTPFixture {
	t.Helper()
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	f := &receiptHTTPFixture{t: t, path: filepath.Join(t.TempDir(), "evie.db")}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	f.store = eviedb.NewStore(f.db)
	f.source, err = f.store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.reader, err = f.store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sourceRuntime := f.runtime(f.source, nil)
	f.accepted, err = sourceRuntime.PrepareRememberLiteral(context.Background(), f.store, "Remember my retrieval marker: azurefolio", memory.RememberLiteralRequest{
		Destination: memory.MemoryEverywhere, IdempotencyKey: "idem:v1:93000000-0000-4000-8000-000000000001", Predicate: "retrieval_marker", PredicateLabel: "retrieval marker", PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "azurefolio"}, Polarity: memory.PolarityAffirmed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sourceRuntime.ResolveRememberLiteral(context.Background(), f.store, f.accepted, tools.Approved); err != nil {
		t.Fatal(err)
	}
	source := f.runtime(f.source, &fakeClient{steps: []fakeStep{{content: "Recorded."}}})
	f.send(source, "The ochreletter is a tentative café visit.")
	events, err := source.HistoryEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Content == "The ochreletter is a tentative café visit." {
			f.conversation = event
		}
	}
	coverage, err := f.store.RefreshMemoryIndex(context.Background(), 256)
	if err != nil || coverage.State != "active" {
		t.Fatalf("fixture index=%+v: %v", coverage, err)
	}
	return f
}

func (f *receiptHTTPFixture) runtime(record memory.Session, client agent.Client) *agent.Session {
	f.t.Helper()
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	holder := memory.LeaseHolderID("receipt-" + string(record.ID))
	return agent.NewWithToolset(client, webTestContextProfile("test"), f.store.BindHistory(record.ID, holder), record.ScopeContext(), f.store.BindTurnOwner(record.ID, holder), tools.NewToolset(definitions))
}

func (f *receiptHTTPFixture) send(runtime *agent.Session, text string) {
	f.t.Helper()
	events, err := newSSEEvents(httptest.NewRecorder())
	if err != nil {
		f.t.Fatal(err)
	}
	if err := runtime.Send(context.Background(), text, events, nil); err != nil {
		f.t.Fatal(err)
	}
}

func receiptHTTPTool(id, name, args string) fakeStep {
	return fakeStep{toolCalls: []openrouter.ToolCall{{ID: id, Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: args}}}}
}

func (f *receiptHTTPFixture) inspect(runtime *agent.Session, selection map[string]string) *httptest.ResponseRecorder {
	f.t.Helper()
	selection["sessionId"] = string(f.reader.ID)
	body, err := json.Marshal(selection)
	if err != nil {
		f.t.Fatal(err)
	}
	server := NewContextMemoryServer(runtime, nil, nil, nil, f.store)
	server.activeSession = f.reader
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/memory/evidence", string(body)))
	return rr
}

type receiptHTTPResponse struct {
	AnswerID   memory.EventID               `json:"answerId"`
	SnapshotID memory.EventID               `json:"snapshotId"`
	Evidence   []memory.RetrievalInspection `json:"evidence"`
	Requests   []struct {
		SnapshotID      memory.EventID               `json:"snapshotId"`
		ResponseID      memory.EventID               `json:"responseId"`
		RequestStatus   string                       `json:"requestStatus"`
		Iteration       int                          `json:"iteration"`
		RequestSHA256   string                       `json:"requestSHA256"`
		SerializedBytes int64                        `json:"serializedBytes"`
		Evidence        []memory.RetrievalInspection `json:"evidence"`
	} `json:"requests"`
}

func TestMemoryReceiptHTTPAssociatesExactRequestsWithOriginalAnswerAfterRestart(t *testing.T) {
	f := newReceiptHTTPFixture(t)
	client := &fakeClient{steps: []fakeStep{
		receiptHTTPTool("accepted", "memory_search", `{"query":"azurefolio"}`),
		receiptHTTPTool("conversation", "memory_search_conversations", `{"query":"ochreletter"}`),
		{content: "I reviewed the supplied records."},
		{content: "This is a later unrelated answer."},
	}}
	runtime := f.runtime(f.reader, client)
	f.send(runtime, "Please proceed.")
	before, err := runtime.HistoryEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	answer := before[len(before)-1]
	var snapshots []memory.Event
	var responses []memory.Event
	for _, event := range before {
		if event.Type == memory.EventContextSnapshot {
			snapshots = append(snapshots, event)
		}
		if event.Type == memory.EventAssistantMessage {
			responses = append(responses, event)
		}
	}
	if len(snapshots) != 3 || len(responses) != 3 {
		t.Fatalf("fixture request sequence=%+v", before)
	}
	f.send(runtime, "Thank you.")
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	runtime = f.runtime(f.reader, nil)
	rr := f.inspect(runtime, map[string]string{"answerId": string(answer.ID)})
	if rr.Code != http.StatusOK || rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("original answer inspection status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got receiptHTTPResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.AnswerID != answer.ID || got.SnapshotID != snapshots[2].ID || len(got.Requests) != 3 {
		t.Fatalf("answer lost its original request sequence or acquired a later turn: %s", rr.Body.String())
	}
	for i, request := range got.Requests {
		var original memory.ContextSnapshotPayload
		if err := json.Unmarshal(snapshots[i].Payload, &original); err != nil {
			t.Fatal(err)
		}
		if request.SnapshotID != snapshots[i].ID || request.ResponseID != responses[i].ID || request.RequestStatus != "completed" || request.Iteration != i+1 || request.RequestSHA256 != original.RequestSHA256 || request.SerializedBytes != original.SerializedBytes {
			t.Fatalf("request association differs from original durable record: %s", rr.Body.String())
		}
		var refs []memory.RetrievalReference
		if original.Memory != nil {
			refs = original.Memory.Evidence
		}
		if len(request.Evidence) != len(refs) {
			t.Fatalf("request %d evidence changed: %+v", i, request)
		}
		for j, item := range request.Evidence {
			if !item.Available || item.Evidence == nil || !reflect.DeepEqual(item.Reference, refs[j]) {
				t.Fatalf("answer inspection reran retrieval or lost original evidence: %+v", item)
			}
		}
	}
	if len(got.Requests[1].Evidence) != 1 || len(got.Requests[2].Evidence) != 2 || got.Requests[2].Evidence[1].Reference.Kind != memory.RetrievalConversationExcerpt || got.Requests[2].Evidence[1].Reference.ClaimID != "" {
		t.Fatalf("source kinds collapsed across original request sequence: %s", rr.Body.String())
	}
	items, err := projectHistory(before)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Kind != "memory" {
			continue
		}
		encoded, err := json.Marshal(item.Memory)
		if err != nil {
			t.Fatal(err)
		}
		var activity map[string]any
		if err := json.Unmarshal(encoded, &activity); err != nil {
			t.Fatal(err)
		}
		if activity["requestStatus"] != "completed" || activity["answerId"] != string(answer.ID) {
			t.Fatalf("replayed activity lost completed answer association: %s", encoded)
		}
	}
	// Caller-supplied answer IDs cannot select another conversation's sources.
	foreign := f.inspect(runtime, map[string]string{"answerId": string(f.conversation.ID)})
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign answer inspection status=%d body=%s", foreign.Code, foreign.Body.String())
	}
}

func TestMemoryReceiptHTTPDistinguishesPreparedAndInterruptedRequests(t *testing.T) {
	f := newReceiptHTTPFixture(t)
	client := &fakeClient{entered: make(chan struct{}), release: make(chan struct{}), steps: []fakeStep{
		receiptHTTPTool("accepted", "memory_search", `{"query":"azurefolio"}`),
		{content: "This answer must never commit after cancellation."},
	}}
	runtime := f.runtime(f.reader, client)
	defer close(client.release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := newSSEEvents(httptest.NewRecorder())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- runtime.Send(ctx, "Please proceed.", events, nil) }()
	<-client.entered
	client.release <- struct{}{}
	<-client.entered
	history, err := f.store.LoadEvents(context.Background(), f.reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := history[len(history)-1]
	if snapshot.Type != memory.EventContextSnapshot {
		t.Fatalf("no prepared request: %+v", snapshot)
	}
	prepared := f.inspect(runtime, map[string]string{"snapshotId": string(snapshot.ID)})
	// Always release the real turn, even if the prepared response fails its assertion.
	cancel()
	client.release <- struct{}{}
	if err := <-done; err == nil {
		t.Fatal("cancelled provider response committed")
	}
	if prepared.Code != http.StatusOK {
		t.Fatalf("prepared inspection status=%d body=%s", prepared.Code, prepared.Body.String())
	}
	var got receiptHTTPResponse
	if err := json.Unmarshal(prepared.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.AnswerID != "" || len(got.Requests) != 2 || got.Requests[1].RequestStatus != "prepared" || got.Requests[1].ResponseID != "" {
		t.Fatalf("pending request was mislabeled as supplied, completed, or interrupted: %s", prepared.Body.String())
	}
	interrupted := f.inspect(runtime, map[string]string{"snapshotId": string(snapshot.ID)})
	got = receiptHTTPResponse{}
	if err := json.Unmarshal(interrupted.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if interrupted.Code != http.StatusOK || got.AnswerID != "" || len(got.Requests) != 2 || got.Requests[0].RequestStatus != "completed" || got.Requests[1].RequestStatus != "interrupted" || got.Requests[1].ResponseID != "" {
		t.Fatalf("durable interruption lost or assigned a fabricated answer: %s", interrupted.Body.String())
	}
}

func (f *receiptHTTPFixture) approve(name, text string, args map[string]any) {
	f.t.Helper()
	args["idempotency_key"] = "idem:v1:" + uuid.NewString()
	encoded, err := json.Marshal(args)
	if err != nil {
		f.t.Fatal(err)
	}
	client := &fakeClient{steps: []fakeStep{receiptHTTPTool("change", name, string(encoded)), {content: "Change recorded."}}}
	events, err := newSSEEvents(httptest.NewRecorder())
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.runtime(f.source, client).Send(context.Background(), text, events, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		f.t.Fatal(err)
	}
	if name == "memory_retract_source" {
		inspected, err := f.store.InspectSemanticObject(context.Background(), f.source.ScopeContext(), memory.SemanticObjectSourceLink, memory.SemanticID(fmt.Sprint(args["source_link_id"])))
		if err != nil || inspected.Status != memory.SemanticStatusSourceRetracted {
			f.t.Fatalf("source retraction was not applied: status=%s error=%v", inspected.Status, err)
		}
	}
	if name == "memory_restore_source" {
		var result string
		for _, message := range client.reqs[len(client.reqs)-1].Messages {
			if message.Role == "tool" && message.ToolCallID == "change" {
				result = message.Content
			}
		}
		if !strings.Contains(result, "Source Link Claim is not active") {
			f.t.Fatalf("source restoration did not enforce the inactive Claim guard: %s", result)
		}
	}
}

func TestMemoryReceiptHTTPKeepsOriginalVersionsThroughCorrectionRetirementRestrictionAndRestart(t *testing.T) {
	f := newReceiptHTTPFixture(t)
	source := f.runtime(f.source, nil)
	second, err := source.PrepareRememberLiteral(context.Background(), f.store, "Remember my marker: embermanifest", memory.RememberLiteralRequest{
		Destination: memory.MemoryEverywhere, IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "retrieval_marker", PredicateLabel: "retrieval marker", PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "embermanifest"}, Polarity: memory.PolarityAffirmed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ResolveRememberLiteral(context.Background(), f.store, second, tools.Approved); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.RefreshMemoryIndex(context.Background(), 256); err != nil {
		t.Fatal(err)
	}
	first := receiptHTTPTool("first", "memory_search", `{"query":"azurefolio"}`)
	first.toolCalls = append(first.toolCalls, receiptHTTPTool("second", "memory_search", `{"query":"embermanifest"}`).toolCalls...)
	first.toolCalls = append(first.toolCalls, receiptHTTPTool("conversation", "memory_search_conversations", `{"query":"ochreletter"}`).toolCalls...)
	runtime := f.runtime(f.reader, &fakeClient{steps: []fakeStep{first, {content: "The original records were checked."}}})
	f.send(runtime, "Please proceed.")
	events, err := runtime.HistoryEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	answer := events[len(events)-1]
	var original memory.ContextSnapshotPayload
	for _, event := range events {
		if event.Type == memory.EventContextSnapshot {
			if err := json.Unmarshal(event.Payload, &original); err != nil {
				t.Fatal(err)
			}
		}
	}
	if original.Memory == nil || len(original.Memory.Evidence) != 3 {
		t.Fatalf("fixture original evidence=%+v", original.Memory)
	}
	f.approve("memory_correct_claim", "Correct the earlier azurefolio marker to jadefolio; the old entry was an error.", map[string]any{
		"claim_id": f.accepted.ClaimID, "subject_entity_id": f.accepted.Subject.ID, "predicate_id": f.accepted.Predicate.ID, "literal_kind": "text", "literal_value": "jadefolio", "polarity": "affirmed", "mode": "error",
	})
	f.approve("memory_retire", "Retire the embermanifest entry.", map[string]any{"object_kind": "claim", "object_id": second.ClaimID})
	for _, reopened := range []bool{false, true} {
		if reopened {
			if err := f.db.Close(); err != nil {
				t.Fatal(err)
			}
			f.db, err = eviedb.OpenDBAt(f.path)
			if err != nil {
				t.Fatal(err)
			}
			f.store = eviedb.NewStore(f.db)
			runtime = f.runtime(f.reader, nil)
		}
		rr := f.inspect(runtime, map[string]string{"answerId": string(answer.ID)})
		if rr.Code != http.StatusOK {
			t.Fatalf("reopen=%v status=%d body=%s", reopened, rr.Code, rr.Body.String())
		}
		var got receiptHTTPResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Evidence) != 3 {
			t.Fatalf("original evidence disappeared after lifecycle change: %s", rr.Body.String())
		}
		for i, item := range got.Evidence {
			if !item.Available || item.Evidence == nil || !reflect.DeepEqual(item.Reference, original.Memory.Evidence[i]) {
				t.Fatalf("original receipt replaced or unavailable: %+v", item)
			}
			switch item.Reference.ClaimID {
			case f.accepted.ClaimID:
				if item.Reference.Status != memory.SemanticStatusActive || item.Reference.CurrentStatus != memory.SemanticStatusActive || item.CurrentStatus != memory.SemanticStatusSuperseded || item.Evidence.CurrentCorrectionMode != memory.CorrectionError || item.Evidence.CorrectionMode != "" || !strings.Contains(item.Evidence.Text, "azurefolio") || strings.Contains(item.Evidence.Text, "jadefolio") || item.Evidence.ClaimOperationID != item.Reference.ClaimOperationID {
					t.Fatalf("later correction replaced original Claim version or obscured its current state: %+v", item)
				}
			case second.ClaimID:
				if item.Reference.Status != memory.SemanticStatusActive || item.CurrentStatus != memory.SemanticStatusRetired || !strings.Contains(item.Evidence.Text, "embermanifest") {
					t.Fatalf("retirement erased the original supplied Claim: %+v", item)
				}
			default:
				if item.Reference.Kind != memory.RetrievalConversationExcerpt || item.Evidence.Text != f.conversation.Content || item.Reference.Sources[0].EvidenceSHA256 != memory.CompilerHash([]byte(f.conversation.Content)) || item.Evidence.Sources[0].Authority != memory.AuthorityOwnerStatement {
					t.Fatalf("conversation attribution changed: %+v", item)
				}
			}
		}
	}
	f.approve("memory_retract_source", "Withdraw access to the original azurefolio source.", map[string]any{"source_link_id": f.accepted.SourceLinkID})
	for _, reopened := range []bool{false, true} {
		if reopened {
			if err := f.db.Close(); err != nil {
				t.Fatal(err)
			}
			f.db, err = eviedb.OpenDBAt(f.path)
			if err != nil {
				t.Fatal(err)
			}
			f.store = eviedb.NewStore(f.db)
			runtime = f.runtime(f.reader, nil)
		}
		rr := f.inspect(runtime, map[string]string{"answerId": string(answer.ID)})
		if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "azurefolio") {
			t.Fatalf("reopen=%v historical inspection leaked restricted source: status=%d body=%s", reopened, rr.Code, rr.Body.String())
		}
		var got receiptHTTPResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		for _, item := range got.Evidence {
			if item.Reference.ClaimID == f.accepted.ClaimID && (item.Available || item.Evidence != nil) {
				t.Fatalf("restricted original source remains available: %+v", item)
			}
			if item.Reference.ClaimID != f.accepted.ClaimID && !item.Available {
				t.Fatalf("unrelated source was removed: %+v", item)
			}
		}
	}
	f.approve("memory_retract_source", "Withdraw the retired embermanifest source.", map[string]any{"source_link_id": second.SourceLinkID})
	for _, sourceID := range []memory.SemanticID{f.accepted.SourceLinkID, second.SourceLinkID} {
		f.approve("memory_restore_source", "Attempt to restore this old source.", map[string]any{"source_link_id": sourceID})
		state, err := f.store.InspectSemanticObject(context.Background(), f.source.ScopeContext(), memory.SemanticObjectSourceLink, sourceID)
		if err != nil || state.Status != memory.SemanticStatusSourceRetracted {
			t.Fatalf("restoration revived source of inactive Claim: status=%s error=%v", state.Status, err)
		}
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	runtime = f.runtime(f.reader, nil)
	rr := f.inspect(runtime, map[string]string{"answerId": string(answer.ID)})
	var restricted receiptHTTPResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &restricted); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || len(restricted.Evidence) != 3 {
		t.Fatalf("restricted receipt failed to reopen: status=%d body=%s", rr.Code, rr.Body.String())
	}
	for _, item := range restricted.Evidence {
		if item.Reference.Kind == memory.RetrievalAcceptedMemory && (item.Available || item.Evidence != nil) {
			t.Fatalf("reopen restored an inactive Claim's retracted source: %+v", item)
		}
		if item.Reference.Kind == memory.RetrievalConversationExcerpt && !item.Available {
			t.Fatal("unrelated conversation source became unavailable")
		}
	}

}
