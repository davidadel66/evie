package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

type denseScopeActor struct {
	name             string
	current, sibling memory.Session
}

func denseScopeActors(t *testing.T, f *retrievalFixture) []denseScopeActor {
	t.Helper()
	ctx := context.Background()
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	actors := []denseScopeActor{{name: "Global", current: f.global(), sibling: f.global()}}
	for _, name := range []string{"General", "Second Workspace"} {
		workspace, err := f.store.RegisterWorkspace(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		current, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		sibling, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		actors = append(actors, denseScopeActor{name, current, sibling})
	}
	for _, name := range []string{"Project A", "Project B"} {
		project, err := f.store.RegisterProject(ctx, name, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		current, err := f.store.CreateProjectSession(ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		sibling, err := f.store.CreateProjectSession(ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		actors = append(actors, denseScopeActor{name, current, sibling})
	}
	return actors
}

func denseLastReceipt(t *testing.T, events []memory.Event) *memory.RetrievalReceipt {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(events[i].Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil {
			return snapshot.Memory
		}
	}
	t.Fatal("reader turn did not persist a retrieval receipt")
	return nil
}

func TestDenseMemoryTurnKeepsContextAndCurrentSessionScopeMatrix(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	actors := denseScopeActors(t, f)
	type claims struct {
		context, current, sibling memory.RememberLiteralProposal
	}
	accepted := make([]claims, len(actors))
	for i, actor := range actors {
		accepted[i] = claims{
			context: f.remember(actor.current, "", actor.name+" context group runs before sunrise"),
			current: f.remember(actor.current, memory.MemorySession, actor.name+" current session group runs before sunrise"),
			sibling: f.remember(actor.sibling, memory.MemorySession, actor.name+" sibling session group runs before sunrise"),
		}
	}
	f.refresh()
	for i, actor := range actors {
		t.Run(actor.name, func(t *testing.T) {
			before, err := f.store.InspectClaims(context.Background(), actor.current.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			client, events := f.search(actor.current, "jogging at dawn")
			want := map[memory.SemanticID]memory.EventID{
				accepted[0].context.ClaimID: accepted[0].context.Source.EventID,
				accepted[i].current.ClaimID: accepted[i].current.Source.EventID,
			}
			if i != 0 {
				want[accepted[i].context.ClaimID] = accepted[i].context.Source.EventID
			}
			got := make(map[memory.SemanticID]memory.EventID)
			for _, item := range denseTurnEvidence(t, client.reqs[len(client.reqs)-1]) {
				if len(item.Sources) != 1 || !reflect.DeepEqual(item.Paths, []string{"dense"}) || item.Status != memory.SemanticStatusActive {
					t.Fatalf("dense evidence lost exact accepted provenance: %+v", item)
				}
				if _, duplicate := got[item.ClaimID]; duplicate {
					t.Fatal("dense accepted evidence repeated a Claim")
				}
				got[item.ClaimID] = item.Sources[0].EventID
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("dense provider Claim/source pairs = %v, want exactly %v", got, want)
			}
			receipt := make(map[memory.SemanticID]memory.EventID)
			recorded := denseLastReceipt(t, events).Evidence
			if len(recorded) != len(want) {
				t.Fatalf("dense accepted receipt count = %d, want %d", len(recorded), len(want))
			}
			for _, ref := range recorded {
				if len(ref.Sources) != 1 {
					t.Fatalf("dense receipt lost exact source: %+v", ref)
				}
				receipt[ref.ClaimID] = ref.Sources[0].EventID
			}
			if !reflect.DeepEqual(receipt, want) {
				t.Fatalf("dense receipt Claim/source pairs = %v, want exactly %v", receipt, want)
			}
			after, err := f.store.InspectClaims(context.Background(), actor.current.ScopeContext(), memory.ClaimQuery{})
			if err != nil || !reflect.DeepEqual(after.ScopeRevisions, before.ScopeRevisions) {
				t.Fatalf("dense read changed accepted scope revisions: %v", err)
			}
		})
	}
}

func TestDenseConversationTurnKeepsExactContextAndExcludesIndexedLiveRoot(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	actors := denseScopeActors(t, f)
	originals := make([]map[memory.EventID]memory.Event, len(actors))
	for i, actor := range actors {
		// Nonzero accepted revisions are independently visible to eligible
		// readers but do not grant access to raw Global conversation history.
		f.remember(actor.current, "", actor.name+" accepted baseline")
		current := f.converse(actor.current, actor.name+" earlier current-session club runs before sunrise")
		sibling := f.converse(actor.sibling, actor.name+" earlier sibling-session club runs before sunrise")
		originals[i] = map[memory.EventID]memory.Event{current.ID: current, sibling.ID: sibling}
	}
	f.refresh()
	for i, actor := range actors {
		t.Run(actor.name, func(t *testing.T) {
			ctx := context.Background()
			before, err := f.store.InspectClaims(ctx, actor.current.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeClient{steps: []step{
				assistantStep("", nil, toolCall("dense-history", "memory_search_conversations", `{"query":"jogging at dawn"}`)),
				assistantStep("Original evidence received.", nil),
			}}
			calls := 0
			client.onCall = func() {
				if calls == 0 {
					// The live user root is actually embedded before the search.
					// Its exclusion must come from the authoritative turn cutoff.
					f.refresh()
				}
				calls++
			}
			if err := f.session(actor.current, client).Send(ctx, "jogging at dawn", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			got := make(map[memory.EventID]memory.SessionID)
			want := make(map[memory.EventID]memory.SessionID)
			for id, original := range originals[i] {
				want[id] = original.SessionID
			}
			for _, item := range denseTurnEvidence(t, client.reqs[len(client.reqs)-1]) {
				if item.Kind != memory.RetrievalConversationExcerpt || item.ClaimID != "" || item.Claim != nil || len(item.Sources) != 1 || !reflect.DeepEqual(item.Paths, []string{"conversation_dense"}) {
					t.Fatalf("dense raw evidence acquired accepted authority or lost its retrieval path: %+v", item)
				}
				source := item.Sources[0]
				original, allowed := originals[i][source.EventID]
				if !allowed || source.SessionID != original.SessionID || item.Text != original.Content || source.Evidence != original.Content || source.EvidenceSHA256 != memory.CompilerHash([]byte(original.Content)) || source.Authority != memory.AuthorityOwnerStatement {
					t.Fatalf("dense evidence disclosed another context, live root, or changed original: %+v", item)
				}
				if _, duplicate := got[source.EventID]; duplicate {
					t.Fatal("dense conversation evidence repeated an original event")
				}
				got[source.EventID] = source.SessionID
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("dense provider event/session pairs = %v, want exactly %v", got, want)
			}
			events, err := f.store.LoadEvents(ctx, actor.current.ID)
			if err != nil {
				t.Fatal(err)
			}
			receipt := make(map[memory.EventID]memory.SessionID)
			recorded := denseLastReceipt(t, events).Evidence
			if len(recorded) != len(want) {
				t.Fatalf("dense conversation receipt count = %d, want %d", len(recorded), len(want))
			}
			for _, ref := range recorded {
				if ref.Kind != memory.RetrievalConversationExcerpt || ref.ClaimID != "" || len(ref.Sources) != 1 {
					t.Fatalf("dense conversation receipt lost exact original authority: %+v", ref)
				}
				receipt[ref.Sources[0].EventID] = ref.Sources[0].SessionID
			}
			if !reflect.DeepEqual(receipt, want) {
				t.Fatalf("dense receipt event/session pairs = %v, want exactly %v", receipt, want)
			}
			after, err := f.store.InspectClaims(ctx, actor.current.ScopeContext(), memory.ClaimQuery{})
			if err != nil || !reflect.DeepEqual(after.ScopeRevisions, before.ScopeRevisions) {
				t.Fatalf("dense conversation read changed accepted scope revisions: %v", err)
			}
		})
	}
}

func TestDenseConversationTurnRechecksRetiredUTF8RangeAndRevokedSource(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	source := f.global()
	basis := f.remember(source, memory.MemoryEverywhere, "dense retirement basis")
	const passage = "café running club runs before sunrise"
	original := f.converse(source, "Mañana—my "+passage+"; the greenhouse contains orchids.")
	claims := f.acceptRangeClaims(source, original, basis, [][2]string{{"runs before sunrise", passage}})
	accepted, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectClaim, claims[0])
	if err != nil || len(accepted.Sources) != 1 {
		t.Fatalf("accepted range fixture has no exact source association: %v", err)
	}
	link := accepted.Sources[0].Source.ID
	if link == "" {
		t.Fatal("accepted range fixture did not retain its Source Link identity")
	}
	start := strings.Index(original.Content, passage)
	locator := fmt.Sprintf("%d:%d", start, start+len(passage))
	f.refresh()
	initial := f.searchConversations(f.global(), "jogging at dawn")
	initialEvidence := denseTurnEvidence(t, initial.reqs[len(initial.reqs)-1])
	if len(initialEvidence) != 1 || initialEvidence[0].Text != original.Content || len(initialEvidence[0].Sources) != 1 || initialEvidence[0].Sources[0].EventID != original.ID {
		t.Fatalf("active dense source fixture was not supplied as its original statement: %+v", initialEvidence)
	}
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[0])
	assertSuppressed := func(client *fakeClient) {
		t.Helper()
		for _, item := range denseTurnEvidence(t, client.reqs[len(client.reqs)-1]) {
			if strings.Contains(item.Text, passage) || item.ClaimID == claims[0] {
				t.Fatalf("ordinary or source-restricted dense read restored a suppressed range: %+v", item)
			}
			for _, ref := range item.Sources {
				if strings.Contains(ref.Evidence, passage) || ref.ID == link {
					t.Fatalf("dense source reference restored suppressed original evidence: %+v", ref)
				}
				if ref.EventID == original.ID {
					var from, to int
					_, err := fmt.Sscanf(ref.LocatorValue, "%d:%d", &from, &to)
					if err != nil || ref.LocatorKind != memory.LocatorUTF8ByteRange || ref.LocatorValue != fmt.Sprintf("%d:%d", from, to) || from < 0 || to <= from || to > len(original.Content) || from < start+len(passage) && to > start {
						t.Fatalf("dense source locator overlaps the suppressed UTF-8 range: %+v", ref)
					}
				}
			}
		}
	}
	// First query sees the old vector generation. The following queries see
	// reconciled documents; both must respect the same current retirement.
	assertSuppressed(f.searchConversations(f.global(), "jogging at dawn"))
	f.refresh()
	assertSuppressed(f.searchConversations(f.global(), "jogging at dawn"))
	assertHistorical := func(options map[string]any, status memory.SemanticObjectStatus) {
		t.Helper()
		client := f.historicalSearch(f.global(), "memory_search_conversations", "jogging at dawn", options)
		evidence := denseTurnEvidence(t, client.reqs[len(client.reqs)-1])
		if len(evidence) != 1 {
			t.Fatalf("historical dense range count = %d, want one original range", len(evidence))
		}
		item := evidence[0]
		if item.Kind != memory.RetrievalConversationExcerpt || item.ClaimID != "" || item.Intent != memory.RetrievalHistorical || item.Status != status || item.CurrentStatus != memory.SemanticStatusRetired || item.Text != passage || !utf8.ValidString(item.Text) || !reflect.DeepEqual(item.Paths, []string{"conversation_dense"}) || len(item.Sources) != 1 {
			t.Fatalf("historical dense range lost original authority, read pin, or current retirement: %+v", item)
		}
		ref := item.Sources[0]
		if ref.EventID != original.ID || ref.SessionID != source.ID || ref.LocatorKind != memory.LocatorUTF8ByteRange || ref.LocatorValue != locator || ref.Evidence != passage || ref.EvidenceSHA256 != memory.CompilerHash([]byte(passage)) || ref.Authority != memory.AuthorityOwnerStatement {
			t.Fatalf("historical dense excerpt changed its original UTF-8 locator: %+v", ref)
		}
	}
	assertHistorical(nil, memory.SemanticStatusRetired)
	beforeAcceptance := map[string]any{"as_known_at": original.RecordedAt.Format(time.RFC3339Nano)}
	assertHistorical(beforeAcceptance, memory.SemanticStatusActive)
	f.lifecycle(source, "memory_retract_source", memory.SemanticObjectSourceLink, link)
	// A historical read pin never restores source access, including before
	// backfill has removed the previously eligible vector.
	assertSuppressed(f.historicalSearch(f.global(), "memory_search_conversations", "jogging at dawn", beforeAcceptance))
	f.refresh()
	assertSuppressed(f.historicalSearch(f.global(), "memory_search_conversations", "jogging at dawn", beforeAcceptance))
	assertSuppressed(f.searchConversations(f.global(), "jogging at dawn"))
}
