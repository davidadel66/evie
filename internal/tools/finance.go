package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/davidadel66/moussa/internal/finance"
	"github.com/davidadel66/moussa/internal/openrouter"
)

// financeSyncTool describes finance_sync to the model: pull new
// transactions for every linked bank, no parameters.
var financeSyncTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "finance_sync",
		Description: "Pull new transactions from every linked bank into the local finance database. Returns per-bank counts of added/modified/removed transactions plus totals. Takes no arguments.",
		Parameters: openrouter.Parameter{
			Type:       "object",
			Properties: map[string]openrouter.Property{},
		},
	},
}

// financeQueryTool describes finance_query to the model: free-form
// read-only SQL over the finance database. The schema in the description
// deliberately omits the items table — it holds bank access tokens and
// queries touching it are rejected.
var financeQueryTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "finance_query",
		Description: `Run a read-only SQL SELECT against the personal finance SQLite database and get the results as a table. Use this to answer any question about transactions, categories, rules, or budgets — filtering, aggregating (SUM/COUNT/GROUP BY), and joining are all fine.

Schema:
  transactions(transaction_id, item_id, account_id, date TEXT 'YYYY-MM-DD', name, merchant_name, amount_cents INTEGER (positive = money out), plaid_category, category, category_source, reviewed INTEGER 0/1, pending INTEGER 0/1, tags)
  categories(name)
  rules(id, merchant, category)

Notes: amounts are integer cents. Awaiting-review transactions are reviewed = 0 AND category IS NULL. The items table is off-limits.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]openrouter.Property{
				"query": {
					Type:        "string",
					Description: "A single SQL SELECT statement. No writes — the connection is read-only.",
				},
			},
		},
	},
}

// financeQuery runs the model's SQL through a read-only connection and
// renders the result as a pipe-separated table. Two fences, layered:
// mode=ro at the SQLite engine level makes writes impossible regardless
// of the SQL, and a crude text check rejects anything that isn't a
// SELECT or that mentions the items table (bank access tokens live
// there; they must never enter the conversation, which flows through a
// remote model provider).
func financeQuery(args string) (string, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	q := strings.TrimSpace(params.Query)
	if !strings.HasPrefix(strings.ToUpper(q), "SELECT") {
		return "", fmt.Errorf("only SELECT statements are allowed")
	}
	if strings.Contains(strings.ToLower(q), "items") {
		return "", fmt.Errorf("the items table is off-limits")
	}

	db, err := finance.OpenDBReadOnly()
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	columns, rows, err := finance.Query(db, q)
	if err != nil {
		return "", err
	}

	out := strings.Join(columns, " | ") + "\n"
	for _, row := range rows {
		out += strings.Join(row, " | ") + "\n"
	}
	out += fmt.Sprintf("(%d rows)\n", len(rows))
	return out, nil
}

// financeSync opens the finance database, syncs all linked banks, and
// renders the SyncResult as text. One bank failing is reported inside the
// result string, not as an error — the sync as a whole still ran, and
// text tells the model exactly which bank failed while still showing the
// banks that succeeded. The error return is reserved for "nothing ran at
// all" (no db, no credentials, no linked banks).
func financeSync(_ string) (string, error) {
	db, err := finance.OpenDB()
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	res, err := finance.Sync(db)
	if err != nil {
		return "", err
	}

	out := ""
	for _, b := range res.Banks {
		for _, w := range b.Warnings {
			out += fmt.Sprintf("warning: %s\n", w)
		}
		if b.Err != nil {
			out += fmt.Sprintf("%s: sync failed: %v\n", b.Label, b.Err)
			continue
		}
		out += fmt.Sprintf("%s: %d added, %d modified, %d removed\n", b.Label, b.Counts.Added, b.Counts.Modified, b.Counts.Removed)
	}
	out += fmt.Sprintf("Total: %d added, %d modified, %d removed\n", res.Totals.Added, res.Totals.Modified, res.Totals.Removed)
	return out, nil
}

// financeRulesTool describes finance_rules to the model: reload the
// categorization rules from the canonical lookup file, no parameters.
var financeRulesTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "finance_rules",
		Description: "Reload merchant-to-category rules into the finance database from ~/.finance/merchantLookup.json. Run this after the lookup file changes. Takes no arguments.",
		Parameters: openrouter.Parameter{
			Type:       "object",
			Properties: map[string]openrouter.Property{},
		},
	},
}

// financeRules seeds the rules table from the canonical lookup file —
// the same hardcoded path the CLI uses. The model gets no path parameter
// on purpose: it has no business inventing file paths.
func financeRules(_ string) (string, error) {
	db, err := finance.OpenDB()
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home dir: %w", err)
	}
	rulesPath := filepath.Join(home, ".finance", "merchantLookup.json")

	if err := finance.RulesSeed(db, rulesPath); err != nil {
		return "", err
	}
	return fmt.Sprintf("Seeded rules from %s\n", rulesPath), nil
}

// financeCategorizeTool describes finance_categorize to the model: apply
// the stored rules to uncategorized transactions, no parameters.
var financeCategorizeTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "finance_categorize",
		Description: "Apply stored merchant-to-category rules to all unreviewed transactions in the finance database. Returns how many transactions were categorized and how many remain uncategorized. Takes no arguments.",
		Parameters: openrouter.Parameter{
			Type:       "object",
			Properties: map[string]openrouter.Property{},
		},
	},
}

// financeCategorize runs rule-based categorization and reports the counts.
// Human-reviewed and hand-categorized transactions are never touched —
// that guarantee lives in the domain query, not here.
func financeCategorize(_ string) (string, error) {
	db, err := finance.OpenDB()
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	matched, unmatched, err := finance.Categorize(db)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d categorized by rule, %d uncategorized\n", matched, unmatched), nil
}
