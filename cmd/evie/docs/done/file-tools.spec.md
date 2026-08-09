# file-tools — spec

## Purpose

Give the agent eyes and hands on the filesystem: `read_file` and
`edit_file` in `internal/tools/file.go`. First customers: editing
`~/.finance/merchantLookup.json` conversationally, reading/writing
notes and configs. Long-term this is the seed of evie doing real
work on code.

## Decisions (2026-07-29)

- **Two tools, not one**: `read_file` (ungated — reading is safe *for
  the filesystem*) and `edit_file` (gated behind the approval
  prompt, like edit_db). Editing without reading first is flying
  blind; the model needs both.
- **Edit semantics: exact string replacement** — `path` +
  `old_string` + `new_string`, old_string must appear exactly once in
  the file (0 matches = error, >1 = error demanding more context).
  This is Claude Code's proven Edit design: models are reliable at
  quoting text, unreliable at line numbers, and whole-file rewrites
  burn tokens and risk silent truncation.
- **The two tools have opposite threat models — teach this in the
  code comments:**
  - `read_file` is the *leaky* one: file contents flow into the
    conversation and thus to the remote model provider. It needs a
    **secrets denylist** — at minimum `.env` files, `~/.finance/`
    (db holds Plaid tokens), `~/.zshrc` (exports keys). Same logic
    as the items-table fence.
  - `edit_file` is the *destructive* one: the approval gate is the
    defense. The approval prompt already shows the arguments, which
    for string-replacement is a readable before/after.
- **Size cap on reads**: max bytes (e.g. 100KB) with a clear error —
  a huge file must not flood the context. No pagination in v1.
- **Approval preview**: rely on the existing gate printing args;
  nicer diff rendering is a future (TUI/web) concern.

## Open questions — RESOLVED 2026-07-30

Reasoning for each lives in `file-tools.build.md` Part 0.

1. **Creation is a separate `write_file`, later.** `edit_file` requires
   an existing file; empty `old_string` is an error, not a whole-file
   write.
2. **No root jail.** Absolute paths, `~` expanded by us (Go doesn't),
   relative treated as cwd-relative — all through one `resolvePath`
   chokepoint. A cwd jail would block the first customer
   (`~/.finance/...`); path scoping belongs to `glob`/`grep`.
3. **Hardcoded patterns, blocking reads *and* writes**, checked inside
   `resolvePath` so no tool can skip the fence.
4. **Numbered lines (`%6d\t`), always the whole file** — no snippets, no
   pagination; the size cap errors instead of truncating. Numbers are
   display-only, so `edit_file` strips them from `old_string`/
   `new_string` (all-or-nothing per line) *and* says so in its
   description.

## Build steps

1. `read_file`: schema + execute (denylist, size cap), registry entry.
2. `edit_file`: schema + execute (read, verify unique match, replace,
   write back), registry entry with NeedsApproval.
3. Live-fire: read a file, edit merchantLookup.json through the gate,
   verify with `finance rules` reseed.
