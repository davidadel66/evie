# bash — decisions

Shipped 2026-08-01. `bash` in `internal/tools/bash.go`, tests in
`internal/tools/bash_test.go`. No spec doc — the tool was designed in
conversation, so this file carries the reasoning.

Built once, then revised against Claude Code's own `BashTool`
(`~/code/claude-code/src/tools/BashTool/`, `src/utils/shell/`) to see
what a mature version of the same tool does differently.

---

## Ungated, and no path denylist

`bash` has no approval gate and no fenced paths.

The fence argument is the easy half: a shell walks around any path
denylist worth the name — `cat ~/.ssh/id_rsa` is one command, and so is
`base64 < ~/.aws/credentials`. A denylist on the `cwd` parameter would
have bought false confidence, not safety, so there isn't one. `cwd` gets
`expandHome` (so `~` works) and deliberately not `denied`.

The gate is the real decision, and it's David's: evie can run arbitrary
shell commands — `rm -rf`, `curl | sh`, anything — with nothing between
the model and the machine. The stated tradeoff was that a gate on a tool
this general is friction on every call, and that reading what evie does
is the control. Recorded here because it is the single largest security
decision in the repo, not because it is in doubt.

Consequence worth stating plainly: `read_file`'s denylist is now a
defense only against *accidental* leakage through the ungated read path.
Anything determined to read `~/.zshrc` can just run `cat`.

## A non-zero exit is a result, not an error

`grep` finding nothing, a failing test suite, a compile error — the model
needs the stderr *and* the status to act. Returning those as a Go `error`
would send them through the dispatcher's "tool call came back with error"
wrapper and bury the output that mattered.

So: combined output plus an `exit status: N` line, returned as normal
content. Go errors are reserved for "could not run it at all" — malformed
arguments, an unusable `cwd`, a timeout.

`CombinedOutput` rather than separate pipes, because interleaving stderr
with stdout in write order is how a human reads a failing build; separate
streams would stack every error after every line of normal output.

## The working directory persists — reversing a file-tools decision

`file-tools.decisions.md` recorded that `internal/tools` is stateless and
that a package-level map would be "hidden mutable state in a package that
has none." `bash` adds exactly that: a package-level `sessionCwd` behind a
mutex.

The reversal was deliberate, and Claude Code decided it the same way. It
appends `pwd -P >| <tmpfile>` to every command, reads the file back, and
calls `setCwd()` — so a `cd` in one call becomes the working directory of
the next. evie now does the same thing with the same mechanism.

Why the earlier reasoning doesn't hold here:

- The state isn't **hidden**. `pwd` reports it, the tool description tells
  the model it persists, and every command's directory is observable.
- It isn't **unsafe**. A shell that can `cd` can already reach anywhere;
  remembering where it went adds no capability.
- The alternative is real friction. A model working in one project for an
  hour would re-pass `cwd` on every single call.

Contrast with the read-before-edit rule, still skipped: *that* state would
be invisible to the model and would silently change whether a tool call
succeeds. Different kind of state, different answer.

Details: an explicit `cwd` starts that one call somewhere else and then
becomes the new session directory too. A remembered directory that has
since been deleted is discarded rather than handed to `exec`, which would
otherwise fail every subsequent command.

## Output is capped at 30k, with the rest spilled to a file

Initially built uncapped, on the reasoning that truncation loses
information. Claude Code caps at 30k characters (`BASH_MAX_OUTPUT_DEFAULT`)
*and* writes the full output to a file the model can read afterwards —
which is the option that makes the tradeoff disappear, and it was adopted.

The case for a cap: `find /`, `cat` on a log, or a verbose build can emit
tens of MB, and unlike `read_file` there is nothing to `Stat` in advance —
you only learn the size after the command ran. One such call doesn't just
waste tokens, it ends the session.

Over the cap, evie returns the first 30k characters plus
`[output trimmed: … full output saved to /tmp/evie-output-<pid>.txt]`.
The model reads the rest with `head`, `tail`, or `grep` — which it has,
because it has a shell. Note the file will usually exceed `read_file`'s
100KB limit, which is why the note names shell tools instead.

## Timeouts: 2 minutes default, 10 minutes maximum

Originally 60s/300s. Raised to match Claude Code (`DEFAULT_TIMEOUT_MS`
120_000, `MAX_TIMEOUT_MS` 600_000) after realising 60s is under the
runtime of an ordinary Go test suite or a cold build.

