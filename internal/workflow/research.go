package workflow

import (
	"cmp"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode"
)

// The built-in deep-research v2's deterministic steps (2026-10-04). They
// follow the deep-research harness Claude Code used to plan Evie's memory
// architecture (scope, search, deduplicate under a budget, extract, rank,
// adversarial three-vote verification, synthesis) and keep Evie's rule that
// only evidence the harness saw a worker fetch counts. Each is pure,
// typed, bounded and versioned like the rest of the registry: a change to
// its behavior ships as a new version.
//
// Source keys. A URL is a usable source only when it is http or https
// (any case), at most 500 bytes, and holds no whitespace (after trimming
// its ends), control or format character, or backslash. Its key is the
// host in lower case without userinfo, port or a leading "www.", then the
// path without trailing slashes (its case kept), then "?" and the query
// when there is one; the fragment and the scheme are ignored, so http and
// https spellings of one page share a key.
//
// dedup_sources@1 orders every search result of every angle by relevance
// (high, medium, low), then angle, then position, so the answer never
// depends on which search finished first; drops invalid URLs; drops each
// result whose key an earlier one has (the first occurrence wins); and
// keeps the first max_sources, dropping the rest as over the budget.
//
// rank_claims@1 reads the extraction map's output. A source is read only
// when its extractor fetched its URL (a fetched URL with the same key, as
// the harness recorded it); the claims of an unread source are dropped. The
// rest are ordered by importance (central, supporting, tangential), then
// source quality (primary, secondary, blog, forum, unreliable), then
// extraction and claim order, and the first max_claims are kept, numbered
// c1, c2, ... in that order.
//
// expand_ballots@2 gives each claim one ballot per voter, carrying the
// claim, its quote, its source and the source's quality and date.
//
// tally_adversarial@1 counts each claim's votes. A vote is valid only when
// one of its evidence URLs has the key of a URL the harness recorded that
// verifier fetching; an invalid vote, and a missing one (its verifier
// failed), is not cast. A claim is refuted with at least
// refutations_required refuting valid votes; confirmed with at least
// min_valid_votes valid votes and fewer refutations than that; otherwise
// unverified (verification failed, which is not a finding). A confirmed
// claim's confidence starts high for a unanimous vote with every voter
// valid and medium otherwise; a secondary source caps it at medium and a
// blog, forum or unreliable one at low; then, when no surviving vote rated
// its own confidence high, it drops one level (never below low). Its
// citable sources are the fetched evidence URLs of its surviving votes, as
// the harness recorded them, in vote order and one per key, then its own
// source URL when a surviving voter fetched it: never a URL nobody
// fetched. A refuted claim keeps its first refuting vote's evidence and
// that voter's counter source only when the voter fetched it. The outcome
// and its summary say honestly whether claims survived, were all refuted,
// could not be verified, or never existed, and the inconclusive report says
// so without writing any URL of its own.

const maxSourceURLBytes = 500

var (
	relevanceRank  = map[string]int{"high": 0, "medium": 1, "low": 2}
	importanceRank = map[string]int{"central": 0, "supporting": 1, "tangential": 2}
	qualityRank    = map[string]int{"primary": 0, "secondary": 1, "blog": 2, "forum": 3, "unreliable": 4}
	confidenceRank = map[string]int{"high": 0, "medium": 1, "low": 2}
	confidences    = []string{"high", "medium", "low"}
	outcomes       = []any{"confirmed", "all_refuted", "verifiers_failed", "refuted_and_unverified", "no_claims"}
)

// rank is a value's position in an ordering; unknown values sort last.
func rank(order map[string]int, value string) int {
	if r, ok := order[value]; ok {
		return r
	}
	return len(order)
}

