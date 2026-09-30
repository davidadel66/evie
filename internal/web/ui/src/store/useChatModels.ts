import { useCallback, useEffect, useRef, useState } from "react";
import { listModels, selectModel, type ModelCatalog } from "../api/models";

export function useChatModels(sessionId?: string) {
  const [state, setState] = useState<{sessionId: string; catalog: ModelCatalog}>();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [problem, setProblem] = useState<string>();
  const [refreshKey, setRefreshKey] = useState(0);
  const generation = useRef({ active: false });
  const changing = useRef(false);
  const catalog = state && state.sessionId === sessionId ? state.catalog : undefined;

  useEffect(() => {
    const current = { active: true };
    generation.current = current;
    const abort = new AbortController();
    setProblem(undefined);
    setSaving(false);
    changing.current = false;
    if (!sessionId) { setLoading(false); return; }
    setLoading(true);
    listModels(sessionId, abort.signal).then(value => {
      if (!current.active) return;
      setState({sessionId, catalog:value});
      setProblem(value.problem);
    }).catch((error: unknown) => {
      if (current.active && !abort.signal.aborted) setProblem(describe(error));
    }).finally(() => {
      if (current.active) setLoading(false);
    });
    return () => { current.active = false; abort.abort(); };
  }, [sessionId, refreshKey]);

  const select = useCallback(async (model: string) => {
    if (!sessionId || !catalog || loading || changing.current || model === catalog.model) return;
    const current = generation.current;
    changing.current = true;
    setSaving(true);
    setProblem(undefined);
    try {
      const selection = await selectModel(sessionId, model, catalog.revision);
      if (current.active) setState({sessionId, catalog:{...catalog, ...selection}});
    } catch (error) {
      if (current.active) setProblem(describe(error));
    } finally {
      if (current.active) { changing.current = false; setSaving(false); }
    }
  }, [sessionId, catalog, loading]);

  return { catalog, loading, saving, problem, select, refresh: () => setRefreshKey(key => key + 1) };
}

function describe(error: unknown): string {
  return error instanceof Error ? error.message : "Models are unavailable. Try again.";
}
