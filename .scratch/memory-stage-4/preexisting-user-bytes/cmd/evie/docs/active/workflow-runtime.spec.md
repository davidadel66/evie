## Problem Statement

Evie can currently interpret instructions and call tools inside a live
conversation, but it cannot own a long-running, reviewed business procedure.
A Markdown skill can tell the model how to calculate tips, yet it cannot provide
durable checkpoints, resumable human interruptions, background execution,
bounded retries, duplicate-run prevention, or proof of whether an external
write occurred before a crash.

Cairo's Kitchen needs more than an ad hoc tool sequence. The owner needs to
create and review a procedure once, grant it narrowly bounded authority for
future runs, start it from a session or schedule, and trust Evie to preserve its
state through disconnects and restarts. Probabilistic model judgment may help
with bounded interpretation, but it must not own arithmetic, authority, or
external effects.

## Solution

Build a small Go-native Workflow Runtime rather than depending on LangGraph.
Procedural Git owns reviewed, versioned Workflow Definitions expressed as a
closed declarative graph. SQLite owns Workflow Runs, node attempts, leases,
checkpoints, interruptions, Effect Intents, Effect Receipts, notifications, and
recovery state. A run has its own lifetime and Execution Composition independent
of the session that started it.

One Workflow Approval activates one exact definition and its bounded Standing
Authority for future runs. Manual starts require no second approval, and a
reviewed Workflow Schedule becomes active through that same approval. Every
external effect follows a durable intent-call-receipt-checkpoint sequence.
Unknown outcomes are reconciled rather than retried blindly.

The primary interface is one deep Workflow Runtime used to validate, simulate,
activate, start, resume, cancel, and inspect workflows. Internally it may use
focused capability and model adapters, but callers and acceptance tests do not
coordinate nodes, leases, receipts, or recovery themselves.

## User Stories

