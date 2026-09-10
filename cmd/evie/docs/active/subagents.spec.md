## Problem Statement

Evie cannot delegate a bounded assignment to another agent through its normal
conversation flow. Research and other supporting work consume the primary
session's attention and context, even when a separate agent could investigate
and return a concise finding. Existing delegated sessions and Task Access
Grants provide useful persistence and authorization foundations, but there is
no production delegation capability or supervisor that owns child execution.

The owner wants subagents configured through Plugins, Capabilities, and Agent
Presets. A child should receive an explicit composition and access scope, with
Memory capabilities available only when deliberately selected. Copying the
primary agent's composition or hiding mutation schemas is insufficient: current
composition includes built-in execution abilities, and inherited Context Scope
does not by itself restrict memory access or distinguish delegated instructions
from owner assertions.

## Solution

Add a compiled First-party Subagents Plugin that exposes foreground delegation.
A non-removable Kernel supervisor admits and runs one bounded child assignment,
using the existing conversation runtime with a separate durable session,
Composition Receipt, history, and turn ownership. The child returns findings
and supporting evidence; the primary agent remains responsible for the answer
to the owner.

The first child Agent Preset is an immutable research preset composed from the
Web Plugin's search and fetch capabilities. It receives the assignment and
explicitly selected supporting context. It has no Memory, Todo mutation,
scheduling, arbitrary shell, filesystem, finance, or delegation capabilities,
and no automatic retrieval of memory. This establishes explicit composition
without implementing every possible worker preset. Later presets may select
additional plugin capabilities under the same Kernel authority rules.

Foreground delegation waits for a bounded result. Parallel foreground workers,
background continuation, and worker management panels are separate outcomes.
The first execution path is eligible Global/project sessions. Workspace
execution additionally depends on reviewed Workspace preset allowances; missing
allowances produce an explicit refusal rather than a change of scope.

## User Stories

