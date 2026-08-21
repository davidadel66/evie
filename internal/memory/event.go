package memory

import (
	"encoding/json"
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

	RoleUser      EventRole = "user"
	RoleAssistant EventRole = "assistant"
	RoleTool      EventRole = "tool"
)

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type AssistantMessagePayload struct {
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
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
