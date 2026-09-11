package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemorySearchTurnFindsSourceBearingTwoHopRelationship(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	source := f.global()
	relationship := f.rememberGraphEntity(source, "Maya's sister is Nora.", memory.RememberEntityRequest{
		Predicate: "sister", PredicateLabel: "sister",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Nora", EntityType: "person", Alias: "Nora"},
	})
	preference := f.rememberGraphEntity(source, "Nora prefers baklava.", memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers",
		Subject: memory.EntitySelector{EntityID: relationship.Claim.ObjectEntityID},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "baklava", EntityType: "food", Alias: "baklava"},
	})
	f.refresh()
	before, err := f.store.InspectClaims(ctx, source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	client, _ := f.search(f.global(), "Maya")
	evidence := expansionBoundaryEvidence(t, client.reqs[1])
	var got []memory.SemanticID
	for _, item := range evidence {
		if item.Kind != memory.RetrievalAcceptedMemory {
			continue
		}
		got = append(got, item.ClaimID)
		var original memory.RememberEntityProposal
		switch item.ClaimID {
		case relationship.Claim.ID:
			original = relationship
		case preference.Claim.ID:
			original = preference
		default:
			t.Fatalf("unrelated Claim filled the focused relationship answer: %+v", item)
		}
		if item.Claim == nil || item.Status != memory.SemanticStatusActive || len(item.Sources) != 1 || item.Sources[0].EventID != original.Source.EventID || item.Sources[0].Evidence != original.Source.Evidence || item.Sources[0].Authority != memory.AuthorityOwnerStatement {
			t.Fatalf("relationship retrieval lost accepted Claim or exact owner source support: %+v", item)
		}
		if item.ClaimID == preference.Claim.ID {
			wantPath := memory.RetrievalGraphPath{AnchorEntityID: relationship.Claim.SubjectEntityID, ClaimIDs: []memory.SemanticID{relationship.Claim.ID, preference.Claim.ID}}
			if !reflect.DeepEqual(item.GraphPaths, []memory.RetrievalGraphPath{wantPath}) || !slices.Contains(item.Paths, "graph_two_hop") {
				t.Fatalf("two-hop evidence lacks its exact accepted support path: %+v", item)
			}
		}
	}
	slices.Sort(got)
	want := []memory.SemanticID{relationship.Claim.ID, preference.Claim.ID}
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Maya relationship question lacks supported two-hop preference: got=%v want=%v", got, want)
	}
	after, err := f.store.InspectClaims(ctx, source.ScopeContext(), memory.ClaimQuery{})
	if err != nil || !reflect.DeepEqual(before.Claims, after.Claims) || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
		t.Fatalf("relationship search changed accepted memory: %v", err)
	}
}

func TestMemorySearchTurnGraphScopeMatrixKeepsGlobalAnchorInsideReaderBoundary(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	global := f.global()
	bridge := f.rememberGraphEntity(global, "Selene's sister is Iris.", memory.RememberEntityRequest{
		Predicate: "sister", PredicateLabel: "sister",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Selene", EntityType: "person", Alias: "Selene"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Iris", EntityType: "person", Alias: "Iris"},
	})
	type actor struct {
		name             string
		current, sibling memory.Session
		claim, session   memory.SemanticID
	}
	actors := []actor{{name: "Global", current: global, sibling: f.global()}}
	composition, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"General", "Second Workspace"} {
		workspace, err := f.store.RegisterWorkspace(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		current, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		sibling, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
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
		for _, location := range []struct {
			name        string
			record      memory.Session
			destination memory.MemoryDestination
		}{
			{name: "context", record: a.current},
			{name: "current", record: a.current, destination: memory.MemorySession},
			{name: "sibling", record: a.sibling, destination: memory.MemorySession},
		} {
			label := a.name + " " + location.name + " pastry"
			p := f.rememberGraphEntity(location.record, "Iris prefers "+label+".", memory.RememberEntityRequest{
				Destination: location.destination, Predicate: "prefers", PredicateLabel: "prefers",
				Subject: memory.EntitySelector{EntityID: bridge.Claim.ObjectEntityID},
				Object:  memory.EntitySelector{Create: true, CanonicalName: label, EntityType: "food", Alias: label},
			})
			if location.name == "context" {
				a.claim = p.Claim.ID
			} else if location.name == "current" {
				a.session = p.Claim.ID
			}
		}
	}
	f.refresh()
	for _, a := range actors {
		t.Run(a.name, func(t *testing.T) {
			client, _ := f.search(a.current, "Selene")
			evidence := expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1])
			var got []memory.SemanticID
			for _, item := range evidence {
				if item.Kind != memory.RetrievalAcceptedMemory {
					t.Fatalf("graph discovery must not widen raw conversation access: %+v", item)
				}
				got = append(got, item.ClaimID)
				for _, path := range item.GraphPaths {
					if path.AnchorEntityID != bridge.Claim.SubjectEntityID {
						t.Fatalf("path introduced an unrelated anchor: %+v", path)
					}
				}
				for _, source := range item.Sources {
					if strings.Contains(source.Evidence, "sibling") {
						t.Fatalf("source disclosed sibling-session evidence: %+v", source)
					}
				}
			}
			want := []memory.SemanticID{bridge.Claim.ID, actors[0].claim, a.session}
			if a.name != "Global" {
				want = append(want, a.claim)
			}
			slices.Sort(got)
			slices.Sort(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Global anchor crossed graph scope boundary: got=%v want exactly %v", got, want)
			}
		})
	}
}

