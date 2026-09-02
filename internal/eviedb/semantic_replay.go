package eviedb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

var (
	ErrSemanticScopeQuarantined = errors.New("semantic memory: scope is quarantined")
	ErrSemanticMaintenanceHeld  = errors.New("semantic memory: projection maintenance is already held")
	ErrSemanticReplay           = errors.New("semantic memory: replay failed")
)

type SemanticReplayError struct {
	OperationID   memory.SemanticID
	SchemaVersion int
	Cause         error
}

func (e *SemanticReplayError) Error() string {
	return fmt.Sprintf("%v at operation %s (schema version %d): %v", ErrSemanticReplay, e.OperationID, e.SchemaVersion, e.Cause)
}

func (e *SemanticReplayError) Unwrap() error { return errors.Join(ErrSemanticReplay, e.Cause) }

type semanticAcceptedReplayOperation struct {
	OperationID     memory.SemanticID
	SchemaVersion   int
	Kind            string
	IdempotencyKey  string
	Actor           memory.SemanticActor
	SessionID       memory.SessionID
	TargetScopeID   memory.SemanticID
	SourceEventID   memory.EventID
	ProposalHash    string
	EffectHash      string
	ProposalJSON    string
	PreparedJSON    string
	ResultJSON      string
	TransactionTime time.Time
	Scopes          []semanticReplayOperationScope
}

type semanticReplayOperationScope struct {
	ScopeID  memory.SemanticID
	ScopeKey string
	Prior    int64
	Result   int64
	Written  bool
}

type semanticProjectionTableSnapshot struct {
	Hash          string
	Rows          int64
	CanonicalRows []string
}

type semanticProjectionScopeSnapshot struct {
	ScopeKey string
	ScopeID  memory.SemanticID
	Revision int64
	Hash     string
	Frontier string
	Tables   map[string]semanticProjectionTableSnapshot
}

type semanticProjectionShadow struct {
	db                  *sql.DB
	dir                 string
	path                string
	live                map[string]semanticProjectionScopeSnapshot
	shadow              map[string]semanticProjectionScopeSnapshot
	liveForeignFailures []semanticForeignKeyFailure
}

type semanticForeignKeyFailure struct {
	Table     string
	RowID     sql.NullInt64
	Parent    string
	ForeignID int64
	ScopeKeys []string
}

func (failure semanticForeignKeyFailure) canonical() string {
	rowID := "NULL"
	if failure.RowID.Valid {
		rowID = strconv.FormatInt(failure.RowID.Int64, 10)
	}
	return fmt.Sprintf("table=%s;rowid=%s;parent=%s;foreign_key=%d", failure.Table, rowID, failure.Parent, failure.ForeignID)
}

func (s *semanticProjectionShadow) close() {
	if s.db != nil {
		_ = s.db.Close()
	}
	if s.dir != "" {
		_ = os.RemoveAll(s.dir)
	}
}

var semanticProjectionTableDescriptors = []struct{ name string }{
	{"semantic_predicates"},
	{"semantic_entities"},
	{"semantic_claims"},
	{"semantic_aliases"},
	{"semantic_source_links"},
	{"semantic_graph_links"},
	{"semantic_claim_corrections"},
	{"semantic_promotions"},
	{"semantic_promotion_entities"},
	{"semantic_state_events"},
}

func semanticProjectionTableNames() []string {
	names := []string{"semantic_scopes"}
	for _, descriptor := range semanticProjectionTableDescriptors {
		names = append(names, descriptor.name)
	}
	return names
}

func semanticProjectionInsertOrder() []string {
	names := make([]string, 0, len(semanticProjectionTableDescriptors))
	for _, descriptor := range semanticProjectionTableDescriptors {
		names = append(names, descriptor.name)
	}
	return names
}

func semanticProjectionDeleteOrder() []string {
	insertOrder := semanticProjectionInsertOrder()
	for left, right := 0, len(insertOrder)-1; left < right; left, right = left+1, right-1 {
		insertOrder[left], insertOrder[right] = insertOrder[right], insertOrder[left]
	}
	return insertOrder
}

// VerifySemanticProjection replays the immutable accepted-operation stream into
// a disposable real-SQLite projection. It never changes live semantic rows; a
// mismatch only records the affected scope's quarantine state.
func (s *Store) VerifySemanticProjection(ctx context.Context) (memory.SemanticProjectionVerification, error) {
	shadow, err := s.buildSemanticProjectionShadow(ctx)
	if err != nil {
		var replayErr *SemanticReplayError
		if errors.As(err, &replayErr) {
			if quarantineErr := s.quarantineReplayOperation(context.WithoutCancel(ctx), replayErr); quarantineErr != nil {
				return memory.SemanticProjectionVerification{}, errors.Join(err, fmt.Errorf("quarantine failed replay scope: %w", quarantineErr))
			}
		}
		return memory.SemanticProjectionVerification{}, err
	}
	defer shadow.close()
	report := compareSemanticProjectionSnapshots(shadow.live, shadow.shadow)
	addSemanticForeignKeyFailures(&report, shadow.liveForeignFailures)
	if !report.Valid {
		if err := s.quarantineProjectionReport(context.WithoutCancel(ctx), report, "canonical replay mismatch", ""); err != nil {
			return report, err
		}
		for index := range report.Scopes {
			if len(report.Scopes[index].Mismatches) != 0 || report.Scopes[index].LiveHash != report.Scopes[index].ShadowHash ||
				report.Scopes[index].LiveRevision != report.Scopes[index].ShadowRevision || report.Scopes[index].LiveFrontier != report.Scopes[index].ShadowFrontier {
				report.Scopes[index].Quarantined = !strings.HasPrefix(report.Scopes[index].ScopeKey, "unattributed:")
			}
		}
	}
	return report, nil
}

// OwnerRebuildSemanticProjection is deliberately absent from agent and tool
// interfaces. It fences semantic writers, validates a complete shadow replay,
// and swaps only projection rows in one live-database transaction.
func (s *Store) OwnerRebuildSemanticProjection(ctx context.Context, holderID string) (memory.SemanticProjectionRebuild, error) {
	token, err := s.acquireSemanticMaintenance(ctx, holderID, 5*time.Minute)
	if err != nil {
		return memory.SemanticProjectionRebuild{}, err
	}
	if s.semanticMaintenance.afterLock != nil {
		if err := s.semanticMaintenance.afterLock(); err != nil {
			return memory.SemanticProjectionRebuild{}, err
		}
	}
	released := false
	defer func() {
		if released {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.releaseSemanticMaintenance(releaseCtx, holderID, token)
	}()

	shadow, err := s.buildSemanticProjectionShadow(ctx)
	if err != nil {
		return memory.SemanticProjectionRebuild{}, err
	}
	defer shadow.close()
	if s.semanticMaintenance.beforeShadowValidation != nil {
		if err := s.semanticMaintenance.beforeShadowValidation(shadow.db); err != nil {
			return memory.SemanticProjectionRebuild{}, err
		}
	}
	validatedShadow, err := snapshotSemanticProjection(ctx, shadow.db)
	if err != nil {
		return memory.SemanticProjectionRebuild{}, err
	}
	shadowSelf := compareSemanticProjectionSnapshots(shadow.shadow, validatedShadow)
	if !shadowSelf.Valid {
		return memory.SemanticProjectionRebuild{}, errors.New("semantic memory: replayed shadow did not verify")
	}
	foreignRows, err := shadow.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return memory.SemanticProjectionRebuild{}, err
	}
	if foreignRows.Next() {
		_ = foreignRows.Close()
		return memory.SemanticProjectionRebuild{}, errors.New("semantic memory: replayed shadow failed foreign-key verification")
	}
	if err := foreignRows.Close(); err != nil {
		return memory.SemanticProjectionRebuild{}, err
	}
	if s.semanticMaintenance.beforeSwap != nil {
		if err := s.semanticMaintenance.beforeSwap(); err != nil {
			return memory.SemanticProjectionRebuild{}, err
		}
	}
	if err := shadow.db.Close(); err != nil {
		shadow.db = nil
		return memory.SemanticProjectionRebuild{}, err
	}
	shadow.db = nil
	if err := s.swapSemanticProjection(ctx, shadow.path, holderID, token); err != nil {
		return memory.SemanticProjectionRebuild{}, err
	}
	released = true
	return memory.SemanticProjectionRebuild{SemanticProjectionVerification: shadowSelf, FencingToken: token}, nil
}

