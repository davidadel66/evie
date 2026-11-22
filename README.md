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
  session shows its last 50 events, or fewer when they come to more than
  about 1 MB, with a line saying how many earlier ones are left out.
- **`/model`** shows which model is in use. `/model` and a name switches to
  another in the middle of a session.
- **`/mode`** shows how freely Evie may act, and `/mode auto`, `/mode manual`
  or `/mode read-only` changes it for the session, which keeps it when evie
  starts again. See "Permissions".
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
| `evie serve --check` | Checks that the server would start with its settings, without starting it, and says what would stop it. |
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
effect when the server is started again, which the web app's settings can
do (see "The web app").

When the connection to the server drops, as when the laptop sleeps or the
server is started again, `evie` says so and connects again by itself, then
shows what it missed, without repeating anything. So does a device the
server cannot send to fast enough: once more than 16 MB waits to be sent to
it, the server closes its connection rather than leave a message out, and
it catches up the same way. What was missed comes in pages of about 1 MB,
each asked for once the one before has arrived, so that a slow line, such
as a phone's after a long session, catches up a page at a time instead of
waiting on one large reply. When it cannot catch up,
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

The page has two columns, the list of sessions and the conversation, with
the session details floating at the conversation's top right. While the page is connected, nothing says so; while the
line is down, one quiet line under the name at the top says Reconnecting,
and why, and the page catches up by itself once the line is back.

The list can be hidden and shown with the button at the left of the
middle column's header, or with Cmd+B (Ctrl+B elsewhere), and the browser
remembers which. At 767px wide or narrower, as on a phone, the columns
stack: the list is at the top of the page and the conversation under it,
a screen tall. Picking a session, even the one open, brings its
conversation into sight, and the same button scrolls back up to the list;
the Reconnecting line is then under the session's header.

At the top of the list, beside evie's name, the squares open the
workspace view, on the workspace of the project you are looking at (or,
with no workspace yet, the New workspace dialog; until the projects have
been listed, it waits), and the pencil starts a new session.

The gear beside the name opens the settings, as does Open settings in
the command palette and the gear at the foot of the workspace strip. They
are a page of their own, as T3 Code's are, over the page dimmed under
them, and the whole screen on a phone. They have an address of their own,
beside whatever is open, such as `#session=…&settings=keys`, or alone,
`#settings=keys`; Back or Escape closes them. Their pages are down the
left, in groups, each with its icon (a rail of the icons on a phone),
under a search over every setting: `/` or Mod+F reaches it, Up and Down
move through what it finds, and Enter goes to the setting's page and
shows it. A setting changed from its default, such as the theme, has a
reset after its title. There is a page each for:

- **Appearance:** the theme, one of Dusk, Moss and Oxblood, which the
  browser remembers.
- **Notifications:** how evie tells you when a session needs you, or a
  turn finishes or fails (see "Notifications" below), which the browser
  remembers.
- **Keys:** the keys the loaded plugins read from `.env` (see
  "Settings"), each with what it is for and only whether it is set. A new
  value is written to the file, readable by you only, and is never shown
  again, here or anywhere else; Clear takes it out. A key set where the
  server runs wins over the file, and the page says so.
- **Plugins:** the plugins `evie.json` loads, each with whether it is
  loaded. A plugin file can be added by its full path, and taken off again;
  it goes under `addPlugins` (see "Choosing which plugins load"). The
  built-in ones are changed in `evie.json` itself.
- **Instructions:** your instruction file, `AGENTS.md` in the data folder,
  to edit and save. It applies from the next message. What you have written
  and not saved is kept when the settings are closed, until you save it.
- **Projects:** each project, with whether its own settings are trusted,
  to trust it or stop trusting it.

Each change is checked as the server would check it when it starts, so one
that would stop it starting is refused there and then, in the words
starting would give, and nothing is written. A plugin file is checked by
`evie serve --check`, in a process of its own: it loads the settings files
and sets the plugins up together, as starting does, without listening or
opening any session. Most changes apply once the server starts again: the
page says so, and **Restart to apply** starts it again, as after a crash,
so that work in progress carries on; the page connects again by itself. A
restart is refused, saying why, when the same check finds the server would
not start. Under the login service (see "Keeping the server running"), the
service starts the server again; run by hand, `evie serve` starts a new
server in its own place, in the same terminal, which Ctrl-C stops as
before.

Keys are typed in, and plugins and trusted projects changed, through the
settings only on the computer evie runs on: the server refuses those
changes from a connection that does not come from this computer's own
address, 127.0.0.1 or ::1, or that comes from a page at any other address,
and a page opened at any other address shows them as they stand. So a key
never crosses the network. This is a seat belt, not a wall: a signed-in
device can still ask evie to do anything evie can do, and can answer the
question whether to trust a project, as opening a project's session from a
phone asks.

Something on this computer that passes connections on to the server, such
as a proxy that serves it over HTTPS, makes them all come from this
computer's address; a browser's page still says where it is, but a client
that is not a browser does not. So keep the server on 127.0.0.1, as it is
unless `evie.json` says otherwise, unless what is in front of it does its
own sign-in.

The list shows every session you have said something in, kept current as
they change, and listed again when the line comes back:

