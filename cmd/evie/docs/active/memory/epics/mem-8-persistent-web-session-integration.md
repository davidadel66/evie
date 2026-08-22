# MEM-8 - Persistent web-session integration

Status: approved; not started

## Outcome

Extend the durable session, immutable scope, lease ownership, history, memory
inspection, and candidate-review model to `evie serve` while preserving existing
origin and approval defenses.

## Specification references

- [Stage 8 - Web session integration](../../memory.spec.md#stage-8---web-session-integration)
- [Scope And Authority](../../memory.spec.md#scope-and-authority)
- [Explicit Memory Operations](../../memory.spec.md#explicit-memory-operations)
- [Active serve specification](../../serve.spec.md)
- [Binding serve decisions](../../serve.decisions.md)

## In scope

- Approved amendments to web session/project/recovery behavior.
- Persistent session and project APIs with lease conflict semantics.
- UI session/project selection, scope display, history loading, and reload resume.
- Memory inspection and candidate approval/rejection parity with the REPL.

## Out of scope

- Weakening origin, approval, or single-owner local-machine defenses.
- Research-topic workspaces.
- New semantic-memory behavior not already available to the REPL.
- Multi-user authentication.

## Stories

### MEM-8.R1 - Serve specification and decision amendments

- Outcome: Define web session/project endpoints, reload recovery, history
  loading, approval flow, concurrent-process ownership, and active-scope display
  before implementation.
- Depends on: MEM-7 and demonstrated REPL behavior for all surfaces being added.
- Acceptance summary: `serve.spec.md` and `serve.decisions.md` unambiguously cover
  create/list/select/resume, immutable scope, lease conflict/loss, reload,
  unknown execution, memory review, error responses, and preserved security
  defenses.
- Verification summary: Spec walkthrough against the REPL and memory invariants,
  route/state examples, affected-link checks, and `git diff --check`.
- Proposed PR boundary: Documentation and decisions only; no server or UI code.

### MEM-8.1 - Persistent session and project web APIs

- Outcome: Expose scope-bound session/project creation, selection, resume, and
  history through server APIs that honor durable turn ownership.
- Depends on: MEM-8.R1 and MEM-1.
- Acceptance summary: Requests cannot choose or mutate harness-owned scope after
  session creation; resume loads ordered durable history; lease conflicts/loss
  and unknown executions have the approved explicit responses; existing origin
  and approval middleware remains effective.
- Verification summary: HTTP route, origin, scope matrix, reload, lease race,
  unknown execution, cancellation, and approval-flow tests.
- Proposed PR boundary: Server/API contracts and tests; retain the current UI
  until MEM-8.2.

### MEM-8.2 - Web session chooser and reload-safe resume

- Outcome: Let the UI create or select a confirmed global/project session,
  display its immutable scope, load history, and resume across page/server
  reloads.
- Depends on: MEM-8.1.
- Acceptance summary: Cwd is never a browser authority; session scope is visible;
  project/global selection is explicit; reload restores the selected session and
  history; lease/unknown-execution states are actionable rather than hidden.
- Verification summary: Frontend state/component tests, browser demonstrations
  for global/project/reload/conflict cases, `npm run lint`, `npm run build`, and
  server tests.
- Proposed PR boundary: Session/scope/history UI only; no memory inspection or
  candidate review.

### MEM-8.3 - Web memory inspection and candidate review

- Outcome: Provide the same scoped memory inspection and candidate
  approval/rejection capabilities as the REPL.
- Depends on: MEM-8.2, MEM-5 read tools, and MEM-4 candidate review.
- Acceptance summary: Every read is scope-bound and redacted; every mutation uses
  the existing approval flow and revalidates candidate revision; stale/rejected
  states are visible; origin and remote-egress defenses remain unchanged.
- Verification summary: API/UI scope and approval matrices, stale-revision races,
  candidate lifecycle, redaction, origin defense, browser demonstrations,
  `npm run lint`, `npm run build`, and `go test ./...`.
- Proposed PR boundary: Memory/candidate endpoints and UI; no new compiler,
  retrieval, or policy behavior.

## Epic completion evidence

- The web UI resumes selected global and project sessions across reload/restart
  with visible immutable scope and the same lease/recovery semantics as the REPL.
- Memory inspection and candidate approvals are scope-safe and preserve existing
  origin/approval defenses.
- `go test ./...`, `go vet ./...`, `npm run lint`, and `npm run build` pass.

## Risks and open decisions

- MEM-8.R1 is mandatory because endpoint, recovery, approval, and ownership
  behavior changes the active serve contract.
- The specification still has an open question about how active project scope is
  selected and displayed; MEM-8.R1 must close it.
- Browser reload and server restart are distinct recovery cases and require
  separate acceptance evidence.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.