func (s *Store) buildSemanticProjectionShadow(ctx context.Context) (*semanticProjectionShadow, error) {
	dir, err := os.MkdirTemp("", "evie-semantic-shadow-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	path := filepath.Join(dir, "projection.db")
	cleanup := func(err error) (*semanticProjectionShadow, error) {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return cleanup(fmt.Errorf("snapshot semantic projection: %w", err))
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return cleanup(err)
	}
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		return cleanup(err)
	}
	db.SetMaxOpenConns(1)
	shadow := &semanticProjectionShadow{db: db, dir: dir, path: path}
	if err := db.PingContext(ctx); err != nil {
		shadow.close()
		return nil, err
	}
	operations, err := loadSemanticReplayOperations(ctx, db)
	if err != nil {
		shadow.close()
		return nil, err
	}
	shadow.liveForeignFailures, err = loadSemanticForeignKeyFailures(ctx, db)
	if err != nil {
		shadow.close()
		return nil, err
	}
	shadow.live, err = snapshotSemanticProjection(ctx, db)
	if err != nil {
		shadow.close()
		return nil, err
	}
	if err := resetSemanticProjection(ctx, db); err != nil {
		shadow.close()
		return nil, err
	}
	if err := replaySemanticOperations(ctx, db, operations); err != nil {
		shadow.close()
		return nil, err
	}
	shadow.shadow, err = snapshotSemanticProjection(ctx, db)
	if err != nil {
		shadow.close()
		return nil, err
	}
	return shadow, nil
}

