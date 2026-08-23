# Story 3 - Safe existing-session selection and resume

Status: proposed; dependency blocked by Story 2

## Outcome

Allow the REPL to explicitly select and resume an existing active global or
project session after restart while preserving immutable scope and durable turn
ownership.

## Sources

- [Epic 1](../README.md)
- [Scope and authority](../../../memory.spec.md#scope-and-authority)
- [Stage 1](../../../memory.spec.md#stage-1---session-identity-and-append-only-events)
- [Binding memory decisions](../../../memory.decisions.md)

## Depends on

- Story 2 - Lease-owned and fenced turns.

## Acceptance summary

- Startup offers an explicit new-session or existing-session choice.
- A matching launch directory may suggest project scope but never grants it
  silently.
- Resuming a session preserves its stored project ID and root snapshot even
  after a project relocation.
- Provider context rebuilds from ordered provider-neutral events.
- A currently held lease is reported as a conflict rather than starting a
  competing turn.
- Closed sessions are not resumed as active sessions.

## Verification summary

- Table-driven global/project/new/resume startup tests.
- Restart and ordered-history reconstruction tests.
- Project-relocation snapshot tests.
- Competing-process or competing-store lease-conflict tests.
- Manual REPL demonstration covering global and project restart resume.
- `go test ./cmd/evie ./internal/agent ./internal/eviedb ./internal/memory`
- `go test ./...`
- `go vet ./...`

## Proposed one-PR boundary

Add the minimum active-session listing/query seams and REPL startup flow needed
for explicit resume. Do not add web-session endpoints, compaction, semantic
memory, execution-outcome synthesis, or session branching.

## Readiness

`DEPENDENCY_BLOCKED` until Story 2 lands. Interactive refinement must also fix
the exact session-list presentation and selection behavior before this story can
be declared ready.