func (f *retrievalFixture) rememberGraphEntity(record memory.Session, text string, request memory.RememberEntityRequest) memory.RememberEntityProposal {
	f.t.Helper()
	request.IdempotencyKey = "idem:v1:" + uuid.NewString()
	if request.PredicateCardinality == "" {
		request.PredicateCardinality = memory.CardinalityMany
	}
	if request.Polarity == "" {
		request.Polarity = memory.PolarityAffirmed
	}
	session := f.session(record, nil)
	proposal, err := session.PrepareRememberEntity(context.Background(), f.store, text, request)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := session.ResolveRememberEntity(context.Background(), f.store, proposal, tools.Approved); err != nil {
		f.t.Fatal(err)
	}
	return proposal
}

func TestMemorySearchTurnGraphPathsBoundCyclesAndKeepContradictions(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	first := f.rememberGraphEntity(source, "Maya's sister is Nora.", memory.RememberEntityRequest{
		Predicate: "sister", PredicateLabel: "sister",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Nora", EntityType: "person", Alias: "Nora"},
	})
	second := f.rememberGraphEntity(source, "Maya knows Nora.", memory.RememberEntityRequest{
		Predicate: "knows", PredicateLabel: "knows",
		Subject: memory.EntitySelector{EntityID: first.Claim.SubjectEntityID}, Object: memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
	})
	cycle := f.rememberGraphEntity(source, "Nora knows Maya.", memory.RememberEntityRequest{
		Predicate: "knows", PredicateLabel: "knows",
		Subject: memory.EntitySelector{EntityID: first.Claim.ObjectEntityID}, Object: memory.EntitySelector{EntityID: first.Claim.SubjectEntityID},
	})
	preferred := f.rememberGraphEntity(source, "Nora prefers baklava.", memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers", Subject: memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
		Object: memory.EntitySelector{Create: true, CanonicalName: "baklava", EntityType: "food", Alias: "baklava"},
	})
	denied := f.rememberGraphEntity(source, "Nora does not prefer baklava.", memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers", Polarity: memory.PolarityDenied,
		Subject: memory.EntitySelector{EntityID: first.Claim.ObjectEntityID}, Object: memory.EntitySelector{EntityID: preferred.Claim.ObjectEntityID},
	})
	thirdHop := f.rememberGraphEntity(source, "Baklava originated in Antep.", memory.RememberEntityRequest{
		Predicate: "originated_in", PredicateLabel: "originated in", Subject: memory.EntitySelector{EntityID: preferred.Claim.ObjectEntityID},
		Object: memory.EntitySelector{Create: true, CanonicalName: "Antep", EntityType: "place", Alias: "Antep"},
	})
	f.refresh()
	client, _ := f.search(f.global(), "Maya")
	evidence := expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1])
	var got []memory.SemanticID
	for _, item := range evidence {
		got = append(got, item.ClaimID)
		if item.ClaimID == thirdHop.Claim.ID {
			t.Fatal("graph search escaped the supported two-hop boundary")
		}
		if item.ClaimID != preferred.Claim.ID && item.ClaimID != denied.Claim.ID {
			continue
		}
		if len(item.GraphPaths) != 2 || len(item.Conflicts) != 1 || item.Conflicts[0].Code != memory.ConflictOppositePolarity {
			t.Fatalf("bounded alternate paths must retain the contradictory evidence: %+v", item)
		}
		var bridges []memory.SemanticID
		for _, path := range item.GraphPaths {
			if path.AnchorEntityID != first.Claim.SubjectEntityID || len(path.ClaimIDs) != 2 || path.ClaimIDs[1] != item.ClaimID || path.ClaimIDs[0] == item.ClaimID {
				t.Fatalf("cyclic, unsupported or mislabeled path: %+v", path)
			}
			bridges = append(bridges, path.ClaimIDs[0])
		}
		if bridges[0] == bridges[1] {
			t.Fatalf("duplicate support paths inflated graph evidence: %+v", item.GraphPaths)
		}
	}
	slices.Sort(got)
	want := []memory.SemanticID{first.Claim.ID, second.Claim.ID, cycle.Claim.ID, preferred.Claim.ID, denied.Claim.ID}
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("direct competition or contradictory terminal evidence lost: got=%v want=%v", got, want)
	}
}

