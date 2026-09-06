## Problem Statement

Evie's active memory design currently recognizes global, project, and session
scope; treats procedural memory as approved Markdown instructions and reusable
workflows; binds every execution event and tool start to a conversational
session lease; and permits later turns to continue past unfinished ordinary tool
intent. Those rules were correct for the conversational and memory features they
describe, but they cannot represent a non-filesystem Workspace or a durable
Workflow Run that continues after its originating session disconnects.

Leaving the current language unchanged would create direct contradictions. It
would either force Cairo's Kitchen into global or project memory, or allow a
background workflow to misuse a short conversational lease. It would also blur
Git's authority over reviewed definitions with SQLite's authority over live
execution and incorrectly apply ordinary conversational recovery rules to
financial or business effects.

## Solution

Amend the active memory specification and decision record narrowly. Add
Workspace to the scope model and define Context Scope as exactly one Workspace,
one filesystem project, or neither for a session. Extend procedural Git to hold
reviewed Workspace Revisions, Agent Presets, Skills, and Workflow Definitions.
Keep Workflow Run state in a separate SQLite runtime with its own identity,
leases, effect ledger, and recovery rules.

Preserve the existing conversational behavior: ordinary retrieved memory cannot
grant authority, conversational tool starts remain fenced by the session-turn
lease, and unfinished ordinary tool intent still does not block later chat
turns. Add explicit exceptions for approved Workflow Definitions and Workflow
Runs rather than silently changing the meaning of those existing guarantees.

## User Stories

1. As Evie's owner, I want Cairo's Kitchen memory scoped to a Workspace, so that it is neither global nor tied to a repository.
2. As Evie's owner, I want every session to have at most one Context Scope, so that scoped memory cannot mix implicitly.
3. As Evie's owner, I want global memory available where appropriate, so that owner-wide preferences remain reusable.
4. As Evie's owner, I want Workspace memory excluded from other Workspaces and projects, so that business and personal knowledge remain isolated.
5. As Evie's owner, I want new memories to default to the active Context Scope, so that scope follows explicit session creation rather than model inference.
6. As Evie's owner, I want Workspace-to-global promotion explicit, so that local facts do not widen scope automatically.
7. As Evie's owner, I want Workspace configuration reviewed in procedural Git, so that access and behavior changes have readable history.
8. As Evie's owner, I want Agent Presets reviewed and versioned as procedural assets, so that session composition changes are inspectable and rollbackable.
9. As Evie's owner, I want Skills distinguished from Workflow Definitions, so that interpreted instructions are not mistaken for durable execution.
10. As Evie's owner, I want Workflow Definitions versioned in Git, so that approval refers to exact readable content.
11. As Evie's owner, I want Workflow Runs stored outside procedural Git, so that changing checkpoints and effect receipts do not dirty the reviewed repository.
12. As Evie's owner, I want Workflow Runs independent of session lifetime, so that closing a chat does not cancel background work.
13. As Evie's owner, I want conversational and workflow leases separated, so that a background run never reuses expired conversational authority.
14. As Evie's owner, I want ordinary chat tool behavior preserved, so that this amendment does not unexpectedly block later conversation after an unfinished tool intent.
15. As Evie's owner, I want dependent workflow nodes blocked by an unresolved effect, so that business automation cannot continue from uncertain external state.
16. As Evie's owner, I want Git, SQLite, and external providers named as distinct authorities, so that recovery never assumes a transaction across systems.
17. As Evie's owner, I want Workflow Approval separated from ordinary procedural approval, so that future-run authority exists only in an explicit typed definition.
18. As Evie's owner, I want Standing Authority enforced by the Kernel, so that procedural text cannot change approval rules by instruction.
19. As Evie's owner, I want Workflow Runs to reference an originating session or schedule optionally, so that durable work has provenance without requiring a live session.
20. As Evie's owner, I want workflow inputs, outputs, prompts, corrections, intents, responses, and receipts durable, so that restart and audit do not rely on model recollection.
21. As Evie's owner, I want sensitive fields redacted and credentials excluded, so that durable workflow evidence does not become a secret store.
22. As Evie's owner, I want semantic replay distinguished from workflow resume, so that rebuilding memory projections never re-executes external effects.
23. As Evie's owner, I want production workflow time travel deferred, so that replay language does not imply safe repetition of side effects.
24. As a maintainer, I want Stage 7 to remain focused on reviewed procedural assets, so that a Git feature does not silently become a workflow engine.
25. As a maintainer, I want durable workflow execution specified as a separate dependent stage, so that its persistence, authority, recovery, and concurrency receive independent verification.
26. As a maintainer, I want existing memory compiler jobs and Workflow Runs kept separate, so that two different lifecycles are not forced through one misleading table or state machine.
27. As a maintainer, I want current memory source authority and retrieval protections preserved, so that workflow additions do not weaken provenance or prompt-injection defenses.

