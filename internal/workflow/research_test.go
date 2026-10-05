package workflow

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The built-in deep-research v2's deterministic steps (2026-10-04):
// dedup_sources@1 normalizes and deduplicates search results across angles
// under a source budget, rank_claims@1 keeps only claims of sources their
// extractor fetched and orders them, expand_ballots@2 gives each claim three
// adversarial voters, and tally_adversarial@1 counts only votes resting on
// evidence the harness saw the voter fetch.

func searchResult(index int, label string, results ...map[string]any) any {
	list := []any{}
	for _, r := range results {
		list = append(list, r)
	}
	return map[string]any{
		"index": int64(index),
		"item":  map[string]any{"label": label, "query": "q " + label, "rationale": "r"},
		"output": map[string]any{
			"status": "succeeded", "summary": "s", "limitations": []any{}, "tokens": int64(1), "sources": []any{},
			"result": map[string]any{"results": list},
		},
	}
}

func hit(url, relevance string) map[string]any {
	return map[string]any{"url": url, "title": "Title " + url, "snippet": "why", "relevance": relevance}
}

func TestWorkflowSourceKeyNormalizesURLs(t *testing.T) {
	same := []string{
		"https://example.org/a/b",
		"https://www.example.org/a/b",
		"http://Example.ORG/a/b/",
		"https://user:pw@www.example.org:8443/a/b#section",
		"  https://example.org/a/b  ",
	}
	for _, u := range same {
		key, ok := sourceKey(u)
		if !ok || key != "example.org/a/b" {
			t.Fatalf("sourceKey(%q) = %q %v", u, key, ok)
		}
	}
	if key, _ := sourceKey("https://example.org/a/b?id=2"); key != "example.org/a/b?id=2" {
		t.Fatalf("query dropped: %q", key)
	}
	if key, _ := sourceKey("https://example.org/A/B"); key != "example.org/A/B" {
		t.Fatalf("path case changed: %q", key)
	}
	for _, bad := range []string{
		"javascript:alert(1)", "ftp://example.org/x", "https://evil.example\\@trusted.example/x", "https://exa mple.org/x",
		"https://example.org/\x07x", "https:///nohost", "example.org/x", "", "https://example.org/" + strings.Repeat("x", 500),
	} {
		if key, ok := sourceKey(bad); ok {
			t.Fatalf("sourceKey(%q) accepted as %q", bad, key)
		}
	}
}