1. As Evie's owner, I want Evie to delegate a bounded research assignment, so that supporting investigation can happen in a separate context.
2. As Evie's owner, I want the primary agent to remain responsible for the answer, so that I have one accountable conversational partner.
3. As Evie's owner, I want subagents configured through Plugins, Capabilities, and Agent Presets, so that the feature follows Evie's domain model.
4. As Evie's owner, I want each child to receive an explicit Agent Preset, so that its available abilities are predictable.
5. As Evie's owner, I want capabilities selected individually from installed Plugins, so that including one ability does not grant every ability its Plugin provides.
6. As Evie's owner, I want a read-only research preset first, so that delegation is useful with a small and verifiable authority surface.
7. As Evie's owner, I want the research child to search and fetch web sources, so that it can return evidence for its findings.
8. As Evie's owner, I want excluded capabilities rejected during execution, so that a model cannot regain them by inventing a call.
9. As Evie's owner, I want built-in execution abilities subject to the child's composition, so that a restricted preset does not inherit hidden powers.
10. As Evie's owner, I want missing required research capabilities to reject delegation visibly, so that a worker never falls back to a broader preset.
11. As Evie's owner, I want each child to pin its exact composition and instructions, so that later configuration changes do not alter what an existing assignment means.
12. As Evie's owner, I want existing primary sessions to retain their pinned capabilities, so that enabling Subagents does not silently change a live conversation.
13. As Evie's owner, I want new eligible primary sessions to expose delegation when the Subagents Plugin is enabled, so that I can use the feature through ordinary conversation.
14. As Evie's owner, I want Plugins to remain replaceable while supervision stays in the Kernel, so that extensions cannot replace their own authority or recovery rules.
15. As Evie's owner, I want child capability exposure bounded by the parent's composition, so that delegating cannot widen available powers.
16. As Evie's owner, I want Workspace preset restrictions enforced for children, so that an inherited Workspace cannot be used to introduce a disallowed preset.
17. As Evie's owner, I want current access revocations applied to running assignments, so that a pinned revision cannot restore access I removed.
18. As Evie's owner, I want a child to receive only the assignment and selected supporting context, so that the entire parent conversation is not copied automatically.
19. As Evie's owner, I want separate child history, so that research transcripts do not consume the primary conversation's context budget.
20. As Evie's owner, I want sibling and unrelated session data excluded, so that delegation does not become a cross-session disclosure mechanism.
21. As Evie's owner, I want Memory capability access and automatically supplied memory controlled separately, so that excluding a Memory Plugin capability actually has the intended effect.
22. As Evie's owner, I want the initial research child to receive no automatically retrieved memory, so that its context is determined by the explicit assignment.
23. As Evie's owner, I want explicitly supplied supporting facts to remain attributed as assignment data, so that sharing a fact does not grant access to its underlying memory store.
24. As Evie's owner, I want delegated instructions distinguished from my own statements, so that memory processing cannot treat agent-generated assignments as owner assertions.
25. As Evie's owner, I want child findings treated as evidence-bearing data, so that they cannot redefine the primary agent's instructions or grant new authority.
26. As Evie's owner, I want a short delegation to work without a durable Task, so that incidental research does not clutter my Task Trees.
27. As Evie's owner, I want an assignment optionally associated with an existing accessible Task, so that tracked work can retain its execution lineage.
28. As Evie's owner, I want a Task association kept distinct from a Task Access Grant or Task Claim, so that tracking does not grant authority or imply completion.
29. As Evie's owner, I want each execution attempt to have its own durable record, so that session state and Task status are not overloaded to describe agent execution.
30. As Evie's owner, I want repeated identical delegation requests to resolve to the same attempt, so that retries do not duplicate work or model usage.
31. As Evie's owner, I want reuse of an idempotency key with a different assignment rejected, so that the system cannot confuse two different requests.
32. As Evie's owner, I want limits enforced on execution time, model calls, context, and returned findings, so that a bounded assignment cannot run indefinitely.
33. As Evie's owner, I want child model configuration derived from the parent's resolved configuration initially, so that delegation does not introduce unexpected provider or model selection.
34. As Evie's owner, I want nested delegation unavailable and rejected, so that one assignment cannot expand into an uncontrolled execution tree.
35. As Evie's owner, I want cancelling the parent turn to stop its child, so that cancellation reaches all work started for that turn.
36. As Evie's owner, I want loss of the parent's execution ownership to end child authority, so that a separately heartbeating child cannot outlive the execution that authorized it.
37. As Evie's owner, I want failure and cancellation outcomes retained, so that Evie can explain why an assignment did not complete.
38. As Evie's owner, I want completed findings retained if delivery to the parent fails, so that useful work is not lost or rerun unnecessarily.
39. As Evie's owner, I want interrupted executions reconciled after restart, so that the system does not leave them permanently marked as running.
40. As Evie's owner, I want recovery to preserve uncertainty, so that Evie does not invent missing conversational outcomes or claim that interrupted work succeeded.
41. As Evie's owner, I want unfinished research left interrupted after restart, so that recovery does not silently initiate additional model calls.
42. As Evie's owner, I want disabling Subagents to block new assignments from already-created sessions, so that disabling the Plugin takes effect at admission.
43. As Evie's owner, I want shutdown to cancel and clean up admitted foreground assignments, so that workers do not continue unnoticed after their supervisor stops.
44. As Evie's owner, I want bounded findings with evidence, status, execution identity, and usage, so that the primary agent can assess and accurately report the result.
45. As Evie's owner, I want unavailable usage distinguished from measured zero usage, so that diagnostics do not understate what is known.
46. As Evie's owner, I want child activity kept distinct from the primary response, so that worker messages do not appear to be the primary agent speaking to me.
47. As a maintainer, I want the existing agent loop reused for children, so that provider transport, context accounting, persistence, and cancellation fixes apply consistently.
48. As a maintainer, I want one high-level acceptance seam for delegation, so that tests demonstrate the owner's observable result across the real composed runtime.
49. As a maintainer, I want deterministic admission and recovery tests with real SQLite, so that duplicate, interrupted, and stale-owner behavior is verified without live models.
50. As a maintainer, I want restricted composition and foreground execution delivered as focused changes, so that authorization and persistence decisions remain inspectable.

## Implementation Decisions

