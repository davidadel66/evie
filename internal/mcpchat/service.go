// Package mcpchat is the Workspace side of MCP connections
// (workspace-mcp-connections.spec.md): which reviewed server tools a chat
// may reach, and the owner's tool review that changes Workspace Access.
// Reviewed tools are Workspace Access resources `<name>@<sha256>` under
// the connection's Connection ID.
package mcpchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/mcpconn"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

var (
	ErrToolNotOffered   = errors.New("the server does not offer that tool")
	ErrToolUnselectable = errors.New("that tool's name cannot be enabled (whitespace, control characters, or longer than 64 characters)")
)

// Store is the Workspace Access and connection state the service reads
// and writes; eviedb implements it.
type Store interface {
	SessionWorkspaceAccessPin(ctx context.Context, session memory.SessionID) (memory.WorkspaceID, int64, error)
	WorkspaceAccess(ctx context.Context, ws memory.WorkspaceID) (eviedb.WorkspaceAccessRevision, error)
	WorkspaceAccessAt(ctx context.Context, ws memory.WorkspaceID, revision int64) (eviedb.WorkspaceAccessRevision, error)
	SetWorkspaceAccess(ctx context.Context, ws memory.WorkspaceID, expected int64, access workflow.WorkspaceAccess) (eviedb.WorkspaceAccessRevision, error)
	MCPConnection(ctx context.Context, id string) (mcpconn.Connection, error)
}

// Tools reaches the servers; mcpconn.Pool implements it.
type Tools interface {
	ListTools(ctx context.Context, connectionID string) ([]mcpconn.Tool, error)
	CallTool(ctx context.Context, connectionID, name string, arguments json.RawMessage) (mcpconn.CallResult, error)
}

type Service struct {
	store Store
	tools Tools
}

func New(store Store, tools Tools) *Service { return &Service{store: store, tools: tools} }

// Reach is a chat's effective MCP access: per Connection ID, the reviewed
// selectors in both the access revision the chat pinned and the current
// access. A chat with no pin reaches nothing.
func (s *Service) Reach(ctx context.Context, session memory.SessionID, ws memory.WorkspaceID) (map[string]map[string]bool, error) {
	reach := map[string]map[string]bool{}
	pinnedWS, revision, err := s.store.SessionWorkspaceAccessPin(ctx, session)
	if errors.Is(err, eviedb.ErrNoWorkspaceAccessPin) {
		return reach, nil
	}
	if err != nil {
		return nil, err
	}
	if pinnedWS != ws {
		return nil, fmt.Errorf("the chat belongs to Workspace %s, not %s", pinnedWS, ws)
	}
	pinned, err := s.store.WorkspaceAccessAt(ctx, ws, revision)
	if err != nil {
		return nil, err
	}
	current, err := s.store.WorkspaceAccess(ctx, ws)
	if err != nil {
		return nil, err
	}
	allowed := map[string]map[string]bool{}
	for _, c := range current.Access.Connections {
		if !strings.HasPrefix(c.ID, "mcp-") {
			continue
		}
		allowed[c.ID] = map[string]bool{}
		for _, r := range c.Resources {
			allowed[c.ID][r] = true
		}
	}
	for _, c := range pinned.Access.Connections {
		for _, r := range c.Resources {
			if allowed[c.ID][r] {
				if reach[c.ID] == nil {
					reach[c.ID] = map[string]bool{}
				}
				reach[c.ID][r] = true
			}
		}
	}
	return reach, nil
}

func (s *Service) Connection(ctx context.Context, id string) (mcpconn.Connection, error) {
	return s.store.MCPConnection(ctx, id)
}

func (s *Service) ListTools(ctx context.Context, id string) ([]mcpconn.Tool, error) {
	return s.tools.ListTools(ctx, id)
}

func (s *Service) CallTool(ctx context.Context, id, name string, arguments json.RawMessage) (mcpconn.CallResult, error) {
	return s.tools.CallTool(ctx, id, name, arguments)
}

// Allowed reports whether the owner set a reviewed tool to Allow in ws.
// Every call asks until the Tool Policy table exists (story 3).
func (s *Service) Allowed(context.Context, memory.WorkspaceID, string, string) (bool, error) {
	return false, nil
}

// ToolState is a server tool's standing in one Workspace.
type ToolState string

const (
	ToolEnabled      ToolState = "enabled"      // its current hash is reviewed
	ToolChanged      ToolState = "changed"      // an older version is reviewed
	ToolAvailable    ToolState = "available"    // offered, not enabled
	ToolUnselectable ToolState = "unselectable" // its name cannot be enabled
)

type ReviewedTool struct {
	mcpconn.Tool
	State ToolState `json:"state"`
}

// Review is one connection's tools as one Workspace sees them.
type Review struct {
	Connection      mcpconn.Connection `json:"connection"`
	AccessRevision  int64              `json:"access_revision"`
	Tools           []ReviewedTool     `json:"tools"`
	NoLongerOffered []string           `json:"no_longer_offered,omitempty"`
	// Unlisted marks a review written without the server (a disable while
	// it is unreachable or signed out); Reviewed then names what remains.
	Unlisted bool     `json:"unlisted,omitempty"`
	Reviewed []string `json:"reviewed,omitempty"`
}

