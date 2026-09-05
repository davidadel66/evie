package eviedb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// These tests cross actual process lifetimes through public Store entry points.
// The only persistence mutations outside those entry points move durable lease
// and retry deadlines relative to SQLite's clock. They do not manufacture work,
// capacity acknowledgements, review effects, or completion receipts.
func TestStage4ProcessConformanceWorkerRecovery(t *testing.T) {
	for _, failure := range []string{"process_crash", "expired_late_delivery", "cancelled_late_delivery"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			f, options, job := stage4ProcessWorkerFixture(t)
			options.Wait = true
			options.Crash = failure == "process_crash"
			old := stage4StartProcess(t, options)
			old.waitFile(t, "ready.json")
			reservation := stage4ProcessReservation(t, f.db)
			if options.Crash {
				old.wait(t, 86)
			}
			// Cancellation and lease recovery must both fence the original client
			// without treating that local action as proof of remote completion.
			if failure == "cancelled_late_delivery" {
				if _, err := f.store.CancelCompilation(ctx, f.owner, job.JobID); err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.ResumeCompilation(ctx, f.owner, job.JobID); err != nil {
					t.Fatal(err)
				}
			} else if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET lease_until=unixepoch('now')-1 WHERE job_id=?`, job.JobID); err != nil {
				t.Fatal(err)
			}
			probe := options
			probe.Wait, probe.Crash = false, false
			blocked := stage4StartProcess(t, probe).result(t)
			if blocked.Worked || blocked.Calls != 0 || blocked.Error != "capacity_blocked" {
				t.Fatalf("unknown completion admitted replacement: %+v", blocked)
			}
			inspection, err := f.store.InspectCompilation(ctx, f.owner, job.JobID)
			if err != nil || inspection.Attempts != 1 || inspection.CapacityState != "release_pending" || len(inspection.Candidates) != 0 {
				t.Fatalf("recovery: state=%s attempts=%d capacity=%s candidates=%d error=%v", inspection.State, inspection.Attempts, inspection.CapacityState, len(inspection.Candidates), err)
			}
			if got := stage4ProcessReservation(t, f.db); got != reservation {
				t.Fatalf("recovery changed dispatch identity: %+v", got)
			}
			if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET retry_at=unixepoch('now') WHERE job_id=? AND state='retry_wait'`, job.JobID); err != nil {
				t.Fatal(err)
			}
			// A completed scripted server request is separate from delivery of its
			// response. The old transport remains gated while this verifier checks
			// its exact durable dispatch against its completion record.
			probe.CompletionFile = filepath.Join(old.dir, "ready.json")
			wrong := reservation
			wrong.Fence++
			probe.Acknowledgement = &wrong
			bad := stage4StartProcess(t, probe).result(t)
			if bad.Worked || bad.Calls != 0 || bad.Error != "capacity_blocked" {
				t.Fatalf("mismatched trusted acknowledgement released capacity: %+v", bad)
			}
			probe.Acknowledgement, probe.Wait = &reservation, true
			replacement := stage4StartProcess(t, probe)
			replacement.waitFile(t, "ready.json")
			current := stage4ProcessReservation(t, f.db)
			if current.RequestID == reservation.RequestID || current.Holder == reservation.Holder || current.Fence <= reservation.Fence {
				t.Fatalf("replacement reused expired dispatch: old=%+v current=%+v", reservation, current)
			}
			if !options.Crash {
				old.release(t)
				late := old.result(t)
				if !late.Worked || late.Calls != 1 || late.Error != "fenced" {
					t.Fatalf("stale successful response escaped fence: %+v", late)
				}
				if got := stage4ProcessReservation(t, f.db); got != current {
					t.Fatalf("stale response released replacement capacity: %+v", got)
				}
			}
			inspection, err = f.store.InspectCompilation(ctx, f.owner, job.JobID)
			if err != nil || inspection.State != "running" || inspection.Attempts != 2 || len(inspection.Candidates) != 0 {
				t.Fatalf("replacement: state=%s attempts=%d candidates=%d error=%v", inspection.State, inspection.Attempts, len(inspection.Candidates), err)
			}
			replacement.release(t)
			completed := replacement.result(t)
			if !completed.Worked || completed.Calls != 1 || completed.Error != "" {
				t.Fatalf("replacement did not publish: %+v", completed)
			}
			// A fresh process observes the durable completion without dispatching;
			// reselecting the same finite source interval preserves its receipt.
			probe.Wait, probe.Acknowledgement = false, nil
			duplicate := stage4StartProcess(t, probe).result(t)
			if duplicate.Worked || duplicate.Calls != 0 || duplicate.Error != "" {
				t.Fatalf("duplicate worker repeated completed extraction: %+v", duplicate)
			}
			repeated, err := f.store.QueueCandidateUnit(ctx, f.owner, job.Window.Selection, f.generation, &stage4ProcessExtractor{})
			if err != nil || repeated.JobID != job.JobID || repeated.Attempts != 2 || repeated.State != "completed_candidates" || len(repeated.Candidates) != 1 {
				t.Fatalf("reselection: job=%s state=%s attempts=%d candidates=%d error=%v", repeated.JobID, repeated.State, repeated.Attempts, len(repeated.Candidates), err)
			}
			candidate := repeated.Candidates[0]
			if candidate.ReviewState != "unresolved" || candidate.Proposal.Proposition.SubjectEntityID != options.Subject || candidate.Proposal.Proposition.PredicateID != options.Predicate || len(candidate.Support) != 1 || candidate.Support[0].Locator.EventID != job.Window.Selection.RootID || candidate.Support[0].Authority != "owner_statement" {
				t.Fatal("replacement changed the exact unreviewed proposition or source authority")
			}
			var capacity, receipts, groups, coverage, stages, accepted int
			if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_compiler_capacity),(SELECT count(*) FROM memory_compiler_release_receipts),(SELECT count(*) FROM memory_compiler_candidate_groups),(SELECT count(*) FROM memory_compiler_coverage),(SELECT count(*) FROM memory_compiler_stages WHERE consumed=1),(SELECT count(*) FROM semantic_operations WHERE schema_version=6)`).Scan(&capacity, &receipts, &groups, &coverage, &stages, &accepted); err != nil {
				t.Fatal(err)
			}
			if capacity != 0 || receipts != 1 || groups != 1 || coverage != 1 || stages != 1 || accepted != 0 {
				t.Fatalf("durable exactly-once publication: capacity=%d receipts=%d groups=%d coverage=%d stages=%d accepted=%d", capacity, receipts, groups, coverage, stages, accepted)
			}
			var receipt CompilerCapacityReservation
			var kind string
			if err := f.db.QueryRow(`SELECT request_id,job_id,fence,holder,server_identity,kind FROM memory_compiler_release_receipts`).Scan(&receipt.RequestID, &receipt.JobID, &receipt.Fence, &receipt.Holder, &receipt.ServerIdentity, &kind); err != nil {
				t.Fatal(err)
			}
			if receipt != reservation || kind != "request_completed" {
				t.Fatal("release receipt no longer identifies the exact completed original request")
			}
			var publishedFence int64
			if err := f.db.QueryRow(`SELECT fence FROM memory_compiler_stages WHERE job_id=?`, job.JobID).Scan(&publishedFence); err != nil || publishedFence != current.Fence {
				t.Fatalf("published output did not belong to the replacement fence: %d %v", publishedFence, err)
			}
			stage4ProcessAssertReview(t, f.db, 0)
			assertions := map[string]bool{"real_processes": true, "durable_attempts": true, "unknown_capacity_blocked": true, "mismatched_ack_blocked": true, "exact_release_verified": true, "single_candidate_publication": true, "exact_source_authority": true, "duplicate_dispatch_absent": true, "unaccepted_graph_unchanged": true}
			if !options.Crash {
				assertions["stale_delivery_fenced"] = true
				assertions["replacement_capacity_preserved"] = true
			}
			stage4ProcessEvidence(t, failure, assertions)
		})
	}
}

func TestStage4ProcessConformanceReviewCommitRecovery(t *testing.T) {
	for _, phase := range []string{"before_commit", "after_commit"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			f, authority, refs := batchBoundaryFixture(t, 1)
			if err := f.store.ReleaseTurnLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken); err != nil {
				t.Fatal(err)
			}
			preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, refs[0], "accept")
			if err != nil {
				t.Fatal(err)
			}
			decision := memory.ReviewDecision{DeliveryKey: "idem:v1:90000000-0000-4000-8000-000000009149", PreviewID: preview.ID, PreviewSHA256: preview.SHA256, Action: "accept"}
			options := stage4ProcessOptions{Mode: "review", DB: f.path, Decision: decision, Phase: phase}
			stage4StartProcess(t, options).wait(t, 86)
			want := 0
			if phase == "after_commit" {
				want = 1
			}
			stage4ProcessAssertReview(t, f.db, want)
			// Both contenders open the ordinary public database path themselves,
			// obtain current owner authority, then start on one shared gate. No
			// process inherits a SQLite connection or an in-memory receipt.
			options.Phase, options.Wait = "", true
			options.Gate = filepath.Join(t.TempDir(), "review-go")
			first, second := stage4StartProcess(t, options), stage4StartProcess(t, options)
			first.waitFile(t, "ready.json")
			second.waitFile(t, "ready.json")
			stage4ProcessWrite(t, options.Gate, true)
			one, two := first.result(t), second.result(t)
			if one.Error != "" || two.Error != "" || one.Review.Operation == nil || !reflect.DeepEqual(one.Review, two.Review) {
				t.Fatalf("competing delivery receipts diverged: %+v %+v", one, two)
			}
			if one.Review.Operation.OperationID != preview.Effect.OperationID || len(one.Review.Operation.ClaimIDs) != 1 || len(one.Review.Operation.SourceLinkIDs) != 1 || one.Review.Action != "accept" {
				t.Fatalf("recovered a different accepted effect: %+v", one.Review)
			}
			stage4ProcessAssertReview(t, f.db, 1)
			options.Wait = false
			options.Decision.DeliveryKey = "idem:v1:90000000-0000-4000-8000-000000019149"
			resolved := stage4StartProcess(t, options).result(t)
			if resolved.Error != "already_resolved" || !reflect.DeepEqual(resolved.Review, one.Review) {
				t.Fatalf("new delivery hid prior durable decision: %+v", resolved)
			}
			stage4ProcessAssertReview(t, f.db, 1)
			item, err := f.store.InspectOwnerCandidate(ctx, authority, refs[0].ID)
			if err != nil || item.Candidate.ReviewState != "accepted" || item.Ref.ReviewRevision != refs[0].ReviewRevision+1 {
				t.Fatalf("candidate resolution: state=%s revision=%d error=%v", item.Candidate.ReviewState, item.Ref.ReviewRevision, err)
			}
			verified, err := f.store.VerifySemanticProjection(ctx)
			if err != nil || !verified.Valid {
				t.Fatalf("accepted process recovery cannot replay: %+v %v", verified, err)
			}
			stage4ProcessEvidence(t, phase, map[string]bool{"real_process_exit_at_review_commit": true, "atomic_effects": true, "competing_duplicate_receipts_equal": true, "single_canonical_operation": true, "prior_resolution_visible": true, "canonical_replay_valid": true})
		})
	}
}

func stage4ProcessWorkerFixture(t *testing.T) (*workerFixture, stage4ProcessOptions, memory.Compilation) {
	t.Helper()
	ctx := context.Background()
	f := newWorkerFixture(t)
	source, err := f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink water."})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := f.store.PrepareRememberLiteral(ctx, f.owner, memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000001149", SourceEventID: source.ID, Predicate: "drinks", PredicateLabel: "drinks", PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "water"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, seed); err != nil {
		t.Fatal(err)
	}
	job := f.queue(t, "I drink tea.")
	if err = f.store.ReleaseTurnLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	return f, stage4ProcessOptions{Mode: "worker", DB: f.path, Subject: seed.Subject.ID, Predicate: seed.Predicate.ID}, job
}

func stage4ProcessReservation(t *testing.T, db *sql.DB) CompilerCapacityReservation {
	t.Helper()
	var r CompilerCapacityReservation
	if err := db.QueryRow(`SELECT request_id,job_id,fence,holder,server_identity FROM memory_compiler_capacity`).Scan(&r.RequestID, &r.JobID, &r.Fence, &r.Holder, &r.ServerIdentity); err != nil {
		t.Fatal(err)
	}
	return r
}

func stage4ProcessAssertReview(t *testing.T, db *sql.DB, accepted int) {
	t.Helper()
	var operations, claims, sources, audits, receipts, resolutions, revision int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM semantic_operations WHERE schema_version=6),(SELECT count(*) FROM semantic_claims),(SELECT count(*) FROM semantic_source_links),(SELECT count(*) FROM memory_review_audits),(SELECT count(*) FROM memory_review_deliveries),(SELECT count(*) FROM memory_review_resolutions),(SELECT revision FROM semantic_scopes WHERE scope_key='global')`).Scan(&operations, &claims, &sources, &audits, &receipts, &resolutions, &revision); err != nil {
		t.Fatal(err)
	}
	if operations != accepted || claims != 1+accepted || sources != 1+accepted || audits != accepted || receipts != accepted || resolutions != accepted || revision != 1+accepted {
		t.Fatalf("partial or duplicate review commit: operations=%d claims=%d sources=%d audits=%d receipts=%d resolutions=%d revision=%d; accepted=%d", operations, claims, sources, audits, receipts, resolutions, revision, accepted)
	}
}

