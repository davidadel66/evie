// Package memoryeval defines the versioned fixture and report contracts shared
// by Semantic Memory evaluation stages. It deliberately contains no model,
// network, capability, or external-effect integration.
package memoryeval

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
)

type Panel string

const (
	PanelSemanticConformance Panel = "semantic_conformance"
	PanelLearnedExtraction   Panel = "learned_extraction"
	PanelRetrievalProvenance Panel = "retrieval_provenance"
	PanelAnswerAbstention    Panel = "answer_abstention"
)

func PanelOrder() []Panel {
	return []Panel{PanelSemanticConformance, PanelLearnedExtraction, PanelRetrievalProvenance, PanelAnswerAbstention}
}

var requiredStage3Coverage = []string{
	"global_scope", "workspace_scope", "project_scope", "session_scope", "same_name_entities",
	"multiple_sources", "correction_error", "correction_changed", "retire_restore", "source_retract_restore",
	"contradictions", "promotion", "temporal_boundaries", "one_hop", "two_hop", "operation_outcomes",
	"scope_revisions", "replay", "idempotence", "scope_isolation", "lifecycle", "provenance",
	"query_equivalence", "restart", "recovery",
}

var canonicalFixtureUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var canonicalSHA256 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Manifest struct {
	ManifestVersion  int           `json:"manifest_version"`
	FixtureVersion   string        `json:"fixture_version"`
	DatasetSHA256    string        `json:"dataset_sha256"`
	Imports          []string      `json:"imports,omitempty"`
	Panels           []Panel       `json:"panels"`
	RequiredCoverage []string      `json:"required_coverage"`
	Cases            []FixtureCase `json:"cases"`
}

type FixtureCase struct {
	ID              string             `json:"id"`
	Description     string             `json:"description"`
	FailureTaxonomy FailureTaxonomy    `json:"failure_taxonomy"`
	Coverage        []string           `json:"coverage"`
	Database        FixtureDatabase    `json:"database"`
	Registry        FixtureRegistry    `json:"registry"`
	Sources         []FixtureSource    `json:"sources"`
	Operations      []FixtureOperation `json:"operations"`
	Expected        FixtureExpected    `json:"expected"`
}

type FixtureDatabase struct {
	Reset             string `json:"reset"`
	ReuseBetweenCases bool   `json:"reuse_between_cases"`
}

type FixtureRegistry struct {
	Scopes     []FixtureScope     `json:"scopes"`
	Workspaces []FixtureWorkspace `json:"workspaces"`
	Projects   []FixtureProject   `json:"projects"`
	Sessions   []FixtureSession   `json:"sessions"`
}

type FixtureScope struct {
	ScopeID  string `json:"scope_id"`
	ScopeKey string `json:"scope_key"`
	Revision int64  `json:"revision"`
}

type FixtureWorkspace struct {
	WorkspaceID       string `json:"workspace_id"`
	DisplayName       string `json:"display_name"`
	CurrentRevisionID string `json:"current_revision_id"`
}

type FixtureProject struct {
	ProjectID   string `json:"project_id"`
	DisplayName string `json:"display_name"`
	Root        string `json:"root"`
}

type FixtureSession struct {
	SessionID           string `json:"session_id"`
	Title               string `json:"title"`
	Status              string `json:"status"`
	ContextScopeKey     string `json:"context_scope_key"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	WorkspaceRevisionID string `json:"workspace_revision_id,omitempty"`
	ProjectID           string `json:"project_id,omitempty"`
}

type FixtureSource struct {
	EventID        string `json:"event_id"`
	SessionID      string `json:"session_id"`
	ScopeKey       string `json:"scope_key"`
	RecordedAt     string `json:"recorded_at"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	Content        string `json:"content"`
}

type FixtureOperation struct {
	Name              string            `json:"name"`
	OperationID       string            `json:"operation_id"`
	SchemaVersion     int               `json:"schema_version"`
	Kind              string            `json:"kind"`
	IdempotencyKey    string            `json:"idempotency_key"`
	Clock             string            `json:"clock"`
	SourceEventIDs    []string          `json:"source_event_ids"`
	GeneratedIDs      map[string]string `json:"generated_ids"`
	Request           json.RawMessage   `json:"request"`
	ExpectedOutcome   string            `json:"expected_outcome"`
	ExpectedRevisions map[string]int64  `json:"expected_revisions"`
}

type FixtureExpected struct {
	Snapshot   *FixtureSnapshot   `json:"snapshot"`
	Queries    []FixtureQuery     `json:"queries"`
	Paths      []FixturePath      `json:"paths"`
	Rejections []FixtureRejection `json:"rejections"`
}

type FixtureSnapshot struct {
	ScopeKeys            []string          `json:"scope_keys,omitempty"`
	ScopeRevisions       map[string]int64  `json:"scope_revisions,omitempty"`
	ScopeHashes          map[string]string `json:"scope_hashes,omitempty"`
	ScopeFrontiers       map[string]string `json:"scope_frontiers,omitempty"`
	PredicateIDs         []string          `json:"predicate_ids,omitempty"`
	EntityIDs            []string          `json:"entity_ids,omitempty"`
	AliasIDs             []string          `json:"alias_ids,omitempty"`
	ClaimIDs             []string          `json:"claim_ids,omitempty"`
	SourceLinkIDs        []string          `json:"source_link_ids,omitempty"`
	GraphLinkIDs         []string          `json:"graph_link_ids,omitempty"`
	ActiveClaimIDs       []string          `json:"active_claim_ids,omitempty"`
	SupersededClaimIDs   []string          `json:"superseded_claim_ids,omitempty"`
	WorkspaceClaimID     string            `json:"workspace_claim_id,omitempty"`
	GlobalClaimID        string            `json:"global_claim_id,omitempty"`
	PromotionOperationID string            `json:"promotion_operation_id,omitempty"`
}

type FixtureQuery struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	SessionID   string   `json:"session_id,omitempty"`
	ScopeKey    string   `json:"scope_key,omitempty"`
	Alias       string   `json:"alias,omitempty"`
	ValidAt     string   `json:"valid_at,omitempty"`
	AsKnownAt   string   `json:"as_known_at,omitempty"`
	ExpectedIDs []string `json:"expected_ids"`
}