## Implementation Decisions

- The memory domain adds Workspace scope with stable Workspace identity. The
  allowed retrieval set for a Workspace session is global, its pinned Workspace,
  and its session. The allowed set for a project session remains global,
  project, and session. A global session uses global and session.
- A session stores exactly one Context Scope variant: Workspace, project, or
  neither. Workspace and project fields cannot both be populated.
- Scope selection remains harness-owned and explicit. The model may not select,
  attach, or widen a Context Scope through text or a tool argument.
- New semantic memory defaults to the active Context Scope. Promotion from
  Workspace or project to global is an explicit accepted semantic operation
  with source linkage.
- Procedural memory remains Git-backed and reviewed, but its taxonomy becomes
  explicit: Workspace Revisions and Agent Presets describe configuration;
  Skills provide model-interpreted instructions; Workflow Definitions provide
  declarative reviewed graphs and requested Standing Authority.
- The procedural repository gains Workspace-specific areas alongside global
  and project assets. Required content is resolved mechanically from immutable
  session scope and pinned content identities.
- Git commit and canonical content hash are authoritative for approved
  Workspace Revisions, Agent Presets, Skills, and Workflow Definitions.
- SQLite conversational events remain authoritative for session history.
  SQLite semantic operations remain authoritative for accepted semantic
  mutations. A separate SQLite workflow ledger is authoritative for Workflow
  Runs, node attempts, leases, interruptions, effects, and notifications.
  External systems remain authoritative for their effects.
- Procedural Git operation state remains distinct from Workflow Run state. The
  existing pending/applying/committed recovery protocol applies only to Git
  mutations and must not be reused as the workflow state machine.
- The invariant that only the current session-turn lease holder may start
  provider calls, conversational tools, and turn events remains unchanged for
  conversational execution.
- Workflow Runs use separate durable run and step leases with fencing tokens.
  They do not hold or revive a conversational session-turn lease while waiting,
  running in the background, or resuming after restart.
- Every conversational event continues to belong to a session. Workflow
  Definitions and Workflow Runs receive separate stable identities. A run may
  optionally cite its originating session, schedule, or manual trigger without
  making that session its lifecycle owner.
- Ordinary conversational tool intent retains its current recovery rule: after
  restart, unfinished intent is visible and does not block later turns. The
  system does not synthesize an outcome.
- Workflow effect intent follows the stronger workflow rule: dependent nodes
  cannot advance until an unfinished effect is reconciled into an Effect Receipt
  or explicitly resolved. Outcome Unknown never retries automatically.
- Retrieved semantic memory and ordinary procedural content still cannot grant
  permissions, alter approval requirements, or authorize tools. The sole
  future-run authority path is Kernel validation of one approved typed Workflow
  Definition and its bounded Standing Authority.
- `approve_procedure` remains the generic proposal-activation concept. Workflow
  review distinguishes definition activation, schedule activation included in
  that definition, run start, per-run durable interruption, and Action Approval
  for work outside Standing Authority.
- One Workflow Approval may activate bounded Standing Authority for future runs
  because that behavior is explicit in the typed definition and enforced in
  code. It does not generalize to Skills, free-form Markdown, retrieved memory,
  or ordinary model tool calls.
- Workflow tables are not extensions of memory compiler job tables. They have
  independent schemas for definitions and approvals, runs, node attempts,
  leases, checkpoints, interruptions, Effect Intents, Effect Receipts,
  notifications, compatibility resolutions, and migration lineage.
- Durable workflow records include accepted input, normalized output, requested
  and effective authority, Model Policy identity, AI proposals, human
  corrections, provider requests and responses subject to redaction, and effect
  evidence. Credentials and raw tokens are forbidden.
