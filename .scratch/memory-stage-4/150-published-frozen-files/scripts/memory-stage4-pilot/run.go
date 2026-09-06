package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type report struct {
	Version         string                                 `json:"version"`
	Kind            string                                 `json:"kind"`
	Workload        workload                               `json:"workload"`
	StartedAt       time.Time                              `json:"started_at"`
	Environment     map[string]string                      `json:"environment"`
	Generation      memory.CompilerGeneration              `json:"generation"`
	GenerationID    string                                 `json:"generation_id"`
	SetupNanos      int64                                  `json:"setup_nanos"`
	ObservedNanos   int64                                  `json:"observed_nanos"`
	Foreground      []memory.CompilerForegroundMeasurement `json:"foreground"`
	Jobs            []memory.CompilerDiagnosticJob         `json:"jobs"`
	Candidates      []memory.CompilerDiagnosticCandidate   `json:"candidates"`
	ResolutionNanos []int64                                `json:"scripted_resolution_nanos"`
	Counts          map[string]int64                       `json:"counts"`
	ExpectedCounts  map[string]int64                       `json:"expected_counts"`
	OutcomeFailures []string                               `json:"outcome_failures"`
	StorageBefore   map[string]int64                       `json:"storage_before"`
	StorageAfter    map[string]int64                       `json:"storage_after"`
	WorkerPIDs      []int                                  `json:"worker_pids"`
	Dispatches      []dispatchObservation                  `json:"dispatches"`
	DiagnosticsAsOf int64                                  `json:"diagnostics_as_of_unix_ms"`
	Cleanup         bool                                   `json:"disposable_database_removed"`
	Error           string                                 `json:"error,omitempty"`
	Limitations     []string                               `json:"limitations"`
	ReleaseEligible bool                                   `json:"release_eligible"`
}

type child struct {
	cmd  *exec.Cmd
	wait chan error
}

