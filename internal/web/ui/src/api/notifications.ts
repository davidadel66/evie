// The app-wide notifications list (workspace-activity.spec.md): every
// notice the owner should hear about, from every Workspace, in one general
// shape so later sources join the same list. Workflow runs are the only
// source today.

/** Where opening a notice leads: a run in its Workspace, or a Workspace's runs. */
export type NoticeTarget = { kind: "workflow_run" | "workspace_runs" | string; workspace_id?: string; run_id?: string };

export type Notice = {
  /** Names its source, e.g. "workflow:12". */
  id: string;
  source: "workflow" | string;
  /** What the notice is about, e.g. the workflow's name. */
  label: string;
  text: string;
  created_at: string;
  read_at?: string;
  target: NoticeTarget;
};

export type NoticePage = { unread: number; notifications: Notice[] };

async function post<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/notifications${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json();
  if (!response.ok) throw new Error(value.error ?? `Notification request failed (${response.status})`);
  return value as T;
}

export const notificationAPI = {
  list: (): Promise<NoticePage> => post("", {}),
  markRead: (ids: string[]): Promise<{ marked: number }> => post("/read", { ids, read: true }),
  markAllRead: (): Promise<{ marked: number }> => post("/read", { all: true, read: true }),
};
export type NotificationAPI = typeof notificationAPI;
