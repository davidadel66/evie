# Evie 2

Evie is one person's always-on personal agent: a coding harness, a
researcher and a general assistant. This repository is a rewrite from
scratch of an earlier version.

Motto: simplicity is the most powerful foundation.

## Status

In design. Nothing is built yet. Milestone 1 is tracked in this
repository's issues.

## What milestone 1 will be

A small program, `evie`, run in a project folder from a terminal. You type
a request, and Evie reads, searches and changes files and runs commands
until the request is done.

- A fixed core: one way in (the inbox), one loop that runs turns, one way
  out (the event stream), and an add-only event log that is the truth.
- Everything else is a plugin, built in or your own: the tools, the model
  provider and the per-model settings.
- Seven tools: `read`, `write`, `edit`, `patch`, `grep`, `glob`, `shell`.
- Models through OpenRouter, chosen by name and switchable mid-session.
- TypeScript on Bun, with state in SQLite.

## Later

A full coding harness (instruction files, compaction, permissions, undo,
subagents), then an always-on server with a web UI and workspaces, then
scheduled work, memory, voice and a phone app.

## Credits

Evie adapts proven pieces from other open-source harnesses. Each is
credited here, and its licence notice kept with the code, as it is adapted.

## Licence

Not chosen yet.
