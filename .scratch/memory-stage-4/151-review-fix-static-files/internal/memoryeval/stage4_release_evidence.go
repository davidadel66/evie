package memoryeval

import (
	"crypto/sha256"
	"fmt"
	"slices"
)

// Stage4EvidenceVerification can only be obtained by hashing the supplied
// artifact bytes. Its private digest binds verification to this submission.
// Corpus/gold/model identities stay opaque and are never evidence-file inputs.
type Stage4EvidenceVerification struct{ submissionSHA256 string }

func Stage4RequiredEvidence(s Stage4Submission) []string {
	hashes := []string{}
	add := func(hash string) {
		if hash != "" && !slices.Contains(hashes, hash) {
			hashes = append(hashes, hash)
		}
	}
	for _, a := range s.Approvals {
		add(a.EvidenceSHA256)
	}
	if s.Custody != nil {
		add(s.Custody.RecordSHA256)
	}
	if s.Execution != nil {
		e := s.Execution
		add(e.InputManifestSHA256)
		add(e.AdjudicationSHA256)
		for _, run := range e.Runs {
			add(run.RawOutputSHA256)
			add(run.RetainedOutputSHA256)
		}
		for _, check := range e.Conformance {
			add(check.ArtifactSHA256)
		}
		for _, m := range e.Measurements {
			add(m.ArtifactSHA256)
		}
		for _, review := range e.ReviewOutcomes {
			add(review.ArtifactSHA256)
		}
	}
	slices.Sort(hashes)
	return hashes
}

func VerifyStage4Evidence(s Stage4Submission, artifacts map[string][]byte) (Stage4EvidenceVerification, error) {
	protected := []string{}
	if p := s.Plan; p != nil {
		protected = []string{p.CorpusSHA256, p.GoldSHA256, p.Configuration.ModelSHA256, p.BaselineConfiguration.ModelSHA256}
	}
	required := Stage4RequiredEvidence(s)
	if len(artifacts) != len(required) {
		return Stage4EvidenceVerification{}, fmt.Errorf("artifact inventory must contain exactly the referenced receipt and output hashes")
	}
	for _, hash := range required {
		if slices.Contains(protected, hash) {
			return Stage4EvidenceVerification{}, fmt.Errorf("corpus, gold, and model artifacts must remain outside the receipt verifier")
		}
		data, ok := artifacts[hash]
		if !ok || !canonicalSHA256.MatchString(hash) || fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != hash {
			return Stage4EvidenceVerification{}, fmt.Errorf("referenced artifact bytes are missing or have a mismatched SHA-256")
		}
	}
	return Stage4EvidenceVerification{submissionSHA256: Stage4Digest(s)}, nil
}
