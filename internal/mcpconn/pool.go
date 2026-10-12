package mcpconn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Live MCP sessions (story 2). Serve keeps at most one per connection,
// shared by chats, opened on first use and reopened after a failure. The
// SDK speaks the protocol only: Evie's transport carries the tokens, the
// client advertises no capabilities, and nothing re-sends a call.

// openTimeout bounds opening a session or listing tools.
const openTimeout = 10 * time.Second

// maxTools bounds one server's tool listing.
const maxTools = 500

// Tool is one server tool as a Workspace reviews it. Hash covers its name,
// description, input schema and annotations as canonical JSON (sorted
// keys), exactly as the server listed them.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
	Hash        string          `json:"hash"`
}

// Selector is the reviewed tool's Workspace Access resource.
func (t Tool) Selector() string { return t.Name + "@" + t.Hash }

// SplitSelector splits a reviewed tool's `<name>@<hash>` at its last @.
func SplitSelector(selector string) (name, hash string) {
	i := strings.LastIndex(selector, "@")
	if i < 0 {
		return selector, ""
	}
	return selector[:i], selector[i+1:]
}

// CallResult is a tool's answer, shaped for the model.
type CallResult struct {
	Text    string
	IsError bool
}

// CallError is a call the server did not answer completely.
type CallError struct {
	Outcome SendOutcome
	Err     error
}

func (e *CallError) Error() string { return fmt.Sprintf("%s: %v", e.Outcome, e.Err) }
func (e *CallError) Unwrap() error { return e.Err }

// ConnectionReader reads connection records.
type ConnectionReader interface {
	MCPConnection(ctx context.Context, id string) (Connection, error)
}

type PoolConfig struct {
	Records ConnectionReader
	Tokens  func(connectionID string) TokenSource
	// Base carries the connection's HTTPS requests under Evie's transport.
	Base http.RoundTripper
}

// Pool holds the live sessions. p.mu guards only the map: no lock is held
// across network work, so a disconnect never waits on a session that is
// opening or busy, and closing a session happens in the background.
type Pool struct {
	cfg  PoolConfig
	mu   sync.Mutex
	live map[string]*poolEntry
}

// poolEntry is one connection's session, possibly still opening.
type poolEntry struct {
	ready chan struct{} // closed once cs or err is set
	cs    *mcp.ClientSession
	err   error
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.Base == nil {
		cfg.Base = http.DefaultTransport
	}
	return &Pool{cfg: cfg, live: map[string]*poolEntry{}}
}

// ListTools lists the server's tools now; nothing is cached.
func (p *Pool) ListTools(ctx context.Context, id string) ([]Tool, error) {
	if _, err := p.cfg.Tokens(id).Token(ctx); err != nil {
		return nil, err
	}
	cs, err := p.session(ctx, id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	var tools []Tool
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			if ctx.Err() == nil && !serverAnswered(err) {
				p.drop(id, cs)
			}
			return nil, err
		}
		if len(tools) == maxTools {
			break
		}
		tools = append(tools, toolFromSDK(tool))
	}
	return tools, nil
}

// CallTool sends one tools/call. A failure is a *CallError naming whether
// the server got and answered it; nothing is retried here. The caller's
// cancellation cancels the request (the SDK tells the server), and only a
// broken connection drops the shared session.
func (p *Pool) CallTool(ctx context.Context, id, name string, arguments json.RawMessage) (CallResult, error) {
	if _, err := p.cfg.Tokens(id).Token(ctx); err != nil {
		return CallResult{}, err
	}
	cs, err := p.session(ctx, id)
	if err != nil {
		if ctx.Err() != nil {
			return CallResult{}, &CallError{Outcome: OutcomeNotSent, Err: ctx.Err()}
		}
		return CallResult{}, &CallError{Outcome: OutcomeNotSent, Err: err}
	}
	traced, trace := WithSendTrace(ctx)
	result, err := cs.CallTool(traced, &mcp.CallToolParams{Name: name, Arguments: arguments})
	switch {
	case err == nil:
		return shapeResult(result), nil
	case serverAnswered(err):
		// A JSON-RPC error is the server's complete answer.
		return CallResult{}, &CallError{Outcome: OutcomeRejected, Err: err}
	case ctx.Err() != nil:
		return CallResult{}, &CallError{Outcome: trace.Outcome(err), Err: err}
	default:
		p.drop(id, cs)
		return CallResult{}, &CallError{Outcome: trace.Outcome(err), Err: err}
	}
}

// serverAnswered reports a JSON-RPC error from the server itself, as
// opposed to the SDK's transport rejection (code -32005).
func serverAnswered(err error) bool {
	var wire *jsonrpc.Error
	return errors.As(err, &wire) && wire.Code != transportRejected
}

