package eviedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"modernc.org/sqlite"
)

type batchReadFault struct {
	phase          string
	candidate      string
	cause          error
	enabled, armed bool
	hits           int
}
type batchFaultConnector struct {
	path  string
	fault *batchReadFault
}

func (c batchFaultConnector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c batchFaultConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
	return &batchFaultConnection{Conn: conn, fault: c.fault}, nil
}

type batchFaultConnection struct {
	driver.Conn
	fault *batchReadFault
}

func (c *batchFaultConnection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}
func (c *batchFaultConnection) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	f := c.fault
	candidate := strings.Contains(q, "c.envelope,c.review_state") && len(args) > 0 && args[0].Value == f.candidate
	if f.enabled && strings.Contains(q, "c.envelope,c.review_state") {
		f.armed = candidate
	}
	match := f.phase == "candidate" && candidate || f.phase == "source" && f.armed && strings.Contains(q, "payload_json") && strings.Contains(q, "FROM events WHERE id=?") || f.phase == "preview" && strings.Contains(q, "SELECT envelope FROM memory_review_batch_previews") || f.phase == "policy" && strings.Contains(q, "SELECT source_policy FROM memory_review_authorization") || f.phase == "scope" && strings.Contains(q, "FROM semantic_projection_quarantine")
	if f.enabled && match {
		f.hits++
		return nil, f.cause
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}

