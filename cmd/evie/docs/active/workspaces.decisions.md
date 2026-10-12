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