- **Domain language and ownership.** Subagents is a First-party Plugin. Its
  delegation Capability is selected by a parent Agent Preset. The Kernel
  supervisor owns admission, authority, execution lifecycle, cancellation,
  persistence, and recovery. Model-callable schemas are an implementation of
  capabilities, not the product's configuration vocabulary.
- **Existing provider role.** Implement the Plugin through the existing focused
  capability-provider role for model-callable execution. Do not introduce a
  generic Subagent Provider family for a single native implementation. The
  Plugin receives a narrow Kernel execution interface; the composition root
  supplies resolved worker composition and runtime dependencies without a
  circular dependency between plugin management and child execution.
- **First child composition.** Add one immutable built-in research Agent Preset
  selecting only the existing Web search and fetch Capability Contracts. The
  full executable capability set must match its Composition Receipt. Do not
  append the primary session's built-in capability set or remove capabilities
  after generating a broader receipt. Preserve exact reconstruction and
  compatibility checks for both new and existing compositions.
- **Parent rollout.** Include delegation in the new version of the standard
  parent preset as an optional capability supplied by an enabled Subagents
  Plugin. A disabled or unavailable optional Plugin produces the normal
  composition diagnostic and omission for new sessions. Old receipts remain
  unchanged; accessing the new capability requires a newly composed session.
  Registration is compiled into Evie and follows existing enablement,
  dependency, version, startup, and shutdown conventions.
- **Capability and scope ceilings.** Admission verifies that the child
  composition is permitted by the parent's pinned capabilities, inherited
  Context Scope, and current applicable access restrictions. Required research
  capabilities missing from that intersection cause rejection. The child keeps
  the parent's Workspace and revision or project identity/root; it cannot
  select a different scope, connection, or resource ceiling through model
  arguments. A Workspace's allowed Agent Presets must explicitly permit the
  research preset. Older revisions that do not permit it reject delegation
  with an actionable explanation; this feature does not silently rewrite their
  allowlists or add an exemption for children.
- **Assignment interface.** Foreground delegation accepts a bounded objective,
  optional explicitly selected context, an optional existing Task identity, and
  an idempotency key. One fixed child preset is supported, so no model-selected
  preset, provider, credentials, parent identity, or authority fields are
  accepted. The current parent invocation supplies the durable source intent,
  session, Context Scope, and live execution ownership. Validate the request
  before starting a child or contacting a model provider.
- **Selected context and instructions.** The child starts with the trusted
  foundational safety instructions, a pinned worker role, and the explicit
  assignment. The worker role makes the parent responsible for the owner's
  final answer. The parent history is not forked. Required trusted scoped
  instructions remain applicable under their existing contracts; any loading
  failure follows those contracts rather than silently omitting governing
  instructions. Supporting context is labeled as data and does not become a
  new instruction or authority source. The child's own recorded history may be
  used for continuation within its one foreground turn.
- **Memory policy.** The initial preset exposes no Memory capabilities and
  permits no Automatic Recall or retrieval of prior conversations, Global
  memory, Workspace memory, or project memory. Inherited scope metadata does
  not enable these paths. Explicit assignment context may contain facts the
  authorized parent selected; that is a bounded transfer of data, not access
  to the source memory. Durable child history remains ordinary Kernel-owned
  Episodic Memory. No new semantic-memory retrieval or write feature is added.
- **Source provenance.** A parent-authored assignment is never owner testimony,
  even if the reused conversation transport represents it in a user-role
  message. Initial delegated sessions are ineligible for live and explicit
  historical memory compilation, enforced before source extraction and any
  candidate publication. Their findings and delegation results must not become
  owner-assertion support through another source-selection path. Parent-side
  results retain their delegated origin and existing restrictions on evidence
  acceptance. This is an eligibility and attribution rule, not a new compiler
  implementation or a rewrite of accepted memories.
- **Optional Task association.** A supplied Task identity must be accessible to
  the parent under current Task authority and scope rules. Record it as an
  association on the execution attempt. The first web-only child has no Todo
  capabilities, Task Access Grant, Task Focus, or Task Claim. It receives no
  automatic Task projection; the parent can include relevant authorized facts
  in the selected assignment context. Delegation neither creates a Task nor
  changes the linked Task's status. Later presets with Task capabilities must
  use existing grant, focus, claim, and cleanup rules explicitly.
