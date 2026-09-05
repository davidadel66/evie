package eviedb

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewTemporalEncodingGoldenAndOlderVersionBoundary(t *testing.T) {
	var fixture struct {
		EffectHash   string               `json:"effect_sha256"`
		EffectBytes  string               `json:"effect_canonical_utf8"`
		PreviewHash  string               `json:"preview_sha256"`
		PreviewBytes string               `json:"preview_canonical_utf8"`
		Preview      memory.ReviewPreview `json:"preview"`
	}
	raw, err := os.ReadFile("testdata/candidate_review_encoding_v3.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	hash, encoded, err := ownerReviewEffectHash(fixture.Preview.Effect)
	if err != nil || hash != fixture.EffectHash || string(encoded) != fixture.EffectBytes {
		t.Fatalf("v3 effect encoding changed: %s %v", hash, err)
	}
	hash, encoded, err = ownerReviewPreviewHash(fixture.Preview)
	if err != nil || hash != fixture.PreviewHash || string(encoded) != fixture.PreviewBytes {
		t.Fatalf("v3 preview encoding changed: %s %v", hash, err)
	}
	if err = validateOwnerReviewEncoding(fixture.Preview); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"v1", "v2"} {
		p := fixture.Preview
		// A fresh decode avoids aliasing the frozen fixture's effect.
		if err = json.Unmarshal(raw, &fixture); err != nil {
			t.Fatal(err)
		}
		p = fixture.Preview
		p.Version = "owner-review-preview-" + version
		p.Effect.Version = "owner-review-effect-" + version
		p.EffectSHA256, _, err = ownerReviewEffectHash(p.Effect)
		if err != nil {
			t.Fatal(err)
		}
		p.SHA256, _, err = ownerReviewPreviewHash(p)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateOwnerReviewEncoding(p); err == nil {
			t.Fatalf("temporal meaning admitted under %s", version)
		}
	}
}
