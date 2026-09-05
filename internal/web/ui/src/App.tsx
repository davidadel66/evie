import { useEffect, useMemo, useState } from "react";
import type {
  ContextScope,
  ContextSessionSelection,
  Project,
  Workspace,
} from "./api/contextSessions";
import { Panel, type InspectorTarget } from "./artifacts/Panel";
import { Chat } from "./chat/Chat";
import { Composer } from "./chat/Composer";
import { DataHub, type DataSource } from "./data/DataHub";
import { Sidebar, type SidebarDestination } from "./shell/Sidebar";
import { selectionForScope } from "./shell/scopeSelection";
import { useContextSessions } from "./store/useContextSessions";
import { useSession } from "./store/useSession";
import { Banner } from "./ui/Banner";
import { Cross, Expand, Folder, MessageSquare, PanelRight, Sidebar as SidebarIcon } from "./ui/Icon";
import {
  defaultChatTextSize,
  resolveChatTextSize,
  type ChatTextSize,
} from "./ui/textSize";
import { WorkspaceHome, Workspaces } from "./workspaces/Workspaces";

export type WorkbenchView =
  | { id: "chat"; kind: "chat" }
  | { id: "data"; kind: "data" }
  | { id: "workspaces"; kind: "workspaces" }
  | { id: `workspace:${string}`; kind: "workspace"; workspaceId: string; label: string };

const textSizeStorageKey = "evie.chatTextSize";
const initialViews: WorkbenchView[] = [{ id: "chat", kind: "chat" }];