func loadSemanticReplayOperations(ctx context.Context, db *sql.DB) ([]semanticAcceptedReplayOperation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT operation_id, typeof(schema_version), CAST(schema_version AS TEXT), operation_kind,
		       idempotency_key, actor, session_id, target_scope_id, source_event_id,
		       proposal_sha256, effect_sha256, proposal_json, prepared_proposal_json, result_json, transaction_time
		FROM semantic_operations ORDER BY transaction_time, operation_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var operations []semanticAcceptedReplayOperation
	for rows.Next() {
		var operation semanticAcceptedReplayOperation
		var schemaType, schemaText, transactionTime string
		if err := rows.Scan(&operation.OperationID, &schemaType, &schemaText, &operation.Kind,
			&operation.IdempotencyKey, &operation.Actor, &operation.SessionID, &operation.TargetScopeID, &operation.SourceEventID,
			&operation.ProposalHash, &operation.EffectHash, &operation.ProposalJSON,
			&operation.PreparedJSON, &operation.ResultJSON, &transactionTime); err != nil {
			return nil, err
		}
		operation.SchemaVersion, err = strconv.Atoi(schemaText)
		if schemaType != "integer" || err != nil || operation.SchemaVersion <= 0 {
			return nil, &SemanticReplayError{OperationID: operation.OperationID, SchemaVersion: operation.SchemaVersion,
				Cause: fmt.Errorf("malformed operation schema version %q with SQLite type %s", schemaText, schemaType)}
		}
		operation.TransactionTime, err = parseSemanticTime(transactionTime)
		if err != nil {
			return nil, &SemanticReplayError{OperationID: operation.OperationID, SchemaVersion: operation.SchemaVersion, Cause: err}
		}
		operations = append(operations, operation)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range operations {
		operation := &operations[index]
		scopeRows, err := db.QueryContext(ctx, `
			SELECT scopes.scope_id, scopes.scope_key, operation_scopes.prior_revision, operation_scopes.resulting_revision, operation_scopes.written
			FROM semantic_operation_scopes AS operation_scopes
			JOIN semantic_scopes AS scopes ON scopes.scope_id = operation_scopes.scope_id
			WHERE operation_scopes.operation_id = ? ORDER BY scopes.scope_key
		`, operation.OperationID)
		if err != nil {
			return nil, err
		}
		for scopeRows.Next() {
			var scope semanticReplayOperationScope
			if err := scopeRows.Scan(&scope.ScopeID, &scope.ScopeKey, &scope.Prior, &scope.Result, &scope.Written); err != nil {
				_ = scopeRows.Close()
				return nil, err
			}
			operation.Scopes = append(operation.Scopes, scope)
		}
		if err := scopeRows.Close(); err != nil {
			return nil, err
		}
		if len(operation.Scopes) == 0 {
			return nil, &SemanticReplayError{OperationID: operation.OperationID, SchemaVersion: operation.SchemaVersion, Cause: errors.New("operation has no scope frontier")}
		}
		if err := validateSemanticReplayOperationPreflight(*operation); err != nil {
			return nil, &SemanticReplayError{OperationID: operation.OperationID, SchemaVersion: operation.SchemaVersion, Cause: err}
		}
	}
	return operations, nil
}

type semanticReplayPreparedHandler struct {
	SchemaVersion     int
	Kind              string
	OperationID       memory.SemanticID
	IdempotencyKey    string
	Actor             memory.SemanticActor
	SessionID         memory.SessionID
	TargetScope       memory.SemanticScope
	Scopes            []memory.SemanticScope
	PriorRevisions    []memory.ScopeRevision
	SourceEventID     memory.EventID
	PreparedProposal  any
	CanonicalProposal any
	CanonicalEffect   any
	ValidateResult    func(semanticAcceptedReplayOperation) error
	Replay            func(context.Context, *Store, semanticAcceptedReplayOperation) (any, error)
}

func semanticReplayHandler(operation semanticAcceptedReplayOperation) (semanticReplayPreparedHandler, error) {
	base := func(schemaVersion int, kind string, operationID memory.SemanticID, idempotencyKey string, actor memory.SemanticActor,
		sessionID memory.SessionID, targetScope memory.SemanticScope, scopes []memory.SemanticScope, priors []memory.ScopeRevision,
		sourceEventID memory.EventID, prepared, canonicalProposal, canonicalEffect any,
		validateResult func(semanticAcceptedReplayOperation) error,
		replay func(context.Context, *Store, semanticAcceptedReplayOperation) (any, error),
	) semanticReplayPreparedHandler {
		return semanticReplayPreparedHandler{SchemaVersion: schemaVersion, Kind: kind, OperationID: operationID,
			IdempotencyKey: idempotencyKey, Actor: actor, SessionID: sessionID, TargetScope: targetScope, Scopes: scopes,
			PriorRevisions: priors, SourceEventID: sourceEventID, PreparedProposal: prepared, CanonicalProposal: canonicalProposal,
			CanonicalEffect: canonicalEffect, ValidateResult: validateResult, Replay: replay}
	}
	switch {
	case operation.SchemaVersion == 1 && operation.Kind == "remember_literal_claim":
		var proposal memory.RememberLiteralProposal
		if err := strictSemanticProposal(operation.PreparedJSON, &proposal); err != nil {
			return semanticReplayPreparedHandler{}, err
		}
		canonical := canonicalRememberLiteralProposal(proposal)
		return base(proposal.SchemaVersion, proposal.Kind, proposal.OperationID, proposal.IdempotencyKey, proposal.Actor, proposal.SessionID,
			proposal.Scope, proposal.Scopes, proposal.PriorRevisions, proposal.Source.EventID, proposal, canonical, canonical.Effect,
			func(operation semanticAcceptedReplayOperation) error {
				return validateConcreteReplayResult(operation, &memory.RememberLiteralResult{},
					func(value memory.RememberLiteralResult) (memory.SemanticID, time.Time, []memory.ScopeRevision) {
						return value.OperationID, value.TransactionTime, value.ResultingRevisions
					}, func(value memory.RememberLiteralResult) bool {
						return value.ClaimID == proposal.ClaimID && value.SourceLinkID == proposal.SourceLinkID && value.ScopeRevision == replayResultRevision(operation, proposal.Scope.Key)
					})
			}, func(ctx context.Context, store *Store, operation semanticAcceptedReplayOperation) (any, error) {
				candidate := proposal
				candidate.ProposalSHA256 = operation.ProposalHash
				candidate.PreparedSHA256, _, _ = semanticHash(candidate)
				return store.ApplyRememberLiteral(ctx, replayLease(candidate.SessionID), candidate)
			}), nil
	case operation.SchemaVersion == 1 && operation.Kind == "remember_entity_claim":
		var proposal memory.RememberEntityProposal
		if err := strictSemanticProposal(operation.PreparedJSON, &proposal); err != nil {
			return semanticReplayPreparedHandler{}, err
		}
		canonical := canonicalRememberEntityProposal(proposal)
		return base(proposal.SchemaVersion, proposal.Kind, proposal.OperationID, proposal.IdempotencyKey, proposal.Actor, proposal.SessionID,
			proposal.Scope, proposal.Scopes, proposal.PriorRevisions, proposal.Source.EventID, proposal, canonical, canonical.Effect,
			func(operation semanticAcceptedReplayOperation) error {
				return validateConcreteReplayResult(operation, &memory.RememberEntityResult{},
					func(value memory.RememberEntityResult) (memory.SemanticID, time.Time, []memory.ScopeRevision) {
						return value.OperationID, value.TransactionTime, value.ResultingRevisions
					}, func(value memory.RememberEntityResult) bool {
						return value.ClaimID == proposal.Claim.ID && value.SourceLinkID == proposal.Source.ID && value.ScopeRevision == replayResultRevision(operation, proposal.Scope.Key)
					})
			}, func(ctx context.Context, store *Store, operation semanticAcceptedReplayOperation) (any, error) {
				candidate := proposal
				candidate.ProposalSHA256 = operation.ProposalHash
				candidate.PreparedSHA256, _, _ = semanticHash(candidate)
				return store.ApplyRememberEntity(ctx, replayLease(candidate.SessionID), candidate)
			}), nil
	case operation.SchemaVersion == 2 && operation.Kind == "correct_claim":
		var proposal memory.CorrectClaimProposal
		if err := strictSemanticProposal(operation.PreparedJSON, &proposal); err != nil {
			return semanticReplayPreparedHandler{}, err
		}
		canonical := canonicalCorrectClaimProposal(proposal)
		return base(proposal.SchemaVersion, proposal.Kind, proposal.OperationID, proposal.IdempotencyKey, proposal.Actor, proposal.SessionID,
			proposal.Scope, proposal.Scopes, proposal.PriorRevisions, proposal.Source.EventID, proposal, canonical, canonical.Effect,
			func(operation semanticAcceptedReplayOperation) error {
				return validateConcreteReplayResult(operation, &memory.CorrectClaimResult{},
					func(value memory.CorrectClaimResult) (memory.SemanticID, time.Time, []memory.ScopeRevision) {
						return value.OperationID, value.TransactionTime, value.ResultingRevisions
					}, func(value memory.CorrectClaimResult) bool {
						return value.OldClaimID == proposal.OldClaim.ID && value.ReplacementClaimID == proposal.ReplacementClaim.ID &&
							value.SourceLinkID == proposal.Source.ID && value.ScopeRevision == replayResultRevision(operation, proposal.Scope.Key)
					})
			}, func(ctx context.Context, store *Store, operation semanticAcceptedReplayOperation) (any, error) {
				candidate := proposal
				candidate.ProposalSHA256 = operation.ProposalHash
				candidate.PreparedSHA256, _, _ = semanticHash(candidate)
				return store.ApplyCorrectClaim(ctx, replayLease(candidate.SessionID), candidate)
			}), nil
	case (operation.SchemaVersion == 3 || operation.SchemaVersion == 5) && isSemanticLifecycleKind(operation.Kind):
		var proposal memory.MemoryLifecycleProposal
		if err := strictSemanticProposal(operation.PreparedJSON, &proposal); err != nil {
			return semanticReplayPreparedHandler{}, err
		}
		canonical := canonicalMemoryLifecycleProposal(proposal)
		return base(proposal.SchemaVersion, proposal.Kind, proposal.OperationID, proposal.IdempotencyKey, proposal.Actor, proposal.SessionID,
			proposal.Scope, proposal.Scopes, proposal.PriorRevisions, proposal.Evidence.EventID, proposal, canonical, canonical.Effect,
			func(operation semanticAcceptedReplayOperation) error {
				return validateConcreteReplayResult(operation, &memory.MemoryLifecycleResult{},
					func(value memory.MemoryLifecycleResult) (memory.SemanticID, time.Time, []memory.ScopeRevision) {
						return value.OperationID, value.TransactionTime, value.ResultingRevisions
					}, func(value memory.MemoryLifecycleResult) bool {
						return value.ObjectKind == proposal.ObjectKind && value.ObjectID == proposal.ObjectID && value.ScopeRevision == replayResultRevision(operation, proposal.Scope.Key)
					})
			}, func(ctx context.Context, store *Store, operation semanticAcceptedReplayOperation) (any, error) {
				candidate := proposal
				candidate.ProposalSHA256 = operation.ProposalHash
				candidate.PreparedSHA256, _, _ = semanticHash(candidate)
				return store.ApplyMemoryLifecycle(ctx, replayLease(candidate.SessionID), candidate)
			}), nil
	case operation.SchemaVersion == 4 && operation.Kind == "promote_claim":
		var proposal memory.PromotionProposal
		if err := strictSemanticProposal(operation.PreparedJSON, &proposal); err != nil {
			return semanticReplayPreparedHandler{}, err
		}
		canonical := canonicalPromoteClaimProposal(proposal)
		return base(proposal.SchemaVersion, proposal.Kind, proposal.OperationID, proposal.IdempotencyKey, proposal.Actor, proposal.SessionID,
			proposal.DestinationScope, proposal.Scopes, proposal.PriorRevisions, proposal.Evidence.EventID, proposal, canonical, canonical.Effect,
			func(operation semanticAcceptedReplayOperation) error {
				return validateConcreteReplayResult(operation, &memory.PromotionResult{},
					func(value memory.PromotionResult) (memory.SemanticID, time.Time, []memory.ScopeRevision) {
						return value.OperationID, value.TransactionTime, value.ResultingRevisions
					}, func(value memory.PromotionResult) bool {
						return value.SourceClaimID == proposal.SourceClaim.ID && value.DestinationClaimID == proposal.DestinationClaim.ID &&
							value.DestinationRevision == replayResultRevision(operation, proposal.DestinationScope.Key)
					})
			}, func(ctx context.Context, store *Store, operation semanticAcceptedReplayOperation) (any, error) {
				candidate := proposal
				candidate.ProposalSHA256 = operation.ProposalHash
				candidate.PreparedSHA256, _, _ = semanticHash(candidate)
				return store.ApplyPromotion(ctx, replayLease(candidate.SessionID), candidate)
			}), nil
	case operation.SchemaVersion == 5 && operation.Kind == "create_graph_link":
		var proposal memory.CreateGraphLinkProposal
		if err := strictSemanticProposal(operation.PreparedJSON, &proposal); err != nil {
			return semanticReplayPreparedHandler{}, err
		}
		canonical := canonicalCreateGraphLinkProposal(proposal)
		return base(proposal.SchemaVersion, proposal.Kind, proposal.OperationID, proposal.IdempotencyKey, proposal.Actor, proposal.SessionID,
			proposal.Scope, proposal.Scopes, proposal.PriorRevisions, proposal.Evidence.EventID, proposal, canonical, canonical.Effect,
			func(operation semanticAcceptedReplayOperation) error {
				return validateConcreteReplayResult(operation, &memory.CreateGraphLinkResult{},
					func(value memory.CreateGraphLinkResult) (memory.SemanticID, time.Time, []memory.ScopeRevision) {
						return value.OperationID, value.TransactionTime, value.ResultingRevisions
					}, func(value memory.CreateGraphLinkResult) bool {
						return value.GraphLinkID == proposal.Link.ID && value.ScopeRevision == replayResultRevision(operation, proposal.Scope.Key)
					})
			}, func(ctx context.Context, store *Store, operation semanticAcceptedReplayOperation) (any, error) {
				candidate := proposal
				candidate.ProposalSHA256 = operation.ProposalHash
				candidate.PreparedSHA256, _, _ = semanticHash(candidate)
				return store.ApplyCreateGraphLink(ctx, replayLease(candidate.SessionID), candidate)
			}), nil
	default:
		return semanticReplayPreparedHandler{}, fmt.Errorf("unknown operation kind %q for schema version %d", operation.Kind, operation.SchemaVersion)
	}
}

func validateSemanticReplayOperationPreflight(operation semanticAcceptedReplayOperation) error {
	if err := validateSemanticUUID(string(operation.OperationID)); err != nil {
		return fmt.Errorf("invalid operation ID: %w", err)
	}
	for label, id := range map[string]string{
		"session": string(operation.SessionID), "target scope": string(operation.TargetScopeID), "source event": string(operation.SourceEventID),
	} {
		if err := validateSemanticUUID(id); err != nil {
			return fmt.Errorf("invalid %s ID: %w", label, err)
		}
	}
	if operation.Actor != memory.SemanticActorOwner {
		return fmt.Errorf("invalid accepted actor %q", operation.Actor)
	}
	if !strings.HasPrefix(operation.IdempotencyKey, "idem:v1:") || validateSemanticUUID(strings.TrimPrefix(operation.IdempotencyKey, "idem:v1:")) != nil {
		return errors.New("invalid accepted idempotency key")
	}

	handler, err := semanticReplayHandler(operation)
	if err != nil {
		return err
	}
	if handler.SchemaVersion != operation.SchemaVersion || handler.Kind != operation.Kind || handler.OperationID != operation.OperationID ||
		handler.IdempotencyKey != operation.IdempotencyKey || handler.Actor != operation.Actor || handler.SessionID != operation.SessionID ||
		handler.TargetScope.ID != operation.TargetScopeID || handler.SourceEventID != operation.SourceEventID {
		return errors.New("accepted operation envelope differs from its prepared proposal")
	}
	if err := validateReplayScopeVectors(handler.Scopes, handler.PriorRevisions, operation.Scopes); err != nil {
		return err
	}
	proposalHash, proposalJSON, err := semanticHash(handler.CanonicalProposal)
	if err != nil {
		return err
	}
	if proposalHash != operation.ProposalHash || string(proposalJSON) != operation.ProposalJSON {
		return errors.New("accepted proposal JSON or hash differs from its prepared proposal")
	}
	effectHash, _, err := semanticHash(handler.CanonicalEffect)
	if err != nil {
		return err
	}
	if effectHash != operation.EffectHash {
		return errors.New("accepted effect hash differs from its prepared proposal")
	}
	preparedJSON, err := json.Marshal(handler.PreparedProposal)
	if err != nil {
		return err
	}
	if string(preparedJSON) != operation.PreparedJSON {
		return errors.New("accepted prepared proposal JSON is not canonical")
	}
	if err := validateSemanticIDsInJSON(operation.PreparedJSON); err != nil {
		return err
	}
	return handler.ValidateResult(operation)
}

func validateReplayScopeVectors(scopes []memory.SemanticScope, priors []memory.ScopeRevision, recorded []semanticReplayOperationScope) error {
	if len(scopes) != len(recorded) || len(priors) != len(recorded) {
		return errors.New("accepted operation scope vector is incomplete")
	}
	for index := range recorded {
		if scopes[index].ID != recorded[index].ScopeID || scopes[index].Key != recorded[index].ScopeKey || scopes[index].Revision != recorded[index].Prior ||
			priors[index].ScopeKey != recorded[index].ScopeKey || priors[index].Revision != recorded[index].Prior ||
			recorded[index].Written != (recorded[index].Result != recorded[index].Prior) || recorded[index].Result < recorded[index].Prior || recorded[index].Result > recorded[index].Prior+1 {
			return errors.New("accepted operation scope frontier differs from its prepared proposal")
		}
	}
	return nil
}

func validateConcreteReplayResult[T any](operation semanticAcceptedReplayOperation, result *T,
	envelope func(T) (memory.SemanticID, time.Time, []memory.ScopeRevision), identitiesMatch func(T) bool,
) error {
	if err := strictSemanticProposal(operation.ResultJSON, result); err != nil {
		return fmt.Errorf("malformed accepted result: %w", err)
	}
	if !identitiesMatch(*result) {
		return errors.New("accepted result identities differ from its prepared proposal")
	}
	canonical, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if string(canonical) != operation.ResultJSON {
		return errors.New("accepted result JSON is not canonical")
	}
	operationID, transactionTime, revisions := envelope(*result)
	if operationID != operation.OperationID || !transactionTime.Equal(operation.TransactionTime) || len(revisions) != len(operation.Scopes) {
		return errors.New("accepted result envelope differs from its operation")
	}
	for index, revision := range revisions {
		if revision.ScopeKey != operation.Scopes[index].ScopeKey || revision.Revision != operation.Scopes[index].Result {
			return errors.New("accepted result revision vector differs from its operation frontier")
		}
	}
	return validateSemanticIDsInJSON(operation.ResultJSON)
}

func replayResultRevision(operation semanticAcceptedReplayOperation, scopeKey string) int64 {
	for _, scope := range operation.Scopes {
		if scope.ScopeKey == scopeKey {
			return scope.Result
		}
	}
	return -1
}

func validateSemanticIDsInJSON(raw string) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var walk func(any, string) error
	walk = func(current any, field string) error {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if err := walk(child, key); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range typed {
				if err := walk(child, field); err != nil {
					return err
				}
			}
		case string:
			if typed != "" && (strings.HasSuffix(field, "_id") || strings.HasSuffix(field, "_ids")) {
				if err := validateSemanticUUID(typed); err != nil {
					return fmt.Errorf("invalid %s %q: %w", field, typed, err)
				}
			}
		}
		return nil
	}
	return walk(value, "")
}

