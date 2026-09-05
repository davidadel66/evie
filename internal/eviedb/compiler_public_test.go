package eviedb_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type scriptedCompiler struct {
	calls atomic.Int32
	run   func(context.Context, memory.CompilerRequest) (eviedb.CompilerExtraction, error)
}

func (s *scriptedCompiler) ServerIdentity() string { return "scripted:136" }
func (s *scriptedCompiler) Extract(ctx context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	s.calls.Add(1)
	return s.run(ctx, r)
}
func compilerGeneration() memory.CompilerGeneration {
	g := memory.CompilerGeneration{Version: "compiler-generation-v1", ModelArtifact: "scripted:test", ModelSHA256: strings.Repeat("1", 64), Quantization: "fixture", RuntimeVersion: "fixture", ProtocolVersion: "ollama-generate-v1", TokenizerSHA256: strings.Repeat("2", 64), Template: "{{.System}}\n{{.Prompt}}", Prompt: "Extract owner assertions only.", Schema: json.RawMessage(`{"type":"object"}`), TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8, Decoding: memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17}}
	g.ModelManifest = json.RawMessage(`{"layers":[{"mediaType":"application/vnd.ollama.image.model","digest":"sha256:` + g.ModelSHA256 + `"}]}`)
	g.ModelManifestSHA256 = memory.CompilerHash(g.ModelManifest)
	g.TemplateSHA256 = memory.CompilerHash([]byte(g.Template))
	g.EvidencePolicy = memory.CompilerPolicyVersion
	g.SecretPolicy = memory.CompilerPolicyVersion
	g.ClosurePolicy = memory.CompilerPolicyVersion
	g.WindowPolicy = memory.CompilerPolicyVersion
	g.PredicatePolicy = memory.CompilerPolicyVersion
	g.EntityPolicy = memory.CompilerPolicyVersion
	g.ValidationPolicy = memory.CompilerPolicyVersion
	g.EquivalencePolicy = memory.CompilerPolicyVersion
	g.EffectPolicy = memory.CompilerPolicyVersion
	return g
}

type compilerFixture struct {
	store              *eviedb.Store
	db                 *sql.DB
	path               string
	session            memory.Session
	lease              memory.TurnLease
	subject, predicate memory.SemanticID
}

func newCompilerFixture(t *testing.T) *compilerFixture {
	t.Helper()
	f := &compilerFixture{path: filepath.Join(t.TempDir(), "evie.db")}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	f.store = eviedb.NewStore(f.db)
	ctx := context.Background()
	f.session, err = f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.lease, err = f.store.AcquireTurnLease(ctx, f.session.ID, "compiler-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "/remember drink tea"})
	proposal, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000001", SourceEventID: source.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ApplyRememberLiteral(ctx, f.lease, proposal); err != nil {
		t.Fatal(err)
	}
	f.subject = proposal.Subject.ID
	f.predicate = proposal.Predicate.ID
	return f
}
func (f *compilerFixture) append(t *testing.T, input memory.EventInput) memory.Event {
	t.Helper()
	e, err := f.store.AppendEventWithLease(context.Background(), f.session.ID, f.lease.HolderID, f.lease.FencingToken, input)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func (f *compilerFixture) selection(t *testing.T, content string, closed bool) memory.CompilationSelection {
	t.Helper()
	root := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content})
	cutoff := root.Sequence
	if closed {
		last := f.append(t, memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Recorded."})
		cutoff = last.Sequence
	}
	return memory.CompilationSelection{SessionID: f.session.ID, RootID: root.ID, Cutoff: cutoff, Destination: "global"}
}
func (f *compilerFixture) candidate(r memory.CompilerRequest) memory.ExtractorCandidate {
	var source memory.CompilerSource
	for _, s := range r.Window.Sources {
		if s.Usage == "new_support" {
			source = s
			break
		}
	}
	return memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: f.subject, PredicateID: f.predicate, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}}, Polarity: memory.PolarityAffirmed}, ValidTime: memory.ValidTime{}, TemporalQualification: "", Support: []memory.EvidenceLocator{source.Locator}, Context: []memory.EvidenceLocator{}}
}
func compilerOutput(r memory.CompilerRequest, candidates []memory.ExtractorCandidate) eviedb.CompilerExtraction {
	b, _ := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: candidates})
	return eviedb.CompilerExtraction{Raw: b, ReleaseEvidence: "completed"}
}

