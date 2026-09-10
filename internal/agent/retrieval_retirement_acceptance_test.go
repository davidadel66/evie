package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// The compiler is the external proposal seam. SQLite, candidate validation,
// owner review, accepted operations, and every reader turn remain real.
type retrievalRangeCompiler struct {
	proposals []memory.ExtractorCandidate
}

func (c retrievalRangeCompiler) ServerIdentity() string { return "scripted:retrieval-ranges-v1" }

func (c retrievalRangeCompiler) Extract(_ context.Context, _ memory.CompilerGeneration, request memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	encoded, err := json.Marshal(memory.CompilerResponse{RequestID: request.ID, Candidates: c.proposals})
	return eviedb.CompilerExtraction{Raw: encoded, ReleaseEvidence: "completed"}, err
}

func retrievalRangeGeneration() memory.CompilerGeneration {
	g := memory.CompilerGeneration{
		Version: "compiler-generation-v1", ModelArtifact: "scripted:retrieval-ranges-v1", ModelSHA256: strings.Repeat("1", 64),
		Quantization: "fixture", RuntimeVersion: "fixture", ProtocolVersion: "ollama-generate-v1", TokenizerSHA256: strings.Repeat("2", 64),
		Template: "{{.System}}\n{{.Prompt}}", Prompt: "Extract owner assertions only.", Schema: json.RawMessage(`{"type":"object"}`),
		TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8,
		Decoding:       memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17},
		EvidencePolicy: memory.CompilerPolicyVersion, SecretPolicy: memory.CompilerPolicyVersion, ClosurePolicy: memory.CompilerPolicyVersion,
		WindowPolicy: memory.CompilerPolicyVersion, PredicatePolicy: memory.CompilerPolicyVersion, EntityPolicy: memory.CompilerPolicyVersion,
		ValidationPolicy: memory.CompilerPolicyVersion, EquivalencePolicy: memory.CompilerPolicyVersion, EffectPolicy: memory.CompilerPolicyVersion,
	}
	g.ModelManifest = json.RawMessage(`{"layers":[{"mediaType":"application/vnd.ollama.image.model","digest":"sha256:` + g.ModelSHA256 + `"}]}`)
	g.ModelManifestSHA256 = memory.CompilerHash(g.ModelManifest)
	g.TemplateSHA256 = memory.CompilerHash([]byte(g.Template))
	return g
}

func (f *retrievalFixture) acceptRangeClaims(record memory.Session, event memory.Event, basis memory.RememberLiteralProposal, ranges [][2]string) []memory.SemanticID {
	f.t.Helper()
	ctx := context.Background()
	events, err := f.store.LoadEvents(ctx, record.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	var candidates []memory.ExtractorCandidate
	for _, item := range ranges {
		start := strings.Index(event.Content, item[1])
		if start < 0 {
			f.t.Fatalf("missing range fixture %q", item[1])
		}
		candidates = append(candidates, memory.ExtractorCandidate{
			Proposition: memory.ClaimProposition{SubjectEntityID: basis.Subject.ID, PredicateID: basis.Predicate.ID,
				Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: item[0]}}, Polarity: memory.PolarityAffirmed},
			Support: []memory.EvidenceLocator{{EventID: event.ID, EventPart: memory.EvidenceContent,
				LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: fmt.Sprintf("%d:%d", start, start+len(item[1])), EvidenceSHA256: memory.CompilerHash([]byte(item[1]))}},
			Context: []memory.EvidenceLocator{},
		})
	}
	selection := memory.CompilationSelection{SessionID: record.ID, RootID: event.ID, Cutoff: events[len(events)-1].Sequence, Destination: "global"}
	compiled, err := f.store.CompileCandidateUnit(ctx, record.ScopeContext(), selection, retrievalRangeGeneration(), retrievalRangeCompiler{proposals: candidates})
	if err != nil || compiled.State != "completed_candidates" || len(compiled.Candidates) != len(ranges) {
		f.t.Fatalf("range compilation = %+v: %v", compiled, err)
	}
	owner, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		f.t.Fatal(err)
	}
	byLiteral := make(map[string]memory.SemanticID)
	for _, candidate := range compiled.Candidates {
		preview, err := f.store.PrepareOwnerCandidateReview(ctx, owner, memory.CandidateRef{ID: candidate.ID, ReviewRevision: candidate.ReviewRevision}, "accept")
		if err != nil {
			f.t.Fatal(err)
		}
		accepted, err := f.store.ResolveOwnerCandidateReview(ctx, owner, memory.ReviewDecision{
			DeliveryKey: "idem:v1:" + uuid.NewString(), PreviewID: preview.ID, PreviewSHA256: preview.SHA256, Action: preview.Action,
		})
		if err != nil || accepted.Operation == nil || len(accepted.Operation.ClaimIDs) != 1 || len(accepted.Operation.SourceLinkIDs) != 1 {
			f.t.Fatalf("range acceptance = %+v: %v", accepted, err)
		}
		source, err := f.store.InspectSemanticObject(ctx, record.ScopeContext(), memory.SemanticObjectSourceLink, accepted.Operation.SourceLinkIDs[0])
		if err != nil || source.Source == nil || source.Source.LocatorKind != memory.LocatorUTF8ByteRange || source.Source.LocatorValue != candidate.Proposal.Support[0].LocatorValue || source.Source.EvidenceSHA256 != "sha256:"+candidate.Proposal.Support[0].EvidenceSHA256 {
			f.t.Fatalf("accepted range lost its exact source: %+v: %v", source.Source, err)
		}
		byLiteral[candidate.Proposal.Proposition.Object.Literal.Value] = accepted.Operation.ClaimIDs[0]
	}
	ids := make([]memory.SemanticID, len(ranges))
	for i, item := range ranges {
		ids[i] = byLiteral[item[0]]
	}
	return ids
}

