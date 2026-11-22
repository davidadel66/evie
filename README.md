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
  Ctrl-C where it runs, stops everything. Restarting it stops nothing for
  good: see "How it works".
- **`evie --continue`** carries on the last session of the project you are
  in, with its conversation. It first shows the end of the earlier
  conversation, as it looked when it happened: your messages, Evie's
  replies, and a line for each tool Evie used and what came back. A long
  session shows its last 50 events, with a line saying how many earlier
  ones are left out.
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
  running; they stop when the server is stopped for good.
- **Evie can hand work to subagents.** For a side task such as searching
  the code or reviewing a change, it can start a second agent with an empty
  conversation, which works in the background and reports back. Several
  can work at once. They show in `/jobs`, where you can stop one. A
  subagent works without asking before each step, so in `manual` mode Evie
  asks you before starting one.
- **`/compact`** replaces the older part of a long conversation with a
  summary, to save room. Evie also does this by itself when a conversation
  gets long.

### Commands at a glance

In your shell:

| Command | What it does |
|---|---|
| `evie serve` | Runs the server, which also serves the web app. Leave it running; every client talks to it. |
| `evie` | Starts the terminal client in the project of the folder you are in. |
| `evie -c` (or `--continue`) | Carries on the last session of this project, joining it if it is still running. |
| `evie web` | Opens the web app in your browser, signed in. `--print` prints the sign-in link instead. |
| `evie new <folder> [name]` | Makes a project in a folder, backed up to a private GitHub repository. See "Workspaces". |
| `evie token add <name>` | Makes a token for another device, and prints it once. |
| `evie token list` | Lists the devices' tokens and when each was made. |
| `evie token remove <name>` | Takes a device's token back, and disconnects that device. |

Inside `evie`, a line starting with `/` is a command, not a message:

| Command | What it does |
|---|---|
| `/model [name]` | Shows the model in use, or switches to the one named. |
| `/mode [read-only, manual or auto]` | Shows or changes how freely Evie may act. |
| `/compact` | Replaces the older part of the conversation with a summary, to save room. |
| `/undo` | Puts back the files Evie's last turn changed. Run it again to go back further. |
| `/jobs` | Lists what is running in the background; `/jobs stop <id>` stops one. |
| `/background` | Moves the command Evie is waiting on to the background. |
| `/skills` | Lists your skills; `/<skill name>` starts one. |
| `/sync` | Backs up the project's `.evie` folder to its private repository now. |

Ctrl-C stops the turn in progress, and quits when nothing is running.

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

### The web app

The server also serves a web app, at its own address. Open it signed in
with:

```sh
evie web           # opens the web app in your browser, signed in
evie web --print   # prints the sign-in link instead, to open by hand
```

`evie web` asks the server for a sign-in link, which works once, within
five minutes, and opens it with `open` on a Mac or `xdg-open` elsewhere.
Only this computer's own token can ask for one, so `evie web` works on the
server's computer.
The page trades the link's code for a cookie, and takes the code out of the
address bar. Each browser signed in this way gets a token of its own, named
`browser` and the day, such as `browser 2026-11-21`, or
`browser 2026-11-21 (2)` for a second one that day. A browser signed in
already keeps its token when `evie web` opens it again. `evie token list`
shows them, and `evie token remove "browser 2026-11-21"` signs that browser
out at once. The cookie lasts 400 days, the longest a browser keeps one, and the
page's scripts cannot read it.

On another device, such as a phone, open the server's address and paste a
token made for that device with `evie token add phone`. The server must be
listening where the device can reach it (see above), and a page reached by
any name other than `127.0.0.1` or `localhost` must be listed under
`origins`, or the server refuses it:

```json
{ "server": { "host": "0.0.0.0", "origins": ["http://my-mac.local:3843"] } }
```

