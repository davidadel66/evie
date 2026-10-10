# workspace-activity — spec

Decided 2026-10-09 with the owner in a grilling session; binding record in
[workspace-activity.decisions.md](workspace-activity.decisions.md). The UI
follows the claude.ai/design template
`templates/workspace-activity/WorkspaceActivity.dc.html`, except where the
decisions say otherwise. The design's Changes tab and review page belong to
[changes.spec.md](changes.spec.md).

## Problem Statement

A Workspace page today is one long scroll of stacked sections: research,
explore, build and verify settings, workflow definitions, workflow runs,
notifications, the folder, and conversations. Finding what needs the owner
means reading the whole page. A run waiting for an answer or a run that
failed shows up only in that Workspace's notification list, so the owner
must visit each Workspace to learn about it. A run's detail is a long
technical dump of attempts, leases and events, not an answer to "what is it
doing, and what does it need from me". Workflows show approval cards and
version history but not what the workflow does, step by step. When Evie
starts a run from a chat, the chat shows only a tool row.

## Solution

Each Workspace gets a header under the app's top bar with tabs: **Runs**,
**Workflows** and **Settings** (and **Changes** once that spec lands), with
badges for what needs the owner now and a line showing today's workflow
spend.

- **Runs** groups runs as Waiting for you, Running and Earlier. A run opens
  in a side panel showing its question, its steps, its outputs, what it
  spent and where it came from.
- **Workflows** lists each workflow with when it runs, its recent runs and
  its status. A workflow opens in a pop-up with a Steps view, its YAML, its
  approval and a way to start a run.
- **Settings** holds today's settings sections unchanged.
- A **notifications** icon in the app's top bar, on every view, gathers
  notices from every Workspace into one list. It is built to carry other
  kinds of notices later.
- When Evie starts a run from chat, the chat shows a live run card that
  opens the run in the chat Inspector.

## User Stories

### Workspace header and navigation

1. As the owner, I want each Workspace to show tabs for Runs, Workflows and Settings, so that I find things by kind instead of scrolling.
2. As the owner, I want a Changes tab added beside them once Changes ships, so that code work sits with the rest of the Workspace's activity.
3. As the owner, I want a Workspace to open on Runs, or on Changes when a Change needs my review, so that I land where I am needed.
4. As the owner, I want badges on the tabs for runs waiting on me and Changes needing review, so that I see at a glance what needs me.
5. As the owner, I want the sidebar to show the same waiting count beside each Workspace, so that I know which Workspace needs me without opening it.
6. As the owner, I want a badge to clear when I act on the item, so that the badge always means "needs you now".
7. As the owner, I want the header to show "Workflows today: X of Y tokens · $Z", or the spend alone when there is no cap, so that I know how close workflow runs are to the daily cap.
8. As the owner, I want the app's top bar kept on every view with its breadcrumb, and the Workspace header under it without repeating the Workspace name, so that navigation is the same everywhere.
9. As the owner, I want the Inspector toggle shown only in chat, so that the Inspector stays a chat tool.
10. As the owner, I want the Settings tab to hold the research, explore, build, verify, folder and conversation sections unchanged, so that nothing I configure today is lost.

### Notifications

