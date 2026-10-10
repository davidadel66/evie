import { useCallback, useEffect, useState } from "react";
import { notificationAPI, type NoticePage, type NotificationAPI } from "../api/notifications";

const pollMs = 30_000;

/** useNotifications reads the app-wide notifications list: on mount,
 *  whenever refreshKey changes (a turn finished), on window focus and every
 *  30 seconds. A failed read keeps the last list rather than showing an
 *  error in the top bar. */
export function useNotifications(refreshKey?: unknown, api: NotificationAPI = notificationAPI) {
  const [page, setPage] = useState<NoticePage>();
  const refresh = useCallback(() => api.list().then(setPage, () => undefined), [api]);
  useEffect(() => {
    let live = true;
    const load = () => { api.list().then((value) => { if (live) setPage(value); }, () => undefined); };
    load();
    const timer = window.setInterval(load, pollMs);
    window.addEventListener("focus", load);
    return () => {
      live = false;
      window.clearInterval(timer);
      window.removeEventListener("focus", load);
    };
  }, [api, refreshKey]);
  const markRead = useCallback((ids: string[]) => api.markRead(ids).then(refresh, () => undefined), [api, refresh]);
  const markAllRead = useCallback(() => api.markAllRead().then(refresh, () => undefined), [api, refresh]);
  return { notices: page?.notifications ?? [], unread: page?.unread ?? 0, loaded: !!page, refresh, markRead, markAllRead };
}