The page has three columns: the list of sessions, the conversation and the
project card. While the page is connected, nothing says so; while the
line is down, one quiet line under the name at the top says Reconnecting,
and why, and the page catches up by itself once the line is back. The gear beside the name opens the settings, where the theme is
picked: one of Dusk, Moss and Oxblood, which the browser remembers.

The list shows every session you have said something in, kept current as
they change, and listed again when the line comes back:

- **At the top**, the sessions waiting for an answer from you ("Needs
  you"), then the other recent ones, newest first. **Working** and
  **Settled** are groups at the bottom, which open and close; the browser
  remembers which are open.
- **Each row** shows its project's icon and name, and how long ago it was
  active, or its state; the session's name; and the git branch its
  project is on, asked for again when the window has the focus back and
  each minute while the page is shown. Until a project's icon and colour can be picked, each
  project has a folder in a colour chosen from its id.
- **New session** makes a draft of a new session, at the top of the list,
  in the project of the selected session or draft, or of the project the
  list is narrowed to; the menu beside it makes one in another project, or
  in none. Nothing is made on the server until its first message is sent,
  so a draft left unused leaves nothing behind. Pressed again, it comes
  back to the same draft. The first message starts the session, asking
  first whether to trust the project when evie needs to know, and the page
  moves to it. A first message that cannot be sent comes back into the
  box, with why.
- **Search** narrows the list by name, and the menu beside it to one
  project.
- **A session's name** is written by a small model from your first
  message, in the background, without holding up the reply; until it
  comes, the row shows the opening words of that message. A request for a
  name that fails or takes too long is given up, and the opening words
  stay. You can rename a session at any time, and evie never replaces
  your name.
- **A row's menu** (the `···` on the selected row, or on any row under the
  pointer) renames the session, moves it to another project or to none,
  settles it or brings it back, and deletes it. A double-click on the row,
  or F2, renames it too: Enter or leaving the field saves the name, Escape
  gives it up, and a blank name is refused. A session moved works in its
  new project, with that project's plugins, from its next turn, and evie
  is told where it now works. Delete asks first, and removes the session
  for good: its conversation, its subagents and the files evie kept for
  it, though not the files it changed in a project. A session that is
  working, or waiting for an answer, is not settled, moved or deleted;
  the row says why.
- **What is selected** is kept in the page's address, as `#session=<id>`,
  or for a draft `#draft=<project id>` or `#draft=none`, so a reload or a
  bookmark opens on it.

The middle column shows the selected session's conversation, joining it as
it stands, even while a reply is being written:

- **Your messages** are on the right; **evie's replies** are on the page,
  drawn from Markdown (headings, lists, code, links and tables). HTML in a
  reply is shown as the text it is, an image is linked to rather than
  loaded, and only a web or mail address is a link.
- **A reply streams in** as it is written.
- **While a turn runs**, it shows in the order things happen: evie's
  words where it wrote them, and each run of tools it used between them as
  one quiet line, which opens to list each call with its result in short;
  each call opens to its whole result.
- **Once a turn has completed**, everything before its final reply folds
  into one quiet line, such as "Worked for 48s", which opens to show it in
  order. A question and its answer, and notes, stay outside the fold. A
  turn that was stopped, failed or had a reply cut short is not folded.
- **Notes** say in quiet words what else happened: a compaction, with its
  summary to open; a note evie was told; a reply cut short; a turn that
  failed or was stopped; a question and your answer; and a background
  job's start and end.
- **The latest page** of the conversation is shown first, and earlier ones
  as you scroll up. While you are at the bottom, new text keeps you there;
  scrolled up, you stay where you are, with a way back down.

The message box at the bottom of the conversation works as T3 Code's does:

- **Enter sends**, and Shift and Enter starts a new line. Up in an empty
  box brings back your earlier messages in the session, and Down goes
  forward again.
- **While evie works**, what you send steers the turn, as in the terminal:
  evie reads it at its next step. A round Stop button beside Send, or
  Escape in the box, stops the turn, and says Stopping until it has.
