# System explorer

## Problem Statement

Evie has grown to four commands, 33 internal packages, three SQLite
databases, more than a hundred HTTP routes, dozens of CLI subcommands, and a
long trail of specs and ADRs. The only architecture pictures are the memory
diagrams drawn by hand on 3 September 2026, which predate most of today's
code. The owner cannot see the whole system in one place, cannot tell which
part does what or who calls it, and cannot trust a written description
without checking it against the code. Future feature work (`/flow`) has
nowhere to show what a change would add or alter.

## Solution

A single interactive page, the **explorer page**, opened straight from disk,
that shows everything Evie does at four levels: system context, containers,
components (every package, grouped under the architecture map in
`AGENTS.md`), and the key runtime flows. Diagrams are drawn by the page from
data, never as images. Every written statement links to the code, spec, ADR,
or glossary term it rests on.

The page reads **explorer data**: generated **facts**, which the **explorer
tool** extracts mechanically from the code (packages, import edges, commands,
routes, tables, glossary terms, ADR titles), and written **content**, which
agents author on top of those facts. Agents edit the data, never the page.
A drift check inside `./scripts/verify-change.sh` re-extracts the facts and
fails when the data no longer matches the code: a missing package, a dead
link, or a stale edge.

A **design proposal** is a data file describing what one feature would add,
change, or remove; the page toggles it on top of the built system.

## User Stories

1. As the owner, I want to open the explorer from disk with a double click, so that I need no server, build, or network.
2. As the owner, I want a system-context view of Evie, its owner, and every outside system it talks to, so that I see its boundaries first.
3. As the owner, I want a containers view of Evie's processes, user interfaces, command-line tools, and data stores, so that I see what actually runs and where data lives.
4. As the owner, I want every Go package under `cmd/` and `internal/` shown as a component, so that nothing in the code is invisible.
5. As the owner, I want components grouped under Memory, Agent runtime, Intelligence, and Infrastructure, so that the map matches how I think about Evie.
6. As the owner, I want a group overview that shows how much each group depends on the others, so that I see coupling at a glance.
7. As the owner, I want to drill into one group and see its packages laid out by dependency depth, so that I read it top to bottom.
8. As the owner, I want a focus view of one package with its callers on one side and its dependencies on the other, so that I understand its place without a tangle of lines.
9. As the owner, I want moving between levels to animate as a zoom, so that I keep my bearings.
10. As the owner, I want hovering a node to highlight its neighbours and dim everything else, so that I can trace connections.
11. As the owner, I want to pan and zoom any diagram and fit it to the screen, so that large views stay usable.
12. As the owner, I want clicking a node to open a panel that says what it does, the data it owns, its interfaces, who calls it, its glossary terms, and its gotchas, so that one click answers "what is this?".
13. As the owner, I want every sentence in that panel to carry links to its sources, so that I can check any claim myself.
14. As the owner, I want code links to open the file in my own checkout, so that they match the branch I am on.
15. As the owner, I want glossary terms shown with their definitions inline, so that I learn the vocabulary where I meet it.
16. As the owner, I want ADR links shown with their titles, so that I know what a decision is before opening it.
17. As the owner, I want "who calls it" and "depends on" computed from the real import graph, so that they cannot drift from the code.
18. As the owner, I want a package's HTTP routes, with the guard that protects each, listed in its panel, so that I see its web surface and its authorization boundary.
19. As the owner, I want a command package's CLI subcommands listed in its panel, so that I see every way to drive it.
20. As the owner, I want a database owner's tables, columns, and foreign keys listed in its panel, so that I see the data it persists.
21. As the owner, I want key runtime flows (a chat turn, a workflow run, memory compilation, plugin activation, and the other flows the code shows are central) drawn as sequence diagrams, so that I see how parts cooperate over time.
22. As the owner, I want to step through a flow one message at a time with the current step highlighted and its sources shown, so that I can follow it at my own pace.
23. As the owner, I want flows that involve a node listed in that node's panel, so that I can jump from a part to the behaviour it takes part in.
24. As the owner, I want the memory diagrams redrawn against today's code inside the explorer, with the original drawings linked, so that I have a current picture and can compare it with the old one.
25. As the owner, I want state diagrams for lifecycles and data diagrams built from real tables, so that lifecycles and persisted shapes are shown accurately.
26. As the owner, I want to search by name, package path, or glossary term and jump to the match, so that I find things fast.
27. As the owner, I want the address bar to hold the current view, node, flow step, and active proposals, so that I can bookmark or share a precise place.
28. As the owner, I want the page to follow my light or dark preference, so that it is comfortable to read.
29. As the owner, I want to toggle a design proposal on top of the built system with added, changed, and removed parts styled distinctly, so that I see what a feature would do before it is built.
30. As the owner, I want a proposal's requirements and decisions listed with their sources, so that I can review the feature's intent beside its shape.
31. As the owner, I want an example proposal I can load on demand, so that I can see the overlay working before any real proposal exists.
32. As an agent, I want a documented data format, so that I can add or correct content without touching the page.
33. As an agent, I want one written-content file per subsystem, so that parallel work rarely conflicts.
34. As an agent, I want one file per design proposal, so that `/flow` can add a proposal without editing anything else.
35. As an agent, I want a single command that regenerates the facts, so that keeping them current costs one step.
36. As an agent, I want the drift check to list every problem with its file, item, and fix, so that I can repair drift in one pass.
37. As an agent, I want the check to fail when a package has no node, so that new packages are always described.
38. As an agent, I want the check to fail when a node names a package that is gone, so that deleted code leaves no ghost.
39. As an agent, I want the check to fail on a dead link to a file, Go symbol, heading, ADR, glossary term, table, route, or command, so that renames surface immediately.
40. As an agent, I want the check to fail on an import edge the code no longer has, so that diagrams never show a dependency that isn't there.
41. As an agent, I want the check to fail when the facts differ from a fresh extraction, so that the facts are always the code's own answer.
42. As an agent, I want every statement required to cite at least one source, so that unsourced prose cannot enter.
43. As an agent, I want the format to reserve room for nested detail beneath a node, so that a later deep-research pass can add depth without a format change.
44. As a reviewer, I want the extraction rules covered by tests on a fixture module, so that I can trust what "mechanical" means.
45. As a reviewer, I want the page's layout and overlay logic covered by tests that run without a browser, so that regressions are caught in the normal check.
46. As the owner, I want the check to run in CI through the existing verification script, so that drift cannot merge.
47. As the owner, I want extraction to touch only throwaway files, so that building the facts never reads or alters my real databases.
48. As the owner, I want the facts to come out byte-identical on macOS and in Linux CI, so that the check never fails for platform reasons.

