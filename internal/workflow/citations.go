package workflow

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The deep-research template's integrity functions (Stage 14 sub-stage 10,
// 2026-10-04). Both are reviewed, pure and bounded like the rest of the
// registry; a change to their behavior ships as a new version.
//
// tally_verdicts@1 counts independent verifiers' votes on each claim. A
// supported or refuted vote counts only when its typed result names at
// least one evidence URL the harness saw that verifier fetch; otherwise it
// counts as insufficient (noted). A missing vote, from a verifier that
// failed, counts against. A claim is accepted with at least quorum
// supported votes, and its citable sources are exactly the fetched evidence
// URLs of its supported votes, in vote order: never a URL only the
// researcher, or nobody, fetched.
//
// check_citations@1 is the citation rule of the synthesized report:
//
//  1. The allowed sources are the citable sources of the accepted claims,
//     compared as exact strings after trimming spaces.
//  2. A citation in the report's list is kept only when its marker has the
//     form S<n> (n of one to three digits), appears once in the list, its
//     URL is an allowed source, and the title, summary or body cites it;
//     every other entry is removed and reported.
//  3. Each inline marker ([S1], or [S1, S2]) is kept only for kept
//     citations; others are deleted from the text with one space before
//     them, and a combined marker is rewritten as separate markers.
//  4. A markdown link or bare URL in the text whose URL is not an allowed
//     source loses its URL: a link keeps only its text, and a bare URL
//     becomes "[unverified link removed]". Links to allowed sources stay.
//  5. The output is the cleaned report with its kept citations (each with
//     the accepted claims whose sources include it), every removal with its
//     reason, the status "verified" when nothing was removed and
//     "corrected" otherwise, and which accepted claims the kept citations
//     cover and which none does.

var (
	verdictTally = []string{"supported", "refuted", "insufficient"}
	markerName   = regexp.MustCompile(`^S[0-9]{1,3}$`)
	// reportToken finds, in order: a markdown link, an inline citation
	// marker (one or several S<n>, comma separated) and a bare URL.
	reportToken = regexp.MustCompile(`\[([^\[\]\n]*)\]\((https?://[^\s()]+)\)|\[(S[0-9]{1,3}(?:[ \t]*,[ \t]*S[0-9]{1,3})*)\]|https?://[^\s<>()\[\]"'` + "`" + `]+`)
	markerSplit = regexp.MustCompile(`[ \t]*,[ \t]*`)
)

const unverifiedLink = "[unverified link removed]"