func TestCompilerDurableUnacceptedGroupAndReopenIdempotency(t *testing.T) {
	f := newCompilerFixture(t)
	selection := f.selection(t, "I prefer café and tea.", true)
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		if len(r.Entities) == 0 || len(r.Predicates) == 0 {
			t.Fatal("missing accepted snapshot")
		}
		return compilerOutput(r, []memory.ExtractorCandidate{f.candidate(r)}), nil
	}}
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), selection, compilerGeneration(), extractor)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "completed_candidates" || len(result.Candidates) != 1 || result.Candidates[0].ReviewState != "unresolved" || result.Attempts != 1 {
		t.Fatalf("result %+v", result)
	}
	accepted, err := f.store.InspectLiteralClaims(context.Background(), f.session.ScopeContext())
	if err != nil || len(accepted.Claims) != 1 || accepted.ScopeRevision != 1 {
		t.Fatalf("accepted memory changed: %+v %v", accepted, err)
	}
	var coverage, groups int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_coverage WHERE outcome='completed_candidates'`).Scan(&coverage); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_candidate_groups`).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if coverage != 1 || groups != 1 {
		t.Fatalf("coverage=%d groups=%d", coverage, groups)
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	repeated, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), selection, compilerGeneration(), extractor)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.JobID != result.JobID || repeated.Candidates[0].ID != result.Candidates[0].ID || extractor.calls.Load() != 1 {
		t.Fatalf("duplicate output %+v calls%d", repeated, extractor.calls.Load())
	}
	wrong := f.session.ScopeContext()
	wrong.OwnerID = "other"
	leak, err := f.store.InspectCompilation(context.Background(), wrong, result.JobID)
	if err == nil || len(leak.Window.Sources) != 0 || len(leak.Candidates) != 0 || leak.JobID != "" {
		t.Fatalf("inspection leaked on error: %+v %v", leak, err)
	}
}

func TestCompilerEmptyMissingSecretAndLiveClosure(t *testing.T) {
	for _, test := range []struct {
		name, content string
		closed        bool
		output        string
		state         string
		calls         int32
	}{{"explicit empty", "I prefer tea", true, "empty", "completed_empty", 1}, {"missing", "I prefer tea", true, "missing", "retry_wait", 1}, {"null", "I prefer tea", true, "null", "retry_wait", 1}, {"secret", "I prefer tea and api_key=syntheticsecretvalue", true, "empty", "excluded", 0}, {"oversize", strings.Repeat("a", 32769), true, "empty", "failed", 0}, {"live", "I prefer tea", false, "empty", "deferred_live", 0}} {
		t.Run(test.name, func(t *testing.T) {
			f := newCompilerFixture(t)
			sel := f.selection(t, test.content, test.closed)
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				switch test.output {
				case "missing":
					return eviedb.CompilerExtraction{ReleaseEvidence: "completed"}, nil
				case "null":
					return compilerOutput(r, nil), nil
				default:
					return compilerOutput(r, []memory.ExtractorCandidate{}), nil
				}
			}}
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
			if test.state != "retry_wait" && err != nil {
				t.Fatal(err)
			}
			if result.State != test.state || extractor.calls.Load() != test.calls {
				t.Fatalf("state=%s calls=%d err=%v", result.State, extractor.calls.Load(), err)
			}
			if test.name == "secret" {
				encoded, _ := json.Marshal(result)
				if strings.Contains(string(encoded), "syntheticsecretvalue") {
					t.Fatal("secret leaked into inspection")
				}
			}
			if test.name == "live" {
				if err := f.store.ReleaseTurnLease(context.Background(), f.lease.SessionID, f.lease.HolderID, f.lease.FencingToken); err != nil {
					t.Fatal(err)
				}
				result, err = f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
				if err != nil || result.State != "completed_empty" || result.Window.Closure != "no_live_lease" {
					t.Fatalf("release closure %+v %v", result, err)
				}
			}
		})
	}
}