func TestOwnerReviewBatchLaterReadFailureRollsBackOuterDelivery(t *testing.T) {
	for _, phase := range []string{"candidate", "source"} {
		for _, cause := range []error{&clockReadFailure{}, context.Canceled, context.DeadlineExceeded, sql.ErrConnDone} {
			t.Run(phase+"/"+cause.Error(), func(t *testing.T) {
				ctx := context.Background()
				f, a, refs := batchBoundaryFixture(t, 2)
				p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 2, "accept"))
				if err != nil {
					t.Fatal(err)
				}
				fault := &batchReadFault{phase: phase, candidate: refs[1].ID, cause: cause, enabled: true}
				db := sql.OpenDB(batchFaultConnector{path: f.path, fault: fault})
				defer db.Close()
				store := NewStore(db)
				result, err := store.ResolveOwnerCandidateBatch(ctx, a, boundaryBatchDecision(p))
				if !errors.Is(err, cause) || len(result.Groups) != 0 || fault.hits == 0 {
					t.Fatalf("read identity/receipt: %+v %v hits%d", result, err, fault.hits)
				}
				var ops, receipts, resolved int
				if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM semantic_operations WHERE schema_version=6),(SELECT count(*) FROM memory_review_batch_deliveries),(SELECT count(*) FROM memory_review_resolutions)`).Scan(&ops, &receipts, &resolved); err != nil {
					t.Fatal(err)
				}
				if ops != 0 || receipts != 0 || resolved != 0 {
					t.Fatalf("earlier group escaped rollback: %d/%d/%d", ops, receipts, resolved)
				}
				fault.enabled = false
				result, err = store.ResolveOwnerCandidateBatch(ctx, a, boundaryBatchDecision(p))
				if err != nil || len(result.Groups) != 2 || result.Groups[1].Outcome != "accepted" {
					t.Fatalf("retry %+v %v", result, err)
				}
			})
		}
	}
}
func TestOwnerReviewBatchInspectionPreservesReadErrors(t *testing.T) {
	for _, phase := range []string{"preview", "policy", "scope"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			f, a, refs := batchBoundaryFixture(t, 1)
			p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 1, "accept"))
			if err != nil {
				t.Fatal(err)
			}
			for _, cause := range []error{&clockReadFailure{}, context.Canceled, context.DeadlineExceeded} {
				fault := &batchReadFault{phase: phase, cause: cause, enabled: true}
				db := sql.OpenDB(batchFaultConnector{path: f.path, fault: fault})
				store := NewStore(db)
				if _, err = store.InspectOwnerCandidateBatch(ctx, a, p.ID); !errors.Is(err, cause) || fault.hits == 0 {
					t.Fatalf("inspect %v hits%d", err, fault.hits)
				}
				if _, err = store.ResolveOwnerCandidateBatch(ctx, a, boundaryBatchDecision(p)); !errors.Is(err, cause) {
					t.Fatalf("resolve %v", err)
				}
				db.Close()
			}
		})
	}
}
func TestOwnerReviewBatchMemberCannotEscapeOuterBinding(t *testing.T) {
	ctx := context.Background()
	f, a, refs := batchBoundaryFixture(t, 2)
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 2, "accept"))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range p.Groups {
		d := memory.ReviewDecision{DeliveryKey: "idem:v1:" + g.Preview.ID, PreviewID: g.Preview.ID, PreviewSHA256: g.Preview.SHA256, Action: g.Preview.Action}
		if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, d); !errors.Is(err, ErrReviewDependencies) {
			t.Fatalf("group escaped batch: %v", err)
		}
	}
	r, err := f.store.ResolveOwnerCandidateBatch(ctx, a, boundaryBatchDecision(p))
	if err != nil || len(r.Groups) != 2 {
		t.Fatalf("bound batch %+v %v", r, err)
	}
	for _, g := range p.Groups {
		op, err := f.store.InspectOwnerReviewOperation(ctx, a, g.Preview.Effect.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		op.Batch = nil
		if err = validateOwnerReviewOperation(op); !errors.Is(err, ErrReviewDependencies) {
			t.Fatalf("canonical group dropped binding: %v", err)
		}
	}
}

func TestOwnerReviewBatchSharedSourceValidationFailureHasNoPartialDefinitions(t *testing.T) {
	ctx := context.Background()
	f, a, independent := batchBoundaryFixture(t, 1)
	f.generation.EntityPolicy = memory.CompilerIdentityPolicyV2
	f.generation.PredicatePolicy = f.generation.EntityPolicy
	f.generation.ValidationPolicy = f.generation.EntityPolicy
	f.generation.EquivalencePolicy = f.generation.EntityPolicy
	f.generation.EffectPolicy = f.generation.EntityPolicy
	f.generationID, _, _ = memory.CompilerGenerationIdentity(f.generation)
	job := f.queue(t, "Maya likes tea and coffee.")
	script := &workerScript{run: func(_ context.Context, r memory.CompilerRequest) (CompilerExtraction, error) {
		var support memory.EvidenceLocator
		for _, source := range r.Window.Sources {
			if source.Usage == "new_support" {
				support = source.Locator
				break
			}
		}
		cs := []memory.ExtractorCandidate{}
		for _, value := range []string{"tea", "coffee"} {
			cs = append(cs, memory.ExtractorCandidate{Proposition: memory.ClaimProposition{Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: value}}, Polarity: memory.PolarityAffirmed}, Support: []memory.EvidenceLocator{support}, Context: []memory.EvidenceLocator{}, Identity: &memory.CandidateIdentityProposal{Subject: &memory.EntityMention{Name: "Maya", EntityType: "person", Support: support}, Predicate: &memory.PredicateDefinition{Token: "likes", Label: "likes", ObjectConstraint: memory.PredicateObjectConstraint(memory.LiteralText), Cardinality: memory.CardinalityMany}}})
		}
		return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: r.ID, Candidates: cs}), ReleaseEvidence: "completed"}, nil
	}}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(script)); err != nil || !worked {
		t.Fatalf("compile %v %v", worked, err)
	}
	compiled, err := f.store.InspectCompilation(ctx, f.owner, job.JobID)
	if err != nil || len(compiled.Candidates) != 2 {
		t.Fatalf("compiled %+v %v", compiled, err)
	}
	refs := []memory.CandidateRef{}
	for _, c := range compiled.Candidates {
		ref := memory.CandidateRef{ID: c.ID}
		o, err := f.store.OwnerCandidateIdentityOptions(ctx, a, ref)
		if err != nil {
			t.Fatal(err)
		}
		chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: o.SHA256, Choices: memory.ReviewIdentityChoices{Subject: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, chosen.Ref)
	}
	deps := []memory.ReviewDependency{{CandidateID: refs[1].ID, Field: "subject", FromCandidateID: refs[0].ID, FromField: "subject"}, {CandidateID: refs[1].ID, Field: "predicate", FromCandidateID: refs[0].ID, FromField: "predicate"}}
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{{ID: "shared", Action: "accept", Candidates: refs, Dependencies: deps}, {ID: "independent", Action: "accept", Candidates: independent, Dependencies: []memory.ReviewDependency{}}}})
	if err != nil {
		t.Fatal(err)
	}
	// The durable source bytes, policy and revisions remain unchanged. Only the
	// first group's exact source lookup returns a proven local ineligibility.
	fault := &batchReadFault{phase: "source", candidate: refs[0].ID, cause: sql.ErrNoRows, enabled: true}
	db := sql.OpenDB(batchFaultConnector{path: f.path, fault: fault})
	defer db.Close()
	store := NewStore(db)
	r, err := store.ResolveOwnerCandidateBatch(ctx, a, boundaryBatchDecision(p))
	if err != nil || len(r.Groups) != 2 || r.Groups[0].FailureCode != "source_ineligible" || r.Groups[1].Outcome != "accepted" || fault.hits == 0 {
		t.Fatalf("shared source failure %+v %v", r, err)
	}
	var entities, predicates, claims, resolutions int
	if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM semantic_entities WHERE canonical_name='Maya'),(SELECT count(*) FROM semantic_predicates WHERE token='likes'),(SELECT count(*) FROM semantic_claims WHERE created_operation_id=?),(SELECT count(*) FROM memory_review_resolutions WHERE candidate_id IN (?,?))`, p.Groups[0].Preview.Effect.OperationID, refs[0].ID, refs[1].ID).Scan(&entities, &predicates, &claims, &resolutions); err != nil {
		t.Fatal(err)
	}
	if entities+predicates+claims+resolutions != 0 {
		t.Fatalf("partial group persisted %d/%d/%d/%d", entities, predicates, claims, resolutions)
	}
	fault.enabled = false
	again, err := store.ResolveOwnerCandidateBatch(ctx, a, boundaryBatchDecision(p))
	if err != nil || string(compilerJSON(again)) != string(compilerJSON(r)) {
		t.Fatalf("failed group retried %+v %v", again, err)
	}
	replay, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !replay.Valid {
		t.Fatalf("partial replay %+v %v", replay, err)
	}
}