1. As Evie's owner, I want to describe a repeatable procedure during a session, so that Evie can turn observed work into a reusable proposal.
2. As Evie's owner, I want a proposed procedure to remain inactive until I review it, so that conversation text cannot create background authority.
3. As Evie's owner, I want each Workflow Definition version stored in readable reviewed files, so that I can inspect logic, prompts, resources, and limits before activation.
4. As Evie's owner, I want a human-readable explanation beside the machine-readable graph, so that review does not require reading orchestration syntax alone.
5. As Evie's owner, I want AI prompts stored separately from graph structure, so that prompt changes are visible and versioned.
6. As Evie's owner, I want workflow definitions restricted to a closed schema, so that reviewed YAML cannot execute hidden shell, Python, JavaScript, or Go code.
7. As Evie's owner, I want validation to reject unknown fields and node kinds, so that misspellings cannot silently change behavior.
8. As Evie's owner, I want review to show the graph and requested authority clearly, so that I understand both what will happen and what it may access.
9. As Evie's owner, I want a dry run with external effects suppressed, so that I can examine expected inputs, outputs, branches, resources, and proposed mutations.
10. As Evie's owner, I want revisions compared with executable and authority diffs, so that I can focus on meaningful behavior changes.
11. As Evie's owner, I want one Workflow Approval to authorize normal future runs within explicit bounds, so that I do not reapprove the same reviewed procedure every day.
12. As Evie's owner, I want approval limited to exact accounts, resources, operations, schedules, recipients, and amounts, so that a workflow never receives broad connector access.
13. As Evie's owner, I want arbitrary instructions and skills unable to grant Standing Authority, so that only a typed reviewed Workflow Definition can do so.
14. As Evie's owner, I want changed logic, prompts, capabilities, resources, schedules, or authority to require a new approval, so that material behavior never changes silently.
15. As Evie's owner, I want documentation-only edits not to invalidate approval, so that correcting explanatory prose does not stop working automation.
16. As Evie's owner, I want approving a manual workflow not to start a run immediately, so that activation and execution remain distinct decisions.
17. As Evie's owner, I want an approved schedule activated by the same review that shows it, so that I do not complete duplicate approval flows.
18. As Evie's owner, I want to start an approved workflow from a chat session, so that Evie can perform it as part of our conversation.
19. As Evie's owner, I want a run to continue after I close or disconnect the session, so that durable work is not tied to a browser or terminal connection.
20. As Evie's owner, I want background and foreground runs to use the same engine, so that safety and recovery do not depend on presentation mode.
21. As Evie's owner, I want to attach a later session to an existing run, so that I can inspect progress or answer an interruption after the original chat is gone.
22. As Evie's owner, I want every run to pin its Workflow Definition, Standing Authority, Workspace Revision, and Execution Composition, so that its meaning remains reproducible.
23. As Evie's owner, I want a run to resolve only capabilities declared by its definition, so that it does not inherit unrelated tools from the starting session's Agent Preset.
24. As Evie's owner, I want missing or incompatible providers to block a run visibly, so that Evie never substitutes behavior silently.
25. As Evie's owner, I want compatible provider substitution audited, so that continued execution names the implementation that actually ran.
26. As Evie's owner, I want each logical execution to have a deterministic Run Key, so that manual and scheduled starts cannot perform the same daily work twice.
27. As Evie's owner, I want an explicit recorded override to rerun a completed key, so that exceptional repetition is possible but never accidental.
28. As Evie's owner, I want deterministic calculations performed by code, so that arithmetic and business limits do not depend on model output.
29. As Evie's owner, I want capability calls represented as explicit nodes, so that reads and effects have known inputs, outputs, and contracts.
30. As Evie's owner, I want conditional branches represented explicitly, so that alternative paths are reviewable and testable.
31. As Evie's owner, I want durable human interruptions, so that a run can wait safely across restart for missing information or exceptional judgment.
32. As Evie's owner, I want AI Nodes limited to typed proposals, so that models can help interpret data without directly changing an external system.
33. As Evie's owner, I want AI output deterministically validated, so that malformed, out-of-range, or unidentified data cannot advance.
34. As Evie's owner, I want AI Nodes to receive only their reviewed prompt and declared inputs, so that ambient chat history or unrelated memory cannot alter a background run.
35. As Evie's owner, I want every AI Node to use a named versioned Model Policy, so that the exact provider, model, parameters, and output schema are pinned.
36. As Evie's owner, I want a Model Policy change to create a new reviewed definition, so that model substitution is not an invisible behavioral change.
37. As Evie's owner, I want accepted workflow input persisted before execution, so that restart never reconstructs intent from an incomplete conversation.
38. As Evie's owner, I want each node attempt recorded before and after work, so that I can see what ran, failed, retried, or paused.
39. As Evie's owner, I want state checkpointed before the next node starts, so that a crash resumes from a durable boundary.
40. As Evie's owner, I want external Effect Intent recorded before a request is sent, so that Evie can distinguish not-started work from uncertain work.
41. As Evie's owner, I want provider responses and Effect Receipts recorded before advancing, so that completed effects survive restart without repetition.
42. As Evie's owner, I want an intent without a receipt marked Outcome Unknown, so that an interrupted network call is never treated as a normal failure.
43. As Evie's owner, I want unknown effects reconciled through provider idempotency or lookup, so that Evie determines what happened before deciding what to do next.
44. As Evie's owner, I want unresolved unknown effects brought to me, so that Evie never guesses whether money or data was sent.
45. As Evie's owner, I want retry rules appropriate to each node, so that pure calculations may retry while external effects require proven idempotency.
46. As Evie's owner, I want Outcome Unknown excluded from automatic retry, so that a timeout cannot duplicate a consequential action.
47. As Evie's owner, I want cancelling a run to prevent future nodes, so that I can stop remaining work safely.
48. As Evie's owner, I want cancellation to preserve completed effects and reconcile in-flight work, so that history remains truthful.
49. As Evie's owner, I want revoking Standing Authority to block new runs and pause active ones before the next effect, so that revocation takes immediate effect.
50. As Evie's owner, I want plugin or connection recovery to resume only safe runs automatically, so that unresolved effects and human questions remain paused.
51. As Evie's owner, I want different runs to calculate concurrently when safe, so that one slow procedure does not block all work.
52. As Evie's owner, I want writes to the same external resource serialized, so that different Run Keys cannot corrupt a shared spreadsheet or record.
53. As Evie's owner, I want human corrections appended instead of overwriting proposals, so that the audit trail retains what Evie suggested and what I changed.
54. As Evie's owner, I want corrected values revalidated before execution, so that a human edit cannot bypass formulas, identities, limits, or authority accidentally.
55. As Evie's owner, I want schedules interpreted in an explicit timezone, so that host settings and daylight-saving changes do not shift business behavior silently.
56. As Evie's owner, I want each schedule to declare a Catch-up Policy, so that downtime recovery is predictable and bounded.
57. As Evie's owner, I want Cairo's daily workflow able to create one run for each missing business date, so that offline time does not lose a day's work.
58. As Evie's owner, I want runs for different business dates allowed to progress independently, so that one exception does not necessarily block later calculations.
59. As Evie's owner, I want a Durable Notification when a background run completes, fails, or needs attention, so that I do not have to keep a session open.
60. As Evie's owner, I want external notification channels to mirror rather than own notifications, so that delivery failure does not erase the durable event.
61. As Evie's owner, I want sensitive notification details omitted by default, so that an email or text does not leak business data.
62. As Evie's owner, I want complete local audit history for runs, so that accepted inputs, model proposals, corrections, requests, responses, and receipts can be examined.
63. As Evie's owner, I want credentials and raw tokens excluded from run history, so that durability does not create a secret archive.
64. As Evie's owner, I want Workspace retention, export, and redaction rules applied to run records, so that business data has an explicit lifecycle.
65. As Evie's owner, I want incompatible unfinished runs paused after an upgrade, so that new code cannot silently reinterpret old state.
66. As Evie's owner, I want migration to create a linked Successor Run, so that the predecessor's definition, state, and audit history remain truthful.
67. As Evie's owner, I want validated state and Effect Receipts carried into a Successor Run, so that migration does not repeat completed actions.
68. As a maintainer, I want one Workflow Runtime interface, so that callers do not orchestrate node scheduling, persistence, fencing, retries, or reconciliation.
69. As a maintainer, I want real SQLite used in durability tests, so that crash recovery and transaction ordering are verified rather than mocked.
70. As a maintainer, I want fake capability and model providers behind focused interfaces, so that all external outcomes can be tested deterministically.

