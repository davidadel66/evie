package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

// The external scripted provider selects tool arguments from the complete
// request it actually received. All retrieval, history and turn ownership are real.
type expansionBoundaryClient struct {
	fakeClient
	reply func(openrouter.ChatRequest, int) step
}

func (c *expansionBoundaryClient) ChatStream(ctx context.Context, request openrouter.ChatRequest, handlers openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.steps = append(c.steps, c.reply(request, len(c.reqs)))
	return c.fakeClient.ChatStream(ctx, request, handlers)
}

func expansionBoundaryEvidence(t *testing.T, request openrouter.ChatRequest) []memory.RetrievalEvidence {
	t.Helper()
	var data struct {
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, request), "EVIE_MEMORY_DATA\n")), &data); err != nil {
		t.Fatal(err)
	}
	return data.Evidence
}

func expansionBoundaryCall(t *testing.T, callID, evidenceID string, before, after int) step {
	t.Helper()
	args, err := json.Marshal(map[string]any{"evidence_id": evidenceID, "before": before, "after": after})
	if err != nil {
		t.Fatal(err)
	}
	return assistantStep("", nil, toolCall(callID, "memory_expand_conversation", string(args)))
}

func expansionBoundaryToolResult(t *testing.T, request openrouter.ChatRequest, callID string) string {
	t.Helper()
	for _, message := range request.Messages {
		if message.Role == "tool" && message.ToolCallID == callID {
			return message.Content
		}
	}
	t.Fatalf("provider request lacks expansion result %s", callID)
	return ""
}

