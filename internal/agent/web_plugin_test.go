package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestWebPluginScriptedConversationRoundTrips(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Web plugin fetch result"))
	}))
	defer server.Close()

	tests := []struct {
		name       string
		toolName   string
		arguments  string
		wantResult string
		wantError  bool
	}{
		{
			name:       "fetch",
			toolName:   "web_fetch",
			arguments:  fmt.Sprintf(`{"url":%q}`, server.URL),
			wantResult: "Web plugin fetch result",
		},
		{
			name:       "search",
			toolName:   "web_search",
			arguments:  `{"query":""}`,
			wantResult: "query must not be empty",
			wantError:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := plugins.NewManager(tools.NewToolset(nil), plugins.NewWeb())
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.SetEnabled(plugins.WebPluginID, true); err != nil {
				t.Fatal(err)
			}
			toolset, err := manager.NewSessionToolset()
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeClient{steps: []step{
				assistantStep("", nil, toolCall("call-1", tc.toolName, tc.arguments)),
				assistantStep("done", nil),
			}}
			session := NewWithToolset(
				client,
				testContextProfile("test-model"),
				&fakeHistory{},
				memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "web-plugin-test"},
				newFakeTurnOwner(),
				toolset,
			)

			if err := session.Send(context.Background(), "use the web", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			if len(client.reqs) != 2 {
				t.Fatalf("provider requests = %d, want 2", len(client.reqs))
			}
			wantSchemas := tools.NewToolset(tools.WebTools()).Schemas()
			for i, request := range client.reqs {
				if !reflect.DeepEqual(request.Tools, wantSchemas) {
					t.Fatalf("request %d schemas = %#v, want Web schemas %#v", i, request.Tools, wantSchemas)
				}
			}
			messages := client.reqs[1].Messages
			result := messages[len(messages)-1]
			if result.Role != "tool" || result.ToolCallID != "call-1" ||
				!strings.Contains(result.Content, tc.wantResult) {
				t.Fatalf("tool result = %+v, want content containing %q", result, tc.wantResult)
			}
			if gotError := strings.HasPrefix(result.Content, "tool call came back with error"); gotError != tc.wantError {
				t.Fatalf("tool result error = %v, want %v: %+v", gotError, tc.wantError, result)
			}
		})
	}
}
