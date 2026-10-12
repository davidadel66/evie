package mcpchat_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/mcpchat"
	"github.com/davidadel66/evie/internal/mcpconn"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type staticTokens string

func (s staticTokens) Token(context.Context) (string, error) { return string(s), nil }
func (s staticTokens) Refresh(context.Context, string) (string, error) {
	return "", mcpconn.ErrNotConnected
}

func receipt() composition.Receipt {
	hash := strings.Repeat("a", 64)
	return composition.Receipt{FormatVersion: composition.FormatVersion, Preset: composition.PresetIdentity{ID: "standard", Version: "sha256:" + hash},
		EvieVersion: "1.0.0", Providers: []composition.Provider{{ID: "fixture", ImplementationVersion: "1.0.0"}},
		Capabilities: []composition.Capability{{ID: "fixture.echo", ProviderID: "fixture", ContractVersion: "1.0.0", SchemaSHA256: hash}},
		ToolSchemas:  []composition.ToolSchema{{Name: "fixture_echo", SHA256: hash}}}
}

// workspace-mcp-connections.spec.md story 2, end to end: Workspace Access
// in SQLite, a chat's pinned access revision, the live pool against a real
// MCP server, and the chat tools.
func TestWorkspaceChatReachesOnlyItsReviewedMCPTools(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "square-like", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "echo", Description: "Echo the text.", InputSchema: map[string]any{"type": "object"}},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			var args struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(req.Params.Arguments, &args)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + args.Text}}}, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+bearer {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	db, err := eviedb.OpenDBAt(filepath.Join(dir, "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "CairosKitchen")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.CreateMCPConnection(ctx, mcpconn.Connection{ID: "mcp-square", Name: "square", URL: srv.URL + "/mcp",
		Status: mcpconn.StatusConnected, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	pool := mcpconn.NewPool(mcpconn.PoolConfig{Records: store, Tokens: func(string) mcpconn.TokenSource { return staticTokens(bearer) }, Base: srv.Client().Transport})
	defer pool.CloseAll()
	service := mcpchat.New(store, pool)
	link := &plugins.MCPLink{}
	link.Bind(service)
	chatTools := map[string]tools.Tool{}
	for _, capability := range plugins.NewMCP(link).ToolCapabilities() {
		chatTools[capability.Tool.Schema.Function.Name] = capability.Tool
	}
	turn := func(session memory.Session) context.Context {
		return tools.WithInvocationContext(ctx, tools.InvocationContext{Scope: memory.ScopeContext{
			OwnerID: memory.LocalOwnerID, SessionID: session.ID, WorkspaceID: ws.ID}})
	}
	callEcho := `{"connection":"square","tool":"echo","arguments":{"text":"hi"}}`

	early, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, receipt())
	if err != nil {
		t.Fatal(err)
	}
	review, err := service.SetToolEnabled(ctx, ws.ID, "square", "echo", true)
	if err != nil {
		t.Fatal(err)
	}
	if review.Tools[0].State != mcpchat.ToolEnabled {
		t.Fatalf("review = %+v", review)
	}
	fresh, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, receipt())
	if err != nil {
		t.Fatal(err)
	}

	listing, err := chatTools[plugins.MCPListToolsToolName].Execute(turn(fresh), `{}`)
	if err != nil || !strings.Contains(listing, `"echo"`) || !strings.Contains(listing, "Echo the text.") {
		t.Fatalf("fresh chat listing = %q, %v", listing, err)
	}
	if ask, err := chatTools[plugins.MCPCallToolName].RequiresApproval(turn(fresh), callEcho); err != nil || !ask {
		t.Fatalf("approval = %v, %v; every call asks in story 2", ask, err)
	}
	out, err := chatTools[plugins.MCPCallToolName].Execute(turn(fresh), callEcho)
	if err != nil || !strings.Contains(out, "echo: hi") {
		t.Fatalf("fresh chat call = %q, %v", out, err)
	}

	outputs := []string{listing, out}

	// A chat started before the tool was enabled does not reach it.
	_, err = chatTools[plugins.MCPCallToolName].Execute(turn(early), callEcho)
	if err == nil || !strings.Contains(err.Error(), "not connected to this Workspace") {
		t.Fatalf("early chat call error = %v", err)
	}
	outputs = append(outputs, err.Error())

	// David disables the tool while its approval card is open, then
	// approves: through the real dispatcher, nothing is sent.
	approve := func(context.Context, string, string, *tools.FileChangePreview) tools.Decision {
		if _, err := service.SetToolEnabled(ctx, ws.ID, "square", "echo", false); err != nil {
			t.Error(err)
		}
		return tools.Approved
	}
	toolset := tools.NewToolset([]tools.Tool{chatTools[plugins.MCPCallToolName]})
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(turn(fresh),
		openrouterCall(callEcho), approve, nil, nil, nil)
	if err != nil || !isErr || !strings.Contains(message.Content, "not connected to this Workspace") {
		t.Fatalf("call removed while its card was open = %q, %v, %v", message.Content, isErr, err)
	}
	outputs = append(outputs, message.Content)
	if n := calls.Load(); n != 1 {
		t.Fatalf("the server ran %d calls, want only the one allowed call", n)
	}

	// An archived Workspace's tools cannot change.
	if _, err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetToolEnabled(ctx, ws.ID, "square", "echo", true); !errors.Is(err, eviedb.ErrWorkspaceInactive) {
		t.Fatalf("enabling in an archived Workspace: %v", err)
	}

	// The bearer token reached none of the tool output and none of the
	// SQLite files.
	for _, text := range outputs {
		if strings.Contains(text, bearer) {
			t.Fatalf("tool output carries the bearer token: %q", text)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "evie.db*"))
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(content, []byte(bearer)) {
			t.Fatalf("%s carries the bearer token", filepath.Base(file))
		}
	}
}

const bearer = "secret-bearer-token-123"

func openrouterCall(arguments string) openrouter.ToolCall {
	return openrouter.ToolCall{ID: "call-1", Function: openrouter.FunctionCall{Name: plugins.MCPCallToolName, Arguments: arguments}}
}
