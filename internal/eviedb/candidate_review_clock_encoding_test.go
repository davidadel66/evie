package eviedb

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

type clockEncodingFixture struct {
	EffectHash   string               `json:"effect_sha256"`
	EffectBytes  string               `json:"effect_canonical_utf8"`
	PreviewHash  string               `json:"preview_sha256"`
	PreviewBytes string               `json:"preview_canonical_utf8"`
	Preview      memory.ReviewPreview `json:"preview"`
}

func TestOwnerReviewClockEncodingGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/candidate_review_encoding_v4.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture clockEncodingFixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	hash, raw, err := ownerReviewEffectHash(fixture.Preview.Effect)
	if err != nil || hash != fixture.EffectHash || string(raw) != fixture.EffectBytes {
		t.Fatal("v4 effect encoding changed")
	}
	hash, raw, err = ownerReviewPreviewHash(fixture.Preview)
	if err != nil || hash != fixture.PreviewHash || string(raw) != fixture.PreviewBytes {
		t.Fatal("v4 preview encoding changed")
	}
	if err = validateOwnerReviewEncoding(fixture.Preview); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"old version", "contract", "authority", "range", "ancestry hash"} {
		t.Run(mutation, func(t *testing.T) {
			var f clockEncodingFixture
			json.Unmarshal(data, &f)
			source := &f.Preview.Candidates[0].Candidate.Support[1]
			switch mutation {
			case "old version":
				f.Preview.Version = "owner-review-preview-v1"
				f.Preview.Effect.Version = "owner-review-effect-v1"
			case "contract":
				source.Observation.Contract = "unknown-clock-v2"
			case "authority":
				source.Actor = memory.SemanticActorOwner
				source.Authority = memory.AuthorityOwnerStatement
			case "range":
				source.Locator.LocatorValue = "0:4"
				source.Evidence = "2026"
				source.Locator.EvidenceSHA256 = memory.CompilerHash([]byte(source.Evidence))
			case "ancestry hash":
				source.Observation.AncestrySHA256 = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
			}
			f.Preview.EffectSHA256, _, _ = ownerReviewEffectHash(f.Preview.Effect)
			f.Preview.SHA256, _, _ = ownerReviewPreviewHash(f.Preview)
			if err := validateOwnerReviewEncoding(f.Preview); err == nil {
				t.Fatal("malformed recorded clock contract admitted")
			}
		})
	}
}