type FixtureQueryRequest struct {
	Kind      string `json:"kind"`
	SessionID string `json:"session_id"`
	ScopeKey  string `json:"scope_key,omitempty"`
	Alias     string `json:"alias,omitempty"`
	ValidAt   string `json:"valid_at,omitempty"`
	AsKnownAt string `json:"as_known_at,omitempty"`
}

type FixtureQueryExpected struct {
	ScopeRevisions map[string]int64 `json:"scope_revisions"`
	ObjectIDs      []string         `json:"object_ids"`
}

type FixturePath struct {
	ID                   string   `json:"id"`
	Depth                int      `json:"depth"`
	ExpectedObjectIDs    []string `json:"expected_object_ids"`
	ExpectedGraphLinkIDs []string `json:"expected_graph_link_ids,omitempty"`
}

type FixturePathRequest struct {
	SessionID string `json:"session_id"`
	ScopeKey  string `json:"scope_key"`
	StartKind string `json:"start_kind"`
	StartID   string `json:"start_id"`
	Depth     int    `json:"depth"`
}

type FixturePathExpected struct {
	ObjectIDs    []string `json:"object_ids"`
	GraphLinkIDs []string `json:"graph_link_ids"`
}

type FixtureRejection struct {
	ID                 string           `json:"id"`
	Kind               string           `json:"kind,omitempty"`
	Request            json.RawMessage  `json:"request,omitempty"`
	ErrorCode          string           `json:"error_code"`
	UnchangedRevisions map[string]int64 `json:"unchanged_revisions"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode evaluation manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	computed, err := manifest.ContentSHA256()
	if err != nil {
		return Manifest{}, err
	}
	if manifest.DatasetSHA256 != computed {
		return Manifest{}, fmt.Errorf("evaluation dataset sha256 = %q, want %q", manifest.DatasetSHA256, computed)
	}
	return manifest, nil
}

// ContentSHA256 identifies the complete manifest except for its own digest.
// Imported frozen operation fixtures are independently protected by their
// encoding-suite hashes.
func (m Manifest) ContentSHA256() (string, error) {
	m.DatasetSHA256 = ""
	encoded, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest), nil
}

func (m Manifest) Validate() error {
	if m.ManifestVersion != 1 {
		return fmt.Errorf("unsupported evaluation manifest version %d", m.ManifestVersion)
	}
	if strings.TrimSpace(m.FixtureVersion) == "" || !strings.HasPrefix(m.DatasetSHA256, "sha256:") {
		return errors.New("fixture version and dataset sha256 are required")
	}
	if !slices.Equal(m.Panels, PanelOrder()) {
		return errors.New("evaluation panels are missing, reordered, or combined")
	}
	required := make(map[string]bool, len(requiredStage3Coverage))
	for _, coverage := range m.RequiredCoverage {
		required[coverage] = true
	}
	for _, coverage := range requiredStage3Coverage {
		if !required[coverage] {
			return fmt.Errorf("manifest omits required Stage 3 coverage %q", coverage)
		}
	}
	if len(m.Cases) == 0 {
		return errors.New("evaluation manifest has no cases")
	}
	covered := make(map[string]bool)
	seenCases := make(map[string]bool)
	seenOperations := make(map[string]bool)
	for _, fixtureCase := range m.Cases {
		if fixtureCase.ID == "" || seenCases[fixtureCase.ID] || fixtureCase.Description == "" || !validFailureTaxonomy(fixtureCase.FailureTaxonomy) {
			return fmt.Errorf("invalid or duplicate fixture case %q", fixtureCase.ID)
		}
		seenCases[fixtureCase.ID] = true
		if fixtureCase.Database.Reset != "new_empty_database" || fixtureCase.Database.ReuseBetweenCases || len(fixtureCase.Registry.Scopes) == 0 || len(fixtureCase.Registry.Sessions) == 0 {
			return fmt.Errorf("fixture case %q must define an isolated executable registry", fixtureCase.ID)
		}
		for _, coverage := range fixtureCase.Coverage {
			if !fixtureHasCoverageEvidence(fixtureCase, coverage) {
				return fmt.Errorf("fixture case %q claims coverage %q without typed evidence", fixtureCase.ID, coverage)
			}
			covered[coverage] = true
		}
		if len(fixtureCase.Sources) == 0 || len(fixtureCase.Operations) == 0 {
			return fmt.Errorf("fixture case %q must fix sources and operations", fixtureCase.ID)
		}
		sources := make(map[string]FixtureSource, len(fixtureCase.Sources))
		for _, source := range fixtureCase.Sources {
			if !canonicalFixtureUUID.MatchString(source.EventID) || !canonicalFixtureUUID.MatchString(source.SessionID) || source.ScopeKey == "" || source.RecordedAt == "" || len(source.EvidenceSHA256) != len("sha256:")+64 || source.Content == "" {
				return fmt.Errorf("fixture case %q has an incomplete source", fixtureCase.ID)
			}
			if _, err := time.Parse(time.RFC3339Nano, source.RecordedAt); err != nil {
				return fmt.Errorf("fixture case %q source time: %w", fixtureCase.ID, err)
			}
			digest := sha256.Sum256([]byte(source.Content))
			if source.EvidenceSHA256 != fmt.Sprintf("sha256:%x", digest) {
				return fmt.Errorf("fixture case %q source %q evidence digest does not match content", fixtureCase.ID, source.EventID)
			}
			if _, duplicate := sources[source.EventID]; duplicate {
				return fmt.Errorf("fixture case %q has duplicate source event %q", fixtureCase.ID, source.EventID)
			}
			sources[source.EventID] = source
		}
		for _, operation := range fixtureCase.Operations {
			if operation.Name == "" || !canonicalFixtureUUID.MatchString(operation.OperationID) || seenOperations[operation.OperationID] || operation.SchemaVersion < 1 || operation.SchemaVersion > 5 || operation.Kind == "" || !strings.HasPrefix(operation.IdempotencyKey, "idem:v1:") || !canonicalFixtureUUID.MatchString(strings.TrimPrefix(operation.IdempotencyKey, "idem:v1:")) || len(operation.SourceEventIDs) != 1 || len(operation.ExpectedRevisions) == 0 || len(operation.Request) == 0 {
				return fmt.Errorf("fixture case %q has an incomplete or duplicate operation %q", fixtureCase.ID, operation.OperationID)
			}
			seenOperations[operation.OperationID] = true
			if !fixtureOperationSchemaAllowed(operation) {
				return fmt.Errorf("fixture case %q operation %q schema version %d does not match kind %q", fixtureCase.ID, operation.OperationID, operation.SchemaVersion, operation.Kind)
			}
			if _, err := time.Parse(time.RFC3339Nano, operation.Clock); err != nil {
				return fmt.Errorf("fixture case %q operation clock: %w", fixtureCase.ID, err)
			}
			for _, eventID := range operation.SourceEventIDs {
				if !canonicalFixtureUUID.MatchString(eventID) {
					return fmt.Errorf("fixture case %q operation %q has invalid source event ID", fixtureCase.ID, operation.OperationID)
				}
				if _, ok := sources[eventID]; !ok {
					return fmt.Errorf("fixture case %q operation %q references unknown source event %q", fixtureCase.ID, operation.OperationID, eventID)
				}
			}
			requestSessionID, useSessionScope, err := validateFixtureOperationRequest(operation.Kind, operation.Request)
			if err != nil {
				return fmt.Errorf("fixture case %q operation %q request: %w", fixtureCase.ID, operation.OperationID, err)
			}
			if requestSessionID != sources[operation.SourceEventIDs[0]].SessionID {
				return fmt.Errorf("fixture case %q operation %q request/source session mismatch", fixtureCase.ID, operation.OperationID)
			}
			var session *FixtureSession
			for index := range fixtureCase.Registry.Sessions {
				if fixtureCase.Registry.Sessions[index].SessionID == requestSessionID {
					session = &fixtureCase.Registry.Sessions[index]
					break
				}
			}
			if session == nil {
				return fmt.Errorf("fixture case %q operation %q references unknown session", fixtureCase.ID, operation.OperationID)
			}
			expectedSourceScope := session.ContextScopeKey
			if operation.Kind == "remember_entity_claim" && useSessionScope {
				expectedSourceScope = "session:" + requestSessionID
			}
			if sources[operation.SourceEventIDs[0]].ScopeKey != expectedSourceScope {
				return fmt.Errorf("fixture case %q operation %q source scope %q does not match request scope %q", fixtureCase.ID, operation.OperationID, sources[operation.SourceEventIDs[0]].ScopeKey, expectedSourceScope)
			}
			for name, generatedID := range operation.GeneratedIDs {
				if name == "" || !canonicalFixtureUUID.MatchString(generatedID) {
					return fmt.Errorf("fixture case %q operation %q has invalid generated ID %q", fixtureCase.ID, operation.OperationID, name)
				}
			}
			if operation.ExpectedOutcome != "accepted" && operation.ExpectedOutcome != "rejected" {
				return fmt.Errorf("fixture case %q has unknown operation outcome %q", fixtureCase.ID, operation.ExpectedOutcome)
			}
		}
		if fixtureCase.Expected.Snapshot == nil {
			return fmt.Errorf("fixture case %q omits expected snapshot", fixtureCase.ID)
		}
		snapshot := fixtureCase.Expected.Snapshot
		if len(snapshot.ScopeKeys) == 0 || len(snapshot.ScopeRevisions) == 0 || len(snapshot.ScopeHashes) == 0 || len(snapshot.ScopeFrontiers) == 0 || snapshot.PredicateIDs == nil || snapshot.EntityIDs == nil || snapshot.AliasIDs == nil || snapshot.ClaimIDs == nil || snapshot.SourceLinkIDs == nil || snapshot.GraphLinkIDs == nil || snapshot.ActiveClaimIDs == nil || snapshot.SupersededClaimIDs == nil {
			return fmt.Errorf("fixture case %q snapshot does not close every canonical projection set", fixtureCase.ID)
		}
		observedSnapshotIDs := make(map[string]struct{})
		for _, values := range [][]string{snapshot.PredicateIDs, snapshot.EntityIDs, snapshot.AliasIDs, snapshot.ClaimIDs, snapshot.SourceLinkIDs, snapshot.GraphLinkIDs, snapshot.ActiveClaimIDs, snapshot.SupersededClaimIDs} {
			for _, value := range values {
				observedSnapshotIDs[value] = struct{}{}
			}
		}
		for _, value := range []string{snapshot.WorkspaceClaimID, snapshot.GlobalClaimID, snapshot.PromotionOperationID} {
			if value != "" {
				observedSnapshotIDs[value] = struct{}{}
			}
		}
		for _, operation := range fixtureCase.Operations {
			if operation.ExpectedOutcome != "accepted" {
				continue
			}
			for name, generatedID := range operation.GeneratedIDs {
				if _, observed := observedSnapshotIDs[generatedID]; !observed {
					return fmt.Errorf("fixture case %q operation %q generated ID %q (%s) is not asserted by its snapshot", fixtureCase.ID, operation.OperationID, name, generatedID)
				}
			}
		}
		for _, path := range fixtureCase.Expected.Paths {
			if path.ID == "" || (path.Depth != 1 && path.Depth != 2) || len(path.ExpectedObjectIDs) == 0 {
				return fmt.Errorf("fixture case %q has an invalid expected path", fixtureCase.ID)
			}
		}
		for _, rejection := range fixtureCase.Expected.Rejections {
			if rejection.ID == "" || rejection.Kind == "" || len(rejection.Request) == 0 || rejection.ErrorCode == "" || len(rejection.UnchangedRevisions) == 0 {
				return fmt.Errorf("fixture case %q has an incomplete expected rejection", fixtureCase.ID)
			}
			if err := validateFixtureRejectionRequest(rejection.Kind, rejection.Request); err != nil {
				return fmt.Errorf("fixture case %q rejection %q request: %w", fixtureCase.ID, rejection.ID, err)
			}
		}
	}
	for _, coverage := range requiredStage3Coverage {
		if !covered[coverage] {
			return fmt.Errorf("fixture cases do not cover %q", coverage)
		}
	}
	return nil
}

func fixtureOperationSchemaAllowed(operation FixtureOperation) bool {
	switch operation.Kind {
	case "remember_literal_claim", "remember_entity_claim":
		return operation.SchemaVersion == 1
	case "correct_claim":
		return operation.SchemaVersion == 2
	case "retract_source", "restore_source":
		return operation.SchemaVersion == 3
	case "promote_claim":
		return operation.SchemaVersion == 4
	case "create_graph_link":
		return operation.SchemaVersion == 5
	case "retire_memory", "restore_memory":
		var request struct {
			ObjectKind string `json:"object_kind"`
		}
		if json.Unmarshal(operation.Request, &request) != nil {
			return false
		}
		switch request.ObjectKind {
		case "graph_link":
			return operation.SchemaVersion == 5
		case "entity":
			// Entity lifecycle is v3 when local and v5 when the prepared
			// transition set cascades through graph links.
			return operation.SchemaVersion == 3 || operation.SchemaVersion == 5
		case "alias":
			return operation.SchemaVersion == 3
		default:
			return operation.SchemaVersion == 3
		}
	default:
		return false
	}
}

func validateFixtureOperationRequest(kind string, raw json.RawMessage) (string, bool, error) {
	allowed := []string{"session_id"}
	required := []string{"session_id"}
	switch kind {
	case "remember_literal_claim":
		allowed = append(allowed, "predicate", "predicate_label", "predicate_cardinality", "literal", "valid_from", "valid_to", "force_prior_revision")
		required = append(required, "predicate", "predicate_label", "predicate_cardinality", "literal")
	case "remember_entity_claim":
		allowed = append(allowed, "predicate", "predicate_label", "predicate_cardinality", "subject", "object", "use_session_scope")
		required = append(required, "predicate", "predicate_label", "predicate_cardinality", "subject", "object")
	case "correct_claim":
		allowed = append(allowed, "old_claim_id", "mode", "replacement_literal", "effective_time")
		required = append(required, "old_claim_id", "mode", "replacement_literal")
	case "retract_source", "restore_source", "retire_memory", "restore_memory":
		allowed = append(allowed, "action", "object_kind", "object_id")
		required = append(required, "action", "object_kind", "object_id")
	case "promote_claim":
		allowed = append(allowed, "source_claim_id", "destination_scope_key")
		required = append(required, "source_claim_id", "destination_scope_key")
	case "create_graph_link":
		allowed = append(allowed, "relation", "source_kind", "source_id", "target_kind", "target_id")
		required = append(required, "relation", "source_kind", "source_id", "target_kind", "target_id")
	default:
		return "", false, fmt.Errorf("unknown operation kind %q", kind)
	}
	fields, err := decodeClosedFixtureObject(raw, allowed, required)
	if err != nil {
		return "", false, err
	}
	var sessionID string
	if err := json.Unmarshal(fields["session_id"], &sessionID); err != nil || !canonicalFixtureUUID.MatchString(sessionID) {
		return "", false, errors.New("session_id must be a canonical UUID")
	}
	if literal, ok := fields["literal"]; ok {
		if err := validateFixtureLiteral(literal); err != nil {
			return "", false, fmt.Errorf("literal: %w", err)
		}
	}
	if literal, ok := fields["replacement_literal"]; ok {
		if err := validateFixtureLiteral(literal); err != nil {
			return "", false, fmt.Errorf("replacement_literal: %w", err)
		}
	}
	for _, name := range []string{"predicate", "predicate_label", "predicate_cardinality", "valid_from", "valid_to", "old_claim_id", "mode", "effective_time", "action", "object_kind", "object_id", "source_claim_id", "destination_scope_key", "relation", "source_kind", "source_id", "target_kind", "target_id"} {
		if value, ok := fields[name]; ok {
			var decoded string
			if json.Unmarshal(value, &decoded) != nil || decoded == "" {
				return "", false, fmt.Errorf("%s must be a nonempty string", name)
			}
		}
	}
	for _, name := range []string{"old_claim_id", "object_id", "source_claim_id", "source_id", "target_id"} {
		if value, ok := fields[name]; ok {
			var id string
			_ = json.Unmarshal(value, &id)
			if !canonicalFixtureUUID.MatchString(id) {
				return "", false, fmt.Errorf("%s must be a canonical UUID", name)
			}
		}
	}
	for _, name := range []string{"valid_from", "valid_to", "effective_time"} {
		if value, ok := fields[name]; ok {
			var timestamp string
			_ = json.Unmarshal(value, &timestamp)
			if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
				return "", false, fmt.Errorf("%s must be an RFC3339Nano timestamp", name)
			}
		}
	}
	stringValue := func(name string) string {
		var value string
		_ = json.Unmarshal(fields[name], &value)
		return value
	}
	switch kind {
	case "remember_literal_claim", "remember_entity_claim":
		if !slices.Contains([]string{"one", "many"}, stringValue("predicate_cardinality")) {
			return "", false, errors.New("predicate_cardinality must be one or many")
		}
	case "correct_claim":
		mode := stringValue("mode")
		if !slices.Contains([]string{"error", "changed"}, mode) {
			return "", false, errors.New("mode must be error or changed")
		}
		_, hasEffectiveTime := fields["effective_time"]
		if (mode == "changed") != hasEffectiveTime {
			return "", false, errors.New("changed mode requires effective_time and error mode forbids it")
		}
	case "retract_source", "restore_source", "retire_memory", "restore_memory":
		expectedActions := map[string]string{"retract_source": "retract_source", "restore_source": "restore_source", "retire_memory": "retire", "restore_memory": "restore"}
		if stringValue("action") != expectedActions[kind] {
			return "", false, fmt.Errorf("action does not match operation kind %q", kind)
		}
		if !slices.Contains([]string{"entity", "alias", "claim", "source_link", "graph_link"}, stringValue("object_kind")) {
			return "", false, errors.New("object_kind is invalid")
		}
	case "create_graph_link":
		if !slices.Contains([]string{"derivation", "generalization", "contradiction"}, stringValue("relation")) || !slices.Contains([]string{"entity", "claim"}, stringValue("source_kind")) || !slices.Contains([]string{"entity", "claim"}, stringValue("target_kind")) {
			return "", false, errors.New("graph relation or endpoint kind is invalid")
		}
	}
	if value, ok := fields["force_prior_revision"]; ok {
		var revision int64
		if json.Unmarshal(value, &revision) != nil || revision < 0 {
			return "", false, errors.New("force_prior_revision must be a nonnegative integer")
		}
	}
	for _, name := range []string{"subject", "object"} {
		if selector, ok := fields[name]; ok {
			if err := validateFixtureEntitySelector(selector); err != nil {
				return "", false, fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	useSessionScope := false
	if value, ok := fields["use_session_scope"]; ok {
		if err := json.Unmarshal(value, &useSessionScope); err != nil {
			return "", false, errors.New("use_session_scope must be boolean")
		}
	}
	return sessionID, useSessionScope, nil
}

func validateFixtureRejectionRequest(kind string, raw json.RawMessage) error {
	switch kind {
	case "inspect_claims":
		fields, err := decodeClosedFixtureObject(raw, []string{"session_id", "scope_key"}, []string{"session_id", "scope_key"})
		if err != nil {
			return err
		}
		for _, name := range []string{"session_id", "scope_key"} {
			var value string
			if json.Unmarshal(fields[name], &value) != nil || value == "" {
				return fmt.Errorf("%s must be a nonempty string", name)
			}
		}
		return nil
	case "remember_literal_claim":
		fields, err := decodeClosedFixtureObject(raw, []string{"operation_name"}, []string{"operation_name"})
		if err != nil {
			return err
		}
		var operationName string
		if json.Unmarshal(fields["operation_name"], &operationName) != nil || operationName == "" {
			return errors.New("operation_name must be a nonempty string")
		}
		return nil
	case "replay_operation":
		fields, err := decodeClosedFixtureObject(raw, []string{"schema_version"}, []string{"schema_version"})
		if err != nil {
			return err
		}
		var schemaVersion int
		if json.Unmarshal(fields["schema_version"], &schemaVersion) != nil || schemaVersion < 1 {
			return errors.New("schema_version must be a positive integer")
		}
		return nil
	default:
		return fmt.Errorf("unknown rejection kind %q", kind)
	}
}

func decodeClosedFixtureObject(raw json.RawMessage, allowed, required []string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("must be a JSON object")
	}
	for name := range fields {
		if !slices.Contains(allowed, name) {
			return nil, fmt.Errorf("unknown field %q", name)
		}
	}
	for _, name := range required {
		if value, ok := fields[name]; !ok || len(value) == 0 || string(value) == "null" {
			return nil, fmt.Errorf("missing required field %q", name)
		}
	}
	return fields, nil
}

func validateFixtureLiteral(raw json.RawMessage) error {
	fields, err := decodeClosedFixtureObject(raw, []string{"kind", "value"}, []string{"kind", "value"})
	if err != nil {
		return err
	}
	var kind, value string
	if json.Unmarshal(fields["kind"], &kind) != nil || json.Unmarshal(fields["value"], &value) != nil || !slices.Contains([]string{"text", "integer", "decimal", "boolean", "date", "datetime"}, kind) {
		return errors.New("kind/value must be a supported string literal")
	}
	return nil
}

func validateFixtureEntitySelector(raw json.RawMessage) error {
	fields, err := decodeClosedFixtureObject(raw, []string{"entity_id", "create", "canonical_name", "entity_type", "alias"}, nil)
	if err != nil {
		return err
	}
	if _, existing := fields["entity_id"]; existing {
		if len(fields) != 1 {
			return errors.New("existing selector may contain only entity_id")
		}
		var id string
		if json.Unmarshal(fields["entity_id"], &id) != nil || !canonicalFixtureUUID.MatchString(id) {
			return errors.New("entity_id must be a canonical UUID")
		}
		return nil
	}
	for _, required := range []string{"create", "canonical_name", "entity_type", "alias"} {
		if _, ok := fields[required]; !ok {
			return fmt.Errorf("missing required field %q", required)
		}
	}
	var create bool
	if json.Unmarshal(fields["create"], &create) != nil || !create {
		return errors.New("create must be true")
	}
	return nil
}

func fixtureHasCoverageEvidence(fixtureCase FixtureCase, coverage string) bool {
	hasOperation := func(kinds ...string) bool {
		for _, operation := range fixtureCase.Operations {
			if slices.Contains(kinds, operation.Kind) {
				return true
			}
		}
		return false
	}
	hasRequest := func(fragment string) bool {
		for _, operation := range fixtureCase.Operations {
			var compact bytes.Buffer
			if json.Compact(&compact, operation.Request) == nil && strings.Contains(compact.String(), fragment) {
				return true
			}
		}
		return false
	}
	hasQuery := func(kind string) bool {
		for _, query := range fixtureCase.Expected.Queries {
			if query.Kind == kind {
				return true
			}
		}
		return false
	}
	hasRejection := func(code string) bool {
		for _, rejection := range fixtureCase.Expected.Rejections {
			if rejection.ErrorCode == code {
				return true
			}
		}
		return false
	}
	hasScope := func(prefix string) bool {
		for _, source := range fixtureCase.Sources {
			if source.ScopeKey == prefix || strings.HasPrefix(source.ScopeKey, prefix+":") {
				return len(fixtureCase.Operations) > 0
			}
		}
		return false
	}
	switch coverage {
	case "global_scope":
		for _, scope := range fixtureCase.Registry.Scopes {
			if scope.ScopeKey == "global" {
				return len(fixtureCase.Operations) > 0
			}
		}
		return false
	case "workspace_scope":
		return hasScope("workspace")
	case "project_scope":
		return hasScope("project")
	case "session_scope":
		return hasScope("session") && hasRequest(`"use_session_scope":true`)
	case "same_name_entities":
		return hasRequest(`"alias":"Alex"`)
	case "multiple_sources":
		return len(fixtureCase.Sources) > 1
	case "correction_error":
		return hasOperation("correct_claim") && hasRequest(`"mode":"error"`)
	case "correction_changed":
		return hasOperation("correct_claim") && hasRequest(`"mode":"changed"`)
	case "retire_restore":
		return hasOperation("retire_memory") && hasOperation("restore_memory")
	case "source_retract_restore":
		return hasOperation("retract_source") && hasOperation("restore_source")
	case "contradictions":
		return hasRequest(`"relation":"contradiction"`)
	case "promotion":
		return hasOperation("promote_claim")
	case "temporal_boundaries":
		return hasQuery("valid_at") && len(fixtureCase.Expected.Queries) >= 5
	case "one_hop":
		for _, path := range fixtureCase.Expected.Paths {
			if path.Depth == 1 {
				return true
			}
		}
		return false
	case "two_hop":
		for _, path := range fixtureCase.Expected.Paths {
			if path.Depth == 2 {
				return true
			}
		}
		return false
	case "operation_outcomes", "scope_revisions":
		return len(fixtureCase.Operations) > 0
	case "replay":
		return hasQuery("replay")
	case "idempotence":
		return len(fixtureCase.Operations) > 0
	case "scope_isolation":
		return hasRejection("scope_isolation") || hasScope("project") || hasScope("session")
	case "lifecycle":
		return hasOperation("retire_memory", "restore_memory", "retract_source", "restore_source")
	case "provenance":
		return len(fixtureCase.Sources) > 0 && len(fixtureCase.Operations) > 0
	case "query_equivalence":
		return len(fixtureCase.Expected.Queries) > 0 || len(fixtureCase.Expected.Paths) > 0
	case "restart":
		return hasQuery("replay")
	case "recovery":
		return hasQuery("replay") && hasRejection("unsupported_operation_schema")
	default:
		return false
	}
}

func validFailureTaxonomy(value FailureTaxonomy) bool {
	switch value {
	case FailureOperationOutcome, FailureRevision, FailureReplay, FailureIdempotence, FailureScopeLeakage, FailureLifecycle, FailureTemporal, FailureProvenance, FailureQuery, FailureRestart, FailureRecovery:
		return true
	default:
		return false
	}
}

type Status string

const (
	StatusPassed       Status = "passed"
	StatusFailed       Status = "failed"
	StatusError        Status = "error"
	StatusSkipped      Status = "skipped"
	StatusNotPopulated Status = "not_populated"
)

type FailureTaxonomy string

const (
	FailureOperationOutcome FailureTaxonomy = "operation_outcome"
	FailureRevision         FailureTaxonomy = "scope_revision"
	FailureReplay           FailureTaxonomy = "replay_divergence"
	FailureIdempotence      FailureTaxonomy = "idempotence"
	FailureScopeLeakage     FailureTaxonomy = "scope_leakage"
	FailureLifecycle        FailureTaxonomy = "stale_lifecycle_state"
	FailureTemporal         FailureTaxonomy = "temporal_boundary_error"
	FailureProvenance       FailureTaxonomy = "missing_or_wrong_provenance"
	FailureQuery            FailureTaxonomy = "query_equivalence"
	FailureRestart          FailureTaxonomy = "restart"
	FailureRecovery         FailureTaxonomy = "recovery"
)

type Report struct {
	ReportSchemaVersion int                          `json:"report_schema_version"`
	Run                 RunIdentity                  `json:"run"`
	Fixture             FixtureIdentity              `json:"fixture"`
	Environment         Environment                  `json:"environment"`
	Components          map[string]ComponentIdentity `json:"components"`
	Cardinality         Cardinality                  `json:"fixture_cardinality"`
	BaselineRunID       string                       `json:"baseline_run_id"`
	Panels              []PanelResult                `json:"panels"`
	Cases               []CaseResult                 `json:"cases"`
	Metrics             []Metric                     `json:"metrics"`
	FailureTaxonomy     []FailureCount               `json:"failure_taxonomy"`
	Summary             Summary                      `json:"summary"`
}

type ComponentIdentity struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Revision   string `json:"revision,omitempty"`
	ConfigHash string `json:"config_hash,omitempty"`
}

type RunIdentity struct {
	ID        string    `json:"id"`
	Commit    string    `json:"commit"`
	StartedAt time.Time `json:"started_at"`
}

type FixtureIdentity struct {
	ManifestVersion int    `json:"manifest_version"`
	FixtureVersion  string `json:"fixture_version"`
	DatasetSHA256   string `json:"dataset_sha256"`
}

type Environment struct {
	Hardware      string   `json:"hardware"`
	OS            string   `json:"os"`
	GoVersion     string   `json:"go_version"`
	SQLiteVersion string   `json:"sqlite_version"`
	JournalMode   string   `json:"journal_mode"`
	JournalSetup  string   `json:"journal_settings"`
	Repetitions   int      `json:"repetitions"`
	Conditions    []string `json:"conditions"`
}

func RuntimeEnvironment(hardware, sqliteVersion, journalMode, journalSettings string, repetitions int, conditions []string) Environment {
	return Environment{Hardware: hardware, OS: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), SQLiteVersion: sqliteVersion, JournalMode: journalMode, JournalSetup: journalSettings, Repetitions: repetitions, Conditions: append([]string(nil), conditions...)}
}

type Cardinality struct {
	Cases      int `json:"cases"`
	Sources    int `json:"sources"`
	Operations int `json:"operations"`
	Entities   int `json:"entities"`
	Claims     int `json:"claims"`
	Links      int `json:"links"`
}

type PanelResult struct {
	Panel     Panel  `json:"panel"`
	Status    Status `json:"status"`
	CaseCount int    `json:"case_count"`
	Passed    int    `json:"passed"`
	Failed    int    `json:"failed"`
	Errors    int    `json:"errors"`
	Skipped   int    `json:"skipped"`
}

func EmptyPanels() []PanelResult {
	panels := make([]PanelResult, 0, len(PanelOrder()))
	for _, panel := range PanelOrder() {
		panels = append(panels, PanelResult{Panel: panel, Status: StatusNotPopulated})
	}
	return panels
}

type CaseResult struct {
	ID         string         `json:"id"`
	Panel      Panel          `json:"panel"`
	Gate       bool           `json:"release_gate"`
	Status     Status         `json:"status"`
	DurationNS int64          `json:"duration_ns"`
	Details    map[string]any `json:"details,omitempty"`
	Failure    *Failure       `json:"failure,omitempty"`
}

type Failure struct {
	Taxonomy FailureTaxonomy `json:"taxonomy"`
	Message  string          `json:"message"`
}

type FailureCount struct {
	Taxonomy FailureTaxonomy `json:"taxonomy"`
	Count    int             `json:"count"`
}

type MetricValues struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

type Metric struct {
	Name        string        `json:"name"`
	Unit        string        `json:"unit"`
	Condition   string        `json:"condition"`
	Repetitions int           `json:"repetitions"`
	Current     MetricValues  `json:"current"`
	Baseline    *MetricValues `json:"baseline,omitempty"`
	Delta       *MetricValues `json:"delta,omitempty"`
	Threshold   *float64      `json:"threshold,omitempty"`
}

func SummarizeMetric(name, unit, condition string, observations []int64) Metric {
	values := append([]int64(nil), observations...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	result := Metric{Name: name, Unit: unit, Condition: condition, Repetitions: len(values)}
	if len(values) == 0 {
		return result
	}
	percentile := func(p float64) float64 {
		index := int(float64(len(values))*p+0.999999999) - 1
		if index < 0 {
			index = 0
		}
		return float64(values[index])
	}
	result.Current = MetricValues{P50: percentile(.50), P95: percentile(.95), Max: float64(values[len(values)-1])}
	return result
}

// ApplyBaseline pairs metrics by stable name, unit, and condition. It never
// turns a performance observation into a release threshold.
func (r *Report) ApplyBaseline(baseline Report) {
	r.BaselineRunID = baseline.Run.ID
	type metricIdentity struct {
		Name      string
		Unit      string
		Condition string
	}
	identity := func(metric Metric) metricIdentity {
		return metricIdentity{Name: metric.Name, Unit: metric.Unit, Condition: metric.Condition}
	}
	byIdentity := make(map[metricIdentity]Metric, len(baseline.Metrics))
	for _, metric := range baseline.Metrics {
		byIdentity[identity(metric)] = metric
	}
	for index := range r.Metrics {
		metric := &r.Metrics[index]
		prior, ok := byIdentity[identity(*metric)]
		if !ok {
			continue
		}
		baselineValues := prior.Current
		metric.Baseline = &baselineValues
		delta := MetricValues{P50: metric.Current.P50 - baselineValues.P50, P95: metric.Current.P95 - baselineValues.P95, Max: metric.Current.Max - baselineValues.Max}
		metric.Delta = &delta
	}
}

type Summary struct {
	Passed  int  `json:"passed"`
	Failed  int  `json:"failed"`
	Errors  int  `json:"errors"`
	Skipped int  `json:"skipped"`
	Release bool `json:"release_gate_passed"`
}

func (r *Report) Summarize() {
	if len(r.Panels) == 0 {
		r.Panels = EmptyPanels()
	}
	byPanel := make(map[Panel][]CaseResult)
	taxonomy := make(map[FailureTaxonomy]int)
	r.Summary = Summary{Release: true}
	for _, result := range r.Cases {
		byPanel[result.Panel] = append(byPanel[result.Panel], result)
		switch result.Status {
		case StatusPassed:
			r.Summary.Passed++
		case StatusFailed:
			r.Summary.Failed++
		case StatusError:
			r.Summary.Errors++
		case StatusSkipped:
			r.Summary.Skipped++
		}
		if result.Gate && result.Status != StatusPassed {
			r.Summary.Release = false
		}
		if result.Failure != nil {
			taxonomy[result.Failure.Taxonomy]++
		}
	}
	for index := range r.Panels {
		panel := &r.Panels[index]
		results := byPanel[panel.Panel]
		panel.CaseCount, panel.Passed, panel.Failed, panel.Errors, panel.Skipped = len(results), 0, 0, 0, 0
		if len(results) == 0 {
			panel.Status = StatusNotPopulated
			continue
		}
		panel.Status = StatusPassed
		for _, result := range results {
			switch result.Status {
			case StatusPassed:
				panel.Passed++
			case StatusFailed:
				panel.Failed++
			case StatusError:
				panel.Errors++
			case StatusSkipped:
				panel.Skipped++
			}
		}
		if panel.Errors > 0 {
			panel.Status = StatusError
		} else if panel.Failed > 0 {
			panel.Status = StatusFailed
		}
	}
	if r.FailureTaxonomy == nil {
		r.FailureTaxonomy = make([]FailureCount, 0)
	} else {
		r.FailureTaxonomy = r.FailureTaxonomy[:0]
	}
	for taxonomy, count := range taxonomy {
		r.FailureTaxonomy = append(r.FailureTaxonomy, FailureCount{Taxonomy: taxonomy, Count: count})
	}
	sort.Slice(r.FailureTaxonomy, func(i, j int) bool { return r.FailureTaxonomy[i].Taxonomy < r.FailureTaxonomy[j].Taxonomy })
}

func (r Report) Passed() bool { return r.Summary.Release }

// Validate enforces the closed machine-report contract before any report is
// emitted. The JSON schema mirrors these checks for non-Go consumers.
func (r Report) Validate() error {
	if r.ReportSchemaVersion != 1 || r.Run.ID == "" || r.Run.Commit == "" || r.Run.StartedAt.IsZero() {
		return errors.New("report has an invalid schema version or run identity")
	}
	if r.Fixture.ManifestVersion < 1 || r.Fixture.FixtureVersion == "" || !canonicalSHA256.MatchString(r.Fixture.DatasetSHA256) {
		return errors.New("report has an invalid fixture identity")
	}
	if r.Environment.Hardware == "" || r.Environment.OS == "" || r.Environment.GoVersion == "" || r.Environment.SQLiteVersion == "" || r.Environment.JournalMode == "" || r.Environment.JournalSetup == "" || r.Environment.Repetitions < 1 || len(r.Environment.Conditions) == 0 {
		return errors.New("report has an incomplete execution environment")
	}
	if len(r.Components) == 0 {
		return errors.New("report has no component identities")
	}
	for key, component := range r.Components {
		if key == "" || component.Name == "" || component.Version == "" {
			return fmt.Errorf("report component %q has an incomplete identity", key)
		}
	}
	if r.Cardinality.Cases < 0 || r.Cardinality.Sources < 0 || r.Cardinality.Operations < 0 || r.Cardinality.Entities < 0 || r.Cardinality.Claims < 0 || r.Cardinality.Links < 0 {
		return errors.New("report has a negative fixture cardinality")
	}
	panels := PanelOrder()
	if len(r.Panels) != len(panels) {
		return fmt.Errorf("report has %d panels, want %d", len(r.Panels), len(panels))
	}
	for index, panel := range r.Panels {
		if panel.Panel != panels[index] || !validStatus(panel.Status) || panel.CaseCount < 0 || panel.Passed < 0 || panel.Failed < 0 || panel.Errors < 0 || panel.Skipped < 0 {
			return fmt.Errorf("report panel %d is invalid", index)
		}
	}
	seenCases := make(map[string]struct{}, len(r.Cases))
	for _, result := range r.Cases {
		if result.ID == "" || !slices.Contains(panels, result.Panel) || !validStatus(result.Status) || result.Status == StatusNotPopulated || result.DurationNS < 0 {
			return fmt.Errorf("report case %q is invalid", result.ID)
		}
		if _, duplicate := seenCases[result.ID]; duplicate {
			return fmt.Errorf("report duplicates case %q", result.ID)
		}
		seenCases[result.ID] = struct{}{}
		if result.Status == StatusFailed || result.Status == StatusError {
			if result.Failure == nil || !validFailureTaxonomy(result.Failure.Taxonomy) || result.Failure.Message == "" {
				return fmt.Errorf("report case %q omits a typed failure", result.ID)
			}
		} else if result.Failure != nil {
			return fmt.Errorf("report case %q attaches failure data to status %q", result.ID, result.Status)
		}
	}
	for _, metric := range r.Metrics {
		if metric.Name == "" || metric.Unit == "" || metric.Condition == "" || metric.Repetitions < 1 || metric.Threshold != nil {
			return fmt.Errorf("report metric %q has an invalid identity, repetitions, or absolute threshold", metric.Name)
		}
		if (metric.Baseline == nil) != (metric.Delta == nil) {
			return fmt.Errorf("report metric %q has an unpaired baseline/delta", metric.Name)
		}
	}
	for _, failure := range r.FailureTaxonomy {
		if !validFailureTaxonomy(failure.Taxonomy) || failure.Count < 1 {
			return errors.New("report has an invalid failure taxonomy count")
		}
	}
	expected := r
	expected.Summarize()
	if !reflect.DeepEqual(r.Panels, expected.Panels) || !reflect.DeepEqual(r.FailureTaxonomy, expected.FailureTaxonomy) || r.Summary != expected.Summary {
		return errors.New("report panels, failure taxonomy, or summary do not match case results")
	}
	return nil
}

func validStatus(status Status) bool {
	return slices.Contains([]Status{StatusPassed, StatusFailed, StatusError, StatusSkipped, StatusNotPopulated}, status)
}

func (r Report) Markdown() string {
	var output strings.Builder
	fmt.Fprintf(&output, "# Semantic memory evaluation\n\nRun `%s` at commit `%s`; report schema v%d, fixture `%s` (manifest v%d). Baseline: `%s`.\n\n", r.Run.ID, r.Run.Commit, r.ReportSchemaVersion, r.Fixture.FixtureVersion, r.Fixture.ManifestVersion, valueOrNone(r.BaselineRunID))
	fmt.Fprintf(&output, "Environment: %s; %s; %s; SQLite %s; journal %s (%s); %d repetitions; %s.\n\n", r.Environment.Hardware, r.Environment.OS, r.Environment.GoVersion, r.Environment.SQLiteVersion, r.Environment.JournalMode, r.Environment.JournalSetup, r.Environment.Repetitions, strings.Join(r.Environment.Conditions, ", "))
	output.WriteString("## Panels\n\n")
	for _, panel := range r.Panels {
		fmt.Fprintf(&output, "- `%s`: %s (%d passed, %d failed, %d errors, %d skipped)", panel.Panel, panel.Status, panel.Passed, panel.Failed, panel.Errors, panel.Skipped)
		if panel.Status == StatusNotPopulated {
			output.WriteString(" — Not populated in Stage 3")
		}
		output.WriteString("\n")
	}
	output.WriteString("\n## Hard-gate failures\n\n")
	failures := 0
	for _, result := range r.Cases {
		if result.Gate && result.Status != StatusPassed {
			failures++
			message := string(result.Status)
			if result.Failure != nil {
				message = string(result.Failure.Taxonomy) + ": " + result.Failure.Message
			}
			fmt.Fprintf(&output, "- `%s`: %s\n", result.ID, message)
		}
	}
	if failures == 0 {
		output.WriteString("None.\n")
	}
	output.WriteString("\n## Paired performance\n\n")
	for _, metric := range r.Metrics {
		fmt.Fprintf(&output, "- `%s` (%s, %s, n=%d): current p50 %.0f, p95 %.0f, max %.0f", metric.Name, metric.Unit, metric.Condition, metric.Repetitions, metric.Current.P50, metric.Current.P95, metric.Current.Max)
		if metric.Baseline != nil && metric.Delta != nil {
			fmt.Fprintf(&output, "; baseline p50 %.0f, p95 %.0f, max %.0f; delta %+.0f/%+.0f/%+.0f", metric.Baseline.P50, metric.Baseline.P95, metric.Baseline.Max, metric.Delta.P50, metric.Delta.P95, metric.Delta.Max)
		} else {
			output.WriteString("; initial baseline (no absolute threshold)")
		}
		output.WriteString("\n")
	}
	return output.String()
}

func valueOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
