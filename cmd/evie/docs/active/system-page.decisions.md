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
- **2026-10-31 — Commit links for unpushed builds (pending).** Asked again
  because the reply named two answers for three questions: accept that a
  build from a commit not yet on GitHub links to a commit GitHub doesn't
  have, with master as the fallback only when there is no commit stamp or
  the build had uncommitted changes.
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