func resetSemanticProjection(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	tables := []string{"semantic_projection_quarantine", "semantic_maintenance_lock"}
	tables = append(tables, semanticProjectionDeleteOrder()...)
	tables = append(tables, "semantic_operation_scopes", "semantic_operations", "semantic_cursor_auth", "semantic_scopes")
	for _, table := range tables {
		if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS `+table); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	if err := ensureSemanticSchema(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM session_turn_leases`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `UPDATE sessions SET status = 'active'`)
	return err
}

func replaySemanticOperations(ctx context.Context, db *sql.DB, operations []semanticAcceptedReplayOperation) error {
	store := NewStore(db)
	current := make(map[string]int64)
	remaining := append([]semanticAcceptedReplayOperation(nil), operations...)
	for len(remaining) != 0 {
		selected := -1
		for index, operation := range remaining {
			ready := true
			for _, scope := range operation.Scopes {
				if current[scope.ScopeKey] != scope.Prior {
					ready = false
					break
				}
			}
			if ready {
				selected = index
				break
			}
		}
		if selected < 0 {
			operation := remaining[0]
			return &SemanticReplayError{OperationID: operation.OperationID, SchemaVersion: operation.SchemaVersion, Cause: errors.New("operation scope frontier is non-contiguous")}
		}
		operation := remaining[selected]
		remaining = append(remaining[:selected], remaining[selected+1:]...)
		if err := ensureReplayLease(ctx, db, operation); err != nil {
			return &SemanticReplayError{OperationID: operation.OperationID, SchemaVersion: operation.SchemaVersion, Cause: err}
		}
		store.now = func() time.Time { return operation.TransactionTime }
		if err := store.replaySemanticOperation(ctx, operation); err != nil {
			return &SemanticReplayError{OperationID: operation.OperationID, SchemaVersion: operation.SchemaVersion, Cause: err}
		}
		for _, scope := range operation.Scopes {
			current[scope.ScopeKey] = scope.Result
		}
	}
	return nil
}