- **Durable execution attempt.** Add a Kernel-owned execution record separate
  from both Session and Task status. It retains the parent session and source
  invocation, original parent ownership identity, idempotency key and canonical
  request digest, child session, pinned composition and execution-policy
  identity, optional Task association, lifecycle timestamps, terminal reason,
  result reference, and available usage. Request data stays in its authorized
  durable scope; receipts and diagnostics contain no credentials.
- **Atomic admission and idempotency.** Parent-session identity and idempotency
  key identify one logical attempt. In one atomic admission, validate the live
  parent fence and committed invocation, reserve the execution identity, and
  create its child session and receipt. Failure leaves no runnable orphan.
  Identical retries locate the original attempt, including after SQLite
  reopen; changed canonical arguments conflict. Concurrent duplicate requests
  do not start another child. An admitted active request may be joined within
  the caller's bounds; completed requests return their retained outcome after
  current access checks. Retrying an interrupted or failed attempt returns its
  outcome and requires a new key to authorize a fresh attempt. Unrelated
  parents cannot inspect or attach to another parent's attempt.
- **Lifecycle and ownership.** Execution states distinguish admitted, running,
  succeeded, failed, cancelled, and interrupted attempts. The child uses its
  own normal history binding and fenced turn lease. Authority to start further
  child activity and accept its result also depends on the originating parent
  invocation remaining authorized. An active child lease alone is insufficient.
  Child execution does not acquire the parent's session lock again or write
  directly into the parent's history.
- **Execution limits.** The first release admits at most one active child per
  parent turn and permits depth one. The child has no delegation capability,
  and Kernel admission also rejects nested requests. Enforce finite configured
  runtime-wide capacity, wall-clock deadline, total model-call allowance, input
  and output context limits, and returned-result size. Count conversational
  and compaction model calls against the same child allowance. Invalid or
  unbounded policy configuration rejects admission. Pin the effective policy
  for diagnostics; the model cannot raise these limits. Reuse the parent's
  resolved model/provider configuration while respecting the child's narrower
  execution policy. No exact monetary-budget guarantee is introduced.
- **Result contract.** Successful delegation returns the attempt and child
  session identities, terminal status, concise findings, supporting source
  references, limitations or blockers, and available usage. Research success
  means the bounded assignment finished, not that every claim is established.
  Failures distinguish provider, policy-limit, cancellation, interruption, and
  infrastructure causes without exposing secrets. Persist the bounded result
  and terminal state consistently before returning them for parent delivery.
  Preserve absent usage as unknown. Return findings and evidence rather than
  reasoning or a full child transcript.
- **Cancellation and terminal races.** Propagate parent cancellation, parent
  ownership loss, shutdown, and relevant revocation to the child. Prevent new
  activity after authority ends, join admitted execution, and release child
  ownership using bounded cleanup. Arbitrate completion and cancellation using
  durable accepted state: an authorized final child assistant event is the
  authoritative completion, even if the execution-record update or parent
  delivery has not happened yet. Preserve that accepted success across racing
  cancellation. If cancellation wins before final acceptance, a stale worker
  cannot later commit success. The
  primary session persists delivery through its own normal fence. Failure or
  cancellation of that delivery does not erase a previously committed child
  result or trigger another model execution.
- **Recovery.** Reconcile abandoned attempts from durable records and committed
  child evidence only after proving the original execution ownership is no
  longer active. An accepted final child answer can reconstruct a missing
  completed execution projection and bounded result; preserve completed
  findings that were not delivered. Mark
  unfinished work interrupted and make it inspectable without automatically
  restarting it. Recovery records execution-level facts; it does not synthesize
  missing child or parent conversational outcomes, append through stale turn
  fences, or adopt another process's still-live child.
