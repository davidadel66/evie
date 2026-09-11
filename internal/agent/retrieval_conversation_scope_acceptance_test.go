package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

func TestConversationSearchTurnScopeMatrixAndEarlierRoots(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	type actor struct {
		name             string
		current, sibling memory.Session
		earlier, other   memory.Event
	}
	actors := []actor{{name: "Global", current: f.global(), sibling: f.global()}}
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
		actors = append(actors, actor{name: name, current: current, sibling: sibling})
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
		actors = append(actors, actor{name: name, current: current, sibling: sibling})
	}
	for i := range actors {
		a := &actors[i]
		// Nonzero accepted revisions make the read-only assertion meaningful.
		// Global accepted memory is visible to every actor but grants no raw
		// Global history access to the Workspace or project actors.
		f.remember(a.current, "", "accepted baseline "+a.name)
		f.remember(a.sibling, memory.MemorySession, "private session baseline "+a.name)
		a.earlier = f.converse(a.current, "scopeviolet earlier current-session statement in "+a.name)
		a.other = f.converse(a.sibling, "scopeviolet earlier sibling-session statement in "+a.name)
	}
	f.refresh()

	for _, a := range actors {
		t.Run(a.name, func(t *testing.T) {
			before, err := f.store.InspectClaims(ctx, a.current.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeClient{steps: []step{
				assistantStep("", nil, toolCall("scope-history", "memory_search_conversations", `{"query":"scopeviolet"}`)),
				assistantStep("Original evidence received.", nil),
			}}
			// The live user root deliberately matches. Only its earlier roots
			// may appear in conversation recall, including in this same session.
			if err := f.session(a.current, client).Send(ctx, "Find scopeviolet in earlier conversations.", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			if len(client.reqs) != 2 {
				t.Fatalf("provider requests = %d, want 2", len(client.reqs))
			}
			var supplied struct {
				Evidence []memory.RetrievalEvidence `json:"evidence"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, client.reqs[1]), "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
				t.Fatal(err)
			}
			allowed := map[memory.EventID]memory.Event{a.earlier.ID: a.earlier, a.other.ID: a.other}
			var providerIDs []string
			for _, evidence := range supplied.Evidence {
				if evidence.Kind != memory.RetrievalConversationExcerpt || evidence.ClaimID != "" || len(evidence.Sources) != 1 {
					t.Fatalf("raw conversation acquired accepted-memory identity: %+v", evidence)
				}
				ref := evidence.Sources[0]
				original, ok := allowed[ref.EventID]
				if !ok || ref.SessionID != original.SessionID || evidence.Text != original.Content || ref.EvidenceSHA256 != memory.CompilerHash([]byte(original.Content)) {
					t.Fatalf("provider received an unauthorized or changed source: %+v", evidence)
				}
				providerIDs = append(providerIDs, string(ref.EventID))
			}
			want := []string{string(a.earlier.ID), string(a.other.ID)}
			sort.Strings(want)
			sort.Strings(providerIDs)
			if !reflect.DeepEqual(providerIDs, want) {
				t.Fatalf("provider source events = %v, want exactly %v", providerIDs, want)
			}
			events, err := f.store.LoadEvents(ctx, a.current.ID)
			if err != nil {
				t.Fatal(err)
			}
			var receiptIDs []string
			for _, event := range events {
				if event.Type != memory.EventContextSnapshot {
					continue
				}
				var snapshot memory.ContextSnapshotPayload
				if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
					t.Fatal(err)
				}
				if snapshot.Memory == nil {
					continue
				}
				for _, ref := range snapshot.Memory.Evidence {
					if ref.Kind != memory.RetrievalConversationExcerpt || ref.ClaimID != "" || len(ref.Sources) != 1 {
						t.Fatalf("invalid conversation source receipt: %+v", ref)
					}
					receiptIDs = append(receiptIDs, string(ref.Sources[0].EventID))
				}
			}
			sort.Strings(receiptIDs)
			if !reflect.DeepEqual(receiptIDs, want) {
				t.Fatalf("durable receipt source events = %v, want exactly %v", receiptIDs, want)
			}
			after, err := f.store.InspectClaims(ctx, a.current.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after.ScopeRevisions, before.ScopeRevisions) || len(after.Claims) != len(before.Claims) {
				t.Fatalf("conversation search changed accepted memory: before=%+v after=%+v", before.ScopeRevisions, after.ScopeRevisions)
			}
		})
	}
}