func startWorker(ctx context.Context, directory string, x scriptedExtractor) (*child, error) {
	b, err := json.Marshal(x)
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(executable, "worker", filepath.Join(directory, "pilot.db"), string(b))
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	c := &child{cmd: cmd, wait: make(chan error, 1)}
	ready := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() || scanner.Text() != "ready" {
			ready <- errors.New("worker did not become ready")
			return
		}
		ready <- nil
		for scanner.Scan() {
		}
	}()
	go func() { c.wait <- cmd.Wait() }()
	select {
	case err = <-ready:
		if err != nil {
			cmd.Process.Kill()
			<-c.wait
			return nil, err
		}
		return c, nil
	case <-ctx.Done():
		cmd.Process.Kill()
		<-c.wait
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		<-c.wait
		return nil, errors.New("worker startup deadline")
	}
}
func (c *child) stop() error {
	if err := c.cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case err := <-c.wait:
		return err
	case <-time.After(10 * time.Second):
		c.cmd.Process.Kill()
		<-c.wait
		return errors.New("worker shutdown deadline")
	}
}
func workerCommand(ctx context.Context, args []string) error {
	if len(args) != 2 {
		return errors.New("worker requires a disposable database and scripted configuration")
	}
	var x scriptedExtractor
	if err := json.Unmarshal([]byte(args[1]), &x); err != nil {
		return err
	}
	db, err := eviedb.OpenDBAtContext(ctx, args[0])
	if err != nil {
		return err
	}
	defer db.Close()
	id, err := checkedID(generation())
	if err != nil {
		return err
	}
	fmt.Println("ready")
	err = eviedb.NewStore(db).RunCompilerHost(ctx, eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: x}})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func runWorkload(ctx context.Context, w workload) (r report, retErr error) {
	r = report{Version: "memory-stage4-pilot-infrastructure-v1", Kind: "scripted_infrastructure_only", Workload: w, StartedAt: time.Now().UTC(), Environment: map[string]string{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "foreground_provider": "scripted constant acknowledgement", "response_host": "actual os.DevNull Write; not browser/SSE/network latency", "model_server": "absent; scripted extraction is inside each worker process"}, Generation: generation(), Counts: map[string]int64{}, Foreground: []memory.CompilerForegroundMeasurement{}, Jobs: []memory.CompilerDiagnosticJob{}, Candidates: []memory.CompilerDiagnosticCandidate{}, ResolutionNanos: []int64{}, Limitations: []string{"No learned model, human semantic adjudication, active David review time, or release threshold is measured.", "Archived event bulk fixture setup is outside measured intervals; foreground sessions do not load archived history.", "Scripted service delay varies occupied runtime capacity; it does not predict a model's inference throughput.", "All proposed semantic content and scripted owner decisions are artificial load. Acceptance rate is not quality.", "Resource sampling and paired repetition metadata belong to the matrix wrapper receipt."}}
	dir, err := os.MkdirTemp("", "evie-stage4-pilot-")
	if err != nil {
		return r, err
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			retErr = errors.Join(retErr, err)
		} else {
			r.Cleanup = true
		}
	}()
	path := filepath.Join(dir, "pilot.db")
	db, err := eviedb.OpenDBAtContext(ctx, path)
	if err != nil {
		return r, err
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	var sqlite, journal string
	if err = db.QueryRowContext(ctx, `SELECT sqlite_version()`).Scan(&sqlite); err != nil {
		return r, err
	}
	if err = db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return r, err
	}
	r.Environment["sqlite"] = sqlite
	r.Environment["journal_mode"] = journal
	if err = seedRetained(ctx, db, store, w.RetainedEvents); err != nil {
		return r, err
	}
	x, err := seedGraph(ctx, store, w.GraphClaims)
	if err != nil {
		return r, err
	}
	x.DelayMS = w.DelayMS
	x.AuditPath = filepath.Join(dir, "dispatches.jsonl")
	id, err := checkedID(r.Generation)
	if err != nil {
		return r, err
	}
	r.GenerationID = id
	sessions := make([]memory.Session, w.Scopes)
	for i := range sessions {
		sessions[i], err = store.CreateGlobalSession(ctx)
		if err != nil {
			return r, err
		}
	}
	// Identical warmup and backfill source histories exist in all three modes;
	// only explicit selection differs. This keeps foreground inputs paired.
	for i, session := range sessions {
		roots := 0
		if i == 0 {
			roots = w.BackfillRoots
		}
		var first, last memory.Event
		for n := 0; n < roots; n++ {
			input := fixtureInput(100+n, w.SourceBytes)
			if n == 0 {
				input = "PILOT_FAILED_GAP " + input
			}
			if n == 1 {
				input = "PILOT_ZERO_CANDIDATES " + input
			}
			a, b, e := appendTurn(ctx, store, session, input)
			if e != nil {
				return r, e
			}
			if n == 0 {
				first = a
			}
			last = b
		}
		if w.Mode == "history" && roots > 0 {
			var receipt memory.CompilerHistoryReceipt
			receipt, err = store.SelectCompilerHistory(ctx, []memory.ScopeContext{session.ScopeContext()}, memory.CompilerHistoryRequest{RequestID: fmt.Sprintf("pilot-history-%d", i), Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: destination(session), SessionID: session.ID, FirstSequence: first.Sequence, LastSequence: last.Sequence, FirstEventID: first.ID, LastEventID: last.ID}}}, r.Generation, x)
			if err != nil {
				return r, err
			}
			r.Counts["history_selected_events"] += receipt.SelectedEvents
		}
		if w.Mode != "disabled" {
			_, err = store.ActivateCompiler(ctx, session.ScopeContext(), memory.CompilerActivationRequest{RequestID: fmt.Sprintf("pilot-live-%d", i), Selector: memory.CompilerLiveSelector{SourceScope: "global", SessionID: session.ID, Destination: destination(session)}}, r.Generation, x)
			if err != nil {
				return r, err
			}
		}
	}
	// Foreground source sessions start with the same fixed context in every mode.
	// The first measured input launches new work; subsequent turns overlap it.
	if _, err = db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return r, err
	}
	r.StorageBefore = storage(path)
	r.SetupNanos = time.Since(r.StartedAt).Nanoseconds()
	var children []*child
	defer func() {
		for _, c := range children {
			retErr = errors.Join(retErr, c.stop())
		}
	}()
	started := time.Now()
	if w.Mode != "disabled" {
		for n := 0; n < w.Processes; n++ {
			c, e := startWorker(ctx, dir, x)
			if e != nil {
				return r, e
			}
			children = append(children, c)
			r.WorkerPIDs = append(r.WorkerPIDs, c.cmd.Process.Pid)
		}
	}
	profile, err := openrouter.NewExplicitContextProfile("scripted-foreground", 1048576, 524288, 1024)
	if err != nil {
		return r, err
	}
	output, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return r, err
	}
	defer output.Close()
	foregroundStarts := make([]int64, len(sessions))
	for i, session := range sessions {
		if err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM events WHERE session_id=?`, session.ID).Scan(&foregroundStarts[i]); err != nil {
			return r, err
		}
	}
	foregroundStarted := time.Now()
	for n := 0; n < w.Turns; n++ {
		session := sessions[n%len(sessions)]
		holder := memory.LeaseHolderID("pilot-foreground")
		a := agent.NewWithToolset(foregroundClient{}, profile, store.BindHistory(session.ID, holder), session.ScopeContext(), store.BindTurnOwner(session.ID, holder), tools.NewToolset(nil))
		measured, finish := agent.BeginResponseMeasurement(ctx)
		ev := &foregroundEvents{}
		if err = a.Send(measured, fixtureInput(n, w.SourceBytes), ev, nil); err != nil {
			return r, err
		}
		_, writeErr := output.WriteString(ev.output.String() + "\n")
		if err = finish(writeErr); err != nil {
			return r, err
		}
		if writeErr != nil {
			return r, writeErr
		}
		// A fixed 50ms arrival interval allows production's 25ms reconciler to
		// compete with the next foreground turn in every mode.
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	r.Counts["foreground_elapsed_nanos"] = time.Since(foregroundStarted).Nanoseconds()
	// Coverage counts every committed member, including context snapshots that
	// cannot support a memory. Count the actual foreground interval in the same
	// persisted-event units; do not infer cardinality from the number of turns.
	for i, session := range sessions {
		rows, err := db.QueryContext(ctx, `SELECT event_type,COUNT(*) FROM events WHERE session_id=? AND sequence>? GROUP BY event_type`, session.ID, foregroundStarts[i])
		if err != nil {
			return r, err
		}
		for rows.Next() {
			var eventType string
			var count int64
			if err = rows.Scan(&eventType, &count); err != nil {
				rows.Close()
				return r, err
			}
			r.Counts["foreground_persisted_events"] += count
			r.Counts["foreground_event_"+eventType] += count
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil {
			return r, err
		}
	}
	if w.Mode != "disabled" {
		want := w.Turns
		if w.Mode == "history" {
			want += w.BackfillRoots
		}
		deadline := time.Now().Add(90 * time.Second)
		for {
			var total, unfinished, undiscovered, unmaterialized int
			if err = db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(state NOT IN ('completed_candidates','completed_empty','failed','cancelled')),0) FROM memory_compiler_jobs`).Scan(&total, &unfinished); err != nil {
				return r, err
			}
			if err = db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM memory_compiler_activation_dirty WHERE scanned_position<high_position),(SELECT COUNT(*) FROM memory_compiler_activation_roots WHERE state IN ('selected_unmaterialized','deferred_live'))`).Scan(&undiscovered, &unmaterialized); err != nil {
				return r, err
			}
			if total >= want && unfinished == 0 && undiscovered == 0 && unmaterialized == 0 {
				break
			}
			if time.Now().After(deadline) {
				return r, fmt.Errorf("catch-up deadline: jobs=%d expected=%d unfinished=%d undiscovered=%d unmaterialized=%d", total, want, unfinished, undiscovered, unmaterialized)
			}
			select {
			case <-ctx.Done():
				return r, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	r.ObservedNanos = time.Since(started).Nanoseconds()
	for _, session := range sessions {
		a, err := store.LocalOwnerReviewContext(ctx, destination(session))
		if err != nil {
			return r, err
		}
		for _, view := range []string{"foreground", "jobs", "candidates"} {
			q := memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: view, Limit: 32}
			for pages := 0; ; pages++ {
				if pages > 128 {
					return r, errors.New("diagnostic pagination exceeded workload bound")
				}
				d, e := store.InspectOwnerCompilerDiagnostics(ctx, a, q)
				if e != nil {
					return r, e
				}
				if d.Indexing {
					return r, errors.New("diagnostic totals remain incomplete")
				}
				r.DiagnosticsAsOf = max(r.DiagnosticsAsOf, d.AsOfUnixMS)
				r.Foreground = append(r.Foreground, d.Foreground...)
				r.Jobs = append(r.Jobs, d.Jobs...)
				r.Candidates = append(r.Candidates, d.Candidates...)
				if d.NextCursor == "" {
					break
				}
				q.Cursor = d.NextCursor
			}
		}
		// Measure actual public preview/resolve cost, independently of human time.
		page, err := store.ListOwnerCandidates(ctx, a, memory.OwnerCandidateQuery{Limit: 32})
		if err != nil {
			return r, err
		}
		for n, candidate := range page.Candidates {
			if n >= 2 {
				break
			}
			start := time.Now()
			action := "accept"
			if n == 1 {
				action = "reject"
			}
			preview, e := store.PrepareOwnerCandidateReview(ctx, a, candidate.Ref, action)
			if e != nil {
				return r, e
			}
			_, e = store.ResolveOwnerCandidateReview(ctx, a, memory.ReviewDecision{DeliveryKey: fmt.Sprintf("idem:v1:90000000-0000-4000-8000-%012d", 1500000+len(r.ResolutionNanos)), PreviewID: preview.ID, PreviewSHA256: preview.SHA256, Action: action, Reason: "Scripted infrastructure resolution; no human quality judgment."})
			if e != nil {
				return r, e
			}
			r.ResolutionNanos = append(r.ResolutionNanos, time.Since(start).Nanoseconds())
		}
	}
	if len(r.Foreground) != w.Turns {
		return r, fmt.Errorf("foreground observation count %d != %d", len(r.Foreground), w.Turns)
	}
	for _, m := range r.Foreground {
		if m.TerminalCommitNanos == nil || m.ResponseFinalizationNanos == nil || m.Outcome != "success" {
			return r, errors.New("missing actual terminal/finalization observation")
		}
	}
	for _, job := range r.Jobs {
		r.Counts["jobs_"+job.State]++
		r.Counts["attempts"] += int64(job.Attempts)
		r.Counts["selected_events"] += job.SelectedNewEvents
		r.Counts["completed_events"] += job.CompletedNewEvents
		for _, m := range job.Measurements {
			r.Counts["observed_attempt_"+m.ObservedOutcome]++
		}
	}
	r.Counts["candidates_before_scripted_review"] = int64(len(r.Candidates))
	for _, table := range []string{"events", "semantic_claims", "memory_compiler_candidates"} {
		var count int64
		if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return r, err
		}
		r.Counts["rows_"+table] = count
	}
	r.StorageAfter = storage(path)
	if w.Mode != "disabled" {
		file, err := os.Open(x.AuditPath)
		if err != nil {
			return r, err
		}
		decoder := json.NewDecoder(file)
		for decoder.More() {
			var d dispatchObservation
			if err = decoder.Decode(&d); err != nil {
				file.Close()
				return r, err
			}
			r.Dispatches = append(r.Dispatches, d)
		}
		if err = file.Close(); err != nil {
			return r, err
		}
		for i, a := range r.Dispatches {
			for j, b := range r.Dispatches {
				if j > i && a.StartedNS < b.FinishedNS && b.StartedNS < a.FinishedNS {
					return r, errors.New("cooperating scripted extraction intervals overlapped")
				}
			}
		}
	}
	err = validateFixtureOutcomes(&r)
	return r, err
}

// These are deterministic fixture obligations, not model-quality thresholds.
// Validate only after retaining raw diagnostics, storage and dispatch evidence.
func validateFixtureOutcomes(r *report) error {
	w := r.Workload
	jobs, failed, empty, selected := int64(0), int64(0), int64(0), int64(0)
	if w.Mode != "disabled" {
		jobs = int64(w.Turns)
		selected = r.Counts["foreground_persisted_events"]
		if w.Mode == "history" {
			jobs += int64(w.BackfillRoots)
			selected += r.Counts["history_selected_events"]
			if w.BackfillRoots > 0 {
				failed = 1
			}
			if w.BackfillRoots > 1 {
				empty = 1
			}
		}
	}
	r.ExpectedCounts = map[string]int64{"jobs": jobs, "jobs_failed": failed, "jobs_completed_empty": empty, "jobs_completed_candidates": jobs - failed - empty, "attempts": jobs, "selected_events": selected, "dispatches": jobs, "candidates_before_scripted_review": jobs - failed - empty}
	r.Counts["jobs"] = int64(len(r.Jobs))
	r.Counts["dispatches"] = int64(len(r.Dispatches))
	r.OutcomeFailures = []string{}
	for key, expected := range r.ExpectedCounts {
		if actual := r.Counts[key]; actual != expected {
			r.OutcomeFailures = append(r.OutcomeFailures, fmt.Sprintf("%s: observed %d, expected %d", key, actual, expected))
		}
	}
	seen := map[string]bool{}
	for _, job := range r.Jobs {
		if seen[job.JobID] || job.Attempts != 1 || job.SelectedNewEvents == 0 {
			r.OutcomeFailures = append(r.OutcomeFailures, fmt.Sprintf("unexpected duplicate, attempt count, or empty selection for job %s", job.JobID))
		}
		seen[job.JobID] = true
		switch job.State {
		case "completed_candidates", "completed_empty":
			if job.CompletedNewEvents != job.SelectedNewEvents {
				r.OutcomeFailures = append(r.OutcomeFailures, fmt.Sprintf("incomplete event coverage for completed job %s", job.JobID))
			}
		case "failed":
			if job.Reason != "invalid_source_or_effect" || job.CompletedNewEvents != 0 {
				r.OutcomeFailures = append(r.OutcomeFailures, fmt.Sprintf("unexpected failure reason or coverage for job %s", job.JobID))
			}
		default:
			r.OutcomeFailures = append(r.OutcomeFailures, fmt.Sprintf("unexpected job state %s for %s", job.State, job.JobID))
		}
	}
	if len(r.OutcomeFailures) != 0 {
		sort.Strings(r.OutcomeFailures)
		return fmt.Errorf("deterministic fixture outcome mismatch: %v", r.OutcomeFailures)
	}
	return nil
}

func destination(s memory.Session) string { return "session:" + string(s.ID) }
func storage(path string) map[string]int64 {
	r := map[string]int64{}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		name := "db" + suffix
		if s, e := os.Stat(path + suffix); e == nil {
			r[name] = s.Size()
		} else if os.IsNotExist(e) {
			r[name] = 0
		} else {
			r[name] = -1
		}
	}
	return r
}