func ensureReplayLease(ctx context.Context, db *sql.DB, operation semanticAcceptedReplayOperation) error {
	var sessionID memory.SessionID
	if err := db.QueryRowContext(ctx, `SELECT session_id FROM events WHERE id = (SELECT source_event_id FROM semantic_operations WHERE operation_id = ?)`, operation.OperationID).Scan(&sessionID); err == nil {
		return nil
	}
	var envelope struct {
		SessionID memory.SessionID `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(operation.PreparedJSON), &envelope); err != nil || envelope.SessionID == "" {
		return errors.New("operation proposal omits its source session")
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO session_turn_leases (session_id, holder_id, fencing_token, lease_generation, expires_at)
		VALUES (?, 'semantic-replay', 1, 1, '9999-12-31T23:59:59.000000000Z')
		ON CONFLICT(session_id) DO NOTHING
	`, envelope.SessionID)
	return err
}

func strictSemanticProposal(raw string, destination any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("operation proposal contains trailing JSON")
	}
	return nil
}

func replayLease(sessionID memory.SessionID) memory.TurnLease {
	return memory.TurnLease{SessionID: sessionID, HolderID: "semantic-replay", FencingToken: 1, Generation: 1,
		ExpiresAt: time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)}
}

func (s *Store) replaySemanticOperation(ctx context.Context, operation semanticAcceptedReplayOperation) error {
	handler, err := semanticReplayHandler(operation)
	if err != nil {
		return err
	}
	result, err := handler.Replay(ctx, s, operation)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	equal, err := semanticJSONEqual(string(encoded), operation.ResultJSON)
	if err != nil || !equal {
		return errors.New("replayed operation result differs from its accepted result")
	}
	var replayed semanticAcceptedReplayOperation
	var transactionTime string
	if err := s.db.QueryRowContext(ctx, `
		SELECT operation_id, schema_version, operation_kind, idempotency_key, actor, session_id,
		       target_scope_id, source_event_id, proposal_sha256, effect_sha256, proposal_json,
		       prepared_proposal_json, result_json, transaction_time
		FROM semantic_operations WHERE operation_id = ?
	`, operation.OperationID).Scan(&replayed.OperationID, &replayed.SchemaVersion, &replayed.Kind, &replayed.IdempotencyKey,
		&replayed.Actor, &replayed.SessionID, &replayed.TargetScopeID, &replayed.SourceEventID, &replayed.ProposalHash,
		&replayed.EffectHash, &replayed.ProposalJSON, &replayed.PreparedJSON, &replayed.ResultJSON, &transactionTime); err != nil {
		return err
	}
	if replayed.OperationID != operation.OperationID || replayed.SchemaVersion != operation.SchemaVersion || replayed.Kind != operation.Kind ||
		replayed.IdempotencyKey != operation.IdempotencyKey || replayed.Actor != operation.Actor || replayed.SessionID != operation.SessionID ||
		replayed.TargetScopeID != operation.TargetScopeID || replayed.SourceEventID != operation.SourceEventID ||
		replayed.ProposalHash != operation.ProposalHash || replayed.EffectHash != operation.EffectHash ||
		replayed.ProposalJSON != operation.ProposalJSON || replayed.PreparedJSON != operation.PreparedJSON || replayed.ResultJSON != operation.ResultJSON ||
		transactionTime != formatSemanticTime(operation.TransactionTime) {
		return errors.New("replayed operation envelope differs from accepted history")
	}
	return nil
}

func isSemanticLifecycleKind(kind string) bool {
	return kind == "retire_memory" || kind == "restore_memory" || kind == "retract_source" || kind == "restore_source"
}

func semanticJSONEqual(left, right string) (bool, error) {
	var leftValue, rightValue any
	if err := json.Unmarshal([]byte(left), &leftValue); err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(right), &rightValue); err != nil {
		return false, err
	}
	leftJSON, _ := json.Marshal(leftValue)
	rightJSON, _ := json.Marshal(rightValue)
	return bytes.Equal(leftJSON, rightJSON), nil
}

