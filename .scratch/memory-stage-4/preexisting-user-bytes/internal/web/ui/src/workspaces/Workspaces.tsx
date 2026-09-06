import { useState } from "react";
import type {
  ContextSessionSnapshot,
  Project,
  StoredSession,
  Workspace,
} from "../api/contextSessions";
import { Folder, MessageSquare, Plus } from "../ui/Icon";

type DirectoryProps = {
  snapshot?: ContextSessionSnapshot;
  busy: boolean;
  problem: string | null;
  onRegister: (name: string) => void;
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
  onRegister,
  onOpenWorkspace,
  onNewWorkspaceChat,
  onNewProjectChat,
  onResume,
  onNewUnscopedChat,
}: DirectoryProps) {
  const [name, setName] = useState("");
  return (
    <main className="min-h-0 flex-1 overflow-y-auto px-6 py-8 sm:px-10">
      <div className="mx-auto max-w-[920px]">
        <header className="max-w-[620px]">
          <h1 className="text-ink text-[22px] font-semibold tracking-[-0.02em]">Workspaces</h1>
          <p className="text-muted-text mt-2 text-[13px] leading-6">
            Durable places for the parts of your life and work that need their own memory, access, and conversations.
          </p>
        </header>

        <form
          className="border-hair mt-7 flex max-w-[620px] gap-2 border-b pb-7"
          onSubmit={(event) => {
            event.preventDefault();
            const value = name.trim();
            if (!value) return;
            onRegister(value);
            setName("");
          }}
        >
          <input
            aria-label="Workspace name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Name a new workspace"
            className="border-hair-input bg-card text-ink placeholder:text-fainter focus:border-teal-hair min-w-0 flex-1 rounded-[8px] border px-3 py-2 focus:outline-none"
          />
          <button
            type="submit"
            disabled={busy || !name.trim()}
            className="bg-teal text-primary-foreground focus-visible:ring-teal rounded-[8px] px-4 py-2 text-xs font-semibold disabled:opacity-40 focus-visible:ring-2 focus-visible:outline-none"
          >
            Create workspace
          </button>
        </form>

        {problem && <p role="alert" className="text-danger mt-4 text-xs">{problem}</p>}
        {!snapshot && <p className="text-faint mt-8">Loading workspaces…</p>}

        <section aria-label="Workspaces" className="mt-7">
          {snapshot?.workspaces.length === 0 && (
            <div className="border-hair text-muted-text border-y py-8 text-sm">
              Create a workspace for an ongoing area such as Cairo&apos;s Kitchen or a research project.
            </div>
          )}
          {snapshot?.workspaces.map((workspace) => {
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

export function WorkspaceHome({
  workspace,
  sessions,
  busy,
  onNewChat,
  onResume,
}: {
  workspace: Workspace;
  sessions: StoredSession[];
  busy: boolean;
  onNewChat: () => void;
  onResume: (session: StoredSession) => void;
}) {
  return (
    <main className="min-h-0 flex-1 overflow-y-auto px-6 py-8 sm:px-10">
      <div className="mx-auto max-w-[880px]">
        <header className="border-hair flex flex-col gap-5 border-b pb-7 sm:flex-row sm:items-end">
          <div className="min-w-0 flex-1">
            <div className="text-teal mb-3 flex items-center gap-2 text-xs"><Folder size={14} /> Workspace</div>
            <h1 className="text-ink truncate text-[24px] font-semibold tracking-[-0.025em]">{workspace.displayName}</h1>
            <p className="text-muted-text mt-2 max-w-[600px] text-[13px] leading-6">A durable context with isolated memory, reviewed configuration, and its own conversations.</p>
          </div>
          {workspace.state === "active" && (
            <button type="button" disabled={busy} onClick={onNewChat} className="bg-teal text-primary-foreground flex items-center justify-center gap-2 rounded-[8px] px-4 py-2 text-xs font-semibold disabled:opacity-40">
              <Plus size={13} /> New chat
            </button>
          )}
        </header>

        <section className="mt-8">
          <div className="mb-4 flex items-baseline justify-between">
            <h2 className="text-body text-sm font-semibold">Conversations</h2>
            <span className="text-fainter text-[11px]">{sessions.length} active</span>
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
      </div>
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