- Reserve distinct terminology: semantic-operation replay rebuilds accepted
  graph state without external work; workflow resume continues from the latest
  durable boundary; production workflow time-travel replay is unsupported.
- Stage 7 continues to deliver procedural repository initialization, reviewed
  proposal and approval, canonical versioning, scoped loading, crash recovery,
  quarantine, and rollback. It also defines the reviewed storage format needed
  by presets, Workspace Revisions, and Workflow Definitions.
- Durable procedural workflow execution becomes a later stage governed by the
  separate Workflow Runtime specification. It depends on Stage 7 definitions
  but does not enlarge Stage 7's implementation outcome.
- Existing event, semantic-memory, FTS, embedding, graph-cache, egress,
  provenance, source-authority, and generic-tool containment rules remain
  unchanged except where this amendment names a precise additional scope or
  separate workflow authority.
- The active memory specification and decision record are amended directly so
  readers do not have to reconcile contradictory source documents.

## Testing Decisions

- Good tests assert allowed scope, persisted identity, fencing, and recovery
  behavior through existing memory/session interfaces and the separate Workflow
  Runtime. They do not test documentation wording through code or couple memory
  tests to workflow scheduler internals.
- Scope matrix tests cover global, project, Workspace, and session combinations,
  including every forbidden cross-scope read and write.
- Session persistence tests use real SQLite and process reopen to prove exactly
  one Context Scope and immutable Workspace or project snapshots.
- Memory operation tests prove Workspace is the default write scope in a
  Workspace session and global promotion remains explicit, source-linked, and
  compare-and-set safe.
- Procedural repository tests cover mechanical Workspace path resolution,
  canonical content hashing, required-file failures, restrictive permissions,
  symlink rejection, dirty-tree quarantine, and rollback.
- Definition-versus-run tests prove Git changes never mutate SQLite run state
  and SQLite checkpoints never mutate the reviewed Git worktree.
- Lease tests run a conversational turn and background Workflow Run concurrently
  and prove each is fenced only by its own lease type.
- Recovery tests prove unfinished conversational tool intent does not block a
  later chat turn while an unfinished workflow Effect Intent blocks dependent
  workflow nodes.
- Authority tests prove retrieved memory, Skills, and arbitrary Markdown cannot
  grant Standing Authority, while an approved typed definition can grant only
  its bounded subset of Workspace Access.
- Replay tests prove semantic graph rebuild performs no provider or capability
  calls and workflow resume begins only from an accepted checkpoint.
- Documentation validation includes link checks, glossary consistency, explicit
  supersession of contradictory decisions, and whitespace checks.
- Existing session scope, chooser race, event append, turn lease, compiler job,
  procedural recovery, and replay tests are prior art for the amended behavior.
- The full repository verification command must pass for implementation changes;
  documentation-only amendments require the repository documentation checks.

## Out of Scope

- Implementing the Plugin Manager, Workspace UI, Workflow Runtime, Connector
  Plugins, or Cairo's Kitchen procedure in the memory amendment itself.
- Replacing the semantic graph, compiler, retrieval ranking, FTS, vector, or
  cache architecture.
- Giving ordinary procedural Markdown authority over external actions.
- Changing conversational tool approval or unfinished-intent behavior globally.
- Storing workflow checkpoints, effects, or schedules in Git.
- Reusing memory compiler jobs as Workflow Runs.
- Multi-Workspace sessions, project-to-Workspace links, multiple owners, or
  role-based access.
- Production workflow time-travel replay.
- Hard erasure semantics, which remain a separate unresolved memory decision.

## Further Notes

This amendment is deliberately narrow even though the future workflow runtime
is substantial. Its purpose is to keep the active memory design internally
consistent and establish ownership: procedural Git owns reviewed definitions;
SQLite workflow state owns execution; connector systems own external effect
truth; and the Kernel owns authority and fencing.

The existing rule that memory cannot alter permissions remains correct. Standing
Authority is not "memory granting permission"; it is a separate code-enforced
authorization object created only when the owner approves a typed Workflow
Definition.

Related specifications: Plugin System Phase 1 is issue #70, Workspace scope is
issue #71, and the Workflow Runtime is issue #72.
