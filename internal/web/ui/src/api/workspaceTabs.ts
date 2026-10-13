// A Workspace's own tabs (workspace-tabs.spec.md): folders under
// .evie/tabs/ in its folder, listed with any problem and served sealed. A
// tab reads only the data its tab.json declares, once the owner allows it.

/** One thing a tab may read: a registered database, files matching a glob, or this Workspace's runs. */
export type TabDataSource = { db?: string; files?: string; runs?: boolean };
export type CustomTab = { id: string; title: string; order: number; data?: TabDataSource[]; dataKey?: string; approved?: boolean; problem?: string };
export type CustomTabList = { tabs: CustomTab[] };
/** A read a tab asks for through Evie's page; the server checks it against the approved list. */
export type TabDataRequest = { kind: string; db?: string; sql?: string; path?: string };

export type WorkspaceTabsAPI = {
  list(workspaceId: string): Promise<CustomTabList>;
  approve(workspaceId: string, tabId: string, dataKey: string): Promise<unknown>;
  withdraw(workspaceId: string, tabId: string): Promise<unknown>;
  data(workspaceId: string, tabId: string, request: TabDataRequest): Promise<{ result: unknown }>;
};

async function post<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = (await response.json()) as T & { error?: string };
  if (!response.ok) throw new Error(value.error ?? `Request failed (${response.status})`);
  return value;
}

export const workspaceTabsAPI: WorkspaceTabsAPI = {
  list: (workspaceId) => post("/api/workspaces/tabs", { workspaceId }),
  approve: (workspaceId, tabId, dataKey) => post("/api/workspaces/tabs/approve", { workspaceId, tabId, dataKey }),
  withdraw: (workspaceId, tabId) => post("/api/workspaces/tabs/withdraw", { workspaceId, tabId }),
  data: (workspaceId, tabId, request) => post("/api/workspaces/tabs/data", { workspaceId, tabId, request }),
};

/** customTabURL is where a tab's page is served; the trailing slash keeps its relative links inside the tab. */
export function customTabURL(workspaceId: string, tabId: string): string {
  return `/workspace-tabs/${encodeURIComponent(workspaceId)}/${encodeURIComponent(tabId)}/`;
}

/** describeSource names a data source the way Settings → Tabs asks the owner about it. */
export function describeSource(source: TabDataSource): string {
  if (source.db) return `the ${source.db} database (read-only)`;
  if (source.files) return `files matching ${source.files}`;
  if (source.runs) return "this workspace's workflow runs";
  return "nothing";
}
