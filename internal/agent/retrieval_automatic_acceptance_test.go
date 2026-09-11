package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func (f *retrievalFixture) automaticSession(record memory.Session, client Client) *Session {
	f.t.Helper()
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	holder := memory.LeaseHolderID("automatic-" + string(record.ID))
	return NewWithToolset(client, testContextProfile("test-model"), f.store.BindHistory(record.ID, holder), record.ScopeContext(), f.store.BindTurnOwner(record.ID, holder), tools.NewToolset(definitions))
}

func TestAutomaticMemoryRecallSuppliesPreferenceOnFirstRequest(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	preference := readerRemember(t, f, source, "dinner_preference", "Remember that I prefer vegetarian dinners.", "vegetarian dinners")
	f.refresh()
	before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{assistantStep("Here is a dinner suggestion based on the supplied preference.", nil)}}
	if err := f.automaticSession(reader, client).Send(context.Background(), "Suggest dinner for me.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 1 {
		t.Fatalf("ordinary request unexpectedly required a search continuation: %d", len(client.reqs))
	}
	data := retrievalData(t, client.reqs[0])
	if !strings.Contains(data, string(preference.ClaimID)) || !strings.Contains(data, string(preference.Source.EventID)) || !strings.Contains(data, "vegetarian dinners") {
		t.Fatalf("first request omitted relevant accepted preference: %s", data)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var supplied bool
	for _, event := range events {
		if strings.Contains(event.Content, "EVIE_MEMORY_DATA") {
			t.Fatal("automatic evidence became a durable conversation episode")
		}
		if event.Type == memory.EventContextSnapshot {
			var snapshot memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Memory != nil {
				for _, reference := range snapshot.Memory.Evidence {
					supplied = supplied || reference.ClaimID == preference.ClaimID
				}
			}
		}
	}
	if !supplied {
		t.Fatal("first request lacks its original evidence receipt")
	}
	after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil || after.ScopeRevision != before.ScopeRevision || len(after.Claims) != len(before.Claims) {
		t.Fatalf("automatic read changed accepted memory: %v", err)
	}
}

