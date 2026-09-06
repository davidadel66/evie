package eviedb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

var errClockInvalidSource = errors.New("invalid clock source linkage")

func validLocalClockDisplay(text string) bool {
	if len(text) != 19 {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] > 127 {
			return false
		}
	}
	parsed, err := time.Parse("2006-01-02 15:04:05", text)
	return err == nil && parsed.Format("2006-01-02 15:04:05") == text
}

func validateClockProjection(source memory.CompilerSource, locator memory.EvidenceLocator) error {
	if source.Observation == nil {
		if source.SourceType == memory.SourceTypeToolSucceeded || source.Actor == memory.SemanticActorTool || source.Authority == memory.AuthorityToolObservation {
			return errors.New("clock source lacks named contract")
		}
		return nil
	}
	if source.Observation.Contract != memory.LocalClockDisplayContract || source.SourceType != memory.SourceTypeToolSucceeded || source.Actor != memory.SemanticActorTool || source.Authority != memory.AuthorityToolObservation || source.Usage == "context" || !validLocalClockDisplay(source.Evidence) {
		return errors.New("invalid clock observation")
	}
	if locator.EventPart != memory.EvidenceContent || (locator.LocatorKind != memory.LocatorWhole || locator.LocatorValue != "") && (locator.LocatorKind != memory.LocatorUTF8ByteRange || locator.LocatorValue != "0:10") {
		return errors.New("uncontracted clock projection")
	}
	return nil
}

// This is a structural co-citation boundary. Whether the owner explicitly
// refers to the checked date is an extraction/review quality judgment, not a
// fact mechanically proved by matching bytes or by owner approval.
func validateClockSupport(candidate memory.MemoryCandidate, admitted bool) error {
	owner, clock := false, false
	for _, source := range candidate.Support {
		if source.SourceType == memory.SourceTypeUserMessage && source.Actor == memory.SemanticActorOwner && source.Authority == memory.AuthorityOwnerStatement && source.Observation == nil {
			owner = true
		}
		if source.Observation != nil || source.SourceType == memory.SourceTypeToolSucceeded || source.Actor == memory.SemanticActorTool || source.Authority == memory.AuthorityToolObservation {
			clock = true
			if !admitted || source.Observation == nil || source.Observation.Contract != memory.LocalClockDisplayContract || source.Observation.RootID == "" || source.Observation.ExecutionID == "" || source.Observation.CallID == "" || !clockHashIdentity(source.Observation.AncestrySHA256) || source.Actor != memory.SemanticActorTool || source.Authority != memory.AuthorityToolObservation || source.SourceType != memory.SourceTypeToolSucceeded {
				return errors.New("uncontracted observation support")
			}
			locator := source.Locator
			if locator.EventPart != memory.EvidenceContent || locator.EvidenceSHA256 != memory.CompilerHash([]byte(source.Evidence)) ||
				!((locator.LocatorKind == memory.LocatorWhole && locator.LocatorValue == "" && validLocalClockDisplay(source.Evidence)) ||
					(locator.LocatorKind == memory.LocatorUTF8ByteRange && locator.LocatorValue == "0:10" && len(source.Evidence) == 10 && validLocalClockDisplay(source.Evidence+" 00:00:00"))) {
				return errors.New("uncontracted observation support projection")
			}
		}
	}
	for _, source := range candidate.Context {
		if source.Observation != nil {
			return errors.New("observation cannot be context")
		}
	}
	if clock && !owner {
		return errors.New("clock requires cited owner assertion referring to checked date")
	}
	return nil
}

