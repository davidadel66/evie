package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

const (
	coordinationClaimHeld         = "claim_held"
	coordinationClaimRequired     = "claim_required"
	coordinationClaimNotOwned     = "claim_not_owned"
	coordinationExecutionInactive = "execution_inactive"
)

type storedTaskClaim struct {
	task.Claim
	ActorID         string
	SessionID       string
	LeaseHolderID   string
	LeaseToken      uint64
	LeaseGeneration uint64
}

type coordinationResult struct {
	RequestSHA256 string
	Operation     task.Operation
	TaskID        task.ID
	EventID       string
	OutcomeCode   string
	Claim         task.Claim
	ReleasedAt    time.Time
	ReleaseReason string
	FromStatus    task.Status
}

func taskUpdateNeedsClaim(current task.Task, input task.UpdateInput) bool {
	if input.ResultSummary != nil {
		return true
	}
	if input.Status == nil {
		return false
	}
	switch *input.Status {
	case task.StatusInProgress, task.StatusBlocked, task.StatusCompleted:
		return true
	case task.StatusOpen:
		return current.Status == task.StatusInProgress || current.Status == task.StatusBlocked
	default:
		return false
	}
}

func authorizeTaskUpdate(ctx context.Context, conn *sql.Conn, current task.Task, input task.UpdateInput,
	attribution task.MutationAttribution, now time.Time, managementOverride bool,
) (storedTaskClaim, bool, error) {
	if managementOverride {
		claim, found, err := getStoredTaskClaim(ctx, conn, current.ID)
		return claim, found, err
	}
	claim, found, err := getStoredTaskClaim(ctx, conn, current.ID)
	if err != nil {
		return storedTaskClaim{}, false, err
	}
	needsClaim := taskUpdateNeedsClaim(current, input)
	cancelsClaimedTask := input.Status != nil && *input.Status == task.StatusCancelled && found
	if !needsClaim && !cancelsClaimedTask {
		return claim, found, nil
	}
	if err := task.ValidateClaimAttribution(attribution); err != nil {
		return claim, found, err
	}
	active, err := claimExecutionActive(ctx, conn, attribution, now)
	if err != nil {
		return claim, found, err
	}
	if !active {
		return claim, found, &task.ClaimExecutionInactiveError{TaskID: current.ID}
	}
	if needsClaim {
		if !found {
			return claim, false, &task.ClaimRequiredError{TaskID: current.ID}
		}
		if !claimOwnedBy(claim, attribution) {
			return claim, true, &task.ClaimNotOwnedError{TaskID: current.ID}
		}
	}
	if input.Status != nil && *input.Status == task.StatusCancelled && found && !claimOwnedBy(claim, attribution) {
		return claim, true, &task.ClaimNotOwnedError{TaskID: current.ID}
	}
	return claim, found, nil
}