func TestMemorySearchTurnGraphSupportRevalidatesBeforeDispatch(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	bridge := f.rememberGraphEntity(source, "Maya's sister is Nora.", memory.RememberEntityRequest{
		Predicate: "sister", PredicateLabel: "sister",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Nora", EntityType: "person", Alias: "Nora"},
	})
	terminal := f.rememberGraphEntity(source, "Nora prefers baklava.", memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers", Subject: memory.EntitySelector{EntityID: bridge.Claim.ObjectEntityID},
		Object: memory.EntitySelector{Create: true, CanonicalName: "baklava", EntityType: "food", Alias: "baklava"},
	})
	f.refresh()
	args, _ := json.Marshal(map[string]string{"idempotency_key": "idem:v1:" + uuid.NewString(), "source_link_id": string(bridge.Source.ID)})
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("search", "memory_search", `{"query":"Maya"}`), toolCall("retract", "memory_retract_source", string(args))),
		assistantStep("The supporting relationship is unavailable.", nil),
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Find Maya's related preference, then retract the cited relationship source.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		t.Fatal(err)
	}
	for _, item := range expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1]) {
		if item.ClaimID == terminal.Claim.ID || item.ClaimID == bridge.Claim.ID {
			t.Fatalf("dispatch retained graph evidence after its only support became unavailable: %+v", item)
		}
	}
	inspection, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectClaim, terminal.Claim.ID)
	if err != nil || inspection.Status != memory.SemanticStatusActive {
		t.Fatalf("graph evidence pruning changed the independently accepted terminal Claim: %+v %v", inspection, err)
	}
}

func TestMemorySearchTurnGraphExcludesRetiredEntityAndItsRelationships(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	bridge := f.rememberGraphEntity(source, "Maya's sister is Nora.", memory.RememberEntityRequest{
		Predicate: "sister", PredicateLabel: "sister",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Nora", EntityType: "person", Alias: "Nora"},
	})
	terminal := f.rememberGraphEntity(source, "Nora prefers baklava.", memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers", Subject: memory.EntitySelector{EntityID: bridge.Claim.ObjectEntityID},
		Object: memory.EntitySelector{Create: true, CanonicalName: "baklava", EntityType: "food", Alias: "baklava"},
	})
	f.refresh()
	f.lifecycle(source, "memory_retire", memory.SemanticObjectEntity, bridge.Claim.ObjectEntityID)
	client, _ := f.search(f.global(), "Maya")
	for _, item := range expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1]) {
		if item.ClaimID == bridge.Claim.ID || item.ClaimID == terminal.Claim.ID {
			t.Fatalf("a retired Entity remained a traversable relationship bridge: %+v", item)
		}
	}
}

func TestMemorySearchTurnExplicitTimeAddsApplicableEvidenceReasonWithoutInventingDates(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	from := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC)
	dated := f.rememberGraphEntity(source, "During 2025 Maya preferred baklava.", memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers", ValidTime: memory.ValidTime{From: &from, To: &until},
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "baklava", EntityType: "food", Alias: "baklava"},
	})
	unknown := f.rememberGraphEntity(source, "Maya knows Nora; the applicable dates are unknown.", memory.RememberEntityRequest{
		Predicate: "knows", PredicateLabel: "knows", Subject: memory.EntitySelector{EntityID: dated.Claim.SubjectEntityID},
		Object: memory.EntitySelector{Create: true, CanonicalName: "Nora", EntityType: "person", Alias: "Nora"},
	})
	f.refresh()
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("search", "memory_search", `{"query":"Maya","intent":"historical","valid_at":"2025-06-01T00:00:00Z"}`)),
		assistantStep("The dated preference applied in June 2025.", nil),
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Which Maya evidence applied in June 2025?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	var datedFound, unknownFound bool
	for _, item := range expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1]) {
		if item.ClaimID == dated.Claim.ID {
			datedFound = true
			if !slices.Contains(item.Paths, "temporal") || item.EffectiveValidTime == nil || item.EffectiveValidTime.From == nil || !item.EffectiveValidTime.From.Equal(from) {
				t.Fatalf("explicit time match lacks its applicable recorded-time reason: %+v", item)
			}
		}
		if item.ClaimID == unknown.Claim.ID {
			unknownFound = true
			if slices.Contains(item.Paths, "temporal") || item.EffectiveValidTime == nil || item.EffectiveValidTime.From != nil || item.EffectiveValidTime.To != nil {
				t.Fatalf("unknown validity acquired a temporal ranking or invented fact date: %+v", item)
			}
		}
	}
	if !datedFound || !unknownFound {
		t.Fatalf("explicit-time lookup omitted eligible dated or unknown-validity evidence: dated=%v unknown=%v", datedFound, unknownFound)
	}
}

