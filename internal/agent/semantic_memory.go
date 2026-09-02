package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

// SemanticMemory is the Kernel-owned prepare/apply/read seam consumed by the
// agent harness. Implementations bind storage, canonical scope, provenance,
// revision, and atomicity rules behind this interface.
type SemanticMemory interface {
	PrepareRememberLiteral(context.Context, memory.ScopeContext, memory.RememberLiteralRequest) (memory.RememberLiteralProposal, error)
	ApplyRememberLiteral(context.Context, memory.TurnLease, memory.RememberLiteralProposal) (memory.RememberLiteralResult, error)
	InspectLiteralClaims(context.Context, memory.ScopeContext) (memory.LiteralClaimsInspection, error)
}

func (s *Session) beginLocalSemanticCommand(
	ctx context.Context,
) (memory.TurnLease, func(*error), error) {
	if !s.mu.TryLock() {
		return memory.TurnLease{}, nil, ErrBusy
	}
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return memory.TurnLease{}, nil, err
	}
	if s.owner == nil {
		s.mu.Unlock()
		return memory.TurnLease{}, nil, errors.New("agent: turn ownership is not configured")
	}
	lease, err := s.owner.Acquire(ctx, s.timing.leaseDuration)
	if err != nil {
		s.mu.Unlock()
		if s.owner.IsConflict(err) {
			return memory.TurnLease{}, nil, fmt.Errorf("%w: %v", ErrLeaseConflict, err)
		}
		if s.owner.IsSessionInactive(err) {
			return memory.TurnLease{}, nil, sessionUnavailableError{cause: err}
		}
		return memory.TurnLease{}, nil, fmt.Errorf("acquire turn lease: %w", err)
	}
	finish := func(retErr *error) {
		defer s.mu.Unlock()
		cleanupCtx, cancel := s.timing.newCleanupContext(ctx, s.timing.cleanupTimeout)
		defer cancel()
		if releaseErr := s.owner.Release(cleanupCtx, lease); releaseErr != nil {
			*retErr = errors.Join(*retErr, fmt.Errorf("release turn lease: %w", releaseErr))
		}
	}
	return lease, finish, nil
}

// PrepareRememberLiteral records the exact owner command as Episodic Memory
// under the ordinary session lease before preparing any semantic intent.
func (s *Session) PrepareRememberLiteral(
	ctx context.Context,
	semantic SemanticMemory,
	command string,
	request memory.RememberLiteralRequest,
) (proposal memory.RememberLiteralProposal, retErr error) {
	if semantic == nil {
		return proposal, errors.New("agent: Semantic Memory is not configured")
	}
	lease, finish, err := s.beginLocalSemanticCommand(ctx)
	if err != nil {
		return proposal, err
	}
	defer finish(&retErr)
	event, err := s.history.Append(ctx, lease, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: command,
	})
	if err != nil {
		return proposal, fmt.Errorf("persist remember request: %w", err)
	}
	request.SourceEventID = event.ID
	proposal, err = semantic.PrepareRememberLiteral(ctx, s.scope, request)
	if err != nil {
		return proposal, fmt.Errorf("prepare remember proposal: %w", err)
	}
	return proposal, nil
}

// ResolveRememberLiteral records the approval decision before an approved
// proposal enters the atomic semantic apply path. Declined and expired
// decisions never call the mutation seam.
func (s *Session) ResolveRememberLiteral(
	ctx context.Context,
	semantic SemanticMemory,
	proposal memory.RememberLiteralProposal,
	decision tools.Decision,
) (result memory.RememberLiteralResult, retErr error) {
	if semantic == nil {
		return result, errors.New("agent: Semantic Memory is not configured")
	}
	if proposal.SessionID != s.scope.SessionID {
		return result, errors.New("agent: memory proposal belongs to another session")
	}
	lease, finish, err := s.beginLocalSemanticCommand(ctx)
	if err != nil {
		return result, err
	}
	defer finish(&retErr)
	approval, err := approvalEventInput(
		proposal.Source.EventID,
		memory.ExecutionID(proposal.OperationID),
		decision,
	)
	if err != nil {
		return result, err
	}
	if _, err := s.history.Append(ctx, lease, approval); err != nil {
		return result, fmt.Errorf("persist memory approval: %w", err)
	}
	if decision != tools.Approved {
		return result, nil
	}
	result, err = semantic.ApplyRememberLiteral(ctx, lease, proposal)
	if err != nil {
		return result, fmt.Errorf("apply remember proposal: %w", err)
	}
	return result, nil
}

// InspectSemanticMemory is deliberately eventless: it performs only the
// immutable session-bound exact read.
func (s *Session) InspectSemanticMemory(ctx context.Context, semantic SemanticMemory) (memory.LiteralClaimsInspection, error) {
	if semantic == nil {
		return memory.LiteralClaimsInspection{}, errors.New("agent: Semantic Memory is not configured")
	}
	return semantic.InspectLiteralClaims(ctx, s.scope)
}