- **At the top**, the sessions waiting for an answer from you ("Needs
  you"), then the other recent ones, newest first. **Working** and
  **Settled** are groups at the bottom, which open and close; the browser
  remembers which are open.
- **Each row** shows its project's icon and name, or for a session in no
  project an empty dashed tile and "No project", and how long ago it was
  active, or its state; the session's name; and the session's own git
  branch: the one its project was on as its latest turn started, which
  evie records at a turn's start when it has changed. Two sessions of one
  project can show different branches. A project has a folder in a colour
  chosen from its id until you pick its icon and colour in its workspace
  (below).
- **New session** (the pencil) makes a draft of a new session, at the top
  of the list, in the project of the selected session or draft, or with
  nothing open, of the latest session. In a draft, the project in the
  header is a menu that starts it in another project, or in none, as does
  the command palette's "New session in a project…". With nothing open,
  the page is such a draft, in the project of the latest session as the
  page first showed it. Nothing is made on the server until its first
  message is sent,
  so a draft left unused leaves nothing behind. Pressed again, it comes
  back to the same draft. The first message starts the session, asking
  first whether to trust the project when evie needs to know, and the page
  moves to it. A first message that cannot be sent comes back into the
  box, with why.
- **Search** narrows the list to the sessions whose name and project's
  name hold every word typed, so a project's name narrows it to that
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
  or on the name at the top of the conversation, or F2, renames it too:
  Enter or leaving the field saves the name, Escape gives it up, and a
  blank name, or one over 80 characters, is refused. A session moved
  works in its new project, with that project's plugins, from its next
  turn, and evie is told where it now works; a move is not activity, so a
  settled session stays settled. Delete asks first, and removes the
  session for good: its conversation, its subagents and the files evie
  kept for it, though not the files it changed in a project. A page that
  has the session open lets it go and says it was deleted. A session that
  is working or waiting for an answer is not settled, moved or deleted,
  nor moved or deleted while it opens; the row says why.
- **What is selected** is kept in the page's address, as `#session=<id>`,
  or for a draft `#draft=<project id>` or `#draft=none`, so a reload or a
  bookmark opens on it.

The middle column shows the selected session's conversation, under a
header with its project's icon and name and the session's name, joining
it as it stands, even while a reply is being written. A session with
nothing said in it yet, or a draft, says "What should evie do?":

- **Your messages** are on the right; **evie's replies** are on the page,
  drawn from Markdown (headings, lists, code, links and tables). HTML in a
  reply is shown as the text it is, an image is linked to rather than
  loaded, and only a web or mail address is a link. What you attached to a
  message is under it: an image as a thumbnail, which opens full size, and
  any other file as its name and size.
- **A reply streams in** as it is written.
- **The work log** is T3 Code's: each tool call is one row with an icon
  and a sentence, such as "Read src/auth.ts", "Edited README.md" or "Ran
  bun test", which opens to its whole result, and shows when it happened
  while the pointer is over it; a call that failed has a red ✕. A run of
  several calls with nothing said between them is one row counting them,
  such as "Read 2 files and ran 1 command", which opens to a row for each,
  scrolling past a height. The words are the same as the workspace's side
  chat's.
- **While a turn runs**, a line at the end of the conversation says
  "Working for 12s · Stop", counting up on the server's clock, also while
  a question waits; a run being carried out says what it is doing, such
  as "Running bun", and lists that call by the command alone; and between
  calls, a row says Thinking.
- **Once a turn has completed**, everything before its final reply folds
  into one quiet line, such as "Worked for 48s" or "Worked for 3.4s",
  which opens to show it in order: the time from its start to its end, as
  T3 Code counts it, waits for your answers included, so it never drops as
  the turn ends. A question and its answer, and notes, stay outside the
  fold. A turn that was stopped, failed or had a reply cut
  short is not folded, and its first run says how long it worked, or
  "You stopped after …".
- **Notes** say in quiet words what else happened: a compaction, with its
  summary to open; a note evie was told; a reply cut short; a turn that
  failed or was stopped; a question and your answer; and a background
  job's start and end.
- **The latest page** of the conversation is shown first, and earlier ones
  as you scroll up. While you are at the bottom, new text keeps you there;
  scrolled up, you stay where you are, with a way back down.

The message box floats over the end of the conversation, which scrolls on
under it, and works as T3 Code's does:

- **Enter sends**, and Shift and Enter starts a new line. Up in an empty
  box brings back your earlier messages in the session, and Down goes
  forward again.
- **While evie works**, Stop takes Send's place while the box is empty,
  and Escape in the box stops the turn too; it says Stopping until it has.
  The conversation's own line, "Working for 12s · Stop", keeps a quiet Stop
  as well. What you send steers the turn, as in the terminal: evie reads it
  at its next step.
- **Queued, not steered:** with Mod held, Mod+Enter or a Mod-click on Send,
  a message sent while evie works waits instead, at the end of the
  conversation, dashed and marked Queued, and is sent as a turn of its
  own once the turn before it has finished: one at a time, the next only
  once the last has reached the session and its turn has ended. While a
  question waits, Mod+Enter answers it, or says no and what to do
  instead, as Enter does, and never queues. Each queued message can be
  sent at once, edited in the box (Escape gives the edit up; Alt+Up edits
  the last) or removed; one is edited at a time, and none while a
  question waits. A turn that is stopped, or fails, holds the queue, which
  says so, "Held after the turn was stopped", with Send now, until you send one of its
  messages by hand; a turn started another way, from the box or a
  terminal, does not let it go on. Leaving the session gives up an edit
  and holds the queue too, so that nothing is sent while you cannot see
  it, and it is there, held, when you come back. When the line comes back
  after a drop, the session is read again before anything is sent, and
  how its last turn ended decides. The page holds the queue, not the
  server: a page closed or reloaded loses it. For a moment after a send,
  Send stays Send, so that a second click is not a Stop.
- **A question** shows at the end of the conversation, as part of evie's
  reply: the question, its choices as round buttons under it, the first
  the main one, and "Needs you. Waiting for your answer". The keys 1 to 9
  pick one while the focus is on nothing or in the empty box, and your
  own words sent answer it when it takes them. Anything else is not sent:
  the box asks you to pick one of the choices above, as the terminal asks
  again. Several waiting are taken one at
  a time, with a count such as
  1/3. A question that arrives while you are typing waits until you have
  not typed for a second and a half, so that a stray key cannot answer it.
  One answered elsewhere, such as in a terminal, goes away.
- **A permission prompt** shows in the same place and the same way, its
  choices in order: Yes, once, as the main one, then the others, such as
  always allowing it, then No. A message sent instead answers it No, and
  goes to evie as what to do instead.
- **A slash** at the start of a word opens a drawer, tucked into the box's
  top edge, of the session's
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
- **Under the text**, the model's name as its provider names it, such as
  Claude Sonnet 5, which opens the picker too, and how
  full the conversation is, as a share of the size at which evie compacts
  it by itself. Its menu gives both sizes, and Compact now.
- **At phone width**, 767px wide or narrower as throughout the page, the
  box is one row until you tap it, and the model picker's makers are a row
  above its models that scrolls sideways.
- **An @** at the start of a word, in a session in a project, opens a menu
  of the project's files, in the same drawer, narrowed and ranked as you
  type, as the slash menu is, with the same keys. Picking one puts it in
  the text as a chip, its icon and its name, which Backspace takes off
  whole; the message says `@` and its path, such as `@src/app.ts`, and
  evie reads it with its tools. A message brought back with Up has its
  files as chips again, and so does text pasted or dropped with `@` and a
  path in it: a chip copied, cut or dragged is its `@` and its path. The
  box keeps its own undo history, so Mod+Z undoes a chip picked, a paste
  or a new line in the order they were made, and Mod+Shift+Z does it
  again. The
  files are those git lists, tracked or not, but not those it ignores, or
  outside git every file; never `.git`, `node_modules`, the `.evie`
  folder, evie's data folder or a folder of keys such as `.ssh`, and at
  most 10,000 of them; the menu says when there are more.
- **Files are attached** with the paperclip, by pasting them, or by dropping
  them on the box. Each shows as a chip above the text, an image with a
  small thumbnail, until it is sent or taken off. A file can be at most
  10 MB, and a message's own text at most 1 MB: a longer text stays in the
  box, which says to attach it as a file instead, and `evie` in a terminal
  says the same. A message with files needs no words, but a command or an answer
  to a question takes no files. When the message is sent, it shows at
  once with its files, as Sending, while each file goes to the server,
  signed in as the page is, and is kept (see "Attachments"). A message
  that cannot be sent comes back to the box with its files, and those the
  server already has are not sent again. A first message of files alone
  names the session after them.

The session details are T3 Code's details card, as the owner drew it: a
card that floats at the top right of the conversation, in small text, with
a slim row at its top naming it, which holds Choose what to show and Pin.
Its sections are the selected session's:

- **Workspace:** the project's icon and name, which open its workspace;
  its folder, written from the home folder, such as `~/code/recipe-box`,
  cut at its start when it is long, and whole when pointed at, which opens
  it in the file manager of the computer evie runs on, as T3 Code's "Open
  in" does, and only from that computer: from another device it says so;
  and Undo last change, with how many files the last turn that changed any
  changed, which runs `/undo`. What `/undo` says is a quiet note in the
  conversation, as the terminal shows it; like the terminal's, the note is
  not kept. A session with no project says so. In a folder not in git,
  Changes in this session counts the lines the session's turns changed,
  from undo's copies, and opens the diff on the whole session.
- **Version control,** in a git repository:
  - The branch, whose caret lists the repository's recent branches; picking
    one asks evie, in words, to switch the folder to it.
  - The branch's pull request, when it has one, as T3 Code's row: its
    title, which opens it on GitHub, in the colour of how it stands; its
    number; its checks summed up, such as "2 of 5 failing", which open a
    menu of each check, with how it stands and a link to its details; and
    Fix while any fails, which asks evie, in words, to fix the failing
    checks in this session. The `github` plugin reads it with the GitHub
    CLI, `gh pr view`, in the project's folder, as `gh` is signed in on the
    server's computer, when the card opens and each minute while the page
    is shown. With no `gh`, one not signed in, no remote on GitHub, or the
    plugin not loaded, pull requests cannot be read, and the card says so,
    and why, under the git button. `gh` is given only the server's path, home, settings
    and temporary folders, language and its own `GH_` and `GITHUB_`
    settings: no git settings and no keys. Only https links are kept, for
    the pull request and its checks. The page only reads it: nothing on
    GitHub is changed from it.
  - One git button, whose label follows how the branch stands, by T3
    Code's rule: Commit & push with work to commit on the main branch or a
    branch with a pull request open; Commit, push & PR with work to commit
    on a branch with none, or commits that lead to a pull request; Commit
    when the branch is behind its remote or there is nowhere to push; Push
    with commits to push, which for a branch with no upstream are those no
    remote branch has, as for a main branch the remote has never had. On a
    branch, a label that would open a pull request waits until the pull
    request is read; where pull requests cannot be read, the button only
    pushes. When there is nothing it can do, it says why when pressed or
    pointed at. Its caret opens Commit, Push and Create pull request, each
    off, saying why, while it does not apply. Each sends evie
    a request in words, as a message in the session, such as "Commit the
    changes in this session with a short message, and push this branch to
    its remote."; evie carries it out as it would if you had typed it, and
    asks before pushing, as it always does. A request on its way, and one
    sent that evie has not started on, keeps the git button, its menu, Fix
    and the branch's menu off, so that it is sent once. A request that
    could not be sent says why under its row.
  - Changes against the branch it is compared with, its pull request's
    base or else the main branch, such as "vs main +71 −6", or on the main
    branch, the work not committed, "Uncommitted"; it opens the diff on the
    same, which starts from the same place. A branch that cannot be
    compared says why, as the diff does. Every changed file in the
    project's folder is counted, with `git diff --numstat`, and files git
    does not track as added. How the branch stands is read from the folder
    with git, which reaches no other computer and writes nothing, when the
    card opens, when a turn ends, when the window has the focus back and
    each minute while the page is shown.
- **Lineage:** while one of the session's subagents' conversations is open,
  the session itself, which leads back to it, with how it stands: Working,
  Needs you, or Waiting while it waits on its subagents; each subagent working, with
  how long its round has run, such as "Working · 31s", and Stop; and those
  that have ended, folded away under Previous agents, each with how it
  ended (Done, Failed, Stopped, or Ended for one with no round since evie's
  server last started). Each list shows six at first, then "Show N more",
  twelve at a time, as T3 Code's. Picking one opens its conversation in the middle,
  read only, under its name, with a line below that leads back to the
  session; while it works, what it adds is read every second.
- **Background work:** each job running in the background, with its
  command, and Stop, which is `/jobs stop`; and those that have ended,
  folded away under Earlier background work, as Previous agents is. Each
  opens to its output, which keeps up while it runs; one open as it ends
  stays open, and its fold opens for it.

Each button is off while it does not apply, and says why: the session still
opening, evie working on a turn, or the line down. With a subagent's
conversation open, a press goes back to the session's own first, so that
what it says is in view.

**Choose what to show** opens a menu of the card's items: Project and
folder, Undo last change, Branch and pull request, Git actions, Changes,
Lineage and Background work, each ticked while it shows, and Reset to
default. The browser remembers what differs from the default. With nothing
chosen, the card offers the menu instead.

**Pinned,** the default, the card stays open beside the conversation, which
keeps clear of it, while the conversation has the room; at 1100px wide or
narrower with the session list showing, or with the diff open, it stays
open over the conversation, with a shadow, as drawn. **Unpinned,** or in a
window with less than 160px of room under the card's top, it opens over
the conversation from the button at the right of the conversation's
header, or with Cmd+Option+B (Ctrl+Alt+B elsewhere), and closes on a click
outside or Escape. The same button and keys close and open the pinned
card. Taller than its room, the card drops to T3 Code's compact density,
without the project, the git button, the lineage and background work,
and below that to its essential one, without the folder too; the lineage
and background work, which scroll as they grow, are not counted. The browser remembers whether it is pinned, and whether the pinned
card was closed. At phone width it is a sheet over the page, from the
right, which goes once another session is picked.

What evie changed is shown as diffs, as T3 Code shows them:

- **In the conversation,** a completed turn that changed files with evie's
  file tools ends with one quiet line, such as "3 changed files +12 −4",
  which opens to list them, and Open diff, which opens the diff on that
  turn. A file in the list opens it at that file. The counts leave out a
  file whose lines were not counted, such as a binary one, which shows
  none.
- **The diff** is a tab of the right panel, beside a tab for the branch's
  pull request when it has one. It opens from that line, from the session
  details' Changes, from the button at the right of the conversation's
  header, or with Cmd+D (Ctrl+D elsewhere), which also closes it. In a
  window wider than 980px it is a column beside the conversation, 540px at
  first and as wide as you drag its left edge, which the browser remembers;
  in a narrower one it is a sheet over the page from the right.
- **Its toolbar** holds, on the left, the menu that picks what to compare,
  and on the right the lines added and removed, Refresh, stacked or side by
  side, wrapping long lines, the file tree, and Close.
- **The menu** picks what to compare: the latest turn; each turn that
  changed files, newest first, with when it ended; the whole session, each
  file from before the session first changed it to now; and in a git
  project, Uncommitted, every change since the last commit, and This
  branch, the branch against the branch it is merged into: its pull
  request's base when it has one, which the server asks the `github`
  plugin for, and otherwise the repository's main branch. Both include
  files git does not track and does not ignore, and count changes as git
  does: a file taken out of git is removed, though it is still in the
  folder, and a submodule moved to another commit says "Submodule
  changed". In a folder known not to be in git, both are listed but cannot
  be picked, and say why; when that is not known, the server is asked, and
  what it says is shown in words.
- **Each file** has a header that stays at the top while the file scrolls
  past: a caret, an icon for how it changed, its path, and the lines it
  removed and added. Pressing it folds or opens the file. Every file starts
  folded but the one the diff was opened at, or else the first with a
  diff. The server sends each change with three unchanged lines around
  it; the lines between are folded under a band that says how many.
  Pressed, the band opens them, once the server has sent the file's whole
  contents, before and after (`read_file_sides`); a file changed since its
  diff was read says so above the diff, and to refresh it, instead.
- **It is drawn as T3 Code draws it,** with the same library,
  `@pierre/diffs`, which the page loads the first time it shows a diff:
  only the lines in sight are drawn, stacked or side by side, which the
  browser remembers, and always stacked at phone width, with long lines
  wrapped or scrolled sideways. The code is coloured by its language, in
  workers apart from the page, and a line longer than 1,000 characters is
  left plain. A line added or removed lies on a faint green or red, with a
  bar of that colour at its edge, dashed for one removed.
- **A file with no diff to draw** is listed after the others, saying why:
  the file is, or was, empty; binary; too large to show; filtered; a
  submodule changed; changed outside evie, with no copy kept of what the
  turn left; or not compared, because it took too long, or because one
  comparison reads at most 300 files, a few at a time, and it was past
  them.
- **The file tree** beside the diff lists its files by folder; a file in
  it scrolls to that file. It is as wide as you drag its edge, and can be
  hidden, which the browser remembers. At phone width it takes the diff's
  place while it shows.
- **The changes by turn come from undo's copies,** which it keeps of each
  file before a turn's file tools change it. What a turn left in a file is
  the file as it is now, or the copy the next turn that changed it kept.
  A turn undone is gone from the list, with its copies. Without the undo
  plugin, the turns' changes are not kept, and the diff says so. What a
  shell command changed is not in a turn's changes; in a git project it is
  in Uncommitted. A turn that has ended changes no more, so what it changed
  is worked out once and kept; the whole session's files are worked out
  only when the page asks for them, and lines are compared on a
  thread of the server's own, so that a long comparison holds nothing up.
- **A repository is read without running anything it names.** A
  repository's own settings can name commands for git to run, and one just
  cloned is not to be trusted with that. Every git that evie runs, in its
  core and its built-in plugins (`src/core/git.ts`), is given settings that
  turn each of them off, through its environment (`GIT_CONFIG_COUNT`),
  which git reads over every settings file: the file system monitor
  (`core.fsmonitor`); hooks (`core.hooksPath`, and no index written,
  `GIT_OPTIONAL_LOCKS`, which would run one); the pager; an external diff
  (`diff.external`, and `--no-ext-diff` for a diff driver's command); text
  conversion (`--no-textconv`); signing (`commit.gpgSign`,
  `tag.gpgSign`); upkeep in the background (`gc.auto`,
  `maintenance.auto`); and for a command that reads the folder's files,
  every filter its settings name, whose clean, smudge and process commands
  are each set to none, whatever the filter's name. Nor does git reach
  another computer: no transport (`protocol.allow`), no fetching of what a
  partial clone lacks (`GIT_NO_LAZY_FETCH`), no credential helper, ssh
  command or password program (`credential.helper`, `core.sshCommand`,
  `core.askPass`), and never a prompt (`GIT_TERMINAL_PROMPT`). The git
  that `gh` runs is given the same. The one exception is the sync plugin's
  push to its private backup repository, which keeps the transports,
  credential helper and ssh command the settings have, and everything else
  above. Your own settings give that push its credentials; it pushes
  nothing while the backup repository's own settings set one of the
  settings the sync plugin lists, such as a credential helper (an empty
  one aside), an ssh command, another address to push to, a proxy, or
  another file of settings to read. The files themselves are read by the server, not through git, and
  the size of each is looked at first: one over 1 MB is never read. A file
  git filters, as git-lfs does, is not compared, since its filter is not
  run, and says "Filtered file, changes not shown". One answer spends at
  most 20 seconds comparing lines, and one file at most 15: the files left
  say they took too long, and are not compared.

**Shortcuts** work as T3 Code's, wherever the focus is, but for Escape and
F2, which belong to the message box and the conversation, and wait while
a window such as the settings page is open. "Mod" is Cmd on a Mac and Ctrl
elsewhere. They are one table, in `web/src/shortcuts.ts`, which the
page's handlers, the command palette and the buttons' titles all read;
they cannot be changed yet.

| Keys | What they do |
|---|---|
| Mod+K | Open or close the command palette |
| Mod+Shift+O, or Mod+N | A new session: where New session makes one, or in a workspace, in its side chat |
| Mod+Alt+[ and ], or Mod+Shift+[ and ] | The session above or below in the list, in the session view |
| Mod+B | Show or hide the session list, in the session view |
| Mod+Alt+B | Show or hide session details |
| Mod+D | Open or close the diff |
| Mod+Shift+M | Pick a model |
| Escape | Stop evie, in the message box |
| F2 | Rename the session: in the list while it is shown, and otherwise in the header over its conversation |

Most browsers open a new window on Mod+N, and a Mac's browsers move
between tabs on Mod+Shift+[ and ], before the page hears of them, so
the first keys in the table are the ones the browser always passes on,
and T3 Code's work where it passes them on too, as in a window of its own.
Mod and a digit is left to the browser, which moves between its tabs with
it, as T3 Code leaves it outside its desktop app. On a phone, with no
keyboard, the palette shows no keys.

**The command palette**, Mod+K, works as T3 Code's. It opens on the
actions and the twelve sessions last active, and what you type searches
the actions, every session and every project: a name that is what you
typed first, then one that starts with it, then a word that starts with
each word typed, then one with it inside a word, and on an equal match, a
command before a session. Each row shows its shortcut. The actions are a
new session, here or in a project; the previous and next session; the
settings; the theme; showing and hiding the list, the card and the diff;
settling the session or bringing it back, moving it, and renaming it; and
Compact now, Stop and Pick a model, while they apply. In a workspace they
are those of its side chat's session, but for renaming it and picking its
model, which the small chat does not do, and it has no list, card or diff. A
session opens it, and a project opens its workspace, with its folder
beside it, cut at its start. A row
with a list of its own, such as the theme or Move to, opens it in the
palette, and Backspace in an empty search goes back. Up and Down move,
Enter runs the row raised, and Escape closes it.

**Notifications** work as T3 Code's. evie tells you when a session comes
to need you, as when a question or a permission prompt waits ("Needs
you"), and when a turn finishes ("Finished") or fails ("Failed", with what
went wrong), with the session's name:

- **With the window in front,** a notice of another session shows at the
  top of the page, with Open. A notice that a session needs you stays
  until it no longer does; the others go after a few seconds.
- **With the window behind others,** the system's notification says it,
  if you have turned them on, and clicking it opens the session.
- **Of the session you are looking at,** nothing is said.

In the settings, Notifications are Off at first, or Notifications, Sound
only, or Notifications with sound; the browser is asked to allow them only
when you turn them on, and a choice of them is kept only once it does,
as T3 Code's; otherwise the setting says how to allow them. A browser shows them only for a page at a secure
address, such as `127.0.0.1` on this computer, so not over plain `http`
from another device. Notices in the page are a switch of their own, on
until you turn them off. The system's notifications are silent, and
evie's own sound plays only in the modes that have one, as T3 Code's;
browsers let a page play a sound only once it has been used, so choosing a
mode with a sound readies it at once. Everything is read from what the server sends every
page already, and for a turn's end, the session's latest events, so the
server does nothing more for it.

Below 640px wide, as on a phone, the controls grow a step, as T3 Code's
do: buttons and text boxes are 36px tall, small buttons and menu items
32px, their words 16px and their icons 18px. What is typed into a text box
or the message box is 16px there too, so that the phone does not zoom the
page in when one is tapped. The text, the rows and the headers stay as at
desktop width. On a touch screen each button also reaches an unseen 44px
round itself, so that a finger finds it although it looks small.

**A workspace** is a project's home, opened from its icon or name in the
session list or the session details. The page then has three other columns:

- **The strip,** on the left: the way back to the session list, a line,
  then a tile for every workspace, the one open with a line in its colour
  on its own edge, and a dot on any with a session waiting for you (its
  name says Needs you). Below them, Add a workspace, a dashed tile, and at
  the bottom, the settings.
- **The workspace,** with its header in one row: its icon and name, its
  dashboards as tabs after them once there are dashboards, then Storage,
  Sessions and Memory, which is to come and says Later, and at the end the
  button that hides and shows the side chat. Pressing the icon or the name
  opens a picker of its icon and colour: the six project colours, and a
  search over about sixty of Phosphor's icons that suit a project. A colour
  applies at once; an icon applies and closes the picker. Both are kept in
  evie's database, and every tile on the page shows them.
- **Sessions** lists the project's sessions, each with Needs you, Working,
  Done or how long ago it was active. Picking one opens it in the session
  view.
- **Storage** lists the files in the project's `.evie` folder as a tree,
  every folder open, each row with its icon, its name and how long ago it
  changed, up to 500 to a folder; a folder folds by its row's caret. At
  its foot it says whether the folder is backed up. A text file or an
  image opens in the workspace, read only. The server reads only
  inside the `.evie` folder: a path that leaves it, by `..` or through a
  link, is refused, and so are the folder's own git repository and a file
  over 1 MB. So is the whole of a `.evie` that is itself a link to another
  folder, or that is, holds, or is inside evie's data folder, which holds
  your keys, as for a project at your home folder. The files attached to
  messages are here, under `attachments/`.
- **The side chat,** on the right, is a small chat with evie about the
  workspace. Its header says evie over the chat's title, with New chat,
  Open this chat full size, which opens it in the session view, and Close
  the chat. It shows the conversation in short: your messages, evie's
  words, and each run of evie's work as one quiet line in plain words, as
  T3 Code's work log says it ("Edited usage.html", "Read 2 files and ran
  1 command"), with background work finishing and other notes the same
  way; notes written for the model are left out. A command such as
  /compact shows a line while it runs. A question waits at its end, its
  first choice the main one, and while evie works a line says so, with
  Stop. Under it is a small box, where Enter sends and Escape stops evie.
  Scrolling up reads the earlier conversation. It shows the session the
  address keeps, or until one is kept, the workspace's latest, or a new
  chat in its project when it has none; New chat, or New session from the
  keys or the palette, starts a new one there. Hidden, it stays hidden in
  this browser until the header's button or New session shows it again,
  and while it is hidden nothing in it counts as looked at.
- **New workspace**, from Add a workspace in the strip, New workspace… in
  the New session button's menu, or a session list with nothing in it yet,
  asks for a name, a folder,
  suggested in your home folder as `~/` and the name, and whether to back
  up its `.evie` folder to a private repository (see "Workspaces"). A
  folder already there, such as a code repository, is added as a project
  the same way. What the server refuses, it says in its own words.

The address keeps where you are, as `#workspace=<project id>&tab=storage`,
with `&chat=<session id>` for the side chat's session and `&file=<path>`
for a file open in Storage. Where the workspace and the side chat no
longer fit side by side, under 880px beside the strip, as on a phone or
an upright tablet, the strip stays a column on the left and the side chat
stacks below the workspace, a screen high. The header stays one row: the
name is cut down to 96px at the least, and then the tabs scroll sideways.

### Attachments

A file attached to a message is kept where its session works:

- **In a project**, in its `.evie` folder, under the day it came, such as
  `.evie/attachments/2026-11-22/plan.pdf`, so that it shows in the
  workspace's Storage, and is backed up with the folder when the project
  syncs. It stays when the session is deleted, as the project's other
  files do.
- **With no project**, in the data folder, under the session, such as
  `attachments/<session id>/2026-11-22/plan.pdf`, where evie's tools may
  read it, as they may read the session's saved output. Deleting the
  session deletes them. So does a session whose project's `.evie` folder
  Storage would not show (see above).

No file is ever replaced: a name taken already that day gets a number,
such as `plan-2.pdf`. A name with a folder in it, a file over 10 MB, and a
folder that leads through a link elsewhere are refused.

A message keeps each file by its full path, so a session moved to another
project still finds the files attached before the move. Moving a
project's folder, though, leaves its earlier messages pointing where the
files were: their thumbnails no longer show, and evie is told paths that
are no longer there.

The message records each file's path, name, type and size. When the
session's model takes images, as OpenRouter's list of models says, an
image attached (PNG, JPEG, WebP or GIF, whose base64 is at most 5 MB, and
no more than 8,000 pixels on a side) goes to it as an image in that
message: the newest four in the conversation, at most 16 MB of base64
together. Any other file, older or larger images, and every image for a
model that takes none, are named to the model by their paths, for it to
read with its tools; a session switched to such a model is given the
paths of its earlier images too, and a summary of the conversation is
written from the paths alone. When the list of models cannot be read, the
last answer for the model holds. A request the provider refuses while it
carries images is sent again with each image by its path, and those
images are not sent again while the server runs, so that a session is
never stuck on an image.

The server takes a file at `POST /attachments?session=<id>&name=<file
name>`, with the file as the body, from a client signed in as for the
WebSocket, and answers with the attachment as JSON. What a file is comes
from its bytes, not from what the browser says: a PNG, JPEG, GIF or WebP
picture by its signature, and anything else is a plain file, never an
image. A message names its files by the paths the server gave back, and
the server reads each one's name, kind and size from the file itself. `GET /attachments?session=<id>&path=<its path>` gives back an image
attached to one of the session's messages, to show it; nothing else.

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
inside a git repository other than at its top, or that is, holds or is
inside Evie's data folder is refused. `evie new` says whether the project is trusted and
whether its `.evie` folder is backed up. The web app makes one with New
workspace, which takes a folder from `~/` as the server's home folder, and
can also keep the `.evie` folder on this computer (`"sync": "none"`). A
name of only dots is refused. Then run `evie` in that folder
to start working there.

Running `evie` in a folder makes it a project too, or the git repository
it is in, but never one that is, holds or is inside Evie's data folder,
such as the home folder, which holds `~/.evie`, nor one whose `.evie` is a
link to such a folder: a project there would take Evie's own settings file
for its own. The session started there belongs to no project, and says so
as it opens; `evie -c` there starts a new one. A folder inside a home
folder kept in git is a project once it has a git repository of its own.
A project an earlier Evie made at such a folder is no project now: its
sessions open with no project and say why, and none can be moved into it.
Nothing of it is deleted.

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
  every push checks that the remote is still that repository, that every
  address it pushes to is that repository's, that GitHub still says it is
  private, and that those settings set none of a list of settings that
  would have git run a command, push somewhere else or read settings from
  elsewhere (`STOPS_A_PUSH` in `src/plugins/sync.ts`); otherwise nothing
  is pushed, and `/sync` names the setting. A minute
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
| `auto` | Allowed | Allowed | Allowed, except the dangerous ones below, which ask | Reading is allowed; changing a file asks, except in the temp folder |

In `auto` mode evie runs any command, wherever it reads, and asks only
before the dangerous ones:

- ones that leave the computer or act for you online: pushes, publishing,
  `gh` changes, POSTs, `ssh` and its kin, deploys and cloud tools;
- ones that delete, move or overwrite files outside the project and the
  temp folder (`/tmp` and `$TMPDIR`), with links followed: `rm`, `rmdir`,
  `unlink`, `shred`, `truncate`, `mv`, `cp` onto a file, `>`, `>>` or
  `tee` to a file, `dd`, `find … -delete`, `tar` extracting, and `git
  clean`, `reset --hard`, `checkout --` or `restore` run in another folder;
- `sudo` and its kin, and ones that change system settings, such as
  `launchctl`, `defaults write`, `pmset`, `diskutil` and `shutdown`.

What is refused in every mode stays refused in `auto`, `sudo` among it.

Sessions start in `manual`, or in the mode that `permissions.mode` in
`evie.json` sets. `/mode` changes the mode for the session it is typed in,
and the session keeps it when evie starts again.

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

An "Always" answer for a command that reaches outside the project allows
exactly that command, since a rule for a kind of command does not reach
outside it. An "Always in this project" answer is saved as a rule in the global
`evie.json`, under `permissions.projects` and the project's folder. A
trusted project's own `.evie/evie.json` can set `mode` and `rules` for that
project; its rules come after the global ones.

When nobody can be asked, as when input is piped in, what would be asked is
denied.

## Settings

Evie keeps its state in `~/.evie`. Set `EVIE_HOME` to use another folder.
Two files in that folder hold its settings.

**`.env` is for keys.** One `NAME=value` per line. Evie reads a key and
never writes it anywhere else; the web app's settings write a key you set
there to this file, which only your user can read.

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
a list, the fifteen built-in ones load:

| Plugin | What it adds |
|---|---|
| `files` | `read`, `write`, `edit`, `patch`, `grep` and `glob` |
| `shell` | `shell` |
| `jobs` | `job_stop`, and the `/jobs` and `/background` commands |
| `permissions` | The rules that allow, deny or ask before a tool runs, and the `/mode` command |
| `undo` | The copies kept of files before they are changed, the `/undo` command, and what each turn changed, for the web app's diffs |
| `models` | What each model family is given, and the `/model` command |
| `ask` | `ask`, the model's way to put a question to you |
| `subagents` | `subagent` and `subagent_send` |
| `skills` | `skill`, the list of your skills, and the `/skills` command |
| `instructions` | The owner's and the project's instruction files |
| `web` | `web_search` and `web_fetch` |
| `sync` | The backing up of a project's `.evie` folder, and the `/sync` command |
| `naming` | A name for each session, written by a small model from its first message |
| `github` | The pull request of a project's branch, read with the GitHub CLI for the web app's session details and diffs |
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
    "github",
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

`addPlugins` lists plugin files, by their paths, to load after the list:
after `plugins`, or after the built-in plugins when there is no `plugins`.
The web app's settings add a plugin file here and take it off again, so
the list of built-in plugins is left as it is. An entry that is not the
path to a plugin file stops the server at start, as a bad entry in
`plugins` does.

```json
{ "addPlugins": ["/path/to/my-plugin.ts"] }
```

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

A plugin can also say which keys it reads from `.env`, each with what it
is for in one line, and the web app's settings list them for you to set:

```ts
export default {
  name: "weather",
  keys: [{ name: "WEATHER_API_KEY", about: "Reads the forecast from the weather service." }],
  setup(host) { /* ... */ },
}
```

For now Evie hands the keys in `.env` to its built-in plugins only. A plugin
file reads a key it lists from the environment the server runs in, and only
from there: under the login service that environment holds just the data
folder and `PATH`, which is why `evie service install` names only the
built-in plugins' keys.

Through the host a plugin can add to six lists:

| List | What an entry is |
|---|---|
| `host.tools` | Something the model can call. Its `run` returns the text the model reads. A problem the model can fix is returned as that text; `run` throws only for a bug. Work that should carry on after the call returns is handed over with `context.jobs.start`: Evie lists it, can stop it, and tells the model when it ends |
| `host.providers` | A way to reach a model |
| `host.profiles` | What one family of models is given: a system prompt and its tools for changing files |
| `host.commands` | A command you type starting with a slash, such as `/model`. A command can tell the model something with `context.tell`, which adds a note to the conversation |
| `host.pullRequests` | What reads the pull request of the branch a folder is on: `read(folder)` gives it, or null when there is none or it cannot ask. The `github` plugin adds one, with the GitHub CLI; the first one added is used |
| `host.changes` | What keeps the files each turn changed, so that a client can show them as diffs: `turns(session)` gives each turn that changed files, with what each file held before the turn and what the turn left in it. `undo` adds one, from the copies it keeps; the first one added is used |

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
Bun's own bundler. There is no build step. `evie serve` builds the page as
it starts, which takes about half a second, and serves it from memory.

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
  memory, minified. The build runs in a short-lived process of its own
  (`src/server/build-web-app.ts`), with the server's own Bun, started in the
  repository's folder: Bun's bundler keeps its working memory, about 300 MB,
  for as long as the process that ran it lives, and the server runs all the
  time. Bun's other way, handing the page to `Bun.serve` with
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

The design system's colours are copied from its `tokens.json` into
`web/src/tokens.css`, one set per theme. Its sizes follow T3 Code's
density, at every width, phone included: 14px text, 12px details, 28 to
32px controls and 24px rows in a turn's work log, where the board first
had 15px text and 40 to 44px controls; below 640px, T3 Code's own
breakpoint for its sizes, its controls grow a step, as T3 Code's do. Its fonts, Geist and Geist Mono, and
its three themes are its own. The sizes are in Tailwind's theme in
`web/src/styles.css`. Tailwind's own defaults are cleared, so each class
names a token, such as `bg-bg-side`, `text-text-muted`, `text-caption` or
`p-7`, and its spacing scale holds only the steps below, so a class off it,
such as `p-9`, is not made at all. The fonts are carried in the
stylesheet, so the page asks no other site for anything.

Below 640px, the sizes of the controls change, and nothing else: `control`
is 36px, `control-sm` 32px, the words on a button or menu item (`control`
text) 16px and its icon (`icon-control`) 18px, as T3 Code's are. The text
typed into a text box, `field`, is 16px, below which iOS zooms the page in
when a text box is tapped, and the page's side padding, `gutter`, is 12px.
Text, rows, headers, chips and the work log keep their sizes, as T3 Code's
do. A test checks that every text box in the web app takes `field`. The
layout's own breakpoint, where the session view's columns stack and the
session details become a sheet, is
768px (`wide`), as T3 Code's sidebar's is.

**Text.** Sans-serif (Geist) but for `code`, in Geist Mono:

| Name | Size and line | Weight | For |
|---|---|---|---|
| `caption` | 12px on 16px | 400 | Details beside or under a title: a project's name, a branch, an age; a menu's heading; notes and refusals |
| `body` | 14px on 20px | 400 | List rows, a turn's work log, paragraphs |
| `prose` | 14px on 1.625 (about 23px) | 400 | A reply and the owner's message, on T3 Code's looser line for reading |
| `field` | 14px on 1.625; 16px below 640px | 400 | What is typed into a text box, the message box or a search |
| `control` | 14px on 20px; 16px on 24px below 640px | 400 | The words on a button, a chip, a menu item or a row of the palette, and a form's labels, which take semibold |
| `label` | 14px on 20px | 600 | Titles: a session's in the list and in a header, a workspace's, a setting's, a question's; the name `evie` at the top |
| `heading` | 18px on 24px, −0.01em | 600 | A reply's second heading |
| `title` | 16px on 24px, −0.01em | 600 | A reply's third heading |
| `page-title` | 20px on 28px, −0.01em | 600 | The one title of a dialog or an empty page; a reply's top heading |
| `figure` | 24px on 32px, −0.02em | 600 | Big numbers on dashboards, when there are any |
| `code` | 13px on 20px | 400 | File paths, commands, diffs and tool calls |
| `detail` | 13px on 16px | 400 | The rows of the session details, as T3 Code's details card |
| `detail-heading` | 11px on 16px | 500 | The headings over the session details' sections, and its name |

**Spacing,** on a 4px grid:

| Name | Value | For |
|---|---|---|
| `space-0.5` | 2px | T3 Code's half step: between the session details' heading buttons |
| `space-1` | 4px | Between a label and its value; inside a menu, around its items |
| `space-1.5` | 6px | T3 Code's half step: above and below a menu's heading; the room a row's `···` reaches into |
| `space-2` | 8px | Between an icon and its text; a menu item's sides; between a turn's steps; the session list's sides |
| `space-2.5` | 10px | T3 Code's half step: the sides of a row of the session details, and the room between its icon and its words |
| `space-3` | 12px | Inside small controls; the sides of a row and a header; a setting's top and bottom |
| `space-4` | 16px | Inside the message box; between turns; above and below the conversation |
| `space-4.5` | 18px | T3 Code's half step: the line of a pull request's title in the session details |
| `space-5` | 20px | Where a column's text starts: the name at the top of the list, a list's indent in a reply; under the message box at desktop width |
| `space-6` | 24px | Inside a dialog; the settings page's distance from the window's edges, and the sides of its settings; between tabs |
| `space-7` | 28px | The height of a chip under the message box's text, at every width, as T3 Code's |
| `space-8` | 32px | Between a setting and its control; between sections; above and below an empty page's words |

**Corners:**

| Name | Value | For |
|---|---|---|
| `radius-chip` | 6px | A small control inside a row of the session details: its icon buttons, its checks and Fix |
| `radius-sm` | 4px | Menu items and the rows of the palette and pickers; badges; an image's thumbnail |
| `radius-md` | 8px | The default: buttons, text boxes, menus, popovers, notices, selected rows |
| `radius-lg` | 16px | The message box and the top of its drawers, the owner's message and a queued one, dialogs, the settings page and the palette |
| `radius-full` | 9999px | Pill buttons such as answer choices, the round send and stop buttons, status dots |
| `radius-tile` | 25% | A project's icon tile's corners, a quarter of its size at any size |

**Sizes,** each by a name of its own, such as `h-control` or `w-session-list`:

| Name | Value | For |
|---|---|---|
| `control` | 32px; 36px below 640px | Buttons, text boxes and icon buttons; the send button, and Stop in its place |
| `control-sm` | 28px; 32px below 640px | Small buttons: the paperclip, the buttons that show and hide the list and the card, a row's `···`, a notice's; the items of a menu, the palette, the `/` and `@` menus and the model picker |
| `row-height` | 32px | A row in a list: a file in Storage, a file's header in the diff, a row of the session details, a page in the settings' column; at the least, a session in a workspace |
| `work-row` | 24px | A row of a turn's work log: a run of calls, each call, Thinking, Worked for, a note; and the column its icon sits in |
| `header-height` | 52px | Every column's header: the list's, the conversation's, the card's, the diff's, a workspace's and its side chat's |
| `gutter` | 20px; 12px below 640px | The page's side padding: the conversation, the message box, a workspace, the sign-in page |
| `touch-min` | 44px | The least a finger aims at: on a touch screen, each button reaches this far round itself, unseen |
| `icon-xxs` | 11px | The icon on a project's tile in the session details, and the square of their Stop buttons |
| `icon-xs` | 12px | Small marks beside text: the caret that opens a group of the list, the spinner, the square in Stop |
| `icon-caret` | 14px | The caret that opens a turn's work or a note, as T3 Code's |
| `icon-sm` | 16px | Icons beside text, in rows and the palette |
| `icon-control` | 16px; 18px below 640px | An icon on a button or a menu item, and a form's checkbox |
| `icon-md` | 20px | The icon on a workspace's tile in the strip, and on the strip's way back |
| `rail-width` | 64px | The strip of workspace tiles |
| `tile-strip` | 44px | A workspace's tile in the strip, and the strip's way back, Add a workspace and settings buttons |
| `tile-header` | 28px | A project's tile in a workspace's header |
| `tile-row` | 18px | A project's icon tile in a session row and the conversation's header |
| `row-inset` | 10px | Between a session row's edge and its words, so they start 18px from the list's edge, as drawn and as T3 Code's |
| `tile-row-icon` | 12px | The icon on that tile |
| `side-chat-width` | 360px | A workspace's side chat |
| `session-list` | 256px | The width of the session list, as T3 Code's sidebar |
| `details-card` | 280px | The width of the session details, as T3 Code's details card |
| `details-top` | 64px | How far below the conversation's top the session details float: 12px under its header |
| `details-height` | the column, less 76px | The tallest the session details grow before they scroll, 12px above the column's foot |
| `details-clear` | 324px | What the conversation keeps clear beside the pinned session details: the card, the 12px at its side and 32px between, as T3 Code's |
| `details-menu` | 236px | The width of the menu that chooses what the session details show |
| `detail-indent` | 36px | Where a row's words start in the session details, past its icon: a pull request's second line |
| `card-sheet` | `min(88vw, 448px)` | The width of the session details as a sheet over the page, at phone width |
| `conversation` | 736px | The widest the conversation and the message box are, as T3 Code's |
| `call-result` | 256px | The tallest a tool call's result grows in the work log before it scrolls, as T3 Code's |
| `work-group` | `min(288px, 50dvh)` | The tallest an opened run of calls grows before its rows scroll, as T3 Code's |
| `palette` | 576px | The widest the command palette is, as T3 Code's |
| `palette-list` | `min(420px, 60dvh)` | The tallest the palette's rows grow before they scroll, as T3 Code's |
| `notice` | `min(360px, calc(100vw - 32px))` | The width of a notice at the top of the page |
| `menu` | 176px | The narrowest a menu is |
| `menu-wide` | `min(400px, calc(100vw - 32px))` | The widest a menu of sentences grows, such as the context meter's |
| `dialog` | 512px | The widest a dialog is, as T3 Code's |
| `settings-page` | 1280px | The widest the settings page is, as T3 Code's |
| `settings-page-height` | 960px | The tallest the settings page is, as T3 Code's |
| `settings-nav` | 240px | The settings page's column of pages and its search, as T3 Code's |
| `settings-rail` | 56px | That column at phone width: a rail of the pages' icons |
| `settings-header` | 48px | The settings page's header, with its breadcrumb and Close |
| `settings-content` | 896px | The widest the column of settings is |
| `settings-about` | 672px | The widest what a setting is for runs under its title |
| `settings-select` | 224px | The least a choice of a setting's is wide, as T3 Code's select |
| `settings-field` | 200px | A text box beside a setting |
| `sign-in-column` | 400px | The width of the sign-in page's column, and of text in the middle of an empty page |
| `workspace-name` | 96px | The least a workspace's name keeps in its header before its tabs scroll sideways |
| `workspace-columns` | 880px | A container width: the least room beside the strip for a workspace and its side chat side by side |
| `icon-picker` | 230px; 254px below 640px | The project icon picker: six 32px columns (36px below 640px), their gaps, its padding and its line |
| `icon-grid` | 220px | The tallest the picker's icons grow before they scroll |
| `composer-text-min` | two lines | The least the message box's text is tall, as drawn and as T3 Code's |
| `composer-text` | 208px | The tallest the message box's text grows before it scrolls, as T3 Code's |
| `composer-menu` | 288px | The tallest the `/` and `@` menus and the model picker above the message box are, as T3 Code's |
| `drawer-inset` | 22px | How much narrower than the message box its `/` and `@` drawers are, on each side, as T3 Code's |
| `drawer-tuck` | 17px | How far those drawers tuck under the box's top edge, as T3 Code's |
| `control-xs` | 24px; 28px below 640px | A small button inside a row: a queued message's Send now, edit and remove; those in the right panel's toolbar, on its tabs and on a turn's changed-files card, as T3 Code's |
| `icon-row` | 14px | The icon on such a button, and on a file's chip in the message box's text |
| `reset` | 20px | A setting's reset, after its title, and the clear button of the settings' search |
| `switch-knob` | 14px; 18px below 640px | The knob of a setting's switch, whose track is `switch-height` by `switch-width` round it, and which travels `switch-travel` |
| `picker-makers` | 144px | The model picker's column of makers |
| `attachment-chip` | 24px | An image's thumbnail in a chip above the message box's text |
| `attachment-shown` | 160px | The largest an image attached shows in the conversation |
| `image-open` | `min(90vw, 1200px)` | The widest an image attached shows opened |
| `image-open-height` | 80dvh | The tallest it shows opened |
| `job-output` | 240px | The tallest a background job's output grows in the session details before it scrolls |
| `diff-toolbar` | 40px | The right panel's diff toolbar, as T3 Code's |
| `panel-tab` | 144px | The widest a tab of the right panel grows |
| `panel-sheet` | `clamp(320px, 42vw, 448px)` | The right panel as a sheet over the page, at 980px or narrower |
| `panel-phone` | `min(88vw, 384px)` | The right panel as a sheet at phone width |
| `wide` | 768px | Phone width is below it: the session view's columns stack, the message box is one row until tapped, the settings and the diff fill the screen, a workspace's columns stack |

The strokes, `line-width` (1px, every divider) and `indicator-width` (2px,
the focus ring and the line under the selected tab), and the one shadow,
`shadow-float` (`0 12px 32px rgba(0, 0, 0, 0.5)`, only on what floats), are
the board's, as are the colours. A change, such as a caret turning, takes
150ms (`default-transition-duration`) and eases on `cubic-bezier(0.4, 0,
0.2, 1)` (`default-transition-timing-function`), Tailwind's own values.

The design system's tokens name no colours for a diff's lines.
`diff-added` and `diff-removed`, in `tokens.css`, are evie's own muted
green and red, the values Done and Failed have, and they mark only lines
added and removed (`diff-view.ts`): such a line lies on 11% of its colour,
its line number's column on 18%, with a 4px bar of the colour at its edge,
dashed for a line removed, and its number is the colour mixed with the
text. The code's own colours are lighter tints of the project colours, the
`syntax-` tokens. Counts such as "+3 −2" are in the same green and red,
and a count of nothing is quiet, in `text-muted`.

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
  and `main`. It is `.github/workflows/check.yml`: it sets up Bun 1.4.5
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
  names sessions, in `src/plugins/naming.ts`, rewritten in evie's words;
  how a pull request's checks are read and summed up, in
  `src/plugins/github.ts` and `web/src/pull-request.ts`; and how a key
  pressed is matched to a shortcut, in `web/src/shortcuts.ts`. The web
  app's layout follows T3 Code's. Its licence is in `LICENSES/`.

The web app's fonts are [Geist and Geist Mono](https://github.com/vercel/geist-font)
(SIL Open Font License 1.1), from the `@fontsource-variable` packages. Its
menus are adapted from [shadcn/ui](https://github.com/shadcn-ui/ui) (MIT),
and its icons are copied from [Phosphor](https://phosphoricons.com) 2.1.1
(MIT). Their licences are in `LICENSES/`.

## Licence

Not chosen yet. Until a licence is added, the code is published to be read,
and no permission to use, copy or change it is granted.
