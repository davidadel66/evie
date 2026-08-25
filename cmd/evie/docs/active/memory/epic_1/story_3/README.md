# Story 3 - Safe existing-session selection and resume

Status: delivered by issue #59 and merged in pull request #60 on 2026-08-24

## Outcome

Allow the REPL to explicitly create or resume an active global or project
session from one combined hierarchy after restart while preserving immutable
scope, durable turn ownership, ordered history, and deterministic session
titles.

## Sources

- [Epic 1](../README.md)
- [Scope and authority](../../../memory.spec.md#scope-and-authority)
- [Stage 1](../../../memory.spec.md#stage-1---session-identity-and-append-only-events)
- [Binding memory decisions](../../../memory.decisions.md)

## Depends on

- Story 2 - Lease-owned and fenced turns, delivered by issue #57 and merged in
  pull request #58 on 2026-08-24.

## Acceptance summary

- Startup shows unarchived registered projects and their active sessions first,
  archived projects only when they retain an active session, and Global last.
  Every existing/new/registration selection is explicit; cwd may annotate one
  unarchived exact match but never preselects it.
- Terminal-safe project labels include the current canonical root, duplicate
  names remain distinguishable, and relocated sessions show a differing stored
  root snapshot before selection.
- Closed sessions are excluded. Active sessions under archived projects remain
  resumable, but archived projects cannot create sessions or receive a cwd
  suggestion.
- Resuming preserves the stored project ID and root snapshot and rebuilds
  provider context from ordered provider-neutral events without reprinting the
  transcript.
- A nullable durable title is initialized from the first nonblank accepted root
  user event inside the turn-lease-fenced transaction. Existing sessions are
  backfilled through a concurrent-start-safe additive upgrade; title writes do
  not change `sessions.updated_at` and are not overwritten by later messages.
- Titles and chooser labels have deterministic terminal-safe rendering. Rename
  APIs, title history, and web title presentation/editing remain deferred.
- A held lease reports `Session busy; message not sent.` and an after-selection
  inactive session reports `Session unavailable; message not sent.` without
  starting or recording work; the selected REPL prompt remains available.
- Concurrent registration or project archival refreshes the chooser without
  silently granting scope or creating a session.

## Verification summary

- Table-driven global/project/new/resume startup tests.
- Duplicate-name, archived-project, concurrent-registration, stale-project,
  deterministic-ordering, and terminal-safe rendering tests.
- Restart and ordered-history reconstruction tests.
- Project-relocation snapshot tests.
- Fresh/legacy/concurrent title-upgrade tests plus blank, Unicode, control,
  truncation, rollback, stale-fence, timestamp-preservation, and no-overwrite
  cases.
- Competing-process or competing-store lease-conflict tests.
- Manual REPL demonstration covering global and project restart resume.
- `go test ./cmd/evie ./internal/agent ./internal/eviedb ./internal/memory`
- `go test -race ./cmd/evie ./internal/agent ./internal/eviedb`
- `go test ./...`
- `go vet ./...`
- `git diff --check`

## Proposed one-PR boundary

Add the minimum active-session listing/query seams, combined REPL startup/resume
flow, additive session-title persistence, common fenced title initialization,
friendly conflict presentation, planning records, and deterministic tests. Do
not add rename/update APIs, title history, web-session endpoints or UI,
project-lifecycle commands, compaction, semantic memory, execution-outcome
synthesis, provider usage, or session branching.

## Approved decisions

David approved the following on 2026-08-24; binding detail is in
`memory.decisions.md`:

1. One combined Codex-like project/session hierarchy with Global last.
2. Unambiguous terminal-safe project/root and relocation-snapshot labels.
3. Resume eligibility based on active session status independently of project
   archival, with no new sessions under archived projects.
4. Durable titles derived without a model from the first nonblank accepted user
   event; title editing is deferred.
5. Concurrent-start-safe legacy title backfill and atomic fenced initialization
   that preserves `sessions.updated_at`.
6. Newest-first deterministic session ordering and fail-closed refresh after
   stale chooser state.
7. No idle lease; concise busy/unavailable messages leave the selected prompt
   available without accepted work.

## Delivery

The reviewed contract was materialized as
[issue #59](https://github.com/davidadel66/evie/issues/59) and delivered by
[pull request #60](https://github.com/davidadel66/evie/pull/60), merged on
2026-08-24 as `5593518cf64abae9558d9dc91f21188dd5d66b3f`.
