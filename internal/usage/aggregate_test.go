package usage

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func ptr(n int64) *int64 { return &n }

func TestAggregatePreservesCoverageAndWeightedCache(t *testing.T) {
	p, err := ParsePeriod("2026-09-01", "2026-09-03", "America/Detroit")
	if err != nil {
		t.Fatal(err)
	}
	observations := []Observation{
		{At: p.Start, Tokens: Tokens{Input: ptr(100), Cached: ptr(90), Output: ptr(20), Total: ptr(120), Reasoning: ptr(5)}},
		{At: p.Start.Add(time.Hour), Tokens: Tokens{Input: ptr(900), Cached: ptr(0), Output: ptr(10), Total: ptr(910)}},
		{At: p.Start.Add(2 * time.Hour), Tokens: Tokens{Input: ptr(500)}},
		{At: p.Start.Add(3 * time.Hour)},
		{At: p.End, Tokens: Tokens{Input: ptr(999)}},
	}
	got, err := Summarize(p, observations)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metrics.Calls != 4 || got.Metrics.MeasuredCalls != 3 || *got.Metrics.Input.Value != 1500 || got.Metrics.Input.Reported != 3 || *got.Metrics.Total.Value != 1030 {
		t.Fatalf("metrics: %+v", got.Metrics)
	}
	if got.Metrics.CachePercent == nil || *got.Metrics.CachePercent != 9 || *got.Metrics.CacheInput != 1000 || got.Metrics.CacheCalls != 2 {
		t.Fatalf("cache: %+v", got.Metrics)
	}
	if got.Metrics.CacheWrite.Value != nil || *got.Metrics.Cached.Value != 90 || got.Metrics.Cached.Reported != 2 {
		t.Fatal("missing or zero cache lost")
	}
	if len(got.Daily) != 2 || got.Daily[1].Metrics.Calls != 0 {
		t.Fatalf("daily: %+v", got.Daily)
	}
}

func TestAggregateRejectsOverflowAndDoesNotClampInvalidSubsets(t *testing.T) {
	p, _ := ParsePeriod("2026-09-01", "2026-09-02", "UTC")
	got, err := Summarize(p, []Observation{{At: p.Start, Tokens: Tokens{Input: ptr(5), Cached: ptr(6), Output: ptr(0), Total: ptr(5)}}})
	if err != nil || got.Metrics.CachePercent != nil || got.Metrics.Anomalies != 1 || *got.Metrics.Cached.Value != 6 {
		t.Fatalf("invalid subset: %+v %v", got, err)
	}
	_, err = Summarize(p, []Observation{{At: p.Start, Tokens: Tokens{Input: ptr(math.MaxInt64)}}, {At: p.Start, Tokens: Tokens{Input: ptr(1)}}})
	if err == nil {
		t.Fatal("overflow accepted")
	}
	got, err = Summarize(p, []Observation{{At: p.Start, Tokens: Tokens{Input: ptr(9007199254740993)}}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(got)
	if !strings.Contains(string(encoded), `"value":"9007199254740993"`) {
		t.Fatal(string(encoded))
	}
}

func TestPeriodUsesCalendarDaysAcrossDST(t *testing.T) {
	p, err := ParsePeriod("2026-03-08", "2026-03-09", "America/Detroit")
	if err != nil || p.End.Sub(p.Start) != 23*time.Hour {
		t.Fatalf("%+v %v", p, err)
	}
	for _, args := range [][3]string{{"2026-02-30", "2026-03-01", "UTC"}, {"2026-01-01", "2026-09-01", "UTC"}, {"2026-09-01", "2026-09-02", "bad"}} {
		if _, err := ParsePeriod(args[0], args[1], args[2]); err == nil {
			t.Fatal(args)
		}
	}
}

func TestPeriodHandlesSkippedCalendarDates(t *testing.T) {
	p, err := ParsePeriod("2011-12-29", "2011-12-31", "Pacific/Apia")
	if err != nil {
		t.Fatal(err)
	}
	summary, err := Summarize(p, nil)
	if err != nil || len(summary.Daily) != 2 || summary.Daily[1].Date != "2011-12-30" {
		t.Fatalf("%+v %v", summary, err)
	}
	// A timezone must not silently move a boundary into a different date.
	for _, args := range [][3]string{
		{"2011-12-30", "2011-12-31", "Pacific/Apia"},
		{"2018-11-04", "2018-11-05", "America/Sao_Paulo"},
	} {
		if _, err := ParsePeriod(args[0], args[1], args[2]); err == nil {
			t.Fatal(args)
		}
	}
}
