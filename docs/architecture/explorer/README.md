# System explorer

The explorer page shows what Evie does: every package under `cmd/` and
`internal/` as a component, grouped under the architecture map in
`AGENTS.md`, with written content on top of facts extracted from the code.
Open [`index.html`](index.html) straight from disk; it needs no server, build,
or network. Specification:
[`system-explorer.spec.md`](../../../cmd/evie/docs/active/system-explorer.spec.md).

Agents edit the data, never the page. The drift check keeps the data honest:
it fails when the data and the code disagree.

## Files

| Path | What it is |
| --- | --- |
| `index.html`, `explorer.js`, `explorer.css` | The page, its one classic-script viewer, and its stylesheet. No modules, no build, no third-party code. |
| `data/manifest.js` | The ordered list of data files the page loads. |
| `data/facts.js` | Generated facts. Never edit by hand; run `extract`. |
| `data/<owner>.js` | Written content, one file per subsystem owner (below). |
| `data/proposals/<id>.js` | One file per design proposal. |
| `examples/<id>.js` | Example design proposals. Checked, but not listed in the manifest; the page will load one through its proposal parameter (ticket #188). |
| `test/*.test.js` | Viewer model tests for Node's built-in runner. |

Content files and the packages whose component nodes they hold:

| File | Packages |
| --- | --- |
| `system.js` | System context and containers (no packages). |
| `intelligence.js` | openrouter, localembedding, localextractor |
| `memory.js` | memory, memorybackground, memoryeval |
| `runtime.js` | agent, tools, plugins, composition, subagents, delegation, task, taskeval, untrusted, repoinstructions |
| `workflows.js` | workflow, workflowruntime, procedural |
| `platform.js` | eviedb, web, usage, cmd/evie, cmd/todo |
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
  Go file, excluding `testdata` and `node_modules`. `doc` is the first
  sentence of the package comment in the first file, by name, that has one.
  `command` means package `main`; `testSupport` means the package name ends
  in `test`, Go's convention for test helpers.
- **imports**: in-module imports between those packages from every non-test
  file, ignoring build constraints, so every platform extracts the same edges.
- **commands**, **routes**, **databases**: empty until their extraction lands
  (ticket #187). The shapes above are what the check and viewer already read.
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
at least one reference. A step with via `import` must follow an import edge:
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
- Added packages and import edges describe code that does not exist yet, so
  they are not matched against the facts. Statements still cite sources that
  exist, usually the feature's spec, and nodes the proposal adds can be cited
  as `node:` references inside it.

To add a design proposal:

1. Write `data/proposals/<id>.js` in the wrapper above.
2. Add `"proposals/<id>.js"` to `data/manifest.js`. The page cannot list a
   directory from `file://`, so the manifest is how it finds the file.
3. Run `go run ./scripts/explorer check` and fix what it reports.

## Reference forms

References are typed strings. Line numbers are never a reference form.

| Form | Resolves when |
| --- | --- |
| `code:path` | The repository file or directory exists. |
| `code:path#Name` | The Go file declares a top-level function, type, variable, or constant `Name`. |
| `code:path#Type.Name` | The Go file declares method `Name` on `Type`, a field `Name` of struct `Type`, or a method `Name` of interface `Type`. |
| `doc:path` | The repository file exists. |
| `doc:path#anchor` | The Markdown file has an ATX heading with that GitHub anchor: lower-cased, letters, digits, `-` and `_` kept, spaces turned into `-`, `-1`, `-2` for repeats; headings in code fences do not count. |
| `adr:0007` | `docs/adr/` has ADR 0007. |
| `term:Agent Preset` | `GLOSSARY.md` has that term, spelled exactly. |
| `table:evie.jobs` | The facts have database `evie` with table `jobs`. |
| `route:GET /api/x`, `route:/api/x` | The facts have that route; without a method, any method matches. |
| `command:todo add`, `command:todo` | The facts have that binary and word path; a bare binary matches any of its commands. |
| `pkg:internal/agent` | The facts have that package. |
| `node:pkg:internal/agent` | An explorer node has that id (in a proposal, also nodes it adds). |

Paths are clean and relative to the repository root. In the page, `code:` and
`doc:` references link into your own checkout, ADRs show their title, terms
show their definition, and `pkg:` and `node:` references select the node.

## What the check reports

- facts that differ from a fresh extraction, naming the extract command;
- a package with no component node, and a `pkg:` node whose package is gone;
- a dead reference of any form above, including node `terms`, `links`,
  `contains`, flow participants, and diagram nodes;
- a statement, non-import edge, flow step, state, or transition with no
  source;
- an import edge or `via: import` flow step the import graph lacks;
- a design proposal that changes or removes a missing item, or adds one that
  exists;
- a data file missing from the manifest, or listed but missing;
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

Pass the glob in quotes; a bare directory does not run. The page's address
fragment holds the view and the selected node, for example
`index.html#node=pkg:internal/agent`.