// sourceKey is a URL's source key, or false when the URL is not a usable
// source.
func sourceKey(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxSourceURLBytes {
		return "", false
	}
	for _, r := range raw {
		if r == '\\' || unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return "", false
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if scheme := strings.ToLower(u.Scheme); scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	if host == "" || u.Opaque != "" {
		return "", false
	}
	key := host + strings.TrimRight(u.EscapedPath(), "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key, true
}

// fetchedKeys maps the key of each URL a worker's output records it
// fetching to the URL as the harness recorded it (the first one per key).
func fetchedKeys(output any) map[string]string {
	out := map[string]string{}
	sources, _ := walkValue(output, []string{"sources"}).([]any)
	for _, s := range sources {
		u, _ := walkValue(s, []string{"url"}).(string)
		if key, ok := sourceKey(u); ok {
			if _, seen := out[key]; !seen {
				out[key] = strings.TrimSpace(u)
			}
		}
	}
	return out
}

func stringField(v any, path ...string) string {
	s, _ := walkValue(v, path).(string)
	return s
}

func intField(v any, path ...string) int64 {
	n, _ := walkValue(v, path).(int64)
	return n
}

// webLine is web text for one markdown line: whitespace collapsed, control
// and format characters removed.
func webLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// count names a number of things: "1 claim", "2 claims".
func count(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func enumType(values ...any) *Type { return &Type{Kind: KindString, Enum: values} }

func boundedList(items *Type, bound *int64) *Type {
	t := ArrayType(items)
	if bound != nil {
		b := *bound
		t.MaxItems = &b
	}
	return t
}

func literalBound(a StaticArg) *int64 {
	if !a.IsConst {
		return nil
	}
	if n, ok := a.Literal.(int64); ok {
		return &n
	}
	return nil
}

func listBound(a StaticArg) *int64 {
	if a.Type != nil && a.Type.Kind == KindArray {
		return a.Type.MaxItems
	}
	return nil
}

func dedupSources() Function {
	params := []Param{
		{Name: "results", Type: ArrayType(openObject(
			Field{Name: "item", Type: openObject(Field{Name: "label", Type: StringType()})},
			Field{Name: "output", Type: openObject(Field{Name: "result", Type: openObject(Field{Name: "results", Type: ArrayType(openObject(
				Field{Name: "url", Type: StringType()},
				Field{Name: "title", Type: StringType()},
				Field{Name: "snippet", Type: StringType(), Optional: true},
				Field{Name: "relevance", Type: StringType()},
			))})})},
		))},
		{Name: "max_sources", Type: IntegerType(), Min: 1, Max: 100},
	}
	source := ObjectType(
		Field{Name: "id", Type: StringType()}, Field{Name: "url", Type: StringType()}, Field{Name: "key", Type: StringType()},
		Field{Name: "title", Type: StringType()}, Field{Name: "snippet", Type: StringType()}, Field{Name: "relevance", Type: StringType()},
		Field{Name: "angle", Type: StringType()},
	)
	result := func(bound *int64) *Type {
		return ObjectType(
			Field{Name: "sources", Type: boundedList(source, bound)},
			Field{Name: "count", Type: IntegerType()},
			Field{Name: "candidates", Type: IntegerType()},
			Field{Name: "duplicates", Type: IntegerType()},
			Field{Name: "budget_dropped", Type: IntegerType()},
			Field{Name: "invalid", Type: IntegerType()},
			Field{Name: "dropped", Type: ArrayType(ObjectType(
				Field{Name: "url", Type: StringType()}, Field{Name: "angle", Type: StringType()}, Field{Name: "reason", Type: StringType()},
			))},
		)
	}
	return Function{
		Name: "dedup_sources", Version: 1, Params: params, Result: result(nil),
		Summary: "order every search result by relevance, angle and position; drop invalid URLs and duplicates by normalized URL; keep the first max_sources",
		Refine:  func(args map[string]StaticArg) *Type { return result(literalBound(args["max_sources"])) },
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			limit := args["max_sources"].(int64)
			type candidate struct {
				angle, position int
				hit             any
				label           string
			}
			var candidates []candidate
			for a, r := range args["results"].([]any) {
				label := stringField(r, "item", "label")
				hits, _ := walkValue(r, []string{"output", "result", "results"}).([]any)
				for p, h := range hits {
					candidates = append(candidates, candidate{angle: a, position: p, hit: h, label: label})
				}
			}
			slices.SortStableFunc(candidates, func(x, y candidate) int {
				return cmp.Or(cmp.Compare(rank(relevanceRank, stringField(x.hit, "relevance")), rank(relevanceRank, stringField(y.hit, "relevance"))),
					cmp.Compare(x.angle, y.angle), cmp.Compare(x.position, y.position))
			})
			sources, dropped := []any{}, []any{}
			kept, overBudget := map[string]string{}, map[string]bool{}
			var duplicates, budget, invalid int64
			drop := func(u, angle, reason string) {
				dropped = append(dropped, map[string]any{"url": u, "angle": angle, "reason": reason})
			}
			for _, c := range candidates {
				u := strings.TrimSpace(stringField(c.hit, "url"))
				key, ok := sourceKey(u)
				if !ok {
					invalid++
					drop(u, c.label, "invalid URL")
					continue
				}
				if id, ok := kept[key]; ok {
					duplicates++
					drop(u, c.label, "duplicate of "+id)
					continue
				}
				if overBudget[key] {
					duplicates++
					drop(u, c.label, "duplicate of a result over the source budget")
					continue
				}
				if int64(len(sources)) >= limit {
					budget++
					overBudget[key] = true
					drop(u, c.label, "over the source budget")
					continue
				}
				id := fmt.Sprintf("s%d", len(sources)+1)
				kept[key] = id
				sources = append(sources, map[string]any{
					"id": id, "url": u, "key": key, "title": stringField(c.hit, "title"), "snippet": stringField(c.hit, "snippet"),
					"relevance": stringField(c.hit, "relevance"), "angle": c.label,
				})
			}
			return map[string]any{
				"sources": sources, "count": int64(len(sources)), "candidates": int64(len(candidates)), "duplicates": duplicates,
				"budget_dropped": budget, "invalid": invalid, "dropped": dropped,
			}, nil
		},
	}
}

// rankedClaimType is one claim rank_claims@1 keeps.
func rankedClaimType() *Type {
	return ObjectType(
		Field{Name: "id", Type: StringType()}, Field{Name: "text", Type: StringType()}, Field{Name: "quote", Type: StringType()},
		Field{Name: "importance", Type: StringType()}, Field{Name: "source_id", Type: StringType()}, Field{Name: "source_url", Type: StringType()},
		Field{Name: "source_title", Type: StringType()}, Field{Name: "source_quality", Type: StringType()}, Field{Name: "publish_date", Type: StringType()},
		Field{Name: "angle", Type: StringType()},
	)
}

func rankClaims() Function {
	source := openObject(Field{Name: "id", Type: StringType()}, Field{Name: "url", Type: StringType()},
		Field{Name: "title", Type: StringType(), Optional: true}, Field{Name: "angle", Type: StringType(), Optional: true})
	params := []Param{
		{Name: "extractions", Type: openObject(
			Field{Name: "results", Type: ArrayType(openObject(
				Field{Name: "index", Type: IntegerType()},
				Field{Name: "item", Type: source},
				Field{Name: "output", Type: openObject(
					Field{Name: "sources", Type: ArrayType(openObject(Field{Name: "url", Type: StringType()}))},
					Field{Name: "result", Type: openObject(
						Field{Name: "source_quality", Type: StringType()},
						Field{Name: "publish_date", Type: StringType(), Optional: true},
						Field{Name: "claims", Type: ArrayType(openObject(
							Field{Name: "claim", Type: StringType()},
							Field{Name: "quote", Type: StringType()},
							Field{Name: "importance", Type: StringType()},
						))},
					)},
				)},
			))},
			Field{Name: "failures", Type: ArrayType(openObject(Field{Name: "index", Type: IntegerType()}, Field{Name: "item", Type: source}))},
			Field{Name: "failed", Type: IntegerType()},
		)},
		{Name: "max_claims", Type: IntegerType(), Min: 1, Max: 100},
	}
	result := func(bound *int64) *Type {
		return ObjectType(
			Field{Name: "claims", Type: boundedList(rankedClaimType(), bound)},
			Field{Name: "count", Type: IntegerType()},
			Field{Name: "extracted", Type: IntegerType()},
			Field{Name: "unfetched", Type: IntegerType()},
			Field{Name: "dropped", Type: IntegerType()},
			Field{Name: "sources_read", Type: IntegerType()},
			Field{Name: "extraction_failed", Type: IntegerType()},
			Field{Name: "sources", Type: ArrayType(ObjectType(
				Field{Name: "id", Type: StringType()}, Field{Name: "url", Type: StringType()}, Field{Name: "quality", Type: StringType()},
				Field{Name: "claims", Type: IntegerType()}, Field{Name: "read", Type: BooleanType()},
			))},
			Field{Name: "notes", Type: ArrayType(StringType())},
		)
	}
	return Function{
		Name: "rank_claims", Version: 1, Params: params, Result: result(nil),
		Summary: "keep claims of sources their extractor fetched; order by importance, then source quality; keep the first max_claims",
		Refine:  func(args map[string]StaticArg) *Type { return result(literalBound(args["max_claims"])) },
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			limit := args["max_claims"].(int64)
			extractions := args["extractions"]
			// Every source in index order: an extraction that succeeded
			// has an output; one that failed has none.
			type entry struct {
				index     int64
				source    any
				output    any
				succeeded bool
			}
			var entries []entry
			results, _ := walkValue(extractions, []string{"results"}).([]any)
			for _, r := range results {
				entries = append(entries, entry{index: intField(r, "index"), source: walkValue(r, []string{"item"}), output: walkValue(r, []string{"output"}), succeeded: true})
			}
			failures, _ := walkValue(extractions, []string{"failures"}).([]any)
			for _, f := range failures {
				entries = append(entries, entry{index: intField(f, "index"), source: walkValue(f, []string{"item"})})
			}
			slices.SortStableFunc(entries, func(a, b entry) int { return cmp.Compare(a.index, b.index) })
			type candidate struct {
				claim               map[string]any
				entry, position     int
				importance, quality int
			}
			var candidates []candidate
			sources, notes := []any{}, []any{}
			var extracted, unfetched, read int64
			for e, en := range entries {
				id, u := stringField(en.source, "id"), strings.TrimSpace(stringField(en.source, "url"))
				summary := map[string]any{"id": id, "url": u, "quality": "", "claims": int64(0), "read": false}
				sources = append(sources, summary)
				if !en.succeeded {
					continue
				}
				output := en.output
				quality := stringField(output, "result", "source_quality")
				summary["quality"] = quality
				claims, _ := walkValue(output, []string{"result", "claims"}).([]any)
				key, ok := sourceKey(u)
				if _, fetched := fetchedKeys(output)[key]; !ok || !fetched {
					unfetched += int64(len(claims))
					if len(claims) > 0 {
						notes = append(notes, fmt.Sprintf("%s: its extractor did not fetch %s, so its %s dropped", id, u, count(int64(len(claims)), "claim was", "claims were")))
					}
					continue
				}
				read++
				summary["read"] = true
				for p, c := range claims {
					text := strings.Join(strings.Fields(stringField(c, "claim")), " ")
					if text == "" {
						continue
					}
					extracted++
					summary["claims"] = summary["claims"].(int64) + 1
					importance := stringField(c, "importance")
					candidates = append(candidates, candidate{
						claim: map[string]any{
							"text": text, "quote": stringField(c, "quote"), "importance": importance, "source_id": id, "source_url": u,
							"source_title": stringField(en.source, "title"), "source_quality": quality, "publish_date": stringField(output, "result", "publish_date"),
							"angle": stringField(en.source, "angle"),
						},
						entry: e, position: p, importance: rank(importanceRank, importance), quality: rank(qualityRank, quality),
					})
				}
			}
			slices.SortStableFunc(candidates, func(a, b candidate) int {
				return cmp.Or(cmp.Compare(a.importance, b.importance), cmp.Compare(a.quality, b.quality), cmp.Compare(a.entry, b.entry), cmp.Compare(a.position, b.position))
			})
			kept := []any{}
			for _, c := range candidates {
				if int64(len(kept)) >= limit {
					break
				}
				c.claim["id"] = fmt.Sprintf("c%d", len(kept)+1)
				kept = append(kept, c.claim)
			}
			return map[string]any{
				"claims": kept, "count": int64(len(kept)), "extracted": extracted, "unfetched": unfetched, "dropped": int64(len(candidates) - len(kept)),
				"sources_read": read, "extraction_failed": intField(extractions, "failed"), "sources": sources, "notes": notes,
			}, nil
		},
	}
}

func expandBallotsV2() Function {
	params := []Param{
		{Name: "claims", Type: ArrayType(openObject(
			Field{Name: "id", Type: StringType()},
			Field{Name: "text", Type: StringType()},
			Field{Name: "quote", Type: StringType()},
			Field{Name: "source_url", Type: StringType()},
			Field{Name: "source_quality", Type: StringType()},
			Field{Name: "publish_date", Type: StringType(), Optional: true},
		))},
		{Name: "votes", Type: IntegerType(), Min: 1, Max: 7},
	}
	ballot := ObjectType(
		Field{Name: "id", Type: StringType()}, Field{Name: "claim_id", Type: StringType()}, Field{Name: "voter", Type: IntegerType()},
		Field{Name: "text", Type: StringType()}, Field{Name: "quote", Type: StringType()}, Field{Name: "source_url", Type: StringType()},
		Field{Name: "source_quality", Type: StringType()}, Field{Name: "publish_date", Type: StringType()},
	)
	result := func(bound *int64) *Type {
		return ObjectType(Field{Name: "ballots", Type: boundedList(ballot, bound)}, Field{Name: "count", Type: IntegerType()})
	}
	return Function{
		Name: "expand_ballots", Version: 2, Params: params, Result: result(nil),
		Summary: "one ballot for each claim and voter, in claim order, carrying the claim, its quote and its source",
		Refine: func(args map[string]StaticArg) *Type {
			claims, votes := listBound(args["claims"]), literalBound(args["votes"])
			if claims == nil || votes == nil {
				return result(nil)
			}
			bound := *claims * *votes
			return result(&bound)
		},
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			votes := args["votes"].(int64)
			ballots := []any{}
			for _, c := range args["claims"].([]any) {
				id := stringField(c, "id")
				for voter := int64(1); voter <= votes; voter++ {
					ballots = append(ballots, map[string]any{
						"id": fmt.Sprintf("%s-v%d", id, voter), "claim_id": id, "voter": voter, "text": stringField(c, "text"), "quote": stringField(c, "quote"),
						"source_url": stringField(c, "source_url"), "source_quality": stringField(c, "source_quality"), "publish_date": stringField(c, "publish_date"),
					})
				}
			}
			return map[string]any{"ballots": ballots, "count": int64(len(ballots))}, nil
		},
	}
}

