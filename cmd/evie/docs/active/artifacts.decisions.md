# Artifacts decisions

Spec: [artifacts.spec.md](artifacts.spec.md).

- **2026-11-02 — Artifacts come before skills, and they are separate features.**
  David asked for both and accepted the recommendation to build artifacts first
  ("go with your recommendations"). Neither depends on the other; the first
  artifacts slice is smaller and shows him something right away. Skills are
  specified in [skills.spec.md](skills.spec.md).

- **2026-11-02 — The first artifact is a worker's stored report, opened in the
  Inspector; no artifact store yet.** David accepted the recommendation from
  [agent-artifacts.md](../research/agent-artifacts.md): start from a reference
  over an existing stored object (option C) rather than a file-only design
  (option A) or a new SQLite store (option B). It adds no table, model tool or
  prompt change, and tests "one stored object, two views" on real reports
  before any schema exists. A store follows only when a measured need appears.

- **2026-11-02 — Messages point to artifacts; they don't carry them.** David
  asked whether an artifact could be a message from one agent to another. The
  answer he accepted: a message is addressed and read once, while an artifact
  is a kept, versioned work product fetched by reference, and a handoff is a
  message that points to artifacts. This matches A2A 1.0.0 ("Messages SHOULD
  NOT be used to deliver task outputs") and the existing worker result, where
  harness fields are the message and the stored report is the artifact.

- **2026-11-02 — Artifacts start as Markdown, tables and SVG; model-written HTML
  comes later, only through the Workspace tab seal. Tabs stay live views, and
  artifacts are kept outputs.** David accepted the recommendation. Stage 1
  renders Markdown only. The `workspace_tabs` prompt line that sends "reports"
  to tabs is unchanged until a later stage gives kept outputs a home.

- **2026-11-02 — The owner view uses the open-chat rule, not the model's read
  checks.** Chosen under David's "go with your recommendations". The report
  route answers only for the open chat and only for workers that chat
  delegated, as the worker list and trace routes do (subagents.decisions.md,
  2026-10-12). It does not apply `ReadSubagentReport`'s Task-grant and
  delegation-time Workspace checks, which limit what a model may read. The
  trace route already shows David the same report text under the open-chat
  rule. Two-way door: the store read can add those checks without changing
  the route.

- **2026-11-02 — Reports render inert: raw HTML as text, http(s) links only,
  no images loaded.** Chosen under David's "go with your recommendations",
  following the research's renderer findings. The chat renderer parses and
  sanitizes HTML and loads images, which suits model replies but not web text
  a worker quoted. Not loading images also prevents tracking requests when a
  report opens. The inert mode applies only to the Report view.

- **2026-11-02 — Artifacts do not feed Memory or Automatic Recall for now.**
  David is going to spend dedicated time on memory ("i think for the memory i
  need to spend more time on it"), so any artifact-to-memory path waits for
  that redesign.

- **2026-11-02 — Test seams approved.** David approved the three seams in the
  spec's Testing Decisions: the store read on real SQLite, the HTTP route with
  the existing fakes, and the UI render with a hostile report.

- **2026-11-02 — Usage counts the worker's model responses beside its
  tokens.** Story 14 asks for tokens and requests, but the stored usage holds
  tokens only. David accepted the recommendation ("yes"): the owner report
  read also counts the worker's committed model responses in that attempt,
  shown as model responses beside the tokens. It is counted at read time from
  the attempt's own events, with no schema change.

- **2026-11-02 — The Activity view keeps the chat renderer for now.** The
  worker's final message in the Activity trace still renders through the chat
  renderer, which sanitizes HTML and loads images. That is the exposure the
  inert mode avoids, but this stage leaves Activity unchanged, as the spec
  says. David accepted the recommendation ("yes") to record it as a
  follow-up rather than widen this stage.

- **2026-11-02 — Built as two tickets on `feat/artifacts-report-view`.**
  David approved the split ("yes"): first, opening a worker's report in the
  Inspector (store read of the text, route, Report and Activity switch, inert
  Markdown, no-report and partial states); then, blocked by it, Evie's facts
  above the report (sources and their verification, unverified URLs, notes,
  limitations labelled as the worker's, usage, and legacy records).

- **2026-11-02 — Build choices (two-way doors).** Decided under the flow
  brief David approved ("yes"); each is cheap to reverse.
  - The read is `Store.SubagentReport` in `eviedb`, beside `SubagentEvents`.
    A missing worker or another chat's answers `sql.ErrNoRows`, as
    `SubagentEvents` does.
  - Run-owned workers read as not found by an explicit check, not only
    through the parent filter.
  - The route is `POST /api/subagents/report` with `sessionId` and
    `executionId`. It reuses the trace route's `subagents_session_changed`
    (409) and `subagent_not_found` (404) codes.
  - `delegation.Source` gains a `Legacy()` accessor, because its legacy flag
    is unexported.
  - The inert mode is Streamdown with its `sanitize` and `harden` rehype
    plugins but without `raw`, plus its own link and image components. Code
    highlighting stays. A local render against streamdown 2.5.0 showed
    that dropping `raw` turns HTML into text. It also showed that default
    links render as buttons without `href`, and that React adds an image
    preload for any `<img>`, so an inert image is never an `<img>`.
  - Each attempt of a continued worker shows its own final report, the one
    `read_subagent_report` pages for that execution.
  - The Report and Activity switch follows its default until David picks a
    tab, so a worker that finishes while he watches moves to Report.
  - The report is read once after the worker finishes and is not polled.
    While the worker runs, Report says the report appears when it finishes.
  - Source URLs in Evie's facts are clickable. They already passed the
    harness's http(s) filter, and they open outside Evie as report links do.
  - A pre-Stage 8 record without a final event shows no report; its inline
    `findings` are not shown. Every finished attempt has had a final event
    since 2026-09-11.

- **2026-11-02 — Choices made while building and reviewing (two-way
  doors).**
  - An image inside a link shows its alt text as part of that link, which
    opens the link's target, because a link cannot hold another link.
    Elsewhere an image's alt text links to the image URL when it is http(s).
  - A URL that is not http(s) shows as plain text without link styling, so
    nothing looks clickable that isn't.
  - A failed report read is retried three times, six seconds apart, and
    then waits for the worker to be reopened.
  - When a record names a final report event that no longer exists, the
    report route answers 500, not 404. The worker exists, and its stored
    report is the fault.
  - The owner report read and `read_subagent_report` load the report through
    one shared loader, and the trace and report routes share one open-chat
    guard, so neither boundary can drift between two copies.

- **2026-11-03 — Older limitations stay labelled as the worker's.** Results
  from before Stage 10 mixed harness notes into `limitations`, so some of
  Evie's own older notes show under the worker's label. David accepted the
  recommendation ("YES"): keep that cautious label, which matches how the
  parent model sees those results.

- **2026-11-03 — Evie's notes show word for word in stage 1.** The notes were
  written for the parent model and can name its tools. David accepted the
  recommendation ("YES") to show them as stored; rewording them for him is a
  later change.

- **2026-11-03 — Canvas review before merge.** The Report view was built
  without the canvas prototype David asks for on UI work. He accepted the
  recommendation ("YES"): before-and-after screenshots go on the design
  canvas for his verdict, and any changes land on this branch before merge.