func TestAutomaticMemoryRecallRevalidatesEgressAfterCompactionBeforeDispatch(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	saved := readerRemember(t, f, source, "dinner_preference", "Remember that I prefer vegetarian dinners.", "vegetarian dinners")
	profile, err := openrouter.NewExplicitContextProfile("test/model", 1000000, 1000000, 1)
	if err != nil {
		t.Fatal(err)
	}
	holder := memory.LeaseHolderID("pressure-setup")
	seed := NewWithToolset(&fakeClient{steps: []step{assistantStep("noted", nil), assistantStep("noted", nil), assistantStep("noted", nil)}}, profile, f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(nil))
	for _, letter := range []string{"a", "b", "c"} {
		if err := seed.Send(context.Background(), strings.Repeat(letter, 60000), &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	f.refresh()
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	compactor := &expansionBoundaryClient{reply: func(openrouter.ChatRequest, int) step {
		t.Setenv("EVIE_REMOTE_MEMORY", "off")
		return assistantStep(validCompactionSummary(), nil)
	}}
	client := &fakeClient{steps: []step{assistantStep("I can help from the current request.", nil)}}
	holder = "automatic-pressure"
	session := NewWithCompactorAndToolset(client, compactor, automaticTestProfile(t, 230000), f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions))
	if err := session.Send(context.Background(), "Suggest dinner. "+strings.Repeat("d", 8000), &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(compactor.reqs) != 1 || len(client.reqs) != 1 {
		t.Fatalf("compactor=%d provider=%d", len(compactor.reqs), len(client.reqs))
	}
	wire, err := openrouter.RequestBytes(client.reqs[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), string(saved.Source.EventID)) || strings.Contains(string(wire), "vegetarian") {
		t.Fatal("provider received memory after opt-out during compaction")
	}
	if !strings.Contains(retrievalData(t, client.reqs[0]), `"status":"unavailable"`) {
		t.Fatal("memory failure was not explicit after compaction")
	}
}

func TestAutomaticMemoryRecallFindsUncompiledFactAndKeepsNoMatchDistinctFromUnavailable(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	original := f.converse(source, "The greenhouse has saffron crocuses, still an experiment.")
	f.converse(source, "I maintain a bicycle chain on Tuesdays.")
	f.refresh()
	for _, tc := range []struct {
		name, query, optin, status string
		want                       bool
	}{
		{"uncompiled", "What did I plant in the greenhouse?", "on", "success", true},
		{"empty", "Tell me about zirconium plating.", "on", "empty", false},
		{"opt out", "What did I plant in the greenhouse?", "off", "unavailable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EVIE_REMOTE_MEMORY", tc.optin)
			client := &fakeClient{steps: []step{assistantStep("Answer uses only available support.", nil)}}
			if err := f.automaticSession(f.global(), client).Send(context.Background(), tc.query, &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			data := retrievalData(t, client.reqs[0])
			if strings.Contains(data, string(original.ID)) != tc.want || !strings.Contains(data, `"status":"`+tc.status+`"`) {
				t.Fatalf("automatic result=%s", data)
			}
			if strings.Contains(data, "bicycle") || strings.Contains(data, `"claim_id"`) {
				t.Fatalf("uncompiled result invented acceptance or included a distractor: %s", data)
			}
		})
	}
}

func TestAutomaticMemoryRecallScopeMatrixKeepsRawHistoryInItsArea(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	composition, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	type area struct {
		name           string
		source, reader memory.Session
		event          memory.Event
	}
	areas := []area{{name: "Global", source: f.global(), reader: f.global()}}
	for _, name := range []string{"General", "Another workspace"} {
		w, err := f.store.RegisterWorkspace(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		source, err := f.store.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, composition.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := f.store.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, composition.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		areas = append(areas, area{name: name, source: source, reader: reader})
	}
	for _, name := range []string{"Project one", "Project two"} {
		p, err := f.store.RegisterProject(ctx, name, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		source, err := f.store.CreateProjectSession(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := f.store.CreateProjectSession(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		areas = append(areas, area{name: name, source: source, reader: reader})
	}
	global := readerRemember(t, f, areas[0].source, "dinner_preference", "Remember that I prefer vegetarian dinners.", "vegetarian dinners")
	for i := range areas {
		areas[i].event = f.converse(areas[i].source, "The greenhouse orchid trial belongs to "+areas[i].name+".")
	}
	f.refresh()
	for _, a := range areas {
		t.Run(a.name, func(t *testing.T) {
			client := &fakeClient{steps: []step{assistantStep("I have the eligible dinner and greenhouse context.", nil)}}
			if err := f.automaticSession(a.reader, client).Send(ctx, "What dinner suits me, and what was the greenhouse trial?", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			evidence := expansionBoundaryEvidence(t, client.reqs[0])
			claim, original := false, false
			for _, e := range evidence {
				if e.Kind == memory.RetrievalAcceptedMemory {
					claim = claim || e.ClaimID == global.ClaimID
					continue
				}
				if e.Kind != memory.RetrievalConversationExcerpt || e.ClaimID != "" {
					t.Fatalf("unexpected evidencekind: %+v", e)
				}
				for _, s := range e.Sources {
					if s.SessionID != a.source.ID {
						t.Fatalf("automatic raw history crossed area: %+v", s)
					}
					original = original || s.EventID == a.event.ID
				}
			}
			if !claim || !original {
				t.Fatalf("automatic recall lacks accepted Global preference or own-area original: %+v", evidence)
			}
		})
	}
}

func TestAutomaticMemoryRecallDropsRetiredEvidenceBeforeContinuation(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	saved := readerRemember(t, f, source, "dinner_preference", "Remember that I prefer vegetarian dinners.", "vegetarian dinners")
	f.refresh()
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		if call == 0 {
			if !strings.Contains(retrievalData(t, request), string(saved.ClaimID)) {
				t.Fatal("first automatic request missed the preference")
			}
			f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, saved.ClaimID)
			return assistantStep("", nil, toolCall("check", "memory_search", `{"query":"dinner"}`))
		}
		return assistantStep("The previous preference is unavailable as current support.", nil)
	}}
	if err := f.automaticSession(reader, client).Send(context.Background(), "Suggest dinner.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	data := retrievalData(t, client.reqs[1])
	if strings.Contains(data, string(saved.ClaimID)) || strings.Contains(data, "vegetarian") {
		t.Fatal("continuation revived retired evidence")
	}
	if !strings.Contains(data, `"status":"unavailable"`) {
		t.Fatalf("withdrawn evidence presented as successful absence: %s", data)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	receipts := 0
	for _, event := range events {
		if event.Type == memory.EventContextSnapshot {
			var snapshot memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Memory != nil && len(snapshot.Memory.Evidence) > 0 {
				receipts++
			}
		}
	}
	if receipts != 1 {
		t.Fatalf("immutable first receipt changed: evidence-bearing requests=%d", receipts)
	}
}

// Use a real independently closed SQLite read connection while the durable turn
// connection stays live. No Kernel response or SQL result is scripted.
type separateMemoryReadHistory struct {
	*eviedb.SessionHistory
	reads *eviedb.Store
}

func (h *separateMemoryReadHistory) SearchMemory(ctx context.Context, scope memory.ScopeContext, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
	return h.reads.SearchMemory(ctx, scope, query)
}
func (h *separateMemoryReadHistory) RevalidateMemoryEvidence(ctx context.Context, scope memory.ScopeContext, evidence []memory.RetrievalEvidence) ([]memory.RetrievalEvidence, error) {
	return h.reads.RevalidateMemoryEvidence(ctx, scope, evidence)
}

func TestAutomaticMemoryRecallFailedSQLiteLookupDoesNotBecomeEmpty(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	readerRemember(t, f, source, "dinner_preference", "Remember that I prefer vegetarian dinners.", "vegetarian dinners")
	f.refresh()
	connection, err := eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	reads := eviedb.NewStore(connection)
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	holder := memory.LeaseHolderID("failed-memory-read")
	history := &separateMemoryReadHistory{SessionHistory: f.store.BindHistory(reader.ID, holder), reads: reads}
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	client := &fakeClient{steps: []step{assistantStep("I can answer a general dinner question, but the memory lookup failed.", nil)}}
	session := NewWithToolset(client, testContextProfile("test-model"), history, reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions))
	if err := session.Send(context.Background(), "Suggest dinner.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	data := retrievalData(t, client.reqs[0])
	if !strings.Contains(data, `"status":"failed"`) || strings.Contains(data, "vegetarian") || strings.Contains(data, "database") {
		t.Fatalf("lookup failure lost its boundary: %s", data)
	}
}

func TestAutomaticMemoryRecallUsesEarlierDiscussionAndCompactionContinuity(t *testing.T) {
	for _, compacted := range []bool{false, true} {
		name := "earlier discussion"
		if compacted {
			name = "persisted compaction after twenty topic changes"
		}
		t.Run(name, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source, reader := f.global(), f.global()
			original := f.converse(source, "The greenhouse trial contains saffron crocuses and remains experimental.")
			f.converse(reader, "We were discussing the greenhouse.")
			var compactionID memory.EventID
			if compacted {
				for i := 0; i < 20; i++ {
					f.converse(reader, "Next, consider compiler diagnostics.")
				}
				summary := strings.ReplaceAll(validCompactionSummary(), "kept", "greenhouse discussion")
				compactor := &fakeClient{steps: []step{assistantStep(summary, nil)}}
				holder := memory.LeaseHolderID("continuity-compaction")
				session := NewWithCompactorAndToolset(compactor, compactor, testContextProfile("test-model"), f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(nil))
				result, err := session.Compact(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				compactionID = result.CompactionEventID
			}
			f.refresh()
			client := &fakeClient{steps: []step{assistantStep("The original statement supplies the answer.", nil)}}
			if err := f.automaticSession(reader, client).Send(context.Background(), "What did that contain?", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range expansionBoundaryEvidence(t, client.reqs[0]) {
				if compacted && strings.Contains(item.Text, "compiler diagnostics") {
					t.Fatal("unrelated earlier debugging topic diluted compaction continuity")
				}
				for _, ref := range item.Sources {
					found = found || ref.EventID == original.ID
					if ref.EventID == compactionID {
						t.Fatal("continuity summary was promoted into source evidence")
					}
				}
			}
			if !found {
				t.Fatalf("vague request did not recall its original through context: %s", retrievalData(t, client.reqs[0]))
			}
			events, err := f.store.LoadEvents(context.Background(), reader.ID)
			if err != nil {
				t.Fatal(err)
			}
			var interpretation *memory.RetrievalInterpretation
			for _, event := range events {
				if event.Type == memory.EventContextSnapshot {
					var snapshot memory.ContextSnapshotPayload
					if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
						t.Fatal(err)
					}
					if snapshot.Memory != nil {
						interpretation = snapshot.Memory.Interpretation
					}
				}
			}
			if interpretation == nil || !compacted && interpretation.EarlierMessages == 0 || compacted && interpretation.SummaryBytes == 0 {
				t.Fatalf("missing content-free interpretation receipt: %+v", interpretation)
			}
		})
	}
}
