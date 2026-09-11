package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestAutomaticMemoryRecallCanNarrowModelReadsBeforeCompositionAndDispatch(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	saved := readerRemember(t, f, source, "dinner_preference", "Remember that I prefer vegetarian dinners.", "vegetarian dinners")
	f.refresh()
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	checks := 0
	definitions = append(definitions, tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "check_request", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) {
		checks++
		return "The request was checked.", nil
	}})
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		for _, schema := range request.Tools {
			switch schema.Function.Name {
			case "memory_search", "memory_search_conversations", "memory_expand_conversation":
				t.Fatalf("disabled model read was advertised before dispatch: %s", schema.Function.Name)
			}
		}
		if !strings.Contains(retrievalData(t, request), string(saved.ClaimID)) {
			t.Fatal("narrowing model reads disabled authorized automatic recall")
		}
		events, err := f.store.LoadEvents(context.Background(), reader.ID)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(events[len(events)-1].Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		body, err := openrouter.RequestBytes(request)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		if snapshot.RequestSHA256 != hex.EncodeToString(digest[:]) || snapshot.SerializedBytes != int64(len(body)) || snapshot.Memory == nil || snapshot.Memory.Investigation.SearchAttempts != 2 {
			t.Fatalf("receipt differs from the actual ablated request or an omitted read executed: %+v", snapshot)
		}
		if call == 0 {
			return assistantStep("", nil,
				toolCall("omitted-accepted", "memory_search", `{"query":"vegetarian"}`),
				toolCall("omitted-conversation", "memory_search_conversations", `{"query":"vegetarian"}`),
				toolCall("omitted-expansion", "memory_expand_conversation", `{"anchor_id":"unavailable","before":1,"after":1}`),
				toolCall("allowed-check", "check_request", `{}`))
		}
		return assistantStep("The supplied preference supports a vegetarian dinner.", nil)
	}}
	holder := memory.LeaseHolderID("ablation-reader")
	session := NewWithToolset(client, testContextProfile("test-model"), f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions), WithModelMemoryRetrieval(false))
	if err := session.Send(context.Background(), "Suggest dinner for me.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 2 || checks != 1 {
		t.Fatalf("unrelated capability was changed: provider requests=%d checks=%d", len(client.reqs), checks)
	}
	for _, id := range []string{"omitted-accepted", "omitted-conversation", "omitted-expansion"} {
		found := false
		for _, message := range client.reqs[1].Messages {
			if message.Role == "tool" && message.ToolCallID == id {
				found = strings.Contains(strings.ToLower(message.Content), "unknown")
			}
		}
		if !found {
			t.Fatalf("omitted capability %s was not rejected as unknown", id)
		}
	}
	diagnostics, err := session.InspectContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Projection.ToolSchemaCount != len(client.reqs[1].Tools) {
		t.Fatalf("inspection advertised a different capability set: %d versus %d", diagnostics.Projection.ToolSchemaCount, len(client.reqs[1].Tools))
	}
}

func TestModelMemoryRetrievalDefaultsToEnabledAndKeepsLegacyTurnResolution(t *testing.T) {
	for _, mode := range []string{"default", "explicit enabled", "legacy narrowed"} {
		t.Run(mode, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source, reader := f.global(), f.global()
			saved := f.remember(source, memory.MemoryEverywhere, "malachite folio")
			f.refresh()
			var definitions []tools.Tool
			for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
				if capability.Tool.Schema.Function.Name == "memory_search" {
					definitions = append(definitions, capability.Tool)
				}
			}
			client := &fakeClient{steps: []step{
				assistantStep("", nil, toolCall("read-folio", "memory_search", `{"query":"malachite"}`)),
				assistantStep("The folio was checked.", nil),
			}}
			holder := memory.LeaseHolderID("model-read-policy")
			options := []SessionOption{WithAutomaticMemoryRecall(false)}
			if mode == "explicit enabled" {
				options = append(options, WithModelMemoryRetrieval(true))
			}
			session := NewWithToolset(client, testContextProfile("test-model"), f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions), options...)
			var extra []tools.Tool
			if mode == "legacy narrowed" {
				session = New(client, testContextProfile("test-model"), f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder))
				WithModelMemoryRetrieval(false)(session)
				extra = definitions
			}
			if err := session.Send(context.Background(), "Check the malachite folio.", &recorder{}, nil, extra...); err != nil {
				t.Fatal(err)
			}
			advertised := false
			for _, schema := range client.reqs[0].Tools {
				advertised = advertised || schema.Function.Name == "memory_search"
			}
			if advertised == (mode == "legacy narrowed") {
				t.Fatalf("unexpected advertised read surface in %s", mode)
			}
			if !strings.Contains(retrievalData(t, client.reqs[1]), string(saved.ClaimID)) {
				t.Fatal("the granted read or automatic legacy read did not supply original evidence")
			}
			if mode == "legacy narrowed" {
				receipts := investigationReceipts(t, f, reader)
				if len(receipts) != 2 || receipts[1].Investigation.SearchAttempts != 1 {
					t.Fatalf("late-resolved omitted read executed: %+v", receipts)
				}
			}
		})
	}
}