## Acceptance Criteria

1. Opening the explorer page from disk, with the network disabled, renders the system-context view and logs no console errors.
2. The four levels are reachable from the page: context, containers, components (group overview, group view, focus view), and flows. Selecting any node opens its panel with summary, what it does, data it owns, interfaces, who calls it, depends on, glossary terms with definitions, gotchas, extracted routes, commands or tables where they exist, involved flows, and source links.
3. Every package that `go list ./cmd/... ./internal/...` reports has a component node whose panel contains written content beyond a one-line summary, and every statement in the data cites at least one source that resolves.
4. The flows view lists at least: chat turn, approval handshake, plugin activation, subagent run, workflow run, workflow schedule activation, memory compilation, background memory pass, automatic recall, MCP sign-in, change merge, and `serve` startup. Each renders as a sequence diagram whose participants are explorer nodes and whose every step cites code.
5. The Memory group's nodes carry redrawn versions of the seven memory diagrams (stage evolution, turn context flow, semantic write path, semantic domain model, scope promotion, temporal lifecycle, replay and recovery) that cite current code. The original D2 and Structurizr files and their SVGs are linked and unchanged.
6. Loading the example proposal through the page's proposal parameter shows a proposal toggle; turning it on restyles added, changed, and removed nodes and edges distinctly and lists the proposal's requirements and decisions with links; turning it off restores the built view.
7. `go run ./scripts/explorer check` exits non-zero and names the item for each of: a package with no node; a node for a package that no longer exists; a dead file, Go symbol, heading, ADR, glossary term, table, route, command, package, or node reference; a statement with no source; an authored import edge or flow step the import graph does not contain; a proposal that changes or removes a node that does not exist; a data file not listed in the manifest or listed but missing; malformed data; facts that differ from a fresh extraction, with the extract command in the message.
8. `go run ./scripts/explorer check` exits zero on the finished branch, and `go run ./scripts/explorer extract` leaves the facts byte-identical.
9. `./scripts/verify-change.sh` runs the drift check and the page's tests, and fails when either fails; `AGENTS.md` describes both.
10. A README beside the page documents the data format, the reference forms, the two commands, and how to add a proposal.

