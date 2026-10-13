import { customTabURL, type CustomTab } from "../api/workspaceTabs";

/**
 * WorkspaceTabFrame shows one of the Workspace's own tabs sealed
 * (workspace-tabs.spec.md): scripts run, but without same-origin, popups,
 * forms or top navigation, and the server's policy adds no network. A new
 * reloadKey reloads the page.
 */
export function WorkspaceTabFrame({workspaceId, tab, reloadKey = 0}: {workspaceId: string; tab: CustomTab; reloadKey?: number}) {
  return <iframe key={`${tab.id}:${reloadKey}`} title={tab.title} src={customTabURL(workspaceId, tab.id)}
    sandbox="allow-scripts" referrerPolicy="no-referrer" className="bg-app min-h-0 w-full flex-1 border-0" />;
}