func TestWorkflowDedupSourcesAcrossAnglesUnderBudget(t *testing.T) {
	results := []any{
		searchResult(0, "broad",
			hit("https://example.org/a", "medium"),
			hit("https://example.org/b", "high"),
			hit("javascript:alert(1)", "high"),
		),
		searchResult(1, "skeptical",
			hit("https://www.example.org/a/", "high"), // the same page as broad's medium a
			hit("http://example.org/b#top", "low"),    // the same page as broad's b
			hit("https://example.org/a?page=2", "low"),
			hit("https://other.example/c", "medium"),
		),
	}
	out := call(t, "dedup_sources@1", map[string]any{"results": results, "max_sources": int64(3)})
	var urls, angles []string
	for _, s := range out["sources"].([]any) {
		m := s.(map[string]any)
		urls = append(urls, m["url"].(string))
		angles = append(angles, m["angle"].(string))
	}
	// Ordered by relevance, then angle, then position: high b (broad), high
	// a (skeptical's www. spelling wins over broad's medium one), medium c.
	if want := []string{"https://example.org/b", "https://www.example.org/a/", "https://other.example/c"}; !reflect.DeepEqual(urls, want) {
		t.Fatalf("sources = %v", urls)
	}
	if want := []string{"broad", "skeptical", "skeptical"}; !reflect.DeepEqual(angles, want) {
		t.Fatalf("angles = %v", angles)
	}
	first := out["sources"].([]any)[0].(map[string]any)
	if first["id"] != "s1" || first["key"] != "example.org/b" || first["relevance"] != "high" || first["snippet"] != "why" {
		t.Fatalf("first source = %v", first)
	}
	if out["count"] != int64(3) || out["candidates"] != int64(7) || out["duplicates"] != int64(2) || out["budget_dropped"] != int64(1) || out["invalid"] != int64(1) {
		t.Fatalf("counts = %v", out)
	}
	reasons := map[string]string{}
	for _, d := range out["dropped"].([]any) {
		m := d.(map[string]any)
		reasons[m["url"].(string)] = m["reason"].(string)
	}
	if reasons["https://example.org/a"] != "duplicate of s2" || reasons["http://example.org/b#top"] != "duplicate of s1" ||
		reasons["https://example.org/a?page=2"] != "over the source budget" || reasons["javascript:alert(1)"] != "invalid URL" {
		t.Fatalf("dropped = %v", reasons)
	}
	// The same results in another order of completion give the same answer:
	// map results are in input order, and ordering never reads timing.
	again := call(t, "dedup_sources@1", map[string]any{"results": results, "max_sources": int64(3)})
	if !reflect.DeepEqual(out, again) {
		t.Fatal("dedup is not deterministic")
	}
	// The static bound comes from the literal budget.
	f := reviewedFunctions()["dedup_sources@1"]
	refined := f.Refine(map[string]StaticArg{"max_sources": {Type: IntegerType(), Literal: int64(15), IsConst: true}})
	if b := refined.Properties["sources"].MaxItems; b == nil || *b != 15 {
		t.Fatalf("sources bound = %v", b)
	}
}

func TestWorkflowDedupSourcesKeepsFifteenByRelevance(t *testing.T) {
	var results []any
	for angle := range 5 {
		var hits []map[string]any
		for i := range 6 {
			relevance := []string{"high", "medium", "low"}[i%3]
			hits = append(hits, hit(fmt.Sprintf("https://site%d.example/%d", angle, i), relevance))
		}
		results = append(results, searchResult(angle, fmt.Sprintf("angle-%d", angle), hits...))
	}
	out := call(t, "dedup_sources@1", map[string]any{"results": results, "max_sources": int64(15)})
	if out["count"] != int64(15) || out["budget_dropped"] != int64(15) || out["duplicates"] != int64(0) {
		t.Fatalf("counts = %v", out)
	}
	for _, s := range out["sources"].([]any) {
		if r := s.(map[string]any)["relevance"]; r == "low" {
			t.Fatalf("a low-relevance result was kept over a higher one: %v", s)
		}
	}
}

func extraction(index int, id, url string, fetched []string, quality string, claims ...map[string]any) any {
	sources := []any{}
	for _, u := range fetched {
		sources = append(sources, map[string]any{"url": u, "cited": true})
	}
	list := []any{}
	for _, c := range claims {
		list = append(list, c)
	}
	result := map[string]any{"source_quality": quality, "claims": list}
	if quality == "primary" {
		result["publish_date"] = "2026-03-01"
	}
	return map[string]any{
		"index": int64(index),
		"item":  map[string]any{"id": id, "url": url, "key": "k", "title": "T " + id, "snippet": "", "relevance": "high", "angle": "broad"},
		"output": map[string]any{
			"status": "succeeded", "summary": "s", "limitations": []any{}, "tokens": int64(1), "sources": sources,
			"result": result,
		},
	}
}

func extracted(text, importance string) map[string]any {
	return map[string]any{"claim": text, "quote": "quote of " + text, "importance": importance}
}