11. As the owner, I want a notifications icon in the top bar on every view, with an unread count, so that I hear about runs in any Workspace without visiting it.
12. As the owner, I want the list newest first, each notice labelled with its source, so that I know where it came from.
13. As the owner, I want finished runs, failed runs, runs waiting on me, a reached cap, and skipped or failed schedule slots in the list, so that nothing a workflow reports is missed.
14. As the owner, I want clicking a notice to open what it points to (for a run, its panel in its Workspace's Runs tab) and mark it read, so that one click takes me there.
15. As the owner, I want "Mark all read", so that I can clear the list after catching up.
16. As the owner, I want the notifications list to accept notices from parts of Evie other than workflows later, so that it is one place for everything that needs my attention.

### Runs tab

17. As the owner, I want runs grouped as Waiting for you, Running and Earlier, so that runs needing me are on top.
18. As the owner, I want to filter runs by workflow with chips, so that I can follow one workflow's history.
19. As the owner, I want each run row to show the workflow and version, a one-line note (the question, the current step, the error or the result), a bar of its steps' states, how it started (schedule, chat, CLI or web), and when it started, so that I can triage without opening it.
20. As the owner, I want a run to open in a 440px panel beside the list, so that I keep my place in the list.
21. As the owner, I want Escape or the close button to close the panel, so that it gets out of the way quickly.
22. As the owner, I want "No runs yet" when a Workspace has none, so that an empty tab is not mistaken for a loading failure.

### Run panel

23. As the owner, I want the panel header to show the workflow, version and state, so that I know which run I am looking at.
24. As the owner, I want to see how the run started, with a link to the chat that started it when there is one, so that I can return to that conversation.
25. As the owner, I want the duration, step progress and spend on one line, so that I can see cost and progress together.
26. As the owner, I want a run's question shown at the top, with buttons when the answer is one of a fixed set and the existing form otherwise, so that I can answer in one click when possible.
27. As the owner, I want a note that I can also answer in any chat in the Workspace, so that I know the panel is not the only place.
28. As the owner, I want a failed run's error shown at the top, so that the reason is the first thing I read.
29. As the owner, I want an effect whose outcome is unknown to show the existing reconciliation form, so that I can settle it from the panel.
30. As the owner, I want the definition's steps in YAML order, marked done, current, not reached, or skipped by a branch once the run finishes, so that I see where the run is and what it skipped.
31. As the owner, I want each step's kind, a one-line detail, and its duration, so that I can tell slow steps from stuck ones.
32. As the owner, I want a succeeded run's outputs listed, so that I can see the result without reading logs.
33. As the owner, I want a collapsed Provenance section with the run ID, run key, executable hash and Workspace revision, so that I can audit exactly what ran.
34. As the owner, I want Cancel, with an optional reason, on an unfinished run, so that I can stop one going wrong.
35. As the owner, I want Rerun, with a required reason, on a succeeded run, so that the audited rerun is one click away.
36. As the owner, I want no action offered that the backend cannot do, so that buttons never fail by design.

### Workflows tab

37. As the owner, I want each workflow listed with its name, version, description, when it runs, dots for its recent runs, and its status, so that I see the whole set at once.
38. As the owner, I want "Needs approval" shown for a workflow with a pending proposal, so that I know it will not run yet.
39. As the owner, I want a workflow to open in a wide pop-up, so that its YAML is readable.
40. As the owner, I want a Steps view listing each step with its kind, description, and notes such as what it runs after, its conditions and its retries, so that I understand what the workflow does without reading YAML.
41. As the owner, I want clicking a step to show its lines in a YAML view, so that I can check the exact definition.
42. As the owner, I want the YAML view to show the active version's `workflow.yaml` with line numbers, so that I see what will run.
43. As the owner, I want a pending approval summarized in a banner whose button expands the full existing approval card, so that I can never approve without seeing the code and permission differences, any refusals and the dry run.
44. As the owner, I want "Start run" with the existing input form in the pop-up, so that I can run a workflow on request.
45. As the owner, I want Pause or Resume for a scheduled workflow and Deactivate in the pop-up, so that I manage it in one place.
46. As the owner, I want the workflow's recent runs listed in the pop-up, with "All runs" opening the Runs tab filtered to it, so that I can jump to its history.
47. As the owner, I want built-in workflows in the Workflows tab, so that every workflow lives in one tab.

### Chat

48. As the owner, I want a live run card in the chat when Evie starts a workflow run there, showing the workflow, state and current step, so that I can follow it from the conversation.
49. As the owner, I want clicking that card to open the run in the chat Inspector, so that I stay in the chat.
50. As the owner, I want the run card to keep updating while the run is unfinished, so that the chat never shows a stale state.

## Implementation Decisions

### Layout and navigation

- The app's top bar stays on every view. In a Workspace view its breadcrumb
  names the Workspace, and the Workspace header is a second row under it
  holding the tabs, badges and spend line.
- The top bar's Inspector toggle shows only in the chat view.
- The Workspace view keeps its selected tab and open panel as view state, in
  the existing single-active-view model. No URL routing is added.
- The default tab is Runs, or Changes when the Workspace has a Change
  needing review (after the Changes spec lands).
- Runs, and later Changes, open in a 440px side panel inside the Workspace
  area. Workflows open in a modal pop-up of up to 920px. Escape closes the
  innermost open layer.
- Colors, type and spacing use the existing theme tokens, not the design's
  raw values.

### Settings tab

- It holds the current Workspace page sections in their current order:
  research, explore, build and verify settings, the folder, and
  conversations. The Changes spec's Settings → Repository joins it.
- The workflow definitions, runs and notifications sections leave the
  Workspace page. They are replaced by the Workflows tab, the Runs tab and
  the notifications icon.

### Badges and counts

- Tab and sidebar badges count what needs the owner now: runs in
  `waiting_for_human` or `needs_reconciliation`, plus Changes needing review
  once Changes ships. A badge clears when the item leaves that state.
- The notifications icon counts unread notices. A waiting run is counted in
  both places until its notice is read.

### Notifications

- A new app-wide notifications route lists notices across all Workspaces,
  newest first, with an unread count. It also marks notices read: one,
  several, or all. It uses a general notice shape: id, source kind, source
  label, text, created time, read time, and a target to open.
- The only source today is the existing workflow notifications. Their
  target is a run in a Workspace, or the Workspace's Runs tab for notices
  without a run, such as a reached cap. Other sources plug into the same
  shape later.
- The per-Workspace workflow notification routes stay for the CLI.

### Runs tab and run panel

- The tab lists runs through the runs route, which takes a workflow filter
  and a limit of up to 200. Earlier is capped by that limit. The list
  refreshes every 15 seconds while the tab is open.
- The runs route carries each run's step states: the state of each step's
  latest attempt, map items aside, read for every listed run in one query.
  So the list never inspects runs one by one; a finance run's attempts
  carry whole transaction lists.
- A new read-only `definition` route takes a run ID and returns the
  `workflow.yaml` that run is pinned to (its recorded commit and path) and
  its outline from the real parser: each step's id, kind, description, the
  steps it can lead to, its retry ceiling and its line range. The UI reads
  it once per workflow version. An older run therefore shows its own steps,
  not the active version's. (Implemented 2026-10-09 in place of reading the
  active version and parsing YAML in the UI.)
