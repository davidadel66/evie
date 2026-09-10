package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func TestResponsesContinuationIsAccountedButNeverPersisted(t *testing.T) {
	t.Setenv("EVIE_REASONING", "on")
	first := assistantStep("Checking.", nil, toolCall("call-1", "echo", `{"value":"one"}`))
	first.res.Choices[0].Message.TextParts = []openrouter.TextPart{{Text: "Checking.", Phase: "commentary"}}
	first.res.Choices[0].Message.ResponseItems = []json.RawMessage{
		json.RawMessage(`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"opaque-canary-one"}`),
		json.RawMessage(`{"type":"message","id":"msg_1","role":"assistant","status":"completed","phase":"commentary","content":[{"type":"output_text","text":"Checking."}]}`),
		json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call-1","name":"echo","arguments":"{\"value\":\"one\"}","status":"completed"}`),
	}
	second := assistantStep("", nil, toolCall("call-2", "echo", `{"value":"two"}`))
	second.res.Choices[0].Message.ResponseItems = []json.RawMessage{
		json.RawMessage(`{"type":"reasoning","id":"rs_2","summary":[],"encrypted_content":"opaque-canary-two"}`),
		json.RawMessage(`{"type":"function_call","id":"fc_2","call_id":"call-2","name":"echo","arguments":"{\"value\":\"two\"}","status":"completed"}`),
	}
	client := &fakeClient{steps: []step{first, second, assistantStep("Done.", nil), assistantStep("Next.", nil)}}
	session := newTestSession(client, openrouter.AstraModel)
	if err := session.Send(context.Background(), "check twice", &recorder{}, nil, echoTool("echo", false, nil)); err != nil {
		t.Fatal(err)
	}
	history := session.history.(*fakeHistory)
	var snapshots []memory.ContextSnapshotPayload
	for _, event := range history.allEvents() {
		if strings.Contains(event.Content+string(event.Payload), "opaque-canary") {
			t.Fatal("opaque state persisted")
		}
		if event.Type == memory.EventContextSnapshot {
			var snapshot memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			snapshots = append(snapshots, snapshot)
		}
	}
	for i, request := range client.reqs {
		body, err := openrouter.RequestBytes(request)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		if snapshots[i].RequestSHA256 != hex.EncodeToString(digest[:]) || snapshots[i].SerializedBytes != int64(len(body)) {
			t.Fatalf("iteration %d snapshot differs from wire", i)
		}
		want := i
		if strings.Count(string(body), "opaque-canary") != want {
			t.Fatalf("iteration %d has wrong continuation count: %s", i, body)
		}
		if strings.Contains(string(body), `"messages"`) {
			t.Fatal("accounted Chat body instead of Responses")
		}
	}
	// New turns reconstruct provider-neutral history, retaining public phases.
	if err := session.Send(context.Background(), "next", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	body, err := openrouter.RequestBytes(client.reqs[3])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "opaque-canary") || !strings.Contains(string(body), `"phase":"commentary"`) {
		t.Fatalf("invalid replay: %s", body)
	}
}

func TestAstraRejectsDisabledReasoningBeforeStartingTurn(t *testing.T) {
	t.Setenv("EVIE_REASONING", "off")
	client := &fakeClient{}
	session := newTestSession(client, openrouter.AstraModel)
	err := session.Send(context.Background(), "hello", &recorder{}, nil)
	if err == nil || !strings.Contains(err.Error(), "EVIE_REASONING") {
		t.Fatalf("error=%v", err)
	}
	if len(client.reqs) != 0 || len(session.history.(*fakeHistory).allEvents()) != 0 {
		t.Fatal("invalid config started a turn")
	}
}

func TestAstraConversationRequestsPublicReasoningSummary(t *testing.T) {
	t.Setenv("EVIE_REASONING", "low")
	client := &fakeClient{steps: []step{reasoningStep("42", []string{""}, []string{"42"})}}
	session := newTestSession(client, openrouter.AstraModel)
	rec := &recorder{}
	if err := session.Send(context.Background(), "Compute the answer", rec, nil); err != nil {
		t.Fatal(err)
	}
	body, err := openrouter.RequestBytes(client.reqs[0])
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Reasoning struct {
			Summary string `json:"summary"`
		} `json:"reasoning"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request.Reasoning.Summary != "concise" {
		t.Errorf("public reasoning summary option=%q, want concise", request.Reasoning.Summary)
	}
	if got := strings.Join(rec.events, "|"); got != "reasoning:|reasoningdone|delta:42|done:42" {
		t.Fatalf("textless reasoning lifecycle=%s", got)
	}
}

func TestTextlessReasoningClosesOnFailureWithoutDiscardWarning(t *testing.T) {
	for _, failure := range []error{errors.New("provider failed"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			client := &fakeClient{steps: []step{{reasoning: []string{""}, err: failure}}}
			rec := &recorder{}
			if err := newTestSession(client, "test-model").Send(context.Background(), "go", rec, nil); err == nil {
				t.Fatal("expected provider failure")
			}
			if got := strings.Join(rec.events, "|"); got != "reasoning:|reasoningdone" {
				t.Fatalf("activity-only failure events=%s", got)
			}
		})
	}
}