func TestConversationExpansionTurnRejectsForgedAndOutOfScopeAnchors(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	f.converse(source, "The amberorchid discussion is the eligible search anchor.")
	unretrieved := f.converse(source, "Unretrieved titanium details must not authorize expansion.")
	project, err := f.store.RegisterProject(context.Background(), "Other conversation area", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private, err := f.store.CreateProjectSession(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	privateEvent := f.converse(private, "Private indigo conversation must stay in its project.")
	f.refresh()
	for _, test := range []struct{ name, anchor string }{
		{"missing", "excerpt:forged-event:0:20"},
		{"same area but never retrieved", fmt.Sprintf("excerpt:%s:0:%d", unretrieved.ID, len(unretrieved.Content))},
		{"other area", fmt.Sprintf("excerpt:%s:0:%d", privateEvent.ID, len(privateEvent.Content))},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
				switch call {
				case 0:
					return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"amberorchid"}`))
				case 1:
					if evidence := expansionBoundaryEvidence(t, request); len(evidence) != 1 || evidence[0].ID == test.anchor {
						t.Fatalf("fixture did not isolate one retrieved anchor: %+v", evidence)
					}
					return expansionBoundaryCall(t, "expand", test.anchor, 2, 2)
				default:
					return assistantStep("No additional evidence supplied.", nil)
				}
			}}
			if err := f.session(f.global(), client).Send(context.Background(), "Investigate the original discussion.", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			if len(client.reqs) != 3 {
				t.Fatalf("provider requests=%d, want search then expansion continuation", len(client.reqs))
			}
			result := expansionBoundaryToolResult(t, client.reqs[2], "expand")
			if !strings.Contains(result, `"status":"unavailable"`) {
				t.Fatalf("untrusted anchor must be unavailable, not successful empty: %s", result)
			}
			data := retrievalData(t, client.reqs[2])
			for _, forbidden := range []string{unretrieved.Content, privateEvent.Content, string(privateEvent.ID)} {
				if strings.Contains(data, forbidden) {
					t.Fatalf("untrusted anchor exposed unauthorized evidence %q", forbidden)
				}
			}
		})
	}
}

func expansionBoundaryFind(t *testing.T, evidence []memory.RetrievalEvidence, eventID memory.EventID) memory.RetrievalEvidence {
	t.Helper()
	for _, item := range evidence {
		if len(item.Sources) == 1 && item.Sources[0].EventID == eventID {
			return item
		}
	}
	t.Fatalf("provider did not receive event %s", eventID)
	return memory.RetrievalEvidence{}
}

func expansionBoundaryExactSpans(t *testing.T, evidence []memory.RetrievalEvidence, originals map[memory.EventID]string) {
	t.Helper()
	type span struct{ start, end int }
	seen := make(map[memory.EventID][]span)
	for _, item := range evidence {
		if item.Kind != memory.RetrievalConversationExcerpt || item.ClaimID != "" || len(item.Sources) != 1 {
			t.Fatalf("expansion acquired semantic authority or lost its source: %+v", item)
		}
		source := item.Sources[0]
		original, ok := originals[source.EventID]
		var current span
		_, err := fmt.Sscanf(source.LocatorValue, "%d:%d", &current.start, &current.end)
		if !ok || err != nil || source.LocatorKind != memory.LocatorUTF8ByteRange || source.LocatorValue != fmt.Sprintf("%d:%d", current.start, current.end) || current.start < 0 || current.start >= current.end || current.end > len(original) {
			t.Fatalf("expansion lacks a canonical original source range: %+v", source)
		}
		if !utf8.ValidString(original[:current.start]) || !utf8.ValidString(original[:current.end]) || item.Text != original[current.start:current.end] || source.Evidence != item.Text || source.EvidenceSHA256 != memory.CompilerHash([]byte(item.Text)) {
			t.Fatalf("expansion changed original UTF-8 bytes or hash: %+v", item)
		}
		for _, previous := range seen[source.EventID] {
			if current.start < previous.end && previous.start < current.end {
				t.Fatalf("same original bytes supplied twice: event=%s ranges=%+v,%+v", source.EventID, previous, current)
			}
		}
		seen[source.EventID] = append(seen[source.EventID], current)
	}
}

func TestConversationExpansionTurnSubtractsOverlappingUTF8EvidenceAndDuplicateRequests(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	anchor := f.converse(source, "amberorchid starts this discussion.")
	long := f.converse(source, "The missing subject is my mother. "+strings.Repeat("été ", 60)+strings.Repeat("petal ", 35)+"amberorchid remains a tentative plan. "+strings.Repeat("garden ", 100))
	later := f.converse(source, "The nearby conclusion still has no booking.")
	f.refresh()
	events, err := f.store.LoadEvents(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	originals := make(map[memory.EventID]string)
	for _, event := range events {
		originals[event.ID] = event.Content
	}
	var firstID, longID string
	var prior []memory.RetrievalEvidence
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"amberorchid"}`))
		case 1:
			evidence := expansionBoundaryEvidence(t, request)
			first := expansionBoundaryFind(t, evidence, anchor.ID)
			searched := expansionBoundaryFind(t, evidence, long.ID)
			if strings.Contains(searched.Text, "The missing subject") {
				t.Fatal("fixture search already supplied the subject that expansion must add")
			}
			firstID, longID = first.ID, searched.ID
			return expansionBoundaryCall(t, "expand-first", firstID, 0, 2)
		case 2:
			prior = expansionBoundaryEvidence(t, request)
			expansionBoundaryExactSpans(t, prior, originals)
			if !strings.Contains(retrievalData(t, request), "The missing subject is my mother") {
				t.Fatal("expansion discarded independently new bytes beside an overlapping search range")
			}
			return expansionBoundaryCall(t, "expand-duplicate", firstID, 0, 2)
		case 3:
			evidence := expansionBoundaryEvidence(t, request)
			expansionBoundaryExactSpans(t, evidence, originals)
			if !reflect.DeepEqual(evidence, prior) {
				t.Fatalf("repeating one expansion changed supplied evidence: before=%+v after=%+v", prior, evidence)
			}
			if result := expansionBoundaryToolResult(t, request, "expand-duplicate"); !strings.Contains(result, `"matches":0`) {
				t.Fatalf("duplicate expansion reported old bytes as new additions: %s", result)
			}
			return expansionBoundaryCall(t, "expand-overlap", longID, 2, 2)
		default:
			evidence := expansionBoundaryEvidence(t, request)
			expansionBoundaryExactSpans(t, evidence, originals)
			expansionBoundaryFind(t, evidence, later.ID)
			return assistantStep("Additional original context received.", nil)
		}
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Read bounded context around the discussion.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 5 {
		t.Fatalf("requests=%d, want search and three bounded expansions", len(client.reqs))
	}
}