## Implementation Decisions

- Evie implements a Go-native Workflow Runtime and does not depend on
  LangGraph. LangGraph is a semantic reference for graph execution,
  checkpointing, and interrupts, not a production dependency.
- Procedural Git is authoritative for approved Workflow Definition content and
  version history. SQLite is authoritative for Workflow Runs, node attempts,
  leases, checkpoints, interruptions, effects, notifications, and audit state.
  External providers remain authoritative for external effects and are
  reconciled into Effect Receipts.
- The primary seam is one Workflow Runtime interface with operations to validate
  and simulate a proposal, activate an approved definition, start a run, resume
  available work, submit interruption input, cancel a run, inspect state, and
  reconcile an uncertain effect. Callers never drive individual nodes.
- The Workflow Runtime owns a durable run scheduler, fenced run or step leases,
  bounded worker concurrency, recovery scans, and state transitions independent
  of conversational turn leases.
- A Workflow Run has a stable ID, Workspace ID, Run Key, pinned definition and
  authority versions, Execution Composition, lifecycle state, input and output,
  timestamps, optional originating session or schedule, and optional predecessor
  or successor link.
- Workflow Run and node state use explicit non-overlapping states. At minimum
  the runtime distinguishes queued, running, waiting for a human, waiting for a
  dependency, needs reconciliation, cancelling, cancelled, failed, and
  succeeded; terminal history is never rewritten.
