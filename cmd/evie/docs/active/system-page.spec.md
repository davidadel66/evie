# System page and History

## Problem Statement

The system explorer (`system-explorer.spec.md`) is a page the owner opens from
disk. It is easy to forget, and it is not where the owner spends time: Evie's
own web UI. The owner also has no single place to see how Evie has changed
over time. The history exists, but it is scattered across 213 commits on
master, 23 merges, a handful of pull requests, closed tickets, completed
feature records, and dated decision entries.

## Solution

A **System** entry in Evie's sidebar opens one page with three tabs of its
own (amended 2026-11-04 and 2026-11-05), all drawn by the vision skill from
`.vision/map.json`, the extracted facts, `CHANGELOG.md` and any design
proposals; Evie draws no tab bar over it:

- **Map**, the default: a map of Evie's flows, opening on its subway front
  door, whose stations open the steps inside them.
- **Architecture**: the parts Evie is built from at four levels, Context,
  Containers, Components and Flows (sequence diagrams), each part's panel
  saying what it does, the data it owns, its interfaces and gotchas, and
  where it appears on the Map.
- **History**: a timeline of every merge to master, read from a written,
  link-checked `CHANGELOG.md`. Each part's panel also lists the History
  entries that touched it. The changelog is backfilled once from git and the
  completed feature records; since 2026-11-04 `/flow` drafts each work
  branch's entry (see the decision record).

While a feature is in flight, its design proposal
(`.vision/proposals/<slug>.json`) can be turned on over the Map and
Architecture, showing what the feature would add, change or remove.

`evie serve` carries the committed page as a snapshot built into the binary,
so it always describes the build that is running. The page shows no code: it
names each source as a plain chip, and the drift check, not the page, holds
every source to the code. The explorer viewer that first drew Architecture
and History (`system-explorer.spec.md`) retired on 2026-11-05; its written
content lives in the map.

## User Stories

1. As the owner, I want a System entry in Evie's sidebar, so that the architecture map is one click away where I already work.
2. As the owner, I want the System entry directly under Data, so that it sits with the other destination views.
3. As the owner, I want System to open full-height inside Evie's window on a map of Evie's flows, whose subway stations open the steps inside them, with Architecture and History tabs beside it, so that I see what Evie does first and what it is built from one click away.
4. As the owner, I want the breadcrumb to say System while it is open, so that I know where I am and can go back to my chat.
5. As the owner, I want System to behave like Data as a workbench view (opened from the sidebar, left through the breadcrumb), so that it works the way Evie's other views do.
6. As the owner, I want the System page inside Evie to be dark, so that it reads comfortably.
7. As the owner, I want the System page served by `evie serve` from inside the binary, so that it needs no files on disk and can't drift from the build.
8. As the owner, I want the served page to describe exactly the build I am running, so that I can trust it while debugging that build.
9. As the owner, I want switching between Map, Architecture and History not to reload the page, so that each keeps my place.
10. As the owner, I want an Architecture tab with Context, Containers, Components and Flows, so that I can see the system's boundaries, what runs, every package, and the key runtime flows.
11. As the owner, I want each part's panel to list the Map steps that cite it, each opening the Map there, so that I can go from what a part is to where it acts.
12. As the owner, I want the System page inside Evie isolated from Evie's API, so that page code can never act on my sessions or approvals.
13. As the owner, I want a History tab beside the Map, so that I can see how Evie has changed over time.
14. As the owner, I want History newest first and grouped by month, so that recent work is on top and older work is easy to scan.
15. As the owner, I want each History entry to show its date, title, branch, merge commit, what changed and why, its sources, and the packages it touched, so that one card tells the whole story.
16. As the owner, I want a merge commit, ticket, or pull request in an entry to open on GitHub, so that I can dig into the details.
17. As the owner, I want touched packages in an entry to open their parts, so that I can see where a change landed in the architecture.
18. As the owner, I want each part's panel to list the History entries that touched it, so that I can see a part's own history.
19. As the owner, I want a touched package that no longer exists to show as plain text, so that old history stays readable after code is removed.
20. As the owner, I want History searchable from the Architecture tab's search, so that I can find when something changed.
21. As the owner, I want the changelog backfilled for all of master's history, so that History is complete from day one.
22. As the owner, I want backfilled entries to cite their commits and the completed feature records, so that I can trust them and skim them quickly.
23. As an agent, I want a documented changelog entry format, so that I can add an entry for a merge without guessing.
24. As an agent, I want the drift check to fail on a malformed entry, a bad date, entries out of order, a dead source link, or a merge commit or commit range git doesn't know, so that History stays trustworthy.
25. As an agent, I want an entry's merge commit to be optional, so that I can write the entry on the branch before the merge exists.
26. As an agent, I want ticket and pull-request numbers accepted by format, so that the check runs offline.
27. As a reviewer, I want the route, the changelog rules, the map's drift rules, and the System view covered at the agreed seams, so that I can trust the change without a browser.
28. As an agent, I want the drift check to fail when a package has no part, a reference in the map no longer resolves, a fact chip no longer says what its reference points at, an import edge or sequence step no longer follows the code's imports, or the committed page was not rebuilt after the map, the facts, the changelog or a design proposal changed, so that the map stays true although CI cannot run the vision skill.