func TestConversationExpansionTurnExcludesCurrentRootFromEarlierCurrentSessionAnchor(t *testing.T) {
	f := newRetrievalFixture(t)
	reader := f.global()
	anchor := f.converse(reader, "cedaranchor identifies an earlier root of this conversation.")
	f.refresh()
	currentMessage := "The current root has CURRENTROOT_ONLY details; inspect the earlier discussion."
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"cedaranchor"}`))
		case 1:
			found := expansionBoundaryFind(t, expansionBoundaryEvidence(t, request), anchor.ID)
			return expansionBoundaryCall(t, "expand", found.ID, 0, 2)
		default:
			return assistantStep("Earlier context received.", nil)
		}
	}}
	if err := f.session(reader, client).Send(context.Background(), currentMessage, &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 3 {
		t.Fatalf("requests=%d, want search and expansion", len(client.reqs))
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var cutoff int64
	sequences := make(map[memory.EventID]int64)
	for _, event := range events {
		sequences[event.ID] = event.Sequence
		if event.Type == memory.EventUserMessage && event.Content == currentMessage {
			cutoff = event.Sequence
		}
	}
	if cutoff == 0 {
		t.Fatal("current root was not persisted")
	}
	evidence := expansionBoundaryEvidence(t, client.reqs[2])
	if len(evidence) != 2 {
		t.Fatalf("want earlier owner and assistant passages, got %+v", evidence)
	}
	for _, item := range evidence {
		for _, source := range item.Sources {
			if source.SessionID != reader.ID || sequences[source.EventID] == 0 || sequences[source.EventID] >= cutoff {
				t.Fatalf("current-root or later content became supplemental evidence: %+v", item)
			}
		}
	}
	if strings.Contains(retrievalData(t, client.reqs[2]), "CURRENTROOT_ONLY") {
		t.Fatal("expansion copied the active root back as supplemental evidence")
	}
}

func TestConversationExpansionTurnOmitsRetiredNeighborRangeButKeepsUnrelatedUTF8Passage(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	basis := f.remember(source, memory.MemoryEverywhere, "expansion interval fixture")
	neighbor := f.converse(source, "Her café preference is sapphire; unrelated kōwhai context remains tentative.")
	anchor := f.converse(source, "lilacanchor is the next message in this discussion.")
	claims := f.acceptRangeClaims(source, neighbor, basis, [][2]string{{"sapphire", "Her café preference is sapphire"}})
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[0])
	f.refresh()
	before, err := f.store.InspectClaims(context.Background(), source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	events, err := f.store.LoadEvents(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	originals := make(map[memory.EventID]string)
	for _, event := range events {
		originals[event.ID] = event.Content
	}
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"lilacanchor"}`))
		case 1:
			found := expansionBoundaryFind(t, expansionBoundaryEvidence(t, request), anchor.ID)
			return expansionBoundaryCall(t, "expand", found.ID, 2, 0)
		default:
			return assistantStep("Eligible neighboring context received.", nil)
		}
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Inspect the earlier surrounding wording.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 3 {
		t.Fatalf("requests=%d, want search and expansion", len(client.reqs))
	}
	evidence := expansionBoundaryEvidence(t, client.reqs[2])
	expansionBoundaryExactSpans(t, evidence, originals)
	passage := expansionBoundaryFind(t, evidence, neighbor.ID)
	if !strings.Contains(passage.Text, "unrelated kōwhai context remains tentative") || passage.Sources[0].Actor != memory.SemanticActorOwner || passage.Sources[0].Authority != memory.AuthorityOwnerStatement {
		t.Fatalf("unrelated passage lost its original uncertainty or attribution: %+v", passage)
	}
	data := retrievalData(t, client.reqs[2])
	if strings.Contains(data, "sapphire") || strings.Contains(data, "café") || strings.Contains(data, string(claims[0])) {
		t.Fatalf("neighbor expansion resurrected retired corresponding evidence: %s", data)
	}
	after, err := f.store.InspectClaims(context.Background(), source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Claims, after.Claims) || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
		t.Fatal("conversation expansion changed accepted memory")
	}
}