- A definition consists of a strict `workflow.yaml`, a human-readable
  `README.md`, and separate Markdown prompt files for AI Nodes. Validation
  rejects unknown fields, unsupported schema versions, missing references,
  unreachable nodes, unbounded or unsafe cycles, and missing limits.
- Activation canonicalizes all behavior-affecting definition content and hashes
  it. Documentation-only content is excluded from the executable hash while
  still remaining reviewable in Git.
- V1 graph execution supports five node kinds: deterministic calculation,
  capability call, AI judgment, conditional branch, and durable interruption.
  Control flow is sequential or conditional with bounded node retries.
- Definitions cannot embed arbitrary Go, Python, JavaScript, shell, templates
  that generate executable code, or dynamically generated graph structure.
- Workflow-to-workflow invocation, parallel branches, arbitrary graph loops,
  dynamic graph mutation, and production time-travel replay are deferred.
  Shared deterministic capabilities provide initial reuse.
- Workflow validation produces a human graph, required Capability Contracts,
  expected inputs and outputs, resource list, Run Key rule, Resource Conflict
  Keys, Retry Policies, Model Policies, requested Standing Authority, schedule,
  and retention classification.
- Effect-suppressed simulation runs deterministic and AI proposal nodes with
  controlled fixtures while replacing external mutations with previews. A dry
  run never becomes an Effect Receipt and never grants authority.
- Workflow Approval accepts one exact executable definition hash and its exact
  requested Standing Authority. It activates the definition for future runs.
  Arbitrary procedural text, a Skill, Workspace Access, an Agent Preset, and an
  Account Connection cannot grant Standing Authority.
- Standing Authority lists exact operations, Connection IDs, external accounts,
  resources, schedules, recipients, and limits. It must be a subset of current
  Workspace Access, and the Kernel enforces both on every effect.
- Changes to logic, prompts, Capability Contracts, Model Policies, resource
  selectors, Run Key rules, schedules, recipients, limits, or Standing
  Authority create a new definition version requiring approval. Explanatory
  README-only edits do not.
- Approval activates a manual definition without starting a run. If the
  reviewed definition contains a Workflow Schedule, that same approval
  activates the highlighted schedule.
- Manual/session start is the first delivery slice. Scheduled start and bounded
  Catch-up Policy follow as a separate slice using the same run-start interface.
  External event triggers remain a later extension.
- Foreground and Background Runs share one runtime and persistent state.
  Session disconnect does not cancel a run, and a later session may attach to
  inspect it or answer an interruption.
- Each run derives a deterministic workflow-specific Run Key from accepted
  logical input. The database prevents a second active or completed run for the
  same key. An explicit audited override is required to rerun a completed key.
- Each run resolves its own Execution Composition from definition requirements.
  It does not inherit the initiating session's Agent Preset, although that
  preset must expose the capability used to start or inspect workflows.
- The Execution Composition pins Capability IDs, contract and schema hashes,
  provider versions, Model Policies, Connection IDs, and non-secret
  configuration. Compatibility Resolution follows the plugin specification;
  changed contracts require an approved new definition or Successor Run.
- A deterministic node receives typed declared state and returns a typed state
  update. It owns calculations, formula enforcement, identity checks, limits,
  normalization, and validation.
- A capability node invokes one canonical Capability ID through a focused
  runtime-owned interface. The provider declares whether the operation is a
  read or effect, its idempotency support, reconciliation method, and Resource
  Conflict Keys.
- An AI Node receives only its reviewed prompt, declared state fields,
  explicitly requested scoped memory, and declared capability results. It has
  no ambient transcript, general memory, tool loop, or authority to perform an
  effect.
- An AI Node references one named, versioned Model Policy resolving the exact
  provider, model, parameters, prompt identity, and output schema. Its output is
  typed proposed data and must pass deterministic validation before use.
