# F11 - Session task ledger

Status: candidate, unapproved. Priority: with agent profiles.

## Purpose

Give long development turns a small, visible statement of current work without
confusing that execution state with David's personal `todo` application.

## How OpenCode does it

OpenCode exposes a `todowrite` tool backed by session-scoped todo state. The
model submits the complete updated list; each item has content, status, and
priority. The UI can render progress, and agent permissions decide whether the
tool is available. The `general` subagent is denied todo writing so the parent
retains orchestration ownership.

Source: [`tool/todo.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/todo.ts).

## EVIE today

EVIE has a personal `todo_add`/`todo_list` domain tool. It is the wrong store for
ephemeral implementation steps: "inspect tests" and "run vet" should not become
personal tasks in `~/.todo`.

## Proposed EVIE adaptation

Create a session-owned execution ledger with a distinct name and storage:

- items: concise action, `pending | in_progress | completed | cancelled`, and
  priority;
- one `in_progress` item at a time;
- full-list replacement validated transactionally;
- persisted as session events/state so resume reconstructs it;
- primary agent owns updates; children can report status but cannot rewrite the
  parent's ledger;
- UI distinguishes it clearly from David's personal todos.

The ledger is coordination, not proof. Marking "tests pass" complete does not
replace a recorded successful verification event.

## Acceptance evidence

- A non-trivial build shows its active step in REPL/web.
- Resume restores the ledger exactly.
- Child agents cannot overwrite parent tasks.
- Invalid transitions or two active items fail loudly.
- Finishing a task without evidence cannot fabricate a verification result.

## Open decisions

1. Keep full-list replacement or add item-level operations?
2. At what task complexity should EVIE create a ledger automatically?
