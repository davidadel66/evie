# changes — decisions

Spec: [changes.spec.md](changes.spec.md). Decided 2026-10-09 with the owner
in a grilling session (Q1–Q26), starting from the claude.ai/design template
`templates/workspace-activity/WorkspaceActivity.dc.html`.

- **None or one Repository per Workspace** (Q1). Changes, checks and
  overlaps each belong to one codebase, and the design's header shows one
  Change list. Two repositories mean two Workspaces. Rejected: several
  repositories per Workspace.
- **Four choices at creation, Publish later** (Q2, Q11). None, new local,
  new GitHub (private preselected), connect existing. Publish in Settings
  is a one-way upgrade from local that pushes only the base branch.
  Personal account only, no organizations. Connect existing was added
  because Evie itself already lives on GitHub. None was added for
  non-code Workspaces such as finance.
- **Fine-grained token from `GITHUB_TOKEN`, under Connection ID `github`**
  (Q3). This is the simplest option that can also create repositories.
  Evie checks the token at use. No secret store exists yet (the ADRs 0024,
  0028 and 0042 store is intent only), so a store later changes only
  startup composition. Rejected: a GitHub App (a bot identity, but more
  setup, and creating personal repositories needs user-to-server tokens).
  Also rejected: reusing the `gh` login (unbounded authority that Evie's
  code cannot limit).
- **A Change is one branch of work: draft → open → merged | closed** (Q4).
  Every open pull request on the connected repository is a Change,
  including the owner's, so the list shows all work in flight. Closed
  sits beside merged as an end state.
- **Local Repositories have Changes** (Q5). They are Evie's build branches
  under the existing merge gate (Verify, rebase, Verify, then fast-forward).
  Only pull requests and GitHub checks are missing. Rejected: no Changes
  until the repository is on GitHub.
- **Evie never merges without the owner's action; merges rebase and
  fast-forward** (Q6, Q19). "Merge when checks pass" counts as the owner's action,
  taken in advance and undoable until it fires. Merges use rebase and
  fast-forward everywhere: the local gate as today, and GitHub's "Rebase
  and merge". History then reads the same whichever side merged. In a
  GitHub Repository every merge goes through its pull request. Rejected:
  squash (it conflicts with the existing gate).
- **First spec covers repository, Change lifecycle with checks, review and
  merge** (Q7). These are moved to the coming software factory spec:
  line comments and revisions by Evie, overlap detection with automatic
  rebase, and `pr-review`.
- **"Change" keeps its name; the folder view's tab becomes "Working
  tree"** (Q8). "Change" fits local and GitHub Repositories alike, and
  "pull request" does not. Rejected: renaming the new concept, and folding
  the folder tab into the Changes tab.
- **The Repository is the Workspace folder** (Q9). A git folder is a local
  Repository, and a github.com `origin` makes it a GitHub Repository. Evie
  stores no separate record, so existing Workspaces need no migration. The
  folder cannot change while a Change is unfinished.
- **Connect existing clones or adopts** (Q10). Evie clones into a folder
  the owner picks, starting in `~/code`. It adopts an existing clone only
  if its `origin` matches.
- **Pushing is outward-facing; workers never see the token** (Q12, revised
  by Q25). Only Evie's GitHub side pushes. The worker sandbox keeps network
  access off.
- **Checks** (Q13, revised 2026-10-10). In a GitHub Repository, checks are
  the GitHub Actions workflow runs (newest per workflow) and commit
  statuses on the latest commit. GitHub's Checks API turned out to refuse
  fine-grained tokens, and there is no Checks permission to grant one. The
  owner kept the fine-grained token and added Actions read. Rejected: a
  classic `repo` token (sees every check, but reaches every repository) and
  a GitHub App. Services that report only as check runs are not seen. In a local Repository, the check is
  the Workspace's Verify command. Evie runs Verify before pushing. The
  owner's push works after a failure and shows it. "Checks pass" means
  every check finished and none failed. No checks means plain "Approve and
  merge".
- **Which branches are Changes** (Q14). In GitHub Repositories: open pull
  requests plus Evie's unpushed build branches. In local Repositories:
  only Evie's build branches. The owner can merge their own pull requests
  from Evie. Lists keep the 20 most recent merged or closed.
- **After a merge** (Q15). Evie deletes its branch on GitHub and locally
  and removes the worktree. It fast-forwards the owner's base branch only
  when that branch is checked out with no uncommitted edits. Otherwise it
  shows "N commits behind".
- **Close** (Q16) asks for confirmation. It closes the pull request and
  removes Evie's branches. It never deletes the owner's.
- **Poll GitHub every 60 seconds while `evie serve` runs, and again when
  the Changes tab opens** (Q17). Webhooks cannot reach a local machine. The
  load is far under GitHub's 5,000 requests an hour.
- **Conflicts show "Needs rebase" and disable merge** (Q18). Resolving is
  the owner's job until the software factory spec gives it to Evie. A
  conflict resolution the owner pushes is picked up.
- **Evie's notes come from the build report and go into the pull request
  description** (Q21). For the owner's pull requests, the notes are the
  pull request body.
- **No token means local only** (Q22). The GitHub creation options are
  greyed out with a "how to connect" note. Settings reports a missing,
  invalid or refused token. The Changes tab is hidden without a Repository.
- **"Viewed" is saved per browser** (Q23), like the folder view, and
  resets when a file changes in a new commit.
- **Six stories in order** (Q24). Repository detection and Settings;
  creation and Publish; Change list and sync; push and open PR; merge and
  close; Changes tab, review page and chat Inspector. Stories 1–5 are
  mostly backend. Story 6 waits for the workspace-activity header.
- **Approval lives only in the Changes tab, never in chat** (Q20, Q25). The
  owner does not want Change or pull request cards in chat. A chat build
  row opens the Inspector with the diffs and an "Open in Changes" button.
  Chat builds end as drafts, and chat never pushes or merges. Workflows
  still push under their approved Standing Authority. `propose_build_merge`
  still runs the gate but no longer asks in chat.
- **The chat merge card stays until the Changes tab ships** (Q26), so the
  owner always has somewhere to approve a local merge. Stories 3–5 add
  `evie changes list|push|merge|close` so each can be demonstrated in a
  terminal. Story 6 removes the card. In a GitHub Repository, the card
  goes away in story 4, because a local fast-forward there would break
  Q19.