func TestWorkflowRankClaimsKeepsFetchedSourcesOrdersAndCaps(t *testing.T) {
	extractions := map[string]any{
		"results": []any{
			extraction(0, "s1", "https://blog.example/p", []string{"https://blog.example/p/"}, "blog",
				extracted("Blog central", "central"), extracted("Blog tangential", "tangential")),
			// Its extractor never fetched the page: its claims are dropped.
			extraction(1, "s2", "https://made-up.example/x", []string{"https://elsewhere.example/y"}, "primary",
				extracted("Unread central", "central")),
			extraction(2, "s3", "https://www.gov.example/report", []string{"https://gov.example/report"}, "primary",
				extracted("Primary supporting", "supporting"), extracted("  Primary   central ", "central")),
		},
		"failures":  []any{map[string]any{"index": int64(3), "item": map[string]any{"id": "s4", "url": "https://down.example/"}, "error": "worker failed"}},
		"succeeded": int64(3), "failed": int64(1),
	}
	out := call(t, "rank_claims@1", map[string]any{"extractions": extractions, "max_claims": int64(3)})
	var texts, ids []string
	for _, c := range out["claims"].([]any) {
		m := c.(map[string]any)
		texts, ids = append(texts, m["text"].(string)), append(ids, m["id"].(string))
	}
	if want := []string{"Primary central", "Blog central", "Primary supporting"}; !reflect.DeepEqual(texts, want) {
		t.Fatalf("ranked = %v", texts)
	}
	if !reflect.DeepEqual(ids, []string{"c1", "c2", "c3"}) {
		t.Fatalf("ids = %v", ids)
	}
	top := out["claims"].([]any)[0].(map[string]any)
	if top["source_id"] != "s3" || top["source_url"] != "https://www.gov.example/report" || top["source_quality"] != "primary" ||
		top["publish_date"] != "2026-03-01" || top["quote"] != "quote of   Primary   central " || top["importance"] != "central" {
		t.Fatalf("top claim = %v", top)
	}
	if blog := out["claims"].([]any)[1].(map[string]any); blog["publish_date"] != "" {
		t.Fatalf("absent publish date = %v", blog["publish_date"])
	}
	if out["count"] != int64(3) || out["extracted"] != int64(4) || out["unfetched"] != int64(1) || out["dropped"] != int64(1) ||
		out["sources_read"] != int64(2) || out["extraction_failed"] != int64(1) {
		t.Fatalf("counts = %v", out)
	}
	read := map[string]bool{}
	for _, s := range out["sources"].([]any) {
		m := s.(map[string]any)
		read[m["id"].(string)] = m["read"].(bool)
	}
	if !read["s1"] || read["s2"] || !read["s3"] || read["s4"] {
		t.Fatalf("sources = %v", out["sources"])
	}
	if !strings.Contains(strings.Join(toStrings(out["notes"].([]any)), "\n"), "s2") {
		t.Fatalf("notes = %v", out["notes"])
	}
}

func toStrings(list []any) []string {
	var out []string
	for _, v := range list {
		out = append(out, v.(string))
	}
	return out
}

func rankedClaim(id, quality string) map[string]any {
	return map[string]any{"id": id, "text": "Claim " + id, "quote": "Quote " + id, "importance": "central", "source_id": "s-" + id,
		"source_url": "https://src.example/" + id, "source_title": "T", "source_quality": quality, "publish_date": "", "angle": "broad"}
}

func TestWorkflowExpandBallotsV2CopiesTheClaimForEachVoter(t *testing.T) {
	claims := []any{rankedClaim("c1", "primary"), rankedClaim("c2", "blog")}
	out := call(t, "expand_ballots@2", map[string]any{"claims": claims, "votes": int64(3)})
	ballots := out["ballots"].([]any)
	if len(ballots) != 6 || out["count"] != int64(6) {
		t.Fatalf("ballots = %v", ballots)
	}
	b := ballots[4].(map[string]any)
	want := map[string]any{"id": "c2-v2", "claim_id": "c2", "voter": int64(2), "text": "Claim c2", "quote": "Quote c2",
		"source_url": "https://src.example/c2", "source_quality": "blog", "publish_date": ""}
	if !reflect.DeepEqual(b, want) {
		t.Fatalf("ballot = %v", b)
	}
	f := reviewedFunctions()["expand_ballots@2"]
	bound := int64(25)
	list := ArrayType(AnyType())
	list.MaxItems = &bound
	refined := f.Refine(map[string]StaticArg{"claims": {Type: list}, "votes": {Type: IntegerType(), Literal: int64(3), IsConst: true}})
	if m := refined.Properties["ballots"].MaxItems; m == nil || *m != 75 {
		t.Fatalf("ballots bound = %v", m)
	}
}