func tallyAdversarial() Function {
	params := []Param{
		{Name: "claims", Type: ArrayType(openObject(
			Field{Name: "id", Type: StringType()},
			Field{Name: "text", Type: StringType()},
			Field{Name: "quote", Type: StringType()},
			Field{Name: "importance", Type: StringType()},
			Field{Name: "source_url", Type: StringType()},
			Field{Name: "source_quality", Type: StringType()},
			Field{Name: "publish_date", Type: StringType(), Optional: true},
		))},
		{Name: "votes", Type: ArrayType(openObject(
			Field{Name: "item", Type: openObject(Field{Name: "claim_id", Type: StringType()}, Field{Name: "voter", Type: IntegerType(), Optional: true})},
			Field{Name: "output", Type: openObject(
				Field{Name: "sources", Type: ArrayType(openObject(Field{Name: "url", Type: StringType()}))},
				Field{Name: "result", Type: openObject(
					Field{Name: "refuted", Type: BooleanType()},
					Field{Name: "evidence", Type: StringType()},
					Field{Name: "evidence_urls", Type: ArrayType(StringType())},
					Field{Name: "confidence", Type: StringType()},
					Field{Name: "counter_source", Type: StringType(), Optional: true},
				)},
			)},
		))},
		{Name: "votes_per_claim", Type: IntegerType(), Min: 1, Max: 7},
		{Name: "refutations_required", Type: IntegerType(), Min: 1, Max: 7},
		{Name: "min_valid_votes", Type: IntegerType(), Min: 1, Max: 7},
		{Name: "sources", Type: openObject(
			Field{Name: "count", Type: IntegerType()}, Field{Name: "candidates", Type: IntegerType()},
			Field{Name: "duplicates", Type: IntegerType()}, Field{Name: "budget_dropped", Type: IntegerType()},
		)},
		{Name: "ranked", Type: openObject(
			Field{Name: "extracted", Type: IntegerType()}, Field{Name: "unfetched", Type: IntegerType()},
			Field{Name: "sources_read", Type: IntegerType()}, Field{Name: "extraction_failed", Type: IntegerType()},
		)},
	}
	confirmed := ObjectType(
		Field{Name: "id", Type: StringType()}, Field{Name: "text", Type: StringType()}, Field{Name: "quote", Type: StringType()},
		Field{Name: "source_url", Type: StringType()}, Field{Name: "source_quality", Type: StringType()}, Field{Name: "publish_date", Type: StringType()},
		Field{Name: "importance", Type: StringType()}, Field{Name: "vote", Type: StringType()}, Field{Name: "confidence", Type: enumType("high", "medium", "low")},
		Field{Name: "evidence", Type: StringType()}, Field{Name: "verifier_confidence", Type: StringType()}, Field{Name: "sources", Type: ArrayType(StringType())},
	)
	refuted := ObjectType(
		Field{Name: "id", Type: StringType()}, Field{Name: "text", Type: StringType()}, Field{Name: "source_url", Type: StringType()},
		Field{Name: "vote", Type: StringType()}, Field{Name: "evidence", Type: StringType()}, Field{Name: "counter_source", Type: StringType()},
	)
	unverified := ObjectType(
		Field{Name: "id", Type: StringType()}, Field{Name: "text", Type: StringType()}, Field{Name: "source_url", Type: StringType()},
		Field{Name: "valid", Type: IntegerType()}, Field{Name: "refuted", Type: IntegerType()}, Field{Name: "errored", Type: IntegerType()},
		Field{Name: "invalid", Type: IntegerType()}, Field{Name: "missing", Type: IntegerType()},
	)
	statNames := []string{"sources", "candidates", "duplicates", "budget_dropped", "sources_read", "extraction_failed", "claims_extracted", "claims_unfetched",
		"claims_verified", "confirmed", "refuted", "unverified", "ballots", "valid_votes", "invalid_votes", "missing_votes"}
	var statFields []Field
	for _, name := range statNames {
		statFields = append(statFields, Field{Name: name, Type: IntegerType()})
	}
	result := func(bound *int64) *Type {
		return ObjectType(
			Field{Name: "confirmed", Type: boundedList(confirmed, bound)},
			Field{Name: "refuted", Type: boundedList(refuted, bound)},
			Field{Name: "unverified", Type: boundedList(unverified, bound)},
			Field{Name: "confirmed_count", Type: IntegerType()},
			Field{Name: "refuted_count", Type: IntegerType()},
			Field{Name: "unverified_count", Type: IntegerType()},
			Field{Name: "outcome", Type: enumType(outcomes...)},
			Field{Name: "summary", Type: StringType()},
			Field{Name: "inconclusive_report", Type: ObjectType(
				Field{Name: "title", Type: StringType()}, Field{Name: "summary", Type: StringType()}, Field{Name: "body_markdown", Type: StringType()},
				Field{Name: "citations", Type: ArrayType(ObjectType(Field{Name: "marker", Type: StringType()}, Field{Name: "url", Type: StringType()}))},
			)},
			Field{Name: "stats", Type: ObjectType(statFields...)},
			Field{Name: "notes", Type: ArrayType(StringType())},
		)
	}
	return Function{
		Name: "tally_adversarial", Version: 1, Params: params, Result: result(nil),
		Summary: "count only votes resting on evidence the verifier fetched; refute a claim with refutations_required refutations, confirm it with min_valid_votes valid votes and fewer, otherwise leave it unverified; rate confirmed claims and cite only fetched evidence",
		Refine:  func(args map[string]StaticArg) *Type { return result(listBound(args["claims"])) },
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			return tallyClaims(args)
		},
	}
}

