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