// vote is one verifier's map result: the URLs the harness saw it fetch,
// and its verdict.
func vote(claimID string, voter int, refuted bool, confidence string, evidence, fetched []string) any {
	sources := []any{}
	for _, u := range fetched {
		sources = append(sources, map[string]any{"url": u, "cited": true})
	}
	ev := []any{}
	for _, u := range evidence {
		ev = append(ev, u)
	}
	return map[string]any{
		"index": int64(0),
		"item":  map[string]any{"id": fmt.Sprintf("%s-v%d", claimID, voter), "claim_id": claimID, "voter": int64(voter)},
		"output": map[string]any{
			"status": "succeeded", "summary": "s", "limitations": []any{}, "tokens": int64(1), "sources": sources,
			"result": map[string]any{"refuted": refuted, "evidence": fmt.Sprintf("%s voter %d (%s)", claimID, voter, confidence), "evidence_urls": ev, "confidence": confidence},
		},
	}
}

var (
	dedupStats  = map[string]any{"count": int64(4), "candidates": int64(9), "duplicates": int64(3), "budget_dropped": int64(2), "invalid": int64(0)}
	rankedStats = map[string]any{"count": int64(4), "extracted": int64(6), "unfetched": int64(1), "dropped": int64(0), "sources_read": int64(3), "extraction_failed": int64(1)}
)

func tally(t *testing.T, claims, votes []any) map[string]any {
	t.Helper()
	return call(t, "tally_adversarial@1", map[string]any{"claims": claims, "votes": votes, "votes_per_claim": int64(3),
		"refutations_required": int64(2), "min_valid_votes": int64(2), "sources": dedupStats, "ranked": rankedStats})
}

func byID(list any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, v := range list.([]any) {
		m := v.(map[string]any)
		out[m["id"].(string)] = m
	}
	return out
}

