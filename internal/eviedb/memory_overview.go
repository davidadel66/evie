package eviedb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// MemoryOverview answers "what do you know about me / this project" in one
// bounded read (docs/memory-architecture.md, Stage 3): the standing set, then
// the other current, approved memories of the profile (Global and this
// conversation) and of the conversation's Workspace or project. It reads only
// the session's own scopes, renders Claim text (never source text), skips
// quarantined scopes, and counts pending inbox proposals without their
// content.
func (s *Store) MemoryOverview(ctx context.Context, scope memory.ScopeContext, query memory.MemoryOverviewQuery) (memory.MemoryOverview, error) {
	query, err := query.Normalize()
	if err != nil {
		return memory.MemoryOverview{}, err
	}
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.MemoryOverview{}, err
	}
	includeProfile := query.Scope != memory.MemoryOverviewWorkspace
	includeWorkspace := query.Scope != memory.MemoryOverviewProfile
	profileKeys := []string{"global", "session:" + string(scope.SessionID)}
	var workspaceKeys []string
	if contextKey := scopeKeyForContext(scope); contextKey != "global" {
		workspaceKeys = []string{contextKey}
	}
	var keys []string
	if includeProfile {
		keys = append(keys, profileKeys...)
	}
	if includeWorkspace {
		keys = append(keys, workspaceKeys...)
	}
	now := s.now()
	result := memory.MemoryOverview{Scope: query.Scope, Standing: []memory.MemoryOverviewItem{}}
	standing, err := loadStandingMemory(ctx, dbLifecycleQueryer{s.db}, keys, now)
	if err != nil {
		return result, err
	}
	for _, item := range standing {
		overviewItem, ok := newOverviewItem(item.ClaimID, item.ScopeKey, item.Kind, true, item.Text)
		if !ok {
			result.StandingWithheld++
			continue
		}
		result.Standing = append(result.Standing, overviewItem)
	}
	if includeProfile {
		if result.Profile, err = s.overviewSection(ctx, profileKeys, now); err != nil {
			return result, err
		}
	}
	if includeWorkspace {
		if len(workspaceKeys) == 0 {
			result.WorkspaceNote = "This conversation has no Workspace or project, so there is no Workspace memory."
		} else if result.Workspace, err = s.overviewSection(ctx, workspaceKeys, now); err != nil {
			return result, err
		}
	}
	if len(keys) > 0 {
		if result.PendingReview, err = s.PendingMemoryProposalCount(ctx, keys...); err != nil {
			return result, err
		}
	}
	return result, nil
}

func newOverviewItem(claimID memory.SemanticID, scopeKey string, kind memory.ClaimKind, standing bool, text string) (memory.MemoryOverviewItem, bool) {
	if kind == "" {
		kind = memory.ClaimKindFact
	}
	shown, shortened := memory.ShortenOverviewText(text)
	item := memory.MemoryOverviewItem{ClaimID: claimID, Applies: memory.StandingScopeLabel(scopeKey), ScopeKey: scopeKey, Kind: kind,
		Standing: standing, Memory: shown, Shortened: shortened}
	if memory.HasRetrievalSecret([]byte(text)) {
		return item, false
	}
	return item, true
}

// overviewCurrentClaims selects active, approved Claims in the given scopes
// that do not always apply, are valid now, keep an eligible source and lie
// outside quarantined scopes. Arguments: the scope keys, then now twice.
const overviewCurrentClaims = `
	FROM semantic_claims AS claims
	JOIN semantic_scopes AS scopes ON scopes.scope_id = claims.scope_id
	JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
	JOIN semantic_entities AS subject ON subject.entity_id = claims.subject_entity_id
	LEFT JOIN semantic_entities AS object ON object.entity_id = claims.object_entity_id
	LEFT JOIN semantic_claim_classifications AS classes ON classes.claim_id = claims.claim_id
	     AND NOT EXISTS (
	         SELECT 1 FROM semantic_claim_classifications AS newer
	         WHERE newer.claim_id = classes.claim_id AND newer.scope_revision > classes.scope_revision)
	WHERE scopes.scope_key IN (%s)
	  AND scopes.scope_id NOT IN (SELECT scope_id FROM semantic_projection_quarantine)
	  AND COALESCE(classes.standing, 0) = 0
	  AND COALESCE((SELECT state FROM semantic_state_events
	      WHERE object_kind = 'claim' AND object_id = claims.claim_id
	      ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), claims.lifecycle) = 'active'
	  AND EXISTS (
	      SELECT 1 FROM semantic_source_links AS sources
	      WHERE sources.claim_id = claims.claim_id
	        AND COALESCE((SELECT state FROM semantic_state_events
	            WHERE object_kind = 'source_link' AND object_id = sources.source_link_id
	            ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), sources.eligibility) = 'eligible')
	  AND (claims.valid_from IS NULL OR claims.valid_from <= ?)
	  AND (claims.valid_to IS NULL OR claims.valid_to > ?)`

// overviewSection lists a section's current memories that do not always
// apply: Claims about the owner first, then the newest.
func (s *Store) overviewSection(ctx context.Context, keys []string, now time.Time) (*memory.MemoryOverviewSection, error) {
	section := &memory.MemoryOverviewSection{Items: []memory.MemoryOverviewItem{}}
	at := formatSemanticTime(now.UTC())
	args := make([]any, 0, len(keys)+3)
	for _, key := range keys {
		args = append(args, key)
	}
	args = append(args, at, at)
	from := fmt.Sprintf(overviewCurrentClaims, strings.TrimSuffix(strings.Repeat("?,", len(keys)), ","))
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+from, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count memory overview: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT claims.claim_id, scopes.scope_key, COALESCE(classes.kind, ''), subject.canonical_name, predicates.label,
		       claims.object_kind, COALESCE(claims.literal_value, ''), COALESCE(object.canonical_name, ''), claims.polarity
		`+from+`
		ORDER BY CASE WHEN subject.anchor_kind = 'owner' THEN 0 ELSE 1 END, claims.transaction_time DESC, claims.claim_id
		LIMIT ?`, append(args, memory.MemoryOverviewSectionLimit)...)
	if err != nil {
		return nil, fmt.Errorf("read memory overview: %w", err)
	}
	defer rows.Close()
	listed := 0
	for rows.Next() {
		var claimID memory.SemanticID
		var scopeKey, subjectName, label, objectKind, literal, objectName string
		var kind memory.ClaimKind
		var polarity memory.ClaimPolarity
		if err := rows.Scan(&claimID, &scopeKey, &kind, &subjectName, &label, &objectKind, &literal, &objectName, &polarity); err != nil {
			return nil, err
		}
		listed++
		item, ok := newOverviewItem(claimID, scopeKey, kind, false, standingClaimText(subjectName, label, objectKind, literal, objectName, polarity))
		if !ok {
			section.Withheld++
			continue
		}
		section.Items = append(section.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	section.Omitted = total - listed
	return section, nil
}
