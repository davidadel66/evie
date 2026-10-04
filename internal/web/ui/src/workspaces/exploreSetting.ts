import type { Workspace, WorkspaceExploreSetting } from "../api/contextSessions";

// Turning explore on returns a Workspace with a folder to its folder default
// (explore stays confined to the folder and ends if it is detached); without
// a folder it is an explicit on. Turning it off is always explicit.
export function exploreSettingFor(workspace: Workspace, checked: boolean): WorkspaceExploreSetting {
  if (!checked) return "off";
  return workspace.folder?.path ? "default" : "on";
}
