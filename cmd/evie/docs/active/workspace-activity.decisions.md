# workspace-activity — decisions

Spec: [workspace-activity.spec.md](workspace-activity.spec.md). Decided
2026-10-09 with the owner, starting from the claude.ai/design template
`templates/workspace-activity/WorkspaceActivity.dc.html`.

- **Tabs are Runs, Workflows and Settings, with Changes added once it
  ships; there is no Chat tab.** Chat stays its own view. Rejected: chats
  living under a Chat tab in the Workspace header.
- **A Settings tab holds today's Workspace settings sections unchanged.**
  Rejected: an Overview tab keeping the current page, and a gear icon in
  the header.
- **Details open in the design's 440px panel inside the Workspace area.**
  The Inspector is only for chat. Rejected: reusing the Inspector for run
  details, and the design's full-page detail mode.
- **Workflows open in the design's wide pop-up** (W2), because YAML is
  cramped at 440px. Runs and Changes use the panel.
- **The default tab is Runs, or Changes when something needs review** (W1).
- **Notifications get an icon in the app's top bar on every view** (W3,
  W10, W11, revised by the owner). The owner rejected unread dots and a
  banner in the Runs tab, and rejected placing the icon inside the
  Workspace header. Notifications will carry more than Workspace notices
  later. So the icon belongs to the app, the list spans all Workspaces,
  and the notice shape is general. CONTEXT.md's Durable Notification was
  widened to cover notices from any part of Evie.
- **The top bar stays on every view, with the Workspace header under it**
  (W11). This follows from the icon's placement. The breadcrumb names the
  Workspace, so the header does not repeat the name. The Inspector toggle
  shows only in chat.
- **Badges count what needs the owner now; the icon counts unread
  notices** (W12). Both are kept. A waiting run counts in both until its
  notice is read.
- **Run steps follow the definition's YAML order** (W4): done, current, not
  reached, or skipped once the run finishes. Real workflows branch, so a
  fixed future path cannot be shown.
- **Only actions the backend supports** (W5). Cancel (optional reason) for
  unfinished runs. Rerun (required reason, the audited override) for
  succeeded runs. Nothing for failed runs. Start run lives in the workflow
  pop-up.
- **A fixed-choice question renders as buttons; anything else uses the
  existing form.** finance-review's answer is a JSON object with
  exceptions, so buttons cannot replace the form in general.
- **The approval banner expands the full existing approval card** (W6). The
  design's one-line banner would let the owner approve without seeing the
  code and permission differences, refusals and dry run. Pause or Resume,
  Deactivate and built-in workflows live in the Workflows tab.
- **The spend line reads "Workflows today: X of Y tokens · $Z"** (W7). The
  daily cap covers workflow runs only, and the label must not read as
  total spend.
- **A live run card in chat when Evie starts a run there** (W8). It opens
  the chat Inspector. Changes never get a card (changes.decisions.md).
- **A new read-only route for a workflow's source.** The Steps and YAML
  views need the definition's text, and no route returns it today.
- **Run steps come from the server, per pinned version** (story 2,
  2026-10-09). The runs route carries each run's step states, read in one
  query. A `definition` route outlines the exact `workflow.yaml` a run is
  pinned to, using the real parser. This replaced inspecting every listed
  run (finance runs carry transaction lists) and parsing YAML in the UI.
  An older run shows its own steps.
- **Theme tokens, not the design's raw colors.** The app is styled with
  Tailwind `@theme` tokens.
- **Four stories in order** (W9): header, tabs, Settings, badges and
  notifications; Runs; Workflows; chat run cards. Story 1 unblocks Changes
  story 6.
