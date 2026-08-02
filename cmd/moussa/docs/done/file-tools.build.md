# file-tools — build guide (tutor)

Companion to `file-tools.spec.md`. That doc says *what* and *why*; this
one says *where to start* and *in what order*, with pseudocode holes for
you to fill. No Go here on purpose — the typing is the point.

---

## Part 0 — the resolved open questions

The spec left four open. Answers, with the reasoning worth keeping:

**1. Creation is a separate `write_file`, later.** Not `old_string: ""`.
A tool whose behavior flips between "surgical replacement" and "replace
the entire file" based on an empty string is a tool the model will
misfire at the worst moment. Different blast radius deserves a different
name. `edit_file` requires the file to exist; empty `old_string` is an
error.

**2. No root jail. One resolve chokepoint instead.** Scoping edits to
the launch directory would break the first customer —
`~/.finance/merchantLookup.json` lives nowhere near a repo. The "noise
from other paths" worry is a *discovery* problem; it belongs to
`glob`/`grep` scoping, not here. So: accept absolute, expand `~`
yourself (Go does **not** — `os.Open("~/x")` looks for a directory
literally named `~`), treat relative as cwd-relative, and funnel
everything through one helper.

**3. Hardcoded patterns, blocking reads *and* writes.** Reading
`~/.zshrc` leaks keys; writing it is a foot-gun with no upside. The
denylist lives at the same chokepoint as path resolution, so no tool can
forget to call it.

**4. Numbered lines, always the whole file.** Claude Code reads up to
2000 lines with `offset`/`limit` escape hatches; moussa substitutes a
hard size cap. Under the cap you get everything, over it you get a clear
error. Silent truncation is the real danger and this design has none.
The cost is the numbering trap — see Part 2.

---

## Part 1 — the anatomy you're building

Every tool in this repo is exactly three things. Go read `db.go` before
you write anything; `query_db`/`edit_db` are the closest twins to
`read_file`/`edit_file` — same ungated-read + gated-write pair, same
"general tool with a fence" shape.

1. **A schema var** — `var editFileTool = openrouter.Tool{...}`. This is
   the entire contract with the model. It is prose, and it is the most
   important thing you'll write today.
2. **An execute func** — `func editFile(args string) (string, error)`.
   Takes raw arguments JSON, returns plain text for the model. Never
   prints. Never builds wire messages.
3. **One registry line** in `registry.go` —
   `{Schema: editFileTool, Execute: editFile, NeedsApproval: true}`.

All of it in `internal/tools/file.go`.

### Where to start (the actual answer to "where do I start")

Not at the schema. Not at the tool. **Start at the pure center and work
outward to the IO.** The order below is deliberately inside-out: each
stage is testable before the next exists, and you never debug string
logic and filesystem logic at the same time.

| Stage | What | Touches disk? | Model involved? |
|---|---|---|---|
| 1 | `applyEdit` + `stripLineNumbers` — pure string funcs | no | no |
| 2 | `resolvePath` + denylist | barely (`Stat`) | no |
| 3 | `read_file` end-to-end | yes | yes |
| 4 | `edit_file` end-to-end | yes | yes |
| 5 | atomic write + live fire | yes | yes |

This is a habit worth internalizing beyond this feature: **find the part
of the problem that is a pure function of its inputs, build and test
that first.** Here it's "given file content, an old string, and a new
string, produce new content or an error." That's the whole feature's
brain, and it needs neither a disk nor an LLM to verify.

---

## Stage 1 — the pure core (test-first)

Two functions, no `os` package anywhere in them.

### `applyEdit`

```
applyEdit(content, old, new string) (string, int, error)
    // returns the new content and the 1-based line number where the
    // replacement landed (for a useful success message)

    if old is empty            -> error: "old_string must not be empty; edit_file cannot create files"
    if old == new              -> error: "old_string and new_string are identical — nothing to do"

    count := number of occurrences of old in content
    if count == 0 -> error: "old_string not found in the file. Re-read the
                             file and quote it exactly, including whitespace."
    if count > 1  -> error: "old_string appears <count> times. Include more
                             surrounding lines so it matches exactly once."

    idx  := index of old in content
    line := 1 + (number of newlines in content[:idx])
    out  := content with the first occurrence of old replaced by new
    return out, line, nil
```

