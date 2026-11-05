# System page and History decisions

Spec: [system-page.spec.md](system-page.spec.md).

- **2026-10-31 — The explorer moves into Evie as a System entry beside Data
  and Settings.** David asked to add it to Evie's main web page and chose a
  System entry beside Data and Settings ("i think it should be a System entry
  beside Data and Settings"). Claude placed it directly under Data, a
  destination view like it, rather than beside Settings in the footer, which
  opens a dialog. This amends `system-explorer.spec.md`'s Out of Scope
  ("Hosting the explorer, GitHub links, or a server mode"; "Any change to
  Evie's product UI").
- **2026-10-31 — `evie serve` serves a snapshot built into the binary, and
  served code links go to GitHub at the build's commit.** David agreed to
  Claude's recommendation ("yeah i gree with your recommendation"): a
  built-in snapshot rather than reading live from the checkout, and GitHub
  links rather than serving source files. Links stay local file links when
  the page is opened from disk.
- **2026-10-31 — History is a fifth explorer tab fed by a written,
  link-checked `CHANGELOG.md`, one entry per merge to master, backfilled
  once.** David agreed to the recommendation: "History" rather than "Logs"
  (runtime logs) or "Stories" (planned work), a written changelog rather than
  one generated from git (which would break the drift check's byte
  comparison), one entry per merge, and entries written by hand until
  `/implement` is changed separately.
- **2026-10-31 — The changelog entry format is approved.** David answered
  "yes to both" to the brief's first two questions. Format: level-two heading
  `YYYY-MM-DD — Title`; `Branch:` (required); `Merge:` (optional) or
  `Commits: <from>..<to>`; `Touches:` package paths (required, not
  link-checked); `Sources:` explorer references and `#n` numbers (at least
  one); then prose. It is the format `/implement` will write later.
- **2026-10-31 — `/explorer/` is served outside the API guard, like Evie's
  own static files.** David approved ("yes to both"). The route serves only
  embedded files of the public repository and reads no data.
- **2026-10-31 — Served code links use the build's commit, falling back to
  master only when there is no commit stamp or the build had uncommitted
  changes.** David agreed ("i agree w your recommendation for q3 as well"),
  accepting that a build from a commit not yet on GitHub (for example a local
  feature branch) links to a commit GitHub doesn't have, so those links 404.
- **2026-10-31 — Two-way doors from the brief.** Claude decided, and the
  brief showed: a Go embed package beside the explorer files (rejected:
  copying into the UI build, which could ship a stale explorer), with the
  explorer tool's extraction extended to that one package outside `cmd/` and
  `internal/` (amends `system-explorer.spec.md`'s extraction rule); a
  sandboxed frame without same-origin access (rejected: a same-origin frame
  that could call Evie's API; precedent: Workspace tabs); the embedded
  explorer matching Evie's light theme and hiding its brand bar; the commit
  read from Go's build stamp rather than linker flags; touched packages
  written by hand and not link-checked; merge commits optional and verified
  with git when given; `#n` numbers format-checked only.
- **2026-10-31 — Merge into master when done.** David asked: "merge into
  master after you are done". Both `feat/system-explorer` and this branch
  merge into master after Finish. Pushing stays with David.
- **2026-10-31 — System is not remembered across reloads, like Data.** The
  spec first said System would be remembered "as it does for Data", but Evie
  remembers no workbench view across reloads. Claude corrected the spec to
  match Data rather than adding view persistence, which would change Data's
  behaviour and was never asked for.
- **2026-10-31 — Changelog `Touches:` accepts any clean repository path, and
  `Commits: a..b` includes both ends.** Decided while building ticket #196.
  `Touches:` was approved as package paths, but a merge can touch no package
  (the system explorer touched only `scripts/` and `docs/`), so any clean
  relative path is accepted and non-package paths show as plain text; every
  entry valid under the approved format stays valid. An inclusive range lets
  the root commit start a range and one commit stand alone; the check
  requires the first commit to be the last or its ancestor.
- **2026-11-01 — The explorer inside Evie is always dark.** David asked,
  after seeing the System view: "now just make it darkmode". This overturns
  the brief's two-way door that matched Evie's light theme. Opened from disk,
  the explorer still follows the system preference. Claude kept one colour
  table using `light-dark()` so the frame forces dark through
  `color-scheme` instead of duplicating the dark palette.
- **2026-11-04 — The System view opens on a vision map.** David approved a
  prototype (branch `prototype/system-vision`, "i love it") and asked to
  switch the System view once the vision skill absorbed it ("once youre done
  with the skill, switch evie's system view"). The view gets three tabs: Map
  (the default: a subway front door whose stations open the flows, with no
  code shown), History, and Where is it? (today's explorer opened on its
  History tab and its components index). David answered "yes to both".
- **2026-11-04 — The built map page is committed and the drift check guards
  the map.** David approved ("yes to both"): the vision-built page is
  committed and embedded like `facts.js`, and the explorer's drift check in
  `verify-change.sh` also fails when a code reference or fact chip in
  `.vision/map.json` no longer resolves, or when the built page does not
  carry the hash of the current map. Rebuilding runs locally through the
  vision skill, because CI cannot run a script that lives outside the repo.
- **2026-11-04 — /flow keeps the System current after each piece of work.**
  David asked: "make sure that flow skill gets updated as well". `/flow`'s
  step 5 now updates the vision map through the skill, drafts the branch's
  `CHANGELOG.md` entry, and regenerates the facts and runs the drift check,
  replacing hand-written changelog entries (this amends the 2026-10-31
  decision that entries are written by hand until `/implement` drafts them).
  AGENTS.md says so in its verification section.
- **2026-11-05 — "Where is it?" and History move into the vision skill, and
  the System page becomes the skill's three tabs: Map, Architecture,
  History.** David asked to move them into the skill ("i think it should be
  part of the skill", "yes, the skill should contain history as well … just
  put them into the skill"). The skill's page now carries Architecture, the
  renamed "Where is it?", with Context, Containers, Components and Flows,
  and History as its own tab beside the Map, and every view's legend
  minimizes. Evie's written explorer content moves into `.vision/map.json`
  as parts, edges, sequences and groups, converted once as the skill's map
  format lays out ("Migrating from Evie's explorer"); the map names
  `CHANGELOG.md` for History and requires a part for every package. The
  fact extractor and `facts.js` stay. The page stays dark inside Evie
  (2026-11-01) through its `theme=dark`. This amends the 2026-11-04 tabs
  (Map, History, Where is it?) and the 2026-10-31 History as a fifth
  explorer tab.
- **2026-11-05 — The explorer viewer and its Node tests retire.** With
  Architecture and History in the skill's page, the viewer (`index.html`,
  `explorer.js`, `explorer.css`), its Node tests, the example proposal, the
  content files and the manifest are removed, and `verify-change.sh` no
  longer runs the Node tests. The drift check's content rules move onto the
  map (every package has a part; references, backed values, imports,
  sequence steps that cite code, participants, contained parts and er
  tables), and its data-file, manifest and design-proposal rules go with
  their inputs. This amends the system explorer's 2026-10-28 decision that
  the viewer's Node tests are a required verification step.
- **2026-11-05 — Design proposals did not carry over into this migration.**
  David wants them back as part of the vision skill ("yes"), added next.
- **2026-11-05 — `build.js` retires, and the 2026-10-31 commit-link rule
  with it.** Claude decided: the skill's page shows no code links (it names
  each source as a plain chip), so nothing on it needs the build's commit;
  History's commits, tickets and pull requests link into the repository the
  map's `meta.repository` names, which the drift check holds to the module
  path in `go.mod`, as `build.js` derived it. `/explorer/` serves only the
  page, with its policy on every response, and the server no longer reads
  Go's build stamp.
- **2026-11-05 — Two-way doors from the migration.** Claude decided: one
  frame for the three tabs, each moving the page's fragment to its own
  address (`#subway`, `#view=architecture`, `#view=history`); Architecture
  opens where the page opens it, on its first level (Context), rather than
  on Components as "Where is it?" did; choosing the selected tab again
  re-applies its view, because the page can change view from inside (a part
  chip on the Map opens Architecture) and Evie's tabs cannot see it, since
  the frame is cross-origin; the page's own tab bar still shows inside the
  frame, as the skill has no option to hide it; `/explorer/` and
  `/explorer/system.html` both serve the page; the drift check now requires
  the map, and changelog sources resolve in the map's reference forms
  (`part:` included, `node:` gone); the facts keep their `history` list,
  which no page reads now, so `extract` is unchanged.
- **2026-11-05 — Evie's outer tabs are dropped in favour of the page's
  own.** Claude decided: the page draws its own Map, Architecture and
  History tabs and can change view from inside, so Evie's bar above the
  frame duplicated them and could fall out of step with what the page
  showed. The System view keeps its System header, styled like Data's, over
  one full-height sandboxed frame of `/explorer/?theme=dark`, which opens on
  the subway map. This amends the migration's two-way doors above: no tab
  moves the frame's fragment, and none re-applies a view.
- **2026-11-05 — The drift check also compares the page's inputs hash.**
  This extends the approved rule that the page carry the current map's hash
  (2026-11-04) to the page's other inputs. The vision skill writes
  `<meta name="vision-inputs-hash">` over the facts file, the changelog and
  the design proposals, and `check` recomputes it as the skill's map format
  says ("Inputs hash") and fails, naming the rebuild, when the page carries
  none or an older one. This closes the spec's risk of a page older than
  the changelog or the facts.
- **2026-11-05 — Design proposals are back in the vision skill.** David
  agreed ("yes", "ok"): a proposal is `.vision/proposals/<slug>.json`, one per
  feature in flight, drawn as an overlay on the Map and Architecture;
  `/flow` writes one during a Big brief and deletes it in "Update the
  System". This settles the entry above that they did not carry over into
  the migration. Claude decided the drift check's share, since CI cannot
  run the skill's full proposal check: each file is one JSON object, its
  `id` is its file name, and its `status` is one the skill knows; the folder
  is absent while there is no proposal, and Evie commits no example.
- **2026-11-05 — The facts drop their `history` list.** Claude decided:
  nothing reads it now that the page's History reads `CHANGELOG.md`, and
  the inputs hash covers the changelog, so `extract` no longer reads the
  changelog and `check` still validates it. This amends the migration's
  two-way door that kept the list.
- **2026-11-05 — The map check keeps the content rules the explorer had.**
  Claude decided, from the branch's review, which found that moving the
  content files into the map dropped some of the drift check's rules: every
  statement of a part and every edge other than an import edge cites a
  source, ids are unique (parts among parts, sequences among sequences,
  diagrams across the map), every sequence step joins two of its
  participants, and an import joins two package parts. Each is a rule of the
  skill's own check too, so a map the skill accepts still passes. This
  extends the migration's entry above that moved the content rules onto the
  map. A missing or malformed map is reported once, and the proposals and
  the page's inputs are still checked without it.
