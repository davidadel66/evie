package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// projectCompilerSource only narrows an offered whole content field. It cannot
// discover an omitted event, change source category, or fall back from a bad
// range/hash to the full event.
func projectCompilerSource(offered memory.CompilerSource, locator memory.EvidenceLocator) (memory.CompilerSource, error) {
	if locator.EventID != offered.Locator.EventID || locator.EventPart != memory.EvidenceContent {
		return memory.CompilerSource{}, errors.New("source was not offered")
	}
	text := offered.Evidence
	switch locator.LocatorKind {
	case memory.LocatorWhole:
		if locator.LocatorValue != "" {
			return memory.CompilerSource{}, errors.New("whole locator has range")
		}
	case memory.LocatorUTF8ByteRange:
		pieces := strings.Split(locator.LocatorValue, ":")
		if len(pieces) != 2 {
			return memory.CompilerSource{}, errors.New("invalid range")
		}
		start, err1 := strconv.Atoi(pieces[0])
		end, err2 := strconv.Atoi(pieces[1])
		if err1 != nil || err2 != nil || strconv.Itoa(start) != pieces[0] || strconv.Itoa(end) != pieces[1] || start < 0 || end <= start || end > len(text) || !utf8.ValidString(text[:start]) || !utf8.ValidString(text[:end]) {
			return memory.CompilerSource{}, errors.New("invalid UTF8 byte range")
		}
		text = text[start:end]
	default:
		return memory.CompilerSource{}, errors.New("unsupported source locator")
	}
	if memory.CompilerHash([]byte(text)) != locator.EvidenceSHA256 {
		return memory.CompilerSource{}, errors.New("source hash mismatch")
	}
	if compilerHasSecret(text) {
		return memory.CompilerSource{}, errors.New("secret source")
	}
	offered.Locator = locator
	offered.Evidence = text
	return offered, nil
}

// ResolveCompilerSource revalidates retained source metadata and current
// eligibility, including whole-field secret exclusion, before rendering an
// exact whole/range quote. It also supports closed source sessions.
func (s *Store) ResolveCompilerSource(ctx context.Context, owner memory.ScopeContext, selection memory.CompilationSelection, source memory.CompilerSource) (memory.CompilerSource, error) {
	var result memory.CompilerSource
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := compilerAuthorize(ctx, conn, owner, selection); err != nil {
			return err
		}
		var err error
		result, err = resolveCompilerSource(ctx, conn, owner, selection, source)
		return err
	})
	return result, err
}
func resolveCompilerSource(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, owner memory.ScopeContext, selection memory.CompilationSelection, source memory.CompilerSource) (memory.CompilerSource, error) {
	var session memory.SessionID
	var seq int64
	var kind, role, text, observed string
	var version int
	var workspace, project sql.NullString
	var size int
	err := q.QueryRowContext(ctx, `SELECT session_id,sequence,event_type,COALESCE(role,''),CASE WHEN length(CAST(content AS BLOB))<=32768 THEN content ELSE '' END,length(CAST(content AS BLOB)),recorded_at,format_version,workspace_id,project_id FROM events WHERE id=?`, source.Locator.EventID).Scan(&session, &seq, &kind, &role, &text, &size, &observed, &version, &workspace, &project)
	if err != nil {
		return memory.CompilerSource{}, err
	}
	if session != selection.SessionID || session != source.SessionID || seq != source.Sequence || seq > selection.Cutoff || source.ScopeKey != scopeKeyForContext(owner) || workspace.String != string(owner.WorkspaceID) || project.String != string(owner.ProjectID) || version != 1 || version != source.FormatVersion || string(source.SourceType) != kind || observed != source.ObservedAt || size > 32768 || !utf8.ValidString(text) || compilerHasSecret(text) {
		return memory.CompilerSource{}, errors.New("source no longer eligible or metadata mismatch")
	}
	if source.Usage == "context" {
		if kind != "assistant_message" || role != "assistant" || source.Actor != "assistant" || source.Authority != "none" {
			return memory.CompilerSource{}, errors.New("context authority mismatch")
		}
	} else if (source.Usage != "new_support" && source.Usage != "overlap") || kind != "user_message" || role != "user" || source.Actor != memory.SemanticActorOwner || source.Authority != memory.AuthorityOwnerStatement {
		return memory.CompilerSource{}, errors.New("support authority mismatch")
	}
	offered := source
	offered.Evidence = text
	offered.Locator = memory.EvidenceLocator{EventID: source.Locator.EventID, EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorWhole, EvidenceSHA256: memory.CompilerHash([]byte(text))}
	projected, err := projectCompilerSource(offered, source.Locator)
	if err != nil {
		return memory.CompilerSource{}, err
	}
	if projected.Evidence != source.Evidence {
		return memory.CompilerSource{}, errors.New("stored source projection mismatch")
	}
	return projected, nil
}

