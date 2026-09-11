package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestReferenceRecallKeepsEarlierRecipientAcrossUnrelatedTopicRoots(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	preference := f.rememberGraphEntity(source, "My mother Maya is fond of jasmine tea.", memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "MOM-27"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "jasmine tea", EntityType: "food", Alias: "jasmine tea"},
	})
	for _, text := range []string{
		"The checksum parser still crashes.",
		"Database retries are under investigation.",
		"Maya, my mother, has a birthday approaching.",
		"The checksum parser still crashes.",
		"Database retries are under investigation.",
		"The checksum parser still crashes.",
		"Database retries are under investigation.",
	} {
		f.converse(reader, text)
	}
	f.refresh()
	before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{assistantStep("Candidate coverage checked; this scripted response does not assess interpretation quality.", nil)}}
	if err := f.automaticSession(reader, client).Send(context.Background(), "What present would she like?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, evidence := range expansionBoundaryEvidence(t, client.reqs[0]) {
		if evidence.ClaimID == preference.Claim.ID {
			found = len(evidence.Sources) == 1 && evidence.Sources[0].EventID == preference.Source.EventID && evidence.Sources[0].Authority == memory.AuthorityOwnerStatement
		}
	}
	if !found {
		t.Fatalf("earlier recipient was omitted from interpretation evidence after unrelated topics: %s", retrievalData(t, client.reqs[0]))
	}
	after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if err != nil || !reflect.DeepEqual(before.Claims, after.Claims) || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
		t.Fatalf("reference interpretation changed accepted identities or facts: %v", err)
	}
	events, err := f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory == nil || snapshot.Memory.Interpretation == nil {
			continue
		}
		diagnostics, _ := json.Marshal(snapshot.Memory.Interpretation)
		if strings.Contains(string(diagnostics), "Maya") || strings.Contains(string(diagnostics), "birthday") || strings.Contains(string(diagnostics), "jasmine") {
			t.Fatal("interpretation diagnostics copied conversation/source text")
		}
	}
}

func referencePreference(f *retrievalFixture, source memory.Session, name, relationship, alias, gift string) memory.RememberEntityProposal {
	f.t.Helper()
	return f.rememberGraphEntity(source, fmt.Sprintf("My %s %s is fond of %s.", relationship, name, gift), memory.RememberEntityRequest{
		Predicate: "prefers", PredicateLabel: "prefers",
		Subject: memory.EntitySelector{Create: true, CanonicalName: name, EntityType: "person", Alias: alias},
		Object:  memory.EntitySelector{Create: true, CanonicalName: gift, EntityType: "gift", Alias: gift},
	})
}

func TestReferenceRecallPreservesExactIdentitySelectors(t *testing.T) {
	for _, kind := range []string{"entity UUID", "opaque alias"} {
		t.Run(kind, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source, reader := f.global(), f.global()
			original := referencePreference(f, source, "Maya", "mother", "MOM-27", "jasmine tea")
			f.refresh()
			selector := string(original.Claim.SubjectEntityID)
			if kind == "opaque alias" {
				selector = "MOM-27"
			}
			client := &fakeClient{steps: []step{assistantStep("Exact identity evidence checked; no model quality claim.", nil)}}
			if err := f.automaticSession(reader, client).Send(context.Background(), "For "+selector+", what present would she like?", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, evidence := range expansionBoundaryEvidence(t, client.reqs[0]) {
				found = found || (evidence.ClaimID == original.Claim.ID && slices.Contains(evidence.Paths, "exact_or_alias"))
			}
			if !found {
				t.Fatalf("explicit %s lost the exact eligible identity lookup: %s", kind, retrievalData(t, client.reqs[0]))
			}
		})
	}
}

func TestReferenceRecallOpaqueNoiseDoesNotSuppressCurrentSubject(t *testing.T) {
	f := newRetrievalFixture(t)
	original := referencePreference(f, f.global(), "Maya", "mother", "MOM-27", "jasmine tea")
	reader := f.global()
	f.refresh()
	client := &fakeClient{steps: []step{assistantStep("Candidate contract checked.", nil)}}
	if err := f.automaticSession(reader, client).Send(context.Background(), "Ignore database error ERR-409. What present would Maya like?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range expansionBoundaryEvidence(t, client.reqs[0]) {
		found = found || item.ClaimID == original.Claim.ID
	}
	if !found {
		t.Fatal("an opaque diagnostic code suppressed the explicit natural subject")
	}
}

func TestReferenceRecallHypothesesCannotReviveRestrictedEvidence(t *testing.T) {
	for _, action := range []string{"retired", "retracted"} {
		t.Run(action, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source, reader := f.global(), f.global()
			original := referencePreference(f, source, "Maya", "mother", "MOM-27", "jasmine tea")
			f.refresh()
			if action == "retired" {
				f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, original.Claim.ID)
			} else {
				f.lifecycle(source, "memory_retract_source", memory.SemanticObjectSourceLink, original.Source.ID)
			}
			before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeClient{steps: []step{assistantStep("No current supported preference is available.", nil)}}
			if err := f.automaticSession(reader, client).Send(context.Background(), "For MOM-27, what present would Maya like?", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			data := retrievalData(t, client.reqs[0])
			if strings.Contains(data, string(original.Claim.ID)) || strings.Contains(data, string(original.Source.EventID)) || strings.Contains(data, "jasmine tea") {
				t.Fatal("reference hypothesis revived restricted original evidence")
			}
			after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
			if err != nil || !reflect.DeepEqual(before.Claims, after.Claims) || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
				t.Fatal("reference read changed lifecycle or acceptance")
			}
		})
	}
}
