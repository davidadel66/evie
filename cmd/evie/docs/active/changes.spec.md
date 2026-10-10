# changes — spec

Decided 2026-10-09 with the owner in a grilling session; binding record in
[changes.decisions.md](changes.decisions.md). **Repository** and **Change**
are defined in the root `CONTEXT.md`. The UI follows the claude.ai/design
template `templates/workspace-activity/WorkspaceActivity.dc.html` (Changes
tab and review page only; see Out of Scope).

## Problem Statement

The owner develops Evie on GitHub and wants other projects there too.
Evie already writes code: chat build workers and workflow build workers
commit on `evie/…` branches in worktrees, and a merge gate (verify, rebase,
verify) fast-forwards the owner's local branch once the owner approves a card in
chat. Nothing reaches GitHub. There is no push, no pull request, no checks,
and no place outside a chat card to see what Evie changed or to approve it.
A Workspace does not know whether it has a repository; creating one or
connecting an existing GitHub repository happens outside Evie. The owner's
own pull requests are invisible to Evie, and approving a merge means finding
the card in whichever chat proposed it.

## Solution

A Workspace can own one Repository, kept only on the owner's machine or also
on GitHub. The owner chooses it when creating the Workspace (no repository, new
local, new GitHub public or private, or connect an existing GitHub
repository) and can publish a local one to GitHub later from Settings. Every
branch of work in it is a Change, listed in the Workspace's Changes tab from
draft through review to merged or closed, with its checks. The owner reviews diffs
and approves in the Changes tab only; a chat shows Evie's diff in the
Inspector with a button to the Change. Evie pushes and opens pull requests
when the owner asks, or when an approved workflow is allowed to. Evie merges only
on the owner's action, by rebase and fast-forward, cleans up its own branches, and
keeps the owner's checkout current without touching work in progress.

## User Stories

### Repository

1. As the owner, I want to choose "No repository" when creating a Workspace, so that areas like finance carry no codebase.
2. As the owner, I want to choose "New local repository" when creating a Workspace, so that I can start a project without GitHub.
3. As the owner, I want to choose "New GitHub repository" with public or private (private preselected), so that a new project starts on my GitHub account.
4. As the owner, I want to choose "Connect existing GitHub repository" from a list of my repositories, so that projects like Evie join without manual setup.
5. As the owner, I want Evie to clone the chosen repository into a folder I pick, starting in `~/code`, so that I decide where code lives.
6. As the owner, I want to point at a clone I already have and have Evie check that its `origin` matches the chosen repository, so that I never connect the wrong folder.
7. As the owner, I want any Workspace whose folder is a GitHub clone, such as one on `~/code/evie`, recognized as a GitHub Repository from its folder and `origin`, so that nothing needs migrating.
8. As the owner, I want Settings → Repository to show whether the Repository is local or on GitHub, its remote, and the token status, so that I know what Evie can do.
9. As the owner, I want to publish a local Repository to GitHub later from Settings, choosing public or private, so that starting local is not a dead end.
10. As the owner, I want publishing to push only the default branch and leave unmerged Changes as drafts, so that nothing half-finished goes public.
11. As the owner, I want the GitHub creation options greyed out with a one-line "how to connect" note when no valid token is set, so that I know why they are unavailable.
12. As the owner, I want Settings to say when the token is missing, invalid or refused an action, naming the refused action, so that I can fix the token's permissions.
13. As the owner, I want local Repositories to keep working with no token, so that GitHub is optional.
14. As the owner, I want Evie to refuse changing or detaching the Workspace folder while a Change is unfinished, so that drafts and open Changes are never orphaned.
15. As the owner, I want the GitHub token never visible to workers and never written to git config, remote URLs, logs or receipts, so that delegated code cannot use or leak it.

### Changes

