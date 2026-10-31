# System explorer

The explorer page shows what Evie does: every package under `cmd/` and
`internal/` as a component, grouped under the architecture map in
`AGENTS.md`, with written content on top of facts extracted from the code.
Open [`index.html`](index.html) straight from disk; it needs no server, build,
or network. `evie serve` also carries it, built into the binary, as the
System view in Evie's sidebar. Specifications:
[`system-explorer.spec.md`](../../../cmd/evie/docs/active/system-explorer.spec.md)
and [`system-page.spec.md`](../../../cmd/evie/docs/active/system-page.spec.md).

Agents edit the data, never the page. The drift check keeps the data honest:
it fails when the data and the code disagree.

## Files

| Path | What it is |
| --- | --- |
| `index.html`, `explorer.js`, `explorer.css` | The page, its one classic-script viewer, and its stylesheet. No modules, no build, no third-party code. |
| `snapshot.go` | Package `explorer`: embeds the page, viewer, stylesheet, `data/`, and `examples/` (not the tests or this README) so `evie serve` can serve them under `/explorer/`. |
| `data/manifest.js` | The ordered list of data files the page loads. |
| `data/facts.js` | Generated facts. Never edit by hand; run `extract`. |
| `data/<owner>.js` | Written content, one file per subsystem owner (below). |
| `data/proposals/<id>.js` | One file per design proposal. |
| `examples/<id>.js` | Example design proposals. Checked, but not listed in the manifest; the page loads one through its proposal parameter (below). `examples/design-proposal.js` ships as the example. |
| `test/*.test.js` | Viewer model tests for Node's built-in runner; `test/fixture.js` and the other non-test files are their shared fixture and helpers. |

Content files and the packages whose component nodes they hold:

| File | Packages |
| --- | --- |
| `system.js` | System context and containers (no packages). |
| `intelligence.js` | openrouter, localembedding, localextractor |
| `memory.js` | memory, memorybackground, memoryeval |
| `runtime.js` | agent, tools, plugins, composition, subagents, delegation, task, taskeval, untrusted, repoinstructions |
| `workflows.js` | workflow, workflowruntime, procedural |
| `platform.js` | eviedb, web, docs/architecture/explorer, usage, cmd/evie, cmd/todo |
| `integrations.js` | mcpconn, mcpconn/mcptest, mcpchat, github, finance, youtube, changes, worktree, localfiles, localterminal, workspacetabs, cmd/finance, cmd/ytscribe |

A file owns nodes; it does not decide their group. Each node's `group` and
`subgroup` follow the spec's component grouping, so `cmd/finance` lives in
`integrations.js` and is drawn under Infrastructure / Surfaces.

## Commands

Run both from the repository root.

```sh
go run ./scripts/explorer extract
```

Rewrites `data/facts.js` from the code. Output is sorted and has no
timestamps or line numbers, so running it twice leaves the file
byte-identical on every platform. Run it whenever a package, import, command,
route, table, glossary term, or ADR is added, removed, or renamed, and commit
the result.

```sh
go run ./scripts/explorer check
```

Re-extracts the facts in memory, compares them byte for byte with
`data/facts.js`, then validates every manifest-listed data file and every
example against them. It prints every problem, one per line, as

```text
<data file>: <item>: <what is wrong>. Fix: <how to fix it>.
```

then a count, and exits 1. It exits 0 when everything agrees and 2 when it
cannot run at all. `./scripts/verify-change.sh` runs it, so CI fails on drift.

## Data files

Every data file is a classic script: a fixed first line, one strict JSON
value (no comments, no trailing commas), and a fixed last line. The tool
parses a file by stripping those two lines; the page runs it as a script.

```text
evieExplorer.register(
{ ...one JSON object... }
);
```

The manifest uses its own call and holds a JSON array of paths relative to
`data/`, loaded in order:

```text
evieExplorer.manifest(
[
  "facts.js",
  "system.js",
  "proposals/my-feature.js"
]
);
```

Every `.js` file under `data/` other than the manifest must be listed, and
every listed file must exist.

Every data object has `"format": 1` and a `"kind"`: `facts`, `content`, or
`proposal`. Decoding is strict: an unknown key, or a key in the wrong case,
is malformed data. Optional lists may be left out.

### Facts (generated)

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

### Content

