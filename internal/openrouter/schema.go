package openrouter

// Message is one entry in a conversation. The same struct covers all roles:
// system and user messages fill Role and Content; an assistant turn that
// requests tools carries ToolCalls; a tool result answers with Role "tool",
// its Content, and the ToolCallID it is responding to.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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
	apiKey string
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

// ChatRequest is the body of one chat-completions call: the full
// conversation so far plus every tool the model is allowed to use.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}