16. As the owner, I want every chat build to end as a draft Change, so that Evie's work is visible in one place.
17. As the owner, I want a workflow build worker's branch to become a draft Change linked to its run, so that I can see which run produced it.
18. As the owner, I want every open pull request on the connected GitHub repository listed as a Change, including mine, so that I see all work in flight.
19. As the owner, I want Evie's unpushed build branches listed as drafts, so that nothing Evie made is hidden.
20. As the owner, I want a local Repository to list only Evie's build branches, so that my own experiments do not become Changes.
21. As the owner, I want Changes grouped as Needs your review, In progress, and Merged and closed (the 20 most recent), so that what needs me is on top.
22. As the owner, I want each Change to show its title, pull request number or "Draft", branch, origin (workflow, chat or me), checks summary, lines added and removed, and when it was updated, so that I can triage without opening it.
23. As the owner, I want a GitHub Repository's checks to be GitHub's check runs and commit statuses on the Change's latest commit, so that CI is the source of truth.
24. As the owner, I want a local Repository's checks to be the Workspace's Verify command, so that local work is verified the same way it is today.
25. As the owner, I want a Repository with no checks to show "None" and offer plain "Approve and merge", so that a project without CI still works.
26. As the owner, I want GitHub merges and closes done outside Evie to show up within a minute, so that the list stays true.
27. As the owner, I want the list to refresh when I open the Changes tab, so that I never act on stale state.
28. As the owner, I want the Changes tab hidden for Workspaces without a Repository, so that non-code Workspaces stay uncluttered.

### Push and pull requests

29. As the owner, I want a "Push and open PR" action on a draft in a GitHub Repository, so that I decide when work becomes public.
30. As the owner, I want Evie to run Verify before it pushes, when a Verify command is set, so that CI is not spent on a broken branch.
31. As the owner, I want to push even when Verify failed, with the failure shown on the Change, so that I stay in control.
32. As the owner, I want Evie's pull request description to be Evie's notes from its build report, so that the reasoning is visible on GitHub too.
33. As the owner, I want a workflow to open pull requests only through the `github` capability under authority I approved and Workspace Access I granted, so that automation stays within bounds.
34. As the owner, I want chat never to push or open a pull request, so that every outward action from chat goes through the Changes tab.

### Merge and close

35. As the owner, I want "Approve and merge" to rebase and fast-forward, locally or with GitHub's "Rebase and merge", so that history stays linear and identical whichever side merged.
36. As the owner, I want every merge in a GitHub Repository to go through its pull request, so that my local default branch never gets ahead of GitHub.
37. As the owner, I want "Merge when checks pass", undoable until it fires, so that I can approve early and walk away.
38. As the owner, I want a failing check to cancel an armed merge, so that a red Change is never merged by a decision I made while it was green.
39. As the owner, I want a Change whose rebase conflicts to show "Needs rebase" with merge disabled, so that I know to resolve it.
40. As the owner, I want a Change to pick up my own pushed conflict resolution, so that I can fix conflicts by hand.
41. As the owner, I want to merge my own pull requests from Evie, so that I have one place to merge.
42. As the owner, I want a pull request marked draft on GitHub to say so and stay unmergeable until it is marked ready, so that Evie follows GitHub's own rule.
43. As the owner, I want Evie to delete its own branch on GitHub and locally and remove the worktree after a merge, including a merge done on GitHub, so that nothing piles up.
44. As the owner, I want Evie never to delete my branches, so that my work is never lost.
45. As the owner, I want my local default branch fast-forwarded after a merge when it is checked out with no uncommitted edits, and otherwise a "N commits behind" note, so that Evie never touches a checkout I am working in.
46. As the owner, I want to close a Change after a confirmation, closing its pull request and removing Evie's branch and worktree, so that abandoned work is cleaned up.
47. As the owner, I want Evie never to merge without my action, so that nothing lands that I did not approve.

### Review and chat

48. As the owner, I want a review page with Evie's notes, a file list with "Viewed" checkboxes, checks, where the Change came from, and its worktree, so that I can review in one place.
49. As the owner, I want side-by-side and unified diff views, so that I can read changes the way that suits each file.
50. As the owner, I want "Viewed" saved per browser and reset when a file changes in a new commit, so that I re-read only what changed.
51. As the owner, I want "came from" links to the workflow run or chat that produced the Change, so that I can trace why it exists.
52. As the owner, I want a chat build's row to open the Inspector with the changed files and diffs, with an "Open in Changes" button, so that I can see the work in context and approve in the tab.
53. As the owner, I want no Change or pull request card in chat, so that approvals live in one place.
54. As the owner, I want the folder view's existing "Changes" tab renamed "Working tree", so that "Change" means one thing.
55. As the owner, I want `evie changes list|push|merge|close` in the terminal, so that each story can be demonstrated and used without the UI.

## Implementation Decisions