func TestConversationSearchRetirementSubtractsSharedAndOverlappingUTF8Ranges(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	basis := f.remember(source, memory.MemoryEverywhere, "range fixture baseline")
	event := f.converse(source, "My café keepsake marker is sapphire; my garden marker is marigold.")
	claims := f.acceptRangeClaims(source, event, basis, [][2]string{
		{"sapphire", "café keepsake marker is sapphire"},
		{"café", "café keepsake marker is sapphire"},
		{"garden", "sapphire; my garden"},
	})
	f.refresh()

	assertExcerpt := func(query, required string, forbidden ...string) {
		t.Helper()
		client := f.searchConversations(f.global(), query)
		data := retrievalData(t, client.reqs[1])
		var result struct {
			Evidence []memory.RetrievalEvidence `json:"evidence"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "EVIE_MEMORY_DATA\n")), &result); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, evidence := range result.Evidence {
			for _, ref := range evidence.Sources {
				if ref.EventID == event.ID {
					found = found || strings.Contains(evidence.Text, required)
					var start, end int
					_, locatorErr := fmt.Sscanf(ref.LocatorValue, "%d:%d", &start, &end)
					if locatorErr != nil || ref.LocatorKind != memory.LocatorUTF8ByteRange || ref.LocatorValue != fmt.Sprintf("%d:%d", start, end) || start < 0 || end > len(event.Content) || start >= end {
						t.Fatalf("excerpt lacks canonical original byte range: %s", data)
					}
					if evidence.Kind != memory.RetrievalConversationExcerpt || evidence.ClaimID != "" || !utf8.ValidString(evidence.Text) || evidence.Text != event.Content[start:end] || ref.Evidence != evidence.Text || ref.EvidenceSHA256 != memory.CompilerHash([]byte(evidence.Text)) || ref.SessionID != source.ID || ref.Authority != memory.AuthorityOwnerStatement {
						t.Fatalf("excerpt lost original UTF-8 text, source hash, or conversation authority: %s", data)
					}
				}
			}
		}
		if !found {
			t.Fatalf("independently eligible original passage %q missing: %s", required, data)
		}
		for _, text := range forbidden {
			if strings.Contains(data, text) {
				t.Fatalf("retired source interval %q reached provider: %s", text, data)
			}
		}
	}

	assertExcerpt("marigold", event.Content)
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[0])
	assertExcerpt("marigold", "marigold", "café", "sapphire")
	// Another active Claim on the exact passage does not cancel retirement.
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[1])
	f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, claims[0])
	assertExcerpt("marigold", "marigold", "café", "sapphire")
	f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, claims[1])
	assertExcerpt("marigold", event.Content)

	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[0])
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, claims[2])
	assertExcerpt("marigold", "marigold", "café", "sapphire", "garden")
	f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, claims[0])
	assertExcerpt("café", "café", "sapphire", "garden")
	f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, claims[2])
	assertExcerpt("marigold", event.Content)
}
