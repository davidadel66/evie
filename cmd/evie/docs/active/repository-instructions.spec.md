# Repository instructions

Status: implemented and verified, 2026-09-19. Requested and approved by David, 2026-09-18.

## Outcome and decisions

Folder-backed Workspaces automatically load root AGENTS.md, falling back to
CLAUDE.md only when AGENTS.md does not exist. A persisted Workspace setting,
initially enabled, controls loading independently of the attached-folder revision
and pinned capability configuration. The owner can change it between turns;
stale changes are rejected. It applies to existing chats on their next turn.

The loader reads only the designated root file, with a 32 KiB UTF-8 text limit.
It never traverses above the attached root, follows symlinks, executes imports,
or silently truncates instructions. Missing files are normal; invalid, oversized,
nonregular or unreadable files are visible errors and stop an enabled turn before
provider execution, so instructions are never silently skipped. Expected file and
configuration failures commit a safe turn-failed record, allowing later turns
and compaction to proceed after the owner fixes the cause.

Instructions are scoped repository guidance, subordinate to explicit current
owner requests and runtime policy, capability, approval and memory boundaries.
These optional repository guides are separate from the approved, required
procedural-memory files governed by memory.spec.md; that contract is unchanged.
They are composed separately from semantic memory and untrusted Task text, after
the stable system prefix and before conversation messages. Context estimation
includes their complete content, including after compaction.

Before each new root user turn, capture filename, exact text, SHA-256, attached
folder/revision, setting/revision and timestamp. Persist once for that session
and turn; all iterations and retries use that immutable snapshot. Later turns
reload disk. Disabled, missing and failed loads also have inspectable receipts.
These receipts are stored outside transcript and semantic-memory events.
Restricted delegated workers never inherit repository instructions implicitly.

The existing Workspace folder settings contain the toggle. A compact instruction
control in existing context UI opens the shared inspector; per-turn inspection
opens its durable snapshot. Current disk previews and historical snapshots are
clearly distinguished, and ordinary file/Terminal tabs remain open.

## Non-goals and verification

Nested instruction discovery, parent-directory rules, arbitrary instruction
paths, imports, new model capabilities and filesystem Project integration are
not part of this root-only Workspace slice. No new production dependencies.

Verify real file boundaries/fallback, SQLite reopen and stale settings, snapshot
immutability, two model iterations plus a later changed-file turn, disabled and
restricted workers, scoped HTTP reads, prompt accounting, and browser toggle/
inspector behavior. Run the full UI suite and ./scripts/verify-change.sh before
installing/restarting the backend and restoring the selected conversation.

## Verification record (2026-09-19)

- `go test ./internal/repoinstructions ./internal/eviedb ./internal/agent ./internal/web ./cmd/evie -run 'TestRootInstructions|TestRepositoryInstruction' -count=1`: passed. Covers root/fallback boundaries, immutable snapshots, SQLite reopen, disabled and delegated exclusion, HTTP session isolation, context accounting, frozen iterations, subsequent file reload and failure recovery through actual compaction.
- `go test ./internal/agent -run '^TestRepositoryInstructionConflict' -count=1`: passed; configuration conflicts finish durably before provider execution.
- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui`: 300 tests across 51 files passed.
- `./scripts/verify-change.sh`: passed full Go tests/vet, UI lint/build and staged/unstaged whitespace checks. Existing Fast Refresh warnings in memory/presentation.tsx and ui/Icon.tsx, and Vite chunk-size warnings remain. No required checks skipped.
- Real HTTP/browser fixture: current preview loads AGENTS.md; disk edits update only the preview; historical instructions retain exact original content. Toggle persists across reload, updates an already-open preview, and does not reconnect Terminal. Shared tabs and narrow inspector layout checked.
- Independent review reproduced a root-directory swap redirect and a failed-load compaction blocker. Both fixed and rerun: anchored directory resolution rejected outside content across 500,000 concurrent root/symlink swaps; invalid guide followed by successful turns compacted normally. The setting-change conflict uses the same durable failure path.

To demonstrate: open a Workspace, attach a local folder containing AGENTS.md (or
CLAUDE.md when AGENTS.md is absent), and leave Use repository instructions checked.
Click its instruction badge for the current preview; send a turn and click that
turn's Repository instructions action for the saved snapshot. Nested rules and
parent-directory discovery remain deferred as specified above.

Deployment: rebuilt and installed `/Users/davidboktor/go/bin/evie`, backed up the
prior binary and SQLite database, gracefully restarted `evie serve` on
`http://127.0.0.1:6687/`, and restored the previously selected Interview Prep
conversation. Verified the new schema, default enabled setting, preview endpoint,
and loaded web app after restart. Its Workspace currently has no attached folder.