func (s *Store) ClaimGlobalTask(ctx context.Context, id task.ID, input task.ClaimInput) (task.Claim, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityClaim, task.AccessContribute)
	if err := validateClaimMutation(ctx, id, input.IdempotencyKey); err != nil {
		return task.Claim{}, err
	}
	attribution, _ := task.MutationAttributionFromContext(ctx)
	if err := task.ValidateClaimAttribution(attribution); err != nil {
		return task.Claim{}, err
	}
	if _, err := taskAccessFromContext(ctx, s.db); err != nil {
		return task.Claim{}, err
	}
	requestSHA256, err := task.CanonicalCoordinationRequestSHA256(id, task.OperationClaim)
	if err != nil {
		return task.Claim{}, err
	}
	identitySHA256 := task.IdempotencySHA256(input.IdempotencyKey)
	var result task.Claim
	var businessErr error
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		transactionAccess, err := taskAccessFromContext(ctx, conn)
		if err != nil {
			return err
		}
		if transactionAccess.delegated {
			if _, err := getGlobalTask(ctx, conn, id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return &task.NotFoundError{ID: id}
				}
				return err
			}
		}
		priorClaim, found, err := readIdempotencyClaim(ctx, conn, attribution, identitySHA256)
		if err != nil {
			return err
		}
		if found {
			if priorClaim.Operation != task.OperationClaim || priorClaim.RequestSHA256 != requestSHA256 {
				if err := insertCoordinationConflict(ctx, conn, attribution, identitySHA256,
					priorClaim.RequestSHA256, requestSHA256, task.OperationClaim, id, s.now().UTC()); err != nil {
					return err
				}
				businessErr = &task.IdempotencyConflictError{Operation: task.OperationClaim}
				return nil
			}
			prior, err := readCoordinationResult(ctx, conn, attribution, identitySHA256)
			if err != nil {
				return err
			}
			result = prior.Claim
			businessErr = replayCoordinationError(id, prior)
			return nil
		}

		current, err := getGlobalTask(ctx, conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			businessErr = &task.NotFoundError{ID: id}
			return insertCoordinationResult(ctx, conn, attribution, identitySHA256, requestSHA256,
				coordinationResult{Operation: task.OperationClaim, TaskID: id, OutcomeCode: mutationNotFound}, s.now().UTC())
		}
		if err != nil {
			return fmt.Errorf("get global Task for claim: %w", err)
		}
		now := s.now().UTC()
		reject := func(cause error, diagnostic task.DiagnosticCode, outcome string, claimID string) error {
			businessErr = cause
			eventID, err := appendClaimEvent(ctx, conn, task.Event{
				TaskID: id, Operation: task.OperationClaim, ActorID: attribution.ActorID,
				SessionID: attribution.SessionID, RunID: attribution.RunID, RecordedAt: now,
				PreviousRevision: current.Revision, ResultingRevision: current.Revision,
				Outcome: task.MutationRejected, DiagnosticCode: diagnostic, ClaimID: claimID,
				IdempotencySHA256: identitySHA256,
			})
			if err != nil {
				return err
			}
			return insertCoordinationResult(ctx, conn, attribution, identitySHA256, requestSHA256,
				coordinationResult{Operation: task.OperationClaim, TaskID: id, EventID: eventID,
					OutcomeCode: outcome, Claim: task.Claim{ID: claimID, TaskID: id}, FromStatus: current.Status}, now)
		}
		if current.Status == task.StatusCompleted || current.Status == task.StatusCancelled {
			return reject(&task.TransitionError{From: current.Status, To: task.StatusInProgress},
				task.DiagnosticInvalidTransition, mutationInvalidTransition, "")
		}
		executionActive, err := claimExecutionActive(ctx, conn, attribution, now)
		if err != nil {
			return err
		}
		if !executionActive {
			return reject(&task.ClaimExecutionInactiveError{TaskID: id}, task.DiagnosticExecutionInactive,
				coordinationExecutionInactive, "")
		}
		active, found, err := getStoredTaskClaim(ctx, conn, id)
		if err != nil {
			return err
		}
		if found && !claimOwnedBy(active, attribution) {
			return reject(&task.ClaimHeldError{TaskID: id}, task.DiagnosticClaimHeld,
				coordinationClaimHeld, active.ID)
		}
		reason := "confirmed"
		if !found {
			claimID, err := uuid.NewRandom()
			if err != nil {
				return fmt.Errorf("generate Task claim ID: %w", err)
			}
			active = storedTaskClaim{
				Claim:   task.Claim{ID: claimID.String(), TaskID: id, ClaimedAt: now},
				ActorID: attribution.ActorID, SessionID: attribution.SessionID,
				LeaseHolderID: attribution.LeaseHolderID, LeaseToken: attribution.LeaseToken,
				LeaseGeneration: attribution.LeaseGeneration,
			}
			if err := insertStoredTaskClaim(ctx, conn, active, attribution.RunID); err != nil {
				return err
			}
			reason = "acquired"
		}
		eventID, err := appendClaimEvent(ctx, conn, task.Event{
			TaskID: id, Operation: task.OperationClaim, ActorID: attribution.ActorID,
			SessionID: attribution.SessionID, RunID: attribution.RunID, RecordedAt: now,
			PreviousRevision: current.Revision, ResultingRevision: current.Revision,
			Outcome: task.MutationAccepted, ClaimID: active.ID, ClaimReason: reason,
			IdempotencySHA256: identitySHA256,
		})
		if err != nil {
			return err
		}
		result = active.Claim
		return insertCoordinationResult(ctx, conn, attribution, identitySHA256, requestSHA256,
			coordinationResult{Operation: task.OperationClaim, TaskID: id, EventID: eventID,
				OutcomeCode: mutationAccepted, Claim: result}, now)
	})
	if err != nil {
		return task.Claim{}, err
	}
	if businessErr != nil {
		return task.Claim{}, businessErr
	}
	return result, nil
}

