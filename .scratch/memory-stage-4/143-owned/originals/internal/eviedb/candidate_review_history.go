package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// Historical validation checks the immutable evidence and authority recorded by
// v6. Current disclosure policy, registry availability, and session status do
// not change an already accepted interpretation.
type historicalReviewQuery interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func validateOwnerReviewHistoricalSources(ctx context.Context, q historicalReviewQuery, op memory.OwnerReviewOperation) error {
	for _, candidate := range op.Preview.Candidates {
		for _, category := range []struct {
			sources []memory.CompilerSource
			context bool
		}{{candidate.Candidate.Support, false}, {candidate.Candidate.Context, true}} {
			for _, source := range category.sources {
				if err := validateReviewHistoricalSource(ctx, q, op.SessionID, op.Preview.ScopeKey, source, category.context); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateReviewHistoricalSource(ctx context.Context, q historicalReviewQuery, sessionID memory.SessionID, destination string, source memory.CompilerSource, interpretationContext bool) error {
	rows, err := q.QueryContext(ctx, `
		SELECT e.session_id,e.sequence,e.event_type,COALESCE(e.role,''),
		       CASE WHEN length(CAST(e.content AS BLOB))<=32768 THEN e.content ELSE '' END,
		       length(CAST(e.content AS BLOB)),e.recorded_at,e.format_version,
		       e.workspace_id,e.project_id,s.workspace_id,s.project_id
		FROM events e JOIN sessions s ON s.id=e.session_id WHERE e.id=?
	`, source.Locator.EventID)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return errors.New("historical review source event is missing")
	}
	var eventSession memory.SessionID
	var sequence int64
	var kind, role, content, observed string
	var size, version int
	var eventWorkspace, eventProject, sessionWorkspace, sessionProject sql.NullString
	if err := rows.Scan(&eventSession, &sequence, &kind, &role, &content, &size, &observed, &version, &eventWorkspace, &eventProject, &sessionWorkspace, &sessionProject); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	scopeKind, registryID, err := splitScopeKey(source.ScopeKey)
	if err != nil {
		return err
	}
	var sourceWorkspace, sourceProject sql.NullString
	switch scopeKind {
	case "global":
	case "workspace":
		sourceWorkspace = sql.NullString{String: registryID, Valid: true}
	case "project":
		sourceProject = sql.NullString{String: registryID, Valid: true}
	default:
		return errors.New("invalid historical review source scope")
	}
	if eventSession != sessionID || eventSession != source.SessionID || eventWorkspace != sourceWorkspace || eventProject != sourceProject || sessionWorkspace != sourceWorkspace || sessionProject != sourceProject || destination != source.ScopeKey && destination != "session:"+string(sessionID) {
		return errors.New("historical review source lineage mismatch")
	}
	if sequence != source.Sequence || sequence < 1 || version != 1 || version != source.FormatVersion || kind != string(source.SourceType) || observed != source.ObservedAt || size > 32768 || !utf8.ValidString(content) {
		return errors.New("historical review source metadata mismatch")
	}
	if interpretationContext {
		if source.Usage != "context" || kind != "assistant_message" || role != "assistant" || source.Actor != "assistant" || source.Authority != "none" {
			return errors.New("historical review context authority mismatch")
		}
	} else if (source.Usage != "new_support" && source.Usage != "overlap") || kind != "user_message" || role != "user" || source.Actor != memory.SemanticActorOwner || source.Authority != memory.AuthorityOwnerStatement {
		return errors.New("historical review support authority mismatch")
	}
	offered := source
	offered.Evidence = content
	projected, err := projectHistoricalCompilerSource(offered, source.Locator)
	if err != nil {
		return err
	}
	if projected.Evidence != source.Evidence {
		return errors.New("historical review source projection mismatch")
	}
	return nil
}

// projectHistoricalCompilerSource applies the v6 whole/content-range contract
// without the live compiler's secret detector. Accepted replay must reproduce
// the old projection even when current disclosure rules have become stricter.
func projectHistoricalCompilerSource(offered memory.CompilerSource, locator memory.EvidenceLocator) (memory.CompilerSource, error) {
	if locator.EventID != offered.Locator.EventID || locator.EventPart != memory.EvidenceContent || !utf8.ValidString(offered.Evidence) {
		return memory.CompilerSource{}, errors.New("invalid historical source content")
	}
	text := offered.Evidence
	switch locator.LocatorKind {
	case memory.LocatorWhole:
		if locator.LocatorValue != "" {
			return memory.CompilerSource{}, errors.New("historical whole locator has range")
		}
	case memory.LocatorUTF8ByteRange:
		pieces := strings.Split(locator.LocatorValue, ":")
		if len(pieces) != 2 {
			return memory.CompilerSource{}, errors.New("invalid historical range")
		}
		start, err1 := strconv.Atoi(pieces[0])
		end, err2 := strconv.Atoi(pieces[1])
		if err1 != nil || err2 != nil || strconv.Itoa(start) != pieces[0] || strconv.Itoa(end) != pieces[1] || start < 0 || end <= start || end > len(text) || !utf8.ValidString(text[:start]) || !utf8.ValidString(text[:end]) {
			return memory.CompilerSource{}, errors.New("invalid historical UTF8 byte range")
		}
		text = text[start:end]
	default:
		return memory.CompilerSource{}, errors.New("unsupported historical source locator")
	}
	if memory.CompilerHash([]byte(text)) != locator.EvidenceSHA256 {
		return memory.CompilerSource{}, errors.New("historical source hash mismatch")
	}
	offered.Locator = locator
	offered.Evidence = text
	return offered, nil
}
