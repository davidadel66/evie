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
	}{reviewIdentityEncodingDomain(preview, "preview"), preview})
}
func canonicalOwnerReviewOperation(op memory.OwnerReviewOperation) any {
	return struct {
		Domain    string                      `json:"domain"`
		Operation memory.OwnerReviewOperation `json:"operation"`
	}{reviewIdentityEncodingDomain(op.Preview, "operation"), op}
}
func validateOwnerReviewEncoding(p memory.ReviewPreview) error {
	if p.Version == "owner-review-preview-v5" {
		if err := validateReviewCompoundEncoding(p); err != nil {
			return err
		}
		return validateReviewPreviewDigests(p)
	}
	if p.BatchID != "" || len(p.Dependencies) != 0 || p.Effect != nil && (len(p.Effect.Members) != 0 || len(p.Effect.Records) != 0 || len(p.Effect.Dependencies) != 0) {
		return ErrReviewDependencies
	}
	for _, candidate := range p.Candidates {
		if candidate.Edit != nil || candidate.Original != nil {
			return errors.New("older review version cannot carry owner edits")
		}
	}
	if (p.Version != "owner-review-preview-v1" && p.Version != "owner-review-preview-v2" && p.Version != "owner-review-preview-v3" && p.Version != "owner-review-preview-v4") || p.Action != "accept" && p.Action != "reject" || len(p.Candidates) != 1 {
		return errors.New("invalid review preview")
	}
	if p.Action == "accept" && (p.Effect == nil || p.Effect.Version != "owner-review-effect-"+p.Version[len("owner-review-preview-"):] || len(p.Effect.Claims) != len(p.Candidates)) {
		return errors.New("incomplete review effects")
	}
	if p.Action == "reject" && p.Effect != nil {
		return errors.New("reject preview contains effects")
	}
	if err := validateReviewClockEncoding(p); err != nil {
		return err
	}
	if err := validateReviewIdentityEffect(p); err != nil {
		return err
	}
	if err := validateReviewTemporalEncoding(p); err != nil {
		return err
	}
	return validateReviewPreviewDigests(p)
}
func validateReviewPreviewDigests(p memory.ReviewPreview) error {
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
	domain := "evie-owner-review-effect-v1"
	if effect != nil && effect.Version == "owner-review-effect-v2" {
		domain = "evie-owner-review-effect-v2"
	}
	if effect != nil && effect.Version == "owner-review-effect-v3" {
		domain = "evie-owner-review-effect-v3"
	}
	if effect != nil && effect.Version == "owner-review-effect-v4" {
		domain = "evie-owner-review-effect-v4"
	}
	if effect != nil && effect.Version == "owner-review-effect-v5" {
		domain = "evie-owner-review-effect-v5"
	}
	return struct {
		Domain string               `json:"domain"`
		Effect *memory.ReviewEffect `json:"effect"`
	}{domain, effect}
}
