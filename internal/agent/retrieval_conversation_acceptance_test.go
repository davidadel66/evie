package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davidadel66/evie/internal/eviedb"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

func (f *retrievalFixture) converse(record memory.Session, text string) memory.Event {
	f.t.Helper()
	client := &fakeClient{steps: []step{assistantStep("Noted.", nil)}}
	if err := f.session(record, client).Send(context.Background(), text, &recorder{}, nil); err != nil {
		f.t.Fatal(err)
	}
	events, err := f.store.LoadEvents(context.Background(), record.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == memory.EventUserMessage {
			return events[i]
		}
	}
	f.t.Fatal("conversation has no source event")
	return memory.Event{}
}

func (f *retrievalFixture) searchConversations(record memory.Session, query string) *fakeClient {
	f.t.Helper()
	args, _ := json.Marshal(map[string]string{"query": query})
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("conversation-lookup", "memory_search_conversations", string(args))), assistantStep("Original statement received.", nil)}}
	if err := f.session(record, client).Send(context.Background(), "Find the original conversation statement.", &recorder{}, nil); err != nil {
		f.t.Fatal(err)
	}
	return client
}

func TestConversationSearchFindsUncompiledOriginalStatement(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	event := f.converse(source, "I grew saffron crocuses on the balcony last autumn.")
	f.refresh()
	client := f.searchConversations(reader, "saffron")
	data := retrievalData(t, client.reqs[1])
	var result struct {
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "EVIE_MEMORY_DATA\n")), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Kind != memory.RetrievalConversationExcerpt || result.Evidence[0].ClaimID != "" || len(result.Evidence[0].Sources) != 1 || result.Evidence[0].Sources[0].EventID != event.ID || result.Evidence[0].Sources[0].SessionID != source.ID || result.Evidence[0].Text != event.Content {
		t.Fatalf("uncompiled evidence lost attribution or acquired an invented Claim: %s", data)
	}
}

func TestConversationSearchKeepsUTF8SpeakerAndRestartSources(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	text := strings.Repeat("été on the terrace. ", 70) + "Maybe the kōwhai trip could happen in June; nothing is booked."
	client := &fakeClient{steps: []step{assistantStep("I infer that the kōwhai trip is tentative.", nil)}}
	if err := f.session(source, client).Send(context.Background(), text, &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	f.converse(source, "kōwhai password=verylongsecret")
	f.refresh()
	found := f.searchConversations(reader, "kōwhai")
	var supplied struct {
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, found.reqs[1]), "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
		t.Fatal(err)
	}
	if len(supplied.Evidence) != 2 {
		t.Fatalf("want owner and assistant excerpts, got %+v", supplied.Evidence)
	}
	var refs []memory.RetrievalReference
	owner, assistant := false, false
	for _, evidence := range supplied.Evidence {
		if len(evidence.Text) > 800 || !utf8.ValidString(evidence.Text) || strings.Contains(evidence.Text, "verylongsecret") {
			t.Fatalf("unsafe excerpt: %+v", evidence)
		}
		if len(evidence.Sources) != 1 {
			t.Fatalf("missing attribution: %+v", evidence)
		}
		s := evidence.Sources[0]
		if s.SessionID != source.ID || s.EvidenceSHA256 != memory.CompilerHash([]byte(evidence.Text)) {
			t.Fatalf("source mismatch: %+v", s)
		}
		switch s.Actor {
		case memory.SemanticActorOwner:
			owner = s.Authority == memory.AuthorityOwnerStatement && strings.Contains(evidence.Text, "nothing is booked") && s.LocatorValue != fmt.Sprintf("0:%d", len(evidence.Text))
		case "assistant":
			assistant = s.Authority == "none" && strings.Contains(evidence.Text, "I infer")
		}
		refs = append(refs, evidence.Reference())
	}
	if !owner || !assistant {
		t.Fatal("tentative owner wording or assistant attribution was lost")
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	inspected, err := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), refs)
	if err != nil || len(inspected) != len(refs) {
		t.Fatalf("restart inspection: %+v %v", inspected, err)
	}
	for i, item := range inspected {
		if !item.Available || item.Evidence.Text != supplied.Evidence[i].Text {
			t.Fatalf("restart changed exact excerpt: %+v", item)
		}
	}
}
