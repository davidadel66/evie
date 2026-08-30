package memory

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type (
	EventID     string
	ExecutionID string
	EventType   string
	EventRole   string
)

const (
	EventUserMessage       EventType = "user_message"
	EventAssistantMessage  EventType = "assistant_message"
	EventToolIntent        EventType = "tool_intent"
	EventToolSucceeded     EventType = "tool_succeeded"
	EventToolFailed        EventType = "tool_failed"
	EventToolCancelled     EventType = "tool_cancelled"
	EventApproval          EventType = "approval"
	EventExecutionResolved EventType = "execution_resolved"
	EventTurnFailed        EventType = "turn_failed"
	EventTurnInterrupted   EventType = "turn_interrupted"
	EventContextSnapshot   EventType = "context_snapshot"

	RoleUser      EventRole = "user"
	RoleAssistant EventRole = "assistant"
	RoleTool      EventRole = "tool"
)

type TurnClassification string

const (
	ClassificationProviderError           TurnClassification = "provider_error"
	ClassificationProviderResponseInvalid TurnClassification = "provider_response_invalid"
	ClassificationCallerCancelled         TurnClassification = "caller_cancelled"
	ClassificationCallerDeadlineExceeded  TurnClassification = "caller_deadline_exceeded"
	ClassificationContextOverflow         TurnClassification = "context_overflow"
)

type TurnStage string

const (
	StageTurnStart       TurnStage = "turn_start"
	StageProvider        TurnStage = "provider"
	StageAssistantCommit TurnStage = "assistant_commit"
	StageToolPrepare     TurnStage = "tool_prepare"
	StageToolApproval    TurnStage = "tool_approval"
	StageToolExecute     TurnStage = "tool_execute"
	StageToolCommit      TurnStage = "tool_commit"
	StageContextCompose  TurnStage = "context_compose"
)

type TurnTerminalPayload struct {
	TurnID         EventID            `json:"turn_id"`
	Classification TurnClassification `json:"classification"`
	Stage          TurnStage          `json:"stage"`
	HTTPStatus     *int               `json:"http_status,omitempty"`
}

func (p TurnTerminalPayload) Validate(eventType EventType) error {
	if p.TurnID == "" {
		return errors.New("terminal turn ID must not be empty")
	}
	switch p.Stage {
	case StageTurnStart, StageProvider, StageAssistantCommit, StageToolPrepare,
		StageToolApproval, StageToolExecute, StageToolCommit:
		if p.Classification == ClassificationContextOverflow {
			return fmt.Errorf("context overflow cannot use lifecycle stage %q", p.Stage)
		}
	case StageContextCompose:
	default:
		return fmt.Errorf("invalid terminal lifecycle stage %q", p.Stage)
	}

	switch eventType {
	case EventTurnFailed:
		if p.Classification != ClassificationProviderError &&
			p.Classification != ClassificationProviderResponseInvalid &&
			p.Classification != ClassificationContextOverflow {
			return fmt.Errorf("invalid failed-turn classification %q", p.Classification)
		}
	case EventTurnInterrupted:
		if p.Classification != ClassificationCallerCancelled &&
			p.Classification != ClassificationCallerDeadlineExceeded {
			return fmt.Errorf("invalid interrupted-turn classification %q", p.Classification)
		}
	default:
		return fmt.Errorf("event type %q is not terminal", eventType)
	}

	if p.HTTPStatus != nil {
		if p.Classification != ClassificationProviderError || *p.HTTPStatus < 100 ||
			*p.HTTPStatus > 999 || (*p.HTTPStatus >= 200 && *p.HTTPStatus < 300) {
			return errors.New("http_status is allowed only for a valid provider_error status")
		}
	}
	return nil
}

func (p TurnTerminalPayload) SafeContent() string {
	switch p.Classification {
	case ClassificationProviderError:
		return "The provider request failed."
	case ClassificationProviderResponseInvalid:
		return "The provider response was invalid."
	case ClassificationCallerCancelled:
		return "The turn was cancelled by the caller."
	case ClassificationCallerDeadlineExceeded:
		return "The caller deadline was exceeded."
	case ClassificationContextOverflow:
		return "The turn could not fit within the configured model context."
	default:
		return ""
	}
}

const ContextSnapshotSchemaVersion = 1

type ContextCompactionFailureCategory string

