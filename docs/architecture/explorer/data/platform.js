evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "platform",
  "nodes": [
    {
      "id": "pkg:internal/eviedb",
      "level": "component",
      "kind": "package",
      "title": "eviedb",
      "group": "infrastructure",
      "subgroup": "Persistence",
      "alsoIn": [
        "memory"
      ],
      "summary": {
        "text": "Evie's Kernel persistence: it opens and upgrades ~/.evie/evie.db and is the one Store through which conversations, Semantic Memory and its review, Tasks, Workspaces, Workflow Runs, subagents, MCP connection records and Changes are written, each under its own transaction and fencing rules.",
        "refs": [
          "code:internal/eviedb/db.go#OpenDBContext",
          "code:internal/eviedb/db.go#openDBAtContextWithHooks",
          "code:internal/eviedb/store.go#Store"
        ]
      },
      "does": [
        {
          "text": "Startup and migrations: every open forces the file to mode 0600, retries only SQLite BUSY while pinging for up to five seconds, runs the base schema, then thirty upgrade steps in a fixed order, from Workspaces, workflows, MCP and subagents through Semantic Memory, the compiler and its review, retrieval and the background pass to plugins, Tasks and session titles, with a check of the Semantic Memory projection among them.",
          "refs": [
            "code:internal/eviedb/db.go#openDBAtContextWithHooks",
            "code:internal/eviedb/db.go#hardenWritableDBFile",
            "code:internal/eviedb/sqlite_startup.go#connectSQLiteStartup",
            "code:internal/eviedb/semantic_replay.go#checkSemanticProjectionStartup"
          ]
        },
        {
          "text": "Transactions: writes that must not interleave run through withImmediateTransaction, which takes BEGIN IMMEDIATE on a dedicated connection, resolves COMMIT and ROLLBACK under a context that ignores caller cancellation but keeps its deadline, and discards a connection whose rollback failed instead of returning it to the pool.",
          "refs": [
            "code:internal/eviedb/immediate_tx.go#withImmediateTransactionResolver",
            "code:internal/eviedb/immediate_tx.go#transactionResolutionContext",
            "code:internal/eviedb/immediate_tx.go#discardImmediateTransactionConnection"
          ]
        },
        {
          "text": "Sessions: creates Global, project and Workspace sessions together with their Composition Receipt, creates delegated child sessions that inherit the parent's Context Scope, and archives an idle owner session by closing it, which is refused while its turn lease or a child subagent is live.",
          "refs": [
            "code:internal/eviedb/sessions.go#Store.CreateWorkspaceSessionWithComposition",
            "code:internal/eviedb/sessions_delegated.go#Store.CreateDelegatedSessionWithComposition",
            "code:internal/eviedb/session_archive.go#Store.setSessionArchived",
            "code:internal/eviedb/composition_receipts.go#Store.SaveCompositionReceipt"
          ]
        },
        {
          "text": "Episodic Memory: AppendEventWithLease checks the holder's fencing token and appends the event in one immediate transaction, giving it the next per-session sequence, validating terminal, snapshot and compaction correlations, titling the session from its first owner message, and projecting the event into the conversation index and session episodes in the same transaction.",
          "refs": [
            "code:internal/eviedb/events.go#Store.AppendEventWithLease",
            "code:internal/eviedb/events.go#Store.appendEvent",
            "code:internal/eviedb/retrieval_conversation_index.go#appendConversationProjection",
            "code:internal/eviedb/session_episodes.go#appendSessionEpisode",
            "term:Episodic Memory"
          ]
        },
        {
          "text": "Turn leases: acquiring a session's lease succeeds only when no unexpired holder exists, bumps the fencing token and generation, can require the chat's current model revision, and first releases that session's inactive Task Claims; SessionTurnOwner binds acquire, heartbeat, authorize and release to one holder for the agent.",
          "refs": [
            "code:internal/eviedb/turn_leases.go#Store.acquireTurnLease",
            "code:internal/eviedb/turn_owner.go#SessionTurnOwner",
            "code:internal/eviedb/turn_owner.go#Store.BindTurnOwnerWithModelRevision"
          ]
        },
        {
          "text": "Semantic Memory kernel: every change is a Prepare/Apply pair (remember a literal or an Entity, correct, retire or restore, link, promote, classify as standing); Prepare resolves the request against SQLite (for a remember, the exact cited owner event) and returns an immutable proposal without writing, and Apply takes the turn lease, revalidates, and commits the Semantic Operation with its projection rows, provenance, state event and Scope Revision atomically.",
          "refs": [
            "code:internal/eviedb/semantic.go#Store.PrepareRememberLiteral",
            "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral",
            "code:internal/eviedb/semantic_correction.go#Store.ApplyCorrectClaim",
            "code:internal/eviedb/semantic_lifecycle.go#Store.ApplyMemoryLifecycle",
            "code:internal/eviedb/semantic_promotion.go#Store.ApplyPromotion",
            "adr:0047"
          ]
        },
        {
          "text": "Replay: accepted operations are the canonical history; VerifySemanticProjection replays them into a disposable shadow projection and quarantines the scopes that disagree, OwnerRebuildSemanticProjection swaps a verified shadow in under a fenced maintenance lock, and startup checks only cheap revision and frontier invariants.",
          "refs": [
            "code:internal/eviedb/semantic_replay.go#Store.VerifySemanticProjection",
            "code:internal/eviedb/semantic_replay.go#Store.OwnerRebuildSemanticProjection",
            "code:internal/eviedb/semantic_replay.go#checkSemanticProjectionStartup",
            "adr:0048",
            "adr:0060"
          ]
        },
        {
          "text": "Memory reads: answers exact Claim inspection, the memory overview and standing set, graph neighbourhood traversal and the archived-session audit, each bound to one Scope Context: the reader's own, or for the audit the closed session's.",
          "refs": [
            "code:internal/eviedb/semantic_correction.go#Store.InspectClaims",
            "code:internal/eviedb/memory_overview.go#Store.MemoryOverview",
            "code:internal/eviedb/semantic_standing.go#Store.StandingMemory",
            "code:internal/eviedb/semantic_graph.go#Store.TraverseSemanticNeighborhood",
            "code:internal/eviedb/semantic_scope.go#Store.InspectArchivedSessionClaims"
          ]
        },
        {
          "text": "Retrieval: SearchMemory treats full-text, exact and alias matches and optional dense vectors as suggestions and re-reads every returned Claim from accepted state inside one read-only transaction with a 500 ms deadline; Conversation Excerpts, conversation expansion and session-episode search have their own bounded reads.",
          "refs": [
            "code:internal/eviedb/retrieval.go#Store.SearchMemory",
            "code:internal/eviedb/retrieval.go#retrievalDeadline",
            "code:internal/eviedb/retrieval_expansion.go",
            "code:internal/eviedb/session_episodes.go#Store.SearchSessionEpisodes",
            "term:Automatic Recall"
          ]
        },
        {
          "text": "Index maintenance: retrieval indexes trail accepted state through generations, checkpoints and dirty queues that triggers fill inside the accepting transaction; RefreshMemoryIndex shares one batch of at most 256 rows between the accepted-Claim, conversation and dense indexes, and dense embedding runs only when EVIE_REMOTE_MEMORY=on and EVIE_MEMORY_EMBEDDING_ENDPOINT is set.",
          "refs": [
            "code:internal/eviedb/retrieval_index.go#retrievalSchema",
            "code:internal/eviedb/retrieval_conversation_index.go#Store.RefreshMemoryIndex",
            "code:internal/eviedb/retrieval_dense_index.go#denseEndpoint"
          ]
        },
        {
          "text": "Proposal inbox and background pass: queues proactive memory proposals exactly as prepared and applies one only on the owner's approval; stores the background pass's activation (also its cross-process runner lease), per-conversation cursors and run records, and commits a pass's proposals with its cursor in one transaction fenced by the conversation's turn lease.",
          "refs": [
            "code:internal/eviedb/memory_proposal_inbox.go#Store.QueueMemoryProposal",
            "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal",
            "code:internal/eviedb/memory_background.go#Store.AcquireMemoryBackgroundRunner",
            "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass"
          ]
        },
        {
          "text": "Memory compiler: CompileCandidateUnit and QueueCandidateUnit seal one bounded source unit into a job under a Compiler Generation, history backfill seals source coordinates for later work, and RunCompilerHost reconciles those obligations and drives extraction through a CompilerExtractor; what it produces is Memory Candidates, never accepted memory.",
          "refs": [
            "code:internal/eviedb/compiler_work.go#Store.CompileCandidateUnit",
            "code:internal/eviedb/compiler_worker.go#Store.QueueCandidateUnit",
            "code:internal/eviedb/compiler_history.go#Store.SelectCompilerHistory",
            "code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost",
            "code:internal/eviedb/compiler_work.go#CompilerExtractor",
            "term:Memory Candidate"
          ]
        },
        {
          "text": "Candidate review: LocalOwnerReviewContext mints an HMAC-sealed review capability bound to one scope and the stored key's revision, and every review read and write re-checks it in its own transaction before preparing, editing, choosing an identity or a time, batching, or resolving a candidate.",
          "refs": [
            "code:internal/eviedb/candidate_review_authority.go#Store.LocalOwnerReviewContext",
            "code:internal/eviedb/candidate_review_authority.go#checkReviewAuthority",
            "code:internal/eviedb/candidate_review_apply.go#Store.ResolveOwnerCandidateReview",
            "code:internal/eviedb/candidate_review_batch.go#Store.PrepareOwnerCandidateBatch",
            "adr:0064"
          ]
        },
        {
          "text": "Workflows: it is the Workflow Runtime's authoritative record: proposals and activations bound to exact hashes and Workspace Access revisions, runs pinned at start with immutable checkpoints and node attempts, fenced run leases with takeover history, Effect Intents and Receipts with Outcome Unknown, interruptions, AI and worker calls, schedules and their slots, notifications, cancellation, migration and retention.",
          "refs": [
            "code:internal/eviedb/workflows.go#Store.ActivateWorkflow",
            "code:internal/eviedb/workflow_runs.go#Store.StartWorkflowRun",
            "code:internal/eviedb/workflow_run_leases.go#Store.AcquireWorkflowRunLease",
            "code:internal/eviedb/workflow_effects.go#Store.BeginWorkflowEffect",
            "code:internal/eviedb/workflow_effects.go#Store.MarkWorkflowEffectUnknown",
            "code:internal/eviedb/workflow_schedules.go#Store.RecordWorkflowScheduleSlot",
            "code:internal/eviedb/workflow_retention_store.go#Store.RedactWorkflowRun"
          ]
        },
        {
          "text": "Cross-process arbitration: the workflow schedule ticker, the retention sweep and the background memory pass each take a single runner lease stored in the database, so serve, the REPL and a CLI run sharing one file do not do the same work twice.",
          "refs": [
            "code:internal/eviedb/workflow_schedules.go#Store.AcquireWorkflowScheduleRunner",
            "code:internal/eviedb/workflow_retention_store.go#Store.AcquireWorkflowRetentionRunner",
            "code:internal/eviedb/memory_background.go#Store.AcquireMemoryBackgroundRunner"
          ]
        },
        {
          "text": "Tasks: stores Tasks and Task Trees with revisions, idempotent mutations and decomposition, Task Claims tied to a session's turn lease, Task Access Grants for delegated children, dependency-aware readiness, and the one-time import of the legacy ~/.todo/DayToDay.json list.",
          "refs": [
            "code:internal/eviedb/tasks.go#Store.CreateGlobalTask",
            "code:internal/eviedb/tasks_tree.go#Store.DecomposeGlobalTask",
            "code:internal/eviedb/tasks_claims.go#Store.ClaimGlobalTask",
            "code:internal/eviedb/tasks_grants.go#Store.IssueTaskAccessGrant",
            "code:internal/eviedb/tasks_ready.go#Store.ReadyGlobalTasks",
            "code:internal/eviedb/tasks_import.go#DefaultLegacyTodoPath"
          ]
        },
        {
          "text": "Workspaces: registers Workspaces whose preset choices and research, explore and build allowances belong to immutable Workspace Revisions, while Workspace Access, the folder, repository-instruction settings, verify commands and MCP tool policies each keep their own append-only revisions; every change is a compare-and-set on the expected revision.",
          "refs": [
            "code:internal/eviedb/workspaces.go#Store.RegisterWorkspaceWithOptions",
            "code:internal/eviedb/workspace_presets.go#ensureWorkspacePresets",
            "code:internal/eviedb/workspace_explore.go#Store.SetWorkspaceExplore",
            "code:internal/eviedb/workflows.go#Store.SetWorkspaceAccess",
            "code:internal/eviedb/workspace_folder.go#Store.SetWorkspaceFolder",
            "code:internal/eviedb/build_verify.go#Store.SetWorkspaceVerify",
            "code:internal/eviedb/mcp_tool_policies.go#Store.SetMCPToolPolicy"
          ]
        },
        {
          "text": "Workspace tabs: records the owner's approval of each tab's data list by its key, so a changed list needs approval again, and keeps each tab's recent problems.",
          "refs": [
            "code:internal/eviedb/workspace_tabs.go#Store.ApproveWorkspaceTabData",
            "code:internal/eviedb/workspace_tabs.go#Store.RecordWorkspaceTabProblem"
          ]
        },
        {
          "text": "Subagents: records every subagent attempt in one execution table, from admission against the parent's lease and policy caps through the running edge only one caller wins, finish states and continuations, to build workers' pinned worktrees and merge-gate steps, run-owned workers for workflow agent steps, reports, and recovery of attempts whose process died.",
          "refs": [
            "code:internal/eviedb/subagents.go#Store.AdmitSubagents",
            "code:internal/eviedb/subagent_execution.go#Store.StartSubagent",
            "code:internal/eviedb/subagent_execution.go#Store.RecoverSubagents",
            "code:internal/eviedb/subagent_build.go#Store.AdmitBuildSubagents",
            "code:internal/eviedb/subagent_gate.go#Store.RecordBuildGate",
            "code:internal/eviedb/subagent_run.go#Store.AdmitRunSubagent",
            "code:internal/eviedb/subagent_result.go#Store.ReadSubagentReport"
          ]
        },
        {
          "text": "MCP connections: stores each connection's record, status and OAuth metadata but never its credentials, which the Keychain holds, and each session's pinned Workspace Access revision, which mcpchat intersects with the current access to decide the MCP tools a chat may reach.",
          "refs": [
            "code:internal/eviedb/mcp_connections.go#Store.CreateMCPConnection",
            "code:internal/eviedb/mcp_connections.go#Store.SetMCPConnectionStatus",
            "code:internal/eviedb/mcp_connections.go#Store.SessionWorkspaceAccessPin",
            "code:internal/mcpchat/service.go#Store"
          ]
        },
        {
          "text": "Changes and build verification: Evie's own Changes are derived from build attempts with nothing new stored, a GitHub Repository's pull requests are kept as the last sync saw them along with the owner's 'merge when checks pass' arms, Change notices go into the notifications store, and BuildVerifyFor gives the merge gate a Workspace's or project's setup and verify commands.",
          "refs": [
            "code:internal/eviedb/build_changes.go#Store.WorkspaceBuilds",
            "code:internal/eviedb/change_pull_requests.go#Store.SaveChangePullRequests",
            "code:internal/eviedb/change_pull_requests.go#Store.ArmChangeMerge",
            "code:internal/eviedb/change_notifications.go#Store.RecordChangeNotification",
            "code:internal/eviedb/build_verify.go#Store.BuildVerifyFor"
          ]
        },
        {
          "text": "Repository instructions: captures a Workspace's instructions once per root turn as an immutable snapshot, reading the folder's AGENTS.md or CLAUDE.md outside the write transaction and re-checking the folder and settings revisions under the turn lease before storing it.",
          "refs": [
            "code:internal/eviedb/repository_instructions.go#SessionHistory.RepositoryInstructions",
            "code:internal/eviedb/repository_instructions.go#Store.PreviewRepositoryInstructions",
            "code:internal/repoinstructions/load.go#Load"
          ]
        },
        {
          "text": "Plugins and composition: seeds each compiled plugin's enabled default once and keeps the owner's later choice, and stores each session's Composition Receipt and the Compatibility Resolutions recorded when it resumes.",
          "refs": [
            "code:internal/eviedb/plugin_configuration.go#Store.ResolvePluginEnabled",
            "code:internal/eviedb/composition_receipts.go#Store.SaveCompositionReceipt",
            "code:internal/eviedb/composition_receipts.go#Store.AppendCompatibilityResolutions"
          ]
        },
        {
          "text": "Owner diagnostics: ReadConversationUsage selects only the usage counters and requested model of assistant events, never their text, and the database browser returns schema topology for every table but row records only for the cron tables, refusing events and semantic tables (typed views only) and everything else.",
          "refs": [
            "code:internal/eviedb/usage.go#Store.ReadConversationUsage",
            "code:internal/eviedb/database_browser.go#Store.InspectDatabaseSchema",
            "code:internal/eviedb/database_browser.go#databaseRowAccess",
            "code:internal/eviedb/database_browser.go#databaseColumnRedacted"
          ]
        }
      ],
      "owns": [
        {
          "text": "Episodic Memory: sessions and their append-only events, plus each session's model setting, turn lease, Composition Receipt, Compatibility Resolutions, Task Focus and pinned Workspace Access.",
          "refs": [
            "table:evie.sessions",
            "table:evie.events",
            "table:evie.session_model_settings",
            "table:evie.session_turn_leases",
            "table:evie.session_composition_receipts",
            "table:evie.session_compatibility_resolutions",
            "table:evie.session_task_focus",
            "table:evie.session_workspace_access_pins"
          ]
        },
        {
          "text": "Projects and Workspaces: the projects and workspaces rows and the revision tables for presets and their revocations, folders, instruction settings, explore and build allowances, verify commands and Workspace Access, plus tab data approvals and tab problems.",
          "refs": [
            "table:evie.projects",
            "table:evie.workspaces",
            "table:evie.workspace_preset_revisions",
            "table:evie.workspace_preset_revocations",
            "table:evie.workspace_folder_revisions",
            "table:evie.workspace_access_revisions",
            "table:evie.build_verify_revisions",
            "table:evie.workspace_tab_data_approvals"
          ]
        },
        {
          "text": "Semantic Memory (17 semantic_* tables): the accepted-operation log with its per-scope revisions, and the projection of scopes, Entities, Aliases, versioned Predicates, Claims, corrections, classifications, Source Links, Graph Links, state events and Promotions, plus quarantine and maintenance-lock state.",
          "refs": [
            "table:evie.semantic_operations",
            "table:evie.semantic_operation_scopes",
            "table:evie.semantic_scopes",
            "table:evie.semantic_claims",
            "table:evie.semantic_entities",
            "table:evie.semantic_source_links",
            "table:evie.semantic_projection_quarantine",
            "code:internal/eviedb/semantic.go#semanticSchema"
          ]
        },
        {
          "text": "Derived retrieval state, which is never memory: FTS5 tables for accepted Claims and for conversation messages with their id-to-rowid lookups, dirty queues and generation checkpoints, dense vector tables for Claims and events, and session episodes with their own full-text table.",
          "refs": [
            "table:evie.memory_retrieval_generations",
            "table:evie.memory_retrieval_fts_v3",
            "table:evie.memory_retrieval_event_fts_v4",
            "table:evie.memory_dense_vectors",
            "table:evie.memory_dense_event_vectors",
            "table:evie.memory_session_episodes",
            "code:internal/eviedb/retrieval_index_rows.go"
          ]
        },
        {
          "text": "The background pass and the proposal inbox: one activation row, a cursor per root conversation, one row per pass, and the queued proposals with the owner's declines.",
          "refs": [
            "table:evie.memory_background_state",
            "table:evie.memory_background_cursors",
            "table:evie.memory_background_runs",
            "table:evie.memory_proposal_inbox",
            "table:evie.memory_proposal_inbox_declines",
            "code:internal/eviedb/memory_background.go#memoryBackgroundSchema"
          ]
        },
        {
          "text": "The memory compiler (43 memory_compiler_* tables): generations, sealed jobs and their schedule, selections, candidates and candidate groups, coverage, per-conversation activations, history backfill requests, diagnostics, recurrence links, status projections, capacity and release receipts.",
          "refs": [
            "table:evie.memory_compiler_generations",
            "table:evie.memory_compiler_jobs",
            "table:evie.memory_compiler_candidates",
            "table:evie.memory_compiler_coverage",
            "table:evie.memory_compiler_activations",
            "table:evie.memory_compiler_history_requests",
            "code:internal/eviedb/compiler_schema.go#ensureCompilerSchema"
          ]
        },
        {
          "text": "Candidate review (12 memory_review_* tables): the review authentication key, previews and deliveries, resolutions and audits, batch previews, and the edit, identity, temporal and inbox revisions.",
          "refs": [
            "table:evie.memory_review_authorization",
            "table:evie.memory_review_previews",
            "table:evie.memory_review_resolutions",
            "table:evie.memory_review_edit_revisions",
            "code:internal/eviedb/candidate_review_schema.go#ensureCandidateReviewSchema"
          ]
        },
        {
          "text": "Tasks (the tasks table and 19 task_* tables): hierarchy, events, revisions, claims and claim events, access and event grants, idempotency and mutation results, decomposition and coordination records, and legacy Todo import provenance.",
          "refs": [
            "table:evie.tasks",
            "table:evie.task_hierarchy",
            "table:evie.task_revisions",
            "table:evie.task_claims",
            "table:evie.task_access_grants",
            "table:evie.task_legacy_todo_imports"
          ]
        },
        {
          "text": "Workflows (33 workflow_* tables): Workspace and Global proposals, activations and deactivations, built-in versions and controls, runs with checkpoints, node attempts, leases and recovery events, Effect Intents, Receipts, unknowns and conflicts, interruptions, model and worker calls, schedule slots and controls, notifications, cancellations, overrides, migrations, substitutions and retention state.",
          "refs": [
            "table:evie.workflow_activations",
            "table:evie.workflow_runs",
            "table:evie.workflow_run_checkpoints",
            "table:evie.workflow_node_attempts",
            "table:evie.workflow_run_leases",
            "table:evie.workflow_effect_intents",
            "table:evie.workflow_effect_receipts",
            "table:evie.workflow_notifications",
            "code:internal/eviedb/workflow_runs.go#ensureWorkflowRunSchema"
          ]
        },
        {
          "text": "Subagents: one subagent_executions row per attempt, which also carries a build worker's worktree and merge-gate record.",
          "refs": [
            "table:evie.subagent_executions",
            "code:internal/eviedb/subagents.go#subagentExecutionsTable"
          ]
        },
        {
          "text": "MCP and Changes: MCP connection records and per-Workspace tool policies, and a Repository's synced pull requests and merge arms.",
          "refs": [
            "table:evie.mcp_connections",
            "table:evie.workspace_mcp_tool_policies",
            "table:evie.change_pull_requests",
            "table:evie.change_merge_arms"
          ]
        },
        {
          "text": "Repository instruction snapshots, keyed by session and root turn and immutable by trigger.",
          "refs": [
            "table:evie.repository_instruction_snapshots",
            "code:internal/eviedb/repository_instructions.go#ensureRepositoryInstructions"
          ]
        },
        {
          "text": "Plugin enabled configuration, and the cron tables jobs and job_runs, whose schema lives here though the cron tools and `evie cron-exec` read and write them directly.",
          "refs": [
            "table:evie.plugin_enabled_configuration",
            "table:evie.jobs",
            "table:evie.job_runs",
            "code:internal/eviedb/db.go#schema",
            "code:cmd/evie/cronexec.go#cronExec"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Opening: OpenDB and OpenDBContext open ~/.evie/evie.db, creating ~/.evie with mode 0700; OpenDBAt and OpenDBAtContext open any path with the same upgrades (tests and the explorer tool use them); OpenDBReadOnlyContext opens with mode=ro and no upgrades, for query_db.",
          "refs": [
            "code:internal/eviedb/db.go#OpenDBContext",
            "code:internal/eviedb/db.go#OpenDBAtContext",
            "code:internal/eviedb/db.go#OpenDBReadOnlyContext",
            "code:internal/tools/db_read.go",
            "code:scripts/explorer/evie_databases.go"
          ]
        },
        {
          "text": "NewStore wraps the *sql.DB; UseClock replaces the clock and the owner's time zone before the Store is shared, so a deterministic evaluation can replay history on a synthetic timeline.",
          "refs": [
            "code:internal/eviedb/store.go#NewStore",
            "code:internal/eviedb/store.go#Store.UseClock"
          ]
        },
        {
          "text": "The Store is concrete; it and its bound views satisfy interfaces their consumers own, such as the agent's History (through SessionHistory) and SemanticGraphMemory, mcpconn.Records, usage.ConversationReader, web's CandidateReviewKernel and DatabaseInspector, and the small per-command kernel interfaces in cmd/evie.",
          "refs": [
            "code:internal/agent/history.go#History",
            "code:internal/agent/semantic_memory.go#SemanticGraphMemory",
            "code:internal/mcpconn/manager.go#Records",
            "code:internal/usage/service.go#ConversationReader",
            "code:internal/web/candidate_review.go#CandidateReviewKernel",
            "code:cmd/evie/compiler_management.go#compilerManagementKernel"
          ]
        },
        {
          "text": "BindHistory and BindTurnOwner give one agent session views bound to its session and lease holder, so every append it makes carries that holder's lease.",
          "refs": [
            "code:internal/eviedb/history.go#Store.BindHistory",
            "code:internal/eviedb/history.go#SessionHistory.Append",
            "code:internal/eviedb/turn_owner.go#Store.BindTurnOwner"
          ]
        },
        {
          "text": "Fenced writes take a TurnLease or a WorkflowRunLease, and the lease writer is package-private, so no caller can pass its own SQL into a fenced transaction.",
          "refs": [
            "code:internal/eviedb/turn_leases.go#turnLeaseWriteExecutor",
            "code:internal/eviedb/turn_leases.go#Store.withTurnLeaseWrite",
            "code:internal/eviedb/workflow_run_leases.go#Store.AcquireWorkflowRunLease"
          ]
        },
        {
          "text": "Sentinel errors are part of the contract, such as ErrTurnLeaseHeld, ErrSessionModelChanged, ErrWorkspaceNotFound and ErrOwnerReviewUnauthorized, which web maps to status codes and messages.",
          "refs": [
            "code:internal/eviedb/turn_leases.go#ErrTurnLeaseHeld",
            "code:internal/eviedb/session_models.go#ErrSessionModelChanged",
            "code:internal/eviedb/workspaces.go#ErrWorkspaceNotFound",
            "code:internal/eviedb/candidate_review_authority.go#ErrOwnerReviewUnauthorized",
            "code:internal/web/serve.go#Server.runTurn"
          ]
        },
        {
          "text": "Kernel-only seams stay off model-facing interfaces: ManagementUpdateGlobalTask, IssueTaskAccessGrant, OwnerRebuildSemanticProjection and LocalOwnerReviewContext are called by trusted hosts, never through task.Service or a tool.",
          "refs": [
            "code:internal/eviedb/tasks.go#Store.ManagementUpdateGlobalTask",
            "code:internal/eviedb/tasks_grants.go#Store.IssueTaskAccessGrant",
            "code:internal/eviedb/semantic_replay.go#Store.OwnerRebuildSemanticProjection",
            "code:internal/eviedb/candidate_review_authority.go#Store.LocalOwnerReviewContext"
          ]
        },
        {
          "text": "Long-running entry points for the hosts in cmd/evie: RunCompilerHost(ctx, CompilerSupervisorConfig) for the compiler host and RefreshMemoryIndex(ctx, limit) for each retrieval maintenance batch.",
          "refs": [
            "code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost",
            "code:internal/eviedb/compiler_worker.go#CompilerSupervisorConfig",
            "code:internal/eviedb/retrieval_conversation_index.go#Store.RefreshMemoryIndex",
            "code:cmd/evie/memory_retrieval_host.go#startMemoryRetrievalHost"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "There is no schema version number: every open re-runs CREATE IF NOT EXISTS and each upgrade step inspects the live schema (a column's presence, a table's SQL) to decide whether it applies, so any process that opens the file migrates it.",
          "refs": [
            "code:internal/eviedb/db.go#openDBAtContextWithHooks",
            "code:internal/eviedb/db.go#tableHasColumn",
            "code:internal/eviedb/db.go#ensureTaskContextScopes",
            "code:internal/eviedb/semantic.go#ensureSemanticSchema"
          ]
        },
        {
          "text": "The startup retry covers only the first ping, when concurrent fresh processes can get BUSY enabling WAL; schema statements are never retried, and the pinned driver's connection setup cannot be interrupted by the five-second budget.",
          "refs": [
            "code:internal/eviedb/sqlite_startup.go#connectSQLiteStartupWithin"
          ]
        },
        {
          "text": "Events refuse UPDATE and DELETE by trigger, and Hard Erasure is not implemented (memory changes are reversible Retirement), so no Store path removes conversation evidence.",
          "refs": [
            "code:internal/eviedb/db.go#schema",
            "term:Hard Erasure",
            "adr:0050",
            "doc:cmd/evie/docs/active/semantic-memory-stage-3.spec.md#out-of-scope"
          ]
        },
        {
          "text": "The package is not pure SQL: dense maintenance calls the local embedding endpoint, repository-instruction capture reads the Workspace folder, and the legacy Todo import reads ~/.todo/DayToDay.json.",
          "refs": [
            "code:internal/eviedb/retrieval_dense_index.go#Store.ensureDenseGeneration",
            "code:internal/eviedb/repository_instructions.go#Store.PreviewRepositoryInstructions",
            "code:internal/eviedb/tasks_import.go#DefaultLegacyTodoPath"
          ]
        },
        {
          "text": "Dense indexing follows the environment of whichever process runs maintenance: one without EVIE_REMOTE_MEMORY=on and an embedding endpoint disables the dense selection on its next tick, and the next process that has them re-enables it from checkpoint zero, re-embedding everything.",
          "refs": [
            "code:internal/eviedb/retrieval_dense_index.go#denseEndpoint",
            "code:internal/eviedb/retrieval_dense_index.go#Store.ensureDenseGeneration",
            "code:internal/eviedb/retrieval_dense_index.go#Store.refreshDenseIndex"
          ]
        },
        {
          "text": "Index names carry each index's own generation, not one schema version: memory_retrieval_fts_v3 holds accepted Claims (accepted-fts-unicode61-v3), while conversation messages live in memory_retrieval_event_fts_v4.",
          "refs": [
            "code:internal/eviedb/retrieval_index.go#memoryIndexGeneration",
            "code:internal/eviedb/retrieval_conversation_index.go#conversationIndexGeneration",
            "table:evie.memory_retrieval_fts_v3",
            "table:evie.memory_retrieval_event_fts_v4"
          ]
        },
        {
          "text": "workflow_notifications is the general Durable Notification store despite its name: Change notices land there too, with an empty workflow and a change_id.",
          "refs": [
            "code:internal/eviedb/change_notifications.go#Store.RecordChangeNotification",
            "table:evie.workflow_notifications",
            "term:Durable Notification"
          ]
        },
        {
          "text": "LocalOwnerReviewContext says web callers must call it only after owner authentication, but web's only check is the loopback guard; there are no user accounts, so the guard is the whole owner check.",
          "refs": [
            "code:internal/eviedb/candidate_review_authority.go#Store.LocalOwnerReviewContext",
            "code:internal/web/candidate_review.go#Server.candidateAuthority",
            "code:internal/web/serve.go#Server.guard"
          ]
        },
        {
          "text": "Per-conversation compiler activation is still implemented here (ActivateCompiler, ResumeCompilerActivation), but nothing outside tests calls it since the background pass replaced it; existing activations can only be read and disabled.",
          "refs": [
            "code:internal/eviedb/compiler_activation.go#Store.ActivateCompiler",
            "code:internal/eviedb/compiler_activation.go#Store.ResumeCompilerActivation",
            "code:cmd/evie/compiler_activation.go#errCompilerActivationRetired"
          ]
        },
        {
          "text": "The package has no package comment, so its extracted doc is empty; the closest overview is the Store type and the open path in db.go.",
          "refs": [
            "pkg:internal/eviedb",
            "code:internal/eviedb/store.go#Store",
            "code:internal/eviedb/db.go#openDBAtContextWithHooks"
          ]
        }
      ],
      "terms": [
        "Kernel",
        "Episodic Memory",
        "Semantic Memory",
        "Semantic Operation",
        "Scope Revision",
        "Promotion",
        "Automatic Recall",
        "Conversation Excerpt",
        "Memory Candidate",
        "Compiler Generation",
        "Compilation Coverage",
        "Context Scope",
        "Composition Receipt",
        "Compatibility Resolution",
        "Task Tree",
        "Task Claim",
        "Task Access Grant",
        "Workspace",
        "Workspace Revision",
        "Workspace Access",
        "Workflow Run",
        "Effect Intent",
        "Effect Receipt",
        "Outcome Unknown",
        "Durable Notification",
        "Change",
        "Repository",
        "Hard Erasure"
      ],
      "links": [
        "doc:docs/memory-architecture.md",
        "adr:0047",
        "adr:0048",
        "adr:0060",
        "adr:0064"
      ]
    },
    {
      "id": "pkg:internal/web",
      "level": "component",
      "kind": "package",
      "title": "web",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The HTTP surface of `evie serve`: one loopback ServeMux that serves the embedded React app, streams chat turns as server-sent events, holds open chats and pending approvals in memory, and exposes the owner's surfaces as JSON routes behind a cross-origin guard.",
        "refs": [
          "code:internal/web/serve.go#Server",
          "code:internal/web/serve.go#Server.Handler",
          "code:internal/web/lifecycle.go#ServeWithContext"
        ]
      },
      "does": [
        {
          "text": "Route table and guards: POST /api/chat and POST /api/approve sit behind guard; /, the OAuth callback and the two GET /workspace-tabs routes have no wrapper; every other route sits behind managementRoute (guard plus POST-only).",
          "refs": [
            "code:internal/web/serve.go#Server.Handler",
            "route:POST /api/chat",
            "route:POST /api/approve",
            "route:/",
            "route:/api/connections/oauth/callback"
          ]
        },
        {
          "text": "guard admits a request only when its Content-Type is exactly application/json, its Origin (if any) names a loopback host and its Host is loopback, answering 403 otherwise; managementRoute answers 405 with Allow: POST to any other method before guard runs.",
          "refs": [
            "code:internal/web/serve.go#Server.guard",
            "code:internal/web/serve.go#Server.managementRoute",
            "code:internal/web/serve.go#loopbackRequestHost",
            "doc:cmd/evie/docs/active/serve.spec.md#part-2--the-server"
          ]
        },
        {
          "text": "listenAddr resolves EVIE_ADDR (default 127.0.0.1:6687) and refuses any host that is not loopback, because there is no authentication: the loopback bind and the guard are the whole boundary.",
          "refs": [
            "code:internal/web/serve.go#listenAddr",
            "doc:docs/decisions.md",
            "doc:cmd/evie/docs/active/serve.decisions.md"
          ]
        },
        {
          "text": "Routes exist only when their seam is wired: constructor arguments and With* options set fields, and each register function returns early when its field is nil or the controller lacks an optional interface, so one Handler serves both the full and the management-only server.",
          "refs": [
            "code:internal/web/serve.go#Server.Handler",
            "code:internal/web/serve.go#NewManagedServer",
            "code:internal/web/terminal.go#Server.registerTerminalRoutes",
            "code:internal/web/workflows.go#Server.registerWorkflowRoutes"
          ]
        },
        {
          "text": "Chat turns: POST /api/chat runs a turn in the chat the request names, streamed as SSE in the response or, with detach, answered 202 at once and streamed through the chat's feed; it is refused with 409 when that chat is not open, already running a turn or changed model, and 503 when the server has no chat.",
          "refs": [
            "code:internal/web/serve.go#Server.handleChat",
            "code:internal/web/serve.go#Server.runTurn",
            "route:POST /api/chat"
          ]
        },
        {
          "text": "A turn runs on the runtime context, not the request's, so closing the browser never cancels it; only POST /api/cancel stops the named chat's turn, and /api/compact runs the agent's manual compaction for that chat.",
          "refs": [
            "code:internal/web/lifecycle.go#turnLifecycleContext",
            "code:internal/web/turn_controls.go#Server.handleCancel",
            "code:internal/web/turn_controls.go#Server.handleCompact",
            "doc:cmd/evie/docs/active/serve.decisions.md"
          ]
        },
        {
          "text": "Open chats: keeps up to eight chats open, each with its own agent session and model revision, closing the least recently used idle chat to make room; each chat's live feed replays the running turn's events to a tab that joins late, and chat status streams which chats run or wait on an approval.",
          "refs": [
            "code:internal/web/open_chats.go#maxOpenChats",
            "code:internal/web/open_chats.go#Server.makeRoomLocked",
            "code:internal/web/feed.go#Server.handleFollow",
            "code:internal/web/status.go#Server.handleChatStatus"
          ]
        },
        {
          "text": "Approvals: holds the pending-approval map and the two approvers (the streaming request's and the detached turn's followed one); the full exchange is the approval handshake flow among the agent loop's flows.",
          "refs": [
            "code:internal/web/approvals.go#Server.approver",
            "code:internal/web/feed.go#Server.followedApprover",
            "code:internal/web/approvals.go#Server.handleApprove",
            "node:pkg:internal/agent"
          ]
        },
        {
          "text": "Context sessions: lists Workspaces, projects and sessions, opens or creates the selected Global, project or Workspace session through the ContextSessionController, returns its history, archives and restores chats and Workspaces, and switches a chat's model.",
          "refs": [
            "code:internal/web/context_sessions.go#Server.handleContextSessionSelect",
            "code:internal/web/history.go#Server.handleContextSessionHistory",
            "code:internal/web/session_archive.go#Server.handleContextSessionLifecycle",
            "code:internal/web/models.go#Server.handleModelSelect"
          ]
        },
        {
          "text": "Workspace surfaces: files and diffs from the attached folder, terminals in that folder (at most eight, each living as long as its streaming request), the Workspace's own tabs with their approved data reads (at most four at once across Evie), Repository and GitHub status, Changes with push, merge and sync, and subagent traces.",
          "refs": [
            "code:internal/web/local_folder.go#Server.registerFolderRoutes",
            "code:internal/web/terminal.go#Server.registerTerminalRoutes",
            "code:internal/web/workspace_tabs.go#Server.registerWorkspaceTabDataRoutes",
            "code:internal/web/repository.go#Server.registerRepositoryRoutes",
            "code:internal/web/changes_merge.go#Server.registerChangeMergeRoutes",
            "code:internal/web/subagents.go#Server.registerSubagentRoutes"
          ]
        },
        {
          "text": "Memory surfaces: Semantic Memory scopes, objects and inspection in the focused chat's scope, evidence behind a recall, compiler candidate review (single, batch, edit, identity, time) with diagnostics, the proposal inbox, and the background pass's status.",
          "refs": [
            "code:internal/web/memory.go#Server.handleMemoryObjects",
            "code:internal/web/memory_evidence.go#Server.handleMemoryEvidence",
            "code:internal/web/candidate_review.go#Server.registerCandidateReviewRoutes",
            "code:internal/web/memory_proposals.go#Server.registerMemoryProposalRoutes",
            "code:internal/web/memory_background.go#Server.registerMemoryBackgroundRoutes"
          ]
        },
        {
          "text": "Data hub and workflows: database schema and allowlisted rows, usage and spending summaries, and the Workflow Runtime's list, review, approve, start, runs, answers, reconciliation, cancellation, schedules, notifications, retention and built-ins.",
          "refs": [
            "code:internal/web/database.go#Server.handleDatabaseRows",
            "code:internal/web/usage.go#Server.handleUsage",
            "code:internal/web/spending.go#Server.handleSpendingSummary",
            "code:internal/web/workflows.go#Server.registerWorkflowRoutes",
            "code:internal/web/notifications.go#Server.handleNotifications"
          ]
        },
        {
          "text": "MCP: lists, adds, connects, waits on and disconnects MCP connections for serve's connection manager, receives the OAuth callback, and reviews which server tools a Workspace may use.",
          "refs": [
            "code:internal/web/connections.go#Server.registerConnectionRoutes",
            "code:internal/web/connection_tools.go#Server.registerConnectionToolRoutes"
          ]
        },
        {
          "text": "The folder chooser opens a native macOS panel through osascript on the server's desktop, so it is the one route that also requires a loopback peer address and an Origin, when present, matching the Host.",
          "refs": [
            "code:internal/web/workspace_folder_picker.go#Server.handleWorkspaceChooseFolder",
            "code:internal/web/native_folder_picker.go#runWorkspaceFolderPickerScript"
          ]
        },
        {
          "text": "Static app: serves the build compiled into the binary, falls back to index.html (no-store, frame-src 'self', frame-ancestors 'none') for any path not in the build, and caches fingerprinted assets for a year.",
          "refs": [
            "code:internal/web/static.go#Server.staticHandler",
            "code:internal/web/static.go#serveIndex"
          ]
        },
        {
          "text": "Lifecycle: ServeWithContext listens, logs the URL and serves until the context ends, then gives the HTTP server five seconds to shut down; Close drains the folder picker and terminals and waits up to ten seconds for detached turns to record how they ended.",
          "refs": [
            "code:internal/web/lifecycle.go#ServeWithContext",
            "code:internal/web/lifecycle.go#serveListenerContext",
            "code:internal/web/terminal.go#Server.Close"
          ]
        }
      ],
      "owns": [
        {
          "text": "Only per-process state: open chats, their feeds and replay buffers, pending approvals and the chat each belongs to, each running turn's stop control, terminals and the folder picker; nothing here survives a restart.",
          "refs": [
            "code:internal/web/serve.go#Server",
            "code:internal/web/open_chats.go#openChat",
            "code:internal/web/feed.go#chatFeed"
          ]
        },
        {
          "text": "No tables: durable reads and writes go through the seams it is handed, mostly *eviedb.Store, which serve passes as the receipt inspector, Semantic Memory, database inspector, candidate review and subagent reader.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/web/serve.go#NewContextDataServer"
          ]
        },
        {
          "text": "Embedded assets: the built UI under ui/dist and, for Workspace tabs, Evie's helper script and the prelude that every tab page starts with.",
          "refs": [
            "code:internal/web/static.go#distFS",
            "code:internal/web/workspace_tabs.go#tabHelper",
            "code:internal/web/workspace_tabs.go#tabPrelude"
          ]
        },
        {
          "text": "Request limits: management bodies are one JSON value of at most 4 KiB with unknown fields refused, unless a route sets its own limit, such as candidate review (8 KiB) and tab requests (32 KiB).",
          "refs": [
            "code:internal/web/management.go#maxManagementBodyBytes",
            "code:internal/web/management.go#decodeManagementJSONUpTo",
            "code:internal/web/candidate_review.go#decodeCandidateRequest",
            "code:internal/web/workspace_tabs.go#maxTabRequestBytes"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "ServeWithContext(ctx, server) is the entry point cmd/evie calls; Handler() returns the mux so tests drive it through httptest with no port and no model.",
          "refs": [
            "code:internal/web/lifecycle.go#ServeWithContext",
            "code:internal/web/serve.go#Server.Handler"
          ]
        },
        {
          "text": "Constructors layer seams: NewServer, NewManagedServer (plugins and receipts), NewContextServer (context sessions), NewContextMemoryServer (Semantic Memory) and NewContextDataServer (database inspection).",
          "refs": [
            "code:internal/web/serve.go#NewServer",
            "code:internal/web/serve.go#NewManagedServer",
            "code:internal/web/serve.go#NewContextServer",
            "code:internal/web/serve.go#NewContextMemoryServer",
            "code:internal/web/serve.go#NewContextDataServer"
          ]
        },
        {
          "text": "With* options add one surface each: candidate review, background status, usage, spending, workflows, repositories, Changes, change sync, push, merge, subagents, MCP connections and MCP tool review.",
          "refs": [
            "code:internal/web/candidate_review.go#WithCandidateReview",
            "code:internal/web/usage.go#WithUsage",
            "code:internal/web/workflows.go#WithWorkflows",
            "code:internal/web/changes_merge.go#WithChangeMerger",
            "code:internal/web/connections.go#WithMCPConnections",
            "code:internal/web/connection_tools.go#WithMCPToolReview"
          ]
        },
        {
          "text": "Its seams are interfaces it owns, such as ContextSessionController (implemented by cmd/evie's webContextSessionController), ReceiptInspector, DatabaseInspector, UsageReader, WorkflowManager and MCPConnections.",
          "refs": [
            "code:internal/web/context_sessions.go#ContextSessionController",
            "code:cmd/evie/web_context_sessions.go#webContextSessionController",
            "code:internal/web/serve.go#ReceiptInspector",
            "code:internal/web/database.go#DatabaseInspector",
            "code:internal/web/usage.go#UsageReader",
            "code:internal/web/workflows.go#WorkflowManager"
          ]
        },
        {
          "text": "sseEvents implements the agent's Events contract and writes each event as one SSE block, the web twin of the REPL's terminal output.",
          "refs": [
            "code:internal/web/events.go#sseEvents",
            "doc:cmd/evie/docs/active/serve.spec.md#sse-event-vocabulary-server--client"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Unguarded `/` is safe: it answers only GET and HEAD with files compiled into the binary (or the index.html shell), reads no state and performs no action, and the shell forbids other sites from framing Evie.",
          "refs": [
            "route:/",
            "code:internal/web/static.go#Server.staticHandler",
            "code:internal/web/static.go#serveIndex"
          ]
        },
        {
          "text": "Unguarded /api/connections/oauth/callback is unguarded by necessity, since the authorization server's redirect is a cross-site browser navigation; it accepts only GET with a single-use, unexpired state that serve minted, exchanges the code with that attempt's PKCE verifier, and returns an escaped, no-store, no-referrer page.",
          "refs": [
            "route:/api/connections/oauth/callback",
            "code:internal/web/connections.go#Server.registerConnectionRoutes",
            "code:internal/mcpconn/manager.go#Manager.Callback",
            "code:internal/web/connections.go#writeCallbackPage"
          ]
        },
        {
          "text": "Unguarded GET /workspace-tabs/{workspace}/{tab} only redirects to the trailing-slash URL, built from IDs that passed their patterns, so it cannot point off Evie and reads nothing.",
          "refs": [
            "route:GET /workspace-tabs/{workspace}/{tab}",
            "code:internal/web/workspace_tabs.go#Server.registerWorkspaceTabRoutes",
            "code:internal/web/workspace_tabs.go#workspaceIDPattern"
          ]
        },
        {
          "text": "Unguarded GET /workspace-tabs/{workspace}/{tab}/{path...} is safe because it is sealed rather than guarded: it requires a loopback Host, serves only files of an active Workspace's tab folder, and every response is sandboxed without same-origin, opens no connections and may be framed only by Evie, so a tab's page cannot reach Evie's routes (its Origin is null); WebRTC remains an accepted residual risk.",
          "refs": [
            "route:GET /workspace-tabs/{workspace}/{tab}/{path...}",
            "code:internal/web/workspace_tabs.go#tabOrigin",
            "code:internal/web/workspace_tabs.go#tabContentSecurityPolicy",
            "code:internal/web/workspace_tabs.go#sealTabResponse",
            "code:internal/workspacetabs/tabs.go#Open",
            "doc:cmd/evie/docs/active/workspace-tabs.spec.md#known-residual-risk"
          ]
        },
        {
          "text": "Comments and the spec say less than the code: Handler's comment says every /api route sits behind the guard and connections.go calls the callback the one unguarded route, and serve.spec.md requires JSON on every /api handler, yet the callback takes plain GET and three other routes are unguarded; the code is what runs.",
          "refs": [
            "code:internal/web/serve.go#Server.Handler",
            "code:internal/web/connections.go#Server.registerConnectionRoutes",
            "doc:cmd/evie/docs/active/serve.spec.md#part-2--the-server"
          ]
        },
        {
          "text": "guard is a cross-site defence, not authentication: any local process that sends JSON with a loopback Host passes it (the CLI's connection commands rely on that), and /api/chats/follow replays a running turn's approval_request events, ids included, to any such caller, so the id is no secret from local software.",
          "refs": [
            "code:internal/web/serve.go#Server.guard",
            "code:cmd/evie/connections.go#connectionCommands.post",
            "code:internal/web/feed.go#Server.handleFollow",
            "code:internal/web/events.go#sseEvents.ApprovalRequest"
          ]
        },
        {
          "text": "A request to an /api path that is not registered (any route of a seam that was not wired) falls through to the static handler: POST gets 405 'static assets are GET only' and GET gets the app shell with 200, never 404.",
          "refs": [
            "code:internal/web/serve.go#Server.Handler",
            "code:internal/web/static.go#Server.staticHandler"
          ]
        },
        {
          "text": "POST /api/chat and POST /api/approve decode their bodies with no size limit and accept unknown fields, unlike management routes.",
          "refs": [
            "code:internal/web/serve.go#Server.handleChat",
            "code:internal/web/approvals.go#Server.handleApprove",
            "code:internal/web/management.go#decodeManagementJSONUpTo"
          ]
        },
        {
          "text": "Most route facts show no method because the patterns have none; managementRoute enforces POST inside the handler instead.",
          "refs": [
            "code:internal/web/serve.go#Server.managementRoute",
            "route:/api/chats/status"
          ]
        },
        {
          "text": "serve.spec.md still describes one global session held in memory, with persistence and multiple conversations out of scope, but the server keeps up to eight open chats over durable sessions and serves their history.",
          "refs": [
            "doc:cmd/evie/docs/active/serve.spec.md#part-2--the-server",
            "doc:cmd/evie/docs/active/serve.spec.md#out-of-scope-v1--each-is-its-own-future-feature",
            "code:internal/web/open_chats.go#maxOpenChats",
            "code:internal/web/history.go#Server.handleContextSessionHistory"
          ]
        },
        {
          "text": "Several files carry a comment above `package web`; the extractor takes the first by file name, so the package doc shown is approvals.go's approval-handshake sentence rather than events.go's 'Package web is the evie web frontend'.",
          "refs": [
            "pkg:internal/web",
            "code:internal/web/approvals.go",
            "code:internal/web/events.go"
          ]
        }
      ],
      "terms": [
        "Action Approval",
        "Agent Preset",
        "Context Scope",
        "Workspace",
        "Semantic Memory",
        "Memory Candidate",
        "Workflow Run",
        "Durable Notification",
        "Repository",
        "Change",
        "Account Connection"
      ],
      "links": [
        "doc:cmd/evie/docs/active/serve.spec.md",
        "doc:cmd/evie/docs/active/serve.decisions.md",
        "doc:cmd/evie/docs/active/multi-chat.spec.md",
        "doc:cmd/evie/docs/active/workspace-tabs.spec.md"
      ]
    },
    {
      "id": "pkg:docs/architecture/explorer",
      "level": "component",
      "kind": "package",
      "title": "explorer",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The explorer snapshot: a tiny package beside the system explorer's files that builds the page, its viewer and stylesheet, its data, its examples and the system map page into the evie binary, so the System view shows the map and explorer of the build that is running.",
        "refs": [
          "code:docs/architecture/explorer/snapshot.go#Files",
          "doc:cmd/evie/docs/active/system-page.spec.md#modules"
        ]
      },
      "does": [
        {
          "text": "Files returns the snapshot rooted at the explorer directory: index.html, explorer.js, explorer.css, data/, examples/ and system.html, the map page the vision skill builds from .vision/map.json, fixed when the binary is built.",
          "refs": [
            "code:docs/architecture/explorer/snapshot.go#Files",
            "code:docs/architecture/explorer/snapshot.go#files"
          ]
        },
        {
          "text": "internal/web serves it under /explorer/, GET and HEAD only, with a 404 for anything else, beside a generated build.js naming the repository, the commit and whether the build had uncommitted changes.",
          "refs": [
            "route:/explorer/",
            "code:internal/web/explorer.go#Server.registerExplorerRoutes",
            "code:internal/web/explorer.go#readExplorerBuild"
          ]
        },
        {
          "text": "The map page gets its own content-security policy: its inline scripts allowed by their hashes, taken once from the snapshot, inline styles, and the Geist web font from Google Fonts; every other file keeps the explorer's policy.",
          "refs": [
            "code:internal/web/explorer.go#systemMapPolicy",
            "code:internal/web/explorer.go#inlineScripts"
          ]
        }
      ],
      "owns": [
        {
          "text": "No data of its own: only the explorer's files as they were in the commit the binary was built from.",
          "refs": [
            "code:docs/architecture/explorer/snapshot.go#files"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "One function, Files, returning an fs.FS; internal/web is its only importer.",
          "refs": [
            "code:docs/architecture/explorer/snapshot.go#Files",
            "code:internal/web/explorer.go#Server.registerExplorerRoutes"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "go:embed reaches only its own directory, so this one package lives under docs/ rather than internal/, and the explorer tool's package extraction names its directory explicitly.",
          "refs": [
            "code:scripts/explorer/extract.go#snapshotDir",
            "doc:cmd/evie/docs/active/system-page.decisions.md"
          ]
        },
        {
          "text": "Edits to the explorer's files reach evie serve only after a rebuild; the tests, README.md and the Go file itself stay out of the snapshot.",
          "refs": [
            "code:docs/architecture/explorer/snapshot.go#files"
          ]
        }
      ],
      "links": [
        "doc:docs/architecture/explorer/README.md",
        "node:pkg:internal/web"
      ]
    },
    {
      "id": "pkg:cmd/evie",
      "level": "component",
      "kind": "package",
      "title": "cmd/evie",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The evie binary and Evie's composition root: main loads the environment, dispatches one-shot commands, and otherwise wires the Store, Plugin Manager, subagent supervisor, Workflow Runtime and memory hosts into the terminal REPL or `evie serve`.",
        "refs": [
          "code:cmd/evie/main.go#main",
          "command:evie serve"
        ]
      },
      "does": [
        {
          "text": "Starts by warming the login-shell environment snapshot in the background and loading ~/.evie/.env, then .env and ../../.env, each in its own call.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/tools/shellsnapshot.go#Warm",
            "node:container:env-file"
          ]
        },
        {
          "text": "Dispatches in a fixed order: cron-exec first; then commands that need no database (workflow review, and the MCP connection commands, which call serve); then it opens the database and offers the arguments to each management handler in turn, every handler answering handled=false for arguments that are not its own.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/workflows.go#runWorkflowManagement",
            "code:cmd/evie/connections.go#runConnectionManagement",
            "code:cmd/evie/compiler_diagnostics.go#runCompilerDiagnostics"
          ]
        },
        {
          "text": "`evie cron-exec <job-id>` runs one cron job's command under a 30-minute bound and records the run, returning 0 even when the job fails so launchd never treats Evie as broken; it builds no model client.",
          "refs": [
            "command:evie cron-exec",
            "code:cmd/evie/cronexec.go#cronExec",
            "code:cmd/evie/cronexec.go#cronExecTimeout",
            "table:evie.job_runs"
          ]
        },
        {
          "text": "Memory, Workspace, project, worktree and Change commands run straight against the Store through small interfaces each handler owns, before any plugin or model is wired.",
          "refs": [
            "code:cmd/evie/compiler_management.go#compilerManagementKernel",
            "code:cmd/evie/candidate_review.go#ownerReviewKernel",
            "code:cmd/evie/workspace_explore.go#runWorkspaceManagement",
            "code:cmd/evie/changes.go#runChangeManagement"
          ]
        },
        {
          "text": "`evie workflows` activation commands build their own subagent supervisor, a narrower Plugin Manager (the workflow capability plugins plus Web, Todo and Subagents) and a Workflow Runtime, run one command, and give the workers five seconds to stop.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/workflow_activation.go#newWorkflowPluginManager",
            "code:cmd/evie/workflow_activation.go#newWorkflowRuntime",
            "code:cmd/evie/workflow_activation.go#runWorkflowActivation"
          ]
        },
        {
          "text": "For everything else it imports the legacy Todo list, recovers inactive Task Claims and subagent records, builds the subagent supervisor from the EVIE_SUBAGENTS_* limits, and compiles the Plugin Manager from the Kernel toolset and ten First-party Plugins with their default enabled states; `plugins`, `presets` and `sessions` then run on that manager and exit.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/main.go#defaultPluginEnabled",
            "code:cmd/evie/subagents.go#configuredSubagentPolicy",
            "code:internal/plugins/manager.go#NewManager",
            "code:cmd/evie/management.go#runManagementCommand"
          ]
        },
        {
          "text": "With no command it runs the REPL: subagent recovery, the workflow scheduler (or a reason why workflows are unavailable), the session chooser, an agent session against OpenRouter, the memory hosts, and the REPL loop until input closes.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/repl.go#selectREPLSession",
            "code:cmd/evie/repl.go#runREPLContextIOWithMemory",
            "code:cmd/evie/runtime_lifecycle.go#closeInputOnCancellation"
          ]
        },
        {
          "text": "`evie serve` wires usage, spending, the Workflow Runtime and its scheduler, change sync and the merger, MCP connections and their session pool, and then either a management-only server or the full chat server; the `evie serve` startup flow walks it step by step.",
          "refs": [
            "command:evie serve",
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/main.go#startChangeSync",
            "code:cmd/evie/connections.go#newMCPConnectionManager"
          ]
        },
        {
          "text": "Late-bound links let chat tools reach things built later: WorkflowLink is bound to the Workflow Runtime once it starts (or marked unavailable), and MCPLink only in serve, so REPL chats report that MCP tools need serve.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/plugins/workflows.go#WorkflowLink",
            "code:internal/plugins/mcp.go#MCPLink"
          ]
        },
        {
          "text": "Every agent session is bound to its pinned Composition Receipt: a new session keeps the snapshot it was created with, an existing one resumes from its receipt and records Compatibility Resolutions, and a legacy session without one records the default once.",
          "refs": [
            "code:cmd/evie/session_composition.go#bindSessionComposition",
            "code:cmd/evie/session_composition.go#resumeAndRecordSessionComposition"
          ]
        },
        {
          "text": "In serve, webContextSessionController opens web chats: it creates or resumes the selected Global, project or Workspace session (a new Workspace chat uses the Workspace's default preset), binds its composition and builds an agent on the chat's own model.",
          "refs": [
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.selectSession",
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.SelectModel"
          ]
        },
        {
          "text": "Owns the long-lived hosts' wiring: the background memory pass (with its own OpenRouter or local model), retrieval maintenance (a batch of up to 256 rows each second, every 100 ms while work is pending, every 5 s after a failure), the optional compiler host from EVIE_COMPILER_CONFIG, subagent recovery and change sync; the three memory hosts' stops wait at most five seconds, while recovery and sync stops wait for their loops to end.",
          "refs": [
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost",
            "code:cmd/evie/memory_retrieval_host.go#startMemoryRetrievalHost",
            "code:cmd/evie/compiler_host.go#startConfiguredCompilerHost",
            "code:cmd/evie/subagents.go#startSubagentRecovery",
            "code:cmd/evie/main.go#startChangeSync"
          ]
        },
        {
          "text": "One changes.Service reads Workspace Repositories, so Settings, the owner's pushes and workflows' pushes share its folder lock; it reaches GitHub only when GITHUB_TOKEN is set.",
          "refs": [
            "code:cmd/evie/main.go#repositoryReader"
          ]
        }
      ],
      "owns": [
        {
          "text": "No tables of its own; durable state goes through eviedb. It owns wiring choices such as each compiled plugin's default enabled state, which is seeded once per database.",
          "refs": [
            "code:cmd/evie/main.go#defaultPluginEnabled",
            "code:internal/eviedb/plugin_configuration.go#Store.ResolvePluginEnabled"
          ]
        },
        {
          "text": "The environment settings parsed here: EVIE_MODEL (default agent.DefaultModel), OPENROUTER_API_KEY, GITHUB_TOKEN, EVIE_ADDR for the connection commands, EVIE_COMPILER_CONFIG, the EVIE_SUBAGENTS_* limits, EVIE_WORKFLOW_WORKERS, EVIE_WORKFLOW_REMOTE_MODELS, and the workflow daily cap and retention policies.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/subagents.go#configuredSubagentPolicy",
            "code:cmd/evie/workflow_runs.go#workflowSchedulerConfig",
            "code:cmd/evie/workflow_activation.go#workflowRemoteModels",
            "code:cmd/evie/workflow_schedules.go#workflowDailyCap",
            "code:cmd/evie/workflow_migrations.go#workflowRetention",
            "code:cmd/evie/connections.go#defaultConnectionCommands"
          ]
        },
        {
          "text": "Checked-in Workspace Workflow Definitions (finance-daily and finance-review) under cmd/evie/workflows, which the owner proposes with `evie workflows propose`; the binary does not embed them.",
          "refs": [
            "code:cmd/evie/workflows/finance-daily/workflow.yaml",
            "code:cmd/evie/workflows/finance-review/workflow.yaml",
            "command:evie workflows propose"
          ]
        },
        {
          "text": "The feature specs and decision records under cmd/evie/docs, the sources of truth for Evie's active features.",
          "refs": [
            "code:cmd/evie/docs/active",
            "code:cmd/evie/docs/done",
            "doc:AGENTS.md#sources-of-truth"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "The CLI itself: `evie` for the REPL, `evie serve`, `evie cron-exec <job-id>` for launchd, and the management commands the panel lists.",
          "refs": [
            "command:evie serve",
            "command:evie cron-exec",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Each management handler takes a small kernel interface of its own that *eviedb.Store satisfies, so it can be tested with a fake.",
          "refs": [
            "code:cmd/evie/compiler_management.go#compilerManagementKernel",
            "code:cmd/evie/candidate_review.go#ownerReviewKernel",
            "code:cmd/evie/compiler_history.go#compilerHistoryKernel",
            "code:cmd/evie/compiler_host.go#compilerHostKernel"
          ]
        },
        {
          "text": "webContextSessionController implements web's ContextSessionController and its optional controllers (archiving, Workspace creation and settings, folders, tabs, repository instructions, models).",
          "refs": [
            "code:cmd/evie/web_context_sessions.go#webContextSessionController",
            "code:internal/web/context_sessions.go#ContextSessionController",
            "code:cmd/evie/workspace_creation.go#webContextSessionController.RegisterWorkspaceWithOptions"
          ]
        },
        {
          "text": "The connection commands are an HTTP client of serve: they POST JSON to serve's /api/connections routes at EVIE_ADDR.",
          "refs": [
            "code:cmd/evie/connections.go#connectionCommands.post",
            "route:/api/connections/list"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The package comment is stale: it says `serve` will run the web frontend once internal/web exists.",
          "refs": [
            "code:cmd/evie/main.go",
            "pkg:cmd/evie"
          ]
        },
        {
          "text": "`evie memory-compiler activate` and `resume` are still recognised commands, but they always fail with the retirement message: the background pass replaced per-conversation activation.",
          "refs": [
            "command:evie memory-compiler activate",
            "command:evie memory-compiler resume",
            "code:cmd/evie/compiler_activation.go#errCompilerActivationRetired",
            "code:cmd/evie/compiler_activation.go#runCompilerActivationManagement"
          ]
        },
        {
          "text": "serve checks EVIE_ADDR only inside ServeWithContext, after the scheduler, change sync and memory hosts have started; a refused address or a busy port ends the process through log.Fatalf, which skips the deferred stops, so a workflow lease taken in that moment is recovered only when it expires.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/web/lifecycle.go#ServeWithContext",
            "code:internal/web/serve.go#listenAddr",
            "code:internal/workflowruntime/scheduler.go#Runtime.Start"
          ]
        },
        {
          "text": "Management-only serve passes the server no context-session controller and no Semantic Memory or database seam, so besides chat it lacks Workspaces, folders, terminals, tabs, Changes, models, memory reads and the database browser.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/web/serve.go#NewManagedServer",
            "code:internal/web/serve.go#Server.Handler"
          ]
        },
        {
          "text": "Changing defaultPluginEnabled reaches only databases that never resolved that plugin: the default is seeded once and the owner's stored choice wins.",
          "refs": [
            "code:cmd/evie/main.go#defaultPluginEnabled",
            "code:internal/eviedb/plugin_configuration.go#Store.ResolvePluginEnabled"
          ]
        },
        {
          "text": "Serve can hold several OpenRouter clients: the chat client, the background pass's own, and the workflow runtime's for AI and agent steps when EVIE_WORKFLOW_REMOTE_MODELS=on.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost",
            "code:cmd/evie/workflow_activation.go#workflowModels",
            "code:cmd/evie/workflow_activation.go#workflowWorkers"
          ]
        }
      ],
      "terms": [
        "Kernel",
        "Agent Preset",
        "Plugin Manager",
        "First-party Plugin",
        "Enabled Plugin",
        "Composition Receipt",
        "Compatibility Resolution",
        "Context Scope",
        "Workspace",
        "Workflow Run",
        "Task Claim",
        "Change",
        "Repository"
      ],
      "links": [
        "doc:cmd/evie/docs/active/serve.spec.md",
        "node:container:serve",
        "node:container:repl",
        "node:container:management-cli",
        "node:container:cron-exec"
      ]
    },
    {
      "id": "pkg:cmd/todo",
      "level": "component",
      "kind": "package",
      "title": "cmd/todo",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The `todo` command: the owner's CLI for Evie's durable Task Trees, which opens the Evie database directly and changes Tasks through the same Store rules the chat's Todo tools use.",
        "refs": [
          "code:cmd/todo/main.go#run",
          "code:cmd/todo/main.go#usageText"
        ]
      },
      "does": [
        {
          "text": "run dispatches list, get, add, update, start, block, done, cancel, reopen, delete, release-claim and show, prints the usage for help, and exits 2 for an unknown command or bad flags and 1 for a failure.",
          "refs": [
            "code:cmd/todo/main.go#run",
            "code:cmd/todo/main.go#knownCommand",
            "code:cmd/todo/main.go#reportUsage",
            "code:cmd/todo/main.go#reportError"
          ]
        },
        {
          "text": "Before every command it opens ~/.evie/evie.db (applying its upgrades), imports the legacy Todo list once, and releases Task Claims whose execution is no longer live.",
          "refs": [
            "code:cmd/todo/main.go#run",
            "code:internal/eviedb/tasks_import.go#Store.ImportDefaultLegacyTodoList",
            "code:internal/eviedb/tasks_claims.go#Store.RecoverInactiveTaskClaims"
          ]
        },
        {
          "text": "Attributes every change to the local owner and a session: the fixed Global session `todo-cli` without --session, or the named primary session and its Workspace or project.",
          "refs": [
            "code:cmd/todo/main.go#ownerContext",
            "code:cmd/todo/main.go#ownerSession",
            "code:cmd/todo/main.go#todoCLISessionID"
          ]
        },
        {
          "text": "A status change that needs a Task Claim (start, block, done, a result summary, reopening work in progress, cancelling claimed work) takes the session's turn lease for one minute and a claim, applies the update, then releases both.",
          "refs": [
            "code:cmd/todo/main.go#cliUpdateNeedsClaim",
            "code:cmd/todo/main.go#applyClaimedCLIUpdate",
            "code:internal/eviedb/turn_leases.go#Store.AcquireTurnLease",
            "term:Task Claim"
          ]
        },
        {
          "text": "Without --revision a lifecycle command uses the Task's current revision, and it generates an idempotency key unless --idempotency-key is given, which then requires --revision.",
          "refs": [
            "code:cmd/todo/main.go#runLifecycle"
          ]
        },
        {
          "text": "release-claim is an audited override with a required reason, and delete cancels through the management override with the reason todo_cli_delete, keeping the Task.",
          "refs": [
            "code:cmd/todo/main.go#runReleaseClaim",
            "code:cmd/todo/main.go#runLifecycle",
            "code:internal/eviedb/tasks.go#Store.ManagementUpdateGlobalTask"
          ]
        },
        {
          "text": "show prints a Task Tree's orchestration progress: each Task's status, dependencies, paths, claim (this session, live or stale) and latest build worker, then the Ready Set todo_ready would offer, batches, builds in flight and blocked Tasks with reasons.",
          "refs": [
            "code:cmd/todo/main.go#runShow",
            "code:cmd/todo/main.go#renderProgress",
            "code:internal/eviedb/tasks_ready.go#Store.TaskTreeProgress",
            "term:Ready Set"
          ]
        }
      ],
      "owns": [
        {
          "text": "No tables of its own: it writes the Task tables through eviedb and creates the `todo-cli` session row on first use.",
          "refs": [
            "table:evie.tasks",
            "table:evie.task_claims",
            "table:evie.sessions",
            "code:internal/eviedb/sessions.go#Store.EnsureGlobalSession"
          ]
        },
        {
          "text": "The usage text, which documents every flag, the status values and the rule that flags precede positional arguments.",
          "refs": [
            "code:cmd/todo/main.go#usageText"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "The `todo` CLI, its words listed in the panel; main hands os.Args to run(ctx, args, stdout, stderr), which returns the exit code so tests drive it without a process.",
          "refs": [
            "command:todo show",
            "code:cmd/todo/main.go#main",
            "code:cmd/todo/main.go#run"
          ]
        },
        {
          "text": "It talks only to the Store, through the task package's types; it needs neither serve nor a model.",
          "refs": [
            "code:cmd/todo/main.go#run",
            "pkg:internal/task"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "todo never loads ~/.evie/.env, and show sizes batches with delegation.DefaultPolicy, so its Ready Set can differ from the chat's todo_ready, which uses the EVIE_SUBAGENTS_* limits.",
          "refs": [
            "code:cmd/todo/main.go#runShow",
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/subagents.go#configuredSubagentPolicy"
          ]
        },
        {
          "text": "With --session naming a chat whose turn is running, a claim-needing update fails, because the session's turn lease is held.",
          "refs": [
            "code:cmd/todo/main.go#applyClaimedCLIUpdate",
            "code:internal/eviedb/turn_leases.go#ErrTurnLeaseHeld"
          ]
        },
        {
          "text": "Flags must come before positional arguments: Go's flag parsing stops at the first non-flag word.",
          "refs": [
            "code:cmd/todo/main.go#usageText",
            "code:cmd/todo/main.go#parseFlags"
          ]
        }
      ],
      "terms": [
        "Task",
        "Subtask",
        "Task Tree",
        "Task Claim",
        "Task Dependency",
        "Ready Set",
        "Context Scope"
      ],
      "links": [
        "node:container:todo-cli"
      ]
    },
    {
      "id": "pkg:internal/usage",
      "level": "component",
      "kind": "package",
      "title": "usage",
      "group": "infrastructure",
      "subgroup": "Observability",
      "summary": {
        "text": "Content-free owner diagnostics for Data > Usage: it reads Codex account activity and local Codex token records and summarizes Evie's own recorded token usage, and none of it becomes conversation evidence, billing authority or runtime budget state.",
        "refs": [
          "code:internal/usage/usage.go",
          "code:internal/usage/service.go#Service",
          "doc:cmd/evie/docs/active/usage.spec.md#first-delivery"
        ]
      },
      "does": [
        {
          "text": "ParsePeriod accepts an inclusive from date, an exclusive to date and a non-empty time zone that time.LoadLocation knows (an IANA name, UTC, or Local for the server's own zone), from 1 to 90 calendar days, and refuses a date that the zone would shift.",
          "refs": [
            "code:internal/usage/usage.go#ParsePeriod",
            "code:internal/usage/usage.go#ErrRange"
          ]
        },
        {
          "text": "Summarize totals observations overall, per local day and per model: a missing counter stays unavailable while a reported zero stays zero, every counter carries how many calls reported it, the cached share is weighted over calls with compatible input and cached counts, and inconsistent records count as anomalies.",
          "refs": [
            "code:internal/usage/usage.go#Summarize",
            "code:internal/usage/usage.go#Metrics.add",
            "code:internal/usage/usage.go#Counter"
          ]
        },
        {
          "text": "InspectUsage returns one source per Codex account (merged across homes that share an account), one unassigned local source per Codex home, and Evie's own source, each with its state, coverage and message, so one failing source never hides the others.",
          "refs": [
            "code:internal/usage/service.go#Service.InspectUsage",
            "code:internal/usage/service.go#preferAccount"
          ]
        },
        {
          "text": "ReadCodexAccount runs `codex app-server --stdio` with MCP servers, analytics and apps turned off, CODEX_HOME set and stderr discarded, for at most 20 seconds, and calls only initialize, account/read, account/rateLimits/read and account/usage/read; raw errors are never forwarded.",
          "refs": [
            "code:internal/usage/codex.go#ReadCodexAccount"
          ]
        },
        {
          "text": "LocalIndex scans the JSONL files under each home's sessions and archived_sessions in memory, keeping only token metadata and per-file offsets, deduplicating records across copies, and rebuilding from the files when one is removed, replaced or truncated.",
          "refs": [
            "code:internal/usage/local.go#LocalIndex",
            "code:internal/usage/local.go#LocalIndex.Scan"
          ]
        },
        {
          "text": "Evie's source comes from the Store's ReadConversationUsage, which reads only the usage counters and requested model of accepted assistant events across all sessions.",
          "refs": [
            "code:internal/usage/service.go#ConversationReader",
            "code:internal/eviedb/usage.go#Store.ReadConversationUsage",
            "table:evie.events"
          ]
        },
        {
          "text": "Configuration: EVIE_CODEX_BIN picks the executable (else the ChatGPT desktop bundle's codex, else codex on PATH), and EVIE_CODEX_HOMES lists up to four absolute homes (else CODEX_HOME, else ~/.codex).",
          "refs": [
            "code:internal/usage/codex.go#CodexBinary",
            "code:internal/usage/codex.go#CodexHomes",
            "doc:cmd/evie/docs/active/usage.operations.md#open-and-inspect"
          ]
        }
      ],
      "owns": [
        {
          "text": "No tables and no files: only in-memory collector caches per Codex home (the last account snapshot and the local index), which a restart rebuilds.",
          "refs": [
            "code:internal/usage/service.go#Service",
            "code:internal/usage/local.go#LocalIndex",
            "doc:cmd/evie/docs/active/usage.decisions.md"
          ]
        },
        {
          "text": "The usage report shapes: Period, Metrics with exact counters sent as decimal strings, Summary, Source, Account with quota windows, and Report.",
          "refs": [
            "code:internal/usage/usage.go#Report",
            "code:internal/usage/usage.go#Source",
            "code:internal/usage/usage.go#Counter"
          ]
        },
        {
          "text": "The bounds: 90 days per request, 200,000 observations, 10,000 local files and 200 models per summary.",
          "refs": [
            "code:internal/usage/usage.go#MaxObservations",
            "code:internal/usage/usage.go#Summarize",
            "code:internal/usage/local.go#LocalIndex.Scan"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "NewService(ctx, conversations, binary, homes) builds the reader serve hands to web; its background reads live as long as ctx.",
          "refs": [
            "code:internal/usage/service.go#NewService",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Service.InspectUsage(ctx, period) satisfies web's UsageReader, which POST /api/data/usage/summary calls with a from, to and timezone.",
          "refs": [
            "code:internal/usage/service.go#Service.InspectUsage",
            "code:internal/web/usage.go#UsageReader",
            "route:/api/data/usage/summary"
          ]
        },
        {
          "text": "ConversationReader is the one interface it owns; the Store implements it, which is why eviedb imports usage and not the reverse.",
          "refs": [
            "code:internal/usage/service.go#ConversationReader",
            "code:internal/eviedb/usage.go#Store.ReadConversationUsage"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Collection runs only when someone asks: InspectUsage starts the background reads (an account read at most once a minute, a local scan every 3 seconds while indexing and every 30 once complete), so the first request after serve starts shows sources still indexing.",
          "refs": [
            "code:internal/usage/service.go#Service.refresh",
            "code:internal/usage/service.go#Service.InspectUsage",
            "doc:cmd/evie/docs/active/usage.operations.md#open-and-inspect"
          ]
        },
        {
          "text": "A bad Codex home setting stops `evie serve` at startup: more than four EVIE_CODEX_HOMES entries, or a relative path there or in CODEX_HOME, is fatal there although usage is only a diagnostic view.",
          "refs": [
            "code:internal/usage/codex.go#CodexHomes",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Account totals and local detail overlap and are never added together; local records carry no account, and Evie's source leaves out compaction, extraction and unrecorded failed calls and names requested, not billed, models.",
          "refs": [
            "code:internal/usage/service.go#Service.InspectUsage",
            "doc:cmd/evie/docs/active/usage.operations.md#coverage-limits"
          ]
        },
        {
          "text": "The account source's label carries the signed-in Codex email, which reaches the browser as an account label; credentials and raw subprocess errors do not.",
          "refs": [
            "code:internal/usage/codex.go#ReadCodexAccount",
            "doc:cmd/evie/docs/active/usage.operations.md#coverage-limits"
          ]
        }
      ],
      "links": [
        "doc:cmd/evie/docs/active/usage.spec.md",
        "doc:cmd/evie/docs/active/usage.decisions.md",
        "doc:cmd/evie/docs/active/usage.operations.md"
      ]
    }
  ],
  "edges": [],
  "flows": [
    {
      "id": "serve-startup",
      "title": "`evie serve` startup",
      "summary": {
        "text": "How `evie serve` goes from the command to a loopback server: environment, database and Kernel recovery, plugins, the subagent supervisor, the Workflow Runtime and background hosts, MCP connections, the chat client, then the HTTP listener.",
        "refs": [
          "code:cmd/evie/main.go#main",
          "code:internal/web/lifecycle.go#ServeWithContext"
        ]
      },
      "trigger": {
        "text": "The owner runs `evie serve`.",
        "refs": [
          "command:evie serve",
          "code:cmd/evie/main.go#main"
        ]
      },
      "participants": [
        "person:owner",
        "container:serve",
        "pkg:cmd/evie",
        "container:env-file",
        "pkg:internal/eviedb",
        "container:evie-db",
        "pkg:internal/subagents",
        "pkg:internal/plugins",
        "container:subagent-recovery",
        "pkg:internal/usage",
        "pkg:internal/workflowruntime",
        "container:workflow-scheduler",
        "container:change-sync",
        "pkg:internal/mcpconn",
        "pkg:internal/openrouter",
        "external:openrouter",
        "container:retrieval-maintenance",
        "container:compiler-host",
        "container:memory-background-pass",
        "pkg:internal/web"
      ],
      "steps": [
        {
          "from": "person:owner",
          "to": "container:serve",
          "label": "Runs `evie serve`",
          "kind": "call",
          "via": "process",
          "refs": [
            "command:evie serve",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "from": "container:serve",
          "to": "pkg:cmd/evie",
          "label": "main() starts and warms the shell-environment snapshot in the background",
          "kind": "call",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/tools/shellsnapshot.go#Warm"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "container:env-file",
          "label": "Loads ~/.evie/.env, then .env and ../../.env; the shell's own variables win",
          "kind": "call",
          "via": "file",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:cmd/evie",
          "label": "`serve` is not cron-exec, workflow review, a connection command or a management command, so dispatch falls through",
          "kind": "note",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/workflows.go#runWorkflowManagement",
            "code:cmd/evie/connections.go#runConnectionManagement"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/eviedb",
          "label": "OpenDB and NewStore",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/eviedb/db.go#OpenDB",
            "code:internal/eviedb/db.go#OpenDBContext",
            "code:internal/eviedb/store.go#NewStore"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Hardens the file to 0600, pings through BUSY, applies the schema and every upgrade, checks the Semantic Memory projection",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/db.go#openDBAtContextWithHooks",
            "code:internal/eviedb/db.go#hardenWritableDBFile",
            "code:internal/eviedb/semantic_replay.go#checkSemanticProjectionStartup"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/eviedb",
          "label": "Imports the legacy Todo list and recovers inactive Task Claims (either failing stops serve)",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/eviedb/tasks_import.go#Store.ImportDefaultLegacyTodoList",
            "code:internal/eviedb/tasks_claims.go#Store.RecoverInactiveTaskClaims"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/subagents",
          "label": "Builds the subagent supervisor from the EVIE_SUBAGENTS_* policy",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/subagents.go#configuredSubagentPolicy",
            "code:internal/subagents/supervisor.go#New"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/eviedb",
          "label": "RecoverSubagents once; a failure is only logged, the background loop retries",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/eviedb/subagent_execution.go#Store.RecoverSubagents"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/plugins",
          "label": "NewManager compiles the Kernel toolset and ten First-party Plugins; ConfigureEnabledState seeds their defaults",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/plugins/manager.go#NewManager",
            "code:internal/plugins/manager.go#Manager.ConfigureEnabledState",
            "code:cmd/evie/main.go#defaultPluginEnabled"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:cmd/evie",
          "label": "newMemoryBackgroundHost resolves the background pass's configuration and model; nothing starts yet",
          "kind": "note",
          "refs": [
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost",
            "code:internal/memorybackground/config.go#FromEnv"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:cmd/evie",
          "label": "Cancels the runtime context on SIGINT or SIGTERM",
          "kind": "note",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "container:subagent-recovery",
          "label": "startSubagentRecovery runs the supervisor's recovery loop",
          "kind": "async",
          "refs": [
            "code:cmd/evie/subagents.go#startSubagentRecovery",
            "code:internal/subagents/recovery.go#Supervisor.RunRecovery"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/usage",
          "label": "CodexHomes (a bad setting stops serve) and NewService for Data > Usage",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/usage/codex.go#CodexHomes",
            "code:internal/usage/service.go#NewService"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/workflowruntime",
          "label": "newWorkflowRuntime with the procedural repo, models, workers, daily cap and retention; an error stops serve",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/workflow_activation.go#newWorkflowRuntime",
            "code:internal/workflowruntime/runtime.go#New"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "container:workflow-scheduler",
          "label": "Binds the chat's WorkflowLink and starts the scheduler: run passes, schedule catch-up and retention, each first at start",
          "kind": "async",
          "refs": [
            "code:cmd/evie/workflow_runs.go#startWorkflowScheduler",
            "code:internal/workflowruntime/scheduler.go#Runtime.Start"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "container:change-sync",
          "label": "startChangeSync loops every 60 s only when GITHUB_TOKEN is set; the merger is stored behind it",
          "kind": "async",
          "refs": [
            "code:cmd/evie/main.go#startChangeSync",
            "code:internal/changes/sync.go#Syncer.Run",
            "code:cmd/evie/changes_push.go#changeMerger"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/mcpconn",
          "label": "Connection manager (Keychain secrets, callback on serve's address) and the session pool; binds MCPLink through mcpchat",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/connections.go#newMCPConnectionManager",
            "code:internal/mcpconn/pool.go#NewPool",
            "code:internal/mcpchat/service.go#New"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/plugins",
          "label": "ValidatePresetContext on the default Agent Preset; if invalid, serve becomes a management-only server with the same hosts and no chat",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/plugins/preset.go#Manager.ValidatePresetContext",
            "code:internal/web/serve.go#NewManagedServer",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/openrouter",
          "label": "NewClient(OPENROUTER_API_KEY), fatal when missing; validates EVIE_MODEL and turn configuration",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/openrouter/client.go#NewClient",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "from": "pkg:internal/openrouter",
          "to": "external:openrouter",
          "label": "Discovers the model's context window (3 s limit); on failure a model with a built-in window falls back to it with a warning, any other stops serve",
          "kind": "call",
          "via": "network",
          "refs": [
            "code:internal/openrouter/context_profile.go#Client.ResolveContextProfile",
            "code:cmd/evie/main.go#contextFallbackWarning"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/subagents",
          "label": "configureSubagentRuntime gives the supervisor the client, profile and worker composer",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/subagents.go#configureSubagentRuntime",
            "code:internal/subagents/supervisor.go#Supervisor.Configure"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:cmd/evie",
          "label": "Builds the web context-session controller, which opens each chat's agent on its own model",
          "kind": "note",
          "refs": [
            "code:cmd/evie/web_context_sessions.go#newWebContextSessionController",
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.selectSession"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "container:retrieval-maintenance",
          "label": "startMemoryRetrievalHost refreshes retrieval indexes in batches",
          "kind": "async",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/memory_retrieval_host.go#startMemoryRetrievalHost"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "container:compiler-host",
          "label": "Starts the compiler host only when EVIE_COMPILER_CONFIG names a usable configuration",
          "kind": "async",
          "refs": [
            "code:cmd/evie/compiler_host.go#startConfiguredCompilerHost",
            "code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "container:memory-background-pass",
          "label": "Starts the pass, which first reads its model's price when remote, or logs why it is off",
          "kind": "async",
          "refs": [
            "code:cmd/evie/memory_background.go#memoryBackgroundHost.Start",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/web",
          "label": "NewContextDataServer plus every With* seam, then ServeWithContext",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/web/serve.go#NewContextDataServer",
            "code:internal/web/lifecycle.go#ServeWithContext",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/web",
          "label": "listenAddr refuses a non-loopback EVIE_ADDR, then net.Listen on 127.0.0.1:6687 by default",
          "kind": "note",
          "refs": [
            "code:internal/web/serve.go#listenAddr",
            "code:internal/web/lifecycle.go#ServeWithContext"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "person:owner",
          "label": "Logs `evie serve listening on http://…` and serves the app and /api until shutdown",
          "kind": "return",
          "refs": [
            "code:internal/web/lifecycle.go#ServeWithContext",
            "code:internal/web/lifecycle.go#serveListenerContext"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:cmd/evie",
          "label": "On shutdown: HTTP gets 5 s, Close waits up to 10 s for detached turns, the memory hosts stop, then deferred stops end the MCP pool, change sync, scheduler, recovery and supervisor",
          "kind": "note",
          "refs": [
            "code:internal/web/lifecycle.go#serveListenerContext",
            "code:internal/web/terminal.go#Server.Close",
            "code:cmd/evie/main.go#main"
          ]
        }
      ]
    }
  ]
}
);
