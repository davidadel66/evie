package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == "worker" || os.Args[1] == "review-session") {
		err := entry(os.Args[1:])
		if err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestReviewSessionSignalsPreserveOnlyCompletedObservations(t *testing.T) {
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "observations.jsonl")
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(executable, "review-session", "--operator", "fixture", "--configuration-sha256", strings.Repeat("1", 64), "--output", path)
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer cmd.Process.Kill()
			lines := make(chan string, 8)
			go func() {
				scanner := bufio.NewScanner(output)
				for scanner.Scan() {
					lines <- scanner.Text()
				}
				close(lines)
			}()
			readLine := func() string {
				t.Helper()
				select {
				case line := <-lines:
					return line
				case <-time.After(5 * time.Second):
					t.Fatal("recorder did not respond")
					return ""
				}
			}
			if !strings.Contains(readLine(), "Timing is explicit") {
				t.Fatal("recorder did not start")
			}
			start := reviewInput{Command: "start", Scope: "global", GenerationID: "generation"}
			start.Candidate.ID = "completed-candidate"
			start.Candidate.InterpretationRevision = 1
			encoder := json.NewEncoder(input)
			if err = encoder.Encode(start); err != nil {
				t.Fatal(err)
			}
			if err = encoder.Encode(reviewInput{Command: "finish", Action: "defer", Reason: "fixture completed observation"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(readLine(), "Observation saved") {
				t.Fatal("completed observation not acknowledged")
			}
			start.Candidate.ID = "incomplete-candidate"
			if err = encoder.Encode(start); err != nil {
				t.Fatal(err)
			}
			// A pause acknowledgement confirms the second candidate was parsed
			// and the recorder is waiting again with the input pipe still open.
			if err = encoder.Encode(reviewInput{Command: "pause"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(readLine(), "Timing paused") {
				t.Fatal("active candidate was not read")
			}
			if err = encoder.Encode(reviewInput{Command: "resume"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(readLine(), "Timing resumed") {
				t.Fatal("active timing was not resumed")
			}
			if err = cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			wait := make(chan error, 1)
			go func() { wait <- cmd.Wait() }()
			select {
			case err = <-wait:
				if err == nil {
					t.Fatal("signal unexpectedly produced a successful session")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("signal left recorder blocked on input")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var observation reviewObservation
			if err = json.Unmarshal(data, &observation); err != nil {
				t.Fatalf("saved data was lost or another record appeared: %v", err)
			}
			if observation.Candidate.ID != "completed-candidate" || observation.Action != "defer" {
				t.Fatalf("wrong saved observation: %+v", observation)
			}
		})
	}
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
			if r.Counts["foreground_persisted_events"] != 9 || r.Counts["foreground_event_context_snapshot"] != 3 || r.Counts["foreground_event_user_message"] != 3 || r.Counts["foreground_event_assistant_message"] != 3 {
				t.Fatalf("persisted foreground interval omitted an event class: %+v", r.Counts)
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
			if mode == "new" && r.Counts["completed_events"] != r.Counts["foreground_persisted_events"] {
				t.Fatalf("arrival/completion units differ: %+v", r.Counts)
			}
			if mode == "history" && (r.Counts["jobs_failed"] != 1 || r.Counts["jobs_completed_empty"] != 1 || r.Counts["jobs_completed_candidates"] != 4) {
				t.Fatalf("failed gap/zero-candidate/later progress: %+v", r.Counts)
			}
			// Reproduce the diagnostic shape discovered in the first matrix:
			// ordinary work completed, with an additional zero-attempt failure.
			// It must invalidate the trial while preserving measured evidence.
			var unexpected memory.CompilerDiagnosticJob
			unexpected.JobID, unexpected.State, unexpected.Reason = "unexpected-empty-selection", "failed", "unavailable_detail"
			r.Jobs = append(r.Jobs, unexpected)
			if err := validateFixtureOutcomes(&r); err == nil || len(r.OutcomeFailures) == 0 || len(r.Foreground) != 3 || len(r.Dispatches) == 0 {
				t.Fatalf("unexpected failed obligation passed or evidence was lost: %v, %+v", err, r)
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