```json
{
  "format": 1,
  "kind": "content",
  "owner": "runtime",
  "nodes": [],
  "edges": [],
  "flows": []
}
```

**Statement**: one sentence and the sources it rests on. Every statement
cites at least one reference. `detail` is reserved for the depth pass.

```json
{"text": "Runs one conversation Session's model-and-tools loop.", "refs": ["code:internal/agent/agent.go#Session"], "detail": "optional"}
```

**Node**:

| Field | Required | Value |
| --- | --- | --- |
| `id` | yes | Unique across all content. A package's node is `pkg:` plus its path, such as `pkg:internal/agent`. |
| `level` | yes | `context`, `container`, or `component`; `part` for children. |
| `kind` | yes, except parts | `person`, `system`, `external-system`, `process`, `user-interface`, `cli`, `store`, `package`. A `pkg:` node is a `component` of kind `package`. |
| `title` | yes | Display name. |
| `summary` | yes | Statement. |
| `does`, `owns`, `interfaces`, `gotchas` | no | Statement lists: what it does, data it owns, its interfaces, its gotchas. |
| `terms` | no | Glossary terms, spelled exactly as in `GLOSSARY.md`. |
| `diagrams` | no | Diagrams, below. |
| `links` | no | Extra references. |
| `contains` | no | Node ids a container zooms into. |
| `group` | components | `memory`, `runtime`, `intelligence`, or `infrastructure`. |
| `subgroup` | no | Display name within the group, such as `Local host`. |
| `alsoIn` | no | Other groups whose views also show the node. |
| `children` | no | Nodes with level `part`, same shape; reserved for the depth pass. |

**Edge**: `from`, `to` (node ids), `label`, `via` (`import`, `call`, `http`,
`sql`, `process`, `network`, `file`, `event`), and `refs`. An `import` edge
joins two package nodes and must be in the facts' imports; it needs no refs.
Every other edge cites at least one source.

**Flow**: `id`, `title`, `summary` and `trigger` statements, `participants`
(node ids), and ordered `steps`. A step has `from` and `to` (participants),
`label`, `kind` (`call`, `return`, `async`, `note`), an optional `via`, and
at least one reference, including a `code:` reference to the code that
carries it (a design proposal's steps describe code that does not exist
yet, so they may cite its spec instead). A step with via `import` must follow an import edge:
the caller imports the callee, or the reverse for a `return`.

**Diagram**: `id` (unique), `title`, `kind`, `summary`, and the fields of its
kind only.

| Kind | Fields |
| --- | --- |
| `graph` | `nodes` (`id`, `label`, optional `node` naming an explorer node, optional `lane`), `edges` (`from`, `to` local ids, optional `label` and `refs`), optional `lanes` (`id`, `title`). |
| `sequence` | The flow shape: `participants`, `steps`, optional `trigger`. |
| `state` | `states` (`id`, optional `label`, `refs`), `initial` (a state id), `transitions` (`from`, `to`, optional `label`, `refs`). Every state and transition cites a source. |
| `er` | `database` (a facts database id) and `tables` (its table names). Columns and relationships come from the facts. |

### Design proposals

A design proposal describes what one feature would add, change, or remove;
the page toggles it on top of the built system.

```json
{
  "format": 1,
  "kind": "proposal",
  "id": "my-feature",
  "feature": "my-feature",
  "title": "Add my feature",
  "status": "proposed",
  "spec": "doc:cmd/evie/docs/active/my-feature.spec.md",
  "summary": {"text": "...", "refs": ["doc:cmd/evie/docs/active/my-feature.spec.md"]},
  "nodes": [
    {"op": "add", "id": "pkg:internal/myfeature", "level": "component", "kind": "package", "title": "myfeature", "group": "runtime", "summary": {"text": "...", "refs": ["doc:cmd/evie/docs/active/my-feature.spec.md#solution"]}},
    {"op": "change", "id": "pkg:internal/agent", "summary": {"text": "...", "refs": ["doc:cmd/evie/docs/active/my-feature.spec.md"]}},
    {"op": "remove", "id": "pkg:internal/untrusted"}
  ],
  "edges": [
    {"op": "add", "from": "pkg:internal/agent", "to": "pkg:internal/myfeature", "via": "import", "label": "starts it"}
  ],
  "flows": [],
  "requirements": [{"text": "...", "refs": ["doc:cmd/evie/docs/active/my-feature.spec.md#user-stories"]}],
  "decisions": [{"text": "...", "refs": ["doc:cmd/evie/docs/active/my-feature.decisions.md"]}]
}
```

