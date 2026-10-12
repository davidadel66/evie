import type {
  ContextSessionSnapshot,
  Project,
  StoredSession,
  Workspace,
} from "../api/contextSessions";
import { Folder, MessageSquare, Plus } from "../ui/Icon";
import { FolderSettings } from "../folder/FolderSettings";
import { WorkspaceResearchSettings } from "./WorkspaceResearchSettings";
import { WorkspaceExploreSettings } from "./WorkspaceExploreSettings";
import { WorkspaceBuildSettings } from "./WorkspaceBuildSettings";
import { WorkspaceVerifySettings } from "./WorkspaceVerifySettings";
import type { WorkflowCapStatus } from "../api/workflows";
import { WorkspaceWorkflowsTab } from "./WorkspaceWorkflowsTab";
import { WorkspaceRuns } from "./WorkspaceRuns";
import { WorkspaceHeader } from "./WorkspaceHeader";
import { WorkspaceRepository } from "./WorkspaceRepository";
import { WorkspaceChanges } from "./WorkspaceChanges";
import { useChangeListing } from "./useChangeListing";
import { reviewCount } from "./changeView";
import type { WorkspaceTab, WorkspaceViewState } from "./workspaceActivity";

type DirectoryProps = {
  snapshot?: ContextSessionSnapshot;
  busy: boolean;
  problem: string | null;
  onCreateWorkspace: () => void;
  onOpenWorkspace: (workspace: Workspace) => void;
  onNewWorkspaceChat: (workspace: Workspace) => void;
  onNewProjectChat: (project: Project) => void;
  onResume: (session: StoredSession) => void;
  onNewUnscopedChat: () => void;
};

export function Workspaces({
  snapshot,
  busy,
  problem,
  onCreateWorkspace,
  onOpenWorkspace,
  onNewWorkspaceChat,
  onNewProjectChat,
  onResume,
  onNewUnscopedChat,
}: DirectoryProps) {
  // Archived ("deleted") Workspaces are restored from Settings.
  const activeWorkspaces = snapshot?.workspaces.filter((workspace) => workspace.state === "active") ?? [];
  return (
    <main className="min-h-0 flex-1 overflow-y-auto px-6 py-8 sm:px-10">
      <div className="mx-auto max-w-[920px]">
        <header className="max-w-[620px]">
          <h1 className="text-ink text-[22px] font-semibold tracking-[-0.02em]">Workspaces</h1>
          <p className="text-muted-text mt-2 text-[13px] leading-6">
            Durable places for the parts of your life and work that need their own memory, access, and conversations.
          </p>
        </header>

        <div className="border-hair mt-7 flex max-w-[620px] border-b pb-7">
          <button
            type="button"
            onClick={onCreateWorkspace}
            disabled={busy}
            className="bg-teal text-primary-foreground focus-visible:ring-teal flex items-center gap-2 rounded-[8px] px-4 py-2 text-xs font-semibold disabled:opacity-40 focus-visible:ring-2 focus-visible:outline-none"
          >
            <Plus size={13} />
            Create workspace
          </button>
        </div>

        {problem && <p role="alert" className="text-danger mt-4 text-xs">{problem}</p>}
        {!snapshot && <p className="text-faint mt-8">Loading workspaces…</p>}

        <section aria-label="Workspaces" className="mt-7">
          {snapshot && activeWorkspaces.length === 0 && (
            <div className="border-hair text-muted-text border-y py-8 text-sm">
              Create a workspace for an ongoing area such as Cairo&apos;s Kitchen or a research project.
            </div>
          )}
          {snapshot && activeWorkspaces.map((workspace) => {
            const sessions = snapshot.sessions.filter((session) => session.workspaceId === workspace.id);
            return (
              <article key={workspace.id} className="border-hair flex flex-col gap-4 border-b py-5 sm:flex-row sm:items-center">
                <button type="button" onClick={() => onOpenWorkspace(workspace)} className="group flex min-w-0 flex-1 items-center gap-3 text-left">
                  <span className="border-teal-hair bg-teal-deep text-teal-hover flex h-9 w-9 items-center justify-center rounded-[9px] border">
                    <Folder size={16} />
                  </span>
                  <span className="min-w-0">
                    <span className="text-ink group-hover:text-teal-hover block truncate text-sm font-medium">{workspace.displayName}</span>
                    <span className="text-fainter mt-1 block text-[11px]">{sessions.length} active {sessions.length === 1 ? "conversation" : "conversations"} · revision {shortId(workspace.currentRevisionId)}</span>
                  </span>
                </button>
                <div className="flex gap-2 pl-12 sm:pl-0">
                  <button type="button" onClick={() => onOpenWorkspace(workspace)} className="border-hair-strong text-muted-text hover:text-body rounded-[7px] border px-3 py-[7px] text-xs">Open</button>
                  {workspace.state === "active" && (
                    <button type="button" disabled={busy} onClick={() => onNewWorkspaceChat(workspace)} className="border-teal-hair text-teal-hover hover:bg-teal-deep rounded-[7px] border px-3 py-[7px] text-xs disabled:opacity-40">New chat</button>
                  )}
                </div>
              </article>
            );
          })}
        </section>

        {snapshot && (snapshot.projects.length > 0 || snapshot.sessions.some((session) => !session.workspaceId && !session.projectId)) && (
          <section className="mt-10">
            <h2 className="text-body text-sm font-semibold">Other contexts</h2>
            <p className="text-fainter mt-1 text-xs">Filesystem projects and unscoped conversations remain separate from Workspaces.</p>
            <div className="border-hair mt-4 border-t">
              {snapshot.projects.map((project) => (
                <ContextRow
                  key={project.id}
                  name={project.displayName}
                  detail={project.canonicalRoot}
                  disabled={busy || project.archived}
                  onNew={() => onNewProjectChat(project)}
                />
              ))}
              <ContextRow name="Unscoped" detail="Global and session memory only" disabled={busy} onNew={onNewUnscopedChat} />
            </div>
          </section>
        )}

        {snapshot && snapshot.sessions.length > 0 && (
          <section className="mt-10 pb-10">
            <h2 className="text-body text-sm font-semibold">Recent conversations</h2>
            <div className="border-hair mt-4 border-t">
              {snapshot.sessions.slice(0, 8).map((session) => (
                <button
                  type="button"
                  disabled={busy}
                  key={session.id}
                  onClick={() => onResume(session)}
                  className="border-hair text-muted-text hover:text-ink flex w-full items-center gap-3 border-b px-1 py-3 text-left disabled:opacity-40"
                >
                  <MessageSquare size={14} />
                  <span className="min-w-0 flex-1 truncate text-xs">{session.title.trim() || "Untitled session"}</span>
                  <span className="text-ghost text-[10px]">{sessionScope(session, snapshot)}</span>
                </button>
              ))}
            </div>
          </section>
        )}
      </div>
    </main>
  );
}