**`cmd.WaitDelay = 2s` is load-bearing and was a real bug, caught by a
test.** `exec.CommandContext` kills the shell on timeout, but a child it
spawned inherits the output pipe and holds it open, so `CombinedOutput`
blocks until that child exits on its own — a 1-second timeout on `sleep 5`
took the full 5 seconds to return. `WaitDelay` bounds the wait after
cancellation and closes the pipes.

A timeout returns a Go `error` with the partial output embedded in the
message, since whatever the command printed before dying is usually the
diagnosis. This differs from a non-zero exit on purpose: a timeout means
evie could not complete the operation, not that the command answered.

## Login shell plus a startup snapshot

Commands run as `$SHELL -l -c` (falling back to `bash`), so `.zprofile` /
`.profile` load and PATH edits are visible. A bare `bash -c` inherits none
of the user's environment setup, which makes half the tools on a
developer's machine invisible.

That alone misses the interactive-only half: aliases and functions live in
`.zshrc` / `.bashrc`, which a login shell never reads. Running an
interactive shell per command would be slow and noisy, so evie does what
Claude Code does — capture once, replay per command. See
`internal/tools/shellsnapshot.go`.

**Capture** (`captureShell`): one `$SHELL -i -l -c <dump script>` run with
a 10s timeout, writing `export PATH=…`, every user function, shell
options, and every alias to a temp file.

**Replay** (`runBash`): each command becomes
`source <snapshot> || true` then `eval '<command>'`, then the existing
`pwd` epilogue.

**Timing:** `tools.Warm()` from `main` kicks the capture off in a
goroutine at startup, so the cost lands during boot rather than mid
conversation. A `sync.Once` means a command arriving before it finishes
just waits, and a build that never calls `Warm` still works — `runBash`
triggers the same capture lazily.

### Three things here are non-obvious and each was load-bearing

**`eval`, not the command inline.** A shell parses an entire line before
running any of it, so an alias defined by the `source` on that same line
would not expand. `eval` forces a second parse where it does. Without it
the snapshot's aliases are inert.

**Source the rc files by name.** The first version relied on shell startup
semantics and came back with no aliases at all — `bash -i -l` reads
`.bash_profile` and *never* touches `.bashrc`. The dump script now sources
each candidate rc file explicitly, guarded by `[ -f ]`. `-i` is still
passed so rc files that gate their contents on `[[ $- == *i* ]]` still run.
A test against a throwaway `$HOME` caught this.

**`printf 'export PATH=%q\n' "$PATH"`.** Quoting `$PATH` for the shell
wrote the literal five characters instead of expanding them, so the
snapshot's PATH line was a no-op. `%q` rather than `%s` so a directory
containing a space survives the round trip.

**`SysProcAttr{Setsid: true}` — this one froze evie.** An interactive
shell does job control and tries to take ownership of the controlling
terminal. Spawned from a background goroutine, which is not the foreground
process group, that raises SIGTTOU against evie's own group and suspends
the whole REPL: launching evie printed
`[1] + 80300 suspended (tty input)` before it could read a single prompt.
`Setsid` puts the snapshot shell in a fresh session with no controlling
terminal, so there is nothing to fight over; stdio is detached for the
same reason. Only reproducible from a real terminal — the tests, which
have no tty, passed throughout.

### Measured on David's zsh

51KB / 1836 lines, 231 aliases available to commands. Sourcing it costs
**~9ms per command** (30ms with, 21ms without). Most of the bulk is zsh's
own `vcs_info` machinery rather than user code; Claude Code filters only
completion functions (`^_[^_]`) and evie matches that. Not worth
filtering harder for 9ms.

Failure is never surfaced to the model: a snapshot that times out or comes
back empty leaves `snapshotPath` empty and commands run under a plain
login shell. A missing alias is a degraded shell, not a broken tool.

## Smaller calls

- **The `pwd` epilogue goes to a temp file, not stdout**, so directory
  bookkeeping never pollutes what the model reads. The script captures
  `$?` first and re-raises it last, so the epilogue cannot change the
  reported exit status.
- **Epilogue lines are newline-separated, not `;`-joined** — a multi-line
  script or a trailing `#` comment in the command would otherwise swallow
  it.
- **`cwd` is a real parameter** rather than making the model write
  `cd x && …`, so the directory shows up as an argument instead of buried
  in a command string.
- **Not adopted from Claude Code:** they disable extglob before each
  command to stop malicious filenames expanding past their command
  validator. evie has no validator to bypass, so there's nothing to
  protect.
