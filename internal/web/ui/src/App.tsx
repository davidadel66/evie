import { ModelPicker, ModelProblem } from "./chat/ModelSelector";
import { composerCommand } from "./chat/composerCommand";
import { useChatModels } from "./store/useChatModels";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  ContextScope,
  ContextSessionSelection,
  Project,
  Workspace,
  WorkspaceCreation,
  StoredSession,
} from "./api/contextSessions";
import { MemoryPresentationProvider } from "./memory/presentation";
import type { InspectorTarget } from "./artifacts/Panel";
import { InstructionBadge } from "./folder/Instructions";
import { InspectorPane } from "./artifacts/InspectorPane";
import { inspectToolFile } from "./artifacts/fileInspection";
import { selectedToolInspection, selectedMemoryTools, type ToolSelection } from "./artifacts/toolSelection";
import { Chat } from "./chat/Chat";
import { Composer } from "./chat/Composer";
import { DataHub, type DataSource } from "./data/DataHub";
import { usePendingMemoryProposals } from "./candidateInbox/pendingProposals";
import { Sidebar, type SidebarDestination } from "./shell/Sidebar";
import { useContextSessions } from "./store/useContextSessions";
import { useSession } from "./store/useSession";
import { Banner } from "./ui/Banner";
import { Folder, MessageSquare, PanelRight, Sidebar as SidebarIcon } from "./ui/Icon";
import {
  defaultChatTextSize,
  resolveChatTextSize,
  type ChatTextSize,
} from "./ui/textSize";
import { WorkspaceHome, Workspaces } from "./workspaces/Workspaces";
import type { WorkspaceTab, WorkspaceViewState } from "./workspaces/workspaceActivity";
import { Notifications } from "./shell/Notifications";
import { noticeDestination } from "./shell/notices";
import { buildsOfTool } from "./artifacts/chatBuilds";
import type { Notice } from "./api/notifications";
import { useWorkflowCaps, useWorkspaceNeeds } from "./store/useWorkspaceNeeds";
import { CreateWorkspaceDialog } from "./workspaces/CreateWorkspaceDialog";
import { SettingsDialog } from "./shell/SettingsDialog";
import { DeleteWorkspaceDialog } from "./shell/DeleteWorkspaceDialog";
import { resolveChatFont, type ChatFont } from "./ui/chatFont";
import { startsSubagents } from "./subagents/subagentView";
import { useChatStatus } from "./chats/useChatStatus";
import { chatMark, loadReadState, markRead } from "./chats/chatMarks";

export type WorkbenchView =
  | { id: "chat"; kind: "chat" }
  | { id: "data"; kind: "data" }
  | { id: "workspaces"; kind: "workspaces" }
  | { id: `workspace:${string}`; kind: "workspace"; workspaceId: string; label: string } & WorkspaceViewState;

const textSizeStorageKey = "evie.chatTextSize";
const sidebarStorageKey = "evie.sidebarCollapsed";
const chatView: WorkbenchView = { id: "chat", kind: "chat" };

