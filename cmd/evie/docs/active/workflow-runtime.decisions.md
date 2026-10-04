# Workflow Runtime implementation decisions

## 2026-10-04 — Effect protocol and reconciliation (Stage 14, sub-stage 6)

Source: sub-stage 6 of Stage 14 in
[docs/memory-architecture.md](../../../../docs/memory-architecture.md) and
the sub-stage 5 entry below ("What sub-stage 6 must pick up"): a separate
effect provider role with idempotency, reconciliation and Resource Conflict
Keys pinned in the same composition entries; per-node admission before each
Effect Intent; `max_calls_per_run` counted by intents; the lease checked
right before the call; an interrupted effect is Outcome Unknown and moves
the run to `needs_reconciliation`, never the interruption rerun; automatic
resume stays off while an effect is unknown. Binding: ADR 0008, 0009,
0010, 0015, 0016, 0017, 0021, 0025, 0027, 0029 and 0043 and this spec's
effect ordering, Outcome Unknown, reconciliation, retry, Resource Conflict
Key and Standing Authority decisions.

### What was built

- **Effect provider role.** Plugins contribute effects through
  `plugins.WorkflowEffectProvider` (ADR 0015), separate from reads and
  tools: a manifest-declared Capability ID and contract version, closed
  object input and output, the exact Connection IDs and resources served,
  and four declarations. `Idempotent`: the provider applies one idempotency
  key at most once, so the same Effect Intent may be sent again.
  `ConflictKeys` with its named `ConflictRule` (such as `account`): the
  Resource Conflict Keys a request touches; without them one key per
  connection and resource. `Perform`: sends a request; only an error
  wrapping `plugins.ErrWorkflowEffectNotPerformed` (`EffectNotPerformed`)
  proves nothing happened. `Reconcile` (optional): for an intent whose
  outcome is unknown, says by lookup or idempotent replay whether the
  effect with its key happened (`happened` with its output,
  `did_not_happen`, or `unknown`); it must never perform the effect twice.
  The Plugin Manager validates contributions like reads (declared once, one
  role only, object types, valid selectors, a perform function, a rule
  exactly when keys are derived) and resolves the behavior only while the
  plugin is ready. `workflow.CapabilityContract` gains `Reconciles` and
  `ConflictKeys`; both enter the schema hash only when set, so every hash
  pinned before (Finance's reads included) is unchanged, which a test pins.
  Runs pin effects in the same Execution Composition entries as reads, with
  `effect`, `idempotent`, `reconciles` and `conflict_keys`; any change is a
  schema change and pauses a pinned run (ADR 0025).
- **Review.** Reports and approval cards describe each effect as "effect,
  idempotent, reconciled by the provider|owner, conflict keys per <rule>".
  Validation warns (it does not refuse) when an operation's effect steps
  could send more intents with their retries than its `max_calls_per_run`.
- **Durable sequence.** The run's input is accepted and checkpointed, and
  its definition, authority and composition pinned, at start. Before each
  effect the pinned provider must be ready (otherwise the run pauses with
  nothing recorded, and resumes by itself: nothing is uncertain). Then one
  fenced transaction (`BeginWorkflowEffect`) applies per-node admission (the
  same `admitWorkflowNode` as `AdmitWorkflowNode`: run admission, the
  operation within the run's own approved Standing Authority, the
  connection and resource within current Workspace Access), counts the
  run's intents of the operation against `max_calls_per_run`, takes the
  Resource Conflict Keys, and records the node attempt with its Effect
  Intent: step, checkpoint, provider called (Capability ID, contract,
  schema hash, plugin, version), connection, resource, idempotency key,
  idempotency, who reconciles, conflict keys, input and the authority
  evaluation (version, authority hash, operation, this call's number, the
  bound, the access revision). The lease is checked again immediately
  before the call, and the call's context ends when the lease is lost or
  the effect timeout passes (`SchedulerConfig.EffectTimeout`, default 30
  seconds, at most 10 minutes), not when the runtime stops: an effect in
  flight completes or ends unknown. The provider's response is normalized,
  checked against the contract and recorded as the Effect Receipt with the
  attempt's output and the next checkpoint in one transaction
  (`FinishWorkflowEffect`), which also releases the conflict keys. A proven
  failure records a failed receipt and retries within the step's Retry
  Policy after its durable backoff, or fails the run after the last
  attempt. Attempt and intent share one transaction, so "after the attempt,
  before the intent" is not a reachable crash state; receipt, output and
  checkpoint share one, so a crash after them reruns only the next step.
- **Idempotency key.** `sha256:` over the canonical JSON of the run's
  Workspace and Run Key (its logical execution, ADR 0021), the step, the
  capability, connection, resource and the deterministic input. Every
  attempt and reconciliation of a node shares it, whichever implementation
  serves it, and so does a later run of the same Run Key after a failure,
  so an idempotent provider never repeats an effect that already happened
  there.
- **Outcome Unknown.** An answer after the effect timeout that is not proof
  of non-performance (after a grace of up to one second for the provider's
  own answer), any other error, a panic, or a response outside the contract
  records the intent's outcome unknown (`workflow_effect_unknowns`, with
  the reason and any malformed response) and moves the run to
  `needs_reconciliation`, in one transaction with the attempt's end. A
  runtime that dies between intent and receipt leaves the attempt in
  flight; the next claim records it interrupted and the intent unknown in
  the claim's own transaction. Every claim also records unknown any intent
  whose attempt ended without a receipt (an Evie that predates the effect
  protocol may have interrupted it and paused the run), and may claim such
  a run before its recheck time. The run's error says what is unknown and
  how it is reconciled. Provider text quoted in receipts, unknown outcomes
  and run errors is clipped to 1,000 bytes.
- **Blocked until reconciled.** While any intent of a run has no receipt
  the store refuses every node attempt (dependent nodes are blocked), every
  pause, resume and failure of the run, ending the effect attempt without
  its receipt, and treating it as a safe interruption. A run that needs
  reconciliation is never paused or resumed; only a receipt moves it back
  to `running`. Interrupted effect attempts do not count toward the
  per-node interruption bound.
- **Automatic reconciliation.** When the provider reconciles, the run is
  due after a settle delay of one effect timeout when its call timed out or
  failed (the pass that found it unknown waits it out under its lease, so
  a request still landing is not looked up too early), and at once after a
  crash, whose holder stopped at least a lease earlier (`DueWorkflowRuns`
  lists `needs_reconciliation` runs with a not-before time). The runtime
  checks the run's pins and approved
  definition, the provider's readiness and per-node admission (a replay
  may perform), checks the lease, calls `Reconcile` with the intent's key
  and records its answer as the receipt (`source reconcile`, with its
  evidence): `happened` with output inside the contract, or
  `did_not_happen`. Then the run continues in the same pass. While the
  provider is unavailable or its reconciliation fails or times out, the run
  stays `needs_reconciliation` and is tried again after the pause recheck
  (one minute). When the provider cannot prove the outcome, answers outside
  the contract, is no longer admitted or does not reconcile, the run waits
  for the owner (no not-before time; no scheduler claims it).
- **Owner reconciliation.** `evie workflows reconcile <run-id> <step>
  --outcome happened|did-not-happen [--evidence <text>] [--output-json
  <file>] [--detach] [--json]` and `POST /api/workflows/reconcile`
  (`run_id`, `step`, `outcome`, `evidence` up to 2,000 bytes, `output`,
  `detach`; the surface is always `web`) claim the run's lease, record an
  immutable receipt with the owner, surface and evidence, and continue the
  run in the foreground unless detached. An effect that happened needs its
  output, checked against the step's contract (`{}` when none is given); a
  run another runtime is advancing, a step without an unknown effect and a
  finished run are refused (`ErrReconcileRefused`, web 409
  `reconcile_refused`). No model-facing tool reconciles.
