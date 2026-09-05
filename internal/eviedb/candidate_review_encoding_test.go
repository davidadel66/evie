package eviedb

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewEncodingGolden(t *testing.T) {
	var fixture struct {
		EffectHash   string               `json:"effect_sha256"`
		EffectBytes  string               `json:"effect_canonical_utf8"`
		PreviewHash  string               `json:"preview_sha256"`
		PreviewBytes string               `json:"preview_canonical_utf8"`
		Preview      memory.ReviewPreview `json:"preview"`
	}
	data, err := os.ReadFile("testdata/candidate_review_encoding_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	hash, encoded, err := ownerReviewEffectHash(fixture.Preview.Effect)
	if err != nil || hash != fixture.EffectHash || string(encoded) != fixture.EffectBytes {
		t.Fatalf("effect encoding changed: %s\n%s\n%v", hash, encoded, err)
	}
	hash, encoded, err = ownerReviewPreviewHash(fixture.Preview)
	if err != nil || hash != fixture.PreviewHash || string(encoded) != fixture.PreviewBytes {
		t.Fatalf("preview encoding changed: %s\n%s\n%v", hash, encoded, err)
	}
	if err = validateOwnerReviewEncoding(fixture.Preview); err != nil {
		t.Fatal(err)
	}
	// Approval identity includes source bytes and scope, independent of renderer
	// whitespace, and never treats an unknown Valid Time as an invented date.
	source := &fixture.Preview.Effect.Claims[0].Sources[0]
	source.Evidence = "cafe"
	changed, _, err := ownerReviewEffectHash(fixture.Preview.Effect)
	if err != nil || changed == fixture.EffectHash {
		t.Fatal("different evidence had same effect identity")
	}
	if fixture.Preview.Effect.Claims[0].Claim.ValidTime.From != nil || fixture.Preview.Effect.Claims[0].Claim.ValidTime.To != nil {
		t.Fatal("unknown bounds changed")
	}
}
