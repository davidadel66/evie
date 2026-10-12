import type {
  ContextSessionSnapshot,
  StoredSession,
  Workspace,
} from "../api/contextSessions";
import { useEffect, useState } from "react";
import { Archive, Database, Folder, MessageSquare, Plus, Settings, Sidebar as SidebarIcon } from "../ui/Icon";

// Desktop width is the person's to set: drag or arrow keys on the edge,
// double-click to reset. Saved per browser like the Inspector's width.
export const sidebarWidth = { initial: 240, min: 200, max: 360 } as const;
const widthKey = "evie.sidebarWidth";
const clampWidth = (value: number) => Math.max(sidebarWidth.min, Math.min(sidebarWidth.max, Math.round(value)));
import { ReviewBadge } from "../candidateInbox/MemoryReviewTabs";
import { waitingText } from "../workspaces/workspaceActivity";

export type SidebarDestination = "chat" | "data" | "workspaces" | `workspace:${string}`;

type Props = {
  snapshot?: ContextSessionSnapshot;
  instructions?: React.ReactNode;
  destination: SidebarDestination;
  busy: boolean;
  /** Memories Evie proposed that await review in Data → Memory → Review. */
  memoryReviewPending?: number;
  /** Runs waiting on the owner now, by Workspace ID. */
  waiting?: Record<string, number>;
  mobileOpen: boolean;
  collapsed?: boolean;
  onSettings: () => void;
  onCreateWorkspace: () => void;
  onArchive: (session: StoredSession) => void;
  /** Asks to delete (archive) a Workspace; the caller confirms first. */
  onDeleteWorkspace?: (workspace: Workspace) => void;
  onCloseMobile: () => void;
  onData: () => void;
  onWorkspace: (workspace: Workspace) => void;
  onNewWorkspaceChat: (workspace: Workspace) => void;
  onSession: (session: StoredSession) => void;
};

export function Sidebar({
  snapshot,
  instructions,
  destination,
  busy,
  memoryReviewPending = 0,
  waiting = {},
  mobileOpen,
  collapsed = false,
  onSettings,
  onCreateWorkspace,
  onArchive,
  onDeleteWorkspace,
  onCloseMobile,
  onData,
  onWorkspace,
  onNewWorkspaceChat,
  onSession,
}: Props) {
  // An archived ("deleted") Workspace leaves the sidebar; Settings restores it.
  const activeWorkspaces = snapshot?.workspaces.filter((workspace) => workspace.state === "active") ?? [];
  const [width, setWidth] = useState(() => {
    try {
      const stored = Number(localStorage.getItem(widthKey));
      return stored ? clampWidth(stored) : sidebarWidth.initial;
    } catch {
      return sidebarWidth.initial;
    }
  });
  useEffect(() => {
    try { localStorage.setItem(widthKey, String(width)); } catch { /* the width stays in memory */ }
  }, [width]);
  const resize = (event: React.PointerEvent) => {
    const start = event.clientX, initial = width;
    event.currentTarget.setPointerCapture(event.pointerId);
    const move = (next: PointerEvent) => setWidth(clampWidth(initial + next.clientX - start));
    const end = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", end);
      window.removeEventListener("pointercancel", end);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", end);
    window.addEventListener("pointercancel", end);
  };
  return (
    <aside
      id="evie-navigation"
      aria-label="Evie navigation"
      style={{ width }}
      className={`${mobileOpen ? "flex" : "hidden"} ${collapsed ? "md:hidden" : "md:flex"} border-hair bg-sidebar absolute inset-y-0 left-0 z-40 max-w-[85vw] flex-none flex-col border-r md:relative md:z-auto`}
    >
      <div
        role="separator"
        aria-label="Resize sidebar"
        aria-orientation="vertical"
        aria-valuenow={width}
        aria-valuemin={sidebarWidth.min}
        aria-valuemax={sidebarWidth.max}
        tabIndex={0}
        title="Drag to resize. Double-click to reset."
        onPointerDown={resize}
        onDoubleClick={() => setWidth(sidebarWidth.initial)}
        onKeyDown={(event) => {
          const next = event.key === "ArrowLeft" ? width - 16 : event.key === "ArrowRight" ? width + 16
            : event.key === "Home" ? sidebarWidth.min : event.key === "End" ? sidebarWidth.max : undefined;
          if (next === undefined) return;
          event.preventDefault();
          setWidth(clampWidth(next));
        }}
        className="hover:bg-teal/40 focus-visible:bg-teal absolute inset-y-0 -right-1 z-10 hidden w-1.5 cursor-col-resize touch-none md:block"
      />
      <div className="flex h-[54px] flex-none items-center px-4">
        <div aria-label="evie." className="text-ink font-sans text-[21px] font-medium tracking-[-0.04em]">
          evie<span className="text-teal">.</span>
        </div>
        <div className="flex-1" />
        <button
          type="button"
          aria-label="Close navigation"
          onClick={onCloseMobile}
          title="Collapse sidebar"
          className="text-faint hover:text-body focus-visible:ring-teal rounded p-2 focus-visible:ring-1 focus-visible:outline-none"
        >
          <SidebarIcon size={16} />
        </button>
      </div>

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-5">
        <SidebarButton
          active={destination === "data"}
          icon={<Database size={15} />}
          label="Data"
          badge={<ReviewBadge pending={memoryReviewPending} />}
          onClick={onData}
        />

        <div className="mt-7 mb-2 flex items-center px-2">
          <span className="text-faint text-[11px] font-medium">Workspaces</span>
          <div className="flex-1" />
          <button
            type="button"
            title="Create workspace"
            aria-label="Create workspace"
            onClick={onCreateWorkspace}
            disabled={busy}
            className="text-faint hover:text-body focus-visible:ring-teal rounded p-1 focus-visible:ring-1 focus-visible:outline-none"
          >
            <Plus size={13} />
          </button>
        </div>

        {activeWorkspaces.length === 0 && snapshot && (
          <button
            type="button"
            onClick={onCreateWorkspace}
            className="text-fainter hover:text-muted-text w-full rounded-[7px] px-2 py-2 text-left text-xs"
          >
            Create your first workspace
          </button>
        )}

        {activeWorkspaces.map((workspace) => (
          <WorkspaceNav
            key={workspace.id}
            workspace={workspace}
            sessions={(snapshot?.sessions ?? []).filter((session) => session.workspaceId === workspace.id && session.status === "active")}
            active={destination === `workspace:${workspace.id}`}
            waiting={waiting[workspace.id] ?? 0}
            activeSessionId={snapshot?.activeSession?.id}
            busy={busy}
            onWorkspace={onWorkspace}
            onNewChat={onNewWorkspaceChat}
            onSession={onSession}
            onArchive={onArchive}
            onDelete={onDeleteWorkspace}
          />
        ))}
      </nav>

      <div className="border-hair flex-none border-t px-2 py-2">
        <SidebarButton active={false} icon={<Settings size={15} />} label="Settings" onClick={onSettings} />
        {instructions}
      </div>
    </aside>
  );
}