- **A permission prompt** shows at the end of the conversation, with its
  choices as buttons: No, then Yes, once, and the others, such as always
  allowing it, under More. A message sent instead answers it No, and goes
  to evie as what to do instead.
- **Any other question** docks just above the box, its choices numbered:
  the keys 1 to 9 pick one while the box does not have the focus, a number
  sent from the box picks one, and your own words sent answer it when it
  takes them. Anything else is not sent: the box asks you to pick one of
  the choices, as the terminal asks again. Several waiting are taken one at
  a time, with a count such as
  1/3. A question that arrives while you are typing waits until you have
  not typed for a second and a half, so that a stray key cannot answer it.
  One answered elsewhere, such as in a terminal, goes away.
- **A slash** at the start of a word opens a menu of the session's
  commands and your skills, each with what it does, narrowed as you type.
  Up and Down move, Enter or Tab picks, and Escape closes it. A message
  that is a slash and a word, such as `/compact`, runs the command, whose
  answer shows as a quiet note, kept nowhere, as in the terminal.
- **`/model`**, or Cmd-Shift-M (Ctrl-Shift-M elsewhere), opens a picker of
  the models the provider offers, grouped by maker, with a search over them
  all; the one in use is marked. In a draft, it picks the model the new
  session starts with; until one is picked, the draft shows the model a new
  session would use. A project's own model is known once a session has
  opened there since the server started. In a draft, the other commands
  wait until the session has started.
- **Under the text**, the model's name, which opens the picker too, and how
  full the conversation is, as a share of the size at which evie compacts
  it by itself. Its menu gives both sizes, and Compact now.
- **At phone width** the box is one row until you tap it.

The project card, on the right, is the selected session's:

- **Its project:** the project's icon and name, its folder, cut at its
  start when it is long, so that its end shows, and whole when pointed at;
  and the git branch it is on, kept current as the session list keeps it,
  or "No branch" in a repository on none of its own, or "Not a git
  repository". A session with no project says so.
- **Changes:** Commit, Push and Pull request each send evie a request in
  words, as a message in the session, such as "Commit the changes in this
  session with a short message."; evie carries it out as it would if you had
  typed it, and asks before pushing, as it always does. A request sent
  keeps the three off until evie starts on it, so it is sent once. Undo
  last change runs `/undo`, and what it says is a quiet note in the
  conversation, as the terminal shows it; like the terminal's, the note is
  not kept, so it is gone once the session is opened again. Each button is
  disabled while it does not apply, and a line under it says why: the
  session still opening, no project, a folder not in a git repository, or
  evie working on a turn. With a subagent's conversation open, a press goes
  back to the session's own first, so that what it says is in view.
- **Subagents:** each subagent the session has started, newest first, with
  its name and how its latest round of work stands, in a word (Working,
  Done, Failed or Stopped), and Stop while it works. One with no round
  since evie's server last started says Ended. Picking one opens its
  conversation in the middle, read only, under its name, with a line below
  that leads back to the session; while it works, what it adds is read
  every second.
- **Background jobs:** each job the session has run in the background since
  evie's server last started, as `/jobs` lists them, with its command and
  how it stands, and Stop while it runs, which is `/jobs stop`. One opens
  to its output, which keeps up while it runs, and stays open when the job
  ends. Ended jobs fold away under their count.

At desktop width the card can be hidden and shown with the button at the
right of the middle column's header, or with Cmd+Option+B (Ctrl+Alt+B
elsewhere), and the browser remembers which. At 980px wide or narrower the
same button opens it as a sheet over the page, from the right, which goes
once something is picked.

### Keeping the server running

On a Mac, the server can run as a login service, so that it is there
whenever you are logged in, and is started again if it stops:

```sh
evie service install     # starts `evie serve` now, and at every login
evie service status      # whether it is installed and running, and where its log is
evie service uninstall   # stops it, and no longer starts it at login
```

