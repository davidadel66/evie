package usage

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

type conversationFixture struct{ failure bool }

func (f conversationFixture) ReadConversationUsage(_ context.Context, p Period) (Summary, error) {
	if f.failure {
		return Summary{}, errors.New("private-db-error")
	}
	return Summarize(p, []Observation{{At: p.Start, Tokens: Tokens{Input: ptr(100), Cached: ptr(80), Output: ptr(20), Total: ptr(120)}}})
}
func TestServiceKeepsSourcesSeparateAndSurvivesAccountFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx, conversationFixture{}, "/nonexistent/codex", []string{t.TempDir()})
	p, _ := ParsePeriod("2026-09-01", "2026-09-03", "UTC")
	deadline := time.Now().Add(3 * time.Second)
	for {
		report, err := service.InspectUsage(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Sources) != 3 || report.Sources[2].Summary == nil || *report.Sources[2].Summary.Metrics.Total.Value != 120 {
			t.Fatal("external failure hid Evie")
		}
		if report.Sources[0].State == "unavailable" && report.Sources[1].State != "indexing" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background readers did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if _, err := service.InspectUsage(ctx, p); err == nil {
		t.Fatal("caller cancellation ignored")
	}
}
func TestServiceDeduplicatesSameAccountAcrossConfiguredHomes(t *testing.T) {
	t.Setenv("EVIE_USAGE_TEST_PEER", "success")
	binary, _ := os.Executable()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx, conversationFixture{failure: true}, binary, []string{t.TempDir(), t.TempDir()})
	p, _ := ParsePeriod("2026-09-01", "2026-09-03", "UTC")
	deadline := time.Now().Add(3 * time.Second)
	for {
		report, err := service.InspectUsage(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		accounts := 0
		ready := 0
		for _, source := range report.Sources {
			if source.Kind == "codex-account" {
				accounts++
				if source.State == "ready" {
					ready++
				}
			}
		}
		if ready == 1 && accounts == 1 {
			if len(report.Sources) != 4 || report.Sources[3].State != "unavailable" {
				t.Fatal("source failure or dedup lost")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("same account was not deduplicated")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestServicePrefersFreshDuplicateAccount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Keep seeded collector snapshots stable.
	service := NewService(ctx, conversationFixture{}, "", []string{"one", "two"})
	for i, state := range []string{"stale", "ready"} {
		service.homes[i].account.source = Source{ID: "same", Kind: "codex-account", State: state}
	}
	p, _ := ParsePeriod("2026-09-01", "2026-09-02", "UTC")
	report, err := service.InspectUsage(context.Background(), p)
	if err != nil || report.Sources[0].State != "ready" {
		t.Fatalf("%+v %v", report, err)
	}
}
