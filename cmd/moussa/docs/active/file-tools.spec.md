# file-tools — spec

## Purpose

Give the agent eyes and hands on the filesystem: `read_file` and
`edit_file` in `internal/tools/file.go`. First customers: editing
`~/.finance/merchantLookup.json` conversationally, reading/writing
notes and configs. Long-term this is the seed of moussa doing real
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

## Open questions — resolve in the build session

1. May `edit_file` **create** a new file (old_string empty ⇒ write
   whole content), or is creation a separate `write_file` later?
2. **Path handling**: absolute-only, or allow `~` expansion (the
   model will try `~/...` — decide who expands it)? Note moussa's
   process cwd is wherever it was launched — relative paths are a
   trap.
3. Denylist contents and shape: hardcoded list vs. patterns; block
   reads only, or writes too?
4. Does `read_file` return line numbers (helps the model quote
   old_string precisely) or raw content?

## Build steps

1. `read_file`: schema + execute (denylist, size cap), registry entry.
2. `edit_file`: schema + execute (read, verify unique match, replace,
   write back), registry entry with NeedsApproval.
3. Live-fire: read a file, edit merchantLookup.json through the gate,
   verify with `finance rules` reseed.