`install` writes a launch agent,
`~/Library/LaunchAgents/com.evie.serve.plist`, for the copy of Evie it is
run from, and starts it. Run it again after updating Evie: it stops the
server that is running and starts it on the new version.

The service is given the data folder (`EVIE_HOME`, when it is set) and the
`PATH` of the terminal it was installed from, so that it finds Bun and the
commands Evie runs; install it again after changing either. It runs the
`bun` that path finds, by the name the path gives it, which a package
manager usually keeps in place when it updates Bun; if Bun moves, install
the service again. Nothing else from that terminal reaches it, so a key or
setting exported there, such as `OPENROUTER_API_KEY` or `EVIE_MODEL`, has
to be in the data folder's `.env` (or, for the model, in `evie.json`) for
the service to have it. `install` names any it finds set only in the
terminal, without showing their values. What the server prints goes to
`serve.log` in the data folder.

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

## Workspaces

Every project keeps what Evie adds to it in a `.evie` folder inside the
project's own folder: its settings, and later its dashboards, files for
retrieval and schedules. This is the same for a code repository and for
any other folder, such as one for your finances. A project's folder is
wherever you keep it; there is no fixed place for them.

**Making one.** With the server running:

```sh
evie new ~/finances              # named "finances", after its folder
evie new ~/finances "Finances"   # or named as you say
```

The folder is made when it is missing. Evie makes `.evie/evie.json` in it,
registers the project, and trusts it (see "Trusting a project"), so its
own settings are used without asking. When its `.evie` folder already
held files, such as skills that came with a cloned repository, the
project is made but not trusted, and its first session asks. A folder
that is already a project, that already has a `.evie/evie.json`, that is
inside a git repository other than at its top, or that is in Evie's data
folder is refused. `evie new` says whether the project is trusted and
whether its `.evie` folder is backed up. Then run `evie` in that folder
to start working there.

**Kept out of the project's own git.** When a project's folder is a git
repository, Evie adds `/.evie/` to the repository's local exclude list,
`.git/info/exclude`, each time a session opens there, unless the
repository already commits something in `.evie/` on purpose. Git reads that list
the way it reads a `.gitignore`, but it is never committed, so a code
repository never commits what Evie keeps in it, and nothing the
repository commits is changed.

**Backed up to a private repository.** Where a project's `.evie` folder
goes is a setting in its own `.evie/evie.json`:

```json
{ "sync": "private" }
```

- `"private"` makes the `.evie` folder a git repository of its own and,
  the first time, a private repository on your GitHub account named
  `evie-` and the project's name, in lower case with spaces and other
  characters made into dashes (`evie-finances`). It is made with GitHub's
  command-line tool, `gh`, which has to be installed and logged in
  (`gh auth login`) on the computer the server runs on. A repository
  already on your account is never taken over: when the name is taken,
  the next free one is used, such as `evie-finances-2`. Evie records the
  repository it made in the `.evie` folder's own git settings, and before
  every push checks that the remote is still that repository and that
  GitHub still says it is private; otherwise nothing is pushed. A minute
  after Evie last changed something in the `.evie` folder, and once an
  hour, what changed is committed and pushed, without asking: the setting
  is your permission. Every project is looked at once an hour from when
  the server starts, whether or not it has been opened. `/sync` does it
  at once and says how it went.
- `"none"`, or no setting, keeps it on this computer.

`evie new` starts a project as `"private"`. Like the rest of a project's
own settings, the setting counts only once the project is trusted, so a
repository cloned from someone else cannot turn syncing on for itself.

A sync that fails, as when `gh` is missing or not logged in, or there is no
network, never interrupts Evie's work. It is tried again a minute after
the next change and every hour, and `/sync` says what went wrong. Nothing
is ever pushed by force: when the repository has commits the folder does
not, as when another computer synced to it, `/sync` says so and how to
bring them in. A `.evie` folder that holds what a data folder holds
(`evie.db`, `.env` or `server-token`) is never synced. Only the `.evie` folder is backed up: keep a
workspace's other documents in `.evie/files/`, or in git yourself, to
have them backed up too.

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
See "Trusting a project" for when that file is used, and "Workspaces" for
its `sync` setting.

