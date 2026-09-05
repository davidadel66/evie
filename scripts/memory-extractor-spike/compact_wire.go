package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

const compactVersion = "compact-v1"

type compactSource struct {
	Alias        string `json:"alias"`
	Source       source `json:"source"`
	Sequence     int    `json:"sequence"`
	RootID       string `json:"root_id"`
	RootSequence int    `json:"root_sequence"`
	Role         string `json:"role"`
	ObservedAt   string `json:"observed_at"`
	EventVersion int    `json:"event_version"`
}
type compactEntity struct {
	Alias    string `json:"alias"`
	EntityID string `json:"entity_id"`
	Original string `json:"original"`
}
type compactSeal struct {
	Version        string          `json:"version"`
	CorpusSHA256   string          `json:"corpus_sha256"`
	WindowID       string          `json:"window_id"`
	WindowSHA256   string          `json:"window_sha256"`
	EvidencePolicy string          `json:"evidence_policy"`
	Scope          string          `json:"scope"`
	Closure        string          `json:"closure"`
	Cutoff         int             `json:"captured_sequence"`
	InputSHA256    string          `json:"input_sha256"`
	Sources        []compactSource `json:"sources"`
	Entities       []compactEntity `json:"entities"`
}
type compactRecord struct {
	Seal                    compactSeal `json:"seal"`
	SealSHA256              string      `json:"seal_sha256"`
	Request                 string      `json:"request"`
	SystemSHA256            string      `json:"system_sha256,omitempty"`
	SchemaSHA256            string      `json:"schema_sha256,omitempty"`
	SchemaDerivationVersion string      `json:"schema_derivation_version,omitempty"`
}
type compactField struct {
	Ref          string `json:"ref"`
	Sequence     int    `json:"sequence"`
	Root         string `json:"root"`
	Role         string `json:"role"`
	Authority    string `json:"authority"`
	Ownership    string `json:"ownership"`
	Text         string `json:"text"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	DateSelector bool   `json:"date_selector,omitempty"`
}
type compactInput struct {
	WindowID  string           `json:"window_id"`
	Cutoff    int              `json:"captured_sequence"`
	Roots     []int            `json:"root_sequences"`
	Omissions [][2]int         `json:"omitted_sequence_ranges"`
	Fields    []compactField   `json:"fields"`
	Accepted  []map[string]any `json:"accepted_identity_context"`
}
type compactRef struct {
	Ref      string `json:"ref"`
	Selector string `json:"selector,omitempty"`
	Start    *int   `json:"start,omitempty"`
	End      *int   `json:"end,omitempty"`
}
type compactCandidate struct {
	SubjectType      string       `json:"subject_type"`
	SubjectName      string       `json:"subject_name"`
	SubjectEntityRef string       `json:"subject_entity_ref"`
	Predicate        string       `json:"predicate"`
	ObjectKind       string       `json:"object_kind"`
	Object           string       `json:"object"`
	Polarity         string       `json:"polarity"`
	Kind             string       `json:"kind"`
	Temporal         string       `json:"temporal"`
	Identity         string       `json:"identity"`
	Effect           string       `json:"effect"`
	Sources          []compactRef `json:"sources"`
	Context          []compactRef `json:"context"`
}

// No source selection happens here. Only the exact already selected projections
// enter Fields; ancestry/sequence metadata supplies boundaries, never extra text.
func sealCompact(h history, w window, policy, corpusSHA string) (compactInput, compactSeal, error) {
	in := compactInput{WindowID: w.ID, Cutoff: w.CapturedSequence, Roots: []int{}, Omissions: [][2]int{}, Fields: []compactField{}, Accepted: []map[string]any{}}
	windowBytes, _ := json.Marshal(w)
	seal := compactSeal{Version: compactVersion, CorpusSHA256: corpusSHA, WindowID: w.ID, WindowSHA256: digest(windowBytes), EvidencePolicy: policy, Scope: w.Input.Scope, Closure: w.Closure, Cutoff: w.CapturedSequence, Sources: []compactSource{}, Entities: []compactEntity{}}
	if err := validateInput(h, w); err != nil {
		return in, seal, err
	}
	events := map[string]event{}
	sequences := map[int]bool{}
	for _, e := range h.Events {
		if e.Sequence <= 0 || events[e.ID].ID != "" || sequences[e.Sequence] {
			return in, seal, errors.New("compact source sequence/identity invalid")
		}
		events[e.ID] = e
		sequences[e.Sequence] = true
	}
	for _, field := range w.Input.Support {
		if field.Ownership == "context" || field.Authority == "none" {
			return in, seal, errors.New("compact input support/context category mismatch")
		}
	}
	for _, field := range w.Input.Context {
		if field.Ownership != "context" || field.Authority != "none" {
			return in, seal, errors.New("compact input support/context category mismatch")
		}
	}
	fields := append(append([]source{}, w.Input.Support...), w.Input.Context...)
	sort.Slice(fields, func(i, j int) bool { return events[fields[i].EventID].Sequence < events[fields[j].EventID].Sequence })
	seen := map[string]bool{}
	first := w.CapturedSequence + 1
	for _, f := range fields {
		e := events[f.EventID]
		if seen[e.ID] || e.Sequence > w.CapturedSequence {
			return in, seal, errors.New("compact duplicate/future projection")
		}
		seen[e.ID] = true
		if _, err := time.Parse(time.RFC3339Nano, e.RecordedAt); err != nil {
			return in, seal, errors.New("compact missing observed time")
		}
		ancestor := e
		ancestry := map[string]bool{}
		for ancestor.ParentID != "" {
			if len(ancestry) >= 256 {
				return in, seal, errors.New("compact root ancestry exceeds bound")
			}
			if ancestry[ancestor.ID] {
				return in, seal, errors.New("compact root cycle")
			}
			ancestry[ancestor.ID] = true
			parent, ok := events[ancestor.ParentID]
			if !ok || parent.Sequence >= ancestor.Sequence || parent.SessionID != e.SessionID || parent.Scope != e.Scope || parent.FormatVersion != 1 {
				return in, seal, errors.New("compact root lineage invalid")
			}
			ancestor = parent
		}
		if ancestor.Type != "user_message" || ancestor.Role != "user" {
			return in, seal, errors.New("compact source has no owner root")
		}
		if ancestor.Sequence < first {
			first = ancestor.Sequence
		}
		alias := fmt.Sprintf("s%d", len(seal.Sources)+1)
		seal.Sources = append(seal.Sources, compactSource{alias, f, e.Sequence, ancestor.ID, ancestor.Sequence, e.Role, e.RecordedAt, e.FormatVersion})
		in.Fields = append(in.Fields, compactField{alias, e.Sequence, fmt.Sprintf("r%d", ancestor.Sequence), e.Role, f.Authority, f.Ownership, f.Text, f.Start, f.End, f.Authority == "tool_observation" && f.Start == 0 && f.End >= 10})
	}
	// Gaps include omitted assistant/control records only as sequence numbers.
	// No excluded text, future event, inferred closure, or reason is synthesized.
	for _, e := range h.Events {
		if e.Sequence >= first && e.Sequence <= w.CapturedSequence && e.Type == "user_message" && e.ParentID == "" {
			in.Roots = append(in.Roots, e.Sequence)
		}
	}
	sort.Ints(in.Roots)
	cursor := first
	for _, field := range in.Fields {
		if field.Sequence > cursor {
			in.Omissions = append(in.Omissions, [2]int{cursor, field.Sequence - 1})
		}
		cursor = field.Sequence + 1
	}
	if cursor <= w.CapturedSequence {
		in.Omissions = append(in.Omissions, [2]int{cursor, w.CapturedSequence})
	}
	if len(w.Input.AcceptedContext) > 16 {
		return in, seal, errors.New("compact accepted context bound")
	}
	seenEntities := map[string]bool{}
	for _, raw := range w.Input.AcceptedContext {
		if len(raw) > 4096 || strings.Contains(string(raw), "EVIE_SPIKE_SECRET_DO_NOT_SEND") || validateJSONStructure(string(raw)) != nil {
			return in, seal, errors.New("compact accepted context shape")
		}
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil {
			return in, seal, errors.New("compact accepted context object required")
		}
		for key := range value {
			if key != "entity_id" && key != "aliases" && key != "accepted_claim_id" && key != "meaning" {
				return in, seal, errors.New("compact unsupported accepted context field")
			}
		}
		entityID, ok := value["entity_id"].(string)
		if !ok || entityID == "" || seenEntities[entityID] {
			return in, seal, errors.New("compact accepted entity identity missing/duplicate")
		}
		seenEntities[entityID] = true
		alias := fmt.Sprintf("a%d", len(seal.Entities)+1)
		seal.Entities = append(seal.Entities, compactEntity{alias, entityID, string(raw)})
		delete(value, "entity_id")
		in.Accepted = append(in.Accepted, map[string]any{"ref": alias, "context": value})
	}
	encoded, _ := json.Marshal(in)
	seal.InputSHA256 = digest(encoded)
	return in, seal, nil
}

func experimentRequest(h history, w window, policy, corpusSHA, wire string, prompt, schema []byte, mode, model string, contextTokens, maxTokens, seed int) ([]byte, *compactRecord, error) {
	if !isCompactWire(wire) {
		return requestBody(w.Input, prompt, schema, mode, model, contextTokens, maxTokens, seed), nil, nil
	}
	in, seal, err := sealCompact(h, w, policy, corpusSHA)
	if err != nil {
		return nil, nil, err
	}
	if wire == "compact-v3" {
		schema, err = categorySchema(seal, schema)
		if err != nil {
			return nil, nil, err
		}
		prompt = append(append([]byte(nil), prompt...), schema...)
	}
	body := requestBody(in, prompt, schema, mode, model, contextTokens, maxTokens, seed)
	b, _ := json.Marshal(seal)
	record := &compactRecord{Seal: seal, SealSHA256: digest(b), Request: string(body)}
	if wire == "compact-v3" {
		record.SystemSHA256 = digest(prompt)
		record.SchemaSHA256 = digest(schema)
		record.SchemaDerivationVersion = compactCategoryVersion
	}
	return body, record, nil
}

func compactResponseBound(raw, windowID string) bool {
	if validateJSONStructure(raw) != nil {
		return false
	}
	var response struct {
		WindowID string `json:"window_id"`
	}
	return json.Unmarshal([]byte(raw), &response) == nil && response.WindowID == windowID
}

func compactShape(raw string, windowID string) error {
	if err := validateJSONStructure(raw); err != nil {
		return err
	}
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &root) != nil || len(root) != 2 {
		return errors.New("closed compact root required")
	}
	var id string
	if json.Unmarshal(root["window_id"], &id) != nil || id != windowID {
		return errors.New("response_binding: window identity mismatch")
	}
	var proposals []json.RawMessage
	if json.Unmarshal(root["candidates"], &proposals) != nil || proposals == nil || len(proposals) > 8 {
		return errors.New("compact candidates array required")
	}
	for _, raw := range proposals {
		if _, err := decodeCompact(raw); err != nil {
			return err
		}
	}
	return nil
}
func decodeCompact(raw []byte) (compactCandidate, error) {
	var c compactCandidate
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 13 {
		return c, errors.New("closed compact candidate fields required")
	}
	for _, key := range []string{"subject_type", "subject_name", "subject_entity_ref", "predicate", "object_kind", "object", "polarity", "kind", "temporal", "identity", "effect", "sources", "context"} {
		if fields[key] == nil || string(fields[key]) == "null" {
			return c, fmt.Errorf("missing compact field %s", key)
		}
	}
	for _, key := range []string{"sources", "context"} {
		var refs []map[string]json.RawMessage
		if json.Unmarshal(fields[key], &refs) != nil || refs == nil {
			return c, errors.New("compact reference array required")
		}
		for _, ref := range refs {
			for key, value := range ref {
				if key != "ref" && key != "selector" && key != "start" && key != "end" {
					return c, errors.New("unknown compact reference field")
				}
				if string(value) == "null" {
					return c, errors.New("null compact reference field")
				}
			}
			if value, exists := ref["selector"]; exists {
				var selector string
				if json.Unmarshal(value, &selector) != nil || (selector != "whole" && selector != "date" && selector != "range") {
					return c, errors.New("invalid_selector: present selector must be whole/date/range")
				}
			}
			var alias string
			if json.Unmarshal(ref["ref"], &alias) != nil || alias == "" {
				return c, errors.New("compact source alias required")
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, err
	}
	return c, nil
}

func expandCompact(raw []byte, seal compactSeal) (candidate, error) {
	c, err := decodeCompact(raw)
	if err != nil {
		return candidate{}, err
	}
	out := candidate{meaning: meaning{Predicate: c.Predicate, ObjectKind: c.ObjectKind, Object: c.Object, Polarity: c.Polarity, Kind: c.Kind, Temporal: c.Temporal, Identity: c.Identity, Effect: c.Effect}, Scope: seal.Scope, Sources: []reference{}, Context: []reference{}}
	switch c.SubjectType {
	case "owner", "project":
		if c.SubjectName != "" || c.SubjectEntityRef != "" {
			return out, errors.New("invalid_subject: incompatible owner/project fields")
		}
		out.Subject = c.SubjectType
	case "new_entity":
		if strings.TrimSpace(c.SubjectName) == "" || c.SubjectEntityRef != "" || c.Identity != "unresolved" {
			return out, errors.New("invalid_subject: new entity needs explicit name and unresolved identity")
		}
		found := false
		for _, s := range seal.Sources {
			if strings.Contains(s.Source.Text, c.SubjectName) {
				found = true
			}
		}
		if !found {
			return out, errors.New("invalid_subject: name absent from supplied source text")
		}
		out.Subject = "new:" + c.SubjectName
	case "accepted_entity":
		if c.SubjectName != "" || c.SubjectEntityRef == "" {
			return out, errors.New("invalid_subject: incompatible accepted entity fields")
		}
		for _, e := range seal.Entities {
			if e.Alias == c.SubjectEntityRef {
				out.Subject = e.EntityID
			}
		}
		if out.Subject == "" {
			return out, errors.New("invalid_subject: unknown accepted entity alias")
		}
	default:
		return out, errors.New("invalid_subject: unknown subject type")
	}
	if c.ObjectKind == "entity" {
		if strings.HasPrefix(c.Object, "new:") {
			if c.Identity != "unresolved" || strings.TrimSpace(strings.TrimPrefix(c.Object, "new:")) == "" {
				return out, errors.New("invalid_subject: unresolved object needs identity uncertainty")
			}
		} else {
			out.Object = ""
			for _, e := range seal.Entities {
				if e.Alias == c.Object {
					out.Object = e.EntityID
				}
			}
			if out.Object == "" {
				return out, errors.New("invalid_subject: unknown accepted object alias")
			}
		}
	}
	// Identity is model-authored: validate combinations, never infer or repair it.
	if c.SubjectType != "new_entity" && c.Identity != "resolved" && !(c.Identity == "unresolved" && c.ObjectKind == "entity" && strings.HasPrefix(c.Object, "new:")) {
		return out, errors.New("invalid_subject: incompatible identity")
	}
	for axis, refs := range [][]compactRef{c.Sources, c.Context} {
		for _, ref := range refs {
			var field *source
			for _, s := range seal.Sources {
				if s.Alias == ref.Ref {
					f := s.Source
					field = &f
					break
				}
			}
			if field == nil {
				return out, errors.New("unknown_alias: source alias not offered")
			}
			if (axis == 1) != (field.Ownership == "context") {
				return out, errors.New("reference_category: support/context alias mismatch")
			}
			canonical := reference{EventID: field.EventID, Start: field.Start, End: field.End}
			switch ref.Selector {
			case "", "whole":
				if ref.Start != nil || ref.End != nil {
					return out, errors.New("invalid_selector: whole selector has coordinates")
				}
			case "date":
				if ref.Start != nil || ref.End != nil || field.Authority != "tool_observation" || field.Start != 0 || field.End < 10 {
					return out, errors.New("invalid_selector: date not permitted")
				}
				canonical.Start = 0
				canonical.End = 10
			case "range":
				if ref.Start == nil || ref.End == nil {
					return out, errors.New("invalid_selector: explicit range coordinates required")
				}
				canonical.Start = *ref.Start
				canonical.End = *ref.End
			default:
				return out, errors.New("invalid_selector: unknown projection selector")
			}
			if _, err := resolveReference(canonical, []source{*field}); err != nil {
				return out, fmt.Errorf("invalid_selector: %w", err)
			}
			if field.Authority == "tool_observation" && (canonical.Start != 0 || (canonical.End != 10 && canonical.End != 19)) {
				return out, errors.New("invalid_selector: forbidden clock range")
			}
			if axis == 0 {
				out.Sources = append(out.Sources, canonical)
			} else {
				out.Context = append(out.Context, canonical)
			}
		}
	}
	return out, nil
}

func checkCompactProposals(raw string, result run, in input) run {
	result.Proposals = []proposalResult{}
	result.RawCount = 0
	result.RetainedCount = 0
	record := result.Compact
	sealBytes, _ := json.Marshal(record.Seal)
	bindingOK := compactResponseBound(raw, result.CaseID) && record.Seal.Version == compactVersion && record.Seal.WindowID == result.CaseID && digest(sealBytes) == record.SealSHA256 && digest([]byte(record.Request)) == result.RequestSHA256
	shapeErr := compactShape(raw, result.CaseID)
	if !bindingOK {
		shapeErr = errors.New("response_binding: sealed request/response identity mismatch")
	}
	result.Status = "ok"
	result.Error = ""
	if shapeErr != nil {
		result.Status = "schema_error"
		result.Error = shapeErr.Error()
	}
	var decoded struct {
		Candidates []json.RawMessage `json:"candidates"`
	}
	if json.Unmarshal([]byte(raw), &decoded) != nil {
		return result
	}
	result.RawCount = len(decoded.Candidates)
	for _, rawCandidate := range decoded.Candidates {
		p := proposalResult{WireRaw: rawCandidate}
		var err error
		if bindingOK {
			p.Candidate, err = expandCompact(rawCandidate, record.Seal)
			p.Expanded = err == nil
		} else {
			err = shapeErr
		}
		if err == nil && shapeErr != nil {
			err = shapeErr
		}
		if err == nil {
			err = validateCandidate(p.Candidate, in)
		}
		if err != nil {
			p.Rejection = err.Error()
		} else {
			p.Retained = true
			result.RetainedCount++
		}
		result.Proposals = append(result.Proposals, p)
	}
	return result
}

// Scoring/revalidation regenerates the seal from reviewed sources and verifies
// the actual retained request and raw bytes. Edited aliases or expanded output
// never become fresh evidence or automatic semantic credit.
func verifyCompactRun(r report, recorded run, h history, w window, policy string) (run, error) {
	expectedPrompt, expectedSchema := compactPromptSHA256, compactSchemaSHA256
	if r.WireVersion == "compact-v2" {
		expectedPrompt, expectedSchema = compactV2PromptSHA256, compactV2SchemaSHA256
	}
	if r.WireVersion == "compact-v3" {
		expectedPrompt, expectedSchema = compactV3PromptSHA256, compactV3SchemaSHA256
		if r.Model != "qwen2.5:7b-instruct-q4_K_M" || r.Mode != "schema" || r.ContextTokens != 8192 || r.MaxOutputTokens != 768 || r.Temperature != 0 || r.Repetitions != 1 || !r.StopOnFailure || recorded.Seed != 17 || recorded.Repetition != 1 {
			return recorded, errors.New("compact-v3 report configuration mismatch")
		}
		if r.SchemaDerivationVersion != compactCategoryVersion {
			return recorded, errors.New("compact schema derivation version mismatch")
		}
	}
	if !isCompactWire(r.WireVersion) || r.PromptSHA256 != expectedPrompt || r.SchemaSHA256 != expectedSchema {
		return recorded, errors.New("compact configuration/prompt/schema identity mismatch")
	}
	if recorded.Compact == nil {
		return recorded, errors.New("missing compact sealed request")
	}
	var req struct {
		System string          `json:"system"`
		Format json.RawMessage `json:"format"`
	}
	if json.Unmarshal([]byte(recorded.Compact.Request), &req) != nil {
		return recorded, errors.New("invalid compact recorded request")
	}
	prompt, schema := []byte(req.System), canonicalProposal(req.Format)
	if r.WireVersion == "compact-v3" {
		var err error
		prompt, schema, err = categoryBaseFiles(r.BudgetSHA256)
		if err != nil {
			return recorded, err
		}
	}
	if digest(prompt) != r.PromptSHA256 || digest(schema) != r.SchemaSHA256 {
		return recorded, errors.New("compact prompt/schema identity mismatch")
	}
	body, binding, err := experimentRequest(h, w, policy, r.CorpusSHA256, r.WireVersion, prompt, schema, r.Mode, r.Model, r.ContextTokens, r.MaxOutputTokens, recorded.Seed)
	if err != nil {
		return recorded, err
	}
	if digest(body) != recorded.RequestSHA256 || !reflect.DeepEqual(binding, recorded.Compact) {
		return recorded, errors.New("compact sealed request/raw binding differs")
	}
	if recorded.Raw != "" && digest([]byte(recorded.Raw)) != recorded.RawSHA256 {
		return recorded, errors.New("compact raw output hash differs")
	}
	if recorded.Raw == "" && recorded.RawSHA256 != "" && recorded.RawSHA256 != digest(nil) && recorded.Status != "output_bound" && recorded.Status != "cancelled_or_timeout" {
		return recorded, errors.New("compact absent raw output binding differs")
	}
	if recorded.Status == "ok" || recorded.Status == "schema_error" {
		if recorded.ServerRelease != "finished_response" {
			return recorded, errors.New("incomplete compact response")
		}
		checked := checkCompactProposals(recorded.Raw, recorded, w.Input)
		if checked.Status != recorded.Status || checked.Error != recorded.Error || !sameJSON(checked.Proposals, recorded.Proposals) || checked.RawCount != recorded.RawCount || checked.RetainedCount != recorded.RetainedCount {
			return recorded, errors.New("compact recorded expansion differs")
		}
	}
	switch recorded.Status {
	case "ok", "schema_error":
	case "request_error", "input_bound", "transport_error", "cancelled_or_timeout", "response_bound_or_read_error", "http_error", "invalid_envelope", "output_bound", "truncated_output":
		if len(recorded.Proposals) != 0 || recorded.RetainedCount != 0 || recorded.RawCount != 0 {
			return recorded, errors.New("failed compact response carries proposal data")
		}
	default:
		return recorded, errors.New("unknown compact run status")
	}
	return recorded, nil
}

func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(canonicalProposal(x), canonicalProposal(y))
}
