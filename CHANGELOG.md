# Changelog

Every merge to master, newest first: what changed and why. The system
explorer's History tab shows this file, and its drift check validates it.
The entry format, the check's rules, and how to add an entry are in the
[explorer README](docs/architecture/explorer/README.md#history). After
editing this file, run `go run ./scripts/explorer extract` and
`go run ./scripts/explorer check`.

## 2026-10-31 — System page and History
Branch: feat/system-page
Touches: internal/web, docs/architecture/explorer, scripts/explorer, internal/web/ui
Sources: doc:cmd/evie/docs/active/system-page.spec.md, #195, #196, #197

Brings the system explorer into Evie. A System entry under Data in the
sidebar opens the explorer full-height in a sandboxed frame. `evie serve`
serves a snapshot of the explorer built into the binary under `/explorer/`,
with a `build.js` naming the repository, the commit, and whether the build had
uncommitted changes. Code links in the served explorer open GitHub at the
build's commit, or master when there is no commit stamp or the build was
modified; opened from disk, they stay local file links.

The explorer gains a fifth tab, History, drawn from this file: every merge to
master and every run of direct-to-master commits, newest first and grouped by
month, and each package's panel lists the entries that touched it. `extract`
parses `CHANGELOG.md` into the facts, and the drift check validates each
entry's format, date, order, sources, and commits. The changelog is backfilled
for master's whole first-parent history.

## 2026-10-29 — System explorer
Branch: feat/system-explorer
Touches: scripts/explorer, docs/architecture/explorer
Sources: doc:cmd/evie/docs/active/system-explorer.spec.md, doc:cmd/evie/docs/active/system-explorer.decisions.md, doc:docs/architecture/explorer/README.md, #186, #187, #188, #189, #190, #191, #192, #193, #194

Adds the system explorer, a page opened straight from disk that shows what
Evie does: every package under `cmd/` and `internal/` as a component, grouped
under the architecture map in `AGENTS.md`, with levels from the system context
down to one package's focus view, key runtime flows as sequence diagrams to
step through, and design proposals toggled on top of the built system.

Written content cites its sources, and the facts beneath it are extracted from
the code: packages, imports, commands, routes, database tables, glossary terms,
and ADRs. The drift check, run by `./scripts/verify-change.sh`, fails when the
data and the code disagree, so the map cannot drift quietly.

## 2026-10-28 — Repository aligned with the current skills setup
Branch: master
Commits: 7bc0936..7bc0936
Touches: GLOSSARY.md, AGENTS.md, docs/agents, cmd/evie/docs
Sources: doc:GLOSSARY.md, doc:docs/agents/domain.md, doc:docs/agents/triage-labels.md, doc:docs/agents/issue-tracker.md

Renames `CONTEXT.md` to `GLOSSARY.md`, the name the skills now read, and
points the four docs that referenced it at the new name. Adds the domain and
triage-label docs the skills expect, and an Agent skills section in
`AGENTS.md` that points at them. The issue-tracker doc now says tickets are
GitHub issues and specs are repository docs, matching `AGENTS.md`.

## 2026-10-26 — Tool results projected in batches, and lighter turns
Branch: cache/batched-projection
Merge: cc69b4d
Touches: internal/agent, internal/eviedb
Sources: doc:cmd/evie/docs/active/memory.decisions.md, code:internal/agent/request_history.go, code:internal/agent/turn_decodes.go

While a request fits usable input, older tool-result groups past the three
retained ones now become eligible for pressure projection in whole batches of
four, so the projected history changes once a batch instead of on almost every
tool call. Cached-token counts from the owner's history showed that rolling
projection caused most avoidable provider cache misses; in a replay of a long
chat, the previous request's bytes left unreused fell from 1.70 MB to 1.37 MB
for 1.6 percent more bytes sent. The composer version becomes
`context-composer-v6`.

The branch also cuts the work each model request repeats within a turn: the
turn reads only the events committed since its previous request
(`EventsAfter`), derives history checks once per request, decodes each stored
event once, and sizes pressure projection without re-encoding the whole
request. On a 70-turn chat, overhead per model request fell from 68 ms to
23 ms; requests are unchanged.

## 2026-10-26 — Opening the database checks only semantic foreign keys
Branch: perf/startup-fk-check
Merge: c114768
Touches: internal/eviedb
Sources: code:internal/eviedb/semantic_replay.go

Every open of Evie's database ran an unscoped `PRAGMA foreign_key_check`,
reading every row of every table, and then kept only the `semantic_*`
failures: 43 ms of a 55 ms open at 50,000 messages, paid by every `evie`
command. The check now runs on each `semantic_*` table in name order
(0.16 ms) and finds the same failures. Its rows are now closed on a scan
error, which had left the connection checked out.

## 2026-10-26 — Idle retrieval ticks read before they write
Branch: perf/dense-idle-no-write
Merge: adf24f5
Touches: internal/eviedb
Sources: code:internal/eviedb/retrieval_dense_index.go#Store.ensureDenseGeneration

The once-a-second retrieval maintenance tick took SQLite's write lock even
when nothing changed: an UPDATE disabling dense retrieval (the default) or,
with it enabled, a `BEGIN IMMEDIATE` around an unchanged selection. While a
turn held the lock, an idle tick waited out the five-second busy timeout and
failed. `ensureDenseGeneration` now reads the selection first and writes only
when it must change.

## 2026-10-26 — Due workflow runs found without reading finished ones
Branch: perf/due-workflow-runs-index
Merge: df8b10e
Touches: internal/eviedb
Sources: code:internal/eviedb/workflow_run_leases.go

The scheduler asks for due runs once a second, and its query ORed a condition
per due state, so SQLite could use no index and scanned every run retention
kept: 6.24 ms with 20,000 finished runs. The query now starts with one `IN`
term over the five due states, read through a new partial index holding just
those runs (0.068 ms for the same history). The `workflow_runs_unfinished`
index, which the query never used, is dropped on open.

## 2026-10-26 — An idle compiler host stops taking the write lock
Branch: perf/compiler-host-idle
Merge: 999d61e
Touches: internal/eviedb, cmd/evie
Sources: code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost, code:cmd/evie/compiler_host.go

`RunCompilerHost` ran four `BEGIN IMMEDIATE` transactions every 25 ms whether
or not there was work, re-checked a turn still in progress 40 times a second,
and dropped reconciliation errors without a word. Each tick now looks for
work read-only first: ready work runs every 25 ms until it is gone, work that
only time or a released turn lease can move runs once a second, and with
nothing pending the host takes no write lock and looks every 250 ms. Failures
retry once a second and reach a new `ReconcileHealth` callback, which the
runtime logs. Idle CPU over 3 s drops from 116 ms to 12 ms with nothing
pending, and from 369 ms to 24 ms with a turn in progress.

## 2026-10-26 — Search-index refreshes and coverage without index scans
Branch: perf/index-coverage-count
Merge: 5382545
Touches: internal/eviedb
Sources: code:internal/eviedb/retrieval_index_rows.go, code:internal/eviedb/retrieval_index.go

The conversation and accepted-memory FTS tables keep their event or Claim id
in an unindexed column, so every refresh deleted a row by scanning the whole
index inside the write lock: 4.16 s for a 256-message batch at 50,000
messages. Each index now has a lookup from id to FTS rowid, written in the
same transaction as the row, and refreshes delete through it (0.10 s);
opening the database rebuilds a lookup that does not match its index.

Coverage, read by every search and maintenance tick, now counts indexed rows
through those lookups and keeps the dirty queue as the outer loop. At 50,000
messages the coverage read drops from 19.9 ms to 0.34 ms, and an idle
maintenance tick from 40.2 ms to 0.71 ms.

## 2026-10-26 — Dead code removed and scratch output untracked
Branch: cleanup/dead-code-and-junk
Merge: e919e80
Touches: internal/web, internal/finance, internal/tools, internal/eviedb, internal/plugins, internal/openrouter, internal/subagents, cmd/evie, .gitignore
Sources: code:internal/delegation/types.go#ExploreToolNames

Deletes the functions nothing calls, not even tests: the legacy `web.Serve`
entry points, which duplicated `ServeWithContext` without shutdown, unused
REPL and file-edit wrappers, `finance.Query` and `OpenDBReadOnly`, and a
handful of orphaned helpers. The explore test now checks the toolset against
`delegation.ExploreToolNames`, the list the supervisor enforces, instead of a
second copy. `.scratch/`, `tmp/` and `__pycache__/`, about 460 MB of local
run output, binaries and rendered drafts, are no longer tracked; curated
evidence stays under `cmd/*/docs/fixtures/`.

## 2026-10-13 — Workspace memories stay in their Workspace, reviewed as a deck
Branch: master
Commits: bda9f7d..0f1f386
Touches: internal/memorybackground, internal/eviedb, internal/memory, internal/web, internal/web/ui
Sources: doc:cmd/evie/docs/active/memory.decisions.md, doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass

The background pass had routed any memory the model called "profile" to
Global, so a Finance chat's merchant-rule request became a Global profile
fact. In a Workspace or project chat every memory now targets that scope; one
that Evie suggests holds everywhere also carries a prepared Global version,
and the review card's "Save everywhere instead" approves that exact version.
Global chats and in-turn proposals are unchanged.

The review then becomes a deck showing one proposal at a time. A card's face
holds the memory, with pills for where it applies, its kind and Evie's
wording, and the Always applies and Everywhere instead toggles; flipping it
shows the message Evie read and the exact change. Swipes or the arrow keys
save, decline or skip, and "Decline all" clears a batch from one message. The
background pass becomes a status dot, and an empty queue says All caught up.

## 2026-10-12 — Workspace tabs read approved data and can show built-in views
Branch: master
Commits: e517a4f..a3fa82e
Touches: internal/workspacetabs, internal/web, internal/web/ui, internal/plugins, internal/tools, internal/eviedb, internal/agent, internal/workflowruntime, cmd/evie, scripts
Sources: doc:cmd/evie/docs/active/workspace-tabs.spec.md, doc:cmd/evie/docs/active/workspace-tabs.decisions.md

A tab's `tab.json` may declare data (a registered database, files in the
Workspace folder matching a glob, or the Workspace's workflow runs) and reads
nothing until the owner approves that exact list in Settings → Tabs. Tabs
read through Evie's helper over a private `MessageChannel` opened with a
one-time key, and the server checks each request against the approved list.
A review then bounded each query to 4 seconds and each value to 2 MB, capped
concurrent tab reads, and refused secret-looking paths, SQLite files and hard
links.

Evie's helper also reports a tab's script errors, failed loads and policy
blocks, each tab keeps its latest 20 problems, and a new `workspace_tabs`
tool gives Workspace chats the tab contract, each tab's status, data list and
approval, and its problems. Tabs can load Evie's own stylesheet at
`_evie/evie.css`, and query rows arrive as objects keyed by column. David
accepted WebRTC as a residual risk and asked for a partial block, so every
tab page now starts with a script that removes the WebRTC constructors.
Finally, `tab.json` may name `"builtin": "spending"` to show the real
Spending view in the Workspace.

## 2026-10-12 — Workspace tabs: a Workspace's own pages, sealed
Branch: master
Commits: 0c4f91d..d529e8a
Touches: internal/workspacetabs, internal/web, internal/web/ui, internal/eviedb, cmd/evie, scripts
Sources: doc:cmd/evie/docs/active/workspace-tabs.spec.md, doc:cmd/evie/docs/active/workspace-tabs.decisions.md

A Workspace can have its own tabs, written by Evie and kept in its folder
under `.evie/tabs/<tab-id>/` with a `tab.json` and an `index.html`.
`internal/workspacetabs` lists and validates them within fixed limits and
opens files only through an `os.Root` on the tab folder, and `evie serve`
serves them with a sandboxing content-security policy: scripts, no
same-origin, no network. A review then hardened serving: redirects built only
from checked IDs, only regular files reached without a link, loading only
from the tab's own folder URL, and an app shell that frames only Evie's own
pages, so a tab cannot navigate its frame to another site.

Tabs show in the Workspace header after Runs, Changes and Workflows, with a
More menu for those that do not fit, each in an iframe sandboxed to scripts.
Settings → Tabs lists every tab folder and what is wrong with it. A browser
check proves the seal in Chrome with a hostile tab.

## 2026-10-12 — Several chats at once
Branch: master
Commits: 7d1613a..af14f41
Touches: internal/web, internal/web/ui, internal/openrouter, internal/agent, cmd/evie
Sources: doc:cmd/evie/docs/active/multi-chat.spec.md, doc:cmd/evie/docs/active/multi-chat.decisions.md

The server held one chat and refused to open another while any turn ran. It
now keeps up to 8 open chats, each with its own agent session and model,
closing the least recently used idle one at the limit, and a turn in one chat
no longer blocks another. Each chat publishes its turn events to a feed the
browser follows, so leaving a running chat or reloading no longer loses the
turn, and an approval in a detached turn waits while any Evie tab is
connected. The sidebar marks chats that are running, waiting on an approval,
or finished since they were last viewed. Opening a chat also reuses a model's
resolved context profile for 10 minutes instead of asking OpenRouter twice
every time.

## 2026-10-12 — MCP connections
Branch: mcp-connections
Merge: 5ca729c
Touches: internal/mcpconn, internal/mcpchat, internal/plugins, internal/eviedb, internal/web, internal/web/ui, internal/tools, cmd/evie
Sources: doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md, doc:cmd/evie/docs/active/workspace-mcp-connections.decisions.md, #179

`evie serve` connects to hosted MCP servers: OAuth sign-in with PKCE,
refresh with rotation saved before use, and tokens kept only in the macOS
Keychain. A spike found that the MCP Go SDK re-sends on 401 and 403 and
cannot tell unsent calls from unanswered ones, so Evie owns tokens, refresh
and sending once, and classifies a failed call as not sent, rejected or
unknown.

Workspace chats reach the MCP tools David reviewed for that Workspace through
two general tools, `mcp_list_tools` and `mcp_call`, which frames every result
as untrusted. A call asks first unless David sets that exact tool version to
Allow, and Settings → Connections adds servers, signs in, and enables tools
and sets their policy per Workspace. The work reached master as pull request
#179, merged here from `origin/master`.

## 2026-10-12 — Watch a chat's subagents
Branch: master
Commits: 36c6087..ee2111a
Touches: internal/web, internal/web/ui, internal/eviedb, cmd/evie
Sources: doc:cmd/evie/docs/active/subagents.decisions.md, doc:cmd/evie/docs/active/chat-activity.spec.md

Two read-only routes, `/api/subagents/list` and `/api/subagents/trace`,
return the children one delegation call admitted, continued or replayed, and
one child's events for that attempt. Both answer only for the open
conversation, and child sessions still cannot be opened as chats. In the
chat, delegate rows say what they do, such as Research with subagents, and
report a live call's children ("Researching · 2 of 4 done"); inspecting the
row lists each child's state, assignment, elapsed time and latest action, and
a child opens to its assignment, its context, and its own trace and report.

## 2026-10-12 — Collapsible Workspaces, selectable models, and the model's current thought
Branch: master
Commits: 38e6ba3..a50ee99
Touches: internal/web/ui, internal/openrouter
Sources: doc:cmd/evie/docs/active/chat-model-selection.spec.md, doc:cmd/evie/docs/active/chat-activity.spec.md, doc:cmd/evie/docs/active/chat-activity.decisions.md

A chevron collapses a Workspace in the sidebar to hide its chats, remembered
per browser. The `/model` picker lists only models that can be selected: 125
of 390 catalog models had failed selection, so variants, routers, and models
without a usable output limit or below the reply reserve are skipped. While
Evie works with its activity collapsed, the status line shows the latest
heading or complete sentence of the streamed reasoning, held at least two
seconds. Turns no longer show a Repository instructions action, memory
activity hides retrievals that found nothing, and the Workspace tab strip no
longer scrolls.

## 2026-10-11 — No memory proposals for archived Workspaces
Branch: master
Commits: 6a3c787..6a3c787
Touches: internal/eviedb, internal/memorybackground
Sources: doc:cmd/evie/docs/active/workspaces.decisions.md

A pending proposal that targets an archived Workspace, or came from one of
its conversations, leaves the review inbox and its count, and approving it is
refused; restoring the Workspace brings it back. The background pass stops
reading an archived Workspace's conversations, and queueing a new proposal for
one is refused.

## 2026-10-11 — Design-sync setup and backlog notes
Branch: master
Commits: 93d2227..0ea2e46
Touches: internal/web/ui, .design-sync, docs
Sources: doc:docs/BACKLOG.md

`designSystem.ts` exports the UI's props-driven components as a library
entry, and `.design-sync` holds the sync configuration, conventions, notes
and component previews for Evie's design system. The backlog records the
finance-review status and items for the folder chooser, tool results and the
turn budget.

## 2026-10-11 — Finance review workflow
Branch: master
Commits: c1e4fc6..c1e4fc6
Touches: internal/finance, internal/plugins, internal/workflow, internal/workflowruntime, internal/tools, internal/agent, internal/web/ui, cmd/evie, cmd/evie/workflows
Sources: doc:cmd/evie/docs/active/finance-review.spec.md, doc:cmd/evie/docs/active/finance-review.decisions.md

A workflow for unclassified posted transactions: a model proposes a category
for each as typed data, the deterministic `gate_proposals` step drops
anything that breaks the rules, and the run pauses for the owner's answer per
line. One approved effect, `finance.apply_review`, writes the accepted
entries with an exact merchant rule each, and `review_decisions` records every
answer so a rejected transaction is never proposed again. The finance plugin
moves to 1.3.0; its tools are unchanged.

## 2026-10-11 — Delete a Workspace by archiving it
Branch: master
Commits: c06ac3e..c06ac3e
Touches: internal/eviedb, internal/web, internal/web/ui, cmd/evie
Sources: doc:cmd/evie/docs/active/workspaces.decisions.md

The sidebar's "Delete workspace", behind a confirmation that says what is
kept, archives the Workspace: it leaves the sidebar and the Workspaces page
and takes no new sessions or runs, but keeps its sessions, memory and
history. Settings → Archived workspaces lists each one with Restore, through
new `/api/workspaces/archive` and `/restore` routes that are refused while a
turn runs.

## 2026-10-11 — Review cards show the message Evie read
Branch: master
Commits: eae9e11..eae9e11
Touches: internal/eviedb, internal/memory, internal/web/ui
Sources: doc:cmd/evie/docs/active/memory.decisions.md

Each inbox proposal now carries the owner message it was prepared from,
clipped to 2,000 bytes, and cards are grouped under that message. A card
leads with the memory in Claim-text form and folds kind, scope and the Always
applies switch into one line; the repeated Evie-proposed warning becomes an
"Evie's wording" tag explained once.

## 2026-10-10 — Changes: the Changes tab, the review page, and builds in the Inspector
Branch: master
Commits: 06115fc..3ae24bb
Touches: internal/changes, internal/localfiles, internal/web, internal/web/ui, internal/plugins, internal/subagents, internal/agent, internal/workflowruntime
Sources: doc:cmd/evie/docs/active/changes.spec.md#delivery-stories, #177

A Workspace with a Repository gets a Changes tab that groups Changes into
"Needs your review", "In progress" and "Merged and closed". A Change opens on
a review page with the action its state calls for and Close, a rail of files
with "Viewed" marks, checks and origin, and Evie's notes beside a
side-by-side or unified diff read through `localfiles.Compare`. A chat's
build rows open the Inspector on the Change they made, and in a Workspace
chat `propose_build_merge` shows no merge card: the Change waits in the
Changes tab, where the owner merges it. The Inspector's folder "Changes" tab
is renamed "Working tree".

## 2026-10-10 — Changes: merge, close, and "Merge when checks pass"
Branch: master
Commits: de5b0fa..e95629e
Touches: internal/changes, internal/github, internal/eviedb, internal/web, internal/web/ui, internal/subagents, internal/delegation, internal/workflowruntime, cmd/evie
Sources: doc:cmd/evie/docs/active/changes.spec.md#delivery-stories, #177

`evie changes merge|close` and their routes let the owner act on Changes
without a chat. In a local Repository a merge goes through the merge gate,
which verifies, rebases and verifies a draft again first; on GitHub it reads
the pull request, refuses a draft or a head other than the one Evie last
showed, and uses GitHub's "Rebase and merge". After any merge Evie deletes
its own branch and worktree and fast-forwards the owner's base branch only
when the checkout is on it with no uncommitted edits; the owner's own
branches are never removed.

"Merge when checks pass" arms a pull request so a sync merges it once every
check has passed, and clears it on a failing check or a conflict. To carry
its notices, the workflow notifications table is rebuilt into one store for
workflow and Change notices.

## 2026-10-10 — Changes: push and open pull requests
Branch: master
Commits: 6fd68f4..ef007f8
Touches: internal/changes, internal/github, internal/plugins, internal/subagents, internal/delegation, internal/workflow, internal/workflowruntime, internal/web, cmd/evie
Sources: doc:cmd/evie/docs/active/changes.spec.md#delivery-stories, doc:cmd/evie/docs/active/workflow-runtime.decisions.md, #177

`evie changes push` and `POST /api/changes/push` push one of Evie's draft
Changes in a GitHub Repository: the Workspace's Verify runs first in the
build's worktree, sandboxed without network, the branch goes to origin with
the token as a per-command header, and the pull request opens with Evie's
notes and how Verify ended. A live test opened a pull request titled with the
build's truncated objective and described with the worker's whole report, so
`propose_build_merge` now requires a reviewer-facing title and description
that name no local folder.

A new GitHub plugin adds the workflow effect `github.open_change`, which does
the same for a workflow's build but requires Verify to pass. Because
Repositories differ per Workspace, an effect may now declare one resource
prefix (`repo:`) instead of a list.

## 2026-10-10 — Changes: the Change list and GitHub sync
Branch: master
Commits: 32c474d..3c88e43
Touches: internal/changes, internal/github, internal/eviedb, internal/web, cmd/evie
Sources: doc:cmd/evie/docs/active/changes.spec.md#delivery-stories, #177

A Change is one branch of work with its lifecycle state, author, origin,
notes, pull request and checks. Evie's own Changes are read from its build
records, GitHub pull requests and their checks are stored by a 60-second sync
with a refresh route, and a pull request on one of Evie's build branches is
that Change on GitHub. `evie changes list` and the list route read both, and
a folder guard protects a Workspace whose Changes are unfinished. A live test
found that GitHub's Checks API refuses fine-grained tokens, so a pull
request's checks are now the newest run of each GitHub Actions workflow on
its head commit plus its commit statuses.

## 2026-10-10 — Agents may create branches and worktrees
Branch: master
Commits: 6cb75cc..6cb75cc
Touches: AGENTS.md
Sources: doc:AGENTS.md#git-safety

`AGENTS.md` lets agents create branches and worktrees as the work needs, such
as one worktree per parallel subagent, and remove each once its work is
merged. The owner's checkout stays on its branch with its changes in place.

## 2026-10-10 — Changes: a Workspace's Repository
Branch: master
Commits: 6cac07f..3e7b57b
Touches: internal/changes, internal/github, internal/worktree, internal/web, internal/web/ui, cmd/evie
Sources: doc:cmd/evie/docs/active/changes.spec.md, doc:cmd/evie/docs/active/changes.decisions.md, #177

A Workspace's Repository is read from its folder on each request, never
stored: local at the top of a git work tree, GitHub when its origin names a
github.com repository. With `GITHUB_TOKEN` set, a small GitHub REST client
checks the token's access, and Settings → Repository shows the Repository,
its base branch and the token status. Settings can publish a local
Repository to GitHub, and creating a Workspace offers no repository, a new
local one, a new one on GitHub, or connecting an existing GitHub repository.
Git commands that reach GitHub run hardened, with the token only as a
per-command auth header, never in arguments, git config, the remote URL or
output.

## 2026-10-09 — Chat shell reads of Evie's own state accepted
Branch: master
Commits: 997bc3f..997bc3f
Touches: cmd/evie/docs
Sources: doc:cmd/evie/docs/done/bash.decisions.md

On 2026-10-09 a Finance Workspace chat read `~/.evie/evie.db` and Evie's
source through the ungated chat shell. David kept the shell ungated, and the
bash decision record names the real alternative, the OS sandbox with an
approval gate to leave it, if this is ever revisited.

## 2026-10-09 — Workspace activity: Runs, Workflows, notifications, and run cards
Branch: master
Commits: 8cc5ea7..c721272
Touches: internal/web, internal/web/ui, internal/workflow, internal/workflowruntime, internal/eviedb
Sources: doc:cmd/evie/docs/active/workspace-activity.spec.md, doc:cmd/evie/docs/active/workspace-activity.decisions.md, doc:cmd/evie/docs/active/changes.spec.md, #177, #178

Specifies Changes (#177) and Workspace activity (#178), then builds the
activity side. A Workspace opens on a header with Runs, Workflows and
Settings tabs, a badge for runs waiting on the owner, and today's workflow
spend against the daily cap, and a notifications icon on every view lists
notices from every Workspace. The Runs tab groups runs as Waiting for you,
Running and Earlier, each with a bar of its steps, and opens a run in a panel
with its question, error, steps, outputs, and Cancel or Rerun. The Workflows
tab opens each workflow in a pop-up with its Steps and YAML views, approval
and recent runs, and a chat's `workflow_start` call shows a live run card
that opens the run in the Inspector.

## 2026-10-07 — Finance: a scheduled daily workflow, name rules, and transfers out of spending
Branch: master
Commits: 8573641..fa59262
Touches: internal/finance, internal/plugins, internal/tools, internal/workflow, internal/workflowruntime, internal/agent, internal/web, internal/web/ui, cmd/evie, cmd/evie/workflows, cmd/finance, scripts
Sources: doc:cmd/evie/docs/active/finance-daily.spec.md, doc:cmd/evie/docs/active/finance-daily.decisions.md, adr:0067, doc:cmd/finance/docs/active/categorize-patterns.spec.md, doc:cmd/evie/docs/active/spending.decisions.md

The daily finance job was a launchd cron job running a separately installed
`finance` binary that had drifted two months behind source, so two banks
wedged on foreign-key failures and categorize was skipped on those days.
Finance becomes the first production workflow effect provider:
`finance.sync_ledger` and `finance.categorize_ledger` record a receipt per
idempotency key, and the `finance-daily` workflow syncs, categorizes and
checks for failed banks at 09:00 America/Detroit. `evie serve` runs as a
KeepAlive LaunchAgent so the scheduler is up with no terminal open, the old
cron job's removal is recorded, and `scripts/release.sh` verifies, installs
and restarts `evie serve` in one step.

Transactions with no merchant name, such as card payments, transfers and
payroll, could never match a merchant rule, so `name_rules` adds prioritized
SQL `LIKE` patterns on the bank string, and `excluded` becomes a real
category for money that deliberately does not count as spending; on the live
database, transactions awaiting review fell from 339 to 125. The Spending
overview then leaves excluded allocations out of every total, and card
payments get their own Credit Card Payments category, which the overview
omits on both legs.

## 2026-10-04 — Built-in deep research v2
Branch: master
Commits: 023f21a..023f21a
Touches: internal/workflow, internal/workflowruntime, internal/eviedb, internal/web, internal/web/ui, internal/agent, internal/plugins, cmd/evie
Sources: doc:cmd/evie/docs/active/workflow-runtime.decisions.md, doc:docs/memory-architecture.md

Deep research is built into Evie and runs in every Workspace without a
proposal or approval, because a built-in holds no Standing Authority: no
capability, schedule or build worker, enforced by the validator and the
database. Each built-in version is recorded in procedural Git before a run
pins it; a name resolves to the Workspace's approved copy, then the Global
one, then the built-in; and the owner can disable built-ins or customize
them. Deep research v2 searches several angles under a 15-source budget,
extracts quoted claims, and puts each claim to three adversarial verifiers
that count only evidence the harness saw, with honest inconclusive outcomes.
The Subagents plugin is on by default for new databases.

## 2026-10-04 — The /model picker and collapsed activity
Branch: master
Commits: a223a4f..a223a4f
Touches: internal/web/ui, scripts
Sources: doc:cmd/evie/docs/active/chat-model-selection.spec.md, doc:cmd/evie/docs/active/chat-activity.spec.md

The always-visible model dropdown becomes a searchable `/model` picker opened
from the composer and grouped by provider. The working trace collapses into
a live status line with a single summary for settled tool problems.

## 2026-10-04 — Workflow runtime: agent and map steps, deep research, schedules, and retention
Branch: master
Commits: 4d58f14..fbe2c73
Touches: internal/workflowruntime, internal/workflow, internal/eviedb, internal/subagents, internal/delegation, internal/plugins, internal/agent, internal/web, internal/web/ui, cmd/evie
Sources: doc:cmd/evie/docs/active/workflow-runtime.spec.md, doc:cmd/evie/docs/active/workflow-runtime.decisions.md, adr:0020, adr:0023, adr:0030, adr:0045, adr:0046

Stage 14 sub-stages 9 to 12 of the memory architecture plan. Agent steps run
research, explore and build workers owned by the run, and map steps fan out
over a list with bounded parallelism and per-item recovery. The deep-research
template ships with verifiers that must fetch their evidence and a
deterministic citation check, and a Workflows plugin gives chats
`workflow_start` and `workflow_status`; nothing model-facing proposes,
approves or cancels a run.

Approved schedules start runs with bounded catch-up after downtime, a daily
cap limits all workflow runs' AI calls and workers (by default $5 and 10
million tokens), and run transitions record durable notifications.
Unfinished runs can migrate to successor runs, and retention policies apply
audited redaction, deletion and checkpoint compaction to finished runs.

## 2026-10-04 — Workflow runtime: capability reads and effects, AI steps, and cancellation
Branch: master
Commits: 1a98fe0..d01b83f
Touches: internal/workflowruntime, internal/workflow, internal/eviedb, internal/plugins, internal/web, internal/web/ui, internal/agent, cmd/evie
Sources: doc:cmd/evie/docs/active/workflow-runtime.decisions.md, adr:0009, adr:0010, adr:0015, adr:0017, adr:0041

Stage 14 sub-stages 5 to 8. Plugins contribute workflow capability reads
through a focused provider role, starting with fixed reads of the local
finance database; runs pin each provider and pause when one is missing or
changed until the owner approves an audited substitution. Effects run through
a durable effect protocol, where an interrupted effect's outcome is unknown
and moves the run to reconciliation rather than a rerun. Interrupt steps wait
for the owner's typed answer from any surface, and AI steps send only the
reviewed prompt and declared input to the exact model of their Model Policy.
The owner can cancel runs, revoked runs pause at node boundaries, and a
succeeded Run Key reruns only under an audited override.

## 2026-10-04 — Workflow runtime: validate, approve, run, and recover
Branch: master
Commits: 5166b3a..9c7dcdb
Touches: internal/workflow, internal/workflowruntime, internal/procedural, internal/eviedb, internal/web, internal/web/ui, cmd/evie
Sources: doc:cmd/evie/docs/active/workflow-runtime.spec.md, doc:cmd/evie/docs/active/workflow-runtime.decisions.md, doc:docs/memory-architecture.md, adr:0007, adr:0008, adr:0011, adr:0018, adr:0021

Stage 14 sub-stages 1 to 4 of the memory architecture plan. `workflow.yaml`
definitions are parsed strictly, validated, hashed and simulated with effects
suppressed (`evie workflows validate|simulate`). Definitions are proposed
into a private procedural Git repository, and one owner approval activates
one exact executable and authority hash within the Workspace's access,
granting standing authority without starting a run. Runs of a Workspace's
active workflow pin their version and record a checkpoint before each node.
They advance under a fenced lease, so the REPL, `evie serve` and CLI starts
share one database, and a background scheduler resumes a dead holder's run
from its checkpoint.

## 2026-10-04 — Build workers, the merge gate, and Task Tree orchestration
Branch: master
Commits: b754c92..a796c99
Touches: internal/subagents, internal/delegation, internal/worktree, internal/plugins, internal/tools, internal/eviedb, internal/agent, internal/task, internal/memory, internal/web, internal/web/ui, cmd/evie, cmd/todo
Sources: doc:docs/memory-architecture.md, doc:cmd/evie/docs/active/subagents.spec.md, doc:cmd/evie/docs/active/subagents.decisions.md, term:Task Tree

Stages 11 to 13 of the memory architecture plan. `delegate_build` gives a
build worker one bounded code change on one Task, with free edits inside a
harness-created git worktree on a fresh `evie/` branch: file tools are
confined through `os.Root`, and bash runs only under Seatbelt or bubblewrap
with no network. Workspaces need an explicit build setting, off by default.
`propose_build_merge` verifies the worker's branch with the owner's verify
command, rebases it onto its target, verifies again, and only then shows an
approval card; on approval the harness fast-forwards the target without
stashing, resetting or pushing. Tasks gain dependencies and paths, and
`todo_ready` returns a tree's dispatchable Tasks in conflict-free batches
sized to the worker caps.

## 2026-10-03 — Memory architecture Stages 6 to 10: recall quality and context management
Branch: master
Commits: a82bb8f..3f41a54
Touches: internal/agent, internal/eviedb, internal/memory, internal/memoryeval, internal/memorybackground, internal/subagents, internal/delegation, internal/plugins, internal/tools, internal/web, internal/web/ui, cmd/evie, cmd/todo
Sources: doc:docs/memory-architecture.md, doc:docs/context-management-comparison.md, doc:cmd/evie/docs/active/memory.spec.md#memory-flush-before-compaction, doc:cmd/evie/docs/active/subagents.spec.md

Read-only explore workers (`delegate_explore`) get `read_file` confined to a
pinned folder, the read-only `query_db`, and a bash that parses each command
line itself and runs only allowlisted read-only commands without a shell. By
default a Workspace allows explore exactly while it has a local folder,
confined to that folder.

When a request cannot fit and no whole-turn prefix reaches the compaction
target, the active turn's oldest completed tool groups are summarized into a
labelled block, keeping the newest two verbatim. Requests that offer
`read_tool_result` show at most 32 KiB per tool result, as a head-and-tail
excerpt that names the omitted range, and memory, todo and delegation results
are protected from shortening. Every committed compaction offers its covered
owner messages to the background memory pass, owed durably across restarts.

Messages that are the source of an accepted Claim are indexed under that
Claim's words, and a deterministic parser reads one explicit past time ("last
week", "in March") so search reads that window first. Automatic Recall now
holds accepted Claims, their companions and dense-only hits to the same
relevance floor as conversation excerpts.

## 2026-10-03 — Memory architecture plan and Stages 1 to 5
Branch: master
Commits: d531b17..5ec2a41
Touches: internal/memory, internal/memorybackground, internal/eviedb, internal/agent, internal/plugins, internal/tools, internal/localextractor, internal/openrouter, internal/web, internal/web/ui, cmd/evie, docs
Sources: doc:docs/memory-architecture.md, doc:cmd/evie/docs/active/memory.spec.md#standing-memory, doc:cmd/evie/docs/active/memory.spec.md#proactive-memory, doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass, doc:cmd/evie/docs/active/memory.decisions.md

Writes down memory architecture research and a staged plan in
`docs/memory-architecture.md`, then extends it with context-management and
orchestration stages. Each accepted compaction now writes or advances its
session's episode, which `memory.search_episodes` searches on demand.
Approved standing memories reach every request by rule under a hard budget;
every kind but plain facts is standing by default, and the owner can switch
that before approving.

The system prompt states Evie's profile and Workspace memory duties. A
`remember` call shows the approval card only when the owner explicitly asked;
other proposals queue in a review inbox, and `memory_overview` returns the
standing set with current profile and Workspace memories in one read. An
always-on background pass reads the owner's messages after a conversation
goes idle or every 20 owner messages, decides whether each candidate adds,
updates or invalidates a memory, and queues every change for review; nothing
applies without approval.

## 2026-10-01 — Harness review: final verification pass
Branch: master
Commits: 2e46764..cf636a6
Touches: internal/agent, internal/eviedb, internal/subagents, internal/tools, internal/memoryeval, internal/openrouter, internal/memory, internal/web, internal/web/ui, cmd/evie
Sources: doc:docs/harness-review-2026-09-30.md, doc:cmd/evie/docs/active/memory-scale-eval.md

The harness review's final verification pass closes the gaps it found. Tool
fences frame cross-host redirect targets, cap bash spill files at 64 MiB,
back the evie database's table fence with an EXPLAIN-based check, and judge
NAT64 and 6to4 addresses by their embedded IPv4. Memory binds owner spans to
the sentence that states the Claim, through one shared clause and subject
analysis that fails safe, narrows free text in operation history outside its
scope, and treats conversational filler and acknowledgements as low content.

A context-length rejection after a same-iteration compaction now ends the
turn as `context_overflow`, and turns that ended without terminal evidence no
longer block compaction. Subagents retry terminal writes on lock contention
and refuse opening child sessions, and web tool errors use fixed harness
wording instead of server-controlled text.

## 2026-10-01 — Harness review: context budgeting, web turn controls, and the delegation contract
Branch: master
Commits: 06e8b72..b1a4bd3
Touches: internal/agent, internal/subagents, internal/delegation, internal/eviedb, internal/plugins, internal/openrouter, internal/untrusted, internal/tools, internal/memory, internal/web, internal/web/ui, cmd/evie
Sources: doc:docs/harness-review-2026-09-30.md, doc:cmd/evie/docs/active/subagents.spec.md, doc:cmd/evie/docs/active/subagents.implementation.md

Automatic compaction plans against a 20 KiB summary reserve and measures the
real summary before committing it, token budgets convert to bytes with a
per-model ratio calibrated from recorded input tokens, and a provider
context-length rejection triggers one compact-and-retry per turn. Requests
are ordered from most to least stable, with cache breakpoints for Anthropic
and Qwen routes; `read_tool_result` rereads the session's durable tool
outcomes; and the rolling summary is sent as a labelled data block. The
browser gains a Stop button and a `/compact` command.

Subagents are budgeted by time (15 minutes) and tokens (1M), end with a
tool-free wrap-up report at 90 percent of either or under context pressure,
and return their summary inline with the rest paged through
`read_subagent_report`. `continue_research` extends a finished child under
the current turn. Child-written text reaches the parent inside the escaped
untrusted-data frame, now shared from `internal/untrusted`, and delegated
child sessions are no longer listed as owner sessions.

## 2026-10-01 — Harness review: stale facts, the tool loop, and memory authority
Branch: master
Commits: f80bd74..f67dead
Touches: internal/eviedb, internal/agent, internal/memory, internal/memoryeval, internal/openrouter, internal/subagents, internal/finance, internal/web/ui, cmd/evie
Sources: doc:docs/harness-review-2026-09-30.md, doc:cmd/evie/docs/active/memory-scale-eval.md, doc:cmd/evie/docs/active/memory.decisions.md

Conversation excerpts now carry historical claims recomputed on each read, so
a corrected source stays retrievable but is marked superseded, and newer
owner statements are linked by deterministic wording rules; in the scale
evaluation, stale misses fell from 20 to 2. Each turn is capped at
`EVIE_TURN_STEP_LIMIT` model responses (default 100), the last requested
without tools, and a cancellation during the tool phase can no longer
deadlock. Proposed memories bind to the quoted owner span that states them,
and correction sources and legacy whole-message sources are narrowed outside
their scope. Concurrent finance writes now wait on lock contention instead of
failing.

## 2026-10-01 — Harness review: findings, scale evaluation, provider hardening, and recall relevance
Branch: master
Commits: 346bed5..83c8043
Touches: internal/memoryeval, internal/openrouter, internal/agent, internal/eviedb, internal/memory, cmd/evie
Sources: doc:docs/harness-review-2026-09-30.md, doc:cmd/evie/docs/active/memory-scale-eval.md

Records the 2026-09-30 harness review's findings and staged plan. A seeded
synthetic history of over 2,000 messages replays through real agent turns,
SQLite and the memory tools to score recall precision, unwanted items and
stale facts, and its baselines record that every target is unmet. Provider
streaming rejects truncated or errored completions before they are
committed, gains idle and connection timeouts, and retries no-response, 429,
502, 503 and 504 failures up to twice before any output. Automatic Recall no
longer searches on low-content messages, requires excerpts to match a
distinctive request term, and stops re-injecting messages still in the
request.

## 2026-10-01 — Harness review: tool fences and subagent supervision
Branch: master
Commits: 9a4f75d..ca12abe
Touches: internal/tools, internal/subagents, internal/eviedb, cmd/evie
Sources: doc:docs/harness-review-2026-09-30.md, doc:cmd/evie/docs/done/bash.decisions.md, doc:cmd/evie/docs/done/file-tools.decisions.md

The first fixes from the harness review harden the tool fences: finance
`query_db` accepts a single SELECT on a connection that cannot attach or
write files, research workers' `web_fetch` refuses loopback and private
addresses after DNS resolution, third-party text has its frame markers
escaped, `edit_file` through a symlink edits the target, and bash spills
output past its cap to a file. Subagent supervision proves parent authority
by the intent's place in the current turn, applies each child's deadline from
its own start, retries SQLite lock contention instead of ending authority,
and recovers each attempt in its own transaction, so unrecoverable records no
longer stop startup.

## 2026-09-29 — Spending accounts, the chat model selector, and Workspace research
Branch: master
Commits: 688c786..fb23624
Touches: internal/finance, internal/web, internal/web/ui, internal/eviedb, internal/openrouter, internal/subagents, internal/tools, internal/plugins, internal/agent, internal/delegation, cmd/evie, AGENTS.md
Sources: doc:cmd/evie/docs/active/spending-accounts-and-navigation.spec.md, doc:cmd/evie/docs/active/spending.spec.md, doc:cmd/evie/docs/active/chat-model-selection.spec.md, doc:cmd/evie/docs/active/workspace-research.spec.md

Data → Spending shows connected banks and their authorized accounts,
refreshed from Plaid only on request, and clicking a calendar day opens its
transactions, whose categories can be edited in place. The sidebar drops its
top-level New chat and Workspaces buttons. The composer gains a model
dropdown grouped by provider, and a chat's model is saved in SQLite for its
later turns. Standard Workspaces get an explicit research-delegation setting,
off by default, and research workers get a 1 MiB request budget and bounded
section and excerpt reading of fetched pages. `AGENTS.md` drops its derivable
repository map.

## 2026-09-21 — Workspace setup and session archive
Branch: master
Commits: ea9fa97..ea9fa97
Touches: internal/web, internal/web/ui, internal/eviedb, internal/memory, cmd/evie, cmd/todo
Sources: doc:cmd/evie/docs/done/workspace-setup-and-session-archive.spec.md

Workspaces are created in a modal with a persisted agent preset and the
native macOS folder picker. Settings gains session archive and restore
controls and font preferences.

## 2026-09-21 — Spending dashboard and folder-backed Workspace tools
Branch: master
Commits: 70c4c8a..70c4c8a
Touches: internal/finance, internal/localfiles, internal/localterminal, internal/repoinstructions, internal/agent, internal/tools, internal/memory, internal/openrouter, internal/eviedb, internal/web, internal/web/ui, cmd/evie
Sources: doc:cmd/evie/docs/active/spending.spec.md, doc:cmd/evie/docs/active/workspace-local-folder.spec.md, doc:cmd/evie/docs/active/repository-instructions.spec.md

Data gains a Spending dashboard: monthly cash-flow charts, daily calendar
navigation, saved transaction classification views, and an explicit bank
refresh. The same commit brings the Workspace Inspector, repository
instructions, local file and terminal tools, model compatibility, and
subagent context updates.

## 2026-09-11 — Tool details and memory results move to the Inspector
Branch: master
Commits: 19a57cd..f1a1a25
Touches: internal/web/ui, cmd/evie
Sources: doc:cmd/evie/docs/active/chat-activity.spec.md, doc:cmd/evie/docs/active/chat-activity.decisions.md

Generic tool arguments and results move from the chat to the selected
action's Inspector, which shows useful details first and the exact strings
behind Debug details, closed by default; the chat keeps compact action rows.
Recorded memory results are decoded for display, so values, scope, status
and search outcomes appear before Debug details.

## 2026-09-11 — Memory Stage 5, merged into the subagents work
Branch: codex/memory-stage-5
Merge: d426061
Touches: internal/agent, internal/eviedb, internal/memory, internal/localembedding, internal/plugins, internal/subagents, internal/tools, internal/web, internal/web/ui, cmd/evie
Sources: doc:cmd/evie/docs/active/memory-stage-5-retrieval.spec.md, doc:cmd/evie/docs/active/memory-stage-5-release-assessment.md, doc:cmd/evie/docs/active/subagents.implementation.md, #176, #154, #156, #157, #158, #159, #160, #161, #162, #163, #164, #165, #166, #167, #168

Brings Memory Stage 5 (pull request #176, bounded recall and a failed release
assessment) into the subagents work. Ordinary messages now receive bounded
recall: accepted evidence with its original sources, scoped conversation
excerpts with safely expanded context, historical and conflicting evidence,
contextual references resolved to eligible identities, reusable turn
investigations, source-supported relationships, and a measured local
semantic retrieval, and each answer's original requests can be inspected.
The frozen held-out assessment then failed its release gates (1,051 of 1,077
passing), and the failure is recorded rather than tuned away.

The merge keeps delegated sessions out of memory: child sessions get no
automatic or model-directed retrieval, and conversation indexing excludes
delegated lineage, so assignments never become owner statements.

## 2026-09-11 — Bounded parallel research subagents
Branch: master
Commits: a24b45a..a24b45a
Touches: internal/subagents, internal/delegation, internal/eviedb, internal/plugins, internal/agent, internal/memory, internal/openrouter, internal/tools, cmd/evie, cmd/todo
Sources: doc:cmd/evie/docs/active/subagents.spec.md, doc:cmd/evie/docs/active/subagents.decisions.md, doc:cmd/evie/docs/active/subagents.implementation.md, #155, #169, #170, #171, #172, #173, #174, #175

The foundation of subagents: an optional Subagents plugin gives the chat
`delegate_research`, whose assignments run in parallel as research workers
limited to web search and fetch, each with its own history and trusted
worker instructions. Each assignment is one durable attempt per parent and
key, admitted only for the committed invocation, fenced together with the
parent, and recovered without being run again. The memory compiler rejects
delegated lineage, so a child's text never becomes owner evidence.

## 2026-09-10 — GPT-6 Astra, the activity timeline, and file inspection
Branch: master
Commits: 4098721..f27546d
Touches: internal/openrouter, internal/agent, internal/eviedb, internal/memory, internal/web, internal/web/ui, cmd/evie, cmd/todo
Sources: doc:cmd/evie/docs/active/gpt-6-astra.spec.md, doc:cmd/evie/docs/active/gpt-6-astra.decisions.md, doc:docs/gpt-6-astra-migration-plan.md, doc:cmd/evie/docs/active/chat-activity.spec.md, adr:0066

Evie's default model becomes `openai/gpt-6-astra`, through OpenRouter's
stateless Responses endpoint, keeping OpenRouter and preserving response
phases and compaction boundaries; Kimi stays available through Chat
Completions. The chat replaces raw tool cards with a collapsible activity
timeline, file actions open in a side Inspector with highlighted source and
side-by-side changes, and the file viewer and Workspace navigation are
simplified. The range also adds the memory Stage 5 retrieval and subagents
specifications and an agent orchestration research draft.

## 2026-09-06 — Usage, restored conversation history, and the architecture record
Branch: master
Commits: 7323826..7323826
Touches: internal/usage, internal/eviedb, internal/memory, internal/web, internal/web/ui, internal/plugins, internal/agent, internal/localextractor, cmd/evie, docs/adr, docs/architecture, CONTEXT.md, AGENTS.md
Sources: doc:cmd/evie/docs/active/usage.spec.md, doc:cmd/evie/docs/active/session-history-and-memory-scope.spec.md, doc:cmd/evie/docs/active/workspaces.spec.md, doc:cmd/evie/docs/active/workflow-runtime.spec.md, adr:0022, adr:0047

A large snapshot commit. Data gains Usage, showing Codex account activity and
Evie's recorded conversation token usage with explicit coverage. Reopening a
saved session restores its messages and recorded tool outcomes, and Evie
recommends where a memory applies (everywhere, this Workspace, or this
conversation), resolved by the harness and confirmed on approval.

It also adds the `CONTEXT.md` glossary, 50 architecture decision records,
specifications for Workspaces, the Workflow Runtime and semantic memory
Stages 3 and 4, memory architecture diagrams and research notes, and commits
local run output under `.scratch/` and `tmp/`.

## 2026-09-05 — Workspace shell and the Data hub
Branch: master
Commits: 2aad5a7..2aad5a7
Touches: internal/web, internal/web/ui, internal/eviedb, internal/memory, cmd/evie
Sources: doc:cmd/evie/docs/active/ui-chat.spec.md, doc:cmd/evie/docs/active/serve.decisions.md

The browser becomes a scope-aware three-pane workbench: left navigation for
New chat, Data and Workspaces, center tabs for chat and Workspace pages, and
one collapsible right Inspector. Data is a typed hub whose first sources are
Database, with live schema topology and records for an allowlist of tables,
and Memory, a graph of current Claims and Entities in one exact scope and
pinned revision.

## 2026-09-05 — Web chooser no longer crashes on empty lists
Branch: codex/memory-stage-4
Merge: 8d45aa0
Touches: internal/web
Sources: code:internal/web/context_sessions.go, #153

The context-session list route returned `null` for an empty list of
Workspaces, projects or sessions, which crashed the web chooser at startup.
It now returns empty arrays, and a test holds the route to that.

## 2026-09-05 — Memory Stage 4: durable memory compilation and owner review
Branch: codex/memory-stage-4
Merge: fac58ff
Touches: internal/eviedb, internal/memory, internal/localextractor, internal/memoryeval, internal/agent, internal/tools, internal/web, internal/web/ui, cmd/evie, scripts
Sources: doc:cmd/evie/docs/active/semantic-memory-stage-4.spec.md, doc:cmd/evie/docs/active/memory.spec.md#stage-4---durable-asynchronous-memory-compiler, doc:cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md, doc:cmd/evie/docs/active/memory-stage-4-work-contract.decisions.md, doc:cmd/evie/docs/active/memory-stage-4-review-contract.decisions.md, adr:0062, adr:0063, adr:0064, adr:0065, #152, #132, #133, #134, #135, #136, #137, #138, #139, #140, #141, #142, #143, #144, #145, #146, #147, #148, #149, #150, #151

Adds the durable memory compiler. After defining Stage 4's evidence, durable
work and owner review contracts, the branch persists bounded owner-assertion
candidates, recovers fenced compiler work across processes, and activates
background compilation at explicit frontiers, with bounded historical
backfill and coverage inspection.

The owner reviews candidates in a web inbox independently of their source
sessions: simple candidates, sourced identities and Predicate definitions,
temporal changes, local clock observations, and edited interpretations with
dependent batches, and review decisions are preserved across compiler
generations. Compiler health and measured progress are exposed, and tooling
measures the local extractor, proves deterministic conformance, measures the
workload, and assesses release evidence.

## 2026-09-04 — Todo and YouTube plugins, and durable Task Trees
Branch: codex/todo-youtube-task-trees
Merge: 9ef0771
Touches: internal/task, internal/taskeval, internal/plugins, internal/eviedb, internal/agent, internal/tools, internal/todo, internal/openrouter, internal/web, cmd/todo, cmd/evie
Sources: term:Task Tree, adr:0004, #130, #117, #118, #119, #120, #121, #122, #123, #124, #125, #126, #127, #128, #129

YouTube and Todo become first-party plugins, and Todo gains durable Global
Tasks with a retained lifecycle, idempotent mutations, and recursive Task
Trees. Work on a Task is claimed by its active execution, the legacy Todo
list is imported exactly once, Task Trees are scoped and focused, and
delegated sessions can be granted a Task subtree. The `todo` CLI moves onto
the shared Task store, and Evie can focus and delegate Task Trees.

## 2026-09-02 — Memory Stage 3: deterministic semantic memory
Branch: codex/memory-stage-3
Merge: 5e5e43d
Touches: internal/eviedb, internal/memory, internal/memoryeval, internal/agent, internal/plugins, internal/tools, internal/web, internal/web/ui, cmd/evie, scripts
Sources: doc:cmd/evie/docs/active/semantic-memory-stage-3.spec.md, doc:cmd/evie/docs/active/semantic-memory-encodings.spec.md, doc:cmd/evie/docs/active/memory.spec.md#stage-3---temporal-graph-domain-and-explicit-memory, adr:0047, adr:0052, adr:0056, #115, #83, #85, #103, #104, #105, #106, #107, #108, #109, #110, #111, #112, #113, #114

Implements deterministic semantic memory on frozen encodings. Evie remembers
one sourced literal Claim at a time, creates Entities and resolves aliases,
enforces exact Claim semantics, corrects Claims across time, and retires and
restores them through a reversible lifecycle, with semantic scope isolation
and exact neighborhood links. Semantic projections can be recovered, focused
memory tools and owner inspection surfaces expose the state, and evaluation
baselines and an acceptance test prove the stage. The branch also adds
creating and entering Workspace sessions.

## 2026-09-01 — Plugin system Phase 1
Branch: codex/plugin-system-phase-1-core
Merge: 751e19e
Touches: internal/plugins, internal/composition, internal/eviedb, internal/tools, internal/agent, internal/web, internal/web/ui, cmd/evie, docs/adr
Sources: doc:cmd/evie/docs/active/plugins.spec.md, adr:0001, adr:0004, adr:0006, adr:0013, adr:0014, #101, #70, #74, #75, #76, #77, #78, #79, #80

Specifies the plugin system's first phase and records its decisions as the
first architecture decision records. Session toolsets are pinned; Web and
Finance load as compiled first-party plugins with a reversible lifecycle;
sessions are composed from a standard preset and keep a composition receipt;
and compatible session resumes are audited. The owner manages plugins and
built-in presets from a management page.

## 2026-08-30 — GitHub issue tracker configured
Branch: master
Commits: 9740c71..9740c71
Touches: docs/agents
Sources: doc:docs/agents/issue-tracker.md

Adds `docs/agents/issue-tracker.md`: issues for this repository live on
GitHub and are handled with the `gh` CLI.

## 2026-08-30 — Memory Stage 2: bounded context and durable compaction
Branch: codex/stage-2-context-management
Merge: d5c1f1a
Touches: internal/agent, internal/eviedb, internal/memory, internal/openrouter, internal/web, cmd/evie
Sources: doc:cmd/evie/docs/active/memory.spec.md#stage-2---working-context-and-compaction, #69, #64, #65, #66, #67, #68

Separates durable history from what the model sees. Each request's
conversational context is composed within a bound, tool results enter it
within their own bound, and eligible turns compact into a durable rolling
summary: manually first, then automatically before a request, with
compaction chains that resume after a restart.

## 2026-08-30 — Route-safe context profile
Branch: master
Commits: ae4cda3..02a7545
Touches: internal/openrouter, internal/agent, internal/web, cmd/evie
Sources: doc:cmd/evie/docs/active/memory.decisions.md, #63

Each process resolves one immutable context profile before opening or
resuming a session, and the profile bounds every conversational request.
Without an explicit window override, startup looks up the model and its
endpoints on OpenRouter and takes the smallest context length among active
endpoints that can honor the output reserve as the route-safe window.
Malformed metadata fails discovery, and other discovery failures fall back
to a built-in window only for Evie's built-in model.

## 2026-08-25 — Delivery loop taken out of AGENTS.md
Branch: master
Commits: 0be3ba0..0be3ba0
Touches: AGENTS.md, docs, cmd/evie/docs
Sources: doc:AGENTS.md

Removes the delivery loop the same day it arrived: its section in
`AGENTS.md` and `docs/delivery-loop.md` go, and the backlog and research
notes stop referring to it.

## 2026-08-25 — Simplified delivery workflow
Branch: codex/simplify-delivery-loop
Merge: edee820
Touches: AGENTS.md, scripts, .github/workflows, .agents/skills, docs
Sources: doc:scripts/verify-change.sh, doc:.github/workflows/verify.yml

Replaces most of the custom story skills, reviewer agents and planning
documents with a lighter loop: a concise `AGENTS.md`, `docs/delivery-loop.md`,
a pull request template, a GitHub Actions Verify workflow, and
`scripts/verify-change.sh` as the deterministic check.

## 2026-08-25 — Memory Epic 1 complete
Branch: master
Commits: af74b05..af74b05
Touches: cmd/evie/docs, docs
Sources: doc:cmd/evie/docs/active/memory/epic_1/README.md

Records Epic 1, restart-safe leased REPL sessions, as completed on
2026-08-25, updating its stories, the memory spec and decisions, and the
backlog.

## 2026-08-25 — Provider token usage evidence
Branch: codex/61-persist-provider-usage
Merge: dcbbe57
Touches: internal/agent, internal/openrouter, internal/memory, internal/eviedb
Sources: doc:cmd/evie/docs/active/memory/epic_1/story_4/README.md, #62, #61

Epic 1 story 4: usage reported by the provider is persisted as immutable,
provider-neutral evidence on each accepted assistant message, for later
context and evaluation diagnostics. Follow-up fixes clarify usage boundaries,
preserve negative zero, and normalize authoritative absence.

## 2026-08-24 — Select and resume titled REPL sessions
Branch: codex/59-select-resume-titled-repl-sessions
Merge: 5593518
Touches: cmd/evie, internal/agent, internal/eviedb, internal/memory, internal/web
Sources: doc:cmd/evie/docs/active/memory/epic_1/story_3/README.md, #60, #59

Epic 1 story 3: after a restart the REPL can create or resume an active
global or project session from one combined list, with deterministic session
titles, immutable scope and durable turn ownership preserved. Follow-up fixes
fence stale chooser state and harden its recovery.

## 2026-08-24 — Lease-own and fence every live turn
Branch: codex/57-lease-own-fence-live-turn
Merge: 02360c5
Touches: internal/agent, internal/eviedb, internal/memory, internal/openrouter, internal/tools, internal/web, internal/web/ui, cmd/evie
Sources: doc:cmd/evie/docs/active/memory/epic_1/story_2/README.md, #58, #57

Epic 1 story 2: a durable session lease owns every live agent turn from its
first event through provider and tool work, with SQLite fencing at every
accepted write. Review rounds closed terminal-boundary races, made turn
boundaries cancellation-safe, gated approvals and isolated provider
transport, and preserved the turn stage and committed output.

## 2026-08-23 — Lease-owned turns story readied
Branch: master
Commits: 02d5cc8..02d5cc8
Touches: cmd/evie/docs
Sources: doc:cmd/evie/docs/active/memory/epic_1/story_2/README.md, doc:cmd/evie/docs/active/memory.decisions.md

Readies Epic 1 story 2, lease-owned turns, before implementation, recording
its decisions in the memory decisions and amending the memory, serve and chat
UI specs.

## 2026-08-23 — Cancellable tool lifecycle
Branch: codex/55-cancellable-tool-lifecycle
Merge: d48c0ac
Touches: internal/tools, internal/agent, internal/eviedb, internal/finance, internal/youtube, internal/web, cmd/evie, cmd/finance
Sources: doc:cmd/evie/docs/active/memory/epic_1/story_1/README.md, #56, #55

Epic 1 story 1: the turn's caller-owned context flows through tool
preparation, approval and execution, so a cancellation seen at a lifecycle
boundary stops later turn-owned work and aborts the turn. Follow-ups closed
cancellation races, kept the cron tool's ordinary cleanup, and filled
recovery decision gaps.

## 2026-08-23 — Story delivery skills and the Epic 1 plan
Branch: master
Commits: dc76f99..adbb2d8
Touches: .agents/skills, .codex/agents, .opencode/agents, tools/story-loop, cmd/evie/docs, docs
Sources: doc:cmd/evie/docs/active/memory/epic_1/README.md

Reworks the agent skills for story work: a review-story skill with contract
and maintainability reviewers, then a deliver-story skill with result
schemas, while the `tools/story-loop` controller is removed. The memory plan
is rewritten around Epic 1, restart-safe leased sessions, with a README per
story.

## 2026-08-22 — Reviewed story repair loop
Branch: codex/devx-1-story-loop
Merge: 51a0387
Touches: tools/story-loop, .gitignore
Sources: #54

Adds `tools/story-loop`, a Python controller for one approved story. It
starts an implementation task in an isolated worktree, reviews the exact
commit in a fresh read-only task, runs deterministic checks outside both, and
resumes the implementation with the findings until the candidate is ready or
a bounded stop condition fires.

## 2026-08-22 — Durable turn-lease storage
Branch: codex/3-mem-1-2-durable-turn-lease-storage
Merge: e57868f
Touches: internal/eviedb, internal/memory
Sources: doc:cmd/evie/docs/active/memory/epic_1/README.md, #52

MEM-1.2 adds durable turn-lease storage in SQLite: acquiring, renewing,
releasing and expiring a session's turn lease, with fencing whose epoch
invariants are pinned by tests.

## 2026-08-21 — Explicit REPL scope selection
Branch: codex/2-mem-1-1-explicit-repl-scope-selection
Merge: 3b7d96e
Touches: cmd/evie, internal/eviedb
Sources: doc:cmd/evie/docs/active/memory/epic_1/README.md, #51

MEM-1.1: the REPL selects a global or project scope explicitly and confirms
before creating a new session in it.

## 2026-08-21 — Durable sessions, AGENTS.md, and the memory epics
Branch: master
Commits: 4b1f113..b5e9d0e
Touches: internal/agent, internal/eviedb, internal/memory, internal/tools, internal/openrouter, internal/web, cmd/evie, AGENTS.md, .agents/skills, cmd/evie/docs, docs
Sources: doc:cmd/evie/docs/active/memory.spec.md#stage-1---session-identity-and-append-only-events, doc:AGENTS.md

Conversations become durable: Evie's database gains projects, sessions and
an append-only event log, and the agent rebuilds its history from the stored
events. `query_db` can read the evie database, fenced to the cron tables.
`AGENTS.md` arrives, with the workflow for implementing stories, and the
memory plan is broken into nine epics with stories, planning and
implementation skills, and execution contracts.

## 2026-08-17 — Backlog research and a revised memory spec
Branch: master
Commits: 362943a..9a6faa6
Touches: cmd/evie/docs, docs, CLAUDE.md
Sources: doc:cmd/evie/docs/research/opencode-backlog/README.md, doc:cmd/evie/docs/active/memory.spec.md, doc:cmd/evie/docs/active/memory.decisions.md

Adds a feature backlog researched from opencode's agents and features, and
substantially revises the memory spec and its decisions. `CLAUDE.md` is
deleted.

## 2026-08-13 — YouTube transcript library
Branch: master
Commits: 7e206b7..7e206b7
Touches: internal/youtube, internal/tools, cmd/ytscribe
Sources: doc:cmd/evie/docs/done/youtube-transcripts.spec.md, doc:cmd/evie/docs/done/youtube-transcripts.decisions.md

Gives Evie a durable, searchable YouTube transcript library in
`~/.evie/transcripts/transcripts.db`. Two ungated tools fetch and cache one
public video's transcript (`youtube_transcript`) and refresh a channel's
newest videos (`youtube_scrape_channel`), and the same package backs a
`ytscribe` CLI for full channel crawls and for importing an existing scraper
corpus. `query_db` can also read the transcripts database.

## 2026-08-12 — System prompt and memory research
Branch: master
Commits: 31439fe..31439fe
Touches: internal/agent, cmd/evie/docs
Sources: doc:cmd/evie/docs/active/memory.spec.md, doc:cmd/evie/docs/active/memory.decisions.md

Gives Evie a stable system prompt covering identity, task ownership, trust
and approval, tool use and communication, and drafts a local,
Letta-inspired memory spec with its decisions.

## 2026-08-09 — Evie: one agent loop, evie serve, reasoning, cron, and web tools
Branch: master
Commits: 19f5904..07344e3
Touches: internal/agent, internal/web, internal/web/ui, internal/tools, internal/eviedb, internal/openrouter, internal/finance, cmd/evie, cmd/moussa
Sources: doc:cmd/evie/docs/active/serve.spec.md, doc:cmd/evie/docs/done/agent-extract.spec.md, doc:cmd/evie/docs/done/serve-core.spec.md, doc:cmd/evie/docs/done/reasoning.spec.md, doc:cmd/evie/docs/done/cron.spec.md, doc:cmd/evie/docs/done/web-fetch.spec.md, doc:cmd/evie/docs/done/web-search.spec.md, doc:cmd/evie/docs/active/ui-chat.spec.md

The harness becomes Evie: `cmd/moussa` becomes `cmd/evie`, in the module
`github.com/davidadel66/evie`. The agent loop moves into `internal/agent`
behind an events interface, so the terminal REPL and the new `evie serve`
web UI are two faces of one loop; `internal/web` streams turns over SSE and
round-trips approvals, and a React chat shows streamed reasoning in a
collapsible block.

New tools: `cron_add`, `cron_list` and `cron_remove` schedule shell commands
that launchd fires, stored in `~/.evie/evie.db`; `web_fetch` returns a page
as lightweight Markdown; and `web_search` returns raw Brave Search results.
The web chat is then polished, with a text-size control and code themes, and
file approvals show full diffs.

## 2026-08-01 — File tools and bash
Branch: master
Commits: 252cb4d..b29b53c
Touches: internal/tools, cmd/moussa, docs
Sources: doc:cmd/evie/docs/done/file-tools.spec.md, doc:cmd/evie/docs/done/file-tools.decisions.md, doc:cmd/evie/docs/done/bash.decisions.md

`read_file` returns numbered lines capped at 100 KB, fenced by a path
denylist inside path resolution, and `edit_file` makes exact-string
replacements, approval-gated and written atomically. `bash` runs a real
login shell with no approval gate or denylist, since a shell walks around any
fence: the working directory persists between calls, interactive aliases come
from a snapshot taken at startup, and its timeouts and 30k output cap follow
Claude Code's BashTool. `docs/BACKLOG.md` and `docs/decisions.md` start here,
through `/wrap`.

## 2026-07-29 — Streamed replies
Branch: master
Commits: 5909731..5c83ef1
Touches: internal/openrouter, cmd/moussa, internal/tools
Sources: pkg:internal/openrouter

Replies stream from OpenRouter over SSE and print to the terminal at a smooth
pace. The database tool descriptions now say recurring budget limits always
go in the standing template, with a monthly row only for a one-month
exception.

## 2026-07-28 — Budget tables and general database tools
Branch: master
Commits: aa87fb5..c85bd6d
Touches: internal/finance, internal/tools, cmd/moussa, cmd/finance/docs
Sources: doc:cmd/finance/docs/active/budget.spec.md

Specifies a monthly budget with per-category limits, kept current by
scheduled sync and rule categorization, with uncertain transactions reviewed
in chat. The agent first gets `finance_query` for read-only SQL, then general
`query_db` and `edit_db` tools that replace it, with writes behind an
approval gate. `budget_entries` and `budget_limits` tables follow, and
categorize now writes a budget entry per transaction instead of the legacy
category column.

## 2026-07-16 — Domain packages and the agent's finance tools
Branch: master
Commits: cdcb0f7..0175449
Touches: internal/finance, internal/todo, internal/tools, internal/openrouter, cmd/agent, cmd/moussa, cmd/finance, cmd/todo
Sources: pkg:internal/finance, pkg:internal/tools

The finance and todo domains move out of their commands into
`internal/finance`, split into client, link and sync, and `internal/todo`,
with every package and function documented and environment loading kept out
of domain code. The merchant lookup file moves to `~/.finance`. Agent tools
move into `internal/tools` and gain `finance_sync`, `finance_rules` and
`finance_categorize`, and `cmd/agent` is renamed `cmd/moussa`.

## 2026-07-14 — First commit
Branch: master
Commits: ef694f5..ef694f5
Touches: cmd/todo, cmd/finance, cmd/agent, internal/openrouter
Sources: doc:cmd/finance/docs/done/sync.spec.md, doc:cmd/finance/docs/active/categorize.spec.md

The repository starts as a personal collection of Go command-line tools,
built partly to learn Go: `todo`, a task manager with JSON persistence;
`finance`, which links banks through Plaid, syncs transactions into SQLite
and categorizes them by merchant rules; and `agent`, a terminal chat loop
through OpenRouter with time and todo tools.