var ErrCompilerTerminalOutput = errors.New("invalid compiler source or forbidden effect")

func validateCompilerOutput(request memory.CompilerRequest, raw []byte) ([]memory.MemoryCandidate, error) {
	var response memory.CompilerResponse
	if err := memory.DecodeCompilerJSON(raw, &response); err != nil {
		return nil, err
	}
	if response.RequestID != "" && response.RequestID != request.ID {
		return nil, fmt.Errorf("%w: request binding mismatch", ErrCompilerTerminalOutput)
	}
	if response.RequestID == "" || response.Candidates == nil || len(response.Candidates) > 16 {
		return nil, errors.New("invalid response binding or candidate array")
	}
	// Required zero-valued members must be present rather than inferred by Go's
	// decoder. This makes absent time/context distinct from explicit unknown/empty.
	var shape struct {
		Candidates []map[string]json.RawMessage `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return nil, err
	}
	offered := map[memory.EventID]memory.CompilerSource{}
	for _, source := range request.Window.Sources {
		offered[source.Locator.EventID] = source
	}
	candidates := make([]memory.MemoryCandidate, 0, len(response.Candidates))
	for index, proposal := range response.Candidates {
		for _, key := range []string{"proposition", "valid_time", "temporal_qualification", "support", "context"} {
			value, ok := shape.Candidates[index][key]
			if !ok || string(value) == "null" {
				return nil, fmt.Errorf("missing candidate %s", key)
			}
		}
		var timeShape map[string]json.RawMessage
		if err := json.Unmarshal(shape.Candidates[index]["valid_time"], &timeShape); err != nil {
			return nil, err
		}
		if _, ok := timeShape["from"]; !ok {
			return nil, errors.New("missing valid time from")
		}
		if _, ok := timeShape["to"]; !ok {
			return nil, errors.New("missing valid time to")
		}
		if err := validateCompilerProposition(request, proposal); err != nil {
			return nil, err
		}
		if proposal.Proposition.Polarity != memory.PolarityAffirmed && proposal.Proposition.Polarity != memory.PolarityDenied {
			return nil, errors.New("invalid polarity")
		}
		if _, err := normalizeValidTime(proposal.ValidTime); err != nil {
			return nil, err
		}
		if len(proposal.TemporalQualification) > 1024 || !utf8.ValidString(proposal.TemporalQualification) || compilerHasSecret(proposal.TemporalQualification) {
			return nil, errors.New("missing or invalid temporal qualification")
		}
		if len(proposal.Support) == 0 || len(proposal.Support) > 64 || proposal.Context == nil || len(proposal.Context) > 8 {
			return nil, errors.New("invalid supporting/context references")
		}
		candidate := memory.MemoryCandidate{Proposal: proposal, Support: []memory.CompilerSource{}, Context: []memory.CompilerSource{}, ReviewState: "unresolved"}
		latest := int64(0)
		latestNew := false
		seen := map[string]bool{}
		for _, entry := range []struct {
			refs    []memory.EvidenceLocator
			context bool
		}{{proposal.Support, false}, {proposal.Context, true}} {
			for _, ref := range entry.refs {
				source, ok := offered[ref.EventID]
				if !ok {
					return nil, fmt.Errorf("%w: unoffered source", ErrCompilerTerminalOutput)
				}
				if (source.Usage == "context") != entry.context {
					return nil, fmt.Errorf("%w: support/context category mismatch", ErrCompilerTerminalOutput)
				}
				identity := string(compilerJSON(ref))
				if seen[identity] {
					return nil, errors.New("duplicate source")
				}
				seen[identity] = true
				projected, err := projectCompilerSource(source, ref)
				if err != nil {
					return nil, errors.Join(ErrCompilerTerminalOutput, err)
				}
				if entry.context {
					candidate.Context = append(candidate.Context, projected)
				} else {
					candidate.Support = append(candidate.Support, projected)
					if source.Sequence > latest {
						latest = source.Sequence
						latestNew = source.Usage == "new_support"
					}
				}
			}
		}
		if !latestNew {
			return nil, fmt.Errorf("%w: latest required support is not newly owned", ErrCompilerTerminalOutput)
		}
		if err := validateCompilerIdentitySupport(candidate); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}