type stage4ProcessOptions struct {
	Mode, DB, Dir, Gate, Phase, CompletionFile string
	Wait, Crash                                bool
	Subject, Predicate                         memory.SemanticID
	Acknowledgement                            *CompilerCapacityReservation
	Decision                                   memory.ReviewDecision
}

type stage4ProcessResult struct {
	Worked bool                `json:"worked"`
	Calls  int                 `json:"calls"`
	Error  string              `json:"error"`
	Review memory.ReviewResult `json:"review"`
}

type stage4ProcessCompletion struct {
	RequestID, ServerIdentity string
}

type stage4ProcessExtractor struct {
	options stage4ProcessOptions
	calls   int
}

func (*stage4ProcessExtractor) ServerIdentity() string { return "scripted:stage4-process" }

func (e *stage4ProcessExtractor) Extract(_ context.Context, _ memory.CompilerGeneration, request memory.CompilerRequest) (CompilerExtraction, error) {
	e.calls++
	var support memory.EvidenceLocator
	for _, source := range request.Window.Sources {
		if source.Usage == "new_support" {
			support = source.Locator
			break
		}
	}
	raw, err := json.Marshal(memory.CompilerResponse{RequestID: request.ID, Candidates: []memory.ExtractorCandidate{{Proposition: memory.ClaimProposition{SubjectEntityID: e.options.Subject, PredicateID: e.options.Predicate, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}}, Polarity: memory.PolarityAffirmed}, Support: []memory.EvidenceLocator{support}, Context: []memory.EvidenceLocator{}}}})
	if err != nil {
		return CompilerExtraction{}, err
	}
	if err := stage4WriteJSON(filepath.Join(e.options.Dir, "ready.json"), stage4ProcessCompletion{RequestID: request.AttemptID, ServerIdentity: e.ServerIdentity()}); err != nil {
		return CompilerExtraction{}, err
	}
	if e.options.Crash {
		os.Exit(86) // Real process death: no client cleanup or SQLite defers run.
	}
	if e.options.Wait {
		// This transport deliberately ignores client cancellation: the scripted
		// server has completed but the response can arrive after replacement.
		if err := stage4WaitFile(filepath.Join(e.options.Dir, "go")); err != nil {
			return CompilerExtraction{}, err
		}
	}
	return CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}, nil
}