### Repository

- The Repository is derived, not stored: it is the Workspace's current folder
  when that folder is a non-bare git work tree's top level. An `origin` URL
  naming `github.com/<owner>/<name>` (HTTPS, SSH or scp-like form) makes it a
  GitHub Repository; anything else (no `origin`, another host) is local.
- Base branch: a GitHub Repository uses GitHub's default branch. A local
  Repository uses the branch the build started from, as the merge gate does
  today (`main` for Repositories Evie creates).
- The folder write (compare-and-insert on the folder revision) also refuses
  a change or detach while the Workspace has a draft or open Change.
- Workspace registration gains a repository choice: none, new local, new
  GitHub (public or private), connect existing (clone into a chosen folder,
  or adopt an existing clone whose `origin` matches).
  - New local: create the folder (one directory, as today), `git init` with
    `main`, one empty first commit.
  - New GitHub: create the repository on the token owner's personal account,
    named after the folder, then clone it.
  - Publish (Settings): create the GitHub repository, set `origin` to its
    HTTPS URL, push the base branch only. Drafts stay drafts.
- Personal account only. No organizations, no visibility changes after
  creation.
- At creation (story 2b, 2026-10-09) the Repository is set up before the
  Workspace is registered, at the folder the setup returns, so a failure
  never leaves a Workspace pointing at a half-made folder. A new local
  Repository's folder is removed again if its first commit fails; a failed
  clone leaves no folder. If registration fails after the setup, the error
  names the ready folder. The dialog takes a typed folder path beside the
  macOS chooser, suggests `~/code/<name>` for a repository's folder, and the
  server expands `~/`. The first commit of a new local Repository uses the
  owner's git identity, or Evie's when none is configured.

### GitHub connection

- GitHub is an Account Connection with Connection ID `github`. For this spec
  the credential is a fine-grained personal access token read from
  `GITHUB_TOKEN` (the environment, or `~/.evie/.env`, which `evie serve`
  loads at startup) by startup composition and handed to the GitHub client.
  There is no secret store; moving the credential into one later changes
  only composition.
- Token permissions the feature uses: Metadata read, Contents read/write,
  Pull requests read/write, Actions read, Commit statuses read;
  Administration read/write only for creating or publishing repositories;
  Workflows read/write only for pushing edits to `.github/workflows`.
  GitHub's Checks API does not accept fine-grained tokens (found live on
  2026-10-10), so a GitHub Repository's checks are its GitHub Actions
  workflow runs (the newest run of each workflow) and its commit statuses.
  Third-party services that report only as check runs are not seen.
- The token check confirms the token authenticates and can read the
  Repository. A missing permission is reported when GitHub refuses an
  action, naming that action.
- The token reaches git only per command, for HTTPS push, fetch and clone
  run by the Kernel side. It is never placed in a remote URL, git config,
  a worktree, the worker sandbox, logs, receipts or tool output. Worker git
  keeps transport disabled and the sandbox keeps credential paths denied.
- The GitHub client takes its API base URL from configuration, so tests can
  point it at a fake server.

### Change model

- Changes are stored per Workspace in Evie's database: branch, pull request
  number once open, lifecycle state, author (Evie or owner), origin (chat
  session, workflow run and step, or none), title, notes, head commit, base
  branch, the last Verify result, a checks snapshot, an armed-merge marker
  (who and when), a needs-rebase marker, and timestamps. Merged and closed
  Changes are kept; lists show the 20 most recent.
- Lifecycle: `draft → open → merged | closed`, and `draft → closed`.
  - Open in a GitHub Repository: it has a pull request.
  - Open in a local Repository: it passed the merge gate and awaits review.
  - Needs rebase is a marker on a draft or open Change, not a state.
- Display labels are derived from the state, the checks and the markers:
  Draft, Checks running, Checks failing, Needs your review, Merges when
  checks pass, Needs rebase, Merged, Closed.
- Sources:
  - chat build workers, through `propose_build_merge`;
  - workflow build workers;
  - in GitHub Repositories, the owner's pull requests found by sync.
- Notes: Evie's come from the worker's build report; the owner's are the
  pull request body.
