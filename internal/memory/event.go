package memory

import (
	"encoding/json"
	"errors"
	"fmt"
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
	default:
		return fmt.Errorf("invalid terminal lifecycle stage %q", p.Stage)
	}

	switch eventType {
	case EventTurnFailed:
		if p.Classification != ClassificationProviderError &&
			p.Classification != ClassificationProviderResponseInvalid {
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
	default:
		return ""
	}
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
