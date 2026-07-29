package tools

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/davidadel66/moussa/internal/finance"
	"github.com/davidadel66/moussa/internal/openrouter"
)

// queryDBTool describes query_db to the model: free-form read-only SQL
// against a registered database — the read twin of edit_db, ungated
// because the connection is read-only at the engine level.
var queryDBTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "query_db",
		Description: `Run a read-only SQL SELECT against a registered SQLite database and get the results as a table. Filtering, aggregating (SUM/COUNT/GROUP BY), and joining are all fine. Use edit_db for writes.

Databases: "finance" — personal finance. Schema:
  transactions(transaction_id, item_id, account_id, date TEXT 'YYYY-MM-DD', name, merchant_name, amount_cents INTEGER (positive = money out), plaid_category, category (legacy — do not use), category_source, reviewed INTEGER 0/1, pending INTEGER 0/1, tags JSON array)
  budget_entries(id, transaction_id, category, amount_cents, source 'rule'|'human', tags JSON array) — where money went; every categorized transaction has one entry with its full amount, a split bill has several entries summing to the total, refunds are negative entries that net the category down
  budget_limits(category, month, limit_cents) — month NULL is the standing monthly template, month 'YYYY-MM' overrides it for that month
  categories(name)
  rules(id, merchant, category)

Notes: amounts are integer cents. Spend per category = SUM(budget_entries.amount_cents) joined to transactions for the date. Awaiting-review transactions are those with NO budget_entries row. The items table is off-limits.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"db", "query"},
			Properties: map[string]openrouter.Property{
				"db": {
					Type:        "string",
					Enum:        []string{"finance"},
					Description: "Which registered database to query.",
				},
				"query": {
					Type:        "string",
					Description: "A single SQL SELECT statement. No writes — the connection is read-only.",
				},
			},
		},
	},
}

// queryDB runs the model's SQL through the named database's read-only
// connection and renders the result as a pipe-separated table. Fences
// per database as in editDB; the SELECT-prefix check and engine-level
// mode=ro are layered defenses.
func queryDB(args string) (string, error) {
	var params struct {
		DB    string `json:"db"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	q := strings.TrimSpace(params.Query)
	if !strings.HasPrefix(strings.ToUpper(q), "SELECT") {
		return "", fmt.Errorf("only SELECT statements are allowed")
	}

	var db *sql.DB
	var err error
	switch params.DB {
	case "finance":
		if strings.Contains(strings.ToLower(q), "items") {
			return "", fmt.Errorf("the items table is off-limits")
		}
		db, err = finance.OpenDBReadOnly()
	default:
		return "", fmt.Errorf("unknown db %q — registered databases: finance", params.DB)
	}
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

// editDBTool describes edit_db to the model: free-form write SQL against
// a registered database, gated behind David's explicit approval per call
// (NeedsApproval in the registry). The db parameter is an enum of
// registered names, never a path — the model doesn't invent filesystem
// locations. Adding a future database means one case in editDB and one
// enum value here; the tool itself belongs to no domain.
var editDBTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "edit_db",
		Description: `Run a write statement (INSERT, UPDATE, DELETE) against a registered SQLite database. Every call is shown to David for approval before it executes — keep statements small and targeted, and explain what you're about to change before calling this. Use query_db for reads.

Databases: "finance" — the personal finance db (same schema as query_db; the items table is off-limits).

Common uses: categorize a transaction (INSERT INTO budget_entries with the full amount_cents and source='human'; insert the category into categories first if new), split a bill (several budget_entries rows summing to the transaction total), set or override a monthly limit in budget_limits, add a rule, tag an entry.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"db", "statement"},
			Properties: map[string]openrouter.Property{
				"db": {
					Type:        "string",
					Enum:        []string{"finance"},
					Description: "Which registered database to write to.",
				},
				"statement": {
					Type:        "string",
					Description: "A single SQL write statement. Executed only after David approves it.",
				},
			},
		},
	},
}

// editDB executes one approved write statement on the named database and
// reports rows affected. Approval protects against bad writes; the
// per-database fences (finance: no items table — bank tokens must never
// enter the conversation) protect secrets; neither substitutes for the
// other.
func editDB(args string) (string, error) {
	var params struct {
		DB        string `json:"db"`
		Statement string `json:"statement"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	q := strings.TrimSpace(params.Statement)

	var db *sql.DB
	var err error
	switch params.DB {
	case "finance":
		if strings.Contains(strings.ToLower(q), "items") {
			return "", fmt.Errorf("the items table is off-limits")
		}
		db, err = finance.OpenDB()
	default:
		return "", fmt.Errorf("unknown db %q — registered databases: finance", params.DB)
	}
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	res, err := db.Exec(q)
	if err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("rows affected: %w", err)
	}
	return fmt.Sprintf("OK — %d row(s) affected\n", n), nil
}
