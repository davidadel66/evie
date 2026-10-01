package agent

import (
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

// Each compaction generation rewrites the prior summary together with newly
// covered turns, so a section can silently collapse to a placeholder that
// discards accumulated continuity ("None", "Unchanged", "See prior
// summary"). carryForwardCompactionSections is the deterministic guard for a
// section that was substantive in the prior generation and is now only a
// placeholder: an empty answer is replaced by the prior section verbatim, and
// a back-reference is kept after it. Sections that can be legitimately
// resolved keep an explicit empty answer such as "None", but not a
// back-reference to a summary that no longer exists.
var resolvableCompactionSections = map[string]bool{
	"Unresolved questions / blockers / risks": true,
	"Next steps": true,
}

var emptyCompactionSectionTexts = map[string]bool{
	"none": true, "none yet": true, "none so far": true, "none recorded": true, "none noted": true,
	"none identified": true, "n/a": true, "na": true, "not applicable": true, "nothing": true,
	"nothing to report": true, "tbd": true, "unknown": true,
}

// referenceCompactionSectionPrefixes start text that points at earlier
// content instead of restating it. They match only short sections.
var referenceCompactionSectionPrefixes = []string{
	"unchanged", "no change", "same as", "see prior", "see previous", "see the prior", "see the previous",
	"see above", "see earlier", "as before", "as above", "as previously", "as in the prior", "as in the previous",
	"carried over", "carried forward", "refer to", "no new ", "nothing new", "no update",
}

const referenceCompactionSectionMaxBytes = 80

type compactionSectionKind int

const (
	compactionSectionSubstantive compactionSectionKind = iota
	compactionSectionEmpty
	compactionSectionReference
)

func classifyCompactionSection(body []string) compactionSectionKind {
	var parts []string
	for _, line := range body {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		line = strings.TrimSpace(strings.TrimLeft(line, "-*+•>"))
		if line != "" {
			parts = append(parts, line)
		}
	}
	text := strings.Join(strings.Fields(strings.ToLower(strings.Join(parts, " "))), " ")
	text = strings.Trim(text, "*_`()[]\"' .!;:")
	if text == "" || emptyCompactionSectionTexts[text] {
		return compactionSectionEmpty
	}
	if len(text) <= referenceCompactionSectionMaxBytes {
		for _, prefix := range referenceCompactionSectionPrefixes {
			if strings.HasPrefix(text, prefix) {
				return compactionSectionReference
			}
		}
	}
	return compactionSectionSubstantive
}

// compactionSectionLines splits a validated summary into lines and the line
// index of each required heading, in heading order.
func compactionSectionLines(summary string) ([]string, []int, error) {
	if err := validateCompactionSummary(summary); err != nil {
		return nil, nil, err
	}
	lines := strings.Split(summary, "\n")
	headings := memory.ContextCompactionSectionHeadings()
	positions := make([]int, len(headings))
	for i, heading := range headings {
		positions[i] = -1
		for index, line := range lines {
			if strings.TrimSuffix(line, "\r") == "## "+heading {
				positions[i] = index
				break
			}
		}
		if positions[i] < 0 {
			return nil, nil, fmt.Errorf("compaction summary is missing heading %q", heading)
		}
	}
	return lines, positions, nil
}

func compactionSectionBody(lines []string, positions []int, section int) []string {
	end := len(lines)
	if section+1 < len(positions) {
		end = positions[section+1]
	}
	return lines[positions[section]+1 : end]
}

// carryForwardCompactionSections applies the guard to a validated generated
// summary. A first generation has no prior summary and is unchanged. The
// result is validated again, so a carried section that pushes it past the
// size limit fails as an invalid summary rather than being truncated.
func carryForwardCompactionSections(prior, generated string) (string, error) {
	if prior == "" {
		return generated, nil
	}
	priorLines, priorPositions, err := compactionSectionLines(prior)
	if err != nil {
		return "", fmt.Errorf("prior compaction summary: %w", err)
	}
	lines, positions, err := compactionSectionLines(generated)
	if err != nil {
		return "", err
	}
	headings := memory.ContextCompactionSectionHeadings()
	carried := false
	out := append([]string(nil), lines[:positions[0]+1]...)
	for i, heading := range headings {
		body := compactionSectionBody(lines, positions, i)
		priorBody := compactionSectionBody(priorLines, priorPositions, i)
		if classifyCompactionSection(priorBody) == compactionSectionSubstantive {
			switch classifyCompactionSection(body) {
			case compactionSectionReference:
				// The generated text follows the restored section, so a short
				// line misread as a back-reference is still kept.
				restored := priorBody
				for len(restored) > 0 && strings.TrimSpace(restored[len(restored)-1]) == "" {
					restored = restored[:len(restored)-1]
				}
				body, carried = append(append([]string(nil), restored...), body...), true
			case compactionSectionEmpty:
				if !resolvableCompactionSections[heading] {
					body, carried = priorBody, true
				}
			}
		}
		out = append(out, body...)
		if i+1 < len(headings) {
			out = append(out, lines[positions[i+1]])
		}
	}
	if !carried {
		return generated, nil
	}
	result := strings.Join(out, "\n")
	if err := validateCompactionSummary(result); err != nil {
		return "", fmt.Errorf("carried-forward compaction summary: %w", err)
	}
	return result, nil
}