type stage4ProcessVerifier struct{ options stage4ProcessOptions }

func (v stage4ProcessVerifier) VerifyCompilerRelease(_ context.Context, reservation CompilerCapacityReservation) (CompilerReleaseAcknowledgement, error) {
	var completed stage4ProcessCompletion
	raw, err := os.ReadFile(v.options.CompletionFile)
	if err != nil {
		return CompilerReleaseAcknowledgement{}, err
	}
	if err = json.Unmarshal(raw, &completed); err != nil {
		return CompilerReleaseAcknowledgement{}, err
	}
	if completed.RequestID != reservation.RequestID || completed.ServerIdentity != reservation.ServerIdentity {
		return CompilerReleaseAcknowledgement{}, ErrCompilerCapacityBlocked
	}
	return CompilerReleaseAcknowledgement{Reservation: *v.options.Acknowledgement, Kind: "request_completed"}, nil
}

func TestStage4ProcessConformanceChild(t *testing.T) {
	optionsFile := os.Getenv("EVIE_STAGE4_PROCESS_OPTIONS")
	if optionsFile == "" {
		return
	}
	var options stage4ProcessOptions
	stage4ProcessRead(t, optionsFile, &options)
	ctx := context.Background()
	db, err := OpenDBAt(options.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	var result stage4ProcessResult
	switch options.Mode {
	case "worker":
		extractor := &stage4ProcessExtractor{options: options}
		generationID, _, err := memory.CompilerGenerationIdentity(compilerCommitGeneration())
		if err != nil {
			t.Fatal(err)
		}
		config := CompilerSupervisorConfig{Extractors: map[string]CompilerExtractor{generationID: extractor}}
		if options.Acknowledgement != nil {
			config.ReleaseVerifier = stage4ProcessVerifier{options: options}
		}
		result.Worked, err = store.RunCompilerStep(ctx, config)
		result.Calls, result.Error = extractor.calls, stage4ProcessError(t, err)
	case "review":
		authority, err := store.LocalOwnerReviewContext(ctx, "global")
		if err != nil {
			t.Fatal(err)
		}
		if options.Wait {
			stage4ProcessWrite(t, filepath.Join(options.Dir, "ready.json"), true)
			if err := stage4WaitFile(options.Gate); err != nil {
				t.Fatal(err)
			}
		}
		if options.Phase != "" {
			// Install the existing transaction-resolution seam only after owner
			// authentication, so this exit belongs to acceptance itself.
			store.resolveImmediateTransaction = func(ctx context.Context, conn *sql.Conn, statement string) (sql.Result, error) {
				if statement == "COMMIT" {
					var n int
					if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM memory_review_deliveries WHERE delivery_key=?`, options.Decision.DeliveryKey).Scan(&n); err != nil {
						return nil, err
					}
					if n != 1 {
						return nil, errors.New("review crash seam did not reach its delivery")
					}
					if options.Phase == "after_commit" {
						if _, err := executeImmediateTransactionStatement(ctx, conn, statement); err != nil {
							return nil, err
						}
					}
					os.Exit(86)
				}
				return executeImmediateTransactionStatement(ctx, conn, statement)
			}
		}
		result.Review, err = store.ResolveOwnerCandidateReview(ctx, authority, options.Decision)
		result.Error = stage4ProcessError(t, err)
	default:
		t.Fatal("unknown process mode")
	}
	stage4ProcessWrite(t, filepath.Join(options.Dir, "result.json"), result)
}

func stage4ProcessError(t *testing.T, err error) string {
	t.Helper()
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrCompilerCapacityBlocked):
		return "capacity_blocked"
	case errors.Is(err, ErrCompilerFence):
		return "fenced"
	case errors.Is(err, ErrReviewResolved):
		return "already_resolved"
	default:
		t.Fatalf("unexpected public operation failure: %v", err)
		return "unexpected"
	}
}

type stage4Process struct {
	dir    string
	cmd    *exec.Cmd
	done   chan struct{}
	err    error
	output bytes.Buffer
}

func stage4StartProcess(t *testing.T, options stage4ProcessOptions) *stage4Process {
	t.Helper()
	p := &stage4Process{dir: t.TempDir(), done: make(chan struct{})}
	options.Dir = p.dir
	optionsFile := filepath.Join(p.dir, "options.json")
	stage4ProcessWrite(t, optionsFile, options)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	p.cmd = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStage4ProcessConformanceChild$", "-test.timeout=35s")
	p.cmd.Env = append(os.Environ(), "EVIE_STAGE4_PROCESS_OPTIONS="+optionsFile)
	p.cmd.Stdout, p.cmd.Stderr = &p.output, &p.output
	if err := p.cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	t.Cleanup(func() { cancel(); <-p.done })
	return p
}

func (p *stage4Process) wait(t *testing.T, code int) {
	t.Helper()
	<-p.done
	if p.cmd.ProcessState == nil || p.cmd.ProcessState.ExitCode() != code {
		t.Fatalf("child exit: %v; want %d\n%s", p.err, code, p.output.String())
	}
}

func (p *stage4Process) waitFile(t *testing.T, name string) {
	t.Helper()
	if err := stage4WaitFile(filepath.Join(p.dir, name)); err != nil {
		_ = p.cmd.Process.Kill()
		<-p.done
		t.Fatalf("child handshake: %v\n%s", err, p.output.String())
	}
}

func (p *stage4Process) release(t *testing.T) {
	t.Helper()
	stage4ProcessWrite(t, filepath.Join(p.dir, "go"), true)
}

func (p *stage4Process) result(t *testing.T) stage4ProcessResult {
	t.Helper()
	p.wait(t, 0)
	var result stage4ProcessResult
	stage4ProcessRead(t, filepath.Join(p.dir, "result.json"), &result)
	return result
}

func stage4WaitFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func stage4ProcessRead(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func stage4ProcessWrite(t *testing.T, path string, value any) {
	t.Helper()
	if err := stage4WriteJSON(path, value); err != nil {
		t.Fatal(err)
	}
}

func stage4WriteJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", raw, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func stage4ProcessEvidence(t *testing.T, scenario string, assertions map[string]bool) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"version": "memory-stage-4-conformance-v1", "scenario": scenario, "assertions": assertions})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("STAGE4_EVIDENCE " + string(raw))
}