func TestConversationExpansionTurnOptOutWithholdsSourceReferencesFromCompleteRequest(t *testing.T) {
	for _, model := range []string{"test-model", openrouter.AstraModel} {
		t.Run(model, func(t *testing.T) {
			runExpansionBoundaryOptOut(t, model)
		})
	}
}

func runExpansionBoundaryOptOut(t *testing.T, model string) {
	t.Helper()
	t.Setenv("EVIE_REASONING", "low")
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	neighbor := f.converse(source, "The surrounding source names my mother.")
	anchor := f.converse(source, "She might enjoy the garnetorchid exhibition.")
	f.refresh()
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		switch call {
		case 0:
			return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"garnetorchid"}`))
		case 1:
			found := expansionBoundaryFind(t, expansionBoundaryEvidence(t, request), anchor.ID)
			expand := expansionBoundaryCall(t, "expand", found.ID, 2, 0)
			expand.res.Choices[0].Message.ToolCalls = append(expand.res.Choices[0].Message.ToolCalls, toolCall("disable", "disable_remote_memory", `{}`))
			if model == openrouter.AstraModel {
				expand.res.Choices[0].Message.ResponseItems = []json.RawMessage{json.RawMessage(`{"type":"reasoning","id":"rs_expansion","summary":[],"encrypted_content":"expansion-opaque-canary"}`)}
				for _, call := range expand.res.Choices[0].Message.ToolCalls {
					item, err := json.Marshal(map[string]string{"type": "function_call", "id": "fc_" + call.ID, "call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments, "status": "completed"})
					if err != nil {
						t.Fatal(err)
					}
					expand.res.Choices[0].Message.ResponseItems = append(expand.res.Choices[0].Message.ResponseItems, item)
				}
			}
			return expand
		default:
			return assistantStep("Memory is unavailable.", nil)
		}
	}}
	disable := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "disable_remote_memory", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) {
		t.Setenv("EVIE_REMOTE_MEMORY", "off")
		return "disabled", nil
	}}
	definitions := []tools.Tool{disable}
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	holder := memory.LeaseHolderID("expansion-egress-" + string(reader.ID))
	session := NewWithToolset(client, testContextProfile(model), f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions))
	if err := session.Send(context.Background(), "Inspect the surrounding wording.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 3 {
		t.Fatalf("requests=%d, want search then expansion and opt-out continuation", len(client.reqs))
	}
	encoded, err := openrouter.RequestBytes(client.reqs[2])
	if err != nil {
		t.Fatal(err)
	}
	if model == openrouter.AstraModel && (strings.Contains(string(encoded), `"messages"`) || !strings.Contains(string(encoded), `"input"`)) {
		t.Fatal("native Responses fixture did not inspect the actual Responses wire body")
	}
	for _, forbidden := range []string{string(source.ID), string(anchor.ID), string(neighbor.ID), anchor.Content, neighbor.Content, "expansion-opaque-canary"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("opt-out leaked supplemental source through complete request replay: %q", forbidden)
		}
	}
	if data := retrievalData(t, client.reqs[2]); !strings.Contains(data, `"status":"unavailable"`) {
		t.Fatalf("opt-out must report unavailable memory: %s", data)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	originalCallPreserved := false
	for _, event := range events {
		if event.Type == memory.EventAssistantMessage && strings.Contains(string(event.Payload), string(anchor.ID)) {
			originalCallPreserved = true
		}
	}
	if !originalCallPreserved {
		t.Fatal("request filtering rewrote the original durable assistant tool call")
	}
}