- Row notes are derived:
  - waiting: "Waiting for you at" the step and its description (the
    question's message can be long instructions, as finance-review's is);
  - running: the current step and its description;
  - failed: the error;
  - succeeded: the outputs summarized.
- The step bar has one segment per definition step, colored by state.
- The panel reads the existing inspect route. Steps are the run's outline in
  YAML order, matched to the latest attempt per step:
  - done: a succeeded attempt;
  - current: the run's next step, running or waiting on the owner;
  - failed, or cancelled where a cancelled run stopped;
  - not reached: no attempt while the run is unfinished, or after it failed
    or was cancelled;
  - skipped: no attempt once the run succeeded (a branch not taken).

  A map step also shows how many of its items are done, failed or running,
  from its latest list of items.
- The panel keeps its frame and header while a newly selected run loads, so
  the list beside it never changes width.
- Questions:
  - A response schema that allows only a fixed set of values (an enum)
    renders as buttons.
  - Any other schema uses the existing answer form.
  - Effect reconciliations use the same existing answer form, which
    already takes a reconciliation.
- Spend is the sum of model-call and worker-call spend, using the existing
  helpers: recorded usage once finished, estimates while open.
- Actions:
  - Cancel (optional reason, asked before it fires) for unfinished runs,
    through the cancel route;
  - Rerun (required reason) for succeeded runs, through the start route
    with `rerun_of`;
  - no action for failed or cancelled runs.

### Workflows tab and pop-up

- The list comes from the existing workflow list. Recent-run dots come from
  the runs route filtered by workflow (the last 10). Status shows Active,
  Needs approval, Update needs approval (an active workflow with a newer
  version waiting), Blocked, Inactive or Paused, from the list and the
  schedule status. "Runs when" reads daily, weekday and single-day cron
  schedules in words with the zone's city; any other schedule shows its
  cron.
- The `definition` route from story 2 is extended to name a workflow
  instead of a run: it reads the active version at its recorded commit, or
  the pending proposal's commit when nothing is active. It never returns
  other files, prompts or fixtures.
- The Steps view uses the route's outline:
  - each step's id, kind and description;
  - notes for where it leads and its retries;
  - its line range.

  Clicking a step switches to the YAML view with those lines highlighted.
- The approval banner summarizes the proposal's new authority. Its button
  expands the existing approval card, and approval goes through the
  existing approve route with the hashes the card showed.
