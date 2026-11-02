# Workspaces decisions

- **2026-10-11 — Deleting a Workspace archives it, and Settings restores it.**
  David asked for a way to delete a Workspace and chose archive over erasure;
  `workspaces.spec.md` keeps hard erasure separate until a retention policy
  exists. The sidebar offers "Delete workspace" behind a confirmation that
  says what is kept. An archived Workspace leaves the sidebar and the
  Workspaces page, takes no new sessions or workflow runs, and keeps its
  sessions, memory and history. A chat open inside it closes, as it does when
  that conversation is archived. Settings → Archived workspaces lists each one
  with Restore, which makes it active again. Archive and restore are refused
  while a turn is running.

- **2026-10-11 — An archived Workspace has no memory proposals.**
  David: "there shouldn't be memory proposals for archived workspaces
  regardless." A pending proposal that targets an archived Workspace, or was
  found in one of its conversations (including one that targets Everywhere),
  is left out of the review inbox and its pending count, and approving it is
  refused ("its workspace is archived; restore the workspace to approve it").
  It is hidden, not declined: restoring the Workspace brings it back. The
  background memory pass does not read an archived Workspace's conversations;
  their cursors hold, so a restore resumes where the pass stopped. Queueing a
  new proposal for an archived Workspace is refused, so a pass or turn already
  running when the Workspace is archived queues nothing and the pass keeps
  its cursor.

- **2026-11-02 — New Workspaces are Standard with research delegation on;
  creation asks for neither.** David found the Agent Preset choice
  over-engineered and wanted research delegation always on. Creating a
  Workspace now asks only for a name, a repository and a folder. The server
  creates it on the Standard preset with research delegation on, unless the
  request sends `allowResearchDelegation: false`; a request that names another
  preset gets no research, as before. The REPL creates Workspaces the same
  way. Workspace settings keep the research switch (David: "sure you can keep
  it"). Reasons: a Standard chat already has
  web search and page reading itself, so handing that work to a worker reaches
  nothing new, and workers have fewer tools than their parent; every real
  Workspace had chosen Standard with research on; chats outside a Workspace
  never had the gate. Existing Workspaces keep their settings. The same
  allowance lets a Workspace chat start the built-in deep-research workflow
  without approval when remote workflow models are on
  (workflow-runtime.decisions.md, "Out of the box"), so a new Workspace can
  start it too. Presets stay as
  the record of each chat's tools and each worker's fence. Whether to offer
  them to the owner again is issue #198, for when subagent and research work
  resumes.
