package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestReferenceRecallSuppliesAcceptedAliasMappingAndRechecksLifecycle(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	original := referencePreference(f, source, "Maya", "mother", "MOM-27", "jasmine tea")
	f.refresh()
	var alias memory.SemanticAlias
	for _, item := range original.Aliases {
		if item.Value == "MOM-27" {
			alias = item
		}
	}
	if alias.ID == "" {
		t.Fatal("fixture lacks accepted alias")
	}
	client := &fakeClient{steps: []step{assistantStep("The accepted alias mapping supports Maya.", nil)}}
	if err := f.automaticSession(reader, client).Send(context.Background(), "For MOM-27, what present would she like?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	var supplied struct {
		Evidence []struct {
			ClaimID memory.SemanticID `json:"claim_id"`
			Matches []struct {
				Kind       string            `json:"kind"`
				EntityID   memory.SemanticID `json:"entity_id"`
				AliasID    memory.SemanticID `json:"alias_id"`
				AliasValue string            `json:"alias_value"`
			} `json:"identity_matches"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, client.reqs[0]), "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range supplied.Evidence {
		if item.ClaimID == original.Claim.ID {
			for _, match := range item.Matches {
				found = found || match.Kind == "alias" && match.EntityID == original.Claim.SubjectEntityID && match.AliasID == alias.ID && match.AliasValue == "MOM-27"
			}
		}
	}
	if !found {
		t.Fatal("exact alias search supplied no Kernel-proven alias-to-entity mapping to the actual reader")
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var refs []memory.RetrievalReference
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil {
			refs = snapshot.Memory.Evidence
			raw, _ := json.Marshal(refs)
			if strings.Contains(string(raw), "MOM-27") {
				t.Fatal("immutable receipt copied alias value instead of content-free identity reference")
			}
			if !strings.Contains(string(raw), string(alias.ID)) {
				t.Fatal("immutable receipt lost exact accepted alias ID")
			}
		}
	}
	f.lifecycle(source, "memory_retire", memory.SemanticObjectAlias, alias.ID)
	inspected, err := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), refs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(inspected)
	if strings.Contains(string(raw), "MOM-27") {
		t.Fatal("old receipt inspection revived retired alias mapping")
	}
	second := &fakeClient{steps: []step{assistantStep("The old alias does not establish current identity.", nil)}}
	if err := f.automaticSession(reader, second).Send(context.Background(), "For MOM-27, what would Maya like?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(retrievalData(t, second.reqs[0]), `"alias_value":"MOM-27"`) {
		t.Fatal("automatic reference recall revived retired alias")
	}
}

func TestReferenceRecallAliasOriginRestrictionDoesNotHideIndependentClaim(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	original := referencePreference(f, source, "Maya", "mother", "MOM-27", "jasmine tea")
	book := f.rememberGraphEntity(source, "Maya recommends the astronomy book.", memory.RememberEntityRequest{Predicate: "recommends", PredicateLabel: "recommends", Subject: memory.EntitySelector{EntityID: original.Claim.SubjectEntityID}, Object: memory.EntitySelector{Create: true, CanonicalName: "astronomy book", EntityType: "book", Alias: "astronomy book"}})
	f.refresh()
	client := &fakeClient{steps: []step{assistantStep("Accepted alias origin and independent recommendation checked.", nil)}}
	if err := f.automaticSession(reader, client).Send(context.Background(), "For MOM-27, remind me of recommendations.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	var ref memory.RetrievalReference
	for _, item := range expansionBoundaryEvidence(t, client.reqs[0]) {
		if item.ClaimID == book.Claim.ID {
			ref = item.Reference()
		}
	}
	if ref.ClaimID == "" || len(ref.IdentityMatches) != 1 || ref.IdentityMatches[0].Source == nil || ref.IdentityMatches[0].Source.EventID != original.Source.EventID {
		t.Fatal("alias mapping did not retain its independent original source")
	}
	f.lifecycle(source, "memory_retract_source", memory.SemanticObjectSourceLink, original.Source.ID)
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	inspected, err := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), []memory.RetrievalReference{ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspected) != 1 || !inspected[0].Available || inspected[0].Evidence == nil || inspected[0].Evidence.ClaimID != book.Claim.ID || len(inspected[0].Evidence.IdentityMatches) != 0 {
		t.Fatalf("alias source restriction changed independent Claim availability or revived mapping: %+v", inspected)
	}
	if len(inspected[0].Reference.IdentityMatches) != 1 || inspected[0].Reference.IdentityMatches[0].AliasID != ref.IdentityMatches[0].AliasID {
		t.Fatal("current access altered immutable alias reference")
	}
}
