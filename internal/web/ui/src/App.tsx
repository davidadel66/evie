import { useEffect, useMemo, useRef, useState } from "react";
import type {
  ContextScope,
  ContextSessionSelection,
  Project,
  Workspace,
} from "./api/contextSessions";
import { MemoryPresentationProvider } from "./memory/presentation";
import { Panel, type InspectorTarget } from "./artifacts/Panel";
import { inspectToolFile, selectedFileInspection, type FileSelection } from "./artifacts/fileInspection";
import { Chat } from "./chat/Chat";
import { Composer } from "./chat/Composer";
import { DataHub, type DataSource } from "./data/DataHub";
import { Sidebar, type SidebarDestination } from "./shell/Sidebar";
import { selectionForScope } from "./shell/scopeSelection";
import { useContextSessions } from "./store/useContextSessions";
import { useSession } from "./store/useSession";
import { Banner } from "./ui/Banner";
import { Cross, Database, Expand, Folder, MessageSquare, PanelRight, Sidebar as SidebarIcon } from "./ui/Icon";
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
const sidebarStorageKey = "evie.sidebarCollapsed";
const initialViews: WorkbenchView[] = [{ id: "chat", kind: "chat" }];

export default function App() {
  const contextSessions = useContextSessions();
  const { items, status, queue, problem, send, answer, dismissProblem, historyLoading, historyProblem, hasOlder, loadOlder, retryHistory } = useSession(contextSessions.snapshot?.activeSession?.id);
  const [views, setViews] = useState<WorkbenchView[]>(initialViews);
  const [activeViewId, setActiveViewId] = useState<WorkbenchView["id"]>("chat");
  const [draft, setDraft] = useState("");
  const [textSize, setTextSize] = useState<ChatTextSize>(loadChatTextSize);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try { return localStorage.getItem(sidebarStorageKey) === "true"; } catch { return false; }
  });
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectorFocused, setInspectorFocused] = useState(false);
  const [inspectorOverride, setInspectorOverride] = useState<InspectorTarget>();
  const [selectedFile, setSelectedFile] = useState<FileSelection>();
  const fileTrigger = useRef<HTMLButtonElement | null>(null);
  const activityTrigger = useRef<HTMLButtonElement | null>(null);
  const [dataSource, setDataSource] = useState<DataSource>("memory");

  const activeView = views.find((view) => view.id === activeViewId) ?? views[0];
  const activeWorkspace = activeView.kind === "workspace"
    ? contextSessions.snapshot?.workspaces.find((workspace) => workspace.id === activeView.workspaceId)
    : undefined;
  const selectedInspection = selectedFileInspection(selectedFile, contextSessions.snapshot?.activeSession?.id, items);
  const latestFileDiff = [...items].reverse().find((item) => item.kind === "tool" && item.approval?.preview);
  const latestFile = latestFileDiff?.kind === "tool" ? inspectToolFile(latestFileDiff) : null;
  const visibleOverride = inspectorOverride?.kind === "memory-evidence" && inspectorOverride.sessionId !== contextSessions.snapshot?.activeSession?.id ? undefined : inspectorOverride;
  const inspectorTarget: InspectorTarget = selectedInspection
    ? {kind: "file", file: selectedInspection}
    : visibleOverride ?? defaultInspectorTarget(activeView, activeWorkspace, contextSessions.snapshot?.activeScope, latestFile ? {kind: "file", file: latestFile} : undefined);

  const closeInspector = () => {
    setInspectorOpen(false);
    setInspectorFocused(false);
    requestAnimationFrame(() => {
      restoreFileFocus(fileTrigger.current, activityTrigger.current);
    });
  };
  useEffect(() => {
    if (!inspectorOpen || !selectedFile) return;
    const onEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setInspectorOpen(false);
      setInspectorFocused(false);
      requestAnimationFrame(() => restoreFileFocus(fileTrigger.current, activityTrigger.current));
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [inspectorOpen, selectedFile]);

  const pending = items.find((item) => item.kind === "tool" && item.approval?.state === "pending");
  const pendingId = pending?.kind === "tool" ? pending.approval?.reqId : undefined;

  useEffect(() => {
    if (!pendingId || activeView.kind !== "chat") return;
    const onKey = (event: KeyboardEvent) => {
      const element = event.target as HTMLElement | null;
      if (element?.tagName === "TEXTAREA" || element?.tagName === "INPUT" || element?.closest('[aria-label="Inspector"]')) return;
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

  useEffect(() => {
    try { localStorage.setItem(sidebarStorageKey, String(sidebarCollapsed)); } catch { /* In-memory preference still works. */ }
  }, [sidebarCollapsed]);

  const closeNavigation = () => {
    setMobileNavOpen(false);
    if (window.matchMedia("(min-width: 768px)").matches) setSidebarCollapsed(true);
    requestAnimationFrame(() => document.querySelector<HTMLButtonElement>('button[aria-label="Open navigation"]')?.focus());
  };

  const openView = (view: WorkbenchView) => {
    setViews((current) => current.some((open) => open.id === view.id) ? current : [...current, view]);
    setActiveViewId(view.id);
    setInspectorOverride(undefined);
    setSelectedFile(undefined);
    setMobileNavOpen(false);
  };

  const openWorkspace = (workspace: Workspace) => {
    openView({ id: `workspace:${workspace.id}`, kind: "workspace", workspaceId: workspace.id, label: workspace.displayName });
  };

  const activateView = (view: WorkbenchView) => {
    setActiveViewId(view.id);
    setInspectorOverride(undefined);
    setSelectedFile(undefined);
  };

  const closeView = (view: WorkbenchView) => {
    if (view.kind === "chat") return;
    const index = views.findIndex((open) => open.id === view.id);
    const next = views.filter((open) => open.id !== view.id);
    setViews(next);
    if (view.id === activeViewId) setActiveViewId(next[Math.max(0, index - 1)]?.id ?? "chat");
    setInspectorOverride(undefined);
    setSelectedFile(undefined);
  };

  const selectSession = async (selection: ContextSessionSelection) => {
    try {
      await contextSessions.select(selection);
      setDraft("");
      setActiveViewId("chat");
      setInspectorOverride(undefined);
      setSelectedFile(undefined);
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
      setDraft("");
      setActiveViewId("chat");
      setInspectorOverride(undefined);
      setSelectedFile(undefined);
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
    <MemoryPresentationProvider snapshot={contextSessions.snapshot}><div data-chat-size={textSize} className="bg-app text-ink flex h-screen overflow-hidden text-[13px]">
      {mobileNavOpen && <button type="button" aria-label="Close navigation overlay" onClick={closeNavigation} className="absolute inset-0 z-30 bg-black/55 md:hidden" />}
      <Sidebar
        snapshot={contextSessions.snapshot}
        destination={sidebarDestination(activeView)}
        busy={contextSessions.busy || status === "streaming"}
        mobileOpen={mobileNavOpen}
        collapsed={sidebarCollapsed}
        textSize={textSize}
        onTextSize={setTextSize}
        onCloseMobile={closeNavigation}
        onNewChat={startNewChat}
        onData={() => openView({ id: "data", kind: "data" })}
        onWorkspaces={() => openView({ id: "workspaces", kind: "workspaces" })}
        onWorkspace={openWorkspace}
        onNewWorkspaceChat={(workspace) => void startWorkspaceChat(workspace)}
        onSession={(session) => void selectSession({ sessionId: session.id })}
      />

      <div className="relative flex min-w-0 flex-1 flex-col">
        <WorkbenchBar
          views={views}
          activeViewId={activeView.id}
          chatLabel={contextSessions.snapshot?.activeSession?.title.trim() || "Chat"}
          inspectorOpen={inspectorOpen}
          inspectorFocused={inspectorFocused}
          navigationCollapsed={sidebarCollapsed}
          onOpenNavigation={() => {
            if (window.matchMedia("(min-width: 768px)").matches) setSidebarCollapsed(false);
            else setMobileNavOpen(true);
            requestAnimationFrame(() => document.querySelector<HTMLButtonElement>('button[aria-label="Close navigation"]')?.focus());
          }}
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

        {contextSessions.problem && activeView.kind !== "workspaces" && <div role="alert" className="border-danger-hair bg-danger-bg text-danger-ink border-b px-5 py-3 text-sm">{contextSessions.problem}</div>}
        {problem && <Banner message={problem} onDismiss={dismissProblem} />}

        <div className="relative flex min-h-0 flex-1">
          {!inspectorFocused && (
            <div className="flex min-w-0 flex-1 flex-col">
              {activeView.kind === "chat" && (
                contextSessions.snapshot?.activeScope ? (
                  <>
                    <Chat key={contextSessions.snapshot.activeSession?.id} items={items} queued={queue} streaming={status === "streaming"} onAnswer={answer} onOpenFile={(key, trigger) => {
                      const sessionId = contextSessions.snapshot?.activeSession?.id;
                      if (!sessionId) return;
                      fileTrigger.current = trigger;
                      activityTrigger.current = trigger.closest('section[aria-label="Turn activity"]')?.querySelector<HTMLButtonElement>('button[aria-controls]') ?? null;
                      setSelectedFile({sessionId, key});
                      setInspectorOpen(true);
                      setInspectorFocused(false);
                    }} onOpenMemory={(snapshotId, trigger) => {
                      const sessionId = contextSessions.snapshot?.activeSession?.id;
                      if (!sessionId) return;
                      fileTrigger.current = trigger;
                      activityTrigger.current = trigger.closest('section[aria-label="Turn activity"]')?.querySelector<HTMLButtonElement>('button[aria-controls]') ?? null;
                      setSelectedFile(undefined);
                      setInspectorOverride({kind: "memory-evidence", sessionId, snapshotId});
                      setInspectorOpen(true);
                      setInspectorFocused(false);
                    }} historyLoading={historyLoading} historyProblem={historyProblem} hasOlder={hasOlder} onOlder={loadOlder} onRetry={retryHistory} />
                    <Composer
                      value={draft}
                      onChange={setDraft}
                      streaming={status === "streaming"}
                      disabled={contextSessions.busy || historyLoading || !!historyProblem}
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
                    setSelectedFile(undefined);
                  }}
                  snapshot={contextSessions.snapshot}
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
              onClose={closeInspector}
            />
          )}
        </div>
      </div>
    </div></MemoryPresentationProvider>
  );
}

export function WorkbenchBar({
  views,
  activeViewId,
  chatLabel,
  inspectorOpen,
  inspectorFocused,
  navigationCollapsed = false,
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
  navigationCollapsed?: boolean;
  onOpenNavigation: () => void;
  onActivate: (view: WorkbenchView) => void;
  onClose: (view: WorkbenchView) => void;
  onToggleInspector: () => void;
  onToggleInspectorFocus: () => void;
}) {
  return (
    <header className="border-hair bg-topbar flex h-[52px] flex-none items-center gap-2 border-b px-2">
      <button type="button" aria-label="Open navigation" aria-controls="evie-navigation" onClick={onOpenNavigation} className={`text-muted-text hover:bg-hover hover:text-body focus-visible:ring-teal flex-none rounded-lg p-2.5 focus-visible:ring-1 focus-visible:outline-none ${navigationCollapsed ? "" : "md:hidden"}`}><SidebarIcon size={16} /></button>
      <div role="tablist" aria-label="Open work" className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto py-1">
        {views.map((view) => {
          const active = view.id === activeViewId;
          return (
            <div key={view.id} className={`${active ? "bg-selected text-ink" : "text-muted-text hover:bg-hover hover:text-body"} group flex h-9 min-w-[96px] max-w-[260px] flex-none items-center rounded-lg`}>
              <button
                type="button"
                role="tab"
                aria-selected={active}
                title={view.kind === "chat" ? chatLabel : view.kind === "data" ? "Data" : view.kind === "workspaces" ? "Workspaces" : view.label}
                onClick={() => onActivate(view)}
                className="focus-visible:ring-teal flex h-full min-w-0 flex-1 items-center gap-2 rounded-lg px-3 text-left text-[12.5px] focus-visible:ring-1 focus-visible:outline-none"
              >
                <span className={active ? "text-teal" : "text-faint"}>{view.kind === "chat" ? <MessageSquare size={14} /> : view.kind === "data" ? <Database size={14} /> : <Folder size={14} />}</span>
                <span className="truncate">{view.kind === "chat" ? chatLabel : view.kind === "data" ? "Data" : view.kind === "workspaces" ? "Workspaces" : view.label}</span>
              </button>
              {view.kind !== "chat" && (
                <button type="button" aria-label={`Close ${view.kind === "workspace" ? view.label : view.kind} tab`} onClick={() => onClose(view)} className={`text-faint hover:bg-app hover:text-body focus-visible:ring-teal mr-1.5 rounded p-1.5 focus-visible:ring-1 focus-visible:outline-none ${active ? "" : "opacity-0 focus:opacity-100 group-hover:opacity-100 max-md:opacity-100"}`}><Cross size={11} /></button>
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



function loadChatTextSize(): ChatTextSize {
  try {
    return resolveChatTextSize(localStorage.getItem(textSizeStorageKey));
  } catch {
    return defaultChatTextSize;
  }
}

function restoreFileFocus(file: HTMLButtonElement | null, activity: HTMLButtonElement | null) {
  // Completion can collapse/unmount the original action while inspection is
  // open. Keep keyboard navigation in chat even if that origin is now hidden.
  const target = [file, activity, document.querySelector<HTMLButtonElement>('button[aria-label="Toggle inspector"]')]
    .find((element) => element?.isConnected && element.getClientRects().length > 0);
  target?.focus();
}
