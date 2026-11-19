# Evie 2

Evie is a personal agent you run in a terminal. You type a request, and it
reads, searches and changes files, runs commands and looks things up on the
internet until the request is done.

It is small on purpose. A fixed core runs the conversation, and everything
else, including every tool, is a plugin you can switch off, replace or add
to. The whole thing is a few thousand lines of TypeScript, meant to be read.

Evie is being built towards an always-on assistant with a server, a web
client and memory. This repository is the first milestone of that: the core,
the plugin system and a terminal client. It is a rewrite from scratch of an
earlier version.

Motto: simplicity is the most powerful foundation.

## Before you use it

**There is no approval step yet.** A tool runs as soon as the model asks
for it, including any shell command, with your permissions, and it is not
confined to the project folder. Use Evie only in a project you can restore
from git, and watch what it runs.

A built-in guard blocks the most clearly destructive commands, such as
deleting the home folder, `git reset --hard`, a forced push, `sudo`, and
piping a download into a shell. It also keeps the tools out of Evie's data
folder and `~/.ssh`, `~/.aws` and `~/.gnupg`. **The guard does not make
Evie safe to run unattended.** It matches the text of a command, so it stops
the obvious accident and nothing more. Rules that allow, deny or ask before
a risky action are planned for the next milestone.

## Install

You need:

