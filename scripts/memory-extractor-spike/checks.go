package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Check the closed output shape separately from source checks. Go's JSON
// decoder otherwise accepts duplicate keys and treats missing strings as empty.
func validateJSONStructure(raw string) error {
	d := json.NewDecoder(strings.NewReader(raw))
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 12 {
			return errors.New("JSON nesting bound")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return errors.New("duplicate JSON key")
				}
				seen[s] = true
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func validateShape(raw string) error {
	if err := validateJSONStructure(raw); err != nil {
		return err
	}
	var top map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &top) != nil || len(top) != 1 {
		return errors.New("closed root object required")
	}
	var proposals []map[string]json.RawMessage
	if json.Unmarshal(top["candidates"], &proposals) != nil || proposals == nil || len(proposals) > 8 {
		return errors.New("candidates array required")
	}
	for _, p := range proposals {
		keys := []string{"subject", "predicate", "object_kind", "object", "polarity", "kind", "temporal", "identity", "effect", "scope", "sources", "context"}
		if len(p) != len(keys) {
			return errors.New("closed candidate fields required")
		}
		for _, key := range keys {
			if value, ok := p[key]; !ok || string(value) == "null" {
				return fmt.Errorf("missing candidate %s", key)
			}
		}
		for _, key := range []string{"sources", "context"} {
			var refs []map[string]json.RawMessage
			if json.Unmarshal(p[key], &refs) != nil || refs == nil {
				return errors.New("reference array required")
			}
			for _, ref := range refs {
				if len(ref) != 3 || ref["event_id"] == nil || ref["start"] == nil || ref["end"] == nil {
					return errors.New("closed reference fields required")
				}
				for _, coordinate := range []string{"start", "end"} {
					var value int
					if string(ref[coordinate]) == "null" || json.Unmarshal(ref[coordinate], &value) != nil {
						return errors.New("reference coordinates must be non-null integers")
					}
				}
				var eventID string
				if string(ref["event_id"]) == "null" || json.Unmarshal(ref["event_id"], &eventID) != nil || eventID == "" {
					return errors.New("reference event ID must be a nonempty string")
				}
			}
		}
	}
	return nil
}

func validateInput(h history, w window) error {
	events := map[string]event{}
	for _, e := range h.Events {
		events[e.ID] = e
	}
	bytesByKind := map[string]int{}
	countByKind := map[string]int{}
	session := ""
	for _, field := range append(append([]source{}, w.Input.Support...), w.Input.Context...) {
		e, ok := events[field.EventID]
		if !ok || e.FormatVersion != 1 || field.SessionID != e.SessionID || field.Scope != w.Input.Scope || e.Scope != field.Scope {
			return errors.New("missing/versioned/cross-scope source")
		}
		if session == "" {
			session = e.SessionID
		}
		if session != e.SessionID {
			return errors.New("cross-session window")
		}
		if field.EventPart != "content" || field.Start < 0 || field.Start >= field.End || field.End > len(e.Content) || !utf8.ValidString(e.Content) {
			return errors.New("invalid content coordinates")
		}
		exact := e.Content[field.Start:field.End]
		if !utf8.ValidString(exact) || exact != field.Text || field.SHA256 != digest([]byte(exact)) {
			return errors.New("source bytes/hash mismatch")
		}
		if strings.Contains(e.Content, "EVIE_SPIKE_SECRET_DO_NOT_SEND") {
			return errors.New("synthetic protected source excluded")
		}
		switch field.Authority {
		case "owner_statement":
			if e.Type != "user_message" || e.Role != "user" {
				return errors.New("owner authority mismatch")
			}
		case "tool_observation":
			if err := validateClock(e, events); err != nil {
				return err
			}
			if field.Start != 0 || (field.End != 10 && field.End != 19) {
				return errors.New("clock projection forbidden")
			}
		case "none":
			if e.Type != "assistant_message" || field.Ownership != "context" {
				return errors.New("interpretation context must be assistant content")
			}
		default:
			return errors.New("undefined authority")
		}
		if field.Ownership != "new" && field.Ownership != "overlap" && field.Ownership != "context" {
			return errors.New("undefined ownership")
		}
		if (field.Authority == "none") != (field.Ownership == "context") {
			return errors.New("support/context authority mismatch")
		}
		bytesByKind[field.Ownership] += len(exact)
		countByKind[field.Ownership]++
	}
	for kind, limit := range map[string]int{"new": 32768, "overlap": 8192, "context": 4096} {
		if bytesByKind[kind] > limit {
			return fmt.Errorf("%s evidence byte bound", kind)
		}
	}
	for kind, limit := range map[string]int{"new": 64, "overlap": 16, "context": 8} {
		if countByKind[kind] > limit {
			return fmt.Errorf("%s event bound", kind)
		}
	}
	return nil
}