func TestWorkflowTallyAdversarialKillsConfirmsAndLeavesUnverified(t *testing.T) {
	ev := func(id string, n int) string { return fmt.Sprintf("https://ev.example/%s/%d", id, n) }
	claims := []any{rankedClaim("c1", "primary"), rankedClaim("c2", "primary"), rankedClaim("c3", "primary"), rankedClaim("c4", "secondary"), rankedClaim("c5", "primary")}
	votes := []any{
		// c1: 3-0, every vote valid, one rated high: high confidence.
		vote("c1", 1, false, "high", []string{ev("c1", 1)}, []string{ev("c1", 1), "https://www.src.example/c1/"}),
		vote("c1", 2, false, "medium", []string{"https://www.ev.example/c1/2/"}, []string{ev("c1", 2)}),
		vote("c1", 3, false, "medium", []string{ev("c1", 3), "https://never.example/x"}, []string{ev("c1", 3)}),
		// c2: two refutations kill it, even with one support.
		vote("c2", 1, true, "high", []string{ev("c2", 1)}, []string{ev("c2", 1), "https://counter.example/c2"}),
		vote("c2", 2, false, "high", []string{ev("c2", 2)}, []string{ev("c2", 2)}),
		vote("c2", 3, true, "medium", []string{ev("c2", 3)}, []string{ev("c2", 3)}),
		// c3: one valid vote, one that fetched nothing it named, one
		// missing (its verifier failed): unverified.
		vote("c3", 1, false, "high", []string{ev("c3", 1)}, []string{ev("c3", 1)}),
		vote("c3", 2, false, "high", []string{"https://unfetched.example/c3"}, []string{"https://other.example/c3"}),
		// c4: 2-1 on a secondary source with no high vote: medium, lowered to low.
		vote("c4", 1, false, "medium", []string{ev("c4", 1)}, []string{ev("c4", 1)}),
		vote("c4", 2, true, "high", []string{ev("c4", 2)}, []string{ev("c4", 2)}),
		vote("c4", 3, false, "low", []string{ev("c4", 3)}, []string{ev("c4", 3)}),
		// c5: 2-0 with one missing vote and a high rating: medium.
		vote("c5", 1, false, "high", []string{ev("c5", 1)}, []string{ev("c5", 1)}),
		vote("c5", 2, false, "medium", []string{ev("c5", 2)}, []string{ev("c5", 2)}),
	}
	out := tally(t, claims, votes)
	confirmed, refuted, unverified := byID(out["confirmed"]), byID(out["refuted"]), byID(out["unverified"])
	if len(confirmed) != 3 || len(refuted) != 1 || len(unverified) != 1 || refuted["c2"] == nil || unverified["c3"] == nil {
		t.Fatalf("confirmed %v refuted %v unverified %v", confirmed, refuted, unverified)
	}
	c1 := confirmed["c1"]
	if c1["confidence"] != "high" || c1["vote"] != "3-0" || c1["verifier_confidence"] != "high" || c1["evidence"] != "c1 voter 1 (high)" {
		t.Fatalf("c1 = %v", c1)
	}
	// Citable sources: the harness-recorded strings of fetched evidence, in
	// vote order and deduplicated by key, then the claim's own source,
	// which a voter fetched; never a URL nobody fetched.
	if want := []any{ev("c1", 1), ev("c1", 2), ev("c1", 3), "https://src.example/c1"}; !reflect.DeepEqual(c1["sources"], want) {
		t.Fatalf("c1 sources = %v", c1["sources"])
	}
	if c4 := confirmed["c4"]; c4["confidence"] != "low" || c4["vote"] != "2-1" || !reflect.DeepEqual(c4["sources"], []any{ev("c4", 1), ev("c4", 3)}) {
		t.Fatalf("c4 = %v", c4)
	}
	if c5 := confirmed["c5"]; c5["confidence"] != "medium" || c5["vote"] != "2-0" {
		t.Fatalf("c5 = %v", c5)
	}
	c2 := refuted["c2"]
	if c2["vote"] != "1-2" || c2["evidence"] != "c2 voter 1 (high)" || c2["counter_source"] != "" {
		t.Fatalf("c2 = %v", c2)
	}
	c3 := unverified["c3"]
	if c3["valid"] != int64(1) || c3["invalid"] != int64(1) || c3["missing"] != int64(1) || c3["errored"] != int64(2) {
		t.Fatalf("c3 = %v", c3)
	}
	if out["outcome"] != "confirmed" || out["summary"] != "3 of 5 claims survived adversarial verification (1 refuted, 1 unverified)." {
		t.Fatalf("outcome %v: %v", out["outcome"], out["summary"])
	}
	stats := out["stats"].(map[string]any)
	if stats["ballots"] != int64(15) || stats["valid_votes"] != int64(12) || stats["invalid_votes"] != int64(1) || stats["missing_votes"] != int64(2) ||
		stats["duplicates"] != int64(3) || stats["claims_unfetched"] != int64(1) || stats["claims_verified"] != int64(5) {
		t.Fatalf("stats = %v", stats)
	}
	if !strings.Contains(strings.Join(toStrings(out["notes"].([]any)), "\n"), "c3 voter 2") {
		t.Fatalf("notes = %v", out["notes"])
	}
}