## Implementation Decisions

### Modules

- **Explorer page**: a static HTML page with one classic-script viewer and one stylesheet, at `docs/architecture/explorer/`. No ES modules, no build step, no third-party code. It loads the data manifest, which loads each data file as a classic script, merges them, and renders. Nothing is fetched, so it works from `file://`.
- **Viewer model**: the viewer's pure functions (data merging and validation-free normalisation, view selection, per-view layout, proposal overlay, address-bar state, reference-to-link resolution) are exported for Node when loaded under Node, so they are testable without a browser. Rendering to SVG and DOM wiring sit on top of the model.
- **Explorer tool**: a Go command under `scripts/` with two subcommands, `extract` and `check`, using only the standard library and modules already in `go.mod`. It may import Evie's own packages to run their real migrations.
- **Explorer data**: a `data` directory beside the page holding the manifest, the generated facts, one written-content file per subsystem, and a `proposals` directory with one file per design proposal. An `examples` directory beside it holds the example proposal, which the manifest does not list.

### Data format (the contract `/flow` will write against)

- Every data file is a classic script whose first line is exactly the viewer's registration call opening, whose body is one strict JSON value (no comments, no trailing commas), and whose last line closes the call. The tool parses a file by stripping the fixed first and last lines. The manifest uses its own registration call with a JSON array of data-file paths relative to the data directory, loaded in order.
- Every data object carries a format version (starting at 1) and a kind: facts, content, or proposal.
- **Facts** (generated, never hand-edited): packages (path relative to the module root, name, command or library, first sentence of the package doc, whether it is test support); in-module import edges between packages; commands (binary, word path, defining package, source as file and function); routes (method if the pattern has one, pattern, defining package, guard wrapper name if any, source as file and function); databases (id, owning package, tables with columns, types, not-null, primary-key position, and foreign keys); glossary entries (term, definition, avoided synonyms); ADRs (number, title, path). Output is sorted and contains no timestamps or line numbers.
- **Content**: an owner id (the subsystem), then nodes, edges, and flows.
  - A **node** has an id, a level (context, container, component), a kind (person, system, external system, process, user interface, CLI, store, package), a title, a summary statement, statement lists for what it does, data it owns, interfaces, and gotchas, glossary terms, diagrams, extra links, and an optional `contains` list naming the nodes a container zooms into. Component nodes also carry a group (memory, runtime, intelligence, infrastructure), an optional subgroup, and an optional list of other groups whose views also show it. A package's node id is the package path with a `pkg:` prefix. Every node may carry `children`, a list of nodes with level `part` in the same shape, reserved for the later depth pass.
  - A **statement** is a text sentence with one or more references and an optional longer detail, reserved for the depth pass. ("Statement", not "claim": Claim is a Semantic Memory term.)
  - An **edge** has from, to, label, a via (import, call, http, sql, process, network, file, event), and references. An edge with via `import` must match a facts import edge between the two packages; every other edge cites at least one source.
  - A **flow** has an id, title, summary statement, trigger statement, participant node ids, and ordered steps; each step has from, to, label, a kind (call, return, async, note), an optional via, and at least one reference.
  - A **diagram** has an id, title, kind (graph, sequence, state, er), and summary statement. A graph has local nodes (optionally pointing at explorer nodes), edges, and optional lanes. A sequence has the flow shape. A state diagram has states, an initial state, and transitions, each cited. An er diagram names a database and the tables to show; columns and relationships come from the facts, so the drawing cannot disagree with the schema.
