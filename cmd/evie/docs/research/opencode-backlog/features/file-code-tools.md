# F13 - File discovery, reading, and mutation tools

Status: candidate, unapproved. Priority: early coding slice.

## Purpose

Give coding profiles a compact, mechanically scoped filesystem interface that
does not rely on unrestricted Bash for every search or mutation.

## How OpenCode does it

OpenCode exposes dedicated glob/grep/read/write/edit/apply-patch tools. `read`
also handles directory listing; there is no separately registered `list` tool at
the pinned revision despite a stale `list` permission entry. Reads support
bounded file slices. Mutations request permission, produce change metadata, and
can trigger post-edit diagnostics. General tool output is truncated centrally
with the full result retained locally.

OpenCode may run configured formatters after approval and after writing, so its
final bytes can differ from the diff the user approved. EVIE must not copy that
ordering because its existing prepared edit binds approval to exact final bytes.
See [F28 - Formatting](formatting.md).

OpenCode's current and newer path implementations differ; EVIE should adopt the
stronger canonical-ancestor/symlink boundary rather than porting either module
wholesale.

Sources:

- [`tool/read.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/read.ts)
- [`tool/edit.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/edit.ts)
- [`tool/external-directory.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/external-directory.ts)
- [`format/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/format/index.ts)

## Already shipped

EVIE's `read_file` and `edit_file` already have good properties: exact unique
replacement, complete approval preview, stale-byte/permission recheck, atomic
same-directory rename, regular-file checks, and actionable errors. Preserve
them.

They are enough for attended changes to existing small text files. Bash already
covers creation, directory listing, glob/grep, rename/delete, and arbitrary
multi-file scripts for the unrestricted primary assistant. F13 is therefore an
extension of the existing tools, not a replacement feature.

## Minimum missing slice

- **Project-aware resolution:** bind read/edit/Bash to F01's selected project and
  session cwd instead of two process-global locations. This is an F01 runtime
  change, not a new file operation.
- **`write_file`:** structured, approval-previewed creation for new text files.
  Bash can create files today, but it bypasses the exact prepared-change UI.
- **Read-only discovery:** directory reads plus typed glob/grep so `plan` and
  `explore` can search without receiving mutation-capable Bash.
- **Paginated/ranged reads:** inspect files over 100 KiB and keep ordinary reads
  token-bounded.

## Later, only when a concrete task needs them

- structured rename/move and delete with previews;
- multi-file/apply-patch transactions;
- binary/image attachment reads;
- post-edit LSP diagnostics;
- formatter integration through F28's separately previewed path.

## Proposed build order

1. Rebind existing tools to F01's scoped session path resolver.
2. `write_file` using the existing prepared preview protocol and `IsNew` UI.
3. Directory reads and paginated/ranged `read_file`.
4. Typed `glob` and `grep` for read-only profiles and token-efficient discovery.
5. Revisit rename/delete, multi-file patch, and diagnostics only after usage
   proves the need.

All tools consume F01's immutable project context and F05 policy. New-file paths
authorize the canonical nearest existing ancestor and revalidate before rename.
Typed tools must keep secret fences independent of user approval.

The prior rejection of glob/grep should be revisited only for two concrete new
reasons: a mechanically read-only `explore` profile cannot receive Bash, and
structured search can save parent context. This is not a general repudiation of
"Bash handles the long tail."

## Acceptance evidence

- New-file preview shows an empty before state and exact final bytes.
- Concurrent path/symlink changes invalidate a prepared mutation.
- Large reads paginate without silent truncation or broken UTF-8 boundaries.
- Glob/grep stay within project scope unless external access is authorized.
- Exact-replacement behavior and permission preservation remain unchanged.
- No formatter or diagnostic hook changes bytes after the approved final
  preview.

## Open decisions

1. Does `write_file` create missing parent directories? Recommendation: no in
   the first slice.
2. Is apply-patch necessary, or is repeated exact edit plus write enough?
3. Should read-before-edit become a mechanical requirement per session?