// metadata reads stay bounded and never call the capability or include control
// payloads in extractor context. Capture reuses its already counted rows;
// historical replay uses the same validator with durable bounded row lookup.
func validateClockAncestry(ctx context.Context, q historicalReviewQuery, session memory.SessionID, outcome compilerEvent, lookup func(memory.EventID) (compilerEvent, error)) (*memory.CompilerObservation, error) {
	fail := func() (*memory.CompilerObservation, error) { return nil, errClockInvalidSource }
	if outcome.kind != memory.EventToolSucceeded || outcome.role != memory.RoleTool || outcome.execution == "" || outcome.version != 1 || !validLocalClockDisplay(outcome.content) {
		return fail()
	}
	var result memory.ToolResultPayload
	if memory.DecodeCompilerJSON([]byte(outcome.payload), &result) != nil || result.IsError || result.ToolCallID == "" {
		return fail()
	}
	chain := []compilerEvent{outcome}
	parent, err := lookup(outcome.parent)
	if err != nil {
		return nil, clockAncestryReadError(err)
	}
	if parent.kind == memory.EventApproval {
		var approval memory.ApprovalPayload
		if parent.execution != outcome.execution || parent.role != "" || memory.DecodeCompilerJSON([]byte(parent.payload), &approval) != nil || approval.Decision != memory.ApprovalApproved || approval.ProposalSHA256 != "" || approval.PreparedSHA256 != "" {
			return fail()
		}
		chain = append(chain, parent)
		parent, err = lookup(parent.parent)
		if err != nil {
			return nil, clockAncestryReadError(err)
		}
	}
	intent := parent
	var call memory.ToolIntentPayload
	if intent.kind != memory.EventToolIntent || intent.execution != outcome.execution || intent.role != "" || memory.DecodeCompilerJSON([]byte(intent.payload), &call) != nil || call.Call.ID != result.ToolCallID || call.Call.Name != "get_time" || !emptyClockArguments(call.Call.Arguments) {
		return fail()
	}
	chain = append(chain, intent)
	assistant, err := lookup(intent.parent)
	if err != nil {
		return nil, clockAncestryReadError(err)
	}
	var declared memory.AssistantMessagePayload
	if assistant.kind != memory.EventAssistantMessage || assistant.role != memory.RoleAssistant || assistant.execution != "" || memory.DecodeCompilerJSON([]byte(assistant.payload), &declared) != nil {
		return fail()
	}
	matches := 0
	for _, c := range declared.ToolCalls {
		if c.ID == call.Call.ID {
			if c != call.Call {
				return fail()
			}
			matches++
		}
	}
	if matches != 1 {
		return fail()
	}
	chain = append(chain, assistant)
	cursor := assistant
	seen := map[memory.EventID]bool{}
	for len(chain) < 128 {
		if seen[cursor.id] {
			return fail()
		}
		seen[cursor.id] = true
		if cursor.parent == "" {
			break
		}
		cursor, err = lookup(cursor.parent)
		if err != nil {
			return nil, clockAncestryReadError(err)
		}
		chain = append(chain, cursor)
	}
	if cursor.parent != "" || cursor.kind != memory.EventUserMessage || cursor.role != memory.RoleUser {
		return fail()
	}
	for i, e := range chain {
		if e.session != session || e.workspace != outcome.workspace || e.project != outcome.project || e.version != 1 || i > 0 && (chain[i-1].parent != e.id || chain[i-1].seq <= e.seq) {
			return fail()
		}
	}
	// Count every durable outcome for this execution, including outcomes outside
	// the captured cutoff or session. Recovery/duplicate terminals fail closed.
	rows, err := q.QueryContext(ctx, `SELECT session_id,event_type,COUNT(*) FROM events WHERE execution_id=? AND event_type IN ('tool_intent','approval','tool_succeeded','tool_failed','tool_cancelled','execution_resolved') GROUP BY session_id,event_type`, outcome.execution)
	if err != nil {
		return nil, err
	}
	intents, approvals, terminals := 0, 0, 0
	for rows.Next() {
		var sid memory.SessionID
		var kind string
		var count int
		if err = rows.Scan(&sid, &kind, &count); err != nil {
			rows.Close()
			return nil, err
		}
		if sid != session {
			rows.Close()
			return fail()
		}
		switch kind {
		case "tool_intent":
			intents += count
		case "approval":
			approvals += count
		default:
			terminals += count
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	expectedApprovals := 0
	if len(chain) > 1 && chain[1].kind == memory.EventApproval {
		expectedApprovals = 1
	}
	if intents != 1 || terminals != 1 || approvals != expectedApprovals {
		return fail()
	}
	type binding struct {
		ID, Parent memory.EventID
		Sequence   int64
		Kind       memory.EventType
		Role       memory.EventRole
		Execution  memory.ExecutionID
		Payload    string
		Version    int
	}
	metadata := make([]binding, 0, len(chain))
	for _, e := range chain {
		metadata = append(metadata, binding{e.id, e.parent, e.seq, e.kind, e.role, e.execution, e.payload, e.version})
	}
	return &memory.CompilerObservation{Contract: memory.LocalClockDisplayContract, RootID: cursor.id, ExecutionID: outcome.execution, CallID: call.Call.ID, AncestrySHA256: memory.CompilerHash(compilerJSON(metadata))}, nil
}

func resolveClockObservation(ctx context.Context, q historicalReviewQuery, source memory.CompilerSource) error {
	lookup := func(id memory.EventID) (compilerEvent, error) {
		return loadCompilerSourceEvent(ctx, q, id)
	}
	outcome, err := lookup(source.Locator.EventID)
	if err != nil {
		return err
	}
	binding, err := validateClockAncestry(ctx, q, source.SessionID, outcome, lookup)
	if err != nil {
		return err
	}
	if string(compilerJSON(binding)) != string(compilerJSON(source.Observation)) {
		return errors.New("clock observation ancestry changed")
	}
	return nil
}

func candidateHasClock(candidate memory.MemoryCandidate) bool {
	for _, source := range candidate.Support {
		if source.Observation != nil {
			return true
		}
	}
	return false
}

func reviewClockSource(source memory.SemanticSource) bool {
	return source.Actor == memory.SemanticActorTool && source.SourceType == memory.SourceTypeToolSucceeded && source.Authority == memory.AuthorityToolObservation
}

func validateReviewClockEncoding(p memory.ReviewPreview) error {
	clock := candidateHasClock(p.Candidates[0].Candidate)
	if (p.Version == "owner-review-preview-v4") != clock {
		return errors.New("review version does not bind observation contract")
	}
	return validateClockSupport(p.Candidates[0].Candidate, clock)
}

// The cache belongs to one validation transaction. Sources and their control
// ancestors share it, so each full bounded event row is inspected at most once.
type compilerSourceCacheKey struct{}
type compilerSourceCache struct {
	events map[memory.EventID]compilerEvent
}

func withCompilerSourceCache(ctx context.Context) context.Context {
	if _, ok := ctx.Value(compilerSourceCacheKey{}).(*compilerSourceCache); ok {
		return ctx
	}
	return context.WithValue(ctx, compilerSourceCacheKey{}, &compilerSourceCache{events: map[memory.EventID]compilerEvent{}})
}
func loadCompilerSourceEvent(ctx context.Context, q historicalReviewQuery, id memory.EventID) (compilerEvent, error) {
	cache, _ := ctx.Value(compilerSourceCacheKey{}).(*compilerSourceCache)
	if cache != nil {
		if e, ok := cache.events[id]; ok {
			return e, nil
		}
		if len(cache.events) >= 128 {
			return compilerEvent{}, errors.New("source_inspection_limit")
		}
	}
	var e compilerEvent
	var err error
	query := `SELECT ` + compilerEventColumns + ` FROM events WHERE id=?`
	if rowQuery, ok := q.(interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}); ok {
		e, err = readCompilerEvent(rowQuery.QueryRowContext(ctx, query, id))
	} else {
		rows, queryErr := q.QueryContext(ctx, query, id)
		if queryErr != nil {
			return e, queryErr
		}
		if !rows.Next() {
			readErr := errors.Join(rows.Err(), rows.Close())
			if readErr != nil {
				return e, readErr
			}
			return e, sql.ErrNoRows
		}
		e, err = readCompilerEvent(rows)
		err = errors.Join(err, rows.Err(), rows.Close())
	}
	if err == nil && cache != nil {
		cache.events[id] = e
	}
	return e, err
}

func emptyClockArguments(args string) bool {
	if len(args) > 8192 || memory.ValidateCompilerJSON([]byte(args)) != nil {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal([]byte(args), &object) == nil && object != nil && len(object) == 0
}

// Accepted Source Links retain authority while their immutable reviewed
// CompilerSource manifest pins the full named contract and ancestry. Promotion
// follows that origin before this shared exact projection is allowed to run.
func acceptedCompilerSource(source memory.SemanticSource) memory.CompilerSource {
	offered := memory.CompilerSource{Locator: memory.EvidenceLocator{EventID: source.EventID}, Evidence: source.Evidence}
	if reviewClockSource(source) {
		offered.SourceType = source.SourceType
		offered.Actor = source.Actor
		offered.Authority = source.Authority
		offered.Observation = &memory.CompilerObservation{Contract: memory.LocalClockDisplayContract}
	}
	return offered
}

func clockHashIdentity(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

// A completed missing-row lookup proves broken linkage. A failed read proves
// nothing about source eligibility: retain its coded SQLite/connection/context
// identity so the worker leaves durable queued work retryable.
func clockAncestryReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) && compilerDataFailure(err) {
		return errClockInvalidSource
	}
	return err
}
