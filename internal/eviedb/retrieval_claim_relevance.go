package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// Privacy in accepted-memory recall (2026-10-03, the follow-up recorded after
// memory architecture Stages 6–7). Automatic Recall holds accepted Claims,
// the newer-statement companions it adds beside them, and dense-only hits to
// the relevance floor conversation excerpts already clear. It fails toward
// injecting nothing: a request that merely shares a word with a saved fact
// ("Write a short story about a therapist...") is not about it, and the model
// can still search explicitly. Model-directed reads carry no relevance and are
// not gated.

// denseAutomaticMinimumCosine is the similarity a dense-only hit needs to
// reach Automatic Recall without lexical corroboration, and then only for a
// request to recall something about the owner. It is far above the dense
// floor (denseMinimumCosine), which still governs model-directed searches.
const denseAutomaticMinimumCosine = 0.6

// claimRelevanceGate is Automatic Recall's floor for one accepted-memory read.
type claimRelevanceGate struct {
	relevance *memory.RetrievalRelevance
	allowed   []string
	// frequency counts each request term over the history the request is
	// judged against: the messages the ordinary conversation read could
	// return, plus the accepted Claims this read could return.
	frequency map[string]int
	words     map[memory.SemanticID][]string
}

func newClaimRelevanceGate(ctx context.Context, tx *sql.Tx, scope memory.ScopeContext, query memory.RetrievalQuery, metadata memory.ExactReadMetadata, scopes [3]string) (*claimRelevanceGate, error) {
	if query.Relevance == nil {
		return nil, nil
	}
	cutoff, err := conversationSessionCutoff(ctx, tx, scope.SessionID, query.Relevance)
	if err != nil {
		return nil, err
	}
	frequency, err := relevanceFrequencies(ctx, tx, query.Relevance, scopeKeyForContext(scope), metadata.AsKnownAt, scope.SessionID, cutoff, conversationKeysAllowed(query), nil)
	if err != nil {
		return nil, err
	}
	claims, err := claimRelevanceFrequencies(ctx, tx, query.Relevance, scopes)
	if err != nil {
		return nil, err
	}
	for term, count := range claims {
		frequency[term] = min(frequency[term]+count, relevanceCommonDocuments)
	}
	return &claimRelevanceGate{relevance: query.Relevance, allowed: metadata.AllowedScopes, frequency: frequency, words: map[memory.SemanticID][]string{}}, nil
}

// claimRelevanceFrequencies counts each relevance term among the indexed
// Claims of the reader's Global, Context and session scopes, the population
// an accepted-memory read could return. Added to the conversation counts, it
// keeps a Global fact read in a project whose own history never used its
// words judged by what the reader can see, rather than absent. Counts stop
// at relevanceCommonDocuments, as the excerpt floor's do.
func claimRelevanceFrequencies(ctx context.Context, tx *sql.Tx, relevance *memory.RetrievalRelevance, scopes [3]string) (map[string]int, error) {
	statement, err := tx.PrepareContext(ctx, `SELECT count(*) FROM (SELECT 1 FROM memory_retrieval_fts_v3 f
 WHERE memory_retrieval_fts_v3 MATCH ? AND f.generation=? AND f.scope_key IN (?,?,?)
 AND f.claim_id NOT IN (SELECT claim_id FROM memory_retrieval_dirty) LIMIT ?)`)
	if err != nil {
		return nil, err
	}
	defer statement.Close()
	frequency := map[string]int{}
	for _, group := range append([][]string{relevance.Current}, relevance.Context...) {
		for _, term := range group {
			if _, done := frequency[term]; done {
				continue
			}
			var count int
			if err := statement.QueryRowContext(ctx, `"`+term+`"`, memoryIndexGeneration, scopes[0], scopes[1], scopes[2], relevanceCommonDocuments).Scan(&count); err != nil {
				return nil, err
			}
			frequency[term] = count
		}
	}
	return frequency, nil
}

