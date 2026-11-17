# Evie 2

Evie is one person's always-on personal agent: a coding harness, a
researcher and a general assistant. This repository is a rewrite from
scratch of an earlier version.

Motto: simplicity is the most powerful foundation.

## Status

Milestone 1 is being built one ticket at a time, tracked in this
repository's issues. So far you can run `evie` in a terminal and talk to a
model. It has no tools yet, so it cannot read or change files.

## Running it

Evie needs [Bun](https://bun.sh) and an [OpenRouter](https://openrouter.ai)
API key.

```sh
bun install
bun link                                  # makes the `evie` command available
export OPENROUTER_API_KEY=...             # your OpenRouter key
export EVIE_MODEL=...                     # a model name from openrouter.ai/models
cd /path/to/a/project
evie
```

Type a message and press Enter. Ctrl-C or Ctrl-D quits.

Evie keeps its state in `~/.evie`. Set `EVIE_HOME` to use another folder.
The API key is read from the environment and is never written anywhere.

Evie is not started in a project. A client opens a session, which belongs
to a project or to none, and one Evie serves every open session. A project
is a record with its own id; its folder is where its files are today.

## Development

```sh
bun install
bun run check   # the type check and the tests
```

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
