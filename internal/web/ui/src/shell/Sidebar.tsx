import type {
  ContextScope,
  ContextSessionSnapshot,
  StoredSession,
  Workspace,
} from "../api/contextSessions";
import { Database, Folder, MessageSquare, Plus, Sidebar as SidebarIcon } from "../ui/Icon";
import { TextSizeMenu } from "../ui/TextSizeMenu";
import type { ChatTextSize } from "../ui/textSize";

export type SidebarDestination = "chat" | "data" | "workspaces" | `workspace:${string}`;

type Props = {
  snapshot?: ContextSessionSnapshot;
  destination: SidebarDestination;
  busy: boolean;
  mobileOpen: boolean;
  textSize: ChatTextSize;
  onTextSize: (value: ChatTextSize) => void;
  onCloseMobile: () => void;
  onNewChat: () => void;
  onData: () => void;
  onWorkspaces: () => void;
  onWorkspace: (workspace: Workspace) => void;
  onSession: (session: StoredSession) => void;
};

export function Sidebar({
  snapshot,
  destination,
  busy,
  mobileOpen,
  textSize,
  onTextSize,
  onCloseMobile,
  onNewChat,
  onData,
  onWorkspaces,
  onWorkspace,
  onSession,
}: Props) {
  return (
    <aside
      aria-label="Evie navigation"
      className={`${mobileOpen ? "flex" : "hidden"} border-hair bg-sidebar absolute inset-y-0 left-0 z-40 w-[276px] flex-none flex-col border-r md:relative md:z-auto md:flex`}
    >
      <div className="flex h-[54px] flex-none items-center px-4">
        <div className="flex items-center gap-[9px]">
          <span className="bg-teal text-primary-foreground flex h-7 w-7 items-center justify-center rounded-[8px] font-sans text-[14px] font-bold">
            E
          </span>
          <span className="text-ink font-sans text-[15px] font-semibold tracking-[0.08em]">EVIE</span>
        </div>
        <div className="flex-1" />
        <button
          type="button"
          aria-label="Close navigation"
          onClick={onCloseMobile}
          className="text-faint hover:text-body focus-visible:ring-teal rounded p-2 focus-visible:ring-1 focus-visible:outline-none md:hidden"
        >
          <SidebarIcon size={16} />
        </button>
      </div>

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-5">
        <SidebarButton
          active={destination === "chat"}
          icon={<Plus size={15} />}
          label="New chat"
          disabled={busy}
          onClick={onNewChat}
        />
        <SidebarButton
          active={destination === "data"}
          icon={<Database size={15} />}
          label="Data"
          onClick={onData}
        />
        <SidebarButton
          active={destination === "workspaces"}
          icon={<Folder size={15} />}
          label="Workspaces"
          onClick={onWorkspaces}
        />

        <div className="mt-7 mb-2 flex items-center px-2">
          <span className="text-faint text-[11px] font-medium">Workspaces</span>
          <div className="flex-1" />
          <button
            type="button"
            title="Create workspace"
            aria-label="Create workspace"
            onClick={onWorkspaces}
            className="text-faint hover:text-body focus-visible:ring-teal rounded p-1 focus-visible:ring-1 focus-visible:outline-none"
          >
            <Plus size={13} />
          </button>
        </div>

        {snapshot?.workspaces.length === 0 && (
          <button
            type="button"
            onClick={onWorkspaces}
            className="text-fainter hover:text-muted-text w-full rounded-[7px] px-2 py-2 text-left text-xs"
          >
            Create your first workspace
          </button>
        )}

        {snapshot?.workspaces.map((workspace) => (
          <WorkspaceNav
            key={workspace.id}
            workspace={workspace}
            sessions={snapshot.sessions.filter((session) => session.workspaceId === workspace.id)}
            active={destination === `workspace:${workspace.id}`}
            activeSessionId={snapshot.activeSession?.id}
            busy={busy}
            onWorkspace={onWorkspace}
            onSession={onSession}
          />
        ))}
      </nav>

      <div className="border-hair flex-none border-t px-3 py-3">
        <div className="flex items-center gap-2">
          <ScopeMark scope={snapshot?.activeScope} />
          <div className="min-w-0 flex-1">
            <div className="text-body truncate text-xs font-medium">
              {snapshot?.activeScope?.displayName ?? "No scope selected"}
            </div>
            <div className="text-fainter mt-px text-[10.5px]">
              {snapshot?.activeScope ? `${scopeLabel(snapshot.activeScope)} scope` : "Choose a workspace to begin"}
            </div>
          </div>
          <TextSizeMenu value={textSize} onChange={onTextSize} placement="top" />
        </div>
      </div>
    </aside>
  );
}

function WorkspaceNav({
  workspace,
  sessions,
  active,
  activeSessionId,
  busy,
  onWorkspace,
  onSession,
}: {
  workspace: Workspace;
  sessions: StoredSession[];
  active: boolean;
  activeSessionId?: string;
  busy: boolean;
  onWorkspace: (workspace: Workspace) => void;
  onSession: (session: StoredSession) => void;
}) {
  return (
    <div className="mb-1">
      <button
        type="button"
        onClick={() => onWorkspace(workspace)}
        aria-current={active ? "page" : undefined}
        className={`${active ? "bg-selected text-ink" : "text-muted-text hover:bg-hover hover:text-body"} flex w-full items-center gap-2 rounded-[7px] px-2 py-[7px] text-left`}
      >
        <Folder size={14} />
        <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium">{workspace.displayName}</span>
        {workspace.state === "archived" && <span className="text-ghost text-[10px]">Archived</span>}
      </button>
      {sessions.slice(0, 5).map((session) => (
        <button
          type="button"
          disabled={busy}
          key={session.id}
          onClick={() => onSession(session)}
          className={`${activeSessionId === session.id ? "text-teal" : "text-faint hover:text-body"} flex w-full items-center gap-2 rounded-[6px] py-[6px] pr-2 pl-7 text-left disabled:opacity-50`}
        >
          <MessageSquare size={12} />
          <span className="truncate text-[11.5px]">{session.title.trim() || "Untitled session"}</span>
        </button>
      ))}
    </div>
  );
}

function SidebarButton({
  active,
  icon,
  label,
  disabled,
  onClick,
}: {
  active: boolean;
  icon: React.ReactNode;
  label: string;
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
    </button>
  );
}

function ScopeMark({ scope }: { scope?: ContextScope }) {
  return (
    <span className={`${scope ? "border-teal-hair bg-teal-deep text-teal-hover" : "border-hair-strong text-ghost"} flex h-7 w-7 flex-none items-center justify-center rounded-[7px] border`}>
      {scope?.kind === "workspace" ? <Folder size={13} /> : scope?.kind === "project" ? <LayersMark /> : <MessageSquare size={13} />}
    </span>
  );
}

function LayersMark() {
  return <span className="font-mono text-[10px]">P</span>;
}

function scopeLabel(scope: ContextScope) {
  if (scope.kind === "workspace") return "Workspace";
  if (scope.kind === "project") return "Project";
  return "Unscoped";
}
