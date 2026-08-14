# F07 - Bounded turn lifecycle

Status: candidate, unapproved. Priority: first.

## Purpose

Turn EVIE's unbounded `for` loop into a cancellable state machine with explicit
retry, loop, budget, and completion semantics.

## How OpenCode does it

OpenCode has one runner per session, propagates cancellation to child jobs, and
normalizes stream events into persisted text/reasoning/tool/step state. Tool
parts transition through `pending`, `running`, `completed`, or `error`.

Three consecutive identical tool calls trigger a `doom_loop` permission request.
Transient provider errors receive at most five retries with two-second
exponential backoff, 25 percent jitter, and `Retry-After` support. Agent profiles
can define step limits.

Sources:

- [`session/run-state.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/run-state.ts)
- [`session/processor.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/processor.ts)
- [`session/retry.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/retry.ts)

## EVIE today

`Session.Send` serializes one turn with `TryLock`, but then loops until a model
returns no tools. It has no context cancellation, model-step/tool-call/token/
cost/wall-time budget, retry policy, repeated-call detector, or truncation finish
handling. A provider error leaves the appended user message in memory.

## Proposed EVIE adaptation

One turn has explicit states:

```text
admitted -> requesting -> streaming -> executing tools -> requesting ...
         -> completed | cancelled | failed | budget_exhausted | blocked
```

Bounds are profile/session configuration:

- maximum model steps;
- maximum tool calls;
- wall-clock deadline;
- input/output token or cost budget when available;
- repeated identical call threshold.

Retry only a request that is known to be side-effect free: no assistant output
has been committed and no tool has started. Never replay a provider turn after a
possibly successful side effect. Expose retry attempt and next retry time to the
frontend.

Cancellation flows from HTTP/REPL command through provider stream, approvals,
tools, shell processes, and descendants. Cleanup marks unfinished tool calls
interrupted rather than leaving them running forever.

## Acceptance evidence

- Every terminal state is distinguishable in history and UI.
- Repeated identical calls stop or ask rather than looping indefinitely.
- A transient pre-output 503 retries with bounded backoff.
- A failure after a side effect does not replay that side effect.
- Cancellation reaches provider, active tool, process, and child session.
- `length`/context finish reasons do not masquerade as complete answers.

## Open decisions

1. Default step/tool/time budgets for attended versus autonomous profiles.
2. Does a doom-loop approval permit one repeat or the named pattern for the
   session?
3. Should a failed request retain the user's admitted message for manual retry?