func snapshotSemanticProjection(ctx context.Context, db *sql.DB) (map[string]semanticProjectionScopeSnapshot, error) {
	rows, err := db.QueryContext(ctx, `SELECT scope_id, scope_key, revision FROM semantic_scopes ORDER BY scope_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scopes []semanticProjectionScopeSnapshot
	for rows.Next() {
		var snapshot semanticProjectionScopeSnapshot
		if err := rows.Scan(&snapshot.ScopeID, &snapshot.ScopeKey, &snapshot.Revision); err != nil {
			return nil, err
		}
		scopes = append(scopes, snapshot)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make(map[string]semanticProjectionScopeSnapshot)
	for _, snapshot := range scopes {
		snapshot.Tables = make(map[string]semanticProjectionTableSnapshot)
		hash := sha256.New()
		for _, table := range semanticProjectionTableNames() {
			tableSnapshot, err := snapshotSemanticProjectionTable(ctx, db, table, snapshot.ScopeID)
			if err != nil {
				return nil, err
			}
			snapshot.Tables[table] = tableSnapshot
			fmt.Fprintf(hash, "%d:%s:%s:%d;", len(table), table, tableSnapshot.Hash, tableSnapshot.Rows)
		}
		snapshot.Hash = fmt.Sprintf("sha256:%x", hash.Sum(nil))
		snapshot.Frontier, err = snapshotSemanticOperationFrontier(ctx, db, snapshot.ScopeID)
		if err != nil {
			return nil, err
		}
		result[snapshot.ScopeKey] = snapshot
	}
	return result, nil
}

func snapshotSemanticProjectionTable(ctx context.Context, db *sql.DB, table string, scopeID memory.SemanticID) (semanticProjectionTableSnapshot, error) {
	columns, err := semanticTableColumns(ctx, db, table)
	if err != nil {
		return semanticProjectionTableSnapshot{}, err
	}
	qualified := make([]string, len(columns))
	order := make([]string, len(columns))
	for index, column := range columns {
		qualified[index] = `quote(t."` + strings.ReplaceAll(column, `"`, `""`) + `")`
		order[index] = `t."` + strings.ReplaceAll(column, `"`, `""`) + `"`
	}
	from := table + " AS t"
	where := "t.scope_id = ?"
	if table == "semantic_promotion_entities" {
		from += " JOIN semantic_promotions AS p ON p.operation_id = t.operation_id"
		where = "p.destination_scope_id = ?"
	} else if table == "semantic_promotions" {
		where = "t.destination_scope_id = ?"
	}
	query := `SELECT ` + strings.Join(qualified, ",") + ` FROM ` + from + ` WHERE ` + where + ` ORDER BY ` + strings.Join(order, ",")
	rows, err := db.QueryContext(ctx, query, scopeID)
	if err != nil {
		return semanticProjectionTableSnapshot{}, err
	}
	defer rows.Close()
	hash := sha256.New()
	snapshot := semanticProjectionTableSnapshot{}
	var count int64
	for rows.Next() {
		values := make([]string, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return semanticProjectionTableSnapshot{}, err
		}
		var canonicalRow strings.Builder
		for _, value := range values {
			fmt.Fprintf(&canonicalRow, "%d:%s;", len(value), value)
		}
		encodedRow := canonicalRow.String()
		_, _ = hash.Write([]byte(encodedRow))
		snapshot.CanonicalRows = append(snapshot.CanonicalRows, encodedRow)
		count++
	}
	if err := rows.Err(); err != nil {
		return semanticProjectionTableSnapshot{}, err
	}
	snapshot.Hash, snapshot.Rows = fmt.Sprintf("sha256:%x", hash.Sum(nil)), count
	return snapshot, nil
}

func semanticTableColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("semantic projection table %s is missing", table)
	}
	return columns, rows.Err()
}

func snapshotSemanticOperationFrontier(ctx context.Context, db *sql.DB, scopeID memory.SemanticID) (string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT operation_scopes.prior_revision, operation_scopes.resulting_revision, operation_scopes.written,
		       operations.operation_id, operations.schema_version, operations.operation_kind,
		       operations.effect_sha256, operations.transaction_time
		FROM semantic_operation_scopes AS operation_scopes
		JOIN semantic_operations AS operations ON operations.operation_id = operation_scopes.operation_id
		WHERE operation_scopes.scope_id = ?
		ORDER BY operation_scopes.resulting_revision, operations.transaction_time, operations.operation_id
	`, scopeID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	hash := sha256.New()
	for rows.Next() {
		var prior, result, version int64
		var written bool
		var operationID, kind, effect, transactionTime string
		if err := rows.Scan(&prior, &result, &written, &operationID, &version, &kind, &effect, &transactionTime); err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%d:%d:%t:%s:%d:%s:%s:%s;", prior, result, written, operationID, version, kind, effect, transactionTime)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

func compareSemanticProjectionSnapshots(live, shadow map[string]semanticProjectionScopeSnapshot) memory.SemanticProjectionVerification {
	keys := make(map[string]struct{}, len(live)+len(shadow))
	for key := range live {
		keys[key] = struct{}{}
	}
	for key := range shadow {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	report := memory.SemanticProjectionVerification{Valid: true}
	for _, key := range ordered {
		left, leftOK := live[key]
		right, rightOK := shadow[key]
		item := memory.SemanticProjectionScopeVerification{ScopeKey: key, LiveHash: left.Hash, ShadowHash: right.Hash,
			LiveFrontier: left.Frontier, ShadowFrontier: right.Frontier, LiveRevision: left.Revision, ShadowRevision: right.Revision}
		for _, table := range semanticProjectionTableNames() {
			leftTable, rightTable := left.Tables[table], right.Tables[table]
			if !leftOK || !rightOK || leftTable.Hash != rightTable.Hash || leftTable.Rows != rightTable.Rows {
				item.Mismatches = append(item.Mismatches, memory.SemanticProjectionMismatch{Table: table, LiveHash: leftTable.Hash,
					ShadowHash: rightTable.Hash, LiveRows: leftTable.Rows, ShadowRows: rightTable.Rows,
					LiveCanonicalRows: append([]string(nil), leftTable.CanonicalRows...), ShadowCanonicalRows: append([]string(nil), rightTable.CanonicalRows...)})
			}
		}
		if !leftOK || !rightOK || item.LiveHash != item.ShadowHash || item.LiveFrontier != item.ShadowFrontier || item.LiveRevision != item.ShadowRevision || len(item.Mismatches) != 0 {
			report.Valid = false
		}
		report.Scopes = append(report.Scopes, item)
	}
	return report
}

func (s *Store) quarantineProjectionReport(ctx context.Context, report memory.SemanticProjectionVerification, reason string, operationID memory.SemanticID) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		for _, scope := range report.Scopes {
			if len(scope.Mismatches) == 0 && scope.LiveHash == scope.ShadowHash && scope.LiveFrontier == scope.ShadowFrontier && scope.LiveRevision == scope.ShadowRevision {
				continue
			}
			if _, err := conn.ExecContext(ctx, `
				INSERT INTO semantic_projection_quarantine (scope_id, reason, operation_id, verified_at)
				SELECT scope_id, ?, NULLIF(?, ''), ? FROM semantic_scopes WHERE scope_key = ?
				ON CONFLICT(scope_id) DO UPDATE SET reason = excluded.reason, operation_id = excluded.operation_id, verified_at = excluded.verified_at
			`, reason, operationID, formatSemanticTime(s.now()), scope.ScopeKey); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) quarantineReplayOperation(ctx context.Context, replayErr *SemanticReplayError) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT scopes.scope_key FROM semantic_operation_scopes AS operation_scopes
		JOIN semantic_scopes AS scopes ON scopes.scope_id = operation_scopes.scope_id
		WHERE operation_scopes.operation_id = ?
		UNION
		SELECT scopes.scope_key FROM semantic_operations AS operations
		JOIN semantic_scopes AS scopes ON scopes.scope_id = operations.target_scope_id
		WHERE operations.operation_id = ?
		ORDER BY scope_key
	`, replayErr.OperationID, replayErr.OperationID)
	if err != nil {
		return err
	}
	var report memory.SemanticProjectionVerification
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return err
		}
		report.Scopes = append(report.Scopes, memory.SemanticProjectionScopeVerification{ScopeKey: key, LiveHash: "failed", ShadowHash: "replay"})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return s.quarantineProjectionReport(ctx, report, replayErr.Error(), replayErr.OperationID)
}