## Acceptance Criteria

1. In `evie serve`, clicking System in the sidebar opens the page, dark, on its subway map, full-height in one sandboxed frame under a System header; the breadcrumb reads System, and the browser console shows no errors. The page's own Map, Architecture and History tabs are the only tab bar, and switching between them never reloads the page.
2. The served page is byte-identical to the one in the commit the binary was built from.
3. The page shows no code and no links to files; History's commits, tickets and pull requests open on GitHub in the repository the map's `meta.repository` names.
4. `/explorer/` answers GET and HEAD only, serves the page at `/explorer/` and `/explorer/system.html`, refuses other methods, returns 404 for anything else, and sends a content-security policy that allows framing only by Evie, sandboxes the page as Evie's frame does, allows its inline scripts by hash (every one the browser may run) and inline styles, and no other host.
5. The Architecture tab shows Context, Containers, Components and Flows, with a part for every package; each part's panel lists the Map steps that cite it (Appears in).
6. The History tab lists every changelog entry newest first, grouped by month. Each card shows date, title, branch, merge commit or commit range, text, sources, and touched packages. Each part's panel lists the entries that touched it.
7. `go run ./scripts/explorer check` fails, naming the entry, on: a missing required field; a date that is not a real calendar date; entries out of newest-first order; a dead source reference; a merge commit or commit range that git does not know; a malformed ticket or pull-request number.
8. `CHANGELOG.md` covers master's whole first-parent history: one entry per merge, and feature-grouped entries with commit ranges for direct-to-master work.
9. `./scripts/verify-change.sh` passes.
10. `go run ./scripts/explorer check` fails, naming the step, part, edge, sequence, chip, code entry or setting, when `.vision/map.json` has a package without a part, a reference that no longer resolves, a fact chip whose value is not in what its reference points at, an import edge or `via: "import"` sequence step the facts do not import, a sequence step that cites no code, a participant or contained part that is not a part, an er diagram over a table the facts lack, a form of the skill's format Evie's map does not use, or a page built from an older map or from an older facts file, changelog or set of design proposals; when a design proposal is not a JSON object, its `id` is not its file name, or its `status` is not one the skill knows; and when the map is missing.

## Implementation Decisions

### Modules

- **Snapshot package**: a tiny Go package beside the page that embeds `system.html`, the page the vision skill builds, its only file: the page is self-contained. `go:embed` cannot reach `docs/` from `internal/web`, so the embedding lives with the file. The explorer tool's package extraction includes this one package outside `cmd/` and `internal/`, and it has a component part (Infrastructure, Surfaces).
- **Web server**: serves the page under `/explorer/` (at the directory and by its name), GET and HEAD only, with one content-security policy that allows framing only by Evie, sandboxes the page, and allows its inline scripts by hash and inline styles. The route sits outside the API guard, like Evie's own static files: it carries only the public repository's architecture. It serves no `build.js` (2026-11-05): the page shows no code links, so it needs no commit.
- **Web UI**: a System sidebar entry directly under Data; a `system` workbench view that behaves like Data (like Data, it is not remembered across reloads, and opens on the map every time), whose body is a System header styled like Data's over one full-height frame of `/explorer/?theme=dark`, sandboxed to allow scripts and popups (popups escape the sandbox so GitHub links open normally). The page opens on its subway map and its own Map, Architecture and History tabs route between its views; the view draws no tabs of its own (2026-11-05), so there is one tab bar. The breadcrumb label is System.
- **System map** (2026-11-04, extended 2026-11-05): `.vision/map.json`, drawn and kept current by the vision skill, which builds it into the committed `system.html`. Its flows draw the Map; its parts, groups, edges and sequences draw Architecture, converted once from the retired explorer's content files; `meta.changelog` names `CHANGELOG.md` for History, `meta.repository` the public repository for its links, and `meta.parts: "complete"` requires a part for every package. Because CI cannot run the skill, the explorer's `check` holds the map to the code itself: every package has a part, code entries resolve by file and symbol (Go, JavaScript and TypeScript declarations), references resolve by the reference forms (`part:` included), each fact chip's value is found in what its reference points at as the skill's check matches it, import edges and import sequence steps follow the facts' imports, every sequence step cites code, er diagrams name real tables, and the page carries the map's hash and the inputs hash over the facts, the changelog and the design proposals, both recomputed as the skill's map format says; each design proposal is one JSON object whose `id` is its file name and whose `status` the skill knows, the rest of a proposal being the skill's own check; it refuses the skill's other forms rather than passing them unread. Details: the explorer README's "What the check holds the map to".
- **Explorer tool, History**: `check` parses `CHANGELOG.md` and validates it as below, its sources in the map's reference forms. Since 2026-11-05 `extract` no longer copies it into the facts: the page reads it directly, and the inputs hash covers it.