const (
	ContextCompactionFailureNone        ContextCompactionFailureCategory = ""
	ContextCompactionSummaryProvider    ContextCompactionFailureCategory = "summary_provider_error"
	ContextCompactionSummaryInvalid     ContextCompactionFailureCategory = "summary_invalid"
	ContextCompactionSummaryPersistence ContextCompactionFailureCategory = "summary_persistence_failed"
)

// ContextPlaceholderManifest describes a projected tool result without
// retaining any of its content in context diagnostics.
type ContextPlaceholderManifest struct {
	EventID        EventID `json:"event_id"`
	OriginalBytes  int64   `json:"original_bytes"`
	ProjectedBytes int64   `json:"projected_bytes"`
	SHA256         string  `json:"sha256"`
}

// ContextSnapshotPayload is a content-free manifest of one exact
// conversational provider request. Content-bearing request fields deliberately
// have no representation here.
type ContextSnapshotPayload struct {
	SchemaVersion             int                              `json:"schema_version"`
	ComposerVersion           string                           `json:"composer_version"`
	EstimatorVersion          string                           `json:"estimator_version"`
	Iteration                 int                              `json:"iteration"`
	ConfiguredModel           string                           `json:"configured_model"`
	CanonicalModel            string                           `json:"canonical_model"`
	AdvertisedModel           string                           `json:"advertised_model,omitempty"`
	ProfileSource             string                           `json:"profile_source"`
	AdvertisedWindowTokens    int64                            `json:"advertised_window_tokens,omitempty"`
	HardWindowTokens          int64                            `json:"hard_window_tokens"`
	WorkingCeilingTokens      int64                            `json:"working_ceiling_tokens"`
	OutputReserveTokens       int64                            `json:"output_reserve_tokens"`
	EstimationMarginTokens    int64                            `json:"estimation_margin_tokens"`
	UsableInputBytes          int64                            `json:"usable_input_bytes"`
	SerializedBytes           int64                            `json:"serialized_bytes"`
	RoughTokenEstimate        int64                            `json:"rough_token_estimate"`
	RequestSHA256             string                           `json:"request_sha256"`
	RetainedFirstEventID      EventID                          `json:"retained_first_event_id"`
	RetainedFirstSequence     int64                            `json:"retained_first_sequence"`
	RetainedLastEventID       EventID                          `json:"retained_last_event_id"`
	RetainedLastSequence      int64                            `json:"retained_last_sequence"`
	ActiveCompactionEventID   EventID                          `json:"active_compaction_event_id,omitempty"`
	CompactionFailureCategory ContextCompactionFailureCategory `json:"compaction_failure_category,omitempty"`
	MessageCount              int                              `json:"message_count"`
	ToolSchemaCount           int                              `json:"tool_schema_count"`
	SystemMessageBytes        int64                            `json:"system_message_bytes"`
	SummaryMessageBytes       int64                            `json:"summary_message_bytes"`
	HistoryMessageBytes       int64                            `json:"history_message_bytes"`
	ToolSchemaBytes           int64                            `json:"tool_schema_bytes"`
	RequestSettingsBytes      int64                            `json:"request_settings_bytes"`
	Placeholders              []ContextPlaceholderManifest     `json:"placeholders,omitempty"`
}

