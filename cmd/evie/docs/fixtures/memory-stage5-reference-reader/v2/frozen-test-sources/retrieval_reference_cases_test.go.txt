package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

var referenceCaseNames = []string{"mother_after_debugging", "mother_or_sister", "misleading_recent_sister", "compacted_mother", "compacted_ambiguous", "explicit_uuid", "opaque_alias", "no_evidence", "scope_excluded_sister"}

type referenceCase struct {
	name, question string
	reader         memory.Session
	required       []memory.RememberEntityProposal
	forbidden      []memory.EventID
	compaction     memory.EventID
	setup          map[string]any
}

func prepareReferenceCase(t *testing.T, f *retrievalFixture, name string) referenceCase {
	t.Helper()
	test := referenceCase{name: name, reader: f.global(), question: "What present would she like?", setup: make(map[string]any)}
	if name == "no_evidence" {
		return test
	}
	maya := referencePreference(f, f.global(), "Maya", "mother", "MOM-27", "jasmine tea")
	test.required = append(test.required, maya)
	switch name {
	case "explicit_uuid":
		test.question = "For " + string(maya.Claim.SubjectEntityID) + ", what present would she like?"
		return test
	case "opaque_alias":
		test.question = "For MOM-27, what present would she like?"
		return test
	}
	if name == "mother_or_sister" || name == "misleading_recent_sister" || name == "compacted_ambiguous" {
		nora := referencePreference(f, f.global(), "Nora", "sister", "SIS-42", "ceramic bowl")
		test.required = append(test.required, nora)
	}
	if name == "scope_excluded_sister" {
		project, err := f.store.RegisterProject(context.Background(), "Unshared", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		private, err := f.store.CreateProjectSession(context.Background(), project.ID)
		if err != nil {
			t.Fatal(err)
		}
		nora := referencePreference(f, private, "Nora", "sister", "SIS-42", "sapphire earrings")
		test.forbidden = append(test.forbidden, nora.Source.EventID)
		project, err = f.store.RegisterProject(context.Background(), "Reader area", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		test.reader, err = f.store.CreateProjectSession(context.Background(), project.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw := f.converse(f.global(), "Nora my sister wants sapphire earrings for the birthday.")
		test.forbidden = append(test.forbidden, raw.ID)
	}
	f.converse(test.reader, "The checksum parser still crashes.")
	f.converse(test.reader, "Database retries are under investigation.")
	topic := "Maya, my mother, has a birthday approaching. I want to choose her present."
	if name == "mother_or_sister" || name == "compacted_ambiguous" {
		topic = "Maya, my mother, and Nora, my sister, both have birthdays approaching. I have not decided whose present to choose first."
	}
	f.converse(test.reader, topic)
	for i := 0; i < 4; i++ {
		f.converse(test.reader, []string{"The checksum parser still crashes.", "Database retries are under investigation."}[i%2])
	}
	if name == "misleading_recent_sister" {
		f.converse(test.reader, "Nora, my sister, helped debug the database. The present we are choosing is still for my mother Maya.")
	}
	if name == "compacted_mother" || name == "compacted_ambiguous" {
		for i := 0; i < 20; i++ {
			f.converse(test.reader, fmt.Sprintf("Debug checksum routine number %02d.", i))
		}
		continuity := "Maya mother birthday present remains to choose; original preference needs retrieval."
		if name == "compacted_ambiguous" {
			continuity = "Maya mother Nora sister birthdays; recipient not yet chosen. Original preferences need retrieval."
		}
		summary := strings.Replace(validCompactionSummary(), "kept", continuity, 1)
		compactor := &fakeClient{steps: []step{assistantStep(summary, nil)}}
		holder := memory.LeaseHolderID("reference-compaction")
		session := NewWithCompactorAndToolset(nil, compactor, testContextProfile("test-model"), f.store.BindHistory(test.reader.ID, holder), test.reader.ScopeContext(), f.store.BindTurnOwner(test.reader.ID, holder), tools.NewToolset(nil), WithAutomaticMemoryRecall(false))
		result, err := session.Compact(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		test.compaction = result.CompactionEventID
		test.setup = map[string]any{"compaction_event_id": result.CompactionEventID, "scripted_compactor_requests": compactor.reqs, "summary": summary, "later_noise_roots": 20, "compactor_quality_evaluated": false}
	}
	return test
}

func checkReferenceEvidence(t *testing.T, test referenceCase, request openrouter.ChatRequest, first bool) {
	t.Helper()
	evidence := expansionBoundaryEvidence(t, request)
	found := make(map[memory.SemanticID]bool)
	wire, err := openrouter.RequestBytes(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range test.forbidden {
		if strings.Contains(string(wire), string(id)) {
			t.Fatal("reference hypothesis disclosed a forbidden original source")
		}
	}
	if strings.Contains(string(wire), "sapphire earrings") {
		t.Fatal("reference hypothesis disclosed excluded preference")
	}
	for _, item := range evidence {
		if item.Intent != memory.RetrievalCurrent || item.CurrentStatus != memory.SemanticStatusActive {
			t.Fatal("reference hypothesis used ineligible current evidence")
		}
		for _, original := range test.required {
			if item.ClaimID != original.Claim.ID {
				continue
			}
			for _, source := range item.Sources {
				if source.EventID == original.Source.EventID && source.Actor == memory.SemanticActorOwner && source.Authority == memory.AuthorityOwnerStatement && source.EvidenceSHA256 == original.Source.EvidenceSHA256 && source.Evidence == original.Source.Evidence {
					found[item.ClaimID] = true
				}
			}
		}
		for _, source := range item.Sources {
			if source.EventID == test.compaction {
				t.Fatal("compaction continuity was promoted to source evidence")
			}
		}
	}
	if first {
		for _, original := range test.required {
			if !found[original.Claim.ID] {
				t.Errorf("initial bounded interpretation omitted eligible candidate %s: %s", original.Claim.ID, retrievalData(t, request))
			}
		}
	}
}

func TestReferenceRecallCompactedCandidatesRemainOriginalEvidence(t *testing.T) {
	for _, name := range []string{"compacted_mother", "compacted_ambiguous"} {
		t.Run(name, func(t *testing.T) {
			f := newRetrievalFixture(t)
			test := prepareReferenceCase(t, f, name)
			f.refresh()
			client := &fakeClient{steps: []step{assistantStep("Candidate evidence checked; compactor and interpretation quality are not measured.", nil)}}
			if err := f.automaticSession(test.reader, client).Send(context.Background(), test.question, &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			checkReferenceEvidence(t, test, client.reqs[0], true)
		})
	}
}