func (s *Store) acquireSemanticMaintenance(ctx context.Context, holderID string, duration time.Duration) (int64, error) {
	if strings.TrimSpace(holderID) == "" || duration <= 0 {
		return 0, errors.New("semantic maintenance holder and duration are required")
	}
	var token int64
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		expires := now.Add(duration)
		err := conn.QueryRowContext(ctx, `
			UPDATE semantic_maintenance_lock
			SET holder_id = ?, fencing_token = fencing_token + 1, expires_at = ?
			WHERE singleton = 1 AND (holder_id IS NULL OR expires_at <= ?)
			RETURNING fencing_token
		`, holderID, formatSemanticTime(expires), formatSemanticTime(now)).Scan(&token)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSemanticMaintenanceHeld
		}
		return err
	})
	return token, err
}

func (s *Store) releaseSemanticMaintenance(ctx context.Context, holderID string, token int64) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		result, err := conn.ExecContext(ctx, `UPDATE semantic_maintenance_lock SET holder_id = NULL, expires_at = NULL WHERE singleton = 1 AND holder_id = ? AND fencing_token = ?`, holderID, token)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return errors.New("semantic maintenance fence was lost")
		}
		return nil
	})
}

func (s *Store) swapSemanticProjection(ctx context.Context, shadowPath, holderID string, token int64) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS semantic_shadow`, "file:"+shadowPath+"?mode=ro"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `DETACH DATABASE semantic_shadow`)
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	var held int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_maintenance_lock WHERE singleton = 1 AND holder_id = ? AND fencing_token = ? AND expires_at > ?`, holderID, token, formatSemanticTime(s.now())).Scan(&held); err != nil || held != 1 {
		if err != nil {
			return err
		}
		return errors.New("semantic maintenance fence was lost before swap")
	}
	for _, trigger := range semanticProjectionTriggerNames() {
		if _, err := conn.ExecContext(ctx, `DROP TRIGGER IF EXISTS `+trigger); err != nil {
			return err
		}
	}
	for _, table := range semanticProjectionDeleteOrder() {
		if _, err := conn.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return err
		}
	}
	for _, table := range semanticProjectionInsertOrder() {
		if _, err := conn.ExecContext(ctx, `INSERT INTO `+table+` SELECT * FROM semantic_shadow.`+table); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, `
		UPDATE semantic_scopes SET revision = (SELECT shadow.revision FROM semantic_shadow.semantic_scopes AS shadow WHERE shadow.scope_id = semantic_scopes.scope_id)
		WHERE scope_id IN (SELECT scope_id FROM semantic_shadow.semantic_scopes)
	`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, semanticProjectionAppendOnlyTriggerSQL()); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM semantic_projection_quarantine`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `UPDATE semantic_maintenance_lock SET holder_id = NULL, expires_at = NULL WHERE singleton = 1 AND holder_id = ? AND fencing_token = ?`, holderID, token); err != nil {
		return err
	}
	foreignRows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	if foreignRows.Next() {
		_ = foreignRows.Close()
		return errors.New("semantic rebuilt projection fails foreign-key verification")
	}
	if err := foreignRows.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := conn.ExecContext(context.WithoutCancel(ctx), `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

func semanticProjectionTriggerNames() []string {
	var names []string
	for _, descriptor := range semanticProjectionTableDescriptors {
		names = append(names, descriptor.name+"_append_only_update", descriptor.name+"_append_only_delete")
	}
	return names
}

func semanticProjectionAppendOnlyTriggerSQL() string {
	var statements strings.Builder
	for _, descriptor := range semanticProjectionTableDescriptors {
		message := strings.ReplaceAll(descriptor.name, "_", " ") + " are append-only"
		fmt.Fprintf(&statements, "CREATE TRIGGER IF NOT EXISTS %s_append_only_update BEFORE UPDATE ON %s BEGIN SELECT RAISE(ABORT, '%s'); END;\n", descriptor.name, descriptor.name, message)
		fmt.Fprintf(&statements, "CREATE TRIGGER IF NOT EXISTS %s_append_only_delete BEFORE DELETE ON %s BEGIN SELECT RAISE(ABORT, '%s'); END;\n", descriptor.name, descriptor.name, message)
	}
	return statements.String()
}

func loadSemanticForeignKeyFailures(ctx context.Context, db *sql.DB) ([]semanticForeignKeyFailure, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, err
	}
	var failures []semanticForeignKeyFailure
	for rows.Next() {
		var failure semanticForeignKeyFailure
		if err := rows.Scan(&failure.Table, &failure.RowID, &failure.Parent, &failure.ForeignID); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(failure.Table, "semantic_") {
			continue
		}
		failures = append(failures, failure)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range failures {
		failures[index].ScopeKeys, err = semanticForeignKeyFailureScopes(ctx, db, failures[index])
		if err != nil {
			return nil, err
		}
	}
	return failures, nil
}