func TestMemorySearchTurnGraphBudgetKeepsDifferentRelationships(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	bridge := f.rememberGraphEntity(source, "Maya's sister is Nora.", memory.RememberEntityRequest{
		Predicate: "sister", PredicateLabel: "sister",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Nora", EntityType: "person", Alias: "Nora"},
	})
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("pastry %02d", i)
		f.rememberGraphEntity(source, "Nora prefers "+name+".", memory.RememberEntityRequest{
			Predicate: "prefers", PredicateLabel: "prefers", Subject: memory.EntitySelector{EntityID: bridge.Claim.ObjectEntityID},
			Object: memory.EntitySelector{Create: true, CanonicalName: name, EntityType: "food", Alias: name},
		})
	}
	location := f.rememberGraphEntity(source, "Nora resides in London.", memory.RememberEntityRequest{
		Predicate: "resides_in", PredicateLabel: "resides in", Subject: memory.EntitySelector{EntityID: bridge.Claim.ObjectEntityID},
		Object: memory.EntitySelector{Create: true, CanonicalName: "London", EntityType: "place", Alias: "London"},
	})
	f.refresh()
	client, _ := f.search(f.global(), "Maya")
	evidence := expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1])
	locationFound := false
	preferences := 0
	for _, item := range evidence {
		locationFound = locationFound || item.ClaimID == location.Claim.ID
		if item.Claim != nil && item.Claim.Predicate.Token == "prefers" {
			preferences++
		}
	}
	if !locationFound || preferences == 0 || len(evidence) > 8 {
		t.Fatalf("the bounded result crowded out a distinct supported relationship: location=%v preferences=%d results=%d", locationFound, preferences, len(evidence))
	}
}

func TestMemorySearchTurnGraphDoesNotReuseSupportFromDifferentTemporalRead(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	bridge := f.rememberGraphEntity(source, "Maya's sister is Nora.", memory.RememberEntityRequest{
		Predicate: "sister", PredicateLabel: "sister",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Nora", EntityType: "person", Alias: "Nora"},
	})
	from, until := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	terminal := f.rememberGraphEntity(source, "During 2025 Nora preferred baklava.", memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers", ValidTime: memory.ValidTime{From: &from, To: &until},
		Subject: memory.EntitySelector{EntityID: bridge.Claim.ObjectEntityID},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "baklava", EntityType: "food", Alias: "baklava"},
	})
	f.refresh()
	targeted, _ := json.Marshal(map[string]string{"query": string(bridge.Claim.ID), "intent": "historical", "valid_at": "2026-06-01T00:00:00Z"})
	client := &fakeClient{steps: []step{
		assistantStep("", nil,
			toolCall("graph", "memory_search", `{"query":"Maya","intent":"historical","valid_at":"2025-06-01T00:00:00Z"}`),
			toolCall("targeted", "memory_search", string(targeted))),
		assistantStep("The relationship was reread for a different time; the earlier path needs a complete refresh.", nil),
	}}
	if err := f.session(f.global(), client).Send(context.Background(), "Look up Maya's 2025 relationship, then inspect its bridge for June 2026.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	request := client.reqs[len(client.reqs)-1]
	if !strings.Contains(expansionBoundaryToolResult(t, request, "graph"), `"matches":2`) {
		t.Fatal("fixture did not first retrieve a supported historical two-hop path")
	}
	bridgeFound := false
	for _, item := range expansionBoundaryEvidence(t, request) {
		if item.ClaimID == terminal.Claim.ID {
			t.Fatalf("graph terminal reused support from a different temporal read: %+v", item)
		}
		bridgeFound = bridgeFound || item.ClaimID == bridge.Claim.ID && item.ValidAt.Year() == 2026
	}
	if !bridgeFound {
		t.Fatal("discarding the old path also discarded the independently requested new bridge view")
	}
}