// admitsClaim applies the excerpt floor to a Claim. Its sources' rendered
// words always count, as an excerpt's would. Its own words (Predicate,
// value, Entity names; the words that key its source messages) count only
// for a request to recall something about the owner: a Claim describes the
// owner's world, and a general or creative request sharing its Predicate
// label is not asking about it (the Stage 6 rule for fact keys). A Claim
// found only by dense similarity qualifies without that corroboration only
// for such a request, at denseAutomaticMinimumCosine or above.
func (g *claimRelevanceGate) admitsClaim(ctx context.Context, q semanticInspectionQueryer, candidate *retrievalCandidate) (bool, error) {
	evidence := candidate.evidence
	tokens := map[string]bool{}
	for _, source := range evidence.Sources {
		for token := range relevanceTokens(source.Evidence) {
			tokens[token] = true
		}
	}
	if relevantTokens(tokens, g.relevance, g.frequency, 0) {
		return true, nil
	}
	if !g.relevance.PersonalRecall {
		return false, nil
	}
	if evidence.Claim != nil {
		words, err := g.claimWords(ctx, q, *evidence.Claim)
		if err != nil {
			return false, err
		}
		for _, word := range words {
			tokens[relevanceKey(word)] = true
		}
		if relevantTokens(tokens, g.relevance, g.frequency, 0) {
			return true, nil
		}
	}
	return candidate.denseScore >= denseAutomaticMinimumCosine, nil
}

// admitsCompanion applies the floor to a newer-statement companion of
// Claims that already qualified. The excerpt's own words must clear it; or,
// for a request to recall something about the owner, the words of the Claims
// the statement may update must, as its keys (their sources' words do not
// count: they are another message).
func (g *claimRelevanceGate) admitsCompanion(ctx context.Context, q semanticInspectionQueryer, text string, family []memory.SemanticClaim) (bool, error) {
	if relevantTokens(relevanceTokens(text), g.relevance, g.frequency, 0) {
		return true, nil
	}
	if !g.relevance.PersonalRecall {
		return false, nil
	}
	keys := map[string]bool{}
	for _, claim := range family {
		words, err := g.claimWords(ctx, q, claim)
		if err != nil {
			return false, err
		}
		for _, word := range words {
			keys[relevanceKey(word)] = true
		}
	}
	return relevantTokens(keys, g.relevance, g.frequency, 0), nil
}

func (g *claimRelevanceGate) claimWords(ctx context.Context, q semanticInspectionQueryer, claim memory.SemanticClaim) ([]string, error) {
	if words, ok := g.words[claim.ID]; ok {
		return words, nil
	}
	words, err := claimRelevanceWords(ctx, q, claim, g.allowed)
	if err != nil {
		return nil, err
	}
	g.words[claim.ID] = words
	return words, nil
}

// denseExcerptCorroborated reports whether a dense hit's excerpt clears the
// excerpt floor by its words, or, where the read may use fact keys (a request
// to recall something about the owner), by the keys of the saved facts its
// span states. Frequencies are the ordinary conversation read's.
func denseExcerptCorroborated(ctx context.Context, tx *sql.Tx, e conversationEvidence, span evidenceSpan, query memory.RetrievalQuery, frequency map[string]int, known time.Time) (bool, error) {
	tokens := relevanceTokens(e.content[span.start:span.end])
	if relevantTokens(tokens, query.Relevance, frequency, 0) {
		return true, nil
	}
	if !conversationKeysAllowed(query) {
		return false, nil
	}
	keys, err := conversationFactKeys(ctx, tx, e, known)
	if err != nil {
		return false, err
	}
	tokens, added := factKeyTokens(tokens, span, keys)
	return added && relevantTokens(tokens, query.Relevance, frequency, 0), nil
}

// claimRelevanceWords returns the words a Claim is about, as fact keys are
// built: its Predicate label and token, its literal value, and the canonical
// names and active aliases, readable in the reader's scopes, of its Entities
// other than anchors (the owner's and Evie's names would match every Claim
// about them).
func claimRelevanceWords(ctx context.Context, q semanticInspectionQueryer, claim memory.SemanticClaim, allowed []string) ([]string, error) {
	parts := []string{claim.Predicate.Label, claim.Predicate.Token}
	if claim.Object.Literal != nil {
		parts = append(parts, claim.Object.Literal.Value)
	}
	for _, id := range []memory.SemanticID{claim.SubjectEntityID, claim.Object.EntityID} {
		if id == "" {
			continue
		}
		entity, err := loadSemanticEntityForInspection(ctx, q, id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if entity.AnchorKind != "" || !containsString(allowed, entity.ScopeKey) {
			continue
		}
		aliases, err := activeEntityAliases(ctx, q, id, allowed)
		if err != nil {
			return nil, err
		}
		parts = append(append(parts, entity.CanonicalName), aliases...)
	}
	words := conversationFactKeyWords(parts)
	if compilerHasSecret(strings.Join(words, " ")) {
		return nil, nil
	}
	return words, nil
}