func tallyVerdicts() Function {
	params := []Param{
		{Name: "claims", Type: ArrayType(openObject(
			Field{Name: "id", Type: StringType()},
			Field{Name: "text", Type: StringType()},
			Field{Name: "angle_id", Type: StringType(), Optional: true},
		))},
		{Name: "votes", Type: ArrayType(openObject(
			Field{Name: "item", Type: openObject(Field{Name: "claim_id", Type: StringType()})},
			Field{Name: "output", Type: openObject(
				Field{Name: "sources", Type: ArrayType(openObject(Field{Name: "url", Type: StringType()}))},
				Field{Name: "result", Type: openObject(
					Field{Name: "verdict", Type: StringType()},
					Field{Name: "evidence", Type: ArrayType(StringType())},
				)},
			)},
		))},
		{Name: "quorum", Type: IntegerType(), Min: 1, Max: 7},
	}
	counted := func(withSources bool) *Type {
		fields := []Field{
			{Name: "id", Type: StringType()}, {Name: "angle_id", Type: StringType()}, {Name: "text", Type: StringType()},
			{Name: "supported", Type: IntegerType()}, {Name: "refuted", Type: IntegerType()}, {Name: "insufficient", Type: IntegerType()},
		}
		if withSources {
			fields = append(fields, Field{Name: "sources", Type: ArrayType(StringType())})
		}
		return ObjectType(fields...)
	}
	result := func(bound *int64) *Type {
		accepted, rejected := ArrayType(counted(true)), ArrayType(counted(false))
		accepted.MaxItems, rejected.MaxItems = bound, bound
		return ObjectType(
			Field{Name: "accepted", Type: accepted},
			Field{Name: "rejected", Type: rejected},
			Field{Name: "accepted_count", Type: IntegerType()},
			Field{Name: "rejected_count", Type: IntegerType()},
			Field{Name: "downgraded", Type: IntegerType()},
			Field{Name: "notes", Type: ArrayType(StringType())},
		)
	}
	return Function{
		Name: "tally_verdicts", Version: 1, Params: params, Result: result(nil),
		Summary: "accept a claim with at least quorum supported votes; a vote counts only with evidence its verifier fetched, a missing vote counts against, and an accepted claim cites only that evidence",
		Refine: func(args map[string]StaticArg) *Type {
			if claims := args["claims"].Type; claims != nil && claims.Kind == KindArray {
				return result(claims.MaxItems)
			}
			return result(nil)
		},
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			quorum := args["quorum"].(int64)
			claims := args["claims"].([]any)
			type tally struct {
				counts  map[string]int64
				sources []any
			}
			tallies := map[string]*tally{}
			for _, c := range claims {
				id, _ := walkValue(c, []string{"id"}).(string)
				tallies[id] = &tally{counts: map[string]int64{"supported": 0, "refuted": 0, "insufficient": 0}, sources: []any{}}
			}
			notes := []any{}
			downgraded := int64(0)
			for _, v := range args["votes"].([]any) {
				id, _ := walkValue(v, []string{"item", "claim_id"}).(string)
				t, ok := tallies[id]
				if !ok {
					return nil, fmt.Errorf("a vote names claim %q, which is not in claims", id)
				}
				verdict, _ := walkValue(v, []string{"output", "result", "verdict"}).(string)
				if !slices.Contains(verdictTally, verdict) {
					return nil, fmt.Errorf("vote for claim %s has verdict %q; expected %s", id, verdict, strings.Join(verdictTally, ", "))
				}
				fetched := map[string]bool{}
				sources, _ := walkValue(v, []string{"output", "sources"}).([]any)
				for _, s := range sources {
					if u, ok := walkValue(s, []string{"url"}).(string); ok {
						fetched[strings.TrimSpace(u)] = true
					}
				}
				var evidence []string
				named, _ := walkValue(v, []string{"output", "result", "evidence"}).([]any)
				for _, e := range named {
					if u, ok := e.(string); ok && fetched[strings.TrimSpace(u)] && !slices.Contains(evidence, strings.TrimSpace(u)) {
						evidence = append(evidence, strings.TrimSpace(u))
					}
				}
				if verdict != "insufficient" && len(evidence) == 0 {
					notes = append(notes, fmt.Sprintf("%s: a %s vote named no evidence URL its verifier fetched; counted insufficient", id, verdict))
					downgraded++
					verdict = "insufficient"
				}
				t.counts[verdict]++
				if verdict == "supported" {
					for _, u := range evidence {
						if !containsValue(t.sources, u) {
							t.sources = append(t.sources, u)
						}
					}
				}
			}
			accepted, rejected := []any{}, []any{}
			for _, c := range claims {
				id, _ := walkValue(c, []string{"id"}).(string)
				text, _ := walkValue(c, []string{"text"}).(string)
				angle, _ := walkValue(c, []string{"angle_id"}).(string)
				t := tallies[id]
				out := map[string]any{"id": id, "angle_id": angle, "text": text,
					"supported": t.counts["supported"], "refuted": t.counts["refuted"], "insufficient": t.counts["insufficient"]}
				if t.counts["supported"] >= quorum {
					out["sources"] = t.sources
					accepted = append(accepted, out)
				} else {
					rejected = append(rejected, out)
				}
			}
			return map[string]any{
				"accepted": accepted, "rejected": rejected, "accepted_count": int64(len(accepted)), "rejected_count": int64(len(rejected)),
				"downgraded": downgraded, "notes": notes,
			}, nil
		},
	}
}

func checkCitations() Function {
	params := []Param{
		{Name: "report", Type: openObject(
			Field{Name: "title", Type: StringType()},
			Field{Name: "summary", Type: StringType()},
			Field{Name: "body_markdown", Type: StringType()},
			Field{Name: "citations", Type: ArrayType(openObject(Field{Name: "marker", Type: StringType()}, Field{Name: "url", Type: StringType()}))},
		)},
		{Name: "claims", Type: ArrayType(openObject(
			Field{Name: "id", Type: StringType()},
			Field{Name: "sources", Type: ArrayType(StringType())},
		))},
	}
	result := ObjectType(
		Field{Name: "title", Type: StringType()},
		Field{Name: "summary", Type: StringType()},
		Field{Name: "body_markdown", Type: StringType()},
		Field{Name: "citations", Type: ArrayType(ObjectType(
			Field{Name: "marker", Type: StringType()}, Field{Name: "url", Type: StringType()}, Field{Name: "claim_ids", Type: ArrayType(StringType())},
		))},
		Field{Name: "removed", Type: ArrayType(ObjectType(
			Field{Name: "kind", Type: &Type{Kind: KindString, Enum: []any{"citation", "marker", "link"}}},
			Field{Name: "value", Type: StringType()}, Field{Name: "reason", Type: StringType()},
		))},
		Field{Name: "status", Type: &Type{Kind: KindString, Enum: []any{"verified", "corrected"}}},
		Field{Name: "cited_claims", Type: ArrayType(StringType())},
		Field{Name: "uncited_claims", Type: ArrayType(StringType())},
	)
	return Function{
		Name: "check_citations", Version: 1, Params: params, Result: result,
		Summary: "keep only report citations of sources a verifier fetched for an accepted claim; remove other citations, markers and links, and report each",
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			return citationCheck(args["report"].(map[string]any), args["claims"].([]any)), nil
		},
	}
}