- A durable interruption records a versioned, serializable request and waits
  without holding a conversational lease. Human responses and corrections are
  append-only inputs and are revalidated before the graph advances.
- Before any node attempt, the runtime durably records its identity, input
  checkpoint, attempt number, and lease fence. It records normalized output and
  the next checkpoint before another node starts.
- External effects use this ordering: accept and checkpoint input; record the
  pinned definition, authority, and composition; record the node attempt;
  record an Effect Intent with stable idempotency identity; invoke the provider;
  persist the provider response and Effect Receipt; persist normalized output;
  then advance the checkpoint.
- SQLite and an external provider cannot commit atomically. No Effect Intent
  means the effect did not start. A durable Effect Receipt permits recovery. An
  intent without a receipt becomes Outcome Unknown and blocks dependent nodes.
- Reconciliation first uses provider-supported idempotency or lookup. If the
  provider cannot prove the outcome, a durable human interruption presents the
  evidence and requires explicit resolution. Outcome Unknown never retries by
  policy.
- Retry Policy is declared per node and bounded. Pure deterministic work may
  retry; reads use limited backoff; AI Nodes may retry malformed typed output;
  effects retry only when the connector proves idempotency for the same Effect
  Intent.
- Cancellation stops future nodes. An in-flight effect completes or enters
  reconciliation; completed effects remain recorded and are not undone.
  Standing Authority or Workspace Access revocation blocks new runs and pauses
  active runs before their next covered effect.
- A restored plugin or Connection may resume a run automatically only when no
  effect is Outcome Unknown and no human interruption is outstanding.
- Capability providers declare Resource Conflict Keys. The runtime permits
  independent computation and reads but serializes effect nodes sharing a key.
  Providers also use conditional or idempotent writes when supported.
- Schedules declare an IANA timezone and bounded Catch-up Policy. The Cairo tip
  workflow later uses business date in its Run Key and creates one recovery run
  for each missing business date. Different keys may progress independently.
- Every Background Run terminal or needs-attention transition creates a
  deduplicated Durable Notification. Future notification plugins may mirror it,
  but SQLite remains authoritative and outbound content omits sensitive data by
  default.
- Audit state retains accepted input, proposals, corrections, node attempts,
  Effect Intents, provider responses, Effect Receipts, checkpoints, approvals,
  authority evaluations, and outputs. Credentials and raw tokens are forbidden.
  Workflow field classifications and Workspace policy govern redaction,
  retention, and export.
- An incompatible unfinished run pauses. Explicit migration creates a linked
  Successor Run under an approved new definition, carrying only validated state
  and durable effect evidence. The predecessor remains immutable.
- Generic CLI and web surfaces list definitions and runs, show validation and
  simulation, present approval diffs, start and cancel runs, display progress,
  answer interruptions, reconcile unknown effects, and inspect notifications.
- Implementation is delivered in independently reviewable stages: definition
  validation and simulation; activation and authority; manual deterministic
  runs; durable restart and leases; capability reads; effect protocol and
  reconciliation; interruptions and AI Nodes; cancellation and recovery;
  schedules and notifications; migration and retention.

## Testing Decisions

- Good tests exercise complete workflow behavior through the Workflow Runtime
  interface. They assert durable states, provider calls, receipts, outputs, and
  errors rather than private scheduler methods or SQL text.
- Acceptance tests use a real temporary SQLite database and reopen it between
  steps to prove persistence and restart behavior. In-memory persistence is not
  acceptable for crash, lease, ordering, or reconciliation tests.
- Focused fake Capability Providers model successful reads and effects,
  deterministic failures, timeouts before and after provider acceptance,
  idempotency replay, provider lookup, unavailable dependencies, conflicting
  resources, and malformed responses.
- Focused fake model providers capture the exact AI Node request and return
  valid, invalid, out-of-range, and nondeterministic proposals. Tests prove the
  request excludes ambient transcript, undeclared memory, credentials, and
  unrelated capability results.