func (s *Store) ReleaseGlobalTaskClaim(ctx context.Context, id task.ID, input task.ReleaseInput) (task.ClaimRelease, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityRelease, task.AccessContribute)
	if err := validateClaimMutation(ctx, id, input.IdempotencyKey); err != nil {
		return task.ClaimRelease{}, err
	}
	attribution, _ := task.MutationAttributionFromContext(ctx)
	if err := task.ValidateClaimAttribution(attribution); err != nil {
		return task.ClaimRelease{}, err
	}
	if _, err := taskAccessFromContext(ctx, s.db); err != nil {
		return task.ClaimRelease{}, err
	}
	requestSHA256, err := task.CanonicalCoordinationRequestSHA256(id, task.OperationRelease)
	if err != nil {
		return task.ClaimRelease{}, err
	}
	identitySHA256 := task.IdempotencySHA256(input.IdempotencyKey)
	var result task.ClaimRelease
	var businessErr error
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		transactionAccess, err := taskAccessFromContext(ctx, conn)
		if err != nil {
			return err
		}
		if transactionAccess.delegated {
			if _, err := getGlobalTask(ctx, conn, id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return &task.NotFoundError{ID: id}
				}
				return err
			}
		}
		priorClaim, found, err := readIdempotencyClaim(ctx, conn, attribution, identitySHA256)
		if err != nil {
			return err
		}
		if found {
			if priorClaim.Operation != task.OperationRelease || priorClaim.RequestSHA256 != requestSHA256 {
				if err := insertCoordinationConflict(ctx, conn, attribution, identitySHA256,
					priorClaim.RequestSHA256, requestSHA256, task.OperationRelease, id, s.now().UTC()); err != nil {
					return err
				}
				businessErr = &task.IdempotencyConflictError{Operation: task.OperationRelease}
				return nil
			}
			prior, err := readCoordinationResult(ctx, conn, attribution, identitySHA256)
			if err != nil {
				return err
			}
			result = task.ClaimRelease{Claim: prior.Claim, ReleasedAt: prior.ReleasedAt, Reason: prior.ReleaseReason}
			businessErr = replayCoordinationError(id, prior)
			return nil
		}
		current, err := getGlobalTask(ctx, conn, id)
		if errors.Is(err, sql.ErrNoRows) {
			businessErr = &task.NotFoundError{ID: id}
			return insertCoordinationResult(ctx, conn, attribution, identitySHA256, requestSHA256,
				coordinationResult{Operation: task.OperationRelease, TaskID: id, OutcomeCode: mutationNotFound}, s.now().UTC())
		}
		if err != nil {
			return fmt.Errorf("get global Task for release: %w", err)
		}
		now := s.now().UTC()
		reject := func(cause error, diagnostic task.DiagnosticCode, outcome string, claimID string) error {
			businessErr = cause
			eventID, err := appendClaimEvent(ctx, conn, task.Event{
				TaskID: id, Operation: task.OperationRelease, ActorID: attribution.ActorID,
				SessionID: attribution.SessionID, RunID: attribution.RunID, RecordedAt: now,
				PreviousRevision: current.Revision, ResultingRevision: current.Revision,
				Outcome: task.MutationRejected, DiagnosticCode: diagnostic, ClaimID: claimID,
				IdempotencySHA256: identitySHA256,
			})
			if err != nil {
				return err
			}
			return insertCoordinationResult(ctx, conn, attribution, identitySHA256, requestSHA256,
				coordinationResult{Operation: task.OperationRelease, TaskID: id, EventID: eventID,
					OutcomeCode: outcome, Claim: task.Claim{ID: claimID, TaskID: id}}, now)
		}
		executionActive, err := claimExecutionActive(ctx, conn, attribution, now)
		if err != nil {
			return err
		}
		if !executionActive {
			return reject(&task.ClaimExecutionInactiveError{TaskID: id}, task.DiagnosticExecutionInactive,
				coordinationExecutionInactive, "")
		}
		active, found, err := getStoredTaskClaim(ctx, conn, id)
		if err != nil {
			return err
		}
		if !found {
			return reject(&task.ClaimRequiredError{TaskID: id}, task.DiagnosticClaimRequired,
				coordinationClaimRequired, "")
		}
		if !claimOwnedBy(active, attribution) {
			return reject(&task.ClaimNotOwnedError{TaskID: id}, task.DiagnosticClaimNotOwned,
				coordinationClaimNotOwned, active.ID)
		}
		var eventID string
		result, eventID, err = releaseStoredTaskClaim(ctx, conn, active, task.Event{
			Operation: task.OperationRelease, ActorID: attribution.ActorID, SessionID: attribution.SessionID,
			RunID: attribution.RunID, RecordedAt: now, IdempotencySHA256: identitySHA256,
		}, "explicit", false)
		if err != nil {
			return err
		}
		return insertCoordinationResult(ctx, conn, attribution, identitySHA256, requestSHA256,
			coordinationResult{Operation: task.OperationRelease, TaskID: id,
				EventID: eventID, OutcomeCode: mutationAccepted, Claim: result.Claim,
				ReleasedAt: result.ReleasedAt, ReleaseReason: result.Reason}, now)
	})
	if err != nil {
		return task.ClaimRelease{}, err
	}
	if businessErr != nil {
		return task.ClaimRelease{}, businessErr
	}
	return result, nil
}

