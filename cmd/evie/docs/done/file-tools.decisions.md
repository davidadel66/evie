# file-tools — decisions

Shipped 2026-08-01. `read_file` (ungated) and `edit_file` (gated) in
`internal/tools/file.go`, tests in `internal/tools/file_test.go`.

Decisions taken during the build that the spec left open, plus the gaps
we knowingly shipped with.

---

## The `~/.finance` fence — removed entirely

The spec fenced `~/.finance` because the sqlite db holds Plaid access
tokens. That also made `merchantLookup.json` — the feature's *first
customer* — unreachable, so the directory came off the denylist.

Considered and rejected: fencing `finance.db` by name and leaving the
rest of the directory open. Verdict was to leave `~/.finance` fully
open.

**Accepted risk:** `read_file ~/.finance/finance.db` is now permitted.
It's a binary sqlite file so the model gets garbage rather than clean
tokens, and the 100KB cap will refuse it once the db grows past that —
but neither of those is a real defense. If bank tokens ever need to be
genuinely unreachable, add `"finance.db"` to `deniedNames`.

The `items`-table fence in `query_db`/`edit_db` is unaffected and still
the primary defense for tokens.

## Two threat models, two defenses

The load-bearing idea of the feature, restated because it's easy to lose:

- `read_file` is the **leaky** tool. It cannot damage the filesystem, so
  it isn't gated — but everything it returns flows into the conversation
  and on to the model provider. The denylist is its defense.
- `edit_file` is the **destructive** tool. It leaks nothing new, but it
  overwrites data. The approval gate is its defense.

Neither substitutes for the other, which is why the fence lives inside
`resolvePath` — where no tool can skip it — rather than in each tool.

## Symlinks — option (b), checked

`filepath.EvalSymlinks` runs after the denylist check and the result is
checked too, so a symlink pointing into `~/.ssh` is caught. Its error is
deliberately ignored: `EvalSymlinks` only works on paths that already
exist, and "nothing to follow" is the normal case for a path being
written.

## Atomic write — no fsync

`writeFileAtomic` writes a temp file in the target's own directory and
`os.Rename`s it into place. Same directory matters: rename across
filesystems isn't atomic and may fail outright.

`tmp.Sync()` before close, plus a sync of the directory after the
rename, would extend the guarantee from "survives a process crash" to
"survives a power loss." Skipped: a disk flush on every edit is a real
cost, and process crash is the risk that actually applies to a
personal CLI tool.

## Known gaps, shipped deliberately

- **CRLF files.** A file with `\r\n` line endings whose content the model
  quotes back with bare `\n` won't match, and `edit_file` will report
  "not found." Not worth solving until it bites.
- **No read-before-edit rule.** Claude Code requires a file to have been
  read in-session before it can be edited. That needs session state, and
  every func in `internal/tools` is currently stateless — a package-level
  map would be hidden mutable state in a package that has none. The
  approval gate stands in: David sees the before/after and declines
  anything invented. Revisit if a session ever gets a real state object.

  **Superseded in part (2026-08-01):** `bash` introduced a package-level
  `sessionCwd`, so `internal/tools` is no longer stateless. That does not
  revive this rule — see `bash.decisions.md`. The distinction that
  survives is *visible* state (the model can run `pwd`; the description
  says it persists) versus *invisible* state (a set of files-read that
  silently decides whether `edit_file` succeeds). The first is fine, the
  second is still declined.
- **Stray temp files on crash.** A crash between `CreateTemp` and
  `Rename` leaves a `.evie-*` file in the target's directory. No
  startup sweep; accepted cost of the pattern.
- **`stripLineNumbers` false positive.** A file where *every* line
  legitimately reads `<digits><tab><text>` — a TSV, say — gets its first
  column stripped when quoted into `old_string`. The all-or-nothing rule
  makes this vanishingly unlikely for source code, which is the target.
- **No file creation.** `edit_file` requires an existing file and an empty
  `old_string` is an error. Creation is a separate `write_file`, unbuilt.
  A tool whose blast radius flips between "surgical replacement" and
  "replace the whole file" based on an empty string is one the model will
  misfire at the worst moment.

## Smaller calls

- **`numbered` emits a trailing numbered blank line** for any file ending
  in `\n`, because splitting on `\n` yields a final empty element.
  Harmless; pinned in a test rather than silently trimmed.
- **The `read_file` description names the fenced locations** (`~/.ssh`,
  `~/.aws`, `~/.gnupg`, `.env`, shell rc files). It tells the model where
  secrets live, but it can't read them anyway, and knowing not to try
  saves a wasted turn.
- **`applyEdit` returns the 1-based line number** of the match so the
  success message can say *where* it changed something — the model
  verifies without re-reading the file.
- **`resolvePath` does two things** (resolve + fence) and the name only
  says one. Kept the name, documented both in the doc comment; the
  alternative was `safePath`.