- `status` is `proposed`, `accepted`, `building`, or `example`.
- Each operation is `op` beside the item's fields. `add` gives the whole item
  and must not collide with an existing one; `change` gives the identity and
  the fields that change; `remove` gives only the identity.
- Nodes and flows are identified by `id`; edges by `from`, `to`, and `via`.
  Existing edges include the facts' import edges between package nodes.
- Node operations target top-level nodes only: a part (a node's child) is
  not changed or removed on its own, and a node operation carries no
  `children`, which stay reserved for the depth pass. To change a part,
  change the node that holds it.
- Added packages and import edges describe code that does not exist yet, so
  they are not matched against the facts. Statements still cite sources that
  exist, usually the feature's spec, and nodes the proposal adds can be cited
  as `node:` references inside it.

To add a design proposal:

1. Write `data/proposals/<id>.js` in the wrapper above.
2. Add `"proposals/<id>.js"` to `data/manifest.js`. The page cannot list a
   directory from `file://`, so the manifest is how it finds the file.
3. Run `go run ./scripts/explorer check` and fix what it reports.

To look at a proposal without listing it, open the page with the proposal
parameter naming its script, relative to this directory:

```text
index.html?proposal=examples/design-proposal.js
```

The page loads that one extra script after the manifest and accepts only
design proposals from it. It refuses absolute paths, schemes, `.` and `..`
segments, escapes, and anything that is not a plain path to a `.js` file
inside this directory, and says so in a banner. A loaded proposal shows a
toggle in the left rail; turning it on draws added parts dashed in the added
colour, changed parts in the changed colour with a Δ badge, and removed parts
struck through and faded, and opens its requirements and decisions.

## Reference forms

References are typed strings. Line numbers are never a reference form.

| Form | Resolves when |
| --- | --- |
| `code:path` | The repository file or directory exists. |
| `code:path#Name` | The Go file declares a top-level function, type, variable, or constant `Name`. |
| `code:path#Type.Name` | The Go file declares method `Name` on `Type`, a field `Name` of struct `Type`, or a method `Name` of interface `Type`. |
| `doc:path` | The repository file exists. |
| `doc:path#anchor` | The Markdown file has an ATX (`## Title`) or setext (`Title` over a line of `=` or `-`) heading with that GitHub anchor: lower-cased, letters, digits, `-` and `_` kept, spaces turned into `-`, `-1`, `-2` for repeats. Headings in code fences and front matter do not count; a fence closes only on a run of its own character (backtick or tilde) at least as long as the one that opened it, and a list item, table row, or quote above a line of `-` is not a heading. |
| `adr:0007` | `docs/adr/` has ADR 0007. |
| `term:Agent Preset` | `GLOSSARY.md` has that term, spelled exactly. |
| `table:evie.jobs` | The facts have database `evie` with table `jobs`. |
| `route:GET /api/x`, `route:/api/x` | The facts have that route; without a method, any method matches. The pattern starts with `/`; a method is upper case and one space separates it from the pattern. |
| `command:todo add`, `command:todo` | The facts have that binary and word path; a bare binary matches any of its commands. |
| `pkg:internal/agent` | The facts have that package. |
| `node:pkg:internal/agent` | An explorer node has that id (in a proposal, also nodes it adds). |

Paths are clean and relative to the repository root. In the page, `code:` and
`doc:` references link into your own checkout when it is opened from disk;
served by `evie serve`, they open GitHub at the build's commit, or at master
when the binary has no commit stamp or was built with uncommitted changes
(`build.js`, below). A `code:` reference to a declaration links its file.
ADRs show their title, terms
show their definition inline, `pkg:` and `node:` references select the node,
and `table:`, `route:`, and `command:` references open the package that
defines them with the fact highlighted.

## What the check reports

- facts that differ from a fresh extraction, naming the extract command;
- a package with no component node, and a `pkg:` node whose package is gone;
- a dead reference of any form above, including node `terms`, `links`,
  `contains`, flow participants, and diagram nodes;
- a statement, non-import edge, flow step, state, or transition with no
  source, and a flow or sequence diagram step outside a design proposal that
  cites no code;
- an import edge or `via: import` flow step the import graph lacks;
- a design proposal that changes or removes a missing item, or adds one that
  exists; a design proposal node operation on a part, or one that carries
  `children`;
- a data file missing from the manifest, or listed but missing;
- a data file with CRLF line endings (the repository's `.gitattributes`
  checks the explorer's files out with LF);
- malformed data: wrapper lines, JSON, unknown or wrongly cased keys, missing
  required fields, values outside their vocabulary, a diagram field of another
  kind, unsupported format versions;
- duplicate node, flow, diagram, or design proposal ids, and component nodes
  without a group.

The check proves a source exists, not that a sentence is true. Re-read every
source you cite.

## Viewer model and tests

`explorer.js` keeps its pure model (data merging, view selection, layout,
reference links, the address fragment) apart from the code that draws SVG and
DOM, and exports the model under Node. The tests load it there:

```sh
node --test 'docs/architecture/explorer/test/*.test.js'
```

Pass the glob in quotes; a bare directory does not run. The tests run every
layout on `test/fixture.js`, which has every diagram kind and two design
proposals, and on the real data: nodes never overlap, every edge starts and
ends on its nodes, positions do not depend on input order, edge labels never
cover one another, and group and focus views keep edges clear of other nodes.
They also cover the proposal overlay and its styling, address round trips,
proposal-path refusal, reference resolution, search, and that labels pass
through as text and the page never parses data as markup.

## The page

Each view has its own layout rather than one general graph engine:

| View | Address | Layout |
| --- | --- | --- |
| Context | `#view=context` | People above Evie; outside systems reached over the network on its right, local programs on its left. |
| Containers | `#view=containers` | Lanes by kind (people, interfaces and CLIs, processes, stores, outside services), Evie's lanes inside a system boundary; edges crossing a lane pass between its nodes. |
| Inside a container | `#view=inside&of=container:serve` | What the container's `contains` list names, layered like a group. |
| Group overview | `#view=components` | One panel per group with its subgroups and packages; arrows between groups labelled with import counts. |
| Group | `#view=group&of=runtime` | Packages layered by import depth (importers above), wide layers wrapped, crossings reduced; out-of-group dependencies as faded chips below, their edges shown on hover. |
| Focus | `#view=focus&of=pkg:internal/agent` | Callers, the node, its dependencies, from imports and content edges. |
| Flows | `#view=flows` | A card per flow. |
| Flow | `#view=flow&of=chat-turn&step=3` | A sequence diagram with lifelines and numbered steps; the current step is highlighted, its statement and sources are in the panel, and the view follows it. |
| Diagram | `#view=diagram&of=<diagram id>` | Graphs layered top to bottom (lanes as bands flowing left to right), state diagrams from an initial marker, er diagrams as table cards from the facts with foreign keys joined column to column. |

The address fragment holds the view and its subject (`of`), the selected node
(`node`) or glossary term (`term`), the flow step (`step`), the open panel
(`panel=proposals`), and the active proposals (`proposals=a,b`), so a reload
or a bookmark lands in the same place. A view whose subject no longer exists
falls back to the view above it with a notice.

Click selects and opens the panel; double click, Enter on a selected node, or
the panel's buttons open the level below (Evie to its containers, a container
to what it contains, a package to its focus view); the breadcrumb leads back,
and level changes zoom in or out of the part opened. Hover lights a node's
neighbours and dims the rest. The wheel zooms, dragging pans, and the fit
button (or `0`) fits the view. `/` searches packages by name or path, glossary
terms by term or avoided synonym, flows, and diagrams; the arrow keys step a
flow; Esc closes the panel. Colours follow the system's light or dark
preference.

### Served by `evie serve`

`evie serve` serves the snapshot under `/explorer/`, GET and HEAD only, with a
404 for anything not in it, and adds a generated `/explorer/build.js`:

```text
evieExplorer.build({"repository":"https://github.com/davidadel66/evie","commit":"<vcs.revision>","modified":false});
```

The repository comes from the main module path and the commit and modified
flag from Go's build stamp; the commit is empty when the binary is
unstamped. The page loads `build.js` only when it is not opened from disk.
Evie's System view frames `/explorer/?embedded=1`; the embedded flag hides
the brand and keeps the light colours, like Evie. The frame is sandboxed
without same-origin access, and the page's content-security policy sandboxes
it the same way when opened on its own.
