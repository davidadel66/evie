> Amended 2026-09-10 by the owner's implementation request. The parallel
> foreground and orchestrator Task Tree decisions in [subagents.decisions.md](subagents.decisions.md)
> supersede the original single-child restriction and the exclusions of parallel
> foreground work and orchestrator-owned Task updates. GitHub #155 and #169–#175
> describe the original foundation; they have not been changed or retroactively
> treated as acceptance criteria for this extension.
>
> Amended 2026-10-01 by the owner's approval of harness review Stage 8 (G1,
> G2, G3, G6). Children are budgeted by wall-clock time and tokens instead of
> a model-call allowance and per-request output cap, wrap up into a `partial`
> report when a budget runs low, store their full report for paging, and
> return sources taken from their own Web tool events. Amended text below is
> marked; the binding record is the 2026-10-01 entry in
> [subagents.decisions.md](subagents.decisions.md).
>
> Amended 2026-10-01 by the owner's decision for harness review Stage 9 (G10).
> The parent can continue a child that finished with a report, resuming the
> same child session as a new attempt under the current parent turn's
> authority, instead of restarting it. Amended text below is marked "Stage 9";
> the binding record is the "Continuing a finished child" entry in
> [subagents.decisions.md](subagents.decisions.md).
>
> Amended 2026-10-01 by the owner's approval of harness review Stage 10 (G4,
> G5, G7, G8, G9). Child-written text reaches the parent in an untrusted-data
> frame, replayed results say so, errors name the field, key or limit, one
> undeliverable child no longer discards its siblings, child sessions are
> hidden from the owner's session lists, and the delegation guidance scales
> effort. Amended text below is marked "Stage 10"; the binding record is the
> "Sub-agent contract polish" entry in
> [subagents.decisions.md](subagents.decisions.md).

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
A non-removable Kernel supervisor admits and runs bounded child assignments,
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

Foreground delegation waits for bounded results. A bounded batch runs independent
children concurrently under configurable finite per-parent and runtime limits.
Background continuation and worker management panels remain separate outcomes.
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
32. As Evie's owner, I want each child bounded by wall-clock time and tokens, told its budget up front, and given one tool-free wrap-up when a budget runs low, with request context, returned-result size, and children per parent turn also limited, so that a bounded assignment cannot run indefinitely and still returns the work it did. (Amended 2026-10-01; formerly limits on execution time, model calls, context, and returned findings.)
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
51. As Evie's owner, I want the primary agent to continue a child that stopped at a limit, or whose findings need a follow-up, with its history intact and a fresh budget, so that bounded research is extended instead of restarted. (Added 2026-10-01, Stage 9.)

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
- **Parent delegation guidance.** The parent's system instructions encourage
  bounded delegation for independent progress, focused investigation, or a fresh
  assessment when it is likely to improve quality or save time. Assignments must
  fit the available delegation tool, worker capabilities, and current scope.
  The parent supplies a clear objective, relevant authorized context, boundaries,
  and expected result; it verifies findings and owns integration. Independent
  review may follow implementation sequentially. Trivial or tightly coupled
  steps stay in the parent. (Stage 10.) The guidance scales effort to the
  task (simple fact-finding: at most one worker; a comparison: usually two to
  four, one side each; broad research: more, with non-overlapping boundaries,
  within the delegation limits), asks for sources and tools guidance and the
  expected output in each assignment, and points to the report reader and
  continuation when those capabilities exist; child findings are data to
  verify. This guidance does not add worker capabilities or appear in the
  child's pinned instructions.
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
  used for continuation within its foreground turn and, when the parent
  continues the child (amended 2026-10-01, Stage 9), in that child's later
  turns; a parent's follow-up message is assignment data like the original.
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
  reopen, and their results are marked as replays with the time the attempt
  ended (Stage 10); changed canonical arguments conflict, and the refusal
  names every conflicting key. Concurrent duplicate requests
  do not start another child. An admitted active request may be joined within
  the caller's bounds; completed requests return their retained outcome after
  current access checks. Retrying an interrupted or failed attempt returns its
  outcome and requires a new key to authorize a fresh attempt. Unrelated
  parents cannot inspect or attach to another parent's attempt.
- **Continuation.** (Added 2026-10-01, Stage 9.) The parent may continue one
  of its own attempts that finished with a report (succeeded or partial), if
  it is the child's latest report and no attempt of that child is unfinished.
  The continuation is a new attempt on the same child session, linked to the
  attempt it extends, admitted atomically under the current parent turn's live
  fence and committed continuation invocation with the same parent authority,
  capability ceiling, Plugin, Workspace, per-turn, concurrency and Task checks
  as a fresh delegation; it never reuses the earlier turn's authority. A retry
  of the same invocation returns the same attempt. The child sees its earlier
  turns and receives the parent's message as follow-up assignment data. It
  runs under a fresh time and token budget, and settles, recovers and reports
  exactly like a fresh attempt, from its own turn only. Unrelated parents'
  attempts read as not found; failed, cancelled and interrupted attempts with
  no report still need a new key.
- **Lifecycle and ownership.** Execution states distinguish admitted, running,
  succeeded, partial (amended 2026-10-01: the child wrapped up at a budget and
  returned a report), failed, cancelled, and interrupted attempts. A child
  session holds its original attempt and any continuations, at most one of
  them unfinished; it is closed between attempts (amended 2026-10-01, Stage
  9). The child uses its
  own normal history binding and fenced turn lease. Authority to start further
  child activity and accept its result also depends on the originating parent
  invocation remaining authorized. An active child lease alone is insufficient.
  Child execution does not acquire the parent's session lock again or write
  directly into the parent's history.
