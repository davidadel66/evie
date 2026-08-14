# F25 - Coding session interface

Status: candidate after durable sessions, unapproved.

## Purpose

Expose project, profile, tools, diffs, approvals, context, children, recovery,
and verification state without making the frontend own agent behavior.

## OpenCode comparison

OpenCode's TUI is a strong client over session events: it selects agents,
streams reasoning/text/tool states, resolves permission requests, browses
sessions, displays diffs/todos/context, and invokes revert/commands. Its useful
lesson is the event-driven client boundary, not that EVIE needs a TUI clone.

## Binding EVIE direction

EVIE already chose a web frontend for rich output. Keep that decision:

- REPL: dependable text interface, session/project/profile commands, concise
  tool and verification status.
- Web: persistent sidebar with `Global` plus registered projects, project-scoped
  session picker, streamed tool cards, diffs, approvals, task ledger, subagent
  tree, context/usage, snapshots/revert, and build report.

Both remain projections over F08 events and call the same session APIs. Browser
reload must resume from durable state rather than presenting an empty transcript
while the server remembers hidden history.

## Candidate controls

- register/archive/relocate projects through an explicit trust flow;
- select `Global` or one registered project before creating a conversation;
- select/resume sessions within that scope;
- visible `build`/`plan` profile switch with authority explanation;
- cancel turn and descendants;
- approve once/always/reject with matched resource/policy;
- inspect full tool output and changed-file diff;
- view task ledger, usage, context pressure, retries, and budgets;
- open child session transcript and final verdict;
- revert selected session steps;
- run/view verification contract and report.

The UI must not infer tool completion from a pretty card. It renders durable
execution state from the harness.

## Acceptance evidence

- REPL and web show the same session/profile/execution state.
- Launch cwd never silently selects or registers a project.
- Switching the picker creates/resumes another session; it cannot mutate the
  active session's immutable scope.
- Reload/restart restores transcript, pending recovery, and task ledger.
- Cancel is explicit and reaches the running turn rather than merely closing
  the HTTP connection.
- Approval remains visible after resolution.
- Unknown execution recovery cannot be hidden behind a normal composer.

## Open decisions

1. Exact project registration/picker API and trust confirmation copy.
2. Whether reasoning is shown live, hidden by default, or omitted from resumed
   history under the existing reasoning policy.
3. How much snapshot/revert complexity belongs in the first coding UI.
