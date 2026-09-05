package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davidadel66/evie/internal/memory"
	"os"
	"strings"
	"testing"
)

func batchBoundaryFixture(t *testing.T, count int) (*workerFixture, OwnerReviewContext, []memory.CandidateRef) {
	t.Helper()
	return batchBoundaryFixtureWithPadding(t, count, "")
}
func batchBoundaryFixtureWithPadding(t *testing.T, count int, padding string) (*workerFixture, OwnerReviewContext, []memory.CandidateRef) {
	t.Helper()
	ctx := context.Background()
	f := newWorkerFixture(t)
	source, err := f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink water."})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := f.store.PrepareRememberLiteral(ctx, f.owner, memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000006144", SourceEventID: source.ID, Predicate: "drinks", PredicateLabel: "drinks", PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "water"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, seed); err != nil {
		t.Fatal(err)
	}
	refs := []memory.CandidateRef{}
	for start := 0; start < count; start += 16 {
		end := start + 16
		if end > count {
			end = count
		}
		job := f.queue(t, fmt.Sprintf("I drink the reviewed varieties %d through %d.", start, end)+padding)
		script := &workerScript{run: func(_ context.Context, r memory.CompilerRequest) (CompilerExtraction, error) {
			var support memory.EvidenceLocator
			for _, source := range r.Window.Sources {
				if source.Usage == "new_support" {
					support = source.Locator
					break
				}
			}
			candidates := []memory.ExtractorCandidate{}
			for n := start; n < end; n++ {
				candidates = append(candidates, memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: seed.Subject.ID, PredicateID: seed.Predicate.ID, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: fmt.Sprintf("variety%d", n)}}, Polarity: memory.PolarityAffirmed}, Support: []memory.EvidenceLocator{support}, Context: []memory.EvidenceLocator{}})
			}
			return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: r.ID, Candidates: candidates}), ReleaseEvidence: "completed"}, nil
		}}
		if worked, err := f.store.RunCompilerStep(ctx, f.config(script)); err != nil || !worked {
			t.Fatalf("compile %v %v", worked, err)
		}
		compiled, err := f.store.InspectCompilation(ctx, f.owner, job.JobID)
		if err != nil || len(compiled.Candidates) != end-start {
			t.Fatalf("compiled %+v %v", compiled, err)
		}
		for _, c := range compiled.Candidates {
			refs = append(refs, memory.CandidateRef{ID: c.ID, ReviewRevision: c.ReviewRevision})
		}
	}
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	return f, a, refs
}
func boundaryBatchRequest(refs []memory.CandidateRef, groups int, action string) memory.ReviewBatchRequest {
	r := memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{}}
	for i := 0; i < groups; i++ {
		r.Groups = append(r.Groups, memory.ReviewBatchGroupRequest{ID: fmt.Sprintf("group%d", i), Action: action, Candidates: []memory.CandidateRef{refs[i]}, Dependencies: []memory.ReviewDependency{}})
	}
	for i := groups; i < len(refs); i++ {
		g := (i - groups) % groups
		r.Groups[g].Candidates = append(r.Groups[g].Candidates, refs[i])
	}
	return r
}
func boundaryBatchDecision(p memory.ReviewBatchPreview) memory.ReviewBatchDecision {
	d := memory.ReviewBatchDecision{DeliveryKey: "idem:v1:90000000-0000-4000-8000-000000008144", PreviewID: p.ID, PreviewSHA256: p.SHA256, Actions: []memory.ReviewBatchAction{}}
	for _, g := range p.Groups {
		d.Actions = append(d.Actions, memory.ReviewBatchAction{GroupID: g.ID, Action: g.Preview.Action})
	}
	return d
}
func TestOwnerReviewBatchInclusiveReferenceAndGroupBounds(t *testing.T) {
	ctx := context.Background()
	f, a, refs := batchBoundaryFixture(t, 65)
	for _, test := range []struct {
		groups, refs int
		want         error
	}{{20, 20, nil}, {21, 21, ErrReviewTooLarge}, {4, 64, nil}, {4, 65, ErrReviewTooLarge}} {
		p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs[:test.refs], test.groups, "reject"))
		if !errors.Is(err, test.want) {
			t.Fatalf("groups%d refs%d: %v", test.groups, test.refs, err)
		}
		if err == nil && len(p.Groups) != test.groups {
			t.Fatal("truncated batch")
		}
	}
}
func TestOwnerReviewBatchInclusiveRecordAndCompleteByteBounds(t *testing.T) {
	// Each counted dimension is separately inclusive; the concrete canonical
	// preview check below includes the domain, hash, evidence and context bytes.
	for _, test := range []struct {
		g, r, e, b int
		bad        bool
	}{{20, 64, 256, 256 * 1024, false}, {21, 64, 256, 256 * 1024, true}, {20, 65, 256, 256 * 1024, true}, {20, 64, 257, 256 * 1024, true}, {20, 64, 256, 256*1024 + 1, true}} {
		err := validateReviewBatchBounds(test.g, test.r, test.e, test.b)
		if errors.Is(err, ErrReviewTooLarge) != test.bad {
			t.Fatalf("bound %+v %v", test, err)
		}
	}
	ctx := context.Background()
	f, a, refs := batchBoundaryFixture(t, 1)
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 1, "reject"))
	if err != nil {
		t.Fatal(err)
	}
	// Rejection retains bounded safe summaries. Grow exact authorized source bytes
	// in the frozen encoding fixture, never by truncating the real preview.
	source := &p.Groups[0].Preview.Candidates[0].Candidate.Support[0]
	rehash := func() {
		g := &p.Groups[0].Preview
		g.EffectSHA256, _, _ = ownerReviewEffectHash(g.Effect)
		g.SHA256, _, _ = ownerReviewPreviewHash(*g)
		p.SHA256, _, _ = ownerReviewBatchHash(p)
	}
	// Grow across separate candidate disclosures so each individual preview also
	// remains within its own256KiB envelope.
	target := 256 * 1024
	rehash()
	padding := target - len(completeReviewBatchBytes(p))
	source.Evidence += strings.Repeat("x", padding)
	rehash()
	if len(completeReviewBatchBytes(p)) != target {
		t.Fatal("wrong inclusive canonical byte fixture")
	}
	if err = validateOwnerReviewBatch(p); err != nil {
		t.Fatalf("exact canonical bytes rejected: %v", err)
	}
	source.Evidence += "x"
	rehash()
	if err = validateOwnerReviewBatch(p); !errors.Is(err, ErrReviewTooLarge) {
		t.Fatalf("max+1 canonical bytes %v", err)
	}
}
func TestOwnerReviewBatchOuterFailuresRollbackAllGroups(t *testing.T) {
	for _, phase := range []string{"candidate", "audit", "delivery", "commit", "cancel", "lost_response"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			f, a, refs := batchBoundaryFixture(t, 2)
			p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 2, "accept"))
			if err != nil {
				t.Fatal(err)
			}
			d := boundaryBatchDecision(p)
			callCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			switch phase {
			case "candidate":
				_, err = f.db.Exec(`CREATE TRIGGER fail_batch_persistence BEFORE UPDATE OF review_state ON memory_compiler_candidates BEGIN SELECT RAISE(ABORT,'candidate write failure');END`)
			case "audit":
				_, err = f.db.Exec(`CREATE TRIGGER fail_batch_persistence BEFORE INSERT ON memory_review_audits BEGIN SELECT RAISE(ABORT,'audit write failure');END`)
			case "delivery":
				_, err = f.db.Exec(`CREATE TRIGGER fail_batch_persistence BEFORE INSERT ON memory_review_batch_deliveries BEGIN SELECT RAISE(ABORT,'delivery write failure');END`)
			default:
				f.store.resolveImmediateTransaction = func(resolveCtx context.Context, conn *sql.Conn, statement string) (sql.Result, error) {
					if statement == "COMMIT" {
						switch phase {
						case "cancel":
							cancel()
							return nil, callCtx.Err()
						case "commit":
							return nil, errors.New("injected commit failure")
						case "lost_response":
							if _, err := executeImmediateTransactionStatement(resolveCtx, conn, statement); err != nil {
								return nil, err
							}
							return nil, errors.New("lost committed response")
						}
					}
					return executeImmediateTransactionStatement(resolveCtx, conn, statement)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.store.ResolveOwnerCandidateBatch(callCtx, a, d)
			if err == nil || len(result.Groups) != 0 {
				t.Fatalf("precommit success %+v %v", result, err)
			}
			f.store.resolveImmediateTransaction = executeImmediateTransactionStatement
			if phase == "candidate" || phase == "audit" || phase == "delivery" {
				if _, err = f.db.Exec(`DROP TRIGGER fail_batch_persistence`); err != nil {
					t.Fatal(err)
				}
			}
			var ops, receipts, audits, resolutions int
			if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM semantic_operations WHERE schema_version=6),(SELECT count(*) FROM memory_review_batch_deliveries),(SELECT count(*) FROM memory_review_audits),(SELECT count(*) FROM memory_review_resolutions)`).Scan(&ops, &receipts, &audits, &resolutions); err != nil {
				t.Fatal(err)
			}
			if phase == "lost_response" {
				if ops != 2 || receipts != 1 || audits != 2 || resolutions != 2 {
					t.Fatalf("committed response lost effects %d/%d/%d/%d", ops, receipts, audits, resolutions)
				}
			} else if ops != 0 || receipts != 0 || audits != 0 || resolutions != 0 {
				t.Fatalf("precommit leak %d/%d/%d/%d", ops, receipts, audits, resolutions)
			}
			result, err = f.store.ResolveOwnerCandidateBatch(ctx, a, d)
			if err != nil || len(result.Groups) != 2 || result.Groups[0].Outcome != "accepted" || result.Groups[1].Outcome != "accepted" {
				t.Fatalf("retry %+v %v", result, err)
			}
			replay, err := f.store.VerifySemanticProjection(ctx)
			if err != nil || !replay.Valid {
				t.Fatalf("replay %+v %v", replay, err)
			}
		})
	}
}

type reviewBatchGolden struct {
	EffectHash         string                    `json:"effect_sha256"`
	EffectBytes        string                    `json:"effect_canonical_utf8"`
	PreviewHash        string                    `json:"preview_sha256"`
	PreviewBytes       string                    `json:"preview_canonical_utf8"`
	BatchHash          string                    `json:"batch_sha256"`
	BatchBytes         string                    `json:"batch_canonical_utf8"`
	CompleteBatchBytes string                    `json:"complete_batch_canonical_utf8"`
	Batch              memory.ReviewBatchPreview `json:"batch"`
}

func TestOwnerReviewBatchEncodingGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/candidate_review_encoding_v5.json")
	if err != nil {
		t.Fatal(err)
	}
	var f reviewBatchGolden
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	hash, bytes, err := ownerReviewEffectHash(f.Batch.Groups[0].Preview.Effect)
	if err != nil || hash != f.EffectHash || string(bytes) != f.EffectBytes {
		t.Fatal("v5 effect bytes/hash changed")
	}
	hash, bytes, err = ownerReviewPreviewHash(f.Batch.Groups[0].Preview)
	if err != nil || hash != f.PreviewHash || string(bytes) != f.PreviewBytes {
		t.Fatal("v5 preview bytes/hash changed")
	}
	hash, bytes, err = ownerReviewBatchHash(f.Batch)
	if err != nil || hash != f.BatchHash || string(bytes) != f.BatchBytes || string(completeReviewBatchBytes(f.Batch)) != f.CompleteBatchBytes {
		t.Fatal("v5 batch bytes/hash changed")
	}
	if err = validateOwnerReviewBatch(f.Batch); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerReviewBatchRealSQLiteCompleteByteCeiling(t *testing.T) {
	ctx := context.Background()
	prepare := func(f *workerFixture, a OwnerReviewContext, refs []memory.CandidateRef) (memory.ReviewBatchPreview, error) {
		return f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 4, "reject"))
	}
	edit := func(f *workerFixture, a OwnerReviewContext, refs []memory.CandidateRef, reason string) {
		t.Helper()
		c, err := f.store.InspectOwnerCandidate(ctx, a, refs[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		c, err = f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: c.Ref, Proposal: c.Candidate.Proposal, Reason: reason})
		if err != nil {
			t.Fatal(err)
		}
		refs[0] = c.Ref
	}
	pilot, authority, pilotRefs := batchBoundaryFixture(t, 64)
	edit(pilot, authority, pilotRefs, "")
	base, err := prepare(pilot, authority, pilotRefs)
	if err != nil {
		t.Fatal(err)
	}
	// All 64 actual source disclosures plus three immutable edit disclosures
	// retain the padded original field. Reason bytes provide the final exact fit.
	padding := (256*1024 - len(completeReviewBatchBytes(base)) - 2048) / 67
	if padding < 1 {
		t.Fatal("base fixture already exceeds ceiling")
	}
	f, a, refs := batchBoundaryFixtureWithPadding(t, 64, strings.Repeat("x", padding))
	edit(f, a, refs, "")
	p, err := prepare(f, a, refs)
	if err != nil {
		t.Fatal(err)
	}
	remaining := 256*1024 - len(completeReviewBatchBytes(p))
	if remaining < 0 || remaining > 4096 {
		t.Fatalf("unexpected bounded reason adjustment %d", remaining)
	}
	edit(f, a, refs, strings.Repeat("r", remaining))
	p, err = prepare(f, a, refs)
	if err != nil || len(completeReviewBatchBytes(p)) != 256*1024 {
		t.Fatalf("exact real SQLite byte ceiling size%d err%v", len(completeReviewBatchBytes(p)), err)
	}
	var before int
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_review_previews`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	edit(f, a, refs, strings.Repeat("r", remaining+1))
	if _, err = prepare(f, a, refs); !errors.Is(err, ErrReviewTooLarge) {
		t.Fatalf("real max+1 preview %v", err)
	}
	var after int
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_review_previews`).Scan(&after); err != nil || after != before {
		t.Fatalf("too-large member previews persisted %d/%d %v", before, after, err)
	}
}
