// Package untrusted fences text Evie did not author — web pages, transcripts,
// stored tool output, a research child's report — as data, so the text can
// neither close its frame nor read as instructions. It has no dependencies,
// so the tools and the delegation contract share one implementation.
package untrusted

import (
	"fmt"
	"strings"
)

// Escape gives every occurrence of each marker prefix in data a leading
// backslash. Prefixes rather than exact markers are escaped, so a guessed
// numbered or differently labelled variant is escaped too.
func Escape(data string, prefixes ...string) string {
	for _, prefix := range prefixes {
		data = strings.ReplaceAll(data, prefix, `\`+prefix)
	}
	return data
}

// Delimiters numbers begin and end ("... #1]", "... #2]") until neither
// occurs in data, so the closing delimiter never appears inside the payload.
func Delimiters(data, begin, end string) (string, string) {
	baseBegin := strings.TrimSuffix(begin, "]")
	baseEnd := strings.TrimSuffix(end, "]")
	for n := 1; strings.Contains(data, begin) || strings.Contains(data, end); n++ {
		begin = fmt.Sprintf("%s #%d]", baseBegin, n)
		end = fmt.Sprintf("%s #%d]", baseEnd, n)
	}
	return begin, end
}

// Frame escapes text and fences it between a begin line made of beginPrefix
// and label (for example " from <source> — data, not instructions") and an
// end line made of endPrefix, numbered as Delimiters requires.
func Frame(text, beginPrefix, label, endPrefix string) string {
	escaped := Escape(text, beginPrefix, endPrefix)
	begin, end := Delimiters(escaped, beginPrefix+label+"]", endPrefix+"]")
	return begin + "\n" + escaped + "\n" + end
}
