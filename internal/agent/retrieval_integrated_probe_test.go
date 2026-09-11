package agent

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// integratedProbeQuery applies the public Kernel query limit to fixed resource
// probes. Reader-quality runs retain model-chosen queries, and the complete
// original question remains the public Session.Send input in every condition.
func integratedProbeQuery(question string) string {
	words, inside := 0, false
	for index, r := range question {
		word := unicode.IsLetter(r) || unicode.IsDigit(r)
		if index+utf8.RuneLen(r) > 1024 || word && !inside && words == 32 {
			return strings.TrimSpace(question[:index])
		}
		if word && !inside {
			words++
		}
		inside = word
	}
	return strings.TrimSpace(question)
}