func (p ContextSnapshotPayload) Validate() error {
	if p.SchemaVersion != ContextSnapshotSchemaVersion {
		return fmt.Errorf("unsupported context snapshot schema version %d", p.SchemaVersion)
	}
	if p.ComposerVersion == "" || p.EstimatorVersion == "" || p.Iteration <= 0 {
		return errors.New("context snapshot versioning and iteration must be present")
	}
	if p.ConfiguredModel == "" || p.CanonicalModel == "" {
		return errors.New("context snapshot model identities must be present")
	}
	switch p.ProfileSource {
	case "remote_metadata":
		if p.AdvertisedModel == "" || p.AdvertisedWindowTokens <= 0 {
			return errors.New("remote context snapshot metadata is incomplete")
		}
	case "explicit_override", "builtin_fallback":
		if p.AdvertisedModel != "" || p.AdvertisedWindowTokens != 0 {
			return errors.New("non-remote context snapshot contains advertised metadata")
		}
	default:
		return fmt.Errorf("invalid context profile source %q", p.ProfileSource)
	}
	if p.HardWindowTokens <= 0 || p.WorkingCeilingTokens <= 0 ||
		p.OutputReserveTokens <= 0 || p.EstimationMarginTokens <= 0 {
		return errors.New("context snapshot budgets must be positive")
	}
	if p.WorkingCeilingTokens > p.HardWindowTokens ||
		p.OutputReserveTokens > math.MaxInt64-p.EstimationMarginTokens {
		return errors.New("context snapshot budgets are inconsistent")
	}
	ceiling := min(p.HardWindowTokens, p.WorkingCeilingTokens)
	if p.OutputReserveTokens+p.EstimationMarginTokens >= ceiling ||
		p.UsableInputBytes != ceiling-p.OutputReserveTokens-p.EstimationMarginTokens {
		return errors.New("context snapshot usable input does not match its budgets")
	}
	if p.SerializedBytes <= 0 || p.SerializedBytes > p.UsableInputBytes ||
		p.RoughTokenEstimate != (p.SerializedBytes+3)/4 {
		return errors.New("context snapshot request estimates are inconsistent")
	}
	if !validSHA256(p.RequestSHA256) {
		return errors.New("context snapshot request hash must be lowercase SHA-256")
	}
	if p.RetainedFirstEventID == "" || p.RetainedLastEventID == "" ||
		p.RetainedFirstSequence <= 0 || p.RetainedLastSequence < p.RetainedFirstSequence {
		return errors.New("context snapshot retained frontier is invalid")
	}
	if p.MessageCount <= 0 || p.ToolSchemaCount < 0 || p.SystemMessageBytes <= 0 ||
		p.SummaryMessageBytes < 0 || p.HistoryMessageBytes < 0 ||
		p.ToolSchemaBytes < 0 || p.RequestSettingsBytes <= 0 {
		return errors.New("context snapshot counts and byte breakdowns are invalid")
	}
	if (p.ActiveCompactionEventID == "") != (p.SummaryMessageBytes == 0) {
		return errors.New("context snapshot summary bytes and compaction identity are inconsistent")
	}
	switch p.CompactionFailureCategory {
	case ContextCompactionFailureNone, ContextCompactionSummaryProvider,
		ContextCompactionSummaryInvalid, ContextCompactionSummaryPersistence:
	default:
		return fmt.Errorf("invalid context compaction failure category %q", p.CompactionFailureCategory)
	}
	seenPlaceholders := make(map[EventID]struct{}, len(p.Placeholders))
	for i, placeholder := range p.Placeholders {
		if placeholder.EventID == "" || placeholder.OriginalBytes <= 0 ||
			placeholder.ProjectedBytes <= 0 || placeholder.ProjectedBytes >= placeholder.OriginalBytes ||
			!validSHA256(placeholder.SHA256) {
			return fmt.Errorf("invalid context placeholder manifest %d", i)
		}
		if _, duplicate := seenPlaceholders[placeholder.EventID]; duplicate {
			return fmt.Errorf("context placeholder manifest repeats event %q", placeholder.EventID)
		}
		seenPlaceholders[placeholder.EventID] = struct{}{}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type AssistantMessagePayload struct {
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
	Usage     *TokenUsage `json:"usage,omitempty"`
}

type TokenUsage struct {
	InputTokens           *int64 `json:"input_tokens,omitempty"`
	OutputTokens          *int64 `json:"output_tokens,omitempty"`
	TotalTokens           *int64 `json:"total_tokens,omitempty"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens,omitempty"`
	CachedInputTokens     *int64 `json:"cached_input_tokens,omitempty"`
	CacheWriteInputTokens *int64 `json:"cache_write_input_tokens,omitempty"`
}

type ToolIntentPayload struct {
	Call ToolCall `json:"call"`
}

type ToolResultPayload struct {
	ToolCallID string `json:"tool_call_id"`
	IsError    bool   `json:"is_error"`
}

type ApprovalDecision string

const (
	ApprovalApproved ApprovalDecision = "approved"
	ApprovalDeclined ApprovalDecision = "declined"
	ApprovalExpired  ApprovalDecision = "expired"
)

type ApprovalPayload struct {
	Decision ApprovalDecision `json:"decision"`
}

type EventInput struct {
	ParentID    EventID
	Type        EventType
	Role        EventRole
	ExecutionID ExecutionID
	Content     string
	Payload     json.RawMessage
}

type Event struct {
	ID            EventID
	SessionID     SessionID
	Sequence      int64
	ProjectID     ProjectID
	ParentID      EventID
	Type          EventType
	Role          EventRole
	ExecutionID   ExecutionID
	Content       string
	Payload       json.RawMessage
	RecordedAt    time.Time
	FormatVersion int
}
