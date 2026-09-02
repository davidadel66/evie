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
	PrepareRememberEntity(context.Context, memory.ScopeContext, memory.RememberEntityRequest) (memory.RememberEntityProposal, error)
	ApplyRememberEntity(context.Context, memory.TurnLease, memory.RememberEntityProposal) (memory.RememberEntityResult, error)
	InspectEntityClaims(context.Context, memory.ScopeContext) (memory.EntityClaimsInspection, error)
	InspectEntityClaimsAtScope(context.Context, memory.ScopeContext, bool) (memory.EntityClaimsInspection, error)
	LookupEntitiesByAlias(context.Context, memory.ScopeContext, string) ([]memory.AliasEntityMatch, error)
	LookupEntitiesByAliasAtScope(context.Context, memory.ScopeContext, string, bool) ([]memory.AliasEntityMatch, error)
	InspectSemanticEntity(context.Context, memory.ScopeContext, memory.SemanticID) (memory.SemanticEntity, error)
	InspectSemanticEntityAtScope(context.Context, memory.ScopeContext, memory.SemanticID, bool) (memory.SemanticEntity, error)
	PrepareCorrectClaim(context.Context, memory.ScopeContext, memory.CorrectClaimRequest) (memory.CorrectClaimProposal, error)
	ApplyCorrectClaim(context.Context, memory.TurnLease, memory.CorrectClaimProposal) (memory.CorrectClaimResult, error)
	InspectClaims(context.Context, memory.ScopeContext, memory.ClaimQuery) (memory.ClaimsInspection, error)
}

