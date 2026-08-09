package tools

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/openrouter"
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
		Description: "Apply stored merchant-to-category rules to every transaction that has no budget entry yet, creating one budget_entries row (full amount) per match. Returns how many entries were created and how many transactions remain awaiting review. Takes no arguments.",
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