export default function App() {
  const contextSessions = useContextSessions();
  const chatModels = useChatModels(contextSessions.snapshot?.activeSession?.id);
  const { items, status, queue, clearQueue, problem, send, answer, setAlwaysApplies, stop, stopping, compacting, notice, dismissProblem, historyLoading, historyProblem, hasOlder, loadOlder, retryHistory } = useSession(contextSessions.snapshot?.activeSession?.id, chatModels.catalog?.model);
  // A running compaction holds the conversation like a turn: the server
  // refuses session and model changes until it finishes.
  const working = status === "streaming" || compacting;
  // One main view at a time; the sidebar chooses it and the bar leads back to chat.
  const [activeView, setActiveView] = useState<WorkbenchView>(chatView);
  const [draft, setDraft] = useState("");
  // Bound to the session it opened in, so switching chats closes the picker.
  const [modelPickerSession, setModelPickerSession] = useState<string>();
  const activeSessionId = contextSessions.snapshot?.activeSession?.id;
  const modelPickerOpen = !!activeSessionId && modelPickerSession === activeSessionId;
  const closeModelPicker = useCallback(() => setModelPickerSession(undefined), []);
  // Sends the draft, or a command chosen from the "/" menu. `/model` opens the
  // picker here: it is a UI action, not a session request.
  const submit = (text: string) => {
    if (composerCommand(text)?.kind === "model") {
      setDraft("");
      setModelPickerSession(activeSessionId);
      return;
    }
    if (chatModels.saving || !chatModels.catalog) return;
    if (send(text)) setDraft("");
  };
  const [textSize, setTextSize] = useState<ChatTextSize>(loadChatTextSize);
  const [createWorkspaceOpen, setCreateWorkspaceOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [deletingWorkspace, setDeletingWorkspace] = useState<Workspace>();
  const [chatFont, setChatFont] = useState<ChatFont>(() => {
    try { return resolveChatFont(localStorage.getItem("evie.chatFont")); } catch { return "default"; }
  });
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try { return localStorage.getItem(sidebarStorageKey) === "true"; } catch { return false; }
  });
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [inspectionRequest, setInspectionRequest] = useState(0);
  const [filesRequest, setFilesRequest] = useState(0);
  const [inspectorFocused, setInspectorFocused] = useState(false);
  const [inspectorOverride, setInspectorOverride] = useState<InspectorTarget>();
  const [selectedAction, setSelectedAction] = useState<ToolSelection>();
  const fileTrigger = useRef<HTMLButtonElement | null>(null);
  const activityTrigger = useRef<HTMLButtonElement | null>(null);
  const [dataSource, setDataSource] = useState<DataSource>("memory");
  // Memories Evie proposed on her own wait for review; recount after each turn.
  const memoryReviewPending = usePendingMemoryProposals(status);
  // Runs waiting on the owner, by Workspace, for the sidebar and tab badges;
  // read again after each turn and whenever the owner acts on a run.
  const [runsChanged, setRunsChanged] = useState(0);
  const waiting = useWorkspaceNeeds(`${status}:${runsChanged}`);
  // Sidebar markers (multi-chat.spec.md Stage 4): live chat status, and what
  // this browser has read. Showing a chat while the page is visible marks it
  // read, again whenever its latest turn finishes or its activity moves.
  const chatStatuses = useChatStatus();
  const [readState, setReadState] = useState(() => loadReadState(Date.now()));
  const viewedId = activeView.kind === "chat" ? contextSessions.snapshot?.activeSession?.id : undefined;
  const viewedFinished = viewedId ? chatStatuses[viewedId]?.finishedAt : undefined;
  const viewedActivity = contextSessions.snapshot?.sessions.find((session) => session.id === viewedId)?.activityAt;
  useEffect(() => {
    if (!viewedId) return;
    const mark = () => { if (document.visibilityState === "visible") setReadState((state) => markRead(state, viewedId, Date.now())); };
    mark();
    document.addEventListener("visibilitychange", mark);
    return () => document.removeEventListener("visibilitychange", mark);
  }, [viewedId, viewedFinished, viewedActivity]);
  const chatMarks = useMemo(() => Object.fromEntries((contextSessions.snapshot?.sessions ?? []).map((session) => [session.id, chatMark(chatStatuses[session.id], session.activityAt, readState, session.id)])), [contextSessions.snapshot?.sessions, chatStatuses, readState]);

  const activeWorkspace = activeView.kind === "workspace"
    ? contextSessions.snapshot?.workspaces.find((workspace) => workspace.id === activeView.workspaceId)
    : undefined;
  const workflowCaps = useWorkflowCaps(activeWorkspace?.id);
  const sessionTitles = useMemo(() => Object.fromEntries((contextSessions.snapshot?.sessions ?? []).map((session) => [session.id, session.title.trim() || "Untitled session"])), [contextSessions.snapshot?.sessions]);
  const workspaceNames = useMemo(() => Object.fromEntries((contextSessions.snapshot?.workspaces ?? []).map((workspace) => [workspace.id, workspace.displayName])), [contextSessions.snapshot?.workspaces]);
  const scopeWorkspace=contextSessions.snapshot?.workspaces.find(workspace=>workspace.id===contextSessions.snapshot?.activeScope?.workspaceId);
  const openInstructions=(workspace:Workspace,trigger:HTMLButtonElement)=>{
    fileTrigger.current=trigger;activityTrigger.current=null;
    setSelectedAction(undefined);setInspectorOverride({kind:"repository-instructions",workspaceId:workspace.id,folderRevision:workspace.folder?.revision??0,settingsRevision:workspace.instructions?.revision??0});
    setInspectionRequest(value=>value+1);setInspectorOpen(true);setInspectorFocused(false);setMobileNavOpen(false);
  };
  const selectedTool = selectedToolInspection(selectedAction, contextSessions.snapshot?.activeSession?.id, items);
  // A build row opens its Change, not a file (changes.spec.md story 6b).
  const selectedBuilds = selectedTool ? buildsOfTool(selectedTool) : [];
  const selectedInspection = selectedTool && selectedBuilds.length === 0 ? inspectToolFile(selectedTool) : null;
  // A delegate row opens its subagents until it names a build.
  const selectedSubagents = selectedTool && selectedBuilds.length === 0 && activeSessionId && startsSubagents(selectedTool) ? selectedTool : undefined;
  const latestFileDiff = [...items].reverse().find((item) => item.kind === "tool" && item.name !== "propose_build_merge" && item.approval?.preview);
  const latestFile = latestFileDiff?.kind === "tool" ? inspectToolFile(latestFileDiff) : null;
  const visibleOverride = inspectorOverride?.kind==="repository-instructions"&&inspectorOverride.turnId&&inspectorOverride.sessionId!==contextSessions.snapshot?.activeSession?.id?undefined:inspectorOverride?.kind === "memory-evidence"
    ? inspectorOverride.sessionId === contextSessions.snapshot?.activeSession?.id
      ? {...inspectorOverride, memoryTools: selectedMemoryTools(inspectorOverride, contextSessions.snapshot?.activeSession?.id, items)}
      : undefined
    : inspectorOverride;
  const inspectorTarget: InspectorTarget = selectedTool && selectedBuilds.length > 0
    ? {kind: "build", key: selectedTool.key, workspaceId: contextSessions.snapshot?.activeScope?.workspaceId, builds: selectedBuilds}
    : selectedSubagents && activeSessionId
    ? {kind: "subagents", sessionId: activeSessionId, tool: selectedSubagents}
    : selectedInspection
    ? {kind: "file", file: selectedInspection}
    : selectedTool ? {kind: "tool", tool: selectedTool}
    : visibleOverride ?? defaultInspectorTarget(activeView, activeWorkspace, contextSessions.snapshot?.activeScope, latestFile ? {kind: "file", file: latestFile} : undefined);

  const closeInspector = () => {
    setInspectorOpen(false);
    setInspectorFocused(false);
    requestAnimationFrame(() => {
      restoreFileFocus(fileTrigger.current, activityTrigger.current);
    });
  };
  useEffect(() => {
    if (!inspectorOpen || (!selectedAction && inspectorOverride?.kind !== "memory-evidence" && inspectorOverride?.kind !== "workflow-run")) return;
    const onEscape = (event: KeyboardEvent) => {
      if (document.querySelector("dialog[open]")) return;
      if (event.key !== "Escape" || (event.target instanceof Element && event.target.closest("[data-terminal]"))) return;
      event.preventDefault();
      setInspectorOpen(false);
      setInspectorFocused(false);
      requestAnimationFrame(() => restoreFileFocus(fileTrigger.current, activityTrigger.current));
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [inspectorOpen, selectedAction, inspectorOverride]);

  const openTool = (key: string, trigger: HTMLButtonElement) => {
    const sessionId = contextSessions.snapshot?.activeSession?.id;
    if (!sessionId) return;
    fileTrigger.current = trigger;
    activityTrigger.current = trigger.closest('section[aria-label="Turn activity"]')?.querySelector<HTMLButtonElement>('button[aria-controls]') ?? null;
    setInspectionRequest(value => value + 1);
    setSelectedAction({sessionId, key});
    setInspectorOverride(undefined);
    setInspectorOpen(true);
    setInspectorFocused(false);
  };

  const pending = items.find((item) => item.kind === "tool" && item.approval?.state === "pending");
  const pendingId = pending?.kind === "tool" ? pending.approval?.reqId : undefined;

  useEffect(() => {
    if (!pendingId || activeView.kind !== "chat") return;
    const onKey = (event: KeyboardEvent) => {
      if (document.querySelector("dialog[open]")) return;
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
    try { localStorage.setItem("evie.chatFont", chatFont); } catch { /* In-memory preference still works. */ }
  }, [chatFont]);

  useEffect(() => {
    try { localStorage.setItem(sidebarStorageKey, String(sidebarCollapsed)); } catch { /* In-memory preference still works. */ }
  }, [sidebarCollapsed]);

  const closeNavigation = () => {
    setMobileNavOpen(false);
    if (window.matchMedia("(min-width: 768px)").matches) setSidebarCollapsed(true);
    requestAnimationFrame(() => document.querySelector<HTMLButtonElement>('button[aria-label="Open navigation"]')?.focus());
  };

  const openView = (view: WorkbenchView) => {
    setActiveView(view);
    setInspectorOverride(undefined);
    setSelectedAction(undefined);
    setMobileNavOpen(false);
  };

  const openWorkspace = (workspace: Workspace, tab?: WorkspaceTab, run?: string, change?: string) => {
    openView({ id: `workspace:${workspace.id}`, kind: "workspace", workspaceId: workspace.id, label: workspace.displayName, tab, run, change });
  };

  // A notice opens what it points to in its Workspace (noticeDestination).
  const openNotice = (notice: Notice) => {
    const workspace = contextSessions.snapshot?.workspaces.find((candidate) => candidate.id === notice.target.workspace_id);
    if (!workspace) return;
    const {tab, run, change} = noticeDestination(notice);
    openWorkspace(workspace, tab, run, change);
  };

  const selectSession = async (selection: ContextSessionSelection) => {
    try {
      await contextSessions.select(selection);
      setDraft("");
      setActiveView(chatView);
      setInspectorOverride(undefined);
      setSelectedAction(undefined);
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

  const registerWorkspace = async (options: WorkspaceCreation) => {
    await contextSessions.register(options);
    setCreateWorkspaceOpen(false);
  };

  const archiveSession = async (session: StoredSession) => {
    try {
      await contextSessions.setArchived(session.id, true);
      if (contextSessions.snapshot?.activeSession?.id === session.id) {
        setDraft("");
        setInspectorOverride(undefined);
        setSelectedAction(undefined);
        setInspectorOpen(false);
        setInspectorFocused(false);
      }
    } catch {
      // The session remains visible and the store displays the server error.
    }
  };

  // Deleting archives the Workspace; its open chat and page close with it.
  const deleteWorkspace = async (workspace: Workspace) => {
    const openInside = contextSessions.snapshot?.activeSession?.workspaceId === workspace.id;
    await contextSessions.setWorkspaceArchived(workspace.id, true);
    if (openInside) {
      setDraft("");
      setInspectorOverride(undefined);
      setSelectedAction(undefined);
      setInspectorOpen(false);
      setInspectorFocused(false);
    }
    setActiveView((view) => view.kind === "workspace" && view.workspaceId === workspace.id ? chatView : view);
  };

  const workspaceSessions = useMemo(
    () => activeWorkspace
      ? contextSessions.snapshot?.sessions.filter((session) => session.workspaceId === activeWorkspace.id) ?? []
      : [],
    [activeWorkspace, contextSessions.snapshot?.sessions],
  );

  return (
    <MemoryPresentationProvider snapshot={contextSessions.snapshot}><div data-chat-size={textSize} data-chat-font={chatFont} className="bg-app text-ink flex h-screen overflow-hidden text-[13px]">
      {mobileNavOpen && <button type="button" aria-label="Close navigation overlay" onClick={closeNavigation} className="absolute inset-0 z-30 bg-black/55 md:hidden" />}
      <Sidebar
        snapshot={contextSessions.snapshot}
        instructions={scopeWorkspace&&<InstructionBadge workspace={scopeWorkspace} refresh={status} onOpen={trigger=>openInstructions(scopeWorkspace,trigger)}/>}
        destination={sidebarDestination(activeView)}
        busy={contextSessions.busy}
        memoryReviewPending={memoryReviewPending}
        waiting={waiting}
        marks={chatMarks}
        mobileOpen={mobileNavOpen}
        collapsed={sidebarCollapsed}
        onSettings={() => setSettingsOpen(true)}
        onCreateWorkspace={() => setCreateWorkspaceOpen(true)}
        onArchive={session => void archiveSession(session)}
        onDeleteWorkspace={setDeletingWorkspace}
        onCloseMobile={closeNavigation}
        onData={() => openView({ id: "data", kind: "data" })}
        onWorkspace={(workspace) => openWorkspace(workspace)}
        onNewWorkspaceChat={(workspace) => void startWorkspaceChat(workspace)}
        onSession={(session) => {
          // Returning to the open chat from another view keeps its draft.
          if (session.id === contextSessions.snapshot?.activeSession?.id) openView(chatView);
          else void selectSession({ sessionId: session.id });
        }}
      />

      {/* The Inspector runs full height beside the bar and the main view. */}
      <div className="@container/workbench relative flex min-w-0 flex-1">
        {!inspectorFocused && (
          <div className="flex min-w-0 flex-1 flex-col">
            <WorkbenchBar
              view={activeView}
              chatLabel={contextSessions.snapshot?.activeSession?.title.trim() || "Chat"}
              inspectorOpen={inspectorOpen}
              navigationCollapsed={sidebarCollapsed}
              onOpenNavigation={() => {
                if (window.matchMedia("(min-width: 768px)").matches) setSidebarCollapsed(false);
                else setMobileNavOpen(true);
                requestAnimationFrame(() => document.querySelector<HTMLButtonElement>('button[aria-label="Close navigation"]')?.focus());
              }}
              onChat={() => openView(chatView)}
              notifications={<Notifications refreshKey={status} workspaceNames={workspaceNames} onOpen={openNotice} />}
              onToggleInspector={() => {
                setInspectorOpen((open) => !open);
                if (inspectorOpen) setInspectorFocused(false);
              }}
            />

            {contextSessions.problem && activeView.kind !== "workspaces" && <div role="alert" className="border-danger-hair bg-danger-bg text-danger-ink border-b px-5 py-3 text-sm">{contextSessions.problem}</div>}
            {problem && <Banner message={problem} onDismiss={dismissProblem} />}
            {activeView.kind === "chat" && (
              contextSessions.snapshot?.activeScope ? (
                <>
                  <Chat sessionId={contextSessions.snapshot.activeSession?.id} key={contextSessions.snapshot.activeSession?.id} items={items} queued={queue} streaming={status === "streaming"} onAnswer={answer} onAlwaysApplies={setAlwaysApplies} onOpenFile={openTool} onOpenTool={openTool}
                    workspaceId={scopeWorkspace?.id} onOpenRun={(runId, trigger) => {
                      fileTrigger.current = trigger;
                      activityTrigger.current = null;
                      setSelectedAction(undefined);
                      setInspectorOverride({kind: "workflow-run", runId});
                      setInspectionRequest(value => value + 1);
                      setInspectorOpen(true);
                      setInspectorFocused(false);
                    }} onOpenMemory={(snapshotId, trigger) => {
                    const sessionId = contextSessions.snapshot?.activeSession?.id;
                    if (!sessionId) return;
                    fileTrigger.current = trigger;
                    activityTrigger.current = trigger.closest('section[aria-label="Turn activity"]')?.querySelector<HTMLButtonElement>('button[aria-controls]') ?? null;
                    setSelectedAction(undefined);
                    setInspectorOverride({kind: "memory-evidence", sessionId, snapshotId});
                    setInspectionRequest(value => value + 1);
                    setInspectorOpen(true);
                    setInspectorFocused(false);
                  }} historyLoading={historyLoading} historyProblem={historyProblem} hasOlder={hasOlder} onOlder={loadOlder} onRetry={retryHistory} />
                  <Composer
                    value={draft}
                    onChange={setDraft}
                    streaming={status === "streaming"}
                    disabled={contextSessions.busy || historyLoading || !!historyProblem || chatModels.saving || !chatModels.catalog}
                    overlay={modelPickerOpen && <ModelPicker models={chatModels.catalog?.models ?? []} value={chatModels.catalog?.model} loading={chatModels.loading} disabled={contextSessions.busy || working || queue.length > 0 || chatModels.saving || !chatModels.catalog?.model} onChange={model => { void chatModels.select(model); }} onClose={closeModelPicker} />}
                    after={<><ModelProblem problem={chatModels.problem ?? (problem?.startsWith("Chat model changed") ? problem : undefined)} disabled={contextSessions.busy || working || queue.length > 0 || chatModels.loading || chatModels.saving} onRetry={() => {
                      const sessionId = contextSessions.snapshot?.activeSession?.id;
                      if (sessionId) void contextSessions.select({sessionId}).then(() => { chatModels.refresh(); dismissProblem(); }).catch(() => {});
                    }} />{status === "error" && queue.length > 0 && <button type="button" className="text-muted-text mt-2 text-xs underline" onClick={() => {
                      setDraft([...queue, draft].filter(Boolean).join("\n\n"));
                      clearQueue();
                    }}>Return queued messages to draft</button>}</>}
                    onSend={() => submit(draft)}
                    onCommand={submit}
                    onStop={() => {
                      // Stop means stop: queued messages return to the draft
                      // instead of firing once the turn ends.
                      if (queue.length > 0) {
                        setDraft([...queue, draft].filter(Boolean).join("\n\n"));
                        clearQueue();
                      }
                      stop();
                    }}
                    stopping={stopping}
                    notice={notice}
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
                  setSelectedAction(undefined);
                }}
                snapshot={contextSessions.snapshot}
              />
            )}
            {activeView.kind === "workspaces" && (
              <Workspaces
                snapshot={contextSessions.snapshot}
                busy={contextSessions.busy}
                problem={contextSessions.problem}
                onCreateWorkspace={() => setCreateWorkspaceOpen(true)}
                onOpenWorkspace={(workspace) => openWorkspace(workspace)}
                onNewWorkspaceChat={(workspace) => void startWorkspaceChat(workspace)}
                onNewProjectChat={(project) => void startProjectChat(project)}
                onResume={(session) => void selectSession({ sessionId: session.id })}
                onNewUnscopedChat={() => void selectSession({ unscoped: true })}
              />
            )}
            {activeView.kind === "workspace" && activeWorkspace && (
              <WorkspaceHome
                workspace={activeWorkspace}
                tab={activeView.tab ?? "runs"}
                selectedRun={activeView.run}
                runFilter={activeView.runFilter}
                selectedChange={activeView.change}
                onView={(patch) => setActiveView((view) => view.kind === "workspace" ? {...view, ...patch} : view)}
                sessionTitles={sessionTitles}
                onOpenSession={(sessionId) => void selectSession({ sessionId })}
                onRunsChanged={() => setRunsChanged((value) => value + 1)}
                waiting={waiting[activeWorkspace.id] ?? 0}
                caps={workflowCaps}
                instructions={<InstructionBadge workspace={activeWorkspace} onOpen={trigger=>openInstructions(activeWorkspace,trigger)}/>}
                onFolderChanged={contextSessions.refresh}
                onResearchChanged={contextSessions.refresh}
                onOpenFiles={() => {setFilesRequest(value => value + 1); setSelectedAction(undefined); setInspectorOverride(undefined); setInspectorOpen(true);}}
                sessions={workspaceSessions}
                busy={contextSessions.busy}
                onNewChat={() => void startWorkspaceChat(activeWorkspace)}
                onResume={(session) => void selectSession({ sessionId: session.id })}
              />
            )}
            {activeView.kind === "workspace" && !activeWorkspace && (
              <div className="text-muted-text flex flex-1 items-center justify-center">This workspace is no longer available.</div>
            )}
          </div>
        )}

        <InspectorPane
          items={items}
          target={selectedAction || visibleOverride ? inspectorTarget : undefined}
          requestVersion={inspectionRequest}
          filesRequest={filesRequest}
          workspace={activeWorkspace ?? contextSessions.snapshot?.workspaces.find(workspace => workspace.id === contextSessions.snapshot?.activeScope?.workspaceId)}
          sessionId={contextSessions.snapshot?.activeSession?.id}
          visible={inspectorOpen}
          focused={inspectorFocused}
          onToggleFocus={() => setInspectorFocused((focused) => !focused)}
          onClose={closeInspector}
          onOpenChange={(workspaceId, changeId) => {
            const workspace = contextSessions.snapshot?.workspaces.find((candidate) => candidate.id === workspaceId);
            if (workspace) openWorkspace(workspace, "changes", undefined, changeId);
          }}
          onManageFolder={() => {
            const workspace = activeWorkspace ?? contextSessions.snapshot?.workspaces.find(workspace => workspace.id === contextSessions.snapshot?.activeScope?.workspaceId);
            if (workspace) openWorkspace(workspace, "settings"); else openView({id: "workspaces", kind: "workspaces"});
            setInspectorOpen(false); setInspectorFocused(false);
          }}
        />
      </div>
      {createWorkspaceOpen && <CreateWorkspaceDialog onClose={() => setCreateWorkspaceOpen(false)} onCreate={registerWorkspace} />}
      {settingsOpen && <SettingsDialog snapshot={contextSessions.snapshot} problem={contextSessions.problem} onRefresh={contextSessions.refresh} busy={contextSessions.busy} textSize={textSize} onTextSize={setTextSize} font={chatFont} onFont={setChatFont} onClose={() => setSettingsOpen(false)} onRestore={session => contextSessions.setArchived(session.id, false)} onRestoreWorkspace={workspace => contextSessions.setWorkspaceArchived(workspace.id, false)} />}
      {deletingWorkspace && <DeleteWorkspaceDialog workspace={deletingWorkspace} onDelete={deleteWorkspace} onClose={() => setDeletingWorkspace(undefined)} />}
    </div></MemoryPresentationProvider>
  );
}

// The slim bar: where you are, and the one inspector toggle. Another view
// reads as a breadcrumb whose first step returns to the open chat.
export function WorkbenchBar({
  view,
  chatLabel,
  inspectorOpen,
  navigationCollapsed = false,
  onOpenNavigation,
  onChat,
  notifications,
  onToggleInspector,
}: {
  view: WorkbenchView;
  chatLabel: string;
  inspectorOpen: boolean;
  navigationCollapsed?: boolean;
  onOpenNavigation: () => void;
  onChat: () => void;
  /** The app-wide notifications icon, on every view. */
  notifications?: React.ReactNode;
  onToggleInspector: () => void;
}) {
  const label = view.kind === "data" ? "Data" : view.kind === "workspaces" ? "Workspaces" : view.kind === "workspace" ? view.label : undefined;
  return (
    <header className="border-hair bg-topbar flex h-11 flex-none items-center gap-1 border-b px-2">
      <button type="button" aria-label="Open navigation" aria-controls="evie-navigation" onClick={onOpenNavigation} className={`text-muted-text hover:bg-hover hover:text-body focus-visible:ring-teal flex-none rounded-lg p-2 focus-visible:ring-1 focus-visible:outline-none ${navigationCollapsed ? "" : "md:hidden"}`}><SidebarIcon size={16} /></button>
      <nav aria-label="Location" className="flex min-w-0 flex-1 items-center gap-1.5 px-2 text-[13px]">
        {label ? <>
          <button type="button" onClick={onChat} title={`Back to ${chatLabel}`} className="text-muted-text hover:text-body focus-visible:ring-teal min-w-0 max-w-[45%] truncate rounded focus-visible:ring-1 focus-visible:outline-none">{chatLabel}</button>
          <span aria-hidden="true" className="text-ghost">/</span>
          <span aria-current="page" className="text-ink min-w-0 truncate font-medium">{label}</span>
        </> : <span aria-current="page" className="text-ink min-w-0 truncate font-medium" title={chatLabel}>{chatLabel}</span>}
      </nav>
      {notifications}
      {/* The Inspector belongs to chat; other views open it only from their own controls. */}
      {view.kind === "chat" && <button type="button" title="Toggle inspector" aria-label="Toggle inspector" aria-pressed={inspectorOpen} onClick={onToggleInspector} className={`${inspectorOpen ? "text-teal" : "text-faint hover:text-body"} focus-visible:ring-teal flex-none rounded p-2 focus-visible:ring-1 focus-visible:outline-none`}><PanelRight size={15} /></button>}
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