// SemanticPromotionMemory is the explicit broader-scope mutation seam. It is
// separate from model-facing SemanticMemory so ordinary memory writes cannot
// select or widen their scope.
type SemanticPromotionMemory interface {
	PreparePromotion(context.Context, memory.ScopeContext, memory.PromotionRequest) (memory.PromotionProposal, error)
	ApplyPromotion(context.Context, memory.TurnLease, memory.PromotionProposal) (memory.PromotionResult, error)
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

// PrepareRememberEntity records the owner request before resolving the complete
// Entity/Alias/Claim proposal through the shared Kernel seam.
func (s *Session) PrepareRememberEntity(
	ctx context.Context,
	semantic SemanticMemory,
	command string,
	request memory.RememberEntityRequest,
) (proposal memory.RememberEntityProposal, retErr error) {
	request.UseSessionScope = false
	return s.prepareRememberEntity(ctx, semantic, command, request)
}

// PrepareRememberEntityForCurrentSession is the explicit local-only target.
// The caller chooses no scope identity: the harness binds this operation to the
// current session and the storage boundary revalidates that exact identity.
func (s *Session) PrepareRememberEntityForCurrentSession(
	ctx context.Context,
	semantic SemanticMemory,
	command string,
	request memory.RememberEntityRequest,
) (proposal memory.RememberEntityProposal, retErr error) {
	request.UseSessionScope = true
	return s.prepareRememberEntity(ctx, semantic, command, request)
}

func (s *Session) prepareRememberEntity(
	ctx context.Context,
	semantic SemanticMemory,
	command string,
	request memory.RememberEntityRequest,
) (proposal memory.RememberEntityProposal, retErr error) {
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
	proposal, err = semantic.PrepareRememberEntity(ctx, s.scope, request)
	if err != nil {
		return proposal, fmt.Errorf("prepare remember Entity proposal: %w", err)
	}
	return proposal, nil
}

// ResolveRememberEntity records approval before applying the exact prepared compound effect.
func (s *Session) ResolveRememberEntity(
	ctx context.Context,
	semantic SemanticMemory,
	proposal memory.RememberEntityProposal,
	decision tools.Decision,
) (result memory.RememberEntityResult, retErr error) {
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
	approval, err := approvalEventInput(proposal.Source.EventID, memory.ExecutionID(proposal.OperationID), decision)
	if err != nil {
		return result, err
	}
	if _, err := s.history.Append(ctx, lease, approval); err != nil {
		return result, fmt.Errorf("persist memory approval: %w", err)
	}
	if decision != tools.Approved {
		return result, nil
	}
	result, err = semantic.ApplyRememberEntity(ctx, lease, proposal)
	if err != nil {
		return result, fmt.Errorf("apply remember Entity proposal: %w", err)
	}
	return result, nil
}

// PreparePromotion records the owner's exact broader-scope command as
// Episodic Memory before the Kernel prepares a source-linked Promotion.
func (s *Session) PreparePromotion(
	ctx context.Context,
	semantic SemanticPromotionMemory,
	command string,
	request memory.PromotionRequest,
) (proposal memory.PromotionProposal, retErr error) {
	if semantic == nil {
		return proposal, errors.New("agent: Semantic Promotion Memory is not configured")
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
		return proposal, fmt.Errorf("persist Promotion request: %w", err)
	}
	request.SourceEventID = event.ID
	proposal, err = semantic.PreparePromotion(ctx, s.scope, request)
	if err != nil {
		return proposal, fmt.Errorf("prepare Promotion proposal: %w", err)
	}
	return proposal, nil
}

// ResolvePromotion durably records the owner's exact proposal and prepared
// hashes before an approved broader-scope effect enters the atomic apply path.
// Declined and expired decisions remain Episodic events and never mutate
// Semantic Memory.
func (s *Session) ResolvePromotion(
	ctx context.Context,
	semantic SemanticPromotionMemory,
	proposal memory.PromotionProposal,
	decision tools.Decision,
) (result memory.PromotionResult, retErr error) {
	if semantic == nil {
		return result, errors.New("agent: Semantic Promotion Memory is not configured")
	}
	if proposal.SessionID != s.scope.SessionID {
		return result, errors.New("agent: Promotion proposal belongs to another session")
	}
	lease, finish, err := s.beginLocalSemanticCommand(ctx)
	if err != nil {
		return result, err
	}
	defer finish(&retErr)
	approval, err := semanticApprovalEventInput(
		proposal.Evidence.EventID,
		memory.ExecutionID(proposal.OperationID),
		decision,
		proposal.ProposalSHA256,
		proposal.PreparedSHA256,
	)
	if err != nil {
		return result, err
	}
	if _, err := s.history.Append(ctx, lease, approval); err != nil {
		return result, fmt.Errorf("persist Promotion approval: %w", err)
	}
	if decision != tools.Approved {
		return result, nil
	}
	result, err = semantic.ApplyPromotion(ctx, lease, proposal)
	if err != nil {
		return result, fmt.Errorf("apply Promotion proposal: %w", err)
	}
	return result, nil
}

// PrepareCorrectClaim records the explicit owner correction before preparing
// its complete replacement, temporal effect, evidence, and lifecycle preview.
func (s *Session) PrepareCorrectClaim(
	ctx context.Context,
	semantic SemanticMemory,
	command string,
	request memory.CorrectClaimRequest,
) (proposal memory.CorrectClaimProposal, retErr error) {
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
		return proposal, fmt.Errorf("persist correction request: %w", err)
	}
	request.SourceEventID = event.ID
	proposal, err = semantic.PrepareCorrectClaim(ctx, s.scope, request)
	if err != nil {
		return proposal, fmt.Errorf("prepare correction proposal: %w", err)
	}
	return proposal, nil
}

// ResolveCorrectClaim records approval before applying the exact immutable
// replacement and append-only supersession effect.
func (s *Session) ResolveCorrectClaim(
	ctx context.Context,
	semantic SemanticMemory,
	proposal memory.CorrectClaimProposal,
	decision tools.Decision,
) (result memory.CorrectClaimResult, retErr error) {
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
	approval, err := approvalEventInput(proposal.Source.EventID, memory.ExecutionID(proposal.OperationID), decision)
	if err != nil {
		return result, err
	}
	if _, err := s.history.Append(ctx, lease, approval); err != nil {
		return result, fmt.Errorf("persist memory approval: %w", err)
	}
	if decision != tools.Approved {
		return result, nil
	}
	result, err = semantic.ApplyCorrectClaim(ctx, lease, proposal)
	if err != nil {
		return result, fmt.Errorf("apply correction proposal: %w", err)
	}
	return result, nil
}

// InspectSemanticClaims is eventless and preserves both bitemporal query axes.
func (s *Session) InspectSemanticClaims(
	ctx context.Context,
	semantic SemanticMemory,
	query memory.ClaimQuery,
) (memory.ClaimsInspection, error) {
	if semantic == nil {
		return memory.ClaimsInspection{}, errors.New("agent: Semantic Memory is not configured")
	}
	return semantic.InspectClaims(ctx, s.scope, query)
}
