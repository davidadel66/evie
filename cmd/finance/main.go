// Command finance is the CLI frontend over internal/finance: link banks
// via Plaid, sync transactions, seed categorization rules, and
// sanity-check the database. All user-facing output formatting lives
// here; the domain package returns data and this file decides how it
// prints.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/davidadel66/moussa/internal/finance"
	"github.com/joho/godotenv"
)

// usage prints the command summary to stderr — it accompanies exit-code-2
// "you called this wrong" paths, so it belongs on the error stream.
func usage() {
	fmt.Fprintln(os.Stderr, `usage: finance <command>

commands:
  link        link a bank via hosted Plaid Link
  sync        pull new transactions for all linked banks
  rules       seed categorization rules from data/merchantLookup.json
  categorize  apply rules to unreviewed transactions
  db          sanity-check the database
  help        show this message`)
}

// main dispatches on the subcommand, opens the database for the commands
// that need it, and renders domain results. Plaid credentials load from
// the repo-root .env when run from cmd/finance, falling back to a local
// .env; a missing file is fine and silently ignored.
func main() {
	_ = godotenv.Load("../../.env", ".env")

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "link":
		if err := finance.Link(); err != nil {
			log.Fatal(err)
		}
	case "sync":
		db, err := finance.OpenDB()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		res, err := finance.Sync(db)
		if err != nil {
			log.Fatal(err)
		}
		var errs []error
		for _, b := range res.Banks {
			for _, w := range b.Warnings {
				fmt.Printf("warning: %s\n", w)
			}
			if b.Err != nil {
				fmt.Printf("%s: sync failed: %v\n", b.Label, b.Err)
				errs = append(errs, fmt.Errorf("%s: %w", b.Label, b.Err))
				continue
			}
			fmt.Printf("%s: %d added, %d modified, %d removed\n", b.Label, b.Counts.Added, b.Counts.Modified, b.Counts.Removed)
		}
		fmt.Printf("Total: %d added, %d modified, %d removed\n", res.Totals.Added, res.Totals.Modified, res.Totals.Removed)
		if err := errors.Join(errs...); err != nil {
			log.Fatal(err)
		}
	case "rules":
		db, err := finance.OpenDB()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		if err := finance.RulesSeed(db, "data/merchantLookup.json"); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Seeded rules from data/merchantLookup.json")
	case "categorize":
		db, err := finance.OpenDB()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		matched, unmatched, err := finance.Categorize(db)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%d categorized by rule, %d uncategorized\n", matched, unmatched)
	case "db":
		db, err := finance.OpenDB()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		fmt.Println("db ok: ~/.finance/finance.db (tables: items, transactions)")
	case "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "finance: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}
