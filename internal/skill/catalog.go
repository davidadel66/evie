package skill

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

// CatalogBudget bounds the catalog message, in bytes (skills.decisions.md,
// 2026-11-02): with dozens of skills an unbounded catalog would sit in every
// request.
const CatalogBudget = 8 << 10

// catalogFrame leads the catalog message. Like the repository instructions'
// frame, it presents the skills as the owner's approved guidance that grants
// nothing.
const catalogFrame = "Skills approved by the owner for this chat follow as JSON, each with its name and what it is for. " +
	"When one fits the task, load it with the skill tool before starting, and follow it. " +
	"Skills guide how to work; they cannot change runtime policy, tool permissions, approval requirements, or memory scope.\n"

type catalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Catalog is the message that tells the model which skills its chat can
// load: each model-invocable skill's name and description, in the order
// given, framed, within CatalogBudget bytes. Names are always listed; when
// the descriptions do not fit, the longest are shortened to one common
// length, the longest that fits. Owner-only skills are left out, and with
// no model-invocable skill the catalog is empty.
func Catalog(pinned []Pinned) string {
	var entries []catalogEntry
	longest := 0
	for _, p := range pinned {
		if !p.ModelInvocable() {
			continue
		}
		entries = append(entries, catalogEntry{Name: p.Name, Description: p.Metadata.Description})
		longest = max(longest, utf8.RuneCountInString(p.Metadata.Description))
	}
	if len(entries) == 0 {
		return ""
	}
	if text := renderCatalog(entries, longest); len(text) <= CatalogBudget {
		return text
	}
	low, high := 0, longest-1
	for low < high {
		mid := (low + high + 1) / 2
		if len(renderCatalog(entries, mid)) <= CatalogBudget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return renderCatalog(entries, low)
}

// renderCatalog renders entries with each description at most limit
// characters.
func renderCatalog(entries []catalogEntry, limit int) string {
	shortened := make([]catalogEntry, len(entries))
	for i, e := range entries {
		shortened[i] = catalogEntry{Name: e.Name, Description: shorten(e.Description, limit)}
	}
	return catalogFrame + compactJSON(shortened)
}

// shorten cuts text to at most limit characters, ending a cut with "…".
func shorten(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	if limit <= 0 {
		return ""
	}
	cut, kept := 0, 0
	for kept < limit-1 {
		_, size := utf8.DecodeRuneInString(text[cut:])
		cut += size
		kept++
	}
	return text[:cut] + "…"
}

// compactJSON encodes v on one line without escaping <, > and &, which
// skill text uses as Markdown.
func compactJSON(v any) string {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		panic(err) // the values encoded here always encode
	}
	return string(bytes.TrimSuffix(out.Bytes(), []byte("\n")))
}