func validateClock(e event, events map[string]event) error {
	ancestor := e
	seen := map[string]bool{}
	rootFound := false
	for depth := 0; depth < 256; depth++ {
		if seen[ancestor.ID] || ancestor.FormatVersion != 1 || ancestor.SessionID != e.SessionID || ancestor.Scope != e.Scope {
			return errors.New("clock ancestry cycle/version/scope/session mismatch")
		}
		seen[ancestor.ID] = true
		if ancestor.Type == "user_message" {
			if ancestor.Role != "user" || ancestor.ParentID != "" {
				return errors.New("clock root is not an owner root")
			}
			rootFound = true
			break
		}
		if ancestor.ParentID == "" {
			return errors.New("clock ancestry missing root")
		}
		next, ok := events[ancestor.ParentID]
		if !ok {
			return errors.New("clock ancestry missing parent")
		}
		ancestor = next
	}
	if !rootFound {
		return errors.New("clock ancestry exceeds bound")
	}
	if e.Type != "tool_succeeded" || e.Role != "tool" || e.ExecutionID == "" || len(e.Content) != 19 {
		return errors.New("undefined/unfinished tool observation")
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", e.Content); err != nil || parsed.Format("2006-01-02 15:04:05") != e.Content {
		return errors.New("invalid unzoned clock")
	}
	var result struct {
		ToolCallID string `json:"tool_call_id"`
		IsError    *bool  `json:"is_error"`
	}
	if json.Unmarshal(e.Payload, &result) != nil || result.IsError == nil || *result.IsError {
		return errors.New("clock outcome is not a success")
	}
	parent, ok := events[e.ParentID]
	if !ok {
		return errors.New("clock intent missing")
	}
	if parent.Type == "approval" {
		var a struct {
			Decision string `json:"decision"`
		}
		if json.Unmarshal(parent.Payload, &a) != nil || a.Decision != "approved" || parent.ExecutionID != e.ExecutionID || parent.SessionID != e.SessionID {
			return errors.New("clock approval invalid")
		}
		parent, ok = events[parent.ParentID]
		if !ok {
			return errors.New("clock intent missing")
		}
	}
	if parent.Type != "tool_intent" || parent.ExecutionID != e.ExecutionID || parent.SessionID != e.SessionID {
		return errors.New("clock intent linkage mismatch")
	}
	type call struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	var intent struct {
		Call call `json:"call"`
	}
	if json.Unmarshal(parent.Payload, &intent) != nil || intent.Call.ID != result.ToolCallID || intent.Call.Name != "get_time" || strings.TrimSpace(intent.Call.Arguments) != "{}" {
		return errors.New("clock capability mismatch")
	}
	assistant, ok := events[parent.ParentID]
	if !ok || assistant.Type != "assistant_message" || assistant.SessionID != e.SessionID {
		return errors.New("assistant tool call missing")
	}
	var payload struct {
		ToolCalls []call `json:"tool_calls"`
	}
	if json.Unmarshal(assistant.Payload, &payload) != nil {
		return errors.New("invalid assistant payload")
	}
	found := false
	for _, c := range payload.ToolCalls {
		if c == intent.Call {
			found = true
		}
	}
	if !found {
		return errors.New("assistant/intent call mismatch")
	}
	terminals := 0
	for _, other := range events {
		if other.ExecutionID == e.ExecutionID && (other.Type == "tool_succeeded" || other.Type == "tool_failed" || other.Type == "tool_cancelled") {
			terminals++
		}
	}
	if terminals != 1 {
		return errors.New("conflicting clock outcomes")
	}
	return nil
}

func validateCandidate(c candidate, in input) error {
	if c.Subject == "" || c.Object == "" || len(c.Object) > 512 || len(c.Subject) > 160 || len(c.Temporal) > 160 {
		return errors.New("missing/oversized meaning")
	}
	for _, check := range []struct{ value, allowed string }{
		{c.Predicate, "preference|habit|relationship|residence|decision|constraint|employment|consideration|intention"},
		{c.ObjectKind, "text|entity|date"}, {c.Polarity, "affirmed|denied"}, {c.Kind, "fact|world_change|error_correction|decision|consideration|intention|additional_support"}, {c.Identity, "resolved|unresolved"}, {c.Effect, "assert|correct|attach_support"},
	} {
		if !slices.Contains(strings.Split(check.allowed, "|"), check.value) {
			return errors.New("schema enum mismatch")
		}
	}
	if c.Scope != in.Scope {
		return errors.New("destination scope mismatch")
	}
	if len(c.Sources) == 0 || len(c.Sources) > 8 || c.Context == nil || len(c.Context) > 8 {
		return errors.New("source/context cardinality")
	}
	encoded, _ := json.Marshal(c)
	if strings.Contains(string(encoded), "EVIE_SPIKE_SECRET_DO_NOT_SEND") {
		return errors.New("synthetic protected candidate excluded")
	}
	newSupport := false
	for _, ref := range c.Sources {
		field, err := resolveReference(ref, in.Support)
		if err != nil {
			return fmt.Errorf("support: %w", err)
		}
		if field.Authority == "none" {
			return errors.New("context cannot support")
		}
		if field.Ownership == "new" {
			newSupport = true
		}
		if field.Authority == "tool_observation" && (ref.Start != 0 || (ref.End != 10 && ref.End != 19)) {
			return errors.New("clock range forbidden")
		}
	}
	if !newSupport {
		return errors.New("candidate has no newly owned support")
	}
	for _, ref := range c.Context {
		if _, err := resolveReference(ref, in.Context); err != nil {
			return fmt.Errorf("context: %w", err)
		}
	}
	return nil
}
func resolveReference(ref reference, fields []source) (source, error) {
	for _, field := range fields {
		if ref.EventID == field.EventID {
			if ref.Start < field.Start || ref.Start >= ref.End || ref.End > field.End {
				return source{}, errors.New("range outside projected source")
			}
			b := field.Text[ref.Start-field.Start : ref.End-field.Start]
			if !utf8.ValidString(b) {
				return source{}, errors.New("range cuts UTF-8 scalar")
			}
			return field, nil
		}
	}
	return source{}, errors.New("unknown source event")
}
