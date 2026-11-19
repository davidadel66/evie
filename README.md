# Evie 2

Evie is one person's always-on personal agent: a coding harness, a
researcher and a general assistant. This repository is a rewrite from
scratch of an earlier version.

Motto: simplicity is the most powerful foundation.

## Status

Milestone 1 is being built one ticket at a time, tracked in this
repository's issues. So far you can run `evie` in a terminal and have a
model read, search, create and change files and run commands in the folder
you started it in, and search the internet and read web pages.

**There is no approval step yet.** A tool runs as soon as the model asks
for it, including any shell command, with your permissions, and it is not
confined to the project folder. Use Evie only in a project you can restore
from git, and watch what it runs.

A built-in guard blocks the most clearly destructive commands, such as
deleting the home folder, `git reset --hard`, a forced push, `sudo`, and
piping a download into a shell. It also keeps the tools out of Evie's data
folder and `~/.ssh`, `~/.aws` and `~/.gnupg`. **The guard does not make
Evie safe to run unattended.** It matches the text of a command, so it stops
the obvious accident and nothing more.

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

Both can be kept in files instead, as described under Settings.

Type a message and press Enter. The line you are typing stays at the
bottom, and Evie's output appears above it, so you can type while it works.
A message you send while Evie is working steers it: it is taken in at the
next step, and if Evie is in the middle of writing a reply, that reply
stops and a new one starts with your message taken into account.

Ctrl-C stops what Evie is doing; with nothing running, Ctrl-C or Ctrl-D
quits. If Evie does not stop, a second Ctrl-C quits at once.

Run `evie --continue` to carry on the last session of the project you are
in, with its conversation.

Type `/model` to see which model is in use, or `/model` and a name to
switch to another in the middle of a session. Each family of models is
given the tools and the system prompt it works best with.

## Settings

Evie keeps its state in `~/.evie`. Set `EVIE_HOME` to use another folder.
Two files in that folder hold its settings.

**`.env` is for keys.** One `NAME=value` per line. Evie reads a key and
never writes it anywhere.

```
OPENROUTER_API_KEY=...
BRAVE_API_KEY=...
```

`BRAVE_API_KEY` is a [Brave Search](https://brave.com/search/api/) key. It
is optional: without it Evie can still read a web page it is given the
address of, but cannot search.

**`evie.json` is for everything else:** the model, how long a reply may be,
and which plugins load.

```json
{
  "model": "a-name-from/openrouter.ai-models",
  "maxTokens": 16000
}
```

`maxTokens` is the longest one reply may be. It is 16,000 unless you change
it. OpenRouter refuses a request whose limit your credits do not cover, so
lower it if you see that.

A project can have settings of its own, in `.evie/evie.json` in its folder,
which win over the global file for that project. See Plugins for when a
project's file is used.

A setting in the environment wins over both files, so `EVIE_MODEL=...  evie`
points one run at another model. `EVIE_MAX_TOKENS` works the same way.

Evie is not started in a project. A client opens a session, which belongs
to a project or to none, and one Evie serves every open session. A project
is a record with its own id; its folder is where its files are today.

## Plugins

Everything outside Evie's core is a plugin: the tools, the model provider,
the per-model settings and the guard. Which ones load is set in a file.

**For every project:** `evie.json` in the data folder (`~/.evie/evie.json`).
It lists the plugins to load, in order. Without the file, the six built-in
ones load: `files`, `shell`, `guard`, `models`, `web` and `openrouter`.

```json
{
  "plugins": [
    "files",
    "shell",
    "guard",
    "models",
    "web",
    "openrouter",
    { "use": "/path/to/my-plugin.ts", "options": { "greeting": "hi" } }
  ]
}
```

Leaving a plugin out switches it off: without `shell`, the model cannot run
commands.

**For one project:** `.evie/evie.json` in that project's folder. It adds
plugins for that project only, and can switch off a global one there. It
can also set that project's model:

```json
{ "model": "another/model", "plugins": ["./tools/deploy.ts"], "disable": ["shell"] }
```

**A plugin file** exports an object with a name and a setup function, or a
function that returns one from its options:

```ts
export default {
  name: "greeter",
  setup(host) {
    host.tools.add({
      name: "greet",
      description: "Says hello.",
      inputSchema: { type: "object" },
      run: async () => "hello",
    })
  },
}
```

Through the host a plugin can add tools, providers, model profiles and
commands, and attach to the hooks `before_request`, `before_tool` and
`after_tool`.

**A plugin runs with Evie's full permissions. Nothing sandboxes it, so
load only plugins you trust.** For that reason a project's settings file is
used only when you list the project's folder under `"trust"` in the global
file:

```json
{ "trust": ["/path/to/my/project"] }
```

Otherwise the project's file is ignored as a whole, and Evie tells you so.
That way opening someone else's repository can neither run their code nor
switch off a protection such as the guard.

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