// Review lists the connection's current tools and their state in ws.
func (s *Service) Review(ctx context.Context, ws memory.WorkspaceID, name string) (Review, error) {
	connection, current, tools, err := s.reviewInputs(ctx, ws, name, true)
	if err != nil {
		return Review{}, err
	}
	return review(connection, current, tools), nil
}

// SetToolEnabled enables (replacing any older reviewed version) or
// disables one tool. The first enabled tool attaches the connection to the
// Workspace and disabling the last one removes it. The write expects the
// access revision it read, so a concurrent change is refused. Disabling
// works without the server: revoking must not wait for a sign-in.
func (s *Service) SetToolEnabled(ctx context.Context, ws memory.WorkspaceID, name, toolName string, enabled bool) (Review, error) {
	connection, current, tools, err := s.reviewInputs(ctx, ws, name, enabled)
	if err != nil {
		return Review{}, err
	}
	var selector string
	if enabled {
		i := slices.IndexFunc(tools, func(t mcpconn.Tool) bool { return t.Name == toolName })
		if i < 0 {
			return Review{}, fmt.Errorf("%w: %s on %s", ErrToolNotOffered, toolName, connection.Name)
		}
		if !SelectableName(toolName) {
			return Review{}, ErrToolUnselectable
		}
		selector = tools[i].Selector()
	}
	next := workflow.WorkspaceAccess{}
	found := false
	for _, c := range current.Access.Connections {
		if c.ID != connection.ID {
			next.Connections = append(next.Connections, c)
			continue
		}
		found = true
		next.Connections = append(next.Connections, withTool(c, toolName, selector))
	}
	if !found && enabled {
		next.Connections = append(next.Connections, workflow.ConnectionAccess{ID: connection.ID, Resources: []string{selector}})
	}
	next.Connections = slices.DeleteFunc(next.Connections, func(c workflow.ConnectionAccess) bool { return len(c.Resources) == 0 })
	written, err := s.store.SetWorkspaceAccess(ctx, ws, current.Revision, next)
	if err != nil {
		return Review{}, err
	}
	return review(connection, written, tools), nil
}

// withTool drops every reviewed version of toolName and adds selector.
func withTool(c workflow.ConnectionAccess, toolName, selector string) workflow.ConnectionAccess {
	out := workflow.ConnectionAccess{ID: c.ID}
	for _, r := range c.Resources {
		if name, _ := mcpconn.SplitSelector(r); name != toolName {
			out.Resources = append(out.Resources, r)
		}
	}
	if selector != "" {
		out.Resources = append(out.Resources, selector)
	}
	return out
}

// reviewInputs reads the connection, the current access and the live tools.
// Without needTools a failed listing leaves tools nil.
func (s *Service) reviewInputs(ctx context.Context, ws memory.WorkspaceID, name string, needTools bool) (mcpconn.Connection, eviedb.WorkspaceAccessRevision, []mcpconn.Tool, error) {
	if err := mcpconn.ValidateName(name); err != nil {
		return mcpconn.Connection{}, eviedb.WorkspaceAccessRevision{}, nil, err
	}
	connection, err := s.store.MCPConnection(ctx, mcpconn.ConnectionID(name))
	if err != nil {
		return mcpconn.Connection{}, eviedb.WorkspaceAccessRevision{}, nil, err
	}
	current, err := s.store.WorkspaceAccess(ctx, ws)
	if err != nil {
		return mcpconn.Connection{}, eviedb.WorkspaceAccessRevision{}, nil, err
	}
	tools, err := s.tools.ListTools(ctx, connection.ID)
	if err != nil {
		if needTools {
			return mcpconn.Connection{}, eviedb.WorkspaceAccessRevision{}, nil, err
		}
		tools = nil
	}
	return connection, current, tools, nil
}

func review(connection mcpconn.Connection, access eviedb.WorkspaceAccessRevision, tools []mcpconn.Tool) Review {
	reviewed := map[string]map[string]bool{} // name -> hashes
	for _, c := range access.Access.Connections {
		if c.ID != connection.ID {
			continue
		}
		for _, r := range c.Resources {
			name, hash := mcpconn.SplitSelector(r)
			if reviewed[name] == nil {
				reviewed[name] = map[string]bool{}
			}
			reviewed[name][hash] = true
		}
	}
	out := Review{Connection: connection, AccessRevision: access.Revision, Tools: []ReviewedTool{}}
	offered := map[string]bool{}
	for _, tool := range tools {
		offered[tool.Name] = true
		state := ToolAvailable
		switch {
		case !SelectableName(tool.Name):
			state = ToolUnselectable
		case reviewed[tool.Name][tool.Hash]:
			state = ToolEnabled
		case reviewed[tool.Name] != nil:
			state = ToolChanged
		}
		out.Tools = append(out.Tools, ReviewedTool{Tool: tool, State: state})
	}
	if tools == nil {
		// The server could not be listed; only the reviewed names are known.
		out.Unlisted = true
		for name := range reviewed {
			out.Reviewed = append(out.Reviewed, name)
		}
		slices.Sort(out.Reviewed)
		return out
	}
	for name := range reviewed {
		if !offered[name] {
			out.NoLongerOffered = append(out.NoLongerOffered, name)
		}
	}
	slices.Sort(out.NoLongerOffered)
	return out
}

// SelectableName reports whether a server tool's name may be enabled: 1-64
// characters with no whitespace or control characters.
func SelectableName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}
