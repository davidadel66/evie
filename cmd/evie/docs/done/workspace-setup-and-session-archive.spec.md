# Workspace setup and session archive

Status: implemented and verified on 2026-09-21.

Authorized by David on 2026-09-21. This amends workspace creation and the sidebar
footer in `ui-chat.spec.md`, and adds owner-session archival to
`session-history-and-memory-scope.spec.md`.

## Outcome

The sidebar workspace plus opens a modal over the current view. The owner names
the workspace, chooses an existing Agent Preset, and optionally attaches an
existing local folder or creates a new one. Creating the workspace leaves the
current view and conversation intact; starting its first chat is a separate
explicit action. The directory's create action opens the same modal.

Standard is the default broad preset. Research is the existing restricted web
search/fetch preset. The selected preset is saved as the default and allowed
preset for this workspace revision. Sessions enforce that allowed set and pin
their composition as before; existing workspaces migrate to Standard, and
existing session receipts are preserved. Presets expose capabilities and do not
grant external accounts or bypass action approval.

Folder selection is optional. The creation modal has one **Choose folder…**
button that opens the native macOS directory picker on the computer running
Evie. The picker offers **New Folder**. The selected absolute path is displayed
and may be cleared; cancelling the picker preserves the prior selection. No
manual path or separate attach/create mode appears in the creation form.

The picker is an owner-facing local management operation, separate from model
tools. It accepts no script or shell input, allows one outstanding picker,
requires a loopback peer and the management guard, and releases its helper on
request cancellation, timeout, or server shutdown. Unsupported platforms return
an actionable error. Folder selection never creates a workspace. The native
picker creates folders immediately when the owner uses New Folder; those folders
remain even if the picker or workspace form is later cancelled. A selected folder
is attached through the existing local-folder contract at workspace creation.
Native paths retain their exact whitespace.

The existing direct registration API's optional folder-creation contract remains:
it creates one directory under an existing parent and refuses an existing
entry. Invalid folder or preset input must not leave a workspace behind. A
successfully created workspace remains successful if the subsequent UI refresh
fails.

Each sidebar conversation has an archive action beside its title, available on
hover, keyboard focus, and touch. Archive hides it from active conversation lists
without deleting history, scope, or composition. The existing durable closed
session state represents archive; Settings lists archived owner conversations
and restores them without implicitly opening them. Delegated sessions retain
their separate parent-managed lifecycle.

Archiving the selected conversation clears its selected scope and composer.
Archiving another leaves the current conversation intact. Busy turns, durable
turn leases, and admitted/running child work block archival. Closed sessions
cannot resume or accept new turns until restored.

The sidebar footer becomes Settings. Settings contains chat font and text-size
preferences, a preview, and archived sessions. Font preferences persist in the
browser; archival persists in SQLite. Repository-instruction access remains
available. Modals contain keyboard focus, support Escape and dismissal, restore
focus to their opener, and retain form inputs on failed creation.

## Boundaries

No per-plugin custom preset editor, preset revision editor, account permission
redesign, deletion, archive retention cleanup, bulk archive, or migration of
existing memory is included. Folder creation does not create missing parent
directories. Plugin startup availability and normal approval rules still apply.

## Verification

- SQLite tests prove archive/restore and workspace preset/folder persistence,
  input rejection without partial workspace creation, and unchanged receipts.
- Controller/HTTP tests prove native picker cancellation, concurrency and guards,
  preset enforcement, folder creation, stale
  revision rejection, busy archival fences, and selected-session clearing.
- UI tests cover creation payloads/validation, Settings, archive transitions,
  and separate workspace creation/session selection.
- A disposable real-HTTP browser fixture demonstrates modal creation, native folder selection/cancellation and New Folder, active and other-session archival, restore, font persistence,
  keyboard focus, and narrow-screen layout without user data or model calls.
- Run UI Vitest from `internal/web/ui`, then `./scripts/verify-change.sh` from
  the repository root.

## Completion evidence

- `npx vitest run` from `internal/web/ui`: 348 tests in 58 files passed.
- `GOFLAGS=-p=2 ./scripts/verify-change.sh` from the repository root: passed
  UI lint/build, full Go tests and vet, and staged/unstaged whitespace checks.
  Reduced Go parallelism limits temporary disk use; no checks were disabled.
- Focused archive race checks passed with `go test -race ./internal/eviedb
  ./internal/web ./cmd/evie -run
  'TestSessionArchive|TestContextSessionArchive|TestContextSessionHTTPEncodes|TestListActiveSessions|TestActiveSessionBoundary|TestWebContextController'
  -count=1`.
- Native picker and registration checks passed with `go test ./internal/web
  ./internal/eviedb -run
  'TestWorkspaceFolderPicker|TestNativeFolderPicker|TestWorkspaceCreation'
  -count=1`, covering exact paths, cancellation, guards, concurrency, timeout,
  and process cleanup.
- The opt-in `TestWorkspaceSetupBrowserFixture` exercised the real HTTP and
  SQLite boundaries without personal data or model calls. Browser checks
  verified modal focus/dismissal, no implicit session or tab creation, preset
  selection, folder attachment, archive/restore, persistent font preferences,
  narrow layouts, failed-input preservation, and reopening saved state.
- David confirmed the native macOS picker works. Browser automation verified
  the single button, pending state, and focus return after cancellation; its
  native accessibility connection could not independently inspect New Folder
  creation or cancellation after an earlier selection.

Existing warnings remain: five Fast Refresh warnings in
`src/memory/presentation.tsx` and `src/ui/Icon.tsx`, plus Vite's large-chunk
warning. Custom per-plugin presets and editing existing workspace presets are
outside this change. The native picker is available only on the host Mac.

To demonstrate: click the workspace plus, enter a name, choose an Agent Preset,
optionally use Choose folder (including New Folder), and create the workspace.
Archive a conversation from its sidebar row, then open Settings to restore it
or change the chat font and text size.
