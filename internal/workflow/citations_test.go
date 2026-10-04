package workflow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Stage 14 sub-stage 10 (2026-10-04): the deep-research template's
// deterministic integrity steps. tally_verdicts@1 counts a verifier's
// supported or refuted vote only when it names evidence the harness saw it
// fetch, and check_citations@1 keeps only citations of sources a verifier
// fetched and relied on for an accepted claim.

func verdictVote(claimID, verdict string, evidence, fetched []string) any {
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
		"item":  map[string]any{"id": claimID + "-v", "claim_id": claimID, "text": "t", "sources": []any{}, "voter": int64(1)},
		"output": map[string]any{
			"status": "succeeded", "summary": "s", "limitations": []any{}, "tokens": int64(1), "sources": sources,
			"result": map[string]any{"verdict": verdict, "evidence": ev, "rationale": "r"},
		},
	}
}

func TestWorkflowTallyVerdictsCountsOnlyFetchedEvidence(t *testing.T) {
	claims := []any{
		map[string]any{"id": "c1", "angle_id": "a", "text": "one", "sources": []any{"https://r/1"}, "unverified_sources": []any{}},
		map[string]any{"id": "c2", "angle_id": "a", "text": "two", "sources": []any{}, "unverified_sources": []any{}},
		map[string]any{"id": "c3", "angle_id": "b", "text": "three", "sources": []any{}, "unverified_sources": []any{}},
	}
	votes := []any{
		verdictVote("c1", "supported", []string{"https://v/1"}, []string{"https://v/1"}),
		verdictVote("c1", "supported", []string{"https://v/2", "https://v/never"}, []string{"https://v/2", "https://v/3"}),
		// A refutation naming nothing it fetched counts as insufficient.
		verdictVote("c1", "refuted", []string{"https://v/4"}, nil),
		// A supported vote without fetched evidence counts as insufficient.
		verdictVote("c2", "supported", []string{"https://v/5"}, []string{"https://v/other"}),
		verdictVote("c2", "supported", []string{"https://v/6"}, []string{"https://v/6"}),
		// c2's third vote is missing (its worker failed): it counts against.
		verdictVote("c3", "refuted", []string{"https://v/7"}, []string{"https://v/7"}),
		verdictVote("c3", "insufficient", nil, []string{"https://v/8"}),
	}
	out := call(t, "tally_verdicts@1", map[string]any{"claims": claims, "votes": votes, "quorum": int64(2)})
	got, _ := json.Marshal(out)
	want := `{"accepted":[{"angle_id":"a","id":"c1","insufficient":1,"refuted":0,"sources":["https://v/1","https://v/2"],"supported":2,"text":"one"}],` +
		`"accepted_count":1,"downgraded":2,"notes":["c1: a refuted vote named no evidence URL its verifier fetched; counted insufficient",` +
		`"c2: a supported vote named no evidence URL its verifier fetched; counted insufficient"],` +
		`"rejected":[{"angle_id":"a","id":"c2","insufficient":1,"refuted":0,"supported":1,"text":"two"},` +
		`{"angle_id":"b","id":"c3","insufficient":1,"refuted":1,"supported":0,"text":"three"}],"rejected_count":2}`
	if string(got) != want {
		t.Fatalf("tally_verdicts\n got %s\nwant %s", got, want)
	}
	f := reviewedFunctions()["tally_verdicts@1"]
	if _, err := f.Call(map[string]any{"claims": claims, "votes": []any{verdictVote("zz", "supported", nil, nil)}, "quorum": int64(2)}); err == nil || !strings.Contains(err.Error(), "zz") {
		t.Fatalf("vote for an unknown claim: %v", err)
	}
	if _, err := f.Call(map[string]any{"claims": claims, "votes": []any{verdictVote("c1", "unclear", nil, nil)}, "quorum": int64(2)}); err == nil || !strings.Contains(err.Error(), "unclear") {
		t.Fatalf("unknown verdict: %v", err)
	}
}