export default function App() {
  const { items, status, queue, problem, send, answer, dismissProblem, reset } = useSession();
  const contextSessions = useContextSessions();
  const [views, setViews] = useState<WorkbenchView[]>(initialViews);
  const [activeViewId, setActiveViewId] = useState<WorkbenchView["id"]>("chat");
  const [draft, setDraft] = useState("");
  const [textSize, setTextSize] = useState<ChatTextSize>(loadChatTextSize);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorFocused, setInspectorFocused] = useState(false);
  const [inspectorOverride, setInspectorOverride] = useState<InspectorTarget>();
  const [dataSource, setDataSource] = useState<DataSource>("memory");

  const activeView = views.find((view) => view.id === activeViewId) ?? views[0];
  const activeWorkspace = activeView.kind === "workspace"
    ? contextSessions.snapshot?.workspaces.find((workspace) => workspace.id === activeView.workspaceId)
    : undefined;
  const latestFileDiff = [...items].reverse().find((item) => item.kind === "tool" && item.approval?.preview);
  const inspectorTarget = inspectorOverride ?? defaultInspectorTarget(
    activeView,
    activeWorkspace,
    contextSessions.snapshot?.activeScope,
    latestFileDiff?.kind === "tool" && latestFileDiff.approval?.preview
      ? {
          kind: "file-diff",
          path: latestFileDiff.approval.preview.path,
          oldText: latestFileDiff.approval.preview.oldText,
          newText: latestFileDiff.approval.preview.newText,
          isNew: latestFileDiff.approval.preview.isNew,
          state: latestFileDiff.approval.state,
        }
      : undefined,
  );

  const pending = items.find((item) => item.kind === "tool" && item.approval?.state === "pending");
  const pendingId = pending?.kind === "tool" ? pending.approval?.reqId : undefined;

  useEffect(() => {
    if (!pendingId || activeView.kind !== "chat") return;
    const onKey = (event: KeyboardEvent) => {
      const element = event.target as HTMLElement | null;
      if (element?.tagName === "TEXTAREA" || element?.tagName === "INPUT") return;
      const key = event.key.toLowerCase();
      if (key === "y") answer(pendingId, true);
      else if (key === "n") answer(pendingId, false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [pendingId, activeView.kind, answer]);

  useEffect(() => {
    try {
      localStorage.setItem(textSizeStorageKey, textSize);
    } catch {
      // The in-memory setting still works in locked-down browser contexts.
    }
  }, [textSize]);

  const openView = (view: WorkbenchView) => {
    setViews((current) => current.some((open) => open.id === view.id) ? current : [...current, view]);
    setActiveViewId(view.id);
    setInspectorOverride(undefined);
    setMobileNavOpen(false);
  };

  const openWorkspace = (workspace: Workspace) => {
    openView({ id: `workspace:${workspace.id}`, kind: "workspace", workspaceId: workspace.id, label: workspace.displayName });
  };

  const activateView = (view: WorkbenchView) => {
    setActiveViewId(view.id);
    setInspectorOverride(undefined);
  };

  const closeView = (view: WorkbenchView) => {
    if (view.kind === "chat") return;
    const index = views.findIndex((open) => open.id === view.id);
    const next = views.filter((open) => open.id !== view.id);
    setViews(next);
    if (view.id === activeViewId) setActiveViewId(next[Math.max(0, index - 1)]?.id ?? "chat");
    setInspectorOverride(undefined);
  };

  const selectSession = async (selection: ContextSessionSelection) => {
    try {
      await contextSessions.select(selection);
      reset();
      setDraft("");
      setActiveViewId("chat");
      setInspectorOverride(undefined);
      setMobileNavOpen(false);
    } catch {
      // useContextSessions owns the actionable error message.
    }
  };

  const startWorkspaceChat = (workspace: Workspace) => selectSession({
    workspaceId: workspace.id,
    workspaceRevision: workspace.currentRevisionId,
  });

  const startProjectChat = (project: Project) => selectSession({ projectId: project.id });

  const startNewChat = () => {
    const scope = contextSessions.snapshot?.activeScope;
    if (!scope) {
      openView({ id: "workspaces", kind: "workspaces" });
      return;
    }
    const selection = selectionForScope(scope);
    if (selection) void selectSession(selection);
    else openView({ id: "workspaces", kind: "workspaces" });
  };

  const registerWorkspace = async (name: string) => {
    try {
      await contextSessions.register(name);
      reset();
      setDraft("");
      setActiveViewId("chat");
      setInspectorOverride(undefined);
    } catch {
      // useContextSessions owns the actionable error message.
    }
  };

  const workspaceSessions = useMemo(
    () => activeWorkspace
      ? contextSessions.snapshot?.sessions.filter((session) => session.workspaceId === activeWorkspace.id) ?? []
      : [],
    [activeWorkspace, contextSessions.snapshot?.sessions],
  );

  return (
    <div data-chat-size={textSize} className="bg-app text-ink flex h-screen overflow-hidden text-[13px]">
      {mobileNavOpen && <button type="button" aria-label="Close navigation overlay" onClick={() => setMobileNavOpen(false)} className="absolute inset-0 z-30 bg-black/55 md:hidden" />}
      <Sidebar
        snapshot={contextSessions.snapshot}
        destination={sidebarDestination(activeView)}
        busy={contextSessions.busy || status === "streaming"}
        mobileOpen={mobileNavOpen}
        textSize={textSize}
        onTextSize={setTextSize}
        onCloseMobile={() => setMobileNavOpen(false)}
        onNewChat={startNewChat}
        onData={() => openView({ id: "data", kind: "data" })}
        onWorkspaces={() => openView({ id: "workspaces", kind: "workspaces" })}
        onWorkspace={openWorkspace}
        onSession={(session) => void selectSession({ sessionId: session.id })}
      />

      <div className="relative flex min-w-0 flex-1 flex-col">
        <WorkbenchBar
          views={views}
          activeViewId={activeView.id}
          chatLabel={contextSessions.snapshot?.activeSession?.title.trim() || "Chat"}
          inspectorOpen={inspectorOpen}
          inspectorFocused={inspectorFocused}
          onOpenNavigation={() => setMobileNavOpen(true)}
          onActivate={activateView}
          onClose={closeView}
          onToggleInspector={() => {
            setInspectorOpen((open) => !open);
            if (inspectorOpen) setInspectorFocused(false);
          }}
          onToggleInspectorFocus={() => {
            setInspectorOpen(true);
            setInspectorFocused((focused) => !focused);
          }}
        />

        {problem && <Banner message={problem} onDismiss={dismissProblem} />}

        <div className="relative flex min-h-0 flex-1">
          {!inspectorFocused && (
            <div className="flex min-w-0 flex-1 flex-col">
              {activeView.kind === "chat" && (
                contextSessions.snapshot?.activeScope ? (
                  <>
                    <ScopeBar scope={contextSessions.snapshot.activeScope} onOpenWorkspaces={() => openView({ id: "workspaces", kind: "workspaces" })} />
                    <Chat items={items} queued={queue} streaming={status === "streaming"} onAnswer={answer} />
                    <Composer
                      value={draft}
                      onChange={setDraft}
                      streaming={status === "streaming"}
                      disabled={contextSessions.busy}
                      onSend={() => {
                        send(draft);
                        setDraft("");
                      }}
                    />
                  </>
                ) : (
                  <ChooseScope
                    snapshot={contextSessions.snapshot}
                    busy={contextSessions.busy}
                    onWorkspace={(workspace) => void startWorkspaceChat(workspace)}
                    onProject={(project) => void startProjectChat(project)}
                    onUnscoped={() => void selectSession({ unscoped: true })}
                    onManage={() => openView({ id: "workspaces", kind: "workspaces" })}
                  />
                )
              )}
              {activeView.kind === "data" && (
                <DataHub
                  source={dataSource}
                  onSource={(source) => {
                    setDataSource(source);
                    setInspectorOverride(undefined);
                  }}
                  onOpenMemoryDetail={(detail) => {
                    setInspectorOverride({ kind: "memory", detail });
                    setInspectorOpen(true);
                  }}
                />
              )}
              {activeView.kind === "workspaces" && (
                <Workspaces
                  snapshot={contextSessions.snapshot}
                  busy={contextSessions.busy || status === "streaming"}
                  problem={contextSessions.problem}
                  onRegister={(name) => void registerWorkspace(name)}
                  onOpenWorkspace={openWorkspace}
                  onNewWorkspaceChat={(workspace) => void startWorkspaceChat(workspace)}
                  onNewProjectChat={(project) => void startProjectChat(project)}
                  onResume={(session) => void selectSession({ sessionId: session.id })}
                  onNewUnscopedChat={() => void selectSession({ unscoped: true })}
                />
              )}
              {activeView.kind === "workspace" && activeWorkspace && (
                <WorkspaceHome
                  workspace={activeWorkspace}
                  sessions={workspaceSessions}
                  busy={contextSessions.busy || status === "streaming"}
                  onNewChat={() => void startWorkspaceChat(activeWorkspace)}
                  onResume={(session) => void selectSession({ sessionId: session.id })}
                />
              )}
              {activeView.kind === "workspace" && !activeWorkspace && (
                <div className="text-muted-text flex flex-1 items-center justify-center">This workspace is no longer available.</div>
              )}
            </div>
          )}

          {inspectorOpen && (
            <Panel
              target={inspectorTarget}
              focused={inspectorFocused}
              onClose={() => {
                setInspectorOpen(false);
                setInspectorFocused(false);
              }}
            />
          )}
        </div>
      </div>
    </div>
  );
}

export function WorkbenchBar({
  views,
  activeViewId,
  chatLabel,
  inspectorOpen,
  inspectorFocused,
  onOpenNavigation,
  onActivate,
  onClose,
  onToggleInspector,
  onToggleInspectorFocus,
}: {
  views: WorkbenchView[];
  activeViewId: WorkbenchView["id"];
  chatLabel: string;
  inspectorOpen: boolean;
  inspectorFocused: boolean;
  onOpenNavigation: () => void;
  onActivate: (view: WorkbenchView) => void;
  onClose: (view: WorkbenchView) => void;
  onToggleInspector: () => void;
  onToggleInspectorFocus: () => void;
}) {
  return (
    <header className="border-hair bg-topbar flex h-[46px] flex-none items-stretch border-b">
      <button type="button" aria-label="Open navigation" onClick={onOpenNavigation} className="text-faint hover:text-body px-4 md:hidden"><SidebarIcon size={16} /></button>
      <div role="tablist" aria-label="Open work" className="flex min-w-0 flex-1 overflow-x-auto">
        {views.map((view) => {
          const active = view.id === activeViewId;
          return (
            <div key={view.id} className={`${active ? "bg-app text-body" : "text-faint hover:text-muted-text"} border-hair group flex max-w-[220px] min-w-[120px] items-center border-r`}>
              <button
                type="button"
                role="tab"
                aria-selected={active}
                onClick={() => onActivate(view)}
                className="min-w-0 flex-1 truncate px-3 text-left text-[11.5px]"
              >
                {view.kind === "chat" ? chatLabel : view.kind === "data" ? "Data" : view.kind === "workspaces" ? "Workspaces" : view.label}
              </button>
              {view.kind !== "chat" && (
                <button type="button" aria-label={`Close ${view.kind === "workspace" ? view.label : view.kind} tab`} onClick={() => onClose(view)} className="hover:text-body mr-2 rounded p-1 opacity-0 focus:opacity-100 group-hover:opacity-100"><Cross size={11} /></button>
              )}
            </div>
          );
        })}
      </div>
      <div className="flex flex-none items-center gap-1 px-2">
        <button type="button" title="Toggle inspector" aria-label="Toggle inspector" aria-pressed={inspectorOpen} onClick={onToggleInspector} className={`${inspectorOpen ? "text-teal" : "text-faint hover:text-body"} focus-visible:ring-teal rounded p-2 focus-visible:ring-1 focus-visible:outline-none`}><PanelRight size={15} /></button>
        <button type="button" title="Focus inspector" aria-label="Focus inspector" aria-pressed={inspectorFocused} onClick={onToggleInspectorFocus} className={`${inspectorFocused ? "text-teal" : "text-faint hover:text-body"} focus-visible:ring-teal rounded p-2 focus-visible:ring-1 focus-visible:outline-none`}><Expand size={14} /></button>
      </div>
    </header>
  );
}

function ScopeBar({ scope, onOpenWorkspaces }: { scope: ContextScope; onOpenWorkspaces: () => void }) {
  return (
    <div className="border-hair flex h-[38px] flex-none items-center gap-2 border-b px-5">
      <span className="text-teal"><Folder size={13} /></span>
      <span className="text-body text-[11.5px] font-medium">{scope.displayName}</span>
      <span className="text-fainter text-[10.5px]">{scope.kind === "workspace" ? "Workspace" : scope.kind === "project" ? "Project" : "Unscoped"}</span>
      {scope.workspaceRevision && <span className="text-ghost hidden font-mono text-[9.5px] sm:inline">revision {shortId(scope.workspaceRevision)}</span>}
      <div className="flex-1" />
      <button type="button" onClick={onOpenWorkspaces} className="text-faint hover:text-body text-[10.5px]">Change context</button>
    </div>
  );
}

function ChooseScope({
  snapshot,
  busy,
  onWorkspace,
  onProject,
  onUnscoped,
  onManage,
}: {
  snapshot: ReturnType<typeof useContextSessions>["snapshot"];
  busy: boolean;
  onWorkspace: (workspace: Workspace) => void;
  onProject: (project: Project) => void;
  onUnscoped: () => void;
  onManage: () => void;
}) {
  return (
    <main className="flex min-h-0 flex-1 items-center justify-center overflow-y-auto p-7">
      <div className="w-full max-w-[560px]">
        <div className="text-teal mb-3 flex items-center gap-2 text-xs"><MessageSquare size={14} /> New conversation</div>
        <h1 className="text-ink text-[22px] font-semibold tracking-[-0.02em]">Choose where this conversation belongs</h1>
        <p className="text-muted-text mt-2 max-w-[520px] text-[13px] leading-6">The selected context fixes which memory and resources are available for the life of the session.</p>
        <div className="border-hair mt-7 border-t">
          {snapshot?.workspaces.filter((workspace) => workspace.state === "active").map((workspace) => (
            <ScopeChoice key={workspace.id} label={workspace.displayName} detail="Workspace" disabled={busy} onClick={() => onWorkspace(workspace)} />
          ))}
          {snapshot?.projects.filter((project) => !project.archived).map((project) => (
            <ScopeChoice key={project.id} label={project.displayName} detail="Filesystem project" disabled={busy} onClick={() => onProject(project)} />
          ))}
          <ScopeChoice label="Unscoped" detail="Global and session memory only" disabled={busy} onClick={onUnscoped} />
        </div>
        <button type="button" onClick={onManage} className="text-teal hover:text-teal-hover mt-5 text-xs">Manage workspaces</button>
      </div>
    </main>
  );
}

function ScopeChoice({ label, detail, disabled, onClick }: { label: string; detail: string; disabled: boolean; onClick: () => void }) {
  return (
    <button type="button" disabled={disabled} onClick={onClick} className="border-hair hover:bg-hover flex w-full items-center gap-3 border-b px-2 py-4 text-left disabled:opacity-40">
      <span className="border-hair-strong text-faint flex h-8 w-8 items-center justify-center rounded-[7px] border"><Folder size={14} /></span>
      <span className="min-w-0 flex-1"><span className="text-body block truncate text-[13px] font-medium">{label}</span><span className="text-fainter mt-1 block text-[10.5px]">{detail}</span></span>
    </button>
  );
}

function sidebarDestination(view: WorkbenchView): SidebarDestination {
  if (view.kind === "workspace") return view.id;
  return view.kind;
}

function defaultInspectorTarget(view: WorkbenchView, workspace?: Workspace, scope?: ContextScope, fileDiff?: InspectorTarget): InspectorTarget {
  if (view.kind === "workspace" && workspace) return { kind: "workspace", workspace };
  if (view.kind === "chat" && fileDiff) return fileDiff;
  if (view.kind === "chat" && scope) return { kind: "scope", scope };
  if (view.kind === "data") return { kind: "data" };
  return { kind: "empty" };
}

function shortId(value: string) {
  return value.length > 12 ? value.slice(0, 12) : value;
}

function loadChatTextSize(): ChatTextSize {
  try {
    return resolveChatTextSize(localStorage.getItem(textSizeStorageKey));
  } catch {
    return defaultChatTextSize;
  }
}
