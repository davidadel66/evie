package tools

import (
	"fmt"

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
