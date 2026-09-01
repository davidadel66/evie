package tools

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/openrouter"
)

func TestFinanceToolsPreserveLegacySchemasAndExecution(t *testing.T) {
	t.Setenv("PLAID_CLIENT_ID", "")
	t.Setenv("PLAID_SECRET", "")

	extracted := NewToolset(FinanceTools())
	wantNames := []string{"finance_sync", "finance_rules", "finance_categorize"}
	if got := toolSchemaNames(extracted.Schemas()); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("Finance schema names = %v, want %v", got, wantNames)
	}

	legacySchemas := schemasNamed(BuiltinToolset().Schemas(), wantNames)
	if got := extracted.Schemas(); !reflect.DeepEqual(got, legacySchemas) {
		t.Fatalf("extracted Finance schemas changed\n got: %#v\nwant: %#v", got, legacySchemas)
	}
	wantLegacyNames := []string{
		"get_time", "todo_list", "todo_add",
		"finance_sync", "finance_rules", "finance_categorize",
		"youtube_transcript", "youtube_scrape_channel",
		"query_db", "edit_db", "read_file", "edit_file", "bash",
		"cron_add", "cron_list", "cron_remove",
		"web_fetch", "web_search",
	}
	if got := toolSchemaNames(BuiltinToolset().Schemas()); !reflect.DeepEqual(got, wantLegacyNames) {
		t.Fatalf("legacy built-in schema names = %v, want %v", got, wantLegacyNames)
	}

	for _, surface := range []struct {
		name    string
		toolset Toolset
	}{
		{name: "legacy", toolset: BuiltinToolset()},
		{name: "extracted", toolset: extracted},
	} {
		t.Run(surface.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			financeDir := filepath.Join(home, ".finance")
			if err := os.MkdirAll(financeDir, 0o700); err != nil {
				t.Fatal(err)
			}
			rulesPath := filepath.Join(financeDir, "merchantLookup.json")
			if err := os.WriteFile(rulesPath, []byte(`{"Coffee Shop":"Coffee"}`), 0o600); err != nil {
				t.Fatal(err)
			}

			assertFinanceToolResult(t, surface.toolset, "finance_rules", "Seeded rules from "+rulesPath+"\n", false)
			seedFinanceTransaction(t)
			assertFinanceToolResult(t, surface.toolset, "finance_categorize", "1 categorized by rule, 0 uncategorized\n", false)
			assertFinanceToolResult(t, surface.toolset, "finance_sync", "tool call came back with error set PLAID_CLIENT_ID and PLAID_SECRET to your environment", true)
		})
	}
}

func seedFinanceTransaction(t *testing.T) {
	t.Helper()
	db, err := finance.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO items (item_id, access_token, linked_at) VALUES ('item-1', 'token', 'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO transactions
		(transaction_id, item_id, merchant_name, amount_cents)
		VALUES ('txn-1', 'item-1', 'Coffee Shop', 350)`); err != nil {
		t.Fatal(err)
	}
}

func assertFinanceToolResult(t *testing.T, toolset Toolset, name, wantContent string, wantIsErr bool) {
	t.Helper()
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(), openrouter.ToolCall{
			ID: "call-1", Type: "function",
			Function: openrouter.FunctionCall{Name: name, Arguments: `{}`},
		}, nil, nil, nil, nil,
	)
	if err != nil || isErr != wantIsErr || message.Content != wantContent {
		t.Fatalf("execute %q = (%+v, %v, %v), want content %q and tool error %v", name, message, isErr, err, wantContent, wantIsErr)
	}
}

func schemasNamed(schemas []openrouter.Tool, names []string) []openrouter.Tool {
	selected := make([]openrouter.Tool, 0, len(names))
	for _, name := range names {
		for _, schema := range schemas {
			if schema.Function.Name == name {
				selected = append(selected, schema)
				break
			}
		}
	}
	return selected
}

func toolSchemaNames(schemas []openrouter.Tool) []string {
	names := make([]string, len(schemas))
	for i, schema := range schemas {
		names[i] = schema.Function.Name
	}
	return names
}