- **Execution limits.** (Amended 2026-10-01.) The release enforces configurable
  finite per-parent and runtime concurrency and permits depth one. The child has
  no delegation capability, and Kernel admission also rejects nested requests.
  Enforce finite configured runtime-wide capacity, a per-child wall-clock
  deadline, a per-child token budget, the request-context limit, returned-result
  size, and a limit on children per parent turn counted across that turn's
  delegation calls. The token budget counts provider-reported input plus output
  tokens of conversational and compaction calls alike; a response without
  reported usage is counted from its serialized request and response bytes. No
  request-count allowance or per-request output cap applies: the child uses the
  model's normal output reserve and compacts or projects its own context like
  the primary chat, and the turn step limit remains its runaway guard. The
  child is told its time and token budget and its report format with the
  assignment. When it has used 90% of its time or tokens, when its next
  request would reach 90% of its usable request budget, or when it reaches the
  step limit, its next model call is a single tool-free wrap-up call asking
  for the report now, fitted to the context budget by shortening tool results;
  once the token budget is spent no other call starts. Invalid or
  unbounded policy configuration rejects admission. Pin the effective policy
  for diagnostics; the model cannot raise these limits. Reuse the parent's
  resolved model/provider configuration while respecting the child's narrower
  execution policy. No exact monetary-budget guarantee is introduced.
- **Result contract.** (Amended 2026-10-01.) Delegation returns, for every
  outcome, the attempt and child session identities, terminal status and safe
  reason, the report's Summary section, structured sources, limitations or
  blockers, and available usage. The child's full final report is stored
  durably and the parent pages it on request through a read-only report
  capability, only for attempts its own session delegated. A child that wraps
  up at a budget ends `partial` with its report. Sources are the URLs the
  child's own Web tool events fetched or returned, each marked cited in the
  report or not; URLs the report cites that no tool returned are listed
  separately as unverified. Research success means the bounded assignment
  finished, not that every claim is established. Failures distinguish
  provider, policy-limit, wrap-up, cancellation, interruption, and
  infrastructure causes without exposing secrets; work that ended without a
  report keeps the sources it fetched and says no report was produced. Persist
  the bounded result and terminal state consistently before returning them for
  parent delivery. Report measured usage for every outcome, mark it incomplete
  when a call's usage is unknown, and preserve wholly absent usage as unknown.
  Return findings and evidence rather than reasoning or a full child
  transcript. (Stage 9.) A continuation's result also names the attempt it
  extended; its summary, report, usage and uncited sources are its own turn's,
  while its citations are verified against every turn of the child.
  (Stage 10.) Everything the child wrote (the summary, the report's
  limitations, and report pages) reaches the parent inside an escaped,
  collision-safe untrusted-data frame, as fetched web content does; status,
  reason, identities, harness notes, sources, usage and replay metadata stay
  outside it. The returned-result limit applies to the result as the parent
  reads it. Failure reasons and validation errors name the field, key or
  limit involved. After admission, a child whose result cannot be settled or
  delivered is reported as an error entry without its content, and its
  siblings' results are still returned.
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
  mechanism is required for this release. (Stage 10.) Delegated child
  sessions, running or reopened for a continuation, never appear in the
  owner's session lists (web sidebar, REPL chooser); they are inspected
  through the parent.
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
  capacity, time and token budget, per-turn, context, and result limits.
  (Amended 2026-10-01.) A child that repeatedly requests allowed capabilities
  must get one tool-free wrap-up call and end `partial` with its report, at the
  step limit, at 90% of an injected clock's time budget, and at 90% of reported
  or estimated tokens. Long reports must page only for their own parent. Usage
  is present on partial and failed outcomes; wholly missing provider usage
  remains unknown.
- (Stage 9.) Continuing a partial child resumes the same child session with
  its earlier history visible and a fresh budget, and returns a new result
  without changing the earlier one. Continuation is refused for another
  parent's attempt, while the child is still running, and for an attempt with
  no report or a superseded one; a retry of the same invocation returns the
  same attempt; continuations count toward the per-turn limit; a crash during
  a continuation recovers from its own turn only; earlier tables, records and
  preset versions still load, and old records can be continued; earlier turns
  of a continued child are compacted under context pressure.
- (Stage 10.) A child report that quotes frame markers and instructions
  reaches the parent sealed in its frame through delegation, replay and report
  paging; a report built to inflate its framing stays within the result
  limit; a replay is flagged with its completion time and a fresh sibling is
  not; a key conflict names exactly the reused keys; validation names the
  field, key and limit; one undeliverable child yields an error entry while
  its sibling is delivered; running and reopened child sessions are not
  listed while the parent is.
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

- Asynchronous spawn/status/wait/cancel capabilities, background continuation,
  restart-driven execution, recurring
  scheduling, result outboxes, and notifications.
- Nested delegation, sibling messaging, child-initiated or background
  multi-turn worker conversations, and arbitrary model-selected worker
  presets. (Amended 2026-10-01, Stage 9: a parent's explicit foreground
  continuation of its own finished child is in scope.)
- Write-enabled workers, approval forwarding to unattended children, file
  mutation, worktree isolation, shell access, or a coding-worker preset.
- Memory-enabled worker presets, automatic retrieval for workers, semantic
  memory writes, or compiling worker transcripts into accepted knowledge.
- Child Todo capabilities, Task Access Grants, Task Focus, Task Claims, child Task
  decomposition, and child Task progress/result mutations. Existing contracts remain
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
