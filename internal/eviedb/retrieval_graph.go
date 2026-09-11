package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

const (
	retrievalExactCandidates    = 16
	retrievalLexicalCandidates  = 24
	retrievalTemporalCandidates = 8
	retrievalGraphAnchors       = 4
	retrievalGraphWidth         = 16
	retrievalGraphDepth         = 2
)

// A plan belongs to one SQLite snapshot and one shared cancellation deadline.
// Candidate sources cannot expand its effective scopes, temporal interpretation,
// source authority or result budgets. The cache exists only inside this read.
type retrievalQueryPlan struct {
	query       memory.RetrievalQuery
	metadata    memory.ExactReadMetadata
	lexical     string
	scopes      [3]string
	graph       retrievalGraphBounds
	authorities []memory.SourceAuthority
}

type retrievalGraphBounds struct{ anchors, width, depth int }

type retrievalCandidate struct {
	evidence memory.RetrievalEvidence
	score    float64
	direct   bool
}

type retrievalCandidates struct {
	store     *Store
	tx        *sql.Tx
	plan      retrievalQueryPlan
	seen      map[memory.SemanticID]bool
	eligible  map[memory.SemanticID]*retrievalCandidate
	truncated bool
}

func (c *retrievalCandidates) read(ctx context.Context, id memory.SemanticID) (*retrievalCandidate, error) {
	if c.seen[id] {
		return c.eligible[id], nil
	}
	if len(c.seen) >= retrievalCandidateLimit {
		c.truncated = true
		return nil, nil
	}
	c.seen[id] = true
	evidence, eligible, err := c.store.retrievalClaim(ctx, c.tx, c.plan.metadata, id, c.plan.query.Intent, c.plan.query.ValidAt != nil)
	if err != nil || !eligible {
		return nil, err
	}
	for _, source := range evidence.Sources {
		allowed := false
		for _, authority := range c.plan.authorities {
			allowed = allowed || source.Authority == authority
		}
		if !allowed {
			return nil, nil
		}
	}
	item := &retrievalCandidate{evidence: evidence}
	c.eligible[id] = item
	return item, nil
}

