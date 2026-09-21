# Workspace local folder and shared inspector

Status: folder attachment, Files, Changes, and shared inspection implemented and
verified, including interactive Terminal. Dependencies approved on 2026-09-18.
Requested by David on 2026-09-18.

## Outcome

A Workspace may attach one existing local folder, remembered across backend
restarts. Existing conversations retain their Workspace memory scope. The shared
right pane supports live files, Git changes, and the existing recorded file,
tool, and memory inspections in closable tabs, alongside an interactive terminal.

## Accepted decisions

- A folder is an optional working location, not a filesystem Project or an
  additional memory scope. Attaching it does not migrate conversations or alter
  pinned capability receipts. This extends workspaces.spec.md's separation of
  Workspaces and filesystem Projects.
- The owner can attach, replace, or detach the folder from the Workspace home.
  Folder revisions are operational metadata separate from the pinned Workspace
  configuration revision; existing chats use the current folder on their next
  turn. Edits reject stale folder revisions and changes during active turns.
- Relative agent file operations and shell commands start in that folder;
  remembered shell directories belong to one live session. This supersedes the
  process-wide working directory in the completed bash decision record. The
  folder is a working directory, not a shell sandbox.
- Live file browsing is explicitly separate from recorded tool evidence. It
  supports expandable directories, filename filtering, source and Markdown
  preview, multiple file tabs, and a tree toggle that preserves the open file.
  This extends chat-activity.spec.md's earlier recorded-only inspector scope.
- Git review is read-only and supports working-tree, staged, and branch-base
  comparisons, including a changed-file list and bounded multi-hunk diffs.
- Memory and tool details continue to use their existing typed, scoped views;
  they open in the same pane without replacing open files. Session-specific
  inspection tabs clear when the active chat changes. Folder changes invalidate
  file/terminal tabs rather than silently retargeting them.
- Keep Evie's existing dark palette, typography, source colors, and compact
  controls. Use a resizable pane, closeable tabs, and a narrower independent
  file-tree rail. Preserve mobile overlay, keyboard focus, and Escape behavior.
- Approved terminal dependencies: `github.com/creack/pty`, `@xterm/xterm`, and
  `@xterm/addon-fit`. The owner explicitly approved their addition.
- Terminal input is owner input, not a model tool or conversation evidence.
  The shell starts in the attached folder with the owner's normal login shell.
  It stays mounted while switching inspector tabs or hiding the pane. Closing
  the terminal tab, switching chat/folder, navigating away, disconnecting, or
  shutting down the backend terminates its shell and ordinary foreground and
  background jobs. Processes explicitly detached into another session are
  outside this lifetime. Terminal sessions do not survive backend restarts.
- The local JSON management guard protects terminal creation, input, resize,
  and close. POST output streaming owns the process lifetime; there is no
  separate reconnect or replay. Stale Workspace folder revisions are rejected.
  The server allows at most eight terminals, bounds output buffering, and
  deadlines blocked writes. Input stays ordered; errors stop delivery without
  replay. Shell exit and connection failure have distinct UI states.
- Terminal output is rendered as text/ANSI in xterm with 2,000 scrollback rows.
  Escape, Tab, arrows, and Ctrl+C reach the shell. Shift+Escape moves focus
  to the Terminal tab; Escape there retains normal inspector dismissal.

## Verification and limits

Use existing public seams: real SQLite Workspace store reopen/scope tests,
tool execution through Toolset, HTTP management routes with real temporary
folders and Git repositories, existing UI rendering tests, and browser
interaction checks. Cover missing folders, stale revisions, path traversal,
outside-root symlinks, bounded/binary files, independent working directories,
preserved memory inspection, tree toggling and session changes.

No new memory compilation, automatic folder inference, file editor, commit/push
operations, or simultaneous second chat runtime. The latter needs separate
session routing: today's chat/history/evidence routes select one active chat.
Run the UI suite and ./scripts/verify-change.sh before handoff.

## Verification on 2026-09-18

- `./scripts/verify-change.sh` passed: UI lint/build, all Go tests, `go vet
  ./...`, and staged/unstaged whitespace checks. Only existing Fast Refresh
  warnings in `memory/presentation.tsx` and `ui/Icon.tsx`, plus Vite's large
  chunk warning, remain.
- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui`
  passed: 290 tests in 49 files.
- Focused runtime, HTTP, and real Git/SQLite tests cover existing-chat next-turn
  folder replacement, restart persistence, separate shell directories, missing
  roots, root symlink replacement, stale/busy updates, pathspec/directory
  exclusion, subfolder repos, staged deletion, untracked/unborn-repo contents,
  malicious Git clean/process filters, and nonblocking special-file rejection.
- Opt-in `TestWorkspaceFolderBrowserFixture` served scripted conversations with
  real memory receipts and a disposable Git folder. Actual browser checks
  verified attaching, filtering, Markdown/source switching, hiding/showing the
  tree, closable file/memory/Changes tabs, working-tree diffs, keyboard resizing,
  reload persistence, and a 390px overlay with source/debug disclosure access,
  focus wrapping, and Escape returning to the fetched-memory control.
- Rebuilt and installed the local binary, restarted the backend on port 6687,
  and restored the previously selected conversation. Real Workspace folders
  were not inferred or attached automatically.

To demonstrate: open a Workspace, enter an existing absolute local folder path,
choose **Attach folder**, then **Open files**. Open a Markdown file and toggle
its folder icon; use **Changes** for Git review. From chat, click a supplied
memory receipt and return to the file through its pane tab.

Files are bounded read-only previews (512 KiB UTF-8 text); Git output is bounded
to 2 MiB and configured executable filters are disabled during inspection.
Changes reflect disk/Git at refresh time, not a filesystem watcher. Tree
visibility and pane width persist in browser preferences; open tabs themselves
last only for the current page/context. Manual file editing, independent side
chat execution are not shipped in this slice.


## Terminal verification on 2026-09-18

- `./scripts/verify-change.sh` passed after the final implementation: UI lint
  and build, full Go tests, Go vet, staged and unstaged whitespace checks.
- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui`
  passed: 295 tests in 50 files. Terminal protocol tests cover split frames,
  render backpressure, explicit exit versus disconnect, cancellation, ordered
  input and resizing, Unicode chunk boundaries, and failed-input suppression.
- `go test ./internal/localterminal ./internal/web -race -run
  'TestTerminal|TestNaturalShellExit' -count=3` passed. Real shells and HTTP
  streams cover cwd, dimensions, exit status, stale requests, Origin rejection,
  disconnect, explicit close, folder replacement, and server shutdown. PTY
  tests exercise Ctrl+C and HUP-ignoring foreground/background jobs, including
  jobs whose controlling terminal disappeared after natural shell exit.
  The interrupt readiness marker comes from the foreground child itself.
- Actual browser tests used disposable `TestWorkspaceFolderBrowserFixture`
  servers and confirmed folder cwd, retained variables across memory tabs and
  pane hiding, shell exit status and restart, Escape reaching terminal input,
  Shift+Escape returning to the tab, narrow and focused layouts, and large-pane
  activation with dimensions capped at 200 rows. Browser automation did not
  reliably deliver Ctrl+C; interrupt delivery was verified by the PTY tests.
- Standards and Spec reviews both identified unbounded fitting on activation;
  both activation and ResizeObserver now use the same bounded fit function.
  Browser testing also exposed conflicting focused-pane flex classes, fixed
  and checked at a narrow viewport. No open review findings remain.
- Existing Fast Refresh and Vite chunk-size warnings remain. Dependency install
  reported three existing development/transitive audit entries (Vitest,
  @vitest/mocker, nanoid); none concerns the added xterm packages. No unrelated
  dependency upgrades were made.

Terminal review entry points: `internal/localterminal/session.go` (PTY lifetime),
`internal/web/terminal.go` (guarded stream/control routes),
`internal/web/ui/src/api/terminal.ts` (transport), and
`internal/web/ui/src/folder/Terminal.tsx` (mounted terminal view).

To demonstrate: attach a Workspace folder, open its inspector, and select the
terminal icon. Run a command, switch to a fetched memory or file tab, and return
to Terminal. Hiding the pane preserves that shell; closing its Terminal tab or
changing chat/folder ends it. A backend restart preserves the folder attachment,
while the next opened Terminal starts a fresh shell.
