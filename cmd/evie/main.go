// Command evie is the agent harness — the nucleus of this repo. The
// loop itself lives in internal/agent; this binary is the thin door:
// dispatch on the subcommand, wire up the provider client, and hand the
// session to a frontend. No args runs the terminal REPL (repl.go);
// "serve" will run the web frontend once internal/web exists.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/usage"
	"github.com/davidadel66/evie/internal/web"
	"github.com/google/uuid"

	"github.com/joho/godotenv"
)

// defaultPluginEnabled is each compiled plugin's enabled state until the
// owner chooses otherwise; the owner's choice is durable (the default is
// seeded once per database, so changing it here reaches only databases
// that never resolved the plugin).
// The Workflows plugin (Stage 14 sub-stage 10) is on: its chat tools start
// only workflows the owner approved or built into Evie, inside every gate a
// start has. The Subagents plugin is on since the built-in decision
// (2026-10-04, subagents.decisions.md "Subagents on by default for built-in
// deep research"): the built-in deep-research runs research workers, which
// need it, and each Workspace's research allowance (off by default) still
// gates them.
var defaultPluginEnabled = map[plugins.PluginID]bool{
	plugins.WebPluginID: true, plugins.FinancePluginID: true,
	plugins.YouTubePluginID: true, plugins.TodoPluginID: true, plugins.MemoryPluginID: true,
	plugins.SubagentsPluginID: true, plugins.WorkflowsPluginID: true, plugins.GitHubPluginID: true,
}

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
	// Workflow review needs no database: it reads one definition directory.
	if handled, err := runWorkflowManagement(context.Background(), os.Args[1:], os.Stdout, os.Getenv); handled {
		if err != nil {
			log.Fatalf("workflows: %v", err)
		}
		return
	}

	db, err := eviedb.OpenDB()
	if err != nil {
		log.Fatalf("failed to open Evie database: %v", err)
	}
	defer db.Close()
	kernelStore := eviedb.NewStore(db)
	if handled, err := runCompilerDiagnostics(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {
		if err != nil {
			log.Fatalf("memory compiler diagnostics: %v", err)
		}
		return
	}
	if handled, err := runCompilerHistoryManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {
		if err != nil {
			log.Fatalf("memory compiler history: %v", err)
		}
		return
	}
	if handled, err := runCompilerActivationManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {
		if err != nil {
			log.Fatalf("memory compiler activation: %v", err)
		}
		return
	}
	if handled, err := runCompilerManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {
		if err != nil {
			log.Fatalf("memory compiler: %v", err)
		}
		return
	}
	if handled, err := runOwnerReviewManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {
		if err != nil {
			log.Fatalf("memory review: %v", err)
		}
		return
	}
	if handled, err := runMemoryBackgroundManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore, os.Getenv); handled {
		if err != nil {
			log.Fatalf("memory background pass: %v", err)
		}
		return
	}
	if handled, err := runWorkspaceManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {
		if err != nil {
			log.Fatalf("workspaces: %v", err)
		}
		return
	}
	if handled, err := runProjectManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {
		if err != nil {
			log.Fatalf("projects: %v", err)
		}
		return
	}
	if handled, err := runWorktreeManagement(context.Background(), os.Args[1:], os.Stdout, kernelStore); handled {
		if err != nil {
			log.Fatalf("worktrees: %v", err)
		}
		return
	}
	if handled, err := runChangeManagement(context.Background(), os.Args[1:], os.Stdout, changeCommands{store: kernelStore, repositories: repositoryReader, getenv: os.Getenv,
		gate: func() (changes.BuildGate, error) {
			// The owner's merge gate runs in this process; evie serve's
			// recovery leaves its steps alone while they run.
			policy, err := configuredSubagentPolicy(os.Getenv)
			if err != nil {
				return nil, err
			}
			return subagents.New(kernelStore, policy)
		}}); handled {
		if err != nil {
			log.Fatalf("changes: %v", err)
		}
		return
	}
	// Workflow activation (Stage 14 sub-stage 2) needs the database and the
	// procedural repository, and runs (sub-stage 5) the workflow capability
	// plugins under the owner's enabled choices.
	if cmd == "workflows" {
		// Agent steps (sub-stage 9) run their workers through a Kernel
		// subagent supervisor of this command's own, as `evie serve` does.
		policy, err := configuredSubagentPolicy(os.Getenv)
		if err != nil {
			log.Fatalf("invalid Subagents configuration: %v", err)
		}
		workers, err := subagents.New(kernelStore, policy)
		if err != nil {
			log.Fatalf("workflows: %v", err)
		}
		workflowPlugins, err := newWorkflowPluginManager(context.Background(), kernelStore, workers)
		if err != nil {
			log.Fatalf("workflows: %v", err)
		}
		workflowRuntime, err := newWorkflowRuntime(kernelStore, os.Getenv, workflowPlugins, workers, workflowPlugins)
		if err != nil {
			log.Fatalf("workflows: %v", err)
		}
		handled, err := runWorkflowActivation(context.Background(), os.Args[1:], os.Stdout, workflowRuntime)
		stopCtx, stopWorkers := context.WithTimeout(context.Background(), 5*time.Second)
		if stopErr := workers.Stop(stopCtx); stopErr != nil {
			log.Printf("Subagents shutdown: %v", stopErr)
		}
		stopWorkers()
		if handled {
			if err != nil {
				log.Fatalf("workflows: %v", err)
			}
			return
		}
	}
	if _, err := kernelStore.ImportDefaultLegacyTodoList(context.Background()); err != nil {
		log.Fatalf("failed to import legacy Todo list: %v", err)
	}
	if _, err := kernelStore.RecoverInactiveTaskClaims(context.Background()); err != nil {
		log.Fatalf("failed to recover inactive Task claims: %v", err)
	}
	subagentPolicy, err := configuredSubagentPolicy(os.Getenv)
	if err != nil {
		log.Fatalf("invalid Subagents configuration: %v", err)
	}
	supervisor, err := subagents.New(kernelStore, subagentPolicy)
	if err != nil {
		log.Fatalf("initialize Subagents: %v", err)
	}
	// Unrecoverable records are isolated per attempt and retried by the
	// background recovery loop, so they must not block startup.
	if _, err := kernelStore.RecoverSubagents(context.Background()); err != nil {
		log.Printf("recover Subagents: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := supervisor.Stop(ctx); err != nil {
			log.Printf("Subagents shutdown: %v", err)
		}
	}()
	// Workspace Repositories (changes.spec.md): one reader, so Settings,
	// the owner's pushes and workflows' pushes share its folder lock.
	repositories := repositoryReader()
	// The chat's workflow tools reach the Workflow Runtime once the runtime
	// exists; it needs this manager first (sub-stage 10).
	workflowLink := &plugins.WorkflowLink{}
	pluginManager, err := plugins.NewManager(
		tools.KernelToolset(),
		plugins.NewWeb(),
		plugins.NewFinance(),
		plugins.NewYouTube(),
		// todo_ready sizes batches to the configured subagent caps (Stage 13).
		plugins.NewTodo(kernelStore).WithDispatchLimits(task.DispatchLimits{MaxBatch: subagentPolicy.MaxBatch,
			PerParent: subagentPolicy.PerParent, Runtime: subagentPolicy.Runtime, PerTurn: subagentPolicy.PerTurn}),
		plugins.NewMemory(kernelStore),
		plugins.NewSubagents(supervisor).WithGitHubRepositories(githubFolders{repositories: repositories}),
		plugins.NewWorkflows(workflowLink),
		workflowGitHub(kernelStore, repositories),
	)
	if err != nil {
		log.Fatalf("failed to load compiled plugins: %v", err)
	}
	if err := pluginManager.ConfigureEnabledState(context.Background(), kernelStore, defaultPluginEnabled); err != nil {
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

	// The always-on background memory pass (memory architecture Stage 4)
	// runs in long-lived REPL and web processes only. Their conversations
	// offer every committed compaction to it (Stage 10).
	memoryBackground := newMemoryBackgroundHost(os.Getenv, kernelStore)

	// cron-exec runs headless under launchd with no API key — it must
	// never pay for (or die on) client construction, so the session is
	// built inside the arms that talk to the model.
	newSession := func(ctx context.Context, selectStoredSession func(*eviedb.Store, plugins.ResolvedComposition) (storedSessionSelection, error)) *agent.Session {
		defaultComposition, err := pluginManager.ResolvePresetContext(ctx, "")
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
		if err := agent.ValidateModelConfiguration(model); err != nil {
			log.Fatalf("invalid model configuration: %v", err)
		}
		if err := agent.ValidateTurnConfiguration(); err != nil {
			log.Fatalf("invalid turn configuration: %v", err)
		}
		profile, err := client.ResolveContextProfile(ctx, model)
		if err != nil {
			log.Fatalf("failed to resolve context profile: %v", err)
		}
		if warning := contextFallbackWarning(profile.Diagnostics()); warning != "" {
			log.Print(warning)
		}

		configureSubagentRuntime(supervisor, pluginManager, client, profile)
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
			ctx, store, pluginManager, selection.ID, defaultComposition,
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
			agent.WithCompactionMemoryFlush(memoryBackground.runner),
		)
	}
	startMemoryForRuntime := func(ctx context.Context) func() {
		ctx, cancel := context.WithCancel(ctx)
		stopRetrieval := startMemoryRetrievalHost(ctx, kernelStore)
		stopCompiler, err := startConfiguredCompilerHost(ctx, os.Getenv("EVIE_COMPILER_CONFIG"), kernelStore)
		if err != nil {
			log.Printf("memory compiler unavailable: %v", err)
			stopCompiler = func() error { return nil }
		}
		if config := memoryBackground.runner.Config(); !config.Enabled {
			log.Printf("memory background pass off: %s", config.Reason)
		}
		stopBackground := memoryBackground.Start(ctx)
		return func() {
			cancel()
			if err := stopRetrieval(); err != nil {
				log.Printf("memory retrieval shutdown: %v", err)
			}
			if err := stopCompiler(); err != nil {
				log.Printf("memory compiler shutdown: %v", err)
			}
			if err := stopBackground(); err != nil {
				log.Printf("memory background pass shutdown: %v", err)
			}
		}
	}

	switch cmd {
	case "":
		runtimeCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stopSignals()
		stopSubagentRecovery := startSubagentRecovery(runtimeCtx, supervisor)
		defer stopSubagentRecovery()
		stopInput := closeInputOnCancellation(runtimeCtx, os.Stdin)
		defer stopInput()
		launchDir, err := os.Getwd()
		if err != nil {
			log.Fatalf("failed to read launch directory: %v", err)
		}
		// The REPL is a long-lived process: it runs the workflow scheduler
		// (Stage 14 sub-stage 4) like the memory background pass. A
		// configuration problem disables it with a reason; chat still starts.
		if workflowRuntime, err := newWorkflowRuntime(kernelStore, os.Getenv, pluginManager, supervisor, pluginManager); err != nil {
			log.Printf("workflow scheduler off: %v", err)
			workflowLink.Unavailable("the workflow runtime did not start: " + err.Error())
		} else {
			workflowLink.Bind(workflowRuntime)
			stopWorkflows := startWorkflowScheduler(runtimeCtx, workflowRuntime)
			defer stopWorkflows()
		}
		scanner := bufio.NewScanner(os.Stdin)
		session := newSession(runtimeCtx, func(store *eviedb.Store, resolved plugins.ResolvedComposition) (storedSessionSelection, error) {
			boundStore := &receiptBoundREPLStore{Store: store, composition: resolved, resolvePreset: pluginManager.ResolvePresetContext}
			selected, err := selectREPLSession(
				runtimeCtx, boundStore,
				launchDir, scanner, os.Stdout,
			)
			return boundStore.selection(selected), err
		})
		stopMemory := startMemoryForRuntime(runtimeCtx)
		runREPLContextIOWithMemory(runtimeCtx, session, scanner, os.Stdout, kernelStore)
		stopMemory()
	case "serve":
		runtimeCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stopSignals()
		stopSubagentRecovery := startSubagentRecovery(runtimeCtx, supervisor)
		defer stopSubagentRecovery()
		codexHomes, err := usage.CodexHomes()
		if err != nil {
			log.Fatalf("usage configuration: %v", err)
		}
		usageReader := usage.NewService(runtimeCtx, kernelStore, usage.CodexBinary(), codexHomes)
		spendingService := finance.NewSpendingService()
		workflowRuntime, err := newWorkflowRuntime(kernelStore, os.Getenv, pluginManager, supervisor, pluginManager)
		if err != nil {
			log.Fatalf("workflow runtime: %v", err)
		}
		workflowLink.Bind(workflowRuntime)
		// The web process runs the workflow scheduler (Stage 14 sub-stage
		// 4) in both its full and management-only modes.
		stopWorkflows := startWorkflowScheduler(runtimeCtx, workflowRuntime)
		defer stopWorkflows()
		changeSync, stopChangeSync := startChangeSync(runtimeCtx, kernelStore)
		defer stopChangeSync()
		presetReport, err := pluginManager.ValidatePresetContext(runtimeCtx, "")
		if err != nil {
			log.Fatalf("failed to refresh plugin enabled configuration: %v", err)
		}
		if !presetReport.Valid {
			log.Printf("starting management-only web server: default Agent Preset is invalid: %v", presetReport.Errors)
			stopMemory := startMemoryForRuntime(runtimeCtx)
			serveErr := web.ServeWithContext(runtimeCtx, web.WithChangeMerger(web.WithChangePusher(web.WithChangeSync(web.WithChanges(web.WithRepositories(web.WithWorkflows(web.WithSpending(web.WithUsage(web.WithMemoryBackground(web.WithCandidateReview(web.NewManagedServer(nil, pluginManager, kernelStore), kernelStore), memoryBackground), usageReader), spendingService), workflowRuntime), repositories), changeLister(repositories, kernelStore)), changeSync), changePusher(kernelStore, repositories, strings.TrimSpace(os.Getenv("GITHUB_TOKEN")), changeSync)), &changes.Merger{Lister: changeLister(repositories, kernelStore), Gate: supervisor}))
			stopMemory()
			if err := serveErr; err != nil {
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
		if err := agent.ValidateModelConfiguration(model); err != nil {
			log.Fatalf("invalid model configuration: %v", err)
		}
		if err := agent.ValidateTurnConfiguration(); err != nil {
			log.Fatalf("invalid turn configuration: %v", err)
		}
		profile, err := client.ResolveContextProfile(runtimeCtx, model)
		if err != nil {
			log.Fatalf("failed to resolve context profile: %v", err)
		}
		if warning := contextFallbackWarning(profile.Diagnostics()); warning != "" {
			log.Print(warning)
		}
		configureSubagentRuntime(supervisor, pluginManager, client, profile)
		controller := newWebContextSessionController(kernelStore, pluginManager, nil)
		controller.modelClient, controller.defaultModel = client, model
		controller.newModelAgent = func(ctx context.Context, session memory.Session, composition plugins.ResolvedComposition, selected string, revision int64) (*agent.Session, error) {
			if err := agent.ValidateModelConfiguration(selected); err != nil {
				return nil, err
			}
			selectedProfile, err := client.ResolveChatContextProfile(ctx, selected)
			if err != nil {
				return nil, err
			}
			holderUUID, err := uuid.NewRandom()
			if err != nil {
				return nil, err
			}
			holderID := memory.LeaseHolderID(holderUUID.String())
			return agent.NewWithToolset(client, selectedProfile, kernelStore.BindHistory(session.ID, holderID), session.ScopeContext(), kernelStore.BindTurnOwnerWithModelRevision(session.ID, holderID, revision), composition.Toolset,
				agent.WithCompactionMemoryFlush(memoryBackground.runner)), nil
		}
		stopMemory := startMemoryForRuntime(runtimeCtx)
		serveErr := web.ServeWithContext(runtimeCtx, web.WithChangeMerger(web.WithChangePusher(web.WithChangeSync(web.WithChanges(web.WithRepositories(web.WithWorkflows(web.WithSpending(web.WithUsage(web.WithMemoryBackground(web.WithCandidateReview(web.NewContextDataServer(nil, pluginManager, kernelStore, controller, kernelStore, kernelStore), kernelStore), memoryBackground), usageReader), spendingService), workflowRuntime), repositories), changeLister(repositories, kernelStore)), changeSync), changePusher(kernelStore, repositories, strings.TrimSpace(os.Getenv("GITHUB_TOKEN")), changeSync)), &changes.Merger{Lister: changeLister(repositories, kernelStore), Gate: supervisor}))
		stopMemory()
		if err := serveErr; err != nil {
			log.Fatalf("serve: %v", err)
		}
	default:
		log.Fatalf("unknown command %q (usage: evie [serve|cron-exec <job-id>|plugins ...|presets ...|sessions ...|workflows ...])", cmd)
	}
}

// contextFallbackWarning makes a degraded startup visible: the process is
// running on a checked-in context window because model metadata discovery
// failed or timed out. It is empty for discovered or explicit profiles.
func contextFallbackWarning(d openrouter.ContextProfileDiagnostics) string {
	if d.Source != openrouter.ContextProfileBuiltinFallback {
		return ""
	}
	return fmt.Sprintf(
		"warning: model metadata for %s was unavailable; using the built-in %d-token context window",
		d.ConfiguredModel, d.HardWindowTokens,
	)
}

// repositoryReader reads Workspace Repositories for Settings → Repository
// (changes.spec.md). GitHub is consulted only when GITHUB_TOKEN is set,
// from the environment or ~/.evie/.env; the token goes only to the GitHub
// client and, as a per-command header, to git commands that reach GitHub.
func repositoryReader() *changes.Service {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		return changes.NewService(changes.Config{})
	}
	return changes.NewService(changes.Config{GitHub: github.New(github.API, token, nil), Token: token})
}

// startChangeSync syncs each Workspace's GitHub pull requests every 60
// seconds while evie serve runs (changes.spec.md story 3b), when
// GITHUB_TOKEN is set; the returned stop ends the loop and waits for it.
// Without a token the Syncer still answers the refresh route: a GitHub
// Repository is not connected, and a local one has nothing to sync.
func startChangeSync(ctx context.Context, store *eviedb.Store) (*changes.Syncer, func()) {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		return changes.NewSyncer(changes.SyncConfig{Store: store}), func() {}
	}
	syncer := changes.NewSyncer(changes.SyncConfig{GitHub: github.New(github.API, token, nil), Store: store, Workspaces: store})
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		syncer.Run(ctx)
	}()
	return syncer, func() {
		cancel()
		<-done
	}
}