- **Continuing from a receipt.** An effect reconciled as having happened
  supplies the step's output from its receipt through an attempt that
  calls nothing (its note names the intent and how it was reconciled), with
  the next checkpoint. One that did not happen retries only within the
  step's Retry Policy, sending the same idempotency key again; with no
  attempt left the run fails ("steps.post: Effect Intent #1 did not happen
  (...); its Retry Policy allows no further attempt (1 attempt)").
- **Resource Conflict Keys.** Each key is scoped by its connection
  (`conn-ledger/account:household`). A partial unique index over
  `workflow_effect_conflicts` lets one unresolved intent hold a key at a
  time, across every run and Workspace; an intent takes its keys in its own
  transaction and releases them with its receipt, so an unknown intent
  keeps its keys until reconciled. A run whose effect needs a held key
  pauses visibly before the intent ("steps.post waits: Resource Conflict
  Key ... is held by run ... steps.post (Effect Intent #1, in flight)"),
  rechecked at the scheduler's poll while the holder is in flight and at
  the pause recheck while the holder waits for reconciliation, and stays
  paused through rechecks until the key is free. Deterministic steps and
  reads are never serialized.
- **Enforcement.** Out-of-bounds effects stop before the provider is
  called: admission refusals pause the run with nothing recorded, and an
  intent past `max_calls_per_run` fails the run ("steps.post: the Standing
  Authority's max_calls_per_run 1 for ledger.post on conn-ledger is reached
  (Effect Intents sent in this run: 1); the effect is refused before the
  provider is called").
- **Records.** `workflow_effect_intents`, `workflow_effect_unknowns` and
  `workflow_effect_receipts` are immutable; `workflow_effect_conflicts`
  rows are only released. `evie workflows inspect` and the inspect route
  list each intent with its key, conflict keys, call number, input,
  unknown outcome and receipt; a run needing reconciliation is labelled so.
- **Runtime version.** Runs pin `evie.workflow.runtime/3`; `/1` and `/2`
  stay compatible (they hold no effect: sub-stage 5 refused effects at
  start). An older Evie pauses a `/3` run as incompatible rather than
  running its effects.
- **No production effect provider.** None of the candidates fits safely
  now. `finance.categorize` is already a tool capability (one Capability,
  one role); a workflow effect over the ledger would need a new contract
  and a Finance version, and the finance database keeps no per-request
  record, so neither lookup by key nor a replay could prove what a given
  intent did or reproduce its output (Categorize is idempotent over state,
  not per request). Task Tree updates belong to the orchestrating
  conversation, which a run does not have. The protocol ships with test
  providers only (`internal/workflow/testdata/post-share` and the fakes in
  the runtime tests); the first real provider should keep a per-key record
  so it can reconcile.
- **AI steps and simulation.** Only a capability effect step reaches
  `Perform`, and only through this protocol; AI, agent and map steps are
  still refused at start, and no other code path holds the effect role
  (ADR 0010). Simulation is unchanged: effects become previews with the
  fixture's response, reach no provider and record no intent or receipt
  (tested through a proposal).

### Decisions and known limits

- `max_calls_per_run` counts every Effect Intent (every provider call),
  retries included, not distinct idempotency keys: the conservative
  reading, which a validation warning makes visible.
- Attempt states are not extended (no table rebuild): a live unknown
  outcome ends its attempt `failed` ("outcome unknown: ..."), a crash
  leaves it `interrupted`; the explicit Outcome Unknown is the intent's
  state, and the run's `needs_reconciliation`.
- No new recovery event kinds (the events table checks its kinds):
  effect history lives in the intent, unknown and receipt records.
- Conflict keys are global, not per Workspace: a key names an external
  resource, which Workspaces may share through a connection (ADR 0042),
  and this is strictly stronger than serializing within one Workspace.
  ADR 0043's "broader single-file execution" is the default rule.
- A lookup that finds "did not happen" while the request is still in
  flight (a stale holder's, or a timed-out one) could race it. The lost
  lease cancels a stale holder's context, the first lookup after a timeout
  waits one effect timeout, retries exist only for idempotent providers
  (validation) and resend the same key; what remains is a non-idempotent
  provider's request landing later still, after its lookup made the run
  fail rather than retry. Such a provider's lookup should only answer
  "did not happen" when it can prove the request can no longer land.
- Stopping the scheduler waits at most five seconds; an effect running
  longer keeps its lease until it expires, and the next holder records it
  unknown.
- A conflict wait re-reads the approved commit at each recheck (one second
  by default while the holder is in flight).
- Standing Authority declares operations, connections, exact resources and
  `max_calls_per_run`; amounts and recipients are not in the schema yet
  (resources stand in for recipients). Declaring them is a schema
  amendment, and changing them a new approved version.
- `limits.max_duration` keeps counting while a run waits for
  reconciliation; a reconciled effect's output is still recorded, and the
  run then fails at its next node.
- ADR 0021's audited override (not built) must add an override identity to
  the key derivation, or an idempotent provider would answer the rerun with
  the earlier result.
- The owner's evidence is optional text; the receipt records who, how and
  when. Reconciliation has no web UI control yet (it needs the Workspace
  home's run view); the API route exists.

### What sub-stage 7 must pick up

- AI steps return typed proposals only: their output may reach an effect
  only as a later capability step's input through deterministic
  validation, and never through the effect role (ADR 0010).
- Durable human interruptions must keep automatic resume off while one is
  outstanding (ADR 0029), as the store now does for unknown effects
  (`ResumeWorkflowRun`, `PauseWorkflowRun`, `BeginWorkflowAttempt`). The
  spec presents an unprovable effect to the owner as a durable human
  interruption; today it is `needs_reconciliation` with the CLI and API, so
  sub-stage 7 decides whether reconciliation also becomes an interruption.
- Bump the runtime to `evie.workflow.runtime/4` when AI steps execute and
  declare `/3` compatible. Sub-stage 8's cancellation must let an effect in
  flight complete or reach reconciliation: the store refuses to fail a run
  with an unresolved intent, and cancellation needs the same rule.

## 2026-10-04 — Capability reads (Stage 14, sub-stage 5)

Source: sub-stage 5 of Stage 14 in
[docs/memory-architecture.md](../../../../docs/memory-architecture.md) and
the sub-stage 4 entry below ("What sub-stage 5 must pick up"): capability
reads as fenced attempts with the read's backoff as the durable
not-before time, providers pinned in the Execution Composition with a
compatibility check that pauses on a missing or changed provider, an
audited record for a compatible substitution, the run's context passed to
providers, and a per-node admission check for sub-stage 6. Binding: ADR
0015, 0016, 0017, 0025, 0027, 0029, 0033, 0035, 0037, 0039 and 0041 and
this spec's capability-node, Execution Composition, retry and
Compatibility Resolution decisions ("a capability node invokes one
canonical Capability ID through a focused runtime-owned interface"; "reads
use limited backoff"). The orchestrator also decided the interruption
adjustment below, which changes a sub-stage 4 decision.

### What was built

- **Provider role.** Plugins contribute workflow capability reads through
  a focused role, `plugins.WorkflowCapabilityProvider` (ADR 0015), separate
  from the tool role: each read has a canonical plugin-namespaced
  Capability ID declared in the manifest (ADR 0035), a contract version,
  closed object input and output types, the exact Connection IDs and
  resource selectors it serves, and a read function. The Plugin Manager
  validates contributions once (declared at the manifest's contract
  version, contributed by exactly one role, object types, valid selectors,
  a read), lists the compiled contracts whatever a plugin's state
  (`WorkflowContract`, what definitions validate against) and resolves a
  callable provider only while its plugin is ready, its optional
  dependencies are loaded and its connection is ready
  (`ResolveWorkflowCapability`, after applying the owner's durable enabled
  choices); otherwise the read is unavailable, returned with the provider
  it would be so a pinned run can still be compared with it. A contract's
  schema hash covers its input, output and read or effect classification.
  A replacement implementation declares the earlier implementations whose
  pinned work it continues through the manifest's existing `ResumableFrom`
  evidence, now also checked against workflow capabilities (ADR 0037).
  The role carries reads only; effects are a separate role in sub-stage 6.
  The runtime consumes the seam through its own interface
  (`workflowruntime.CapabilityProviders`); `ContractsOnly` is a catalog no
  run can call.
- **Production catalog.** Finance 1.1.0 contributes two fixed,
  parameterized reads of the local finance database through the read-only
  Spending queries the Spending view uses: `finance.month_cash_flow`
  (input `month` as `YYYY-MM`; the month's posted inflow, outflow, net and
  transaction count as of its last day, pending transactions, and the
  month by category exactly as the Spending view reconciles it) and
  `finance.month_transactions` (input `month`, optional `status`, `offset`
  and `page_size` 1 to 100; one page newest first with each transaction's
  saved classification entries and the month's counts). Both are contract
  1.0.0, return whole cents as integers (a sum outside int64 is refused,
  never rounded), refuse undeclared input fields, never take query text,
  tables or columns from a definition and never read bank credentials. No
  Account Connection registry exists, so the plugin names the one
  connection it serves, `finance-local` (the owner's local finance
  database and the banks linked through it), with one resource, `ledger`;
  a Workspace grants it with `evie workspaces access <id> --set
  finance-local=ledger`. Finance's tools are unchanged, so 1.1.0 declares
  that it continues 1.0.0's pinned sessions (a session pinned to 1.0.0
  records a Compatibility Resolution on its next resume, as Web's 1.1.0
  did).
- **Left out, deliberately.** `web.search` sends workflow data (the query)
  to a third-party search service that Standing Authority cannot bound to
  exact resources; `web.fetch` could be bound to exact reviewed URLs, but
  its untrusted pages have no workflow consumer before AI steps (sub-stage
  7), and research already reads the web through agent steps with the
  research worker's fences (sub-stages 9 and 10). YouTube reads have the
  same shape as web reads. Memory reads are not capability nodes: the spec
  gives AI Nodes "explicitly requested scoped memory" instead (sub-stage
  7). Finance sync, rules and categorize are effects (sub-stage 6). Each
  can be added later as a reviewed contract.
- **Validation.** Definitions validate against the compiled catalog: the
  production runtime's is the Plugin Manager's, and `evie workflows
  validate|simulate` use the same compiled contracts (they were reviewed
  against an empty catalog). A step must name a connection and resource
  its provider serves. Workspace Access belongs to a Workspace, so the
  standalone commands do not check it (`Environment.AccessUnchecked`, with
  a printed note); proposing, approving and running always do. Simulation
  still uses fixtures and reaches no provider. `internal/workflow/testdata
  /shared-spending` (read the month's cash flow, split its outflow, branch
  on the remainder) is the example.
- **Pinning.** A run's Execution Composition (runtime
  `evie.workflow.runtime/2`) lists, per capability it reads, the Capability
  ID, contract version, schema hash, plugin and implementation version and
  the Connection IDs its steps use. A start resolves each provider and is
  refused, with nothing recorded, while one is unknown or unavailable
  (`ErrCapabilityUnavailable`; the web answers 409
  `capability_unavailable`). A definition with a capability effect is
  refused at start as "capability effect" steps, like AI, agent, map and
  interruption steps; such a definition cannot be approved yet anyway,
  because no plugin contributes an effect.
- **Compatibility at every claim.** After the pinned runtime check, each
  pinned provider is resolved again and the run pauses visibly
  (`waiting_for_dependency` with the reason, rechecked like any pause) when
  the capability is missing, its contract version or schema hash changed,
  it no longer serves a pinned connection, or another implementation
  version provides it without an approved substitution. The reason names
  the next step: a replacement that declares (ADR 0037) it continues the
  pinned implementation with the same contract and schema gets the exact
  `evie workflows substitute <run-id> <capability> <version>` command;
  one that does not, or that changed the contract, can only be continued
  by an Evie with the pinned provider or by migration (sub-stage 12).
  Nothing is substituted silently (ADR 0025). When the run's next step is
  a read, its provider must also be ready now, so a run paused for a
  disabled plugin stays paused through rechecks instead of resuming and
  pausing again.
- **Audited substitution (ADR 0041).** `evie workflows substitute
  <run-id> <capability> <version> [--json]` (owner, CLI only; no web
  route and no model tool) records, for an unfinished run, that it
  continues one pinned capability with the replacement this Evie has, only
  when that is the named version, offers exactly the pinned contract and
  schema, still serves the run's connections and declares the
  continuation. `workflow_run_substitutions` keeps the run, capability,
  plugin, pinned and replacement versions, contract, schema hash, the
  evidence text, approver `owner`, surface and time; rows are immutable and
  a repeated approval returns the same row. The run's pins never change;
  inspection shows each substitution. The run continues at its next
  recheck.
- **Per-node admission.** Before a read, `AdmitWorkflowNode` applies, in
  one fenced transaction: run admission for the run's own version
  (deactivation and every block, including any access revocation since
  its pinned revision), the operation within the Standing Authority
  approved with that version, and the connection and resource within the
  Workspace's current access. A refusal pauses the run before the node
  and the provider is never called (ADR 0027, 0033). Sub-stage 6 calls the
  same check before each Effect Intent.
- **Fenced reads.** A read step then: checks its limits (Retry Policy,
  `limits.max_attempts`, `limits.max_duration`), resolves its input and
  checks it against the contract input (a mismatch fails the run: it is the
  same every time), records the attempt under the lease, checks the lease
  again immediately before the call (`CheckWorkflowRunLease`), and calls
  the provider with a context that ends when the lease is lost, the
  runtime stops or the read timeout passes (`SchedulerConfig.ReadTimeout`,
  default 30 seconds, at most 10 minutes). A provider that ignores its
  context is not waited for, and a panic is caught. The output is
  normalized to canonical JSON (at most 1 MiB) and checked against the
  contract output; it is recorded with the next checkpoint in one fenced
  transaction. A provider error, a timeout ("the read did not finish within
  30s"), a panic or a malformed response ("malformed response: …") fails
  the attempt; with attempts left, the step's `retry.backoff` becomes the
  run's durable not-before time (reads use the limited backoff validation
  already bounds: at most five attempts and ten minutes), otherwise the run
  fails "steps.<id> failed after N attempts: …".
- **Cancellation in flight.** A lost lease cancels the read; the stale
  holder writes nothing and whichever runtime claims the run next records
  the attempt interrupted and reads again. Stopping the runtime cancels
  the read too; its holder records the attempt interrupted itself
  (`InterruptWorkflowAttempt`, with an `attempt_interrupted` event) and
  releases the run, which the next process continues at once.
- **Interruption adjustment (decided by the orchestrator).** An
  `interrupted` attempt of a deterministic step, a branch decision, a
  capability read or (sub-stage 7) an AI step no longer counts against the
  step's Retry Policy or `limits.max_attempts`, because rerunning such a
  node from the same checkpoint is safe; this supersedes sub-stage 4's
  "Retry after interruption". Instead a separate per-node interruption
  bound (`SchedulerConfig.Interruptions`, default 3, at most 10) counts the
  node's interrupted attempts at one checkpoint, and when it is reached the
  next claim fails the run visibly ("steps.split was interrupted 3 times at
  checkpoint 0 (each time its runtime crashed, stopped or lost its lease
  before recording the outcome); the run fails instead of running it
  again: …") rather than looping on a node that kills its process. Attempt
  numbers still count every attempt. A graceful stop during a read counts
  toward the bound like a crash. Effects (sub-stage 6) are different: an
  interrupted effect is Outcome Unknown and must never be rerun
  automatically.
- **Earlier runs.** This runtime declares `evie.workflow.runtime/1`
  compatible: those runs hold only deterministic and branch steps, whose
  semantics are unchanged, and the interruption adjustment only lets a pure
  node run again. An older Evie pauses a `/2` run as incompatible.
- **Wiring.** `evie serve` and the REPL give the Workflow Runtime their
  Plugin Manager; a one-shot `evie workflows` command builds a Plugin
  Manager of the workflow capability plugins (Finance) that follows the
  owner's durable enabled choices with the same defaults. `evie workflows
  inspect` shows the pinned providers and substitutions.

### Decisions and known limits

- Every pinned provider is checked at every claim, including providers
  whose steps already ran: a run continues only under its whole pinned
  Execution Composition or an audited substitute (ADR 0025), even when no
  read is left.
- A provider that becomes unavailable during a read fails that attempt like
  any other read failure, within the Retry Policy; only unavailability
  seen before the read pauses the run.
- `max_calls_per_run` is not enforced at run time for reads: an acyclic
  graph reads each step at most once successfully, and retries are bounded
  by the Retry Policy. Sub-stage 6 enforces it for effects, counting
  Effect Intents.
- Reads are not serialized by Resource Conflict Keys (ADR 0043 serializes
  effects only); providers declare none yet.
- A replaced provider needs one substitution per run; there is no bulk
  approval.
- The finance reads see the local database as last synced; they do not
  sync. A missing finance database reads as an empty month, as in the
  Spending view.

### What sub-stage 6 must pick up

- A separate effect provider role with idempotency, reconciliation and
  Resource Conflict Keys, pinned in the same composition entries.
- Call `AdmitWorkflowNode` before each Effect Intent, enforce
  `max_calls_per_run` by Effect Intents, and check the lease right before
  the provider call, as reads do.
- An interrupted effect attempt is Outcome Unknown: it must not be rerun by
  the interruption path or counted as a safe interruption; the run moves to
  `needs_reconciliation` instead, and automatic resume stays off while an
  effect is unknown (ADR 0029).

## 2026-10-04 — Durable restart and leases (Stage 14, sub-stage 4)

Source: sub-stage 4 of Stage 14 in
[docs/memory-architecture.md](../../../../docs/memory-architecture.md) and
the sub-stage 3 entry below ("What sub-stage 4 must pick up"): fenced
leases on attempt begin, finish and run failure, a background scheduler
and a recovery scan over unfinished runs, a terminal record for an attempt
a crash left running, admission and the approved definition re-checked
before a run continues, pinned runtime compatibility, and a durable
not-before time for backoff. Binding: ADR 0016, 0017, 0025, 0029 and 0033
and this spec's lease, scheduler, recovery, state and retry decisions
("foreground and background runs share one runtime"; "session disconnect
does not cancel a run").

### What was built

- **Leases.** `workflow_run_leases` holds one row per run any runtime ever
  claimed: the holder (one runtime instance, `evie-<pid>-<uuid>`), a
  fencing token, and acquired, heartbeat, expiry and release times. Each
  claim raises the token by one (a trigger refuses a smaller token and the
  rows are never deleted). A run can be claimed when it was never claimed,
  when its lease was released, or once its lease expired; claiming an
  expired lease is a takeover. The holder renews the lease every third of
  its duration (default 30 seconds) and releases it when it stops
  advancing the run. Lease and retry times are stored as fixed-width UTC
  text so SQLite compares them in time order.
- **Fencing.** Every attempt begin and finish (and so every checkpoint),
  run failure, pause and resume takes the lease and runs in one immediate
  transaction that requires the holder's exact token, unreleased and
  unexpired, at its start and again at a fresh time before it commits; a
  holder whose lease expired, was released or was taken over writes
  nothing (`ErrWorkflowRunLeaseLost`), even before anyone takes over.
  Each attempt records the holder and token it ran under. Several Evie
  processes (the REPL, `evie serve`, a one-shot CLI start) share the
  database; exactly one of them can record a run's progress at a time.
- **Interrupted attempts.** Attempts gain the state `interrupted`.
  Sub-stage 3's attempt table had a closed state check, so it is rebuilt
  once at startup (rows and rowids copied unchanged, lease columns `''`
  and `0`, triggers recreated; the shape is re-read under the write lock
  and an unknown shape fails closed), and runs gain `not_before`. A claim
  records, in its own transaction and before anything else, every attempt
  an earlier lease left `running` as `interrupted` ("interrupted: holder X
  (lease N) stopped before the attempt's outcome was recorded"), so the
  new holder starts from a settled checkpoint. This covers a crash, a lost
  lease and a write that failed. Chosen over recording such attempts
  `failed` so inspection tells a crash from a function's own error.
- **Retry after interruption.** (Superseded by sub-stage 5's interruption
  adjustment above: interruptions no longer count against the Retry Policy
  or `limits.max_attempts`, and a per-node bound stops crash loops.) An
  interrupted attempt counts as an
  attempt of its step's Retry Policy and of `limits.max_attempts`, like a
  failed one, as the sub-stage 3 entry asked ("retries from the same
  checkpoint within its policy") and as the amendment already says of agent
  attempts. A deterministic step retries from the same checkpoint with the
  same recorded input, so the run's output is identical to an
  uninterrupted one; a step whose policy allows one attempt (the default)
  fails the run visibly ("steps.leftover failed after 1 attempt:
  interrupted: …") rather than looping on a node that kills its process.
  A branch decision is pure and is simply decided again.
- **Output and checkpoint together.** An attempt's output and the next
  checkpoint were already one transaction; now it is also fenced. A crash
  or fault between them cannot leave an output without its checkpoint:
  both commit or neither does, and recovery then treats the attempt as
  interrupted (proved by a test that makes the checkpoint write fail).
- **Durable backoff.** A failed attempt with attempts left records the
  step's `retry.backoff` as the run's `not_before`, in the transaction that
  records the failure, with a `retry_scheduled` event, and its pass
  releases the lease. No runtime can claim the run before that time
  (`ErrWorkflowRunNotDue`), across restarts; the retry's begin clears it.
  The in-process sleep is gone.
- **Continuing a run.** Every claim checks, in order, before any node
  runs: the run's pinned `composition.runtime` is one this runtime declares
  compatible (`evie.workflow.runtime/1`, unchanged because this sub-stage
  keeps the node semantics; the run's pinned step kinds must also be ones
  it executes) (ADR 0025); run admission (`AdmitWorkflowRun`:
  `authorizeWorkflowActivation`, so the definition's latest version is
  approved, not deactivated and not blocked, and the run's own pinned
  version is neither deactivated nor blocked, ADR 0033); and the pinned
  commit read through procedural Git still validates and hashes to the
  approval (`approvedDefinition`). Then `ResumeWorkflowRun` re-applies
  admission in the transaction that resumes a paused run. A run of a
  superseded version continues under its own pins.
- **Visible pause.** When a check fails, the run pauses: state
  `waiting_for_dependency`, its `error` holding the reason (for example
  "workflow is not active: version 1 was deactivated", "workflow is
  blocked: the Workspace is archived", "the approved workflow definition
  no longer matches its approval: …", or "its pinned runtime … is not one
  this Evie declares compatible …; continue it with an Evie that does, or
  migrate it…"), and a recheck time (`not_before`, default one minute).
  A `paused` event is recorded once per reason. At each recheck the
  checks run again; when they pass, the run resumes by itself (a `resumed`
  event; safe under ADR 0029 because no run yet has an effect or a human
  interruption). Nothing is migrated or failed: a run whose own version
  was deactivated or whose access was revoked stays paused, holding its
  Run Key, until cancellation (sub-stage 8) or migration (sub-stage 12).
- **Scheduler.** `Runtime.Start` runs a scheduler in long-lived processes:
  `evie serve` (full and management-only modes) and the REPL start it
  like the memory background pass (a configuration problem disables it in
  the REPL with a logged reason). Each poll (default every second, and at
  once when a local start wakes it) lists due runs (`DueWorkflowRuns`:
  queued, running or paused runs past their `not_before` whose lease was
  never taken, was released or expired, oldest first) and runs an
  execution pass for each, at most `EVIE_WORKFLOW_WORKERS` at once
  (default 2, 1 to 32; an invalid value is refused). The first poll is at
  start, so every startup is a recovery scan, and every later poll is one
  too: a run whose holder died is due once its lease expires.
- **Shutdown.** Stopping the scheduler ends every pass between nodes: a
  node in flight finishes and is recorded, the lease is released (with a
  `lease_released` event when the run still has work), and stop waits at
  most five seconds. The next process continues the run at once, with no
  takeover and nothing interrupted. A process killed outright leaves its
  lease to expire, and the next scheduler recovers the run.
- **Starting.** `StartRun` records the run as before. With `Detach` it
  returns the queued run at once and wakes the local scheduler. Otherwise
  it waits until the run finishes or pauses: in a process whose scheduler
  runs, the scheduler executes it and the start only watches; elsewhere
  (the one-shot CLI) the start executes it itself, detached from the
  caller's cancellation. If another process holds the run, the start
  watches until that process finishes it or its lease expires. A caller
  that stops waiting (a closed browser, a cancelled request) never stops
  the run.
- **Surfaces.** `evie workflows run [--detach] <workspace-id> <name> …`:
  by default it waits and shows the finished or paused run (a paused run
  exits with "workflow run … is paused: …"); `--detach` shows the queued
  run and says a running Evie (`evie serve` or the REPL) executes it.
  Stopping a foreground CLI start ends only that process: its lease
  expires and a long-lived Evie recovers the run. `POST
  /api/workflows/start` accepts `"detach": true`. `evie workflows inspect`
  and `POST /api/workflows/inspect` show the lease (holder, token, expiry
  or release, last heartbeat), a paused run's reason and recheck time, a
  pending retry time, each attempt's holder and token (JSON), interrupted
  attempts, and the recovery events (`workflow_run_events`, append-only:
  `lease_taken_over`, `attempt_interrupted`, `paused`, `resumed`,
  `retry_scheduled`, `lease_released`). The Workspace home's run view
  shows the same and labels a paused run's reason as a pause, not an
  error.

### Decisions and known limits

- Routine claims and clean releases record no event; the lease row keeps
  the latest holder. Rechecks of a paused run that stays paused claim and
  release the lease (raising the token) without recording anything.
- Leases fence durable writes, not computation: after a takeover the stale
  holder may still finish computing a node, but its result is refused and
  the node runs again under the new lease. Deterministic nodes are pure,
  so this is harmless; sub-stage 6 must check the lease immediately before
  invoking a provider, after recording the Effect Intent under the fence.
- Admission is re-checked at every claim (start, recovery, each retry
  after its backoff, each recheck), not before every node of an
  uninterrupted pass; revocation pauses a run at its next claim.
  Sub-stage 6 adds the check before each covered effect (ADR 0017, 0033).
- A paused run holds its Run Key, so a new start with the same key returns
  it, until sub-stage 8 lets the owner cancel it.
- `limits.max_duration` keeps counting while a run waits for a retry or is
  paused; a run resumed after its limit fails at its next node.
- Lease expiry is wall-clock based; processes share one machine's clock.
- Automatic resume of a paused run assumes no Outcome Unknown and no
  outstanding human interruption, true until sub-stages 6 and 7; they must
  keep such runs paused (ADR 0029).

### What sub-stage 5 must pick up

- Capability reads run as node attempts under the same lease: begin the
  attempt under the fence, call the provider, finish it with its output and
  checkpoint in one fenced transaction; a read interrupted by a crash is
  recorded `interrupted` and retried within its Retry Policy, with the
  read's limited backoff as the durable `not_before`.
- Pin capability providers (Capability IDs, contract and schema hashes,
  provider versions, Connection IDs) in the run's Execution Composition and
  extend `incompatible` so a run whose pinned provider or contract is
  missing or changed pauses visibly (ADR 0016, 0025), instead of
  substituting; an audited compatible substitution (ADR 0041) needs its own
  record.
- Pass the run's context to providers so that a lost lease or a stopping
  runtime stops a read in flight, and make the per-node admission check
  that effects (sub-stage 6) will need.

## 2026-10-04 — Manual deterministic runs (Stage 14, sub-stage 3)

Source: sub-stage 3 of Stage 14 in
[docs/memory-architecture.md](../../../../docs/memory-architecture.md) and
the sub-stage 2 entry below, which left this sub-stage run admission
through `authorizeWorkflowActivation` inside the run's transaction, the
definition read from the activation's commit through `procedural.Source`
and re-hashed, and the version, both hashes, Workspace revision and access
revision pinned on the run. Binding: ADR 0007, 0016, 0017, 0021, 0025 and
0033 and this spec's run, Run Key, checkpoint, state and retry decisions.
Leases, the scheduler and restart recovery are sub-stage 4's.

### What was built

- **Run start.** `Runtime.StartRun` takes a Workspace, a workflow name,
  inputs (JSON values, or command-line text converted to each input's
  declared type) and an optional originating session, from the owner's CLI
  or web surface only. Before anything is written it reads the active
  version (`ActiveWorkflow`), reads that version's exact commit through
  procedural Git, validates it against the Workspace's current access, the
  catalog and the worker ceiling, and requires its executable hash and
  authority hash to equal the approval's (otherwise `ErrDefinitionMismatch`,
  "approve the definition again"); refuses step kinds this runtime cannot
  execute; accepts the input against the declared inputs (types, bounds,
  enums, defaults, required, nothing unknown or given twice; otherwise
  `ErrRunInput`); and derives the Run Key. Then one immediate SQLite
  transaction calls `authorizeWorkflowActivation` (latest version, not
  deactivated, not blocked: every block is re-derived at each start),
  requires the version and both hashes to be the ones just validated (a
  version approved in between is validated again, up to three times),
  returns the run already holding the key, checks the originating session
  belongs to the run's Workspace, and records the run with its first
  checkpoint.
- **Run record.** `workflow_runs`: stable ID, Workspace, workflow, the
  approved version with its commit and path, executable and authority
  hashes, the Workspace revision and access revision current when the run
  starts (ADR 0033: the run pins what it started under; the activation
  keeps its approval-time pins, and admission evaluated every later access
  revision), its Execution Composition (`{"runtime":
  "evie.workflow.runtime/1", "step_kinds": [...], "functions": [...]}`;
  capability providers join it in sub-stage 5), Run Key, accepted input
  (canonical JSON), state, outputs, error, origin surface (`cli` or `web`),
  optional session, and created, started, finished and updated times.
  Pinned fields are immutable, a terminal run is never rewritten, a started
  run cannot be queued again, and nothing is deleted (triggers).
- **Run Key.** `<workflow>:sha256:<digest>` over the canonical JSON of the
  `run_key` inputs, the simulation's own rule, so manual and (later)
  scheduled starts agree. A partial unique index on (Workspace, workflow,
  Run Key) over runs that are not failed or cancelled makes the database
  refuse a second active or succeeded run for one key across versions. A
  duplicate start returns the existing run (`existing: true`) and executes
  nothing; a failed or cancelled run leaves its key free, so the same
  logical execution can be started again after a failure.
- **Step kinds.** This runtime version executes `deterministic` and
  `branch` steps and the graph's end. A definition using any other kind
  (including a map item's kind) is refused at start with
  `UnsupportedStepsError` ("uses ai, capability, interrupt steps, not yet
  supported in this runtime version ...; no run was started") before
  anything is recorded, so no run is ever partly executed. Approval is not
  refused: the definition can be approved now and run once its kinds ship.
- **Execution and checkpoints.** The run executes in the caller's process,
  foreground, before `StartRun` returns. Checkpoint 0 is the accepted input
  with the first step. Before each node attempt one transaction records the
  attempt (step, kind, attempt number, the checkpoint it starts from and its
  resolved input) and moves the run to `running`; after it, one transaction
  records the normalized output (canonical JSON) and appends the next
  checkpoint (the step after it, `""` when the graph ended, and every step
  output so far), and when the graph ended records the outputs and
  `succeeded`. Another attempt cannot start before that: every write is a
  compare-and-set on the run's latest checkpoint, one attempt per run may be
  in flight (unique index), attempt numbers per checkpoint are unique, and
  finished attempts and checkpoints are immutable. A branch is recorded as
  an attempt too (input: the values its cases read; output: the chosen step
  and case; note: which case matched); like in simulation it does not count
  against `limits.max_attempts`. The loop reads the run's progress back from
  SQLite before every step, so it continues from the last checkpoint and
  holds nothing in memory between nodes. A web start does not stop when the
  browser disconnects.
- **States.** Run states are the spec's nine, explicit and non-overlapping
  in the schema: `queued`, `running`, `waiting_for_human`,
  `waiting_for_dependency`, `needs_reconciliation`, `cancelling`,
  `cancelled`, `failed`, `succeeded`. This sub-stage reaches `queued`
  (recorded, no node started), `running`, `failed` and `succeeded`. Node
  attempts are `running`, `succeeded` or `failed`.
- **Retries and limits.** A deterministic step retries within its Retry
  Policy (`retry.max_attempts`, default 1) from the same input checkpoint
  with the same recorded input, waiting `retry.backoff` in process between
  attempts; after the last attempt the run fails with "steps.<id> failed
  after N attempts: <error>". A function error, a panic and a result outside
  the function's declared type all fail the attempt. `limits.max_attempts`
  (all attempts of the run) and `limits.max_duration` (from the run's
  creation) are checked before each attempt and fail the run when reached.
- **Execution primitives.** `internal/workflow` exports what a run needs
  and nothing that stores or schedules: the step plan, step kinds and
  functions, limits, `AcceptInputs` (JSON and text), `ParseInputs`,
  `RunKey`, `StepInput` (refusing a reference to a step that has not run),
  `CallFunction`, `Branch`, `Outputs`, and canonical `DecodeValue` and
  `EncodeValue` (integers stay integers after a restart). The simulator now
  uses the same helpers, so a run and its dry run cannot disagree.
- **Function registry.** `split_evenly@1` (a whole-number total, such as
  cents, split into 1 to 1,000 parts that differ by at most one and add up
  exactly; the first `remainder` shares carry the extra unit) is the one new
  reviewed function, pure and bounded, so a deterministic example exists.
  `internal/workflow/testdata/split-bill` (split, branch on the remainder,
  one deterministic step per path) is the test example; its README says so.
- **Surfaces.** `evie workflows run <workspace-id> <name> [--input
  <name>=<value>]... [--input-json <file>] [--json]` prints the run with
  each node and its outputs (a duplicate says the key is held and shows that
  run; a failed run exits with an error after it is shown); `evie workflows
  runs [<workspace-id>] [--workflow <name>]` and `evie workflows inspect
  <run-id>`. `POST /api/workflows/start` (`workspace_id`, `name`, `inputs`
  object, optional `session_id`; the surface is always `web`), `POST
  /api/workflows/runs` and `POST /api/workflows/inspect` are management
  routes (loopback, JSON, exact bounded bodies). A Workflow runs section on
  the Workspace home starts an active workflow with a JSON inputs object,
  lists runs and inspects one (pins, nodes, outputs). No model-facing tool
  starts or inspects a run.

### Decisions and known limits

- ADR 0021's audited override to rerun a succeeded key is not built: a
  succeeded key cannot be run again. It needs an owner-only surface and a
  durable override record; schedules (sub-stage 11) are the natural place.
- A crash mid-run today leaves the run `running` (or `queued` before its
  first attempt) at its last checkpoint, with the interrupted attempt
  `running` and no output. Nothing resumes it, and its Run Key stays held,
  so a duplicate start returns that stuck run until sub-stage 4 recovers it.
  No completed node is lost and no checkpoint is half written.
- `StartRun` blocks until the run ends; there is no background execution,
  cancellation (sub-stage 8) or later-session attachment yet.
- Checkpoints hold every step output so far, which keeps resumption simple;
  their size grows with each step, to be revisited for large map outputs in
  sub-stage 9.
- Accepted input, including inputs a definition marks `sensitive`, is stored
  and shown on the owner's inspect surfaces as given; redaction and
  retention are sub-stage 12's.

### What sub-stage 4 must pick up

- Fenced run leases on `BeginWorkflowAttempt`, `FinishWorkflowAttempt` and
  `FailWorkflowRun` (each is already a compare-and-set on the latest
  checkpoint), a scheduler that executes recorded runs in the background,
  and a recovery scan over `queued` and `running` runs.
- An attempt left `running` by a crash needs a terminal record (the attempt
  state set allows `running`, `succeeded`, `failed`; adding `interrupted`
  needs a table rebuild, or record it `failed` with that reason) before a
  deterministic step retries from the same checkpoint within its policy.
- Before resuming, re-apply admission (`authorizeWorkflowActivation`:
  revocations apply immediately, ADR 0033), re-read and re-hash the pinned
  commit (`approvedDefinition`), and require the run's pinned
  `composition.runtime` to be one this runtime declares compatible (ADR
  0025); otherwise pause the run visibly instead of continuing.
- `Runtime.advance` already continues from the latest checkpoint and
  refuses a run with an attempt in flight (`ErrRunInterrupted`); recovery
  resolves that attempt first. Backoff should become a durable not-before
  time rather than an in-process wait.

## 2026-10-04 — Activation and Standing Authority (Stage 14, sub-stage 2)

Source: sub-stage 2 of Stage 14 in
[docs/memory-architecture.md](../../../../docs/memory-architecture.md) and
the sub-stage 1 entry below, which left this sub-stage the procedural Git
source, binding to a Workspace with its real Workspace Access, activation of
an exact hash with its requested authority, the worker allowance check and
the diffs between revisions. Binding: ADR 0007, 0008, 0011, 0012, 0016,
0020, 0024, 0025, 0027 and 0033 and this spec's approval decisions. No run
starts in this sub-stage.

### What was built

- **Procedural repository.** `internal/procedural` keeps reviewed
  procedural assets in a private bare Git repository at
  `~/.evie/procedural` (created on first use, mode 0700, branch `master`,
  identity `Evie <evie@localhost>`). That path was already on the file
  tools' denylist, and all of `~/.evie` is outside every explore root and
  sandboxed shell, so no worker can read or write it. Evie writes it only
  with plumbing (`hash-object`, `mktree`, `commit-tree`, then a
  compare-and-swap `update-ref` of master retried on top of a concurrent
  writer's commit): there is no work tree to leave dirty, and a crash
  before the ref moves leaves only unreachable objects. Every command runs
  with a built environment (no inherited `GIT_DIR`, index, global or system
  configuration), hooks off and no transport. A root that is a link, a
  file, or a folder holding anything but this repository is refused; paths
  are clean, relative and never name `.git`. A Workflow Definition lives at
  `workspaces/<workspace-id>/workflows/<name>/` with exactly its reviewed
  files: `workflow.yaml`, `README.md` and the prompts and schemas it
  references (nothing else in the source folder is copied).
- **Git source.** `procedural.Source` implements sub-stage 1's `Source` at
  one exact commit: a file must be a regular blob of at most 1 MiB, and a
  later commit never changes what an earlier commit reads.
- **Proposal.** `evie workflows propose <workspace-id> <dir> [--fixtures
  <file>]` validates the folder against the Workspace's current Workspace
  Access, the runtime's capability catalog and the operator's worker
  ceiling, simulates it with the fixtures (fixture problems refuse before
  anything is written), commits its files, validates the commit again (the
  hash must match), and records the proposal in SQLite with its review
  report and dry run. An invalid definition is not proposed. The proposal
  records the Workspace revision and access revision it was made under.
  Only the owner proposes in this sub-stage (CLI); a model-facing proposal
  tool is deferred, and the store already accepts `evie` as proposer and
  `chat` as surface for it.
- **Workspace Access.** Workspaces had no access record, so this sub-stage
  adds the smallest one the spec defines: per Workspace, Connection IDs each
  with exact resource selectors (opaque, no spaces or controls, sorted, no
  duplicates), stored as immutable revisions in
  `workspace_access_revisions`, written with a compare-and-set on the
  expected revision by the owner only (`evie workspaces access
  <workspace-id> --expect <n> --set <connection>=<resource>,... | --clear`;
  without flags it prints the current access). The latest revision applies
  at once. Access grants nothing by itself: no Account Connection registry
  exists yet, so an ID is only a ceiling entry.
- **Approval.** `evie workflows approve <proposal-id> --hash <executable>
  --authority <authority> --expect-version <n>` and `POST
  /api/workflows/approve` approve exactly what a card showed: the
  proposal's executable hash, its authority hash (`sha256` over a canonical
  document of the requested operations, order-insensitive), and the version
  the review compared with (0 for none). The runtime re-reads the
  proposal's exact commit and re-validates it against the current catalog,
  access and ceiling (the hash must be unchanged); then one SQLite
  transaction requires the hashes, the reviewed version (a stale review is
  refused), a dry run of that proposal that **succeeded** (ADR 0011's
  "completing a dry run"; failed, waiting and missing dry runs refuse), an
  active Workspace, the Workspace's current allowance for every worker
  preset its agent steps run (the same check chat delegation uses), and
  requested authority within the latest Workspace Access. It records version
  N+1 with the commit, both hashes, the authority, workers and schedule,
  the Workspace revision and access revision it was checked against, and
  the approver (`owner`, surface `cli` or `web`, a detail line and time).
  Approval starts no run; a declared schedule is recorded with it and
  starts runs once schedules ship (sub-stage 11).
- **Versions and history.** `workflow_proposals`, `workflow_activations`
  and `workflow_deactivations` are immutable (triggers refuse update and
  delete). A later approval supersedes the earlier version; re-approving an
  older proposal is how a rollback is done. The database itself refuses an
  approver other than the owner and a surface other than the CLI or web.
- **README-only changes.** A proposal whose executable hash equals the
  active version's is recorded as documentation of that version
  (`documents_version`) and needs no approval; the active version is
  unchanged (runs read its approved commit).
- **Blocking.** A definition's status is derived on every read and at run
  admission from immutable history: `proposed`, `active`, `inactive`
  (deactivated) or `blocked` with every reason. It is blocked while its
  Workspace is archived, while the Workspace's worker allowance no longer
  covers its agent steps (pinned and current revision and every revocation
  of the pinned one, exactly as for chats, so turning a worker off and on
  again does not restore it), or once any access revision after the pinned
  one failed to cover its authority (the effective access is the
  intersection of the pinned revision and every later one, so restoring
  access does not restore it). Only a new approval clears a block. Nothing
  is stored as blocked, so no path that changes a Workspace can miss it.
  `ActiveWorkflow` (and `authorizeWorkflowActivation` inside a transaction)
  is the run-admission check sub-stage 3 calls.
- **Deactivation.** `evie workflows deactivate <workspace-id> <name>
  --version <n> [--reason]` and `POST /api/workflows/deactivate` end the
  current version (compare-and-set on its number); it stays in history.
- **Diffs and the card.** `evie workflows diff <proposal-id>` and `POST
  /api/workflows/diff` show the approval card: both hashes, the current
  version and its status, the executable diff by report section (graph,
  inputs, outputs, run key, contracts, workers, functions, resources,
  retries, model policies, schedule, retention, limits, prompt and schema
  digests), the authority diff (operations added, removed, or changed in
  resources or call bounds), whether documentation changed, the requested
  authority, workers and schedule (highlighted: approving activates it),
  validation warnings, the dry run's outcome, every reason approval would
  be refused now, the file patch against the current version's commit, and
  the exact approve command. `list` and `show` give status, block reasons,
  versions and proposals.
- **Surfaces.** CLI as above; management routes `POST
  /api/workflows/{list,show,diff,approve,deactivate}` behind the usual
  loopback and JSON guard, with exact, bounded request bodies (an approval
  cannot name its approver or surface). The Workspace home has a Workflows
  section listing definitions with status and block reasons, and an
  approval card that approves exactly what it shows. There is no web
  proposal route, so the browser never names a folder to read. No
  model-facing tool proposes, approves or deactivates a workflow.

### Decisions and known limits

- Workspace Access lives in SQLite revisions, like the Workspace's
  presets, worker settings, folder and merge commands today, not yet as a
  reviewed procedural asset in Git as `workspaces.spec.md` intends for
  Workspace Revisions. Moving Workspace configuration into procedural Git
  is a Workspace slice of its own.
- Git first, then SQLite, with no `procedural_operations` journal: a crash
  between them leaves a commit no proposal names, which is harmless history
  and never approvable. Approvals and blocks are in SQLite only.
- The production capability catalog is still empty (sub-stage 5), so only
  definitions without capability steps can be proposed and approved today;
  Workspace Access matters once providers declare contracts.
- The conversational `bash` tool stays the documented privileged bypass: it
  could run the owner's CLI or call the loopback API, as for every other
  management surface. Approval still needs the exact hashes of a recorded
  proposal and a dry run that succeeded.
- Approval re-validates against the current worker ceiling; lowering the
  operator's ceiling later does not block an approved version (sub-stage 9
  checks budgets at admission).
- Sub-stage 3 must start runs only through `authorizeWorkflowActivation`
  in its run transaction, read the definition from the activation's commit
  through `procedural.Source`, re-hash it, and refuse a mismatch.

## 2026-10-04 — Map and agent steps; definition validation and simulation (Stage 14, sub-stage 1)

Source: the owner's decisions recorded under Stage 14 of
[docs/memory-architecture.md](../../../../docs/memory-architecture.md)
("Decided (David, 2026-10-04)"): amend the spec with a bounded parallel map
step and agent steps that run the existing workers, keep the no-tools rule
for plain AI steps, keep external effects in the effect protocol, and parse
`workflow.yaml` strictly with `gopkg.in/yaml.v3`. Sub-stage 1 is the
amendment plus definition validation and simulation.

Superseded wording in [workflow-runtime.spec.md](workflow-runtime.spec.md):
"V1 graph execution supports five node kinds" (now seven), "parallel
branches" in the deferred list and the out-of-scope "Parallel graph
branches" insofar as they would cover the bounded map step, the out-of-scope
"Letting AI Nodes call tools" insofar as it would cover agent steps (which
are not AI Nodes), and the ten-item delivery list (now the twelve
sub-stages). Every ADR stays binding as written; in particular ADR 0010 and
0019 still describe AI Nodes, which keep no tools.

### Amendment

- **Map step.** `over` names a list in an earlier step's output (never a run
  input); `item` is one deterministic, AI or agent step with no `id` or
  `next`; `max_items` (1 to 1,000), `max_parallel` (at most the run's
  `limits.max_parallel`), `on_item_failure` (`fail` or `partial` with
  `min_succeeded`). A longer list fails the step before any item starts.
  Output: `results` (succeeded items in input order, each `index`, `item`,
  `output`), `failures` (`index`, `item`, `error`), `succeeded`, `failed`.
  Nesting stops at one level: no map, branch, interruption or capability
  item. Capability items are left out of v1 because effects inside a fan-out
  would need per-item Resource Conflict Key serialization that sub-stage 6
  has not built; reads could be added later.
- **Agent step.** `worker` is `research`, `explore` or `build` (the existing
  immutable presets, no other and none chosen by a model); `objective` is a
  prompt file; `input` fields are assignment data; `model_policy` names a
  Model Policy, because a run has no parent session whose model a chat child
  would reuse, and ADR 0019's pinning applies to every model a run uses;
  `budget` (`time`, `tokens`) is required and at most the operator's
  subagent policy (`EVIE_SUBAGENTS_DEADLINE`, `EVIE_SUBAGENTS_TOKEN_BUDGET`;
  defaults 15 minutes and 1,000,000 tokens) and the run's limits. A build
  step must pass `task_id`. `output_schema` is optional; with it the worker
  is asked to end its report with a `## Result` section holding one JSON
  object, which is extracted (the last fenced `json` block in that section)
  and validated like AI output. `succeeded` and `partial` attempts succeed;
  an attempt whose typed result is missing or invalid fails and retries by
  its policy.
- **Fences.** Agent steps reuse the worker's own fences, admission checks
  and recovery. The Workflow Run's fenced lease takes the parent turn's
  place; sub-stage 9 builds that admission path, because the supervisor
  today admits children only under a live parent turn.
- **No merge in v1.** A workflow cannot merge a build branch: there is no
  merge step, and validation refuses any `subagents.*` capability node with
  that reason. Build branches stay for the owner's Stage 12 gate. Allowing
  it later needs an amendment declaring the exact repository and target as
  approved Standing Authority. Unattended merge would be the first workflow
  effect on the owner's own checkout, so it waits for the effect protocol
  and an explicit decision rather than riding on this amendment.
- **Authority.** Worker presets are not Standing Authority (ADR 0027: a
  preset exposes, it does not authorize); the run's Workspace allowance for
  each worker is checked at admission exactly as for chat children. Requested
  Standing Authority lists capability operations only.

### Sub-stage 1: what was built

- **Package.** `internal/workflow` owns the Definition model, strict YAML
  loading, validation, the review report, canonical hashing, the reviewed
  deterministic function registry and effect-suppressed simulation. It
  imports no other Evie package, no network and no process execution; a test
  enforces that. Definitions are read through a `Source` (a directory today;
  the procedural Git tree at a reviewed commit is sub-stage 2's seam).
- **Accepted YAML subset.** One document, at most 256 KiB. Refused: anchors,
  aliases, merge keys, explicit tags (`!!str`, `!x`), duplicate keys, null
  values (empty, `~`, `null`), non-text mapping keys, and plain scalars that
  YAML 1.1 and 1.2 read differently or that hide a conversion: `yes`, `no`,
  `on`, `off`, `y`, `n` in any case, `True`/`FALSE` (only `true` and `false`),
  dates, hexadecimal, octal, exponents, infinities, `+1`, `1_000`, and
  decimals other than digits with one point. Unknown fields are refused by
  `KnownFields`, then every scalar is checked against the field's type: a text
  field accepts only text (quote `"1.10"`), an integer field only a plain
  decimal integer, a number field an integer or plain decimal, a boolean only
  `true` or `false`. Fields that belong to another step kind are refused.
- **Definition.** `schema_version` (1), `name`, `description`, typed
  `inputs` (`string`, `integer`, `number`, `boolean`; `required` or a
  `default`; `enum`, `minimum`, `maximum`, `max_length`, `sensitive`),
  `run_key.inputs`, `model_policies` (`version`, `provider: openrouter`, an
  exact `model` not ending in `latest`, `temperature`, `max_output_tokens`),
  `limits` (`max_duration`, `max_attempts`, `max_parallel`, `max_tokens`, all
  required), `authority.operations` (`capability`, `connection`,
  `resources`, `max_calls_per_run`), optional `schedule` (five-field cron
  without names, IANA `timezone`, `catch_up` `skip` or `each_missed` with
  `max_runs` up to 31, `inputs` from literals or `$schedule.date` /
  `$schedule.time`), `retention.classification` (`ordinary` or
  `sensitive`), `start`, `steps` and `outputs`. Steps: `deterministic`
  (`function: name@version`), `capability` (`capability`, `contract`,
  `connection`, `resource`), `ai` (`model_policy`, `prompt`,
  `output_schema`), `agent`, `map`, `branch` (ordered `cases` of `{ref, op,
  value}` with `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `empty`, `not_empty`, and
  a required `default`) and `interrupt` (`message`, `response_schema`,
  `input`). Deterministic, capability, AI and agent steps and map items have
  optional `retry` (`max_attempts` 1 to 5, `backoff` only with retries); a
  step without `next` ends the run, and `start` names the first step.
- **References.** `$inputs.<name>…`, `$steps.<id>.output…` and, inside a map
  item, `$item…`, with dotted field names and no indexing. A literal text
  value cannot start with `$`. A step may read only steps that run before it
  on every path (dominators), so a branch cannot leave an input missing;
  outputs may name any step and the report says whether each is always
  produced. Field paths are checked against JSON schemas, function result
  types and capability contracts; function arguments and capability inputs
  must be assignable (a required field must be required by the source).
- **JSON schemas.** A closed subset: `type` (one of the six), `properties`,
  `required`, `items`, `enum`, `minimum`, `maximum`, `minLength`,
  `maxLength`, `minItems`, `maxItems`, `additionalProperties` (must be
  `false` on every object), `title`, `description`. Anything else (`$ref`,
  `oneOf`, `pattern`, `format`…) is refused. AI, agent result and
  interruption response schemas have an object root.
- **Validation rules.** Unknown fields, unsupported `schema_version`,
  a missing `README.md`, prompt (`.md`) and schema (`.json`)
  files (relative, inside the definition, read through `os.Root`), unknown
  or duplicate step IDs, unknown `next`/case targets, unreachable steps,
  any graph cycle (v1 allows none; retries are per step), missing or
  out-of-range limits, branches without cases or `default`, conditions
  whose operand or value type does not fit (or a value outside an enum),
  Run Key rules (empty, unknown, repeated, optional without default, a
  `number` input, more than eight inputs, or a schedule whose run key no
  `$schedule` input feeds), unknown capabilities or contract versions,
  connections and resources outside Workspace Access, capability steps not
  covered by requested authority, requested operations, connections or
  resources no step uses, more calls than `max_calls_per_run`, retried
  effects without proven idempotency, unknown model policies or worker
  presets, agent budgets above the worker ceiling or the run's limits, map
  bounds (including a list whose declared `maxItems` exceeds `max_items`),
  deterministic functions not in the registry, and literal arguments outside
  a function's bounds. Unused model policies and inputs, and map lists whose
  type does not bound their length, are warnings, as are worst-case agent
  tokens and attempts above the run's limits. Every problem names its path
  (`steps.research.max_parallel`) and YAML problems their line.
- **Function registry.** `collect_claims@1` (deduplicates claims from map
  results in order, keeps only sources the worker actually fetched, caps at
  `max_claims` and reports what it dropped), `expand_ballots@1` (one ballot
  per claim and voter, 1 to 7 votes) and `majority_filter@1` (accepts a
  claim with at least `quorum` `supported` votes; missing votes count
  against). They are pure, typed, versioned, and their result types are
  refined from literal arguments so list bounds are checked statically. A
  change to a function's behavior ships as a new version, so the hash of
  every definition that names it changes.
- **Executable hash.** `sha256` over a canonical JSON document: the
  normalized definition (descriptions left out, defaults applied, durations
  in milliseconds, steps, inputs, policies and operations sorted), each
  prompt's exact bytes' digest, each schema's canonical JSON digest, and
  each referenced function's `name@version` with its signature digest.
  `README.md`, comments, key order, quoting and formatting are excluded.
- **Report.** Graph in topological order with each edge, inputs, outputs
  and whether each is always produced, Run Key rule, required Capability
  Contracts, workers, deterministic functions, resources, retry policies,
  model policies, requested Standing Authority, schedule, retention
  classification and sensitive inputs, limits, worst-case bounds, file
  digests (README marked as outside the hash) and warnings. Text by default,
  JSON with `--json`.
- **Simulation.** A fixture file (strict JSON) supplies run inputs and,
  per step, AI outputs, agent worker results (`status`, `summary`,
  `sources`, `result`, `tokens`), capability read results, effect responses
  and interruption responses, with `attempts` for retries and, for maps,
  `items` by index and `item_default`. Deterministic steps run their real
  code and cannot be replaced by fixtures. AI output and agent results are
  validated as the runtime will (an agent result travels through the same
  `## Result` extraction), effects become previews and are never executed
  (their fixture gives only a simulated response; effect failures and
  reconciliation are simulated once sub-stage 6 builds the protocol),
  maps run their items in index order with the failure policy, branches
  evaluate their cases, and a missing interruption response stops the trace
  `waiting_for_human`. Run limits on attempts and tokens apply. The same
  definition and fixtures give a byte-identical trace. Nothing is written,
  no receipt exists and no authority is granted.
- **CLI.** `evie workflows validate <dir> [--json]` and `evie workflows
  simulate <dir> --fixtures <file> [--json]`, dispatched before the
  database opens. Until sub-stage 2 binds a definition to a Workspace and
  sub-stage 5 lets providers declare workflow contracts, the CLI validates
  against an empty capability catalog and an empty Workspace Access, so a
  definition with capability nodes is reported as unauthorized there; the
  worker ceiling is the operator's subagent policy.
- **Example.** `internal/workflow/testdata/deep-research` (plan angles, map
  research workers, collect claims, expand ballots, map verify workers with
  three votes per claim, majority filter, synthesize) with its prompts,
  schemas, README and fixtures; `testdata/notes-digest` covers capability
  reads and effects, branches, interruptions and schedules against a test
  catalog. Sub-stage 10 ships the deep-research workflow for real.

### Known limits

- Agent inputs are assignment data, and a research worker can fetch any URL,
  so data a workflow passes to a research step can leave through a fetched
  URL, as in chat delegation. Validation does not track sensitive data flow
  into agent steps; the report shows every step's inputs for review.
- Worker preset versions are not in the executable hash; like capability
  providers they are pinned by the run's Execution Composition (ADR 0016,
  0025).
- Resource selectors match exactly; no wildcard selectors.
- The report's worst-case token figure counts agent budgets only; AI step
  input tokens are unbounded by declaration and are capped by
  `limits.max_tokens` at run time.