// citationCheck applies the citation rule above.
func citationCheck(report map[string]any, claims []any) map[string]any {
	// 1. The allowed sources, each with the accepted claims it supports.
	supports := map[string][]any{}
	var claimIDs []string
	for _, c := range claims {
		id, _ := walkValue(c, []string{"id"}).(string)
		claimIDs = append(claimIDs, id)
		sources, _ := walkValue(c, []string{"sources"}).([]any)
		for _, s := range sources {
			if u, ok := s.(string); ok {
				u = strings.TrimSpace(u)
				if u != "" && !containsValue(supports[u], id) {
					supports[u] = append(supports[u], id)
				}
			}
		}
	}
	type entry struct {
		marker, url, problem string
	}
	removed := []any{}
	remove := func(kind, value, reason string) {
		r := map[string]any{"kind": kind, "value": value, "reason": reason}
		for _, earlier := range removed {
			m := earlier.(map[string]any)
			if m["kind"] == kind && m["value"] == value && m["reason"] == reason {
				return
			}
		}
		removed = append(removed, r)
	}
	// 2. The list, checked before the text.
	listed, _ := report["citations"].([]any)
	entries := make([]entry, 0, len(listed))
	valid := map[string]string{}
	known := map[string]bool{}
	for _, c := range listed {
		marker, _ := walkValue(c, []string{"marker"}).(string)
		u, _ := walkValue(c, []string{"url"}).(string)
		e := entry{marker: strings.TrimSpace(marker), url: strings.TrimSpace(u)}
		switch {
		case !markerName.MatchString(e.marker):
			e.problem = "malformed marker; use S1, S2, …"
		case known[e.marker]:
			e.problem = "duplicate marker"
		case len(supports[e.url]) == 0:
			e.problem = e.url + " is not a source a verifier fetched and relied on for an accepted claim"
		}
		if markerName.MatchString(e.marker) {
			known[e.marker] = true
		}
		if e.problem == "" {
			valid[e.marker] = e.url
		}
		entries = append(entries, e)
	}
	// 3 and 4. The text, cleaned in order; citations it uses are recorded.
	used := map[string]bool{}
	var textRemovals []func()
	clean := func(text string) string {
		var b strings.Builder
		last := 0
		for _, m := range reportToken.FindAllStringSubmatchIndex(text, -1) {
			b.WriteString(text[last:m[0]])
			last = m[1]
			token := text[m[0]:m[1]]
			switch {
			case m[4] >= 0: // a markdown link
				label, u := text[m[2]:m[3]], text[m[4]:m[5]]
				if len(supports[u]) > 0 {
					b.WriteString(token)
				} else {
					b.WriteString(label)
					textRemovals = append(textRemovals, func() { remove("link", u, "not a verified source") })
				}
			case m[6] >= 0: // a citation marker
				var kept []string
				for _, name := range markerSplit.Split(text[m[6]:m[7]], -1) {
					switch {
					case valid[name] != "":
						kept = append(kept, name)
						used[name] = true
					case known[name]:
						textRemovals = append(textRemovals, func() { remove("marker", name, "cites a removed citation") })
					default:
						textRemovals = append(textRemovals, func() { remove("marker", name, "cites no listed citation") })
					}
				}
				if len(kept) == 0 {
					// Delete the marker, with one space before it unless
					// another marker or a word follows directly.
					out := b.String()
					if strings.HasSuffix(out, " ") && (last == len(text) || strings.ContainsRune(" \t\r\n.,;:!?)", rune(text[last]))) {
						b.Reset()
						b.WriteString(strings.TrimSuffix(out, " "))
					}
					continue
				}
				for _, name := range kept {
					b.WriteString("[" + name + "]")
				}
			default: // a bare URL, without trailing punctuation
				u := strings.TrimRight(token, ".,;:!?*_")
				tail := token[len(u):]
				if len(supports[u]) > 0 {
					b.WriteString(token)
				} else {
					b.WriteString(unverifiedLink + tail)
					textRemovals = append(textRemovals, func() { remove("link", u, "not a verified source") })
				}
			}
		}
		b.WriteString(text[last:])
		return b.String()
	}
	title, _ := report["title"].(string)
	summary, _ := report["summary"].(string)
	body, _ := report["body_markdown"].(string)
	title, summary, body = clean(title), clean(summary), clean(body)
	// 2, continued: list problems in list order, then the text's.
	citations := []any{}
	cited := map[string]bool{}
	kept := map[string]bool{}
	for _, e := range entries {
		switch {
		case e.problem != "":
			remove("citation", e.marker, e.problem)
		case !used[e.marker]:
			remove("citation", e.marker, "never cited in the report")
		case kept[e.marker]:
		default:
			kept[e.marker] = true
			ids := supports[e.url]
			for _, id := range ids {
				cited[id.(string)] = true
			}
			citations = append(citations, map[string]any{"marker": e.marker, "url": e.url, "claim_ids": slices.Clone(ids)})
		}
	}
	for _, r := range textRemovals {
		r()
	}
	citedClaims, uncited := []any{}, []any{}
	for _, id := range claimIDs {
		if cited[id] {
			citedClaims = append(citedClaims, id)
		} else {
			uncited = append(uncited, id)
		}
	}
	status := "verified"
	if len(removed) > 0 {
		status = "corrected"
	}
	return map[string]any{
		"title": title, "summary": summary, "body_markdown": body, "citations": citations, "removed": removed,
		"status": status, "cited_claims": citedClaims, "uncited_claims": uncited,
	}
}
