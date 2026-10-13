# workspace-tabs — spec

Decided 2026-10-12 with the owner; binding record in
[workspace-tabs.decisions.md](workspace-tabs.decisions.md).

## Problem Statement

Every Workspace has the same tabs: Runs, Workflows, Settings (and Changes
with a Repository). What a Workspace is *about* has no home on its page.
The Finance Workspace's reports live under Data → Spending, away from the
Workspace that owns them, and they had to be built into Evie itself. A model
training Workspace would want evaluation metrics and analysis; a research
Workspace a reading map. Each of those would need Evie's own code changed
and released today, so Evie cannot build them for the owner on request.

## Solution

A Workspace can have its own tabs, as many as it needs, shown after the
built-in ones. Each tab is a small web page kept in the Workspace's folder
under `.evie/tabs/<tab-id>/`, so it is versioned with the Workspace's own
work and is specific to it. Evie writes and changes these pages like any
other file in the folder, so the owner can ask "add a Reports tab with
monthly spend by category" or "chart eval loss per checkpoint" and Evie
builds it.

Because Evie writes code the owner will not read line by line, a tab runs
sealed off from the rest of Evie:

- it renders in a sandboxed frame with its own opaque origin, so it cannot
  call Evie's routes, read Evie's storage, open windows or navigate the app;
- it cannot reach the network;
- it gets data only by asking the host page, and the host answers only the
  read-only data the tab's manifest declares and the owner approved.

## User Stories

1. As the owner, I want a Workspace to show tabs specific to it next to
   Runs, Workflows and Settings, so that what the Workspace is about lives
   on its page.
2. As the owner, I want more than one such tab per Workspace, so that, for
   example, Reports and Budget can sit side by side.
3. As the owner, I want to ask Evie in a Workspace chat to create or change
   a tab, so that I get the view I need without a release of Evie.
4. As the owner, I want each tab's source in the Workspace's folder, so that
   it is versioned, reviewable and revertible with the Workspace's work.
5. As the owner, I want a tab unable to act on Evie or reach the internet,
   so that code Evie wrote cannot approve memory, archive anything, start
   runs or leak my data.
6. As the owner, I want to approve what data each tab may read, so that a
   tab never sees more than I agreed to.
7. As the owner, I want a broken tab to say what is wrong instead of a blank
   page, and Evie to see the same problem, so that Evie can fix it.
8. As the owner, I want tabs that do not fit on a narrow screen to move into
   a More menu, so that the header never scrolls or clips.

## Implementation Decisions

### Tab files

- A tab is a folder `.evie/tabs/<tab-id>/` in the Workspace folder.
  `<tab-id>` is the folder name: lowercase letters, digits and hyphens, 1–32
  characters. It holds `tab.json` and `index.html`; any other files (scripts,
  styles, images) are served from the same folder.
- `tab.json` v1: `{"title": "Reports", "order": 10}`. `title` is 1–24
  characters; `order` is an integer that sorts tabs (ties by id). Story 2
  adds `data`. Unknown fields are a problem, not ignored, so a newer
  manifest on an older Evie says so.
- A Workspace without a folder has no custom tabs (decisions, 2026-10-12).
- Limits: at most 12 tabs per Workspace; a tab folder at most 5 MB and 200
  files and folders; `.evie/tabs` at most 256 entries, of which the first 32
  folders by name are examined. Anything over is listed as a problem, not
  served.

### Discovery and serving

- A management route lists a Workspace's tabs: id, title, order, and a
  problem message for an invalid one. Invalid tabs show in Settings → Tabs
  with their problem, never in the header.
- A GET route serves files from one tab folder only: regular files reached
  without a link, with no `..`, absolute path or dotfile. (A link inside the
  tab could serve a dotfile, and a pipe would hang the request.) Responses
  carry a Content-Security-Policy that sandboxes the document (scripts
  allowed, same-origin not allowed), allows scripts, styles, images, fonts
  and media only from that tab's folder URL or inline, and allows no network
  connections, frames, workers, forms, popups or top navigation. Error
  responses run nothing. The trailing-slash redirect is built only from
  checked IDs, so it cannot point off Evie.
