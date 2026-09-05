package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
)

// DatabaseInspector is the owner-facing seam for physical SQLite inspection.
// Its adapter owns table policy and never accepts free-form SQL.
type DatabaseInspector interface {
	InspectDatabaseSchema(context.Context) (eviedb.DatabaseSchema, error)
	InspectDatabaseRows(context.Context, eviedb.DatabaseRowsQuery) (eviedb.DatabaseRows, error)
}

type databaseRowsRequest struct {
	Table    string `json:"table"`
	PageSize int    `json:"pageSize,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

func (s *Server) handleDatabaseSchema(w http.ResponseWriter, r *http.Request) {
	if status, err := decodeManagementEmptyObject(w, r); err != nil {
		jsonError(w, status, "body must be one empty JSON object")
		return
	}
	schema, err := s.databaseInspector.InspectDatabaseSchema(r.Context())
	if err != nil {
		managementJSONError(w, http.StatusUnprocessableEntity, "database_schema_unavailable", "the database schema could not be inspected")
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleDatabaseRows(w http.ResponseWriter, r *http.Request) {
	var request databaseRowsRequest
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must be one valid database row query")
		return
	}
	request.Table = strings.TrimSpace(request.Table)
	if request.Table == "" || request.PageSize < 0 || request.PageSize > 50 || request.Offset < 0 {
		managementJSONError(w, http.StatusBadRequest, "invalid_database_row_query", "table, pageSize from 1 to 50, and a non-negative offset are required")
		return
	}
	rows, err := s.databaseInspector.InspectDatabaseRows(r.Context(), eviedb.DatabaseRowsQuery{
		Table: request.Table, PageSize: request.PageSize, Offset: request.Offset,
	})
	if err != nil {
		writeDatabaseReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func writeDatabaseReadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, eviedb.ErrDatabaseTableNotFound):
		managementJSONError(w, http.StatusBadRequest, "database_table_not_found", "the requested database table does not exist")
	case errors.Is(err, eviedb.ErrDatabaseTableTypedOnly):
		managementJSONError(w, http.StatusForbidden, "database_table_typed_only", "records for this table are available only through its scoped domain view")
	case errors.Is(err, eviedb.ErrDatabaseTableUnavailable):
		managementJSONError(w, http.StatusForbidden, "database_table_unavailable", "records for this internal table are not available in the Data hub")
	default:
		managementJSONError(w, http.StatusUnprocessableEntity, "database_rows_unavailable", "the database records could not be inspected")
	}
}
