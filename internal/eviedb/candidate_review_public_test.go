package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davidadel66/evie/internal/composition"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func reviewCandidateFixture(t *testing.T) (*compilerFixture, memory.Compilation, eviedb.OwnerReviewContext) {
	t.Helper()
	f := newCompilerFixture(t)
	selection := f.selection(t, "I prefer café and tea.", true)
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		candidate := f.candidate(r)
		candidate.Proposition.Object.Literal.Value = "café"
		for _, source := range r.Window.Sources {
			if source.Usage == "new_support" {
				start := strings.Index(source.Evidence, "café")
				candidate.Support = []memory.EvidenceLocator{{EventID: source.Locator.EventID, EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: fmt.Sprintf("%d:%d", start, start+len("café")), EvidenceSHA256: memory.CompilerHash([]byte("café"))}}
			}
			if source.Usage == "context" {
				candidate.Context = append(candidate.Context, source.Locator)
			}
		}
		return compilerOutput(r, []memory.ExtractorCandidate{candidate}), nil
	}}
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), selection, compilerGeneration(), extractor)
	if err != nil || result.State != "completed_candidates" {
		t.Fatalf("compile %+v: %v", result, err)
	}
	a, err := f.store.LocalOwnerReviewContext(context.Background(), "global")
	if err != nil {
		t.Fatal(err)
	}
	return f, result, a
}
func decisionFor(p memory.ReviewPreview, key string) memory.ReviewDecision {
	return memory.ReviewDecision{DeliveryKey: "idem:v1:" + key, PreviewID: p.ID, PreviewSHA256: p.SHA256, Action: p.Action}
}

func TestOwnerReviewClosedSessionExactSourceAndReplay(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	if err := f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := f.db.QueryRow(`SELECT count(*) FROM events`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Effect.Claims) != 1 || p.Effect.Claims[0].Sources[0].Evidence != "café" || p.Effect.Claims[0].Sources[0].Authority != memory.AuthorityOwnerStatement || len(p.Effect.Claims[0].Context) != 1 || p.Effect.Claims[0].Context[0].Authority != "none" {
		t.Fatalf("preview %+v", p)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000140"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation == nil || len(result.Operation.ClaimIDs) != 1 || result.Candidates[0].ReviewRevision != 1 {
		t.Fatalf("result %+v", result)
	}
	inspectionSession, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.InspectSemanticObject(ctx, inspectionSession.ScopeContext(), memory.SemanticObjectSourceLink, result.Operation.SourceLinkIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if source.Source.Evidence != "café" || source.Source.LocatorValue != p.Effect.Claims[0].Sources[0].LocatorValue {
		t.Fatalf("source expanded %+v", source)
	}
	operation, err := f.store.InspectOwnerReviewOperation(ctx, a, result.Operation.OperationID)
	if err != nil || operation.Preview.Effect.Claims[0].Context[0].Evidence != "Recorded." {
		t.Fatalf("accepted context %+v %v", operation, err)
	}
	var after int
	var status string
	var leases int
	f.db.QueryRow(`SELECT count(*) FROM events`).Scan(&after)
	f.db.QueryRow(`SELECT status FROM sessions WHERE id=?`, f.session.ID).Scan(&status)
	f.db.QueryRow(`SELECT count(*) FROM session_turn_leases WHERE holder_id IS NOT NULL`).Scan(&leases)
	if before != after || status != "closed" || leases != 0 {
		t.Fatalf("source authority changed events %d/%d status %s leases %d", before, after, status, leases)
	}
	replay, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !replay.Valid {
		t.Fatalf("replay %+v %v", replay, err)
	}
	if err = f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	repeated, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000140"))
	if err != nil || string(mustJSON(t, repeated)) != string(mustJSON(t, result)) {
		t.Fatalf("repeated %+v %v", repeated, err)
	}
	replay, err = f.store.VerifySemanticProjection(ctx)
	if err != nil || !replay.Valid {
		t.Fatalf("reopen replay %+v %v", replay, err)
	}
}
func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOwnerReviewRejectRedactionStalenessAndIdempotency(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-detector-v2'`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000141")); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("policy changed: %v", err)
	}
	item, err := f.store.InspectOwnerCandidate(ctx, a, compiled.Candidates[0].ID)
	if err != nil || !item.Redacted || strings.Contains(string(mustJSON(t, item)), "café") {
		t.Fatalf("redacted %+v %v", item, err)
	}
	reject, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "reject")
	if err != nil {
		t.Fatal(err)
	}
	decision := decisionFor(reject, "90000000-0000-4000-8000-000000000142")
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decision)
	if err != nil || result.Operation != nil {
		t.Fatalf("reject %+v %v", result, err)
	}
	same, err := f.store.ResolveOwnerCandidateReview(ctx, a, decision)
	if err != nil || string(mustJSON(t, same)) != string(mustJSON(t, result)) {
		t.Fatalf("idempotence %+v %v", same, err)
	}
	decision.Reason = "different"
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decision); !errors.Is(err, eviedb.ErrIdempotencyConflict) {
		t.Fatalf("key conflict: %v", err)
	}
	var ops, revision int
	f.db.QueryRow(`SELECT count(*) FROM semantic_operations`).Scan(&ops)
	f.db.QueryRow(`SELECT revision FROM semantic_scopes WHERE scope_key='global'`).Scan(&revision)
	if ops != 1 || revision != 1 {
		t.Fatalf("rejection changed graph: ops %d revision %d", ops, revision)
	}
}

