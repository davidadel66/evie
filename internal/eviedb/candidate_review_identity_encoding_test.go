package eviedb

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewIdentityEncodingGoldenAndV1Boundary(t *testing.T) {
	var fixture struct {
		EffectHash   string               `json:"effect_sha256"`
		EffectBytes  string               `json:"effect_canonical_utf8"`
		PreviewHash  string               `json:"preview_sha256"`
		PreviewBytes string               `json:"preview_canonical_utf8"`
		Preview      memory.ReviewPreview `json:"preview"`
	}
	data, err := os.ReadFile("testdata/candidate_review_encoding_v2.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	hash, encoded, err := ownerReviewEffectHash(fixture.Preview.Effect)
	if err != nil || hash != fixture.EffectHash || string(encoded) != fixture.EffectBytes {
		t.Fatalf("v2 effect encoding changed: %s %v", hash, err)
	}
	hash, encoded, err = ownerReviewPreviewHash(fixture.Preview)
	if err != nil || hash != fixture.PreviewHash || string(encoded) != fixture.PreviewBytes {
		t.Fatalf("v2 preview encoding changed: %s %v", hash, err)
	}
	if err = validateOwnerReviewEncoding(fixture.Preview); err != nil {
		t.Fatal(err)
	}
	// Even recomputed hashes cannot admit new effects under the old contract.
	fixture.Preview.Version = "owner-review-preview-v1"
	fixture.Preview.Effect.Version = "owner-review-effect-v1"
	fixture.Preview.EffectSHA256, _, err = ownerReviewEffectHash(fixture.Preview.Effect)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Preview.SHA256, _, err = ownerReviewPreviewHash(fixture.Preview)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateOwnerReviewEncoding(fixture.Preview); err == nil {
		t.Fatal("identity effect admitted under v1 encoding")
	}
}