- Evie's app shell sends `frame-src 'self'`, so a tab cannot navigate its own
  frame to another site.
- The frame element is `sandbox="allow-scripts"`: no `allow-same-origin`,
  `allow-top-navigation`, `allow-popups` or `allow-forms`.
- The existing management-route guard already refuses the frame's requests:
  an opaque-origin page sends `Origin: null`, which is not loopback
  (`internal/web/serve.go`, `guard`). A test makes that explicit.

### Header

- Order: Runs, Changes (with a Repository), Workflows, the custom tabs by
  `order`, then Settings last.
- The strip does not scroll (2026-10-12 fix). Built-in tabs always show.
  Custom tabs that do not fit move, from the right, into a More menu after
  Settings; the selected tab always stays visible.
- A custom tab reloads when it is selected again, and has a Reload control.

### Known residual risk

- WebRTC (`RTCPeerConnection` with a STUN or TURN server) is outside what a
  page policy can block in Chrome, so a tab's code could signal out over DNS
  or UDP. The owner accepted this (decisions, 2026-10-12) with its
  mitigations, the owner's approval of each tab's data list and reviewable
  tab code, plus a partial block: every HTML page a tab serves starts with
  an inline script that removes the WebRTC constructors and blocks reaching
  a fresh window through a frame before the tab's code runs. It is defence
  in depth, not a seal.

### Data channel (story 2)

- `tab.json` declares what the tab may read, for example
  `"data": [{"db": "finance"}, {"files": "eval/**/*.json"}, {"runs": true}]`:
  a registered database read through the existing read-only `query_db` path
  with its row and size limits; files in the Workspace folder matching a
  glob, never dotfiles or secret-looking names; this Workspace's workflow
  run outputs.
- The owner approves a tab's data list in Settings → Tabs. The approval is
  pinned to the exact list; a changed list runs the tab with no data until
  approved again.
- The page talks to the host through a small helper Evie serves to every
  tab at `_evie/tab.js` (a tab folder's own `_evie/` is never served):
  `evie.query(db, sql)`, `evie.file(path)` and `evie.runs()`. Evie's page puts
  a one-time key in the tab's address; the helper greets with it, sending
  the far end of a private `MessageChannel` it made (and greeting again
  until Evie answers), and Evie answers only over the first such channel
  from that frame, so a page the frame is navigated to afterwards cannot use
  the tab's access. At most 4 requests are in flight per tab and 4 tab reads
  across Evie; a tab's requests are cancelled when it closes or reloads. The
  server checks every request against the approved list and answers or
  refuses with a reason. There are no writes.
- Databases are read through `query_db`'s own fences (`tools.ReadDB`):
  finance and Evie's listed tables, typed rows (the helper gives a tab
  objects keyed by column name), at most 5,000 rows, 2 MB in
  all and 2 MB per value, ending within 4 seconds (before a writer's busy
  wait on finance.db gives up). Files: at most 1 MB each, regular files with
  one link reached without a symlink, never a dotfile, a path with a
  secret-looking folder or file name, an env or key file, or a SQLite
  database by name or header. Globs match in time proportional to pattern
  × path, with at most 16 pattern and 32 path segments. Runs: this
  Workspace's latest 100, scrubbed of sensitive inputs as a chat sees them,
  without inputs, at most 1 MB of outputs.
- An approval pins the data list's key in the Workspace's folder, so a
  changed folder asks again; a tab folder that is gone takes its approval
  with it. Evie's page cannot be framed by another site, so no page can
  trick the owner into allowing a tab's data.

### Evie's loop (story 3)

- Evie knows the tab contract (folder layout, manifest, helper API, limits)
  and creates or edits tabs with its existing file tools, or as a Change in a
  Workspace with a Repository.
- The `workspace-tabs` plugin gives Workspace chats one read tool,
  `workspace_tabs`: the contract (in the result, so it can change without a
  new tool version), each tab's status, data list and approval, and the
  recorded problems, with tab and problem text framed as data. `clear` with a
  tab id forgets that tab's problems once fixed. The standard preset adds it
  as an optional Capability; version d52283fa… is frozen so earlier chats
  resume unchanged. Sessions with the tool get one prompt line: build a
  Workspace's own views as its tabs, and check `workspace_tabs` first.