func TestOwnerReviewConcurrentAcceptRejectAndForgedCapability(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	accept, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	reject, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "reject")
	if err != nil {
		t.Fatal(err)
	}
	var forged eviedb.OwnerReviewContext
	if err = json.Unmarshal(mustJSON(t, a), &forged); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ListOwnerCandidates(ctx, forged, memory.OwnerCandidateQuery{}); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
		t.Fatalf("forged authority: %v", err)
	}
	second, err := eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	other := eviedb.NewStore(second)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, p := range []memory.ReviewPreview{accept, reject} {
		wg.Add(1)
		go func(i int, p memory.ReviewPreview) {
			defer wg.Done()
			<-start
			store := f.store
			if i == 1 {
				store = other
			}
			_, err := store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, fmt.Sprintf("90000000-0000-4000-8000-%012d", 143+i)))
			results <- err
		}(i, p)
	}
	close(start)
	wg.Wait()
	close(results)
	wins, loses := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, eviedb.ErrReviewResolved) {
			loses++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || loses != 1 {
		t.Fatalf("race %d/%d", wins, loses)
	}
	var audits int
	f.db.QueryRow(`SELECT count(*) FROM memory_review_audits`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("audits %d", audits)
	}
}

func candidateRef(compiled memory.Compilation) memory.CandidateRef {
	return memory.CandidateRef{ID: compiled.Candidates[0].ID, ReviewRevision: compiled.Candidates[0].ReviewRevision}
}

func TestOwnerReviewMixedRecurrenceAllowsFreshIndependentCandidate(t *testing.T) {
	ctx := context.Background()
	f, first, a := reviewCandidateFixture(t)
	generation := compilerGeneration()
	generation.Prompt += " Pinned independent comparison."
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		original := first.Candidates[0].Proposal
		fresh := f.candidate(r)
		for _, source := range r.Window.Sources {
			if source.Usage == "new_support" {
				start := strings.Index(source.Evidence, "tea")
				fresh.Support = []memory.EvidenceLocator{{EventID: source.Locator.EventID, EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: fmt.Sprintf("%d:%d", start, start+3), EvidenceSHA256: memory.CompilerHash([]byte("tea"))}}
			}
		}
		return compilerOutput(r, []memory.ExtractorCandidate{original, fresh}), nil
	}}
	second, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), first.Window.Selection, generation, extractor)
	if err != nil || second.State != "completed_candidates" {
		t.Fatalf("second %+v %v", second, err)
	}
	if len(second.Candidates) != 2 || second.Candidates[0].EquivalentTo != first.Candidates[0].ID || second.Candidates[1].EquivalentTo != "" {
		t.Fatalf("recurrence %+v", second.Candidates)
	}
	if _, err = f.store.PrepareOwnerCandidateReview(ctx, a, memory.CandidateRef{ID: second.Candidates[0].ID}, "accept"); err == nil {
		t.Fatal("suppressed candidate became executable")
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, memory.CandidateRef{ID: second.Candidates[1].ID}, "accept")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Candidates) != 1 || p.Candidates[0].Ref.ID != second.Candidates[1].ID || p.Effect.Claims[0].Create {
		t.Fatalf("wrong independent/equal effect %+v", p)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000145"))
	if err != nil || result.Operation == nil {
		t.Fatalf("accept fresh %+v %v", result, err)
	}
	original, err := f.store.InspectOwnerCandidate(ctx, a, first.Candidates[0].ID)
	if err != nil || original.Candidate.ReviewState != "unresolved" {
		t.Fatalf("unselected original changed %+v %v", original, err)
	}
}

