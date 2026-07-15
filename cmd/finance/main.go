package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func usage() {
	fmt.Fprintln(os.Stderr, `usage: finance <command>

commands:
  link    link a bank via hosted Plaid Link
  sync    pull new transactions for all linked banks
  db      sanity-check the database
  help    show this message`)
}

func main() {
	// Load creds from the repo-root .env when run from cmd/finance;
	// falls back to a local .env. Missing file is fine (ignored).
	_ = godotenv.Load("../../.env", ".env")

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "link":
		if err := runLink(); err != nil {
			log.Fatal(err)
		}
	case "sync":
		db, err := openDB()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		if err := runSync(db); err != nil {
			log.Fatal(err)
		}
	case "rules":
		db, err := openDB()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		if err := runRulesSeed(db, "data/merchantLookup.json"); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Seeded rules from data/merchantLookup.json")
	case "categorize":
		db, err := openDB()
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		matched, unmatched, err := runCategorize(db)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%d categorized by rule, %d uncategorized\n", matched, unmatched)
	case "db":
		db, err := openDB()
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
