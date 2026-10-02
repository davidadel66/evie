package eviedb_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// Confirmation review (defect 6): a compiler review edit's free-text reason
// is the owner's words in the Global review. Operation history shown to a
// reader outside that scope blanks it, like any other Global text that does
// not state the Claim's value; a Global reader still sees it.
func TestCompilerReviewReasonIsNotShownOutsideItsScope(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	selection := f.selection(t, "My HIV test was negative yesterday. I drink cocoa every morning.", true)
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		c := f.candidate(r)
		c.Proposition.Object.Literal.Value = "cocoa"
		return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
	}}
	compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selection, compilerGeneration(), extractor)
	if err != nil || len(compiled.Candidates) == 0 {
		t.Fatalf("compile: %+v %v", compiled, err)
	}
	review, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.store.InspectOwnerCandidate(ctx, review, candidateRef(compiled).ID)
	if err != nil {
		t.Fatal(err)
	}
	reason := "Keep it; my oncologist approved cocoa during chemo."
	edited, err := f.store.EditOwnerCandidate(ctx, review, memory.ReviewEditDecision{Candidate: current.Ref, Proposal: current.Candidate.Proposal, Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, review, edited.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, review, decisionFor(preview, "90000000-0000-4000-8000-000000000999"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation == nil || len(result.Operation.ClaimIDs) == 0 {
		t.Fatalf("review accepted no Claim: %+v", result)
	}
	project, err := f.store.RegisterProject(ctx, "Kitchen remodel", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := f.store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range result.Operation.ClaimIDs {
		inspected, err := f.store.InspectSemanticObject(ctx, reader.ScopeContext(), memory.SemanticObjectClaim, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(inspected.Operations) == 0 {
			t.Fatal("project reader lost the Claim's operation history")
		}
		encoded, _ := json.Marshal(inspected)
		for _, private := range []string{"HIV", "oncologist", "chemo"} {
			if strings.Contains(string(encoded), private) {
				t.Fatalf("project reader saw Global review text %q: %s", private, encoded)
			}
		}
		own, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectClaim, id)
		if err != nil {
			t.Fatal(err)
		}
		if encoded, _ := json.Marshal(own); !strings.Contains(string(encoded), "oncologist approved cocoa") {
			t.Fatalf("Global reader lost the review reason: %s", encoded)
		}
	}
}
