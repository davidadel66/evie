package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func TestMemoryReceiptIsDurableBeforeEachProviderRequestIncludingInterruption(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		name := "completed"
		if interrupted {
			name = "interrupted"
		}
		t.Run(name, func(t *testing.T) {
			f := newRetrievalFixture(t)
			turnCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source, reader := f.global(), f.global()
			accepted := f.remember(source, memory.MemoryEverywhere, "azurefolio")
			conversation := f.converse(source, "The ochreletter is still only a tentative conversation note.")
			f.refresh()
			var requests []memory.Event
			client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
				events, err := f.store.LoadEvents(context.Background(), reader.ID)
				if err != nil {
					t.Fatal(err)
				}
				last := events[len(events)-1]
				if last.Type != memory.EventContextSnapshot {
					t.Fatalf("provider started before its durable request snapshot: %+v", last)
				}
				var snapshot memory.ContextSnapshotPayload
				if err := json.Unmarshal(last.Payload, &snapshot); err != nil {
					t.Fatal(err)
				}
				body, err := openrouter.RequestBytes(request)
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(body)
				if snapshot.Iteration != call+1 || snapshot.SerializedBytes != int64(len(body)) || snapshot.RequestSHA256 != hex.EncodeToString(digest[:]) {
					t.Fatalf("request %d differs from its already committed snapshot: %+v", call+1, snapshot)
				}
				var supplied []memory.RetrievalReference
				for _, message := range request.Messages {
					if !strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
						continue
					}
					var block struct {
						Evidence []memory.RetrievalEvidence `json:"evidence"`
					}
					if err := json.Unmarshal([]byte(strings.TrimPrefix(message.Content, "EVIE_MEMORY_DATA\n")), &block); err != nil {
						t.Fatal(err)
					}
					for _, evidence := range block.Evidence {
						supplied = append(supplied, evidence.Reference())
					}
				}
				var recorded []memory.RetrievalReference
				if snapshot.Memory != nil {
					recorded = snapshot.Memory.Evidence
				}
				if len(recorded) != len(supplied) || len(supplied) > 0 && !reflect.DeepEqual(recorded, supplied) {
					t.Fatalf("request %d receipt differs from actual supplied evidence: recorded=%+v supplied=%+v", call+1, recorded, supplied)
				}
				if strings.Contains(string(last.Payload), "azurefolio") || strings.Contains(string(last.Payload), conversation.Content) {
					t.Fatal("receipt diagnostics copied source content")
				}
				requests = append(requests, last)
				switch call {
				case 0:
					return assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"azurefolio"}`))
				case 1:
					if len(recorded) != 1 || recorded[0].ClaimID != accepted.ClaimID {
						t.Fatalf("first retrieval was not an exact accepted Claim: %+v", recorded)
					}
					return assistantStep("", nil, toolCall("conversation", "memory_search_conversations", `{"query":"ochreletter"}`))
				default:
					if len(recorded) != 2 || recorded[1].Kind != memory.RetrievalConversationExcerpt || recorded[1].ClaimID != "" || recorded[1].Sources[0].EventID != conversation.ID {
						t.Fatalf("later request lost its independently attributed conversation evidence: %+v", recorded)
					}
					if interrupted {
						cancel()
						return step{err: context.Canceled}
					}
					return assistantStep("The supplied records were reviewed.", nil)
				}
			}}
			err := f.session(reader, client).Send(turnCtx, "Please proceed.", &recorder{}, nil)
			if (err != nil) != interrupted || len(requests) != 3 {
				t.Fatalf("turn interruption=%v requests=%d error=%v", interrupted, len(requests), err)
			}
			before, err := f.store.LoadEvents(context.Background(), reader.ID)
			if err != nil {
				t.Fatal(err)
			}
			if interrupted && before[len(before)-1].Type != memory.EventTurnInterrupted {
				t.Fatalf("interrupted request has no durable terminal association: %+v", before[len(before)-1])
			}
			if err := f.db.Close(); err != nil {
				t.Fatal(err)
			}
			f.db, err = eviedb.OpenDBAt(f.path)
			if err != nil {
				t.Fatal(err)
			}
			f.store = eviedb.NewStore(f.db)
			after, err := f.store.LoadEvents(context.Background(), reader.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("request sequence or interruption changed on reopen: %v", err)
			}
		})
	}
}

func TestMemoryReceiptInspectionRejectsChangedOriginalProvenance(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	f.remember(source, memory.MemoryEverywhere, "tourmalinearchive")
	f.converse(source, "The tourmalineconversation is an uncompiled observation.")
	f.refresh()
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"tourmalinearchive"}`), toolCall("conversation", "memory_search_conversations", `{"query":"tourmalineconversation"}`)),
		assistantStep("Original evidence received.", nil),
	}}
	if err := f.session(reader, client).Send(context.Background(), "Please proceed.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	evidence := expansionBoundaryEvidence(t, client.reqs[1])
	if len(evidence) != 2 {
		t.Fatalf("fixture evidence=%+v", evidence)
	}
	for _, original := range evidence {
		for _, change := range []struct {
			name  string
			apply func(*memory.RetrievalReference)
		}{
			{"hash", func(ref *memory.RetrievalReference) { ref.Sources[0].EvidenceSHA256 = strings.Repeat("0", 64) }},
			{"locator", func(ref *memory.RetrievalReference) { ref.Sources[0].LocatorValue = "1:2" }},
			{"event", func(ref *memory.RetrievalReference) { ref.Sources[0].EventID = "forged-event" }},
			{"authority", func(ref *memory.RetrievalReference) { ref.Sources[0].Authority = memory.AuthorityToolObservation }},
			{"observed time", func(ref *memory.RetrievalReference) { ref.Sources[0].ObservedAt = "2001-01-01T00:00:00Z" }},
		} {
			t.Run(original.Kind+"/"+change.name, func(t *testing.T) {
				ref := original.Reference()
				change.apply(&ref)
				items, err := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), []memory.RetrievalReference{ref})
				if err != nil {
					t.Fatal(err)
				}
				if len(items) != 1 || items[0].Available || items[0].Evidence != nil {
					t.Fatalf("altered original provenance returned source text: %+v", items)
				}
				valid, err := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), []memory.RetrievalReference{original.Reference()})
				if err != nil || len(valid) != 1 || !valid[0].Available || valid[0].Evidence == nil || valid[0].Evidence.Text != original.Text {
					t.Fatalf("failed inspection modified the genuine receipt or source: %+v %v", valid, err)
				}
			})
		}
	}
}