func TestCompilerRejectsUntrustedSourceIdentityAndShape(t *testing.T) {
	cases := []struct {
		name   string
		change func(*memory.ExtractorCandidate, memory.CompilerRequest)
	}{
		{"unoffered", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) { c.Support[0].EventID = "absent" }},
		{"hash", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) {
			c.Support[0].EvidenceSHA256 = strings.Repeat("0", 64)
		}},
		{"utf8", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) {
			c.Support[0].LocatorKind = memory.LocatorUTF8ByteRange
			c.Support[0].LocatorValue = "0:13"
			c.Support[0].EvidenceSHA256 = memory.CompilerHash([]byte("I prefer caf\xc3"))
		}},
		{"context authority", func(c *memory.ExtractorCandidate, r memory.CompilerRequest) {
			for _, s := range r.Window.Sources {
				if s.Usage == "context" {
					c.Support = []memory.EvidenceLocator{s.Locator}
				}
			}
		}},
		{"overlap only", func(c *memory.ExtractorCandidate, r memory.CompilerRequest) {
			for _, s := range r.Window.Sources {
				if s.Usage == "overlap" {
					c.Support = []memory.EvidenceLocator{s.Locator}
				}
			}
		}},
		{"identity", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) {
			c.Proposition.SubjectEntityID = "invented"
		}},
		{"predicate", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) { c.Proposition.PredicateID = "invented" }},
		{"polarity", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) { c.Proposition.Polarity = "maybe" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newCompilerFixture(t)
			sel := f.selection(t, "I prefer café and tea.", true)
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				candidate := f.candidate(r)
				test.change(&candidate, r)
				return compilerOutput(r, []memory.ExtractorCandidate{candidate}), nil
			}}
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
			wantState := "failed"
			if test.name == "polarity" {
				wantState = "retry_wait"
			}
			if err == nil || result.State != wantState || len(result.Candidates) != 0 {
				t.Fatalf("accepted invalid: %+v %v", result, err)
			}
		})
	}
}

func TestCompilerUnknownReleaseBlocksOtherJobsWithoutSecondDispatch(t *testing.T) {
	f := newCompilerFixture(t)
	first := f.selection(t, "I prefer tea", true)
	second := f.selection(t, "I prefer coffee", true)
	extractor := &scriptedCompiler{run: func(context.Context, memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return eviedb.CompilerExtraction{}, errors.New("transport timeout")
	}}
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), first, compilerGeneration(), extractor)
	if err == nil || result.CapacityState != "release_pending" {
		t.Fatalf("unknown release %+v %v", result, err)
	}
	_, err = f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), second, compilerGeneration(), extractor)
	if !errors.Is(err, eviedb.ErrCompilerCapacityBlocked) || extractor.calls.Load() != 1 {
		t.Fatalf("capacity bypass: %v calls%d", err, extractor.calls.Load())
	}
}

func TestCompilerCancellationFencesBeforeClientSignal(t *testing.T) {
	f := newCompilerFixture(t)
	sel := f.selection(t, "I prefer tea", true)
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	extractor := &scriptedCompiler{run: func(ctx context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		close(started)
		<-ctx.Done()
		var state string
		var fence int64
		if err := f.db.QueryRow(`SELECT state,fence FROM memory_compiler_jobs`).Scan(&state, &fence); err != nil {
			return eviedb.CompilerExtraction{}, err
		}
		if state != "cancelled" || fence != 2 {
			return eviedb.CompilerExtraction{}, errors.New("client cancelled before durable fence")
		}
		return compilerOutput(r, []memory.ExtractorCandidate{f.candidate(r)}), ctx.Err()
	}}
	done := make(chan error, 1)
	go func() {
		result, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, compilerGeneration(), extractor)
		if result.State != "cancelled" || result.CapacityState != "release_pending" || len(result.Candidates) != 0 {
			done <- errors.New("cancelled result published or released")
			return
		}
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("local cancellation exceeded one second")
	}
}

func TestCompilerRangeInspectionAndImmutableSeals(t *testing.T) {
	f := newCompilerFixture(t)
	sel := f.selection(t, "I prefer café and tea.", true)
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		candidate := f.candidate(r)
		candidate.Support[0].LocatorKind = memory.LocatorUTF8ByteRange
		candidate.Support[0].LocatorValue = "9:14"
		candidate.Support[0].EvidenceSHA256 = memory.CompilerHash([]byte("café"))
		return compilerOutput(r, []memory.ExtractorCandidate{candidate}), nil
	}}
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
	if err != nil {
		t.Fatal(err)
	}
	source := result.Candidates[0].Support[0]
	if source.Evidence != "café" {
		t.Fatalf("range became whole: %+v", source)
	}
	if _, err := f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	projected, err := f.store.ResolveCompilerSource(context.Background(), f.session.ScopeContext(), sel, source)
	if err != nil || projected.Evidence != "café" {
		t.Fatalf("closed source: %+v %v", projected, err)
	}
	for _, query := range []string{`UPDATE memory_compiler_generations SET manifest='{}'`, `UPDATE memory_compiler_jobs SET request='{}'`, `UPDATE memory_compiler_stages SET envelope='[]'`, `UPDATE memory_compiler_candidates SET envelope='{}'`, `UPDATE memory_compiler_event_positions SET commit_position=999`} {
		if _, err := f.db.Exec(query); err == nil {
			t.Fatalf("mutable seal: %s", query)
		}
	}
}