Idiom notes: `strings.Count`, `strings.Index`, and `strings.Replace` with
`n = 1` (not `ReplaceAll` — you've already proven uniqueness, and the
`1` documents the intent). Note that `strings.Count` with a non-empty
substring is exactly what you want; `Count` with `""` returns
`len(s)+1`, which is one more reason the empty check comes first.

Why return the line number rather than just `error`/`string`? Because a
tool that reports *where* it changed something lets the model verify
without re-reading the file. Cheap to compute, saves a round trip.

### `stripLineNumbers` — the numbering trap

This is the piece you said you wanted handled correctly, so here's the
whole problem stated plainly:

> `read_file` shows the model `    42\tfoo := bar`. The bytes on disk are
> `foo := bar`. If the model quotes what it saw into `old_string`, the
> match count is 0 and the edit fails.

Two defenses, and you want both:

**Defense 1 — tell the model.** The `edit_file` description must say the
line-number prefixes are display-only and must not appear in
`old_string` or `new_string`. This is your first real use of the
tool-description convention you sharpened last session.

**Defense 2 — repair it anyway.** Be liberal in what you accept:

```
stripLineNumbers(s string) string
    lines := split s on "\n"
    for every line:
        if it does NOT match ^[ \t]*\d+\t  -> return s unchanged   // bail out
    // every single line carried a prefix, so it's safe to strip
    for every line:
        cut everything up to and including the first "\t"
    return rejoined lines
```

The **all-or-nothing** rule is the important bit. If you strip
per-line, a legitimate `old_string` containing a line like
`3\tcolumn_header` gets silently mangled. Requiring *every* line to
carry a `spaces + digits + tab` prefix makes a false positive
vanishingly unlikely — real source lines don't all begin that way.

Call it on both `old_string` and `new_string` (a model that prefixed one
will have prefixed the other). Then never touch the strings again — do
**not** trim whitespace, ever. Leading indentation is significant
content.

Regex or hand-rolled? `regexp.MustCompile` at package level is the
readable choice for a pattern this fiddly, and it's the standard-library
tool for the job. Hand-rolling it with `strings.IndexByte` and a digit
loop is a fine exercise if you want it.

### Tests for stage 1

`internal/tools/file_test.go`, table-driven, and it needs no `t.TempDir()`
at all — that's the payoff for keeping this layer pure. Cases worth
covering:

- `applyEdit`: unique match; zero matches; two matches; empty `old`;
  `old == new`; replacement at the very start of the file; line number
  correctness on a multi-line file.
- `stripLineNumbers`: all lines prefixed → stripped; one line missing
  its prefix → untouched; content that legitimately starts with a digit
  and a tab → untouched; single-line input; empty string.

Known v1 gap, write it down and move on: a CRLF file whose content the
model quotes back with bare `\n` won't match. Not worth solving until it
bites.

---

## Stage 2 — the path layer

One function that every tool calls first, before anything else:

```
resolvePath(p string) (string, error)
    p := trim spaces from p
    if p == ""              -> error

    if p == "~" or starts with "~/":
        home, err := os.UserHomeDir()
        replace the leading "~" with home
    else if p starts with "~":
        error: "~user paths are not supported; use an absolute path"

    abs, err := filepath.Abs(p)        // resolves relative against cwd, and cleans

    if denied(abs) -> error: "<path> is off-limits (contains secrets)"

    return abs, nil
```

Three judgment calls in there worth understanding:

**Why the denylist lives inside the resolver.** Any tool that forgets to
call the fence is a hole. Making the fence inseparable from "turn this
into a usable path" means forgetting it is impossible — you literally
cannot get a path to open without passing the check. Same instinct as
the items-table fence sitting inside `queryDB`/`editDB` rather than
somewhere a caller could skip.

