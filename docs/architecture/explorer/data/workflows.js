evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "workflows",
  "nodes": [
    {
      "id": "pkg:internal/workflow",
      "level": "component",
      "kind": "package",
      "title": "workflow",
      "group": "runtime",
      "summary": {
        "text": "Loads, validates, hashes, reviews and simulates reviewed Workflow Definitions, and supplies the pure step semantics the Workflow Runtime executes; it imports no other Evie package and stores nothing.",
        "refs": [
          "code:internal/workflow/definition.go",
          "pkg:internal/workflow"
        ]
      },
      "does": [
        {
          "text": "Parses workflow.yaml with gopkg.in/yaml.v3 into the closed Definition schema and accepts only a safe YAML subset: anchors, aliases, merge keys, explicit tags, duplicate keys, nulls, YAML 1.1 booleans and silent number or date conversions are refused with their line, so the reviewed text means exactly one thing.",
          "refs": [
            "code:internal/workflow/yaml.go#parseDefinition",
            "code:internal/workflow/definition.go#Definition",
            "doc:cmd/evie/docs/active/workflow-runtime.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "Validate reads a definition directory through a Source and checks it against an Environment (the capability catalog, the Workspace's access and the operator's worker ceiling); it returns every problem with its definition path, or a Validated value holding the executable hash and the review report.",
          "refs": [
            "code:internal/workflow/validate.go#Validate",
            "code:internal/workflow/validate.go#InvalidError",
            "code:internal/workflow/catalog.go#Environment"
          ]
        },
        {
          "text": "Validation requires a README.md beside workflow.yaml, schema_version 1, every run limit and a retention classification, and rejects cycles (v1 graphs are acyclic; retries are per step) and steps unreachable from start.",
          "refs": [
            "code:internal/workflow/validate.go#validator.run",
            "code:internal/workflow/validate.go#validator.checkGraph",
            "code:internal/workflow/definition.go#Limits"
          ]
        },
        {
          "text": "Deterministic steps call only reviewed functions compiled into Evie, named with a version such as collect_claims@1; a definition cannot add one, and each is pure, so the same arguments give the same result.",
          "refs": [
            "code:internal/workflow/functions.go#Function",
            "code:internal/workflow/functions.go#reviewedFunctions",
            "doc:cmd/evie/docs/active/workflow-runtime.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "Computes the executable hash, a SHA-256 over one canonical document (format evie.workflow.executable/1) holding the normalized definition with defaults applied and descriptions left out, the digests of the referenced prompts and schemas, and the identities of the referenced functions; README.md, comments, quoting and formatting never reach it.",
          "refs": [
            "code:internal/workflow/hash.go#executableHash",
            "code:internal/workflow/hash.go#normDefinition",
            "code:internal/workflow/hash.go#hashFormat"
          ]
        },
        {
          "text": "Builds the review Report a reviewer reads (the human graph, inputs and outputs, the Run Key rule, capabilities, workers, functions, resources, Retry Policies, Model Policies, requested authority, schedule, retention, limits, the worst-case bounds and warnings) and the ReviewDiff between two versions that the approval card shows.",
          "refs": [
            "code:internal/workflow/report.go#Report",
            "code:internal/workflow/review.go#DiffReports"
          ]
        },
        {
          "text": "RequirementsOf extracts what one approval binds besides the executable hash, including the authority hash, and AuthorityOutside lists requested operations the Workspace's current access does not cover.",
          "refs": [
            "code:internal/workflow/review.go#RequirementsOf",
            "code:internal/workflow/review.go#AuthorityHash",
            "code:internal/workflow/review.go#AuthorityOutside"
          ]
        },
        {
          "text": "Simulate dry-runs a definition over fixtures with every external effect replaced by a preview; fixtures supply AI outputs, worker results, capability read results and answers, so a dry run reaches no model, worker or capability provider and never becomes an Effect Receipt.",
          "refs": [
            "code:internal/workflow/simulate.go#Validated.Simulate",
            "code:internal/workflow/simulate.go#Fixtures",
            "code:internal/workflow/simulate.go#EffectPreview",
            "doc:cmd/evie/docs/active/workflow-runtime.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "Supplies the runtime's pure execution helpers (the step graph, accepted run inputs, the Run Key, each step's resolved input, deterministic function calls, branch decisions and the run's outputs); the simulator uses the same helpers, so a run and its dry run cannot disagree about semantics.",
          "refs": [
            "code:internal/workflow/execute.go#StepPlan",
            "code:internal/workflow/execute.go#Validated.AcceptInputs",
            "code:internal/workflow/execute.go#Validated.Branch",
            "code:internal/workflow/execute.go#Validated.Outputs"
          ]
        },
        {
          "text": "Plans map and agent steps: a map's bounds, failure policy and item step over a list a prior step produced, and an agent step's worker preset, Model Policy, reviewed objective, budget and typed result, taken from the worker report's last ## Result block.",
          "refs": [
            "code:internal/workflow/fanout.go#MapPlan",
            "code:internal/workflow/fanout.go#AgentPlan",
            "code:internal/workflow/fanout.go#Validated.AgentOutput"
          ]
        },
        {
          "text": "Computes Workflow Schedule slots on the schedule's IANA timezone wall clock: a wall time skipped by daylight saving is due when the gap ends and marked shifted, a repeated one is due once at its first occurrence, and $schedule.date and $schedule.time carry the slot's nominal local date and time into the run's inputs.",
          "refs": [
            "code:internal/workflow/schedule.go#Timetable",
            "code:internal/workflow/schedule.go#Slot",
            "code:internal/workflow/schedule.go#Validated.ScheduleInputs"
          ]
        },
        {
          "text": "Decides from the definition alone whether a Successor Run can resume a predecessor's state under a newer version (PlanResume), and which fields a version classifies as sensitive for masking and redaction.",
          "refs": [
            "code:internal/workflow/resume.go#Validated.PlanResume",
            "code:internal/workflow/resume.go#Sensitivity"
          ]
        },
        {
          "text": "Embeds the bundled definitions (today only deep-research) and owns QualifyBuiltin, the rule a built-in or Global definition must meet: no capability steps, no requested Standing Authority, no schedule, and only the read-only research and explore workers.",
          "refs": [
            "code:internal/workflow/templates.go#bundled",
            "code:internal/workflow/builtin.go#QualifyBuiltin",
            "code:internal/workflow/builtin/deep-research/workflow.yaml"
          ]
        }
      ],
      "owns": [
        {
          "text": "The closed workflow.yaml schema: a Definition with its typed inputs, Run Key rule, Model Policies, limits, requested authority, schedule, retention classification, start step, steps and outputs, at schema version 1.",
          "refs": [
            "code:internal/workflow/definition.go#Definition",
            "code:internal/workflow/definition.go#SchemaVersion"
          ]
        },
        {
          "text": "The seven step kinds: deterministic, capability, ai, branch, interrupt, map and agent (map and agent were added by the 2026-10-04 amendment).",
          "refs": [
            "code:internal/workflow/definition.go#KindDeterministic",
            "code:internal/workflow/definition.go#KindAgent",
            "doc:cmd/evie/docs/active/workflow-runtime.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "The schema's hard bounds, such as at most 168 hours per run, 5 attempts per step, a 10-minute backoff, 1,000 map items, 1,000 calls of one operation per run and 31 catch-up runs.",
          "refs": [
            "code:internal/workflow/definition.go#maxRunDuration",
            "code:internal/workflow/definition.go#maxStepAttempts",
            "code:internal/workflow/definition.go#maxCatchUpRuns"
          ]
        },
        {
          "text": "The reviewed registry of deterministic functions, keyed by name@version, compiled into Evie.",
          "refs": [
            "code:internal/workflow/functions.go#reviewedFunctions"
          ]
        },
        {
          "text": "The embedded built-in definition files under internal/workflow/builtin, each with the dry-run fixtures its customized copy is proposed with.",
          "refs": [
            "code:internal/workflow/templates.go#bundled",
            "code:internal/workflow/templates.go#TemplateFixtures",
            "code:internal/workflow/builtin/deep-research/fixtures.json"
          ]
        },
        {
          "text": "No persistent state: it does not run workflows, and activation, runs, leases and effects belong to the runtime and its store.",
          "refs": [
            "code:internal/workflow/definition.go",
            "pkg:internal/workflowruntime"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Validate(src Source, env Environment) is the entry point; the returned Validated exposes Hash, Report, Files, Name, Simulate and the execution helpers the runtime calls.",
          "refs": [
            "code:internal/workflow/validate.go#Validate",
            "code:internal/workflow/validate.go#Validated",
            "code:internal/workflow/review.go#Validated.Files"
          ]
        },
        {
          "text": "Source (ReadFile) is how a definition is read: DirSource for a local directory, procedural.Source for one directory at an exact commit, and an embedded file system for the built-ins.",
          "refs": [
            "code:internal/workflow/catalog.go#Source",
            "code:internal/workflow/catalog.go#DirSource",
            "code:internal/procedural/repo.go#Source",
            "code:internal/workflow/templates.go#TemplateSource"
          ]
        },
        {
          "text": "Catalog is the set of capability contracts a definition validates against; the CLI and the runtime implement it over the Plugin Manager's compiled workflow capabilities.",
          "refs": [
            "code:internal/workflow/catalog.go#Catalog",
            "code:internal/workflow/catalog.go#CapabilityContract",
            "code:cmd/evie/workflow_activation.go#managerCatalog"
          ]
        },
        {
          "text": "EncodeValue and DecodeValue are the canonical JSON in which the runtime records accepted inputs, step outputs and checkpoint state.",
          "refs": [
            "code:internal/workflow/execute.go#EncodeValue",
            "code:internal/workflow/execute.go#DecodeValue",
            "code:internal/workflowruntime/runs.go#successEnd"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "ADR 0019 and the spec let an AI Node receive explicitly requested scoped memory, but references have only the $inputs, $steps, $item and $schedule roots and the runtime sends an AI step only its prompt and declared input, so no step can request memory today.",
          "refs": [
            "adr:0019",
            "doc:cmd/evie/docs/active/workflow-runtime.spec.md#implementation-decisions",
            "code:internal/workflow/refs.go#Ref",
            "code:internal/workflowruntime/ai.go#aiRequest"
          ]
        },
        {
          "text": "ADR 0008 and the glossary bound Standing Authority by accounts, recipients and limits, but an Operation declares only a capability, a connection, exact resources and max_calls_per_run: resources stand in for recipients, and amounts are not in the schema yet.",
          "refs": [
            "adr:0008",
            "term:Standing Authority",
            "code:internal/workflow/definition.go#Operation",
            "doc:cmd/evie/docs/active/workflow-runtime.decisions.md#2026-10-04--effect-protocol-and-reconciliation-stage-14-sub-stage-6"
          ]
        },
        {
          "text": "The Run Key is the workflow name plus a digest of only the inputs the run_key rule names, so inputs outside the rule never distinguish two logical executions.",
          "refs": [
            "code:internal/workflow/execute.go#Validated.RunKey",
            "term:Run Key"
          ]
        },
        {
          "text": "Because README.md and description fields are outside the executable hash, editing them never needs a new approval; the change is still committed to procedural Git and recorded against the active version.",
          "refs": [
            "code:internal/workflow/hash.go#normDefinition",
            "adr:0008",
            "code:internal/workflowruntime/runtime.go#Card.Text"
          ]
        },
        {
          "text": "Prompts are static Markdown with no templating: an AI step's declared input reaches the model as separate data in the user message, never spliced into the reviewed prompt.",
          "refs": [
            "doc:cmd/evie/docs/active/workflow-runtime.spec.md#implementation-decisions",
            "code:internal/workflowruntime/ai.go#aiRequest"
          ]
        },
        {
          "text": "Missed slots are scanned at most 400 days back and the next slot is looked for at most eight years ahead, so a longer outage finds only the bounded window and an impossible date such as 31 February never comes.",
          "refs": [
            "code:internal/workflow/schedule.go#Timetable.Due",
            "code:internal/workflow/schedule.go#Timetable.Next",
            "code:internal/workflow/schedule.go#maxScheduleScanDays"
          ]
        },
        {
          "text": "Schedules need a named IANA timezone (an empty one or Local is refused), and the package embeds time/tzdata so every host computes slots from the same timezone database.",
          "refs": [
            "code:internal/workflow/schedule.go#NewTimetable",
            "code:internal/workflow/validate.go"
          ]
        }
      ],
      "terms": [
        "Workflow Definition",
        "Procedural Workflow",
        "AI Node",
        "Model Policy",
        "Run Key",
        "Retry Policy",
        "Standing Authority",
        "Workflow Schedule",
        "Catch-up Policy",
        "Resource Conflict Key",
        "Capability ID",
        "Workspace Access",
        "Successor Run"
      ],
      "links": [
        "doc:cmd/evie/docs/active/workflow-runtime.spec.md",
        "doc:cmd/evie/docs/active/workflow-runtime.decisions.md",
        "adr:0011",
        "adr:0018",
        "adr:0019"
      ]
    },
    {
      "id": "pkg:internal/workflowruntime",
      "level": "component",
      "kind": "package",
      "title": "workflowruntime",
      "group": "runtime",
      "summary": {
        "text": "The Workflow Runtime's single entry point: it proposes and activates reviewed definitions, starts Workflow Runs for the owner, a chat or a schedule, and advances them under fenced leases through checkpoints, interruptions and the durable effect protocol.",
        "refs": [
          "code:internal/workflowruntime/runtime.go#Runtime",
          "code:internal/workflowruntime/runs.go#Runtime.advance",
          "code:internal/workflowruntime/effects.go#Runtime.effect"
        ]
      },
      "does": [
        {
          "text": "Propose validates a definition against the Workspace's current access, simulates it with any fixtures, commits exactly its reviewed files to procedural Git, validates the commit again (its hash must match), and records the proposal with its report and dry run; nothing is activated.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Runtime.Propose"
          ]
        },
        {
          "text": "The approval card compares the proposal with the current version (the review diff and the Git patch), lists why approval would be refused now, and gives the exact approve command with both hashes and the expected version.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Runtime.card",
            "code:internal/workflowruntime/runtime.go#Runtime.precheck",
            "code:internal/workflowruntime/runtime.go#Card"
          ]
        },
        {
          "text": "Approve re-validates the proposal's exact commit and records the owner's approval, which the store checks again in one transaction (the hashes, a dry run that succeeded, the expected version, worker allowances and authority within access); it activates the version and any schedule it declares, and starts no run.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Runtime.Approve",
            "code:internal/eviedb/workflows.go#Store.ActivateWorkflow",
            "adr:0020"
          ]
        },
        {
          "text": "StartRun resolves the version to pin (the Workspace's approved version, else a Global one, else an enabled built-in), re-reads and re-hashes its approved commit, refuses step kinds it cannot run, accepts the input, derives the Run Key and pins the Execution Composition; the store then records the run with its first checkpoint, or returns the run already holding the key.",
          "refs": [
            "code:internal/workflowruntime/retention.go#Runtime.StartRun",
            "code:internal/workflowruntime/runs.go#Runtime.prepareRun",
            "code:internal/workflowruntime/builtins.go#Runtime.resolve",
            "code:internal/eviedb/workflow_runs.go#Store.StartWorkflowRun"
          ]
        },
        {
          "text": "A start comes from the owner's CLI or web, from a chat's workflow_start (always detached, origin chat), or from a schedule slot (always detached, origin schedule, inputs taken from the approved schedule and the slot).",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.startRun",
            "code:internal/workflowruntime/session.go#Runtime.StartFromChat",
            "code:internal/workflowruntime/schedules.go#Runtime.startSlot"
          ]
        },
        {
          "text": "Start runs the background scheduler in evie serve and the REPL: every poll (one second by default) lists the due runs and starts a pass for each, at most SchedulerConfig.Workers at once (2 by default, EVIE_WORKFLOW_WORKERS up to 32); the first poll is at start, so every startup is a recovery scan.",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.Start",
            "code:internal/workflowruntime/scheduler.go#Runtime.schedule",
            "code:internal/workflowruntime/runtime.go#SchedulerConfig",
            "code:cmd/evie/workflow_runs.go#workflowSchedulerConfig"
          ]
        },
        {
          "text": "Each pass claims the run's fenced lease (30 seconds by default, renewed every third of that), re-checks that the run may continue (a compatible pinned runtime, its pinned providers, run admission, its commit still hashing to its approval) and pauses it visibly when it may not, then advances node by node, reading the run back from SQLite before every step.",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.execute",
            "code:internal/workflowruntime/runs.go#Runtime.heartbeat",
            "code:internal/workflowruntime/runs.go#Runtime.continueRun",
            "code:internal/workflowruntime/runs.go#Runtime.advance"
          ]
        },
        {
          "text": "It executes seven step kinds: branch decisions and deterministic functions, capability reads and effects, AI steps, interruptions, agent steps and map fan-out.",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.step",
            "code:internal/workflowruntime/runs.go#executableKinds"
          ]
        },
        {
          "text": "A failed attempt retries within its step's Retry Policy after the step's backoff, recorded as the run's durable not-before time, so whichever process claims the run next retries from the same checkpoint with the same input.",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.settle",
            "code:internal/workflowruntime/runs.go#Runtime.deterministic",
            "code:internal/eviedb/workflow_runs.go#finishWorkflowAttempt"
          ]
        },
        {
          "text": "Effects follow the intent-call-receipt-checkpoint order: after per-node admission an Effect Intent with a stable idempotency key and its Resource Conflict Keys is recorded with the attempt, the lease is checked once more, the provider is called, and its answer is recorded as the Effect Receipt with the output and the next checkpoint in one transaction.",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.effect",
            "code:internal/workflowruntime/effects.go#Runtime.recordAnswer",
            "code:internal/eviedb/workflow_effects.go#Store.BeginWorkflowEffect",
            "code:internal/eviedb/workflow_effects.go#Store.FinishWorkflowEffect"
          ]
        },
        {
          "text": "An effect without a provable outcome becomes Outcome Unknown: the run needs reconciliation, a provider that reconciles is asked by lookup or idempotent replay after a settle delay, and otherwise the owner reconciles it with evidence; it is never retried by policy.",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.reconcileAutomatically",
            "code:internal/workflowruntime/effects.go#Runtime.Reconcile",
            "code:internal/eviedb/workflow_effects.go#markUnknown",
            "adr:0009"
          ]
        },
        {
          "text": "AI steps send the exact model of their pinned Model Policy two messages, the reviewed prompt and the declared input framed as data with the output schema, through the Models seam; the OpenRouter adapter is wired only when EVIE_WORKFLOW_REMOTE_MODELS is on.",
          "refs": [
            "code:internal/workflowruntime/ai.go#aiRequest",
            "code:internal/workflowruntime/models.go#Models",
            "code:internal/workflowruntime/models.go#OpenRouterModels",
            "code:cmd/evie/workflow_activation.go#workflowModels"
          ]
        },
        {
          "text": "Agent steps run an existing research, explore or build worker through the Kernel subagent supervisor under the run's lease, with the step's Model Policy and budget; the worker's result is the step's output.",
          "refs": [
            "code:internal/workflowruntime/agent.go#Agents",
            "code:internal/workflowruntime/agent.go#Runtime.agentStep",
            "code:cmd/evie/workflow_activation.go#workflowWorkers"
          ]
        },
        {
          "text": "An interrupt step records a typed question and the run waits for the owner holding no lease; Answer validates the reply against the step's schema before the run continues.",
          "refs": [
            "code:internal/workflowruntime/interrupts.go#Runtime.interrupt",
            "code:internal/workflowruntime/interrupts.go#Runtime.answerRun",
            "code:internal/workflowruntime/retention.go#Runtime.Answer"
          ]
        },
        {
          "text": "Cancel records the owner's cancellation at once: no further node starts, an effect in flight completes or is reconciled first, and the holder then ends the run cancelled with its Run Key free.",
          "refs": [
            "code:internal/workflowruntime/retention.go#Runtime.Cancel",
            "code:internal/workflowruntime/cancel.go#Runtime.cancelRun",
            "code:internal/eviedb/workflow_cancellations.go#Store.FinishWorkflowCancellation"
          ]
        },
        {
          "text": "The schedule ticker decides due Workflow Schedule slots under one cross-process runner lease and starts them through the same start path with surface schedule.",
          "refs": [
            "code:internal/workflowruntime/schedules.go#Runtime.tickSchedules",
            "code:internal/workflowruntime/schedules.go#Runtime.startSlot"
          ]
        },
        {
          "text": "Every process enforces the operator's daily cap on all runs' AI calls and worker starts (10,000,000 tokens and $5 per UTC day by default); a refused call pauses its run until the next 00:00 UTC.",
          "refs": [
            "code:internal/workflowruntime/caps.go#Runtime.untilReset",
            "code:internal/workflowruntime/runtime.go#Config.DailyCap",
            "code:cmd/evie/workflow_schedules.go#workflowDailyCap"
          ]
        },
        {
          "text": "Migrate creates a Successor Run under a newer approved version when the predecessor's state is valid there, keeping its Run Key, schedule slot, Effect Receipts and what it already used of its limits.",
          "refs": [
            "code:internal/workflowruntime/migrate.go#Runtime.Migrate",
            "code:internal/eviedb/workflow_migrations.go#Store.MigrateWorkflowRun"
          ]
        },
        {
          "text": "The retention sweep runs hourly under its own runner lease and on the owner's command, applying each classification's policy: mask finished runs' sensitive values on the owner's surfaces, compact checkpoints after seven days, and redact or delete only when configured.",
          "refs": [
            "code:internal/workflowruntime/retention.go#Runtime.Sweep",
            "code:internal/workflowruntime/retention.go#DefaultRetention",
            "code:internal/workflowruntime/scheduler.go#Runtime.Start"
          ]
        },
        {
          "text": "Before the first run pins a built-in version, the runtime commits its embedded files to procedural Git at builtin/workflows/<name> and records the version, so built-in runs read their pinned commit like any approved version.",
          "refs": [
            "code:internal/workflowruntime/builtins.go#Runtime.recordBuiltin"
          ]
        },
        {
          "text": "It implements the Workflows plugin's WorkflowSessions seam, so a chat can start its Workspace's active workflows and read their status.",
          "refs": [
            "code:internal/workflowruntime/session.go#Runtime.StartFromChat",
            "code:internal/workflowruntime/session.go#Runtime.ChatRunStatus",
            "code:internal/plugins/workflows.go#WorkflowSessions"
          ]
        },
        {
          "text": "It lists and marks Durable Notifications and reports today's spend against the daily cap.",
          "refs": [
            "code:internal/workflowruntime/notifications.go#Runtime.Notifications",
            "code:internal/workflowruntime/caps.go#Runtime.Caps"
          ]
        }
      ],
      "owns": [
        {
          "text": "No tables of its own: every durable record goes through its consumer-owned Store interface, which eviedb's Store implements with the workflow tables for proposals, activations, runs, checkpoints, node attempts, leases, effects, interruptions, schedules, notifications, migrations and retention.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Store",
            "code:cmd/evie/workflow_activation.go#newWorkflowRuntime",
            "code:internal/eviedb/workflow_runs.go",
            "code:internal/eviedb/workflow_effects.go",
            "code:internal/eviedb/workflow_schedules.go"
          ]
        },
        {
          "text": "The Execution Composition each run pins as JSON: the runtime version (evie.workflow.runtime/6 today), its step kinds and functions, the pinned capability providers, the Model Policies, and the worker presets with the digest of their composition at start.",
          "refs": [
            "code:internal/workflowruntime/runs.go#composition",
            "code:internal/workflowruntime/runs.go#RuntimeVersion",
            "code:internal/workflowruntime/agent.go#pinnedWorker"
          ]
        },
        {
          "text": "The list of pinned runtime versions it declares it can continue, evie.workflow.runtime/1 through /6; a run pinned to any other pauses visibly rather than being reinterpreted.",
          "refs": [
            "code:internal/workflowruntime/runs.go#compatibleRuntimes",
            "code:internal/workflowruntime/runs.go#Runtime.incompatible",
            "adr:0025"
          ]
        },
        {
          "text": "Only per-process scheduling state lives in memory (the runs this process is advancing, cancellation signals, a schedule-yield time, the built-in cache); each pass reads the run back from SQLite, so any process can continue it from its last checkpoint.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Runtime",
            "code:internal/workflowruntime/runs.go#Runtime.advance"
          ]
        },
        {
          "text": "Its holder name, evie-<pid>-<uuid>, which it takes every run lease under and records on every schedule slot it decides.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#New",
            "code:internal/workflowruntime/schedules.go#Runtime.startSlot"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "New(Config) builds a Runtime from the store, the procedural repository, the capability providers, the operator's worker ceiling, scheduler settings, the model and worker providers, the daily cap and the retention policy; it touches neither Git nor the database.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#New",
            "code:internal/workflowruntime/runtime.go#Config"
          ]
        },
        {
          "text": "Owner operations (Propose, Card, Approve, Deactivate, StartRun, InspectRun, ListRuns, Cancel, Answer, Reconcile, Substitute, Schedules, ControlSchedule, Migrate, Sweep, Notifications) are called by the CLI and the web management API; Propose, Approve, Deactivate, Cancel, Answer, Reconcile, Substitute, ControlSchedule, Migrate and Sweep accept only the cli and web surfaces, and StartRun takes a chat or schedule start only through their own paths.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#ownerSurface",
            "code:cmd/evie/workflow_activation.go#workflowCommands",
            "code:internal/web/workflows.go#WorkflowManager",
            "code:internal/eviedb/workflow_cancellations.go#Store.RequestWorkflowRunCancellation",
            "code:internal/workflowruntime/capabilities.go#Runtime.Substitute",
            "code:internal/workflowruntime/runs.go#Runtime.startRun"
          ]
        },
        {
          "text": "Store is the consumer-owned persistence seam; every run write in it is fenced by the run's current lease, so only the holder of an unexpired lease can begin or finish an attempt, write a checkpoint or fail the run.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Store",
            "code:internal/eviedb/workflow_runs.go#Store.BeginWorkflowAttempt",
            "code:internal/eviedb/workflow_run_leases.go#Store.AcquireWorkflowRunLease"
          ]
        },
        {
          "text": "CapabilityProviders (WorkflowContract and ResolveWorkflowCapability) is the capability seam the Plugin Manager implements; ContractsOnly gives validation contracts with no providers.",
          "refs": [
            "code:internal/workflowruntime/capabilities.go#CapabilityProviders",
            "code:internal/workflowruntime/capabilities.go#ContractsOnly",
            "code:internal/plugins/workflow.go#Manager.ResolveWorkflowCapability"
          ]
        },
        {
          "text": "Models (Complete and Pricing) is the AI step provider seam, with NewOpenRouterModels as its production adapter.",
          "refs": [
            "code:internal/workflowruntime/models.go#Models",
            "code:internal/workflowruntime/models.go#NewOpenRouterModels"
          ]
        },
        {
          "text": "Agents runs agent steps' workers; cmd/evie passes the Kernel subagent supervisor.",
          "refs": [
            "code:internal/workflowruntime/agent.go#Agents",
            "code:cmd/evie/workflow_activation.go#workflowWorkers"
          ]
        },
        {
          "text": "StartFromChat, ChatRunStatus and ChatWorkflows implement plugins.WorkflowSessions for the Workflows plugin's workflow_start and workflow_status tools.",
          "refs": [
            "code:internal/workflowruntime/session.go#Runtime.StartFromChat",
            "code:internal/workflowruntime/session.go#Runtime.ChatWorkflows",
            "code:internal/plugins/workflows.go#WorkflowSessions"
          ]
        },
        {
          "text": "Start(ctx) returns a stop function that ends every pass between nodes, releases the leases and waits at most five seconds.",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.Start",
            "code:internal/workflowruntime/scheduler.go#stopWait"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "ADR 0005 and user story 1 have a workflow proposed and reviewed in an Evie session, but Propose accepts only the owner's cli or web surface, the web management API offers no proposal route (proposals come from the CLI), and a chat cannot propose.",
          "refs": [
            "adr:0005",
            "doc:cmd/evie/docs/active/workflow-runtime.spec.md#user-stories",
            "code:internal/workflowruntime/runtime.go#Runtime.Propose",
            "code:internal/web/workflows.go#WorkflowManager",
            "code:internal/workflowruntime/session.go"
          ]
        },
        {
          "text": "ADR 0017 says revoked Standing Authority pauses active runs before their next covered effect; the code applies run admission in the transaction that records every node attempt, so a deactivation, block or revocation pauses a run at its very next node of any kind.",
          "refs": [
            "adr:0017",
            "code:internal/eviedb/workflow_runs.go#beginWorkflowAttempt",
            "code:internal/workflowruntime/runs.go#Runtime.advance"
          ]
        },
        {
          "text": "ADR 0045 says each Workspace may define retention and export rules, but retention is the operator's policy per classification read from the environment; Workspace rules and export are a follow-up.",
          "refs": [
            "adr:0045",
            "code:internal/workflowruntime/retention.go#RetentionConfig",
            "code:cmd/evie/workflow_migrations.go#workflowRetention",
            "doc:cmd/evie/docs/active/workflow-runtime.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "ADR 0021 says Evie rejects a second active run; the code does not refuse a duplicate start but returns the run already holding the Run Key (Existing is true), and failed, cancelled and superseded runs free the key for a new start.",
          "refs": [
            "adr:0021",
            "code:internal/eviedb/workflow_runs.go#Store.StartWorkflowRun",
            "code:internal/workflowruntime/runs.go#RunResult"
          ]
        },
        {
          "text": "ADR 0046 speaks of Background Runs, but a trigger on workflow_runs records a Durable Notification for every run's terminal or needs-attention transition, foreground CLI starts included.",
          "refs": [
            "adr:0046",
            "code:internal/eviedb/workflow_notifications.go#WorkflowNotificationKind",
            "doc:cmd/evie/docs/active/workflow-runtime.decisions.md#2026-10-04--schedules-and-notifications-stage-14-sub-stage-11"
          ]
        },
        {
          "text": "A foreground start executes the run inline in the caller's process when no scheduler runs there, detached from the caller's cancellation: a caller that stops waiting never stops the run.",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.await"
          ]
        },
        {
          "text": "A detached start from a one-shot evie workflows command only records the run; a running evie serve or REPL executes it.",
          "refs": [
            "code:cmd/evie/workflow_runs.go#runWorkflowRun",
            "code:cmd/evie/workflow_runs.go#startWorkflowScheduler"
          ]
        },
        {
          "text": "AI and agent steps are off unless EVIE_WORKFLOW_REMOTE_MODELS is on with an OpenRouter key: a start of a definition that needs them is refused, and a run that reaches one in an Evie without them pauses until an Evie that allows them claims it.",
          "refs": [
            "code:cmd/evie/workflow_activation.go#workflowRemoteModels",
            "code:internal/workflowruntime/runs.go#Runtime.prepareRun",
            "code:internal/workflowruntime/runs.go#Runtime.continueRun"
          ]
        },
        {
          "text": "While any Effect Intent of a run has no receipt, no node starts and the run is never paused, resumed or failed; that wait still counts toward limits.max_duration, while waits for an answer or at the daily cap do not.",
          "refs": [
            "code:internal/eviedb/workflow_effects.go#requireEffectsResolved",
            "code:internal/workflowruntime/runs.go#Runtime.elapsed",
            "doc:cmd/evie/docs/active/workflow-runtime.decisions.md#2026-10-04--effect-protocol-and-reconciliation-stage-14-sub-stage-6"
          ]
        },
        {
          "text": "A node interrupted three times at one checkpoint (its runtime crashed, stopped or lost the lease before recording the outcome) fails the run instead of running again; interrupted effect attempts do not count, since their outcome was reconciled.",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.step",
            "code:internal/workflowruntime/runtime.go#DefaultInterruptions"
          ]
        },
        {
          "text": "Stopping waits at most five seconds; a pass still inside a node keeps its lease until it expires, and the next claim records that attempt interrupted, or its effect's outcome unknown.",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.Start",
            "code:internal/eviedb/workflow_run_leases.go#Store.AcquireWorkflowRunLease"
          ]
        },
        {
          "text": "An effect's idempotency key derives from the Workspace, the Run Key, the step, the capability, connection, resource and input, so a later run of the same logical execution reuses it; only the owner's rerun override changes the Run Key, and with it every key.",
          "refs": [
            "code:internal/workflowruntime/effects.go#idempotencyKey",
            "code:internal/workflowruntime/cancel.go#rerunKey"
          ]
        },
        {
          "text": "Resource Conflict Keys are scoped only by connection and held across every run and Workspace, so two Workspaces sharing a connection serialize their writes, and max_calls_per_run counts every Effect Intent, retries and a predecessor's intents included.",
          "refs": [
            "code:internal/workflowruntime/effects.go#conflictKeys",
            "code:internal/eviedb/workflow_effects.go#Store.BeginWorkflowEffect",
            "doc:cmd/evie/docs/active/workflow-runtime.decisions.md#2026-10-04--effect-protocol-and-reconciliation-stage-14-sub-stage-6"
          ]
        },
        {
          "text": "A run paused at the daily cap is rechecked only at the next 00:00 UTC; raising the cap during the day helps new calls at once but not that run.",
          "refs": [
            "code:internal/workflowruntime/caps.go#Runtime.untilReset",
            "doc:cmd/evie/docs/active/workflow-runtime.decisions.md#2026-10-04--schedules-and-notifications-stage-14-sub-stage-11"
          ]
        }
      ],
      "terms": [
        "Workflow Run",
        "Workflow Definition",
        "Workflow Approval",
        "Standing Authority",
        "Execution Composition",
        "Effect Intent",
        "Effect Receipt",
        "Outcome Unknown",
        "Run Key",
        "Retry Policy",
        "Resource Conflict Key",
        "Workflow Schedule",
        "Catch-up Policy",
        "Background Run",
        "Foreground Run",
        "Durable Notification",
        "Successor Run",
        "Compatibility Resolution",
        "AI Node",
        "Model Policy",
        "Kernel",
        "Workspace Access"
      ],
      "diagrams": [
        {
          "id": "workflow-run-lifecycle",
          "title": "Workflow Run lifecycle",
          "kind": "state",
          "summary": {
            "text": "The explicit, non-overlapping states a Workflow Run moves through; succeeded, failed, cancelled and superseded are terminal and their history is never rewritten.",
            "refs": [
              "code:internal/eviedb/workflow_runs.go#WorkflowRunState",
              "code:internal/eviedb/workflow_runs.go#WorkflowRunState.Terminal",
              "code:internal/eviedb/workflow_retention.go#workflowRetentionTriggers"
            ]
          },
          "states": [
            {
              "id": "queued",
              "label": "queued",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunQueued",
                "code:internal/eviedb/workflow_runs.go#Store.StartWorkflowRun"
              ]
            },
            {
              "id": "running",
              "label": "running",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunRunning"
              ]
            },
            {
              "id": "waiting_for_human",
              "label": "waiting for a human",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunWaitingForHuman"
              ]
            },
            {
              "id": "waiting_for_dependency",
              "label": "waiting for a dependency (paused)",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunWaitingForDependency"
              ]
            },
            {
              "id": "needs_reconciliation",
              "label": "needs reconciliation",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunNeedsReconciliation"
              ]
            },
            {
              "id": "cancelling",
              "label": "cancelling",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunCancelling"
              ]
            },
            {
              "id": "cancelled",
              "label": "cancelled",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunCancelled"
              ]
            },
            {
              "id": "failed",
              "label": "failed",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunFailed"
              ]
            },
            {
              "id": "succeeded",
              "label": "succeeded",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowRunSucceeded"
              ]
            },
            {
              "id": "superseded",
              "label": "superseded",
              "refs": [
                "code:internal/eviedb/workflow_migrations.go#WorkflowRunSuperseded"
              ]
            }
          ],
          "initial": "queued",
          "transitions": [
            {
              "from": "queued",
              "to": "running",
              "label": "its first node attempt begins",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#beginWorkflowAttempt"
              ]
            },
            {
              "from": "queued",
              "to": "waiting_for_human",
              "label": "an interrupt step asks before any attempt",
              "refs": [
                "code:internal/eviedb/workflow_interruptions.go#Store.AskWorkflowQuestion"
              ]
            },
            {
              "from": "queued",
              "to": "waiting_for_dependency",
              "label": "paused at its claim (provider, admission, models or workers unavailable)",
              "refs": [
                "code:internal/eviedb/workflow_run_leases.go#Store.PauseWorkflowRun",
                "code:internal/workflowruntime/runs.go#Runtime.continueRun"
              ]
            },
            {
              "from": "queued",
              "to": "failed",
              "label": "fails between attempts (a run limit or an unreadable checkpoint)",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#Store.FailWorkflowRun",
                "code:internal/workflowruntime/runs.go#Runtime.step"
              ]
            },
            {
              "from": "running",
              "to": "succeeded",
              "label": "the attempt that ends the graph records the run's outputs",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#finishWorkflowAttempt"
              ]
            },
            {
              "from": "running",
              "to": "failed",
              "label": "an attempt fails with no retry left, or a run limit is reached",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#finishWorkflowAttempt",
                "code:internal/eviedb/workflow_runs.go#Store.FailWorkflowRun"
              ]
            },
            {
              "from": "running",
              "to": "waiting_for_human",
              "label": "an interrupt step records its question",
              "refs": [
                "code:internal/eviedb/workflow_interruptions.go#Store.AskWorkflowQuestion"
              ]
            },
            {
              "from": "waiting_for_human",
              "to": "running",
              "label": "the owner's answer is recorded",
              "refs": [
                "code:internal/eviedb/workflow_interruptions.go#Store.AnswerWorkflowQuestion"
              ]
            },
            {
              "from": "running",
              "to": "waiting_for_dependency",
              "label": "paused: provider unavailable, admission refused, Resource Conflict Key held, or the daily cap reached",
              "refs": [
                "code:internal/eviedb/workflow_run_leases.go#Store.PauseWorkflowRun",
                "code:internal/workflowruntime/runs.go#Runtime.advance",
                "code:internal/workflowruntime/effects.go#Runtime.waitForConflict"
              ]
            },
            {
              "from": "waiting_for_dependency",
              "to": "running",
              "label": "resumed at a claim once its cause is gone and no effect is unresolved",
              "refs": [
                "code:internal/eviedb/workflow_run_leases.go#Store.ResumeWorkflowRun"
              ]
            },
            {
              "from": "running",
              "to": "needs_reconciliation",
              "label": "an effect's outcome becomes unknown",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#Store.MarkWorkflowEffectUnknown",
                "code:internal/eviedb/workflow_effects.go#markOrphanedEffectsUnknown"
              ]
            },
            {
              "from": "needs_reconciliation",
              "to": "running",
              "label": "a receipt from the provider's or the owner's reconciliation",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#Store.ResolveWorkflowEffect"
              ]
            },
            {
              "from": "queued",
              "to": "cancelling",
              "label": "the owner cancels",
              "refs": [
                "code:internal/eviedb/workflow_cancellations.go#Store.RequestWorkflowRunCancellation"
              ]
            },
            {
              "from": "running",
              "to": "cancelling",
              "label": "the owner cancels",
              "refs": [
                "code:internal/eviedb/workflow_cancellations.go#Store.RequestWorkflowRunCancellation"
              ]
            },
            {
              "from": "waiting_for_human",
              "to": "cancelling",
              "label": "the owner cancels",
              "refs": [
                "code:internal/eviedb/workflow_cancellations.go#Store.RequestWorkflowRunCancellation"
              ]
            },
            {
              "from": "waiting_for_dependency",
              "to": "cancelling",
              "label": "the owner cancels",
              "refs": [
                "code:internal/eviedb/workflow_cancellations.go#Store.RequestWorkflowRunCancellation"
              ]
            },
            {
              "from": "needs_reconciliation",
              "to": "cancelling",
              "label": "the owner cancels; it finishes once the effect has its receipt",
              "refs": [
                "code:internal/eviedb/workflow_cancellations.go#Store.RequestWorkflowRunCancellation"
              ]
            },
            {
              "from": "cancelling",
              "to": "cancelled",
              "label": "nothing in flight and every effect has its receipt; open questions are withdrawn",
              "refs": [
                "code:internal/eviedb/workflow_cancellations.go#Store.FinishWorkflowCancellation"
              ]
            },
            {
              "from": "queued",
              "to": "superseded",
              "label": "the owner migrates it to a Successor Run",
              "refs": [
                "code:internal/eviedb/workflow_migrations.go#Store.MigrateWorkflowRun"
              ]
            },
            {
              "from": "running",
              "to": "superseded",
              "label": "the owner migrates it once no lease holds it",
              "refs": [
                "code:internal/eviedb/workflow_migrations.go#Store.MigrateWorkflowRun"
              ]
            },
            {
              "from": "waiting_for_dependency",
              "to": "superseded",
              "label": "the owner migrates a paused run",
              "refs": [
                "code:internal/eviedb/workflow_migrations.go#Store.MigrateWorkflowRun"
              ]
            }
          ]
        },
        {
          "id": "workflow-node-attempt-lifecycle",
          "title": "Node attempt lifecycle",
          "kind": "state",
          "summary": {
            "text": "Every node attempt is recorded running before the node runs and finished exactly once; a finished attempt is immutable, and a retry is a new attempt from the same checkpoint.",
            "refs": [
              "code:internal/eviedb/workflow_runs.go#WorkflowAttemptState",
              "code:internal/eviedb/workflow_runs.go#workflowNodeAttemptsIndexes"
            ]
          },
          "states": [
            {
              "id": "running",
              "label": "running (in flight)",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowAttemptRunning",
                "code:internal/eviedb/workflow_runs.go#beginWorkflowAttempt"
              ]
            },
            {
              "id": "succeeded",
              "label": "succeeded",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowAttemptSucceeded"
              ]
            },
            {
              "id": "failed",
              "label": "failed",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowAttemptFailed"
              ]
            },
            {
              "id": "interrupted",
              "label": "interrupted",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#WorkflowAttemptInterrupted"
              ]
            }
          ],
          "initial": "running",
          "transitions": [
            {
              "from": "running",
              "to": "succeeded",
              "label": "its output is recorded with the next checkpoint in one transaction",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#finishWorkflowAttempt"
              ]
            },
            {
              "from": "running",
              "to": "failed",
              "label": "its error is recorded; retried after the backoff or the run fails; an effect whose outcome became unknown also ends failed",
              "refs": [
                "code:internal/eviedb/workflow_runs.go#finishWorkflowAttempt",
                "code:internal/eviedb/workflow_effects.go#Store.MarkWorkflowEffectUnknown"
              ]
            },
            {
              "from": "running",
              "to": "interrupted",
              "label": "its holder stopped before recording the outcome: the next claim, or the holder itself while stopping, records it",
              "refs": [
                "code:internal/eviedb/workflow_run_leases.go#Store.AcquireWorkflowRunLease",
                "code:internal/eviedb/workflow_run_nodes.go#interruptAttempt"
              ]
            }
          ]
        },
        {
          "id": "workflow-effect-intent-lifecycle",
          "title": "Effect Intent lifecycle",
          "kind": "state",
          "summary": {
            "text": "An Effect Intent's state is derived from its immutable records: pending until a receipt or an unknown outcome is recorded, and settled only by an Effect Receipt.",
            "refs": [
              "code:internal/eviedb/workflow_effects.go#WorkflowEffectState",
              "code:internal/eviedb/workflow_effects.go#WorkflowEffectIntent.State"
            ]
          },
          "states": [
            {
              "id": "pending",
              "label": "pending (call in flight)",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#WorkflowEffectPending",
                "code:internal/eviedb/workflow_effects.go#Store.BeginWorkflowEffect"
              ]
            },
            {
              "id": "unknown",
              "label": "Outcome Unknown",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#WorkflowEffectUnknown",
                "term:Outcome Unknown"
              ]
            },
            {
              "id": "succeeded",
              "label": "succeeded (it happened)",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#WorkflowEffectSucceeded"
              ]
            },
            {
              "id": "failed",
              "label": "failed (proven not to have happened)",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#WorkflowEffectFailed"
              ]
            }
          ],
          "initial": "pending",
          "transitions": [
            {
              "from": "pending",
              "to": "succeeded",
              "label": "the provider's response is recorded as the receipt",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#Store.FinishWorkflowEffect"
              ]
            },
            {
              "from": "pending",
              "to": "failed",
              "label": "the provider proves nothing happened (ErrWorkflowEffectNotPerformed)",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#Store.FinishWorkflowEffect",
                "code:internal/workflowruntime/effects.go#Runtime.recordAnswer"
              ]
            },
            {
              "from": "pending",
              "to": "unknown",
              "label": "no answer in time, a panic, a malformed response, an unproven error, or the holder stopped",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#Store.MarkWorkflowEffectUnknown",
                "code:internal/eviedb/workflow_effects.go#markOrphanedEffectsUnknown"
              ]
            },
            {
              "from": "unknown",
              "to": "succeeded",
              "label": "reconciled as happened, by the provider's lookup or the owner's evidence",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#Store.ResolveWorkflowEffect"
              ]
            },
            {
              "from": "unknown",
              "to": "failed",
              "label": "reconciled as did not happen",
              "refs": [
                "code:internal/eviedb/workflow_effects.go#Store.ResolveWorkflowEffect"
              ]
            }
          ]
        }
      ],
      "links": [
        "doc:cmd/evie/docs/active/workflow-runtime.spec.md",
        "doc:cmd/evie/docs/active/workflow-runtime.decisions.md",
        "adr:0007",
        "adr:0009",
        "adr:0016",
        "adr:0017",
        "adr:0020",
        "adr:0021",
        "adr:0023",
        "adr:0025",
        "adr:0029",
        "adr:0030",
        "adr:0043",
        "adr:0046",
        "adr:0067"
      ]
    },
    {
      "id": "pkg:internal/procedural",
      "level": "component",
      "kind": "package",
      "title": "procedural",
      "group": "runtime",
      "summary": {
        "text": "Evie's procedural Git repository: a private bare repository at ~/.evie/procedural, written only with Git plumbing, that holds proposed and approved Workflow Definitions at exact commits; Git answers what was proposed and approved, while SQLite owns approval records and runs.",
        "refs": [
          "code:internal/procedural/repo.go#Repo",
          "code:internal/procedural/repo.go#DefaultRoot",
          "adr:0007"
        ]
      },
      "does": [
        {
          "text": "New touches nothing; the repository is created on first use with mode 0700 and git init --bare on branch master, and a root that is a link, a file or not Evie's bare repository is refused.",
          "refs": [
            "code:internal/procedural/repo.go#New",
            "code:internal/procedural/repo.go#Repo.ensure"
          ]
        },
        {
          "text": "Commit makes one commit on master whose tree is master's with one directory replaced by exactly the given files, built with hash-object, mktree and commit-tree; it makes no commit when the tree would not change.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.Commit",
            "code:internal/procedural/repo.go#Repo.writeFiles",
            "code:internal/procedural/repo.go#Repo.splice"
          ]
        },
        {
          "text": "Master moves only by a compare-and-swap update-ref, retried on top of a concurrent writer's commit up to 32 times, so no writer's commit is lost and a crash before the ref moves leaves only unreferenced objects.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.Commit",
            "code:internal/procedural/repo.go#maxCASRetries",
            "code:internal/procedural/repo.go"
          ]
        },
        {
          "text": "ReadFile reads one regular file at an exact commit, refusing anything that is not a full commit hash, a clean relative path or a file of at most 1 MiB.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.ReadFile",
            "code:internal/procedural/repo.go#checkCommit",
            "code:internal/procedural/repo.go#MaxFileBytes"
          ]
        },
        {
          "text": "Source returns a workflow Source reading one directory at one commit, so validation at approval and at every run start reads exactly the approved files.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.Source",
            "code:internal/procedural/repo.go#Source.ReadFile",
            "code:internal/workflowruntime/runs.go#Runtime.approvedDefinition"
          ]
        },
        {
          "text": "Diff gives the patch of a directory between two commits (or from nothing) for the approval card, truncated at 512 KiB.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.Diff",
            "code:internal/workflowruntime/runtime.go#Runtime.card"
          ]
        }
      ],
      "owns": [
        {
          "text": "The bare repository at ~/.evie/procedural and its single branch, master.",
          "refs": [
            "code:internal/procedural/repo.go#DefaultRoot",
            "code:internal/procedural/repo.go#branch",
            "node:container:procedural-repo"
          ]
        },
        {
          "text": "The tree its callers lay out: workspaces/<id>/workflows/<name> for Workspace definitions, global/workflows/<name> for Global ones, builtin/workflows/<name> for recorded built-in versions and global/skills/<name> for proposed and approved skills.",
          "refs": [
            "code:internal/eviedb/workflows.go#WorkflowDefinitionPath",
            "code:internal/eviedb/workflow_builtins.go#GlobalWorkflowDefinitionPath",
            "code:internal/eviedb/workflow_builtins.go#BuiltinWorkflowDefinitionPath",
            "code:internal/eviedb/skills.go#GlobalSkillPath"
          ]
        },
        {
          "text": "Commit messages that carry the definition's Executable-Hash and Authority-Hash, written by Evie's own identity.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Runtime.Propose",
            "code:internal/workflowruntime/builtins.go#Runtime.recordBuiltin",
            "code:internal/procedural/repo.go#Repo.Commit"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "*Repo with Head, Commit, ReadFile, Source, Diff and Root; the Workflow Runtime receives one through Config.Repo, and cmd/evie builds it at the default root.",
          "refs": [
            "code:internal/procedural/repo.go#Repo",
            "code:internal/workflowruntime/runtime.go#Config",
            "code:cmd/evie/workflow_activation.go#newWorkflowRuntime"
          ]
        },
        {
          "text": "Source satisfies the workflow package's Source interface (ReadFile by slash path relative to the definition's directory).",
          "refs": [
            "code:internal/procedural/repo.go#Source",
            "code:internal/workflow/catalog.go#Source"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "It shells out to the git binary: every command runs with a built environment (only PATH and HOME inherited, no global or system configuration), hardening flags that turn off hooks, fsmonitor, signing, maintenance and transports, and a two-second wait delay.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.git",
            "code:internal/procedural/repo.go#env",
            "code:internal/procedural/repo.go#hardening"
          ]
        },
        {
          "text": "Every file tool refuses ~/.evie/procedural, so no worker or chat can read or write the reviewed definitions behind the runtime's back.",
          "refs": [
            "code:internal/tools/file.go#deniedDirs",
            "code:internal/procedural/repo.go"
          ]
        },
        {
          "text": "Readers always name an exact commit, so a later commit never changes what an earlier approval or run reads; there is no work tree to leave dirty.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.ReadFile",
            "code:internal/procedural/repo.go#Repo.Source"
          ]
        },
        {
          "text": "ADR 0012 also plans user-authored Agent Presets as reviewed assets in procedural Git, but no code stores presets here: presets are compiled into the plugins package, and only cmd/evie and workflowruntime use this repository.",
          "refs": [
            "adr:0012",
            "code:internal/plugins/preset.go",
            "pkg:internal/procedural"
          ]
        }
      ],
      "terms": [
        "Procedural Memory",
        "Workflow Definition",
        "Workflow Approval",
        "Agent Preset"
      ],
      "links": [
        "adr:0007",
        "adr:0012",
        "adr:0018"
      ]
    },
    {
      "id": "pkg:internal/skill",
      "level": "component",
      "kind": "package",
      "title": "skill",
      "group": "runtime",
      "summary": {
        "text": "Checks one skill in the Agent Skills format against Evie's rules (no scripts, executables or links; only UTF-8 text within the size limits; a valid name and description) and identifies each version by the SHA-256 of its canonical manifest.",
        "refs": [
          "code:internal/skill/skill.go#Parse",
          "code:internal/skill/skill.go#Manifest.Hash",
          "doc:cmd/evie/docs/active/skills.spec.md#implementation-decisions"
        ]
      },
      "does": [
        {
          "text": "ReadFolder checks every entry of a skill folder with Lstat through an os.Root before anything is read or committed, refusing links inside the folder, a top-level scripts/ folder, executable and non-regular files, and stopping at 64 files or 1 MiB; only a folder that is itself a link is followed.",
          "refs": [
            "code:internal/skill/folder.go#ReadFolder",
            "doc:cmd/evie/docs/active/skills.decisions.md"
          ]
        },
        {
          "text": "Parse refuses a missing or invalid name or description, a name unlike its folder or clashing with /compact or /model, and files that are not UTF-8, hold a NUL byte or exceed 32 KiB; it warns about frontmatter Evie ignores, inline shell commands, argument placeholders and mentions of Claude Code tools, and never runs or substitutes any of them.",
          "refs": [
            "code:internal/skill/skill.go#Parse",
            "code:internal/skill/skill.go#Warning"
          ]
        },
        {
          "text": "ReadVersion reads a version back from where it was stored, such as one procedural commit, and requires it to check and hash exactly as recorded.",
          "refs": [
            "code:internal/skill/skill.go#ReadVersion",
            "code:cmd/evie/skills.go#skillCommands.propose"
          ]
        }
      ],
      "terms": [
        "Skill"
      ],
      "links": [
        "doc:cmd/evie/docs/active/skills.spec.md",
        "doc:cmd/evie/docs/active/skills.decisions.md"
      ]
    }
  ],
  "edges": [
    {
      "from": "pkg:internal/procedural",
      "to": "container:procedural-repo",
      "label": "runs hardened git plumbing against",
      "via": "process",
      "refs": [
        "code:internal/procedural/repo.go#Repo.git",
        "code:internal/procedural/repo.go#Repo.Commit"
      ]
    }
  ],
  "flows": [
    {
      "id": "workflow-run",
      "title": "Workflow Run: start to Effect Receipt",
      "summary": {
        "text": "The owner starts a run of an approved workflow; the runtime pins it and records it, the scheduler claims its fenced lease and advances it node by node, and a capability effect goes through the intent-call-receipt-checkpoint sequence until its Effect Receipt and the next checkpoint are recorded together.",
        "refs": [
          "code:internal/workflowruntime/runs.go#Runtime.startRun",
          "code:internal/workflowruntime/runs.go#Runtime.execute",
          "code:internal/workflowruntime/effects.go#Runtime.effect",
          "doc:cmd/evie/docs/active/workflow-runtime.spec.md#implementation-decisions"
        ]
      },
      "trigger": {
        "text": "The owner starts a run from the web with detach (the CLI's evie workflows run, a chat's workflow_start and a schedule slot reach the same startRun).",
        "refs": [
          "code:internal/web/workflows.go#Server.handleWorkflowStart",
          "code:cmd/evie/workflow_runs.go#runWorkflowRun",
          "code:internal/workflowruntime/runs.go#Runtime.startRun"
        ]
      },
      "participants": [
        "person:owner",
        "pkg:internal/web",
        "pkg:internal/workflowruntime",
        "pkg:internal/workflow",
        "pkg:internal/procedural",
        "container:procedural-repo",
        "pkg:internal/eviedb",
        "container:evie-db",
        "pkg:internal/plugins"
      ],
      "steps": [
        {
          "from": "person:owner",
          "to": "pkg:internal/web",
          "label": "POST /api/workflows/start: Workspace, workflow, inputs, detach",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/workflows.go#Server.handleWorkflowStart",
            "code:internal/web/workflows.go#WithWorkflows"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/workflowruntime",
          "label": "StartRun(surface web)",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/web/workflows.go#Server.handleWorkflowStart",
            "code:internal/workflowruntime/retention.go#Runtime.StartRun"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "ResolveWorkflow: Workspace version, else Global, else enabled built-in",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/builtins.go#Runtime.resolve",
            "code:internal/eviedb/workflow_builtins.go#Store.ResolveWorkflow"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/procedural",
          "label": "Source(approved commit, path)",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.approvedDefinition",
            "code:internal/procedural/repo.go#Repo.Source"
          ]
        },
        {
          "from": "pkg:internal/procedural",
          "to": "container:procedural-repo",
          "label": "git cat-file at the exact commit",
          "kind": "call",
          "via": "process",
          "refs": [
            "code:internal/procedural/repo.go#Repo.ReadFile",
            "code:internal/procedural/repo.go#Repo.git"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflow",
          "label": "Validate the commit; its executable and authority hashes must equal the approval's",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.approvedDefinition",
            "code:internal/workflow/validate.go#Validate"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflow",
          "label": "AcceptInputs and RunKey",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.prepareRun",
            "code:internal/workflow/execute.go#Validated.AcceptInputs",
            "code:internal/workflow/execute.go#Validated.RunKey"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/plugins",
          "label": "ResolveWorkflowCapability per capability step: pin the provider in the Execution Composition",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/capabilities.go#Runtime.pinProviders",
            "code:internal/plugins/workflow.go#Manager.ResolveWorkflowCapability"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "StartWorkflowRun: run admission, Run Key check, run and checkpoint 0 in one transaction",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.startRun",
            "code:internal/eviedb/workflow_runs.go#Store.StartWorkflowRun"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "INSERT workflow_runs (queued) and workflow_run_checkpoints",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/workflow_runs.go#Store.StartWorkflowRun"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/web",
          "label": "RunResult: the recorded run, or the run already holding the Run Key",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.startRun",
            "code:internal/workflowruntime/runs.go#RunResult"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflowruntime",
          "label": "wake the scheduler",
          "kind": "note",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.startRun",
            "code:internal/workflowruntime/scheduler.go#Runtime.wake"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "DueWorkflowRuns: runs that are due and whose lease is free",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.schedule",
            "code:internal/eviedb/workflow_run_leases.go#Store.DueWorkflowRuns"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "AcquireWorkflowRunLease: fenced token; an earlier holder's in-flight attempts recorded interrupted",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.execute",
            "code:internal/eviedb/workflow_run_leases.go#Store.AcquireWorkflowRunLease"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "AdmitWorkflowRun, re-read the pinned commit, ResumeWorkflowRun",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.continueRun",
            "code:internal/eviedb/workflow_run_leases.go#Store.AdmitWorkflowRun",
            "code:internal/eviedb/workflow_run_leases.go#Store.ResumeWorkflowRun"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflowruntime",
          "label": "advance: each step from the latest checkpoint, recorded before it runs and finished with the next checkpoint",
          "kind": "note",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.advance",
            "code:internal/workflowruntime/runs.go#Runtime.step",
            "code:internal/eviedb/workflow_runs.go#Store.FinishWorkflowAttempt"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/plugins",
          "label": "checkPinned: the pinned provider must be unchanged and ready, or the run pauses",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.effect",
            "code:internal/workflowruntime/capabilities.go#Runtime.checkPinned"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "BeginWorkflowEffect: per-node admission, max_calls_per_run, Resource Conflict Keys, attempt and Effect Intent",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.effect",
            "code:internal/eviedb/workflow_effects.go#Store.BeginWorkflowEffect"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "INSERT workflow_node_attempts, workflow_effect_intents, workflow_effect_conflicts",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/workflow_effects.go#Store.BeginWorkflowEffect",
            "code:internal/eviedb/workflow_runs.go#beginWorkflowAttempt"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "CheckWorkflowRunLease immediately before the call",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.effect",
            "code:internal/eviedb/workflow_run_nodes.go#Store.CheckWorkflowRunLease"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/plugins",
          "label": "Perform(request with the idempotency key), bounded by the effect timeout",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.callEffect",
            "code:internal/plugins/workflow.go#WorkflowProvider",
            "code:internal/plugins/workflow_effect.go#WorkflowEffectRequest"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/plugins",
          "label": "the provider performs the effect on its external system (finance ledger sync and categorization, GitHub open change)",
          "kind": "note",
          "refs": [
            "code:internal/plugins/finance_effects.go",
            "code:internal/plugins/github.go",
            "adr:0067"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/workflowruntime",
          "label": "response, or an error wrapping ErrWorkflowEffectNotPerformed that proves nothing happened",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.recordAnswer",
            "code:internal/plugins/workflow_effect.go#ErrWorkflowEffectNotPerformed"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflow",
          "label": "CheckCapabilityOutput: the normalized response must fit the contract",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.recordAnswer",
            "code:internal/workflow/execute.go#Validated.CheckCapabilityOutput"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "FinishWorkflowEffect: Effect Receipt, attempt output, next checkpoint, keys released",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.recordAnswer",
            "code:internal/eviedb/workflow_effects.go#Store.FinishWorkflowEffect"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "INSERT workflow_effect_receipts and the next workflow_run_checkpoints row; the run succeeds when the graph ends",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/workflow_effects.go#insertReceipt",
            "code:internal/eviedb/workflow_runs.go#finishWorkflowAttempt"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "otherwise MarkWorkflowEffectUnknown: needs reconciliation, never retried by policy",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/effects.go#Runtime.recordAnswer",
            "code:internal/eviedb/workflow_effects.go#Store.MarkWorkflowEffectUnknown"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "ReleaseWorkflowRunLease when the pass ends",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.execute",
            "code:internal/eviedb/workflow_run_leases.go#Store.ReleaseWorkflowRunLease"
          ]
        }
      ]
    },
    {
      "id": "workflow-schedule-activation",
      "title": "Workflow Schedule activation and catch-up",
      "summary": {
        "text": "The approval that shows a schedule activates it; from then on the schedule ticker of evie serve or the REPL computes the slots due since the schedule's cursor, applies the Catch-up Policy, records skipped slots with a notification and starts each chosen slot through the ordinary start path, recording the slot with its run so no slot starts twice.",
        "refs": [
          "code:internal/workflowruntime/schedules.go#Runtime.tickSchedule",
          "code:internal/workflowruntime/schedules.go#Runtime.startSlot",
          "adr:0020",
          "adr:0023"
        ]
      },
      "trigger": {
        "text": "The owner approves a definition that declares a schedule; afterwards every schedule tick (every 15 seconds by default, first at startup) decides its due slots.",
        "refs": [
          "code:internal/workflowruntime/runtime.go#Runtime.Approve",
          "code:internal/workflowruntime/scheduler.go#Runtime.Start",
          "code:internal/workflowruntime/schedules.go#defaultScheduleTick"
        ]
      },
      "participants": [
        "person:owner",
        "pkg:cmd/evie",
        "pkg:internal/workflowruntime",
        "pkg:internal/workflow",
        "pkg:internal/eviedb",
        "container:evie-db"
      ],
      "steps": [
        {
          "from": "person:owner",
          "to": "pkg:cmd/evie",
          "label": "evie workflows approve <proposal-id> --hash --authority --expect-version",
          "kind": "call",
          "via": "process",
          "refs": [
            "code:cmd/evie/workflows.go#workflowsUsage",
            "code:cmd/evie/workflow_activation.go#runWorkflowActivation"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/workflowruntime",
          "label": "Approve(surface cli)",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:cmd/evie/workflow_activation.go#runWorkflowActivation",
            "code:internal/workflowruntime/runtime.go#Runtime.Approve"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "ActivateWorkflow: the version with its schedule in one transaction; no run starts",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Runtime.Approve",
            "code:internal/eviedb/workflows.go#Store.ActivateWorkflow"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "INSERT workflow_activations (hashes, authority, workers, schedule)",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/workflows.go#Store.ActivateWorkflow",
            "code:internal/eviedb/workflows.go"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/workflowruntime",
          "label": "evie serve or the REPL: Start the scheduler and its schedule ticker",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/workflow_runs.go#startWorkflowScheduler",
            "code:internal/workflowruntime/scheduler.go#Runtime.Start"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "AcquireWorkflowScheduleRunner: single cross-process runner lease, four ticks long",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/schedules.go#Runtime.tickSchedules",
            "code:internal/eviedb/workflow_schedules.go#Store.AcquireWorkflowScheduleRunner",
            "code:internal/workflowruntime/schedules.go#scheduleRunnerLease"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "UPDATE workflow_scheduler_state",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/workflow_schedules.go#Store.AcquireWorkflowScheduleRunner"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "WorkflowSchedules: every current version with a schedule, its status and the owner's pause or resume",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/schedules.go#Runtime.tickSchedules",
            "code:internal/eviedb/workflow_schedules.go#Store.WorkflowSchedules"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "pkg:internal/workflowruntime",
          "label": "cursor: the latest of the approval, the owner's latest resume and the latest decided slot",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/eviedb/workflow_schedules.go#Store.WorkflowSchedules"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflow",
          "label": "NewTimetable, Next, Due(cursor, now, 64) on the timezone's wall clock",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/workflowruntime/schedules.go#Runtime.tickSchedule",
            "code:internal/workflow/schedule.go#NewTimetable",
            "code:internal/workflow/schedule.go#Timetable.Due"
          ]
        },
        {
          "from": "pkg:internal/workflow",
          "to": "pkg:internal/workflowruntime",
          "label": "SlotWindow: the latest due slots oldest first, their total and the first",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/workflow/schedule.go#SlotWindow",
            "code:internal/workflow/schedule.go#Timetable.Due"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflowruntime",
          "label": "Catch-up Policy: skip starts only slots first seen within ScheduleGrace (5 minutes); each_missed starts the latest max_runs; a blocked definition starts none; a paused or deactivated schedule decides nothing",
          "kind": "note",
          "refs": [
            "code:internal/workflowruntime/schedules.go#Runtime.tickSchedule",
            "code:internal/workflowruntime/runtime.go#SchedulerConfig",
            "code:internal/workflow/schedule.go#CatchUpEachMissed",
            "adr:0023"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "RecordWorkflowScheduleSlot(skipped): the latest skipped slot, how many and why",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/schedules.go#Runtime.tickSchedule",
            "code:internal/eviedb/workflow_schedules.go#Store.RecordWorkflowScheduleSlot"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "INSERT workflow_schedule_slots and a workflow_notifications row",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/workflow_schedules.go#Store.RecordWorkflowScheduleSlot"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflowruntime",
          "label": "startSlot, oldest first: startRun with surface schedule, detached, never a rerun",
          "kind": "note",
          "refs": [
            "code:internal/workflowruntime/schedules.go#Runtime.startSlot",
            "code:internal/workflowruntime/runs.go#Runtime.startRun"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "ResolveWorkflow: the active version must still be the slot's version and timezone",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.prepareRun",
            "code:internal/workflowruntime/builtins.go#Runtime.resolve",
            "code:internal/workflowruntime/schedules.go#errScheduleChanged"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflow",
          "label": "ScheduleInputs(slot): the schedule's literals with $schedule.date and $schedule.time, then AcceptInputs and RunKey",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.prepareRun",
            "code:internal/workflow/schedule.go#Validated.ScheduleInputs",
            "code:internal/workflow/execute.go#Validated.RunKey"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "StartWorkflowRun with the slot: run and slot started, or existing when the Run Key is held",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/eviedb/workflow_runs.go#Store.StartWorkflowRun",
            "code:internal/eviedb/workflow_schedules.go#recordStartedSlot"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "INSERT workflow_runs, workflow_run_checkpoints and workflow_schedule_slots in one transaction",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/workflow_runs.go#Store.StartWorkflowRun",
            "code:internal/eviedb/workflow_schedules.go#recordStartedSlot"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "pkg:internal/workflowruntime",
          "label": "ErrWorkflowScheduleSlotTaken when another process decided the slot first; nothing new starts",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/eviedb/workflow_schedules.go#recordStartedSlot",
            "code:internal/eviedb/workflow_schedules.go#ErrWorkflowScheduleSlotTaken",
            "code:internal/workflowruntime/schedules.go#Runtime.startSlot"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/eviedb",
          "label": "a refusal another Evie may not meet leaves the slot undecided, releases the runner lease and keeps away from it for four ticks; any other refusal records the slot failed with a notification",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/workflowruntime/schedules.go#cannotStartHere",
            "code:internal/workflowruntime/schedules.go#Runtime.tickSchedules",
            "code:internal/eviedb/workflow_schedules.go#Store.ReleaseWorkflowScheduleRunner",
            "code:internal/eviedb/workflow_schedules.go#Store.RecordWorkflowScheduleSlot"
          ]
        },
        {
          "from": "pkg:internal/workflowruntime",
          "to": "pkg:internal/workflowruntime",
          "label": "wake: the scheduler advances each started run as in the Workflow Run flow",
          "kind": "note",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.startRun",
            "code:internal/workflowruntime/scheduler.go#Runtime.schedule"
          ]
        }
      ]
    }
  ]
}
);
