package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func TestConversationExpansionSharesSearchBudgetAndRejectsLargeWindows(t *testing.T) {
	for _, test := range []struct {
		name          string
		calls, window int
		status        string
	}{{"mixed call budget", 8, 0, "exhausted"}, {"invalid large window", 1, 3, "failed"}} {
		t.Run(test.name, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source := f.global()
			f.converse(source, "boundedlotus marks the tentative plan.")
			f.refresh()
			client := &expansionBoundaryClient{reply: func(req openrouter.ChatRequest, call int) step {
				if call == 0 {
					return assistantStep("", nil, toolCall("search", "memory_search_conversations", `{"query":"boundedlotus"}`))
				}
				if call == 1 {
					evidence := expansionBoundaryEvidence(t, req)
					if len(evidence) != 1 {
						t.Fatalf("missing anchor: %+v", evidence)
					}
					var calls []openrouter.ToolCall
					for i := 0; i < test.calls; i++ {
						calls = append(calls, expansionBoundaryCall(t, string(rune('a'+i)), evidence[0].ID, test.window, test.window).res.Choices[0].Message.ToolCalls...)
					}
					return assistantStep("", nil, calls...)
				}
				return assistantStep("Only the supplied statement is supported.", nil)
			}}
			if err := f.session(f.global(), client).Send(context.Background(), "Read the plan's context.", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			data := retrievalData(t, client.reqs[2])
			if !strings.Contains(data, `"status":"`+test.status+`"`) || !strings.Contains(data, "boundedlotus") {
				t.Fatalf("bounds lost supported evidence/status: %s", data)
			}
		})
	}
}

func TestConversationExpansionCancellationHasNoLateEvidenceReceipt(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	f.converse(source, "cancelorchid remains a possibility.")
	f.refresh()
	before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &expansionBoundaryClient{reply: func(req openrouter.ChatRequest, call int) step {
		if call == 0 {
			return assistantStep("", nil, toolCall("search", "memory_search_conversations", `{"query":"cancelorchid"}`))
		}
		evidence := expansionBoundaryEvidence(t, req)
		if len(evidence) != 1 {
			t.Fatalf("missing anchor: %+v", evidence)
		}
		cancel()
		return expansionBoundaryCall(t, "cancelled-expansion", evidence[0].ID, 2, 2)
	}}
	err = f.session(reader, client).Send(ctx, "Read the original context.", &recorder{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled turn=%v", err)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("late provider dispatch after cancellation: %d", len(client.reqs))
	}
	after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil || after.ScopeRevision != before.ScopeRevision {
		t.Fatalf("cancellation changed accepted memory: %+v %v", after, err)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Payload), "conversation_expansion") {
			t.Fatalf("cancelled expansion supplied a durable receipt: %s", event.ID)
		}
	}
}
