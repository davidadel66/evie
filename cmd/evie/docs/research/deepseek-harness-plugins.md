# DeepSeek Harness plugins: evidence and implications for Evie

**Research date:** 2026-08-30<br>
**DeepSeek Harness baseline:** commit [`0a53fb55bea101816fa226bb964ae2bed71c343b`](https://github.com/deepseek-ai/deepseek-harness/tree/0a53fb55bea101816fa226bb964ae2bed71c343b), package version `0.1.2-alpha.2`

## Executive conclusion

DeepSeek Harness is useful evidence that a personal agent can make models,
tools, skills, sessions, persistence, sandboxes, storage, loops, schedules,
subagents, workflows, and UI features independently composable. That is not
just a slogan: the shipped base profile is a configuration tree of plugin rows,
and the repository defines explicit service/provider/consumer seams for these
capabilities. Its strongest ideas for Evie are:

1. distinguish a **capability contract**, its **providers**, and its
   **consumers**;
2. compose named plugins into process profiles and per-session presets;
3. make registration reversible and dependency-aware;
4. record the effective composition in the durable session history; and
5. separate "a provider is installed" from "the agent is authorized to invoke
   it."

Evie should adopt those ideas incrementally, not transplant Cordis or make
literally everything replaceable. DeepSeek Harness still has a fixed kernel:
Cordis context/fiber semantics, its loader and configuration format, boot code,
the browser module loader, and event/service contracts. "Everything is a
plugin" accurately describes its **product capability layer**, not an absence
of a trusted substrate. DeepSeek also labels this release a developer preview
with compatibility-breaking changes expected, and its safety document says it
is experimental, unaudited, and not production-ready
([README](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/README.md),
[SAFETY.md](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/SAFETY.md)).

For Evie, authorization, approval, scope resolution, credential mediation,
durable event ordering, lease fencing, migrations, and procedure-run recovery
should remain non-removable kernel invariants. Models, external connectors,
tool collections, skill sources, subagent providers, and eventually a coherent
execution-world provider are good extension seams. The first proof should be a
**Cairo's Kitchen feature pack** built from three narrow capability providers
(Square, Google Sheets, and later a payment rail) plus a reviewed, durable tip
procedure. It should validate the seams before Evie publishes a broad plugin
API.

## What was actually inspected

This note uses the official repository at the exact commit above, including its
vendored Cordis implementation, package manifests, tests, examples, and first-
party documentation. The repository is MIT-licensed
([LICENSE](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/LICENSE)).
The root manifest requires Node `^22.19.0 || >=24.0.0` and identifies the alpha
version
([package.json](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/package.json)).

DeepSeek vendors Cordis so that it is pinned, auditable, and patchable. Its
manifest records upstream snapshot
`56b3d4f725681cf4556c1a8695a709cc3b6eed74`; the checked-in Cordis package
manifest currently reports `4.0.2`
([vendor policy](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/README.md),
[Cordis package](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/cordis/package.json)).
The vendored table and package version are not perfectly synchronized, another
reason to treat the exact commit—not a marketing label—as the reproducible
unit.

## Verified architecture

### Kernel, lifecycle, and dependency graph

A Cordis plugin is a function, object, or `Service` class applied to a
`Context`. The context is a service repository and event bus. A plugin can
declare required and optional injected services; it remains pending until
required providers exist. Registrations are effects with disposers, so unloading
a plugin removes its services and listeners
([Cordis primer](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/cordis-primer.md)).

The implementation has an explicit fiber state machine: pending, loading,
active, failed, unloading, and disposed. It validates Standard Schema
configuration before startup, awaits cleanup, and reevaluates dependents when
an injected provider changes. A provider disappearing unloads its dependents;
its return can reactivate them. Startup failure marks the fiber failed and
rethrows after cleanup
([fiber implementation](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/cordis/src/fiber.ts)).
Effects are reversed as a collection, but asynchronous effect cleanup may run
concurrently; a plugin needing strict internal teardown order must implement
that ordering inside one disposer.

Service isolation is a **namespace/realm mechanism**, not a security boundary:
`Context.isolate()` changes which service identity a subtree sees
([context implementation](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/cordis/src/context.ts)).
The event system provides parallel, serial, bail, and waterfall dispatch, and
listener registration is lifecycle-scoped
([events implementation](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/cordis/src/events.ts)).

Configuration updates are more careful than a simple unload/reload. The loader
imports the replacement before disposing the active entry, applies the change,
and attempts to restore the prior plugin/configuration when the update fails.
Rollback failure is surfaced rather than hidden
([config entry implementation](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/loader/src/config/entry.ts)).
Missing dependencies can nevertheless leave an individual row quietly pending;
profile and preset startup add separate settling/audit checks. This distinction
matters for Evie: dependency reactivity is useful, but application readiness
must fail visibly when a required capability is unavailable.

### Composition: bundle, profile, preset, plugin

DeepSeek uses four related concepts:

| Concept | Actual role |
| --- | --- |
| Plugin | Runtime code contributing a service, event listener, tool, UI element, or other reversible effect. |
| Bundle | Installable package that contributes a configuration patch and code. It is not itself a runnable agent. |
| Profile | Process-level composition of bundles and configuration patches. Later layers replace a row's entire config value. Web mode can live-reload ordinary patch edits; headless, SDK, and ACP modes load at startup. |
| Agent preset | Per-session composition of tools, prompt contributions, skills, and other plugins. Different sessions can use different presets in one process. |

These distinctions are explicit in the
[architecture](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/architecture.md),
[publishing guide](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/user/develop/basic/publish.md),
and [agent preset documentation](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/README.md).
Preset mounting audits reject unscoped targets, pending rows, and services that
leak into the root realm. A preset is still as privileged as the plugins it
names; realm scoping prevents accidental service visibility, not hostile-code
access.

The `dsh plugin` command invokes `pnpm` in a profile. Adding/removing bundle
membership requires restart, while eligible config changes can hot reload. Git
dependencies may run allowlisted `prepare` scripts at install time **outside the
agent sandbox**, so the official guide recommends trusted sources and commit
pins
([CLI reference](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/apps/cli/reference/README.md)).
Thus the install boundary is a software-supply-chain trust decision, not merely
a configuration operation.

At this commit there is no documented, separate plugin-ABI version field or
runtime negotiation protocol. Compatibility comes from package versions and
peer dependencies, schema validation, imports, and runtime service contracts;
the project simultaneously warns that APIs will break during the preview.
Evie should not copy that omission if it ever supports third-party binaries or
processes.

### How a capability reaches one model request

Availability is resolved in layers; the model does not normally choose from an
installed-plugin marketplace at request time.

1. **Profile and bundles establish the host graph.** At boot, the selected
   profile expands its bundles and then applies the profile, home, and CLI patch
   layers. Enabled rows whose dependencies reach `ACTIVE` contribute host
   services and global registrations. This is a deployment/user choice, not a
   model decision
   ([architecture](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/architecture.md)).
2. **An agent preset establishes the session's standing composition.** Session
   creation selects an explicit preset or the configured default. Its
   `agent.cordis.yml` is mounted once per process under a standing scope, and
   the session's agent scope is parented to it. The agent therefore inherits
   that preset's tools, prompt sections, and skill providers without seeing a
   sibling preset's registrations; child agents join the parent's composition.
   A session may switch presets only before producing any message or tool call,
   and the selected preset is recorded durably
   ([agent presets](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/README.md),
   [mount implementation](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/src/mount.ts)).
3. **The tool registry resolves the exact request surface.** Tool plugins
   register definitions into global or scoped layers. For each step, prompt
   assembly merges global registrations with the agent's scope chain, applies
   scoped shadowing and intersecting allow/deny restrictions, and projects the
   surviving definitions to name, description, and JSON schema. Normal mode
   sends all of those schemas; PTC mode sends `run_code` plus an exact generated
   SDK for the same visible set; `both` sends both forms. The assembled set is
   captured in the durable `request/header`
   ([tools](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/tools/README.md),
   [prompt assembly](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/system-prompt/src/index.ts),
   [request construction](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/agent-loop/src/agent.ts)).
4. **Skills use compact discovery plus on-demand loading.** Skill providers are
   merged across the host and agent scope chain; invocation policy filters the
   model-visible view. When the exact `skill` tool is visible, its consumer
   injects a durable sorted catalog containing only each skill's name and a
   capped description. The model selects a name and calls `skill` to load the
   full current body. Provider invalidation can append a complete replacement
   catalog; a user `/name` gesture can instead load a user-invocable skill
   deterministically
   ([skill registry](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/skill/skill/README.md),
   [skill tool](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/skill/tool-skill/README.md)).

Thus, the normal model gets full schemas (or a generated SDK) for exposed tools
and a compact summary catalog only for skills; it reads that list rather than
calling a separate catalog-search API. At this commit, the host plugin
inventory is a read-only Web settings API with explicitly no
model-facing contribution, and no shipped model tool enables an installed
profile row or selects a different preset
([plugin inventory](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/host/plugin-inventory/README.md),
[shipped tool catalog](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/tool-catalog.md)).
Creator mode is the deliberate exception: the opt-in `tool-cordis` tools can
navigate compact directories of **already live** service, event, builtin, and
tool contracts, then define and run process-local JavaScript packages. They do
not install packages or change `cordis.yml`, and no shipped bundle exposes this
toolset by default
([tool-cordis](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/extensions/tool-cordis/README.md)).

### Preset files, authoring, and durable identity

The preset mechanism is filesystem-backed but not Git-backed or review-backed.
Its exact behavior at this commit is:

- **Locations and precedence.** The four shipped directories—`standard`,
  `ptc`, `minimal`, and `cordis`—live inside the agent-presets package under
  `presets/` and enter the roster as a leading `system` root. Configured
  `roots` follow in declaration order, with `trust: user` by default; the
  derived `<dshHome>/.agent-presets` `user` root comes last. Either derived
  root can be disabled. Discovery scans each root on every read, and an earlier
  root wins a duplicate ID, so the shipped set shadows configured/user copies
  unless the shipped root is disabled
  ([shipped tree](https://github.com/deepseek-ai/deepseek-harness/tree/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/presets),
  [configuration and discovery](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/src/index.ts),
  [first-root-wins implementation](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/src/discovery.ts)).
- **Directory format.** The directory name is the stable preset ID and must
  match `[a-z0-9][a-z0-9-]*`. `agent.cordis.yml` is required and must be a
  top-level list of named plugin rows. Optional `preset.yml` contains only
  display `name`, `description`, and roster `order`; trust comes from the root,
  not metadata. The directory may also carry skills, assets, and helpers
  ([preset vocabulary](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/src/preset.ts),
  [metadata](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/src/metadata.ts)).
- **Creation and editing.** The product's only preset-authoring write is
  copy-only: it copies an existing whole directory into the first configured
  `user` root, dereferences symlinks, tightens permissions, refuses overwrite,
  retains the source description, and removes its name/order unless a new name
  is supplied. The browser sends no composition text; custom cards open/reveal
  the directory, and subsequent edits happen in those files outside the
  browser. Shipped presets are read-only in this API. Agent-driven edits to the
  user root may require normal filesystem/sandbox approval, but that approves a
  write operation, not the preset's semantics
  ([authoring implementation](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/src/authoring.ts),
  [Web UI](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/client/ui-agent-preset/README.md),
  [shipped editing skill](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/presets/cordis/skills/editing-cordis-compositions/SKILL.md)).
- **Live change behavior.** Deployment configuration supplies the required
  default, optionally overlaid by a hot-reloaded per-user setting; an explicit
  creation choice overrides it. A changed default affects only later sessions.
  A changed `agent.cordis.yml` is detected by modification-time-plus-size when
  the next session asks for that preset, which starts a new standing
  generation; sessions already joined retain the old in-memory generation even
  if the file is edited or deleted. A blank session can be relinked and logs the
  new ID, but after its first turn it is locked. Deleting a user preset likewise
  leaves existing sessions running and removes it only for later selection
  ([preset lifecycle](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/README.md),
  [generation and selection source](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/src/index.ts)).

There is no preset Git journal, review state, semantic approval record, version
field, or content digest in this subsystem. The durable session header stores
only the starting `agentPreset` string; a blank-session switch appends only
`agent-preset/selected: { agentPreset }`, and a projection folds those IDs
([session header type](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/session/src/types.ts),
[preset session event](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/src/session.ts)).
Neither root/trust, file contents, plugin rows, mtime/size generation stamp, nor
a package-version set is persisted as preset identity. The exact prompt and
tool schemas sent to a model are separately captured in `request/header`, but
on resume the controller reads the stored preset ID and mounts whatever that ID
resolves to then
([resume path](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/api/session-controller/src/agent.ts)).
Therefore, **inference:** a live session is insulated from an on-disk edit, but
a later process resume is not guaranteed to recreate the byte-identical preset
composition that originally produced its history. Evie's reviewed procedural
memory should be stricter: persist a content/version identity and make upgrade
or rebinding an explicit reviewed transition.

### Are the claimed subsystems really plugins?

Yes at the product layer, with important differences in seam maturity.
DeepSeek's generated capability map explicitly distinguishes a service
definition, provider, and consumer; a folder containing multiple packages is
not automatically a swappable seam
([capability seams](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/capability-seams.md)).
The shipped base bundle confirms actual configured rows for the major
capabilities
([base composition](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/bundle/base/cordis.patch.yml)).

| Claimed subsystem | Verified implementation and qualification |
| --- | --- |
| Models | `llm` defines the service; DeepSeek and Pi AI packages provide adapters. New providers subclass/register an adapter rather than alter the loop ([LLM service](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/llm/llm/README.md)). |
| Tools | `tools` owns a typed registry and execution/policy pipeline; `tool-*` packages contribute definitions. Native calls and programmatic tool calling are presentation modes over the same registry ([tools](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/tools/README.md)). |
| Skills | `skill` is a registry, `skill-filesystem` supplies a source, and `tool-skill` exposes lookup to the model ([skill registry](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/skill/skill/README.md)). |
| Sessions and persistence | `session` is an event-sourced in-memory service. JSONL and SQLite durability are separate providers, which is a real storage seam ([session](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/session/README.md), [persistence contract](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/session/session-persistence/README.md)). |
| Sandbox/execution | `sandbox` defines policy/confinement services and `sandbox-local` provides one implementation. The docs stress that the current seam constrains same-world filesystem/argv effects; a container, microVM, or remote implementation should replace the coherent shell/filesystem capability together, not pretend a worker thread is a security boundary ([sandbox](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/sandbox/sandbox/README.md)). |
| Storage | A storage service is backed by JSON or SQLite providers and a domain layer; the base profile chooses them in configuration ([base composition](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/bundle/base/cordis.patch.yml)). |
| Agent and loop | The agent service and default `agent-loop` are separate packages/config rows. The loop is replaceable in principle, but its session/tool/compaction contracts make it a wide and expensive seam, not a casual swap ([architecture](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/architecture.md)). |
| Scheduling | `schedule` is an opt-in plugin with durable schedule/change events and session reminders. It is not advertised as a complete always-on daemon by itself ([schedule](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/schedule/schedule/README.md)). |
| UI | Web features are client plugin rows, while a fixed browser module kernel loads packages declaring a `dsh.client` entry. The UI is extensively extensible, but module delivery remains substrate ([web composition](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/bundle/web-app/cordis.patch.yml)). |
| Agents/subagents | `subagent` supplies a registry and providers include in-process, Codex, and Claude Code. Installing the Codex bundle only adds a dormant provider; a separate tool row is needed to expose it to the model—an excellent availability-versus-authority separation ([Codex provider](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/subagent/subagent-codex/README.md), [bundle patch](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/subagent/subagent-codex/cordis.patch.yml)). |
| Workflow | The workflow service and worker-thread engine are plugins. Today it runs model-written JavaScript orchestration over subagents but explicitly has no journal/resume mechanism, so it is not a durable business-workflow engine ([workflow](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/workflow/workflow/README.md), [worker engine](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/workflow/workflow-worker-thread/README.md)). |

### Static extensions versus model-created runtime extensions

DeepSeek has two plugin stories that should not be conflated.

The normal system installs package code and composes it statically through
profiles/bundles/presets. Separately, the optional Cordis Creator tool lets a
model inspect the live graph and define, run, stop, and remove session-scoped
plugin code. Those definitions are process-local and disappear on restart;
while active, they can affect other sessions in the same process. No shipped
bundle exposes the authoring tool by default
([tool-cordis](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/extensions/tool-cordis/README.md),
[dynamic Cordis guide](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/user/develop/practice/dynamic-cordis.md)).

The host runner uses a VM to isolate globals, but its own documentation says
this is not a security boundary and should be treated as Bash-equivalent
authority. Synchronous VM timeouts do not contain asynchronous work, and
browser-side activation may wait indefinitely for an approving page
([host runner](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/extensions/cordis-host-runner/README.md)).
Likewise, the programmatic code runtime uses a worker thread for containment,
not security; Node access and child-process lifetime create explicit escape
limits
([worker-thread runtime](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/code-runtime/code-runtime-worker-thread/README.md)).
This is an experimental live-extension laboratory, not a suitable foundation
for unattended restaurant payments.

### Modes: what is architectural and what is packaging

The four marketed modes are shipped agent presets, not four different kernels:

| Marketed mode | Source preset | What changes |
| --- | --- | --- |
| Standard | `standard` | Full coding-agent composition. |
| Code | `ptc` | Largely the standard capabilities presented through a generated TypeScript SDK and `run_code`. |
| Minimal | `minimal` | Fixed prompt plus persistent Bash and editor tools; no compaction. Intended for controlled benchmarking. |
| Creator | `cordis` | Standard composition plus runtime inspection/authoring tools and composition-writing skills. |

The exact definitions are in the immutable
[standard](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/presets/standard/agent.cordis.yml),
[PTC](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/presets/ptc/agent.cordis.yml),
[minimal](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/presets/minimal/agent.cordis.yml),
and [Cordis](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/presets/cordis/agent.cordis.yml)
presets. Modes demonstrate the value of named composition; they do not prove
that every agent needs runtime self-modification.

### Durability, observability, and failure handling

The session model is a particularly strong design. It stores an append-only,
typed event stream; model history is derived rather than separately persisted.
The system prompt, tool schemas, request header, raw model chunks, tool calls,
and results are represented in the log. Fork, resume, projection, and query
operate on that stream
([session design](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/session/README.md)).
The persistence contract requires contiguous sequence numbers and durable
append. Recovery repairs an interrupted tool call with a synthetic
"outcome unknown" event instead of replaying the side effect blindly. Its
format is still version `0` and documents no general migration path
([persistence](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/session/session-persistence/README.md)).

"Everything the model sees is logged" is substantially true. It should not be
expanded into "everything the runtime did is reproducible": worker-program
intermediate state, OS side effects, network responses, package code, external
service state, and nondeterminism require independent capture or pinning.
Profiles make controlled comparison easier, but no inspected primary source
establishes that merely selecting Minimal mode makes a benchmark reproducible.

The framework also supports package-owned runtime invariants through a registry
([diagnostics invariants](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/runtime-diagnostics/invariants/README.md)).
That is useful for plugin health checks, but an Evie safety invariant must not
become optional merely because the package that checks it was omitted from a
profile.

## Claim check against the attached article

The article was used only as a list of claims to test.

| Claim | Verdict | Evidence and qualification |
| --- | --- | --- |
| The project is open source. | Verified. | Official repository; MIT license at the inspected commit ([LICENSE](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/LICENSE)). |
| "Everything is a plugin," including model, memory, tools, skills, loop, sandbox, session, storage, scheduler, UI, and subagents. | Verified for product capabilities, overstated literally. | The architecture and base/web configuration make these rows composable, but Cordis, loaders, boot, browser module delivery, and contracts remain kernel ([architecture](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/architecture.md), [base](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/bundle/base/cordis.patch.yml)). |
| A model or sandbox can be swapped through configuration without changing core source. | Mostly verified. | Providers are selected as configuration rows. The replacement must implement the service contract; remote/container execution should replace a coherent shell/filesystem world, and alpha compatibility is not stable ([capability seams](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/capability-seams.md), [sandbox](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/sandbox/sandbox/README.md)). |
| Four modes are Standard, Code, Minimal, and Creator. | Verified naming, with a source nuance. | Their source IDs are `standard`, `ptc`, `minimal`, and `cordis`; they are presets over one runtime, not independent harnesses ([preset definitions](https://github.com/deepseek-ai/deepseek-harness/tree/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/presets)). |
| Code mode performs TypeScript orchestration. | Verified. | PTC presents registered tools as a generated SDK executed through `run_code` ([tools](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/tools/README.md)). Comparisons to unrelated agents are editorial, not established by the source. |
| Minimal has Bash and editor tools for benchmarking. | Verified. | The immutable preset contains those tools and no compaction ([minimal preset](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/presets/minimal/agent.cordis.yml)). Reproducibility still requires pinned model route, code, environment, inputs, and external state. |
| Creator can inspect and create live plugins. | Verified, with a major trust qualification. | It is optional, ephemeral, process-local, cross-session in effect, and Bash-equivalent—not a security boundary ([tool-cordis](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/extensions/tool-cordis/README.md), [host runner](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/extensions/cordis-host-runner/README.md)). |
| The append-only log captures everything the model sees and supports replay/fork/search. | Mostly verified. | Model-visible context is event-derived and query/fork facilities exist. This is not a complete recording of external reality, and persistence is format version `0` ([session](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/session/README.md), [persistence](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/session/session-persistence/README.md)). |

## Evie today: the useful seams already exist

Evie currently composes concrete services manually in `main`: OpenRouter,
SQLite, the session runtime, tools, and web/CLI frontends
([main wiring](../../../../cmd/evie/main.go)). `internal/agent` already owns a
small consumer-side `Client` interface for model streaming, while frontend
events are abstracted separately
([agent session](../../../../internal/agent/agent.go)). This is a good beginning:
an extension architecture should formalize existing consumer-owned seams, not
replace them with a universal service locator.

Tools are currently registered in a package-global slice and per-turn extras
are merged during schema construction/dispatch. Crucially, centralized
execution performs authorization and approval around prepare/execute
([tool registry](../../../../internal/tools/registry.go)). That centralized
boundary is more important than making the registry dynamically replaceable.

The active [memory specification](../active/memory.spec.md) already defines the
right durable invariants: provider-neutral append-only events; harness-resolved
scope; retrieved material that cannot change permissions, the system prompt, or
approval; lease-only provider/tool/event activity; durable and idempotent
background work; tool intent/approval/outcome ordering; no blind retry after an
unknown result; and isolation of memory data from generic file/SQL tools.
Procedural memory is approved as reviewed workflows, skills, and checklists in
Git-backed Markdown, but is not yet implemented.

This means Evie's procedural layer need not be "just a skill." A useful split
is:

- a **procedure definition** is reviewed, versioned declarative content;
- a **procedure engine** is trusted core runtime state that checkpoints steps,
  enforces idempotency/leases/approval, and resumes safely;
- an **agent decision step** may ask a model to classify, extract, reconcile, or
  propose;
- a **deterministic step** computes, validates, or calls a typed capability;
- a **capability provider** is a plugin behind a narrow contract; and
- a **feature pack** selects providers, procedure definitions, policy, and UI.

That is a deterministic agentic graph with bounded AI nodes. It preserves the
reviewed-procedure decision while allowing intelligence where inputs are messy.
DeepSeek's current workflow plugin is useful evidence for a swappable engine,
but its lack of journaling/resume makes it unsuitable as Evie's durable
procedure engine.

## Recommended target shape

### Keep these in the trusted kernel

The following are policy and consistency boundaries, not third-party extension
points:

- durable event sequencing, transactions, recovery, and schema migration;
- turn/run ownership, leases, cancellation, and duplicate suppression;
- authorization, approval, monotonic policy enforcement, and audit records;
- credential custody and scoped token issuance;
- scope resolution and data-boundary enforcement;
- procedure-run checkpoints, idempotency keys, compensation state, and
  "outcome unknown" handling;
- plugin/process admission, version negotiation, health/readiness, and
  effective-composition recording.

Plugins may contribute policy requests or health checks, but they must not be
able to remove these guards. DeepSeek's tools pipeline provides a related
pattern: extensible pre-execution hooks are followed by guarded policy so a
later extension cannot simply re-allow a denied operation
([tools policy pipeline](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/tools/README.md)).

### Make these extension seams, when a second implementation exists

1. **Model provider/route** behind the existing consumer-owned chat interface.
2. **Tool or connector provider** that registers typed operations but must pass
   the kernel execution/approval pipeline.
3. **Skill/procedure source** that supplies content; trust and activation remain
   separate decisions.
4. **Subagent provider** with bounded invocation, cancellation, and observable
   results.
5. **Execution world** as one coherent capability covering filesystem,
   subprocess, shell, and containment guarantees. Do not independently swap
   pieces that could refer to different worlds.
6. **Presentation/UI contribution** only after its security and data access are
   explicit.

Memory storage, scheduling, workflow engines, and the main agent loop could
eventually have internal provider seams for testing or deployment variants.
Publishing them as third-party contracts now would freeze Evie's most rapidly
changing semantics.

### Vocabulary for Evie

| Evie term | Meaning |
| --- | --- |
| Capability | A narrow, consumer-owned interface such as `Model`, `SquareRead`, `SheetWrite`, or `ExecutionWorld`. |
| Provider | One implementation registered under a stable ID and version. Installation makes it available, not agent-visible. |
| Plugin | A lifecycle-managed provider or contribution compiled into Evie initially, and possibly out-of-process later. |
| Feature pack | A curated domain bundle selecting plugins, procedures, policy defaults, mappings, and optional UI. Cairo's Kitchen belongs here. |
| Preset | A named per-session selection of already-admitted capabilities and tools. |
| Procedure | A reviewed, versioned, durable graph of deterministic and bounded-agent steps. |

This avoids calling Cairo's Kitchen one giant plugin. Square access, Sheets
access, payout access, the restaurant's rules, and the tip workflow change for
different reasons and cross different risk boundaries. The feature pack
composes them into one user-facing capability without coupling their internals.

## Phased adaptation

### Phase 0 — define seams from actual pressure

Inventory current construction and identify service definition/provider/
consumer triples. Write criteria for an extension seam: at least two plausible
providers, a consumer-owned interface, explicit authority, lifecycle,
cancellation, health, and durable identity. Do not add a generic container yet.

### Phase 1 — internal, compile-time plugin descriptors

Move wiring behind a small typed composition root. Register built-in providers
with stable IDs, configuration schemas, declared dependencies, start/stop, and
health. Keep Go interfaces in consuming packages. Route every contributed tool
through today's centralized authorization/approval executor.

This provides modularity without runtime code loading. It is also the portable
default for Go: the standard library's `plugin` package is supported on only a
subset of operating systems, requires exact agreement on toolchain/build tags/
dependencies, cannot close plugins, and warns that IPC may be more suitable
([official Go `plugin` documentation](https://pkg.go.dev/plugin)).

### Phase 2 — declarative profiles and per-session presets

Allow configuration to select **already admitted** provider IDs. Validate the
whole dependency graph before readiness; do not accept silently pending required
services. Record provider IDs, versions, config hashes, prompt version, and tool
schema snapshot/hash in the session/run header. Require restart for storage,
authorization, procedure-engine, or active-session topology changes.

### Phase 3 — Cairo's Kitchen vertical slice

Build one feature pack with:

1. a read-only Square provider for shifts, employees, and tip inputs;
2. a narrowly scoped Google Sheets provider for the designated workbook/range;
3. Cairo-specific mappings, allocation rules, validations, and a reviewed
   `calculate-and-record-daily-tips` procedure.

The procedure should fetch a dated immutable input snapshot, resolve identities,
compute deterministically in integer cents, reconcile totals, produce a review
artifact, require approval before the Sheets write, write with an idempotency
key, and record the receipt/outcome. Add Venmo or another payment provider only
after this read/calculate/review/write path has demonstrated reliable retries,
correction, and audit. Payment becomes a separate high-risk procedure step with
fresh approval—not ambient authority inherited by the entire Cairo pack.

This slice answers the real architecture questions: provider identity, OAuth
custody, configuration scoping, tool contribution, procedure versioning,
idempotency, recovery, and observability. A general plugin loader built first
would guess at all of them.

### Phase 4 — external-process protocol, only when distribution demands it

If two or three independently shipped providers need runtime installation,
prefer a supervised process boundary over Go's in-process `plugin` package.
Define an explicit handshake with protocol version, plugin ID/version,
capabilities, schemas, authority requests, health, cancellation, deadlines, and
message-size/resource limits. Pin and verify artifacts. Mediate credentials and
filesystem/network access from the kernel. Start with install-plus-restart;
dynamic unload is not required for useful extensibility.

### Phase 5 — narrowly scoped hot reload

Only after lifecycle tests exist, allow live replacement of stateless
contributions such as prompts, skills, or UI metadata. Quiesce consumers before
replacement; reject unload during active calls; roll back configuration on
failed activation; surface failed and pending state. Storage, migrations,
authorization, active procedures, and active session loops should remain
restart-boundary changes.

### Phase 6 — optional creator laboratory

If model-authored plugins remain desirable, run them in a disposable,
separately permissioned environment with no production credentials. Treat the
result as a prototype that must be exported, reviewed, tested, versioned, and
installed through the normal admission path. Do not interpret a VM or worker
thread as isolation.

## Risks and open questions

- **Contract freeze:** publishing a generic service locator or broad loop/store
  interface too early will make internal evolution expensive. Start with
  consumer-owned contracts and proven alternative providers.
- **Authority laundering:** a feature pack must not gain all permissions of
  every installed provider. Availability, activation, agent exposure, and
  per-call approval need separate states.
- **Lifecycle complexity:** provider loss, partial activation, asynchronous
  cleanup, active calls, and rollback form a distributed-state problem even
  inside one process. Restart boundaries are a feature until Evie needs more.
- **Replay ambiguity:** a composition hash helps explain a run but cannot replay
  external service state. Input snapshots and external receipts remain
  necessary.
- **Credential boundary:** can a provider receive a scoped operation/token
  without ever reading the durable credential? This should be designed before
  third-party providers.
- **Migration ownership:** plugin-owned arbitrary SQL would undermine recovery
  and downgrade. Prefer kernel-reviewed, monotonic migrations or provider-owned
  stores outside Evie's database.
- **Execution-world semantics:** what guarantees must be common across local,
  container, remote, and subagent execution? The interface must describe
  containment honestly, not merely use the word "sandbox."
- **Procedure schema:** the memory specification should decide node types,
  versioning, checkpoint semantics, approval placement, compensation, and
  upgrade behavior before Cairo's workflow is persisted.
- **Install trust:** signature/provenance, dependency scripts, revocation, and
  rollback are prerequisites for a plugin marketplace, not later polish.

## Primary-source index

- DeepSeek Harness
  [README](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/README.md),
  [architecture](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/architecture.md),
  [capability seams](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/capability-seams.md),
  [safety](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/SAFETY.md),
  and [CLI reference](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/apps/cli/reference/README.md).
- Vendored Cordis
  [primer](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/docs/cordis-primer.md),
  [fiber](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/cordis/src/fiber.ts),
  [context](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/cordis/src/context.ts),
  [events](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/cordis/src/events.ts),
  and [loader transaction](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/vendor/loader/src/config/entry.ts).
- DeepSeek package contracts:
  [tools](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/tools/README.md),
  [session](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/core/session/README.md),
  [session persistence](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/session/session-persistence/README.md),
  [sandbox](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/sandbox/sandbox/README.md),
  [presets](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/preset/agent-presets/README.md),
  [workflow](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/workflow/workflow/README.md),
  and [dynamic extensions](https://github.com/deepseek-ai/deepseek-harness/blob/0a53fb55bea101816fa226bb964ae2bed71c343b/packages/extensions/tool-cordis/README.md).
- Official Go [`plugin` package documentation](https://pkg.go.dev/plugin).