### Changelog format (approved)

`CHANGELOG.md` at the repository root, newest first. Each entry is a level-two heading `YYYY-MM-DD — Title`, then metadata lines, then a blank line and one or more paragraphs saying what changed and why:

- `Branch:` the merged branch (required).
- `Merge:` the merge commit (optional, because an entry is written on its branch before the merge exists), or `Commits: <from>..<to>` for direct-to-master work.
- `Touches:` comma-separated package paths (required; written by hand; not link-checked, so history survives removed packages).
- `Sources:` comma-separated references (the reference forms of the explorer README; `part:` names a part of the map) and ticket or pull-request numbers such as `#179` (at least one).

`check` requires the fields, real dates, newest-first order, resolving source references, commits and ranges known to git (verified with git; CI already fetches full history), and well-formed `#n` numbers (format only; they cannot be verified offline). The facts carry no copy of the changelog, so editing it leaves them byte-stable; the page is rebuilt instead.

### Backfill

One entry per merge on master's first-parent history; the 190 direct-to-master commits are grouped into feature entries with commit ranges. Entries are drafted from the commits and the completed feature records under `cmd/*/docs/done/`, and cite them.

## Testing Decisions

- Good tests exercise external behaviour only: what the route serves, what the tool extracts and reports, what the UI renders.
- **Seam 1, `internal/web` tests with `httptest`**: `/explorer/` serves the page byte for byte, methods, 404s for the retired viewer's files and anything else, and headers. Misses: behaviour inside the frame.
- **Seam 2, explorer tool Go tests**: changelog parsing and each check failure, with git history created in `t.TempDir()`, and each system map rule (steps, parts, edges, sequences, diagrams, settings, both page hashes, design proposals) on a fixture map and page; the map hash and the inputs hash agree with what the skill's `vision.py` computes on fixtures. Misses: whether the prose is true.
- **Seam 3, UI vitest**: the sidebar shows System, and the System view renders its System header, styled like Data's, over one sandboxed frame of the map page, with no tabs of its own. Misses: the page's own behaviour and tabs, which need a browser and are checked at Finish.
- Prior art: `internal/web` route tests with `httptest`; `cmd/evie/workspace_folder_browser_test.go` runs `git` in temporary repositories; `App.test.tsx` renders shell components with `renderToStaticMarkup`.

## Dependencies

- The system explorer (`feat/system-explorer`), whose fact extractor and drift check this builds on.
- The vision skill (`~/.claude/skills/vision`), outside the repository, which builds the page; CI checks the map without it.
- No new dependencies.

## Risks

- **A route outside the API guard.** It serves only the embedded page of the public repository; it reads no data and takes no input beyond the path, which names the page or nothing.
- **Page code running inside Evie.** The frame is sandboxed without same-origin access, so the page cannot call Evie's API or read its storage.
- **History links rot.** A spec that moves from `active/` to `done/` breaks old entries' source links; the drift check names them and the fix is a link update.
- **The check needs git** for commit validation; it runs in a git checkout locally and in CI with full history.
- **The check's hashes must stay the skill's.** The map hash and the inputs hash are recomputed in Go; tests hold both to what the skill's `vision.py` computes on fixtures, and a change to either rule in the skill fails the check on every rebuilt page until the Go side follows.
- **Public exposure.** The repository is public, so pushing publishes the map's content, including its cited gotchas about local approval replay.

## Verification

- `./scripts/verify-change.sh` from the repository root.
- `go run ./scripts/explorer extract` leaves the facts byte-identical; `go run ./scripts/explorer check` exits 0.
- Manual: build the UI and `evie`, run `evie serve`, click System (the map opens on its subway under one tab bar, the page's; open a station), switch to Architecture (the page does not reload) and walk Components to a part's panel and its Appears in, step through a sequence on the Flows level, switch to History and open an entry, and open a part's panel and its History section.

## Out of Scope

- Changing `/implement` or `/wrap` to draft changelog entries (done separately).
- A changelog generated from git at build or run time.
- Serving source files from `evie serve`.
- A dark theme for Evie.
- Checking with GitHub whether a commit exists.

## Further Notes

- "History" avoids the glossary's **Change** (the Changes feature) and runtime "logs".
- This amends `system-explorer.spec.md`, and on 2026-11-05 retires its viewer; see the decision records.

## Tickets

- #195 System page: the explorer inside Evie (blocked by: none)
- #196 System page: History tab fed by CHANGELOG.md (blocked by: none)
- #197 System page: backfill CHANGELOG.md from master's history (blocked by: #196)