func validateClaimMutation(ctx context.Context, id task.ID, key task.IdempotencyKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(string(id)) == "" {
		return &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	return task.ValidateIdempotencyKey(key)
}

func claimExecutionActive(ctx context.Context, conn *sql.Conn, attribution task.MutationAttribution, now time.Time) (bool, error) {
	var active int
	err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sessions s
			JOIN session_turn_leases l ON l.session_id = s.id
			WHERE s.id = ? AND s.status = 'active' AND l.holder_id = ?
			  AND l.fencing_token = ? AND l.lease_generation = ? AND l.expires_at > ?
		)
	`, attribution.SessionID, attribution.LeaseHolderID, attribution.LeaseToken,
		attribution.LeaseGeneration, now.UTC().Format(turnLeaseTimeFormat)).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("check Task claim execution: %w", err)
	}
	return active == 1, nil
}

func claimOwnedBy(claim storedTaskClaim, attribution task.MutationAttribution) bool {
	return claim.ActorID == attribution.ActorID && claim.SessionID == attribution.SessionID &&
		claim.LeaseHolderID == attribution.LeaseHolderID && claim.LeaseToken == attribution.LeaseToken &&
		claim.LeaseGeneration == attribution.LeaseGeneration
}

func insertStoredTaskClaim(ctx context.Context, conn *sql.Conn, claim storedTaskClaim, runID string) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_claims (
			task_id, claim_id, actor_id, session_id, lease_holder_id, lease_token,
			lease_generation, acquired_run_id, claimed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, claim.TaskID, claim.ID, claim.ActorID, claim.SessionID, claim.LeaseHolderID,
		claim.LeaseToken, claim.LeaseGeneration, runID, claim.ClaimedAt.UTC().Format(turnLeaseTimeFormat))
	if err != nil {
		return fmt.Errorf("insert active Task claim: %w", err)
	}
	return nil
}

func getStoredTaskClaim(ctx context.Context, source queryRower, id task.ID) (storedTaskClaim, bool, error) {
	var claim storedTaskClaim
	var claimedAt string
	err := source.QueryRowContext(ctx, `
		SELECT claim_id, task_id, claimed_at, actor_id, session_id, lease_holder_id,
		       lease_token, lease_generation
		FROM task_claims WHERE task_id = ?
	`, id).Scan(&claim.ID, &claim.TaskID, &claimedAt, &claim.ActorID, &claim.SessionID,
		&claim.LeaseHolderID, &claim.LeaseToken, &claim.LeaseGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return storedTaskClaim{}, false, nil
	}
	if err != nil {
		return storedTaskClaim{}, false, fmt.Errorf("read active Task claim: %w", err)
	}
	claim.ClaimedAt, err = time.Parse(time.RFC3339Nano, claimedAt)
	if err != nil {
		return storedTaskClaim{}, false, fmt.Errorf("parse Task claim claimed_at: %w", err)
	}
	return claim, true, nil
}

func (s *Store) GetGlobalTaskClaim(ctx context.Context, id task.ID) (task.Claim, bool, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityGet, task.AccessRead)
	if strings.TrimSpace(string(id)) == "" {
		return task.Claim{}, false, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if _, err := s.GetGlobalTask(ctx, id); err != nil {
		return task.Claim{}, false, err
	}
	access, err := taskAccessFromContext(ctx, s.db)
	if err != nil {
		return task.Claim{}, false, err
	}
	activeGrantSQL, activeGrantArgs := access.activeGrantPredicate()
	arguments := []any{id}
	arguments = append(arguments, activeGrantArgs...)
	var claim storedTaskClaim
	var claimedAt string
	err = s.db.QueryRowContext(ctx, `
		SELECT claim_id, task_id, claimed_at, actor_id, session_id, lease_holder_id,
		       lease_token, lease_generation
		FROM task_claims WHERE task_id = ?`+activeGrantSQL+`
	`, arguments...).Scan(&claim.ID, &claim.TaskID, &claimedAt, &claim.ActorID, &claim.SessionID,
		&claim.LeaseHolderID, &claim.LeaseToken, &claim.LeaseGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Claim{}, false, nil
	}
	if err != nil {
		return task.Claim{}, false, fmt.Errorf("read active Task claim: %w", err)
	}
	claim.ClaimedAt, err = time.Parse(time.RFC3339Nano, claimedAt)
	if err != nil {
		return task.Claim{}, false, fmt.Errorf("parse Task claim claimed_at: %w", err)
	}
	return claim.Claim, true, nil
}

func appendClaimEvent(ctx context.Context, conn *sql.Conn, event task.Event) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate Task claim event ID: %w", err)
	}
	event.ID = id.String()
	if err := conn.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1 FROM (
			SELECT sequence FROM task_events WHERE task_id = ?
			UNION ALL SELECT sequence FROM task_hierarchy_events WHERE task_id = ?
			UNION ALL SELECT sequence FROM task_claim_events WHERE task_id = ?
		)
	`, event.TaskID, event.TaskID, event.TaskID).Scan(&event.Sequence); err != nil {
		return "", fmt.Errorf("allocate Task claim event sequence: %w", err)
	}
	_, err = conn.ExecContext(ctx, `
		INSERT INTO task_claim_events (
			id, task_id, sequence, operation, actor_id, session_id, run_id, recorded_at,
			previous_revision, resulting_revision, outcome, diagnostic_code, identity_sha256,
			claim_id, claim_reason, management_override, management_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''),
		          NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''))
	`, event.ID, event.TaskID, event.Sequence, event.Operation, event.ActorID, event.SessionID,
		event.RunID, formatTaskTime(event.RecordedAt), event.PreviousRevision, event.ResultingRevision,
		event.Outcome, event.DiagnosticCode, event.IdempotencySHA256, event.ClaimID,
		event.ClaimReason, event.ManagementOverride, event.ManagementReason)
	if err != nil {
		return "", fmt.Errorf("append Task claim event: %w", err)
	}
	if err := linkActiveTaskGrant(ctx, conn, event.ID, event.SessionID); err != nil {
		return "", err
	}
	return event.ID, nil
}