func TestCompilerAtomicPublicationAndOutstandingStageReservation(t *testing.T) {
	f := newCompilerFixture(t)
	first := f.selection(t, "I prefer tea", true)
	second := f.selection(t, "I prefer coffee", true)
	if _, err := f.db.Exec(`CREATE TRIGGER compiler_test_abort_publish BEFORE INSERT ON memory_compiler_coverage WHEN NEW.outcome='completed_candidates' BEGIN SELECT RAISE(ABORT,'test publication failure');END`); err != nil {
		t.Fatal(err)
	}
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return compilerOutput(r, []memory.ExtractorCandidate{f.candidate(r)}), nil
	}}
	if _, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), first, compilerGeneration(), extractor); err == nil {
		t.Fatal("publication should fail")
	}
	var stages, groups, candidates, coverage, capacity int
	for query, target := range map[string]*int{`SELECT COUNT(*) FROM memory_compiler_stages WHERE consumed=0`: &stages, `SELECT COUNT(*) FROM memory_compiler_candidate_groups`: &groups, `SELECT COUNT(*) FROM memory_compiler_candidates`: &candidates, `SELECT COUNT(*) FROM memory_compiler_coverage`: &coverage, `SELECT COUNT(*) FROM memory_compiler_capacity`: &capacity} {
		if err := f.db.QueryRow(query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if stages != 1 || groups != 0 || candidates != 0 || coverage != 0 || capacity != 0 {
		t.Fatalf("partial publication %d %d %d %d %d", stages, groups, candidates, coverage, capacity)
	}
	// Existing retained presentation plus this unpublished 16-item reservation
	// leaves insufficient room for a second request, even though inference ended.
	var job string
	if err := f.db.QueryRow(`SELECT job_id FROM memory_compiler_jobs`).Scan(&job); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO memory_compiler_candidate_groups VALUES(?,'fixture')`, job); err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2030; i++ {
		if _, err := tx.Exec(`INSERT INTO memory_compiler_candidates(candidate_id,job_id,ordinal,envelope,equivalence_hash) VALUES(?,?,?,'{}','fixture')`, fmt.Sprintf("reserved-%d", i), job, i); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), second, compilerGeneration(), extractor)
	if !errors.Is(err, eviedb.ErrCompilerCapacityBlocked) || extractor.calls.Load() != 1 {
		t.Fatalf("stage reservation bypass: %v calls%d", err, extractor.calls.Load())
	}
}

func TestCompilerMalformedRawJSONNeverBecomesEmpty(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  func(memory.CompilerRequest) []byte
	}{{"duplicate", func(r memory.CompilerRequest) []byte {
		return []byte(`{"request_id":"` + r.ID + `","candidates":[],"candidates":[]}`)
	}}, {"invalid utf8", func(r memory.CompilerRequest) []byte {
		return append(append([]byte(`{"request_id":"`+r.ID+`","candidates":[],"`), 0xff), []byte(`":0}`)...)
	}}, {"unknown field", func(r memory.CompilerRequest) []byte {
		return []byte(`{"request_id":"` + r.ID + `","candidates":[],"scope":"global"}`)
	}}} {
		t.Run(test.name, func(t *testing.T) {
			f := newCompilerFixture(t)
			sel := f.selection(t, "I prefer tea", true)
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				return eviedb.CompilerExtraction{Raw: test.raw(r), ReleaseEvidence: "completed"}, nil
			}}
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
			if err == nil || result.State != "retry_wait" {
				t.Fatalf("invalid output %+v %v", result, err)
			}
		})
	}
}

func TestCompilerKnownNoDispatchFailureReleasesCapacity(t *testing.T) {
	f := newCompilerFixture(t)
	first := f.selection(t, "I prefer tea", true)
	second := f.selection(t, "I prefer coffee", true)
	extractor := &scriptedCompiler{run: func(context.Context, memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return eviedb.CompilerExtraction{ReleaseEvidence: "not_dispatched"}, errors.New("identity preflight failed")
	}}
	for _, sel := range []memory.CompilationSelection{first, second} {
		result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
		if err == nil || result.Attempts != 1 || result.CapacityState != "" {
			t.Fatalf("zero-dispatch capacity %+v %v", result, err)
		}
	}
	if extractor.calls.Load() != 2 {
		t.Fatal("known free capacity remained blocked")
	}
}

func TestCompilerEventPositionsLegacyMigrationAndRollback(t *testing.T) {
	f := newCompilerFixture(t)
	// Simulate a pre-compiler database: existing episodes have no side records or
	// append trigger. Reopen installs the facility without ordering this cohort.
	if _, err := f.db.Exec(`DROP TRIGGER memory_compiler_append_position;DELETE FROM memory_compiler_event_positions;UPDATE memory_compiler_position_counter SET value=0`); err != nil {
		t.Fatal(err)
	}
	legacy := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "legacy"})
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	var cohort string
	var position sql.NullInt64
	if err := f.db.QueryRow(`SELECT cohort,commit_position FROM memory_compiler_event_coordinates WHERE event_id=?`, legacy.ID).Scan(&cohort, &position); err != nil {
		t.Fatal(err)
	}
	if cohort != "legacy" || position.Valid {
		t.Fatalf("invented legacy order %s %+v", cohort, position)
	}
	current := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "new"})
	if err := f.db.QueryRow(`SELECT commit_position FROM memory_compiler_event_positions WHERE event_id=?`, current.ID).Scan(&position); err != nil || position.Int64 != 1 {
		t.Fatalf("new position %+v %v", position, err)
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO events(id,session_id,sequence,event_type,role,content,recorded_at) VALUES('rollback-event',?,999,'user_message','user','rollback','2026-09-05T00:00:00Z')`, f.session.ID); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var counter int
	if err := f.db.QueryRow(`SELECT value FROM memory_compiler_position_counter`).Scan(&counter); err != nil || counter != 1 {
		t.Fatalf("rollback allocated position %d %v", counter, err)
	}
}

