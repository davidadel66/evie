# MEM-7 - Reviewed procedural Git memory

Status: approved; not started

## Outcome

Add explicitly initialized, reviewed, versioned instructions and workflows that
can influence Evie's behavior without mixing procedural authority with factual
semantic memory.

## Specification references

- [Procedural memory](../../memory.spec.md#procedural-memory)
- [Procedural Memory](../../memory.spec.md#procedural-memory-1)
- [Explicit Memory Operations](../../memory.spec.md#explicit-memory-operations)
- [Security Boundary](../../memory.spec.md#security-boundary)
- [Stage 7 - Procedural Git memory](../../memory.spec.md#stage-7---procedural-git-memory)
- [Binding memory decisions](../../memory.decisions.md)

## In scope

- Secure procedural-repository initialization and repository-local Git identity.
- Scope-bound required instruction loading and an on-demand project manifest.
- Procedural proposals, approvals, operation journaling, and marked Git commits.
- Crash recovery, cross-process locking, rollback, and quarantine.

## Out of scope

- Storing factual semantic claims in Git.
- Silent initialization or automatic activation of proposed procedures.
- Resetting or discarding dirty/divergent worktrees.
- Bypassing the remote-memory opt-in for model-facing procedural context.

## Stories

### MEM-7.1 - Secure procedural-repository initialization

- Outcome: Explicitly initialize `~/.evie/procedural` with restrictive modes and
  repository-local Git identity.
- Depends on: MEM-6 and existing generic file-tool path fencing.
- Acceptance summary: Initialization is opt-in and idempotent; root is `0700`,
  managed files are `0600`, identity is `Evie <evie@localhost>`, global Git
  configuration is untouched, and symlinked/non-regular roots or managed paths
  are rejected.
- Verification summary: Mode, identity, idempotency, symlink, non-regular path,
  global-config preservation, and generic-file-fence tests.
- Proposed PR boundary: Repository bootstrap and secure path abstraction only;
  no context loading or mutation tools.

### MEM-7.2 - Scoped procedural context loading

- Outcome: Mechanically load required global and active-project instructions
  under strict budgets while exposing other project files through an on-demand
  scoped manifest.
- Depends on: MEM-7.1, MEM-2's context composer, and MEM-5's remote-egress gate.
- Acceptance summary: Immutable session project ID selects content; missing,
  unreadable, quarantined, or over-budget required files fail visibly; approved
  content is a supplemental system-role block without changing the base prompt;
  opt-in off withholds model-facing procedural context.
- Verification summary: Global/project isolation, required-file failures,
  budgets, manifest, role placement, remote opt-in, restart, and symlink-race
  tests.
- Proposed PR boundary: Read-only loader, context integration, and manifest; no
  proposal or Git-write operation.

### MEM-7.3 - Procedural proposal and approval journal

- Outcome: Propose, approve, reject, and roll back procedural changes through
  typed operations backed by durable SQLite intent and marked Git commits.
- Depends on: MEM-7.1 and MEM-7.2.
- Acceptance summary: Every approved change records expected parent/content hash
  and operation state, commits under a cross-process lock with required trailers,
  preserves facts outside the tree, and activates only after explicit approval;
  generic file tools remain fenced.
- Verification summary: Approval/rejection, expected-parent conflict, commit
  trailers, content hashes, rollback, local identity, model tool approval, and
  semantic-tree exclusion tests.
- Proposed PR boundary: Proposal store and dedicated mutation tools through the
  successful commit path; crash reconciliation follows separately.

### MEM-7.4 - Procedural crash recovery and quarantine

- Outcome: Reconcile interrupted SQLite/Git operations without resetting or
  guessing about dirty, divergent, or malformed repositories.
- Depends on: MEM-7.3.
- Acceptance summary: Matching marker/hash commits finalize idempotently; absent
  markers retry only with expected parent and clean tree; expired process locks
  recover safely; dirty/divergent/malformed states quarantine the repository and
  prevent it from serving instructions until local resolution.
- Verification summary: Documented crash matrix, process-kill fixtures,
  cross-process locks, malformed trailers, dirty/divergent trees, permission
  drift, quarantine, and restart tests.
- Proposed PR boundary: Startup reconciliation, lock recovery, quarantine, and
  diagnostics; no background consolidation policy.

## Epic completion evidence

- An approved project workflow is versioned, project-scoped, loaded under budget,
  and rollbackable without changing semantic facts.
- Every documented crash point either reconciles idempotently or quarantines
  visibly without resetting user data.
- Restrictive modes, symlink rejection, generic file fences, and remote opt-in
  behavior pass.
- Focused tests, `go test ./...`, and `go vet ./...` pass.

## Risks and open decisions

- Git and SQLite cannot share a transaction; the journal/marker protocol and
  quarantine behavior are correctness requirements, not optional hardening.
- A quarantined repository intentionally fails requests that require its
  instructions rather than silently omitting governing context.
- Background procedural consolidation may propose changes but remains outside
  active behavior until explicitly approved.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.