Naming honesty: this function does two things, and `resolvePath`
undersells the second. Either document it in one line ("resolve to an
absolute path, rejecting fenced locations") or name it something like
`safePath`. Your call — but a name that hides a security check is a bad
name.

**Why `~foo` is rejected rather than expanded.** `~alice` means another
user's home directory. Expanding it correctly needs the user database;
guessing `/Users/alice` is wrong on other platforms and a security
surprise anywhere. Reject the case you can't do right.

**Symlinks.** A symlink pointing into `~/.finance/` sails straight past
a prefix-based denylist, because the *resolved-but-not-followed* path
doesn't look fenced. `filepath.EvalSymlinks` closes that hole but only
works on paths that exist. Options: (a) ignore it in v1 and write down
the gap, (b) call `EvalSymlinks` and check the result too when it
succeeds, ignoring the error when it doesn't. (b) is about four lines
and strictly better; take it if you have the appetite, skip it
knowingly if not. Do not skip it *unknowingly* — that's the difference
between a v1 limitation and a bug.

### The denylist

Hardcoded patterns, two lists, no regex needed:

```
var deniedDirs  = [ "~/.finance", "~/.ssh", "~/.aws", "~/.gnupg" ]
var deniedNames = [ ".zshrc", ".bashrc", ".zprofile", ".bash_profile", ".netrc" ]

denied(abs string) bool
    base := filepath.Base(abs)
    if base == ".env" or base starts with ".env"   -> true   // .env.local, .env.production
    if base is in deniedNames                      -> true
    for each dir in deniedDirs:
        expand its "~" the same way resolvePath does
        if abs == dir or abs is under dir + separator -> true
    return false
```

Two traps here. First, **the prefix check needs the separator**:
comparing against `home + "/.finance"` alone means `~/.financeXYZ`
matches. Compare against `dir + string(os.PathSeparator)` (or use
`filepath.Rel` and reject results that escape with `..`). Second, these
patterns must only ever be checked against an **already-absolute,
already-cleaned** path — that's why `denied` is called at the end of
`resolvePath` and nowhere else. Checking `../../.env` as a raw string is
how denylists get walked around.

Test it with a table: a plain path, a `~`-relative path, each denied
directory, `.env` and `.env.local`, the `~/.financeXYZ` near-miss, and a
`..` traversal aiming at a denied dir.

---

## Stage 3 — `read_file`

Now the first end-to-end loop, and it's small because stages 1–2 did the
work.

```
const maxReadBytes = 100 * 1024   // 100KB

readFileTool = openrouter.Tool{ ... }   // see below

readFile(args string) (string, error)
    params := struct{ Path string `json:"path"` }
    unmarshal args into params, wrap the error with fmt.Errorf("parse arguments: %w", err)

    abs, err := resolvePath(params.Path)

    info, err := os.Stat(abs)
    if info.IsDir()                -> error: "<abs> is a directory, not a file"
    if !info.Mode().IsRegular()    -> error: "not a regular file"
    if info.Size() > maxReadBytes  -> error: "<abs> is <n>KB; the limit is 100KB"

    data, err := os.ReadFile(abs)
    return numbered(data), nil
```

`Stat` before `ReadFile`, not after — checking the size *after* loading
a 2GB file into memory defeats the entire point of a cap.

The numbering format:

```
numbered(content) string
    for i, line := range split content on "\n":
        write fmt.Sprintf("%6d\t%s\n", i+1, line)
```

Why a **tab** as the separator and not spaces: it's a single
unambiguous byte the model learns to strip, and `stripLineNumbers` can
key off it without guessing how many spaces you used. This is the same
format `cat -n` and Claude Code's Read use — familiar territory for the
model, which is worth real accuracy.

Edge case to decide: a file ending in `\n` splits into a final empty
element, so you'll emit a trailing numbered blank line. Harmless, but
notice it rather than being surprised later.

### The schema — where most of the value is

Description prose to get across, in roughly this order: reads a text
file from the local filesystem; returns the **whole** file with line
numbers prefixed as `<number><tab>`; the numbers are for reference and
are **not part of the file**; paths may be absolute or `~`-relative;
files over 100KB are refused rather than truncated; some paths holding
secrets are off-limits and will error.

One parameter: `path`, string, required. Its own description should
carry the path rules — the model reads parameter descriptions.

Registry line: `{Schema: readFileTool, Execute: readFile}` — no
`NeedsApproval`. Reading doesn't damage the filesystem. (It *does* leak
into the conversation, which is what the denylist is for. Two different
threat models, two different defenses — the spec's central point.)

**Checkpoint: build, install, and actually use it.** `go build -o
~/go/bin/moussa ./cmd/moussa`, then ask moussa to read a file. Then ask
it to read `~/.zshrc` and confirm you get the fence error. Don't start
stage 4 until reading works.

---

## Stage 4 — `edit_file`

```
editFile(args string) (string, error)
    params := struct{ Path, OldString, NewString string }   // json tags: path, old_string, new_string
    unmarshal, wrap errors

    abs, err := resolvePath(params.Path)

    info, err := os.Stat(abs)
    if IsDir / not regular / Size > maxReadBytes -> same errors as readFile
    // (file must already exist — creation is write_file's job)

    data, err := os.ReadFile(abs)

    old := stripLineNumbers(params.OldString)
    new := stripLineNumbers(params.NewString)

    out, line, err := applyEdit(string(data), old, new)
    if err != nil -> return it (the model reads this and retries)

    write out back to abs, preserving info.Mode().Perm()

    return fmt.Sprintf("OK — replaced 1 occurrence in %s at line %d\n", abs, line), nil
```

Registry: `{Schema: editFileTool, Execute: editFile, NeedsApproval: true}`.

Three things to think about rather than type on autopilot:

**Should `edit_file` require the file to have been read first?** Claude
Code enforces exactly that, and it's a genuinely good rule — editing
text you haven't seen is guessing. But it needs session state (a set of
files read this conversation), and every func in `internal/tools` is
currently stateless. A package-level map would be hidden mutable state
in a package that has none. Verdict for v1: skip it. The approval gate
is your read-before-edit — you see the before/after and say no if it
looks invented. Worth revisiting if a session ever gets a real state
object.