func TestCompilerWindowBoundsAndDoesNotBridgeOmittedOverlap(t *testing.T) {
	f := newCompilerFixture(t)
	f.selection(t, "earlier small assertion", true)
	f.selection(t, strings.Repeat("x", 9000), true)
	sel := f.selection(t, "I prefer tea", true)
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		for _, source := range r.Window.Sources {
			if source.Usage == "overlap" {
				t.Errorf("bridged nearest oversized whole overlap: %+v", source)
			}
		}
		return compilerOutput(r, []memory.ExtractorCandidate{}), nil
	}}
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
	if err != nil || result.State != "completed_empty" {
		t.Fatalf("bounded context %+v %v", result, err)
	}
	root := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "root"})
	last := root
	for i := 0; i < 64; i++ {
		last = f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: root.ID, Content: "another assertion"})
	}
	last = f.append(t, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "done"})
	sel = memory.CompilationSelection{SessionID: f.session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}
	result, err = f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
	if err != nil || result.State != "failed" || result.Reason != "oversized_input" || extractor.calls.Load() != 1 {
		t.Fatalf("event cap %+v %v calls%d", result, err, extractor.calls.Load())
	}
}

func TestCompilerCapturedRootExcludesLaterRootTextAndHandlesSequenceGap(t *testing.T) {
	f := newCompilerFixture(t)
	sel := f.selection(t, "I prefer tea", true)
	later := f.selection(t, "unrelated later assertion", true)
	sel.Cutoff = later.Cutoff
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		for _, source := range r.Window.Sources {
			if r.Window.Selection.RootID == sel.RootID && strings.Contains(source.Evidence, "unrelated later") {
				t.Error("later root became source")
			}
		}
		return compilerOutput(r, []memory.ExtractorCandidate{}), nil
	}}
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
	if err != nil || len(result.Window.NewEventIDs) != 2 {
		t.Fatalf("root coverage %+v %v", result, err)
	}
	if _, err := f.db.Exec(`INSERT INTO events(id,session_id,sequence,event_type,role,content,recorded_at) VALUES('huge-gap-root',?,1000000000000,'user_message','user','gap assertion','2026-09-05T00:00:00Z')`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseTurnLease(context.Background(), f.lease.SessionID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err = f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), memory.CompilationSelection{SessionID: f.session.ID, RootID: "huge-gap-root", Cutoff: 1000000000000, Destination: "global"}, compilerGeneration(), extractor)
	if err != nil || result.State != "completed_empty" {
		t.Fatalf("numeric gap processing %+v %v", result, err)
	}
}

