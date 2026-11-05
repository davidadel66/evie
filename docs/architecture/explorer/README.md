# Facts and the system map

Evie's System view, in its sidebar, is one page with three tabs of its own:
**Map**, a subway map of Evie's flows whose stations open into the steps
inside them; **Architecture**, every part Evie is built from at four levels
(Context, Containers, Components, Flows), each part with the map steps it
appears in; and **History**, a timeline of `CHANGELOG.md`. The vision skill
(`~/.claude/skills/vision`) draws the page from the
[system map](#the-system-map), `.vision/map.json`, the facts in this
directory, the changelog, and any [design proposals](#design-proposals) in
`.vision/proposals/`. This directory holds what Evie's own tools keep true:

| Path | What it is |
| --- | --- |
| `data/facts.js` | The facts: what the code answers mechanically (packages, imports, commands, routes, tables, glossary terms, ADRs). Never edit it by hand; run `extract`. |
| `system.html` | The page, built by the vision skill from the map, the facts, the changelog, and the design proposals. Never edit it by hand; rebuild it. |
| `snapshot.go` | Package `explorer`: embeds `system.html`, the snapshot's only file, so `evie serve` can serve it under `/explorer/` (see [Served by `evie serve`](#served-by-evie-serve)). |

The drift check holds the facts, the map, the changelog, and the page to the
code, so CI fails when any of them drifts. Specifications:
[`system-explorer.spec.md`](../../../cmd/evie/docs/active/system-explorer.spec.md)
(the facts and the check) and
[`system-page.spec.md`](../../../cmd/evie/docs/active/system-page.spec.md)
(the System view, the map, and History); decisions sit beside each.

## Commands

Run both from the repository root.

```sh
go run ./scripts/explorer extract
```

Rewrites `data/facts.js` from the code. Output is sorted and has no
timestamps or line numbers, so running it twice leaves the file
byte-identical on every platform. Run it whenever a package, import, command,
route, table, glossary term, or ADR is added, removed, or renamed, and commit
the result with the page [rebuilt](#changing-the-map) from it.

```sh
go run ./scripts/explorer check
```

Re-extracts the facts in memory, compares them byte for byte with
`data/facts.js`, then checks the [system map](#what-the-check-holds-the-map-to),
the [design proposals](#design-proposals), the page, and
[`CHANGELOG.md`](#history) against them. It prints every problem, one per
line, as

```text
<file>: <item>: <what is wrong>. Fix: <how to fix it>.
```

then a count, and exits 1. It exits 0 when everything agrees and 2 when it
cannot run at all. Its output is the same on every run.
`./scripts/verify-change.sh` runs it, so CI fails on drift.

## The facts

`data/facts.js` is a classic script: a fixed first line, one strict JSON
value, and a fixed last line. The vision skill reads it in that wrapper.

```text
evieExplorer.register(
{ ...one JSON object... }
);
```

`check` fails when the file differs from a fresh extraction, naming the
extract command, and when it has CRLF line endings (the repository's
`.gitattributes` checks this directory out with LF).

### What extract reads

```json
{
  "format": 1,
  "kind": "facts",
  "packages": [{"path": "internal/agent", "name": "agent", "command": false, "doc": "Package agent is ...", "testSupport": false}],
  "imports": [{"from": "internal/agent", "to": "internal/openrouter"}],
  "commands": [{"binary": "todo", "words": ["add"], "package": "cmd/todo", "source": {"file": "cmd/todo/main.go", "function": "run"}}],
  "routes": [{"method": "GET", "pattern": "/api/x", "package": "internal/web", "guard": "requireOwner", "source": {"file": "...", "function": "..."}}],
  "databases": [{"id": "evie", "package": "internal/eviedb", "tables": [{"name": "jobs", "columns": [{"name": "id", "type": "INTEGER", "notNull": false, "primaryKey": 1}], "foreignKeys": [{"columns": ["job_id"], "table": "jobs", "references": ["id"]}]}]}],
  "glossary": [{"term": "Plugin", "definition": "...", "avoid": ["Tool", "skill"]}],
  "adrs": [{"number": "0001", "title": "...", "path": "docs/adr/0001-....md"}]
}
```

- **packages**: every directory under `cmd/` and `internal/` with a non-test
  Go file, excluding `testdata` and `node_modules`, and this directory itself
  (not the directories below it) once `snapshot.go` is there: the one package
  outside `cmd/` and `internal/`, because `go:embed` reaches only its own
  directory. `doc` is the first
  sentence of the package comment in the first file, by name, that has one.
  `command` means package `main`; `testSupport` means the package name ends
  in `test` and its directory sits directly inside another package's
  directory, Go's layout for a package's test helpers (as `net/http/httptest`
  sits in `net/http`), so `internal/mcpconn/mcptest` is test support and a
  package such as `internal/latest` is not.
- **imports**: in-module imports between those packages from every non-test
  file, ignoring build constraints, so every platform extracts the same edges.
- **commands**: the word paths of each command package's binary (named after
  its directory). A string literal compared by `==`, `!=`, or a `switch`
  case with an element of the argument slice is the word at that element's
  position, counted from the first argument after the binary name. Earlier
  words come from the conditions around the comparison: enclosing `if` and
  `switch` branches, the left side of `&&` and `||`, and the rest of a block
  after an `if` that returns. The reading starts at `main` and follows calls
  into the package's own functions and methods that receive the arguments, a
  slice of them, or one of them. Literals starting with `-` are flags and
  `""` is no word; both are skipped. Any other comparison whose position or
  earlier words cannot be known fails extraction, naming the file and
  function. `source` is the function nearest `main` that compares the last
  word. A comparison with a copy of the arguments built another way (such as
  a filtered `append`) is not an argument comparison and is not seen; the
  golden test in `scripts/explorer/commands_test.go` reconciles every path
  with the binaries' usage strings and lists the known gaps.
- **routes**: every `Handle` and `HandleFunc` call in a non-test file on a
  ServeMux (a `*http.ServeMux` parameter or variable, a variable set from
  `http.NewServeMux()`, or the `http` package's default mux). The pattern is
  a string literal, or a literal prefix joined to the key of a range over a
  map literal with literal keys, or to the element of a range over a slice
  literal of strings, expanded once per key or element; any other pattern
  fails extraction, naming the file and function. A leading method such as
  `GET ` becomes `method`. `guard` is the method called on the enclosing
  method's receiver with the handler as an argument, seen through
  conversions such as `http.HandlerFunc`, local variables, and a closure that
  delegates with `ServeHTTP`; a route without one has no `guard`.
- **databases**: Evie's databases (`evie`, `finance`, `youtube`), each
  created by its package's real open-and-migrate entry point on a fresh
  temporary file, with the home directory pointed at that file's temporary
  directory while it runs (finance takes its path from the home directory).
  The tool sets every variable `os.UserHomeDir` reads (`HOME`,
  `USERPROFILE` on Windows, `home` on Plan 9) and restores each afterwards. A database opened
  outside its temporary directory fails extraction, so the owner's databases
  are never opened. Tables are read from SQLite, sorted by name, leaving out
  SQLite's internal `sqlite_` tables, the shadow tables of virtual tables,
  and views; virtual tables themselves are listed. Columns keep their
  declared order; `primaryKey` is the column's 1-based position in the
  primary key, or 0. A foreign key that names no parent columns references
  the parent's primary key.
- **glossary**: the bold `**Term**:` entries of `GLOSSARY.md`, the paragraph
  after each as the definition, and its `_Avoid_:` line.
- **adrs**: the numbered files in `docs/adr/` with their first heading.

`extract` does not read `CHANGELOG.md`: the page's History tab reads it
directly, so the facts carry no copy of it and a changelog edit leaves them
unchanged.


## The system map

[`.vision/map.json`](../../../.vision/map.json) is the vision skill's map of
Evie (its format: the skill's `references/map-format.md`). It holds:

- the **flows** the Map tab draws: one chat turn left to right, and the
  concept flows it starts (Memory, Workflows, Subagents, Changes & merge, MCP
  sign-in, Serve startup), each step pointing at the code it describes;
- the **parts** the Architecture tab draws: the people and outside systems
  around Evie (Context), its processes, interfaces and stores (Containers),
  and a component part for every package (Components), in the four groups of
  the architecture map in `AGENTS.md`. Each part has a cited summary and,
  where the code says them, what it does, the data it owns, its interfaces,
  its gotchas, its glossary terms, and diagrams (graph, state, er, and
  sequence);
- the **edges** between parts the facts' imports do not draw, and the
  **sequences** the Flows level steps through (a chat turn, an approval,
  plugin activation, a subagent run, a workflow run and its schedule, memory
  compilation and the background pass, automatic recall, MCP sign-in, a
  change merge, and `serve` startup);
- its settings: `meta.facts` (`data/facts.js`), `meta.page` (`system.html`),
  `meta.changelog` (`CHANGELOG.md`, which History draws), `meta.repository`
  (where History's commits, tickets and pull requests open), and
  `meta.parts: "complete"` (every package has a part).

The page shows no code. Statements and steps cite their sources, and the
check resolves every citation; the page names a source as a plain chip, never
a link into the repository.

### Changing the map

Change the map through the vision skill, which `/flow` runs after each piece
of work: it updates the steps and parts a change touched. A new package needs
a component part with `"package": "<path>"`, a `group`, and a cited summary.
Then rebuild the page from the repository root and commit the two together:

```sh
python3 ~/.claude/skills/vision/scripts/vision.py build
```

`build` runs the skill's own check first (every highlight and backed value
against the code, every part, edge, sequence, changelog entry, and design
proposal) and writes two hashes into the page's head: the map's, and one
over the other files it embeds, the facts, the changelog, and the design
proposals. Rebuild after any of them changes, not only the map: `check`
fails on a page with either hash out of date
([The page's hashes](#the-pages-hashes)).

### What the check holds the map to

CI cannot run the skill, which lives outside the repository, so `check` holds
the map to the code itself. A page built from the current map passed the
skill's check when it was built, so `check` reads what drifts when the code
does, and the page's two hashes catch a map, facts file, changelog, or
proposal changed without a rebuild. It reports, naming the step, part, edge,
sequence, code entry, proposal, or setting, and how to fix it:

- a package in the facts with no part whose `package` names it, and a part
  whose `package` the facts no longer have;
- a reference that does not resolve ([Reference forms](#reference-forms)):
  in a step's `refs`, fact chips, and flags; in a part's `refs`, `links`,
  `terms`, fact chips, and the statements of its summary, `does`, `owns`,
  `interfaces`, and `gotchas`; in an edge; in a sequence's summary, trigger,
  and steps; and in a diagram's summary, trigger, edges, states,
  transitions, and steps;
- a fact chip `[label, value, reference]`, on a step or a part, whose value
  is not in what its reference points at ([Backed values](#backed-values));
- an `import` edge, or a sequence step with `via: "import"`, between two
  package parts whose packages the facts do not import (the caller imports
  the callee, or the reverse for a `return`), or with an end that is a part
  without a package;
- a statement of a part (its summary, `does`, `owns`, `interfaces`, or
  `gotchas`), or an edge other than an `import` edge, that cites no source;
- a sequence step, or a sequence diagram's step, that cites no code: none of
  its references is a `code:` reference or a key of the map's `code`;
- a sequence step whose `from` or `to` is not one of its sequence's
  participants;
- a sequence participant, a `contains` entry, an edge's end, or a graph
  diagram node's `node` that is not a part;
- an id used twice: a part's among the parts, a sequence's among the
  sequences, or a diagram's among every part's diagrams;
- an er diagram over a database the facts do not have, or a table that
  database does not have;
- a code entry whose file is gone, or whose `symbol` the file no longer
  declares, and a step's `code.key` that is not in the map's `code`;
- a step's `files` path or a part's `paths` entry that does not exist;
- a setting other than Evie's: `meta.facts` other than
  `docs/architecture/explorer/data/facts.js`, `meta.page` other than
  `docs/architecture/explorer/system.html`, `meta.parts` other than
  `"complete"`, `meta.changelog` other than `CHANGELOG.md`, and
  `meta.repository` other than `https://` and the module path in `go.mod`;
- a page that is missing, carries no `<meta name="vision-map-hash">` or no
  `<meta name="vision-inputs-hash">`, or carries the hash of an older map or
  of an older facts file, changelog, or set of design proposals
  ([The page's hashes](#the-pages-hashes));
- a design proposal that is not one JSON object, whose `id` is not its file
  name, or whose `status` the skill does not know
  ([Design proposals](#design-proposals));
- a missing or malformed map: it holds every package's part, and the
  binary embeds the page built from it. It is reported once; the design
  proposals and the page's inputs hash are still checked, and the
  changelog's `part:` sources wait for a map whose parts they can name.

Any other form of the skill's format is refused rather than passed unread,
naming the skill's own check
(`python3 ~/.claude/skills/vision/scripts/vision.py check .vision/map.json`),
which reads it: a code entry's `lines` or `cell`, an inline `src` entry, a
single top-level `flow`, library definitions in `refs`, a value panel's `ref`
or a `kv` row's reference, a value backed by a `doc:` reference or that is not
a string, and the skill's notebook, text-source, and index reference kinds
(`undef:`, `sec:`, and the rest). Teach `scripts/explorer/visionmap.go` the
form before the map uses it. The check does not re-read highlights or the
map's structure (titles, levels, groups, or a sequence's or diagram's
statements without a source), which only the skill's `check` does.

### Backed values

A fact chip `[label, value, reference]` must say what its reference points
at, as the skill's check matches it ("backed values" in the map format): the
value appears, as whole words, in the declaration's lines (from its declaring
line through its matching brace), the whole file for `code:path`, the part's
id for `part:`, or the fact's own fields (a route's method, pattern, guard,
package, and registering function; a table's name and columns; a package's
path, name, and doc; a command's words and package; an ADR's title and path;
a term's definition). The case of ASCII letters, runs of whitespace,
digit-group commas, and the kind of minus sign are ignored, a word
hyphenated across a line end is joined, and an ellipsis (`…`) elides between
pieces that must appear in order. The skill also applies Unicode
compatibility normalization (NFKC) and full case folding, which Go's
standard library lacks, so `check` compares every other character exactly
(`CAFÉ` is not `café`) and takes no match whose edges NFKC could change: one
beside a character outside ASCII other than punctuation NFKC leaves alone
(such as `—`, `→`, and curly quotes), one that starts with a combining mark,
or one sharing a digit group with such a character. It can only be stricter
than the skill.

### The page's hashes

The skill writes both hashes into the page's head, and `check` recomputes
each exactly as the skill's `references/map-format.md` says, so CI can tell
a page that was not rebuilt without running the skill:

- **Map hash** (`vision-map-hash`, "Map hash"): the map without
  `meta.commit`, `meta.stamped`, `meta.artifact` and the stamp's code
  fingerprints, in RFC 8785 canonical JSON, then SHA-256. Stamping and
  reformatting the map leave it unchanged.
- **Inputs hash** (`vision-inputs-hash`, "Inputs hash"): the other files the
  page embeds, in this order: the facts file `meta.facts` names, the
  changelog `meta.changelog` names, then every design proposal (each `.json`
  file directly in `.vision/proposals/` whose name does not start with `.`)
  sorted by file name as UTF-8 bytes. Each file is one record: its kind
  (`facts`, `changelog`, or `proposal`), a space, its name (the path as
  `meta` gives it, or the proposal's file name), a newline, its length in
  bytes in decimal, a newline, its bytes with every CR LF read as LF, and a
  newline. A file the map does not name, or that is missing, has no record.
  The hash is the SHA-256 of the records joined, as lowercase hex after
  `sha256:`; with no records it is the SHA-256 of nothing,
  `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
  Because CR LF reads as LF, a checkout with CRLF line endings agrees with
  the page.

`scripts/explorer`'s tests hold both to what the skill's own `vision.py`
computes on fixtures (`testdata/maphash` and `testdata/inputshash`).

### Design proposals

A design proposal draws what one planned feature would add, change, or
remove over the map as built, with the feature's requirements and decisions;
the page draws it over the Map and Architecture while a reader turns it on.
Each is one file, `.vision/proposals/<id>.json`, on the feature's branch, in
the format of the skill's `references/map-format.md` ("Design proposals").
`/flow` writes one during a Big brief and deletes it in "Update the System",
once the map describes what was built. The folder exists only while a
proposal does; Evie commits no example.

CI cannot run the skill's check, which validates a proposal in full (its
fields, statements and references, and where each operation points on the
map), so `check` holds each file only to what it can check without the
skill: it is one JSON object, its `id` is its file name without `.json`, and
its `status` is `proposed`, `accepted`, `building`, or `example`. It reads the
files the skill reads, every `.json` file directly in the folder whose name
does not start with `.`, and they count toward the
[inputs hash](#the-pages-hashes). The rest is the skill's: run
`python3 ~/.claude/skills/vision/scripts/vision.py check .vision/map.json`,
or `build`, which runs it first.

## Reference forms

References are typed strings, `<kind>:<target>`, or in the map a key of its
`code`. Line numbers are never a reference form. The map, its parts, edges
and sequences, and the changelog's sources all use these:

| Form | Resolves when |
| --- | --- |
| `code:path` | The repository file or folder exists. |
| `code:path#Name` | The file declares `Name`: in Go a top-level function, type, variable, or constant; in JavaScript or TypeScript a function, class, interface, type, enum, namespace, or variable. |
| `code:path#Type.Name` | The Go file declares method `Name` on `Type`, a field `Name` of struct `Type`, or a method `Name` of interface `Type`; in JavaScript or TypeScript, a member declared on a line directly inside `Type`'s braces. |
| `doc:path` | The repository file or folder exists. |
| `doc:path#anchor` | The Markdown file has an ATX (`## Title`) or setext (`Title` over a line of `=` or `-`) heading with that GitHub anchor: lower-cased, letters, digits, `-` and `_` kept, spaces turned into `-`, `-1`, `-2` for repeats. Headings in code fences and front matter do not count; a fence closes only on a run of its own character (backtick or tilde) at least as long as the one that opened it, and a list item, table row, or quote above a line of `-` is not a heading. |
| `adr:0007` | `docs/adr/` has ADR 0007. |
| `term:Agent Preset` | `GLOSSARY.md` has that term, spelled exactly. |
| `table:evie.jobs` | The facts have database `evie` with table `jobs`. |
| `route:GET /api/x`, `route:/api/x` | The facts have that route; without a method, any method matches. The pattern starts with `/`; a method is upper case and one space separates it from the pattern. |
| `command:todo add`, `command:todo` | The facts have that binary and word path; a bare binary matches any of its commands. |
| `pkg:internal/agent` | The facts have that package. |
| `part:pkg:internal/agent` | The map has a part with that id. |

Paths are clean and relative to the repository root. A key of the map's
`code` resolves in the map; a changelog source is always `<kind>:<target>`
or a `#n` number.

## Served by `evie serve`

`evie serve` serves the snapshot under `/explorer/`: the page at
`/explorer/` and at `/explorer/system.html`, GET and HEAD only, with a 404
for anything else. Every response carries one content-security policy: the
page's inline scripts by their hashes, inline styles, images only from Evie
or `data:` URLs, no connections, frames, or forms, and framing only by Evie.
The page is sandboxed with scripts and popups but without same-origin, so
even opened on its own it cannot use Evie's routes or storage, and popups
escape the sandbox so History's GitHub links open normally. Like the rest of
Evie it reaches no other host: built to `meta.page`, the page uses the system
fonts instead of Geist from Google Fonts. The route sits outside the API
guard, like Evie's own static files: it serves only the public repository's
architecture and reads no data.

Evie's System view frames `/explorer/?theme=dark`, under a System header
and full height; `theme=dark` pins the page dark whatever the system
preference, and the page opens on its subway map. The page's own Map,
Architecture, and History tabs are the only tab bar: Evie draws none, since
the page routes between its views itself (as when a part chip on the Map
opens Architecture), and a second bar outside the frame could not follow
it.

## History

[`CHANGELOG.md`](../../../CHANGELOG.md) at the repository root records every
merge to master, newest first: what changed and why. The page's History tab
draws it (the map's `meta.changelog`), and `check` validates it.
`/flow` drafts each work branch's entry; it can also be written by hand.
Specification: the "Changelog format" section of
[`system-page.spec.md`](../../../cmd/evie/docs/active/system-page.spec.md).

An entry looks like this:

```markdown
## 2026-11-02 — Name the change
Branch: feat/my-feature
Merge: 1a2b3c4
Touches: internal/agent, internal/web
Sources: doc:cmd/evie/docs/active/my-feature.spec.md, #201

What changed and why, in one or more paragraphs. `Backticks` mark code.

A second paragraph.
```

| Part | Rule |
| --- | --- |
| Heading | `## YYYY-MM-DD — Title`: level two, a real calendar date, an em dash with a space on each side, then the title. The date is the merge's; an entry written on its branch before the merge takes the day it is written. A `##` line inside a code fence is not an entry, and everything before the first entry is a preamble. |
| Metadata | The first block after the heading: one `Field: value` line per field, each field at most once, ended by a blank line. |
| `Branch:` | Required. The merged branch; `master` for direct-to-master work. |
| `Merge:` | Optional. The merge commit: a SHA, full or at least 7 lowercase hex digits, long enough to be unambiguous. Leave it out while the entry is written on its branch, and add it once the merge exists. |
| `Commits:` | Instead of `Merge:`, for direct-to-master work: `<first>..<last>`, the first and last commit of the work, both included. Unlike `git log A..B`, which leaves out A, the first commit belongs to the entry, so the root commit can start a range and one commit is `abc1234..abc1234`. The first commit is the last one or an ancestor of it. |
| `Touches:` | Required. Comma-separated repository paths the change touched, normally package paths (`internal/agent`, `cmd/evie`). Each is a clean relative path. They are not link-checked, so history stays readable after a package is removed. A path that belongs to a part (its package, one of its `paths`, or the file of a definition it lists) shows as a chip that opens the part, whose panel lists the entry; any other path, such as a removed package, shows as plain text. |
| `Sources:` | Required, at least one. Comma-separated references in the [reference forms](#reference-forms), checked the same way, and ticket or pull-request numbers such as `#179`: `#` and a number with no leading zero, checked for form only because GitHub cannot be asked offline. |
| Text | One or more paragraphs after the blank line. The lines of a paragraph join with spaces; text between backticks shows as code. |

Entries are newest first by date. Entries on the same day may stand in
either order; put the later merge first.

`check` reports each problem against the entry, by its heading:

```text
CHANGELOG.md: entry "2026-11-02 — Name the change": missing Branch. Fix: add a Branch: line naming the merged branch.
```

It reports:

- a heading that is not `YYYY-MM-DD — Title`, or has no title;
- a missing `Branch:`, `Touches:`, or `Sources:` line, or no text;
- a metadata line that is not one of the fields above (often prose with no
  blank line before it), a field given twice, and an empty list item;
- a touched path that is not a clean relative path;
- a date that is not a real calendar date, such as `2026-02-30`;
- an entry newer than the entry above it;
- a source reference that does not resolve, and a malformed `#n` number;
- a `Merge:` or `Commits:` value that is not a SHA or a `<first>..<last>`
  range, an entry with both, a commit git does not know
  (`git cat-file -e <sha>^{commit}`, run from the repository root), and a
  range whose first commit is neither its last commit nor an ancestor of it
  (`git merge-base --is-ancestor`).

Verifying commits needs git and the full history: run the check in a git
checkout (CI checks out with `fetch-depth: 0`). When git is not on `PATH` or
the directory is not a repository, the check reports
`CHANGELOG.md: commits: git cannot verify the changelog's commits` once and
fails. An entry with neither `Merge:` nor `Commits:` needs no git.

To add an entry (`/flow` does steps 1 and 2 on the work branch):

1. Write it above every older entry in `CHANGELOG.md`, in the format above.
   On the branch, before the merge exists, leave out `Merge:`.
2. Rebuild the page (`python3 ~/.claude/skills/vision/scripts/vision.py
   build`), since it embeds the changelog, then run `go run
   ./scripts/explorer check` and fix what it reports. Commit `CHANGELOG.md`
   with `system.html`.
3. Once the merge exists, add its `Merge:` line, then rebuild and check
   again.
