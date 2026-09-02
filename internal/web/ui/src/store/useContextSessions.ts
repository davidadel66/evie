import { useCallback, useEffect, useState } from "react";
import {
  listContextSessions,
  registerWorkspace,
  selectContextSession,
  type ContextSessionSelection,
  type ContextSessionSnapshot,
  type OpenedContextSession,
} from "../api/contextSessions";

export function useContextSessions() {
  const [snapshot, setSnapshot] = useState<ContextSessionSnapshot>();
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setSnapshot(await listContextSessions());
  }, []);
  useEffect(() => {
    refresh().catch((error: unknown) => setProblem(describe(error)));
  }, [refresh]);

  const runSelection = useCallback(
    async (operation: () => Promise<OpenedContextSession>) => {
      setBusy(true);
      setProblem(null);
      try {
        const opened = await operation();
        setSnapshot((current) => applyOpenedSession(current, opened));
        await refresh().catch((error: unknown) => setProblem(describe(error)));
        return opened;
      } catch (error) {
        setProblem(describe(error));
        throw error;
      } finally {
        setBusy(false);
      }
    },
    [refresh],
  );

  const select = useCallback(
    (selection: ContextSessionSelection) =>
      runSelection(() => selectContextSession(selection)),
    [runSelection],
  );

  const register = useCallback(
    (name: string) =>
      runSelection(async () => {
        const workspace = await registerWorkspace(name);
        return selectContextSession({
          workspaceId: workspace.id,
          workspaceRevision: workspace.currentRevisionId,
        });
      }),
    [runSelection],
  );

  return { snapshot, busy, problem, select, register };
}

export function applyOpenedSession(
  snapshot: ContextSessionSnapshot | undefined,
  opened: OpenedContextSession,
): ContextSessionSnapshot {
  return {
    workspaces: snapshot?.workspaces ?? [],
    projects: snapshot?.projects ?? [],
    sessions: snapshot?.sessions ?? [],
    activeSession: opened.session,
    activeScope: opened.scope,
  };
}

function describe(error: unknown): string {
  return error instanceof Error ? error.message : "Context Scope operation failed";
}