// A Workspace opens on its header and one tab at a time
// (workspace-activity.spec.md): Runs by default, Changes when it has a
// Repository (changes.spec.md story 6a), Workflows, and Settings, which
// holds the Workspace's settings sections and its conversations.
export function WorkspaceHome({
  workspace,
  sessions,
  busy,
  tab = "runs",
  waiting = 0,
  caps,
  selectedRun,
  runFilter = "",
  selectedChange,
  sessionTitles,
  onView,
  onOpenSession,
  onRunsChanged,
  onNewChat,
  onResume,
  onFolderChanged,
  onResearchChanged,
  onOpenFiles,
  instructions,
}: {
  workspace: Workspace;
  sessions: StoredSession[];
  busy: boolean;
  tab?: WorkspaceTab;
  /** Runs in this Workspace waiting on the owner now. */
  waiting?: number;
  caps?: WorkflowCapStatus;
  /** The run open in the Runs tab's panel. */
  selectedRun?: string;
  /** The workflow the Runs tab is narrowed to; "" for all. */
  runFilter?: string;
  /** The Change open on the Changes tab's review page. */
  selectedChange?: string;
  sessionTitles?: Record<string, string>;
  /** Moves within the Workspace: a tab, the open run, the run filter, together. */
  onView?: (view: WorkspaceViewState) => void;
  onOpenSession?: (sessionId: string) => void;
  onRunsChanged?: () => void;
  onNewChat: () => void;
  onResume: (session: StoredSession) => void;
  onFolderChanged?: () => Promise<void>;
  onResearchChanged?: () => Promise<void>;
  onOpenFiles?: () => void;
  instructions?: React.ReactNode;
}) {
  const {listing, reload} = useChangeListing(workspace.id);
  const hasRepository = !!listing && listing.repository.kind !== "none";
  return (
    <main className="flex min-h-0 flex-1 flex-col">
      <WorkspaceHeader tab={tab} waiting={waiting} caps={caps} showChanges={hasRepository || tab === "changes"}
        review={listing ? reviewCount(listing.changes, listing.repository.kind, new Date()) : 0} onTab={(next) => onView?.({tab: next})} />
      {tab === "changes" ? <div id="workspace-tab-panel" role="tabpanel" aria-labelledby="workspace-tab-changes" className="flex min-h-0 flex-1">
        <WorkspaceChanges key={`changes:${workspace.id}`} workspace={workspace} listing={listing} selected={selectedChange} sessionTitles={sessionTitles}
          onSelect={(change) => onView?.({change})} onChanged={reload}
          onOpenRun={(run) => onView?.({tab: "runs", run, runFilter: ""})} onOpenSession={onOpenSession} />
      </div> : tab === "runs" ? <div id="workspace-tab-panel" role="tabpanel" aria-labelledby="workspace-tab-runs" className="flex min-h-0 flex-1">
        <WorkspaceRuns key={`workflow-runs:${workspace.id}`} workspace={workspace} selectedId={selectedRun} filter={runFilter} sessionTitles={sessionTitles}
          onSelect={(run) => onView?.({run})} onFilter={(filter) => onView?.({runFilter: filter})} onOpenSession={onOpenSession} onChanged={onRunsChanged} />
      </div> : tab === "workflows" ? <div id="workspace-tab-panel" role="tabpanel" aria-labelledby="workspace-tab-workflows" className="flex min-h-0 flex-1">
        <WorkspaceWorkflowsTab key={`workflows:${workspace.id}`} workspace={workspace}
          onOpenRun={(run) => onView?.({tab: "runs", run, runFilter: ""})} onAllRuns={(workflow) => onView?.({tab: "runs", run: undefined, runFilter: workflow})} />
      </div> : <div id="workspace-tab-panel" role="tabpanel" aria-labelledby={`workspace-tab-${tab}`} className="min-h-0 flex-1 overflow-y-auto px-6 pb-10 sm:px-10">
      <div className="mx-auto max-w-[880px]">
        <>
        <WorkspaceRepository key={`repository:${workspace.id}`} workspace={workspace} />
        {instructions}
        <WorkspaceResearchSettings key={workspace.id} workspace={workspace} onChanged={onResearchChanged} />
        <WorkspaceExploreSettings key={`explore:${workspace.id}`} workspace={workspace} onChanged={onResearchChanged} />
        <WorkspaceBuildSettings key={`build:${workspace.id}`} workspace={workspace} onChanged={onResearchChanged} />
        <WorkspaceVerifySettings key={`verify:${workspace.id}`} workspace={workspace} onChanged={onResearchChanged} />
        <FolderSettings key={`${workspace.id}:${workspace.folder?.revision ?? 0}`} workspace={workspace} busy={busy} onChanged={onFolderChanged} onOpen={onOpenFiles} />
        <section className="mt-8">
          <div className="mb-4 flex items-center gap-3">
            <h2 className="text-body text-sm font-semibold">Conversations</h2>
            <span className="text-fainter flex-1 text-[11px]">{sessions.length} active</span>
            {workspace.state === "active" && (
              <button type="button" disabled={busy} onClick={onNewChat} className="border-teal-hair text-teal-hover hover:bg-teal-deep flex items-center gap-1.5 rounded-[7px] border px-3 py-[6px] text-xs disabled:opacity-40">
                <Plus size={12} /> New chat
              </button>
            )}
          </div>
          {sessions.length === 0 ? (
            <button type="button" disabled={busy || workspace.state !== "active"} onClick={onNewChat} className="border-hair text-muted-text hover:border-teal-hair hover:text-body w-full rounded-[10px] border border-dashed px-5 py-10 text-center text-sm disabled:opacity-40">
              Start the first conversation in this workspace
            </button>
          ) : (
            <div className="border-hair border-t">
              {sessions.map((session) => (
                <button type="button" disabled={busy} key={session.id} onClick={() => onResume(session)} className="border-hair hover:bg-hover flex w-full items-center gap-3 border-b px-2 py-4 text-left disabled:opacity-40">
                  <MessageSquare size={14} className="text-faint" />
                  <span className="text-body min-w-0 flex-1 truncate text-[13px]">{session.title.trim() || "Untitled session"}</span>
                  <span className="text-fainter text-[10.5px]">Open chat</span>
                </button>
              ))}
            </div>
          )}
        </section>
        </>
      </div>
      </div>}
    </main>
  );
}

function ContextRow({ name, detail, disabled, onNew }: { name: string; detail: string; disabled: boolean; onNew: () => void }) {
  return (
    <div className="border-hair flex items-center gap-3 border-b px-1 py-3">
      <div className="min-w-0 flex-1">
        <div className="text-body text-xs font-medium">{name}</div>
        <div className="text-ghost mt-1 truncate font-mono text-[10px]">{detail}</div>
      </div>
      <button type="button" disabled={disabled} onClick={onNew} className="text-muted-text hover:text-teal-hover px-2 py-1 text-xs disabled:opacity-40">New chat</button>
    </div>
  );
}

function shortId(value: string) {
  return value.length > 12 ? value.slice(0, 12) : value;
}

function sessionScope(session: StoredSession, snapshot: ContextSessionSnapshot) {
  if (session.workspaceId) return snapshot.workspaces.find((workspace) => workspace.id === session.workspaceId)?.displayName ?? "Workspace";
  if (session.projectId) return snapshot.projects.find((project) => project.id === session.projectId)?.displayName ?? "Project";
  return "Unscoped";
}
