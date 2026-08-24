package memory

import (
	"strings"
	"unicode"
)

const SessionTitleRuneLimit = 80

// NormalizeSessionTitle turns accepted user evidence into deterministic,
// terminal-safe single-line metadata. An empty result means no title evidence.
func NormalizeSessionTitle(input string) string {
	title := TerminalSafeLine(input)
	runes := []rune(title)
	if len(runes) > SessionTitleRuneLimit {
		runes = append(runes[:SessionTitleRuneLimit-1], '…')
	}
	return string(runes)
}

// TerminalSafeLine removes terminal-active code points and normalizes all
// Unicode spacing without applying a display-length policy.
func TerminalSafeLine(input string) string {
	input = strings.ToValidUTF8(input, "\uFFFD")
	runes := make([]rune, 0, len(input))
	spacePending := false
	for _, r := range input {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			continue
		}
		if unicode.IsSpace(r) {
			if len(runes) > 0 {
				spacePending = true
			}
			continue
		}
		if spacePending {
			runes = append(runes, ' ')
			spacePending = false
		}
		runes = append(runes, r)
	}

	return string(runes)
}
