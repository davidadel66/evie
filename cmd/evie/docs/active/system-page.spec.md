# System page and History

## Problem Statement

The system explorer (`system-explorer.spec.md`) is a page the owner opens from
disk. It is easy to forget, and it is not where the owner spends time: Evie's
own web UI. The owner also has no single place to see how Evie has changed
over time. The history exists, but it is scattered across 213 commits on
master, 23 merges, a handful of pull requests, closed tickets, completed
feature records, and dated decision entries.

## Solution

A **System** entry in Evie's sidebar opens a view with three tabs (amended
2026-11-04): **Map**, the default, a map of Evie's flows that the vision skill
draws from `.vision/map.json`, opening on its subway front door; and
**History** and **Where is it?**, the explorer opened on its History and on its
components index. `evie serve` carries the explorer and the committed map page
as a snapshot built into the binary, so both always describe the build that is
running. In Evie, the explorer's code links open on GitHub at the commit the
binary was built from; from disk they still open the owner's local files.

The explorer gains a fifth tab, **History**: a timeline of every merge to
master, read from a written, link-checked `CHANGELOG.md`. Each package's
panel also lists the History entries that touched it. The changelog is
backfilled once from git and the completed feature records; since
2026-11-04 `/flow` drafts each work branch's entry (see the decision record).

## User Stories

1. As the owner, I want a System entry in Evie's sidebar, so that the architecture map is one click away where I already work.
2. As the owner, I want the System entry directly under Data, so that it sits with the other destination views.
3. As the owner, I want System to open full-height inside Evie's window on a map of Evie's flows, whose subway stations open the steps inside them, with History and Where is it? tabs beside it opening the explorer, so that I see what Evie does first and where the code lives one click away.
4. As the owner, I want the breadcrumb to say System while it is open, so that I know where I am and can go back to my chat.
5. As the owner, I want System to behave like Data as a workbench view (opened from the sidebar, left through the breadcrumb), so that it works the way Evie's other views do.
6. As the owner, I want the map and the explorer inside Evie to be dark, and the explorer to drop its own brand bar, so that they read comfortably and don't look like a page within a page.
7. As the owner, I want the map and the explorer served by `evie serve` from inside the binary, so that they need no files on disk and can't drift from the build.
8. As the owner, I want the served explorer to describe exactly the build I am running, so that I can trust it while debugging that build.
9. As the owner, I want code links in the served explorer to open on GitHub at the build's commit, so that I read the code that is actually running.
10. As the owner, I want code links to fall back to master when the binary has no commit stamp or was built with uncommitted changes, so that a link still leads somewhere useful.
11. As the owner, I want the explorer opened from disk to keep linking to my local files, so that the offline use I already have keeps working.
12. As the owner, I want the map and the explorer inside Evie isolated from Evie's API, so that page code can never act on my sessions or approvals.
13. As the owner, I want a History tab in the explorer, so that I can see how Evie has changed over time.
14. As the owner, I want History newest first and grouped by month, so that recent work is on top and older work is easy to scan.
15. As the owner, I want each History entry to show its date, title, branch, merge commit, what changed and why, its sources, and the packages it touched, so that one card tells the whole story.
16. As the owner, I want a merge commit, ticket, or pull request in an entry to open on GitHub, so that I can dig into the details.
17. As the owner, I want touched packages in an entry to jump to their nodes, so that I can see where a change landed in the architecture.
18. As the owner, I want each package's panel to list the History entries that touched it, so that I can see a part's own history.
19. As the owner, I want a touched package that no longer exists to show as plain text, so that old history stays readable after code is removed.
20. As the owner, I want History searchable from the explorer's search, so that I can find when something changed.
21. As the owner, I want the changelog backfilled for all of master's history, so that History is complete from day one.
22. As the owner, I want backfilled entries to cite their commits and the completed feature records, so that I can trust them and skim them quickly.
23. As an agent, I want a documented changelog entry format, so that I can add an entry for a merge without guessing.
24. As an agent, I want the drift check to fail on a malformed entry, a bad date, entries out of order, a dead source link, or a merge commit or commit range git doesn't know, so that History stays trustworthy.
25. As an agent, I want an entry's merge commit to be optional, so that I can write the entry on the branch before the merge exists.
26. As an agent, I want ticket and pull-request numbers accepted by format, so that the check runs offline.
27. As a reviewer, I want the new route, the changelog rules, the viewer's history and link logic, the map's drift rules, and the System view covered at the agreed seams, so that I can trust the change without a browser.
28. As an agent, I want the drift check to fail when a reference in the map no longer resolves, a fact chip no longer says what its reference points at, or the committed page was not rebuilt after the map changed, so that the map stays true although CI cannot run the vision skill.

