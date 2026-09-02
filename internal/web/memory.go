package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type memoryTemporalRequest struct {
	ScopeKey  string `json:"scopeKey"`
	ValidAt   string `json:"validAt,omitempty"`
	AsKnownAt string `json:"asKnownAt,omitempty"`
	History   bool   `json:"history,omitempty"`
	Predicate string `json:"predicate,omitempty"`
	Polarity  string `json:"polarity,omitempty"`
}

type memoryObjectsRequest struct {
	memoryTemporalRequest
	Kinds    []memory.SemanticObjectKind `json:"kinds,omitempty"`
	PageSize int                         `json:"pageSize,omitempty"`
	Cursor   string                      `json:"cursor,omitempty"`
}

type memoryInspectRequest struct {
	memoryTemporalRequest
	Kind memory.SemanticObjectKind `json:"kind"`
	ID   memory.SemanticID         `json:"id"`
}

func (s *Server) handleMemoryScopes(w http.ResponseWriter, r *http.Request) {
	if status, err := decodeManagementEmptyObject(w, r); err != nil {
		jsonError(w, status, "body must be one empty JSON object")
		return
	}
	scope, ok := s.activeMemoryScope(w)
	if !ok {
		return
	}
	page, err := s.semanticMemory.ListSemanticScopes(r.Context(), scope, memory.SemanticScopeListQuery{PageSize: 100})
	if err != nil {
		writeMemoryReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleMemoryObjects(w http.ResponseWriter, r *http.Request) {
	var request memoryObjectsRequest
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must be one valid Semantic Memory object query")
		return
	}
	scope, ok := s.activeMemoryScope(w)
	if !ok {
		return
	}
	query, err := request.memoryQuery()
	if err != nil || strings.TrimSpace(request.ScopeKey) == "" {
		managementJSONError(w, http.StatusBadRequest, "invalid_memory_query", "an explicit scopeKey and valid RFC3339 time filters are required")
		return
	}
	page, err := s.semanticMemory.ListSemanticObjects(r.Context(), scope, memory.SemanticObjectListQuery{
		ClaimQuery: query, Kinds: request.Kinds, PageSize: request.PageSize, Cursor: request.Cursor,
	})
	if err != nil {
		writeMemoryReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleMemoryInspect(w http.ResponseWriter, r *http.Request) {
	var request memoryInspectRequest
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must be one valid Semantic Memory inspection query")
		return
	}
	scope, ok := s.activeMemoryScope(w)
	if !ok {
		return
	}
	query, err := request.memoryQuery()
	if err != nil || strings.TrimSpace(request.ScopeKey) == "" || request.ID == "" || !validMemoryObjectKind(request.Kind) {
		managementJSONError(w, http.StatusBadRequest, "invalid_memory_inspection", "scopeKey, a supported object kind, ID, and valid RFC3339 time filters are required")
		return
	}
	inspection, err := s.semanticMemory.InspectSemanticObjectAt(r.Context(), scope, request.Kind, request.ID, query)
	if err != nil {
		writeMemoryReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inspection)
}

func (s *Server) activeMemoryScope(w http.ResponseWriter) (memory.ScopeContext, bool) {
	s.sessionMu.RLock()
	active := s.activeSession
	s.sessionMu.RUnlock()
	if active.ID == "" {
		managementJSONError(w, http.StatusConflict, "memory_scope_required", "select one Context Scope before browsing Semantic Memory")
		return memory.ScopeContext{}, false
	}
	return active.ScopeContext(), true
}

func (r memoryTemporalRequest) memoryQuery() (memory.ClaimQuery, error) {
	query := memory.ClaimQuery{ScopeKey: strings.TrimSpace(r.ScopeKey), PredicateToken: strings.TrimSpace(r.Predicate), Polarity: memory.ClaimPolarity(r.Polarity)}
	var err error
	if r.ValidAt != "" {
		query.ValidAt, err = parseMemoryTime(r.ValidAt)
		if err != nil {
			return query, err
		}
	}
	if r.AsKnownAt != "" {
		query.AsKnownAt, err = parseMemoryTime(r.AsKnownAt)
	}
	return query, err
}

func parseMemoryTime(value string) (*time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func validMemoryObjectKind(kind memory.SemanticObjectKind) bool {
	switch kind {
	case memory.SemanticObjectEntity, memory.SemanticObjectAlias, memory.SemanticObjectClaim,
		memory.SemanticObjectSourceLink, memory.SemanticObjectGraphLink:
		return true
	default:
		return false
	}
}

func writeMemoryReadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, eviedb.ErrStaleCursor):
		managementJSONError(w, http.StatusConflict, "memory_stale_cursor", "Semantic Memory changed; restart this paginated listing")
	case errors.Is(err, eviedb.ErrInvalidCursor):
		managementJSONError(w, http.StatusBadRequest, "memory_invalid_cursor", "the Semantic Memory cursor is invalid for this query")
	case errors.Is(err, eviedb.ErrSemanticScopeQuarantined):
		managementJSONError(w, http.StatusLocked, "memory_scope_quarantined", "the selected Semantic Memory scope is quarantined; verify or rebuild it locally")
	default:
		managementJSONError(w, http.StatusUnprocessableEntity, "memory_inspection_failed", "Semantic Memory could not be inspected")
	}
}
