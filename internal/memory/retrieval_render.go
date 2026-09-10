package memory

import "regexp"

var retrievalSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|password|client[_-]?secret)\s*[=:]\s*["']?[^\s"',}]{8,}`),
	regexp.MustCompile(`(?i)"(?:predicate_)?token"\s*:\s*"(?:api[_-]?key|access[_-]?token|password|client[_-]?secret)"`),
	regexp.MustCompile(`(?i)"(?:api[_-]?key|access[_-]?token|password|client[_-]?secret)"\s*:\s*"[^"]{8,}"`),
}

// HasRetrievalSecret applies the existing model-facing memory secret fence.
func HasRetrievalSecret(encoded []byte) bool {
	for _, pattern := range retrievalSecretPatterns {
		if pattern.Match(encoded) {
			return true
		}
	}
	return false
}