func readCoordinationResult(ctx context.Context, conn *sql.Conn, attribution task.MutationAttribution, identity string) (coordinationResult, error) {
	var result coordinationResult
	var claimID, claimedAt, releasedAt, reason, fromStatus sql.NullString
	err := conn.QueryRowContext(ctx, `
		SELECT request_sha256, operation, task_id, COALESCE(event_id, ''), outcome_code,
		       claim_id, claimed_at, released_at, release_reason, from_status
		FROM task_coordination_results
		WHERE actor_id = ? AND session_id = ? AND identity_sha256 = ?
	`, attribution.ActorID, attribution.SessionID, identity).Scan(
		&result.RequestSHA256, &result.Operation, &result.TaskID, &result.EventID, &result.OutcomeCode,
		&claimID, &claimedAt, &releasedAt, &reason, &fromStatus,
	)
	if err != nil {
		return coordinationResult{}, fmt.Errorf("read Task coordination result: %w", err)
	}
	result.Claim = task.Claim{ID: claimID.String, TaskID: result.TaskID}
	if claimedAt.Valid {
		result.Claim.ClaimedAt, err = time.Parse(time.RFC3339Nano, claimedAt.String)
		if err != nil {
			return coordinationResult{}, err
		}
	}
	if releasedAt.Valid {
		result.ReleasedAt, err = time.Parse(time.RFC3339Nano, releasedAt.String)
		if err != nil {
			return coordinationResult{}, err
		}
	}
	result.ReleaseReason = reason.String
	result.FromStatus = task.Status(fromStatus.String)
	return result, nil
}

