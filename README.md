# Evie 2

Evie is one person's always-on personal agent: a coding harness, a
researcher and a general assistant. This repository is a rewrite from
scratch of an earlier version.

Motto: simplicity is the most powerful foundation.

## Status

Milestone 1 is being built one ticket at a time, tracked in this
repository's issues. So far you can run `evie` in a terminal and have a
model read, search, create and change files and run commands in the folder
you started it in.

**There is no approval step yet.** A tool runs as soon as the model asks
for it, including any shell command, with your permissions, and it is not
confined to the project folder. Use Evie only in a project you can restore
from git, and watch what it runs.

Searching needs [ripgrep](https://github.com/BurntSushi/ripgrep).

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

Type a message and press Enter. Ctrl-C stops what Evie is doing; with
nothing running, Ctrl-C or Ctrl-D quits. If Evie does not stop, a second
Ctrl-C quits at once.

Type `/model` to see which model is in use, or `/model` and a name to
switch to another in the middle of a session. Each family of models is
given the tools and the system prompt it works best with.

Evie keeps its state in `~/.evie`. Set `EVIE_HOME` to use another folder.

Settings can also be kept in a `.env` file in that folder, one `NAME=value`
per line, so they need not be exported in every shell:

```
OPENROUTER_API_KEY=...
EVIE_MODEL=...
```

A setting in the real environment wins over the file. Evie reads the API
key and never writes it anywhere.

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

- [OpenCode](https://github.com/anomalyco/opencode) (MIT): the matching in
  the `edit` tool, in `src/plugins/files/edit.ts`.
- [OpenAI Codex](https://github.com/openai/codex) (Apache 2.0): the patch
  format and the way the `patch` tool finds lines, in
  `src/plugins/files/patch.ts`, rewritten from Rust. The licence and Codex's
  notice are in `LICENSES/`.

## Licence

Not chosen yet.
