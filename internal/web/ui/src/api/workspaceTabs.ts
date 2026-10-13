// A Workspace's own tabs (workspace-tabs.spec.md): folders under
// .evie/tabs/ in its folder, listed with any problem and served sealed.

export type CustomTab = { id: string; title: string; order: number; problem?: string };
export type CustomTabList = { tabs: CustomTab[] };

export type WorkspaceTabsAPI = { list(workspaceId: string): Promise<CustomTabList> };

export const workspaceTabsAPI: WorkspaceTabsAPI = {
  async list(workspaceId) {
    const response = await fetch("/api/workspaces/tabs", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ workspaceId }), cache: "no-store",
    });
    const value = (await response.json()) as CustomTabList & { error?: string };
    if (!response.ok) throw new Error(value.error ?? `Request failed (${response.status})`);
    return value;
  },
};

/** customTabURL is where a tab's page is served; the trailing slash keeps its relative links inside the tab. */
export function customTabURL(workspaceId: string, tabId: string): string {
  return `/workspace-tabs/${encodeURIComponent(workspaceId)}/${encodeURIComponent(tabId)}/`;
}