- Definition tests cover canonical hashing, unknown fields, unsupported schema
  versions, missing prompts, unreachable nodes, unsafe cycles, missing limits,
  invalid branches, invalid Run Key rules, unauthorized resources, and
  documentation-only changes.
- Review tests assert the human graph, executable diff, authority diff,
  Capability Contracts, schedules, resources, expected input/output, and
  effect-suppressed preview.
- Approval tests prove one approval activates the exact definition and Standing
  Authority, manual approval starts no run, a reviewed schedule becomes active,
  ordinary future runs need no Action Approval, and every out-of-bounds effect
  stops before provider invocation.
- Run tests cover manual and scheduled starts, foreground detachment, background
  continuation, later-session attachment, deterministic Run Key collisions,
  completed-key override, and different-key concurrency.
- Node tests cover all five supported node kinds, typed state transitions,
  deterministic validation, branch selection, bounded retries, and durable
  interruption response.
- Crash-matrix tests stop after every durable boundary around an external effect:
  before intent, after intent, after provider acceptance, after response, after
  receipt, after normalized output, and after checkpoint. Each restart must
  produce the defined recovery state without duplicating an effect.
- Outcome Unknown tests prove no automatic retry, successful reconciliation by
  idempotency and lookup, unresolved human escalation, and blocked dependent
  nodes.
- Lease and concurrency tests use two runtime instances over one database to
  prove fencing, expired-owner takeover, late-write rejection, run-level
  independence, and Resource Conflict Key serialization.
- Cancellation tests cover every node category and both sides of an effect
  boundary. Revocation tests prove an existing pin cannot start a newly
  forbidden operation.
- Recovery tests distinguish dependency restoration, human interruption, and
  Outcome Unknown so only safe runs resume automatically.
- Schedule tests use an injected clock and timezone database to cover daylight
  saving transitions, offline gaps, bounded catch-up, duplicate manual and
  scheduled starts, and independent business dates.
- Migration tests prove the predecessor remains unchanged, invalid state is not
  carried forward, Effect Receipts retain identity, and a Successor Run cannot
  repeat completed effects.
- Retention and notification tests prove field redaction, credential exclusion,
  deduplicated notices, external mirror failure tolerance, explicit deletion
  audit, and local authoritative history.
- Existing session lease, append-only event, cancellation, tool approval, cron
  recovery, and SQLite reopen tests are prior art for fencing, terminal causes,
  external uncertainty, and deterministic persistence.
- The full repository verification command must pass after every implementation
  stage.

## Out of Scope

- LangGraph or another workflow-engine runtime dependency.
- Arbitrary embedded code, shell nodes, user-provided executables, or dynamic
  graph generation.
- Parallel graph branches, arbitrary graph cycles, time-travel execution, or
  mutation of a live graph.
- Workflow-to-workflow calls; later pinned subworkflow nodes remain a named
  extension.
- Automatic external event triggers in the first runtime.
- Distributed execution across multiple machines.
- Compensating transactions or pretending completed external effects can always
  be undone.
- Letting AI Nodes call tools, authorize effects, perform financial arithmetic,
  or receive ambient session context.
- Treating ordinary conversational tool intent as Workflow Run state.
- Cairo-specific calculations, employee mapping, Square queries, Google Sheets
  layout, or Venmo execution.

## Further Notes

Markdown remains sufficient for human-readable instructions, prompts, review,
and Git versioning. It is not sufficient for durable execution. The separation
between Workflow Definition and Workflow Run is the core design: Git answers
"what was approved?" while SQLite answers "what has happened?"

The Cairo's Kitchen vertical slice should follow this runtime and the required
Connector Plugins. It will provide the first real proof of deterministic tip
calculation, Square reads, Google Sheets writes, human exception handling, and
eventual payment behavior without embedding restaurant-specific rules into the
generic engine.

Dependencies and related specifications: Plugin System Phase 1 is issue #70,
Workspace scope is issue #71, and the memory-model amendment is issue #73.