func TestOwnerReviewAtomicFailureAndSourceSeal(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`CREATE TRIGGER fail_review_audit BEFORE INSERT ON memory_review_audits BEGIN SELECT RAISE(ABORT,'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	decision := decisionFor(p, "90000000-0000-4000-8000-000000000146")
	failed, err := f.store.ResolveOwnerCandidateReview(ctx, a, decision)
	if err == nil || failed.Operation != nil || failed.AuditID != "" {
		t.Fatalf("failed commit leaked success %+v %v", failed, err)
	}
	var ops, audits, deliveries int
	var state string
	f.db.QueryRow(`SELECT count(*) FROM semantic_operations`).Scan(&ops)
	f.db.QueryRow(`SELECT count(*) FROM memory_review_audits`).Scan(&audits)
	f.db.QueryRow(`SELECT count(*) FROM memory_review_deliveries`).Scan(&deliveries)
	f.db.QueryRow(`SELECT review_state FROM memory_compiler_candidates WHERE candidate_id=?`, compiled.Candidates[0].ID).Scan(&state)
	if ops != 1 || audits != 0 || deliveries != 0 || state != "unresolved" {
		t.Fatalf("partial acceptance ops %d audits %d deliveries %d state %s", ops, audits, deliveries, state)
	}
	if _, err = f.db.Exec(`DROP TRIGGER fail_review_audit`); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = f.store.ResolveOwnerCandidateReview(cancelled, a, decision); err == nil {
		t.Fatal("cancelled approval committed")
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decision); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE memory_compiler_candidates SET envelope=json_set(envelope,'$.support[0].locator.locator_value','0:1') WHERE candidate_id=?`, compiled.Candidates[0].ID); err == nil {
		t.Fatal("frozen source projection mutable")
	}
}

func TestOwnerReviewStaleVectorAndRevokedAuthorization(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "/remember drink water"})
	explicit, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000147", SourceEventID: event.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "water"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, explicit); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000148")); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("external semantic write: %v", err)
	}
	refreshed, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil || refreshed.ID == p.ID || refreshed.SHA256 == p.SHA256 {
		t.Fatalf("refresh %+v %v", refreshed, err)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(refreshed, "90000000-0000-4000-8000-000000000149")); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
		t.Fatalf("revoked approval: %v", err)
	}
}

