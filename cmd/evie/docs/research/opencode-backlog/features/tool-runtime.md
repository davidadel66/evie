# F04 - Typed tool runtime

Status: candidate, unapproved. Priority: first.

## Purpose

Make every tool invocation typed, cancellable, attributable, bounded, and
persistable through one runtime boundary.

## How OpenCode does it

OpenCode's tool definition includes an ID, description, parameter schema,
execution context, structured result, metadata, optional attachments, and a
permission callback. A central wrapper decodes arguments, emits actionable
validation errors, tags traces with session/message/call identity, and truncates
large output. The execution context includes session, message, agent, abort
signal, history, metadata updates, and approval requests.

General output defaults to 2,000 lines or 50 KiB. Complete oversized output is
stored locally for seven days and the model gets a path plus instructions for
targeted inspection.

Sources:

- [`tool/tool.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/tool.ts)
- [`tool/truncate.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/truncate.ts)

## EVIE today

EVIE already has a useful `Tool`/`PreparedTool` split and fail-closed approval
gate. Prepared file edits bind previewed bytes to execution, which should be
preserved. Missing pieces are typed execution context, cancellation, stable call
identity beyond provider IDs, central output retention, structured metadata,
profile filtering, and durable status transitions.

## Proposed EVIE adaptation

Each invocation receives:

```text
session ID, turn/message ID, execution ID, provider call ID
profile name and immutable project context
context.Context for cancellation/deadline
effective permission evaluator and approver
metadata/progress event sink
```

The runtime, not each tool, owns:

- JSON argument validation and model-facing correction errors;
- permission evaluation before preparation and again before side effects;
- `pending -> running -> completed | error | declined | unknown` transitions;
- output byte/line limits and unique full-output storage;
- secret redaction before persistence/model return;
- timing and cancellation metadata.

Retained output lives under a user-only EVIE data directory, not a predictable
shared `/tmp` name. Parent directories are mode `0700`; files are created
uniquely with mode `0600` and without following a pre-existing symlink. Cleanup
never follows links outside that root. The result path itself is capability
scoped so a restricted child cannot browse another session's retained output.

Prepared mutations remain two phase: validate and build a preview without side
effects, obtain authority, then revalidate stale state immediately before
execution.

## Acceptance evidence

- Invalid arguments yield a corrective tool result rather than aborting a turn.
- Every result is correlated with one durable execution ID.
- Cancellation reaches a cooperative tool and records an interrupted state.
- Oversized outputs never collide and remain inspectable for a defined period.
- Retained-output directories/files are `0700`/`0600`, created symlink-safely,
  and inaccessible through another session's scoped tools.
- A permission change while approval is pending is re-evaluated before mutation.

## Open decisions

1. What output retention period and cleanup policy fit EVIE?
2. Which metadata is model-visible versus UI-only?
3. Can a tool stream progress before durable session events exist, or should
   progress wait for F08?
