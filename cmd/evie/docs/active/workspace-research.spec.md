# Workspace research with bounded document reading

Origin: the owner's September 29 request for a 1 MiB worker request budget,
explicit Workspace research delegation, and section/excerpt reading that returns
concise sourced findings to Evie.

## Accepted behavior

- New research executions default to 1,048,576 serialized request bytes. Existing
  operator overrides remain valid. A worker uses the invoking chat's resolved
  model profile, with its own working budget bounded by the model's route-safe
  window, output reserve, and estimation margin. Parent budgets do not change.
- Standard Workspaces expose an explicit, initially off research-delegation
  setting at creation and in Workspace settings. Enabling adds the reviewed
  Research preset for new chats. Existing session snapshots are immutable.
  Disabling revokes existing research immediately; re-enabling never restores
  a revoked older session's permission. Changes use expected-revision checks.
- Admission, execution, and child final acceptance enforce active Workspace,
  pinned/current Research allowance, persistent revocations, and the parent's
  existing Web/delegation capabilities. Retained outcomes remain inspectable
  after execution permission is revoked, subject to current Workspace access.
- New Web fetch contracts return at most 16 KiB of text by default, up to 32 KiB
  when requested. Optional literal query and UTF-8 byte offset select relevant
  excerpts; responses identify source, full extracted-content hash, extent, and
  next offset. Nonzero-offset continuation requires that hash and refuses changed
  content. Explicit excerpt requests refetch under existing HTTP limits.
  JSON escaping may further reduce text size to keep the serialized tool result
  within 64 KiB; continuation always advances at a UTF-8 boundary.
- Existing Web receipts retain their exact schema and execution contract through
  explicit compatibility support. Workers retain separate history, restricted
  Web tools, and concise findings/source/limitation results under existing caps.
  Children inherit the parent's pinned Web contract; compatibility substitutions
  are recorded before child execution. Existing chats need a new chat to acquire
  the new excerpt schema.

## Decisions and boundaries

This request supersedes the unconditional Workspace refusal in
`subagents.decisions.md` and the workspace-setup exclusion of preset editing only
for this reviewed Research allowance. It extends the shipped immutable SQLite
preset revisions with owner-controlled updates and persistent revocations;
general procedural Git authoring in `workspaces.spec.md` remains separate work.
No existing Workspace is automatically opted in. Research-only parent presets
cannot delegate; the Standard preset remains the default for delegation-enabled
Workspaces. No new dependencies, PDF extraction, unrestricted workers, background
execution, or larger parent-result cap are introduced.

## Verification

Use the existing public profile, tool execution, composed supervisor/SQLite,
Workspace management API, and UI seams. Verify larger and smaller model windows,
selected-model propagation, excerpt/UTF-8/hash boundaries, frozen receipt resume,
Workspace opt-in/stale revisions/reopen, revocation and re-enable, separate child
history and bounded sourced results. Exercise creation/settings in a synthetic
browser with disposable storage; run focused race tests, the UI suite, and
`./scripts/verify-change.sh`. Preserve unrelated work and report live-provider
and installed-server boundaries.