## Acceptance Criteria

1. In `evie serve`, clicking System in the sidebar opens the Map tab: the map page, dark, at its subway, full-height in a sandboxed frame; the breadcrumb reads System, and the browser console shows no errors. History and Where is it? open the explorer at `#view=history` and `#view=components` in one shared frame, so switching between them moves its fragment without a reload; the map keeps its own frame.
2. The served explorer's files and map page are byte-identical to those in the commit the binary was built from.
3. In the served explorer, a code link opens GitHub at the build's commit; with no commit stamp, or a build with uncommitted changes, it opens master. Opened from disk, code links stay relative file links.
4. `/explorer/` answers GET and HEAD only, refuses other methods, returns 404 for files that are not in the snapshot, and sends a content-security policy that allows framing only by Evie itself. The map page's policy shares the explorer's sandbox, `base-uri`, `form-action` and `frame-ancestors`, and allows its inline scripts by hash (every one the browser may run) and inline styles, and no other host.
5. `/explorer/build.js` names the repository URL, the commit, and whether the build had uncommitted changes.
6. The History tab lists every changelog entry newest first, grouped by month. Each card shows date, title, branch, merge commit or commit range, text, sources, and touched packages. Each package's panel lists the entries that touched it.
7. `go run ./scripts/explorer check` fails, naming the entry, on: a missing required field; a date that is not a real calendar date; entries out of newest-first order; a dead source reference; a merge commit or commit range that git does not know; a malformed ticket or pull-request number.
8. `CHANGELOG.md` covers master's whole first-parent history: one entry per merge, and feature-grouped entries with commit ranges for direct-to-master work.
9. `./scripts/verify-change.sh` passes.
10. `go run ./scripts/explorer check` fails, naming the step, chip, or code entry, when `.vision/map.json` has a reference that no longer resolves, a fact chip whose value is not in what its reference points at, a form of the skill's format Evie's map does not use, or a page built from an older map; and when the page is committed without its map.

## Implementation Decisions

### Modules

- **Explorer snapshot package**: a tiny Go package beside the explorer files that embeds the page, the viewer, the stylesheet, the data, the examples, and the map page (`system.html`). `go:embed` cannot reach `docs/` from `internal/web`, so the embedding lives with the files. The explorer tool's package extraction grows to include this one package outside `cmd/` and `internal/`, and it gets a component node (Infrastructure, Surfaces).
- **Web server**: serves the snapshot under `/explorer/`, GET and HEAD only, with a content-security policy allowing framing only by Evie (the map page's allows its inline scripts by hash and inline styles), and a generated `build.js` carrying the repository URL (from the main module path), the commit, and the modified flag (from Go's build stamp). The route sits outside the API guard, like Evie's own static files: it carries only the public repository's architecture.
- **Explorer page**: when it is not opened from disk, it loads `build.js` and resolves code links to GitHub (`blob/<commit>/<path>`, or `blob/master/<path>` when there is no commit or the build was modified). From disk, links stay relative. An embedded flag in the address hides the brand bar and forces the dark theme.
- **Web UI**: a System sidebar entry directly under Data; a `system` workbench view that behaves like Data (like Data, it is not remembered across reloads, and opens on Map every time), whose body is three tabs over frames sandboxed to allow scripts and popups (popups escape the sandbox so GitHub links open normally): Map frames `/explorer/system.html?theme=dark#subway`, and History and Where is it? share one frame of `/explorer/?embedded=1`, keyed by page, whose fragment a tab switch moves in place; the breadcrumb label System.
- **System map** (2026-11-04): `.vision/map.json`, drawn and kept current by the vision skill, which builds it into the committed `system.html`. Because CI cannot run the skill, the explorer's `check` holds the map to the code itself: code entries by file and symbol (Go, JavaScript and TypeScript declarations), references by the explorer's reference forms plus the skill's JavaScript and TypeScript `#Name` and `doc:` folders, each fact chip's value found in what its reference points at as the skill's check matches it, and the page's map hash; it refuses the skill's other forms rather than passing them unread. Details: the explorer README's "System map" section.
- **Explorer tool, History**: `extract` parses `CHANGELOG.md` into a `history` list in the facts; `check` validates it as below.
- **Explorer page, History**: a fifth level tab, History, with a month-grouped timeline; a History section on each package's panel; history entries included in search.