- Start run uses the existing start form inside the pop-up. It starts the
  run detached and opens it in the Runs tab. The pop-up also lists the
  approved versions behind a Versions disclosure. Pause and Resume use the
  schedule route. Deactivate uses the deactivate route. Built-in workflows
  reuse the existing built-in list in the tab.

### Chat run card

- When a chat turn's `workflow_start` tool call returns a run, the chat
  shows a run card in place of the generic tool row: workflow, state,
  current step. It refreshes while the run is unfinished.
- Clicking the card opens the run in the chat Inspector, using the same run
  detail as the panel.
- Changes never get a card in chat (per the Changes spec).

### Module shape

- The run detail (header, question, steps, outputs, provenance, actions) is
  one props-driven component, used by the Runs panel and the chat
  Inspector.
- The step derivation (definition steps plus attempts to step states) is a
  pure function, used by the step bar, the panel and the chat card.
- The notification list is a props-driven component that takes general
  notices.

## Delivery Stories

Each story is independently demonstrable and ends with
`./scripts/verify-change.sh` passing.

1. **Header, tabs, Settings tab, badges and the notifications icon.**
   - Outcome: a Workspace opens on its header with Runs, Workflows and
     Settings. For now, the Runs and Workflows tabs show the existing runs
     and workflow sections as they are. The Settings tab holds the settings
     sections. Badges and the spend line are live. The notifications icon
     lists notices from all Workspaces through the new app-wide route.
   - Acceptance:
     - a waiting run badges its tab and its Workspace in the sidebar;
     - a notice from another Workspace appears in the icon's list and opens
       that Workspace's Runs tab;
     - Mark all read clears the count;
     - the Inspector toggle shows only in chat.
   - Risk: moving sections must not drop any setting.
2. **Runs tab and run panel.**
   - Outcome: the design's Runs tab and run panel on real runs.
   - Acceptance:
     - grouping, filter chips and step states are right for a waiting, a
       running, a failed and a succeeded run, including a skipped branch
       step;
     - an enum question answers by button;
     - Cancel and Rerun go through their routes;
     - a failed run offers no action.
   - Depends on 1.
3. **Workflows tab, pop-up and source endpoint.**
   - Outcome: the design's Workflows tab and pop-up on real workflows.
   - Acceptance:
     - the source route returns the active `workflow.yaml` and nothing else;
     - Steps and YAML views agree on line ranges for finance-review;
     - approval is unreachable without the expanded card;
     - Start run, Pause or Resume, and Deactivate work.
   - Depends on 1.
4. **Run cards in chat.**
   - Outcome: a `workflow_start` from chat shows a live run card that opens
     the run in the chat Inspector.
   - Acceptance:
     - the card updates from running to finished;
     - clicking it opens the shared run detail in the Inspector.
   - Depends on 2.

## Testing Decisions

- Tests check what the owner sees and what the server receives, not
  component internals.
- No new seams. UI tests are component tests with the network faked at the
  API module, as in the existing workspace and workflow component tests
  (runs, interruption answer, notifications). Each test drives a props-driven
  component, or a container with faked API calls, and asserts visible text,
  roles and the requests sent.
- The step derivation is unit-tested as a pure function over definitions
  and attempts: linear, branching with a skipped step, a map step, a
  waiting question, a failed step.
- The new routes (app-wide notifications, workflow source) get Go route
  tests like the existing workflow route tests in `internal/web`, against a
  real database in a temporary folder. The source route test asserts that
  only `workflow.yaml` is returned and that an unknown workflow is refused.

## Out of Scope

- The Changes tab, review page and chat Inspector diffs (changes.spec.md).
- A Chat tab in the Workspace header; chat stays its own view.
- Rerun for failed runs; actions the backend lacks today.
- Notification sources other than workflows, and external delivery (email,
  push). Both stay in the backlog; the notice shape leaves room for them.
- URL routing or deep links.
- Editing workflow YAML in the UI.
- The design's mock data and its "detailMode" page variant.

## Further Notes

- Decided while writing this spec, not asked in the session. The owner
  should confirm or change them:
  - Runs in `needs_reconciliation` also count as needing the owner in
    badges.
  - The Runs tab shows at most 200 runs, the existing route's limit, with
    no paging.
  - A workflow's recent-run dots show its last 10 runs.
  - Failed and cancelled runs offer no action.
  - The YAML view falls back to the pending proposal's `workflow.yaml` when
    no version is active.
- Story 1 is the dependency for Changes story 6 (the Changes tab).