- [Bun](https://bun.sh), which runs Evie;
- [ripgrep](https://github.com/BurntSushi/ripgrep), which Evie uses to
  search files;
- an [OpenRouter](https://openrouter.ai) API key, which gives access to
  models from many companies through one account.

```sh
git clone https://github.com/davidadel66/evie-2.0.git
cd evie-2.0
bun install
bun link          # makes the `evie` command available
```

If `evie` is not found afterwards, add Bun's bin folder to your path. Bun
prints where that is; it is usually `~/.bun/bin`.

Then tell Evie your key and which model to use:

```sh
export OPENROUTER_API_KEY=...     # your OpenRouter key
export EVIE_MODEL=...             # a model name from openrouter.ai/models
```

Both can be kept in files so you do not have to export them each time. See
Settings.

## Use

```sh
cd /path/to/a/project
evie
```

Type a message and press Enter.

- **You can type while Evie works.** The line you are typing stays at the
  bottom, and Evie's output appears above it.
- **A message sent while Evie is working steers it.** It is taken in at the
  next step. If Evie is in the middle of writing a reply, that reply stops
  and a new one starts with your message taken into account.
- **Ctrl-C stops what Evie is doing** and keeps the conversation. With
  nothing running, Ctrl-C or Ctrl-D quits. If Evie does not stop, a second
  Ctrl-C quits at once.
- **`evie --continue`** carries on the last session of the project you are
  in, with its conversation.
- **`/model`** shows which model is in use. `/model` and a name switches to
  another in the middle of a session.

### What Evie can do

| Tool | What it does |
|---|---|
| `read` | Reads a file, or lists a folder |
| `write` | Creates a file, or replaces one |
| `edit` | Replaces a piece of text in a file |
| `patch` | Changes several files from one block of text |
| `grep` | Searches the contents of files |
| `glob` | Finds files by name |
| `shell` | Runs a command |
| `web_search` | Searches the internet |
| `web_fetch` | Reads a web page as text |

Models differ in which tools they handle well. Evie gives GPT models `patch`
and every other model `edit` and `write`, each with a system prompt written
for it.

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

A setting in the environment wins over the files, so `EVIE_MODEL=... evie`
points one run at another model. `EVIE_MAX_TOKENS` works the same way.

A project can have settings of its own, in `.evie/evie.json` in its folder.
See "Trusting a project" for when that file is used.

## Plugins

Everything outside Evie's core is a plugin: the tools, the model provider,
the per-model settings and the guard.

### Choosing which plugins load

`evie.json` in the data folder lists the plugins to load, in order. Without
a list, the six built-in ones load:

| Plugin | What it adds |
|---|---|
| `files` | `read`, `write`, `edit`, `patch`, `grep` and `glob` |
| `shell` | `shell` |
| `guard` | The check that refuses the most destructive commands |
| `models` | What each model family is given, and the `/model` command |
| `web` | `web_search` and `web_fetch` |
| `openrouter` | The connection to models through OpenRouter |

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

A built-in plugin is named. A plugin of your own is given by the path to
its file. Leaving a plugin out switches it off: without `shell`, the model
cannot run commands.

A project's own file, `.evie/evie.json` in its folder, adds plugins for that
project only and can switch off a global one there. It can also set that
project's model:

```json
{ "model": "another/model", "plugins": ["./tools/deploy.ts"], "disable": ["shell"] }
```

### Trusting a project

**A plugin runs with Evie's full permissions. Nothing sandboxes it, so load
only plugins you trust.**

For that reason a project's settings file is used only when you list the
project's folder under `"trust"` in the global file:

```json
{ "trust": ["/path/to/my/project"] }
```

Otherwise the project's file is ignored as a whole, and Evie tells you so.
That way opening someone else's repository can neither run their code nor
switch off a protection such as the guard.

### Writing a plugin

A plugin file exports an object with a name and a setup function. Evie
calls setup once for each project and passes it the *host*, which is the
only way a plugin adds anything.

```ts
export default {
  name: "greeter",
  setup(host) {
    host.tools.add({
      name: "greet",
      description: "Says hello to someone.",
      inputSchema: {
        type: "object",
        properties: { name: { type: "string", description: "Who to greet" } },
        required: ["name"],
      },
      run: async (input) => `Hello, ${input.name}.`,
    })
  },
}
```

To take options from the settings file, export a function that returns the
plugin:

```ts
export default (options) => ({
  name: "greeter",
  setup(host) {
    /* use options.greeting */
  },
})
```

Through the host a plugin can add to four lists:

| List | What an entry is |
|---|---|
| `host.tools` | Something the model can call. Its `run` returns the text the model reads. A problem the model can fix is returned as that text; `run` throws only for a bug |
| `host.providers` | A way to reach a model |
| `host.profiles` | What one family of models is given: a system prompt and its tools for changing files |
| `host.commands` | A command you type starting with a slash, such as `/model` |

It can also attach to three *hooks*, moments in the loop where a plugin's
code runs:

| Hook | When it runs | What it can do |
|---|---|---|
| `before_request` | After a request is built, before it is sent | Change the system prompt and the tools the model is offered |
| `before_tool` | After a tool call's input is checked, before the tool runs | Refuse the call with a reason |
| `after_tool` | After a tool has run | Change the result the model sees |

```ts
host.on("before_tool", (call) => {
  if (call.name === "shell" && String(call.input.command).includes("deploy")) {
    return { deny: "Deploying is not allowed from here." }
  }
})
```

Hooks run in the order their plugins are listed. Each is told the session,
its project and the conversation so far. `host.project` says which project
the plugin is being set up for.

The built-in plugins in `src/plugins/` use exactly this interface, and are
the best examples to read.

## How it works

- **The event log is the truth.** Everything that happens in a session is
  added to a log in a SQLite database and never changed. Before every
  request to the model, the conversation is rebuilt from that log. Evie
  keeps no conversation in memory, which is why resuming a session, or
  recovering from being stopped part way, needs nothing special.
- **One way in, one way out.** A client sends commands and subscribes to
  events. The terminal is a client like any other and uses nothing else, so
  a web or phone client can be added without changing the core.
- **A turn is a loop.** The model is asked, the tools it asks for are run,
  and it is asked again with the results, until it answers without asking
  for a tool.
- **Projects have ids.** A session belongs to a project, or to none. A
  project is a record with its own id, and its folder is only where its
  files are today.

The core is in `src/core/`, in about 1,300 lines. `types.ts` is the
place to start.

## Development

```sh
bun install
bun run check   # the type check and the tests
```

The tests drive Evie the way a client does, with a scripted stand-in for the
model, so they need no key and no network.

## What comes next

- Instruction files, compaction of long conversations, permissions, undo
  and subagents.
- An always-on server, a web client, workspaces, skills and MCP servers.
- Scheduled work, memory, voice and a phone app.

## Credits

Evie adapts proven pieces from other open-source harnesses. Each is
credited here, and its licence notice is kept with the code.

- [OpenCode](https://github.com/anomalyco/opencode) (MIT): the matching in
  the `edit` tool, in `src/plugins/files/edit.ts`.
- [OpenAI Codex](https://github.com/openai/codex) (Apache 2.0): the patch
  format and the way the `patch` tool finds lines, in
  `src/plugins/files/patch.ts`, rewritten from Rust. The licence and Codex's
  notice are in `LICENSES/`.

## Licence

Not chosen yet. Until a licence is added, the code is published to be read,
and no permission to use, copy or change it is granted.