// ballotVote is one verifier's vote as the tally reads it.
type ballotVote struct {
	voter      int64
	valid      bool
	refuted    bool
	confidence string
	evidence   string
	urls       []string // the fetched evidence URLs as the harness recorded them
	fetched    map[string]string
	counter    string
}

func tallyClaims(args map[string]any) (any, error) {
	perClaim := args["votes_per_claim"].(int64)
	needed := args["refutations_required"].(int64)
	minValid := args["min_valid_votes"].(int64)
	claims := args["claims"].([]any)
	byClaim := map[string][]ballotVote{}
	for _, c := range claims {
		byClaim[stringField(c, "id")] = nil
	}
	notes := []any{}
	for _, v := range args["votes"].([]any) {
		id := stringField(v, "item", "claim_id")
		list, ok := byClaim[id]
		if !ok {
			return nil, fmt.Errorf("a vote names claim %q, which is not in claims", id)
		}
		if int64(len(list)) >= perClaim {
			return nil, fmt.Errorf("claim %s has more than votes_per_claim %d votes", id, perClaim)
		}
		output := walkValue(v, []string{"output"})
		b := ballotVote{voter: intField(v, "item", "voter"), fetched: fetchedKeys(output)}
		if b.voter == 0 {
			b.voter = int64(len(list) + 1)
		}
		b.refuted, _ = walkValue(output, []string{"result", "refuted"}).(bool)
		b.confidence = stringField(output, "result", "confidence")
		b.evidence = stringField(output, "result", "evidence")
		b.counter = stringField(output, "result", "counter_source")
		named, _ := walkValue(output, []string{"result", "evidence_urls"}).([]any)
		seen := map[string]bool{}
		for _, e := range named {
			u, _ := e.(string)
			key, ok := sourceKey(u)
			recorded, fetched := b.fetched[key]
			if ok && fetched && !seen[key] {
				seen[key] = true
				b.urls = append(b.urls, recorded)
			}
		}
		b.valid = len(b.urls) > 0
		if !b.valid {
			notes = append(notes, fmt.Sprintf("%s voter %d: named no evidence URL its verifier fetched; its vote was not counted", id, b.voter))
		}
		byClaim[id] = append(list, b)
	}
	confirmedList, refutedList, unverifiedList := []any{}, []any{}, []any{}
	var validVotes, invalidVotes, missingVotes int64
	for _, c := range claims {
		id := stringField(c, "id")
		votes := byClaim[id]
		var valid, refuted int64
		var surviving []ballotVote
		for _, b := range votes {
			if !b.valid {
				invalidVotes++
				continue
			}
			valid++
			if b.refuted {
				refuted++
			} else {
				surviving = append(surviving, b)
			}
		}
		validVotes += valid
		missing := perClaim - int64(len(votes))
		missingVotes += missing
		vote := fmt.Sprintf("%d-%d", valid-refuted, refuted)
		switch {
		case refuted >= needed:
			out := map[string]any{"id": id, "text": stringField(c, "text"), "source_url": stringField(c, "source_url"), "vote": vote, "evidence": "", "counter_source": ""}
			for _, b := range votes {
				if !b.valid || !b.refuted {
					continue
				}
				out["evidence"] = b.evidence
				if b.counter != "" {
					key, ok := sourceKey(b.counter)
					if recorded, fetched := b.fetched[key]; ok && fetched {
						out["counter_source"] = recorded
					} else {
						notes = append(notes, fmt.Sprintf("%s voter %d: its counter source was not a URL it fetched; left out", id, b.voter))
					}
				}
				break
			}
			refutedList = append(refutedList, out)
		case valid >= minValid:
			quality := stringField(c, "source_quality")
			level := 1
			if refuted == 0 && valid == perClaim {
				level = 0
			}
			switch quality {
			case "primary":
			case "secondary":
				level = max(level, 1)
			default:
				level = max(level, 2)
			}
			best := surviving[0]
			for _, b := range surviving[1:] {
				if rank(confidenceRank, b.confidence) < rank(confidenceRank, best.confidence) {
					best = b
				}
			}
			if best.confidence != "high" {
				level = min(level+1, 2)
			}
			sources := []any{}
			cited := map[string]bool{}
			for _, b := range surviving {
				for _, u := range b.urls {
					key, _ := sourceKey(u)
					if !cited[key] {
						cited[key] = true
						sources = append(sources, u)
					}
				}
			}
			own := stringField(c, "source_url")
			if key, ok := sourceKey(own); ok && !cited[key] {
				for _, b := range surviving {
					if _, fetched := b.fetched[key]; fetched {
						cited[key] = true
						sources = append(sources, strings.TrimSpace(own))
						break
					}
				}
			}
			confirmedList = append(confirmedList, map[string]any{
				"id": id, "text": stringField(c, "text"), "quote": stringField(c, "quote"), "source_url": own,
				"source_quality": quality, "publish_date": stringField(c, "publish_date"), "importance": stringField(c, "importance"),
				"vote": vote, "confidence": confidences[level], "evidence": best.evidence, "verifier_confidence": best.confidence, "sources": sources,
			})
		default:
			unverifiedList = append(unverifiedList, map[string]any{
				"id": id, "text": stringField(c, "text"), "source_url": stringField(c, "source_url"), "valid": valid, "refuted": refuted,
				"errored": perClaim - valid, "invalid": int64(len(votes)) - valid, "missing": missing,
			})
		}
	}
	nConfirmed, nRefuted, nUnverified := int64(len(confirmedList)), int64(len(refutedList)), int64(len(unverifiedList))
	sources, ranked := args["sources"], args["ranked"]
	var outcome, summary string
	switch {
	case len(claims) == 0:
		outcome = "no_claims"
		summary = fmt.Sprintf("No claims were extracted: %s selected and %s read, with no checkable claim (%s; %s). %s and %s dropped.",
			count(intField(sources, "count"), "source was", "sources were"), countWere(intField(ranked, "sources_read")),
			count(intField(ranked, "extraction_failed"), "extraction failed", "extractions failed"),
			count(intField(ranked, "unfetched"), "claim named a source its extractor did not fetch", "claims named a source their extractor did not fetch"),
			count(intField(sources, "duplicates"), "duplicate URL", "duplicate URLs"),
			count(intField(sources, "budget_dropped"), "result over the source budget was", "results over the source budget were"))
	case nConfirmed > 0:
		outcome = "confirmed"
		summary = fmt.Sprintf("%d of %d claims survived adversarial verification (%d refuted, %d unverified).", nConfirmed, len(claims), nRefuted, nUnverified)
	case nUnverified == 0:
		outcome = "all_refuted"
		summary = fmt.Sprintf("All %s refuted by adversarial verification. The research is inconclusive: the sources may be weak or the claims overstated.",
			count(nRefuted, "claim was", "claims were"))
	case nRefuted == 0:
		outcome = "verifiers_failed"
		summary = fmt.Sprintf("Could not verify any claim: the verifiers of all %s failed or gave no evidence they fetched (likely rate limits or provider errors). This is an infrastructure failure, not a research finding; start the run again or check the claims yourself.",
			count(nUnverified, "claim", "claims"))
	default:
		outcome = "refuted_and_unverified"
		their := "their verifiers failed"
		if nUnverified == 1 {
			their = "its verifiers failed"
		}
		summary = fmt.Sprintf("%s refuted by adversarial verification and %d could not be verified (%s). No claim survived; the research is inconclusive.",
			count(nRefuted, "claim was", "claims were"), nUnverified, their)
	}
	stats := map[string]any{
		"sources": intField(sources, "count"), "candidates": intField(sources, "candidates"), "duplicates": intField(sources, "duplicates"),
		"budget_dropped": intField(sources, "budget_dropped"), "sources_read": intField(ranked, "sources_read"),
		"extraction_failed": intField(ranked, "extraction_failed"), "claims_extracted": intField(ranked, "extracted"),
		"claims_unfetched": intField(ranked, "unfetched"), "claims_verified": int64(len(claims)), "confirmed": nConfirmed, "refuted": nRefuted,
		"unverified": nUnverified, "ballots": int64(len(claims)) * perClaim, "valid_votes": validVotes, "invalid_votes": invalidVotes, "missing_votes": missingVotes,
	}
	return map[string]any{
		"confirmed": confirmedList, "refuted": refutedList, "unverified": unverifiedList,
		"confirmed_count": nConfirmed, "refuted_count": nRefuted, "unverified_count": nUnverified,
		"outcome": outcome, "summary": summary,
		"inconclusive_report": inconclusiveReport(summary, refutedList, unverifiedList, perClaim),
		"stats":               stats, "notes": notes,
	}, nil
}

// countWere is "1 was" or "N were".
func countWere(n int64) string {
	if n == 1 {
		return "1 was"
	}
	return fmt.Sprintf("%d were", n)
}

// inconclusiveReport is the report of a run whose claims did not survive:
// what happened and which claims were refuted or could not be verified,
// with web text on one line each and no URL written here.
func inconclusiveReport(summary string, refuted, unverified []any, perClaim int64) map[string]any {
	var b strings.Builder
	b.WriteString("## What happened\n\n" + summary + "\n")
	if len(refuted) > 0 {
		b.WriteString("\n## Refuted claims\n\n")
		for _, r := range refuted {
			fmt.Fprintf(&b, "- %s (vote %s)\n", webLine(stringField(r, "text")), stringField(r, "vote"))
		}
	}
	if len(unverified) > 0 {
		b.WriteString("\n## Unverified claims\n\n")
		for _, u := range unverified {
			fmt.Fprintf(&b, "- %s (%d of %d votes not counted)\n", webLine(stringField(u, "text")), intField(u, "errored"), perClaim)
		}
	}
	return map[string]any{"title": "Research inconclusive", "summary": summary, "body_markdown": b.String(), "citations": []any{}}
}
