package openrouter

import "encoding/json"

// Message is one entry in a conversation. The same struct covers all roles:
// system and user messages fill Role and Content; an assistant turn that
// requests tools carries ToolCalls; a tool result answers with Role "tool",
// its Content, and the ToolCallID it is responding to.
type Message struct {
	Role             string          `json:"role"`
	Reasoning        string          `json:"reasoning,omitempty"`
	ReasoningDetails json.RawMessage `json:"reasoning_details,omitempty"`
	Content          string          `json:"content,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
}

// Choice is one candidate completion in a response; we only ever use the
// first.
type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Client holds what every request needs. The api key is unexported on
// purpose: construction goes through NewClient so an empty key can never
// reach request time.
type Client struct {
	apiKey  string
	baseURL string
}

// Tool is the wire format for advertising one tool to the model. Type is
// always "function" — the OpenAI-compatible API wraps everything in that
// envelope.
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function is the model-facing definition of a tool: its name, what it
// does, and the JSON-Schema shape of its arguments.
type Function struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Parameters  Parameter `json:"parameters"`
}

// Parameter is the top level of a tool's JSON-Schema arguments: an object
// with named properties, some of which may be required.
type Parameter struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single tool argument. Items is only set when Type is
// "array" (the element type); Enum restricts a string to fixed values.
type Property struct {
	Type        string    `json:"type"`
	Items       *Property `json:"items,omitempty"`
	Description string    `json:"description,omitempty"`
	Enum        []string  `json:"enum,omitempty"`
}

// ToolCall is the model asking for one tool to be run. Its ID must be
// echoed back as the ToolCallID of the responding tool message.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall names the tool being invoked and carries its arguments as a
// raw JSON string — the caller unmarshals it against the tool's own schema.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatResponse is the body of a successful chat-completions reply.
type ChatResponse struct {
	Choices []Choice `json:"choices"`
}

// ReasoningConfig opts a request into reasoning. Effort ("low", "medium",
// "high") implies enabled; Enabled alone asks for the provider's default.
type ReasoningConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

// ChatRequest is the body of one chat-completions call: the full
// conversation so far plus every tool the model is allowed to use.
// Stream is set by ChatStream, never by callers. Reasoning is a pointer so
// a nil omits the key entirely — a model without reasoning support must
// see exactly the request it saw before this field existed.
type ChatRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Tools     []Tool           `json:"tools,omitempty"`
	Stream    bool             `json:"stream,omitempty"`
	Reasoning *ReasoningConfig `json:"reasoning,omitempty"`
}

// streamChunk is one SSE "data:" event in a streaming response. Deltas
// are fragments: content arrives token by token, and tool calls arrive
// as indexed pieces (id and name first, arguments split across many
// chunks) that the client reassembles.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role             string            `json:"role"`
			Content          string            `json:"content"`
			ToolCalls        []toolCallDelta   `json:"tool_calls"`
			Reasoning        string            `json:"reasoning"`
			ReasoningDetails []json.RawMessage `json:"reasoning_details"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// toolCallDelta is one fragment of a tool call in a stream. Index says
// which call it belongs to (a message can carry several); ID, Type, and
// the function name arrive on the first fragment, argument JSON drips
// in across the rest.
type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
