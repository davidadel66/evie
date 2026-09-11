package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestConversationRetrievalExcludesDelegatedAssignmentsAcrossReopenAndRebuild(t *testing.T) {
	for _, dense := range []bool{false, true} {
		name := "lexical"
		if dense {
			name = "dense"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var endpoint *denseFixtureEndpoint
			t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
			if dense {
				endpoint = newDenseFixtureEndpoint(t)
				t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
			}
			f := newRetrievalFixture(t)
			parent := f.global()
			owner := f.converse(parent, "ownerorchid runs before sunrise")
			resolved, err := standardManager(t, f.store).ResolvePreset(plugins.ResearchPresetID)
			if err != nil {
				t.Fatal(err)
			}
			child, err := f.store.CreateDelegatedSessionWithComposition(ctx, parent.ID, resolved.Receipt)
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeClient{steps: []step{assistantStep("childonly findings: runs before sunrise", nil)}}
			worker := NewDelegatedWithToolset(client, testContextProfile("test-model"), f.store.BindHistory(child.ID, "worker"), child.ScopeContext(), f.store.BindTurnOwner(child.ID, "worker"), resolved.Toolset, plugins.ResearchInstructions)
			if err = worker.Send(ctx, "childonly assignment: runs before sunrise", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			childEvents, err := f.store.LoadEvents(ctx, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			// Reconstruct exact references a pre-integration index could have held.
			// The same constructor also creates the positive owner control.
			legacyEvidence := func(event memory.Event, record memory.Session) memory.RetrievalEvidence {
				now := time.Now().UTC()
				return memory.RetrievalEvidence{ID: fmt.Sprintf("excerpt:%s:0:%d", event.ID, len(event.Content)), Kind: memory.RetrievalConversationExcerpt, Intent: memory.RetrievalCurrent, ScopeKey: "global", AsKnownAt: now, ValidAt: now, Status: memory.SemanticStatusActive, CurrentStatus: memory.SemanticStatusActive, Text: event.Content, Sources: []memory.SemanticSource{integratedEventSource(event, record)}}
			}
			legacy := []memory.RetrievalEvidence{legacyEvidence(owner, parent)}
			for _, event := range childEvents {
				if event.Type == memory.EventUserMessage || event.Type == memory.EventAssistantMessage {
					legacy = append(legacy, legacyEvidence(event, child))
				}
			}
			var indexed int
			if err = f.db.QueryRow(`SELECT COUNT(*) FROM memory_retrieval_event_fts_v3 i JOIN events e ON e.id=i.event_id WHERE e.session_id=?`, child.ID).Scan(&indexed); err != nil {
				t.Fatal(err)
			}
			if indexed != 0 {
				t.Errorf("delegated content entered immediate conversation index: %d rows", indexed)
			}
			check := func(phase string) {
				t.Helper()
				f.refresh()
				query := "sunrise"
				if dense {
					query = "jogging at dawn"
				}
				reader := f.global()
				var refs []memory.RetrievalReference
				for _, item := range legacy {
					refs = append(refs, item.Reference())
				}
				inspected, err := f.store.InspectMemoryEvidence(ctx, reader.ScopeContext(), refs)
				if err != nil || len(inspected) != len(refs) || !inspected[0].Available {
					t.Fatalf("%s owner reference unavailable: %+v %v", phase, inspected, err)
				}
				for _, item := range inspected[1:] {
					if item.Available || item.Evidence != nil {
						t.Errorf("%s disclosed retained child reference", phase)
					}
				}
				valid, err := f.store.RevalidateMemoryEvidence(ctx, reader.ScopeContext(), legacy)
				if err != nil || len(valid) != 1 || valid[0].ID != legacy[0].ID {
					t.Fatalf("%s retained invalid child evidence: %+v %v", phase, valid, err)
				}
				for _, ref := range refs[1:] {
					expanded, err := f.store.SearchMemory(ctx, reader.ScopeContext(), memory.RetrievalQuery{Kind: memory.RetrievalConversationExpansion, AnchorID: ref.ID, Anchor: &ref, Before: 1, After: 1})
					if err != nil || expanded.Status != memory.RetrievalUnavailable || len(expanded.Evidence) != 0 {
						t.Fatalf("%s expanded a child reference: %+v %v", phase, expanded, err)
					}
				}
				found := f.searchConversations(reader, query)
				foundOwner := false
				for _, item := range denseTurnEvidence(t, found.reqs[1]) {
					for _, source := range item.Sources {
						if source.SessionID == child.ID || strings.Contains(source.Evidence, "childonly") {
							t.Errorf("%s disclosed delegated history: %+v", phase, item)
						}
						foundOwner = foundOwner || source.EventID == owner.ID && source.Authority == memory.AuthorityOwnerStatement && item.Text == owner.Content
					}
				}
				if !foundOwner {
					t.Errorf("%s lost eligible owner evidence", phase)
				}
				autoReader := f.global()
				autoClient := &fakeClient{steps: []step{assistantStep("Evidence checked.", nil)}}
				var definitions []tools.Tool
				for _, c := range plugins.NewMemory(f.store).ToolCapabilities() {
					definitions = append(definitions, c.Tool)
				}
				autoSession := NewWithToolset(autoClient, testContextProfile("test-model"), f.store.BindHistory(autoReader.ID, "automatic"), autoReader.ScopeContext(), f.store.BindTurnOwner(autoReader.ID, "automatic"), tools.NewToolset(definitions))
				if err = autoSession.Send(ctx, "Recall the sunrise running club.", &recorder{}, nil); err != nil {
					t.Fatal(err)
				}
				autoData := retrievalData(t, autoClient.reqs[0])
				if strings.Contains(autoData, "childonly") || strings.Contains(autoData, string(child.ID)) || !strings.Contains(autoData, string(owner.ID)) {
					t.Fatalf("%s automatic recall lost isolation or owner evidence: %s", phase, autoData)
				}
				events, err := f.store.LoadEvents(ctx, reader.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
					if event.Type == memory.EventContextSnapshot {
						var snapshot memory.ContextSnapshotPayload
						if err = json.Unmarshal(event.Payload, &snapshot); err != nil {
							t.Fatal(err)
						}
						b, _ := json.Marshal(snapshot.Memory)
						if strings.Contains(string(b), string(child.ID)) {
							t.Errorf("%s retained child evidence in a memory receipt", phase)
						}
					}
				}
				if endpoint != nil {
					endpoint.mu.Lock()
					defer endpoint.mu.Unlock()
					for _, batch := range endpoint.inputs {
						for _, text := range batch {
							if strings.Contains(text, "childonly") {
								t.Errorf("%s disclosed child content to embedding endpoint", phase)
							}
						}
					}
				}
			}
			reopen := func() {
				t.Helper()
				if err = f.db.Close(); err != nil {
					t.Fatal(err)
				}
				f.db, err = eviedb.OpenDBAt(f.path)
				if err != nil {
					t.Fatal(err)
				}
				f.store = eviedb.NewStore(f.db)
			}
			check("append")
			reopen()
			check("reopen")
			if _, err = f.db.Exec(`DROP TABLE memory_retrieval_fts_v3; DROP TABLE memory_retrieval_event_fts_v3; DELETE FROM memory_retrieval_generations WHERE generation IN ('accepted-fts-unicode61-v3','conversation-fts-unicode61-v3')`); err != nil {
				t.Fatal(err)
			}
			reopen()
			if dense {
				if _, err = f.store.RebuildMemoryEmbeddings(ctx); err != nil {
					t.Fatal(err)
				}
			}
			check("rebuild")
		})
	}
}
