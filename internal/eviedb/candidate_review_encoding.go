package eviedb

import (
	"encoding/json"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

// The public structs use fixed JSON field order; source/candidate collections
// are sorted by their immutable IDs/locators before reaching this boundary.
// Null pointers explicitly retain unknown/absent values. Golden tests freeze
// this domain-separated encoding independently of rendered CLI text.
func ownerReviewEffectHash(effect *memory.ReviewEffect) (string, []byte, error) {
	return semanticHash(canonicalOwnerReviewEffect(effect))
}
func ownerReviewPreviewHash(preview memory.ReviewPreview) (string, []byte, error) {
	preview.SHA256 = ""
	return semanticHash(struct {
		Domain  string               `json:"domain"`
		Preview memory.ReviewPreview `json:"preview"`
	}{"evie-owner-review-preview-v1", preview})
}
func canonicalOwnerReviewOperation(op memory.OwnerReviewOperation) any {
	return struct {
		Domain    string                      `json:"domain"`
		Operation memory.OwnerReviewOperation `json:"operation"`
	}{"evie-owner-review-operation-v1", op}
}
func validateOwnerReviewEncoding(p memory.ReviewPreview) error {
	if p.Version != "owner-review-preview-v1" || p.Action != "accept" && p.Action != "reject" || len(p.Candidates) != 1 {
		return errors.New("invalid review preview")
	}
	if p.Action == "accept" && (p.Effect == nil || p.Effect.Version != "owner-review-effect-v1" || len(p.Effect.Claims) != len(p.Candidates)) {
		return errors.New("incomplete review effects")
	}
	if p.Action == "reject" && p.Effect != nil {
		return errors.New("reject preview contains effects")
	}
	hash, _, err := ownerReviewEffectHash(p.Effect)
	if err != nil {
		return err
	}
	if hash != p.EffectSHA256 {
		return errors.New("review effect digest changed")
	}
	hash, _, err = ownerReviewPreviewHash(p)
	if err != nil {
		return err
	}
	if hash != p.SHA256 {
		return errors.New("review preview digest changed")
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > 256*1024 {
		return errors.New("review_too_large")
	}
	return nil
}

func canonicalOwnerReviewEffect(effect *memory.ReviewEffect) any {
	return struct {
		Domain string               `json:"domain"`
		Effect *memory.ReviewEffect `json:"effect"`
	}{"evie-owner-review-effect-v1", effect}
}
