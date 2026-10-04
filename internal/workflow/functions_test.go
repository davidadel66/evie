package workflow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func call(t *testing.T, id string, args map[string]any) map[string]any {
	t.Helper()
	f, ok := reviewedFunctions()[id]
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	out, err := f.Call(args)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	if err := checkValue(f.Result, out); err != nil {
		t.Fatalf("%s returned a value outside its declared result: %v", id, err)
	}
	return out.(map[string]any)
}

func finding(angle string, fetched []string, claims ...map[string]any) map[string]any {
	sources := []any{}
	for _, u := range fetched {
		sources = append(sources, map[string]any{"url": u, "cited": true})
	}
	list := []any{}
	for _, c := range claims {
		list = append(list, c)
	}
	return map[string]any{
		"index": int64(0),
		"item":  map[string]any{"id": angle, "question": "q"},
		"output": map[string]any{
			"status": "succeeded", "summary": "s", "report": "r", "limitations": []any{}, "tokens": int64(1),
			"sources": sources,
			"result":  map[string]any{"claims": list},
		},
	}
}

func claim(text string, sources ...string) map[string]any {
	s := []any{}
	for _, u := range sources {
		s = append(s, u)
	}
	return map[string]any{"text": text, "sources": s}
}

func TestWorkflowCollectClaimsMergesKeepsFetchedSourcesAndCaps(t *testing.T) {
	findings := []any{
		finding("a", []string{"https://x/1", "https://x/2"}, claim("Fuel stores 12 kWh/kg.", "https://x/1", "https://x/never"), claim("Cells store 0.3 kWh/kg.", "https://x/2")),
		finding("b", []string{"https://y/1"}, claim("  fuel   STORES 12 kWh/kg. ", "https://y/1"), claim("Motors are efficient.", "https://y/1")),
	}
	out := call(t, "collect_claims@1", map[string]any{"findings": findings, "max_claims": int64(2)})
	got, _ := json.Marshal(out)
	want := `{"claims":[{"angle_id":"a","id":"c1","sources":["https://x/1","https://y/1"],"text":"Fuel stores 12 kWh/kg.","unverified_sources":["https://x/never"]},{"angle_id":"a","id":"c2","sources":["https://x/2"],"text":"Cells store 0.3 kWh/kg.","unverified_sources":[]}],"count":2,"dropped":1}`
	if string(got) != want {
		t.Fatalf("collect_claims\n got %s\nwant %s", got, want)
	}
}

func TestWorkflowExpandBallotsAndMajorityFilter(t *testing.T) {
	claims := []any{
		map[string]any{"id": "c1", "angle_id": "a", "text": "one", "sources": []any{"u"}, "unverified_sources": []any{}},
		map[string]any{"id": "c2", "angle_id": "a", "text": "two", "sources": []any{}, "unverified_sources": []any{}},
	}
	ballots := call(t, "expand_ballots@1", map[string]any{"claims": claims, "votes": int64(3)})
	list := ballots["ballots"].([]any)
	if len(list) != 6 || ballots["count"] != int64(6) {
		t.Fatalf("ballots = %v", ballots)
	}
	if b := list[4].(map[string]any); b["id"] != "c2-v2" || b["claim_id"] != "c2" || b["voter"] != int64(2) || b["text"] != "two" {
		t.Fatalf("ballot 4 = %v", b)
	}
	vote := func(index int, verdict string) any {
		return map[string]any{"index": int64(index), "item": list[index], "output": map[string]any{"result": map[string]any{"verdict": verdict, "rationale": "r"}}}
	}
	// c1: supported, supported, (missing). c2: supported, refuted, unclear.
	votes := []any{vote(0, "supported"), vote(1, "supported"), vote(3, "supported"), vote(4, "refuted"), vote(5, "unclear")}
	out := call(t, "majority_filter@1", map[string]any{"claims": claims, "votes": votes, "quorum": int64(2)})
	accepted, rejected := out["accepted"].([]any), out["rejected"].([]any)
	if len(accepted) != 1 || accepted[0].(map[string]any)["id"] != "c1" || out["accepted_count"] != int64(1) {
		t.Fatalf("accepted = %v", accepted)
	}
	c1 := accepted[0].(map[string]any)
	if c1["supported"] != int64(2) || c1["refuted"] != int64(0) || c1["unclear"] != int64(0) {
		t.Fatalf("c1 tally = %v", c1)
	}
	if len(rejected) != 1 || rejected[0].(map[string]any)["supported"] != int64(1) || rejected[0].(map[string]any)["refuted"] != int64(1) {
		t.Fatalf("rejected = %v", rejected)
	}
}

func TestWorkflowFunctionsRefuseOutOfBoundsArguments(t *testing.T) {
	f := reviewedFunctions()["expand_ballots@1"]
	if _, err := f.Call(map[string]any{"claims": []any{}, "votes": int64(0)}); err == nil || !strings.Contains(err.Error(), "votes") {
		t.Fatalf("zero votes: %v", err)
	}
	g := reviewedFunctions()["majority_filter@1"]
	_, err := g.Call(map[string]any{"claims": []any{}, "votes": []any{map[string]any{"item": map[string]any{"claim_id": "zz"}, "output": map[string]any{"result": map[string]any{"verdict": "supported"}}}}, "quorum": int64(1)})
	if err == nil || !strings.Contains(err.Error(), "zz") {
		t.Fatalf("vote for an unknown claim: %v", err)
	}
}

// The registry is the reviewed set: listing it is part of review, and each
// function is pure, so the same arguments give the same result.
func TestWorkflowFunctionRegistryIsReviewedAndPure(t *testing.T) {
	ids := []string{}
	for id := range reviewedFunctions() {
		ids = append(ids, id)
	}
	want := []string{"check_citations@1", "collect_claims@1", "expand_ballots@1", "majority_filter@1", "split_evenly@1", "tally_verdicts@1"}
	if !reflect.DeepEqual(sortedStrings(ids), want) {
		t.Fatalf("registry = %v, want %v", ids, want)
	}
	args := map[string]any{"findings": []any{finding("a", []string{"u"}, claim("x", "u"))}, "max_claims": int64(5)}
	first, _ := json.Marshal(call(t, "collect_claims@1", args))
	second, _ := json.Marshal(call(t, "collect_claims@1", args))
	if string(first) != string(second) {
		t.Fatal("collect_claims is not deterministic")
	}
}