func (c *retrievalCandidates) ids(ctx context.Context, limit int, query string, args ...any) ([]memory.SemanticID, error) {
	args = append(args, limit+1)
	rows, err := c.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []memory.SemanticID
	for rows.Next() {
		var id memory.SemanticID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if len(ids) == limit {
			c.truncated = true
			break
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func addRetrievalRank(candidate *retrievalCandidate, reason string, rank int) {
	if containsString(candidate.evidence.Paths, reason) {
		return
	}
	candidate.evidence.Paths = append(candidate.evidence.Paths, reason)
	candidate.score += 1 / float64(60+rank)
}

func (c *retrievalCandidates) directGenerator(ctx context.Context, ids []memory.SemanticID, reason string) error {
	rank := 0
	for _, id := range ids {
		candidate, err := c.read(ctx, id)
		if err != nil {
			return err
		}
		if candidate == nil {
			continue
		}
		// Ineligible candidates never consume a rank in any generator.
		rank++
		candidate.direct = true
		addRetrievalRank(candidate, reason, rank)
	}
	return nil
}

func (s *Store) acceptedRetrievalCandidates(ctx context.Context, tx *sql.Tx, scope memory.ScopeContext, query memory.RetrievalQuery, metadata memory.ExactReadMetadata, lexical string) (*retrievalCandidates, error) {
	c := &retrievalCandidates{store: s, tx: tx,
		plan: retrievalQueryPlan{query: query, metadata: metadata, lexical: lexical,
			scopes:      [3]string{"global", scopeKeyForContext(scope), "session:" + string(scope.SessionID)},
			graph:       retrievalGraphBounds{anchors: retrievalGraphAnchors, width: retrievalGraphWidth, depth: retrievalGraphDepth},
			authorities: []memory.SourceAuthority{memory.AuthorityOwnerStatement, memory.AuthorityToolObservation}},
		seen: make(map[memory.SemanticID]bool), eligible: make(map[memory.SemanticID]*retrievalCandidate)}
	keys := c.plan.scopes
	ids, err := c.ids(ctx, retrievalExactCandidates, `SELECT DISTINCT c.claim_id FROM semantic_claims c JOIN semantic_scopes sc ON sc.scope_id=c.scope_id
 WHERE sc.scope_key IN (?,?,?) AND (c.claim_id=? OR c.subject_entity_id=? OR c.object_entity_id=?
 OR c.subject_entity_id IN (SELECT a.entity_id FROM semantic_aliases a WHERE a.normalized_value=?
 AND (SELECT state FROM semantic_state_events se WHERE se.object_kind='alias' AND se.object_id=a.alias_id AND se.transaction_time<=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1)='active')
 OR c.object_entity_id IN (SELECT a.entity_id FROM semantic_aliases a WHERE a.normalized_value=?
 AND (SELECT state FROM semantic_state_events se WHERE se.object_kind='alias' AND se.object_id=a.alias_id AND se.transaction_time<=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1)='active'))
 ORDER BY c.claim_id LIMIT ?`, keys[0], keys[1], keys[2], query.Text, query.Text, query.Text,
		normalizeAlias(query.Text), formatSemanticTime(metadata.AsKnownAt), normalizeAlias(query.Text), formatSemanticTime(metadata.AsKnownAt))
	if err != nil {
		return nil, err
	}
	if err := c.directGenerator(ctx, ids, "exact_or_alias"); err != nil {
		return nil, err
	}
	if err := c.exactIdentityMatches(ctx, ids); err != nil {
		return nil, err
	}
	ids, err = c.ids(ctx, retrievalLexicalCandidates, `SELECT claim_id FROM memory_retrieval_fts WHERE memory_retrieval_fts MATCH ?
 AND generation=? AND scope_key IN (?,?,?) AND claim_id NOT IN (SELECT claim_id FROM memory_retrieval_dirty)
 ORDER BY bm25(memory_retrieval_fts),claim_id LIMIT ?`, lexical, memoryIndexGeneration, keys[0], keys[1], keys[2])
	if err != nil {
		return nil, err
	}
	if err := c.directGenerator(ctx, ids, "lexical"); err != nil {
		return nil, err
	}
	if query.ValidAt != nil {
		// This generator intersects textual relevance with actual recorded
		// Valid Time; observation time cannot stand in for unknown fact dates.
		ids, err = c.ids(ctx, retrievalTemporalCandidates, `SELECT c.claim_id FROM memory_retrieval_fts f JOIN semantic_claims c ON c.claim_id=f.claim_id
 WHERE memory_retrieval_fts MATCH ? AND f.generation=? AND f.scope_key IN (?,?,?)
 AND f.claim_id NOT IN (SELECT claim_id FROM memory_retrieval_dirty)
 AND (c.valid_from IS NOT NULL OR c.valid_to IS NOT NULL)
 AND (c.valid_from IS NULL OR c.valid_from<=?) AND (c.valid_to IS NULL OR c.valid_to>?)
 ORDER BY c.valid_from IS NULL,c.valid_from DESC,c.claim_id LIMIT ?`, lexical, memoryIndexGeneration, keys[0], keys[1], keys[2], formatSemanticTime(metadata.ValidAt), formatSemanticTime(metadata.ValidAt))
		if err != nil {
			return nil, err
		}
		if err := c.directGenerator(ctx, ids, "temporal"); err != nil {
			return nil, err
		}
	}
	if err := c.graphGenerator(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *retrievalCandidates) ordered() []*retrievalCandidate {
	ordered := make([]*retrievalCandidate, 0, len(c.eligible))
	for _, item := range c.eligible {
		if len(item.evidence.Paths) == 0 {
			continue
		}
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		// A supported direct match precedes proximity-only evidence. Fusion
		// orders within those classes without making proximity truth authority.
		if left.direct != right.direct {
			return left.direct
		}
		if left.score != right.score {
			return left.score > right.score
		}
		if len(left.evidence.Sources) != len(right.evidence.Sources) {
			return len(left.evidence.Sources) > len(right.evidence.Sources)
		}
		return left.evidence.ClaimID < right.evidence.ClaimID
	})
	// Give each discovered subject/predicate a place before adding another
	// proximity-only fact about the same relationship. Direct matches and
	// explicit conflict companions retain their independent priority.
	layers := make(map[memory.SemanticID]int)
	groups := make(map[string]int)
	for _, item := range ordered {
		if item.direct || item.evidence.Claim == nil {
			continue
		}
		claim := item.evidence.Claim
		key := claim.ScopeKey + ":" + string(claim.SubjectEntityID) + ":" + string(claim.Predicate.ID)
		layers[claim.ID] = groups[key]
		groups[key]++
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].direct != ordered[j].direct {
			return ordered[i].direct
		}
		if ordered[i].direct {
			return false
		}
		return layers[ordered[i].evidence.ClaimID] < layers[ordered[j].evidence.ClaimID]
	})
	return ordered
}

func (c *retrievalCandidates) entity(ctx context.Context, id memory.SemanticID) (memory.SemanticEntity, bool, error) {
	entity, err := loadSemanticEntityForInspection(ctx, c.tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return entity, false, nil
	}
	if err != nil || !containsString(c.plan.metadata.AllowedScopes, entity.ScopeKey) {
		return entity, false, err
	}
	state, err := latestStateAt(ctx, c.tx, memory.SemanticObjectEntity, id, formatSemanticTime(c.plan.metadata.AsKnownAt))
	if errors.Is(err, sql.ErrNoRows) {
		return entity, false, nil
	}
	if err != nil || state != memory.SemanticStateActive {
		return entity, false, err
	}
	if c.plan.query.Intent != memory.RetrievalHistorical {
		var current memory.SemanticStateValue
		err = c.tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='entity' AND object_id=e.entity_id ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),e.lifecycle) FROM semantic_entities e WHERE e.entity_id=?`, id).Scan(&current)
		if err != nil || current != memory.SemanticStateActive {
			return entity, false, err
		}
	}
	return entity, true, nil
}

func retrievalContainsPhrase(query, phrase string) bool {
	query, phrase = strings.Trim(retrievalPhrase(query), `"`), strings.Trim(retrievalPhrase(phrase), `"`)
	return phrase != "" && strings.Contains(" "+strings.ToLower(query)+" ", " "+strings.ToLower(phrase)+" ")
}

func (c *retrievalCandidates) graphGenerator(ctx context.Context) error {
	var anchors []memory.SemanticID
	for _, direct := range c.ordered() {
		claim := direct.evidence.Claim
		if claim == nil {
			continue
		}
		for _, id := range []memory.SemanticID{claim.SubjectEntityID, claim.Object.EntityID} {
			if id == "" || containsSemanticID(anchors, id) {
				continue
			}
			entity, eligible, err := c.entity(ctx, id)
			if err != nil {
				return err
			}
			if !eligible {
				continue
			}
			matched := c.plan.query.Text == string(id) || retrievalContainsPhrase(c.plan.query.Text, entity.CanonicalName)
			if !matched {
				var alias int
				err := c.tx.QueryRowContext(ctx, `SELECT count(*) FROM semantic_aliases a WHERE a.entity_id=? AND a.normalized_value=?
 AND (SELECT state FROM semantic_state_events se WHERE se.object_kind='alias' AND se.object_id=a.alias_id AND se.transaction_time<=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1)='active'`, id, normalizeAlias(c.plan.query.Text), formatSemanticTime(c.plan.metadata.AsKnownAt)).Scan(&alias)
				if err != nil {
					return err
				}
				matched = alias > 0
			}
			if !matched && id == claim.SubjectEntityID && claim.Object.EntityID != "" {
				matched = retrievalContainsPhrase(c.plan.query.Text, claim.Predicate.Label) || retrievalContainsPhrase(c.plan.query.Text, claim.Predicate.Token)
			}
			if matched && len(anchors) < c.plan.graph.anchors {
				anchors = append(anchors, id)
			}
		}
	}
	type frontier struct {
		anchor, entity memory.SemanticID
		claims, nodes  []memory.SemanticID
	}
	var current []frontier
	for _, anchor := range anchors {
		current = append(current, frontier{anchor: anchor, entity: anchor, nodes: []memory.SemanticID{anchor}})
	}
	keys := c.plan.scopes
	rank := 0
	for depth := 0; depth < c.plan.graph.depth; depth++ {
		var next []frontier
		for _, start := range current {
			ids, err := c.ids(ctx, c.plan.graph.width, `SELECT c.claim_id FROM semantic_claims c JOIN semantic_scopes sc ON sc.scope_id=c.scope_id
 WHERE sc.scope_key IN (?,?,?) AND (c.subject_entity_id=? OR c.object_entity_id=?) AND c.transaction_time<=?
 ORDER BY c.claim_id LIMIT ?`, keys[0], keys[1], keys[2], start.entity, start.entity, formatSemanticTime(c.plan.metadata.AsKnownAt))
			if err != nil {
				return err
			}
			for _, id := range ids {
				if containsSemanticID(start.claims, id) {
					continue
				}
				item, err := c.read(ctx, id)
				if err != nil {
					return err
				}
				if item == nil || item.evidence.Claim == nil {
					continue
				}
				claim := item.evidence.Claim
				_, eligible, err := c.entity(ctx, claim.SubjectEntityID)
				if err != nil {
					return err
				}
				if !eligible {
					continue
				}
				endpoint := claim.Object.EntityID
				if endpoint != "" {
					_, eligible, err = c.entity(ctx, endpoint)
					if err != nil {
						return err
					}
					if !eligible {
						continue
					}
					if endpoint == start.entity {
						endpoint = claim.SubjectEntityID
					}
					if containsSemanticID(start.nodes, endpoint) {
						continue
					}
				}
				path := memory.RetrievalGraphPath{AnchorEntityID: start.anchor, ClaimIDs: append(append([]memory.SemanticID(nil), start.claims...), id)}
				if !item.direct {
					addRetrievalGraphPath(&item.evidence, path)
					rank++
					reason := "graph_one_hop"
					if depth == 1 {
						reason = "graph_two_hop"
					}
					addRetrievalRank(item, reason, rank)
				}
				if endpoint != "" && claim.Polarity == memory.PolarityAffirmed {
					next = append(next, frontier{anchor: start.anchor, entity: endpoint, claims: path.ClaimIDs, nodes: append(append([]memory.SemanticID(nil), start.nodes...), endpoint)})
				}
			}
		}
		current = next
	}
	return nil
}

func addRetrievalGraphPath(evidence *memory.RetrievalEvidence, path memory.RetrievalGraphPath) {
	for _, existing := range evidence.GraphPaths {
		if existing.AnchorEntityID == path.AnchorEntityID && semanticIDsEqual(existing.ClaimIDs, path.ClaimIDs) {
			return
		}
	}
	if len(evidence.GraphPaths) < 2 {
		evidence.GraphPaths = append(evidence.GraphPaths, path)
	}
}

func semanticIDsEqual(left, right []memory.SemanticID) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func pruneRetrievalGraphSupport(evidence []memory.RetrievalEvidence) {
	selected := make(map[memory.SemanticID]*memory.RetrievalEvidence)
	for i := range evidence {
		item := &evidence[i]
		if item.Kind == memory.RetrievalAcceptedMemory && item.Claim != nil {
			selected[item.ClaimID] = item
		}
	}
	for i := range evidence {
		item := &evidence[i]
		var paths []memory.RetrievalGraphPath
		for _, path := range item.GraphPaths {
			if len(path.ClaimIDs) == 0 || len(path.ClaimIDs) > retrievalGraphDepth || path.ClaimIDs[len(path.ClaimIDs)-1] != item.ClaimID {
				continue
			}
			entity := path.AnchorEntityID
			seen := []memory.SemanticID{entity}
			valid := entity != ""
			for j, id := range path.ClaimIDs {
				support := selected[id]
				if support == nil || support.Intent != item.Intent || support.ValidAtConstrained != item.ValidAtConstrained || !support.ValidAt.Equal(item.ValidAt) || !support.AsKnownAt.Equal(item.AsKnownAt) {
					valid = false
					break
				}
				claim := support.Claim
				if (claim.SubjectEntityID != entity && claim.Object.EntityID != entity) || (j+1 < len(path.ClaimIDs) && claim.Polarity != memory.PolarityAffirmed) {
					valid = false
					break
				}
				endpoint := claim.Object.EntityID
				if endpoint == entity {
					endpoint = claim.SubjectEntityID
				}
				if endpoint == "" && j+1 < len(path.ClaimIDs) || endpoint != "" && containsSemanticID(seen, endpoint) {
					valid = false
					break
				}
				seen = append(seen, endpoint)
				entity = endpoint
			}
			if valid {
				paths = append(paths, path)
			}
		}
		item.GraphPaths = paths
	}
}

func pruneRetrievalGraphPaths(evidence []memory.RetrievalEvidence) []memory.RetrievalEvidence {
	pruneRetrievalGraphSupport(evidence)
	var kept []memory.RetrievalEvidence
	for _, item := range evidence {
		direct := false
		graph := false
		for _, reason := range item.Paths {
			if strings.HasPrefix(reason, "graph_") {
				graph = true
			} else {
				direct = true
			}
		}
		if !graph || direct || len(item.GraphPaths) > 0 {
			kept = append(kept, item)
		}
	}
	return kept
}
