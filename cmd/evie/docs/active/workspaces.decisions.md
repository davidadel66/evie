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