- Identity and linking (2026-10-10): Evie's Change is named by its branch
  (`build:<branch>`), so a continuation attempt on the same branch stays the
  same Change. A pull request on that branch is that Change on GitHub: it
  takes the pull request, its checks, head and GitHub's state, and keeps
  Evie's title, notes and origin. Of several pull requests on one branch,
  the open one, else the newest, is the branch's. Any other pull request
  is the owner's own Change, `pr:<number>`.
- Sync bounds (2026-10-10): one list plus two reads per open pull request
  (check runs and commit statuses) plus one per pull request that left the
  open list. A sync GitHub refuses records nothing, so a missing check is
  never read as "no checks". `mergeable` is unknown for listed pull
  requests; story 5 reads it at merge time.

### Sync

- While `evie serve` runs, each Workspace with a GitHub Repository is synced
  every 60 seconds, one sync at a time per Workspace. A sync lists open pull
  requests, refreshes each open Change's state, head commit, checks (check
  runs and combined status) and mergeability, and notices merges and closes
  done on GitHub.
- Opening the Changes tab requests an immediate sync.
- Sync is where an armed merge fires and where post-merge cleanup runs for
  merges done on GitHub.

### Push and pull requests

- A push sends Evie's branch, name unchanged (`evie/<id8>-<slug>`), to
  `origin` over HTTPS with the token.
- Before Evie pushes, it runs the Workspace's Verify command in the worktree,
  if one is set, with the existing gate command runner. A workflow push
  requires Verify to pass. The owner's push goes ahead regardless and records
  the result on the Change.
- The pull request targets the base branch, with the Change's title and
  notes. It is a ready pull request, not a GitHub draft.
- Story 4a (2026-10-10):
  - The base is the branch the Change was built from. A push is refused,
    naming that branch, when GitHub does not have it.
  - The pull request body is Evie's notes, then where the Change came from
    (a chat, or a workflow and step), then how Verify ended: "Verify
    passed" with the command, or "Verify failed … Pushed anyway by the
    owner" with the end of its output.
  - The title and notes are Evie's own (2026-10-10, after the live test
    put a worker's whole report in a pull request). `propose_build_merge`
    requires a one-line title of at most 72 characters and a description
    of at most 4096 bytes. The gate records them with the proposal. It
    refuses text that names the worktree, the repository folder or the
    owner's home folder. Chats pinned before this keep the old tool; their
    Changes fall back to the first commit's subject and the worker's
    summary.
  - Verify runs sandboxed without network in the build's worktree, with
    the gate's runner and its 20-minute limit.
  - Pushing again sends new commits and keeps the branch's open pull
    request. A sync runs straight after, so the Change shows its pull
    request at once.
  - In a GitHub Repository a build that passed the gate stays Draft until
    it has a pull request.
- Who may push and open a pull request:
  - the owner, from the Changes tab or `evie changes push`;
  - a workflow, through a `github.open_change` effect capability (connection
    `github`, resource `repo:<owner>/<name>`), idempotent by branch,
    reconciled by finding the pull request for the head branch, and bounded
    by Standing Authority and Workspace Access.

  Chat never pushes.
- `propose_build_merge` still runs the gate (Verify, rebase, Verify).
  - In a GitHub Repository it then records a draft Change and shows no
    chat card. Until story 6, its result tells Evie to give the owner the
    `evie changes push` command for the branch.
  - In a local Repository, until story 6, today's chat card stays and
    approving it is the same action as "Approve and merge". From story 6 it
    records an open Change and shows no card.

### Merge, close and cleanup

- Merging is the owner's action only (Changes tab, `evie changes merge`, or
  the local chat card until story 6). No workflow capability merges.
- Local merge reuses the gate: if the base moved since the gate last passed,
  rebase and Verify again, then fast-forward with compare-and-swap. A
  conflict sets needs-rebase.
- GitHub merge uses the merge API with the rebase method and the expected
  head commit. A conflict or not-mergeable answer sets needs-rebase. A
  pull request marked draft on GitHub, or a repository that disallows
  rebase merging, is refused with GitHub's reason shown.
- "Merge when checks pass" sets the armed marker. Sync merges once every
  check has completed and none failed (success, neutral or skipped). A
  failed check clears the marker. Undo clears it. Nothing relies on GitHub
  auto-merge or branch protection.
