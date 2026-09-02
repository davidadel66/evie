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
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"
	"github.com/google/uuid"

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
	if cmd == "cron-exec" {
		runCronExec(os.Args[2:])
		return
	}

	db, err := eviedb.OpenDB()
	if err != nil {
		log.Fatalf("failed to open Evie database: %v", err)
	}
	defer db.Close()
	kernelStore := eviedb.NewStore(db)
	pluginManager, err := plugins.NewManager(
		tools.KernelToolset(),
		plugins.NewWeb(),
		plugins.NewFinance(),
		plugins.NewYouTube(),
		plugins.NewTodo(),
		plugins.NewMemory(kernelStore),
	)
	if err != nil {
		log.Fatalf("failed to load compiled plugins: %v", err)
	}
	if err := pluginManager.ConfigureEnabledState(context.Background(), kernelStore, map[plugins.PluginID]bool{
		plugins.WebPluginID: true, plugins.FinancePluginID: true,
		plugins.YouTubePluginID: true, plugins.TodoPluginID: true, plugins.MemoryPluginID: true,
	}); err != nil {
		log.Fatalf("failed to apply plugin enabled configuration: %v", err)
	}
	if cmd == "plugins" || cmd == "presets" || cmd == "sessions" {
		handled, err := runManagementCommand(
			context.Background(), os.Args[1:], os.Stdout, pluginManager, kernelStore,
		)
		if err != nil {
			log.Fatalf("management command: %v", err)
		}
		if handled {
			return
		}
	}

	// cron-exec runs headless under launchd with no API key — it must
	// never pay for (or die on) client construction, so the session is
	// built inside the arms that talk to the model.
	newSession := func(selectStoredSession func(*eviedb.Store, plugins.ResolvedComposition) (storedSessionSelection, error)) *agent.Session {
		defaultComposition, err := pluginManager.ResolvePresetContext(context.Background(), "")
		if err != nil {
			log.Fatalf("failed to resolve default Agent Preset: %v", err)
		}
		client, err := openrouter.NewClient(os.Getenv("OPENROUTER_API_KEY"))
		if err != nil {
			log.Fatalf("failed to create client: %v", err)
		}
		model := os.Getenv("EVIE_MODEL")
		if model == "" {
			model = agent.DefaultModel
		}
		profile, err := client.ResolveContextProfile(context.Background(), model)
		if err != nil {
			log.Fatalf("failed to resolve context profile: %v", err)
		}

		store := kernelStore

		selection, err := selectStoredSession(store, defaultComposition)
		if err != nil {
			log.Fatalf("failed to create session: %v", err)
		}
		holderUUID, err := uuid.NewRandom()
		if err != nil {
			log.Fatalf("failed to create turn lease holder: %v", err)
		}

		holderID := memory.LeaseHolderID(holderUUID.String())
		resolvedComposition, err := bindSessionComposition(
			context.Background(), store, pluginManager, selection.ID, defaultComposition,
			selection.createdComposition,
		)
		if err != nil {
			log.Fatalf("failed to compose session from its Agent Preset: %v", err)
		}
		for _, warning := range resolvedComposition.Warnings {
			log.Printf("session composition warning [%s]: %s", warning.Code, warning.Message)
		}
		return agent.NewWithToolset(
			client,
			profile,
			store.BindHistory(selection.ID, holderID),
			selection.ScopeContext(),
			store.BindTurnOwner(selection.ID, holderID),
			resolvedComposition.Resolved.Toolset,
		)
	}

	switch cmd {
	case "":
		launchDir, err := os.Getwd()
		if err != nil {
			log.Fatalf("failed to read launch directory: %v", err)
		}
		scanner := bufio.NewScanner(os.Stdin)
		session := newSession(func(store *eviedb.Store, resolved plugins.ResolvedComposition) (storedSessionSelection, error) {
			boundStore := &receiptBoundREPLStore{Store: store, composition: resolved}
			selected, err := selectREPLSession(
				context.Background(), boundStore,
				launchDir, scanner, os.Stdout,
			)
			return boundStore.selection(selected), err
		})
		runREPLWithMemory(session, scanner, kernelStore)
	case "serve":
		presetReport, err := pluginManager.ValidatePresetContext(context.Background(), "")
		if err != nil {
			log.Fatalf("failed to refresh plugin enabled configuration: %v", err)
		}
		if !presetReport.Valid {
			log.Printf("starting management-only web server: default Agent Preset is invalid: %v", presetReport.Errors)
			if err := web.ServeManaged(nil, pluginManager, kernelStore); err != nil {
				log.Fatalf("serve degraded management: %v", err)
			}
			break
		}
		client, err := openrouter.NewClient(os.Getenv("OPENROUTER_API_KEY"))
		if err != nil {
			log.Fatalf("failed to create client: %v", err)
		}
		model := os.Getenv("EVIE_MODEL")
		if model == "" {
			model = agent.DefaultModel
		}
		profile, err := client.ResolveContextProfile(context.Background(), model)
		if err != nil {
			log.Fatalf("failed to resolve context profile: %v", err)
		}
		controller := newWebContextSessionController(kernelStore, pluginManager, func(
			session memory.Session,
			composition plugins.ResolvedComposition,
		) (*agent.Session, error) {
			holderUUID, err := uuid.NewRandom()
			if err != nil {
				return nil, err
			}
			holderID := memory.LeaseHolderID(holderUUID.String())
			return agent.NewWithToolset(
				client, profile,
				kernelStore.BindHistory(session.ID, holderID), session.ScopeContext(),
				kernelStore.BindTurnOwner(session.ID, holderID), composition.Toolset,
			), nil
		})
		if err := web.ServeContextManaged(pluginManager, kernelStore, controller, kernelStore); err != nil {
			log.Fatalf("serve: %v", err)
		}
	default:
		log.Fatalf("unknown command %q (usage: evie [serve|cron-exec <job-id>|plugins ...|presets ...|sessions ...])", cmd)
	}
}
