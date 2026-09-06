package eviedb

import (
	"context"
	"errors"
	"github.com/davidadel66/evie/internal/memory"
	"sort"
	"strings"
)

// The inbox remains source-scoped. Only the sealed semantic effect changes
// destination. Source lineage is still checked by the existing review boundary.
func candidateEffectDestination(c memory.OwnerCandidate) (string, error) {
	d := c.Candidate.Proposal.Destination
	if d == "" {
		return c.Destination, nil
	}
	if len(c.Candidate.Support) == 0 {
		return "", ErrReviewInvalidSource
	}
	first := c.Candidate.Support[0]
	bound := memory.ScopeContext{SessionID: first.SessionID}
	if strings.HasPrefix(first.ScopeKey, "workspace:") {
		bound.WorkspaceID = memory.WorkspaceID(strings.TrimPrefix(first.ScopeKey, "workspace:"))
	} else if strings.HasPrefix(first.ScopeKey, "project:") {
		bound.ProjectID = memory.ProjectID(strings.TrimPrefix(first.ScopeKey, "project:"))
	}
	for _, source := range c.Candidate.Support {
		if source.SessionID != first.SessionID || source.ScopeKey != first.ScopeKey {
			return "", errors.New("scope recommendation requires one source conversation")
		}
	}
	return memory.ResolveMemoryDestination(bound, d, false)
}

func candidateEffectScopes(ctx context.Context, q reviewQuery, candidates []memory.OwnerCandidate) (string, []string, error) {
	if len(candidates) == 0 {
		return "", nil, errors.New("missing candidates")
	}
	target, err := candidateEffectDestination(candidates[0])
	if err != nil {
		return "", nil, err
	}
	keys := map[string]bool{}
	for _, c := range candidates {
		other, err := candidateEffectDestination(c)
		if err != nil {
			return "", nil, err
		}
		if other != target {
			return "", nil, errors.New("review memories with different applicability separately")
		}
		for _, scope := range []string{c.Destination, target} {
			allowed, err := reviewLineageScopeKeys(ctx, q, scope)
			if err != nil {
				return "", nil, err
			}
			for _, key := range allowed {
				keys[key] = true
			}
		}
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return target, out, nil
}

func validateCandidateDestinations(ctx context.Context, q reviewQuery, op memory.OwnerReviewOperation) error {
	if op.Preview.Effect == nil {
		return errors.New("missing effect")
	}
	recommended := false
	for _, c := range op.Preview.Candidates {
		recommended = recommended || c.Candidate.Proposal.Destination != ""
	}
	if !recommended {
		return nil
	} // Preserve legacy accepted envelopes.
	target, keys, err := candidateEffectScopes(ctx, q, op.Preview.Candidates)
	if err != nil {
		return err
	}
	if target != op.Preview.Effect.Scope.Key || len(keys) != len(op.Preview.Effect.Scopes) {
		return errors.New("candidate destination or scope vector changed")
	}
	for i, key := range keys {
		if op.Preview.Effect.Scopes[i].Key != key {
			return errors.New("unauthorized candidate scope")
		}
	}
	for _, c := range op.Preview.Candidates {
		if c.Candidate.Proposal.Destination == "" {
			continue
		}
		bound, err := reviewSourceContext(ctx, q, c.Candidate.Support[0].SessionID)
		if err != nil {
			return err
		}
		expected, err := memory.ResolveMemoryDestination(bound, c.Candidate.Proposal.Destination, false)
		if err != nil {
			return err
		}
		if expected != target {
			return errors.New("candidate destination differs from its source session")
		}
	}
	return nil
}

// Historical validation uses immutable lineage, independent of current archive
// or authorization state; live preparation separately checks availability.
func reviewLineageScopeKeys(ctx context.Context, q reviewQuery, key string) ([]string, error) {
	kind, id, err := splitScopeKey(key)
	if err != nil {
		return nil, err
	}
	keys := []string{"global"}
	if kind != "global" {
		keys = append(keys, key)
	}
	if kind == "session" {
		bound, err := reviewSourceContext(ctx, q, memory.SessionID(id))
		if err != nil {
			return nil, err
		}
		contextKey := scopeKeyForContext(bound)
		if contextKey != "global" {
			keys = append(keys, contextKey)
		}
	}
	sort.Strings(keys)
	return keys, nil
}