func semanticForeignKeyFailureScopes(ctx context.Context, db *sql.DB, failure semanticForeignKeyFailure) ([]string, error) {
	if !failure.RowID.Valid {
		return nil, nil
	}
	keys := make(map[string]struct{})
	targetOperations := make(map[string]struct{})
	allScopeOperations := make(map[string]struct{})
	collect := func(query string, destinations ...any) error {
		return db.QueryRowContext(ctx, query, failure.RowID.Int64).Scan(destinations...)
	}
	var scopeKey, operationID sql.NullString
	switch failure.Table {
	case "semantic_predicates", "semantic_entities", "semantic_aliases", "semantic_claims", "semantic_source_links", "semantic_graph_links":
		query := `SELECT scopes.scope_key, projection.created_operation_id FROM ` + failure.Table + ` AS projection
			LEFT JOIN semantic_scopes AS scopes ON scopes.scope_id = projection.scope_id WHERE projection.rowid = ?`
		if err := collect(query, &scopeKey, &operationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	case "semantic_claim_corrections":
		if err := collect(`SELECT scopes.scope_key, projection.operation_id FROM semantic_claim_corrections AS projection
			LEFT JOIN semantic_scopes AS scopes ON scopes.scope_id = projection.scope_id WHERE projection.rowid = ?`, &scopeKey, &operationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	case "semantic_state_events":
		if err := collect(`SELECT scopes.scope_key, projection.operation_id FROM semantic_state_events AS projection
			LEFT JOIN semantic_scopes AS scopes ON scopes.scope_id = projection.scope_id WHERE projection.rowid = ?`, &scopeKey, &operationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	case "semantic_promotions":
		var sourceKey, destinationKey sql.NullString
		if err := collect(`SELECT source.scope_key, destination.scope_key, projection.operation_id FROM semantic_promotions AS projection
			LEFT JOIN semantic_scopes AS source ON source.scope_id = projection.source_scope_id
			LEFT JOIN semantic_scopes AS destination ON destination.scope_id = projection.destination_scope_id
			WHERE projection.rowid = ?`, &sourceKey, &destinationKey, &operationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if sourceKey.Valid {
			keys[sourceKey.String] = struct{}{}
		}
		if destinationKey.Valid {
			keys[destinationKey.String] = struct{}{}
		}
	case "semantic_promotion_entities":
		if err := collect(`SELECT operation_id FROM semantic_promotion_entities WHERE rowid = ?`, &operationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	case "semantic_operations":
		if err := collect(`SELECT operation_id FROM semantic_operations WHERE rowid = ?`, &operationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	case "semantic_operation_scopes":
		if err := collect(`SELECT scopes.scope_key, frontier.operation_id FROM semantic_operation_scopes AS frontier
			LEFT JOIN semantic_scopes AS scopes ON scopes.scope_id = frontier.scope_id WHERE frontier.rowid = ?`, &scopeKey, &operationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	case "semantic_projection_quarantine":
		if err := collect(`SELECT scopes.scope_key FROM semantic_projection_quarantine AS quarantine
			LEFT JOIN semantic_scopes AS scopes ON scopes.scope_id = quarantine.scope_id WHERE quarantine.rowid = ?`, &scopeKey); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if scopeKey.Valid {
		keys[scopeKey.String] = struct{}{}
	}
	if operationID.Valid {
		targetOperations[operationID.String] = struct{}{}
	}
	if failure.Table == "semantic_operations" || failure.Table == "semantic_operation_scopes" {
		allScopeOperations[operationID.String] = struct{}{}
		delete(targetOperations, operationID.String)
	}
	for id := range targetOperations {
		var key string
		err := db.QueryRowContext(ctx, `
			SELECT scopes.scope_key FROM semantic_operations AS operations
			JOIN semantic_scopes AS scopes ON scopes.scope_id = operations.target_scope_id
			WHERE operations.operation_id = ?
		`, id).Scan(&key)
		if err == nil {
			keys[key] = struct{}{}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		} else {
			allScopeOperations[id] = struct{}{}
		}
	}
	for id := range allScopeOperations {
		rows, err := db.QueryContext(ctx, `
			SELECT scopes.scope_key FROM semantic_operation_scopes AS frontier
			JOIN semantic_scopes AS scopes ON scopes.scope_id = frontier.scope_id
			WHERE frontier.operation_id = ? ORDER BY scopes.scope_key
		`, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				_ = rows.Close()
				return nil, err
			}
			keys[key] = struct{}{}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result, nil
}

func addSemanticForeignKeyFailures(report *memory.SemanticProjectionVerification, failures []semanticForeignKeyFailure) {
	byScope := make(map[string]int, len(report.Scopes))
	for index := range report.Scopes {
		byScope[report.Scopes[index].ScopeKey] = index
	}
	for failureIndex, failure := range failures {
		keys := failure.ScopeKeys
		if len(keys) == 0 {
			keys = []string{fmt.Sprintf("unattributed:foreign-key:%d", failureIndex)}
		}
		for _, key := range keys {
			index, ok := byScope[key]
			if !ok {
				report.Scopes = append(report.Scopes, memory.SemanticProjectionScopeVerification{ScopeKey: key})
				index = len(report.Scopes) - 1
				byScope[key] = index
			}
			report.Scopes[index].Mismatches = append(report.Scopes[index].Mismatches, memory.SemanticProjectionMismatch{
				Table: "foreign_key:" + failure.Table, LiveRows: 1, LiveCanonicalRows: []string{failure.canonical()},
			})
			report.Valid = false
		}
	}
	sort.Slice(report.Scopes, func(left, right int) bool { return report.Scopes[left].ScopeKey < report.Scopes[right].ScopeKey })
}

// checkSemanticProjectionStartup deliberately avoids canonical row hashing or
// replay. It checks only cheap relational invariants needed to decide whether a
// scope may be served safely at process open.
func checkSemanticProjectionStartup(ctx context.Context, db *sql.DB) error {
	quarantine := make(map[string]string)
	rows, err := db.QueryContext(ctx, `
		SELECT scope_id, scope_key, revision FROM semantic_scopes ORDER BY scope_key
	`)
	if err != nil {
		return err
	}
	type scopeState struct {
		id       string
		key      string
		revision int64
	}
	var scopes []scopeState
	for rows.Next() {
		var scope scopeState
		if err := rows.Scan(&scope.id, &scope.key, &scope.revision); err != nil {
			_ = rows.Close()
			return err
		}
		scopes = append(scopes, scope)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, scope := range scopes {
		var count, distinct, invalidStep int64
		var minimum, maximum sql.NullInt64
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*), COUNT(DISTINCT resulting_revision), MIN(resulting_revision), MAX(resulting_revision),
			       COALESCE(SUM(CASE WHEN resulting_revision != prior_revision + 1 THEN 1 ELSE 0 END), 0)
			FROM semantic_operation_scopes WHERE scope_id = ? AND written = 1
		`, scope.id).Scan(&count, &distinct, &minimum, &maximum, &invalidStep); err != nil {
			return err
		}
		frontierValid := count == scope.revision && distinct == count && invalidStep == 0
		if scope.revision == 0 {
			frontierValid = frontierValid && !minimum.Valid && !maximum.Valid
		} else {
			frontierValid = frontierValid && minimum.Valid && minimum.Int64 == 1 && maximum.Valid && maximum.Int64 == scope.revision
		}
		var invalidStates int64
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM semantic_state_events AS states
			LEFT JOIN semantic_operation_scopes AS frontier
			  ON frontier.operation_id = states.operation_id AND frontier.scope_id = states.scope_id AND frontier.written = 1
			WHERE states.scope_id = ? AND (states.scope_revision > ? OR frontier.resulting_revision IS NULL OR frontier.resulting_revision != states.scope_revision)
		`, scope.id, scope.revision).Scan(&invalidStates); err != nil {
			return err
		}
		if !frontierValid || invalidStates != 0 {
			quarantine[scope.key] = "startup revision or operation-frontier check failed"
		}
	}

	unknownRows, err := db.QueryContext(ctx, `
		SELECT operations.operation_id, typeof(operations.schema_version), CAST(operations.schema_version AS TEXT), operations.operation_kind, scopes.scope_key
		FROM semantic_operations AS operations
		JOIN semantic_operation_scopes AS operation_scopes ON operation_scopes.operation_id = operations.operation_id
		JOIN semantic_scopes AS scopes ON scopes.scope_id = operation_scopes.scope_id
		WHERE NOT (
			(operations.schema_version = 1 AND operations.operation_kind IN ('remember_literal_claim', 'remember_entity_claim')) OR
			(operations.schema_version = 2 AND operations.operation_kind = 'correct_claim') OR
			(operations.schema_version = 3 AND operations.operation_kind IN ('retire_memory', 'restore_memory', 'retract_source', 'restore_source')) OR
			(operations.schema_version = 4 AND operations.operation_kind = 'promote_claim') OR
			(operations.schema_version = 5 AND operations.operation_kind IN ('create_graph_link', 'retire_memory', 'restore_memory', 'retract_source', 'restore_source'))
		)
	`)
	if err != nil {
		return err
	}
	for unknownRows.Next() {
		var operationID, versionType, versionText, kind, key string
		if err := unknownRows.Scan(&operationID, &versionType, &versionText, &kind, &key); err != nil {
			_ = unknownRows.Close()
			return err
		}
		quarantine[key] = fmt.Sprintf("startup operation compatibility check failed at %s (schema version %s/%s, kind %s)", operationID, versionType, versionText, kind)
	}
	if err := unknownRows.Close(); err != nil {
		return err
	}

	foreignFailures, err := loadSemanticForeignKeyFailures(ctx, db)
	if err != nil {
		return err
	}
	for _, failure := range foreignFailures {
		for _, key := range failure.ScopeKeys {
			quarantine[key] = "startup semantic foreign-key check failed: " + failure.canonical()
		}
	}
	if len(quarantine) == 0 {
		return nil
	}
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		for _, scope := range scopes {
			reason, affected := quarantine[scope.key]
			if !affected {
				continue
			}
			if _, err := conn.ExecContext(ctx, `
				INSERT INTO semantic_projection_quarantine (scope_id, reason, operation_id, verified_at)
				VALUES (?, ?, NULL, ?)
				ON CONFLICT(scope_id) DO UPDATE SET reason = excluded.reason, operation_id = NULL, verified_at = excluded.verified_at
			`, scope.id, reason, formatSemanticTime(time.Now().UTC())); err != nil {
				return err
			}
		}
		return nil
	})
}