- **References** are typed strings: `code:` a repository file or directory, optionally `#Name` or `#Type.Method` for a Go declaration in that file; `doc:` a repository file, optionally `#anchor` for a Markdown heading (GitHub slug rules); `adr:` a four-digit ADR number; `term:` a glossary term; `table:` database id and table name; `route:` an optional method and a pattern; `command:` binary and words; `pkg:` a package path; `node:` an explorer node id. Line numbers are not a reference form.
- **Design proposal**: an id, feature name, title, status (proposed, accepted, building, example), an optional spec reference, a summary statement, operations on nodes, edges, and flows (each add, change, or remove, with the item's fields), and requirement and decision statements. A change or remove must target an existing item; an add must not collide with one.

### Extraction rules

- **Packages**: every directory under `cmd/` and `internal/` with at least one non-test Go file, excluding `testdata` and `node_modules`.
- **Import edges**: in-module imports from all non-test Go files of a package, ignoring build constraints (the union), so every platform extracts the same edges.
- **Commands**: string literals compared, by equality, inequality, or switch case, against an element of a command's argument slice, assembled into word paths by argument position relative to the arguments after the binary name. A golden test pins the result for the real commands against their usage strings. A comparison the rule cannot place fails extraction rather than being skipped.
- **Routes**: `Handle` and `HandleFunc` calls on a `ServeMux` in non-test files. A literal pattern is taken as is; a literal prefix joined to the key of a range over a composite literal with literal keys is expanded once per key; any other pattern expression fails extraction, naming the file and function. The guard is the name of the server method wrapping the handler argument, if there is one.
- **Tables**: each database's real open-and-migrate entry point runs against a fresh temporary file (finance with its home directory pointed at a temporary directory inside the tool's own process), then the tool reads the schema from SQLite (tables, excluding SQLite-internal and shadow tables, with columns and foreign keys). Extraction never opens the owner's real databases.
- **Glossary and ADRs**: glossary entries are the bold `Term:` headings in `GLOSSARY.md` with their definition text and avoided synonyms; ADRs are the numbered files in `docs/adr/` with their first heading as title.

### Check rules

`check` re-extracts in memory, compares the result byte-for-byte with the facts file, then loads every manifest-listed data file plus the examples and validates them against the merged data. It reports every problem, not only the first, each with the data file, the item, and a fix hint, and exits non-zero if there is any. The problems are those listed in acceptance criterion 7, plus duplicate node ids and component nodes without a group.

### Viewer behaviour

- **Frame**: a top bar with the level tabs and search; a left rail with the group list, legend, and proposal toggles; the diagram canvas; and a right panel for the selected node, flow step, or proposal.
- **Purpose-built layouts** per view rather than one general graph-layout engine:
  - context: owner, Evie, and outside systems arranged by role;
  - containers: lanes by kind (people, interfaces and CLIs, processes, stores, outside services);
  - component group overview: one panel per group with its subgroups and packages, and aggregated cross-group dependency arrows labelled with counts;
  - group view: the group's packages layered by import depth (longest path) and ordered to reduce crossings, with out-of-group dependencies as faded boundary chips;
  - focus view: callers, the node, and dependencies in three columns;
  - flow: a sequence diagram with lifelines and numbered steps, stepped with buttons or the arrow keys;
  - state and graph diagrams: small layered layouts; er diagrams: table cards with foreign-key connectors.
