// Command evie is the agent harness — the nucleus of this repo. The
// loop itself lives in internal/agent; this binary is the thin door:
// dispatch on the subcommand, wire up the provider client, and hand the
// session to a frontend. No args runs the terminal REPL (repl.go);
// "serve" will run the web frontend once internal/web exists.
package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"

	"github.com/joho/godotenv"
)

func main() {
	// Capture the user's shell environment in the background now, so the
	// first bash call doesn't pay for it mid-conversation.
	tools.Warm()

	// Config home is ~/.evie/.env — the binary lives on PATH, so cwd
	// can't be trusted. The cwd-relative loads stay as a dev convenience
	// (running from the repo root or cmd/evie). Separate calls, not one
	// variadic Load — godotenv aborts on the first missing file instead of
	// trying the next; it never overrides variables the shell already set.
	if home, err := os.UserHomeDir(); err == nil {
		_ = godotenv.Load(filepath.Join(home, ".evie", ".env"))
	}
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../../.env")

	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	// cron-exec runs headless under launchd with no API key — it must
	// never pay for (or die on) client construction, so the session is
	// built inside the arms that talk to the model.
	newSession := func(selectStoredSession func(*eviedb.Store) (memory.Session, error)) *agent.Session {
		client, err := openrouter.NewClient(os.Getenv("OPENROUTER_API_KEY"))
		if err != nil {
			log.Fatalf("failed to create client: %v", err)
		}

		db, err := eviedb.OpenDB()
		if err != nil {
			log.Fatalf("failed to open Evie database: %v", err)
		}
		store := eviedb.NewStore(db)

		storedSession, err := selectStoredSession(store)
		if err != nil {
			log.Fatalf("failed to create session: %v", err)
		}

		return agent.New(
			client,
			"",
			store.BindHistory(storedSession.ID),
			storedSession.ScopeContext(),
		)
	}

	switch cmd {
	case "":
		launchDir, err := os.Getwd()
		if err != nil {
			log.Fatalf("failed to read launch directory: %v", err)
		}
		scanner := bufio.NewScanner(os.Stdin)
		session := newSession(func(store *eviedb.Store) (memory.Session, error) {
			return selectREPLSession(context.Background(), store, launchDir, scanner, os.Stdout)
		})
		runREPL(session, scanner)
	case "serve":
		globalSession := newSession(func(store *eviedb.Store) (memory.Session, error) {
			return store.CreateGlobalSession(context.Background())
		})
		if err := web.Serve(globalSession); err != nil {
			log.Fatalf("serve: %v", err)
		}
	case "cron-exec":
		runCronExec(os.Args[2:])
	default:
		log.Fatalf("unknown command %q (usage: evie [serve|cron-exec <job-id>])", cmd)
	}
}