- After any merge, whether Evie's or one observed on GitHub:
  - Evie deletes its own branch on GitHub and locally and removes the
    worktree. The owner's branches are never deleted.
  - Evie then fetches. If the owner's checkout is on the base branch with
    no uncommitted edits, Evie fast-forwards it. Otherwise it records
    "N commits behind", shown in Settings → Repository and the Changes tab.
- Close asks for confirmation. On GitHub it closes the pull request. It
  removes Evie's branch and worktree, and never the owner's branch.

### Surfaces

- Web: owner-only management routes, like the workflow routes, to read the
  Repository, publish, list Changes, show one with its diff, push, merge,
  arm, disarm, close, and request a sync. Workspace registration accepts
  the repository choice.
- CLI: `evie changes list|push|merge|close`, using the same `changes` module
  as the web routes.
- Chat (story 6): a build row opens the Inspector with the branch's changed
  files and diffs and an "Open in Changes" button. There are no Change cards
  in chat.
- UI:
  - The Changes tab follows the design: groups, rows with checks dots and
    a +/− bar.
  - The review page follows the design: actions in the header, a file rail
    with "Viewed", checks, came from, worktree, Evie's notes, and
    side-by-side or unified diffs.
  - Diffs are computed from local git (fetching the pull request head for
    the owner's pull requests), reusing the folder view's git diff code.
- The folder view's "Changes" tab is renamed "Working tree".
- Settings → Repository sits with the other Workspace settings sections and
  moves into the Settings tab when the workspace-activity header lands.

### Module shape

- A new `changes` module owns Repository detection, creation and publishing,
  the Change store, sync, push, merge, close and cleanup. Web routes, the
  CLI, the subagents merge gate and the workflow capability call it.
- It owns the interface it consumes for GitHub. The production
  implementation is an HTTP client for GitHub's REST API.
- Git work reuses the `worktree` package (fast-forward, worktree removal)
  and adds network operations (fetch, push, clone) that only this module
  runs.

## Delivery Stories

Each story is independently demonstrable and ends with
`./scripts/verify-change.sh` passing.

1. **Repository detection and Settings → Repository.**
   - Outcome: every Workspace reports none, local or GitHub (owner/name,
     base branch) plus token status, in Settings → Repository and through
     its web route.
   - Acceptance:
     - a Workspace whose folder is `~/code/evie` reads as GitHub
       `davidadel66/evie` (no such Workspace existed on 2026-10-09; the owner
       creates one with that folder);
     - a folder without git reads as none;
     - a missing or invalid token is reported.
   - The folder guard moved to story 3 (2026-10-09): there are no stored
     Changes to check against until then.
   - Risk: parsing `origin` forms.
2. **Repository at creation and Publish to GitHub.** Split on 2026-10-09
   into 2a (Publish to GitHub from Settings, with the token-carrying git
   commands) and 2b (the four choices at creation), each demonstrated on
   its own, because together they cross GitHub writes, token transport and
   UI at once.
   - Outcome: the four creation options and Publish work end to end.
   - Acceptance:
     - each option leaves the expected folder, git state and GitHub
       repository;
     - GitHub options are greyed out without a token;
     - a clone with a mismatched `origin` is refused;
     - Publish pushes only the base branch.
   - Depends on 1.
   - Risk: repository creation is outward-facing, so demonstrate it on a
     throwaway repository.
3. **Change list and sync.** Split on 2026-10-10 into 3a (Evie's Changes,
   read from the build attempts that already record their branch, base,
   origin, gate and worktree removal, so nothing new is stored for them;
   the folder guard; `evie changes list`; the list route) and 3b (GitHub
   pull requests and checks, stored in a new table by a 60-second sync,
   and the refresh route), built in parallel on one shared `Change` type;
   linking a build branch to its pull request comes after both.
   - Outcome: `evie changes list` and the web route show Evie's build
     branches and, for GitHub Repositories, every open pull request with
     checks, refreshed every 60 seconds and on request.
   - Acceptance:
     - changing or detaching the folder is refused while a Change is
       unfinished (moved here from story 1);
     - chat and workflow build branches appear as drafts with their origin;
     - local Repositories hide the owner's branches;
     - merges and closes done on GitHub appear after a sync.
   - Depends on 1.
   - Risk: GitHub rate limits; the sync is bounded per Workspace.
4. **Push and open PR.** Split on 2026-10-10 into 4a (the owner's push:
   `evie changes push`, the push route, and no chat card in a GitHub
   Repository) and 4b (`github.open_change` for workflows).
   - Outcome: `evie changes push` and the web route push a draft and open
     its pull request; `github.open_change` does the same for workflows
     under authority.
   - Acceptance:
     - Verify runs first, the owner's push goes ahead after a failure, a
       workflow's push does not;
     - the pull request body is the notes;
     - the token never appears in git config, remote URLs or output;
     - in a GitHub Repository, `propose_build_merge` records a draft and
       shows no chat card.
   - Depends on 3.
   - Risk: token handling, so tests assert its absence everywhere.
5. **Merge and close.**
   - Outcome: `evie changes merge|close` and the web routes merge (local
     fast-forward or GitHub rebase merge), arm and disarm "Merge when
     checks pass", close, clean up, and bring the owner's base branch up
     to date.
   - Acceptance:
     - conflicts set needs-rebase;
     - an armed merge fires on green and clears on red;
     - the owner's branches survive cleanup;
     - a dirty checkout or one on another branch is never touched;
     - the local chat card and "Approve and merge" are one action.
   - Depends on 3; push-dependent cases depend on 4.
6. **Changes tab, review page and chat Inspector.**
   - Outcome: the Changes tab and review page per the design; the chat
     build row opens the Inspector with diffs and "Open in Changes"; the
     local chat merge card is removed; the folder tab is renamed
     "Working tree".
   - Acceptance:
     - every action from stories 4–5 is reachable in the tab;
     - "Viewed" persists per browser and resets on a file change;
     - the tab is hidden without a Repository.
   - Depends on 5 and on the workspace-activity header (separate spec).

## Testing Decisions

- Tests exercise external behavior through the highest seam: the `changes`
  module's public interface, given a Workspace and its folder, asserting
  what the owner would observe (Change list, states, git refs on disk, what
  GitHub received). They do not test internal helpers.
- The one new seam is a fake GitHub REST server. Its repository-creation
  answers give clone URLs that point at local bare repositories, so pushes,
  fetches and clones run for real on disk. It covers creating repositories,
  listing and reading pull requests, checks and statuses, merging, closing,
  and refusing for missing permissions.
- Git uses real repositories in temporary folders, as the `worktree` tests
  and the subagents merge-gate tests already do. The database is real
  SQLite in a temporary folder, as the `eviedb` tests use.
- The token is passed in directly. Tests assert that it never appears in git
  config, remote URLs, worktrees, command output or stored records.
- Thin surface tests follow existing prior art:
  - web routes like the workflow route tests in `internal/web`;
  - `evie changes` like the verify and worktrees command tests in
    `cmd/evie`;
  - `propose_build_merge` like the merge-gate tests in `internal/subagents`;
  - `github.open_change` like the finance workflow effect tests in
    `internal/plugins`.
- UI tests are component tests with the network faked at the API module,
  as in the existing workspace component tests. They cover creation options,
  Settings → Repository, the Changes tab, the review page and the chat
  Inspector button.

## Out of Scope

- Moved to the coming **software factory** spec:
  - line comments and "Request changes" that send Evie to revise;
  - overlap detection and automatic rebase between Changes;
  - the `pr-review` workflow.
- The rest of the workspace-activity design: the header, Runs and Workflows
  tabs, the run panel, and chat live pointers for runs. These have a
  separate spec.
- GitHub organizations, changing repository visibility, disconnecting a
  Repository from GitHub.
- A secret store or Account Connection registry.
- GitHub webhooks; Evie polls instead.
- More than one Repository per Workspace.
- Changes for the owner's own local branches.
- Squash or merge-commit merges.

## Further Notes

- Decided while writing this spec, not asked in the session. The owner
  should confirm or change them:
  - "Unfinished Change" in the folder guard includes drafts, because a
    draft's worktree lives inside the folder.
  - A failed check clears an armed "Merge when checks pass".
  - A local Change whose base moved since the gate passed is rebased and
    verified again at merge time.
  - A workflow push requires Verify to pass; only the owner's push goes
    ahead after a failure.
  - Merges done on GitHub trigger the same cleanup as Evie's merges.
  - The owner's pull requests marked draft on GitHub are listed but cannot
    be merged until marked ready.
- A new GitHub Repository is created with GitHub's initial commit, and a new
  local one with an empty first commit. Either way the base branch exists
  before the first build branches from it.
