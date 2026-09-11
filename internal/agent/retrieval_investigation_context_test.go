package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestMemoryInvestigationBoundsEvidenceToActualRequestHeadroom(t *testing.T) {
	memoryInvestigationContext(t, "", "", true)
}

func TestMemoryInvestigationContextLimitsPreserveOriginalDiscussion(t *testing.T) {
	memoryInvestigationContext(t, "The present is for my aunt. The scanner problem was about a different recipient; keep that distinction when continuing.", "", true)
}

func TestMemoryInvestigationContextKeepsSmallerOriginalWhenFirstFindingCannotFit(t *testing.T) {
	memoryInvestigationContext(t, "", strings.Repeat("Keep each supplied context line exact. ", 100), false)
}

func memoryInvestigationContext(t *testing.T, earlier, constraint string, wantClaim bool) {
	t.Helper()
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	saved := f.remember(source, memory.MemoryEverywhere, strings.Repeat("fern ", 360))
	original := f.converse(source, "My herbarium note says "+strings.Repeat("keep the paper dry. ", 30))
	if earlier != "" {
		f.converse(reader, earlier)
	}
	f.refresh()
	priorEvents, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := openrouter.NewExplicitContextProfile(openrouter.AstraModel, 1_000_000, 24_576, 768)
	if err != nil {
		t.Fatal(err)
	}
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		switch capability.Tool.Schema.Function.Name {
		case "memory_search", "memory_search_conversations", "memory_expand_conversation":
			definitions = append(definitions, capability.Tool)
		}
	}
	question := "Look up my fern preference and herbarium note. Preserve this request while reporting supported findings and any remaining gaps." + constraint
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"fern"}`), toolCall("original", "memory_search_conversations", `{"query":"herbarium"}`)),
		assistantStep("I can report only the supplied original sources; further memory is limited.", nil),
	}}
	holder := memory.LeaseHolderID("context-investigation")
	session := NewWithToolset(client, profile, f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions), WithAutomaticMemoryRecall(false))
	before, err := f.store.InspectClaims(context.Background(), source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send(context.Background(), question, &recorder{}, nil); err != nil {
		t.Fatalf("bounded memory prevented the supported continuation: %v", err)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("provider requests=%d, want the search and supported continuation", len(client.reqs))
	}
	data := retrievalData(t, client.reqs[1])
	wantedSource := string(saved.ClaimID)
	if !wantClaim {
		wantedSource = string(original.ID)
	}
	if !strings.Contains(data, `"status":"exhausted"`) || !strings.Contains(data, wantedSource) {
		t.Fatalf("limited context did not retain an eligible original with explicit exhaustion (held=%d)", len(expansionBoundaryEvidence(t, client.reqs[1])))
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []memory.ContextSnapshotPayload
	for _, event := range events[len(priorEvents):] {
		if event.Type == memory.EventContextSnapshot {
			var snapshot memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			snapshots = append(snapshots, snapshot)
		}
	}
	if len(snapshots) != len(client.reqs) {
		t.Fatal("provider requests and immutable receipts differ")
	}
	cumulative := 0
	for i, request := range client.reqs {
		wire, err := openrouter.RequestBytes(request)
		if err != nil {
			t.Fatal(err)
		}
		// These are the declared profile's conservative byte budget and the
		// existing cumulative memory bound, independent of admission internals.
		if len(wire) > 19_712 || snapshots[i].SerializedBytes != int64(len(wire)) || snapshots[i].RequestSHA256 != memory.CompilerHash(wire) {
			t.Fatalf("request %d escaped the actual profile or its snapshot: bytes=%d snapshot=%+v", i, len(wire), snapshots[i])
		}
		questionPresent, discussionPresent := false, earlier == ""
		for _, message := range request.Messages {
			questionPresent = questionPresent || message.Role == "user" && message.Content == question
			discussionPresent = discussionPresent || message.Role == "user" && message.Content == earlier
			if strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") || message.Role == "tool" && (message.ToolCallID == "accepted" || message.ToolCallID == "original") {
				encoded, err := json.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				cumulative += len(encoded)
			}
		}
		if !questionPresent || !discussionPresent || cumulative > 36*1024 {
			t.Fatal("memory displaced the original request or escaped cumulative delivery")
		}
		if i > 0 {
			var refs []memory.RetrievalReference
			for _, evidence := range expansionBoundaryEvidence(t, request) {
				refs = append(refs, evidence.Reference())
				if evidence.Kind == memory.RetrievalConversationExcerpt && (len(evidence.Sources) != 1 || evidence.Sources[0].EventID != original.ID || evidence.Sources[0].Evidence != original.Content || strings.TrimPrefix(evidence.Sources[0].EvidenceSHA256, "sha256:") != memory.CompilerHash([]byte(original.Content))) {
					t.Fatal("context sizing altered or truncated the smaller original excerpt")
				}
				if evidence.ClaimID == saved.ClaimID && (len(evidence.Sources) != 1 || evidence.Sources[0].ID != saved.SourceLinkID || evidence.Sources[0].EventID != saved.Source.EventID || evidence.Sources[0].Evidence != saved.Source.Evidence || evidence.Sources[0].EvidenceSHA256 != saved.Source.EvidenceSHA256) {
					t.Fatal("context sizing altered or truncated the accepted original source")
				}
			}
			receipt := snapshots[i].Memory
			if receipt == nil || receipt.Status != memory.RetrievalExhausted || !reflect.DeepEqual(refs, receipt.Evidence) || receipt.Investigation == nil || receipt.Investigation.CumulativeMemoryBytes != cumulative || receipt.Investigation.SearchAttempts != 2 {
				t.Fatalf("request %d lacks exact limited evidence/accounting receipt: %+v", i, receipt)
			}
		}
	}
	after, err := f.store.InspectClaims(context.Background(), source.ScopeContext(), memory.ClaimQuery{})
	if err != nil || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
		t.Fatalf("context pressure mutated accepted memory: %v", err)
	}
}
