import { useCallback, useEffect, useRef, useState } from "react";
import {
  listContextSessions,
  registerWorkspace,
  selectContextSession,
  archiveSession,
  restoreSession,
  type StoredSession,
  type WorkspaceCreation,
  type ContextSessionSelection,
  type ContextSessionSnapshot,
  type OpenedContextSession,
} from "../api/contextSessions";

export function useContextSessions() {
  const [snapshot, setSnapshot] = useState<ContextSessionSnapshot>();
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const snapshotGeneration = useRef(0);

  const refresh = useCallback(async () => {
    const generation = ++snapshotGeneration.current;
    try {
      const next = await listContextSessions();
      if (generation === snapshotGeneration.current) {
        setSnapshot(next);
        setProblem(null);
      }
    } catch (error) {
      if (generation !== snapshotGeneration.current) return;
      setProblem(describe(error));
      throw error;
    }
  }, []);
  useEffect(() => {
    refresh().catch((error: unknown) => setProblem(describe(error)));
  }, [refresh]);

  const runSelection = useCallback(
    async (operation: () => Promise<OpenedContextSession>) => {
      ++snapshotGeneration.current;
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
    async (options: WorkspaceCreation) => {
      ++snapshotGeneration.current;
      setBusy(true);
      setProblem(null);
      try {
        const workspace = await registerWorkspace(options);
        setSnapshot((current) => ({
          ...current,
          projects: current?.projects ?? [],
          sessions: current?.sessions ?? [],
          workspaces: [...(current?.workspaces ?? []), workspace],
        }));
        // Registration has committed. A failed refresh must not invite a
        // second create request for the same workspace.
        await refresh().catch((error: unknown) => setProblem(describe(error)));
        return workspace;
      } catch (error) {
        setProblem(describe(error));
        throw error;
      } finally {
        setBusy(false);
      }
    },
    [refresh],
  );

  const setArchived = useCallback(async (sessionId: string, archived: boolean) => {
    ++snapshotGeneration.current;
    setBusy(true);
    setProblem(null);
    try {
      const result = await (archived ? archiveSession(sessionId) : restoreSession(sessionId));
      setSnapshot(current => current ? applySessionArchive(current, result.session) : current);
      await refresh().catch((error: unknown) => setProblem(describe(error)));
    } catch (error) {
      setProblem(describe(error));
      throw error;
    } finally {
      setBusy(false);
    }
  }, [refresh]);

  return { snapshot, busy, problem, select, register, refresh, setArchived };
}

export function applySessionArchive(snapshot: ContextSessionSnapshot, session: StoredSession): ContextSessionSnapshot {
  const archived = session.status === "closed";
  const clearSelection = archived && snapshot.activeSession?.id === session.id;
  return {
    ...snapshot,
    sessions: [...snapshot.sessions.filter(entry => entry.id !== session.id), ...(archived ? [] : [session])],
    archivedSessions: [...(snapshot.archivedSessions ?? []).filter(entry => entry.id !== session.id), ...(archived ? [session] : [])],
    activeSession: clearSelection ? undefined : snapshot.activeSession,
    activeScope: clearSelection ? undefined : snapshot.activeScope,
  };
}

export function applyOpenedSession(
  snapshot: ContextSessionSnapshot | undefined,
  opened: OpenedContextSession,
): ContextSessionSnapshot {
  return {
    ownerDisplayName: snapshot?.ownerDisplayName,
    workspaces: snapshot?.workspaces ?? [],
    projects: snapshot?.projects ?? [],
    sessions: snapshot?.sessions ?? [],
    archivedSessions: snapshot?.archivedSessions ?? [],
    activeSession: opened.session,
    activeScope: opened.scope,
  };
}

function describe(error: unknown): string {
  return error instanceof Error ? error.message : "Context Scope operation failed";
}