## Plugins

Everything outside Evie's core is a plugin: the tools, the model provider,
the per-model settings and the permissions.

### Choosing which plugins load

`evie.json` in the data folder lists the plugins to load, in order. Without
a list, the fourteen built-in ones load:

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
| `sync` | The backing up of a project's `.evie` folder, and the `/sync` command |
| `naming` | A name for each session, written by a small model from its first message |
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
    "sync",
    "naming",
    "openrouter",
    { "use": "/path/to/my-plugin.ts", "options": { "greeting": "hi" } }
  ]
}
```

Sessions are named by `anthropic/claude-haiku-5.5`, through the same
provider as the rest. Another model is set in the plugin's options:
`{ "use": "naming", "options": { "model": "another/model" } }`.

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

It can also attach to *hooks*, moments where a plugin's code runs:

| Hook | When it runs | What it can do |
|---|---|---|
| `before_request` | After a request is built, before it is sent | Change the system prompt and the tools the model is offered |
| `before_tool` | After a tool call's input is checked, before the tool runs | Refuse the call with a reason |
| `after_tool` | After a tool has run | Change the result the model sees |
| `session_deleted` | After a session has been deleted for good | Remove the files the plugin kept for it |

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

Hooks run in the order their plugins are listed. Each of the first three
is told the session, its project and the conversation so far, and can
give the session a name with `context.name`, which does nothing once it
has one. `host.project` says which project
the plugin is being set up for.

A plugin with work of its own over time, such as the `sync` plugin's
hourly backups, can also have a `start` function. The server calls it
once it is listening, with every project and a signal that fires when the
server stops:

```ts
start({ projects, signal }) {
  const timer = setInterval(() => { /* look at each of projects */ }, 60 * 60_000)
  signal.addEventListener("abort", () => clearInterval(timer))
}
```

The built-in plugins in `src/plugins/` use exactly this interface, and are
the best examples to read.

## How it works

- **The event log is the truth.** Everything that happens in a session is
  added to a log in a SQLite database and never changed. Before every
  request to the model, the conversation is rebuilt from that log. Evie
  keeps no conversation in memory, which is why resuming a session, or
  recovering from being stopped part way, needs nothing special.
- **Work carries on across a restart.** Each command is a program of its
  own that writes what it prints straight to a file, so it keeps running
  while the server restarts, and the new server finds it again, with its
  output and how it ended. A turn the restart cut off within the last hour
  is carried on by itself, with a note telling the model to check before
  repeating anything that changes something: a turn of your own session at
  most three times in five minutes, and a subagent's at most three times
  for its task. A turn that was waiting for an answer, or was cut off
  longer ago, is left stopped, with a note, for you to carry on. Only
  stopping the server for good, with Ctrl-C, by closing the terminal it
  runs in, or by removing its login service, stops background commands.
- **One way in, one way out.** A client sends commands and subscribes to
  events. The terminal is a client like any other and uses nothing else, so
  a web or phone client can be added without changing the core.
- **The server carries them over the network.** `evie serve` runs the core
  and passes commands and events over one WebSocket, as JSON in the core's
  own shapes. It is in `src/server/`; `wire.ts` holds the messages.
- **The web app is one more client.** It is in `web/`, and connects
  through the same network client as the terminal, from
  `web/src/connection.ts`, which keeps the connection apart from the screens.
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

### Working on the web app

The web app is in `web/`: a React page, styled with Tailwind, bundled by
Bun's own bundler. There is no build step. `evie serve` builds the page in
memory as it starts, which takes a few milliseconds, and serves it.

To work on it, start a development server in the repository's folder:

```sh
cd /path/to/the/development/checkout
EVIE_HOME="$HOME/.evie-dev" bun src/cli.ts serve --dev
```

With `--dev`, Bun bundles the page again on each request, and updates the
open page as its files are saved.

Two questions were settled by trying them:

- **A component keeps its state when its file is edited.** Bun's
  development server applies React Fast Refresh: an edited component is
  swapped into the open page without reloading it, and what was on screen,
  such as an open menu, stays as it was. This holds for files whose exports
  are all components. Editing any other file, such as `connection.ts` or
  `theme.ts`, reloads the page. The connection is kept across an update, so
  saving a file opens no second one.
- **Tailwind is built by `evie serve` itself, with no build script.** It
  calls `Bun.build` on `web/index.html` with Tailwind's plugin,
  `bun-plugin-tailwind`, handed over in code, and serves the result from
  memory, minified. Bun's other way, handing the page to `Bun.serve` with
  `development: false`, bundles on the first request, but takes its plugins
  only from a `bunfig.toml` in the folder the server was started in. A login
  service is not started in the repository's folder, so the page would be
  served without Tailwind. For the same reason the stylesheet names its own
  folder for Tailwind to scan for class names (`source(".")`), since
  Tailwind otherwise scans the folder the server was started in. `--dev`
  does use Bun's development server, which reads the plugin from the
  repository's `bunfig.toml`: that is why it must be started there.

Menus, and later popovers and dialogs, are [shadcn/ui](https://ui.shadcn.com)
parts: their code is copied into `web/src/components/ui/` and styled to
the design system, and they are built on Radix, which handles the
keyboard, focus and screen readers. They join their classes with `cn()` in
`web/src/lib/utils.ts`, which is told the design system's names for text
sizes and sizes, so that, say, `text-caption` and `text-text-muted` are
both kept. A size added to the theme in `styles.css` is added to its list
too; a test checks the two agree.

The design system's tokens are copied from its `tokens.json`: the colours
into `web/src/tokens.css`, one set per theme, and the rest into Tailwind's
theme in `web/src/styles.css`. Tailwind's own defaults are cleared, so each
class names a token, such as `bg-bg-side`, `text-text-muted`,
`text-caption` or `p-7`, and its spacing scale holds only the design
system's steps, so a class off it, such as `p-5`, is not made at all. The
fonts, Geist and Geist Mono, are carried in the stylesheet, so the page asks
no other site for anything.

A few values the design system's board does not name are named once in
`styles.css`, each by a name of its own:

| Name | Value | For |
|---|---|---|
| `session-list` | 288px | The width of the session list |
| `project-card` | 320px | The width of the project card |
| `card-sheet` | `min(88vw, 448px)` | The width of the project card as a sheet over the page, at 980px or narrower |
| `sign-in-column` | 400px | The width of the sign-in page's column, and of text in the middle |
| `conversation` | 740px | The widest the conversation is, which the board's Message gives in words |
| `call-name` | 48px | The room a tool call's name takes in a turn's steps, as the board's Message draws them |
| `control` | 40px | The height of buttons and menu items, which the board's Button and Menu give in words |
| `menu` | 176px | The narrowest a menu is |
| `dialog` | 400px | The widest a dialog is, such as the settings window |
| `tile-row` | 18px | A project's icon tile in a session row, which the board's WorkspaceIcon gives in words |
| `tile-row-icon` | 12px | The icon on that tile, as the board's SessionRow draws it |
| `project-filter` | 128px | The widest the session list's project filter grows |
| `composer-text` | 240px | The tallest the message box's text grows before it scrolls |
| `composer-menu` | 320px | The tallest the `/` menu and the model picker above the message box are, as T3 Code's |
| `picker-makers` | 144px | The model picker's column of makers |
| `job-output` | 240px | The tallest a background job's output grows in the project card before it scrolls |
| `radius-tile` | 25% | A project's icon tile's corners, a quarter of its size at any size |
| `default-transition-duration` | 150ms | How long a change takes, such as a group's caret turning |
| `default-transition-timing-function` | `cubic-bezier(0.4, 0, 0.2, 1)` | How it eases |

The strokes, `line-width` (1px) and `indicator-width` (2px, for the focus
ring), are the board's, kept beside the colours in `tokens.css`.

`bunfig.toml` also stops `bun install` from installing peer dependencies.
The Tailwind plugin names Bun itself as one, and a second Bun in
`node_modules` would then be the one `bun run check` uses.

### How changes are checked and released

The repository has two long-lived branches:

- `dev`, where finished work lands first;
- `main`, the stable version, which is what most people should run.

```
 a branch per change --pull request--> dev --release: pull request--> main
                         check runs              check runs
