# F14 - Deterministic verification pipeline

Status: candidate, unapproved. Priority: early coding slice.

## Purpose

Define "done" with executable checks and evidence instead of relying on the
implementing model's confidence.

## OpenCode comparison

OpenCode provides the shell, todos, profiles, subagents, snapshots, and session
state needed to run checks, but it does not impose one universal software-build
workflow. That is correct: verification is project-specific.

EVIE's strongest design evidence comes from its own
`docs/multi-agent-dev-loop.md`: exact candidate identity, project-specific
deterministic checks, fresh independent review, structured evidence, bounded
repair, and explicit safe stops.

## Proposed EVIE adaptation

Each trusted project defines a verification contract, for example:

```text
format/check commands
build/type-check commands
test commands
optional lint/security commands
optional live-fire command or human step
timeouts and required/optional status
```

Commands run through F06 and are recorded verbatim with cwd, start/end time,
exit status, bounded display output, and full-output path. A build report states:

- project and source revision;
- task/spec content hash;
- files changed;
- each exact gate and result;
- highest verification tier cleared;
- checks skipped and why;
- known gaps accepted deliberately;
- reviewer verdicts labeled as model judgment.

The core pipeline can ship with ordinary project gates before worktree-based
blind tests exist. When the spec-derived blind-test tier is enabled, F16 and A09
become prerequisites and the anti-vacuity rule is mandatory:

```text
new tests against old tree -> must fail
new tests against new tree -> must pass
```

Do not hardcode `go build ./... && go vet ./... && go test ./...` as a universal
gate. EVIE itself also has frontend checks; other projects will differ.

## Acceptance evidence

- A failed required gate prevents completed status.
- Empty/no-op new tests cannot satisfy the blind-test tier.
- Exact outputs remain inspectable after truncation and restart.
- User-owned dirty changes are not attributed to the agent.
- Skipped checks and accepted gaps remain visible in the final report.

## Open decisions

1. Verification contract format and trusted location.
2. Are gates run after every mutating turn or only at stage boundaries?
3. Which checks can an attended user explicitly waive, and how is that recorded?