func insertCoordinationResult(ctx context.Context, conn *sql.Conn, attribution task.MutationAttribution,
	identity, request string, result coordinationResult, recordedAt time.Time,
) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO task_coordination_results (
			actor_id, session_id, run_id, identity_sha256, request_sha256, operation,
			task_id, event_id, outcome_code, claim_id, claimed_at, released_at,
			release_reason, from_status, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''),
		          NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?)
	`, attribution.ActorID, attribution.SessionID, attribution.RunID, identity, request, result.Operation,
		result.TaskID, result.EventID, result.OutcomeCode, result.Claim.ID,
		formatOptionalTaskTime(result.Claim.ClaimedAt), formatOptionalTaskTime(result.ReleasedAt),
		result.ReleaseReason, result.FromStatus, formatTaskTime(recordedAt))
	if err != nil {
		return fmt.Errorf("insert Task coordination result: %w", err)
	}
	return nil
}

func formatOptionalTaskTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return formatTaskTime(value)
}

func insertCoordinationConflict(ctx context.Context, conn *sql.Conn, attribution task.MutationAttribution,
	identity, original, attempted string, operation task.Operation, id task.ID, recordedAt time.Time,
) error {
	conflictID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `
		INSERT INTO task_coordination_conflicts (
			id, actor_id, session_id, identity_sha256, original_request_sha256,
			attempted_request_sha256, operation, task_id, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)
	`, conflictID.String(), attribution.ActorID, attribution.SessionID, identity, original, attempted,
		operation, id, formatTaskTime(recordedAt))
	if err != nil {
		return fmt.Errorf("insert Task coordination idempotency conflict: %w", err)
	}
	return nil
}

func replayCoordinationError(id task.ID, result coordinationResult) error {
	switch result.OutcomeCode {
	case mutationAccepted:
		return nil
	case mutationNotFound:
		return &task.NotFoundError{ID: id}
	case mutationInvalidTransition:
		return &task.TransitionError{From: result.FromStatus, To: task.StatusInProgress}
	case coordinationClaimHeld:
		return &task.ClaimHeldError{TaskID: id}
	case coordinationClaimRequired:
		return &task.ClaimRequiredError{TaskID: id}
	case coordinationClaimNotOwned:
		return &task.ClaimNotOwnedError{TaskID: id}
	case coordinationExecutionInactive:
		return &task.ClaimExecutionInactiveError{TaskID: id}
	default:
		return fmt.Errorf("replay Task coordination outcome %q", result.OutcomeCode)
	}
}

func releaseStoredTaskClaim(ctx context.Context, conn *sql.Conn, claim storedTaskClaim,
	event task.Event, reason string, managementOverride bool,
) (task.ClaimRelease, string, error) {
	current, err := getGlobalTask(ctx, conn, claim.TaskID)
	if err != nil {
		return task.ClaimRelease{}, "", err
	}
	result, err := conn.ExecContext(ctx, `DELETE FROM task_claims WHERE task_id = ? AND claim_id = ?`, claim.TaskID, claim.ID)
	if err != nil {
		return task.ClaimRelease{}, "", fmt.Errorf("release active Task claim: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return task.ClaimRelease{}, "", fmt.Errorf("release active Task claim affected %d rows: %w", rows, err)
	}
	event.TaskID = claim.TaskID
	event.PreviousRevision = current.Revision
	event.ResultingRevision = current.Revision
	event.Outcome = task.MutationAccepted
	event.ClaimID = claim.ID
	event.ClaimReason = reason
	event.ManagementOverride = managementOverride
	eventID, err := appendClaimEvent(ctx, conn, event)
	if err != nil {
		return task.ClaimRelease{}, "", err
	}
	return task.ClaimRelease{Claim: claim.Claim, ReleasedAt: event.RecordedAt, Reason: reason}, eventID, nil
}

// OverrideReleaseGlobalTaskClaim is a Kernel-only recovery boundary. It is not
// exposed through task.Service or model-facing tool arguments.
func (s *Store) OverrideReleaseGlobalTaskClaim(ctx context.Context, id task.ID, reason string) (task.ClaimRelease, error) {
	ctx = withTaskAuthorization(ctx, task.CapabilityUpdate, task.AccessManage)
	if strings.TrimSpace(string(id)) == "" {
		return task.ClaimRelease{}, &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if strings.TrimSpace(reason) == "" {
		return task.ClaimRelease{}, &task.InputError{Field: "management_reason", Message: "must not be blank"}
	}
	attribution, err := task.MutationAttributionFromContext(ctx)
	if err != nil {
		return task.ClaimRelease{}, err
	}
	if attribution.ParentSessionID != "" {
		access, accessErr := taskAccessFromContext(ctx, s.db)
		if accessErr != nil {
			if errors.Is(accessErr, task.ErrAccessDenied) || errors.Is(accessErr, task.ErrScopeDenied) {
				return task.ClaimRelease{}, task.ErrManagementOverrideDenied
			}
			return task.ClaimRelease{}, accessErr
		}
		if !access.delegated || !grantAllows(access.grant.Level, task.AccessManage) {
			return task.ClaimRelease{}, task.ErrManagementOverrideDenied
		}
	}
	access, err := taskAccessFromContext(ctx, s.db)
	if err != nil {
		return task.ClaimRelease{}, err
	}
	if access.delegated && !grantAllows(access.grant.Level, task.AccessManage) {
		return task.ClaimRelease{}, task.ErrManagementOverrideDenied
	}
	var released task.ClaimRelease
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		transactionAccess, err := taskAccessFromContext(ctx, conn)
		if err != nil {
			return err
		}
		if transactionAccess.delegated {
			if _, err := getGlobalTask(ctx, conn, id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return &task.NotFoundError{ID: id}
				}
				return err
			}
		}
		claim, found, err := getStoredTaskClaim(ctx, conn, id)
		if err != nil {
			return err
		}
		if !found {
			return &task.ClaimRequiredError{TaskID: id}
		}
		released, _, err = releaseStoredTaskClaim(ctx, conn, claim, task.Event{
			Operation: task.OperationRelease, ActorID: attribution.ActorID,
			SessionID: attribution.SessionID, RunID: attribution.RunID, RecordedAt: s.now().UTC(),
			ManagementReason: reason,
		}, "management_override", true)
		return err
	})
	return released, err
}

// RecoverInactiveTaskClaims releases projections whose fenced execution lease
// is missing, expired, superseded, or attached to an inactive session.
func (s *Store) RecoverInactiveTaskClaims(ctx context.Context) (int, error) {
	now := s.now().UTC()
	count := 0
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		count, err = releaseTaskClaimsMatching(ctx, conn, `
			WHERE NOT EXISTS (
				SELECT 1 FROM sessions s
				JOIN session_turn_leases l ON l.session_id = s.id
				WHERE s.id = c.session_id AND s.status = 'active'
				  AND l.holder_id = c.lease_holder_id
				  AND l.fencing_token = c.lease_token
				  AND l.lease_generation = c.lease_generation
				  AND l.expires_at > ?
			)
		`, []any{now.UTC().Format(turnLeaseTimeFormat)}, now, "recovery")
		return err
	})
	return count, err
}

func releaseTaskClaimsForLease(ctx context.Context, conn *sql.Conn, sessionID memory.SessionID,
	holderID memory.LeaseHolderID, token memory.FencingToken, now time.Time, reason string,
) (int, error) {
	return releaseTaskClaimsMatching(ctx, conn, `
		WHERE c.session_id = ? AND c.lease_holder_id = ?
		  AND c.lease_token = ? AND c.lease_generation = ?
	`, []any{sessionID, holderID, token, token}, now, reason)
}

func releaseInactiveTaskClaimsForSession(ctx context.Context, conn *sql.Conn, sessionID memory.SessionID,
	now time.Time,
) (int, error) {
	return releaseTaskClaimsMatching(ctx, conn, `
		WHERE c.session_id = ? AND NOT EXISTS (
			SELECT 1 FROM sessions s
			JOIN session_turn_leases l ON l.session_id = s.id
			WHERE s.id = c.session_id AND s.status = 'active'
			  AND l.holder_id = c.lease_holder_id
			  AND l.fencing_token = c.lease_token
			  AND l.lease_generation = c.lease_generation
			  AND l.expires_at > ?
		)
	`, []any{sessionID, now.UTC().Format(turnLeaseTimeFormat)}, now, "recovery")
}

func releaseTaskClaimsMatching(ctx context.Context, conn *sql.Conn, where string, args []any,
	now time.Time, reason string,
) (int, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT c.claim_id, c.task_id, c.claimed_at, c.actor_id, c.session_id,
		       c.lease_holder_id, c.lease_token, c.lease_generation
		FROM task_claims c
	`+where+`
		ORDER BY c.task_id
	`, args...)
	if err != nil {
		return 0, fmt.Errorf("list Task claims for lease cleanup: %w", err)
	}
	var claims []storedTaskClaim
	for rows.Next() {
		var claim storedTaskClaim
		var claimedAt string
		if err := rows.Scan(&claim.ID, &claim.TaskID, &claimedAt, &claim.ActorID, &claim.SessionID,
			&claim.LeaseHolderID, &claim.LeaseToken, &claim.LeaseGeneration); err != nil {
			_ = rows.Close()
			return 0, err
		}
		claim.ClaimedAt, err = time.Parse(time.RFC3339Nano, claimedAt)
		if err != nil {
			_ = rows.Close()
			return 0, err
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, claim := range claims {
		if _, _, err := releaseStoredTaskClaim(ctx, conn, claim, task.Event{
			Operation: task.OperationRelease, ActorID: "kernel", SessionID: claim.SessionID,
			RunID: "turn-lease-cleanup", RecordedAt: now,
		}, reason, false); err != nil {
			return 0, err
		}
	}
	return len(claims), nil
}
