package eviedb

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/davidadel66/evie/internal/memory"
)

// loadReviewCandidate checks destination before decoding protected envelopes.
func loadReviewCandidate(ctx context.Context, q reviewQuery, a OwnerReviewContext, id string, requireSources bool) (memory.OwnerCandidate, error) {
	out := memory.OwnerCandidate{}
	var raw, requestRaw []byte
	var state, equivalent string
	var session memory.SessionID
	err := q.QueryRowContext(ctx, `SELECT c.candidate_id,c.job_id,j.generation_id,j.destination,c.envelope,c.review_state,c.review_revision,COALESCE(c.equivalent_to,''),j.request,j.session_id FROM memory_compiler_candidates c JOIN memory_compiler_jobs j ON j.job_id=c.job_id WHERE c.candidate_id=? AND j.destination=?`, id, a.scope).Scan(&out.Ref.ID, &out.JobID, &out.GenerationID, &out.Destination, &raw, &state, &out.Ref.ReviewRevision, &equivalent, &requestRaw, &session)
	if err != nil {
		return out, ErrOwnerReviewUnauthorized
	}
	out.Ref.InterpretationRevision = 0
	var request memory.CompilerRequest
	if json.Unmarshal(raw, &out.Candidate) != nil || json.Unmarshal(requestRaw, &request) != nil {
		return out, errors.New("invalid retained candidate")
	}
	out.Candidate.ReviewState = state
	out.Candidate.ReviewRevision = out.Ref.ReviewRevision
	out.Candidate.EquivalentTo = equivalent
	owner, err := reviewSourceContext(ctx, q, session)
	if err != nil {
		return out, ErrOwnerReviewUnauthorized
	}
	sourceErr := compilerAuthorize(ctx, q, owner, request.Window.Selection)
	if request.Window.Selection.Destination != a.scope || out.Candidate.ID != id {
		sourceErr = ErrReviewInvalidSource
	}
	if sourceErr == nil {
		sourceErr = validateReviewCandidateSeal(ctx, q, out, request)
	}
	if sourceErr == nil {
		var policy string
		if err = q.QueryRowContext(ctx, `SELECT source_policy FROM memory_review_authorization WHERE singleton=1`).Scan(&policy); err != nil {
			return out, err
		}
		if policy != memory.CompilerPolicyVersion || compilerHasSecret(string(raw)) {
			sourceErr = ErrReviewInvalidSource
		}
	}
	ctx = withCompilerSourceCache(ctx)
	if sourceErr == nil {
		for _, sources := range [][]memory.CompilerSource{out.Candidate.Support, out.Candidate.Context} {
			for _, source := range sources {
				if _, err = resolveCompilerSource(ctx, q, owner, request.Window.Selection, source); err != nil {
					sourceErr = ErrReviewInvalidSource
					break
				}
			}
		}
	}
	if sourceErr != nil {
		if requireSources {
			return out, ErrReviewInvalidSource
		}
		out.Candidate = memory.MemoryCandidate{ID: id, ReviewState: state, ReviewRevision: out.Ref.ReviewRevision, EquivalentTo: equivalent}
		out.Redacted = true
	}
	if err := loadReviewIdentityRevision(ctx, q, &out); err != nil {
		return out, err
	}
	if err := loadReviewTemporalRevision(ctx, q, &out); err != nil {
		return out, err
	}
	canonicalizeReviewCandidate(&out)
	return out, nil
}

func (s *Store) InspectOwnerCandidate(ctx context.Context, a OwnerReviewContext, id string) (memory.OwnerCandidate, error) {
	var out memory.OwnerCandidate
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return out, err
	}
	out, err = loadReviewCandidate(ctx, tx, a, id, false)
	if err != nil {
		return memory.OwnerCandidate{}, err
	}
	return out, tx.Commit()
}

func (s *Store) ListOwnerCandidates(ctx context.Context, a OwnerReviewContext, query memory.OwnerCandidateQuery) (memory.OwnerCandidatePage, error) {
	out := memory.OwnerCandidatePage{ScopeKey: a.scope, Candidates: []memory.OwnerCandidate{}}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 100 {
		return out, errors.New("invalid inbox limit")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT revision FROM memory_review_inbox_revisions WHERE scope_key=?),0)`, a.scope).Scan(&out.Revision); err != nil {
		return out, err
	}
	var cursor struct {
		Revision   int64
		Last, Seal string
	}
	if query.Cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(query.Cursor)
		if err != nil || json.Unmarshal(b, &cursor) != nil || cursor.Seal != reviewCursor(a, cursor.Revision, cursor.Last) {
			return out, errors.New("invalid inbox cursor")
		}
		if cursor.Revision != out.Revision {
			return out, ErrReviewStale
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.candidate_id FROM memory_compiler_candidates c JOIN memory_compiler_jobs j ON j.job_id=c.job_id WHERE j.destination=? AND c.review_state='unresolved' AND c.equivalent_to IS NULL AND c.candidate_id>? ORDER BY c.candidate_id LIMIT ?`, a.scope, cursor.Last, query.Limit+1)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	more := len(ids) > query.Limit
	if more {
		ids = ids[:query.Limit]
	}
	for _, id := range ids {
		item, err := loadReviewCandidate(ctx, tx, a, id, false)
		if err != nil {
			return out, err
		}
		out.Candidates = append(out.Candidates, item)
	}
	if more {
		cursor.Revision = out.Revision
		cursor.Last = ids[len(ids)-1]
		cursor.Seal = reviewCursor(a, cursor.Revision, cursor.Last)
		out.NextCursor = base64.RawURLEncoding.EncodeToString(compilerJSON(cursor))
	}
	return out, tx.Commit()
}