func TestOwnerReviewExactScopeFamiliesAndClosedLineage(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	workspaceA, err := f.store.RegisterWorkspace(ctx, "review A")
	if err != nil {
		t.Fatal(err)
	}
	workspaceB, err := f.store.RegisterWorkspace(ctx, "review B")
	if err != nil {
		t.Fatal(err)
	}
	receipt := reviewTestReceipt()
	wa, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspaceA.ID, workspaceA.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspaceB.ID, workspaceB.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	projectA, err := f.store.RegisterProject(ctx, "review project A", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := f.store.RegisterProject(ctx, "review project B", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pa, err := f.store.CreateProjectSession(ctx, projectA.ID)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := f.store.CreateProjectSession(ctx, projectB.ID)
	if err != nil {
		t.Fatal(err)
	}
	sessionOnly, err := f.store.CreateProjectSession(ctx, projectA.ID)
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		session  memory.Session
		scope    string
		compiled memory.Compilation
	}
	entries := []entry{{session: f.session, scope: "global"}, {session: wa, scope: "workspace:" + string(workspaceA.ID)}, {session: wb, scope: "workspace:" + string(workspaceB.ID)}, {session: pa, scope: "project:" + string(projectA.ID)}, {session: pb, scope: "project:" + string(projectB.ID)}, {session: sessionOnly, scope: "session:" + string(sessionOnly.ID)}}
	for i := range entries {
		f.session = entries[i].session
		if i != 0 {
			f.lease, err = f.store.AcquireTurnLease(ctx, f.session.ID, "review-scope", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
		}
		sel := f.selection(t, "I prefer tea.", true)
		sel.Destination = entries[i].scope
		extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
			return compilerOutput(r, []memory.ExtractorCandidate{f.candidate(r)}), nil
		}}
		compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, compilerGeneration(), extractor)
		if err != nil || compiled.State != "completed_candidates" {
			t.Fatalf("scope %s: %+v %v", entries[i].scope, compiled, err)
		}
		entries[i].compiled = compiled
		if err = f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
			t.Fatal(err)
		}
		if _, err = f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
			t.Fatal(err)
		}
	}
	for i, item := range entries {
		a, err := f.store.LocalOwnerReviewContext(ctx, item.scope)
		if err != nil {
			t.Fatal(err)
		}
		page, err := f.store.ListOwnerCandidates(ctx, a, memory.OwnerCandidateQuery{})
		if err != nil || len(page.Candidates) != 1 || page.Candidates[0].Ref.ID != item.compiled.Candidates[0].ID {
			t.Fatalf("scope inbox %s %+v %v", item.scope, page, err)
		}
		for j, foreign := range entries {
			if i == j {
				continue
			}
			leak, err := f.store.InspectOwnerCandidate(ctx, a, foreign.compiled.Candidates[0].ID)
			if !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) || len(leak.Candidate.Support) != 0 {
				t.Fatalf("scope %s leaked %s: %+v %v", item.scope, foreign.scope, leak, err)
			}
		}
		p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(item.compiled), "accept")
		if err != nil {
			t.Fatalf("prepare %s: %v", item.scope, err)
		}
		result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, fmt.Sprintf("90000000-0000-4000-8000-%012d", 150+i)))
		if err != nil {
			t.Fatalf("accept %s: %v", item.scope, err)
		}
		if result.Operation == nil || p.Effect.Scope.Key != item.scope {
			t.Fatalf("destination widened %+v", p)
		}
	}
	replay, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !replay.Valid {
		t.Fatalf("scope replay %+v %v", replay, err)
	}
	authority, err := f.store.LocalOwnerReviewContext(ctx, "workspace:"+string(workspaceA.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ArchiveWorkspace(ctx, workspaceA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ListOwnerCandidates(ctx, authority, memory.OwnerCandidateQuery{}); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
		t.Fatalf("archived scope access: %v", err)
	}
}

func reviewTestReceipt() composition.Receipt {
	hash := strings.Repeat("0", 64)
	return composition.Receipt{FormatVersion: composition.FormatVersion, Preset: composition.PresetIdentity{ID: "standard", Version: "sha256:" + hash}, EvieVersion: "1.0.0", Providers: []composition.Provider{{ID: "fixture", ImplementationVersion: "1.0.0"}}, Capabilities: []composition.Capability{{ID: "fixture.echo", ProviderID: "fixture", ContractVersion: "1.0.0", SchemaSHA256: hash}}, ToolSchemas: []composition.ToolSchema{{Name: "fixture_echo", SHA256: hash}}, Instructions: []composition.InstructionReference{{ID: "fixture-instructions", SHA256: hash}}, Configuration: []composition.ConfigurationReference{{Kind: composition.ConfigurationConnection, ID: "da73b499-4df4-4a91-bbe8-4fd3a223e634"}}}
}

func TestOwnerReviewAcceptedSourcePolicyRedactsWithoutRewritingHistory(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000160"))
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, result.Operation.OperationID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-detector-v2'`); err != nil {
		t.Fatal(err)
	}
	hidden, err := f.store.InspectOwnerReviewOperation(ctx, a, result.Operation.OperationID)
	if !errors.Is(err, eviedb.ErrReviewInvalidSource) || hidden.OperationID != "" {
		t.Fatalf("old preview source leaked %+v %v", hidden, err)
	}
	source, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectSourceLink, result.Operation.SourceLinkIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if source.Source.Evidence != "" || len(source.Operations) != 1 || source.Operations[0].PreparedJSON != "" || source.Operations[0].ProposalJSON != "" {
		t.Fatalf("accepted source disclosure bypass %+v", source)
	}
	claims, err := f.store.InspectLiteralClaims(ctx, f.session.ScopeContext())
	if err != nil || len(claims.Claims) != 2 {
		t.Fatalf("accepted truth changed %+v %v", claims, err)
	}
	replay, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !replay.Valid {
		t.Fatalf("current policy rewrote replay %+v %v", replay, err)
	}
	var after string
	f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, result.Operation.OperationID).Scan(&after)
	if after != stored {
		t.Fatal("historical authority envelope changed")
	}
	repeat, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000160"))
	if err != nil || repeat.AuditID != result.AuditID {
		t.Fatalf("duplicate treated as fresh acceptance %+v %v", repeat, err)
	}
}