func TestCompilerGrowingReviewedVocabularyUsesBoundedOfferedSnapshot(t *testing.T) {
	f := newCompilerFixture(t)
	for i := 0; i < 34; i++ {
		event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: fmt.Sprintf("/remember fact_%d tea", i)})
		proposal, err := f.store.PrepareRememberLiteral(context.Background(), f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: fmt.Sprintf("idem:v1:91000000-0000-4000-8000-%012d", i), SourceEventID: event.ID, Predicate: fmt.Sprintf("fact_%d", i), PredicateLabel: "fixture fact", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.ApplyRememberLiteral(context.Background(), f.lease, proposal); err != nil {
			t.Fatal(err)
		}
	}
	sel := f.selection(t, "I prefer tea", true)
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		if len(r.Predicates) != 32 || !r.AcceptedContextOmitted {
			t.Errorf("unbounded or silent vocabulary snapshot %+v", r)
		}
		return compilerOutput(r, []memory.ExtractorCandidate{}), nil
	}}
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), extractor)
	if err != nil || result.State != "completed_empty" || extractor.calls.Load() != 1 {
		t.Fatalf("corpus growth disabled compiler %+v %v", result, err)
	}
}

func TestCompilerReorderedReferencesLinkOriginalReviewWithoutRewritingOutput(t *testing.T) {
	f := newCompilerFixture(t)
	f.selection(t, "I prefer tea.", true)
	selection := f.selection(t, "Tea remains my preference.", true)
	reverse := false
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		candidate := f.candidate(r)
		for _, source := range r.Window.Sources {
			if source.Usage == "overlap" {
				candidate.Support = append(candidate.Support, source.Locator)
				break
			}
		}
		for _, source := range r.Window.Sources {
			if source.Usage == "context" {
				candidate.Context = append(candidate.Context, source.Locator)
			}
		}
		if len(candidate.Support) != 2 || len(candidate.Context) != 2 {
			t.Fatalf("fixture references %+v", candidate)
		}
		if reverse {
			candidate.Support[0], candidate.Support[1] = candidate.Support[1], candidate.Support[0]
			candidate.Context[0], candidate.Context[1] = candidate.Context[1], candidate.Context[0]
		}
		return compilerOutput(r, []memory.ExtractorCandidate{candidate}), nil
	}}
	generation := compilerGeneration()
	first, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), selection, generation, extractor)
	if err != nil {
		t.Fatal(err)
	}
	original := first.Candidates[0]
	if _, err := f.db.Exec(`UPDATE memory_compiler_candidates SET review_state='rejected',review_revision=7 WHERE candidate_id=?`, original.ID); err != nil {
		t.Fatal(err)
	}
	reverse = true
	generation.Decoding.Seed++
	second, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), selection, generation, extractor)
	if err != nil {
		t.Fatal(err)
	}
	later := second.Candidates[0]
	if later.EquivalentTo != original.ID || later.ReviewState != "unresolved" || later.ReviewRevision != 0 {
		t.Fatalf("repeat copied/lost review %+v", later)
	}
	if later.Proposal.Support[0] != original.Proposal.Support[1] || later.Proposal.Context[0] != original.Proposal.Context[1] {
		t.Fatal("equivalence rewrote retained original output order")
	}
	retained, err := f.store.InspectCompilation(context.Background(), f.session.ScopeContext(), first.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if retained.Candidates[0].ReviewState != "rejected" || retained.Candidates[0].ReviewRevision != 7 || retained.Candidates[0].Proposal.Support[0] != original.Proposal.Support[0] {
		t.Fatalf("earlier review changed %+v", retained.Candidates[0])
	}
	repeated, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), selection, generation, extractor)
	if err != nil || repeated.Candidates[0].ID != later.ID || extractor.calls.Load() != 2 {
		t.Fatalf("repeat delivery %+v %v calls%d", repeated, err, extractor.calls.Load())
	}
	var actionable int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_candidates WHERE review_state='unresolved' AND equivalent_to IS NULL`).Scan(&actionable); err != nil || actionable != 0 {
		t.Fatalf("reopened rejected item count=%d err=%v", actionable, err)
	}
}