// transportRejected is the SDK's code for a request its transport could
// not complete.
const transportRejected = -32005

// Close forgets a connection's session, after a disconnect, and closes it
// in the background. A session still opening is closed as soon as it opens.
func (p *Pool) Close(id string) {
	p.mu.Lock()
	entry := p.live[id]
	delete(p.live, id)
	p.mu.Unlock()
	if entry != nil {
		go func() {
			<-entry.ready
			if entry.cs != nil {
				_ = entry.cs.Close()
			}
		}()
	}
}

// CloseAll closes every live session.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	ids := make([]string, 0, len(p.live))
	for id := range p.live {
		ids = append(ids, id)
	}
	p.mu.Unlock()
	for _, id := range ids {
		p.Close(id)
	}
}

// drop forgets a broken session so the next call opens a new one.
func (p *Pool) drop(id string, cs *mcp.ClientSession) {
	p.mu.Lock()
	if entry := p.live[id]; entry != nil && entry.cs == cs {
		delete(p.live, id)
	}
	p.mu.Unlock()
	go func() { _ = cs.Close() }()
}

// session returns the connection's live session, opening one if needed;
// callers that arrive while it opens wait for it, or for their context.
// It opens under a fresh context so no call's trace or deadline sticks to
// the session.
func (p *Pool) session(ctx context.Context, id string) (*mcp.ClientSession, error) {
	p.mu.Lock()
	entry, ok := p.live[id]
	if !ok {
		entry = &poolEntry{ready: make(chan struct{})}
		p.live[id] = entry
		go p.open(id, entry)
	}
	p.mu.Unlock()
	select {
	case <-entry.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return entry.cs, entry.err
}

func (p *Pool) open(id string, entry *poolEntry) {
	cs, err := p.connect(id)
	p.mu.Lock()
	current := p.live[id] == entry
	if err != nil && current {
		delete(p.live, id) // the next caller tries again
	}
	p.mu.Unlock()
	if err == nil && !current {
		// Disconnected while opening.
		go func() { _ = cs.Close() }()
		cs, err = nil, errors.New("the connection was disconnected")
	}
	entry.cs, entry.err = cs, err
	close(entry.ready)
}

func (p *Pool) connect(id string) (*mcp.ClientSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()
	connection, err := p.cfg.Records.MCPConnection(ctx, id)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(connection.URL)
	if err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "evie", Version: "1.0.0"}, &mcp.ClientOptions{
		Capabilities:   &mcp.ClientCapabilities{},
		MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true},
	})
	transport := &mcp.StreamableClientTransport{
		Endpoint:             connection.URL,
		HTTPClient:           NewHTTPClient(p.cfg.Base, u.Host, p.cfg.Tokens(id)),
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("could not open an MCP session with %s: %w", connection.Name, err)
	}
	return cs, nil
}

func toolFromSDK(t *mcp.Tool) Tool {
	schema, _ := json.Marshal(t.InputSchema)
	annotations := []byte("null")
	if t.Annotations != nil {
		annotations, _ = json.Marshal(t.Annotations)
	}
	canonical := canonicalJSON(map[string]json.RawMessage{
		"name":        mustJSON(t.Name),
		"description": mustJSON(t.Description),
		"inputSchema": schema,
		"annotations": annotations,
	})
	sum := sha256.Sum256(canonical)
	tool := Tool{Name: t.Name, Description: t.Description, InputSchema: schema, Hash: hex.EncodeToString(sum[:])}
	if t.Annotations != nil {
		tool.Annotations = annotations
	}
	return tool
}

// canonicalJSON re-encodes a value with sorted object keys and exact
// numbers.
func canonicalJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return raw
	}
	out, _ := json.Marshal(generic)
	return out
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func shapeResult(result *mcp.CallToolResult) CallResult {
	parts := []string{}
	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			parts = append(parts, c.Text)
		case *mcp.ImageContent:
			parts = append(parts, "[image content omitted]")
		case *mcp.AudioContent:
			parts = append(parts, "[audio content omitted]")
		case *mcp.ResourceLink:
			parts = append(parts, "[resource link omitted]")
		case *mcp.EmbeddedResource:
			parts = append(parts, "[embedded resource omitted]")
		default:
			parts = append(parts, fmt.Sprintf("[%s content omitted]", strings.TrimPrefix(fmt.Sprintf("%T", content), "*mcp.")))
		}
	}
	if result.StructuredContent != nil {
		if encoded, err := json.Marshal(result.StructuredContent); err == nil {
			parts = append(parts, "structuredContent: "+string(encoded))
		}
	}
	return CallResult{Text: strings.Join(parts, "\n"), IsError: result.IsError}
}