function WorkspaceNav({
  workspace,
  sessions,
  active,
  waiting,
  activeSessionId,
  busy,
  onWorkspace,
  onNewChat,
  onSession,
  onArchive,
  onDelete,
}: {
  workspace: Workspace;
  sessions: StoredSession[];
  active: boolean;
  waiting: number;
  activeSessionId?: string;
  busy: boolean;
  onWorkspace: (workspace: Workspace) => void;
  onNewChat: (workspace: Workspace) => void;
  onSession: (session: StoredSession) => void;
  onArchive: (session: StoredSession) => void;
  onDelete?: (workspace: Workspace) => void;
}) {
  return (
    <div className="mb-1">
      <div className="group flex min-w-0 items-center gap-1">
      <button
        type="button"
        onClick={() => onWorkspace(workspace)}
        aria-current={active ? "page" : undefined}
        className={`${active ? "bg-selected text-ink" : "text-muted-text hover:bg-hover hover:text-body"} flex min-w-0 flex-1 items-center gap-2 rounded-[7px] px-2 py-[7px] text-left`}
      >
        <Folder size={14} />
        <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium">{workspace.displayName}</span>
        {waiting > 0 && <>
          <span aria-hidden="true" title="Waiting on you" className="border-amber-hair-strong bg-amber-bg text-amber-ink min-w-[18px] flex-none rounded-full border px-[5px] text-center text-[10.5px] leading-4">{waiting}</span>
          <span className="sr-only">, {waitingText(waiting)}</span>
        </>}
      </button>
      {onDelete && <button type="button" aria-label={`Delete ${workspace.displayName}`} title="Delete workspace" disabled={busy} onClick={() => onDelete(workspace)} className="text-faint hover:text-body hover:bg-hover focus-visible:ring-teal flex-none cursor-pointer rounded p-1.5 opacity-100 focus-visible:ring-1 focus-visible:outline-none disabled:cursor-default disabled:opacity-30 md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100">
        <Archive size={13} />
      </button>}
      <button type="button" aria-label={`New session in ${workspace.displayName}`} title={`New session in ${workspace.displayName}`} disabled={busy || workspace.state === "archived" || !workspace.currentRevisionId} onClick={() => onNewChat(workspace)} className="text-faint hover:text-body hover:bg-hover focus-visible:ring-teal mr-1 flex-none cursor-pointer rounded p-1.5 focus-visible:ring-1 focus-visible:outline-none disabled:cursor-default disabled:opacity-30">
        <Plus size={14} />
      </button>
      </div>
      {sessions.slice(0, 5).map((session) => (
        <div key={session.id} className="group flex items-center rounded-[6px] hover:bg-hover focus-within:bg-hover">
          <button
            type="button"
            disabled={busy}
            onClick={() => onSession(session)}
            className={`${activeSessionId === session.id ? "text-teal" : "text-muted-text hover:text-body"} focus-visible:ring-teal flex min-w-0 flex-1 items-center gap-2 rounded-[6px] py-[7px] pr-1 pl-7 text-left focus-visible:ring-1 focus-visible:outline-none disabled:opacity-50`}
          >
            <MessageSquare size={12} />
            <span className="truncate text-[11.5px]">{session.title.trim() || "Untitled session"}</span>
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => onArchive(session)}
            aria-label={`Archive ${session.title.trim() || "Untitled session"}`}
            title="Archive session"
            className="text-faint hover:text-body focus-visible:ring-teal mr-1 rounded p-1.5 opacity-100 focus-visible:ring-1 focus-visible:outline-none disabled:opacity-30 md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100"
          >
            <Archive size={13} />
          </button>
        </div>
      ))}
    </div>
  );
}

function SidebarButton({
  active,
  icon,
  label,
  badge,
  disabled,
  onClick,
}: {
  active: boolean;
  icon: React.ReactNode;
  label: string;
  badge?: React.ReactNode;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      aria-current={active ? "page" : undefined}
      className={`${active ? "bg-selected text-ink" : "text-muted-text hover:bg-hover hover:text-body"} mb-1 flex w-full items-center gap-3 rounded-[8px] px-3 py-[9px] text-left disabled:opacity-50`}
    >
      {icon}
      <span className="text-[13px] font-medium">{label}</span>
      {badge}
    </button>
  );
}