- **Interaction**: click selects; drill-down (double click or the panel's zoom button) goes container → contained nodes, group → group view, package → focus view, with a breadcrumb back. Level changes animate the viewport. Hover highlights neighbours and dims the rest. Wheel zooms, drag pans, and a fit control resets. Nodes are keyboard-focusable with accessible names.
- **Proposal overlay**: added parts are drawn in the "added" colour with dashed outlines, changed parts in the "changed" colour with a badge, removed parts in the "removed" colour, struck through and faded. The legend explains each.
- **Address bar**: the URL fragment holds view, node, flow step, and active proposals. A proposal query parameter loads one extra proposal script by a relative path inside the explorer directory; absolute paths, schemes, and `..` are refused.
- **Links**: code and doc references become relative links into the checkout; glossary terms show their definition; ADRs show their title; table, route, command, package, and node references jump within the page.
- **Theme**: colours are tokens with light and dark values following the system preference.

### Component grouping

Memory: memory, memorybackground, memoryeval. Agent runtime: agent, tools, plugins, composition, subagents, delegation, task, taskeval, untrusted, repoinstructions, workflow, workflowruntime, procedural. Intelligence: openrouter, localembedding, localextractor. Infrastructure, by subgroup: Persistence (eviedb, also shown in the Memory view), Surfaces (web and the four commands), Integrations (mcpconn and its test server, mcpchat, github, finance, youtube), Local host (changes, worktree, localfiles, localterminal, workspacetabs), Observability (usage). Grouping is data and may be refined with a reason in the decision record.

### Verification script

`./scripts/verify-change.sh` gains two required steps, the drift check and the viewer model's Node tests, and `AGENTS.md`'s Verification section names them. CI inherits both through the existing workflow.

## Testing Decisions

- Good tests here exercise external behaviour only: what the tool extracts from a given module, what the check reports for given data, what the viewer model returns for given data. They never assert on private helpers.
- **Seam 1, explorer tool tests on fixture modules**: Go tests run extraction and checking against small modules and data sets under the tool's `testdata`, and run table extraction against Evie's real migrations in a temporary directory. Catches: every extraction rule (build-tagged files, test-only imports excluded, range-expanded routes, guard names, command word paths, refusal of unplaceable patterns) and every check failure with its message. Misses: whether written prose is true.
- **Seam 2, the drift check on the real repository** inside the verification script. Catches: real drift on every change. Misses: a statement that cites a real source and still says something false. Content tickets therefore re-read every source they cite, and the review samples statements against their sources.
- **Seam 3, Node's built-in test runner on the viewer model**: tests load the viewer under Node and call its exported functions. Catches: data merging, view selection, layout geometry (no overlapping nodes, every edge endpoint on its node, deterministic positions), proposal overlay styling, address-bar round trips, reference resolution and escaping. Misses: real browser rendering and interaction feel, covered by the manual demonstration.
- Prior art: `testdata` fixtures in `internal/workflow`, `internal/tools`, and `internal/eviedb`; temporary databases via `OpenDBAt` under `t.TempDir()` in `internal/plugins` and `internal/tools` tests; `go/parser` in `internal/workflow` tests.

## Dependencies

- No new production or development dependency. Node 24 is already installed by CI for the UI.
- The Go toolchain and module cache, already required by the verification script.

## Risks

- **Wrong but cited prose.** The check proves a source exists, not that the sentence is true. Contained by symbol-level references, per-ticket re-reading of cited sources, review sampling, and keeping derived facts (callers, routes, tables, schema in data diagrams) out of prose entirely.
- **Friction on every change.** Any change that adds or removes a package, import edge, command, route, table, glossary term, or ADR fails verification until the facts are regenerated, and a new package needs a node. Accepted by the owner; contained by a single extract command named in the failure message.
- **Command extraction misses a subcommand.** The rule is a static reading of argument comparisons. Contained by a golden test against the usage strings and by failing on comparisons it cannot place.
- **Platform-dependent facts.** Contained by ignoring build constraints, sorted output, no timestamps, and the pure-Go SQLite driver pinned in `go.mod`.
- **Touching real data.** Extraction runs migrations only on temporary files; finance's home override happens only inside the tool's process. Tests assert the owner's paths are never opened.
- **Script injection through the proposal parameter.** Only relative paths inside the explorer directory load; the page is a local file the owner opens.
- **Parallel content edits.** One content file per subsystem and one file per proposal keep merges conflict-free in the common case.

## Verification

- `./scripts/verify-change.sh` from the repository root (after `npm --prefix internal/web/ui ci` and a UI build in a fresh worktree), which now includes `go run ./scripts/explorer check` and the viewer's Node tests.
- `go run ./scripts/explorer extract` followed by `git diff --exit-code` on the facts file.
- Manual demonstration: open the explorer page from disk with the network off; walk context → containers → a group → a package's focus view; open a package panel and follow a code link, a glossary term, and an ADR; play the chat-turn flow step by step; open a memory diagram and its original; load the example proposal and toggle it; reload with the fragment and land in the same place.

## Out of Scope

- Changing `/flow` or `/implement` to write proposals or keep the map current (done separately).
- Depth beyond one level per node; `children` and statement detail are reserved, not filled.
- Real design proposals; only the example ships.
- Editing or deleting the original memory diagrams.
- Hosting the explorer, GitHub links, or a server mode.
- Any change to Evie's product UI, runtime, or schema.
- Extracting facts beyond packages, imports, commands, routes, tables, glossary terms, and ADRs (for example plugin manifests or tool registries).

## Further Notes

- The explorer is repository tooling, not Evie domain language, so no glossary entries are added. Within the explorer, "design proposal" distinguishes its proposals from the glossary's Memory Operation Proposal.
- The memory originals live in `docs/architecture/memory/`.

## Tickets

- #186 System explorer: tracer bullet from extraction to page (blocked by: none)
- #187 System explorer: extract commands, routes, and tables (blocked by: #186)
- #188 System explorer: full viewer with levels, flows, and design proposals (blocked by: #186)
- #189 System explorer content: system context, containers, and Intelligence (blocked by: #186)
- #190 System explorer content: agent loop and capabilities (blocked by: #186)
- #191 System explorer content: workflows (blocked by: #186)
- #192 System explorer content: integrations and local host (blocked by: #186)
- #193 System explorer content: Memory and the redrawn memory diagrams (blocked by: #186, #187)
- #194 System explorer content: persistence and surfaces (blocked by: #186, #187)
