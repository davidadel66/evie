package eviedb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

const semanticProcessRaceHelperEnvironment = "EVIE_SEMANTIC_PROCESS_RACE_HELPER"

type semanticProcessRaceRequest struct {
	Kind            string                          `json:"kind"`
	DBPath          string                          `json:"db_path"`
	ReadyPath       string                          `json:"ready_path"`
	ResultPath      string                          `json:"result_path"`
	Lease           memory.TurnLease                `json:"lease"`
	Remember        *memory.RememberLiteralProposal `json:"remember,omitempty"`
	Correction      *memory.CorrectClaimProposal    `json:"correction,omitempty"`
	Promotion       *memory.PromotionProposal       `json:"promotion,omitempty"`
	ProposalHash    string                          `json:"proposal_sha256"`
	PreparationHash string                          `json:"prepared_sha256"`
}

type semanticProcessRaceResult struct {
	Class  string          `json:"class"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func TestSemanticMemoryTwoProcessRacesCommitOnlyValidOperations(t *testing.T) {
	t.Run("idempotent mutation", testSemanticTwoProcessIdempotentMutation)
	t.Run("stale correction", testSemanticTwoProcessStaleCorrection)
	t.Run("stale Promotion", testSemanticTwoProcessStalePromotion)
}

func TestSemanticMemoryProcessRaceHelper(t *testing.T) {
	if os.Getenv(semanticProcessRaceHelperEnvironment) != "1" {
		return
	}
	encoded, err := os.ReadFile(os.Getenv("EVIE_SEMANTIC_PROCESS_RACE_REQUEST"))
	if err != nil {
		t.Fatal(err)
	}
	var request semanticProcessRaceRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDBAt(request.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := os.WriteFile(request.ReadyPath, []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	var release [1]byte
	if _, err := io.ReadFull(os.Stdin, release[:]); err != nil {
		t.Fatalf("wait for race release: %v", err)
	}

	store := NewStore(db)
	var result any
	switch request.Kind {
	case "remember_literal":
		if request.Remember == nil {
			t.Fatal("remember race omitted proposal")
		}
		request.Remember.ProposalSHA256 = request.ProposalHash
		request.Remember.PreparedSHA256 = request.PreparationHash
		result, err = store.ApplyRememberLiteral(context.Background(), request.Lease, *request.Remember)
	case "correct_claim":
		if request.Correction == nil {
			t.Fatal("correction race omitted proposal")
		}
		request.Correction.ProposalSHA256 = request.ProposalHash
		request.Correction.PreparedSHA256 = request.PreparationHash
		result, err = store.ApplyCorrectClaim(context.Background(), request.Lease, *request.Correction)
	case "promote_claim":
		if request.Promotion == nil {
			t.Fatal("Promotion race omitted proposal")
		}
		request.Promotion.ProposalSHA256 = request.ProposalHash
		request.Promotion.PreparedSHA256 = request.PreparationHash
		result, err = store.ApplyPromotion(context.Background(), request.Lease, *request.Promotion)
	default:
		t.Fatalf("unknown semantic process race kind %q", request.Kind)
	}

	encodedResult, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	response := semanticProcessRaceResult{Class: semanticProcessRaceErrorClass(err), Result: encodedResult}
	if err != nil {
		response.Error = err.Error()
	}
	encodedResponse, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(request.ResultPath, encodedResponse, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testSemanticTwoProcessIdempotentMutation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "process-idempotency", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Remember Detroit",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:9a000000-0000-4000-8000-000000000001", SourceEventID: event.ID,
		Predicate: "process_city", PredicateLabel: "process city",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	request := semanticProcessRaceRequest{
		Kind: "remember_literal", DBPath: path, Lease: lease, Remember: &proposal,
		ProposalHash: proposal.ProposalSHA256, PreparationHash: proposal.PreparedSHA256,
	}
	responses := runSemanticProcessRace(t, dir, request, request)
	if responses[0].Class != "ok" || responses[1].Class != "ok" {
		t.Fatalf("idempotent process results = %+v", responses)
	}
	var first, second memory.RememberLiteralResult
	decodeSemanticProcessResult(t, responses[0], &first)
	decodeSemanticProcessResult(t, responses[1], &second)
	if !reflect.DeepEqual(first, second) || first.OperationID != proposal.OperationID || first.ScopeRevision != 1 {
		t.Fatalf("idempotent process results differ: first=%+v second=%+v", first, second)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inspection, err := NewStore(db).InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 1 || len(inspection.Claims) != 1 ||
		inspection.Claims[0].ID != first.ClaimID || inspection.Claims[0].Source.ID != first.SourceLinkID {
		t.Fatalf("idempotent process projection = %+v", inspection)
	}
	assertSemanticTableCount(t, ctx, db, "semantic_operations", 1)
	assertSemanticTableCount(t, ctx, db, "semantic_claims", 1)
	assertSemanticTableCount(t, ctx, db, "semantic_source_links", 1)
}

func testSemanticTwoProcessStaleCorrection(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "process-correction", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	old := prepareLiteralForCorrection(t, ctx, store, session, lease,
		"idem:v1:9a000000-0000-4000-8000-000000000010", "I live in Detroit", "Detroit", memory.ValidTime{})
	oldResult, err := store.ApplyRememberLiteral(ctx, lease, old)
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(key, value string) memory.CorrectClaimProposal {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: value,
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
			IdempotencyKey: key, SourceEventID: event.ID, OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionError,
			Replacement: memory.ClaimProposition{
				SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID,
				Object:   memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: value}},
				Polarity: memory.PolarityAffirmed,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	proposals := []memory.CorrectClaimProposal{
		prepare("idem:v1:9a000000-0000-4000-8000-000000000011", "Chicago"),
		prepare("idem:v1:9a000000-0000-4000-8000-000000000012", "New York"),
	}
	if proposals[0].ExpectedRevision != proposals[1].ExpectedRevision {
		t.Fatalf("correction process race revisions differ: %d != %d", proposals[0].ExpectedRevision, proposals[1].ExpectedRevision)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	requests := make([]semanticProcessRaceRequest, 2)
	for index := range proposals {
		proposal := proposals[index]
		requests[index] = semanticProcessRaceRequest{
			Kind: "correct_claim", DBPath: path, Lease: lease, Correction: &proposal,
			ProposalHash: proposal.ProposalSHA256, PreparationHash: proposal.PreparedSHA256,
		}
	}
	responses := runSemanticProcessRace(t, dir, requests[0], requests[1])
	winner, loser := assertOneSemanticProcessSuccessAndStale(t, responses)
	var accepted memory.CorrectClaimResult
	decodeSemanticProcessResult(t, responses[winner], &accepted)
	if accepted.OperationID != proposals[winner].OperationID || accepted.ReplacementClaimID != proposals[winner].ReplacementClaim.ID {
		t.Fatalf("accepted correction = %+v, proposal = %+v", accepted, proposals[winner])
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inspection, err := NewStore(db).InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 2 || len(inspection.Claims) != 1 ||
		inspection.Claims[0].ID != accepted.ReplacementClaimID || inspection.Claims[0].Object.Literal == nil ||
		inspection.Claims[0].Object.Literal.Value != proposals[winner].ReplacementClaim.Object.Literal.Value {
		t.Fatalf("correction process projection = %+v", inspection)
	}
	for table, want := range map[string]int{
		"semantic_operations": 2, "semantic_claims": 2, "semantic_source_links": 2,
		"semantic_claim_corrections": 1,
	} {
		assertSemanticTableCount(t, ctx, db, table, want)
	}
	assertNoSemanticRowsForOperation(t, ctx, db, proposals[loser].OperationID)
}

func testSemanticTwoProcessStalePromotion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	sessions := make([]memory.Session, 2)
	leases := make([]memory.TurnLease, 2)
	sourceClaims := make([]memory.RememberEntityProposal, 2)
	proposals := make([]memory.PromotionProposal, 2)
	for index := range sessions {
		workspace, err := store.RegisterWorkspace(ctx, fmt.Sprintf("Process Promotion %d", index))
		if err != nil {
			t.Fatal(err)
		}
		sessions[index], err = store.CreateWorkspaceSessionWithComposition(
			ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t),
		)
		if err != nil {
			t.Fatal(err)
		}
		sourceClaims[index] = rememberScopeClaim(t, ctx, store, sessions[index], false, 410+index)
		leases[index], err = store.AcquireTurnLease(ctx, sessions[index].ID,
			memory.LeaseHolderID(fmt.Sprintf("process-promotion-%d", index)), 10*time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		event, err := store.AppendEventWithLease(ctx, sessions[index].ID, leases[index].HolderID, leases[index].FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote explicitly",
		})
		if err != nil {
			t.Fatal(err)
		}
		proposals[index], err = store.PreparePromotion(ctx, sessions[index].ScopeContext(), memory.PromotionRequest{
			IdempotencyKey: fmt.Sprintf("idem:v1:9a000000-0000-4000-8001-%012d", index+1),
			SourceEventID:  event.ID, SourceClaimID: sourceClaims[index].Claim.ID, DestinationScopeKey: "global",
		})
		if err != nil {
			t.Fatal(err)
		}
		approvePromotion(t, ctx, store, leases[index], proposals[index], memory.ApprovalApproved)
	}
	if proposals[0].DestinationScope.Revision != proposals[1].DestinationScope.Revision {
		t.Fatalf("Promotion process race revisions differ: %d != %d",
			proposals[0].DestinationScope.Revision, proposals[1].DestinationScope.Revision)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	requests := make([]semanticProcessRaceRequest, 2)
	for index := range proposals {
		proposal := proposals[index]
		requests[index] = semanticProcessRaceRequest{
			Kind: "promote_claim", DBPath: path, Lease: leases[index], Promotion: &proposal,
			ProposalHash: proposal.ProposalSHA256, PreparationHash: proposal.PreparedSHA256,
		}
	}
	responses := runSemanticProcessRace(t, dir, requests[0], requests[1])
	winner, loser := assertOneSemanticProcessSuccessAndStale(t, responses)
	var accepted memory.PromotionResult
	decodeSemanticProcessResult(t, responses[winner], &accepted)
	if accepted.OperationID != proposals[winner].OperationID || accepted.DestinationClaimID != proposals[winner].DestinationClaim.ID {
		t.Fatalf("accepted Promotion = %+v, proposal = %+v", accepted, proposals[winner])
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	global, err := NewStore(db).CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := NewStore(db).InspectClaims(ctx, global.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != accepted.DestinationRevision || len(inspection.Claims) != 1 ||
		inspection.Claims[0].ID != accepted.DestinationClaimID {
		t.Fatalf("Promotion process projection = %+v, accepted = %+v", inspection, accepted)
	}
	assertSemanticTableCount(t, ctx, db, "semantic_promotions", 1)
	assertNoSemanticRowsForOperation(t, ctx, db, proposals[loser].OperationID)
	for index := range sessions {
		workspaceInspection, err := NewStore(db).InspectClaims(ctx, sessions[index].ScopeContext(), memory.ClaimQuery{})
		if err != nil {
			t.Fatal(err)
		}
		foundSource := false
		for _, claim := range workspaceInspection.Claims {
			foundSource = foundSource || claim.ID == sourceClaims[index].Claim.ID
		}
		if !foundSource {
			t.Fatalf("source Workspace %d changed by Promotion race: %+v", index, workspaceInspection)
		}
	}
}

func runSemanticProcessRace(t *testing.T, dir string, requests ...semanticProcessRaceRequest) []semanticProcessRaceResult {
	t.Helper()
	type child struct {
		command *exec.Cmd
		stdin   io.WriteCloser
		stdout  bytes.Buffer
		stderr  bytes.Buffer
	}
	children := make([]child, len(requests))
	t.Cleanup(func() {
		for index := range children {
			if children[index].stdin != nil {
				_ = children[index].stdin.Close()
			}
			if children[index].command != nil && children[index].command.Process != nil && children[index].command.ProcessState == nil {
				_ = children[index].command.Process.Kill()
				_ = children[index].command.Wait()
			}
		}
	})
	for index := range requests {
		requests[index].ReadyPath = filepath.Join(dir, fmt.Sprintf("ready-%d", index))
		requests[index].ResultPath = filepath.Join(dir, fmt.Sprintf("result-%d.json", index))
		requestPath := filepath.Join(dir, fmt.Sprintf("request-%d.json", index))
		encoded, err := json.Marshal(requests[index])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(requestPath, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		command := exec.Command(os.Args[0], "-test.run=^TestSemanticMemoryProcessRaceHelper$", "-test.count=1", "-test.timeout=30s")
		command.Env = append(os.Environ(),
			semanticProcessRaceHelperEnvironment+"=1",
			"EVIE_SEMANTIC_PROCESS_RACE_REQUEST="+requestPath,
		)
		stdin, err := command.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		children[index] = child{command: command, stdin: stdin}
		command.Stdout = &children[index].stdout
		command.Stderr = &children[index].stderr
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for index := range requests {
		waitForSemanticProcessReady(t, requests[index].ReadyPath)
	}
	for index := range children {
		if _, err := children[index].stdin.Write([]byte{1}); err != nil {
			t.Fatalf("release semantic process %d: %v", index, err)
		}
		if err := children[index].stdin.Close(); err != nil {
			t.Fatalf("close semantic process %d stdin: %v", index, err)
		}
	}
	for index := range children {
		if err := children[index].command.Wait(); err != nil {
			t.Fatalf("semantic process %d failed: %v\nstdout:\n%s\nstderr:\n%s",
				index, err, children[index].stdout.String(), children[index].stderr.String())
		}
	}
	responses := make([]semanticProcessRaceResult, len(requests))
	for index := range requests {
		encoded, err := os.ReadFile(requests[index].ResultPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &responses[index]); err != nil {
			t.Fatal(err)
		}
		if responses[index].Class != "ok" && responses[index].Class != "stale_scope_revision" {
			t.Fatalf("unexpected semantic process %d result: %+v", index, responses[index])
		}
	}
	return responses
}

func waitForSemanticProcessReady(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("semantic subprocess did not become ready: %s", path)
		}
		time.Sleep(time.Millisecond)
	}
}

func semanticProcessRaceErrorClass(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, ErrStaleScopeRevision):
		return "stale_scope_revision"
	case errors.Is(err, ErrIdempotencyConflict):
		return "idempotency_conflict"
	default:
		return "unexpected"
	}
}

func assertOneSemanticProcessSuccessAndStale(t *testing.T, responses []semanticProcessRaceResult) (winner, loser int) {
	t.Helper()
	winner, loser = -1, -1
	for index, response := range responses {
		switch response.Class {
		case "ok":
			winner = index
		case "stale_scope_revision":
			loser = index
		default:
			t.Fatalf("semantic process result %d = %+v", index, response)
		}
	}
	if winner == -1 || loser == -1 {
		t.Fatalf("semantic process race did not yield one success and one stale result: %+v", responses)
	}
	return winner, loser
}

func decodeSemanticProcessResult(t *testing.T, response semanticProcessRaceResult, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Result, target); err != nil {
		t.Fatal(err)
	}
}

func assertSemanticTableCount(t *testing.T, ctx context.Context, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s rows = %d, want %d", table, got, want)
	}
}

func assertNoSemanticRowsForOperation(t *testing.T, ctx context.Context, db *sql.DB, operationID memory.SemanticID) {
	t.Helper()
	queries := map[string]string{
		"semantic_operations":         `SELECT COUNT(*) FROM semantic_operations WHERE operation_id = ?`,
		"semantic_entities":           `SELECT COUNT(*) FROM semantic_entities WHERE created_operation_id = ?`,
		"semantic_aliases":            `SELECT COUNT(*) FROM semantic_aliases WHERE created_operation_id = ?`,
		"semantic_claims":             `SELECT COUNT(*) FROM semantic_claims WHERE created_operation_id = ?`,
		"semantic_source_links":       `SELECT COUNT(*) FROM semantic_source_links WHERE created_operation_id = ?`,
		"semantic_claim_corrections":  `SELECT COUNT(*) FROM semantic_claim_corrections WHERE operation_id = ?`,
		"semantic_promotions":         `SELECT COUNT(*) FROM semantic_promotions WHERE operation_id = ?`,
		"semantic_promotion_entities": `SELECT COUNT(*) FROM semantic_promotion_entities WHERE operation_id = ?`,
		"semantic_state_events":       `SELECT COUNT(*) FROM semantic_state_events WHERE operation_id = ?`,
	}
	for table, query := range queries {
		var rows int
		if err := db.QueryRowContext(ctx, query, operationID).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 0 {
			t.Fatalf("stale operation %s wrote %d rows to %s", operationID, rows, table)
		}
	}
}