### Changelog format (approved)

`CHANGELOG.md` at the repository root, newest first. Each entry is a level-two heading `YYYY-MM-DD — Title`, then metadata lines, then a blank line and one or more paragraphs saying what changed and why:

- `Branch:` the merged branch (required).
- `Merge:` the merge commit (optional, because an entry is written on its branch before the merge exists), or `Commits: <from>..<to>` for direct-to-master work.
- `Touches:` comma-separated package paths (required; written by hand; not link-checked, so history survives removed packages).
- `Sources:` comma-separated explorer references and ticket or pull-request numbers such as `#179` (at least one).

`check` requires the fields, real dates, newest-first order, resolving source references (the explorer's reference rules), commits and ranges known to git (verified with git; CI already fetches full history), and well-formed `#n` numbers (format only; they cannot be verified offline). History facts are a pure function of `CHANGELOG.md`, so facts stay byte-stable.

### Backfill

One entry per merge on master's first-parent history; the 190 direct-to-master commits are grouped into feature entries with commit ranges. Entries are drafted from the commits and the completed feature records under `cmd/*/docs/done/`, and cite them.

## Testing Decisions

- Good tests exercise external behaviour only: what the route serves, what the tool extracts and reports, what the viewer model returns, what the UI renders.
- **Seam 1, `internal/web` tests with `httptest`**: `/explorer/` files, `build.js`, methods, 404s, and headers. Misses: behaviour inside the frame.
- **Seam 2, explorer tool Go tests**: changelog parsing and each check failure, with git history created in `t.TempDir()`, and each system map rule on a fixture map and page. Misses: whether the prose is true.
- **Seam 3, Node tests on the viewer model**: timeline grouping, each package's history, link targets (GitHub commit, master fallback, local file), embedded mode. Misses: real rendering, covered by a browser check at Finish.
- **Seam 4, UI vitest**: the sidebar shows System, and the System view renders its tabs and each tab's sandboxed frame, with History and Where is it? on one page. Misses: the pages' own behaviour, and the in-place fragment move, which needs a browser.
- Prior art: `internal/web` route tests with `httptest`; `cmd/evie/workspace_folder_browser_test.go` runs `git` in temporary repositories; the explorer's Node tests; `App.test.tsx` renders shell components with `renderToStaticMarkup`.

## Dependencies

- The system explorer (`feat/system-explorer`), on which this branch is stacked.
- No new dependencies.

## Risks

- **A new route outside the API guard.** It serves only embedded static files of the public repository; it reads no data and takes no input beyond the file path, which resolves only inside the embedded snapshot.
- **Explorer code running inside Evie.** The frame is sandboxed without same-origin access, so the explorer cannot call Evie's API or read its storage.
- **Commit links that 404.** A build from a commit not yet on GitHub links to a commit GitHub doesn't have.
- **History links rot.** A spec that moves from `active/` to `done/` breaks old entries' source links; the drift check names them and the fix is a link update.
- **The check now needs git** for commit validation; it runs in a git checkout locally and in CI with full history.
- **Public exposure.** The repository is public, so pushing publishes the explorer's content, including its cited gotchas about local approval replay.

## Verification

- `./scripts/verify-change.sh` from the repository root.
- `go run ./scripts/explorer extract` leaves the facts byte-identical; `go run ./scripts/explorer check` exits 0.
- Manual: build the UI and `evie`, run `evie serve`, click System (the map opens on its subway; open a station), switch to Where is it? and walk a level, open a code link (GitHub at the build's commit), switch to History (the explorer does not reload), and open a package's panel and its History section.

## Out of Scope

- Changing `/implement` or `/wrap` to draft changelog entries (done separately).
- A changelog generated from git at build or run time.
- Serving source files from `evie serve`.
- A dark theme for Evie.
- Checking with GitHub whether a commit exists.

## Further Notes

- "History" avoids the glossary's **Change** (the Changes feature) and runtime "logs".
- This amends `system-explorer.spec.md`; see the decision record.

## Tickets

- #195 System page: the explorer inside Evie (blocked by: none)
- #196 System page: History tab fed by CHANGELOG.md (blocked by: none)
- #197 System page: backfill CHANGELOG.md from master's history (blocked by: #196)
