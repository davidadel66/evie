# Evie

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

**Evie asks before it changes anything, but it is not a sandbox.** Tools
run with your permissions and are not confined to the project folder. What
stands between the model and your machine is a set of rules that allow,
deny or ask, described under "Permissions". The rules judge a shell command
by its text, so they make the obvious risk visible and can be fooled by a
command hidden in a script. Use Evie in a project you can restore from git.

Some things are refused in every mode: the most clearly destructive
commands, such as deleting the home folder, `git reset --hard`, a forced
push, `sudo`, and piping a download into a shell; and any tool reaching
into Evie's data folder or `~/.ssh`, `~/.aws` and `~/.gnupg`.

## Install

You need:

- [Bun](https://bun.sh), which runs Evie;
- [ripgrep](https://github.com/BurntSushi/ripgrep), which Evie uses to
  search files;
- an [OpenRouter](https://openrouter.ai) API key, which gives access to
  models from many companies through one account.

```sh
git clone https://github.com/davidadel66/evie.git
cd evie
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

Evie runs as a server on your computer, and the terminal is one of its
clients. Start the server and leave it running:

```sh
evie serve
```

On a Mac it can instead start by itself whenever you log in; see "Keeping
the server running".

Then, in another terminal, go to a project and start the client:

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
- **Closing the terminal stops nothing.** A turn still running carries on
  in the server, and a question still waiting for your answer keeps
  waiting: `evie --continue` shows it again. Stopping the server, with
  Ctrl-C where it runs, stops everything.
- **`evie --continue`** carries on the last session of the project you are
  in, with its conversation.
- **`/model`** shows which model is in use. `/model` and a name switches to
  another in the middle of a session.
- **`/mode`** shows how freely Evie may act, and `/mode auto`, `/mode manual`
  or `/mode read-only` changes it for the session. See "Permissions".
- **`/undo`** puts files back as they were before Evie's last turn that
  changed any. Run it again to go back a turn further. It covers what the
  `write`, `edit` and `patch` tools changed. It does not undo what a shell
  command changed, and it leaves alone any file you have changed since.
- **A long command does not hold Evie up.** A command still running after
  two minutes moves to the background and carries on, and Evie can start
  one there on purpose, such as a server. When it ends, Evie is told and
  picks the work up again by itself. `/jobs` lists what is running, and
  `/jobs stop` with an id stops one. `/background` moves the command Evie
  is waiting on right now to the background, so you need not wait for it. Ctrl-C leaves background commands
  running; they stop when the server stops.
- **Evie can hand work to subagents.** For a side task such as searching
  the code or reviewing a change, it can start a second agent with an empty
  conversation, which works in the background and reports back. Several
  can work at once. They show in `/jobs`, where you can stop one. A
  subagent works without asking before each step, so in `manual` mode Evie
  asks you before starting one.
- **`/compact`** replaces the older part of a long conversation with a
  summary, to save room. Evie also does this by itself when a conversation
  gets long.

### The server and other devices

`evie serve` listens on this computer only, at `http://127.0.0.1:3843`.
It reads the settings files when it starts, so a change to them takes
effect when the server is started again.

When the connection to the server drops, as when the laptop sleeps or the
server is started again, `evie` says so and connects again by itself, then
shows what it missed, without repeating anything. When it cannot catch up,
as when the project's plugins fail to load as the server opens the session
again, it says that some of what happened may be missing, and why. A
message or command it had sent when the line dropped is sent again, and
carried out once, however long the line was down, as long as the server
kept running: the server remembers the replies to the last 200 commands of
each device. Ctrl-C while it is trying stops it. When nobody is typing in
it, as when its input comes from a pipe or a script, it gives up after two
minutes of trying, says why, and exits with an error.

Every connection has to bring a token, a long random secret, even on this
computer: the server can run commands, and any web page open in a browser
can try to reach it. The first time the server starts, it makes a token
named "this computer" and saves it in `server-token` in the data folder,
where only your user can read it. `evie` on this computer uses it, with
nothing to set up, and sends it to no other server. Once taken back it
stays taken back; to make a new one, remove `server-token` and start
`evie serve` again.

Each other device gets a token of its own, so that one can be taken back
without the others:

```sh
evie token add phone      # prints the new token, once only
evie token list           # the tokens there are, and when each was made
evie token remove phone   # takes it back; a device using it is disconnected
```

These work whether or not the server is running. On the other device, set
`EVIE_SERVER` to the server's address, such as `http://my-mac.local:3843`
or just `my-mac.local`, and `EVIE_TOKEN` to its token. The server listens on this computer only
unless `host` under `server` in `evie.json` says otherwise, and the
connection is not encrypted, so only change that on a network you trust.

### Keeping the server running

On a Mac, the server can run as a login service, so that it is there
whenever you are logged in, and is started again if it stops:

```sh
evie service install     # starts `evie serve` now, and at every login
evie service status      # whether it is installed and running, and where its log is
evie service uninstall   # stops it, and no longer starts it at login
```

`install` writes a launch agent, `~/Library/LaunchAgents/com.evie.serve.plist`,
for the copy of Evie it is run from, and starts it. Run it again after
updating Evie: it stops the server that is running and starts it on the
new version. The service is given the data folder (`EVIE_HOME`, when it is
set) and the `PATH` of the terminal it was installed from, so that it finds
Bun and the commands Evie runs; install it again after changing either.
What the server prints goes to `serve.log` in the data folder.

The service does not keep the Mac awake. While it sleeps, work in progress
pauses, and `evie` catches up when it wakes. On other systems, run
`evie serve` yourself for now.

### What Evie can do

| Tool | What it does |
|---|---|
| `read` | Reads a file, or lists a folder |
| `write` | Creates a file, or replaces one |
| `edit` | Replaces a piece of text in a file |
| `patch` | Changes several files from one block of text |
| `grep` | Searches the contents of files |
| `glob` | Finds files by name |
| `shell` | Runs a command, in the background when it is a long one |
| `job_stop` | Stops something running in the background |
| `subagent` | Hands a piece of work to a second agent, which reports back |
| `subagent_send` | Sends a further message to a subagent |
| `skill` | Loads one of your skills |
| `web_search` | Searches the internet |
| `web_fetch` | Reads a web page as text |
| `ask` | Asks you a question with choices, and waits for your answer |

Models differ in which tools they handle well. Evie gives GPT models `patch`
and every other model `edit` and `write`, each with a system prompt written
for it.

## Instruction files

Write down how you want Evie to work, once, and it is told every time.

- **`~/.evie/AGENTS.md`** applies in every project: how you like answers,
  what to always or never do.
- **`AGENTS.md` in a project's folder** applies in that project: its
  conventions, how to run its tests, what not to touch. If there is no
  `AGENTS.md`, a `CLAUDE.md` is used instead.

Both are plain text. They are added to what the model is told, yours first
and then the project's. An edit takes effect at the next message. A file
longer than 32 KiB is cut.

A project's instruction file is read whether or not you have trusted the
project, so look at it before running Evie in a repository you did not
write.

**Giving Evie a character.** The built-in prompt says how to work and how to
communicate, and is neutral about personality on purpose. `~/.evie/AGENTS.md`
is where you say who Evie is to you: its manner, how much it should push
back, when it should stop and show you options, how you like to be taught.
The built-in prompt is in `src/plugins/models/prompts.ts`, if you want to
see what yours is added to.

Evie is told, at the top of its prompt, which model it is running on and
when the session started. Those facts are fixed for the session, so a
session left open for days still says when it began; Evie is told to check
the clock when the present time matters.

## Agents

An agent is a role you define, with its own instructions. Evie can start a
subagent as one: a reviewer that only reads, a planner, a tester.

An agent is a Markdown file. A short header says what it is, and the rest
is its instructions:

```
---
name: reviewer
description: Reviews a change for bugs. Give it the diff.
mode: read-only
model: some/model
---
You review code changes. Read the diff and the files it touches.
Report bugs, most serious first, each with the file and line.
```

- `description` is what Evie reads to decide when to use the agent.
- `mode: read-only` holds the agent to reading. Leave it out, or write
  `mode: auto`, and the agent works as any subagent does. Any other value,
  a misspelling included, is taken as read-only, so a slip never gives an
  agent more than you meant.
- `model` is optional. Without it the agent uses the model you are using.
- `name` is optional too: without it the file's name is used.

Put the file in **`~/.evie/agents/`** to have the agent in every project,
or in **`.evie/agents/`** in a project's folder for that project. A
project's agents are used only when you have trusted the project, and one
with the same name as yours replaces yours there. It cannot lift a
read-only limit that yours has.

An agent can be used as soon as its file is saved. You can ask Evie to
write one. For one in `~/.evie/agents/`, Evie asks you every time, in every
mode, because it applies in every project from then on.

## Skills

A skill is your written instructions for one kind of task: how to cut a
release, how you like a change reviewed. Evie reads each skill's
one-line description, and loads the full instructions only when a task
calls for them. So you can have many without paying for them all on every
request.

A skill is a folder holding a file named `SKILL.md`:

```
---
name: release
description: How to cut a release of this project.
---
1. Update the changelog.
2. Run scripts/tag.sh.
```

The folder can hold other files the instructions refer to, such as a
script or a template. Evie is told where they are when it loads the skill.

Put the folder in **`~/.evie/skills/`** to have the skill in every project,
or in **`.evie/skills/`** in a project's folder for that project. A
project's skills are used only when you have trusted the project.

There are three ways a skill is used:

- **Evie picks it up** when a task matches its description.
- **You ask for it in words:** "use the release skill".
- **You type a slash and its name:** `/release fix the changelog first`.
  This always loads it. A built-in command of the same name wins, so a
  skill called `mode` is reached in words.

`/skills` lists them. A skill can be used as soon as its file is saved. You
can ask Evie to write one; for one in `~/.evie/skills/`, Evie asks you
every time, as it does for an agent.

## Permissions

Before a tool runs, Evie decides whether it is allowed, denied, or needs
your say-so. When it needs your say-so, Evie asks, and you answer "Yes,
once", "Always in this project", or "No". You can also type a reason in
place of a number, and the model is told it.

What happens without a rule depends on the mode:

| | Read and search | Change files in the project | Shell commands | Outside the project |
|---|---|---|---|---|
| `read-only` | Allowed | Denied | Only ones that just look, such as `ls` and `git status` | Denied |
| `manual` | Allowed | Asks | Asks, except ones that just look | Asks |
| `auto` | Allowed | Allowed | Allowed, except ones that send, publish or deploy, which ask | Asks |

Sessions start in `manual`. `/mode` changes the mode for the session.

Rules go in `evie.json`, and are looked at before the mode:

```json
{
  "permissions": {
    "mode": "manual",
    "rules": [
      { "action": "shell", "match": "bun test *", "effect": "allow" },
      { "action": "edit", "match": "*.env", "effect": "deny" },
      { "action": "shell", "match": "git push *", "effect": "ask" }
    ]
  }
}
```

- `action` is `read`, `edit`, `shell`, `web`, or the name of any other tool.
- `effect` is `allow`, `deny` or `ask`.
- `match` is a pattern for what the action is done to. Without it, the rule
  covers every such action.
  - For `read` and `edit` it is a path. `*` stands for anything within one
    folder, and `**` for anything at all. A pattern with no slash, such as
    `.env` or `*.pem`, is a file name and matches it in any folder. Any
    other pattern that is not a full path is taken from the project folder.
  - For `shell` it is the command, and `*` stands for anything. A pattern
    that ends in a space and `*`, such as `git push *`, covers `git push`
    by itself and with anything after it.

A command line is judged one command at a time, so `git status && git push`
is asked about for the push. A command that names a place outside the
project, such as `cat /etc/hosts`, counts as reaching outside it. A rule
that denies reading or changing a file also stops a command that names it.

A rule that denies always wins. Otherwise the last rule that matches
decides. In `read-only` mode no rule can allow a change.

An "Always in this project" answer is saved as a rule in the global
`evie.json`, under `permissions.projects` and the project's folder. A
trusted project's own `.evie/evie.json` can set `mode` and `rules` for that
project; its rules come after the global ones.

When nobody can be asked, as when input is piped in, what would be asked is
denied.

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
address of, but cannot search. `EVIE_TOKEN` is for a device that connects
to a server on another computer; see "The server and other devices".

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

**Long conversations.** Every request carries the whole conversation, so a
long session would eventually be more than the model can read. Before that
happens, Evie replaces the older part with a summary and carries on. Nothing
is deleted from its record. It does this at 80% of what the model can read,
or at 200,000 tokens, whichever comes first, and `/compact` does it on
request. Two settings change that:

```json
{
  "compactAt": 120000,
  "contextLimits": { "some/model": 64000 }
}
```

`compactAt` is the size at which a conversation is compacted. Evie looks up
what each model can read from OpenRouter; `contextLimits` is for a model it
gets wrong or does not know.

**The server.** Where `evie serve` listens, and which web pages besides its
own may connect to it, such as a web client being developed on another
port:

```json
{
  "server": { "host": "127.0.0.1", "port": 3843, "origins": ["http://localhost:5173"] }
}
```

A setting in the environment wins over the files, so `EVIE_MODEL=... evie serve`
points the server at another model. `EVIE_MAX_TOKENS` works the same way,
and `EVIE_SERVER` points `evie` at another server.

A project can have settings of its own, in `.evie/evie.json` in its folder.
See "Trusting a project" for when that file is used.

## Plugins

Everything outside Evie's core is a plugin: the tools, the model provider,
the per-model settings and the permissions.

### Choosing which plugins load

`evie.json` in the data folder lists the plugins to load, in order. Without
a list, the twelve built-in ones load:

| Plugin | What it adds |
|---|---|
| `files` | `read`, `write`, `edit`, `patch`, `grep` and `glob` |
| `shell` | `shell` |
| `jobs` | `job_stop`, and the `/jobs` and `/background` commands |
| `permissions` | The rules that allow, deny or ask before a tool runs, and the `/mode` command |
| `undo` | The copies kept of files before they are changed, and the `/undo` command |
| `models` | What each model family is given, and the `/model` command |
| `ask` | `ask`, the model's way to put a question to you |
| `subagents` | `subagent` and `subagent_send` |
| `skills` | `skill`, the list of your skills, and the `/skills` command |
| `instructions` | The owner's and the project's instruction files |
| `web` | `web_search` and `web_fetch` |
| `openrouter` | The connection to models through OpenRouter |

```json
{
  "plugins": [
    "files",
    "shell",
    "jobs",
    "permissions",
    "undo",
    "models",
    "ask",
    "subagents",
    "skills",
    "instructions",
    "web",
    "openrouter",
    { "use": "/path/to/my-plugin.ts", "options": { "greeting": "hi" } }
  ]
}
```

One more built-in plugin, `guard`, is not loaded by default. It is the
part of `permissions` that refuses the most destructive commands, by
itself, for running Evie with no other rules.

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

For that reason a project's settings file is used only when the project's
folder is listed under `"trust"` in the global file. The first time you
open a project that has a settings file, Evie asks whether to trust it and
writes your answer to the global file: under `"trust"` for a yes, and under
`"untrusted"` for a no, so you are not asked again.

```json
{ "trust": ["/path/to/my/project"], "untrusted": ["/path/to/someone-elses"] }
```

Until a project is trusted its file is ignored as a whole, and Evie tells
you so. That way opening someone else's repository can neither run their
code nor switch off a protection such as the guard.

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
| `host.tools` | Something the model can call. Its `run` returns the text the model reads. A problem the model can fix is returned as that text; `run` throws only for a bug. Work that should carry on after the call returns is handed over with `context.jobs.start`: Evie lists it, can stop it, and tells the model when it ends |
| `host.providers` | A way to reach a model |
| `host.profiles` | What one family of models is given: a system prompt and its tools for changing files |
| `host.commands` | A command you type starting with a slash, such as `/model`. A command can tell the model something with `context.tell`, which adds a note to the conversation |

It can also attach to three *hooks*, moments in the loop where a plugin's
code runs:

| Hook | When it runs | What it can do |
|---|---|---|
| `before_request` | After a request is built, before it is sent | Change the system prompt and the tools the model is offered |
| `before_tool` | After a tool call's input is checked, before the tool runs | Refuse the call with a reason |
| `after_tool` | After a tool has run | Change the result the model sees |

A hook and a tool can also ask you a question and wait for the answer.
`context.ask` returns the choice you made, or `null` when the question was
refused: the turn was cancelled, or the client cannot show questions. Treat
`null` as a no.

```ts
host.on("before_tool", async (call, context) => {
  if (call.name !== "shell") return
  const answer = await context.ask({ text: `Run ${call.input.command}?`, choices: ["Yes", "No"] })
  if (answer !== "Yes") return { deny: "The owner did not allow it." }
})
```

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
- **The server carries them over the network.** `evie serve` runs the core
  and passes commands and events over one WebSocket, as JSON in the core's
  own shapes. It is in `src/server/`; `wire.ts` holds the messages.
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

### A development copy beside the everyday one

To try a change without risking the Evie you use every day, run a second
copy, from another checkout, with a data folder of its own. Its server,
database, tool output, undo copies and settings are then its own, so a
change to its tables cannot stop the everyday Evie from opening its own.

A small launcher on your path sets the folder, such as
`~/.local/bin/evie-dev`:

```sh
#!/bin/sh
EVIE_HOME="$HOME/.evie-dev" exec bun /path/to/the/development/checkout/src/cli.ts "$@"
```

Make the folder once, with links to what you wrote in the everyday one, so
that both use the same keys, instruction file, agents and skills. Give it a
settings file of its own, with a model and a port for its server:

```sh
mkdir -p ~/.evie-dev
for name in .env AGENTS.md agents skills; do ln -s ~/.evie/$name ~/.evie-dev/$name; done
echo '{ "model": "a-name-from/openrouter.ai-models", "server": { "port": 3844 } }' > ~/.evie-dev/evie.json
```

Then `evie-dev serve` starts the development server, and `evie-dev` in
another terminal connects to it. Start that server by hand rather than as
a login service: a broken development build kept alive by one would be
started again and again. Anything written through the links, such as a new
agent, is shared with the everyday Evie.

### Made-up sessions

`evie seed` fills an empty data folder with made-up projects and sessions,
for trying out a development copy or a client without your real ones: a
plain conversation, tool calls, a question answered and one still waiting,
a background job, a subagent, and a compacted conversation. The projects
are small git repositories beside the data folder, in a folder of the same
name with `-seed` added, such as `~/.evie-dev-seed`: the tools are kept out
of the data folder, so they could not work in a project inside it. Every
session is made by running real turns through Evie against a scripted
stand-in for the model, so it needs no key and costs nothing. The question
still waiting is left as a server that is stopped all at once leaves one,
so it is refused when a server first opens that session.

```sh
evie-dev seed
```

It refuses a data folder that already has sessions, so made-up sessions are
never mixed in with real ones. To seed a folder again, remove `evie.db` and
`server-token` from it first.

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
