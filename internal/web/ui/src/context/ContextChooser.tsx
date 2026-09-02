import { useState } from "react";
import type {
  ContextSessionSelection,
  ContextSessionSnapshot,
  StoredSession,
} from "../api/contextSessions";

type Props = {
  snapshot?: ContextSessionSnapshot;
  busy: boolean;
  problem: string | null;
  onRegister: (name: string) => void;
  onSelect: (selection: ContextSessionSelection) => void;
};

export function ContextChooser({
  snapshot,
  busy,
  problem,
  onRegister,
  onSelect,
}: Props) {
  const [name, setName] = useState("");
  if (!snapshot) {
    return (
      <div className="border-hair border-b px-5 py-3 text-xs text-faint">
        Loading Context Scopes…
      </div>
    );
  }
  const choices = (
    <div className="grid gap-4 p-4 md:grid-cols-3">
      <section>
        <h2 className="mb-2 font-semibold">Workspaces</h2>
        {snapshot.workspaces.map((workspace) => (
          <div className="border-hair mb-2 rounded border p-3" key={workspace.id}>
            <div className="font-medium">
              {workspace.displayName}
              {workspace.state === "archived" ? " (archived)" : ""}
            </div>
            {workspace.state === "active" && (
              <button
                type="button"
                disabled={busy}
                onClick={() =>
                  onSelect({
                    workspaceId: workspace.id,
                    workspaceRevision: workspace.currentRevisionId,
                  })
                }
              >
                New session
              </button>
            )}
            <SessionButtons
              sessions={snapshot.sessions.filter(
                (session) => session.workspaceId === workspace.id,
              )}
              busy={busy}
              onSelect={onSelect}
            />
          </div>
        ))}
        <form
          onSubmit={(event) => {
            event.preventDefault();
            const value = name.trim();
            if (value) onRegister(value);
          }}
        >
          <label className="block text-xs" htmlFor="workspace-name">
            Register Workspace
          </label>
          <div className="flex gap-2">
            <input
              id="workspace-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              className="border-hair min-w-0 flex-1 rounded border px-2 py-1"
            />
            <button type="submit" disabled={busy || !name.trim()}>
              Register
            </button>
          </div>
        </form>
      </section>
      <section>
        <h2 className="mb-2 font-semibold">Projects</h2>
        {snapshot.projects.map((project) => (
          <div key={project.id} className="mb-2">
            <div>
              {project.displayName}
              {project.archived ? " (archived)" : ""}
            </div>
            {!project.archived && (
              <button
                type="button"
                disabled={busy}
                onClick={() => onSelect({ projectId: project.id })}
              >
                New session
              </button>
            )}
            <SessionButtons
              sessions={snapshot.sessions.filter(
                (session) => session.projectId === project.id,
              )}
              busy={busy}
              onSelect={onSelect}
            />
          </div>
        ))}
      </section>
      <section>
        <h2 className="mb-2 font-semibold">Unscoped</h2>
        <button
          type="button"
          disabled={busy}
          onClick={() => onSelect({ unscoped: true })}
        >
          New unscoped session
        </button>
        <SessionButtons
          sessions={snapshot.sessions.filter(
            (session) => !session.workspaceId && !session.projectId,
          )}
          busy={busy}
          onSelect={onSelect}
        />
      </section>
    </div>
  );
  return (
    <div className="border-hair border-b">
      {snapshot.activeScope ? (
        <details>
          <summary className="cursor-pointer px-5 py-2 text-xs">
            Context Scope: {scopeKind(snapshot.activeScope.kind)} —{" "}
            {snapshot.activeScope.displayName} ·{" "}
            <span className="text-faint">Switch session</span>
          </summary>
          {choices}
        </details>
      ) : (
        <>
          <h1 className="px-5 pt-4 font-semibold">Choose a Context Scope</h1>
          {choices}
        </>
      )}
      {problem && <p className="px-5 pb-3 text-xs text-red-600">{problem}</p>}
    </div>
  );
}

function SessionButtons({
  sessions,
  busy,
  onSelect,
}: {
  sessions: StoredSession[];
  busy: boolean;
  onSelect: Props["onSelect"];
}) {
  return (
    <>
      {sessions.map((session) => (
        <button
          className="block"
          type="button"
          disabled={busy}
          key={session.id}
          onClick={() => onSelect({ sessionId: session.id })}
        >
          {session.title.trim() || "Untitled session"}
        </button>
      ))}
    </>
  );
}

function scopeKind(kind: string): string {
  if (kind === "workspace") return "Workspace";
  if (kind === "project") return "Project";
  return "Unscoped";
}