func citedReport(summary, body string, citations ...[2]string) map[string]any {
	list := []any{}
	for _, c := range citations {
		list = append(list, map[string]any{"marker": c[0], "url": c[1]})
	}
	return map[string]any{"title": "Range", "summary": summary, "body_markdown": body, "citations": list}
}

var acceptedForCitations = []any{
	map[string]any{"id": "c1", "angle_id": "a", "text": "one", "supported": int64(3), "refuted": int64(0), "insufficient": int64(0), "sources": []any{"https://a/1", "https://a/2"}},
	map[string]any{"id": "c2", "angle_id": "b", "text": "two", "supported": int64(2), "refuted": int64(0), "insufficient": int64(1), "sources": []any{"https://b/1"}},
}

func TestWorkflowCheckCitationsKeepsOnlyVerifiedSources(t *testing.T) {
	report := citedReport("Answer [S1].",
		"Fact one [S1]. Fact two [S2][S3]. Made up [S9]. See https://evil.example/x, [a link](https://a/2) and [bad](https://evil.example/y). Combined [S1, S9, S3].",
		[2]string{"S1", "https://a/1"}, [2]string{"S2", "https://evil.example/z"}, [2]string{"S3", " https://b/1 "},
		[2]string{"S4", "https://a/2"}, [2]string{"S1", "https://b/1"}, [2]string{"source 5", "https://a/1"})
	out := call(t, "check_citations@1", map[string]any{"report": report, "claims": acceptedForCitations})
	if out["summary"] != "Answer [S1]." {
		t.Fatalf("summary = %q", out["summary"])
	}
	wantBody := "Fact one [S1]. Fact two [S3]. Made up. See [unverified link removed], [a link](https://a/2) and bad. Combined [S1][S3]."
	if out["body_markdown"] != wantBody {
		t.Fatalf("body\n got %q\nwant %q", out["body_markdown"], wantBody)
	}
	citations, _ := json.Marshal(out["citations"])
	if string(citations) != `[{"claim_ids":["c1"],"marker":"S1","url":"https://a/1"},{"claim_ids":["c2"],"marker":"S3","url":"https://b/1"}]` {
		t.Fatalf("citations = %s", citations)
	}
	var removed []string
	for _, r := range out["removed"].([]any) {
		m := r.(map[string]any)
		removed = append(removed, m["kind"].(string)+" "+m["value"].(string)+": "+m["reason"].(string))
	}
	want := []string{
		"citation S2: https://evil.example/z is not a source a verifier fetched and relied on for an accepted claim",
		"citation S4: never cited in the report",
		"citation S1: duplicate marker",
		"citation source 5: malformed marker; use S1, S2, …",
		"marker S2: cites a removed citation",
		"marker S9: cites no listed citation",
		"link https://evil.example/x: not a verified source",
		"link https://evil.example/y: not a verified source",
	}
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("removed\n got %q\nwant %q", removed, want)
	}
	if out["status"] != "corrected" || !reflect.DeepEqual(out["cited_claims"], []any{"c1", "c2"}) || !reflect.DeepEqual(out["uncited_claims"], []any{}) {
		t.Fatalf("status %v cited %v uncited %v", out["status"], out["cited_claims"], out["uncited_claims"])
	}
}

func TestWorkflowCheckCitationsPassesACleanReport(t *testing.T) {
	report := citedReport("Cells store 0.3 kWh/kg [S1].", "## Energy\n\nCells store 0.3 kWh/kg [S1].\n", [2]string{"S1", "https://a/2"})
	out := call(t, "check_citations@1", map[string]any{"report": report, "claims": acceptedForCitations})
	if out["status"] != "verified" || len(out["removed"].([]any)) != 0 || out["body_markdown"] != report["body_markdown"] ||
		!reflect.DeepEqual(out["cited_claims"], []any{"c1"}) || !reflect.DeepEqual(out["uncited_claims"], []any{"c2"}) {
		t.Fatalf("clean report = %v", out)
	}
}
