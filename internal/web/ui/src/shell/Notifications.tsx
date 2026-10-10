import { useEffect, useState } from "react";
import type { Notice } from "../api/notifications";
import { useNotifications } from "../store/useNotifications";
import { Bell } from "../ui/Icon";
import { noticeContext, noticeTime, unreadBadge } from "./notices";

// The notifications icon in the top bar (workspace-activity.spec.md): one
// list for the whole app, newest first, each notice labelled with where it
// came from. Opening a notice marks it read and shows what it points to.

export function NotificationList({notices, workspaceNames, now, onOpen}: {
  notices: Notice[];
  workspaceNames: Record<string, string>;
  now: Date;
  onOpen: (notice: Notice) => void;
}) {
  if (notices.length === 0) {
    return <p className="text-muted-text px-4 py-6 text-xs leading-5">Nothing yet. Workflow runs that finish or need you show up here.</p>;
  }
  return <ul className="max-h-[420px] overflow-y-auto">
    {notices.map((notice) => <li key={notice.id} className="border-hair border-t first:border-t-0">
      <button type="button" onClick={() => onOpen(notice)} className="hover:bg-hover focus-visible:bg-hover flex w-full gap-3 px-4 py-3 text-left focus-visible:outline-none">
        <span aria-hidden="true" className={`mt-[5px] h-[7px] w-[7px] flex-none rounded-full ${notice.read_at ? "bg-transparent" : "bg-teal"}`} />
        <span className="min-w-0 flex-1">
          {!notice.read_at && <span className="sr-only">Unread: </span>}
          <span className="flex items-baseline gap-2 text-[11px]">
            <span className="text-faint min-w-0 flex-1 truncate font-mono">{noticeContext(notice, workspaceNames)}</span>
            <span className="text-fainter flex-none">{noticeTime(notice.created_at, now)}</span>
          </span>
          <span className={`mt-1 block break-words text-[12.5px] leading-5 ${notice.read_at ? "text-muted-text" : "text-body"}`}>{notice.text}</span>
        </span>
      </button>
    </li>)}
  </ul>;
}

export function NotificationsMenu({notices, unread, workspaceNames, now, open, onToggle, onOpen, onMarkAllRead}: {
  notices: Notice[];
  unread: number;
  workspaceNames: Record<string, string>;
  now: Date;
  open: boolean;
  onToggle: () => void;
  onOpen: (notice: Notice) => void;
  onMarkAllRead: () => void;
}) {
  return <div className="relative flex-none">
    <button type="button" onClick={onToggle} aria-expanded={open} aria-haspopup="dialog"
      aria-label={unread > 0 ? `Notifications, ${unread} unread` : "Notifications"} title="Notifications"
      className={`${open ? "text-teal" : "text-faint hover:text-body"} focus-visible:ring-teal relative flex rounded p-2 focus-visible:ring-1 focus-visible:outline-none`}>
      <Bell size={15} />
      {unread > 0 && <span aria-hidden="true" className="bg-teal text-app absolute -top-px right-0 min-w-[15px] rounded-full px-[4px] text-center font-sans text-[9.5px] leading-[15px] font-semibold">{unreadBadge(unread)}</span>}
    </button>
    {open && <>
      <button type="button" aria-label="Close notifications" tabIndex={-1} onClick={onToggle} className="fixed inset-0 z-30 cursor-default" />
      <div role="dialog" aria-label="Notifications" className="border-hair-bubble bg-card absolute top-full right-0 z-40 mt-1 w-[360px] max-w-[calc(100vw-16px)] overflow-hidden rounded-[10px] border shadow-[0_16px_40px_rgba(0,0,0,0.45)]">
        <div className="flex items-center px-4 py-3">
          <span className="text-ink flex-1 text-[13px] font-medium">Notifications</span>
          {unread > 0 && <button type="button" onClick={onMarkAllRead} className="text-teal hover:text-teal-hover text-xs">Mark all read</button>}
        </div>
        <NotificationList notices={notices} workspaceNames={workspaceNames} now={now} onOpen={onOpen} />
      </div>
    </>}
  </div>;
}

/** The top bar's notifications icon with its list, kept current by polling. */
export function Notifications({refreshKey, workspaceNames, onOpen}: {
  refreshKey?: unknown;
  workspaceNames: Record<string, string>;
  onOpen: (notice: Notice) => void;
}) {
  const {notices, unread, refresh, markRead, markAllRead} = useNotifications(refreshKey);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    if (!open) return;
    const onEscape = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [open]);
  return <NotificationsMenu notices={notices} unread={unread} workspaceNames={workspaceNames} now={new Date()} open={open}
    onToggle={() => { if (!open) void refresh(); setOpen(!open); }}
    onOpen={(notice) => {
      setOpen(false);
      if (!notice.read_at) void markRead([notice.id]);
      onOpen(notice);
    }}
    onMarkAllRead={() => void markAllRead()} />;
}