func TestWorkflowTallyAdversarialConfidenceRule(t *testing.T) {
	for _, tc := range []struct {
		quality string
		ratings []string
		refute  int
		want    string
	}{
		{"primary", []string{"high", "high", "high"}, 0, "high"},
		{"primary", []string{"medium", "medium", "low"}, 0, "medium"}, // no high rating lowers it
		{"secondary", []string{"high", "high", "high"}, 0, "medium"},  // secondary caps at medium
		{"secondary", []string{"low", "medium", "low"}, 0, "low"},     // capped, then lowered
		{"blog", []string{"high", "high", "high"}, 0, "low"},          // blog caps at low
		{"forum", []string{"low", "low", "low"}, 0, "low"},            // never below low
		{"primary", []string{"high", "high", "high"}, 1, "medium"},    // a split vote starts at medium
		{"unreliable", []string{"high", "high", "medium"}, 0, "low"},  // unreliable caps at low
	} {
		var votes []any
		for i, rating := range tc.ratings {
			u := fmt.Sprintf("https://ev.example/%d", i)
			votes = append(votes, vote("c1", i+1, i < tc.refute, rating, []string{u}, []string{u}))
		}
		out := tally(t, []any{rankedClaim("c1", tc.quality)}, votes)
		c := byID(out["confirmed"])["c1"]
		if c == nil || c["confidence"] != tc.want {
			t.Errorf("%s %v refuted %d: %v, want %s", tc.quality, tc.ratings, tc.refute, c, tc.want)
		}
	}
}

func TestWorkflowTallyAdversarialCounterSourceOnlyWhenFetched(t *testing.T) {
	refuting := func(voter int, counter string, fetched []string) any {
		v := vote("c1", voter, true, "high", []string{fmt.Sprintf("https://ev.example/%d", voter)}, append([]string{fmt.Sprintf("https://ev.example/%d", voter)}, fetched...)).(map[string]any)
		v["output"].(map[string]any)["result"].(map[string]any)["counter_source"] = counter
		return v
	}
	out := tally(t, []any{rankedClaim("c1", "primary")}, []any{
		refuting(1, "https://counter.example/fetched", []string{"https://www.counter.example/fetched/"}),
		refuting(2, "https://counter.example/never", nil),
	})
	c1 := byID(out["refuted"])["c1"]
	if c1["counter_source"] != "https://www.counter.example/fetched/" {
		t.Fatalf("counter source = %v", c1["counter_source"])
	}
	out = tally(t, []any{rankedClaim("c1", "primary")}, []any{
		refuting(1, "https://counter.example/never", nil),
		refuting(2, "https://counter.example/fetched", []string{"https://counter.example/fetched"}),
	})
	if c1 := byID(out["refuted"])["c1"]; c1["counter_source"] != "" {
		t.Fatalf("an unfetched counter source was kept: %v", c1)
	}
}

