package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

func TestConversationExpansionReportsOmittedSecretNeighbor(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	f.converse(source, "The earlier message contains password=verylongsecret.")
	anchor := f.converse(source, "The silvermaple statement is the eligible anchor.")
	f.refresh()
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"silvermaple"}`))
		case 1:
			found := expansionBoundaryFind(t, expansionBoundaryEvidence(t, request), anchor.ID)
			return expansionBoundaryCall(t, "expand", found.ID, 2, 0)
		default:
			return assistantStep("Only eligible context received.", nil)
		}
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Inspect the earlier discussion.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	data := retrievalData(t, client.reqs[2])
	if strings.Contains(data, "verylongsecret") || !strings.Contains(data, "Noted.") {
		t.Fatalf("expanded source policy lost the eligible neighbor or exposed a secret: %s", data)
	}
	if result := expansionBoundaryToolResult(t, client.reqs[2], "expand"); !strings.Contains(result, `"truncated":true`) {
		t.Fatalf("omitted neighboring source was reported as a complete window: %s", result)
	}
}

func TestConversationExpansionReportsBoundedSequenceGap(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	var calls []openrouter.ToolCall
	for i := 0; i < 40; i++ {
		calls = append(calls, toolCall(fmt.Sprintf("gap-%d", i), "gap_marker", `{}`))
	}
	marker := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "gap_marker", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) {
		return "durable tool event", nil
	}}
	sourceClient := &fakeClient{steps: []step{assistantStep("", nil, calls...), assistantStep("Distant public statement beyond the event window.", nil)}}
	if err := f.session(source, sourceClient, marker).Send(context.Background(), "The copperwillow discussion is the anchor.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	events, err := f.store.LoadEvents(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	var anchor, distant memory.Event
	for _, event := range events {
		if event.Type == memory.EventUserMessage {
			anchor = event
		}
		if event.Type == memory.EventAssistantMessage && event.Content != "" {
			distant = event
		}
	}
	if distant.Sequence-anchor.Sequence <= 64 {
		t.Fatalf("fixture requires a public neighbor beyond 64 durable events: anchor=%d later=%d", anchor.Sequence, distant.Sequence)
	}
	f.refresh()
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"copperwillow"}`))
		case 1:
			found := expansionBoundaryFind(t, expansionBoundaryEvidence(t, request), anchor.ID)
			return expansionBoundaryCall(t, "expand", found.ID, 0, 1)
		default:
			return assistantStep("The bounded window contains no additional public statement.", nil)
		}
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Inspect neighboring statements.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if data := retrievalData(t, client.reqs[2]); strings.Contains(data, "Distant public statement") {
		t.Fatalf("expansion exceeded its durable-event window: %s", data)
	}
	if result := expansionBoundaryToolResult(t, client.reqs[2], "expand"); !strings.Contains(result, `"truncated":true`) || !strings.Contains(result, `"matches":0`) {
		t.Fatalf("bounded sequence gap was reported as a complete window: %s", result)
	}
}

func TestConversationExpansionReportsRetiredNeighborInterval(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	basis := f.remember(source, memory.MemoryEverywhere, "expansion interval fixture")
	neighbor := f.converse(source, "My café marker is sapphire; the garden stays marigold.")
	claims := f.acceptRangeClaims(source, neighbor, basis, [][2]string{{"sapphire", "café marker is sapphire"}})
	anchor := f.converse(source, "The goldenlarch statement is the eligible anchor.")
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[0])
	f.refresh()
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"goldenlarch"}`))
		case 1:
			found := expansionBoundaryFind(t, expansionBoundaryEvidence(t, request), anchor.ID)
			return expansionBoundaryCall(t, "expand", found.ID, 2, 0)
		default:
			return assistantStep("Only eligible context received.", nil)
		}
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Inspect the earlier discussion.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	data := retrievalData(t, client.reqs[2])
	if strings.Contains(data, "sapphire") || strings.Contains(data, "café") || !strings.Contains(data, "marigold") {
		t.Fatalf("retired neighboring interval was exposed or independent text was omitted: %s", data)
	}
	if result := expansionBoundaryToolResult(t, client.reqs[2], "expand"); !strings.Contains(result, `"truncated":true`) {
		t.Fatalf("omitted neighboring interval was reported as a complete window: %s", result)
	}
}