- Problems inside a tab (script errors, refused data requests, invalid
  manifest) are recorded per tab and readable by Evie in that Workspace, so
  it can fix what it wrote.
- The helper reports the page's script errors, failed loads and anything its
  policy blocks (at most 20 per page load) over the same private channel;
  the server records refused data requests itself. Each tab keeps its latest
  20 problems in SQLite; Settings → Tabs shows the latest 3.

## Delivery Stories

Each story is independently demonstrable and ends with
`./scripts/verify-change.sh` passing.

1. **Tabs from the folder, sealed, no data.**
   - Outcome: a Workspace with a folder shows each valid
     `.evie/tabs/*/` as a tab after the built-ins, rendered in the sealed
     frame. Settings → Tabs lists every tab with any problem.
   - Acceptance:
     - two tab folders show as two tabs in `order`; an invalid `tab.json`
       shows only in Settings → Tabs with its problem;
     - the file route refuses `..`, an escaping symlink, a dotfile and an
       over-limit folder;
     - in a real browser, a tab's script cannot fetch `/api/...`, cannot
       fetch an external URL, cannot read Evie's localStorage, and cannot
       navigate the top window;
     - at phone width, tabs that do not fit are in More and the header does
       not scroll.
   - Risks: the sandbox and CSP are the security boundary, so they get a
     browser check, not only header assertions.
2. **Data channel with owner approval.**
   - Outcome: a tab reads approved data through the helper.
   - Acceptance: an approved `finance` query returns rows; an undeclared
     database, a write statement, a file outside the glob and a dotfile are
     refused with reasons; changing `data` withdraws access until approved.
   - Depends on 1.
3. **Evie builds and fixes tabs.**
   - Outcome: in a Workspace chat, "make a tab that …" produces a working
     tab; a broken tab's problem is visible to Evie, which fixes it.
   - Depends on 2.
4. **Finance Reports.**
   - Outcome: the Finance Workspace has a folder, and Evie builds a Reports
     tab there on the approved `finance` database.
   - Acceptance: the tab shows monthly spend by category from live data, and
     Evie changes it from a Finance chat request.
   - Depends on 3.
5. **Evie's look inside tabs.** The design-system library (`designSystem.ts`,
   design-sync 2026-10-08) and theme tokens are served to tabs so Evie's tabs
   match the app.
   - Depends on 1.
   - Built (2026-10-12): the app's own built stylesheet is served to every tab
     at `_evie/evie.css`, giving its theme tokens, font and utility classes;
     it references no other files. The React components of `designSystem.ts`
     need React bundled for tabs and are left for a later story.

## Testing Decisions

- Go route tests, against a real temporary folder and database, for
  discovery (valid, invalid, over-limit), the file route's confinement and
  headers, and the guard refusing `Origin: null` on management routes.
- UI component tests for the header order, the More menu and Settings →
  Tabs, with the network faked at the API module as in the existing
  workspace tests.
- A headless-browser check, like `scripts/memory-stage-4-browser.cjs`, that
  loads a hostile test tab and asserts each sealing property in story 1.
- Story 2 adds bridge tests: every refusal path, and approval pinning.

## Out of Scope

- Running Evie-written code inside Evie's own process. Tabs run only in the
  browser sandbox.
- Writes or actions from a tab (starting runs, editing data). A later story
  could route them through existing approval gates.
- Network access from tabs, and third-party scripts not vendored in the tab
  folder.
- Editing tab code in the UI.
- Removing Data → Spending; that waits on the owner after story 4.
- Sharing one tab across Workspaces.

## Decisions

Confirmed by the owner on 2026-10-12 (workspace-tabs.decisions.md): tabs
are real pages sealed by the browser sandbox (amending `plugins.spec.md`);
custom Workspace UI is allowed for tabs only (an exception to
`workspaces.spec.md`); tabs need a Workspace folder; and Evie builds Finance
Reports on the data channel.