func validateReviewCandidateSeal(ctx context.Context, q reviewQuery, item memory.OwnerCandidate, request memory.CompilerRequest) error {
	requestID := request.ID
	request.ID = ""
	if memory.CompilerHash(compilerJSON(request)) != requestID || memory.CompilerHash(compilerJSON(request.Window)) != request.WindowSHA256 || request.GenerationID != item.GenerationID {
		return ErrReviewInvalidSource
	}
	request.ID = requestID
	var windowHash, groupHash string
	var stage, manifest []byte
	var ordinal int
	err := q.QueryRowContext(ctx, `SELECT j.window_hash,g.envelope_hash,s.envelope,c.ordinal,gen.manifest FROM memory_compiler_jobs j JOIN memory_compiler_candidate_groups g ON g.job_id=j.job_id JOIN memory_compiler_stages s ON s.job_id=j.job_id JOIN memory_compiler_candidates c ON c.job_id=j.job_id JOIN memory_compiler_generations gen ON gen.generation_id=j.generation_id WHERE c.candidate_id=?`, item.Ref.ID).Scan(&windowHash, &groupHash, &stage, &ordinal, &manifest)
	if err != nil || windowHash != request.WindowSHA256 || memory.CompilerHash(stage) != groupHash {
		return ErrReviewInvalidSource
	}
	var generation memory.CompilerGeneration
	if json.Unmarshal(manifest, &generation) != nil {
		return ErrReviewInvalidSource
	}
	generationID, canonical, err := memory.CompilerGenerationIdentity(generation)
	if err != nil || generationID != item.GenerationID || string(canonical) != string(manifest) {
		return ErrReviewInvalidSource
	}
	if request.EvidencePolicy != "" && request.EvidencePolicy != generation.EvidencePolicy || request.EvidencePolicy == "" && generation.EvidencePolicy != memory.CompilerPolicyVersion {
		return ErrReviewInvalidSource
	}
	if request.IdentityPolicy != "" && generation.EntityPolicy != request.IdentityPolicy || request.IdentityPolicy == "" && generation.EntityPolicy != memory.CompilerPolicyVersion {
		return ErrReviewInvalidSource
	}
	if item.Ref.ID != memory.CompilerHash([]byte(fmt.Sprintf("%s:%s:%d", item.JobID, groupHash, ordinal))) {
		return ErrReviewInvalidSource
	}
	var original []memory.MemoryCandidate
	if json.Unmarshal(stage, &original) != nil || ordinal < 0 || ordinal >= len(original) || len(original) > 16 {
		return ErrReviewInvalidSource
	}
	stripped := item.Candidate
	stripped.ID = ""
	stripped.ReviewState = "unresolved"
	stripped.ReviewRevision = 0
	stripped.EquivalentTo = ""
	if string(compilerJSON(stripped)) != string(compilerJSON(original[ordinal])) {
		return ErrReviewInvalidSource
	}
	validated, err := validateCompilerOutput(request, compilerJSON(memory.CompilerResponse{RequestID: requestID, Candidates: []memory.ExtractorCandidate{item.Candidate.Proposal}}))
	if err != nil || len(validated) != 1 || string(compilerJSON(validated[0])) != string(compilerJSON(stripped)) {
		return ErrReviewInvalidSource
	}
	return nil
}

func canonicalizeReviewCandidate(item *memory.OwnerCandidate) {
	sortSources := func(sources []memory.CompilerSource) {
		sort.Slice(sources, func(i, j int) bool {
			return string(compilerJSON(sources[i].Locator)) < string(compilerJSON(sources[j].Locator))
		})
	}
	sortRefs := func(refs []memory.EvidenceLocator) {
		sort.Slice(refs, func(i, j int) bool { return string(compilerJSON(refs[i])) < string(compilerJSON(refs[j])) })
	}
	sortSources(item.Candidate.Support)
	sortSources(item.Candidate.Context)
	sortRefs(item.Candidate.Proposal.Support)
	sortRefs(item.Candidate.Proposal.Context)
}
