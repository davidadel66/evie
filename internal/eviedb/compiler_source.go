package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

var compilerSecrets = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|password|client[_-]?secret)\s*[=:]\s*["']?[^\s"',}]{8,}`),
	regexp.MustCompile(`(?i)"(?:predicate_)?token"\s*:\s*"(?:api[_-]?key|access[_-]?token|password|client[_-]?secret)"`),
	regexp.MustCompile(`(?i)"(?:api[_-]?key|access[_-]?token|password|client[_-]?secret)"\s*:\s*"[^"]{8,}"`),
}

func compilerHasSecret(text string) bool {
	for _, pattern := range compilerSecrets {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

type compilerEvent struct {
	id, parent                 memory.EventID
	execution                  memory.ExecutionID
	session                    memory.SessionID
	workspace, project         string
	hasParent                  bool
	seq                        int64
	kind                       memory.EventType
	role                       memory.EventRole
	content, payload, observed string
	size, version              int
}

// Index coordinates and aggregate counts do not inspect source fields. The
// complete bounded event projection below is the counted source-read boundary.
const compilerEventColumns = `id,parent_id,sequence,event_type,COALESCE(role,''),CASE WHEN length(CAST(content AS BLOB))<=32768 THEN COALESCE(content,'') ELSE '' END,length(CAST(COALESCE(content,'') AS BLOB)),CASE WHEN length(CAST(payload_json AS BLOB))<=8192 THEN COALESCE(payload_json,'') ELSE '' END,recorded_at,format_version,COALESCE(execution_id,''),session_id,COALESCE(workspace_id,''),COALESCE(project_id,'')`

func readCompilerEvent(row interface{ Scan(...any) error }) (e compilerEvent, err error) {
	var parent sql.NullString
	err = row.Scan(&e.id, &parent, &e.seq, &e.kind, &e.role, &e.content, &e.size, &e.payload, &e.observed, &e.version, &e.execution, &e.session, &e.workspace, &e.project)
	e.parent = memory.EventID(parent.String)
	e.hasParent = parent.Valid
	return
}

// Owner authority is independent of the source session's active/closed state.
// Registry identity is checked against SQLite; names and paths grant nothing.
func compilerAuthorize(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, owner memory.ScopeContext, sel memory.CompilationSelection) error {
	if owner.OwnerID != memory.LocalOwnerID || owner.SessionID != sel.SessionID {
		return errors.New("compiler requires exact owner source context")
	}
	var workspace, project sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT workspace_id,project_id FROM sessions WHERE id=?`, sel.SessionID).Scan(&workspace, &project); err != nil {
		return err
	}
	if workspace.String != string(owner.WorkspaceID) || project.String != string(owner.ProjectID) {
		return errors.New("compiler source lineage mismatch")
	}
	destination := scopeKeyForContext(owner)
	if sel.Destination != destination && sel.Destination != "session:"+string(sel.SessionID) {
		return errors.New("compiler destination would widen source scope")
	}
	if owner.WorkspaceID != "" {
		var state string
		if err := q.QueryRowContext(ctx, `SELECT lifecycle_state FROM workspaces WHERE id=?`, owner.WorkspaceID).Scan(&state); err != nil {
			return err
		}
		if state != "active" {
			return errors.New("compiler Workspace unavailable")
		}
	}
	if owner.ProjectID != "" {
		var archived int
		if err := q.QueryRowContext(ctx, `SELECT archived FROM projects WHERE id=?`, owner.ProjectID).Scan(&archived); err != nil {
			return err
		}
		if archived != 0 {
			return errors.New("compiler Project unavailable")
		}
	}
	return requireSemanticScopeKeysAvailable(ctx, q, []string{"global", destination, sel.Destination})
}

func captureCompilerWindow(ctx context.Context, conn compilerQueryer, owner memory.ScopeContext, sel memory.CompilationSelection, first int64, evidencePolicy ...string) (memory.CompilerWindow, string, string, error) {
	clockPolicy := len(evidencePolicy) == 1 && evidencePolicy[0] == memory.CompilerClockEvidencePolicy
	w := memory.CompilerWindow{Selection: sel, FirstSequence: first, NewEventIDs: []memory.EventID{}, Sources: []memory.CompilerSource{}, Omissions: []memory.CompilerOmission{}}
	// Root eligibility is source inspection too. Cache its complete bounded row
	// and omit it from the range read below, so a 128-event window is read once.
	root, err := readCompilerEvent(conn.QueryRowContext(ctx, `SELECT `+compilerEventColumns+` FROM events WHERE id=? AND session_id=?`, sel.RootID, sel.SessionID))
	if err != nil {
		return w, "failed", "invalid_root", err
	}
	rootseq := root.seq
	if root.kind != memory.EventUserMessage || root.role != memory.RoleUser || root.hasParent || sel.Cutoff < rootseq || first < rootseq {
		return w, "failed", "invalid_root", nil
	}

	var last int64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM events WHERE session_id=?`, sel.SessionID).Scan(&last); err != nil {
		return w, "", "", err
	}
	if sel.Cutoff > last {
		return w, "failed", "invalid_cutoff", nil
	}
	// Inspect no more than 128 real events. Locate at most two previous roots;
	// ordinal sequence gaps do not cause loops or synthesized source text.
	start := rootseq
	overlapUnits := 2
	if first > rootseq {
		overlapUnits = 1
	}
	rows, err := conn.QueryContext(ctx, `SELECT sequence FROM events WHERE session_id=? AND event_type='user_message' AND role='user' AND parent_id IS NULL AND sequence<? ORDER BY sequence DESC LIMIT ?`, sel.SessionID, rootseq, overlapUnits)
	if err != nil {
		return w, "", "", err
	}
	for rows.Next() {
		if err := rows.Scan(&start); err != nil {
			rows.Close()
			return w, "", "", err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return w, "", "", err
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM events WHERE session_id=? AND sequence BETWEEN ? AND ? LIMIT 129)`, sel.SessionID, start, sel.Cutoff).Scan(&count); err != nil {
		return w, "", "", err
	}
	if count > 128 {
		return w, "failed", "source_inspection_limit", nil
	}
	rows, err = conn.QueryContext(ctx, `SELECT `+compilerEventColumns+` FROM events WHERE session_id=? AND sequence BETWEEN ? AND ? AND id<>? ORDER BY sequence`, sel.SessionID, start, sel.Cutoff, sel.RootID)
	if err != nil {
		return w, "", "", err
	}
	events := []compilerEvent{root}
	for rows.Next() {
		e, err := readCompilerEvent(rows)
		if err != nil {
			rows.Close()
			return w, "", "", err
		}
		events = append(events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return w, "", "", err
	}
	sort.Slice(events, func(i, j int) bool { return events[i].seq < events[j].seq })
	byID := make(map[memory.EventID]compilerEvent, len(events))
	for _, e := range events {
		byID[e.id] = e
	}

	rootOf := func(e compilerEvent) (memory.EventID, error) {
		visited := map[memory.EventID]bool{}
		for n := 0; n < 128; n++ {
			if visited[e.id] {
				return "", errors.New("event ancestry cycle")
			}
			visited[e.id] = true
			if e.kind == memory.EventTurnFailed || e.kind == memory.EventTurnInterrupted {
				var terminal memory.TurnTerminalPayload
				if json.Unmarshal([]byte(e.payload), &terminal) != nil || terminal.Validate(e.kind) != nil {
					return "", errors.New("invalid terminal payload")
				}
				r, ok := byID[terminal.TurnID]
				if !ok || r.kind != memory.EventUserMessage || r.parent != "" {
					return "", errors.New("invalid terminal root")
				}
				return terminal.TurnID, nil
			}
			if e.parent == "" {
				if e.kind != memory.EventUserMessage || e.role != memory.RoleUser {
					return "", errors.New("event has no owner root")
				}
				return e.id, nil
			}
			parent, ok := byID[e.parent]
			if !ok || parent.seq >= e.seq {
				return "", errors.New("invalid event ancestry")
			}
			e = parent
		}
		return "", errors.New("event ancestry bound")
	}
	newBytes, newCount := 0, 0
	overlap, contexts := []memory.CompilerSource{}, []memory.CompilerSource{}
	for _, e := range events {
		if clockPolicy && (e.session != sel.SessionID || e.workspace != string(owner.WorkspaceID) || e.project != string(owner.ProjectID)) {
			return w, "failed", "invalid_source_lineage", nil
		}
		root, err := rootOf(e)
		if err != nil {
			return w, "failed", "invalid_source_ancestry", nil
		}
		isNew := root == sel.RootID && e.seq >= first
		if isNew {
			w.NewEventIDs = append(w.NewEventIDs, e.id)
		}
		if root == sel.RootID && (e.kind == memory.EventTurnFailed || e.kind == memory.EventTurnInterrupted) {
			w.Closure = string(e.kind)
		}
		if root == sel.RootID && e.kind == memory.EventAssistantMessage {
			var a memory.AssistantMessagePayload
			if e.payload == "" || json.Unmarshal([]byte(e.payload), &a) != nil {
				return w, "failed", "invalid_assistant_payload", nil
			}
			if len(a.ToolCalls) == 0 {
				w.Closure = "final_assistant"
			}
		}
		if e.seq > rootseq && e.kind == memory.EventUserMessage && e.parent == "" && root != sel.RootID {
			w.Closure = "later_root"
		}
		var observation *memory.CompilerObservation
		reason := "prohibited_source"
		usage := ""
		if e.kind == memory.EventUserMessage && e.role == memory.RoleUser {
			if isNew {
				usage = "new_support"
			} else if e.seq < first {
				usage = "overlap"
			}
		} else if e.kind == memory.EventAssistantMessage && e.role == memory.RoleAssistant {
			usage = "context"
		}
		if clockPolicy && e.kind == memory.EventToolSucceeded && compilerHasSecret(e.content) {
			reason = "secret_field"
		}
		if clockPolicy && e.kind == memory.EventToolSucceeded && !compilerHasSecret(e.content) {
			lookup := func(id memory.EventID) (compilerEvent, error) {
				found, ok := byID[id]
				if !ok {
					return compilerEvent{}, errors.New("missing clock ancestry")
				}
				return found, nil
			}
			parent, ok := byID[e.parent]
			if ok && parent.kind == memory.EventApproval {
				parent, ok = byID[parent.parent]
			}
			var intent memory.ToolIntentPayload
			if !ok || parent.kind != memory.EventToolIntent || json.Unmarshal([]byte(parent.payload), &intent) != nil || intent.Call.Name == "" {
				return w, "failed", "invalid_tool_observation", nil
			}
			if intent.Call.Name == "get_time" {
				observation, err = validateClockAncestry(ctx, conn, sel.SessionID, e, lookup)
				if err != nil {
					if !errors.Is(err, errClockInvalidSource) {
						return w, "", "", err
					}
					return w, "failed", "invalid_tool_observation", nil
				}
				if isNew {
					usage = "new_support"
				} else if e.seq < first {
					usage = "overlap"
				}
			}
		}
		if root != sel.RootID && e.seq >= rootseq {
			usage = ""
		}
		if usage != "" {
			switch {
			case e.version != 1:
				return w, "failed", "unsupported_event_version", nil
			case e.size > 32768:
				if usage == "new_support" {
					return w, "failed", "oversized_input", nil
				}
				reason = "field_over_budget"
				usage = ""
			case !utf8.ValidString(e.content):
				return w, "failed", "invalid_utf8", nil
			case compilerHasSecret(e.content):
				reason = "secret_field"
				usage = ""
			case strings.TrimSpace(e.content) == "":
				reason = "empty_field"
				usage = ""
			}
		}
		if usage == "" {
			if isNew {
				w.Omissions = append(w.Omissions, memory.CompilerOmission{EventID: e.id, Sequence: e.seq, Reason: reason, FormatVersion: e.version})
			}
			continue
		}
		source := memory.CompilerSource{Observation: observation, SourceType: memory.SemanticSourceType(e.kind), Locator: memory.EvidenceLocator{EventID: e.id, EventPart: "content", LocatorKind: memory.LocatorWhole, EvidenceSHA256: memory.CompilerHash([]byte(e.content))}, SessionID: sel.SessionID, ScopeKey: scopeKeyForContext(owner), Sequence: e.seq, FormatVersion: e.version, ObservedAt: e.observed, Usage: usage, Evidence: e.content}
		if usage == "context" {
			source.Actor = "assistant"
			source.Authority = "none"
			contexts = append(contexts, source)
		} else {
			source.Actor = memory.SemanticActorOwner
			source.Authority = memory.AuthorityOwnerStatement
			if observation != nil {
				source.Actor = memory.SemanticActorTool
				source.Authority = memory.AuthorityToolObservation
			}
			if usage == "new_support" {
				newBytes += len(e.content)
				newCount++
				w.Sources = append(w.Sources, source)
			} else {
				overlap = append(overlap, source)
			}
		}
	}
	if newBytes > 32768 || newCount > 64 {
		return w, "failed", "oversized_input", nil
	}
	choose := func(items []memory.CompilerSource, maxBytes, maxCount int) {
		bytes, count := 0, 0
		stopped := false
		for i := len(items) - 1; i >= 0; i-- {
			item := items[i]
			if stopped || count >= maxCount || bytes+len(item.Evidence) > maxBytes {
				stopped = true
				w.Omissions = append(w.Omissions, memory.CompilerOmission{EventID: item.Locator.EventID, Sequence: item.Sequence, Reason: item.Usage + "_budget", FormatVersion: item.FormatVersion})
				continue
			}
			w.Sources = append(w.Sources, item)
			bytes += len(item.Evidence)
			count++
		}
	}
	choose(overlap, 8192, 16)
	choose(contexts, 4096, 8)
	sort.Slice(w.Sources, func(i, j int) bool { return w.Sources[i].Sequence < w.Sources[j].Sequence })
	sort.Slice(w.Omissions, func(i, j int) bool { return w.Omissions[i].Sequence < w.Omissions[j].Sequence })
	if w.Closure == "" {
		// A later root closes this captured historical prefix even when that
		// root is outside selection. This indexed count loads no outside text
		// and does not extend the immutable evidence cutoff or covered IDs.
		var later int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM events WHERE session_id=? AND sequence>? AND event_type='user_message' AND role='user' AND parent_id IS NULL LIMIT 1)`, sel.SessionID, sel.Cutoff).Scan(&later); err != nil {
			return w, "", "", err
		}
		if later != 0 {
			w.Closure = "later_root"
		}
	}
	if w.Closure == "" {
		var live int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_turn_leases WHERE session_id=? AND holder_id IS NOT NULL AND julianday(expires_at)>julianday('now')`, sel.SessionID).Scan(&live); err != nil {
			return w, "", "", err
		}
		if live != 0 {
			return w, "deferred_live", "unfinished_live_turn", nil
		}
		w.Closure = "no_live_lease"
	}
	if len(w.NewEventIDs) == 0 {
		return w, "failed", "empty_selection", nil
	}
	if newCount == 0 {
		return w, "excluded", "no_admitted_support", nil
	}
	return w, "queued", "", nil
}

func compilerAcceptedContext(ctx context.Context, conn *sql.Conn, owner memory.ScopeContext, request *memory.CompilerRequest) error {
	keys := []string{"global"}
	if key := scopeKeyForContext(owner); key != "global" {
		keys = append(keys, key)
	}
	if request.Window.Selection.Destination == "session:"+string(owner.SessionID) {
		keys = append(keys, request.Window.Selection.Destination)
	}
	request.Entities = []memory.SemanticEntity{}
	request.Predicates = []memory.SemanticPredicate{}
	request.ScopeRevisions = []memory.ScopeRevision{}
	for _, key := range keys {
		var revision int64
		err := conn.QueryRowContext(ctx, `SELECT revision FROM semantic_scopes WHERE scope_key=?`, key).Scan(&revision)
		if errors.Is(err, sql.ErrNoRows) {
			revision = 0
		} else if err != nil {
			return err
		}
		request.ScopeRevisions = append(request.ScopeRevisions, memory.ScopeRevision{ScopeKey: key, Revision: revision})
		rows, err := conn.QueryContext(ctx, `SELECT e.entity_id,CASE WHEN length(CAST(e.canonical_name AS BLOB))+length(CAST(e.entity_type AS BLOB))<=4096 THEN e.canonical_name ELSE '' END,CASE WHEN length(CAST(e.canonical_name AS BLOB))+length(CAST(e.entity_type AS BLOB))<=4096 THEN e.entity_type ELSE '' END,COALESCE(e.anchor_kind,'') FROM semantic_entities e JOIN semantic_scopes s ON s.scope_id=e.scope_id WHERE s.scope_key=? AND e.lifecycle='active' ORDER BY CASE e.anchor_kind WHEN 'owner' THEN 0 WHEN 'context' THEN 1 ELSE 2 END,e.entity_id LIMIT 33`, key)
		if err != nil {
			return err
		}
		for rows.Next() {
			var e memory.SemanticEntity
			e.ScopeKey = key
			if err := rows.Scan(&e.ID, &e.CanonicalName, &e.EntityType, &e.AnchorKind); err != nil {
				rows.Close()
				return err
			}
			if e.CanonicalName == "" || !utf8.ValidString(e.CanonicalName) || compilerHasSecret(e.CanonicalName+" "+e.EntityType) {
				request.AcceptedContextOmitted = true
				continue
			}
			request.Entities = append(request.Entities, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		rows, err = conn.QueryContext(ctx, `SELECT p.predicate_id,CASE WHEN length(CAST(p.label AS BLOB))<=4096 THEN p.token ELSE '' END,p.version,CASE WHEN length(CAST(p.label AS BLOB))<=4096 THEN p.label ELSE '' END,p.object_constraint,p.cardinality FROM semantic_predicates p JOIN semantic_scopes s ON s.scope_id=p.scope_id WHERE s.scope_key=? ORDER BY p.predicate_id LIMIT 33`, key)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p memory.SemanticPredicate
			if err := rows.Scan(&p.ID, &p.Token, &p.Version, &p.Label, &p.ObjectConstraint, &p.Cardinality); err != nil {
				rows.Close()
				return err
			}
			if p.Token == "" || !utf8.ValidString(p.Label) || compilerHasSecret(p.Token+" "+p.Label) {
				request.AcceptedContextOmitted = true
				continue
			}
			request.Predicates = append(request.Predicates, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	sort.SliceStable(request.Entities, func(i, j int) bool {
		rank := func(e memory.SemanticEntity) int {
			if e.AnchorKind == "owner" {
				return 0
			}
			if e.AnchorKind == "context" {
				return 1
			}
			return 2
		}
		return rank(request.Entities[i]) < rank(request.Entities[j])
	})
	if len(request.Entities) > 32 {
		request.Entities = request.Entities[:32]
		request.AcceptedContextOmitted = true
	}
	if len(request.Predicates) > 32 {
		request.Predicates = request.Predicates[:32]
		request.AcceptedContextOmitted = true
	}
	for i := range request.Entities {
		if len(compilerJSON(request.Entities[:i+1])) > 8192 {
			request.Entities = request.Entities[:i]
			request.AcceptedContextOmitted = true
			break
		}
	}
	for i := range request.Predicates {
		if len(compilerJSON(request.Predicates[:i+1])) > 8192 {
			request.Predicates = request.Predicates[:i]
			request.AcceptedContextOmitted = true
			break
		}
	}
	if err := compilerIdentityContext(ctx, conn, request); err != nil {
		return err
	}
	data, _ := json.Marshal(struct {
		Entities   []memory.SemanticEntity
		Predicates []memory.SemanticPredicate
	}{request.Entities, request.Predicates})
	if compilerHasSecret(string(data)) {
		return errors.New("accepted_context_secret")
	}
	return nil
}

func compilerJSON(value any) []byte {
	b, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("compiler internal JSON: %v", err))
	}
	return b
}