func TestWorkflowTallyAdversarialHonestInconclusiveOutcomes(t *testing.T) {
	u := func(id string, n int) string { return fmt.Sprintf("https://ev.example/%s/%d", id, n) }
	refutedVotes := func(id string) []any {
		return []any{vote(id, 1, true, "high", []string{u(id, 1)}, []string{u(id, 1)}), vote(id, 2, true, "high", []string{u(id, 2)}, []string{u(id, 2)})}
	}
	claims := []any{rankedClaim("c1", "primary"), rankedClaim("c2", "primary")}

	allRefuted := tally(t, claims, append(refutedVotes("c1"), refutedVotes("c2")...))
	if allRefuted["outcome"] != "all_refuted" || allRefuted["summary"] != "All 2 claims were refuted by adversarial verification. The research is inconclusive: the sources may be weak or the claims overstated." {
		t.Fatalf("all refuted: %v %v", allRefuted["outcome"], allRefuted["summary"])
	}

	failed := tally(t, claims, nil)
	if failed["outcome"] != "verifiers_failed" || !strings.Contains(failed["summary"].(string), "infrastructure failure, not a research finding") ||
		!strings.Contains(failed["summary"].(string), "all 2 claims") {
		t.Fatalf("verifiers failed: %v %v", failed["outcome"], failed["summary"])
	}

	mixed := tally(t, claims, refutedVotes("c1"))
	if mixed["outcome"] != "refuted_and_unverified" || mixed["summary"] != "1 claim was refuted by adversarial verification and 1 could not be verified (its verifiers failed). No claim survived; the research is inconclusive." {
		t.Fatalf("mixed: %v %v", mixed["outcome"], mixed["summary"])
	}

	none := tally(t, []any{}, nil)
	if none["outcome"] != "no_claims" || none["summary"] != "No claims were extracted: 4 sources were selected and 3 were read, with no checkable claim (1 extraction failed; 1 claim named a source its extractor did not fetch). 3 duplicate URLs and 2 results over the source budget were dropped." {
		t.Fatalf("no claims: %v %v", none["outcome"], none["summary"])
	}

	// The inconclusive report names what was refuted and what could not be
	// verified, cleans web text and writes no URL of its own.
	claims[0].(map[string]any)["text"] = "Claim with\nnew lines\u202e and https://evil.example/link"
	report := tally(t, claims, refutedVotes("c1"))["inconclusive_report"].(map[string]any)
	body := report["body_markdown"].(string)
	if report["title"] != "Research inconclusive" || report["summary"] != mixed["summary"] || len(report["citations"].([]any)) != 0 {
		t.Fatalf("report = %v", report)
	}
	for _, want := range []string{"## What happened", "## Refuted claims", "## Unverified claims", "Claim with new lines and", "vote 0-2", "Claim c2"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "\u202e") || strings.Contains(body, "src.example") || strings.Contains(body, "ev.example") {
		t.Fatalf("body carries control characters or URLs of its own:\n%s", body)
	}
	// check_citations@1, which the inconclusive step runs, strips the URL
	// the web claim carried.
	checked := call(t, "check_citations@1", map[string]any{"report": report, "claims": []any{}})
	if strings.Contains(checked["body_markdown"].(string), "evil.example") {
		t.Fatalf("checked body = %s", checked["body_markdown"])
	}
}

func TestWorkflowTallyAdversarialRefusesVotesOutsideTheClaims(t *testing.T) {
	f := reviewedFunctions()["tally_adversarial@1"]
	args := map[string]any{"claims": []any{rankedClaim("c1", "primary")}, "votes_per_claim": int64(3), "refutations_required": int64(2), "min_valid_votes": int64(2),
		"sources": dedupStats, "ranked": rankedStats}
	args["votes"] = []any{vote("c9", 1, false, "high", nil, nil)}
	if _, err := f.Call(args); err == nil || !strings.Contains(err.Error(), "c9") {
		t.Fatalf("a vote for an unknown claim: %v", err)
	}
	var four []any
	for i := 1; i <= 4; i++ {
		four = append(four, vote("c1", i, false, "high", nil, nil))
	}
	args["votes"] = four
	if _, err := f.Call(args); err == nil || !strings.Contains(err.Error(), "votes_per_claim") {
		t.Fatalf("more votes than ballots: %v", err)
	}
}

// Every reviewed function version stays registered: a definition pinned to
// one keeps running, and a new behavior ships as a new version.
func TestWorkflowReviewedFunctionVersionsAreNeverRemoved(t *testing.T) {
	registry := reviewedFunctions()
	for _, id := range []string{"collect_claims@1", "expand_ballots@1", "majority_filter@1", "split_evenly@1", "tally_verdicts@1", "check_citations@1",
		"dedup_sources@1", "rank_claims@1", "expand_ballots@2", "tally_adversarial@1"} {
		if _, ok := registry[id]; !ok {
			t.Errorf("%s is not registered", id)
		}
	}
}
