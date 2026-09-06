package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "worker" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err := command(ctx, os.Args[1:])
		stop()
		if err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestPilotRealKernelSmoke(t *testing.T) {
	for _, mode := range []string{"disabled", "new", "history"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			r, err := runWorkload(ctx, workload{Mode: mode, RetainedEvents: 8, SourceBytes: 64, GraphClaims: 1, Scopes: 2, DelayMS: 10, Processes: 2, Turns: 3, BackfillRoots: 3})
			if err != nil {
				t.Fatalf("pilot: %v, report: %+v", err, r)
			}
			if !r.Cleanup || r.ReleaseEligible || r.Kind != "scripted_infrastructure_only" || len(r.Foreground) != 3 {
				t.Fatalf("incorrect observation boundaries: %+v", r)
			}
			if mode == "disabled" {
				if len(r.Jobs) != 0 || len(r.Candidates) != 0 || len(r.WorkerPIDs) != 0 {
					t.Fatal("disabled compilation performed work")
				}
				return
			}
			if len(r.WorkerPIDs) != 2 || len(r.Jobs) < 3 || len(r.Candidates) < 3 || len(r.ResolutionNanos) == 0 {
				t.Fatalf("missing actual Kernel work: %+v", r)
			}
			if mode == "history" && (r.Counts["jobs_failed"] != 1 || r.Counts["jobs_completed_empty"] != 1 || r.Counts["jobs_completed_candidates"] != 4) {
				t.Fatalf("failed gap/zero-candidate/later progress: %+v", r.Counts)
			}
		})
	}
}

func TestReviewActiveTimeAndIncompleteObservation(t *testing.T) {
	clock := &reviewClock{}
	start := time.Now()
	input := reviewInput{Command: "start", Scope: "global", GenerationID: "generation"}
	input.Candidate.ID = "candidate"
	input.Candidate.InterpretationRevision = 1
	if _, err := clock.apply(input, start); err != nil {
		t.Fatal(err)
	}
	if _, err := clock.apply(reviewInput{Command: "pause"}, start.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := clock.apply(reviewInput{Command: "resume"}, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := clock.apply(reviewInput{Command: "finish", Action: "accept", Reason: "useful"}, start.Add(time.Hour+2*time.Second)); err == nil {
		t.Fatal("missing receipt accepted")
	}
	r, err := clock.apply(reviewInput{Command: "finish", Action: "defer", Reason: "needs another source"}, start.Add(time.Hour+2*time.Second))
	if err != nil || r.ActiveNanos != int64(5*time.Second) || r.ReceiptVerified || r.Useful != nil {
		t.Fatalf("invented observation: %+v %v", r, err)
	}
	path := filepath.Join(t.TempDir(), "review.jsonl")
	bytes, _ := json.Marshal(input)
	err = reviewCommand([]string{"--operator", "fixture", "--configuration-sha256", strings.Repeat("1", 64), "--output", path}, strings.NewReader(string(bytes)+"\n"), &strings.Builder{})
	if err == nil {
		t.Fatal("incomplete session silently passed")
	}
	saved, err := os.ReadFile(path)
	if err != nil || len(saved) != 0 {
		t.Fatal("incomplete human observation fabricated")
	}
}

func TestPilotRejectsOutOfBoundsAndExistingOutput(t *testing.T) {
	w := workload{Mode: "new", RetainedEvents: 1000001, SourceBytes: 64, GraphClaims: 1, Scopes: 1, Processes: 1, Turns: 1}
	if w.validate() == nil {
		t.Fatal("oversized retained fixture admitted")
	}
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := command(context.Background(), []string{"run", "--output", path}); err == nil {
		t.Fatal("existing report overwritten")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "preserve" {
		t.Fatal("existing report changed")
	}
}