**Error messages are prompts.** Everything you return from `applyEdit`
goes straight into the model's next turn as its only feedback. "old
string not found" leaves it guessing; "not found — re-read the file and
quote it exactly, including whitespace" tells it what to do next. Write
these for a reader who will act on them.

**The description must earn the approval gate.** Say that every call is
shown to David before it runs, that `old_string` must match exactly once
and the tool errors rather than guessing, that more surrounding context
is the fix for an ambiguous match, that whitespace and indentation are
significant, that line-number prefixes from `read_file` must be
stripped, and that creating files is not supported. Mirror `edit_db`'s
tone — it's the same "powerful, gated, explain-before-calling" contract.

---

## Stage 5 — atomic write, then live fire

`os.WriteFile` truncates first and writes second. Crash in between and
the file is gone — on `merchantLookup.json`, that's real data. The fix
is the standard pattern, and it's short:

```
write temp file in the SAME directory as abs   (os.CreateTemp(filepath.Dir(abs), ".moussa-*"))
write out to it, close it, chmod it to info.Mode().Perm()
os.Rename(temp, abs)     // atomic on the same filesystem
on any failure before the rename: os.Remove(temp)
```

Same directory matters: `os.Rename` across filesystems isn't atomic (and
may fail outright), so a temp file in `/tmp` defeats the purpose.

Then the live-fire from the spec's build steps:

1. Ask moussa to read `~/.finance/merchantLookup.json` — this should
   **fail**, `~/.finance` is denied. Decide right then whether the
   denylist is too blunt: the db holds Plaid tokens, a JSON lookup table
   doesn't. Options are a file-level exception, or moving the lookup
   file out of `~/.finance`. Note the decision in
   `file-tools.decisions.md`.
2. Read something harmless and confirm the numbering renders.
3. Have it make a one-line edit, approve it at the gate, and verify with
   `finance rules`.
4. Try to make it edit a string that appears twice, and confirm the
   error teaches it to add context.

---

## Definition of done

- `internal/tools/file.go` with `read_file` and `edit_file`, both
  registered, `edit_file` gated.
- `internal/tools/file_test.go` covering `applyEdit`,
  `stripLineNumbers`, and `resolvePath`/`denied`.
- `go vet ./...` clean.
- Live-fire done: a read, a denied read, an approved edit, an ambiguous
  edit.
- `file-tools.decisions.md` written with the `~/.finance` verdict and
  the known gaps (CRLF, no read-before-edit, symlinks if you skipped
  them). Then both docs move to `docs/done/`.