- **Plugin lifecycle.** Check live Subagents availability when admitting new
  work even if a parent still holds a pinned delegation capability. Disabling
  or stopping the Plugin closes admission and cancels its active foreground
  assignments through the Kernel supervisor. Required capability failure must
  become an explicit failed/interrupted outcome rather than silent fallback.
  The Kernel retains outcome inspection and recovery responsibilities after
  the Plugin stops.
- **Conversation presentation.** Use the existing foreground capability
  invocation/result presentation in the CLI and web conversation. Child
  streaming and reasoning never enter the parent's response stream as if the
  parent produced them. The parent receives the bounded result, continues its
  normal turn, and reports the outcome. No new worker panel or notification
  mechanism is required for this release.
- **Delivery sequence.** Implement restricted preset composition and its
  faithful reconstruction first. Workspace execution additionally requires a
  separate prerequisite outcome that resolves reviewed Workspace Revision
  content and its allowed Agent Presets; an opaque revision identity alone
  cannot prove permission. Complete the independently reviewable foreground
  execution outcome on the existing Global/project session path, and validate
  its Workspace success path only when that prerequisite is available. Until
  then, Workspace admission fails visibly and does not substitute Global scope
  or assume that the research preset is allowed. Memory eligibility,
  parent-authority enforcement,
  idempotency, and recovery are required parts of safe foreground execution.
  Do not use this specification to convert unrelated built-ins to Plugins or
  expand into general Workspace/preset authoring.

## Testing Decisions

- **Primary acceptance seam:** use the existing composed conversation
  interface. A deterministic parent model requests the selected delegation
  Capability through the normal conversation entry point; the real Plugin
  adapter and Kernel supervisor run a real child session against temporary
  SQLite; deterministic child research returns through the parent conversation
  and produces an owner-visible final answer. Replace only external model and
  Web execution at existing dependency seams. Do not require live network
  access, API credentials, browser automation, or assertions about goroutine
  arrangement and private helper calls.
- Extend the prior pattern of standard-preset acceptance tests that reopen
  exact receipts, inspect composed model requests, exercise selected and
  unavailable capabilities through the normal dispatcher, and inspect durable
  events. Reuse the delegated-session Task isolation and independent-lease
  fixtures as evidence of existing contracts, without adding unnecessary Task
  grants to the web-only researcher. Put cross-module tests at the composition
  root or in an external test package if needed to avoid an import cycle.
- Through that primary seam, prove the complete assignment/result flow, child
  role and context separation, exact research capability exposure, rejection
  of fabricated excluded capability calls, no recursion, no automatic memory
  or Task projection, optional authorized Task association, and the absence of
  incidental Task creation or Task status changes.
- Inspect captured child requests and durable source attribution to prove that
  parent history, unrelated scope data, and automatically retrieved memory are
  absent. Explicitly supplied context must remain bounded and attributed.
  Exercise both live and historical compiler entry points to prove delegated
  assignments cannot become owner-assertion support or publish candidates.
- Keep supporting composition checks at the existing Plugin Manager/preset
  seam: required capability failure, Workspace preset denial, parent capability
  ceiling, old receipt preservation, exact worker receipt reconstruction, and
  unavailable or incompatible pinned providers. Exercise dispatch rejection as
  well as schema omission.
- Use focused real-SQLite contract tests only where atomicity or reopen cannot
  be demonstrated clearly through the conversation flow. Cover competing
  identical admissions, changed arguments with the same key, failure during
  admission, current access revalidation, unrelated-parent access, and result
  persistence before parent delivery. Assert durable observable invariants;
  avoid tests that duplicate private implementation structure.
- Use deterministic gates, clocks, and small injected policies for deadline,
  capacity, total model-call, context, and result limits. A child that repeatedly
  requests allowed capabilities must terminate at its limit, including when
  compaction consumes model calls. Missing provider usage must remain unknown.
- Follow existing turn-ownership race-test patterns for parent cancellation,
  parent lease expiry/replacement, child lease loss, disable after a parent pins
  delegation, and shutdown. Prove no further child admission or external
  execution after authority ends, no stale terminal overwrite, and bounded
  cleanup. Preserve accepted results when cancellation races parent delivery.