func TestTextlessReasoningClosesBeforeToolExecution(t *testing.T) {
	client := &fakeClient{steps: []step{
		reasoningStep("", []string{""}, nil, toolCall("c1", "echo", "{}")),
		assistantStep("done", []string{"done"}),
	}}
	rec := &recorder{}
	if err := newTestSession(client, "test-model").Send(context.Background(), "go", rec, nil, echoTool("echo", false, nil)); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rec.events, "|"); got != "reasoning:|reasoningdone|done:|call:c1:echo:{}|result:c1:false:echo:{}|delta:done|done:done" {
		t.Fatalf("activity-only tool lifecycle=%s", got)
	}
}

func TestResponsesOversizedContinuationStopsBeforeNextDispatch(t *testing.T) {
	t.Setenv("EVIE_REASONING", "low")
	first := assistantStep("", nil, toolCall("call-1", "echo", `{}`))
	opaque, err := json.Marshal(map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{}, "encrypted_content": strings.Repeat("x", 300000)})
	if err != nil {
		t.Fatal(err)
	}
	first.res.Choices[0].Message.ResponseItems = []json.RawMessage{opaque, json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call-1","name":"echo","arguments":"{}"}`)}
	client := &fakeClient{steps: []step{first}}
	session := newTestSession(client, openrouter.AstraModel)
	err = session.Send(context.Background(), "check", &recorder{}, nil, echoTool("echo", false, nil))
	if !IsContextOverflow(err) || len(client.reqs) != 1 {
		t.Fatalf("error=%v calls=%d", err, len(client.reqs))
	}
	for _, event := range session.history.(*fakeHistory).allEvents() {
		if strings.Contains(string(event.Payload), "encrypted_content") {
			t.Fatal("overflow persisted continuation")
		}
	}
}

func TestAstraCompactionUsesLowEffortAndExactResponsesAccounting(t *testing.T) {
	events := completedCompactionTurn("one", 1, "one", "answer one")
	events = append(events, completedCompactionTurn("two", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("three", 5, "three", "answer three")...)
	plan, err := selectManualCompaction(events, testContextProfile(openrouter.AstraModel), CanonicalRequestEstimator{})
	if err != nil {
		t.Fatal(err)
	}
	r := plan.Request
	if r.Temperature != nil || r.Reasoning == nil || r.Reasoning.Effort != "low" || r.MaxTokens != 4096 || len(r.Tools) != 0 {
		t.Fatalf("compactor settings=%+v", r)
	}
	body, err := openrouter.RequestBytes(r)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["input"] == nil || wire["messages"] != nil || wire["temperature"] != nil || string(wire["store"]) != "false" {
		t.Fatalf("invalid compaction wire=%s", body)
	}
	if strings.Contains(string(wire["reasoning"]), "summary") {
		t.Fatal("compaction requested an unused display summary")
	}
	estimate, err := (CanonicalRequestEstimator{}).Estimate(r)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	if estimate.SerializedBytes != int64(len(body)) || estimate.RequestSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("compaction estimate differs from wire")
	}
}

func TestAstraCompactionPreservesDiscoveredPromptCap(t *testing.T) {
	// Discovery represents a 100,000-token prompt cap as prompt + the
	// conversation's 16,384-token output reserve. Exercise a working override
	// reaching that hard ceiling rather than the smaller default budget.
	profile, err := openrouter.NewExplicitContextProfile(openrouter.AstraModel, 116384, 116384, 16384)
	if err != nil {
		t.Fatal(err)
	}
	events := completedCompactionTurn("one", 1, strings.Repeat("x", 100000), "answer one")
	events = append(events, completedCompactionTurn("two", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("three", 5, "three", "answer three")...)
	_, err = selectManualCompaction(events, profile, CanonicalRequestEstimator{})
	if !IsContextOverflow(err) {
		t.Fatalf("prompt-limited compaction error=%v", err)
	}
}