```

- **Every change is its own branch and pull request into `dev`.** The
  change is reviewed before it is merged.
- **GitHub runs the check on every pull request** and on every push to `dev`
  and `main`. It is `.github/workflows/check.yml`: it sets up Bun 1.4.0
  and ripgrep, which the search tools run, and then runs `bun run check`,
  the type check and every test. A pull request whose check fails is not
  merged.
- **A release is a pull request from `dev` into `main`.** It lists what it
  brings, and is merged once its check passes. There is no separate build
  or deploy step: Evie runs from its source with Bun.
- **Taking a release:** in a copy of the repository that follows `main`, pull
  it (`git pull origin main`), then start the server again so it runs the
  new version: `evie service install` does that for the login service, or
  stop and start `evie serve` if you run it by hand.

To try work on `dev` without risking the copy you use every day, keep two
copies of the repository side by side: one on `main` that you use, and one
on `dev` that you try things in, as the next section describes.

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
another terminal connects to it. Start that server by hand rather than as a
login service: a broken development build kept alive by one would be
started again and again. There is one login service, so `evie-dev service
install` refuses while the everyday one is installed, and `evie-dev service
status` says whose it is. Anything written through the links, such as a new
agent, is shared with the everyday Evie.

### Made-up sessions

`evie seed` fills an empty data folder with made-up projects and sessions,
for trying out a development copy or a client without your real ones: a
plain conversation, tool calls, a question answered, a background job, a
subagent, and a compacted conversation. The projects are small git
repositories beside the data folder, in a folder of the same name with
`-seed` added, such as `~/.evie-dev-seed`: the tools are kept out of the
data folder, so they could not work in a project inside it. Every session
is made by running real turns through Evie against a scripted stand-in for
the model, so it needs no key and costs nothing. None of them is waiting
for an answer: Evie refuses a question still open when it stops, so a
session shows as waiting for an answer only while a real question is open.

```sh
evie-dev seed
```

It fills only a folder that `EVIE_HOME` names, never the everyday
`~/.evie`, and it refuses a data folder that already has sessions, so
made-up sessions are never mixed in with real ones. It also refuses when
one of its project folders is already there. To seed a folder again,
remove `evie.db` and `server-token` from it, and its `-seed` folder, first.

## What comes next

- The rest of the web client: the changes and their diffs, and a
  workspace for each project.
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
- [T3 Code](https://github.com/pingdotgg/t3code) (MIT): the prompt that
  names sessions, in `src/plugins/naming.ts`, rewritten in evie's words.
  Its licence is in `LICENSES/`.

The web app's fonts are [Geist and Geist Mono](https://github.com/vercel/geist-font)
(SIL Open Font License 1.1), from the `@fontsource-variable` packages. Its
menus are adapted from [shadcn/ui](https://github.com/shadcn-ui/ui) (MIT),
and its icons are copied from [Phosphor](https://phosphoricons.com) 2.1.1
(MIT). Their licences are in `LICENSES/`.

## Licence

Not chosen yet. Until a licence is added, the code is published to be read,
and no permission to use, copy or change it is granted.