- Reopen SQLite after failures before admission commit, after admission before
  child start, during child execution, after final child answer acceptance
  before execution-result persistence, and after child result commit before
  parent delivery. Assert no duplicate child execution, correct execution-level
  recovery, no fabricated conversational outcomes, and no interference with an
  execution still owned by another live process.
- Manual demonstration: enable the compiled Subagents Plugin, start a newly
  composed eligible Global or project parent session, and request a bounded web
  research assignment. Observe the delegation invocation and returned evidence, then
  the parent's answer. Repeat cancellation and a missing/disabled capability
  case. Demonstrate Workspace rejection when reviewed preset allowances cannot
  be resolved. Once the separate Workspace prerequisite is available, use a
  revision that explicitly permits the research preset for the success case and
  demonstrate rejection from one that does not.
- While implementing, run focused composition, conversation, ownership, and
  SQLite tests, including the Go race detector for new concurrency behavior.
  Before code handoff, format changed Go code and run the repository's complete
  verify-change check. Documentation-only changes require the repository's
  whitespace check. Model review supplements these checks and does not replace
  them.

## Out of Scope

- Parallel foreground fan-out, asynchronous spawn/status/wait/cancel
  capabilities, background continuation, restart-driven execution, recurring
  scheduling, result outboxes, and notifications.
- Nested delegation, sibling messaging, child-session resume or multi-turn
  worker conversations, and arbitrary model-selected worker presets.
- Write-enabled workers, approval forwarding to unattended children, file
  mutation, worktree isolation, shell access, or a coding-worker preset.
- Memory-enabled worker presets, automatic retrieval for workers, semantic
  memory writes, or compiling worker transcripts into accepted knowledge.
- Child Todo capabilities, Task Access Grants, Task Focus, Task Claims, Task
  decomposition, and Task progress/result mutations. Existing contracts remain
  the basis for a later Task-capable preset.
- A generic Subagent Provider family, remote agent frameworks, third-party
  plugin distribution, dynamic executable loading, universal provider
  interfaces, or a new conversational agent loop.
- General reviewed user-preset authoring, a Workspace authoring overhaul,
  implicit Workspace allowlist changes, mid-session composition changes, or
  converting every existing built-in execution ability into a Plugin.
- A worker management dashboard, dedicated progress streams, frontend redesign,
  provider/model selection policies, exact monetary budgets, or broad changes
  to existing primary-session behavior.

## Further Notes

This is a concrete foreground delegation feature, not the generic subagent
provider family deferred by the original Plugin specification. It follows the
binding decisions to keep supervision in the Kernel, ship compiled First-party
Plugins, use one pinned Agent Preset per session, keep Capability Provider
interfaces focused, treat Workspace Access as an authority ceiling, apply
revocations immediately, and bound Workspaces to reviewed Agent Presets.

The existing foreground/background subagent research proposal is supporting
backlog material. This specification defines a narrower first outcome: one
web-only researcher. Optional Task linkage records intended-work lineage; it
does not introduce unused child grants or Todo capability access. The owner may
choose additional plugin capabilities through future approved presets, with
explicit memory and resource policies.

Implementation depends on the existing composed agent runtime, Web Plugin,
durable child-session shape, Composition Receipts, and fenced turn ownership.
Restricted child composition and Workspace preset validation must be completed
before enabling delegation in their applicable scopes. Current Workspace
registration records an opaque revision identity without the reviewed
preset-allowance content needed for the positive Workspace path. That content
and its approval provenance are a separate prerequisite from
[Workspace feature work, issue #71](https://github.com/davidadel66/evie/issues/71);
this specification does not authorize bypassing them or implementing a
general authoring system. Foreground research is independently demonstrable
from eligible Global/project sessions while Workspace execution remains
unavailable with an explicit dependency explanation. The current model/transport
changes in progress are not part of this specification; integrate with the
runtime contract present when implementation begins.

The principal risks are accidental built-in capability inheritance, memory
disclosure or source misattribution, duplicated execution after a retry,
continued child authority after parent ownership ends, and incomplete recovery
after a persisted result is not delivered. The acceptance and persistence
contracts above make these requirements observable and deterministic.
