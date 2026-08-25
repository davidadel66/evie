# OpenCode-inspired software development backlog

Status: research backlog. Unapproved, unscheduled, and documentation-only.

Nothing in this directory authorizes implementation, changes EVIE's current
behavior, or supersedes an existing decision. When David approves an item, move
that one item into a normal `cmd/evie/docs/active/<feature>.spec.md` and
`<feature>.decisions.md` pair before development starts.

## Research basis

- Project: [anomalyco/opencode](https://github.com/anomalyco/opencode)
- Inspected revision: [`14b37df39168eaf6a6faf862ec4a7bbe9c825bbd`](https://github.com/anomalyco/opencode/tree/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd)
- Inspected: 2026-08-12
- License: [MIT](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/LICENSE)
- Authority: current source under `packages/opencode`, not marketing copy or
  older documentation. OpenCode is mid-migration, so its V2/core code is noted
  where useful but is not assumed to be the shipped path.

The intended use is to learn from OpenCode's mechanics and reimplement the
smallest useful Go version. It is not a proposal to port OpenCode, copy its TUI,
or reproduce its TypeScript/Effect architecture.

## Recommendation

Build the substrate before adding named agents:

1. Add a centralized project registry and bind each conversation immutably to
   global scope or one selected project.
2. Compose trusted project instructions separately from EVIE's stable prompt.
3. Give providers and tools typed, cancellable, provider-neutral boundaries.
4. Enforce profile permissions mechanically, not through prompt promises.
5. Persist sessions and in-flight tool state before introducing subagents.
6. Add context accounting and compaction before long autonomous runs.
7. Introduce `build`, mechanically read-only `plan`, and read-only `explore`.
8. Put deterministic verification and recovery around all mutating work.

OpenCode's most valuable pattern is its execution substrate, not its visual
interface. EVIE should keep its existing web-first rich-output decision.

## Backlog map

### Foundation

| ID | Brief | Recommendation | Depends on |
|---|---|---|---|
| F01 | [Project registry and scoped sessions](features/project-registry.md) | First | None |
| F02 | [Project instructions](features/project-instructions.md) | First | F01 |
| F03 | [Provider-neutral protocol](features/provider-protocol.md) | First | None |
| F04 | [Tool runtime](features/tool-runtime.md) | First | F01, F03 |
| F05 | [Permission policy](features/permission-policy.md) | First | F04 |
| F06 | [Shell and processes](features/shell-processes.md) | First | F01, F04 |
| F07 | [Turn lifecycle](features/turn-lifecycle.md) | First | F03-F06 |
| F08 | [Durable sessions](features/durable-sessions.md) | First | F01, F03, F04, F07 |
| F09 | [Context management](features/context-management.md) | First | F02, F03, F08 |

### Agents and workflows

| ID | Brief | Recommendation | Depends on |
|---|---|---|---|
| F10 | [Agent profiles](agents/profiles.md) | After durable policy | F02, F04, F05, F08 |
| F11 | [Task ledger](features/task-ledger.md) | With profiles | F08 |
| F12 | [Subagent runtime](features/subagents.md) | After persistence | Foreground: F05, F07-F10, F13; background: durable jobs; mutable: F16 |
| F13 | [File and code tools](features/file-code-tools.md) | Early coding slice | F01, F04, F05 |
| F14 | [Verification pipeline](features/verification-pipeline.md) | Early coding slice | Core: F01, F06-F08, F13; blind tier: F16 |
| F15 | [Snapshots and revert](features/snapshots-revert.md) | Before autonomous writes | F08, F13 |
| F16 | [Git worktrees](features/git-worktrees.md) | Before parallel writers | F01, F08, F13; integrates with F12 |
| F17 | [Autonomous builds](features/autonomous-builds.md) | Much later | F07-F16 |

### Optional extensions

| ID | Brief | Recommendation | Trigger |
|---|---|---|---|
| F18 | [Skills and commands](features/skills-commands.md) | Later | Repeated prompt workflows |
| F19 | [Structured output](features/structured-output.md) | Selective | Machine-consumed agent results |
| F20 | [LSP diagnostics](features/lsp-diagnostics.md) | Defer | Shell verification is insufficient |
| F21 | [MCP](features/mcp.md) | Defer | A specific MCP server earns it |
| F22 | [Plugins](features/plugins.md) | Do not port now | Third-party extension demand |
| F23 | [File watching](features/file-watching.md) | Defer | Measured stale-cache problem |
| F24 | [Providers and models](features/providers-models.md) | Keep narrow | A selected second provider |
| F25 | [Coding session UI](features/coding-session-ui.md) | After durable sessions | F08, F10, F15 |
| F26 | [Export and sharing](features/export-sharing.md) | Local export only | F08 |
| F27 | [Agent evaluations](features/agent-evaluations.md) | Start fixtures early | Cross-cutting |
| F28 | [Formatting](features/formatting.md) | Explicit, not automatic | F13, F14; mutation path: F16 |
| F29 | [Project references](features/project-references.md) | Defer | Cross-repository context need |

## Agent catalog

OpenCode has exactly seven built-in profiles at the inspected revision. EVIE's
three review profiles below are additions derived from its proven development
workflow, not OpenCode built-ins.

| ID | Profile | OpenCode status | EVIE direction |
|---|---|---|---|
| A01 | [`build`](agents/build.md) | Visible primary, default | First visible coding profile |
| A02 | [`plan`](agents/plan.md) | Visible primary | Adopt only with mechanical read-only enforcement |
| A03 | [`explore`](agents/explore.md) | Subagent | Best first subagent |
| A04 | [`general`](agents/general.md) | Subagent | Defer; insufficient specialization |
| A05 | [`compaction`](agents/compaction.md) | Hidden internal | Part of F09 |
| A06 | [`title`](agents/title.md) | Hidden internal | Defer until session picker exists |
| A07 | [`summary`](agents/summary.md) | Hidden, apparently dormant | Do not copy without a product use |
| A08 | [Spec reviewer](agents/spec-reviewer.md) | Not built in | EVIE-specific workflow role |
| A09 | [Blind test writer](agents/test-writer.md) | Not built in | EVIE-specific workflow role |
| A10 | [Code reviewer](agents/code-reviewer.md) | Not built in | EVIE-specific workflow role |

OpenCode documentation at this revision mentions a `scout` agent, but no such
built-in exists in the source registry. This backlog follows source.

## Binding EVIE constraints

- This repository remains tutor-first: David writes learning-relevant Go.
  A future `build` profile needs explicit implementation authority and must obey
  project instructions. Merely selecting a profile must not override
  `CLAUDE.md`.
- EVIE's web interface remains the rich surface. This research does not reopen
  the prior decision against investing in a graphics-heavy TUI.
- Purpose-built tools and Bash continue to coexist. Restricted agents cannot be
  called read-only while retaining unrestricted Bash.
- Typed read fences reduce accidental secret leakage; unrestricted Bash is
  outside that guarantee until separately contained.
- SQLite is the selected append-only session/event store in the current memory
  draft. This backlog does not revive the older JSONL storage proposal.
- Deterministic checks outrank model agreement. A reviewer verdict is evidence,
  not proof.

## Promotion rule

Before promoting any brief to `docs/active`:

1. Resolve every item marked `Open decisions` that affects its first stage.
2. Recheck OpenCode's current revision; these notes are pinned in time.
3. Reconcile overlaps with `memory.spec.md`, `docs/delivery-loop.md`, and the
   shipped Bash/file-tool decisions.
4. Write an EVIE-specific spec. OpenCode behavior is evidence, not acceptance
   criteria by itself.
