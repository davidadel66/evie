// Package agent is the harness core: the model↔tools loop, extracted from
// any particular frontend. A Session holds one conversation; frontends
// (terminal REPL, web server) drive it through Send and receive everything
// worth rendering through the Events interface. This package never prints —
// the repo's "domain layer silent, frontends own output" convention applied
// to the agent itself.
package agent

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// DefaultModel is the fallback when neither the caller nor EVIE_MODEL
// says otherwise. One source of truth for every frontend.
const DefaultModel = "moonshotai/kimi-k3"

// ErrBusy means a turn is already running on this session. Frontends map
// it to their own affordance (the web server answers HTTP 409).
var ErrBusy = errors.New("agent: a turn is already in progress")

// Client is what a Session needs from a provider: one streaming chat
// call. Satisfied by *openrouter.Client; defined here, on the consumer
// side, so tests can script a fake and a second provider can slot in
// without touching the loop.
type Client interface {
	ChatStream(req openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error)
}

// Events receives everything a frontend needs to render a turn, in the
// order it happens: Delta* AssistantDone (ToolCall ToolResult)* repeating
// until an AssistantDone with no tool calls ends the turn. Implementations
// must be fast or buffer internally — the loop blocks on them.
type Events interface {
	Delta(text string)                         // streaming assistant text
	Reasoning(text string)                     // streaming thinking text
	ReasoningDone()                            // thinking ended for this assistant message
	AssistantDone(content string)              // every assistant message, even empty (tool-only)
	ToolCall(id, name, args string)            // emitted immediately before executing
	ToolResult(id, content string, isErr bool) // tool finished (includes declines)
}

// Session is one conversation: the transcript plus the client that
// extends it. One turn at a time — Send fails fast with ErrBusy rather
// than queueing, so a frontend always knows whether it got the slot.
type Session struct {
	mu        sync.Mutex
	client    Client
	model     string
	reasoning *openrouter.ReasoningConfig
	messages  []openrouter.Message
}

// Send runs one full turn: append the user message, then model↔tools
// until the model answers without tool calls. extra tools are offered to
// the model for this turn only (the web frontend passes "show"; the REPL
// passes none). The error return is the turn aborting — a failed request
// or an empty response — never a tool failure; those go back to the model
// as tool messages so it can correct itself.
func (s *Session) Send(input string, ev Events, approve tools.Approver, extra ...tools.Tool) error {
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()

	s.messages = append(s.messages, openrouter.Message{Role: "user", Content: input})

	for {
		req := openrouter.ChatRequest{
			Model:     s.model,
			Messages:  s.messages,
			Tools:     tools.SchemasWith(extra),
			Reasoning: s.reasoning,
		}

		// thinking tracks whether reasoning is open for this one assistant
		// message: the first content delta ends it, and a tool-only message
		// (no delta ever arrives) ends it after assembly. ReasoningDone
		// fires at most once per message, and only if reasoning streamed.
		thinking := false
		h := openrouter.StreamHandlers{
			OnReasoning: func(text string) {
				thinking = true
				ev.Reasoning(text)
			},
			OnContent: func(text string) {
				if thinking {
					thinking = false
					ev.ReasoningDone()
				}
				ev.Delta(text)
			},
		}

		res, err := s.client.ChatStream(req, h)
		if err != nil {
			return fmt.Errorf("chat request failed: %w", err)
		}
		if len(res.Choices) == 0 {
			return errors.New("agent: provider returned no choices")
		}

		msg := res.Choices[0].Message
		s.messages = append(s.messages, msg)
		if thinking {
			ev.ReasoningDone()
		}
		ev.AssistantDone(msg.Content)

		if len(msg.ToolCalls) == 0 {
			return nil
		}

		for _, call := range msg.ToolCalls {
			ev.ToolCall(call.ID, call.Function.Name, call.Function.Arguments)
			result, isErr := tools.ExecuteWith(extra, call, approve)
			s.messages = append(s.messages, result)
			ev.ToolResult(call.ID, result.Content, isErr)
		}
	}
}

// New creates a session seeded with the system prompt. model may be ""
// to mean "resolve it": EVIE_MODEL if set, else DefaultModel.
// resolveReasoning maps EVIE_REASONING to a request config: "off" leaves
// the key out entirely (nil), "on"/"" ask for the provider default, and an
// effort level implies enabled. Unknown values fall back to on.
func resolveReasoning(v string) *openrouter.ReasoningConfig {
	switch v {
	case "off":
		return nil
	case "high", "medium", "low":
		return &openrouter.ReasoningConfig{Effort: v}
	default:
		return &openrouter.ReasoningConfig{Enabled: true}
	}
}

func New(client Client, model string) *Session {
	if model == "" {
		model = os.Getenv("EVIE_MODEL")
	}
	if model == "" {
		model = DefaultModel
	}
	return &Session{
		client:    client,
		model:     model,
		reasoning: resolveReasoning(os.Getenv("EVIE_REASONING")),
		messages: []openrouter.Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
		},
	}
}
